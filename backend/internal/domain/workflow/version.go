package workflow

import "time"

// Version is a frozen WorkflowVersion snapshot; on accept service assigns max+1.
//
// Version 是冻结的 WorkflowVersion 快照；accept 时 service 赋 version=max+1。
type Version struct {
	ID           string    `gorm:"primaryKey;type:text" json:"id"`
	WorkflowID   string    `gorm:"not null;index:idx_workflow_versions_workflow_id;type:text" json:"workflowId"`
	Status       string    `gorm:"not null;type:text;default:'pending'" json:"status"`
	Version      *int      `gorm:"index:idx_workflow_versions_version" json:"version,omitempty"`
	Graph        string    `gorm:"type:text;not null;default:'{}'" json:"-"`
	GraphParsed  *Graph    `gorm:"-" json:"graph,omitempty"`
	ChangeReason string    `gorm:"type:text;default:''" json:"changeReason,omitempty"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

func (Version) TableName() string { return "workflow_versions" }

const (
	StatusPending  = "pending"
	StatusAccepted = "accepted"
	StatusRejected = "rejected"
)

// Graph is the parsed-JSON shape of workflow_versions.graph; version is a complete frozen snapshot.
//
// Graph 是 workflow_versions.graph 的解析形态；Version 是完整冻结快照，revert 即重指 active。
type Graph struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Tags        []string       `json:"tags,omitempty"`
	Variables   []VariableSpec `json:"variables,omitempty"`
	Nodes       []NodeSpec     `json:"nodes"`
	Edges       []EdgeSpec     `json:"edges"`
}

type ListFilter struct {
	Cursor      string
	Limit       int
	EnabledOnly bool
}

type VersionListFilter struct {
	Cursor string
	Limit  int
	Status string
}
