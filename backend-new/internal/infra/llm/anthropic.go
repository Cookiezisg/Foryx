package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"iter"
	"net/http"
	"strings"
)

const (
	anthropicVersion          = "2023-06-01"
	anthropicMessagesPath     = "/v1/messages"
	anthropicDefaultMaxTokens = 8096
	anthropicDefaultBaseURL   = "https://api.anthropic.com"
)

// anthropicProvider speaks Anthropic's native /v1/messages dialect, fully self-contained:
// block-form messages, x-api-key auth, cache_control breakpoints, named-event SSE, and
// thinking-block + signature round-trip. Nothing here is shared with the OpenAI-compat
// providers — its wire is genuinely different and evolves on its own.
//
// anthropicProvider 完整自包含地讲 Anthropic 原生 /v1/messages 方言：block 形式 messages、
// x-api-key 鉴权、cache_control 断点、命名事件 SSE、thinking block + signature 回传。与
// OpenAI-compat 各家不共享任何东西——它的 wire 确实不同、自行演化。
type anthropicProvider struct{}

func newAnthropicProvider() *anthropicProvider { return &anthropicProvider{} }

func (p *anthropicProvider) Name() string           { return "anthropic" }
func (p *anthropicProvider) DefaultBaseURL() string { return anthropicDefaultBaseURL }

// BuildRequest encodes a Request into an Anthropic /v1/messages HTTP request. Auth: x-api-key.
// Two orthogonal native knobs from Options: thinking ("adaptive"/"enabled"/"disabled") and effort
// (output_config.effort). 1M context is GA on current models — no beta header.
//
// BuildRequest 把 Request 编码为 Anthropic /v1/messages 请求。Auth：x-api-key。两个正交原生旋钮
// 取自 Options：thinking（adaptive/enabled/disabled）与 effort（output_config.effort）。当前模型 1M 已 GA，无需 beta header。
func (p *anthropicProvider) BuildRequest(ctx context.Context, req Request) (*http.Request, error) {
	body, err := buildAnthropicBody(req)
	if err != nil {
		return nil, fmt.Errorf("llm.anthropic: build body: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(
		ctx, http.MethodPost, req.BaseURL+anthropicMessagesPath, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("llm.anthropic: new request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", req.Key)
	httpReq.Header.Set("anthropic-version", anthropicVersion)
	return httpReq, nil
}

func (p *anthropicProvider) ParseStream(ctx context.Context, resp *http.Response, _ Request) iter.Seq[StreamEvent] {
	return func(yield func(StreamEvent) bool) {
		parseAnthropicSSE(ctx, resp.Body, yield)
	}
}

// parseAnthropicSSE consumes Anthropic's named-event SSE stream into StreamEvents. Unlike
// the OpenAI data-only stream, this tracks the current "event: <name>" line, so it cannot
// use the shared scanSSELines — named-event parsing is part of Anthropic's wire.
//
// parseAnthropicSSE 读 Anthropic 命名事件 SSE 流转成 StreamEvent。它要跟踪当前
// "event: <name>" 行，故不能用共享的 scanSSELines——命名事件解析是 Anthropic wire 的一部分。
func parseAnthropicSSE(ctx context.Context, body io.Reader, yield func(StreamEvent) bool) {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), maxSSELineBytes)
	var eventName string
	var inputTokens, outputTokens int

	for scanner.Scan() {
		if ctx.Err() != nil {
			return
		}
		line := scanner.Text()

		if name, ok := strings.CutPrefix(line, "event: "); ok {
			eventName = name
			continue
		}
		data, ok := strings.CutPrefix(line, "data: ")
		if !ok {
			continue
		}

		switch eventName {
		case "message_start":
			var e anthropicMsgStart
			if err := json.Unmarshal([]byte(data), &e); err != nil {
				yield(StreamEvent{Type: EventError, Err: fmt.Errorf("llm.anthropic: parse message_start: %w", err)})
				return
			}
			if e.Message.Usage != nil {
				inputTokens = e.Message.Usage.InputTokens
			}

		case "content_block_start":
			var e anthropicBlockStart
			if err := json.Unmarshal([]byte(data), &e); err != nil {
				yield(StreamEvent{Type: EventError, Err: fmt.Errorf("llm.anthropic: parse content_block_start: %w", err)})
				return
			}
			if e.ContentBlock.Type == "tool_use" {
				if !yield(StreamEvent{
					Type:      EventToolStart,
					ToolIndex: e.Index,
					ToolID:    e.ContentBlock.ID,
					ToolName:  e.ContentBlock.Name,
				}) {
					return
				}
			}

		case "content_block_delta":
			var e anthropicBlockDelta
			if err := json.Unmarshal([]byte(data), &e); err != nil {
				yield(StreamEvent{Type: EventError, Err: fmt.Errorf("llm.anthropic: parse content_block_delta: %w", err)})
				return
			}
			if !emitAnthropicDelta(e, yield) {
				return
			}

		case "message_delta":
			var e anthropicMsgDelta
			if err := json.Unmarshal([]byte(data), &e); err != nil {
				yield(StreamEvent{Type: EventError, Err: fmt.Errorf("llm.anthropic: parse message_delta: %w", err)})
				return
			}
			if e.Usage != nil {
				outputTokens = e.Usage.OutputTokens
			}
			if e.Delta.StopReason != "" {
				if !yield(StreamEvent{
					Type:         EventFinish,
					FinishReason: e.Delta.StopReason,
					InputTokens:  inputTokens,
					OutputTokens: outputTokens,
				}) {
					return
				}
			}
		}
	}

	if err := scanner.Err(); err != nil && ctx.Err() == nil {
		yield(StreamEvent{Type: EventError, Err: fmt.Errorf("llm.anthropic: scan: %w", err)})
	}
}

func emitAnthropicDelta(e anthropicBlockDelta, yield func(StreamEvent) bool) bool {
	switch e.Delta.Type {
	case "text_delta":
		if e.Delta.Text != "" {
			return yield(StreamEvent{Type: EventText, Delta: e.Delta.Text})
		}
	case "thinking_delta":
		if e.Delta.Thinking != "" {
			return yield(StreamEvent{Type: EventReasoning, Delta: e.Delta.Thinking})
		}
	case "signature_delta":
		// A zero-Delta EventReasoning carrying only the signature, so the consumer can
		// store it next to the reasoning content for verbatim round-trip next turn.
		// 一个 Delta 为空、只带 Signature 的 EventReasoning，让消费者把签名和 reasoning
		// 一起存，下一轮原样回传。
		if e.Delta.Signature != "" {
			return yield(StreamEvent{Type: EventReasoning, Signature: e.Delta.Signature})
		}
	case "input_json_delta":
		if e.Delta.PartialJSON != "" {
			return yield(StreamEvent{Type: EventToolDelta, ToolIndex: e.Index, ArgsDelta: e.Delta.PartialJSON})
		}
	}
	return true
}

// ── request body ──────────────────────────────────────────────────────────────

func buildAnthropicBody(req Request) ([]byte, error) {
	// Anthropic permanently 400s on any orphan tool_use_id — sanitize first.
	// Anthropic 一个孤儿 tool_use_id 就 400 锁死，先 sanitize。
	req.Messages = SanitizeMessages(req.Messages)
	msgs, err := toAnthropicMsgs(req.Messages)
	if err != nil {
		return nil, err
	}

	// max_tokens is required; the caller supplies the model's cap via Request.MaxTokens
	// (0 → default), so the provider never down-caps silently nor reads a catalog.
	// max_tokens 必填；caller 经 Request.MaxTokens 提供 model 上限（0 → 默认），provider
	// 既不静默截低也不读 catalog。
	maxTok := req.MaxTokens
	if maxTok == 0 {
		maxTok = anthropicDefaultMaxTokens
	}

	body := anthropicRequest{
		Model:     req.ModelID,
		MaxTokens: maxTok,
		Messages:  msgs,
		Stream:    true,
	}

	// thinking + effort are two orthogonal Anthropic knobs, both from Options with native values.
	// thinking.type ∈ adaptive | enabled | disabled (per model; flagships take only adaptive).
	// "enabled" needs a budget_tokens (≥1024, < max_tokens; bump max_tokens if needed). "effort"
	// lives in output_config and scales total token spend.
	//
	// thinking 与 effort 是 Anthropic 两个正交旋钮，均从 Options 取原生值。thinking.type ∈
	// adaptive | enabled | disabled（按模型；旗舰只收 adaptive）；"enabled" 需 budget_tokens
	// （≥1024 且 < max_tokens，必要时上调 max_tokens）；"effort" 在 output_config，缩放总 token 花费。
	switch req.Options["thinking"] {
	case "adaptive":
		body.Thinking = &anthropicThinking{Type: "adaptive"}
	case "enabled":
		budget := min(max(maxTok/2, 1024), 8192)
		if budget >= maxTok {
			maxTok = budget + 1024
			body.MaxTokens = maxTok
		}
		body.Thinking = &anthropicThinking{Type: "enabled", BudgetTokens: budget}
	case "disabled":
		body.Thinking = &anthropicThinking{Type: "disabled"}
	}
	if v := req.Options["effort"]; v != "" {
		body.OutputConfig = &anthropicOutputConfig{Effort: v}
	}

	if req.System != "" {
		// Send system as a block array (not a plain string) so cache_control attaches.
		// 用 block 数组形式发 system（而非纯字符串），以便附加 cache_control。
		raw, err := json.Marshal([]anthropicSystemBlock{{
			Type:         "text",
			Text:         req.System,
			CacheControl: &cacheControl{Type: "ephemeral"},
		}})
		if err != nil {
			return nil, fmt.Errorf("llm.anthropic: marshal system block: %w", err)
		}
		body.System = raw
	}
	if len(req.Tools) > 0 {
		body.Tools = toAnthropicTools(req.Tools)
	}
	return json.Marshal(body)
}

// toAnthropicMsgs converts LLMMessages; consecutive RoleTool entries merge into one user message.
//
// toAnthropicMsgs 把 LLMMessage 列表转为 Anthropic 格式；连续 RoleTool 合并成一条 user 消息。
func toAnthropicMsgs(msgs []LLMMessage) ([]anthropicMessage, error) {
	var out []anthropicMessage
	for i := 0; i < len(msgs); {
		m := msgs[i]
		if m.Role == RoleTool {
			var blocks []anthropicContent
			for i < len(msgs) && msgs[i].Role == RoleTool {
				blocks = append(blocks, anthropicContent{
					Type:      "tool_result",
					ToolUseID: msgs[i].ToolCallID,
					Content:   msgs[i].Content,
				})
				i++
			}
			out = append(out, anthropicMessage{Role: "user", Content: blocks})
			continue
		}
		am, err := toAnthropicMsg(m)
		if err != nil {
			return nil, err
		}
		out = append(out, am)
		i++
	}
	return out, nil
}

func toAnthropicMsg(m LLMMessage) (anthropicMessage, error) {
	switch m.Role {
	case RoleUser:
		return buildAnthropicUserMsg(m), nil
	case RoleAssistant:
		return buildAnthropicAssistantMsg(m), nil
	default:
		return anthropicMessage{}, fmt.Errorf("llm.anthropic: unexpected role %q: %w", m.Role, ErrBadRequest)
	}
}

func buildAnthropicUserMsg(m LLMMessage) anthropicMessage {
	if len(m.Parts) == 0 {
		return anthropicMessage{Role: "user", Content: []anthropicContent{{Type: "text", Text: m.Content}}}
	}
	blocks := make([]anthropicContent, 0, len(m.Parts))
	for _, part := range m.Parts {
		switch part.Type {
		case "text":
			blocks = append(blocks, anthropicContent{Type: "text", Text: part.Text})
		case "image_url":
			blocks = append(blocks, anthropicContent{
				Type: "image",
				Source: &anthropicImageSource{
					Type:      "base64",
					MediaType: extractMediaType(part.ImageURL),
					Data:      extractBase64Data(part.ImageURL),
				},
			})
		case "file":
			// PDF / document: same {type:base64, media_type, data} source as image,
			// carried in a document block. Anthropic reads PDFs natively (text + page images).
			// PDF/文档：与图同款 source，装在 document 块；Anthropic 原生读 PDF（文本+页图）。
			blocks = append(blocks, anthropicContent{
				Type:   "document",
				Source: &anthropicImageSource{Type: "base64", MediaType: part.MediaType, Data: part.Data},
			})
		}
	}
	return anthropicMessage{Role: "user", Content: blocks}
}

func buildAnthropicAssistantMsg(m LLMMessage) anthropicMessage {
	var blocks []anthropicContent
	if m.ReasoningContent != "" {
		blocks = append(blocks, anthropicContent{
			Type:      "thinking",
			Thinking:  m.ReasoningContent,
			Signature: m.ReasoningSignature,
		})
	}
	for _, tc := range m.ToolCalls {
		// Malformed persisted args → fall back to "{}" silently (history corruption
		// must not 400 the live turn).
		// 历史里 arguments JSON 烂了 → 静默回退 "{}"（历史损坏不该让当前回合 400）。
		input := json.RawMessage("{}")
		if tc.Arguments != "" && json.Valid([]byte(tc.Arguments)) {
			input = json.RawMessage(tc.Arguments)
		}
		blocks = append(blocks, anthropicContent{Type: "tool_use", ID: tc.ID, Name: tc.Name, Input: input})
	}
	if m.Content != "" {
		blocks = append(blocks, anthropicContent{Type: "text", Text: m.Content})
	}
	return anthropicMessage{Role: "assistant", Content: blocks}
}

func toAnthropicTools(defs []ToolDef) []anthropicTool {
	out := make([]anthropicTool, len(defs))
	for i, d := range defs {
		out[i] = anthropicTool{Name: d.Name, Description: d.Description, InputSchema: d.Parameters}
	}
	// Cache breakpoint on the last tool caches the whole tools block (stable prefix).
	// 在最后一个工具上打断点，缓存整个 tools 块（稳定前缀）。
	out[len(out)-1].CacheControl = &cacheControl{Type: "ephemeral"}
	return out
}

// extractMediaType pulls the MIME from a base64 data URL; falls back to image/jpeg.
//
// extractMediaType 从 data URL 提取 MIME；非 data URL 回退 image/jpeg。
func extractMediaType(dataURL string) string {
	if !strings.HasPrefix(dataURL, "data:") {
		return "image/jpeg"
	}
	rest := strings.TrimPrefix(dataURL, "data:")
	if idx := strings.Index(rest, ";"); idx > 0 {
		return rest[:idx]
	}
	return "image/jpeg"
}

func extractBase64Data(dataURL string) string {
	if _, data, ok := strings.Cut(dataURL, ","); ok {
		return data
	}
	return dataURL
}

// ── wire types ────────────────────────────────────────────────────────────────

type anthropicRequest struct {
	Model        string                 `json:"model"`
	MaxTokens    int                    `json:"max_tokens"`
	System       json.RawMessage        `json:"system,omitempty"`
	Messages     []anthropicMessage     `json:"messages"`
	Tools        []anthropicTool        `json:"tools,omitempty"`
	Stream       bool                   `json:"stream"`
	Thinking     *anthropicThinking     `json:"thinking,omitempty"`
	OutputConfig *anthropicOutputConfig `json:"output_config,omitempty"`
}

// anthropicThinking is the wire form of the thinking param. type "adaptive" lets Claude decide
// (no budget); "enabled" requires budget_tokens ≥ 1024 and < max_tokens; "disabled" turns it off.
//
// anthropicThinking 是 thinking 参数 wire 形式。type "adaptive" 由 Claude 自决（无 budget）；
// "enabled" 要求 budget_tokens ≥ 1024 且 < max_tokens；"disabled" 关闭。
type anthropicThinking struct {
	Type         string `json:"type"`
	BudgetTokens int    `json:"budget_tokens,omitempty"`
}

// anthropicOutputConfig carries the effort knob (low|medium|high|xhigh|max), a soft signal scaling
// total output token spend; sibling of thinking in the request body.
//
// anthropicOutputConfig 携带 effort 旋钮（low|medium|high|xhigh|max），缩放总输出 token 花费的
// 软信号；请求体里与 thinking 平级。
type anthropicOutputConfig struct {
	Effort string `json:"effort,omitempty"`
}

type anthropicMessage struct {
	Role    string             `json:"role"`
	Content []anthropicContent `json:"content"`
}

type anthropicContent struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
	// Thinking + Signature: signature is the opaque Anthropic token authorising re-use of
	// a thinking block; echo it verbatim when present.
	// Thinking + Signature：signature 是授权重用 thinking block 的不透明令牌，存在时原样回传。
	Thinking  string                `json:"thinking,omitempty"`
	Signature string                `json:"signature,omitempty"`
	ID        string                `json:"id,omitempty"`
	Name      string                `json:"name,omitempty"`
	Input     json.RawMessage       `json:"input,omitempty"`
	ToolUseID string                `json:"tool_use_id,omitempty"`
	Content   string                `json:"content,omitempty"`
	Source    *anthropicImageSource `json:"source,omitempty"`
}

type anthropicImageSource struct {
	Type      string `json:"type"`
	MediaType string `json:"media_type"`
	Data      string `json:"data"`
}

type cacheControl struct {
	Type string `json:"type"`
}

type anthropicTool struct {
	Name         string          `json:"name"`
	Description  string          `json:"description"`
	InputSchema  json.RawMessage `json:"input_schema"`
	CacheControl *cacheControl   `json:"cache_control,omitempty"`
}

type anthropicSystemBlock struct {
	Type         string        `json:"type"`
	Text         string        `json:"text"`
	CacheControl *cacheControl `json:"cache_control,omitempty"`
}

type anthropicMsgStart struct {
	Message struct {
		Usage *struct {
			InputTokens int `json:"input_tokens"`
		} `json:"usage"`
	} `json:"message"`
}

type anthropicBlockStart struct {
	Index        int `json:"index"`
	ContentBlock struct {
		Type string `json:"type"`
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"content_block"`
}

type anthropicBlockDelta struct {
	Index int `json:"index"`
	Delta struct {
		Type        string `json:"type"`
		Text        string `json:"text"`
		Thinking    string `json:"thinking"`
		Signature   string `json:"signature"`
		PartialJSON string `json:"partial_json"`
	} `json:"delta"`
}

type anthropicMsgDelta struct {
	Delta struct {
		StopReason string `json:"stop_reason"`
	} `json:"delta"`
	Usage *struct {
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

// ── model catalog (static; Anthropic /v1/models lists ids but its numeric specs are doc
// placeholders, so window/output/knobs live here, maintained by software update) ───────────────

// anthropicKnobs builds the thinking knob (per-model native type set) plus, when supported, the
// effort knob. thinkingValues[0] is the default.
//
// anthropicKnobs 构造 thinking 旋钮（按模型的原生 type 集）+（若支持）effort 旋钮。
// thinkingValues[0] 为默认值。
func anthropicKnobs(thinkingValues, effortValues []string) []Knob {
	ks := []Knob{enumKnob("thinking", "Thinking", thinkingValues, thinkingValues[0])}
	if len(effortValues) > 0 {
		ks = append(ks, enumKnob("effort", "Effort", effortValues, "high"))
	}
	return ks
}

// anthropicSpecs is Anthropic's static catalog, most-specific prefix first. Flagships (Opus
// 4.8/4.7) take only thinking:adaptive (+ effort incl. xhigh); 4.6 / Sonnet-4.6 add enabled; older
// models take enabled/disabled with no effort. Numbers per Anthropic model overview, 2026-06.
//
// anthropicSpecs 是 Anthropic 静态目录，最具体前缀在前。旗舰（Opus 4.8/4.7）只收 thinking:adaptive
// （+ effort 含 xhigh）；4.6 / Sonnet-4.6 增 enabled；更老的模型 enabled/disabled 无 effort。数值据
// Anthropic model overview 2026-06。
var anthropicSpecs = []modelSpec{
	{"claude-opus-4-8", 1000000, 128000, anthropicKnobs([]string{"adaptive", "disabled"}, []string{"low", "medium", "high", "xhigh", "max"}), true, true},
	{"claude-opus-4-7", 1000000, 128000, anthropicKnobs([]string{"adaptive", "disabled"}, []string{"low", "medium", "high", "xhigh", "max"}), true, true},
	{"claude-opus-4-6", 1000000, 128000, anthropicKnobs([]string{"adaptive", "enabled", "disabled"}, []string{"low", "medium", "high", "max"}), true, true},
	{"claude-sonnet-4-6", 1000000, 64000, anthropicKnobs([]string{"adaptive", "enabled", "disabled"}, []string{"low", "medium", "high", "max"}), true, true},
	{"claude-haiku-4-5", 200000, 64000, anthropicKnobs([]string{"enabled", "disabled"}, nil), true, true},
	{"claude-opus-4-5", 200000, 64000, anthropicKnobs([]string{"enabled", "disabled"}, nil), true, true},
	{"claude-sonnet-4-5", 200000, 64000, anthropicKnobs([]string{"enabled", "disabled"}, nil), true, true},
	{"claude-opus-4-1", 200000, 32000, anthropicKnobs([]string{"enabled", "disabled"}, nil), true, true},
	{"claude-opus-4", 200000, 32000, anthropicKnobs([]string{"enabled", "disabled"}, nil), true, true},
	{"claude-sonnet-4", 200000, 64000, anthropicKnobs([]string{"enabled", "disabled"}, nil), true, true},
	{"claude-haiku-4", 200000, 64000, anthropicKnobs([]string{"enabled", "disabled"}, nil), true, true},
	{"claude", 200000, 64000, anthropicKnobs([]string{"enabled", "disabled"}, nil), true, true},
}

// DescribeModels parses Anthropic's /v1/models id list ({"data":[{"id":...}]}) against the static
// catalog. The payload also carries capability fields, but its numeric specs are doc placeholders,
// so the static table stays authoritative for window/output/knobs.
//
// DescribeModels 解析 Anthropic /v1/models 的 id 列表（{"data":[{"id":...}]}）查静态目录。载荷虽带
// 能力字段，但数值是文档占位，故窗口/输出/旋钮以静态表为准。
func (p *anthropicProvider) DescribeModels(raw string) ([]ModelInfo, error) {
	return describeFromSpecs(anthropicSpecs, raw), nil
}
