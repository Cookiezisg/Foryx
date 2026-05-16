package handler

import (
	"context"
	"time"
)

type ListFilter struct {
	Cursor string
	Limit  int
}

type VersionListFilter struct {
	Cursor string
	Limit  int
	Status string
}

// Repository is the storage contract for Handler + Version + ConfigEncrypted, scoped by ctx userID.
//
// Repository 是 Handler + Version + ConfigEncrypted 的存储契约，按 ctx userID 过滤。
type Repository interface {
	SaveHandler(ctx context.Context, h *Handler) error
	GetHandler(ctx context.Context, id string) (*Handler, error)
	GetHandlerByName(ctx context.Context, name string) (*Handler, error)
	GetHandlersByIDs(ctx context.Context, ids []string) ([]*Handler, error)
	ListHandlers(ctx context.Context, filter ListFilter) ([]*Handler, string, error)
	ListAllHandlers(ctx context.Context) ([]*Handler, error)
	DeleteHandler(ctx context.Context, id string) error
	SetActiveVersion(ctx context.Context, handlerID, versionID string) error

	SaveVersion(ctx context.Context, v *Version) error
	GetVersion(ctx context.Context, versionID string) (*Version, error)
	GetVersionByNumber(ctx context.Context, handlerID string, versionN int) (*Version, error)
	ListVersions(ctx context.Context, handlerID string, filter VersionListFilter) ([]*Version, string, error)
	GetPending(ctx context.Context, handlerID string) (*Version, error)
	UpdateVersionStatus(ctx context.Context, versionID, status string, versionN *int) error
	UpdateVersionEnv(ctx context.Context, versionID, envStatus, envError, envSyncStage, envSyncDetail string, syncedAt *time.Time) error
	HardDeleteOldestAccepted(ctx context.Context, handlerID string, keep int) error
	HardDeleteVersion(ctx context.Context, versionID string) error

	// UpdateConfigEncrypted writes the AES-GCM blob; ciphertext is opaque to repo.
	//
	// UpdateConfigEncrypted 写 AES-GCM 密文 blob；密文对 repo 不透明。
	UpdateConfigEncrypted(ctx context.Context, handlerID, ciphertext string) error

	ClearConfig(ctx context.Context, handlerID string) error
	GetConfigEncrypted(ctx context.Context, handlerID string) (string, error)

	CallRepository
}
