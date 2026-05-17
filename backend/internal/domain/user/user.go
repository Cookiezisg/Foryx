// Package user is the domain layer for local profile switching (single-machine multi-account).
//
// Package user 是本地多账号 profile 切换的 domain 层（单机多用户）。
package user

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
)

// User is one local profile; Username is unique per machine, displayName is the pretty label.
//
// User 是本机一个 profile；Username 全机唯一，DisplayName 是展示用昵称。
type User struct {
	ID          string         `gorm:"primaryKey;type:text" json:"id"`
	Username    string         `gorm:"not null;uniqueIndex:idx_users_username;type:text" json:"username"`
	DisplayName string         `gorm:"type:text;default:''" json:"displayName"`
	AvatarColor string         `gorm:"type:text;default:''" json:"avatarColor,omitempty"` // hex like #4f46e5
	Language    string         `gorm:"type:text;default:'zh-CN';check:language IN ('zh-CN','en')" json:"language"`
	LastUsedAt  *time.Time     `json:"lastUsedAt,omitempty"`
	CreatedAt   time.Time      `json:"createdAt"`
	UpdatedAt   time.Time      `json:"updatedAt"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`
}

const (
	LanguageZhCN = "zh-CN"
	LanguageEn   = "en"
)

// IsValidLanguage reports whether l is in the supported set (zh-CN / en).
//
// IsValidLanguage 报告 l 是否在支持集（zh-CN / en）。
func IsValidLanguage(l string) bool {
	return l == LanguageZhCN || l == LanguageEn
}

func (User) TableName() string { return "users" }

// DefaultUsername is the migration name assigned to the pre-existing "local-user".
//
// DefaultUsername 是给老的 "local-user" 迁移时赋的用户名。
const DefaultUsername = "default"

var (
	ErrNotFound          = errors.New("user: not found")
	ErrUsernameRequired  = errors.New("user: username required")
	ErrUsernameConflict  = errors.New("user: username already exists")
	ErrUsernameInvalid   = errors.New("user: username must be 1-32 chars, [a-z0-9_-]")
	ErrCannotDeleteLast  = errors.New("user: cannot delete the last user")
	ErrLanguageInvalid   = errors.New("user: language must be one of zh-CN, en")
)

// Repository is the storage contract for User; the only un-user-scoped entity (it IS the user).
//
// Repository 是 User 的存储契约；user 本身没法按 user_id scope，因为它就是 user。
type Repository interface {
	Save(ctx context.Context, u *User) error
	Get(ctx context.Context, id string) (*User, error)
	GetByUsername(ctx context.Context, username string) (*User, error)
	List(ctx context.Context) ([]*User, error)
	Delete(ctx context.Context, id string) error
	Count(ctx context.Context) (int, error)
	TouchLastUsed(ctx context.Context, id string) error
}
