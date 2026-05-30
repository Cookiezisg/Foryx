package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"iter"
	"net/http"
)

// deepseekProvider speaks DeepSeek's /chat/completions API directly.
// It owns its own BuildRequest (reasoning_content round-trip rule + thinking
// encoding per 03 §3) and ParseStream (reads delta.reasoning_content →
// EventReasoning before delta.content → EventText). Written to DeepSeek's
// documented standard; shares only transport-level primitives.
//
// deepseekProvider 直接按 DeepSeek /chat/completions API 标准实现。
// 自有 BuildRequest（含 reasoning_content round-trip 规则 + 03 §3 thinking 编码）
// 和 ParseStream（delta.reasoning_content→EventReasoning 先于 content→EventText）；
// 仅共享 transport 层原语。

type deepseekProvider struct{}

func newDeepSeekProvider() *deepseekProvider { return &deepseekProvider{} }

func (p *deepseekProvider) Name() string           { return "deepseek" }
func (p *deepseekProvider) DefaultBaseURL() string { return "https://api.deepseek.com" }

// BuildRequest encodes a Request into a DeepSeek /chat/completions HTTP request.
//
// Reasoning_content round-trip rule (deepseekStripReasoning):
//   - Plain assistant turns (no tool_calls): strip reasoning_content — DeepSeek
//     rejects it on continuation turns that don't carry a tool response.
//   - Tool-call assistant turns: preserve reasoning_content (V3.2+ requires it
//     to reconstruct the chain-of-thought when the tool result comes back).
//
// Thinking encoding (03 §3):
//   - on  → thinking:{type:"enabled"} + reasoning_effort (map low/medium→high, xhigh→max)
//   - off → thinking:{type:"disabled"}
//   - nil/auto → omit both fields (byte-identical default)
//
// BuildRequest 把 Request 编码为 DeepSeek /chat/completions HTTP 请求。
// reasoning_content round-trip 规则：纯文字 turn 剥；含 tool_calls turn 保留（V3.2+）。
// thinking 编码（03 §3）：on→enabled+effort；off→disabled；nil/auto→省略。
func (p *deepseekProvider) BuildRequest(ctx context.Context, req Request) (*http.Request, error) {
	// Apply DeepSeek's reasoning_content round-trip rule before sanitize/encode.
	// The rule is provider-specific, so it lives inside BuildRequest rather than
	// as a separate beforeRequest hook.
	//
	// 在 sanitize/encode 前应用 DeepSeek reasoning_content round-trip 规则；
	// 该规则属 provider 专属，故直接内嵌在 BuildRequest 中而非独立钩子。
	for i := range req.Messages {
		m := &req.Messages[i]
		if m.Role != RoleAssistant {
			continue
		}
		// Plain assistant turn: strip; tool-call turn: preserve (V3.2+ requires it).
		// 纯文字 turn 剥；含 tool_calls turn 保留（V3.2+ 必须）。
		if len(m.ToolCalls) == 0 {
			m.ReasoningContent = ""
		}
	}

	req.Messages = SanitizeMessages(req.Messages)
	msgs, err := toOpenAIMsgs(req.Messages, req.System)
	if err != nil {
		return nil, fmt.Errorf("llm.deepseek: build messages: %w", err)
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
	// Thinking encoding per DeepSeek V4 API (03 §3).
	// nil / "auto" → no fields emitted (default: thinking not activated).
	//
	// 按 DeepSeek V4 API 编码 thinking（03 §3）；nil/"auto"→不发字段。
	if req.Thinking != nil && req.Thinking.Mode != "auto" {
		switch req.Thinking.Mode {
		case "on":
			body.Thinking = &oaiThinkingField{Type: "enabled"}
			body.ReasoningEffort = deepseekMapEffort(req.Thinking.Effort)
		case "off":
			body.Thinking = &oaiThinkingField{Type: "disabled"}
		}
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

// ParseStream reads DeepSeek SSE chunks and yields StreamEvents.
// DeepSeek sends delta.reasoning_content before delta.content; this parser
// preserves that order explicitly. Uses transport-level scanSSELines for line
// mechanics.
//
// ParseStream 读 DeepSeek SSE chunk 并 yield StreamEvent。
// DeepSeek 先发 delta.reasoning_content 后发 delta.content；此处显式保序。
// 用共享的 scanSSELines 处理原始 SSE 行语义。
func (p *deepseekProvider) ParseStream(ctx context.Context, resp *http.Response, req Request) iter.Seq[StreamEvent] {
	return func(yield func(StreamEvent) bool) {
		if req.DisableStream {
			parseDeepSeekNonStreaming(resp.Body, yield)
			return
		}
		state := newToolCallState()
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

// emitDeepSeekChunk converts one DeepSeek SSE chunk to StreamEvents.
// reasoning_content → EventReasoning (before content), content → EventText,
// tool_calls → EventToolStart + EventToolDelta, finish_reason → EventFinish.
//
// emitDeepSeekChunk 把一个 DeepSeek SSE chunk 转为 StreamEvent 序列。
// reasoning_content→EventReasoning（先于 content）；content→EventText；
// tool_calls→EventToolStart+EventToolDelta；finish_reason→EventFinish。
func emitDeepSeekChunk(chunk dsChunk, state *toolCallState, yield func(StreamEvent) bool) bool {
	// Surface mid-stream error envelope (same pattern as OpenRouter).
	// 检测流中错误信封（与 OpenRouter 相同模式）。
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

	// DeepSeek sends reasoning_content first, then content — preserve that order.
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
		idx := state.resolveIndex(oaiToolCallDelta(tc))
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

// parseDeepSeekNonStreaming reads a single non-streaming DeepSeek JSON response
// and synthesizes StreamEvents. Mirrors the OpenAI non-streaming path but reads
// only reasoning_content (DeepSeek's field — no "reasoning" alias needed here).
//
// parseDeepSeekNonStreaming 读单条非流式 DeepSeek JSON 响应并合成 StreamEvent。
// 镜像 OpenAI 非流式路径，但只读 reasoning_content（DeepSeek 字段）。
func parseDeepSeekNonStreaming(body interface{ Read([]byte) (int, error) }, yield func(StreamEvent) bool) {
	raw, err := readAll(body)
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
		yield(StreamEvent{Type: EventError, Err: fmt.Errorf("llm: provider returned error: %s", resp.Error.Message)})
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

// readAll is a package-local io.ReadAll wrapper used by deepseek non-streaming.
//
// readAll 是 deepseek 非流式路径专用的包内 io.ReadAll 封装。
func readAll(r interface{ Read([]byte) (int, error) }) ([]byte, error) {
	var buf bytes.Buffer
	_, err := buf.ReadFrom(readerAdapter{r})
	return buf.Bytes(), err
}

// readerAdapter wraps the minimal read interface into io.Reader for buf.ReadFrom.
//
// readerAdapter 把最小 read 接口包装为 io.Reader 供 buf.ReadFrom 使用。
type readerAdapter struct {
	r interface{ Read([]byte) (int, error) }
}

func (a readerAdapter) Read(p []byte) (int, error) { return a.r.Read(p) }

// ── DeepSeek-specific wire types ─────────────────────────────────────────────
//
// These mirror the OpenAI chunk types but are DeepSeek-specific — they don't
// include the Qwen flat-error fields or the Ollama "reasoning" alias, which are
// not part of DeepSeek's documented wire format.
//
// 这些类型镜像 OpenAI chunk 类型但专属 DeepSeek——不含 Qwen 扁平错误字段
// 或 Ollama "reasoning" 别名，它们不在 DeepSeek 官方协议中。

type dsChunk struct {
	Choices []dsChoice    `json:"choices"`
	Usage   *oaiUsage     `json:"usage"`
	Error   *oaiChunkError `json:"error,omitempty"`
}

type dsChoice struct {
	Delta        dsDelta `json:"delta"`
	FinishReason string  `json:"finish_reason"`
}

type dsDelta struct {
	Content          string             `json:"content"`
	ReasoningContent string             `json:"reasoning_content"`
	ToolCalls        []dsToolCallDelta  `json:"tool_calls"`
}

// dsToolCallDelta mirrors oaiToolCallDelta for DeepSeek — same shape but typed
// separately so deepseek.go reads end-to-end as its own provider story.
//
// dsToolCallDelta 镜像 oaiToolCallDelta；类型独立，保持 deepseek.go 可独立阅读。
type dsToolCallDelta struct {
	Index    int          `json:"index"`
	ID       string       `json:"id"`
	Function oaiFuncDelta `json:"function"`
}

type dsNonStreamResponse struct {
	Choices []dsNonStreamChoice `json:"choices"`
	Usage   *oaiUsage           `json:"usage"`
	Error   *oaiChunkError      `json:"error,omitempty"`
}

type dsNonStreamChoice struct {
	Message      dsNonStreamMessage `json:"message"`
	FinishReason string             `json:"finish_reason"`
}

type dsNonStreamMessage struct {
	Role             string             `json:"role"`
	Content          string             `json:"content"`
	ReasoningContent string             `json:"reasoning_content"`
	ToolCalls        []dsToolCallDelta  `json:"tool_calls"`
}
