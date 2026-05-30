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
// thinkingEncoder is an optional hook for encoding ThinkingSpec into the wire
// body; nil = emit no thinking fields (default behavior — critical for P3).
//
// openAICompatProvider 是所有 /chat/completions provider 共用的 OpenAI-compat
// wire 方言。body/SSE 逻辑只此一份；per-provider 身份（name + base URL）注入。
// beforeRequest 是可选的 per-provider Request 变换钩子，在 BuildRequest 前执行。
// thinkingEncoder 是可选的 thinking 编码钩子；nil = 不发 thinking 字段（默认）。
type openAICompatProvider struct {
	name            string
	defaultBaseURL  string
	beforeRequest   func(*Request)                    // nil if no per-provider mutation needed
	thinkingEncoder func(*oaiRequest, *ThinkingSpec)  // nil = no thinking fields
}

func newOpenAICompatProvider(name, defaultBaseURL string) *openAICompatProvider {
	return &openAICompatProvider{name: name, defaultBaseURL: defaultBaseURL}
}

func (p *openAICompatProvider) Name() string           { return p.name }
func (p *openAICompatProvider) DefaultBaseURL() string { return p.defaultBaseURL }

func (p *openAICompatProvider) BuildRequest(ctx context.Context, req Request) (*http.Request, error) {
	body, err := buildOpenAIBody(req, p.thinkingEncoder)
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

func buildOpenAIBody(req Request, thinkingEncoder func(*oaiRequest, *ThinkingSpec)) ([]byte, error) {
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
	// Apply per-provider thinking encoding when spec is non-nil and encoder is
	// registered. nil spec = auto = no-op (byte-identical to old behaviour).
	// Qwen guard: enable_thinking=true requires stream:true — skip encoding if
	// the request is already forced to non-streaming (DisableStream=true).
	//
	// spec 非 nil 且 encoder 已注册时编码 thinking；spec=nil 即 auto，不发任何字段。
	// Qwen 守卫：enable_thinking=true 必须 stream=true；非流式时跳过编码。
	if req.Thinking != nil && thinkingEncoder != nil && req.Thinking.Mode != "auto" {
		if !(req.DisableStream && req.Thinking.Mode == "on") {
			thinkingEncoder(&body, req.Thinking)
		}
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

	// Per-provider thinking fields — at most one provider populates a given field
	// per request. Groups that share a JSON key use the same struct type.
	// 各 provider 的 thinking 字段；每次请求每个 JSON 字段至多一个 provider 填。

	// openai / google-compat / ollama: reasoning_effort string
	ReasoningEffort string `json:"reasoning_effort,omitempty"`
	// deepseek / zhipu / moonshot / doubao: thinking:{type:..., budget_tokens?}
	Thinking *oaiThinkingField `json:"thinking,omitempty"`
	// deepseek (V4): top-level reasoning_effort string (separate from openai's)
	// Note: reuses ReasoningEffort field — DeepSeek sends it alongside Thinking.
	// qwen: top-level enable_thinking bool (pointer to distinguish false vs absent)
	EnableThinking *bool `json:"enable_thinking,omitempty"`
	// qwen: thinking_budget int (only when enable_thinking=true and budget>0)
	ThinkingBudget int `json:"thinking_budget,omitempty"`
	// openrouter: reasoning:{effort|max_tokens,...}
	Reasoning *oaiOpenRouterReasoning `json:"reasoning,omitempty"`
}

// oaiThinkingField is the shared thinking object used by DeepSeek, Zhipu,
// Moonshot, and Doubao. BudgetTokens is only populated by Doubao.
//
// oaiThinkingField 是 DeepSeek / Zhipu / Moonshot / Doubao 共用的 thinking 对象；
// BudgetTokens 只由豆包填。
type oaiThinkingField struct {
	Type         string `json:"type"`
	BudgetTokens int    `json:"budget_tokens,omitempty"`
}

// oaiOpenRouterReasoning is OpenRouter's top-level reasoning object.
// Effort and MaxTokens are mutually exclusive; prefer Effort when both are set.
//
// oaiOpenRouterReasoning 是 OpenRouter 的顶层 reasoning 对象；
// Effort 与 MaxTokens 互斥，两者同时设置优先用 Effort。
type oaiOpenRouterReasoning struct {
	Effort    string `json:"effort,omitempty"`
	MaxTokens int    `json:"max_tokens,omitempty"`
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

// ── Per-provider thinking encoders ───────────────────────────────────────────
//
// Each encoder is registered as openAICompatProvider.thinkingEncoder.
// Called only when ThinkingSpec is non-nil and Mode != "auto".
// Mode="off" emits the provider's explicit-disable form.
// Mode="on"  emits the provider's enable form with Effort/Budget.
//
// 每个 encoder 注册为 openAICompatProvider.thinkingEncoder。
// 仅在 ThinkingSpec 非 nil 且 Mode != "auto" 时调用。

// clampEffort returns spec.Effort if it appears in allowed; otherwise returns
// fallback. Empty spec.Effort also returns fallback.
//
// clampEffort 若 spec.Effort 在 allowed 列表则返回它，否则返 fallback。
func clampEffort(effort string, allowed []string, fallback string) string {
	if effort == "" {
		return fallback
	}
	for _, v := range allowed {
		if v == effort {
			return effort
		}
	}
	return fallback
}

// encodeThinkingOpenAI encodes thinking for OpenAI reasoning models.
// on  → reasoning_effort = Effort (clamp to allowed; default "medium")
// off → reasoning_effort = "none" if "none" is in allowed, else omit.
//
// encodeThinkingOpenAI 编码 OpenAI 推理参数：
// on→reasoning_effort（取 Effort 或 "medium"）；off→"none"（若可用）。
func encodeThinkingOpenAI(allowed []string) func(*oaiRequest, *ThinkingSpec) {
	return func(body *oaiRequest, spec *ThinkingSpec) {
		switch spec.Mode {
		case "on":
			body.ReasoningEffort = clampEffort(spec.Effort, allowed, "medium")
		case "off":
			// Only emit "none" if the model family supports it; otherwise omit.
			// 只有当模型家族支持 "none" 时才 emit；否则省略。
			for _, v := range allowed {
				if v == "none" {
					body.ReasoningEffort = "none"
					break
				}
			}
		}
	}
}

// deepseekMapEffort maps generic effort values to DeepSeek's {high,max} set.
//
// deepseekMapEffort 把通用 effort 映射到 DeepSeek 的 {high,max}。
func deepseekMapEffort(effort string) string {
	switch effort {
	case "max", "xhigh":
		return "max"
	default:
		// low, medium, high, empty → high
		return "high"
	}
}

// encodeThinkingQwen encodes thinking for Qwen DashScope.
// on  → enable_thinking=true (+thinking_budget if spec.Budget>0)
// off → enable_thinking=false
// GUARD: if DisableStream=true and Mode=on, skip encoding (Qwen requires stream
// for enable_thinking=true; callers should not set DisableStream+thinking:on).
// The guard is applied in the calling context (buildOpenAIBody checks
// req.DisableStream before invoking the encoder).
//
// encodeThinkingQwen 编码 Qwen thinking 参数：
// on→enable_thinking=true（+budget）；off→false。
// 流式守卫：非流式请求跳过 enable_thinking=true（Qwen 要求 stream）。
func encodeThinkingQwen(body *oaiRequest, spec *ThinkingSpec) {
	switch spec.Mode {
	case "on":
		t := true
		body.EnableThinking = &t
		if spec.Budget > 0 {
			body.ThinkingBudget = spec.Budget
		}
	case "off":
		f := false
		body.EnableThinking = &f
	}
}

// encodeThinkingZhipu encodes thinking for Zhipu GLM.
// on  → thinking:{type:"enabled"}
// off → thinking:{type:"disabled"}
//
// encodeThinkingZhipu 编码 Zhipu GLM thinking 参数。
func encodeThinkingZhipu(body *oaiRequest, spec *ThinkingSpec) {
	switch spec.Mode {
	case "on":
		body.Thinking = &oaiThinkingField{Type: "enabled"}
	case "off":
		body.Thinking = &oaiThinkingField{Type: "disabled"}
	}
}

// encodeThinkingMoonshot encodes thinking for Moonshot kimi-k2.5/k2.6.
// on  → thinking:{type:"enabled"}
// off → thinking:{type:"disabled"}
// Note: kimi-k2-thinking model-id needs no param; the caller decides whether
// to pass a ThinkingSpec at all for that model.
//
// encodeThinkingMoonshot 编码 Moonshot kimi-k2.5/6 thinking 参数；
// kimi-k2-thinking 模型 id 本身内禀 thinking，不传 ThinkingSpec 即可。
func encodeThinkingMoonshot(body *oaiRequest, spec *ThinkingSpec) {
	switch spec.Mode {
	case "on":
		body.Thinking = &oaiThinkingField{Type: "enabled"}
	case "off":
		body.Thinking = &oaiThinkingField{Type: "disabled"}
	}
}

// encodeThinkingDoubao encodes thinking for Doubao Seed models.
// on  → thinking:{type:"enabled", budget_tokens?}
// off → thinking:{type:"disabled"}
//
// encodeThinkingDoubao 编码豆包 Seed 模型 thinking 参数。
func encodeThinkingDoubao(body *oaiRequest, spec *ThinkingSpec) {
	switch spec.Mode {
	case "on":
		tf := &oaiThinkingField{Type: "enabled"}
		if spec.Budget > 0 {
			tf.BudgetTokens = spec.Budget
		}
		body.Thinking = tf
	case "off":
		body.Thinking = &oaiThinkingField{Type: "disabled"}
	}
}

// encodeThinkingOpenRouter encodes thinking for OpenRouter.
// on  → reasoning:{effort:Effort} or reasoning:{max_tokens:Budget} (effort preferred)
// off → omit (no clean "off" documented; leaving reasoning absent lets the
//
//	upstream model use its default — emitting nothing is safer than a
//	non-standard field).
//
// encodeThinkingOpenRouter 编码 OpenRouter reasoning 参数：
// on→{effort} 或 {max_tokens}（effort 优先）；off→省略（无官方关闭形，不发更安全）。
func encodeThinkingOpenRouter(body *oaiRequest, spec *ThinkingSpec) {
	if spec.Mode != "on" {
		return // off: omit (no documented disable form)
	}
	r := &oaiOpenRouterReasoning{}
	if spec.Effort != "" {
		r.Effort = spec.Effort
	} else if spec.Budget > 0 {
		r.MaxTokens = spec.Budget
	} else {
		r.Effort = "medium" // default
	}
	body.Reasoning = r
}

// encodeThinkingGeminiCompat encodes thinking for Gemini OpenAI-compat surface.
// Same as OpenAI: reasoning_effort string.
//
// encodeThinkingGeminiCompat 编码 Gemini compat 面的 thinking（与 OpenAI 相同）。
func encodeThinkingGeminiCompat(allowed []string) func(*oaiRequest, *ThinkingSpec) {
	return encodeThinkingOpenAI(allowed)
}

// ── Self-contained openaiProvider ────────────────────────────────────────────
//
// openaiProvider speaks OpenAI's /chat/completions standard directly. It owns
// its own BuildRequest (including reasoning_effort for reasoning models per
// 03 §2) and ParseStream (OpenAI SSE → StreamEvents). All logic is written to
// OpenAI's documented API — no shared mega-parser.
//
// openaiProvider 直接按 OpenAI /chat/completions 标准实现。自有 BuildRequest
// （含 03 §2 的 reasoning_effort 编码）和 ParseStream（OpenAI SSE→StreamEvent）；
// 逻辑完全基于 OpenAI 官方文档，不依赖共享 mega-parser。

type openaiProvider struct{}

func newOpenAIProvider() *openaiProvider { return &openaiProvider{} }

func (p *openaiProvider) Name() string           { return "openai" }
func (p *openaiProvider) DefaultBaseURL() string { return "https://api.openai.com/v1" }

// BuildRequest encodes a Request into an OpenAI /chat/completions HTTP request.
// Reasoning models (o-series) accept reasoning_effort; standard models ignore it.
// Auth: Bearer token in Authorization header. URL: base + /chat/completions.
//
// BuildRequest 把 Request 编码为 OpenAI /chat/completions HTTP 请求。
// 推理模型（o 系列）接受 reasoning_effort；标准模型忽略。
func (p *openaiProvider) BuildRequest(ctx context.Context, req Request) (*http.Request, error) {
	req.Messages = SanitizeMessages(req.Messages)
	msgs, err := toOpenAIMsgs(req.Messages, req.System)
	if err != nil {
		return nil, fmt.Errorf("llm.openai: build messages: %w", err)
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
	// reasoning_effort for o-series reasoning models (03 §2).
	// on  → reasoning_effort = Effort clamp to {none,minimal,low,medium,high,xhigh}; default medium
	// off → reasoning_effort = "none"
	// nil/auto → omit (byte-identical to pre-P3 behaviour)
	//
	// o 系列推理模型的 reasoning_effort（03 §2）：
	// on→clamp effort（默认 medium）；off→"none"；nil/auto→省略。
	if req.Thinking != nil && req.Thinking.Mode != "auto" {
		openAIAllowed := []string{"none", "minimal", "low", "medium", "high", "xhigh"}
		switch req.Thinking.Mode {
		case "on":
			body.ReasoningEffort = clampEffort(req.Thinking.Effort, openAIAllowed, "medium")
		case "off":
			body.ReasoningEffort = "none"
		}
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("llm.openai: marshal body: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(
		ctx, http.MethodPost, req.BaseURL+"/chat/completions", bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("llm.openai: new request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+req.Key)
	return httpReq, nil
}

// ParseStream reads OpenAI SSE chunks and yields StreamEvents.
// Uses the shared transport-level scanSSELines for the raw line mechanics.
//
// ParseStream 读 OpenAI SSE chunk 并 yield StreamEvent。
// 用共享的 scanSSELines 处理原始 SSE 行语义。
func (p *openaiProvider) ParseStream(ctx context.Context, resp *http.Response, req Request) iter.Seq[StreamEvent] {
	return func(yield func(StreamEvent) bool) {
		if req.DisableStream {
			parseOpenAINonStreaming(resp.Body, yield)
			return
		}
		state := newToolCallState()
		scanErr := scanSSELines(resp.Body, func(payload []byte) bool {
			if ctx.Err() != nil {
				return false
			}
			var chunk oaiChunk
			if err := json.Unmarshal(payload, &chunk); err != nil {
				yield(StreamEvent{Type: EventError, Err: fmt.Errorf("llm.openai: malformed SSE chunk: %w", err)})
				return false
			}
			return emitOpenAIChunk(chunk, state, yield)
		})
		if scanErr != nil && ctx.Err() == nil {
			yield(StreamEvent{Type: EventError, Err: fmt.Errorf("llm.openai: scan: %w", scanErr)})
		}
	}
}

// encodeThinkingOllama encodes thinking for Ollama /v1.
// on  → reasoning_effort = Effort (clamp to {high,medium,low,none}; default "medium")
// off → reasoning_effort = "none"
//
// encodeThinkingOllama 编码 Ollama /v1 thinking：
// on→reasoning_effort（clamp）；off→"none"。
func encodeThinkingOllama(body *oaiRequest, spec *ThinkingSpec) {
	allowed := []string{"high", "medium", "low", "none"}
	switch spec.Mode {
	case "on":
		body.ReasoningEffort = clampEffort(spec.Effort, allowed, "medium")
	case "off":
		body.ReasoningEffort = "none"
	}
}
