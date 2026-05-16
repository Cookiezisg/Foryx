// Package memory is the domain layer for cross-conversation persistent facts.
//
// Package memory 是跨对话长期事实的 domain 层。
package memory

import (
	"context"
	"errors"
	"regexp"
	"time"

	"gorm.io/gorm"
)

// Memory is one persisted fact; Name is LLM-facing identifier (unique while live).
//
// Memory 是一条持久化事实；Name 是 LLM-facing 标识，未软删时唯一。
type Memory struct {
	ID          string         `gorm:"primaryKey;type:text" json:"id"`
	Name        string         `gorm:"not null;type:text" json:"name"`
	Type        string         `gorm:"not null;type:text;check:type IN ('user','feedback','project','reference')" json:"type"`
	Description string         `gorm:"not null;type:text" json:"description"`
	Content     string         `gorm:"not null;type:text;default:''" json:"content"`
	Pinned      bool           `gorm:"not null;default:false" json:"pinned"`
	Source      string         `gorm:"not null;type:text;check:source IN ('user','ai')" json:"source"`
	Metadata    map[string]any `gorm:"serializer:json;type:text;default:'{}'" json:"metadata,omitempty"`
	CreatedAt   time.Time      `json:"createdAt"`
	UpdatedAt   time.Time      `json:"updatedAt"`
	AccessedAt  *time.Time     `json:"accessedAt,omitempty"`
	AccessCount int            `gorm:"not null;default:0" json:"accessCount"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`
}

func (Memory) TableName() string { return "memories" }

const (
	TypeUser      = "user"
	TypeFeedback  = "feedback"
	TypeProject   = "project"
	TypeReference = "reference"
)

func IsValidType(t string) bool {
	switch t {
	case TypeUser, TypeFeedback, TypeProject, TypeReference:
		return true
	}
	return false
}

// ListTypes returns every recognised type; backs the contract test.
//
// ListTypes 返所有合法 type，支撑契约测试。
func ListTypes() []string {
	return []string{TypeUser, TypeFeedback, TypeProject, TypeReference}
}

const (
	SourceUser = "user"
	SourceAI   = "ai"
)

func IsValidSource(s string) bool {
	return s == SourceUser || s == SourceAI
}

// NameRegex constrains memory names: lowercase start + lowercase/digits/_ up to 64 chars.
//
// NameRegex 约束 memory name：小写字母开头 + 小写/数字/下划线，长度 ≤ 64。
var NameRegex = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)

var (
	ErrNotFound     = errors.New("memory: not found")
	ErrNameConflict = errors.New("memory: name already exists")
	ErrInvalidName  = errors.New("memory: invalid name format")
)

// UpsertInput is the write payload; Pinned pointer distinguishes keep-current from explicit false.
//
// UpsertInput 是写入载荷；Pinned 用指针，nil 保持原值，&false 显式取消 pin。
type UpsertInput struct {
	Name        string
	Type        string
	Description string
	Content     string
	Pinned      *bool
	Source      string
	Metadata    map[string]any
}

type ListFilter struct {
	Type   string
	Pinned *bool
}

// SystemPromptProvider is the narrow port chat runner consumes to fetch the memory section.
//
// SystemPromptProvider 是 chat runner 取 memory 块的窄接口（同 catalog.SystemPromptProvider 模式）。
type SystemPromptProvider interface {
	ForSystemPrompt(ctx context.Context) string
}

// Repository is the storage contract for Memory; global scope (no userID/conv filter axes).
//
// Repository 是 Memory 存储契约；全局作用域，无 userID/conv 过滤。
type Repository interface {
	Save(ctx context.Context, m *Memory) error
	GetByName(ctx context.Context, name string) (*Memory, error)
	GetByID(ctx context.Context, id string) (*Memory, error)
	List(ctx context.Context, filter ListFilter) ([]*Memory, error)
	ListPinned(ctx context.Context) ([]*Memory, error)

	// ListForIndex returns top-N non-pinned memories ranked by access freq for prompt index.
	//
	// ListForIndex 返按 access 频率排序前 N 条非 pinned，给 system prompt index 用。
	ListForIndex(ctx context.Context, limit int) ([]*Memory, error)

	MarkAccessed(ctx context.Context, name string) error
	Delete(ctx context.Context, name string) error
}
