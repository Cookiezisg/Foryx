// Package llm is a provider-agnostic LLM streaming client built on iter.Seq.
//
// Package llm 是基于 iter.Seq 的 provider-agnostic LLM 流式客户端。
package llm

import (
	"context"
	"encoding/json"
	"errors"
	"iter"
	"strings"
)

var (
	ErrAuthFailed    = errors.New("llm: authentication failed")
	ErrRateLimited   = errors.New("llm: rate limited")
	ErrBadRequest    = errors.New("llm: bad request")
	ErrModelNotFound = errors.New("llm: model not found")
	ErrProviderError = errors.New("llm: provider error")
)

// StreamEventType identifies a Client.Stream event variant.
//
// StreamEventType 标识 Client.Stream 输出的事件类型。
type StreamEventType string

const (
	EventText      StreamEventType = "text"
	EventReasoning StreamEventType = "reasoning"
	EventToolStart StreamEventType = "tool_start"
	EventToolDelta StreamEventType = "tool_delta"
	EventFinish    StreamEventType = "finish"
	EventError     StreamEventType = "error"
)

// StreamEvent is one typed event from Client.Stream; field set varies by Type.
//
// StreamEvent 是 Client.Stream 的类型化事件；字段集随 Type 而异。
type StreamEvent struct {
	Type StreamEventType

	Delta string

	ToolIndex int
	ToolID    string
	ToolName  string
	ArgsDelta string

	FinishReason string
	InputTokens  int
	OutputTokens int

	Err error
}

// Role is the speaker role on a conversation turn.
//
// Role 是对话回合中的发言方角色。
type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// LLMMessage is a provider-agnostic conversation turn sent to the LLM.
//
// LLMMessage 是发给 LLM 的、与 provider 无关的对话回合。
type LLMMessage struct {
	Role             Role
	Content          string
	Parts            []ContentPart
	ToolCalls        []LLMToolCall
	ToolCallID       string
	ReasoningContent string
}

// ContentPart is one element of a multi-modal user message (text or image_url).
//
// ContentPart 是多模态 user 消息中的一个内容元素（text 或 image_url）。
type ContentPart struct {
	Type     string
	Text     string
	ImageURL string
}

// LLMToolCall is one tool invocation in an assistant message; Arguments is JSON object string.
//
// LLMToolCall 描述 assistant 消息中的一次工具调用；Arguments 为 JSON object 字符串。
type LLMToolCall struct {
	ID        string
	Name      string
	Arguments string
}

// ToolDef is the tool description sent to the LLM; Parameters must be a JSON Schema object.
//
// ToolDef 是发给 LLM 的工具描述；Parameters 必须是 JSON Schema object。
type ToolDef struct {
	Name        string
	Description string
	Parameters  json.RawMessage
}

// Request specifies one LLM call.
//
// Request 是一次 LLM 调用规格。
type Request struct {
	ModelID  string
	Key      string
	BaseURL  string
	System   string
	Messages []LLMMessage
	Tools    []ToolDef

	// DisableStream forces non-streaming wire mode (used by Ollama+tools workaround).
	// DisableStream 强制 non-streaming（Ollama 有 tools 时绕 bug 用）。
	DisableStream bool
}

// Client streams LLM events via iter.Seq; ctx cancel stops cleanly.
//
// Client 通过 iter.Seq 流式输出 LLM 事件；ctx 取消可干净停止。
type Client interface {
	Stream(ctx context.Context, req Request) iter.Seq[StreamEvent]
}

// Generate consumes Stream, concatenates text deltas, returns the assembled string.
//
// Generate 消费 Stream 并拼接文字 delta，返回完整字符串。
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
