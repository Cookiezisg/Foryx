package reqctx

import (
	"context"
	"errors"
)

// Per-agent-run identifiers (conversation / assistant message / LLM tool call).
// Stamped by the chat layer just before invoking a tool. Lifetime: one tool
// call. Missing values are not bugs — events with empty filter keys silently
// go nowhere; only conversationID has a Require-form sentinel for callers
// that need to surface it.
//
// Per-agent-run ID（conversation / 助手消息 / LLM tool call）。chat 层调 tool
// 前注入。生命周期：单次 tool 调用。缺失非 bug——事件 filter key 为空时静默
// 无订阅者；仅 conversationID 提供 Require-form sentinel 供需上抛的调用方用。

// ErrMissingConversationID is returned by RequireConversationID.
//
// ErrMissingConversationID 由 RequireConversationID 返回。
var ErrMissingConversationID = errors.New("reqctx: missing conversation id in context")

type conversationIDKey struct{}
type messageIDKey struct{}
type toolCallIDKey struct{}

// WithConversationID returns a copy of ctx carrying id.
//
// WithConversationID 返回携带 id 的 ctx 拷贝。
func WithConversationID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, conversationIDKey{}, id)
}

// GetConversationID returns the conversation ID; ok=false when absent or empty.
//
// GetConversationID 取 conversation ID；缺失或空时 ok=false。
func GetConversationID(ctx context.Context) (string, bool) {
	id, ok := ctx.Value(conversationIDKey{}).(string)
	return id, ok && id != ""
}

// RequireConversationID returns the ID or ErrMissingConversationID.
//
// RequireConversationID 返回 ID 或 ErrMissingConversationID。
func RequireConversationID(ctx context.Context) (string, error) {
	if id, ok := GetConversationID(ctx); ok {
		return id, nil
	}
	return "", ErrMissingConversationID
}

// WithMessageID returns a copy of ctx carrying the in-flight assistant message ID.
//
// WithMessageID 返回携带当前生成中的 assistant 消息 ID 的 ctx 拷贝。
func WithMessageID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, messageIDKey{}, id)
}

// GetMessageID returns the assistant message ID; ok=false when absent or empty.
//
// GetMessageID 取 assistant 消息 ID；缺失或空时 ok=false。
func GetMessageID(ctx context.Context) (string, bool) {
	id, ok := ctx.Value(messageIDKey{}).(string)
	return id, ok && id != ""
}

// WithToolCallID returns a copy of ctx carrying the LLM tool-call ID.
//
// WithToolCallID 返回携带 LLM tool-call ID 的 ctx 拷贝。
func WithToolCallID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, toolCallIDKey{}, id)
}

// GetToolCallID returns the LLM tool-call ID; ok=false when absent or empty.
//
// GetToolCallID 取 LLM tool-call ID；缺失或空时 ok=false。
func GetToolCallID(ctx context.Context) (string, bool) {
	id, ok := ctx.Value(toolCallIDKey{}).(string)
	return id, ok && id != ""
}
