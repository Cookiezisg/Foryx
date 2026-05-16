// Package todo is the domain layer for the LLM-facing per-conversation to-do tracker.
//
// Package todo 是 LLM 对话级 TODO 追踪 domain。
package todo

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
)

// Todo is one entry on a conversation's todo list; not portable across conversations.
//
// Todo 是对话 TODO 列表上一条，归属创建对话，不可跨对话。
type Todo struct {
	ID             string         `gorm:"primaryKey;type:text" json:"id"`
	ConversationID string         `gorm:"not null;index:idx_td_conv_status,priority:1;type:text" json:"conversationId"`
	Subject        string         `gorm:"not null;type:text" json:"subject"`
	Description    string         `gorm:"type:text" json:"description,omitempty"`
	ActiveForm     string         `gorm:"type:text" json:"activeForm,omitempty"`
	Status         string         `gorm:"not null;type:text;index:idx_td_conv_status,priority:2;default:pending" json:"status"`
	Owner          string         `gorm:"type:text" json:"owner,omitempty"`
	BlockedBy      []string       `gorm:"serializer:json" json:"blockedBy,omitempty"`
	Metadata       map[string]any `gorm:"serializer:json" json:"metadata,omitempty"`
	CreatedAt      time.Time      `json:"createdAt"`
	UpdatedAt      time.Time      `json:"updatedAt"`
	DeletedAt      gorm.DeletedAt `gorm:"index" json:"-"`
}

const (
	StatusPending    = "pending"
	StatusInProgress = "in_progress"
	StatusCompleted  = "completed"
	StatusDeleted    = "deleted"
)

func IsValidStatus(s string) bool {
	switch s {
	case StatusPending, StatusInProgress, StatusCompleted, StatusDeleted:
		return true
	default:
		return false
	}
}

// ListStatuses returns every recognised status; backs the contract test (not used by production).
//
// ListStatuses 返所有合法状态，支撑契约测试，生产不调。
func ListStatuses() []string {
	return []string{StatusPending, StatusInProgress, StatusCompleted, StatusDeleted}
}

var (
	ErrNotFound        = errors.New("todo: not found")
	ErrSubjectRequired = errors.New("todo: subject is required")
	ErrInvalidStatus   = errors.New("todo: invalid status")
)

// Repository is the storage contract for Todo, filtered by row's ConversationID.
//
// Repository 是 Todo 存储契约，按行 ConversationID 过滤。
type Repository interface {
	Create(ctx context.Context, t *Todo) error
	Get(ctx context.Context, id string) (*Todo, error)
	ListByConversation(ctx context.Context, conversationID string) ([]*Todo, error)
	Update(ctx context.Context, t *Todo) error
	SoftDelete(ctx context.Context, id string) error
}
