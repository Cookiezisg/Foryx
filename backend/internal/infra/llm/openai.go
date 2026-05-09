// openai.go — OpenAI-compatible streaming client.
// Covers: OpenAI, DeepSeek, Qwen, Moonshot, Ollama (/v1 endpoint), and any
// provider that speaks the OpenAI chat-completions wire format.
//
// openai.go — OpenAI 兼容流式客户端。
// 覆盖：OpenAI / DeepSeek / Qwen / Moonshot / Ollama 及所有兼容 OpenAI
// chat-completions 协议的 provider。
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
	"time"
)

// openAIClient implements Client for all OpenAI-compatible providers.
//
// openAIClient 为所有 OpenAI 兼容 provider 实现 Client 接口。
type openAIClient struct {
	http *http.Client
}

func newOpenAIClient() *openAIClient {
	return &openAIClient{
		http: &http.Client{Timeout: 120 * time.Second},
	}
}

// Stream sends a streaming chat-completions request and returns an iter.Seq
// of typed StreamEvents. Break out of the loop to stop early; context
// cancellation also terminates iteration cleanly.
//
// Stream 发起流式 chat-completions 请求，返回类型化 StreamEvent 的 iter.Seq。
// break 可提前退出，ctx 取消时迭代干净终止。
func (c *openAIClient) Stream(ctx context.Context, req Request) iter.Seq[StreamEvent] {
	return func(yield func(StreamEvent) bool) {
		body, err := buildOpenAIBody(req)
		if err != nil {
			yield(StreamEvent{Type: EventError, Err: fmt.Errorf("llm/openai: build body: %w", err)})
			return
		}

		httpReq, err := http.NewRequestWithContext(
			ctx, http.MethodPost, req.BaseURL+"/chat/completions", bytes.NewReader(body))
		if err != nil {
			yield(StreamEvent{Type: EventError, Err: fmt.Errorf("llm/openai: new request: %w", err)})
			return
		}
		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("Authorization", "Bearer "+req.Key)

		resp, err := c.http.Do(httpReq)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			yield(StreamEvent{Type: EventError, Err: fmt.Errorf("llm/openai: do: %w", err)})
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
			yield(StreamEvent{Type: EventError, Err: classifyHTTPError(resp.StatusCode, raw)})
			return
		}

		// TE-24: req.DisableStream=true forces non-streaming reception
		// (Ollama-with-tools quirk: ollama #12557 / #9632 silently drop
		// tool_calls when streaming is on). The OpenAI client handles
		// both shapes here so callers see the same StreamEvent sequence
		// regardless of wire mode.
		// req.DisableStream=true 走非流式接收（Ollama+tools quirk）。
		if req.DisableStream {
			parseOpenAINonStreaming(resp.Body, yield)
		} else {
			parseOpenAISSE(ctx, resp.Body, yield)
		}
	}
}

// parseOpenAINonStreaming reads a single non-streaming JSON response and
// synthesizes a StreamEvent sequence equivalent to what parseOpenAISSE
// would emit for the same content. Lets the rest of the system (chat
// runner, loop, etc.) treat both wire modes identically.
//
// parseOpenAINonStreaming 读单条非流式 JSON 响应并合成 StreamEvent 序列，
// 让系统其它部分（chat / loop）对两种 wire mode 一视同仁。
func parseOpenAINonStreaming(body io.Reader, yield func(StreamEvent) bool) {
	raw, err := io.ReadAll(io.LimitReader(body, 8<<20)) // 8 MiB cap
	if err != nil {
		yield(StreamEvent{Type: EventError, Err: fmt.Errorf("llm/openai: read non-streaming body: %w", err)})
		return
	}
	var resp oaiNonStreamResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		yield(StreamEvent{Type: EventError, Err: fmt.Errorf("llm/openai: parse non-streaming response: %w", err)})
		return
	}
	if resp.Error != nil {
		yield(StreamEvent{Type: EventError, Err: fmt.Errorf("llm: provider returned error: %s", resp.Error.Message)})
		return
	}
	if len(resp.Choices) == 0 {
		yield(StreamEvent{Type: EventError, Err: fmt.Errorf("llm/openai: non-streaming response has no choices")})
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

// ── SSE parser ────────────────────────────────────────────────────────────────

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
			continue // keep-alive or empty line — not an error
		}
		var chunk oaiChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			yield(StreamEvent{Type: EventError, Err: fmt.Errorf("llm/openai: malformed SSE chunk: %w", err)})
			return
		}
		if !emitOpenAIChunk(chunk, state, yield) {
			return
		}
	}
	if err := scanner.Err(); err != nil && ctx.Err() == nil {
		yield(StreamEvent{Type: EventError, Err: fmt.Errorf("llm/openai: scan: %w", err)})
	}
}

// emitOpenAIChunk converts one parsed SSE chunk into StreamEvents.
// Returns false when the consumer signals stop.
//
// emitOpenAIChunk 把一个解析好的 SSE chunk 转换为 StreamEvent 发出。
// consumer 发出停止信号时返回 false。
// toolCallState tracks tool-call streaming state across chunks. Holds:
//   - toolNameSent: which (resolved) tool index has had its EventToolStart emitted
//   - idToSyntheticIdx + nextSyntheticIdx: TE-24 fallback for providers
//     (Ollama, some Gemini paths) that leave tool_calls[].index at 0
//     for every parallel tool call. Without this, second + third tool
//     calls collide on key 0 and get silently merged.
//
// toolCallState 跨 chunk 跟踪 tool-call 流式状态。
// idToSyntheticIdx 给 Ollama 等不填 index 的 provider 兜底，按 ID 分配 index。
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

// resolveIndex returns a unique stream-local index for the tool call.
// Trusts a real non-zero index when present; for index=0 with an ID,
// disambiguates same-vs-different tool by ID; for index=0 without an ID,
// passes through as 0 (best-effort — the provider gave us nothing to
// disambiguate with).
//
// resolveIndex 返流内唯一 index。真 index 非零时信任；index=0 且有 ID 时
// 按 ID 区分；index=0 且无 ID 时 fallback 0（provider 没给可区分信号）。
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
	// TE-23: OpenRouter (and any OpenAI-compat provider that forwards
	// upstream errors mid-stream) embeds errors as a top-level Error
	// field on the chunk while HTTP status stays 200. Without this
	// detection the stream would just terminate silently. Fire EventError
	// so the chat layer surfaces it to the user instead of showing an
	// empty assistant reply.
	//
	// TE-23：OpenRouter（以及任何在流中透传上游错误的 OpenAI-compat
	// provider）把错误嵌入 chunk 的顶层 Error 字段，HTTP 状态保持 200。
	// 没有这个检测流就静默终止。fire EventError 让 chat 层暴露给用户，
	// 而不是显示空白 assistant 回复。
	if chunk.Error != nil {
		// In-stream provider error (TE-23 OpenRouter pattern). Wrap
		// with ErrProviderError sentinel so callers can discriminate
		// the same way as classifyHTTPError above.
		// in-stream provider 错误用 ErrProviderError sentinel %w 包装，
		// 让调用方与 classifyHTTPError 同样可 errors.Is 区分。
		yield(StreamEvent{
			Type: EventError,
			Err:  fmt.Errorf("%w: in-stream: %s", ErrProviderError, chunk.Error.Message),
		})
		return false
	}
	if len(chunk.Choices) == 0 {
		// Usage-only chunk — some providers send this as the final event.
		// 仅含 usage 的 chunk，某些 provider 在流末单独发送。
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

	if delta.Content != "" {
		if !yield(StreamEvent{Type: EventText, Delta: delta.Content}) {
			return false
		}
	}
	if delta.ReasoningContent != "" {
		if !yield(StreamEvent{Type: EventReasoning, Delta: delta.ReasoningContent}) {
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

// ── Request builder ───────────────────────────────────────────────────────────

func buildOpenAIBody(req Request) ([]byte, error) {
	// TE-25: enforce protocol invariants (tool_call ↔ tool_result pairing)
	// before encoding. Orphan tool blocks would otherwise lock the entire
	// conversation in a 400 trap. See sanitizer.go for the failure
	// scenarios this guards against.
	// TE-25：编码前过 sanitizer，防 orphan tool block 把对话永久锁死。
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
	// stream_options.include_usage only applies to streaming requests.
	// 仅流式请求才设 stream_options.include_usage。
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
		return oaiMessage{}, fmt.Errorf("llm/openai: unknown role %q", m.Role)
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
			return oaiMessage{}, fmt.Errorf("llm/openai: unknown part type %q", p.Type)
		}
	}
	raw, err := json.Marshal(parts)
	if err != nil {
		return oaiMessage{}, fmt.Errorf("llm/openai: marshal parts: %w", err)
	}
	return oaiMessage{Role: "user", Content: raw}, nil
}

// buildOpenAIAssistantMsg encodes one LLMMessage (assistant role) to the
// OpenAI wire shape. Two protocol-baseline robustness fixes (TE-23):
//
//  1. Reasoning-only fallback (originally TE-22, lived in app/loop/history.go,
//     moved here): when the message has only reasoning_content with no
//     content and no tool_calls (DeepSeek V3.x reasoning mode quirk —
//     occasionally emits the user-facing reply entirely via
//     reasoning_content), copy reasoning_content into content. Without this,
//     the next turn's history trips HTTP 400:
//       "Invalid assistant message: content or tool_calls must be set"
//     This trades the wire-level reasoning/content distinction for keeping
//     the conversation alive — the alternative is an unrecoverable 400
//     that can't be cleared without manually editing the DB.
//
//  2. Force-emit content even when empty: if assistant has tool_calls but
//     no text, OpenAI / GLM / strict providers reject `content: null` (the
//     omitempty default behavior). Always emit `""` so the JSON shape is
//     valid even when there's no text to send.
//
// Both fixes apply to all OpenAI-compat providers, not specific ones —
// they live here (the OpenAI baseline client) rather than in adapter.go
// (per-provider quirks).
//
// buildOpenAIAssistantMsg 把一条 assistant LLMMessage 编码到 OpenAI wire
// 形状。TE-23 的两个协议基线 robust 修复：
//
//  1. reasoning-only fallback（原 TE-22 在 app/loop/history.go，搬到这里）：
//     仅 reasoning_content + 无 content + 无 tool_calls 时（DeepSeek V3.x
//     reasoning 模式偶发把回复全放 reasoning_content），把 reasoning_content
//     拷给 content。否则下一轮 history 400 锁死对话。
//
//  2. content 即使为空也强制 emit `""`：assistant 有 tool_calls 但无文字时，
//     OpenAI / GLM 等严格 provider 拒 `content: null`（omitempty 默认行为）。
//     总是 emit `""` 让 JSON shape 合法。
//
// 两个修复对所有 OpenAI-compat provider 都适用，故住 baseline client 而非
// adapter.go（per-provider quirk）。
func buildOpenAIAssistantMsg(m LLMMessage) oaiMessage {
	// TE-23 fix #1: reasoning-only fallback.
	if m.Content == "" && len(m.ToolCalls) == 0 && m.ReasoningContent != "" {
		m.Content = m.ReasoningContent
	}

	om := oaiMessage{
		Role:             "assistant",
		ReasoningContent: m.ReasoningContent,
		// TE-23 fix #2: always emit content (even empty string), never let
		// omitempty drop it. Strict providers require content || tool_calls
		// to be set; an empty string satisfies the "set" check, null does not.
		Content: jsonString(m.Content),
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
			Function: oaiFuncDef(d), // ToolDef and oaiFuncDef have identical fields; tags ignored by conversion
		}
	}
	return out
}

// jsonString returns the JSON encoding of a Go string (a quoted JSON string).
// Used to produce json.RawMessage values for string-typed content fields.
//
// jsonString 返回 Go 字符串的 JSON 编码（带引号的 JSON 字符串）。
func jsonString(s string) json.RawMessage {
	b, _ := json.Marshal(s)
	return b
}

// ── HTTP error classification ─────────────────────────────────────────────────

// classifyHTTPError maps HTTP status codes to descriptive errors.
// The raw response body is included for debugging. Each branch wraps
// a sentinel (llm.ErrAuthFailed / ErrRateLimited / ErrBadRequest /
// ErrModelNotFound / ErrProviderError) so callers can discriminate
// via errors.Is — most importantly, 401 detection lights up the
// apikey.MarkInvalid path so a stale API key flips to "error" status
// in the UI rather than silently producing 500s on every call.
//
// classifyHTTPError 把 HTTP 状态码映射为描述性错误。每分支用 sentinel
// %w 包装让调用方可 errors.Is 区分——最重要：401 检测让
// apikey.MarkInvalid 链路点亮，过期 key 翻 UI "error" 状态而非每次调
// 都静默 500。
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

// ── Wire types ────────────────────────────────────────────────────────────────

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

// oaiMessage uses json.RawMessage for Content so it can be either a quoted
// string or a JSON array of content parts without a custom marshaler.
//
// oaiMessage 用 json.RawMessage 存 Content，无需自定义 marshaler 即可兼容
// 字符串和 content parts 数组两种格式。
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
	Choices []oaiChoice `json:"choices"`
	Usage   *oaiUsage   `json:"usage"`
	// Error is the OpenRouter quirk: once any byte streams the HTTP status
	// is locked at 200, but a downstream provider error arrives as an
	// in-stream SSE chunk with this field populated and choices empty.
	// Without this field declaration the chunk would silently parse to
	// {Choices: nil, Usage: nil} and the stream would terminate without
	// surfacing the error. TE-23 added this + the matching detection in
	// emitOpenAIChunk.
	//
	// Error 是 OpenRouter quirk：流开始后 HTTP 状态码锁 200，下游 provider
	// 错误以 SSE chunk 形式抵达，本字段填充而 choices 为空。无此字段则
	// chunk 静默 parse 为空 → 流终止但用户看不到错。TE-23 加此字段 +
	// emitOpenAIChunk 内的检测。
	Error *oaiChunkError `json:"error,omitempty"`
}

type oaiChunkError struct {
	Message string `json:"message"`
	Code    any    `json:"code,omitempty"` // OpenRouter sometimes int, sometimes string
	Type    string `json:"type,omitempty"`
}

// oaiNonStreamResponse is the single-shot JSON shape returned when
// stream:false. Only used when req.DisableStream=true (currently:
// Ollama-with-tools per ollamaAdapter.BeforeRequest).
//
// oaiNonStreamResponse 是 stream:false 时的单条 JSON 形状。仅 DisableStream
// 路径用（当前：ollamaAdapter 在有 tools 时设）。
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
	Role             string             `json:"role"`
	Content          string             `json:"content"`
	ReasoningContent string             `json:"reasoning_content"`
	ToolCalls        []oaiToolCallDelta `json:"tool_calls"`
}

type oaiChoice struct {
	Delta        oaiDelta `json:"delta"`
	FinishReason string   `json:"finish_reason"`
}

type oaiDelta struct {
	Content          string             `json:"content"`
	ReasoningContent string             `json:"reasoning_content"`
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
