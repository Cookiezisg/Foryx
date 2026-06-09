package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"iter"
	"net/http"
)

// customProvider speaks a generic OpenAI-compatible /chat/completions API for
// user-configured custom endpoints, fully self-contained: its own body shape,
// message encoding, SSE chunk parsing, and wire types — no sharing with the
// openai provider even though the wire is OpenAI-shaped. The deliberate quirk
// here is the ABSENCE of any thinking encoding: a generic endpoint may 400 on
// a thinking field it doesn't recognize, so this provider never emits one.
//
// customProvider 自包含地讲通用 OpenAI-compat /chat/completions（用户自配端点）：
// 自己的 body 形状、消息编码、SSE 解析、wire 类型——即使 wire 是 OpenAI 形状也不与
// openai 共享。本家特点是「不发 thinking」：通用端点可能因不认识的 thinking 字段而 400。
type customProvider struct{}

func newCustomProvider() *customProvider { return &customProvider{} }

func (p *customProvider) Name() string           { return "custom" }
func (p *customProvider) DefaultBaseURL() string { return "" } // caller must supply base_url

// BuildRequest encodes a Request into a generic OpenAI-compat /chat/completions
// HTTP request. Auth: Bearer token.
//
// No knobs are emitted: a custom endpoint is generic, and any reasoning/thinking field it does
// not recognise would risk a 400, so req.Options is deliberately ignored here.
//
// BuildRequest 把 Request 编码为通用 OpenAI-compat /chat/completions 请求。Auth：Bearer。
// 不发任何旋钮：通用端点不一定支持某推理/thinking 字段，发了会触发 400，故此处刻意忽略 req.Options。
func (p *customProvider) BuildRequest(ctx context.Context, req Request) (*http.Request, error) {
	req.Messages = SanitizeMessages(req.Messages)
	msgs, err := toCustomMsgs(req.Messages, req.System)
	if err != nil {
		return nil, fmt.Errorf("llm.custom: build messages: %w", err)
	}
	body := customRequest{
		Model:    req.ModelID,
		Messages: msgs,
		Stream:   !req.DisableStream,
	}
	if !req.DisableStream {
		body.StreamOptions = &customStreamOptions{IncludeUsage: true}
	}
	if len(req.Tools) > 0 {
		body.Tools = toCustomTools(req.Tools)
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("llm.custom: marshal body: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(
		ctx, http.MethodPost, req.BaseURL+"/chat/completions", bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("llm.custom: new request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+req.Key)
	return httpReq, nil
}

// ParseStream reads OpenAI-compat SSE chunks (or one non-streaming body) from a
// custom endpoint into StreamEvents, using the shared scanSSELines for raw SSE
// line mechanics.
//
// ParseStream 读自定义端点的 OpenAI-compat SSE chunk（或单条非流式 body）为 StreamEvent，
// 用共享的 scanSSELines 处理原始 SSE 行语义。
func (p *customProvider) ParseStream(ctx context.Context, resp *http.Response, req Request) iter.Seq[StreamEvent] {
	return func(yield func(StreamEvent) bool) {
		if req.DisableStream {
			parseCustomNonStreaming(resp.Body, yield)
			return
		}
		state := newCustomToolState()
		scanErr := scanSSELines(resp.Body, func(payload []byte) bool {
			if ctx.Err() != nil {
				return false
			}
			var chunk customChunk
			if err := json.Unmarshal(payload, &chunk); err != nil {
				yield(StreamEvent{Type: EventError, Err: fmt.Errorf("llm.custom: malformed SSE chunk: %w", err)})
				return false
			}
			return emitCustomChunk(chunk, state, yield)
		})
		if scanErr != nil && ctx.Err() == nil {
			yield(StreamEvent{Type: EventError, Err: fmt.Errorf("llm.custom: scan: %w", scanErr)})
		}
	}
}

func emitCustomChunk(chunk customChunk, state *customToolState, yield func(StreamEvent) bool) bool {
	// A chunk-level error object inside a 200 stream (rare; e.g. content filter) — surface it.
	// 200 流里出现 chunk 级 error 对象（罕见，如内容过滤）——透出。
	if chunk.Error != nil {
		yield(StreamEvent{Type: EventError, Err: fmt.Errorf("%w: in-stream: %s", ErrProviderError, chunk.Error.Message)})
		return false
	}
	if len(chunk.Choices) == 0 {
		if chunk.Usage != nil {
			return yield(StreamEvent{
				Type:         EventFinish,
				InputTokens:  chunk.Usage.PromptTokens,
				OutputTokens: chunk.Usage.CompletionTokens,
			})
		}
		return true
	}

	choice := chunk.Choices[0]
	delta := choice.Delta

	// Pass reasoning_content through if a DeepSeek-compatible endpoint happens to
	// send it; generic endpoints simply never populate it. Best-effort, not relied on.
	// 若兼容端点恰好发 reasoning_content 则透传；通用端点不填。尽力而为，不依赖。
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

// parseCustomNonStreaming reads a single non-streaming JSON body into StreamEvents.
//
// parseCustomNonStreaming 读单条非流式 JSON 响应并合成 StreamEvent 序列。
func parseCustomNonStreaming(body io.Reader, yield func(StreamEvent) bool) {
	raw, err := io.ReadAll(io.LimitReader(body, 8<<20))
	if err != nil {
		yield(StreamEvent{Type: EventError, Err: fmt.Errorf("llm.custom: read non-streaming body: %w", err)})
		return
	}
	var resp customNonStreamResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		yield(StreamEvent{Type: EventError, Err: fmt.Errorf("llm.custom: parse non-streaming response: %w", err)})
		return
	}
	if resp.Error != nil {
		yield(StreamEvent{Type: EventError, Err: fmt.Errorf("%w: %s", ErrProviderError, resp.Error.Message)})
		return
	}
	if len(resp.Choices) == 0 {
		yield(StreamEvent{Type: EventError, Err: fmt.Errorf("llm.custom: non-streaming response has no choices: %w", ErrProviderError)})
		return
	}
	msg := resp.Choices[0].Message
	if msg.ReasoningContent != "" {
		if !yield(StreamEvent{Type: EventReasoning, Delta: msg.ReasoningContent}) {
			return
		}
	}
	if msg.Content != "" {
		if !yield(StreamEvent{Type: EventText, Delta: msg.Content}) {
			return
		}
	}
	for i, tc := range msg.ToolCalls {
		if !yield(StreamEvent{Type: EventToolStart, ToolIndex: i, ToolID: tc.ID, ToolName: tc.Function.Name}) {
			return
		}
		if tc.Function.Arguments != "" {
			if !yield(StreamEvent{Type: EventToolDelta, ToolIndex: i, ArgsDelta: tc.Function.Arguments}) {
				return
			}
		}
	}
	ev := StreamEvent{Type: EventFinish, FinishReason: resp.Choices[0].FinishReason}
	if resp.Usage != nil {
		ev.InputTokens = resp.Usage.PromptTokens
		ev.OutputTokens = resp.Usage.CompletionTokens
	}
	yield(ev)
}

// ── message encoding ──────────────────────────────────────────────────────────

func toCustomMsgs(msgs []LLMMessage, system string) ([]customMessage, error) {
	var out []customMessage
	if system != "" {
		out = append(out, customMessage{Role: "system", Content: customJSONString(system)})
	}
	for _, m := range msgs {
		cm, err := toCustomMsg(m)
		if err != nil {
			return nil, err
		}
		out = append(out, cm)
	}
	return out, nil
}

func toCustomMsg(m LLMMessage) (customMessage, error) {
	switch m.Role {
	case RoleUser:
		return buildCustomUserMsg(m)
	case RoleAssistant:
		return buildCustomAssistantMsg(m), nil
	case RoleTool:
		return customMessage{Role: "tool", Content: customJSONString(m.Content), ToolCallID: m.ToolCallID}, nil
	default:
		return customMessage{}, fmt.Errorf("llm.custom: unknown role %q: %w", m.Role, ErrBadRequest)
	}
}

func buildCustomUserMsg(m LLMMessage) (customMessage, error) {
	if len(m.Parts) == 0 {
		return customMessage{Role: "user", Content: customJSONString(m.Content)}, nil
	}
	parts := make([]customContentPart, 0, len(m.Parts))
	for _, part := range m.Parts {
		switch part.Type {
		case "text":
			parts = append(parts, customContentPart{Type: "text", Text: part.Text})
		case "image_url":
			parts = append(parts, customContentPart{Type: "image_url", ImageURL: &customImageURL{URL: part.ImageURL}})
		default:
			// Unsupported part type (e.g. PDF "file"): skip; the attachment layer extracts it to text.
			// 不支持的 part 类型（如 PDF "file"）：跳过；附件层抽成文本。
		}
	}
	raw, err := json.Marshal(parts)
	if err != nil {
		return customMessage{}, fmt.Errorf("llm.custom: marshal parts: %w", err)
	}
	return customMessage{Role: "user", Content: raw}, nil
}

func buildCustomAssistantMsg(m LLMMessage) customMessage {
	// Reasoning-only turn → copy reasoning into content, else a strict endpoint 400s
	// next turn on an assistant message with neither content nor tool_calls.
	// 仅 reasoning 的回合 → 把 reasoning 复制进 content，否则严格端点下一轮会 400。
	if m.Content == "" && len(m.ToolCalls) == 0 && m.ReasoningContent != "" {
		m.Content = m.ReasoningContent
	}
	// Always emit content (even "") — strict endpoints reject a null content field.
	// content 即使空也 emit ""——严格端点拒 null。
	cm := customMessage{
		Role:    "assistant",
		Content: customJSONString(m.Content),
	}
	for _, tc := range m.ToolCalls {
		cm.ToolCalls = append(cm.ToolCalls, customToolCall{
			ID:       tc.ID,
			Type:     "function",
			Function: customFuncCall{Name: tc.Name, Arguments: customSafeArgs(tc.Arguments)},
		})
	}
	return cm
}

func toCustomTools(defs []ToolDef) []customTool {
	out := make([]customTool, len(defs))
	for i, d := range defs {
		out[i] = customTool{Type: "function", Function: customFuncDef{Name: d.Name, Description: d.Description, Parameters: d.Parameters}}
	}
	return out
}

func customJSONString(s string) json.RawMessage {
	b, _ := json.Marshal(s)
	return b
}

// customSafeArgs guards a stored tool-call arguments string against malformed
// history: a non-empty value that isn't valid JSON would make a strict endpoint
// 400 on the whole continuation, so it's silently replaced with an empty object.
// Empty stays empty (a no-arg call is legitimate).
//
// customSafeArgs 守护历史 tool-call arguments：非空但非合法 JSON 会让严格端点对整段
// 续写 400，故静默替换为空对象；空保持空（无参调用合法）。
func customSafeArgs(args string) string {
	if args == "" {
		return ""
	}
	if !json.Valid([]byte(args)) {
		return "{}"
	}
	return args
}

// ── tool-call streaming state ──────────────────────────────────────────────────

// customToolState tracks per-chunk tool-call streaming state; synthesizes an index
// by ID for chunks that omit it.
//
// customToolState 跨 chunk 跟踪 tool-call 流式状态；对不填 index 的 chunk 按 ID 合成 index。
type customToolState struct {
	nameSent     map[int]bool
	idToIdx      map[string]int
	nextSynthIdx int
}

func newCustomToolState() *customToolState {
	return &customToolState{nameSent: map[int]bool{}, idToIdx: map[string]int{}}
}

func (s *customToolState) resolveIndex(tc customToolCallDelta) int {
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

// ── custom wire types ───────────────────────────────────────────────────────────
//
// Plain OpenAI-compat shape with no thinking fields. reasoning_content on the
// response delta is read as pass-through only.
//
// 纯 OpenAI-compat 形、无 thinking 字段。响应 delta 的 reasoning_content 仅作透传。

type customRequest struct {
	Model         string               `json:"model"`
	Messages      []customMessage      `json:"messages"`
	Tools         []customTool         `json:"tools,omitempty"`
	Stream        bool                 `json:"stream"`
	StreamOptions *customStreamOptions `json:"stream_options,omitempty"`
}

type customStreamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

// customMessage holds Content as RawMessage to accept either a string or a content-part array.
//
// customMessage Content 用 RawMessage，可装字符串或 content-part 数组。
type customMessage struct {
	Role       string           `json:"role"`
	Content    json.RawMessage  `json:"content,omitempty"`
	ToolCalls  []customToolCall `json:"tool_calls,omitempty"`
	ToolCallID string           `json:"tool_call_id,omitempty"`
}

type customContentPart struct {
	Type     string          `json:"type"`
	Text     string          `json:"text,omitempty"`
	ImageURL *customImageURL `json:"image_url,omitempty"`
}

type customImageURL struct {
	URL string `json:"url"`
}

type customToolCall struct {
	ID       string         `json:"id"`
	Type     string         `json:"type"`
	Function customFuncCall `json:"function"`
}

type customFuncCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type customTool struct {
	Type     string        `json:"type"`
	Function customFuncDef `json:"function"`
}

type customFuncDef struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

type customChunk struct {
	Choices []customChoice    `json:"choices"`
	Usage   *customUsage      `json:"usage"`
	Error   *customChunkError `json:"error,omitempty"`
}

type customChunkError struct {
	Message string `json:"message"`
}

type customChoice struct {
	Delta        customDelta `json:"delta"`
	FinishReason string      `json:"finish_reason"`
}

type customDelta struct {
	Content          string                `json:"content"`
	ReasoningContent string                `json:"reasoning_content"`
	ToolCalls        []customToolCallDelta `json:"tool_calls"`
}

type customToolCallDelta struct {
	Index    int             `json:"index"`
	ID       string          `json:"id"`
	Function customFuncDelta `json:"function"`
}

type customFuncDelta struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type customUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
}

type customNonStreamResponse struct {
	Choices []customNonStreamChoice `json:"choices"`
	Usage   *customUsage            `json:"usage"`
	Error   *customChunkError       `json:"error,omitempty"`
}

type customNonStreamChoice struct {
	Message      customNonStreamMessage `json:"message"`
	FinishReason string                 `json:"finish_reason"`
}

type customNonStreamMessage struct {
	Role             string                `json:"role"`
	Content          string                `json:"content"`
	ReasoningContent string                `json:"reasoning_content"`
	ToolCalls        []customToolCallDelta `json:"tool_calls"`
}

// DescribeModels best-effort parses an OpenAI-compat /models id list from a custom endpoint. A
// generic endpoint has no static catalog, so models carry no knobs or window specs — the user can
// still target a model id directly.
//
// DescribeModels 尽力解析自定义端点的 OpenAI-compat /models id 列表。通用端点无静态目录，故模型
// 不带旋钮或窗口规格——用户仍可直接用某 model id。
func (p *customProvider) DescribeModels(raw string) ([]ModelInfo, error) {
	ids := decodeOpenAICompatModelIDs(raw)
	out := make([]ModelInfo, 0, len(ids))
	for _, id := range ids {
		out = append(out, ModelInfo{ID: id, DisplayName: id})
	}
	return out, nil
}
