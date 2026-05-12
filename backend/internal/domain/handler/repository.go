// repository.go — Handler + HandlerVersion + ConfigEncrypted storage contract.
//
// Mirrors functiondomain.Repository shape for Handler / Version CRUD, plus
// 3 config methods for the per-Definition AES-GCM encrypted init args blob
// (D-handler config). Phase 7.5 of Plan 02 will add CallRepository embed
// for D22 handler_calls execution log.
//
// repository.go —— Handler + HandlerVersion + ConfigEncrypted 存储契约。
// Handler / Version CRUD 跟 functiondomain.Repository 同形;额外 3 个 config
// 方法管 per-Definition AES-GCM 加密的 init args blob。Phase 7.5 会 embed
// CallRepository 接 D22 handler_calls 执行日志。

package handler

import (
	"context"
	"time"
)

// ListFilter is the query shape for Repository.ListHandlers.
//
// ListFilter 是 ListHandlers 接受的查询形状。
type ListFilter struct {
	Cursor string
	Limit  int
}

// VersionListFilter adds optional status filter on top of ListFilter.
//
// VersionListFilter 在 ListFilter 上加 status 过滤。
type VersionListFilter struct {
	Cursor string
	Limit  int
	Status string
}

// Repository is the storage contract for Handler + Version + ConfigEncrypted.
// All methods scope by ctx userID;cross-user reads return ErrNotFound.
//
// Repository 是 Handler + Version + ConfigEncrypted 的存储契约。按 ctx userID
// 过滤;跨用户读返 ErrNotFound。
type Repository interface {
	// ── Handler CRUD ──────────────────────────────────────────────────────

	SaveHandler(ctx context.Context, h *Handler) error
	GetHandler(ctx context.Context, id string) (*Handler, error)
	GetHandlerByName(ctx context.Context, name string) (*Handler, error)
	GetHandlersByIDs(ctx context.Context, ids []string) ([]*Handler, error)
	ListHandlers(ctx context.Context, filter ListFilter) ([]*Handler, string, error)
	ListAllHandlers(ctx context.Context) ([]*Handler, error)
	DeleteHandler(ctx context.Context, id string) error
	SetActiveVersion(ctx context.Context, handlerID, versionID string) error

	// ── Version CRUD ──────────────────────────────────────────────────────

	SaveVersion(ctx context.Context, v *Version) error
	GetVersion(ctx context.Context, versionID string) (*Version, error)
	GetVersionByNumber(ctx context.Context, handlerID string, versionN int) (*Version, error)
	ListVersions(ctx context.Context, handlerID string, filter VersionListFilter) ([]*Version, string, error)
	GetPending(ctx context.Context, handlerID string) (*Version, error)
	UpdateVersionStatus(ctx context.Context, versionID, status string, versionN *int) error
	UpdateVersionEnv(ctx context.Context, versionID, envStatus, envError, envSyncStage, envSyncDetail string, syncedAt *time.Time) error
	HardDeleteOldestAccepted(ctx context.Context, handlerID string, keep int) error

	// HardDeleteVersion physically deletes one Version row by ID. Called by
	// Service.RejectPending after destroying the venv (per D-redo-12).
	//
	// HardDeleteVersion 按 ID 物理删一行 Version。Service.RejectPending 销 venv
	// 后调用。
	HardDeleteVersion(ctx context.Context, versionID string) error

	// ── Config (D-handler — AES-GCM ciphertext blob) ──────────────────────

	// UpdateConfigEncrypted writes the AES-GCM ciphertext blob covering all
	// init_args values for one (user, handler) pair. Ciphertext is opaque to
	// the repo; encryption / decryption is the Service's responsibility
	// (uses infra/crypto via the encryptor port).
	//
	// UpdateConfigEncrypted 写 (user, handler) 对应的 AES-GCM 密文 blob。
	// 密文对 repo 不透明;加解密在 Service 层(经 infra/crypto)。
	UpdateConfigEncrypted(ctx context.Context, handlerID, ciphertext string) error

	// ClearConfig wipes ConfigEncrypted to "" (reverts to unconfigured state).
	//
	// ClearConfig 清 ConfigEncrypted 到 ""(回到未配置态)。
	ClearConfig(ctx context.Context, handlerID string) error

	// GetConfigEncrypted returns the raw ciphertext (or "" if unconfigured).
	//
	// GetConfigEncrypted 返原始密文("" 表示未配置)。
	GetConfigEncrypted(ctx context.Context, handlerID string) (string, error)

	// Call-log methods (D22) — embedded so Service.repo gets all execution-log
	// methods alongside core CRUD.
	//
	// Call-log 方法(D22)——embed 让 Service.repo 一并拿 execution-log。
	CallRepository
}
