package flowrun

import (
	"errors"
	"time"

	"gorm.io/gorm"
)

const (
	NodeStatusPending   = "pending"
	NodeStatusRunning   = "running"
	NodeStatusOK        = "ok"
	NodeStatusFailed    = "failed"
	NodeStatusCancelled = "cancelled"
	NodeStatusTimeout   = "timeout"
	NodeStatusSkipped   = "skipped"
)

// Node is one node-dispatch record within a FlowRun.
//
// Node 是 FlowRun 内一次节点 dispatch 的记录。
type Node struct {
	ID             string         `gorm:"primaryKey;type:text" json:"id"`
	UserID         string         `gorm:"not null;index:idx_frn_user_id;type:text" json:"userId"`
	Status         string         `gorm:"not null;check:status IN ('pending','running','ok','failed','cancelled','timeout','skipped');type:text" json:"status"`
	TriggeredBy    string         `gorm:"not null;check:triggered_by IN ('chat','workflow','http','test');type:text" json:"triggeredBy"`
	Input          map[string]any `gorm:"serializer:json;type:text;default:'{}'" json:"input"`
	Output         any            `gorm:"serializer:json;type:text" json:"output,omitempty"`
	ErrorCode      string         `gorm:"type:text;default:''" json:"errorCode,omitempty"`
	ErrorMessage   string         `gorm:"type:text;default:''" json:"errorMessage,omitempty"`
	ElapsedMs      int64          `gorm:"not null;default:0" json:"elapsedMs"`
	StartedAt      time.Time      `gorm:"not null;index:idx_frn_started_at,sort:desc" json:"startedAt"`
	EndedAt        time.Time      `gorm:"not null" json:"endedAt"`
	ConversationID string         `gorm:"type:text;default:'';index:idx_frn_conv,priority:1" json:"conversationId,omitempty"`
	MessageID      string         `gorm:"type:text;default:'';index:idx_frn_conv,priority:2" json:"messageId,omitempty"`
	ToolCallID     string         `gorm:"type:text;default:''" json:"toolCallId,omitempty"`
	FlowrunID      string         `gorm:"not null;type:text;index:idx_frn_flowrun,priority:1" json:"flowrunId"`
	FlowrunNodeID  string         `gorm:"type:text;default:''" json:"flowrunNodeId,omitempty"`

	NodeID   string `gorm:"not null;type:text" json:"nodeId"`
	NodeType string `gorm:"not null;type:text" json:"nodeType"`
	Attempts int    `gorm:"not null;default:1" json:"attempts"`

	CreatedAt time.Time      `gorm:"index:idx_frn_flowrun,priority:2,sort:desc" json:"createdAt"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

func (Node) TableName() string { return "flowrun_nodes" }

type NodeFilter struct {
	FlowrunID      string
	NodeType       string
	Status         string
	ConversationID string
	Cursor         string
	Limit          int
}

var ErrNodeNotFound = errors.New("flowrun: node not found")
