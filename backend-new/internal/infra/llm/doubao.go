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

// doubaoProvider speaks Doubao (Volcengine Ark)'s /chat/completions API, fully self-contained:
// its own wire types, message encoding, and SSE chunk parsing — no sharing with the openai
// provider even though the wire is OpenAI-shaped. Doubao specifics: the top-level
// thinking:{type:enabled|disabled|auto} request object + reasoning_effort tier, and
// reasoning_content arriving before content in the stream.
//
// doubaoProvider 完整自包含地讲豆包（Volcengine Ark）/chat/completions：自己的 wire 类型、消息编码、
// SSE 解析——即使 wire 是 OpenAI 形状也不与 openai 共享。豆包特有：请求中的顶层
// thinking:{type:enabled|disabled|auto} 对象 + reasoning_effort 力度档、流中 reasoning_content 先于 content。
type doubaoProvider struct{}

func newDoubaoProvider() *doubaoProvider { return &doubaoProvider{} }

func (p *doubaoProvider) Name() string           { return "doubao" }
func (p *doubaoProvider) DefaultBaseURL() string { return "https://ark.cn-beijing.volces.com/api/v3" }

// BuildRequest encodes a Request into a Doubao /chat/completions HTTP request.
//
// Native knobs from Options (verbatim, no normalization): thinking ({type: enabled|disabled|auto})
// + reasoning_effort (minimal|low|medium|high|max — effort tiers, not a token budget; Ark's Chat
// API has no budget_tokens field).
//
// BuildRequest 把 Request 编码为豆包 /chat/completions HTTP 请求。原生旋钮取自 Options（原样不归一）：
// thinking（{type:enabled|disabled|auto}）+ reasoning_effort（力度档，非 token 预算；方舟 Chat API 无 budget_tokens）。
func (p *doubaoProvider) BuildRequest(ctx context.Context, req Request) (*http.Request, error) {
	req.Messages = SanitizeMessages(req.Messages)
	msgs, err := todoubaoMsgs(req.Messages, req.System)
	if err != nil {
		return nil, fmt.Errorf("llm.doubao: build messages: %w", err)
	}
	body := doubaoRequest{
		Model:    req.ModelID,
		Messages: msgs,
		Stream:   !req.DisableStream,
	}
	if !req.DisableStream {
		body.StreamOptions = &doubaoStreamOptions{IncludeUsage: true}
	}
	if len(req.Tools) > 0 {
		body.Tools = todoubaoTools(req.Tools)
	}
	if req.MaxTokens > 0 {
		body.MaxTokens = req.MaxTokens
	}
	if v := req.Options["thinking"]; v != "" {
		body.Thinking = &doubaoThinking{Type: v}
	}
	if v := req.Options["reasoning_effort"]; v != "" {
		body.ReasoningEffort = v
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("llm.doubao: marshal body: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(
		ctx, http.MethodPost, req.BaseURL+"/chat/completions", bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("llm.doubao: new request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+req.Key)
	return httpReq, nil
}

func (p *doubaoProvider) ParseStream(ctx context.Context, resp *http.Response, req Request) iter.Seq[StreamEvent] {
	return func(yield func(StreamEvent) bool) {
		if req.DisableStream {
			parsedoubaoNonStreaming(resp.Body, yield)
			return
		}
		state := newdoubaoToolState()
		scanErr := scanSSELines(resp.Body, func(payload []byte) bool {
			if ctx.Err() != nil {
				return false
			}
			var chunk doubaoChunk
			if err := json.Unmarshal(payload, &chunk); err != nil {
				yield(StreamEvent{Type: EventError, Err: fmt.Errorf("llm.doubao: malformed SSE chunk: %w", err)})
				return false
			}
			return emitdoubaoChunk(chunk, state, yield)
		})
		if scanErr != nil && ctx.Err() == nil {
			yield(StreamEvent{Type: EventError, Err: fmt.Errorf("llm.doubao: scan: %w", scanErr)})
		}
	}
}

func emitdoubaoChunk(chunk doubaoChunk, state *doubaoToolState, yield func(StreamEvent) bool) bool {
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

	// Doubao sends reasoning_content before content — preserve that order.
	// 豆包先发 reasoning_content 再发 content——严格保序。
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

func parsedoubaoNonStreaming(body io.Reader, yield func(StreamEvent) bool) {
	raw, err := io.ReadAll(io.LimitReader(body, 8<<20))
	if err != nil {
		yield(StreamEvent{Type: EventError, Err: fmt.Errorf("llm.doubao: read non-streaming body: %w", err)})
		return
	}
	var resp doubaoNonStreamResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		yield(StreamEvent{Type: EventError, Err: fmt.Errorf("llm.doubao: parse non-streaming response: %w", err)})
		return
	}
	if resp.Error != nil {
		yield(StreamEvent{Type: EventError, Err: fmt.Errorf("%w: %s", ErrProviderError, resp.Error.Message)})
		return
	}
	if len(resp.Choices) == 0 {
		yield(StreamEvent{Type: EventError, Err: fmt.Errorf("llm.doubao: non-streaming response has no choices: %w", ErrProviderError)})
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

func todoubaoMsgs(msgs []LLMMessage, system string) ([]doubaoMessage, error) {
	var out []doubaoMessage
	if system != "" {
		out = append(out, doubaoMessage{Role: "system", Content: doubaoJSONString(system)})
	}
	for _, m := range msgs {
		dm, err := todoubaoMsg(m)
		if err != nil {
			return nil, err
		}
		out = append(out, dm)
	}
	return out, nil
}

func todoubaoMsg(m LLMMessage) (doubaoMessage, error) {
	switch m.Role {
	case RoleUser:
		return builddoubaoUserMsg(m)
	case RoleAssistant:
		return builddoubaoAssistantMsg(m), nil
	case RoleTool:
		return doubaoMessage{Role: "tool", Content: doubaoJSONString(m.Content), ToolCallID: m.ToolCallID}, nil
	default:
		return doubaoMessage{}, fmt.Errorf("llm.doubao: unknown role %q: %w", m.Role, ErrBadRequest)
	}
}

func builddoubaoAssistantMsg(m LLMMessage) doubaoMessage {
	dm := doubaoMessage{
		Role:    "assistant",
		Content: doubaoJSONString(m.Content),
	}
	for _, tc := range m.ToolCalls {
		// Historic tool args may be malformed; send valid JSON or an empty object so a
		// strict provider doesn't reject the whole continuation on one bad arg blob.
		// 历史 tool 参数可能损坏；发合法 JSON，否则降级为空对象，避免严格 provider 因单条坏参整体拒绝。
		args := json.RawMessage(tc.Arguments)
		if !json.Valid(args) {
			args = json.RawMessage("{}")
		}
		dm.ToolCalls = append(dm.ToolCalls, doubaoToolCall{
			ID:       tc.ID,
			Type:     "function",
			Function: doubaoFuncCall{Name: tc.Name, Arguments: args},
		})
	}
	return dm
}

func todoubaoTools(defs []ToolDef) []doubaoTool {
	out := make([]doubaoTool, len(defs))
	for i, d := range defs {
		out[i] = doubaoTool{Type: "function", Function: doubaoFuncDef{Name: d.Name, Description: d.Description, Parameters: d.Parameters}}
	}
	return out
}

type doubaoContentPart struct {
	Type     string          `json:"type"`
	Text     string          `json:"text,omitempty"`
	ImageURL *doubaoImageURL `json:"image_url,omitempty"`
}
type doubaoImageURL struct {
	URL string `json:"url"`
}

// builddoubaoUserMsg renders a user turn: plain text, or multimodal content parts (text +
// image_url data-URL for Doubao-vision; Volcengine Ark is OpenAI-compatible). PDF "file" parts
// are skipped — no inline document input; the attachment layer extracts those to text.
//
// builddoubaoUserMsg 渲染 user 回合：纯文本，或多模态内容块（text + image_url data-URL，供
// Doubao 视觉；Volcengine Ark OpenAI 兼容）。PDF "file" part 跳过——无内联文档输入；附件层抽成文本。
func builddoubaoUserMsg(m LLMMessage) (doubaoMessage, error) {
	if len(m.Parts) == 0 {
		return doubaoMessage{Role: "user", Content: doubaoJSONString(m.Content)}, nil
	}
	parts := make([]doubaoContentPart, 0, len(m.Parts))
	for _, part := range m.Parts {
		switch part.Type {
		case "text":
			parts = append(parts, doubaoContentPart{Type: "text", Text: part.Text})
		case "image_url":
			parts = append(parts, doubaoContentPart{Type: "image_url", ImageURL: &doubaoImageURL{URL: part.ImageURL}})
		}
	}
	raw, err := json.Marshal(parts)
	if err != nil {
		return doubaoMessage{}, fmt.Errorf("llm.doubao: marshal parts: %w", err)
	}
	return doubaoMessage{Role: "user", Content: raw}, nil
}

func doubaoJSONString(s string) json.RawMessage {
	b, _ := json.Marshal(s)
	return b
}

// ── tool-call streaming state ──────────────────────────────────────────────────

type doubaoToolState struct {
	nameSent     map[int]bool
	idToIdx      map[string]int
	nextSynthIdx int
}

func newdoubaoToolState() *doubaoToolState {
	return &doubaoToolState{nameSent: map[int]bool{}, idToIdx: map[string]int{}}
}

func (s *doubaoToolState) resolveIndex(tc doubaoToolCallDelta) int {
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

// ── Doubao wire types ─────────────────────────────────────────────────────────

type doubaoRequest struct {
	Model           string               `json:"model"`
	Messages        []doubaoMessage      `json:"messages"`
	Tools           []doubaoTool         `json:"tools,omitempty"`
	Stream          bool                 `json:"stream"`
	StreamOptions   *doubaoStreamOptions `json:"stream_options,omitempty"`
	MaxTokens       int                  `json:"max_tokens,omitempty"`
	Thinking        *doubaoThinking      `json:"thinking,omitempty"`
	ReasoningEffort string               `json:"reasoning_effort,omitempty"`
}

type doubaoStreamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

type doubaoThinking struct {
	Type string `json:"type"`
}

type doubaoMessage struct {
	Role       string           `json:"role"`
	Content    json.RawMessage  `json:"content,omitempty"`
	ToolCalls  []doubaoToolCall `json:"tool_calls,omitempty"`
	ToolCallID string           `json:"tool_call_id,omitempty"`
}

type doubaoToolCall struct {
	ID       string         `json:"id"`
	Type     string         `json:"type"`
	Function doubaoFuncCall `json:"function"`
}

type doubaoFuncCall struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

type doubaoTool struct {
	Type     string        `json:"type"`
	Function doubaoFuncDef `json:"function"`
}

type doubaoFuncDef struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

type doubaoChunk struct {
	Choices []doubaoChoice    `json:"choices"`
	Usage   *doubaoUsage      `json:"usage"`
	Error   *doubaoChunkError `json:"error,omitempty"`
}

type doubaoChunkError struct {
	Message string `json:"message"`
}

type doubaoChoice struct {
	Delta        doubaoDelta `json:"delta"`
	FinishReason string      `json:"finish_reason"`
}

type doubaoDelta struct {
	Content          string                `json:"content"`
	ReasoningContent string                `json:"reasoning_content"`
	ToolCalls        []doubaoToolCallDelta `json:"tool_calls"`
}

type doubaoToolCallDelta struct {
	Index    int             `json:"index"`
	ID       string          `json:"id"`
	Function doubaoFuncDelta `json:"function"`
}

type doubaoFuncDelta struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type doubaoUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
}

type doubaoNonStreamResponse struct {
	Choices []doubaoNonStreamChoice `json:"choices"`
	Usage   *doubaoUsage            `json:"usage"`
	Error   *doubaoChunkError       `json:"error,omitempty"`
}

type doubaoNonStreamChoice struct {
	Message      doubaoNonStreamMessage `json:"message"`
	FinishReason string                 `json:"finish_reason"`
}

type doubaoNonStreamMessage struct {
	Role             string                `json:"role"`
	Content          string                `json:"content"`
	ReasoningContent string                `json:"reasoning_content"`
	ToolCalls        []doubaoToolCallDelta `json:"tool_calls"`
}

// ── model catalog (static; Ark has no /models endpoint) ─────────────────────────

// doubaoKnobs builds the two native knobs; thinkingValues varies per family (only seed-1-6
// offers "auto"), so it's passed in rather than hardcoded.
//
// doubaoKnobs 构造两个原生旋钮；thinking 取值各族不同（仅 seed-1-6 提供 "auto"）故由外部传入。
func doubaoKnobs(thinkingValues []string) []Knob {
	return []Knob{
		enumKnob("thinking", "Thinking", thinkingValues, "enabled"),
		enumKnob("reasoning_effort", "Reasoning effort", []string{"minimal", "low", "medium", "high", "max"}, "medium"),
	}
}

// doubaoSpecs is the static catalog (most-specific prefix first). Ark exposes no /models endpoint,
// so DescribeModels has nothing to enumerate against — the catalog itself is the source of truth.
// Seed family: 256K context; only doubao-seed-1-6 supports thinking "auto". Numbers per Volcengine
// Ark docs, 2026-06-04.
//
// doubaoSpecs 是静态目录（最具体前缀在前）。方舟无 /models 端点，DescribeModels 无可枚举对象——目录
// 本身即事实源。seed 族：256K context；仅 doubao-seed-1-6 支持 thinking "auto"。数值据火山方舟文档 2026-06-04。
var doubaoSpecs = []modelSpec{
	{"doubao-seed-1-6", 256000, 32000, doubaoKnobs([]string{"enabled", "disabled", "auto"}), false, false},
	{"doubao-seed-1-8", 256000, 64000, doubaoKnobs([]string{"enabled", "disabled"}), false, false},
	{"doubao-seed-2-0", 256000, 128000, doubaoKnobs([]string{"enabled", "disabled"}), false, false},
	{"doubao-seed-character", 128000, 32000, nil, false, false},
	{"doubao-seed", 256000, 32000, doubaoKnobs([]string{"enabled", "disabled"}), false, false},
}

// DescribeModels returns the static catalog; raw is ignored since Ark has no /models endpoint.
//
// DescribeModels 返回静态目录；方舟无 /models 端点故忽略 raw。
func (p *doubaoProvider) DescribeModels(raw string) ([]ModelInfo, error) {
	return describeFromSpecs(doubaoSpecs, raw), nil
}
