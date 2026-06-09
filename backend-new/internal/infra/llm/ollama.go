package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"iter"
	"net/http"
	"strconv"
	"strings"
)

// ollamaProvider speaks Ollama's /v1/chat/completions OpenAI-compat API, fully
// self-contained: its own wire types, message encoding, and SSE chunk parsing — no
// sharing with the openai provider even though the wire is OpenAI-shaped. Ollama
// specifics: reasoning_effort thinking encoding, delta.reasoning (no underscore) for
// thinking content (NOT reasoning_content), and a forced non-streaming path when tools
// are present.
//
// ollamaProvider 完整自包含地讲 Ollama /v1/chat/completions：自己的 wire 类型、消息编码、
// SSE 解析——即使 wire 是 OpenAI 形状也不与 openai 共享。Ollama 特有：reasoning_effort
// 编码 thinking、delta.reasoning（无下划线）传思考内容（非 reasoning_content）、有 tools 时
// 强制非流式路径。
type ollamaProvider struct{}

func newOllamaProvider() *ollamaProvider { return &ollamaProvider{} }

func (p *ollamaProvider) Name() string { return "ollama" }

// DefaultBaseURL is empty: Ollama is a local daemon with a user-chosen host/port, so the
// caller must always supply base_url.
//
// DefaultBaseURL 为空：Ollama 是本地 daemon、host/port 由用户定，caller 必须自带 base_url。
func (p *ollamaProvider) DefaultBaseURL() string { return "" }

// BuildRequest encodes a Request into an Ollama /v1/chat/completions HTTP request.
//
// Stream disable: forces non-streaming when tools are present — Ollama drops tool_calls
// in streaming mode, so the only reliable way to read them is a single JSON response.
//
// Native knobs from Options (verbatim, no normalization): top-level "think" carries the
// thinking switch — most models take a bool ("true"/"false"), GPT-OSS takes an effort
// string ("low"/"medium"/"high"), hence the wire field is `any`. options.num_ctx is the
// per-request context window — a local-runtime feature absent from cloud APIs (the client
// decides how much KV cache to allocate). MaxTokens maps to options.num_predict.
//
// BuildRequest 把 Request 编码为 Ollama /v1/chat/completions 请求。流式强制：有 tools 时走
// 非流式——Ollama streaming 模式会吞 tool_calls，单条 JSON 响应才能可靠读到。原生旋钮取自
// Options（原样不归一）：顶层 "think" 是思考开关——多数 model 收 bool（"true"/"false"），
// GPT-OSS 收 effort 串（"low"/"medium"/"high"），故 wire 字段用 any。options.num_ctx 是每请求
// 上下文窗口——本地 runtime 特性、云 API 没有（客户端自决分配多少 KV cache）。MaxTokens 映射
// options.num_predict。
func (p *ollamaProvider) BuildRequest(ctx context.Context, req Request) (*http.Request, error) {
	if len(req.Tools) > 0 {
		req.DisableStream = true
	}

	req.Messages = SanitizeMessages(req.Messages)
	msgs, err := toOllamaMsgs(req.Messages, req.System)
	if err != nil {
		return nil, fmt.Errorf("llm.ollama: build messages: %w", err)
	}
	body := ollamaRequest{
		Model:    req.ModelID,
		Messages: msgs,
		Stream:   !req.DisableStream,
	}
	if !req.DisableStream {
		body.StreamOptions = &ollamaStreamOptions{IncludeUsage: true}
	}
	if len(req.Tools) > 0 {
		body.Tools = toOllamaTools(req.Tools)
	}
	if v := req.Options["think"]; v != "" {
		if v == "true" || v == "false" {
			body.Think = v == "true"
		} else {
			body.Think = v // GPT-OSS effort: low/medium/high
		}
	}
	if v := req.Options["num_ctx"]; v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			body.setOption("num_ctx", n)
		}
	}
	if req.MaxTokens > 0 {
		body.setOption("num_predict", req.MaxTokens)
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("llm.ollama: marshal body: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(
		ctx, http.MethodPost, req.BaseURL+"/chat/completions", bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("llm.ollama: new request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+req.Key)
	return httpReq, nil
}

// ParseStream reads Ollama /v1 SSE chunks and yields StreamEvents. When tools are present
// the request is non-streaming, so this routes to the non-streaming path. On the streaming
// path Ollama /v1 carries thinking content in delta.reasoning (no underscore).
//
// ParseStream 读 Ollama /v1 SSE chunk 并 yield StreamEvent。有 tools 时请求非流式，路由到
// 非流式路径。流式路径中 Ollama /v1 用 delta.reasoning（无下划线）传思考内容。
func (p *ollamaProvider) ParseStream(ctx context.Context, resp *http.Response, req Request) iter.Seq[StreamEvent] {
	return func(yield func(StreamEvent) bool) {
		if req.DisableStream {
			parseOllamaNonStreaming(resp.Body, yield)
			return
		}
		state := newOllamaToolState()
		scanErr := scanSSELines(resp.Body, func(payload []byte) bool {
			if ctx.Err() != nil {
				return false
			}
			var chunk ollamaChunk
			if err := json.Unmarshal(payload, &chunk); err != nil {
				yield(StreamEvent{Type: EventError, Err: fmt.Errorf("llm.ollama: malformed SSE chunk: %w", err)})
				return false
			}
			return emitOllamaChunk(chunk, state, yield)
		})
		if scanErr != nil && ctx.Err() == nil {
			yield(StreamEvent{Type: EventError, Err: fmt.Errorf("llm.ollama: scan: %w", scanErr)})
		}
	}
}

func emitOllamaChunk(chunk ollamaChunk, state *ollamaToolState, yield func(StreamEvent) bool) bool {
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

	// Ollama /v1 carries thinking in delta.reasoning (no underscore). Some models put the
	// whole answer in reasoning with empty content, so surface it as its own reasoning event.
	//
	// Ollama /v1 用 delta.reasoning（无下划线）传思考；部分 model 把全文落 reasoning 而 content
	// 空——照样作为 reasoning 事件呈现。
	if delta.Reasoning != "" {
		if !yield(StreamEvent{Type: EventReasoning, Delta: delta.Reasoning}) {
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

// parseOllamaNonStreaming reads a single non-streaming Ollama JSON response and synthesizes
// StreamEvents. Used when tools are present (forced non-streaming). Ollama's non-streaming
// message carries thinking in message.reasoning (no underscore).
//
// parseOllamaNonStreaming 读单条非流式 Ollama JSON 响应并合成 StreamEvent。有 tools 时使用
// （强制非流式）。Ollama 非流式 message 用 message.reasoning（无下划线）传思考。
func parseOllamaNonStreaming(body io.Reader, yield func(StreamEvent) bool) {
	raw, err := io.ReadAll(io.LimitReader(body, 8<<20))
	if err != nil {
		yield(StreamEvent{Type: EventError, Err: fmt.Errorf("llm.ollama: read non-streaming body: %w", err)})
		return
	}
	var resp ollamaNonStreamResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		yield(StreamEvent{Type: EventError, Err: fmt.Errorf("llm.ollama: parse non-streaming response: %w", err)})
		return
	}
	if resp.Error != nil {
		yield(StreamEvent{Type: EventError, Err: fmt.Errorf("%w: %s", ErrProviderError, resp.Error.Message)})
		return
	}
	if len(resp.Choices) == 0 {
		yield(StreamEvent{Type: EventError, Err: fmt.Errorf("llm.ollama: non-streaming response has no choices: %w", ErrProviderError)})
		return
	}
	msg := resp.Choices[0].Message
	if msg.Reasoning != "" {
		if !yield(StreamEvent{Type: EventReasoning, Delta: msg.Reasoning}) {
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

func toOllamaMsgs(msgs []LLMMessage, system string) ([]ollamaMessage, error) {
	var out []ollamaMessage
	if system != "" {
		out = append(out, ollamaMessage{Role: "system", Content: system})
	}
	for _, m := range msgs {
		om, err := toOllamaMsg(m)
		if err != nil {
			return nil, err
		}
		out = append(out, om)
	}
	return out, nil
}

func toOllamaMsg(m LLMMessage) (ollamaMessage, error) {
	switch m.Role {
	case RoleUser:
		return buildOllamaUserMsg(m), nil
	case RoleAssistant:
		return buildOllamaAssistantMsg(m), nil
	case RoleTool:
		return ollamaMessage{Role: "tool", Content: m.Content, ToolCallID: m.ToolCallID}, nil
	default:
		return ollamaMessage{}, fmt.Errorf("llm.ollama: unknown role %q: %w", m.Role, ErrBadRequest)
	}
}

// buildOllamaUserMsg renders a user turn for Ollama: text → content, images → the native base64
// `images` array (Ollama's multimodal format for models like llava). The data-URL prefix is
// stripped to raw base64. Ollama has no document/PDF input — a "file" part is skipped; the
// attachment layer extracts it to text.
//
// buildOllamaUserMsg 渲染 Ollama 的 user 回合：text → content，图 → 原生 base64 `images` 数组
// （Ollama 多模态格式，供 llava 等）。剥 data-URL 前缀取裸 base64。Ollama 无文档/PDF 输入——"file"
// part 跳过；附件层抽成文本。
func buildOllamaUserMsg(m LLMMessage) ollamaMessage {
	if len(m.Parts) == 0 {
		return ollamaMessage{Role: "user", Content: m.Content}
	}
	om := ollamaMessage{Role: "user"}
	var text strings.Builder
	for _, part := range m.Parts {
		switch part.Type {
		case "text":
			text.WriteString(part.Text)
		case "image_url":
			om.Images = append(om.Images, ollamaStripDataURL(part.ImageURL))
		}
	}
	om.Content = text.String()
	return om
}

// ollamaStripDataURL returns the raw base64 payload of a data-URL ("data:<mime>;base64,<data>"),
// or the input unchanged when it isn't one (Ollama wants raw base64, not a data-URL).
//
// ollamaStripDataURL 返回 data-URL 的裸 base64 负载；非 data-URL 原样返回（Ollama 要裸 base64）。
func ollamaStripDataURL(s string) string {
	if len(s) > 5 && s[:5] == "data:" {
		for i := 0; i < len(s); i++ {
			if s[i] == ',' {
				return s[i+1:]
			}
		}
	}
	return s
}

func buildOllamaAssistantMsg(m LLMMessage) ollamaMessage {
	om := ollamaMessage{Role: "assistant", Content: m.Content}
	for _, tc := range m.ToolCalls {
		// Guard malformed historical args: Ollama 400s on non-JSON arguments, so a
		// non-object string is silently replaced with {} rather than failing the turn.
		//
		// 守历史 malformed args：Ollama 对非 JSON arguments 会 400，故非合法 object 静默换成 {}
		// 而非让整轮失败。
		args := json.RawMessage(tc.Arguments)
		if !json.Valid(args) {
			args = json.RawMessage("{}")
		}
		om.ToolCalls = append(om.ToolCalls, ollamaToolCall{
			ID:       tc.ID,
			Type:     "function",
			Function: ollamaFuncCall{Name: tc.Name, Arguments: string(args)},
		})
	}
	return om
}

func toOllamaTools(defs []ToolDef) []ollamaTool {
	out := make([]ollamaTool, len(defs))
	for i, d := range defs {
		out[i] = ollamaTool{Type: "function", Function: ollamaFuncDef{Name: d.Name, Description: d.Description, Parameters: d.Parameters}}
	}
	return out
}

// ── tool-call streaming state ──────────────────────────────────────────────────

// ollamaToolState tracks per-chunk tool-call streaming state, synthesizing an index by ID
// for chunks that omit it.
//
// ollamaToolState 跨 chunk 跟踪 tool-call 流式状态；对不填 index 的 chunk 按 ID 合成 index。
type ollamaToolState struct {
	nameSent     map[int]bool
	idToIdx      map[string]int
	nextSynthIdx int
}

func newOllamaToolState() *ollamaToolState {
	return &ollamaToolState{nameSent: map[int]bool{}, idToIdx: map[string]int{}}
}

func (s *ollamaToolState) resolveIndex(tc ollamaToolCallDelta) int {
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

// ── Ollama wire types ───────────────────────────────────────────────────────────
//
// Ollama /v1 follows OpenAI-compat with one key difference: thinking content arrives in
// the "reasoning" field (no underscore), both in SSE delta and in the non-streaming
// message — distinct from providers that use "reasoning_content".
//
// Ollama /v1 与 OpenAI-compat 一处关键差异：思考内容落 "reasoning" 字段（无下划线），SSE
// delta 与非流式 message 皆然——区别于用 "reasoning_content" 的 provider。

type ollamaRequest struct {
	Model         string               `json:"model"`
	Messages      []ollamaMessage      `json:"messages"`
	Tools         []ollamaTool         `json:"tools,omitempty"`
	Stream        bool                 `json:"stream"`
	StreamOptions *ollamaStreamOptions `json:"stream_options,omitempty"`
	// Think is top-level (not under options): bool for most models, effort string
	// ("low"/"medium"/"high") for GPT-OSS — hence any.
	//
	// Think 是顶层（不在 options 下）：多数 model 用 bool，GPT-OSS 用 effort 串——故 any。
	Think any `json:"think,omitempty"`
	// Options holds Ollama's per-request runtime knobs (num_ctx, num_predict, …) — these
	// are nested under "options", distinct from the top-level OpenAI-compat fields.
	//
	// Options 装 Ollama 每请求运行时旋钮（num_ctx、num_predict…）——嵌在 "options" 下，区别于
	// 顶层 OpenAI-compat 字段。
	Options map[string]any `json:"options,omitempty"`
}

// setOption lazily allocates the options map and sets one native runtime knob.
//
// setOption 惰性建 options map 并填一个原生运行时旋钮。
func (r *ollamaRequest) setOption(key string, val any) {
	if r.Options == nil {
		r.Options = map[string]any{}
	}
	r.Options[key] = val
}

type ollamaStreamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

type ollamaMessage struct {
	Role       string           `json:"role"`
	Content    string           `json:"content"`
	Images     []string         `json:"images,omitempty"` // base64 images (Ollama's native multimodal array)
	ToolCalls  []ollamaToolCall `json:"tool_calls,omitempty"`
	ToolCallID string           `json:"tool_call_id,omitempty"`
}

type ollamaToolCall struct {
	ID       string         `json:"id"`
	Type     string         `json:"type"`
	Function ollamaFuncCall `json:"function"`
}

type ollamaFuncCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type ollamaTool struct {
	Type     string        `json:"type"`
	Function ollamaFuncDef `json:"function"`
}

type ollamaFuncDef struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

type ollamaChunk struct {
	Choices []ollamaChoice    `json:"choices"`
	Usage   *ollamaUsage      `json:"usage"`
	Error   *ollamaChunkError `json:"error,omitempty"`
}

type ollamaChunkError struct {
	Message string `json:"message"`
}

type ollamaChoice struct {
	Delta        ollamaDelta `json:"delta"`
	FinishReason string      `json:"finish_reason"`
}

type ollamaDelta struct {
	Content string `json:"content"`
	// Reasoning is Ollama /v1's thinking field — "reasoning" (no underscore), NOT
	// "reasoning_content".
	//
	// Reasoning 是 Ollama /v1 的思考字段——"reasoning"（无下划线），非 "reasoning_content"。
	Reasoning string                `json:"reasoning"`
	ToolCalls []ollamaToolCallDelta `json:"tool_calls"`
}

type ollamaToolCallDelta struct {
	Index    int             `json:"index"`
	ID       string          `json:"id"`
	Function ollamaFuncDelta `json:"function"`
}

type ollamaFuncDelta struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type ollamaUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
}

type ollamaNonStreamResponse struct {
	Choices []ollamaNonStreamChoice `json:"choices"`
	Usage   *ollamaUsage            `json:"usage"`
	Error   *ollamaChunkError       `json:"error,omitempty"`
}

type ollamaNonStreamChoice struct {
	Message      ollamaNonStreamMessage `json:"message"`
	FinishReason string                 `json:"finish_reason"`
}

type ollamaNonStreamMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
	// Reasoning is Ollama /v1's thinking field name (no underscore).
	//
	// Reasoning 是 Ollama /v1 的思考字段名（无下划线）。
	Reasoning string                `json:"reasoning"`
	ToolCalls []ollamaToolCallDelta `json:"tool_calls"`
}

// ── model catalog (dynamic; /api/tags lists installed models, no caps/knobs) ────

// DescribeModels parses Ollama's GET /api/tags body ({"models":[{"name":...}]}) — its native
// discovery shape, unlike the OpenAI {"data":[{"id"}]} the chat path mimics. /api/tags carries
// neither capabilities nor context window (those live behind /api/show, which the probe doesn't
// fetch), so every installed model gets the generic local-runtime knobs and an unset window.
//
// DescribeModels 解析 Ollama 的 GET /api/tags 返回（{"models":[{"name":...}]}）——其原生发现形状，
// 区别于 chat 路径模仿的 OpenAI {"data":[{"id"}]}。/api/tags 既不带能力也不带上下文窗口（那在
// /api/show 后面、探针没取），故每个已装模型都给通用本地 runtime 旋钮、窗口留空。
func (p *ollamaProvider) DescribeModels(raw string) ([]ModelInfo, error) {
	var resp struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		return nil, nil
	}
	out := make([]ModelInfo, 0, len(resp.Models))
	for _, m := range resp.Models {
		if m.Name == "" {
			continue
		}
		out = append(out, ModelInfo{
			ID:          m.Name,
			DisplayName: m.Name,
			// Local model: context window is client-set per request via num_ctx, not a fixed spec.
			// 本地模型：上下文窗口由客户端每请求 num_ctx 设定，无固定规格。
			Knobs: []Knob{
				boolKnob("think", "Thinking", "false"),
				intKnob("num_ctx", "Context window", ""),
			},
		})
	}
	return out, nil
}
