// Package subagent is the domain layer for the Subagent system tool —
// Forgify's "spawn an isolated LLM loop" primitive (Phase 4 prep, ships
// in the V1.2 backend round). The LLM sees one tool, Subagent(prompt,
// type), which boots a sub-runner with its own context window, a curated
// tool list (filtered to the type's whitelist + the Subagent tool itself
// physically removed to prevent recursion), and bounded turns. The
// sub-runner returns its last assistant message as the parent LLM's
// tool_result.
//
// Two persistent entities:
//
//   - SubagentRun     — one row per spawn; status / token totals / model /
//     timestamps. Five gorm:"-" fields carry transient "what's the run
//     doing right now" state so chat.message snapshots can show a live
//     status strip without extra queries.
//   - SubagentMessage — one row per message inside a run; reuses
//     chatdomain.Block so the frontend renders subagent transcripts
//     with zero new code.
//
// One in-memory registry (SubagentType) lists the built-in subagents
// (Explore / Plan / general-purpose). Future evolution: file-loaded
// definitions analogous to Skill, but V1 ships with the registry baked in.
//
// Three ports for the rest of the system:
//
//   - Repository — store contract (implemented by infra/store/subagent)
//   - app/subagent.Service consumes Repository + a registry +
//     loop.Run (chat / subagent both build their own loop.Host
//     and call the same engine; no SubRunner port).
//
// Naming convention (S13): all three subagent packages — domain /
// app / store — declare `package subagent`. Callers alias by role:
//
//	subagentdomain "…/internal/domain/subagent"
//	subagentapp    "…/internal/app/subagent"
//	subagentstore  "…/internal/infra/store/subagent"
//
// Package subagent 是 Subagent system tool 的 domain 层——Forgify "起一个
// 隔离 LLM loop" 的原语（Phase 4 准备件，本轮 V1.2 后端落地）。LLM 看到
// 一个工具 Subagent(prompt, type)，启一个 sub-runner：独立 context window、
// 经类型白名单过滤的 tool 列表（Subagent 工具本身物理排除以防递归）、
// 有界 turns；sub-runner 跑完把 last assistant message 当 tool_result 返父 LLM。
//
// 两个持久化实体（SubagentRun / SubagentMessage）+ 一个内存注册表
// （SubagentType，V1 内置 Explore/Plan/general-purpose）+ Repository port。
// chat 与 subagent 都通过 internal/app/loop 共享 ReAct 引擎，不互相 import。
package subagent

import (
	"context"
	"errors"
	"time"

	chatdomain "github.com/sunweilin/forgify/backend/internal/domain/chat"
)

// ── Status enum ──────────────────────────────────────────────────────

// SubagentRun.Status values. Five-way enum: running is the only active
// state, the other four are terminal. completed/cancelled/max_turns are
// "non-error" terminations (sub-runner exited cleanly); failed marks
// genuine fault paths (panic, total timeout, internal error).
//
// SubagentRun.Status 五值枚举：running 是唯一活跃态，其他四个是终态。
// completed/cancelled/max_turns 是"非错误"终止；failed 标记真故障路径
// （panic、总超时、内部错）。
const (
	StatusRunning   = "running"
	StatusCompleted = "completed"
	StatusCancelled = "cancelled"
	StatusMaxTurns  = "max_turns"
	StatusFailed    = "failed"
)

// IsTerminal reports whether status is one of the four terminal states.
// Convenience for queries / UI gating.
//
// IsTerminal 报告 status 是否四种终态之一。查询/UI gating 便利函数。
func IsTerminal(status string) bool {
	switch status {
	case StatusCompleted, StatusCancelled, StatusMaxTurns, StatusFailed:
		return true
	}
	return false
}

// ── Roles ────────────────────────────────────────────────────────────

// SubagentMessage.Role values. Mirrors chat's role taxonomy because
// SubagentMessage.Blocks are the same chatdomain.Block type — UI rendering
// can be reused unchanged.
//
// SubagentMessage.Role 取值。与 chat 的 role 分类一致——SubagentMessage.Blocks
// 是同一份 chatdomain.Block，UI 可复用渲染。
const (
	RoleUser      = "user"
	RoleAssistant = "assistant"
	RoleTool      = "tool"
	RoleSystem    = "system"
)

// ── SubagentType (registry entry) ────────────────────────────────────

// SubagentType is one entry in the in-memory registry the LLM sees as
// the legal `subagent_type` argument values. AllowedTools is matched
// against Tool.Name() at spawn time; an empty slice means "inherit
// the parent registry minus the Subagent tool itself" (general-purpose
// uses this; explicit types use a whitelist).
//
// SubagentType 是内存注册表中的一项；LLM 把它当 `subagent_type` 合法值。
// AllowedTools 在 spawn 时按 Tool.Name() 匹配；空 slice = "继承父注册表
// 但去掉 Subagent 工具本身"（general-purpose 用这条；显式类型用白名单）。
type SubagentType struct {
	Name            string   `json:"name"`
	Description     string   `json:"description"`
	SystemPrompt    string   `json:"systemPrompt"`
	AllowedTools    []string `json:"allowedTools"`
	DefaultModel    string   `json:"defaultModel"`
	DefaultMaxTurns int      `json:"defaultMaxTurns"`
}

// ── SubagentRun ──────────────────────────────────────────────────────

// SubagentRun is the per-spawn ledger: parent linkage, type, status,
// token totals, model, timestamps. The five gorm:"-" trailing fields
// are transient "what is the run doing right now" state — they ride
// inside the chat.message snapshot the bridge publishes (see
// service-design-documents/subagent.md §10) so the UI can render a
// live status strip without separate queries. Gone after process
// restart, which is fine — runs that were running at restart time
// are reconciled to failed by app/subagent.Service.Bootstrap (or the
// next read whichever comes first), and live-status fields apply only
// to the running phase.
//
// SubagentRun 每次 spawn 一条总账：父引用、类型、status、token 累计、
// 模型、时间戳。末尾 5 个 gorm:"-" 字段是瞬时"当前在做什么"状态——随
// chat.message 快照外发让 UI 显示实时状态条（见 subagent.md §10），无需
// 额外查询。重启后丢失无所谓（Bootstrap 把 running 状态 reconcile 成
// failed；live-status 只在 running 阶段有意义）。
type SubagentRun struct {
	// ── persistent fields ───────────────────────────────────────────
	ID                   string     `gorm:"primaryKey;type:text" json:"id"`
	ParentConversationID string     `gorm:"not null;index;type:text" json:"parentConversationId"`
	ParentMessageID      string     `gorm:"type:text;index" json:"parentMessageId,omitempty"`
	ParentToolCallID     string     `gorm:"type:text" json:"parentToolCallId,omitempty"`
	Type                 string     `gorm:"not null;type:text" json:"type"`
	Prompt               string     `gorm:"type:text" json:"prompt"`
	Result               string     `gorm:"type:text" json:"result,omitempty"`
	Status               string     `gorm:"not null;type:text;default:running" json:"status"`
	TotalTokensIn        int        `json:"totalTokensIn"`
	TotalTokensOut       int        `json:"totalTokensOut"`
	StepsUsed            int        `json:"stepsUsed"`
	Model                string     `gorm:"type:text" json:"model,omitempty"`
	StartedAt            time.Time  `json:"startedAt"`
	EndedAt              *time.Time `json:"endedAt,omitempty"`
	ErrorMsg             string     `gorm:"type:text" json:"errorMsg,omitempty"`
	CreatedAt            time.Time  `json:"createdAt"`
	UpdatedAt            time.Time  `json:"updatedAt"`

	// ── streaming UI transient (in-memory only; not persisted) ──────
	LastToolCalled      string     `gorm:"-" json:"lastToolCalled,omitempty"`
	LastToolArgsBrief   string     `gorm:"-" json:"lastToolArgsBrief,omitempty"`
	LastToolResultBrief string     `gorm:"-" json:"lastToolResultBrief,omitempty"`
	LastStepDurationMs  int        `gorm:"-" json:"lastStepDurationMs,omitempty"`
	LastStepAt          *time.Time `gorm:"-" json:"lastStepAt,omitempty"`
}

// TableName locks the table to "subagent_runs".
//
// TableName 把表名钉死在 "subagent_runs"。
func (SubagentRun) TableName() string { return "subagent_runs" }

// ── SubagentMessage ──────────────────────────────────────────────────

// SubagentMessage is one message inside a run. Seq is assigned by the
// store on AppendMessage (SELECT COALESCE(MAX(seq),-1)+1) so callers
// don't race; UpdateMessage rewrites Blocks while preserving seq for
// streaming refinement of the same row.
//
// Blocks reuses chatdomain.Block (text / reasoning / tool_call /
// tool_result / attachment_ref) so the frontend renders subagent
// transcripts with the same component tree as main-chat messages.
//
// SubagentMessage 是 run 内的一条消息。Seq 由 store 在 AppendMessage 内
// 分配（SELECT COALESCE(MAX(seq),-1)+1），调用方无需竞争；UpdateMessage
// 重写 Blocks 但保留 seq，用于同条消息的流式精化。
//
// Blocks 复用 chatdomain.Block——前端用同一套组件渲染 subagent transcript。
type SubagentMessage struct {
	ID               string             `gorm:"primaryKey;type:text" json:"id"`
	SubagentRunID    string             `gorm:"not null;index:idx_smm_run_seq,priority:1;type:text" json:"subagentRunId"`
	Seq              int                `gorm:"not null;index:idx_smm_run_seq,priority:2" json:"seq"`
	Role             string             `gorm:"not null;type:text" json:"role"`
	Blocks           []chatdomain.Block `gorm:"serializer:json" json:"blocks"`
	PromptTokens     int                `json:"promptTokens,omitempty"`
	CompletionTokens int                `json:"completionTokens,omitempty"`
	CreatedAt        time.Time          `json:"createdAt"`
}

// TableName locks the table to "subagent_messages".
//
// TableName 把表名钉死在 "subagent_messages"。
func (SubagentMessage) TableName() string { return "subagent_messages" }

// ── Sentinels ────────────────────────────────────────────────────────

// Sentinels surfaced by app/subagent.Service. Only the first two reach
// HTTP handlers — ErrMaxTurnsExceeded / ErrCancelled are converted to
// friendly tool_result strings inside SubagentTool.Execute and never
// propagate (the run's terminal status reflects them).
//
// Sentinels 由 app/subagent.Service 抛出。只有前两个会到 handler——
// ErrMaxTurnsExceeded / ErrCancelled 在 SubagentTool.Execute 内转友好
// tool_result 字符串，不上抛（run 的终态 status 已反映）。
var (
	ErrTypeNotFound     = errors.New("subagent: type not found")
	ErrRecursionAttempt = errors.New("subagent: nested spawn not allowed")
	ErrMaxTurnsExceeded = errors.New("subagent: max turns exceeded")
	ErrCancelled        = errors.New("subagent: cancelled")
)

// ── Repository port ──────────────────────────────────────────────────

// Repository is the storage port. Implemented by infra/store/subagent.
// CreateRun / UpdateRun cover the run ledger; AppendMessage assigns Seq
// transactionally so concurrent appends within one run can't collide.
// UpdateMessage exists separately because the sub-runner streams blocks
// into the same final assistant message (status="streaming" → "completed").
//
// Repository 是存储 port，由 infra/store/subagent 实现。CreateRun /
// UpdateRun 管 run 总账；AppendMessage 在事务内分配 Seq，单 run 内并发
// append 不会撞号。UpdateMessage 独立——sub-runner 把 blocks 流式累加到同
// 一条 assistant 消息（status streaming → completed）。
type Repository interface {
	// SubagentRun
	CreateRun(ctx context.Context, r *SubagentRun) error
	GetRun(ctx context.Context, id string) (*SubagentRun, error)
	UpdateRun(ctx context.Context, r *SubagentRun) error
	ListRunsByConversation(ctx context.Context, conversationID string) ([]*SubagentRun, error)

	// SubagentMessage
	AppendMessage(ctx context.Context, m *SubagentMessage) error
	UpdateMessage(ctx context.Context, m *SubagentMessage) error
	ListMessagesByRun(ctx context.Context, runID string) ([]*SubagentMessage, error)
}
