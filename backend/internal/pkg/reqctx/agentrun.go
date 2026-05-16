package reqctx

import (
	"context"
	"errors"
)

// ErrMissingConversationID is returned by RequireConversationID.
//
// ErrMissingConversationID 由 RequireConversationID 返回。
var ErrMissingConversationID = errors.New("reqctx: missing conversation id in context")

type conversationIDKey struct{}
type messageIDKey struct{}
type toolCallIDKey struct{}
type parentBlockIDKey struct{}

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

// WithParentBlockID returns a copy of ctx carrying the current emit-tree parent block.
//
// WithParentBlockID 返回携带当前 emit 树父 block 的 ctx 拷贝。
func WithParentBlockID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, parentBlockIDKey{}, id)
}

// GetParentBlockID returns the current emit-tree parent block ID; ok=false when absent or empty.
//
// GetParentBlockID 返回当前 emit 树父 block ID；缺失或空时 ok=false。
func GetParentBlockID(ctx context.Context) (string, bool) {
	id, ok := ctx.Value(parentBlockIDKey{}).(string)
	return id, ok && id != ""
}

type subagentDepthKey struct{}

// WithSubagentDepth returns a copy of ctx with depth (≥ 0); increment per sub-runner spawn.
//
// WithSubagentDepth 返回带 depth（≥ 0）的 ctx 拷贝；每起一次 sub-runner +1。
func WithSubagentDepth(ctx context.Context, depth int) context.Context {
	return context.WithValue(ctx, subagentDepthKey{}, depth)
}

// GetSubagentDepth returns the current subagent depth; absent means 0.
//
// GetSubagentDepth 返回当前 subagent 深度；缺失即 0。
func GetSubagentDepth(ctx context.Context) int {
	if d, ok := ctx.Value(subagentDepthKey{}).(int); ok {
		return d
	}
	return 0
}
