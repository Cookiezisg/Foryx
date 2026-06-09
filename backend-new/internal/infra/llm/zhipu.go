package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"iter"
	"net/http"
)

// zhipuProvider speaks Zhipu GLM's BigModel /api/paas/v4 /chat/completions wire, fully
// self-contained: its own body shape, message encoding, tool-call streaming state, and
// wire types — no sharing with the openai provider even though the wire is OpenAI-shaped.
// Zhipu specifics: thinking:{type:enabled/disabled}, tool_choice restricted to "auto",
// reasoning_content arriving before content, and extended finish_reason values
// (sensitive, network_error) that pass through verbatim.
//
// zhipuProvider 完整自包含地讲智谱 GLM BigModel /api/paas/v4 wire：自己的 body 形状、消息
// 编码、tool-call 流式状态、wire 类型——即使 wire 是 OpenAI 形状也不与 openai 共享。Zhipu
// 特有：thinking:{type:enabled/disabled}、tool_choice 只支持 "auto"、流中 reasoning_content
// 先于 content、扩展 finish_reason（sensitive/network_error）原样透传。
type zhipuProvider struct{}

func newZhipuProvider() *zhipuProvider { return &zhipuProvider{} }

func (p *zhipuProvider) Name() string           { return "zhipu" }
func (p *zhipuProvider) DefaultBaseURL() string { return "https://open.bigmodel.cn/api/paas/v4" }

// BuildRequest encodes a Request into a Zhipu GLM /chat/completions HTTP request. Auth:
// the raw API key as a Bearer token (JWT is legacy, not implemented).
//
// Native knob from Options: thinking ("enabled"/"disabled") passes through verbatim into
// thinking:{type}.
//
// tool_choice quirk: Zhipu only supports "auto" — any other value risks a 400. When tools
// are present we always send "auto"; tool-less requests omit it.
//
// BuildRequest 把 Request 编码为智谱 GLM /chat/completions HTTP 请求。Auth：原始 key 作
// Bearer（JWT 是 legacy，不实现）。原生旋钮取自 Options：thinking 原样进 thinking:{type}。
// tool_choice quirk：只支持 "auto"，有 tools 时固定发 "auto"。
func (p *zhipuProvider) BuildRequest(ctx context.Context, req Request) (*http.Request, error) {
	req.Messages = SanitizeMessages(req.Messages)
	msgs, err := toZhipuMsgs(req.Messages, req.System)
	if err != nil {
		return nil, fmt.Errorf("llm.zhipu: build messages: %w", err)
	}
	body := zhipuRequest{
		Model:    req.ModelID,
		Messages: msgs,
		Stream:   !req.DisableStream,
	}
	if !req.DisableStream {
		body.StreamOptions = &zhipuStreamOptions{IncludeUsage: true}
	}
	if len(req.Tools) > 0 {
		body.Tools = toZhipuTools(req.Tools)
		// Zhipu only supports tool_choice:"auto"; sending other values may 400.
		// Zhipu 的 tool_choice 只支持 "auto"，其他值可能返 400。
		body.ToolChoice = "auto"
	}
	if req.MaxTokens > 0 {
		body.MaxTokens = req.MaxTokens
	}
	if v := req.Options["thinking"]; v != "" {
		body.Thinking = &zhipuThinking{Type: v}
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("llm.zhipu: marshal body: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(
		ctx, http.MethodPost, req.BaseURL+"/chat/completions", bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("llm.zhipu: new request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+req.Key)
	return httpReq, nil
}

// ParseStream reads Zhipu GLM SSE chunks into StreamEvents, using the shared scanSSELines
// for raw SSE line mechanics.
//
// ParseStream 读智谱 GLM SSE chunk 为 StreamEvent，用共享 scanSSELines 处理原始 SSE 行语义。
func (p *zhipuProvider) ParseStream(ctx context.Context, resp *http.Response, req Request) iter.Seq[StreamEvent] {
	return func(yield func(StreamEvent) bool) {
		state := newZhipuToolState()
		scanErr := scanSSELines(resp.Body, func(payload []byte) bool {
			if ctx.Err() != nil {
				return false
			}
			var chunk zhipuChunk
			if err := json.Unmarshal(payload, &chunk); err != nil {
				yield(StreamEvent{Type: EventError, Err: fmt.Errorf("llm.zhipu: malformed SSE chunk: %w", err)})
				return false
			}
			return emitZhipuChunk(chunk, state, yield)
		})
		if scanErr != nil && ctx.Err() == nil {
			yield(StreamEvent{Type: EventError, Err: fmt.Errorf("llm.zhipu: scan: %w", scanErr)})
		}
	}
}

func emitZhipuChunk(chunk zhipuChunk, state *zhipuToolState, yield func(StreamEvent) bool) bool {
	if chunk.Error != nil {
		yield(StreamEvent{Type: EventError, Err: fmt.Errorf("%w: in-stream: %s", ErrProviderError, chunk.Error.Message)})
		return false
	}
	if len(chunk.Choices) == 0 {
		if chunk.Usage != nil {
			return yield(StreamEvent{Type: EventFinish, InputTokens: chunk.Usage.PromptTokens, OutputTokens: chunk.Usage.CompletionTokens})
		}
		return true
	}

	choice := chunk.Choices[0]
	delta := choice.Delta

	// Zhipu GLM-4.5+ streams reasoning_content before content — preserve that order.
	// GLM-4.5+ 先流 reasoning_content 再流 content——严格保序。
	if delta.ReasoningContent != "" {
		if !yield(StreamEvent{Type: EventReasoning, Delta: delta.ReasoningContent}) {
			return false
		}
	}
	if delta.Content != "" {
		if !yield(StreamEvent{Type: EventText, Delta: delta.Content}) {
			return false
		}
	}

	for _, tc := range delta.ToolCalls {
		idx := state.resolveIndex(tc)
		if !state.nameSent[idx] && tc.Function.Name != "" {
			state.nameSent[idx] = true
			if !yield(StreamEvent{Type: EventToolStart, ToolIndex: idx, ToolID: tc.ID, ToolName: tc.Function.Name}) {
				return false
			}
		}
		if tc.Function.Arguments != "" {
			if !yield(StreamEvent{Type: EventToolDelta, ToolIndex: idx, ArgsDelta: tc.Function.Arguments}) {
				return false
			}
		}
	}

	// finish_reason may be a Zhipu-specific "sensitive" or "network_error" on top of the
	// standard stop/tool_calls/length — pass it through as-is for the caller's display policy.
	// finish_reason 除标准 stop/tool_calls/length 外还可能是 Zhipu 专属的 sensitive/network_error，直接透传。
	if choice.FinishReason != "" {
		ev := StreamEvent{Type: EventFinish, FinishReason: choice.FinishReason}
		if chunk.Usage != nil {
			ev.InputTokens = chunk.Usage.PromptTokens
			ev.OutputTokens = chunk.Usage.CompletionTokens
		}
		return yield(ev)
	}
	return true
}

// ── message encoding ──────────────────────────────────────────────────────────

func toZhipuMsgs(msgs []LLMMessage, system string) ([]zhipuMessage, error) {
	var out []zhipuMessage
	if system != "" {
		out = append(out, zhipuMessage{Role: "system", Content: zhipuJSONString(system)})
	}
	for _, m := range msgs {
		zm, err := toZhipuMsg(m)
		if err != nil {
			return nil, err
		}
		out = append(out, zm)
	}
	return out, nil
}

func toZhipuMsg(m LLMMessage) (zhipuMessage, error) {
	switch m.Role {
	case RoleUser:
		return buildZhipuUserMsg(m)
	case RoleAssistant:
		return buildZhipuAssistantMsg(m), nil
	case RoleTool:
		return zhipuMessage{Role: "tool", Content: zhipuJSONString(m.Content), ToolCallID: m.ToolCallID}, nil
	default:
		return zhipuMessage{}, fmt.Errorf("llm.zhipu: unknown role %q: %w", m.Role, ErrBadRequest)
	}
}

func buildZhipuUserMsg(m LLMMessage) (zhipuMessage, error) {
	if len(m.Parts) == 0 {
		return zhipuMessage{Role: "user", Content: zhipuJSONString(m.Content)}, nil
	}
	parts := make([]zhipuContentPart, 0, len(m.Parts))
	for _, part := range m.Parts {
		switch part.Type {
		case "text":
			parts = append(parts, zhipuContentPart{Type: "text", Text: part.Text})
		case "image_url":
			parts = append(parts, zhipuContentPart{Type: "image_url", ImageURL: &zhipuImageURL{URL: part.ImageURL}})
		default:
			// Unsupported part type (e.g. PDF "file"): skip; the attachment layer extracts it to text.
			// 不支持的 part 类型（如 PDF "file"）：跳过；附件层抽成文本。
		}
	}
	raw, err := json.Marshal(parts)
	if err != nil {
		return zhipuMessage{}, fmt.Errorf("llm.zhipu: marshal parts: %w", err)
	}
	return zhipuMessage{Role: "user", Content: raw}, nil
}

func buildZhipuAssistantMsg(m LLMMessage) zhipuMessage {
	// Reasoning-only turn → copy reasoning into content, else a strict provider 400s next
	// turn on an assistant message carrying neither content nor tool_calls.
	// 仅 reasoning 的回合 → 把 reasoning 复制进 content，否则下一轮严格 provider 会 400。
	if m.Content == "" && len(m.ToolCalls) == 0 && m.ReasoningContent != "" {
		m.Content = m.ReasoningContent
	}
	// Always emit content (even "") — strict providers reject a null content field.
	// content 即使空也 emit ""——严格 provider 拒 null。
	zm := zhipuMessage{
		Role:    "assistant",
		Content: zhipuJSONString(m.Content),
	}
	for _, tc := range m.ToolCalls {
		zm.ToolCalls = append(zm.ToolCalls, zhipuToolCall{
			ID:       tc.ID,
			Type:     "function",
			Function: zhipuFuncCall{Name: tc.Name, Arguments: zhipuToolArgs(tc.Arguments)},
		})
	}
	return zm
}

func toZhipuTools(defs []ToolDef) []zhipuTool {
	out := make([]zhipuTool, len(defs))
	for i, d := range defs {
		out[i] = zhipuTool{Type: "function", Function: zhipuFuncDef{Name: d.Name, Description: d.Description, Parameters: d.Parameters}}
	}
	return out
}

func zhipuJSONString(s string) json.RawMessage {
	b, _ := json.Marshal(s)
	return b
}

// zhipuToolArgs guards against a malformed historical tool-call arguments string: Zhipu
// rejects a non-JSON arguments field, so a non-JSON value silently falls back to "{}".
//
// zhipuToolArgs 守历史 tool-call arguments 字符串：Zhipu 拒非 JSON 的 arguments，非 JSON 静默回退 "{}"。
func zhipuToolArgs(s string) string {
	if s == "" || !json.Valid([]byte(s)) {
		return "{}"
	}
	return s
}

// ── tool-call streaming state ──────────────────────────────────────────────────

// zhipuToolState tracks per-chunk tool-call streaming state; synthesizes an index by ID
// for chunks that omit it.
//
// zhipuToolState 跨 chunk 跟踪 tool-call 流式状态；对不填 index 的 chunk 按 ID 合成 index。
type zhipuToolState struct {
	nameSent     map[int]bool
	idToIdx      map[string]int
	nextSynthIdx int
}

func newZhipuToolState() *zhipuToolState {
	return &zhipuToolState{nameSent: map[int]bool{}, idToIdx: map[string]int{}}
}

func (s *zhipuToolState) resolveIndex(tc zhipuToolCallDelta) int {
	if tc.Index > 0 {
		return tc.Index
	}
	if tc.ID == "" {
		return 0
	}
	if idx, ok := s.idToIdx[tc.ID]; ok {
		return idx
	}
	idx := s.nextSynthIdx
	s.idToIdx[tc.ID] = idx
	s.nextSynthIdx++
	return idx
}

// ── Zhipu wire types ────────────────────────────────────────────────────────────

type zhipuRequest struct {
	Model         string              `json:"model"`
	Messages      []zhipuMessage      `json:"messages"`
	Tools         []zhipuTool         `json:"tools,omitempty"`
	ToolChoice    string              `json:"tool_choice,omitempty"`
	Stream        bool                `json:"stream"`
	StreamOptions *zhipuStreamOptions `json:"stream_options,omitempty"`
	MaxTokens     int                 `json:"max_tokens,omitempty"`
	Thinking      *zhipuThinking      `json:"thinking,omitempty"`
}

type zhipuStreamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

type zhipuThinking struct {
	Type string `json:"type"`
}

// zhipuMessage holds Content as RawMessage to accept either a string or a content-part array.
//
// zhipuMessage Content 用 RawMessage，可装字符串或 content-part 数组。
type zhipuMessage struct {
	Role       string          `json:"role"`
	Content    json.RawMessage `json:"content,omitempty"`
	ToolCalls  []zhipuToolCall `json:"tool_calls,omitempty"`
	ToolCallID string          `json:"tool_call_id,omitempty"`
}

type zhipuContentPart struct {
	Type     string         `json:"type"`
	Text     string         `json:"text,omitempty"`
	ImageURL *zhipuImageURL `json:"image_url,omitempty"`
}

type zhipuImageURL struct {
	URL string `json:"url"`
}

type zhipuToolCall struct {
	ID       string        `json:"id"`
	Type     string        `json:"type"`
	Function zhipuFuncCall `json:"function"`
}

type zhipuFuncCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type zhipuTool struct {
	Type     string       `json:"type"`
	Function zhipuFuncDef `json:"function"`
}

type zhipuFuncDef struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

type zhipuChunk struct {
	Choices []zhipuChoice    `json:"choices"`
	Usage   *zhipuUsage      `json:"usage"`
	Error   *zhipuChunkError `json:"error,omitempty"`
}

type zhipuChunkError struct {
	Message string `json:"message"`
}

type zhipuChoice struct {
	Delta        zhipuDelta `json:"delta"`
	FinishReason string     `json:"finish_reason"`
}

type zhipuDelta struct {
	Content          string               `json:"content"`
	ReasoningContent string               `json:"reasoning_content"`
	ToolCalls        []zhipuToolCallDelta `json:"tool_calls"`
}

type zhipuToolCallDelta struct {
	Index    int            `json:"index"`
	ID       string         `json:"id"`
	Function zhipuFuncDelta `json:"function"`
}

type zhipuFuncDelta struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type zhipuUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
}

// ── model catalog (static; Zhipu /models returns ids only) ──────────────────────

func zhipuThinkingKnobs() []Knob {
	return []Knob{enumKnob("thinking", "Thinking", []string{"enabled", "disabled"}, "enabled")}
}

// zhipuSpecs is Zhipu's static catalog, most-specific prefix first. GLM-4.5+ defaults thinking
// to enabled and exposes the thinking knob; glm-4-long/glm-4-flash predate thinking and carry
// none. Numbers per Zhipu BigModel docs, 2026-06-04.
//
// zhipuSpecs 是智谱静态目录，最具体前缀在前。GLM-4.5+ thinking 默认 enabled 且暴露旋钮；
// glm-4-long/glm-4-flash 早于 thinking 无旋钮。数值据智谱 BigModel 文档 2026-06-04。
var zhipuSpecs = []modelSpec{
	{"glm-5.1", 200000, 128000, zhipuThinkingKnobs(), false, false},
	{"glm-5-turbo", 200000, 128000, zhipuThinkingKnobs(), false, false},
	{"glm-5", 200000, 128000, zhipuThinkingKnobs(), false, false},
	{"glm-4.7-flashx", 200000, 128000, zhipuThinkingKnobs(), false, false},
	{"glm-4.7-flash", 200000, 128000, zhipuThinkingKnobs(), false, false},
	{"glm-4.7", 200000, 128000, zhipuThinkingKnobs(), false, false},
	{"glm-4.6", 200000, 128000, zhipuThinkingKnobs(), false, false},
	{"glm-4.5", 131072, 96000, zhipuThinkingKnobs(), false, false},
	{"glm-4-long", 1000000, 4096, nil, false, false},
	{"glm-4-flash", 131072, 16000, nil, false, false},
}

// DescribeModels parses Zhipu's id-only /models body against the static catalog.
//
// DescribeModels 解析智谱仅含 id 的 /models 返回，查静态目录。
func (p *zhipuProvider) DescribeModels(raw string) ([]ModelInfo, error) {
	return describeFromSpecs(zhipuSpecs, raw), nil
}
