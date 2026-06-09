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

// openaiProvider speaks OpenAI's /chat/completions wire directly and self-contained:
// its own body shape, message encoding, SSE chunk parsing, and wire types. Other
// OpenAI-compat providers each carry their OWN copy — duplication is deliberate so a
// per-provider quirk (a new thinking field, a different error envelope) never forces a
// branch into shared code. Reasoning models accept reasoning_effort.
//
// openaiProvider 直接、自包含地讲 OpenAI /chat/completions wire：自己的 body 形状、
// 消息编码、SSE chunk 解析、wire 类型。其他 OpenAI-compat provider 各持自己的一份——
// 重复是故意的：某家的特性（新 thinking 字段、不同错误信封）永不逼共享代码加分支。
type openaiProvider struct{}

func newOpenAIProvider() *openaiProvider { return &openaiProvider{} }

func (p *openaiProvider) Name() string           { return "openai" }
func (p *openaiProvider) DefaultBaseURL() string { return "https://api.openai.com/v1" }

// BuildRequest encodes a Request into an OpenAI /chat/completions HTTP request. Auth: Bearer.
// Reasoning/verbosity knobs come straight from Options by their native keys (reasoning_effort,
// verbosity); max_completion_tokens is sent only when MaxTokens is set.
//
// BuildRequest 把 Request 编码为 OpenAI /chat/completions 请求。Auth：Bearer。推理/verbosity
// 旋钮按原生 key（reasoning_effort、verbosity）直接取自 Options；MaxTokens 非零才发 max_completion_tokens。
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
	if req.MaxTokens > 0 {
		body.MaxCompletionTokens = req.MaxTokens
	}
	// Native knobs straight from Options — no neutral abstraction, no clamping. The UI only
	// offers values from Knobs(modelID); a non-reasoning model simply carries none of these keys.
	// 原生旋钮直接取自 Options——无中立抽象、不 clamp。UI 只给 Knobs(modelID) 声明的值；
	// 非推理模型自然不带这些 key。
	if v := req.Options["reasoning_effort"]; v != "" {
		body.ReasoningEffort = v
	}
	if v := req.Options["verbosity"]; v != "" {
		body.Verbosity = v
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

// ParseStream reads OpenAI SSE chunks (or one non-streaming body) into StreamEvents,
// using the shared scanSSELines for raw SSE line mechanics.
//
// ParseStream 读 OpenAI SSE chunk（或单条非流式 body）为 StreamEvent，用共享的
// scanSSELines 处理原始 SSE 行语义。
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

// ── message encoding ──────────────────────────────────────────────────────────

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
		return oaiMessage{Role: "tool", Content: jsonString(m.Content), ToolCallID: m.ToolCallID}, nil
	default:
		return oaiMessage{}, fmt.Errorf("llm.openai: unknown role %q: %w", m.Role, ErrBadRequest)
	}
}

func buildOpenAIUserMsg(m LLMMessage) (oaiMessage, error) {
	if len(m.Parts) == 0 {
		return oaiMessage{Role: "user", Content: jsonString(m.Content)}, nil
	}
	parts := make([]oaiContentPart, 0, len(m.Parts))
	for _, part := range m.Parts {
		switch part.Type {
		case "text":
			parts = append(parts, oaiContentPart{Type: "text", Text: part.Text})
		case "image_url":
			parts = append(parts, oaiContentPart{Type: "image_url", ImageURL: &oaiImageURL{URL: part.ImageURL}})
		case "file":
			parts = append(parts, oaiContentPart{Type: "file", File: &oaiFile{Filename: part.Filename, FileData: "data:" + part.MediaType + ";base64," + part.Data}})
		default:
			return oaiMessage{}, fmt.Errorf("llm.openai: unknown part type %q: %w", part.Type, ErrBadRequest)
		}
	}
	raw, err := json.Marshal(parts)
	if err != nil {
		return oaiMessage{}, fmt.Errorf("llm.openai: marshal parts: %w", err)
	}
	return oaiMessage{Role: "user", Content: raw}, nil
}

func buildOpenAIAssistantMsg(m LLMMessage) oaiMessage {
	// Reasoning-only turn → copy reasoning into content, else a strict provider 400s
	// next turn on an assistant message with neither content nor tool_calls.
	// 仅 reasoning 的回合 → 把 reasoning 复制进 content，否则严格 provider 下一轮会 400。
	if m.Content == "" && len(m.ToolCalls) == 0 && m.ReasoningContent != "" {
		m.Content = m.ReasoningContent
	}
	// Always emit content (even "") — strict providers reject a null content field.
	// content 即使空也 emit ""——严格 provider 拒 null。
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
		out[i] = oaiTool{Type: "function", Function: oaiFuncDef(d)}
	}
	return out
}

func jsonString(s string) json.RawMessage {
	b, _ := json.Marshal(s)
	return b
}

// ── SSE chunk parsing ─────────────────────────────────────────────────────────

// toolCallState tracks per-chunk tool-call streaming state; synthesizes an index by ID
// for chunks that omit it.
//
// toolCallState 跨 chunk 跟踪 tool-call 流式状态；对不填 index 的 chunk 按 ID 合成 index。
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
		if !state.toolNameSent[idx] && tc.Function.Name != "" {
			state.toolNameSent[idx] = true
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
		yield(StreamEvent{Type: EventError, Err: fmt.Errorf("%w: %s", ErrProviderError, resp.Error.Message)})
		return
	}
	if len(resp.Choices) == 0 {
		yield(StreamEvent{Type: EventError, Err: fmt.Errorf("llm.openai: non-streaming response has no choices: %w", ErrProviderError)})
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

// ── wire types ────────────────────────────────────────────────────────────────

type oaiRequest struct {
	Model         string            `json:"model"`
	Messages      []oaiMessage      `json:"messages"`
	Tools         []oaiTool         `json:"tools,omitempty"`
	Stream        bool              `json:"stream"`
	StreamOptions *oaiStreamOptions `json:"stream_options,omitempty"`
	// Native knobs (reasoning models only); each omitted when empty.
	ReasoningEffort     string `json:"reasoning_effort,omitempty"`
	Verbosity           string `json:"verbosity,omitempty"`
	MaxCompletionTokens int    `json:"max_completion_tokens,omitempty"`
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
	File     *oaiFile     `json:"file,omitempty"`
}

type oaiImageURL struct {
	URL string `json:"url"`
}

// oaiFile carries a document (PDF) inline per OpenAI chat-completions file input:
// {type:"file", file:{filename, file_data:"data:application/pdf;base64,…"}}.
//
// oaiFile 按 OpenAI chat-completions 文件输入内联文档（PDF）。
type oaiFile struct {
	Filename string `json:"filename"`
	FileData string `json:"file_data"`
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
}

type oaiChunkError struct {
	Message string `json:"message"`
	Code    any    `json:"code,omitempty"`
	Type    string `json:"type,omitempty"`
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

type oaiUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
}

// ── model catalog (static; OpenAI /v1/models returns ids only) ──────────────────

// oaiKnobs builds the reasoning_effort + verbosity descriptors for a GPT-5-era model with the
// given native effort set and default. o-series models get effort only (see openaiSpecs).
//
// oaiKnobs 为 GPT-5 代模型构造 reasoning_effort + verbosity 描述符（给定原生 effort 集与默认）。
// o 系列只有 effort（见 openaiSpecs）。
func oaiKnobs(effortDefault string, efforts ...string) []Knob {
	return []Knob{
		enumKnob("reasoning_effort", "Reasoning effort", efforts, effortDefault),
		enumKnob("verbosity", "Verbosity", []string{"low", "medium", "high"}, "medium"),
	}
}

// openaiSpecs is OpenAI's static catalog (capability numbers + native knobs), most-specific prefix
// first. GET /v1/models returns ids only, so these live here, maintained by software update.
// Numbers per OpenAI model pages, 2026-06.
//
// openaiSpecs 是 OpenAI 静态目录（能力数字 + 原生旋钮），最具体前缀在前。/v1/models 仅返回 id，
// 故规格在此、随软件更新维护。数值据 OpenAI model 页 2026-06。
var openaiSpecs = []modelSpec{
	{"gpt-5.5", 1_050_000, 128_000, oaiKnobs("medium", "none", "low", "medium", "high", "xhigh"), true, false},
	{"gpt-5.4-mini", 400_000, 128_000, oaiKnobs("none", "none", "low", "medium", "high", "xhigh"), true, false},
	{"gpt-5.4", 1_050_000, 128_000, oaiKnobs("none", "none", "low", "medium", "high", "xhigh"), true, false},
	{"gpt-5.1", 400_000, 128_000, oaiKnobs("none", "none", "low", "medium", "high"), true, false},
	{"gpt-5", 400_000, 128_000, oaiKnobs("medium", "minimal", "low", "medium", "high"), true, false},
	{"o3", 200_000, 100_000, []Knob{enumKnob("reasoning_effort", "Reasoning effort", []string{"low", "medium", "high"}, "medium")}, true, false},
	{"o4", 200_000, 100_000, []Knob{enumKnob("reasoning_effort", "Reasoning effort", []string{"low", "medium", "high"}, "medium")}, true, false},
	{"gpt-4.1", 1_047_576, 32_768, nil, true, false},
	{"gpt-4o", 128_000, 16_384, nil, true, false},
}

// DescribeModels parses OpenAI's id-only /v1/models body and resolves each id against the static
// catalog; ids absent from the catalog are skipped.
//
// DescribeModels 解析 OpenAI 仅含 id 的 /v1/models 返回，对每个 id 查静态目录；目录外 id 跳过。
func (p *openaiProvider) DescribeModels(raw string) ([]ModelInfo, error) {
	return describeFromSpecs(openaiSpecs, raw), nil
}
