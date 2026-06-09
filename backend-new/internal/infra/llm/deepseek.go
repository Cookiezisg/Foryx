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

// deepseekProvider speaks DeepSeek's /chat/completions API, fully self-contained: its own
// wire types, message encoding, and SSE chunk parsing — no sharing with the openai
// provider even though the wire is OpenAI-shaped. DeepSeek specifics: the reasoning_content
// round-trip rule, thinking:{type} + reasoning_effort encoding, and reasoning_content
// arriving before content in the stream.
//
// deepseekProvider 完整自包含地讲 DeepSeek /chat/completions：自己的 wire 类型、消息编码、
// SSE 解析——即使 wire 是 OpenAI 形状也不与 openai 共享。DeepSeek 特有：reasoning_content
// round-trip 规则、thinking:{type}+reasoning_effort 编码、流中 reasoning_content 先于 content。
type deepseekProvider struct{}

func newDeepSeekProvider() *deepseekProvider { return &deepseekProvider{} }

func (p *deepseekProvider) Name() string           { return "deepseek" }
func (p *deepseekProvider) DefaultBaseURL() string { return "https://api.deepseek.com" }

// BuildRequest encodes a Request into a DeepSeek /chat/completions HTTP request.
//
// reasoning_content round-trip rule: plain assistant turns (no tool_calls) must strip
// reasoning_content (DeepSeek rejects it on a continuation that carries no tool response);
// tool-call turns preserve it (V3.2+ reconstructs the chain-of-thought from it).
//
// Native knobs from Options: thinking ("enabled"/"disabled") + reasoning_effort ("high"/"max",
// DeepSeek's only two native levels).
//
// BuildRequest 把 Request 编码为 DeepSeek 请求。reasoning_content round-trip 规则：纯文字
// turn 剥、含 tool_calls turn 保留（V4）。原生旋钮取自 Options：thinking + reasoning_effort（仅 high/max）。
func (p *deepseekProvider) BuildRequest(ctx context.Context, req Request) (*http.Request, error) {
	for i := range req.Messages {
		m := &req.Messages[i]
		if m.Role == RoleAssistant && len(m.ToolCalls) == 0 {
			m.ReasoningContent = ""
		}
	}

	req.Messages = SanitizeMessages(req.Messages)
	msgs, err := toDeepSeekMsgs(req.Messages, req.System)
	if err != nil {
		return nil, fmt.Errorf("llm.deepseek: build messages: %w", err)
	}
	body := dsRequest{
		Model:    req.ModelID,
		Messages: msgs,
		Stream:   !req.DisableStream,
	}
	if !req.DisableStream {
		body.StreamOptions = &dsStreamOptions{IncludeUsage: true}
	}
	if len(req.Tools) > 0 {
		body.Tools = toDeepSeekTools(req.Tools)
	}
	if req.MaxTokens > 0 {
		body.MaxTokens = req.MaxTokens
	}
	if v := req.Options["thinking"]; v != "" {
		body.Thinking = &dsThinking{Type: v}
	}
	if v := req.Options["reasoning_effort"]; v != "" {
		body.ReasoningEffort = v
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("llm.deepseek: marshal body: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(
		ctx, http.MethodPost, req.BaseURL+"/chat/completions", bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("llm.deepseek: new request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+req.Key)
	return httpReq, nil
}

func (p *deepseekProvider) ParseStream(ctx context.Context, resp *http.Response, req Request) iter.Seq[StreamEvent] {
	return func(yield func(StreamEvent) bool) {
		if req.DisableStream {
			parseDeepSeekNonStreaming(resp.Body, yield)
			return
		}
		state := newDeepSeekToolState()
		scanErr := scanSSELines(resp.Body, func(payload []byte) bool {
			if ctx.Err() != nil {
				return false
			}
			var chunk dsChunk
			if err := json.Unmarshal(payload, &chunk); err != nil {
				yield(StreamEvent{Type: EventError, Err: fmt.Errorf("llm.deepseek: malformed SSE chunk: %w", err)})
				return false
			}
			return emitDeepSeekChunk(chunk, state, yield)
		})
		if scanErr != nil && ctx.Err() == nil {
			yield(StreamEvent{Type: EventError, Err: fmt.Errorf("llm.deepseek: scan: %w", scanErr)})
		}
	}
}

func emitDeepSeekChunk(chunk dsChunk, state *dsToolState, yield func(StreamEvent) bool) bool {
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

	// DeepSeek sends reasoning_content before content — preserve that order.
	// DeepSeek 先发 reasoning_content 再发 content——严格保序。
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

func parseDeepSeekNonStreaming(body io.Reader, yield func(StreamEvent) bool) {
	raw, err := io.ReadAll(io.LimitReader(body, 8<<20))
	if err != nil {
		yield(StreamEvent{Type: EventError, Err: fmt.Errorf("llm.deepseek: read non-streaming body: %w", err)})
		return
	}
	var resp dsNonStreamResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		yield(StreamEvent{Type: EventError, Err: fmt.Errorf("llm.deepseek: parse non-streaming response: %w", err)})
		return
	}
	if resp.Error != nil {
		yield(StreamEvent{Type: EventError, Err: fmt.Errorf("%w: %s", ErrProviderError, resp.Error.Message)})
		return
	}
	if len(resp.Choices) == 0 {
		yield(StreamEvent{Type: EventError, Err: fmt.Errorf("llm.deepseek: non-streaming response has no choices: %w", ErrProviderError)})
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

func toDeepSeekMsgs(msgs []LLMMessage, system string) ([]dsMessage, error) {
	var out []dsMessage
	if system != "" {
		out = append(out, dsMessage{Role: "system", Content: dsJSONString(system)})
	}
	for _, m := range msgs {
		dm, err := toDeepSeekMsg(m)
		if err != nil {
			return nil, err
		}
		out = append(out, dm)
	}
	return out, nil
}

func toDeepSeekMsg(m LLMMessage) (dsMessage, error) {
	switch m.Role {
	case RoleUser:
		return buildDeepSeekUserMsg(m)
	case RoleAssistant:
		return buildDeepSeekAssistantMsg(m), nil
	case RoleTool:
		return dsMessage{Role: "tool", Content: dsJSONString(m.Content), ToolCallID: m.ToolCallID}, nil
	default:
		return dsMessage{}, fmt.Errorf("llm.deepseek: unknown role %q: %w", m.Role, ErrBadRequest)
	}
}

func buildDeepSeekAssistantMsg(m LLMMessage) dsMessage {
	if m.Content == "" && len(m.ToolCalls) == 0 && m.ReasoningContent != "" {
		m.Content = m.ReasoningContent
	}
	dm := dsMessage{
		Role:             "assistant",
		ReasoningContent: m.ReasoningContent,
		Content:          dsJSONString(m.Content),
	}
	for _, tc := range m.ToolCalls {
		dm.ToolCalls = append(dm.ToolCalls, dsToolCall{
			ID:       tc.ID,
			Type:     "function",
			Function: dsFuncCall{Name: tc.Name, Arguments: tc.Arguments},
		})
	}
	return dm
}

func toDeepSeekTools(defs []ToolDef) []dsTool {
	out := make([]dsTool, len(defs))
	for i, d := range defs {
		out[i] = dsTool{Type: "function", Function: dsFuncDef{Name: d.Name, Description: d.Description, Parameters: d.Parameters}}
	}
	return out
}

type dsContentPart struct {
	Type     string      `json:"type"`
	Text     string      `json:"text,omitempty"`
	ImageURL *dsImageURL `json:"image_url,omitempty"`
}
type dsImageURL struct {
	URL string `json:"url"`
}

// buildDeepSeekUserMsg renders a user turn: plain text, or multimodal content parts (text +
// image_url, image carried as a data-URL for vision models). A part type this provider can't
// carry inline (e.g. a PDF "file") is skipped — the attachment layer extracts those to text.
//
// buildDeepSeekUserMsg 渲染 user 回合：纯文本，或多模态内容块（text + image_url，图为 data-URL，
// 供视觉模型）。本 provider 无法内联承载的 part（如 PDF "file"）跳过——附件层为它抽成文本。
func buildDeepSeekUserMsg(m LLMMessage) (dsMessage, error) {
	if len(m.Parts) == 0 {
		return dsMessage{Role: "user", Content: dsJSONString(m.Content)}, nil
	}
	parts := make([]dsContentPart, 0, len(m.Parts))
	for _, part := range m.Parts {
		switch part.Type {
		case "text":
			parts = append(parts, dsContentPart{Type: "text", Text: part.Text})
		case "image_url":
			parts = append(parts, dsContentPart{Type: "image_url", ImageURL: &dsImageURL{URL: part.ImageURL}})
		}
	}
	raw, err := json.Marshal(parts)
	if err != nil {
		return dsMessage{}, fmt.Errorf("llm.deepseek: marshal parts: %w", err)
	}
	return dsMessage{Role: "user", Content: raw}, nil
}

func dsJSONString(s string) json.RawMessage {
	b, _ := json.Marshal(s)
	return b
}

// ── tool-call streaming state ──────────────────────────────────────────────────

type dsToolState struct {
	nameSent     map[int]bool
	idToIdx      map[string]int
	nextSynthIdx int
}

func newDeepSeekToolState() *dsToolState {
	return &dsToolState{nameSent: map[int]bool{}, idToIdx: map[string]int{}}
}

func (s *dsToolState) resolveIndex(tc dsToolCallDelta) int {
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

// ── DeepSeek wire types ─────────────────────────────────────────────────────────

type dsRequest struct {
	Model           string           `json:"model"`
	Messages        []dsMessage      `json:"messages"`
	Tools           []dsTool         `json:"tools,omitempty"`
	Stream          bool             `json:"stream"`
	StreamOptions   *dsStreamOptions `json:"stream_options,omitempty"`
	MaxTokens       int              `json:"max_tokens,omitempty"`
	Thinking        *dsThinking      `json:"thinking,omitempty"`
	ReasoningEffort string           `json:"reasoning_effort,omitempty"`
}

type dsStreamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

type dsThinking struct {
	Type string `json:"type"`
}

type dsMessage struct {
	Role             string          `json:"role"`
	Content          json.RawMessage `json:"content,omitempty"`
	ReasoningContent string          `json:"reasoning_content,omitempty"`
	ToolCalls        []dsToolCall    `json:"tool_calls,omitempty"`
	ToolCallID       string          `json:"tool_call_id,omitempty"`
}

type dsToolCall struct {
	ID       string     `json:"id"`
	Type     string     `json:"type"`
	Function dsFuncCall `json:"function"`
}

type dsFuncCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type dsTool struct {
	Type     string    `json:"type"`
	Function dsFuncDef `json:"function"`
}

type dsFuncDef struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

type dsChunk struct {
	Choices []dsChoice    `json:"choices"`
	Usage   *dsUsage      `json:"usage"`
	Error   *dsChunkError `json:"error,omitempty"`
}

type dsChunkError struct {
	Message string `json:"message"`
}

type dsChoice struct {
	Delta        dsDelta `json:"delta"`
	FinishReason string  `json:"finish_reason"`
}

type dsDelta struct {
	Content          string            `json:"content"`
	ReasoningContent string            `json:"reasoning_content"`
	ToolCalls        []dsToolCallDelta `json:"tool_calls"`
}

type dsToolCallDelta struct {
	Index    int         `json:"index"`
	ID       string      `json:"id"`
	Function dsFuncDelta `json:"function"`
}

type dsFuncDelta struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type dsUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
}

type dsNonStreamResponse struct {
	Choices []dsNonStreamChoice `json:"choices"`
	Usage   *dsUsage            `json:"usage"`
	Error   *dsChunkError       `json:"error,omitempty"`
}

type dsNonStreamChoice struct {
	Message      dsNonStreamMessage `json:"message"`
	FinishReason string             `json:"finish_reason"`
}

type dsNonStreamMessage struct {
	Role             string            `json:"role"`
	Content          string            `json:"content"`
	ReasoningContent string            `json:"reasoning_content"`
	ToolCalls        []dsToolCallDelta `json:"tool_calls"`
}

// ── model catalog (static; DeepSeek /models returns ids only) ───────────────────

func dsKnobs() []Knob {
	return []Knob{
		enumKnob("thinking", "Thinking", []string{"enabled", "disabled"}, "enabled"),
		enumKnob("reasoning_effort", "Reasoning effort", []string{"high", "max"}, "high"),
	}
}

// deepseekSpecs is DeepSeek's static catalog, most-specific prefix first. The V4 line (1M ctx /
// 384K out) controls thinking by request params; deepseek-chat/reasoner are compat aliases onto
// deepseek-v4-flash. Numbers per DeepSeek pricing, 2026-06.
//
// deepseekSpecs 是 DeepSeek 静态目录，最具体前缀在前。V4 线（1M/384K）靠请求参数控思考；
// deepseek-chat/reasoner 是指向 deepseek-v4-flash 的兼容别名。数值据 DeepSeek 定价 2026-06。
var deepseekSpecs = []modelSpec{
	{"deepseek-v4-pro", 1_000_000, 384_000, dsKnobs(), false, false},
	{"deepseek-v4-flash", 1_000_000, 384_000, dsKnobs(), false, false},
	{"deepseek-v4", 1_000_000, 384_000, dsKnobs(), false, false},
	{"deepseek-reasoner", 1_000_000, 384_000, dsKnobs(), false, false},
	{"deepseek-chat", 1_000_000, 384_000, dsKnobs(), false, false},
	{"deepseek", 128_000, 64_000, dsKnobs(), false, false},
}

// DescribeModels parses DeepSeek's id-only /models body against the static catalog.
//
// DescribeModels 解析 DeepSeek 仅含 id 的 /models 返回，查静态目录。
func (p *deepseekProvider) DescribeModels(raw string) ([]ModelInfo, error) {
	return describeFromSpecs(deepseekSpecs, raw), nil
}
