// Package flowrun is the workflow execution-record domain.
//
// Package flowrun 是 workflow 执行记录 domain。
package flowrun

import (
	"errors"
	"time"

	"gorm.io/gorm"
)

const (
	StatusRunning   = "running"
	StatusPaused    = "paused"
	StatusCompleted = "completed"
	StatusFailed    = "failed"
	StatusCancelled = "cancelled"
)

const (
	TriggerKindCron     = "cron"
	TriggerKindFsnotify = "fsnotify"
	TriggerKindWebhook  = "webhook"
	TriggerKindManual   = "manual"
)

const DefaultRetentionLimit = 200

// PausedState is the persisted ExecutionContext snapshot for approval/wait nodes.
//
// PausedState 是 approval/wait 节点暂停时持久化的 ExecutionContext 快照。
type PausedState struct {
	NodeID    string                    `json:"nodeId"`
	Variables map[string]any            `json:"variables"`
	Outputs   map[string]map[string]any `json:"outputs"`
	Position  []string                  `json:"position"`
	PausedAt  time.Time                 `json:"pausedAt"`
}

// FlowRun is one execution record of a Workflow's pinned version.
//
// FlowRun 是 Workflow 一次执行记录，固定起跑时的 Version。
type FlowRun struct {
	ID           string         `gorm:"primaryKey;type:text" json:"id"`
	UserID       string         `gorm:"not null;index:idx_flowruns_user_id;type:text" json:"userId"`
	WorkflowID   string         `gorm:"not null;type:text;index:idx_flowruns_workflow,priority:1" json:"workflowId"`
	VersionID    string         `gorm:"not null;type:text" json:"versionId"`
	TriggerKind  string         `gorm:"not null;check:trigger_kind IN ('cron','fsnotify','webhook','manual');type:text" json:"triggerKind"`
	TriggerInput map[string]any `gorm:"serializer:json;type:text;default:'{}'" json:"triggerInput"`
	Status       string         `gorm:"not null;check:status IN ('running','paused','completed','failed','cancelled');index:idx_flowruns_workflow,priority:2;type:text" json:"status"`
	StartedAt    time.Time      `gorm:"not null;index:idx_flowruns_workflow,priority:3,sort:desc" json:"startedAt"`
	EndedAt      *time.Time     `json:"endedAt,omitempty"`
	ElapsedMs    int64          `gorm:"not null;default:0" json:"elapsedMs"`
	Output       any            `gorm:"serializer:json;type:text" json:"output,omitempty"`
	ErrorCode    string         `gorm:"type:text;default:''" json:"errorCode,omitempty"`
	ErrorMessage string         `gorm:"type:text;default:''" json:"errorMessage,omitempty"`
	PausedState  *PausedState   `gorm:"serializer:json;type:text" json:"pausedState,omitempty"`

	CreatedAt time.Time      `json:"createdAt"`
	UpdatedAt time.Time      `json:"updatedAt"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

func (FlowRun) TableName() string { return "flowruns" }

type ListFilter struct {
	WorkflowID  string
	Status      string
	TriggerKind string
	Cursor      string
	Limit       int
}

var (
	ErrNotFound                = errors.New("flowrun: not found")
	ErrNotCancellable          = errors.New("flowrun: not cancellable")
	ErrNotPaused               = errors.New("flowrun: not paused")
	ErrApprovalNodeNotFound    = errors.New("flowrun: approval node not found in paused state")
	ErrApprovalDecisionInvalid = errors.New("flowrun: approval decision invalid")
)
