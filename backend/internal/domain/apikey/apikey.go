// Package apikey is the domain layer for credential management.
//
// Package apikey 是凭证管理的 domain 层。
package apikey

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
)

// APIKey is a user credential for one LLM provider.
//
// APIKey 是用户在某 provider 下的凭证。
type APIKey struct {
	ID           string         `gorm:"primaryKey;type:text" json:"id"`
	UserID       string         `gorm:"not null;index:idx_api_keys_user_id;index:idx_api_keys_user_provider,priority:1;type:text" json:"userId"`
	Provider     string         `gorm:"not null;index:idx_api_keys_user_provider,priority:2;type:text" json:"provider"`
	DisplayName  string         `gorm:"not null;type:text;default:''" json:"displayName"`
	KeyEncrypted string         `gorm:"not null;type:text" json:"-"`
	KeyMasked    string         `gorm:"not null;type:text" json:"keyMasked"`
	BaseURL      string         `gorm:"type:text;default:''" json:"baseUrl"`
	APIFormat    string         `gorm:"type:text;default:''" json:"apiFormat"`
	TestStatus   string         `gorm:"type:text;default:'pending'" json:"testStatus"`
	TestError    string         `gorm:"type:text;default:''" json:"testError"`
	LastTestedAt *time.Time     `json:"lastTestedAt"`
	ModelsFound  []string       `gorm:"serializer:json;type:text;default:'[]'" json:"modelsFound"`
	CreatedAt    time.Time      `json:"createdAt"`
	UpdatedAt    time.Time      `json:"updatedAt"`
	DeletedAt    gorm.DeletedAt `gorm:"index" json:"-"`
}

func (APIKey) TableName() string { return "api_keys" }

const (
	TestStatusPending = "pending"
	TestStatusOK      = "ok"
	TestStatusError   = "error"
)

const (
	APIFormatAnthropicCompatible = "anthropic-compatible"
)

// Credentials is the per-call bundle returned to LLM consumers; Key is plaintext.
//
// Credentials 是返给 LLM 调用方的凭证包；Key 为明文，禁日志 / 禁持久化。
type Credentials struct {
	Key     string
	BaseURL string
}

type ListFilter struct {
	Cursor   string
	Limit    int
	Provider string
}

var (
	ErrNotFound            = errors.New("apikey: not found")
	ErrNotFoundForProvider = errors.New("apikey: no key for provider")
	ErrInvalidProvider     = errors.New("apikey: invalid provider")
	ErrBaseURLRequired     = errors.New("apikey: base_url required for this provider")
	ErrAPIFormatRequired   = errors.New("apikey: api_format required for custom provider")
	ErrKeyRequired         = errors.New("apikey: key value is required")
	ErrDisplayNameConflict = errors.New("apikey: display name already in use")
)

// Repository is the storage contract for APIKey, scoped by ctx userID.
//
// Repository 是 APIKey 的存储契约，按 ctx userID 过滤。
type Repository interface {
	Get(ctx context.Context, id string) (*APIKey, error)
	List(ctx context.Context, filter ListFilter) ([]*APIKey, string, error)

	// GetByProvider picks best active key: test_status='ok' > last_tested_at DESC > created_at DESC.
	//
	// GetByProvider 挑最佳活跃 Key，无则返 ErrNotFoundForProvider。
	GetByProvider(ctx context.Context, provider string) (*APIKey, error)

	Save(ctx context.Context, k *APIKey) error
	Delete(ctx context.Context, id string) error
	UpdateTestResult(ctx context.Context, id, status, errMsg string, models []string) error
}

// KeyProvider is the cross-domain port for resolving ready-to-use credentials.
//
// KeyProvider 是跨 domain 端口，消费方拿可用凭证而不接触 Repository。
type KeyProvider interface {
	ResolveCredentials(ctx context.Context, provider string) (Credentials, error)
	MarkInvalid(ctx context.Context, provider string, reason string) error
	HasKeyForProvider(ctx context.Context, provider string) (bool, error)
}

// SearchProviderPriority is the order WebSearch tries BYOK keys; must match app/apikey/providers.go.
//
// SearchProviderPriority 是 WebSearch 多 key 尝试顺序，须与 app/apikey/providers.go 同步。
var SearchProviderPriority = []string{"brave", "serper", "tavily", "bocha"}
