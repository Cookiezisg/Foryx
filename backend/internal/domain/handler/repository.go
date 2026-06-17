package handler

import (
	"context"
	"time"
)

// VersionCap bounds how many versions one handler retains; edits beyond this trim the
// oldest — never the active version.
//
// VersionCap 限制单 handler 保留的版本数；超出裁最老的——绝不裁 active 版本。
const VersionCap = 50

// ListFilter is a cursor page request for handlers.
type ListFilter struct {
	Cursor string
	Limit  int
}

// VersionListFilter is a cursor page request for one handler's versions.
type VersionListFilter struct {
	Cursor string
	Limit  int
}

// Repository is the storage contract for Handler + Version + Call. Workspace isolation
// is applied by the orm layer from ctx (the ,ws column tag).
//
// Repository 是 Handler + Version + Call 的存储契约。workspace 隔离由 orm 层据 ctx 施加。
type Repository interface {
	// --- handlers ---

	SaveHandler(ctx context.Context, h *Handler) error
	GetHandler(ctx context.Context, id string) (*Handler, error)
	GetHandlerByName(ctx context.Context, name string) (*Handler, error)
	GetHandlersByIDs(ctx context.Context, ids []string) ([]*Handler, error)
	ListHandlers(ctx context.Context, filter ListFilter) ([]*Handler, string, error)
	ListAllHandlers(ctx context.Context) ([]*Handler, error)
	DeleteHandler(ctx context.Context, id string) error // soft-delete
	SetActiveVersion(ctx context.Context, handlerID, versionID string) error
	CreateWithVersion(ctx context.Context, e *Handler, v *Version) error      // create + v1, one tx
	SaveVersionAndActivate(ctx context.Context, v *Version, h *Handler) error // new version + pointer move + row meta, one tx

	// --- encrypted config (init-args values) ---

	GetConfigEncrypted(ctx context.Context, handlerID string) (string, error)
	UpdateConfigEncrypted(ctx context.Context, handlerID, ciphertext string) error
	ClearConfig(ctx context.Context, handlerID string) error

	// --- versions ---

	SaveVersion(ctx context.Context, v *Version) error
	GetVersion(ctx context.Context, versionID string) (*Version, error)
	GetVersionByNumber(ctx context.Context, handlerID string, versionN int) (*Version, error)
	ListVersions(ctx context.Context, handlerID string, filter VersionListFilter) ([]*Version, string, error)
	MaxVersionNumber(ctx context.Context, handlerID string) (int, error)
	UpdateVersionEnv(ctx context.Context, versionID, envStatus, envError string, deps []string, syncedAt *time.Time) error
	TrimOldestVersions(ctx context.Context, handlerID string, keep int) error

	CallRepository
}
