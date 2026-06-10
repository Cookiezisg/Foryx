// Package messages is the content model for one conversation turn: the Block tree
// (reasoning / text / tool_call / tool_result) an assistant turn decomposes into, plus
// the in-memory ToolCallData a streamed tool call parses to. It is deliberately separate
// from domain/stream — stream is the TRANSPORT (how a frame reaches the front end),
// messages is the CONTENT (what a turn is made of). The shared ReAct engine (app/loop)
// produces Blocks and depends on THIS package, not on any single consumer like chat, so
// agent / subagent / chat all share one neutral content model.
//
// Package messages 是对话单回合的内容模型：一个 assistant 回合分解成的 Block 树
// （reasoning / text / tool_call / tool_result），加上流式 tool call 解析出的内存
// ToolCallData。它刻意与 domain/stream 分离——stream 是传输（帧怎么到前端），messages
// 是内容（回合由什么组成）。共享 ReAct 引擎（app/loop）产 Block 并依赖**本包**、而非
// 依赖 chat 这种具体消费者，故 agent / subagent / chat 共享一个中立内容模型。
package messages

import (
	"context"
	"time"

	errorsdomain "github.com/sunweilin/forgify/backend/internal/domain/errors"
)

// Block is one node of an assistant turn's content tree, persisted to message_blocks.
// loop produces Blocks in memory; the host persists them (the store lives in chat M5.2).
// Seq is assigned at persist time, not by loop. ContextRole is set later by the compactor
// (contextmgr M5.3) and only projects how the block reaches LLM history — stored Content
// is never rewritten.
//
// Block 是 assistant 回合内容树的一个节点，持久化到 message_blocks。loop 内存产 Block；
// host 落盘（store 在 chat M5.2）。Seq 在落盘时分配、非 loop 设。ContextRole 后由压缩器
// （contextmgr M5.3）设置，只投影 block 如何进入 LLM 历史——落库 Content 永不改写。
type Block struct {
	ID             string         `db:"id,pk" json:"id"`
	ConversationID string         `db:"conversation_id" json:"conversationId"`
	WorkspaceID    string         `db:"workspace_id,ws" json:"-"` // D2 物理隔离；orm 自动填/过滤（落盘时设，loop 内存态不填）
	MessageID      string         `db:"message_id" json:"messageId"`
	ParentBlockID  string         `db:"parent_block_id" json:"parentBlockId,omitempty"`
	Seq            int64          `db:"seq" json:"seq"`
	Type           string         `db:"type" json:"type"`
	Attrs          map[string]any `db:"attrs,json" json:"attrs,omitempty"`
	Content        string         `db:"content" json:"content"`
	Status         string         `db:"status" json:"status"`
	Error          string         `db:"error" json:"error,omitempty"`
	ContextRole    string         `db:"context_role" json:"contextRole,omitempty"`
	CreatedAt      time.Time      `db:"created_at,created" json:"createdAt"`
	UpdatedAt      time.Time      `db:"updated_at,updated" json:"updatedAt"`
}

// Block types — the content-tree node kinds loop emits. Deeper hierarchy (a subagent's
// message subtree under a tool_call) is expressed via stream Open.ParentID, NOT via new
// block types, so this set stays minimal.
//
// Block 类型——loop 发的内容树节点种类。更深层级（subagent 消息子树挂在 tool_call 下）
// 经 stream Open.ParentID 表达、**不**靠新增块型，故此集合保持精简。
const (
	BlockTypeText       = "text"
	BlockTypeReasoning  = "reasoning"
	BlockTypeToolCall   = "tool_call"
	BlockTypeToolResult = "tool_result"
	// BlockTypeCompaction marks a context-compaction summary (contextmgr M5.3); loop drops
	// it from LLM history because the content already lives in conversation.summary.
	//
	// BlockTypeCompaction 标记上下文压缩摘要（contextmgr M5.3）；loop 从 LLM 历史丢弃它，
	// 内容已在 conversation.summary。
	BlockTypeCompaction = "compaction"

	// BlockTypeProgress is a tool's intermediate progress (bash output, env-fix log, a handler
	// method's yields …), streamed live under its tool_call (Open.ParentID = tool_call id) via
	// loop.ToolProgress AND persisted with the turn so the front end can replay it after a reload.
	// It is a first-class persisted block (in IsValidBlockType + the store CHECK), but the LLM
	// history projection (BlocksToAssistantLLM) is a type whitelist — text/reasoning/tool_call/
	// tool_result — so a progress block is never fed back to the model. The durable answer the LLM
	// reads is still the tool_result; progress is durable UI detail.
	//
	// BlockTypeProgress 是工具的中间过程（bash 输出、env-fix log、handler method 的 yield…），经
	// loop.ToolProgress 在其 tool_call 下（Open.ParentID=tool_call id）实时流，**并随回合持久化**，使前端
	// 刷新后可重放。它是一等持久块（进 IsValidBlockType + store CHECK），但 LLM 历史投影
	// （BlocksToAssistantLLM）是类型白名单——text/reasoning/tool_call/tool_result——故 progress 块永不回喂
	// 模型。LLM 读的耐久答案仍是 tool_result；progress 是耐久的 UI 细节。
	BlockTypeProgress = "progress"
)

// IsValidBlockType reports whether t is a known block type (store CHECK / contract对账).
//
// IsValidBlockType 报告 t 是否已知块型（供 store CHECK / 契约对账）。
func IsValidBlockType(t string) bool {
	switch t {
	case BlockTypeText, BlockTypeReasoning, BlockTypeToolCall, BlockTypeToolResult, BlockTypeCompaction, BlockTypeProgress:
		return true
	}
	return false
}

// Statuses span both message and block lifecycle. A message is pending before its turn
// starts; a block is implicitly streaming between its open and close. The three terminal
// states are shared and map 1:1 onto stream.Close statuses; pending applies only to a
// message, streaming to both.
//
// 状态横跨 message 与 block 生命周期。message 在回合开始前为 pending；block 在 open 与 close
// 之间隐含为 streaming。三个终态共享、与 stream.Close 状态 1:1 映射；pending 仅用于 message，
// streaming 两者皆用。
const (
	StatusPending   = "pending"
	StatusStreaming = "streaming"
	StatusCompleted = "completed"
	StatusError     = "error"
	StatusCancelled = "cancelled"
)

// IsValidStatus reports whether s is a known message/block status.
//
// IsValidStatus 报告 s 是否已知 message/block 状态。
func IsValidStatus(s string) bool {
	switch s {
	case StatusPending, StatusStreaming, StatusCompleted, StatusError, StatusCancelled:
		return true
	}
	return false
}

// StopReason is why an assistant turn ended. MaxSteps is a non-success terminal — the loop
// hit its step ceiling before the model finished, surfaced honestly so the UI can offer
// "continue" (rather than masquerading as a completed end_turn).
//
// StopReason 是 assistant 回合结束原因。MaxSteps 是非成功终态——loop 在模型完成前撞到
// 步数上限，诚实暴露使 UI 能提供「继续」（而非冒充 completed end_turn）。
const (
	StopReasonEndTurn   = "end_turn"
	StopReasonMaxTokens = "max_tokens"
	StopReasonMaxSteps  = "max_steps"
	StopReasonCancelled = "cancelled"
	StopReasonError     = "error"
)

// ContextRole projects how a block reaches LLM history without rewriting stored Content:
// hot = full, warm = truncated preview, cold = omitted-with-marker, archived = dropped
// (content folded into conversation.summary). Set by the compactor (contextmgr M5.3); the
// default at write time is hot.
//
// ContextRole 投影 block 如何进入 LLM 历史而不改写落库 Content：hot 全文、warm 截断预览、
// cold 省略带标记、archived 丢弃（内容并入 conversation.summary）。由压缩器（contextmgr
// M5.3）设置；落盘默认 hot。
const (
	ContextRoleHot      = "hot"
	ContextRoleWarm     = "warm"
	ContextRoleCold     = "cold"
	ContextRoleArchived = "archived"
)

// IsValidContextRole reports whether r is a known context role.
//
// IsValidContextRole 报告 r 是否已知 context role。
func IsValidContextRole(r string) bool {
	switch r {
	case ContextRoleHot, ContextRoleWarm, ContextRoleCold, ContextRoleArchived:
		return true
	}
	return false
}

// ToolCallData is the in-memory parsed form of one LLM tool call (never persisted as-is —
// it becomes a tool_call Block). Summary / Danger / ExecutionGroup are the framework
// standard fields the LLM self-declares on every call (stripped from Arguments by
// tool.StripStandardFields): Summary = one-line intent, Danger = self-reported risk
// (safe/cautious/dangerous, kept as a plain string so domain stays free of app/tool),
// ExecutionGroup = parallel-batch key.
//
// ToolCallData 是单个 LLM tool call 的内存解析形态（不原样落库——它转成 tool_call
// Block）。Summary / Danger / ExecutionGroup 是 LLM 每次调用自报的 framework 标准字段
// （由 tool.StripStandardFields 从 Arguments 剥离）：Summary 一句话意图、Danger 自报风险
// （safe/cautious/dangerous，存为纯字符串使 domain 不沾 app/tool）、ExecutionGroup 并行批键。
type ToolCallData struct {
	ID             string         `json:"id"`
	Name           string         `json:"name"`
	Summary        string         `json:"summary"`
	Danger         string         `json:"danger"`
	ExecutionGroup int            `json:"executionGroup"`
	Arguments      map[string]any `json:"arguments"`
}

// Message is one conversation turn — a user utterance or an assistant generation — that owns
// a Block tree. It lands in the `messages` table (message_blocks' sibling). The chat host
// persists it: CreateMessage opens the turn (and, for a user turn, writes its text Block);
// FinalizeMessage closes an assistant turn with terminal status + token accounting + blocks.
// loop never touches the table — it produces Blocks in memory and streams them; persistence
// is the host's job (the WriteFinalize seam, loop.Host).
//
// Message 是一个对话回合——用户发言或 assistant 生成——拥有一棵 Block 树。落 `messages` 表
// （message_blocks 的姊妹表）。由 chat host 持久化：CreateMessage 开回合（user 回合顺带写其
// text Block）；FinalizeMessage 以终态 + token 记账 + blocks 收一个 assistant 回合。loop 永不
// 碰表——它内存产 Block 并推流；落盘是 host 的事（WriteFinalize 缝，loop.Host）。
type Message struct {
	ID             string `db:"id,pk" json:"id"` // msg_<16hex>
	ConversationID string `db:"conversation_id" json:"conversationId"`
	WorkspaceID    string `db:"workspace_id,ws" json:"-"` // D2 物理隔离；orm 自动填/过滤
	// SubagentID marks a turn produced by a subagent run (recursive sub-conversation, M5.2+):
	// "" for a top-level turn; the subagent run id otherwise. chat's LoadHistory excludes
	// SubagentID != "" so a subagent's internal trace never pollutes the parent's LLM history
	// (the parent only sees the spawning tool_call + its tool_result). ListMessages still returns
	// them so a reload can rebuild the subagent subtree (the spawning tool_call id rides Attrs).
	//
	// SubagentID 标记由 subagent run 产出的回合（递归子对话，M5.2+）：顶层回合 ""；否则 subagent
	// run id。chat 的 LoadHistory 排除 SubagentID != ""，使 subagent 内部 trace 绝不污染父的 LLM
	// 历史（父只见派它的 tool_call + 其 tool_result）。ListMessages 仍返回它们，使 reload 能重建
	// subagent 子树（派它的 tool_call id 走 Attrs）。
	SubagentID   string         `db:"subagent_id" json:"subagentId,omitempty"`
	Role         string         `db:"role" json:"role"`     // RoleUser | RoleAssistant
	Status       string         `db:"status" json:"status"` // Status* (assistant 回合开始前为 pending)
	StopReason   string         `db:"stop_reason" json:"stopReason,omitempty"`
	ErrorCode    string         `db:"error_code" json:"errorCode,omitempty"`
	ErrorMessage string         `db:"error_message" json:"errorMessage,omitempty"`
	InputTokens  int            `db:"input_tokens" json:"inputTokens"`
	OutputTokens int            `db:"output_tokens" json:"outputTokens"`
	Provider     string         `db:"provider" json:"provider,omitempty"` // 溯源：产此回合的 provider
	ModelID      string         `db:"model_id" json:"modelId,omitempty"`  // 溯源：产此回合的模型
	Attrs        map[string]any `db:"attrs,json" json:"attrs,omitempty"`  // attachments / mentions 快照（freeze-on-send）
	CreatedAt    time.Time      `db:"created_at,created" json:"createdAt"`
	UpdatedAt    time.Time      `db:"updated_at,updated" json:"updatedAt"`

	// Blocks is the turn's content tree, hydrated by the store on read and supplied by the
	// caller on write — never a column (db:"-"). On a user turn it's the lone text block; on
	// an assistant turn it's what loop produced (text / reasoning / tool_call / tool_result).
	//
	// Blocks 是回合的内容树，读时由 store hydrate、写时由 caller 提供——非列（db:"-"）。user 回合
	// 是单个 text block；assistant 回合是 loop 产出（text / reasoning / tool_call / tool_result）。
	Blocks []Block `db:"-" json:"blocks,omitempty"`
}

// Roles a Message carries. There is no system/tool message row — the system prompt is built
// per-turn by chat (not persisted as a turn) and tool results are tool_result Blocks under an
// assistant turn, not standalone messages.
//
// Message 的角色。无 system/tool 消息行——system prompt 由 chat 每回合现拼（不作为回合落盘）、
// tool 结果是 assistant 回合下的 tool_result Block 而非独立消息。
const (
	RoleUser      = "user"
	RoleAssistant = "assistant"
)

// IsValidRole reports whether r is a known message role (store CHECK / 契约对账).
//
// IsValidRole 报告 r 是否已知消息角色（供 store CHECK / 契约对账）。
func IsValidRole(r string) bool {
	return r == RoleUser || r == RoleAssistant
}

// ErrMessageNotFound: GetMessage on an unknown message id.
//
// ErrMessageNotFound：对未知 message id 调 GetMessage。
var ErrMessageNotFound = errorsdomain.New(errorsdomain.KindNotFound, "MESSAGE_NOT_FOUND", "message not found")

// Repository persists conversation turns and their Block trees. Workspace isolation is
// automatic (orm fills/filters workspace_id from ctx via the ,ws tag), so no method takes a
// workspace id. Both tables are append-only (no deleted_at, D1: the content journal is never
// deleted). The two-phase write — CreateMessage (open) then FinalizeMessage (close) — mirrors
// the loop.Host contract: the host creates the message row before loop.Run to obtain the id
// for the live stream, then writes the terminal state and blocks once the turn ends.
//
// Repository 持久化对话回合及其 Block 树。workspace 隔离自动（orm 据 ctx 经 ,ws tag 填/过滤），
// 故方法不带 workspace id。两表皆 append-only（无 deleted_at，D1：内容日志永不删）。两段式写
// ——CreateMessage（开）再 FinalizeMessage（收）——对应 loop.Host 契约：host 在 loop.Run 前建
// message 行拿 id 供实时流，回合结束后写终态与 blocks。
type Repository interface {
	// CreateMessage inserts the turn row + (seq-assigned) blocks in one transaction. blocks may
	// be nil (an assistant turn opened before loop.Run produces none yet).
	//
	// CreateMessage 在一个事务内 insert 回合行 + （分配 seq 的）blocks。blocks 可为 nil
	// （loop.Run 前开的 assistant 回合尚无 block）。
	CreateMessage(ctx context.Context, m *Message, blocks []Block) error

	// FinalizeMessage updates an existing turn's terminal fields (status / stopReason / error /
	// tokens / provider / modelId) and appends its (seq-assigned) blocks, in one transaction.
	//
	// FinalizeMessage 在一个事务内更新已存在回合的终态字段（status / stopReason / error /
	// tokens / provider / modelId）并追加其（分配 seq 的）blocks。
	FinalizeMessage(ctx context.Context, m *Message, blocks []Block) error

	// GetMessage returns one turn with its Blocks hydrated; ErrMessageNotFound when absent.
	//
	// GetMessage 返回一个回合并 hydrate 其 Blocks；缺失时 ErrMessageNotFound。
	GetMessage(ctx context.Context, id string) (*Message, error)

	// ListMessages returns one keyset page of a conversation's turns, oldest-first, each with
	// Blocks hydrated (the REST history endpoint, N4 pagination).
	//
	// ListMessages 返回一个对话回合的一页 keyset（最旧在前），每条 hydrate Blocks（REST 历史端点，N4 分页）。
	ListMessages(ctx context.Context, conversationID, cursor string, limit int) (items []*Message, next string, err error)

	// LoadThread returns the whole conversation, oldest-first, every turn with Blocks hydrated
	// — the source chat's LoadHistory composes LLM history from (not paginated: a single local
	// user's thread fits in memory).
	//
	// LoadThread 返回整个对话（最旧在前），每个回合都 hydrate Blocks——chat 的 LoadHistory 据此
	// 组装 LLM 历史（不分页：单用户本地一条线程可装进内存）。
	LoadThread(ctx context.Context, conversationID string) ([]*Message, error)

	// SumTokens returns a conversation's total input + output tokens across all turns — the
	// usage endpoint's source. Zero for an empty / unknown conversation.
	//
	// SumTokens 返回一个对话所有回合的 input + output token 总和——usage 端点的来源。空 / 未知
	// 对话返 0。
	SumTokens(ctx context.Context, conversationID string) (inputTokens, outputTokens int, err error)

	// UpdateBlocksContextRole batch-sets the ContextRole of the given blocks (the compactor's
	// only write into message_blocks: a projection change, never a content rewrite). Empty ids is
	// a no-op. role must be a valid ContextRole (CHECK enforces it).
	//
	// UpdateBlocksContextRole 批量设置给定 block 的 ContextRole（压缩器对 message_blocks 的唯一写：
	// 投影变更、绝不改写 content）。空 ids 为 no-op。role 须是合法 ContextRole（CHECK 兜底）。
	UpdateBlocksContextRole(ctx context.Context, blockIDs []string, role string) error
}
