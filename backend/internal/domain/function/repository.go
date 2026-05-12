package function

import (
	"context"
	"time"
)

// ListFilter is the query shape for Repository.ListFunctions /
// Repository.ListVersions. Cursor is the opaque pagination token (the
// concrete shape lives in pkg/pagination.Cursor;repo layer encodes /
// decodes it as opaque base64).
//
// ListFilter 是 Repository List 接受的查询形状。Cursor 是不透明分页 token
// (具体 shape 在 pkg/pagination.Cursor)。
type ListFilter struct {
	Cursor string // "" = first page
	Limit  int    // 0 = repo default(50);max 200
}

// VersionListFilter is the query shape for ListVersions — adds optional
// status filter on top of generic ListFilter.
//
// VersionListFilter 是 ListVersions 接受的查询形状,加 status 过滤。
type VersionListFilter struct {
	Cursor string
	Limit  int
	Status string // "" = any;否则按 'pending'/'accepted'/'rejected' 过滤
}

// Repository is the storage contract for Function + FunctionVersion.
// All methods scoped to ctx userID — caller must run InjectUserID middleware
// first;repo enforces ctx-scoped reads (cross-user reads return ErrNotFound).
//
// Implementation: infra/store/function.Store
// Consumer: app/function.Service only (no cross-domain direct repo access)
//
// Repository 是 Function + FunctionVersion 的存储契约。所有方法按 ctx userID
// 过滤;调用方先跑 InjectUserID 中间件。
type Repository interface {
	// ── Function CRUD ─────────────────────────────────────────────────────

	// SaveFunction inserts or updates a Function by primary key.
	// On duplicate (user_id, name) returns ErrDuplicateName.
	//
	// SaveFunction 按主键插入或更新;name 重复返 ErrDuplicateName。
	SaveFunction(ctx context.Context, f *Function) error

	// GetFunction fetches by id, scoped to current user. Returns ErrNotFound
	// if absent or soft-deleted. Pending / Env* computed fields NOT populated;
	// service layer's attachPending / attachActiveEnv fills them.
	//
	// GetFunction 按 id 查,按当前用户过滤;未命中或已软删返 ErrNotFound。
	// 计算字段不填(service 层 attachPending / attachActiveEnv 填)。
	GetFunction(ctx context.Context, id string) (*Function, error)

	// GetFunctionByName fetches by name, scoped to current user — for
	// create-time duplicate name check. Returns ErrNotFound if absent.
	//
	// GetFunctionByName 按 name 查,按当前用户过滤;create 时查重名用。
	GetFunctionByName(ctx context.Context, name string) (*Function, error)

	// GetFunctionsByIDs fetches multiple Functions by ID slice, preserving
	// input order. Used by search after LLM returns ranked IDs.
	//
	// GetFunctionsByIDs 按 id 切片批量查,保持输入顺序。
	GetFunctionsByIDs(ctx context.Context, ids []string) ([]*Function, error)

	// ListFunctions returns a cursor-paginated page of live functions for
	// current user. Returns (rows, nextCursor, err).
	//
	// ListFunctions 返当前用户活跃 function 的 cursor 分页结果。
	ListFunctions(ctx context.Context, filter ListFilter) ([]*Function, string, error)

	// ListAllFunctions returns all live functions for current user without
	// pagination — used by SearchFunction (LLM ranking on full list) and
	// CatalogSource.ListItems.
	//
	// ListAllFunctions 返当前用户全部活跃 function(无分页);
	// SearchFunction 跟 CatalogSource 用。
	ListAllFunctions(ctx context.Context) ([]*Function, error)

	// DeleteFunction soft-deletes a function by id, scoped to current user.
	// Cascade: triggers `function_deleted` notification (publisher in service
	// layer reads this returning row to publish);引用此 function 的 workflow
	// 由 listener 标 needs_attention(D20)。
	//
	// DeleteFunction 软删,按当前用户过滤。级联 D20 在 service 层。
	DeleteFunction(ctx context.Context, id string) error

	// SetActiveVersion atomically updates Function.ActiveVersionID. Used on
	// accept-pending / revert flows.
	//
	// SetActiveVersion 原子更新 ActiveVersionID(accept / revert 时用)。
	SetActiveVersion(ctx context.Context, functionID, versionID string) error

	// ── Versions (including pending) ──────────────────────────────────────

	// SaveVersion inserts a FunctionVersion (pending / accepted / rejected).
	//
	// SaveVersion 插入 FunctionVersion。
	SaveVersion(ctx context.Context, v *Version) error

	// GetVersion fetches by version id (fnv_<16hex>). Returns ErrVersionNotFound
	// if absent.
	//
	// GetVersion 按 version id 查。
	GetVersion(ctx context.Context, versionID string) (*Version, error)

	// GetVersionByNumber fetches an accepted version by function id + version
	// integer. Used by revert flow.
	//
	// GetVersionByNumber 按 function + version 整数查 accepted 版本。
	GetVersionByNumber(ctx context.Context, functionID string, versionN int) (*Version, error)

	// ListVersions returns cursor-paginated versions for a function, newest
	// first. Filter.Status filters by 'pending' / 'accepted' / 'rejected'.
	//
	// ListVersions 返某 function 的版本 cursor 分页(新→旧),可按 status 过滤。
	ListVersions(ctx context.Context, functionID string, filter VersionListFilter) ([]*Version, string, error)

	// GetPending returns the active pending version for a function — there
	// should be at most one;returns ErrPendingNotFound if none.
	//
	// GetPending 返某 function 的活动 pending 版本(至多一个);无则
	// ErrPendingNotFound。
	GetPending(ctx context.Context, functionID string) (*Version, error)

	// UpdateVersionStatus transitions a version's status (pending → accepted
	// / rejected). When transitioning to accepted, versionN must be non-nil
	// (the new sequential integer);for pending / rejected pass nil.
	//
	// UpdateVersionStatus 状态机转换(pending → accepted / rejected)。
	// 转 accepted 时 versionN 非 nil(新递增整数);其他传 nil。
	UpdateVersionStatus(ctx context.Context, versionID, status string, versionN *int) error

	// UpdateVersionEnv writes all env_* fields atomically (status / error /
	// stage / detail / syncedAt). Called by sandbox sync progress + final.
	//
	// UpdateVersionEnv 原子写 env_* 字段(status / error / stage / detail /
	// syncedAt)。sandbox sync 进度 + 终态写。
	UpdateVersionEnv(ctx context.Context, versionID, envStatus, envError, envSyncStage, envSyncDetail string, syncedAt *time.Time) error

	// HardDeleteOldestAccepted enforces the per-function accepted-version
	// cap (50 by default) — service layer calls this after each new accept.
	//
	// HardDeleteOldestAccepted 强制单 function accepted 版本数上限(默认 50)。
	HardDeleteOldestAccepted(ctx context.Context, functionID string, keep int) error

	// HardDeleteVersion physically deletes one Version row by ID (no soft-
	// delete on function_versions). Called by Service.RejectPending after
	// destroying the venv — pending row is gone from DB.
	//
	// HardDeleteVersion 按 ID 物理删一行 Version(function_versions 无软删)。
	// Service.RejectPending 在销毁 venv 后调,从 DB 抹掉 pending 行。
	HardDeleteVersion(ctx context.Context, versionID string) error

	// Execution-log methods (D22) — embedded so Service.repo has both core
	// CRUD and execution history methods without a second repo field.
	//
	// Execution-log 方法(D22)——embed 让 Service.repo 同时拿 CRUD + history,
	// 不开第二字段。
	ExecutionRepository
}

// AcceptedVersionCap is the max number of accepted versions kept per
// function. After this many, oldest accepted is hard-deleted on each new
// accept (called from service layer).
//
// AcceptedVersionCap 是每 function 保留的 accepted 版本上限。
const AcceptedVersionCap = 50
