// Package llm provides a provider-agnostic LLM streaming client built on
// iter.Seq. It replaces the Eino framework dependency with a self-owned,
// fully observable implementation. All SSE parsing and request serialisation
// live in this package.
//
// Package llm 提供与 provider 无关的 LLM 流式客户端，基于 iter.Seq。
// 以自主实现取代 Eino 框架，所有 SSE 解析和请求序列化均在本包内完成。
package llm

import (
	"context"
	"encoding/json"
	"errors"
	"iter"
	"strings"
)

// ── Sentinel errors ───────────────────────────────────────────────────────────
//
// HTTP-status-classified errors from upstream LLM providers. Wrapping with
// these sentinels lets callers `errors.Is(err, llm.ErrAuthFailed)` and
// react accordingly (e.g. apikey.Service.MarkInvalid on 401/403). All
// registered in transport/httpapi/response/errmap.go.
//
// 上游 LLM provider 按 HTTP 状态分类的 sentinel。调用方可用
// `errors.Is(err, llm.ErrAuthFailed)` 判别并相应反应（例：401/403
// 触发 apikey.MarkInvalid）。全部登记 errmap。
var (
	ErrAuthFailed    = errors.New("llm: authentication failed")
	ErrRateLimited   = errors.New("llm: rate limited")
	ErrBadRequest    = errors.New("llm: bad request")
	ErrModelNotFound = errors.New("llm: model not found")
	ErrProviderError = errors.New("llm: provider error")
)

// ── Stream events ─────────────────────────────────────────────────────────────

// StreamEventType identifies the kind of event emitted by Client.Stream.
//
// StreamEventType 标识 Client.Stream 输出的事件类型。
type StreamEventType string

const (
	// EventText is a text content delta.
	// EventText 是普通文字 delta。
	EventText StreamEventType = "text"

	// EventReasoning is a thinking/reasoning delta (e.g. DeepSeek-R1).
	// EventReasoning 是思考内容 delta（如 DeepSeek-R1）。
	EventReasoning StreamEventType = "reasoning"

	// EventToolStart fires when a tool call name is first known.
	// The chat pipeline rebuilds the in-progress Message snapshot and
	// publishes a chat.message SSE event with the new tool_call block.
	//
	// EventToolStart 在 tool call name 首次确定时触发；chat pipeline 据此
	// 重建 in-progress Message 快照并推一次 chat.message 事件（含新 tool_call block）。
	EventToolStart StreamEventType = "tool_start"

	// EventToolDelta carries a fragment of a tool call's arguments JSON.
	//
	// EventToolDelta 携带 tool call arguments JSON 的一个片段。
	EventToolDelta StreamEventType = "tool_delta"

	// EventFinish signals stream completion, carrying token usage.
	//
	// EventFinish 标志流结束，携带 token 用量。
	EventFinish StreamEventType = "finish"

	// EventError signals an unrecoverable error. The iterator stops after this.
	//
	// EventError 标志不可恢复的错误，发出后迭代器停止。
	EventError StreamEventType = "error"
)

// StreamEvent is one typed event produced by Client.Stream.
// Fields are populated according to Type; all others are zero values.
//
// StreamEvent 是 Client.Stream 产生的一个类型化事件，字段按 Type 按需填充。
type StreamEvent struct {
	Type StreamEventType

	// EventText / EventReasoning
	Delta string

	// EventToolStart
	ToolIndex int
	ToolID    string
	ToolName  string

	// EventToolDelta — ToolIndex reused from EventToolStart fields
	ArgsDelta string

	// EventFinish
	FinishReason string
	InputTokens  int
	OutputTokens int

	// EventError
	Err error
}

// ── Message / request types ───────────────────────────────────────────────────

// Role is the speaker role in a conversation turn.
//
// Role 是对话回合中的发言方角色。
type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"

	// RoleTool carries one tool execution result back to the LLM.
	// Anthropic native format serialises these as user-role content blocks.
	//
	// RoleTool 把一次工具执行结果回传给 LLM。
	// Anthropic 原生格式将其序列化为 user 角色的 content block。
	RoleTool Role = "tool"
)

// LLMMessage is a provider-agnostic conversation turn sent to the LLM.
//
// LLMMessage 是发给 LLM 的、与 provider 无关的对话回合。
type LLMMessage struct {
	Role    Role
	Content string // plain text; empty when Parts is set

	// Parts is set for multi-modal user messages (text + images).
	//
	// Parts 用于多模态 user 消息（文字 + 图片）。
	Parts []ContentPart

	// ToolCalls is populated on assistant messages that invoke tools.
	//
	// ToolCalls 在 assistant 消息发起 tool call 时填充。
	ToolCalls []LLMToolCall

	// ToolCallID links a RoleTool message to the assistant tool call that triggered it.
	//
	// ToolCallID 把 RoleTool 消息关联到触发它的 assistant tool call。
	ToolCallID string

	// ReasoningContent must be echoed back unchanged to thinking-mode APIs
	// (e.g. DeepSeek-R1) on subsequent turns.
	//
	// ReasoningContent 在后续请求中须原样回传给 thinking-mode API（如 DeepSeek-R1）。
	ReasoningContent string
}

// ContentPart is one element in a multi-modal user message.
//
// ContentPart 是多模态 user 消息中的一个内容元素。
type ContentPart struct {
	Type     string // "text" | "image_url"
	Text     string
	ImageURL string // base64 data URL or https URL
}

// LLMToolCall describes one tool invocation within an assistant message.
// Arguments is a complete JSON object string and never contains "summary".
//
// LLMToolCall 描述 assistant 消息中的一次工具调用。
// Arguments 是完整 JSON object 字符串，不含 summary 字段。
type LLMToolCall struct {
	ID        string
	Name      string
	Arguments string
}

// ToolDef is the tool description sent to the LLM.
// Parameters must be a valid JSON Schema object.
//
// ToolDef 是发给 LLM 的工具描述，Parameters 须为合法的 JSON Schema object。
type ToolDef struct {
	Name        string
	Description string
	Parameters  json.RawMessage
}

// Request is a complete LLM call specification.
//
// Request 是一次完整的 LLM 调用规格。
type Request struct {
	ModelID  string
	Key      string
	BaseURL  string
	System   string
	Messages []LLMMessage
	Tools    []ToolDef

	// DisableStream forces non-streaming mode at the wire level. Default
	// (false) = stream every call (current product behavior). The only
	// production setter today is ollamaAdapter.BeforeRequest, which flips
	// this true when len(Tools) > 0 — Ollama's OpenAI-compat path silently
	// drops tool_calls when streaming is on (open issue ollama#12557 /
	// #9632). Streaming back on for non-tool turns is fine.
	//
	// DisableStream 在 wire 层强制关流式。默认 false（当前产品行为=都流式）。
	// 唯一 production setter 是 ollamaAdapter.BeforeRequest——有 tools 时翻
	// true。Ollama OpenAI-compat 在 stream+tools 下静默吞 tool_calls
	// （ollama #12557 / #9632）。无 tools 的 turn 仍流式。
	DisableStream bool
}

// ── Client interface ──────────────────────────────────────────────────────────

// Client is the LLM streaming interface.
// Stream returns an iter.Seq[StreamEvent]; callers consume it with for-range
// and may break at any time. Context cancellation stops iteration cleanly.
//
// Client 是 LLM 流式接口。Stream 返回 iter.Seq[StreamEvent]，
// 调用方用 for range 消费，随时可 break，ctx 取消时迭代干净停止。
type Client interface {
	Stream(ctx context.Context, req Request) iter.Seq[StreamEvent]
}

// Generate runs a non-streaming completion by consuming Stream and
// collecting all text deltas. For short internal LLM calls (tool ranking,
// auto-titling) that do not need streaming.
//
// Generate 通过消费 Stream 并拼接文字 delta 实现非流式调用。
// 用于不需要流式输出的内部短文本调用（工具排序、自动标题等）。
func Generate(ctx context.Context, c Client, req Request) (string, error) {
	var sb strings.Builder
	for event := range c.Stream(ctx, req) {
		switch event.Type {
		case EventText:
			sb.WriteString(event.Delta)
		case EventError:
			return "", event.Err
		}
	}
	return sb.String(), nil
}
