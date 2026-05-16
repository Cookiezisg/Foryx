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
	"time"
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

// Generate consumes Stream, concatenates text deltas, returns the assembled
// string. Auto-retries transient upstream failures (429 / 5xx / connection
// errors) up to retryMaxAttempts with exponential backoff — see withRetry
// for the policy. Retries are only safe here because Generate has no
// observable side effects until it returns (no partial UI emission), so
// "retry from scratch" never loses user-visible content. Stream() callers
// that yield events as they arrive (chat agent loop) must NOT use this
// retry policy — they get raw Client.Stream() instead.
//
// Generate 消费 Stream 拼接文字 delta 返完整串。upstream 短期失败
// （429 / 5xx / 连接错）自动重试至 retryMaxAttempts，指数退避（见
// withRetry）。Generate 在返回前无可观察副作用（不向 UI emit），所以
// "从头重试"不丢用户已见内容；这是 Stream() 直调（chat agent loop）
// 不能套同样 retry 的关键区别。
func Generate(ctx context.Context, c Client, req Request) (string, error) {
	return withRetry(ctx, func() (string, error) {
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
	})
}

const (
	retryMaxAttempts  = 3                      // initial + 2 retries
	retryInitialDelay = 500 * time.Millisecond // first backoff
	retryDelayFactor  = 3                      // each retry waits factor× the previous
)

// withRetry runs fn up to retryMaxAttempts times, sleeping between
// attempts when fn returned a retryable error. Returns the last error
// when retries exhaust, or ctx.Err() if cancellation interrupts the
// backoff sleep.
//
// withRetry 把 fn 跑至多 retryMaxAttempts 次；fn 返可重试错时退避 sleep。
// 用完 attempts 返最后一次的错；backoff sleep 期间 ctx 取消返 ctx.Err。
func withRetry(ctx context.Context, fn func() (string, error)) (string, error) {
	delay := retryInitialDelay
	var lastErr error
	for attempt := 0; attempt < retryMaxAttempts; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(delay):
			}
			delay *= retryDelayFactor
		}
		out, err := fn()
		if err == nil {
			return out, nil
		}
		if !isRetryable(err) {
			return "", err
		}
		lastErr = err
	}
	return "", lastErr
}

// isRetryable identifies upstream errors worth a second try: rate limit,
// generic provider errors (often 5xx / network blips), and ctx-derived
// errors EXCEPT explicit cancellation (which is caller intent). Auth /
// bad-request / model-not-found are not retryable — same input would
// fail the same way.
//
// isRetryable 识别值得重试的 upstream 错：限流、通用 provider 错（多
// 半是 5xx / 网络抖动）、ctx 派生错（但显式 cancel 排除——那是 caller
// 意图）。Auth / 参数错 / model-不存在 不重试——同样输入只会再挂。
func isRetryable(err error) bool {
	if err == nil {
		return false
	}
	switch {
	case errors.Is(err, ErrRateLimited):
		return true
	case errors.Is(err, ErrProviderError):
		return true
	case errors.Is(err, context.DeadlineExceeded):
		return true
	case errors.Is(err, ErrAuthFailed),
		errors.Is(err, ErrBadRequest),
		errors.Is(err, ErrModelNotFound),
		errors.Is(err, context.Canceled):
		return false
	}
	return false
}
