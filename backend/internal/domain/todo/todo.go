// Package todo is the domain layer for the LLM-facing per-conversation
// to-do tracker. v1 consumer: app/tool/todo via app/todo.Service.
//
// Package todo 是 LLM 对话级 TODO 追踪 domain。v1 消费者：通过
// app/todo.Service 的 app/tool/todo。
package todo

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
)

// Todo is one entry on a conversation's todo list; owned by the creating
// conversation, not portable across conversations.
//
// Todo 是对话 TODO 列表上一条；归属创建对话，不可跨对话。
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

// Status lifecycle: pending → in_progress → completed (terminal).
// Todos may be deleted at any point. App-layer validation, not DB CHECK,
// so adding new statuses needs no schema migration.
//
// Status 生命周期：pending → in_progress → completed（终态）。任何时点可
// 标 deleted。校验在 app 层（非 DB CHECK），新增状态不需 schema 迁移。
const (
	StatusPending    = "pending"
	StatusInProgress = "in_progress"
	StatusCompleted  = "completed"
	StatusDeleted    = "deleted"
)

// IsValidStatus reports whether s is a recognised status.
// IsValidStatus 报告 s 是否合法状态。
func IsValidStatus(s string) bool {
	switch s {
	case StatusPending, StatusInProgress, StatusCompleted, StatusDeleted:
		return true
	default:
		return false
	}
}

// ListStatuses returns every recognised status. Backs the contract test
// asserting ListStatuses ≡ IsValidStatus; production code does not call it.
//
// ListStatuses 返所有合法状态。支撑 ListStatuses ≡ IsValidStatus 契约测试；生产不调。
func ListStatuses() []string {
	return []string{StatusPending, StatusInProgress, StatusCompleted, StatusDeleted}
}

var (
	ErrNotFound        = errors.New("todo: not found")
	ErrSubjectRequired = errors.New("todo: subject is required")
	ErrInvalidStatus   = errors.New("todo: invalid status")
	// ErrConversationMismatch: caller tried to mutate a todo from a different
	// conversation than ctx — defensive reject to prevent scope leak.
	// ErrConversationMismatch：调用方改了归属另一对话的 todo——防御性拒绝防作用域泄漏。
	ErrConversationMismatch = errors.New("todo: conversation mismatch")
)

// Repository is the storage contract for Todo. Filters by row's
// ConversationID; callers don't mutate ConversationID after Create.
//
// Repository 是 Todo 存储契约。按行 ConversationID 过滤；调用方 Create 后
// 不改 ConversationID。
type Repository interface {
	// Create inserts a new todo; caller fills ID / ConversationID / Subject / valid Status.
	// Create 插入新 todo；调用方先填 ID / ConversationID / Subject / 合法 Status。
	Create(ctx context.Context, t *Todo) error

	// Get fetches by ID; ErrNotFound when absent or soft-deleted.
	// Get 按 ID 取；不存在或软删返 ErrNotFound。
	Get(ctx context.Context, id string) (*Todo, error)

	// ListByConversation returns active todos for one conversation, created_at ASC.
	// ListByConversation 返某对话活跃 todo，created_at 升序。
	ListByConversation(ctx context.Context, conversationID string) ([]*Todo, error)

	// Update writes back; caller follows load → mutate → pass same pointer.
	// Update 写回；调用方按 load → 修改 → 传同一指针。
	Update(ctx context.Context, t *Todo) error

	// SoftDelete sets deleted_at; row kept for audit, hidden from List/Get.
	// SoftDelete 置 deleted_at；行保留可审计，不再被 List/Get 见。
	SoftDelete(ctx context.Context, id string) error
}
