package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"iter"
	"log/slog"
	"net/http"
	"strings"

	modelcapspkg "github.com/sunweilin/forgify/backend/internal/pkg/modelcaps"
)

const (
	anthropicVersion          = "2023-06-01"
	anthropicMessagesPath     = "/v1/messages"
	anthropicDefaultMaxTokens = 8096
	anthropicDefaultBaseURL   = "https://api.anthropic.com"
)

// anthropicProvider speaks Anthropic's native /v1/messages dialect: block-form
// messages, x-api-key auth, cache_control breakpoints, named-event SSE.
//
// anthropicProvider 讲 Anthropic 原生 /v1/messages 方言：block 形式 messages、
// x-api-key 鉴权、cache_control 断点、命名事件 SSE。
type anthropicProvider struct{}

func newAnthropicProvider() *anthropicProvider { return &anthropicProvider{} }

func (p *anthropicProvider) Name() string           { return "anthropic" }
func (p *anthropicProvider) DefaultBaseURL() string { return anthropicDefaultBaseURL }

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

// parseAnthropicSSE consumes Anthropic's named-event SSE stream into StreamEvents.
//
// parseAnthropicSSE 读取 Anthropic 命名事件 SSE 流并转成 StreamEvent。
func parseAnthropicSSE(ctx context.Context, body io.Reader, yield func(StreamEvent) bool) {
	scanner := bufio.NewScanner(body)
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
	case "input_json_delta":
		if e.Delta.PartialJSON != "" {
			return yield(StreamEvent{
				Type:      EventToolDelta,
				ToolIndex: e.Index,
				ArgsDelta: e.Delta.PartialJSON,
			})
		}
	}
	return true
}

func buildAnthropicBody(req Request) ([]byte, error) {
	// TE-25: Anthropic 400s permanently on any orphan tool_use_id — sanitize first.
	// TE-25：Anthropic 一个孤儿 tool_use_id 就 400 锁死，先 sanitize。
	req.Messages = SanitizeMessages(req.Messages)
	msgs, err := toAnthropicMsgs(req.Messages)
	if err != nil {
		return nil, err
	}

	// Derive max_tokens from per-model capability; fall back to the old constant
	// for unknown models so callers are never silently down-capped.
	//
	// 从 per-model 能力派生 max_tokens；未知 model 退到旧常量，不静默截低。
	cap := modelcapspkg.Lookup("anthropic", req.ModelID)
	maxTok := cap.MaxOutput
	if maxTok == 0 {
		maxTok = anthropicDefaultMaxTokens
	}

	body := anthropicRequest{
		Model:     req.ModelID,
		MaxTokens: maxTok,
		Messages:  msgs,
		Stream:    true,
	}

	// Encode thinking per 03 §4: enabled → budget_tokens (≥1024, < max_tokens);
	// off → disabled form; nil/auto → omit entirely.
	//
	// 按 03 §4 编码 thinking：enabled → budget_tokens（≥1024 且 < max_tokens）；
	// off → disabled 形；nil/auto → 完全省略。
	if req.Thinking != nil && req.Thinking.Mode == "on" {
		budget := req.Thinking.Budget
		if budget == 0 {
			// Default: half of max_tokens, at least 1024, at most 8192.
			// 默认：max_tokens 的一半，至少 1024，至多 8192。
			budget = maxTok / 2
			if budget < 1024 {
				budget = 1024
			}
			if budget > 8192 {
				budget = 8192
			}
		}
		// Enforce minimum.
		if budget < 1024 {
			budget = 1024
		}
		// Enforce budget < max_tokens: bump max_tokens if needed.
		// Anthropic 400s when budget >= max_tokens.
		//
		// 保证 budget < max_tokens；budget ≥ max_tokens 时上调 max_tokens。
		if budget >= maxTok {
			maxTok = budget + 1024
			body.MaxTokens = maxTok
		}
		body.Thinking = &anthropicThinking{Type: "enabled", BudgetTokens: budget}
	} else if req.Thinking != nil && req.Thinking.Mode == "off" {
		body.Thinking = &anthropicThinking{Type: "disabled"}
	}
	// Note: anthropicRequest has no Temperature/TopP/TopK fields, so there is
	// nothing to guard off when thinking is enabled. Future additions of those
	// fields must add the guard here per 03 §4.
	//
	// 注：anthropicRequest 当前无 Temperature/TopP/TopK，无需 guard。
	// 若将来加这些字段，必须在此处加 thinking-on 时的禁发 guard。

	if req.System != "" {
		// Send system as a block array so cache_control can be attached.
		// Anthropic accepts system as string OR []text-block; block form is required for caching.
		//
		// 用 block 数组形式发送 system，以便附加 cache_control。
		sysBlock := anthropicSystemBlock{
			Type:         "text",
			Text:         req.System,
			CacheControl: &cacheControl{Type: "ephemeral"},
		}
		raw, err := json.Marshal([]anthropicSystemBlock{sysBlock})
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
		} else {
			am, err := toAnthropicMsg(m)
			if err != nil {
				return nil, err
			}
			out = append(out, am)
			i++
		}
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
		return anthropicMessage{}, fmt.Errorf("llm.anthropic: unexpected role %q in toAnthropicMsg: %w", m.Role, ErrBadRequest)
	}
}

func buildAnthropicUserMsg(m LLMMessage) anthropicMessage {
	if len(m.Parts) == 0 {
		return anthropicMessage{
			Role:    "user",
			Content: []anthropicContent{{Type: "text", Text: m.Content}},
		}
	}
	blocks := make([]anthropicContent, 0, len(m.Parts))
	for _, p := range m.Parts {
		switch p.Type {
		case "text":
			blocks = append(blocks, anthropicContent{Type: "text", Text: p.Text})
		case "image_url":
			blocks = append(blocks, anthropicContent{
				Type: "image",
				Source: &anthropicImageSource{
					Type:      "base64",
					MediaType: extractMediaType(p.ImageURL),
					Data:      extractBase64Data(p.ImageURL),
				},
			})
		}
	}
	return anthropicMessage{Role: "user", Content: blocks}
}

func buildAnthropicAssistantMsg(m LLMMessage) anthropicMessage {
	var blocks []anthropicContent
	if m.ReasoningContent != "" {
		blocks = append(blocks, anthropicContent{
			Type:     "thinking",
			Thinking: m.ReasoningContent,
		})
	}
	for _, tc := range m.ToolCalls {
		// Bad JSON in persisted history → fall back to "{}" and log loudly.
		// 历史里 arguments JSON 烂了 → 回退 "{}" 并高声 log。
		input := json.RawMessage("{}")
		if tc.Arguments != "" {
			if err := json.Unmarshal([]byte(tc.Arguments), &input); err != nil {
				slog.Warn("llm.anthropic: history tool-call arguments are malformed JSON, falling back to {}",
					"tool_call_id", tc.ID, "tool_name", tc.Name, "raw", tc.Arguments, "err", err)
				input = json.RawMessage("{}")
			}
		}
		blocks = append(blocks, anthropicContent{
			Type:  "tool_use",
			ID:    tc.ID,
			Name:  tc.Name,
			Input: input,
		})
	}
	if m.Content != "" {
		blocks = append(blocks, anthropicContent{Type: "text", Text: m.Content})
	}
	return anthropicMessage{Role: "assistant", Content: blocks}
}

func toAnthropicTools(defs []ToolDef) []anthropicTool {
	out := make([]anthropicTool, len(defs))
	for i, d := range defs {
		out[i] = anthropicTool{
			Name:        d.Name,
			Description: d.Description,
			InputSchema: d.Parameters,
		}
	}
	// Marking the last tool caches the entire tools block (stable prefix).
	// Anthropic caches all content up to and including this breakpoint.
	//
	// 在最后一个工具上打断点，Anthropic 会缓存到此处的所有内容（含 tools 整块）。
	out[len(out)-1].CacheControl = &cacheControl{Type: "ephemeral"}
	return out
}

// extractMediaType pulls the MIME from a base64 data URL; falls back to image/jpeg.
//
// extractMediaType 从 data URL 提取 MIME；非 data URL 时回退 image/jpeg。
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

type anthropicRequest struct {
	Model     string             `json:"model"`
	MaxTokens int                `json:"max_tokens"`
	System    json.RawMessage    `json:"system,omitempty"`
	Messages  []anthropicMessage `json:"messages"`
	Tools     []anthropicTool    `json:"tools,omitempty"`
	Stream    bool               `json:"stream"`
	Thinking  *anthropicThinking `json:"thinking,omitempty"`
}

// anthropicThinking is the wire form of Anthropic's thinking parameter.
// type "enabled" requires budget_tokens ≥ 1024 and < max_tokens.
//
// anthropicThinking 是 Anthropic thinking 参数的 wire 形式。
// type "enabled" 要求 budget_tokens ≥ 1024 且 < max_tokens。
type anthropicThinking struct {
	Type         string `json:"type"`
	BudgetTokens int    `json:"budget_tokens,omitempty"`
}

type anthropicMessage struct {
	Role    string             `json:"role"`
	Content []anthropicContent `json:"content"`
}

type anthropicContent struct {
	Type      string                `json:"type"`
	Text      string                `json:"text,omitempty"`
	Thinking  string                `json:"thinking,omitempty"`
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

// anthropicSystemBlock is one element of Anthropic's block-form system array.
// Sending system as blocks (vs plain string) lets us attach cache_control.
//
// anthropicSystemBlock 是 Anthropic block 形式 system 数组的一个元素。
// 用 block 形式（而非纯字符串）可以附加 cache_control。
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
		Type     string `json:"type"`
		ID       string `json:"id"`
		Name     string `json:"name"`
		Text     string `json:"text"`
		Thinking string `json:"thinking"`
	} `json:"content_block"`
}

type anthropicBlockDelta struct {
	Index int `json:"index"`
	Delta struct {
		Type        string `json:"type"`
		Text        string `json:"text"`
		Thinking    string `json:"thinking"`
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
