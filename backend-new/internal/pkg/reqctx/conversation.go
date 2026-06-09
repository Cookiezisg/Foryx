package reqctx

import (
	"context"
	"errors"
)

// ErrMissingConversationID is returned when a conversation-scoped call runs without a
// conversation id in ctx — a wiring bug (the chat / agent loop must seed it), not a user
// error. Mirrors ErrMissingWorkspaceID (500, not 401).
//
// ErrMissingConversationID 在对话作用域调用缺 ctx 对话 id 时返回——接线 bug（chat / agent loop
// 须埋种子），非用户错误。对称 ErrMissingWorkspaceID（500，非 401）。
var ErrMissingConversationID = errors.New("reqctx: missing conversation id in context")

type (
	conversationIDKey struct{}
	subagentIDKey     struct{}
	messageIDKey      struct{}
	toolCallIDKey     struct{}
)

// SetConversationID returns a copy of ctx carrying the conversation id. Seeded by the
// chat / agent loop when it starts running a conversation's turn — the first per-run
// identity beyond workspace, planted here so conversation-scoped modules (todo, …) read
// it without importing the conversation business package.
//
// SetConversationID 返回携带对话 id 的 ctx 拷贝。chat / agent loop 跑某对话回合时埋下——超出
// workspace 的首个 per-run 身份，埋在此处使对话作用域模块（todo 等）无需 import conversation 业务包即可读。
func SetConversationID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, conversationIDKey{}, id)
}

// GetConversationID returns the conversation id; ok=false when missing or empty.
//
// GetConversationID 取对话 id；缺失或空时 ok=false。
func GetConversationID(ctx context.Context) (string, bool) {
	id, ok := ctx.Value(conversationIDKey{}).(string)
	return id, ok && id != ""
}

// RequireConversationID is the (string, error) form of GetConversationID, for
// conversation-scoped store / app methods that bubble up the wiring error.
//
// RequireConversationID 是 GetConversationID 的 (string, error) 版，供冒泡接线错误的对话作用域 store / app 方法用。
func RequireConversationID(ctx context.Context) (string, error) {
	id, ok := GetConversationID(ctx)
	if !ok {
		return "", ErrMissingConversationID
	}
	return id, nil
}

// SetSubagentID returns a copy of ctx marking the current run as a subagent invocation.
// Seeded by the subagent loop (波次 3); absent in a main-conversation turn. It refines
// the execution scope: todo lists and other per-run scratch state key on
// (conversation, subagent?) so a subagent's plan never pollutes the parent's board.
// The seed is planted now (todo is its first consumer) even though the writer ships later.
//
// SetSubagentID 返回标记当前运行为 subagent 调用的 ctx 拷贝。subagent loop（波次 3）埋下；
// 主对话回合无此值。它细化执行作用域：todo 清单等 per-run 草稿状态按 (conversation, subagent?)
// 分键，使 subagent 的计划永不污染父看板。种子现在就埋（todo 是首个消费者），尽管写入方更晚才上线。
func SetSubagentID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, subagentIDKey{}, id)
}

// GetSubagentID returns the subagent id with ok=true only inside a subagent run; a main
// conversation turn returns ok=false. Optional by nature, unlike the required ids above.
//
// GetSubagentID 仅在 subagent 运行内返回 subagent id 且 ok=true；主对话回合返 ok=false。
// 与上面的必填 id 不同，它天然可选。
func GetSubagentID(ctx context.Context) (string, bool) {
	id, ok := ctx.Value(subagentIDKey{}).(string)
	return id, ok && id != ""
}

// SetMessageID seeds the current assistant turn's message id. The host (chat / agent)
// creates the message row before loop.Run and plants its id here so the loop's stream
// emitter can anchor every block it produces to that message (a block's parentId is the
// message id). Absent on a non-streaming run — the emitter then skips the live push.
//
// SetMessageID 埋当前 assistant 回合的 message id。host（chat / agent）在 loop.Run 前建 message
// 行、把 id 埋此，使 loop 的流式 emitter 把它产的每个 block 锚到该 message（block 的 parentId 即
// message id）。非流式运行无此值——emitter 届时跳过 live 推送。
func SetMessageID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, messageIDKey{}, id)
}

// GetMessageID returns the message id; ok=false when missing or empty.
//
// GetMessageID 取 message id；缺失或空时 ok=false。
func GetMessageID(ctx context.Context) (string, bool) {
	id, ok := ctx.Value(messageIDKey{}).(string)
	return id, ok && id != ""
}

// SetToolCallID seeds the id of the tool_call a tool is executing under. The loop plants it
// right before a tool's Execute so a tool can know its own call's block id — the anchor a
// Subagent tool needs so the subagent run's message subtree nests under the spawning tool_call
// (E3). Absent outside a tool execution.
//
// SetToolCallID 埋当前正在执行的工具所属 tool_call 的 id。loop 在工具 Execute 前埋下，使工具能知道
// 自己这次调用的 block id——Subagent 工具需要的锚点，使 subagent run 的 message 子树挂在派它的
// tool_call 下（E3）。工具执行之外无此值。
func SetToolCallID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, toolCallIDKey{}, id)
}

// GetToolCallID returns the tool_call id the current tool is executing under; ok=false outside
// a tool execution.
//
// GetToolCallID 取当前工具执行所属的 tool_call id；工具执行之外 ok=false。
func GetToolCallID(ctx context.Context) (string, bool) {
	id, ok := ctx.Value(toolCallIDKey{}).(string)
	return id, ok && id != ""
}
