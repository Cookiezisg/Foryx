// Package workflow is the authoring-side domain for trinity workflows.
// A Workflow is a user-named DAG that triggers run when a trigger fires
// (execution lives in Plan 05 — scheduler / trigger / flowrun domains).
// This package owns the entity shape, version + pending lifecycle,
// 13 node-type enum, ops constants, and sentinel errors.
//
// See documents/version-1.2/adhoc-topic-documents/forge_redesign/04-workflow.md.
//
// Package workflow 是 trinity workflow 的 authoring 域。Workflow 是用户命名
// 的 DAG;触发执行在 Plan 05(scheduler / trigger / flowrun)。本包定义实体、
// 版本 + pending 生命周期、13 种节点 type 枚举、ops 常量、sentinel。
package workflow

import (
	"errors"
	"time"

	"gorm.io/gorm"
)

// Workflow is a user-named DAG entity. Active version is the frozen
// graph the scheduler executes; Pending is the LLM-driven edit awaiting
// user accept (iterate-same-pending semantic per D-redo-11). Soft-deleted
// rows hide from List + GET; partial UNIQUE (user_id, name) WHERE
// deleted_at IS NULL is enforced in schema_extras.
//
// Workflow 是用户命名的 DAG 实体。Active version 是 scheduler 执行的冻结图;
// Pending 是 LLM 改动等用户 accept(iterate-same-pending D-redo-11)。软删
// 行隐藏于 List + GET;partial UNIQUE 在 schema_extras 强制。
type Workflow struct {
	ID              string         `gorm:"primaryKey;type:text" json:"id"`
	UserID          string         `gorm:"not null;index:idx_workflows_user_id;type:text" json:"userId"`
	Name            string         `gorm:"not null;type:text" json:"name"`
	Description     string         `gorm:"type:text;default:''" json:"description"`
	Tags            []string       `gorm:"serializer:json;type:text;default:'[]'" json:"tags"`
	Enabled         bool           `gorm:"default:true" json:"enabled"`
	Concurrency     string         `gorm:"type:text;default:'serial'" json:"concurrency"`
	NeedsAttention  bool           `gorm:"default:false" json:"needsAttention"`
	AttentionReason string         `gorm:"type:text;default:''" json:"attentionReason,omitempty"`
	ActiveVersionID string         `gorm:"type:text;default:''" json:"activeVersionId"`
	CreatedAt       time.Time      `json:"createdAt"`
	UpdatedAt       time.Time      `json:"updatedAt"`
	DeletedAt       gorm.DeletedAt `gorm:"index" json:"-"`

	// Computed fields (gorm:"-") populated by service-layer attach hooks
	// after Repository read. LiveRuns / LastFiredAt / NextFireAt are
	// Plan 05 territory (scheduler / flowrun) — kept here so the GET
	// response shape is stable from Plan 04 onward.
	//
	// 计算字段:Pending 由 attachPending 填;LiveRuns / LastFiredAt / NextFireAt
	// 是 Plan 05 领域(scheduler / flowrun)— 此处先占位让 GET 响应稳定。
	Pending     *Version   `gorm:"-" json:"pending,omitempty"`
	LiveRuns    int        `gorm:"-" json:"liveRuns,omitempty"`
	LastFiredAt *time.Time `gorm:"-" json:"lastFiredAt,omitempty"`
	NextFireAt  *time.Time `gorm:"-" json:"nextFireAt,omitempty"`
}

// TableName pins the table name so GORM does not pluralize it elsewhere.
//
// TableName 显式指定表名(防 GORM 复数化漂移)。
func (Workflow) TableName() string { return "workflows" }

// Concurrency constants for Workflow.Concurrency. V1 default is "serial"
// — a workflow runs at most one FlowRun at a time. "parallel(N)" syntax
// is reserved for V1.5 (parser lives in scheduler domain, not enforced here).
//
// Concurrency 常量。V1 默认 serial;"parallel(N)" 留 V1.5。
const (
	ConcurrencySerial = "serial"
)

// AcceptedVersionCap caps accepted versions per workflow (oldest hard-
// deleted on each new accept, same pattern as function / handler).
//
// AcceptedVersionCap 每 workflow accepted 版本上限(超时硬删最旧)。
const AcceptedVersionCap = 50

// Sentinel errors. Wire codes registered in transport/httpapi/response/errmap.go.
// errors.Is must unwrap through fmt.Errorf("workflowstore.Method: %w", err)
// chains back to these sentinels (§S16 error wrap discipline).
//
// 哨兵错误。HTTP wire code 在 errmap.go 登记。errors.Is 必须能从最外层
// fmt.Errorf 包装 unwrap 到这里的 sentinel(§S16)。
//
// ErrPendingConflict is intentionally absent — Edit uses iterate-same-
// pending semantic (D-redo-11) so a pending coexists with subsequent
// edits via rewrite rather than conflict.
//
// 故意不含 ErrPendingConflict — Edit 走 iterate-same-pending(D-redo-11),
// pending 与后续 edit 共存靠重写,不返冲突。
var (
	ErrNotFound              = errors.New("workflow: not found")
	ErrDuplicateName         = errors.New("workflow: duplicate name")
	ErrVersionNotFound       = errors.New("workflow: version not found")
	ErrPendingNotFound       = errors.New("workflow: pending not found")
	ErrNoActiveVersion       = errors.New("workflow: no active version")
	ErrDAGCycle              = errors.New("workflow: DAG cycle detected")
	ErrInvalidReference      = errors.New("workflow: invalid reference")
	ErrNoTrigger             = errors.New("workflow: at least one trigger node required")
	ErrOpInvalid             = errors.New("workflow: op invalid")
	ErrCapabilityNotFound    = errors.New("workflow: capability not found")
	ErrMCPServerNotInstalled = errors.New("workflow: MCP server not installed")
)
