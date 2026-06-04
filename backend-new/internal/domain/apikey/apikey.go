// Package apikey is the domain layer for encrypted credential management. It
// owns a key's own lifecycle — store, encrypt, probe-test, hand out by id — and
// deliberately holds NO provider semantics: which key to use, and what models a
// key implies, are decided by other modules (model / search config). The probe
// test only answers "is this key live"; its raw response is archived verbatim
// for those modules to parse.
//
// Package apikey 是加密凭证管理的 domain 层。只管钥匙自身的生命周期——存、加密、探测连通、
// 按 id 发放——刻意不持任何 provider 语义：该用哪把、key 背后有哪些模型，由别的模块
// （model / 搜索配置）决定。探测测试只回答「这把钥匙活没活」，原始返回原样存档供那些模块解析。
package apikey

import (
	"context"
	"time"

	errorsdomain "github.com/sunweilin/forgify/backend/internal/domain/errors"
)

// APIKey is one encrypted credential for one provider. WorkspaceID isolates it
// (orm auto-fills on write, auto-filters on read). KeyEncrypted never leaves the
// process in plaintext. TestResponse stores the upstream probe's raw body
// verbatim — apikey never parses it; model/search modules do.
//
// APIKey 是某 provider 下的一把加密凭证。WorkspaceID 做隔离（orm 写时自动填、读时自动过滤）。
// KeyEncrypted 明文绝不出进程。TestResponse 原样存上游探测返回——apikey 从不解析，由
// model/搜索模块解析。
type APIKey struct {
	ID           string     `db:"id,pk" json:"id"`
	WorkspaceID  string     `db:"workspace_id,ws" json:"-"`
	Provider     string     `db:"provider" json:"provider"`
	DisplayName  string     `db:"display_name" json:"displayName"`
	KeyEncrypted string     `db:"key_encrypted" json:"-"`
	KeyMasked    string     `db:"key_masked" json:"keyMasked"`
	BaseURL      string     `db:"base_url" json:"baseUrl,omitempty"`
	APIFormat    string     `db:"api_format" json:"apiFormat,omitempty"`
	TestStatus   string     `db:"test_status" json:"testStatus"`
	TestError    string     `db:"test_error" json:"testError,omitempty"`
	TestResponse string     `db:"test_response" json:"-"` // raw probe body; parsed by model/search, never by apikey
	LastTestedAt *time.Time `db:"last_tested_at" json:"lastTestedAt,omitempty"`
	CreatedAt    time.Time  `db:"created_at,created" json:"createdAt"`
	UpdatedAt    time.Time  `db:"updated_at,updated" json:"updatedAt"`
	DeletedAt    *time.Time `db:"deleted_at,deleted" json:"-"`
}

// Probe test outcome states.
//
// 探测测试结果状态。
const (
	TestStatusPending = "pending"
	TestStatusOK      = "ok"
	TestStatusError   = "error"
)

// APIFormatAnthropicCompatible marks a custom key whose wire dialect is Anthropic's.
//
// APIFormatAnthropicCompatible 标记 wire 方言为 Anthropic 的 custom key。
const APIFormatAnthropicCompatible = "anthropic-compatible"

// Credentials is the per-call plaintext bundle handed to LLM/search callers.
// Key is plaintext — never log, never persist.
//
// Credentials 是发给调用方的明文凭证包；Key 为明文，禁日志 / 禁持久化。
type Credentials struct {
	Provider  string
	Key       string
	BaseURL   string
	APIFormat string
}

// ProbedKey is a read-only snapshot of one key's probe archive, handed to the
// model module to parse available models from. ID + DisplayName let the model
// module attribute each parsed model to the key that offers it. Carries no secret.
//
// ProbedKey 是一把 key 探测档案的只读快照，交给 model 模块解析可用模型。ID + DisplayName 让
// model 模块把每个解析出的模型归属到提供它的 key。不含密钥。
type ProbedKey struct {
	ID           string
	DisplayName  string
	Provider     string
	TestStatus   string
	TestResponse string
}

// ListFilter pages the key list, optionally narrowed to one provider.
//
// ListFilter 分页过滤 key 列表，可按 provider 收窄。
type ListFilter struct {
	Cursor   string
	Limit    int
	Provider string
}

// Domain sentinels — built via errorsdomain.New so transport reads Kind/Code
// directly (§S20); wire codes align with error-codes.md.
//
// domain sentinel——经 errorsdomain.New 构造，transport 直接读 Kind/Code（§S20）；
// wire code 对齐 error-codes.md。
var (
	ErrNotFound            = errorsdomain.New(errorsdomain.KindNotFound, "API_KEY_NOT_FOUND", "api key not found")
	ErrInvalidProvider     = errorsdomain.New(errorsdomain.KindInvalid, "API_KEY_INVALID_PROVIDER", "unknown provider")
	ErrKeyRequired         = errorsdomain.New(errorsdomain.KindInvalid, "API_KEY_VALUE_REQUIRED", "key value is required")
	ErrBaseURLRequired     = errorsdomain.New(errorsdomain.KindInvalid, "API_KEY_BASE_URL_REQUIRED", "base url is required for this provider")
	ErrAPIFormatRequired   = errorsdomain.New(errorsdomain.KindInvalid, "API_KEY_API_FORMAT_REQUIRED", "api format is required for custom provider")
	ErrDisplayNameConflict = errorsdomain.New(errorsdomain.KindConflict, "API_KEY_DISPLAY_NAME_CONFLICT", "display name already in use")
	ErrInUse               = errorsdomain.New(errorsdomain.KindUnprocessable, "API_KEY_IN_USE", "api key is referenced and cannot be deleted")
)

// Repository is the storage contract for APIKey, isolated by ctx workspace.
//
// Repository 是 APIKey 存储契约，按 ctx workspace 隔离。
type Repository interface {
	Get(ctx context.Context, id string) (*APIKey, error)
	List(ctx context.Context, filter ListFilter) ([]*APIKey, string, error)
	Save(ctx context.Context, k *APIKey) error
	Delete(ctx context.Context, id string) error

	// UpdateTestResult writes only the probe-outcome fields (status / error /
	// last_tested_at / raw response), avoiding a full-row round-trip.
	//
	// UpdateTestResult 仅写探测结果字段（状态 / 错误 / 时间 / 原始返回），避免整行往返。
	UpdateTestResult(ctx context.Context, id, status, errMsg, response string) error

	// ListProbed returns the probe archive of every key in the workspace
	// (provider + status + raw response), for the model module to parse.
	//
	// ListProbed 返回本 workspace 每把 key 的探测档案（provider + 状态 + 原始返回），供 model 解析。
	ListProbed(ctx context.Context) ([]ProbedKey, error)
}

// KeyProvider is the cross-module port for resolving ready-to-use credentials by
// key id, and flagging a key invalid by id. It is the ONLY way other modules
// touch credentials — always by explicit id (chosen upstream in model / search
// config), never by heuristic.
//
// KeyProvider 是跨模块端口：按 key id 解析可用凭证、按 id 标失效。是其他模块碰凭证的唯一途径
// ——永远按显式 id（由上游 model / 搜索配置选定），绝不启发式。
type KeyProvider interface {
	ResolveCredentialsByID(ctx context.Context, apiKeyID string) (Credentials, error)
	MarkInvalidByID(ctx context.Context, apiKeyID, reason string) error
}

// ProbeReader is the read-only port the model module uses to fetch probe
// archives and parse available models from them. apikey owns the raw data;
// model owns the interpretation.
//
// ProbeReader 是 model 模块用的只读端口：取探测档案并从中解析可用模型。apikey 持原始数据，
// model 持解读。
type ProbeReader interface {
	ListProbed(ctx context.Context) ([]ProbedKey, error)
}
