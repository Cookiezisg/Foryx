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

// Subagent ctx keys: SubagentDepth gates the recursion check inside
// SubagentTool.Execute (the structural defense is registry-level tool
// filtering, but depth is a runtime belt-and-suspenders); SubagentRunID
// lets sub-runner code attribute downstream events back to the spawn.
//
// Subagent ctx key：SubagentDepth 给 SubagentTool.Execute 内的递归检查兜底
// （结构性防线是注册表层的工具过滤，深度只是运行时双保险）；SubagentRunID
// 让 sub-runner 代码把下游事件归属回到 spawn。
type subagentDepthKey struct{}
type subagentRunIDKey struct{}

// WithSubagentDepth returns a copy of ctx with depth (≥ 0). Increment by
// one each time SubagentTool.Execute spawns a sub-runner.
//
// WithSubagentDepth 返回带 depth（≥ 0）的 ctx 拷贝。每次 SubagentTool
// .Execute 起 sub-runner 时 +1。
func WithSubagentDepth(ctx context.Context, depth int) context.Context {
	return context.WithValue(ctx, subagentDepthKey{}, depth)
}

// GetSubagentDepth returns the current subagent depth (0 in main chat).
// Always returns a usable int; absent means depth=0.
//
// GetSubagentDepth 返回当前 subagent 深度（主对话为 0）。总返可用 int；
// 缺失即 depth=0。
func GetSubagentDepth(ctx context.Context) int {
	if d, ok := ctx.Value(subagentDepthKey{}).(int); ok {
		return d
	}
	return 0
}

// WithSubagentRunID returns a copy of ctx carrying the sub-run's ID. Set
// by the spawning Service before invoking loop.Run; read by tools that
// want to surface the sub-run reference in their telemetry.
//
// WithSubagentRunID 返回携带 sub-run ID 的 ctx 拷贝。spawn Service 在调
// loop.Run 前设置；想在 telemetry 里附带 sub-run 引用的工具读它。
func WithSubagentRunID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, subagentRunIDKey{}, id)
}

// GetSubagentRunID returns the sub-run's ID; ok=false when absent or
// empty (we are not inside a subagent loop).
//
// GetSubagentRunID 返回 sub-run ID；缺失或空时 ok=false（不在 subagent
// loop 内）。
func GetSubagentRunID(ctx context.Context) (string, bool) {
	id, ok := ctx.Value(subagentRunIDKey{}).(string)
	return id, ok && id != ""
}
