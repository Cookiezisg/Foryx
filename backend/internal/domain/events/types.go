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
	mcpdomain "github.com/sunweilin/forgify/backend/internal/domain/mcp"
	skilldomain "github.com/sunweilin/forgify/backend/internal/domain/skill"
	subagentdomain "github.com/sunweilin/forgify/backend/internal/domain/subagent"
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
// Subagent contexts: when a subagent sub-runner publishes through this
// event, SubagentRunID + ParentConversationID + SubagentRun are filled
// in so the frontend can route the message into the per-run small-window
// AND read live run state (token totals, lastTool*) from the same frame.
// Main-chat publishes leave all three nil/zero — wire stays exactly
// backward-compatible (all three are omitempty + the embedded Message
// JSON is hoisted to the top level by MarshalJSON).
//
// ChatMessage 携带完整 Message 快照。在 message 级关键时刻触发（slot 创建 /
// 每个 LLM token / tool_call 识别 / args 完整 / tool_result / 终态写入），
// 不在 tool 执行内部逐字节推送。
//
// Subagent 上下文：subagent sub-runner 发本事件时填 SubagentRunID +
// ParentConversationID + SubagentRun，让前端按 run 路由到流式小窗 +
// 同帧读 run 实时状态（token 累计、lastTool*）。主对话发布全置 nil/zero
// ——wire 完全向后兼容（三字段 omitempty + MarshalJSON 把嵌入 Message
// 的 JSON 提升到顶层）。
type ChatMessage struct {
	*chatdomain.Message
	SubagentRunID        string                       `json:"subagentRunId,omitempty"`
	ParentConversationID string                       `json:"parentConversationId,omitempty"`
	SubagentRun          *subagentdomain.SubagentRun  `json:"subagentRun,omitempty"`
}

func (ChatMessage) EventName() string { return "chat.message" }

// MarshalJSON hoists the embedded *chatdomain.Message fields to the
// top level (so wire shape matches GET /api/v1/messages/{id}) and merges
// the three subagent-context fields beside them when set. Main-chat
// callers (no subagent fields) produce the exact same bytes as
// json.Marshal(e.Message) did before the extension.
//
// MarshalJSON 把嵌入的 *chatdomain.Message 字段提升到顶层（wire 形状与
// GET /api/v1/messages/{id} 一致），并在设置时合并三个 subagent 上下文字段。
// 主对话调用方（三字段未设）产出与扩展前 json.Marshal(e.Message) 完全相同
// 的字节。
func (e ChatMessage) MarshalJSON() ([]byte, error) {
	if e.Message == nil {
		return []byte("null"), nil
	}
	// Fast path: zero subagent context → emit Message unchanged so existing
	// wire bytes match exactly (no new keys, no key reordering).
	// 快路径：零 subagent 上下文 → Message 原样输出，wire 字节完全不变。
	if e.SubagentRunID == "" && e.ParentConversationID == "" && e.SubagentRun == nil {
		return json.Marshal(e.Message)
	}
	// Slow path: marshal Message, decode to a generic map, splice the three
	// extras in. Cheap because subagent path runs at sub-runner cadence
	// (much less frequent than main chat publishing).
	// 慢路径：marshal Message → 解到 map → 插入三字段。subagent 路径以
	// sub-runner 节奏运行（远少于主对话推送），开销可接受。
	base, err := json.Marshal(e.Message)
	if err != nil {
		return nil, err
	}
	var merged map[string]any
	if err := json.Unmarshal(base, &merged); err != nil {
		return nil, err
	}
	if e.SubagentRunID != "" {
		merged["subagentRunId"] = e.SubagentRunID
	}
	if e.ParentConversationID != "" {
		merged["parentConversationId"] = e.ParentConversationID
	}
	if e.SubagentRun != nil {
		merged["subagentRun"] = e.SubagentRun
	}
	return json.Marshal(merged)
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

// MCP carries a full snapshot of every configured MCP server's current
// status (per mcp.md §9). Fired on Connect / Disconnect / Reconnect /
// AddServer / RemoveServer / subprocess Wait detecting disconnect /
// degraded transitions / auto-heal back to ready. Whole-snapshot model
// (not single-server delta) because the UI replaces local state in one
// go and snapshot count is small (~10 servers max in practice).
//
// JSON shape: {"servers": [ServerStatus, ...]} — flat object, NOT a
// hoisted Servers slice, so the wire matches what callers naturally
// expect from a "list of servers" event.
//
// MCP 携全配置 MCP server 的当前状态快照（mcp.md §9）。Connect /
// Disconnect / Reconnect / AddServer / RemoveServer / 子进程 Wait 检测到
// disconnect / degraded 转换 / 自愈回 ready 都触发。整快照（非单 server
// 增量）——UI 一次性替换本地，server 数实践中很小（≤ 10）。
//
// JSON 形状：{"servers": [...]}——扁平对象，非 Servers 升顶层，让 wire
// 与"server 列表事件"的自然预期一致。
type MCP struct {
	Servers []mcpdomain.ServerStatus `json:"servers"`
}

func (MCP) EventName() string { return "mcp" }

// Skill carries the full snapshot of every loaded skill (skill.md §10).
// Service.Scan publishes after each rescan (initial boot, fsnotify event,
// manual :refresh). Whole-list snapshot (no per-skill delta) — UI replaces
// local cache in one go; skill counts are small in practice (≤ ~50).
//
// Wire shape: {"skills": [...]} — body is NOT included on each Skill
// (per spec progressive-disclosure: L2 body fetched separately via
// GET /skills/{name}/body when the user opens the editor).
//
// Skill 携每个已加载 skill 的全快照（skill.md §10）。Service.Scan 后发布
// （首次 boot、fsnotify、手动 :refresh）。全快照（无 per-skill 增量）让
// UI 一次性替换本地；实践 skill 数量小（≤ ~50）。
//
// 线形：{"skills": [...]} — body 不含（spec progressive-disclosure：L2
// body 经 GET /skills/{name}/body 单独取，用户打开编辑器时拉）。
type Skill struct {
	Skills []*skilldomain.Skill `json:"skills"`
}

func (Skill) EventName() string { return "skill" }

