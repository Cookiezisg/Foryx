// Package workflow is the authoring-side domain for trinity workflows (user-named DAGs).
//
// Package workflow 是 trinity workflow 的 authoring 域（用户命名的 DAG）。
package workflow

import (
	"errors"
	"time"

	"gorm.io/gorm"
)

// Workflow is a user-named DAG; ActiveVersion is the frozen graph the scheduler executes.
//
// Workflow 是用户命名的 DAG；ActiveVersion 是 scheduler 执行的冻结图。
type Workflow struct {
	ID          string   `gorm:"primaryKey;type:text" json:"id"`
	UserID      string   `gorm:"not null;index:idx_workflows_user_id;type:text" json:"userId"`
	Name        string   `gorm:"not null;type:text" json:"name"`
	Description string   `gorm:"type:text;default:''" json:"description"`
	Tags        []string `gorm:"serializer:json;type:text;default:'[]'" json:"tags"`

	// Enabled/Concurrency/NeedsAttention set explicitly by service; no GORM default to avoid masking false writes.
	// Enabled/Concurrency/NeedsAttention 由 service 显式赋值，避免 GORM default 静默覆盖 false 写入。
	Enabled         bool           `gorm:"not null" json:"enabled"`
	Concurrency     string         `gorm:"type:text;not null" json:"concurrency"`
	NeedsAttention  bool           `gorm:"not null" json:"needsAttention"`
	AttentionReason string         `gorm:"type:text;default:''" json:"attentionReason,omitempty"`
	ActiveVersionID string         `gorm:"type:text;default:''" json:"activeVersionId"`
	CreatedAt       time.Time      `json:"createdAt"`
	UpdatedAt       time.Time      `json:"updatedAt"`
	DeletedAt       gorm.DeletedAt `gorm:"index" json:"-"`

	// Computed fields (gorm:"-") filled by service attach hooks; placeholder for Plan 05 scheduler fields.
	// 计算字段（gorm:"-"），由 service attach 钩子填；Plan 05 scheduler 字段先占位让 GET 响应稳定。
	Pending     *Version   `gorm:"-" json:"pending,omitempty"`
	LiveRuns    int        `gorm:"-" json:"liveRuns,omitempty"`
	LastFiredAt *time.Time `gorm:"-" json:"lastFiredAt,omitempty"`
	NextFireAt  *time.Time `gorm:"-" json:"nextFireAt,omitempty"`
}

func (Workflow) TableName() string { return "workflows" }

const (
	ConcurrencySerial = "serial"
)

const AcceptedVersionCap = 50

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
