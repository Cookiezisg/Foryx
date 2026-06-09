// Package llm is a provider-agnostic LLM streaming client built on iter.Seq.
// It speaks each provider's wire dialect with the standard library only (no SDKs),
// exposing one Client.Stream contract upward. Errors that reach the wire use the
// structured domain error types so transport maps them via statusForKind.
//
// Package llm 是基于 iter.Seq 的 provider-agnostic LLM 流式客户端。仅用标准库
// （无 SDK）讲各家 wire 方言，对上暴露统一的 Client.Stream 契约。会上线缆的错误用
// 结构化 domain error，使 transport 经 statusForKind 映射。
package llm

import (
	"context"
	"errors"
	"iter"
	"strings"
	"time"

	"encoding/json"

	errorsdomain "github.com/sunweilin/forgify/backend/internal/domain/errors"
)

// LLM upstream failures, classified by HTTP status (see classifyHTTPError). These are
// structured domain errors so a failure surfaced through Stream maps to the right HTTP
// status at transport with no special case.
//
// LLM upstream 失败，按 HTTP 状态分类（见 classifyHTTPError）。均为结构化 domain 错误，
// 经 Stream 冒泡后 transport 零特例映射到正确 HTTP 状态。
var (
	ErrAuthFailed    = errorsdomain.New(errorsdomain.KindUnauthorized, "LLM_AUTH_FAILED", "llm: authentication failed")
	ErrRateLimited   = errorsdomain.New(errorsdomain.KindRateLimited, "LLM_RATE_LIMITED", "llm: rate limited")
	ErrBadRequest    = errorsdomain.New(errorsdomain.KindInvalid, "LLM_BAD_REQUEST", "llm: bad request")
	ErrModelNotFound = errorsdomain.New(errorsdomain.KindNotFound, "LLM_MODEL_NOT_FOUND", "llm: model not found")
	ErrProviderError = errorsdomain.New(errorsdomain.KindBadGateway, "LLM_PROVIDER_ERROR", "llm: provider error")
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
	// Signature carries the Anthropic-issued opaque signature for a completed thinking
	// block. Set on the final EventReasoning event so the round-trip can echo it verbatim.
	//
	// Signature 是 Anthropic 颁发的不透明签名，随最后一个 thinking block 的
	// EventReasoning 事件到达，多轮对话时必须原样回传。
	Signature string

	ToolIndex int
	ToolID    string
	ToolName  string
	ArgsDelta string

	FinishReason string
	InputTokens  int
	OutputTokens int

	Err error
}

// Role is the speaker role on a conversation turn (LLM wire role, includes tool).
//
// Role 是对话回合中的发言方角色（LLM wire 角色，含 tool）。
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
	// ReasoningSignature is the opaque Anthropic-issued signature echoed verbatim with
	// the thinking block in subsequent requests. Empty for non-Anthropic / non-thinking.
	//
	// ReasoningSignature 是 Anthropic 颁发的不透明签名，后续请求必须原样随 thinking
	// block 回传；非 Anthropic / 无 thinking 响应留空。
	ReasoningSignature string
}

// ContentPart is one element of a multi-modal user message. Type selects the shape:
//   - PartText     → Text
//   - PartImageURL → ImageURL holds a data-URL ("data:<mime>;base64,<data>") for a local
//     attachment, or a remote https URL. Each provider parses/forwards it natively.
//   - PartFile     → MediaType + base64 Data + Filename, for a document (PDF) sent inline.
//
// Each provider renders these into its own wire (no shared base — infra/llm keeps every
// provider self-contained; a provider that can't carry a part type degrades on its own).
//
// ContentPart 是多模态 user 消息的一个元素。Type 选形态：PartText→Text；PartImageURL→ImageURL 为
// data-URL（本地附件）或远程 https URL，各家自行解析/透传；PartFile→MediaType + base64 Data +
// Filename，文档（PDF）内联。每家 provider 各自渲成自己的 wire（无共享基座——各家自包含；无法承载
// 某 part 类型的家各自优雅降级）。
type ContentPart struct {
	Type      string
	Text      string
	ImageURL  string
	MediaType string
	Data      string
	Filename  string
}

// ContentPart.Type values. PartImageURL keeps the legacy "image_url" wire name (the existing
// per-provider switch convention); PartFile is the document/PDF carrier.
//
// ContentPart.Type 取值。PartImageURL 沿用历史 "image_url" 线缆名（既有各家 switch 约定）；
// PartFile 是文档/PDF 载体。
const (
	PartText     = "text"
	PartImageURL = "image_url"
	PartFile     = "file"
)

// LLMToolCall is one tool invocation in an assistant message; Arguments is a JSON object string.
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

	// MaxTokens optionally overrides the model's max output cap; 0 → the provider fills it
	// from its own static spec. Each provider owns its model knowledge; infra/llm holds no
	// cross-provider catalog.
	//
	// MaxTokens 可选覆盖模型输出上限；0 → provider 用自身静态规格自填。每家 provider 自持
	// 模型知识，infra/llm 不持跨家目录。
	MaxTokens int

	// Options is the sole carrier of user-selected reasoning/config knobs, keyed by each
	// provider's native parameter name with native values (e.g. {"reasoning_effort":"high"},
	// {"thinking":"enabled"}, {"thinkingLevel":"high"}, {"effort":"max"}). Each adapter reads
	// only the keys it recognises — no neutral abstraction across providers.
	//
	// Options 是用户所选推理/配置旋钮的唯一载体，按各家原生参数名 + 原生取值（如
	// {"reasoning_effort":"high"}）。每个 adapter 只读自己认识的 key——跨家零中立抽象。
	Options map[string]string

	// DisableStream forces non-streaming wire mode (Ollama+tools workaround).
	// DisableStream 强制 non-streaming（Ollama 有 tools 时绕 bug）。
	DisableStream bool
}

// Client streams LLM events via iter.Seq; ctx cancel stops cleanly.
//
// Client 通过 iter.Seq 流式输出 LLM 事件；ctx 取消可干净停止。
type Client interface {
	Stream(ctx context.Context, req Request) iter.Seq[StreamEvent]
}

// Generate consumes Stream, concatenates text deltas, returns the assembled string.
// Auto-retries transient upstream failures (429 / 5xx / connection) with exponential
// backoff. Safe only because Generate has no observable side effects until it returns
// (no partial UI emission) — Stream() callers that emit as events arrive (chat loop)
// must NOT use this; they consume raw Client.Stream().
//
// Generate 消费 Stream 拼接 text delta 返完整串，upstream 短期失败自动指数退避重试。
// 仅因 Generate 返回前无可观察副作用（不向 UI emit）才安全——边到边 emit 的 Stream()
// 直调方（chat loop）不能套此 retry，直接消费裸 Client.Stream()。
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

// withRetry runs fn up to retryMaxAttempts times, backing off between attempts when fn
// returns a retryable error. Returns the last error when retries exhaust, or ctx.Err()
// if cancellation interrupts the backoff sleep.
//
// withRetry 把 fn 跑至多 retryMaxAttempts 次，可重试错时退避；用完返最后一次错；
// backoff 期间 ctx 取消返 ctx.Err。
func withRetry(ctx context.Context, fn func() (string, error)) (string, error) {
	delay := retryInitialDelay
	var lastErr error
	for attempt := range retryMaxAttempts {
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

// isRetryable identifies upstream errors worth a retry: rate limit, generic provider
// errors (often 5xx / network blips), and deadline. Auth / bad-request / model-not-found
// and explicit cancellation are not retryable — same input fails the same way.
//
// isRetryable 识别值得重试的 upstream 错：限流、通用 provider 错（多半 5xx/网络抖动）、
// 超时。Auth / 参数错 / model-不存在 与显式 cancel 不重试。
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
