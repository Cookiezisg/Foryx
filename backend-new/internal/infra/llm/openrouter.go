package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"iter"
	"net/http"
	"slices"
)

// openrouterProvider speaks OpenRouter's /chat/completions API, fully self-contained: its
// own wire types, message encoding, and SSE chunk parsing — no sharing with the openai
// provider even though OpenRouter is an OpenAI-compat aggregator. OpenRouter specifics: a
// top-level reasoning:{effort} object that routes thinking across whichever upstream the
// model resolves to, a reasoning_content delta alias alongside the primary reasoning field,
// mid-stream error objects, and the industry's richest /models payload (per-model
// supported_parameters, parsed live by DescribeModels rather than a static catalog).
//
// openrouterProvider 完整自包含地讲 OpenRouter /chat/completions：自己的 wire 类型、消息
// 编码、SSE 解析——即使它是 OpenAI-compat 聚合器也不与 openai 共享。OpenRouter 特有：顶层
// reasoning:{effort} 对象（按模型解析到的上游路由思考），delta 里 reasoning_content 别名与主
// reasoning 字段并存，流中 error 对象，以及全行业最富的 /models 载荷（每模型 supported_parameters，
// 由 DescribeModels 实时解析而非静态目录）。
type openrouterProvider struct{}

func newOpenRouterProvider() *openrouterProvider { return &openrouterProvider{} }

func (p *openrouterProvider) Name() string           { return "openrouter" }
func (p *openrouterProvider) DefaultBaseURL() string { return "https://openrouter.ai/api/v1" }

// BuildRequest encodes a Request into an OpenRouter /chat/completions HTTP request.
//
// Native knob from Options: reasoning_effort, surfaced per-model by DescribeModels and fed
// straight into reasoning:{effort} (OpenRouter routes that effort across whichever upstream
// the model resolves to). max_tokens caps output; absent (0) lets the upstream decide.
//
// BuildRequest 把 Request 编码为 OpenRouter 请求。原生旋钮取自 Options：reasoning_effort
// （由 DescribeModels 按模型暴露），直接塞进 reasoning:{effort}。max_tokens 限输出，缺省（0）由上游定。
func (p *openrouterProvider) BuildRequest(ctx context.Context, req Request) (*http.Request, error) {
	req.Messages = SanitizeMessages(req.Messages)
	msgs, err := toOpenRouterMsgs(req.Messages, req.System)
	if err != nil {
		return nil, fmt.Errorf("llm.openrouter: build messages: %w", err)
	}
	body := orRequest{
		Model:    req.ModelID,
		Messages: msgs,
		Stream:   !req.DisableStream,
	}
	if !req.DisableStream {
		body.StreamOptions = &orStreamOptions{IncludeUsage: true}
	}
	if len(req.Tools) > 0 {
		body.Tools = toOpenRouterTools(req.Tools)
	}
	if req.MaxTokens > 0 {
		body.MaxTokens = req.MaxTokens
	}
	if v := req.Options["reasoning_effort"]; v != "" {
		body.Reasoning = &orReasoning{Effort: v}
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("llm.openrouter: marshal body: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(
		ctx, http.MethodPost, req.BaseURL+"/chat/completions", bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("llm.openrouter: new request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+req.Key)
	return httpReq, nil
}

// ParseStream reads OpenRouter SSE chunks and yields StreamEvents. OpenRouter emits ':'
// keep-alive comment lines (": OPENROUTER PROCESSING") that scanSSELines already skips via
// its "data: " prefix filter.
//
// ParseStream 读 OpenRouter SSE chunk 并 yield StreamEvent。OpenRouter 发 ':' 心跳注释行
// （": OPENROUTER PROCESSING"），已由 scanSSELines 的 "data: " 前缀过滤跳过。
func (p *openrouterProvider) ParseStream(ctx context.Context, resp *http.Response, req Request) iter.Seq[StreamEvent] {
	return func(yield func(StreamEvent) bool) {
		state := newOpenRouterToolState()
		scanErr := scanSSELines(resp.Body, func(payload []byte) bool {
			if ctx.Err() != nil {
				return false
			}
			var chunk orChunk
			if err := json.Unmarshal(payload, &chunk); err != nil {
				yield(StreamEvent{Type: EventError, Err: fmt.Errorf("llm.openrouter: malformed SSE chunk: %w", err)})
				return false
			}
			return emitOpenRouterChunk(chunk, state, yield)
		})
		if scanErr != nil && ctx.Err() == nil {
			yield(StreamEvent{Type: EventError, Err: fmt.Errorf("llm.openrouter: scan: %w", scanErr)})
		}
	}
}

// emitOpenRouterChunk converts one OpenRouter SSE chunk to StreamEvents. A mid-stream
// error object terminates the stream as an EventError (OpenRouter surfaces upstream
// provider failures this way after a 200, not only via HTTP status).
//
// emitOpenRouterChunk 把一个 OpenRouter SSE chunk 转为 StreamEvent。流中 error 对象终止流
// 并发 EventError（OpenRouter 在 200 之后用这种方式暴露上游 provider 失败，不止靠 HTTP 状态）。
func emitOpenRouterChunk(chunk orChunk, state *orToolState, yield func(StreamEvent) bool) bool {
	if chunk.Error != nil {
		yield(StreamEvent{
			Type: EventError,
			Err:  fmt.Errorf("%w: in-stream: %s", ErrProviderError, chunk.Error.Message),
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

	// reasoning is OpenRouter's primary field; reasoning_content is the alias upstream
	// CN-family models (DeepSeek/Qwen) emit, so fall back to it when reasoning is empty.
	//
	// reasoning 是 OpenRouter 主字段；reasoning_content 是上游 CN 家族（DeepSeek/Qwen）发的
	// 别名，故 reasoning 为空时回退到它。
	reasoningDelta := delta.Reasoning
	if reasoningDelta == "" {
		reasoningDelta = delta.ReasoningContent
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

// ── message encoding ──────────────────────────────────────────────────────────

func toOpenRouterMsgs(msgs []LLMMessage, system string) ([]orMessage, error) {
	var out []orMessage
	if system != "" {
		out = append(out, orMessage{Role: "system", Content: orJSONString(system)})
	}
	for _, m := range msgs {
		om, err := toOpenRouterMsg(m)
		if err != nil {
			return nil, err
		}
		out = append(out, om)
	}
	return out, nil
}

func toOpenRouterMsg(m LLMMessage) (orMessage, error) {
	switch m.Role {
	case RoleUser:
		return buildOpenRouterUserMsg(m)
	case RoleAssistant:
		return buildOpenRouterAssistantMsg(m), nil
	case RoleTool:
		return orMessage{Role: "tool", Content: orJSONString(m.Content), ToolCallID: m.ToolCallID}, nil
	default:
		return orMessage{}, fmt.Errorf("llm.openrouter: unknown role %q: %w", m.Role, ErrBadRequest)
	}
}

func buildOpenRouterAssistantMsg(m LLMMessage) orMessage {
	om := orMessage{Role: "assistant", Content: orJSONString(m.Content)}
	for _, tc := range m.ToolCalls {
		// A malformed historical arguments string would make the upstream reject the whole
		// request; coerce it to "{}" so one bad past turn can't poison the continuation.
		//
		// 历史 arguments 串若非法会让上游拒掉整个请求；强制成 "{}"，避免一条坏历史毒化续传。
		args := tc.Arguments
		if !json.Valid([]byte(args)) {
			args = "{}"
		}
		om.ToolCalls = append(om.ToolCalls, orToolCall{
			ID:       tc.ID,
			Type:     "function",
			Function: orFuncCall{Name: tc.Name, Arguments: args},
		})
	}
	return om
}

func toOpenRouterTools(defs []ToolDef) []orTool {
	out := make([]orTool, len(defs))
	for i, d := range defs {
		out[i] = orTool{Type: "function", Function: orFuncDef{Name: d.Name, Description: d.Description, Parameters: d.Parameters}}
	}
	return out
}

type orContentPart struct {
	Type     string      `json:"type"`
	Text     string      `json:"text,omitempty"`
	ImageURL *orImageURL `json:"image_url,omitempty"`
}
type orImageURL struct {
	URL string `json:"url"`
}

// buildOpenRouterUserMsg renders a user turn: plain text, or multimodal content parts (text +
// image_url data-URL; OpenRouter is OpenAI-compatible and routes images to the target model). A
// PDF "file" part is skipped here — OpenRouter's document parsing needs a per-request plugin we
// don't enable; the attachment layer extracts PDFs to text instead.
//
// buildOpenRouterUserMsg 渲染 user 回合：纯文本，或多模态内容块（text + image_url data-URL；
// OpenRouter OpenAI 兼容、把图路由到目标模型）。PDF "file" part 跳过——其文档解析需逐请求插件、
// 本端不开；附件层改为抽成文本。
func buildOpenRouterUserMsg(m LLMMessage) (orMessage, error) {
	if len(m.Parts) == 0 {
		return orMessage{Role: "user", Content: orJSONString(m.Content)}, nil
	}
	parts := make([]orContentPart, 0, len(m.Parts))
	for _, part := range m.Parts {
		switch part.Type {
		case "text":
			parts = append(parts, orContentPart{Type: "text", Text: part.Text})
		case "image_url":
			parts = append(parts, orContentPart{Type: "image_url", ImageURL: &orImageURL{URL: part.ImageURL}})
		}
	}
	raw, err := json.Marshal(parts)
	if err != nil {
		return orMessage{}, fmt.Errorf("llm.openrouter: marshal parts: %w", err)
	}
	return orMessage{Role: "user", Content: raw}, nil
}

func orJSONString(s string) json.RawMessage {
	b, _ := json.Marshal(s)
	return b
}

// ── tool-call streaming state ──────────────────────────────────────────────────

type orToolState struct {
	nameSent     map[int]bool
	idToIdx      map[string]int
	nextSynthIdx int
}

func newOpenRouterToolState() *orToolState {
	return &orToolState{nameSent: map[int]bool{}, idToIdx: map[string]int{}}
}

// resolveIndex maps a tool-call delta to a stable index. Some upstreams omit a positive
// index and only carry an id, so synthesize a monotonic index keyed by id for those.
//
// resolveIndex 把 tool-call delta 映射到稳定 index。部分上游不给正 index 只带 id，
// 故对它们按 id 合成单调递增 index。
func (s *orToolState) resolveIndex(tc orToolCallDelta) int {
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

// ── OpenRouter wire types ─────────────────────────────────────────────────────────

type orRequest struct {
	Model         string           `json:"model"`
	Messages      []orMessage      `json:"messages"`
	Tools         []orTool         `json:"tools,omitempty"`
	Stream        bool             `json:"stream"`
	StreamOptions *orStreamOptions `json:"stream_options,omitempty"`
	MaxTokens     int              `json:"max_tokens,omitempty"`
	Reasoning     *orReasoning     `json:"reasoning,omitempty"`
}

type orStreamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

// orReasoning carries OpenRouter's reasoning:{effort} control. effort and max_tokens are
// mutually exclusive on the wire; this build drives reasoning purely by effort.
//
// orReasoning 承载 OpenRouter 的 reasoning:{effort} 控制。effort 与 max_tokens 在 wire 上互斥；
// 本版只用 effort 驱动 reasoning。
type orReasoning struct {
	Effort string `json:"effort,omitempty"`
}

type orMessage struct {
	Role       string          `json:"role"`
	Content    json.RawMessage `json:"content,omitempty"`
	ToolCalls  []orToolCall    `json:"tool_calls,omitempty"`
	ToolCallID string          `json:"tool_call_id,omitempty"`
}

type orToolCall struct {
	ID       string     `json:"id"`
	Type     string     `json:"type"`
	Function orFuncCall `json:"function"`
}

type orFuncCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type orTool struct {
	Type     string    `json:"type"`
	Function orFuncDef `json:"function"`
}

type orFuncDef struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

type orChunk struct {
	Choices []orChoice    `json:"choices"`
	Usage   *orUsage      `json:"usage"`
	Error   *orChunkError `json:"error,omitempty"`
}

type orChunkError struct {
	Message string `json:"message"`
}

type orChoice struct {
	Delta        orDelta `json:"delta"`
	FinishReason string  `json:"finish_reason"`
}

type orDelta struct {
	Content string `json:"content"`
	// reasoning is the primary field; reasoning_content is the CN-family alias.
	// reasoning 是主字段；reasoning_content 是 CN 家族别名。
	Reasoning        string            `json:"reasoning"`
	ReasoningContent string            `json:"reasoning_content"`
	ToolCalls        []orToolCallDelta `json:"tool_calls"`
}

type orToolCallDelta struct {
	Index    int         `json:"index"`
	ID       string      `json:"id"`
	Function orFuncDelta `json:"function"`
}

type orFuncDelta struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type orUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
}

// ── model catalog (dynamic; OpenRouter /models is the richest in the industry) ──

// DescribeModels parses OpenRouter's GET /api/v1/models body. As an aggregator of hundreds of
// upstream models it cannot use a static catalog: each entry carries its own context_length,
// top_provider.max_completion_tokens, and supported_parameters, so the whole catalog is derived
// from the live payload rather than a hand-maintained spec table.
//
// DescribeModels 解析 OpenRouter 的 GET /api/v1/models。它聚合上百上游模型，无法用静态目录：
// 每条自带 context_length、top_provider.max_completion_tokens、supported_parameters，故整份目录
// 由实时载荷推导，而非手维护的 spec 表。
func (p *openrouterProvider) DescribeModels(raw string) ([]ModelInfo, error) {
	var resp struct {
		Data []struct {
			ID            string `json:"id"`
			Name          string `json:"name"`
			ContextLength int    `json:"context_length"`
			TopProvider   struct {
				MaxCompletionTokens int `json:"max_completion_tokens"`
			} `json:"top_provider"`
			SupportedParameters []string `json:"supported_parameters"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		return nil, nil
	}
	out := make([]ModelInfo, 0, len(resp.Data))
	for _, m := range resp.Data {
		if m.ID == "" {
			continue
		}
		mi := ModelInfo{
			ID:            m.ID,
			DisplayName:   m.Name,
			ContextWindow: m.ContextLength,
			MaxOutput:     m.TopProvider.MaxCompletionTokens,
		}
		// OpenRouter publishes per-model knob support; surface reasoning effort when offered.
		// OpenRouter 公布每模型旋钮支持；该模型支持时暴露 reasoning effort 旋钮。
		if slices.Contains(m.SupportedParameters, "reasoning") {
			mi.Knobs = []Knob{enumKnob("reasoning_effort", "Reasoning effort",
				[]string{"minimal", "low", "medium", "high", "xhigh"}, "medium")}
		}
		out = append(out, mi)
	}
	return out, nil
}
