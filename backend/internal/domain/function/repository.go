package function

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

// Repository is the storage contract for Function + FunctionVersion, scoped by ctx userID.
//
// Repository 是 Function + FunctionVersion 的存储契约，按 ctx userID 过滤。
type Repository interface {
	SaveFunction(ctx context.Context, f *Function) error

	// GetFunction fetches by id, scoped to user; computed fields filled by service attach hooks.
	//
	// GetFunction 按 id 查；计算字段由 service attach 钩子填，不在 repo 层。
	GetFunction(ctx context.Context, id string) (*Function, error)

	GetFunctionByName(ctx context.Context, name string) (*Function, error)
	GetFunctionsByIDs(ctx context.Context, ids []string) ([]*Function, error)
	ListFunctions(ctx context.Context, filter ListFilter) ([]*Function, string, error)
	ListAllFunctions(ctx context.Context) ([]*Function, error)
	DeleteFunction(ctx context.Context, id string) error
	SetActiveVersion(ctx context.Context, functionID, versionID string) error

	SaveVersion(ctx context.Context, v *Version) error
	GetVersion(ctx context.Context, versionID string) (*Version, error)
	GetVersionByNumber(ctx context.Context, functionID string, versionN int) (*Version, error)
	ListVersions(ctx context.Context, functionID string, filter VersionListFilter) ([]*Version, string, error)
	GetPending(ctx context.Context, functionID string) (*Version, error)

	// UpdateVersionStatus transitions pending → accepted/rejected; versionN non-nil only on accept.
	//
	// UpdateVersionStatus 状态转 pending → accepted/rejected；转 accepted 时 versionN 非 nil。
	UpdateVersionStatus(ctx context.Context, versionID, status string, versionN *int) error

	UpdateVersionEnv(ctx context.Context, versionID, envStatus, envError, envSyncStage, envSyncDetail string, syncedAt *time.Time) error
	HardDeleteOldestAccepted(ctx context.Context, functionID string, keep int) error
	HardDeleteVersion(ctx context.Context, versionID string) error

	ExecutionRepository
}

const AcceptedVersionCap = 50
