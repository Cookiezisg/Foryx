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

// openAICompatProvider is the shared OpenAI-compatible wire dialect backing
// every /chat/completions provider (openai, deepseek, qwen, zhipu, moonshot,
// doubao, openrouter, google's compat surface, ollama, custom). One copy of
// the body/SSE logic; per-provider identity (name + base URL) is injected.
// beforeRequest is an optional hook for per-provider Request mutations applied
// before BuildRequest (e.g. deepseek reasoning strip, ollama stream-disable).
//
// openAICompatProvider 是所有 /chat/completions provider 共用的 OpenAI-compat
// wire 方言。body/SSE 逻辑只此一份；per-provider 身份（name + base URL）注入。
// beforeRequest 是可选的 per-provider Request 变换钩子，在 BuildRequest 前执行。
type openAICompatProvider struct {
	name           string
	defaultBaseURL string
	beforeRequest  func(*Request) // nil if no per-provider mutation needed
}

func newOpenAICompatProvider(name, defaultBaseURL string) *openAICompatProvider {
	return &openAICompatProvider{name: name, defaultBaseURL: defaultBaseURL}
}

func (p *openAICompatProvider) Name() string           { return p.name }
func (p *openAICompatProvider) DefaultBaseURL() string { return p.defaultBaseURL }

func (p *openAICompatProvider) BuildRequest(ctx context.Context, req Request) (*http.Request, error) {
	body, err := buildOpenAIBody(req)
	if err != nil {
		return nil, fmt.Errorf("llm.%s: build body: %w", p.name, err)
	}
	httpReq, err := http.NewRequestWithContext(
		ctx, http.MethodPost, req.BaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("llm.%s: new request: %w", p.name, err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+req.Key)
	return httpReq, nil
}

func (p *openAICompatProvider) ParseStream(ctx context.Context, resp *http.Response, req Request) iter.Seq[StreamEvent] {
	return func(yield func(StreamEvent) bool) {
		if req.DisableStream {
			parseOpenAINonStreaming(resp.Body, yield)
		} else {
			parseOpenAISSE(ctx, resp.Body, yield)
		}
	}
}

// parseOpenAINonStreaming reads a single non-streaming JSON body into StreamEvents.
//
// parseOpenAINonStreaming 读单条非流式 JSON 响应并合成 StreamEvent 序列。
func parseOpenAINonStreaming(body io.Reader, yield func(StreamEvent) bool) {
	raw, err := io.ReadAll(io.LimitReader(body, 8<<20))
	if err != nil {
		yield(StreamEvent{Type: EventError, Err: fmt.Errorf("llm.openai: read non-streaming body: %w", err)})
		return
	}
	var resp oaiNonStreamResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		yield(StreamEvent{Type: EventError, Err: fmt.Errorf("llm.openai: parse non-streaming response: %w", err)})
		return
	}
	if resp.Error != nil {
		yield(StreamEvent{Type: EventError, Err: fmt.Errorf("llm: provider returned error: %s", resp.Error.Message)})
		return
	}
	if len(resp.Choices) == 0 {
		yield(StreamEvent{Type: EventError, Err: fmt.Errorf("llm.openai: non-streaming response has no choices: %w", ErrProviderError)})
		return
	}
	msg := resp.Choices[0].Message
	// Prefer reasoning_content (CN family); fall back to reasoning (Ollama /v1).
	// 优先用 reasoning_content（CN 家族）；fallback 到 reasoning（Ollama /v1）。
	reasoningText := msg.ReasoningContent
	if reasoningText == "" {
		reasoningText = msg.Reasoning
	}
	if reasoningText != "" {
		if !yield(StreamEvent{Type: EventReasoning, Delta: reasoningText}) {
			return
		}
	}
	if msg.Content != "" {
		if !yield(StreamEvent{Type: EventText, Delta: msg.Content}) {
			return
		}
	}
	for i, tc := range msg.ToolCalls {
		if !yield(StreamEvent{
			Type: EventToolStart, ToolIndex: i,
			ToolID: tc.ID, ToolName: tc.Function.Name,
		}) {
			return
		}
		if tc.Function.Arguments != "" {
			if !yield(StreamEvent{
				Type: EventToolDelta, ToolIndex: i,
				ArgsDelta: tc.Function.Arguments,
			}) {
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

func parseOpenAISSE(ctx context.Context, body io.Reader, yield func(StreamEvent) bool) {
	scanner := bufio.NewScanner(body)
	state := newToolCallState()

	for scanner.Scan() {
		if ctx.Err() != nil {
			return
		}
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			return
		}
		if data == "" {
			continue
		}
		var chunk oaiChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			yield(StreamEvent{Type: EventError, Err: fmt.Errorf("llm.openai: malformed SSE chunk: %w", err)})
			return
		}
		if !emitOpenAIChunk(chunk, state, yield) {
			return
		}
	}
	if err := scanner.Err(); err != nil && ctx.Err() == nil {
		yield(StreamEvent{Type: EventError, Err: fmt.Errorf("llm.openai: scan: %w", err)})
	}
}

// toolCallState tracks per-chunk tool-call streaming state; synthesizes index for providers that drop it.
//
// toolCallState 跨 chunk 跟踪 tool-call 流式状态；对不填 index 的 provider 按 ID 合成 index。
type toolCallState struct {
	toolNameSent     map[int]bool
	idToSyntheticIdx map[string]int
	nextSyntheticIdx int
}

func newToolCallState() *toolCallState {
	return &toolCallState{
		toolNameSent:     map[int]bool{},
		idToSyntheticIdx: map[string]int{},
	}
}

// resolveIndex returns a stream-local unique index; trusts non-zero index, else uses ID.
//
// resolveIndex 返流内唯一 index；非零 index 直信，零 index 按 ID 区分。
func (s *toolCallState) resolveIndex(tc oaiToolCallDelta) int {
	if tc.Index > 0 {
		return tc.Index
	}
	if tc.ID == "" {
		return 0
	}
	if idx, ok := s.idToSyntheticIdx[tc.ID]; ok {
		return idx
	}
	idx := s.nextSyntheticIdx
	s.idToSyntheticIdx[tc.ID] = idx
	s.nextSyntheticIdx++
	return idx
}

func emitOpenAIChunk(chunk oaiChunk, state *toolCallState, yield func(StreamEvent) bool) bool {
	// TE-23: surface OpenRouter-style mid-stream errors instead of silently terminating.
	// TE-23：检测 OpenRouter 风格流中错误，不静默终止。
	if chunk.Error != nil {
		yield(StreamEvent{
			Type: EventError,
			Err:  fmt.Errorf("%w: in-stream: %s", ErrProviderError, chunk.Error.Message),
		})
		return false
	}
	// Qwen DashScope flat error envelope: {"code":"...","message":"...","request_id":"..."}.
	// These arrive as a 200 SSE chunk with no nested "error" object.
	//
	// Qwen 扁平错误信封以 200 SSE chunk 形式返回，无嵌套 "error" 字段。
	if chunk.Code != "" {
		yield(StreamEvent{
			Type: EventError,
			Err:  fmt.Errorf("%w: qwen: %s: %s", ErrProviderError, chunk.Code, chunk.ErrMsg),
		})
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

	// Emit reasoning before content: CN-family uses reasoning_content; Ollama /v1 uses reasoning.
	// 先 emit reasoning 再 emit content：CN 家族用 reasoning_content，Ollama 用 reasoning（无下划线）。
	reasoningDelta := delta.ReasoningContent
	if reasoningDelta == "" {
		reasoningDelta = delta.Reasoning
	}
	if reasoningDelta != "" {
		if !yield(StreamEvent{Type: EventReasoning, Delta: reasoningDelta}) {
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
		if !state.toolNameSent[idx] && tc.Function.Name != "" {
			state.toolNameSent[idx] = true
			if !yield(StreamEvent{
				Type: EventToolStart, ToolIndex: idx,
				ToolID: tc.ID, ToolName: tc.Function.Name,
			}) {
				return false
			}
		}
		if tc.Function.Arguments != "" {
			if !yield(StreamEvent{
				Type: EventToolDelta, ToolIndex: idx,
				ArgsDelta: tc.Function.Arguments,
			}) {
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

func buildOpenAIBody(req Request) ([]byte, error) {
	// TE-25: sanitize tool_call ↔ tool_result pairing — orphans → 400 lockout.
	// TE-25：sanitize 配对，orphan 会 400 锁对话。
	req.Messages = SanitizeMessages(req.Messages)
	msgs, err := toOpenAIMsgs(req.Messages, req.System)
	if err != nil {
		return nil, err
	}
	body := oaiRequest{
		Model:    req.ModelID,
		Messages: msgs,
		Stream:   !req.DisableStream,
	}
	if !req.DisableStream {
		body.StreamOptions = &oaiStreamOptions{IncludeUsage: true}
	}
	if len(req.Tools) > 0 {
		body.Tools = toOpenAITools(req.Tools)
	}
	return json.Marshal(body)
}

func toOpenAIMsgs(msgs []LLMMessage, system string) ([]oaiMessage, error) {
	var out []oaiMessage
	if system != "" {
		out = append(out, oaiMessage{Role: "system", Content: jsonString(system)})
	}
	for _, m := range msgs {
		om, err := toOpenAIMsg(m)
		if err != nil {
			return nil, err
		}
		out = append(out, om)
	}
	return out, nil
}

func toOpenAIMsg(m LLMMessage) (oaiMessage, error) {
	switch m.Role {
	case RoleUser:
		return buildOpenAIUserMsg(m)
	case RoleAssistant:
		return buildOpenAIAssistantMsg(m), nil
	case RoleTool:
		return oaiMessage{
			Role:       "tool",
			Content:    jsonString(m.Content),
			ToolCallID: m.ToolCallID,
		}, nil
	default:
		return oaiMessage{}, fmt.Errorf("llm.openai: unknown role %q: %w", m.Role, ErrBadRequest)
	}
}

func buildOpenAIUserMsg(m LLMMessage) (oaiMessage, error) {
	if len(m.Parts) == 0 {
		return oaiMessage{Role: "user", Content: jsonString(m.Content)}, nil
	}
	parts := make([]oaiContentPart, 0, len(m.Parts))
	for _, p := range m.Parts {
		switch p.Type {
		case "text":
			parts = append(parts, oaiContentPart{Type: "text", Text: p.Text})
		case "image_url":
			parts = append(parts, oaiContentPart{
				Type: "image_url", ImageURL: &oaiImageURL{URL: p.ImageURL},
			})
		default:
			return oaiMessage{}, fmt.Errorf("llm.openai: unknown part type %q: %w", p.Type, ErrBadRequest)
		}
	}
	raw, err := json.Marshal(parts)
	if err != nil {
		return oaiMessage{}, fmt.Errorf("llm.openai: marshal parts: %w", err)
	}
	return oaiMessage{Role: "user", Content: raw}, nil
}

// buildOpenAIAssistantMsg encodes an assistant LLMMessage; reasoning-only fallback + force-emit content.
//
// buildOpenAIAssistantMsg 编码 assistant 消息；reasoning-only 回退、content 强制 emit。
func buildOpenAIAssistantMsg(m LLMMessage) oaiMessage {
	// TE-23 #1: reasoning-only → copy into content to avoid next-turn 400.
	// TE-23 #1：仅 reasoning_content 时复制到 content 避免下一轮 400。
	if m.Content == "" && len(m.ToolCalls) == 0 && m.ReasoningContent != "" {
		m.Content = m.ReasoningContent
	}

	// TE-23 #2: always emit content (even ""); strict providers reject null.
	// TE-23 #2：content 即使空也 emit ""；严格 provider 拒 null。
	om := oaiMessage{
		Role:             "assistant",
		ReasoningContent: m.ReasoningContent,
		Content:          jsonString(m.Content),
	}
	for _, tc := range m.ToolCalls {
		om.ToolCalls = append(om.ToolCalls, oaiToolCall{
			ID:       tc.ID,
			Type:     "function",
			Function: oaiFuncCall{Name: tc.Name, Arguments: tc.Arguments},
		})
	}
	return om
}

func toOpenAITools(defs []ToolDef) []oaiTool {
	out := make([]oaiTool, len(defs))
	for i, d := range defs {
		out[i] = oaiTool{
			Type:     "function",
			Function: oaiFuncDef(d),
		}
	}
	return out
}

func jsonString(s string) json.RawMessage {
	b, _ := json.Marshal(s)
	return b
}

// classifyHTTPError maps an HTTP status + body to a sentinel-wrapped error.
//
// classifyHTTPError 把 HTTP 状态 + body 映射为带 sentinel 包装的错误。
func classifyHTTPError(status int, body []byte) error {
	msg := strings.TrimSpace(string(body))
	if len(msg) > 200 {
		msg = msg[:200] + "..."
	}
	switch status {
	case http.StatusUnauthorized:
		return fmt.Errorf("%w (401): %s", ErrAuthFailed, msg)
	case http.StatusForbidden:
		return fmt.Errorf("%w (403): %s", ErrAuthFailed, msg)
	case http.StatusTooManyRequests:
		return fmt.Errorf("%w (429): %s", ErrRateLimited, msg)
	case http.StatusBadRequest:
		return fmt.Errorf("%w (400): %s", ErrBadRequest, msg)
	case http.StatusNotFound:
		return fmt.Errorf("%w (404): %s", ErrModelNotFound, msg)
	default:
		return fmt.Errorf("%w (%d): %s", ErrProviderError, status, msg)
	}
}

type oaiRequest struct {
	Model         string            `json:"model"`
	Messages      []oaiMessage      `json:"messages"`
	Tools         []oaiTool         `json:"tools,omitempty"`
	Stream        bool              `json:"stream"`
	StreamOptions *oaiStreamOptions `json:"stream_options,omitempty"`
}

type oaiStreamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

// oaiMessage holds Content as RawMessage to accept either a string or a content-part array.
//
// oaiMessage Content 用 RawMessage，可装字符串或 content-part 数组。
type oaiMessage struct {
	Role             string          `json:"role"`
	Content          json.RawMessage `json:"content,omitempty"`
	ReasoningContent string          `json:"reasoning_content,omitempty"`
	ToolCalls        []oaiToolCall   `json:"tool_calls,omitempty"`
	ToolCallID       string          `json:"tool_call_id,omitempty"`
}

type oaiContentPart struct {
	Type     string       `json:"type"`
	Text     string       `json:"text,omitempty"`
	ImageURL *oaiImageURL `json:"image_url,omitempty"`
}

type oaiImageURL struct {
	URL string `json:"url"`
}

type oaiToolCall struct {
	ID       string      `json:"id"`
	Type     string      `json:"type"`
	Function oaiFuncCall `json:"function"`
}

type oaiFuncCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type oaiTool struct {
	Type     string     `json:"type"`
	Function oaiFuncDef `json:"function"`
}

type oaiFuncDef struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

type oaiChunk struct {
	Choices []oaiChoice    `json:"choices"`
	Usage   *oaiUsage      `json:"usage"`
	Error   *oaiChunkError `json:"error,omitempty"`
	// Qwen flat error envelope: {"code":"...","message":"...","request_id":"..."}.
	// Detected when Error is nil but Code is non-empty (no nested "error" object).
	//
	// Qwen 扁平 error 信封：code/message 在顶层，无 "error" 嵌套。
	Code      string `json:"code,omitempty"`
	ErrMsg    string `json:"message,omitempty"`
	RequestID string `json:"request_id,omitempty"`
}

type oaiChunkError struct {
	Message string `json:"message"`
	Code    any    `json:"code,omitempty"`
	Type    string `json:"type,omitempty"`
}

type oaiNonStreamResponse struct {
	Choices []oaiNonStreamChoice `json:"choices"`
	Usage   *oaiUsage            `json:"usage"`
	Error   *oaiChunkError       `json:"error,omitempty"`
}

type oaiNonStreamChoice struct {
	Message      oaiNonStreamMessage `json:"message"`
	FinishReason string              `json:"finish_reason"`
}

type oaiNonStreamMessage struct {
	Role string `json:"role"`
	// reasoning is Ollama /v1's field name; reasoning_content is the CN-family name.
	//
	// reasoning 是 Ollama /v1 用的字段名；reasoning_content 是 CN 家族用的。
	Content          string             `json:"content"`
	ReasoningContent string             `json:"reasoning_content"`
	Reasoning        string             `json:"reasoning"`
	ToolCalls        []oaiToolCallDelta `json:"tool_calls"`
}

type oaiChoice struct {
	Delta        oaiDelta `json:"delta"`
	FinishReason string   `json:"finish_reason"`
}

type oaiDelta struct {
	Content string `json:"content"`
	// reasoning_content is used by DeepSeek, Qwen, Zhipu, Moonshot, Doubao (CN family).
	// reasoning is used by Ollama /v1 (no underscore — different field name).
	//
	// reasoning_content 是 CN 家族（DeepSeek/Qwen/Zhipu 等）用的字段名；
	// Ollama /v1 用 reasoning（无下划线）。两者均映射到 EventReasoning。
	ReasoningContent string             `json:"reasoning_content"`
	Reasoning        string             `json:"reasoning"`
	ToolCalls        []oaiToolCallDelta `json:"tool_calls"`
}

type oaiToolCallDelta struct {
	Index    int          `json:"index"`
	ID       string       `json:"id"`
	Function oaiFuncDelta `json:"function"`
}

type oaiFuncDelta struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type oaiUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
}
