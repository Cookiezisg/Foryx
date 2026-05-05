// Package events defines typed payloads streamed over Bridge + SSE.
// Model: entity-state — each event carries the full current snapshot of one
// domain entity in the same shape as the corresponding REST GET endpoint;
// subscribers render by replacing their local copy keyed on entity ID.
// Names: snake_case dotted (e.g. "chat.message", "forge", "conversation").
//
// Package events 定义流过 Bridge + SSE 的类型化载荷。
// 模型 entity-state：每个事件携带某 domain entity 完整当前快照，形状与
// 对应 REST GET 一致；订阅方按 entity ID 替换本地拷贝。
// 命名：snake_case 加点号前缀（"chat.message" / "forge" / "conversation"）。
package events

import (
	"encoding/json"

	chatdomain "github.com/sunweilin/forgify/backend/internal/domain/chat"
	convdomain "github.com/sunweilin/forgify/backend/internal/domain/conversation"
	forgedomain "github.com/sunweilin/forgify/backend/internal/domain/forge"
	tododomain "github.com/sunweilin/forgify/backend/internal/domain/todo"
)

// Event is any typed message flowing through a Bridge.
//
// Event 是 Bridge 中流动的类型化消息。
type Event interface {
	EventName() string
}

// ChatMessage carries a full Message snapshot. Fired at message-level
// milestones (slot open, each LLM token, tool_call identified, args complete,
// tool_result, final write) — never per-byte inside tool execution.
//
// ChatMessage 携带完整 Message 快照。在 message 级关键时刻触发（slot 创建 /
// 每个 LLM token / tool_call 识别 / args 完整 / tool_result / 终态写入），
// 不在 tool 执行内部逐字节推送。
type ChatMessage struct {
	*chatdomain.Message
}

func (ChatMessage) EventName() string { return "chat.message" }

func (e ChatMessage) MarshalJSON() ([]byte, error) {
	if e.Message == nil {
		return []byte("null"), nil
	}
	return json.Marshal(e.Message)
}

// Forge carries a full Forge snapshot (including .Pending). Fired on every
// Forge change, including per-token updates during create_forge / edit_forge
// code streaming. create_forge pre-saves a stub so streaming always carries a
// persisted entity.
//
// Forge 携带完整 Forge 快照（含 .Pending）。任何变化都触发，含
// create_forge / edit_forge 期间逐 token 更新。create_forge 入口预存 stub，
// 让流式快照始终承载已落库 entity。
type Forge struct {
	*forgedomain.Forge
}

func (Forge) EventName() string { return "forge" }

func (e Forge) MarshalJSON() ([]byte, error) {
	if e.Forge == nil {
		return []byte("null"), nil
	}
	return json.Marshal(e.Forge)
}

// Conversation carries a full Conversation snapshot.
//
// Conversation 携带完整 Conversation 快照。
type Conversation struct {
	*convdomain.Conversation
}

func (Conversation) EventName() string { return "conversation" }

func (e Conversation) MarshalJSON() ([]byte, error) {
	if e.Conversation == nil {
		return []byte("null"), nil
	}
	return json.Marshal(e.Conversation)
}

// Todo carries a full Todo snapshot. Fired on Create / Update / SoftDelete
// so the LLM and any UI sees todo-list state changes in near real time.
//
// Todo 携带完整 Todo 快照。Create / Update / SoftDelete 触发，
// 让 LLM 与 UI 近实时看到 TODO 列表变化。
type Todo struct {
	*tododomain.Todo
}

func (Todo) EventName() string { return "todo" }

func (e Todo) MarshalJSON() ([]byte, error) {
	if e.Todo == nil {
		return []byte("null"), nil
	}
	return json.Marshal(e.Todo)
}
