// Package conversation is the domain layer for chat thread management.
//
// Package conversation 是对话线程 domain 层（消息归 chat domain）。
package conversation

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
)

// Conversation is a chat thread container; Summary is the anchored-append summary from app/contextmgr.
//
// Conversation 是对话线程容器；Summary 是 app/contextmgr 维护的 anchored-append 摘要。
type Conversation struct {
	ID                   string         `gorm:"primaryKey;type:text" json:"id"`
	UserID               string         `gorm:"not null;index;type:text" json:"-"`
	Title                string         `gorm:"not null;type:text;default:''" json:"title"`
	AutoTitled           bool           `gorm:"not null;default:false" json:"autoTitled"`
	SystemPrompt         string         `gorm:"type:text;default:''" json:"systemPrompt,omitempty"`
	Summary              string         `gorm:"type:text;default:''" json:"summary,omitempty"`
	SummaryCoversUpToSeq int64          `gorm:"not null;default:0" json:"summaryCoversUpToSeq,omitempty"`
	CreatedAt            time.Time      `json:"createdAt"`
	UpdatedAt            time.Time      `json:"updatedAt"`
	DeletedAt            gorm.DeletedAt `gorm:"index" json:"-"`
}

func (Conversation) TableName() string { return "conversations" }

type ListFilter struct {
	Cursor string
	Limit  int
}

var ErrNotFound = errors.New("conversation: not found")

// Repository is the storage contract for Conversation, scoped by ctx user.
//
// Repository 是 Conversation 的存储契约，按 ctx 用户过滤。
type Repository interface {
	Save(ctx context.Context, c *Conversation) error
	Get(ctx context.Context, id string) (*Conversation, error)
	List(ctx context.Context, filter ListFilter) ([]*Conversation, string, error)
	Delete(ctx context.Context, id string) error
}
