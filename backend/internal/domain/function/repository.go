package function

import (
	"context"
	"time"
)

// VersionCap bounds how many versions one function retains; edits beyond this trim the
// oldest — but never the active version (it can be old after a revert).
//
// VersionCap 限制单 function 保留的版本数；超出裁最老的——但绝不裁 active 版本（revert 后它
// 可能很老）。
const VersionCap = 50

// ListFilter is a cursor page request for functions.
//
// ListFilter 是 function 的 cursor 分页请求。
type ListFilter struct {
	Cursor string
	Limit  int
}

// VersionListFilter is a cursor page request for one function's versions.
//
// VersionListFilter 是单 function 版本的 cursor 分页请求。
type VersionListFilter struct {
	Cursor string
	Limit  int
}

// Repository is the storage contract for Function + Version + Execution. Workspace
// isolation is applied by the orm layer from ctx (the ,ws column tag), so methods take
// no workspace id.
//
// Repository 是 Function + Version + Execution 的存储契约。workspace 隔离由 orm 层据 ctx
// 施加（,ws 列 tag），故方法不带 workspace id。
type Repository interface {
	// --- functions ---

	SaveFunction(ctx context.Context, f *Function) error
	GetFunction(ctx context.Context, id string) (*Function, error)
	GetFunctionByName(ctx context.Context, name string) (*Function, error)
	GetFunctionsByIDs(ctx context.Context, ids []string) ([]*Function, error)
	ListFunctions(ctx context.Context, filter ListFilter) ([]*Function, string, error)
	ListAllFunctions(ctx context.Context) ([]*Function, error)
	DeleteFunction(ctx context.Context, id string) error // soft-delete (tombstone)
	SetActiveVersion(ctx context.Context, functionID, versionID string) error
	CreateWithVersion(ctx context.Context, e *Function, v *Version) error      // create + v1, one tx
	SaveVersionAndActivate(ctx context.Context, v *Version, f *Function) error // new version + pointer move + row meta, one tx

	// --- versions ---

	SaveVersion(ctx context.Context, v *Version) error
	GetVersion(ctx context.Context, versionID string) (*Version, error)
	GetVersionByNumber(ctx context.Context, functionID string, versionN int) (*Version, error)
	ListVersions(ctx context.Context, functionID string, filter VersionListFilter) ([]*Version, string, error)

	// MaxVersionNumber returns the highest version number for a function (0 if none) —
	// the next write is MaxVersionNumber+1.
	//
	// MaxVersionNumber 返某 function 的最大版本号（无则 0）——下一次写入为 +1。
	MaxVersionNumber(ctx context.Context, functionID string) (int, error)

	// UpdateVersionEnv writes a version's env terminal state plus the (possibly
	// env-fix-corrected) dependency list, in one update.
	//
	// UpdateVersionEnv 一次写入版本的 env 终态 + （可能被 env-fix 修正过的）依赖列表。
	UpdateVersionEnv(ctx context.Context, versionID, envStatus, envError string, deps []string, syncedAt *time.Time) error

	// TrimOldestVersions hard-deletes versions beyond the newest `keep`, never deleting
	// the function's current active version.
	//
	// TrimOldestVersions 硬删超出最新 keep 个的版本，绝不删 function 当前 active 版本。
	TrimOldestVersions(ctx context.Context, functionID string, keep int) error

	ExecutionRepository
}
