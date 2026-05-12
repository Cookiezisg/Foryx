// version.go — Version entity + Graph JSON shape + Status enum.
//
// version.go —— Version 实体 + Graph JSON 形态 + Status 枚举。

package workflow

import "time"

// Version is a frozen WorkflowVersion snapshot — the immutable graph the
// scheduler executes after accept. Status is one of pending / accepted /
// rejected (CHECK constraint in schema_extras). version (int) is NULL
// while status != accepted; on accept the service assigns max+1.
//
// Version 是冻结的 WorkflowVersion 快照 — accept 后 scheduler 执行的不可变图。
// Status pending/accepted/rejected;pending/rejected 时 version 为 NULL,
// accept 时 service 赋 max+1。
type Version struct {
	ID           string    `gorm:"primaryKey;type:text" json:"id"`
	WorkflowID   string    `gorm:"not null;index:idx_workflow_versions_workflow_id;type:text" json:"workflowId"`
	Status       string    `gorm:"not null;type:text;default:'pending'" json:"status"`
	Version      *int      `gorm:"index:idx_workflow_versions_version" json:"version,omitempty"`
	Graph        string    `gorm:"type:text;not null;default:'{}'" json:"-"` // raw JSON; service serializes/deserializes
	GraphParsed  *Graph    `gorm:"-" json:"graph,omitempty"`                 // populated by service after read
	ChangeReason string    `gorm:"type:text;default:''" json:"changeReason,omitempty"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

// TableName pins the table name (GORM would pluralize to workflow_versions
// anyway — explicit for diff-grep clarity).
//
// TableName 显式指定表名。
func (Version) TableName() string { return "workflow_versions" }

// Status constants for Version.Status. Drift breaks DB CHECK + service
// state-machine — keep this single source of truth.
//
// Status 常量;漂移破 DB CHECK + service 状态机。
const (
	StatusPending  = "pending"
	StatusAccepted = "accepted"
	StatusRejected = "rejected"
)

// Graph is the parsed-JSON shape of a workflow_versions.graph row. The
// raw JSON blob lives in Version.Graph (string); GraphParsed is populated
// by the service layer's Marshal / Unmarshal hooks.
//
// Top-level fields:
//   - Name / Description / Tags — workflow metadata; redundant with Workflow
//     row but kept in graph so a version is a complete frozen snapshot
//     (revert is just "swap active to this graph").
//   - Variables — workflow-level variables referenced by node configs
//     via {{ vars.x }} expressions.
//   - Nodes / Edges — the DAG itself.
//
// Graph 是 workflow_versions.graph 的解析形态。raw JSON 存 Version.Graph;
// GraphParsed 由 service 层 marshal/unmarshal 钩子填。Version 是完整
// 冻结快照,revert 只需把 active 指向某 version。
type Graph struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Tags        []string       `json:"tags,omitempty"`
	Variables   []VariableSpec `json:"variables,omitempty"`
	Nodes       []NodeSpec     `json:"nodes"`
	Edges       []EdgeSpec     `json:"edges"`
}

// ListFilter is the query shape for Repository.ListWorkflows /
// ListVersions (Cursor + Limit). EnabledOnly defaults to false (List
// returns all live workflows regardless of enabled flag).
//
// ListFilter 是 List 查询形状。EnabledOnly 默认 false。
type ListFilter struct {
	Cursor      string
	Limit       int
	EnabledOnly bool
}

// VersionListFilter adds optional Status filter on top of ListFilter.
//
// VersionListFilter 加 Status 过滤。
type VersionListFilter struct {
	Cursor string
	Limit  int
	Status string // "" = any; otherwise pending / accepted / rejected
}
