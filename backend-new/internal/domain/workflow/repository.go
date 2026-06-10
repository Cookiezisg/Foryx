package workflow

import "context"

// VersionCap bounds how many graph versions one workflow retains; edits beyond this trim
// the oldest — but never the active version (it can be old after a revert).
//
// VersionCap 限制单 workflow 保留的图版本数；超出裁最老的——但绝不裁 active 版本（revert 后它
// 可能很老）。
const VersionCap = 50

// ListFilter is a cursor page request for workflows.
//
// ListFilter 是 workflow 的 cursor 分页请求。
type ListFilter struct {
	Cursor string
	Limit  int
}

// VersionListFilter is a cursor page request for one workflow's versions.
//
// VersionListFilter 是单 workflow 版本的 cursor 分页请求。
type VersionListFilter struct {
	Cursor string
	Limit  int
}

// MetaUpdate carries the lifecycle/concurrency/attention column writes the store applies in
// one update (a header-state change with no version bump). Nil pointers are left untouched.
//
// MetaUpdate 携带 store 一次更新写入的 lifecycle/concurrency/attention 列（不升版本的头部状态
// 变更）。nil 指针保持不动。
type MetaUpdate struct {
	Active          *bool
	LifecycleState  *string
	Concurrency     *string
	NeedsAttention  *bool
	AttentionReason *string
	LastActionBy    *string
}

// Repository is the storage contract for Workflow + Version. Workspace isolation is applied
// by the orm layer from ctx (the ,ws column tag), so methods take no workspace id.
//
// Repository 是 Workflow + Version 的存储契约。workspace 隔离由 orm 层据 ctx 施加（,ws 列 tag），
// 故方法不带 workspace id。
type Repository interface {
	// --- workflows ---

	SaveWorkflow(ctx context.Context, w *Workflow) error
	GetWorkflow(ctx context.Context, id string) (*Workflow, error)
	GetWorkflowByName(ctx context.Context, name string) (*Workflow, error)
	GetWorkflowsByIDs(ctx context.Context, ids []string) ([]*Workflow, error)
	ListWorkflows(ctx context.Context, filter ListFilter) ([]*Workflow, string, error)
	ListAllWorkflows(ctx context.Context) ([]*Workflow, error)

	// ListActiveWorkflows returns every live workflow with active=true (the scheduler's
	// candidate set; no pagination).
	//
	// ListActiveWorkflows 返回所有 active=true 的活跃 workflow（调度器候选集；不分页）。
	ListActiveWorkflows(ctx context.Context) ([]*Workflow, error)

	DeleteWorkflow(ctx context.Context, id string) error // soft-delete (tombstone)
	SetActiveVersion(ctx context.Context, workflowID, versionID string) error

	// UpdateWorkflowMeta writes the lifecycle/concurrency/attention columns in one update
	// (no version bump). A nil field in MetaUpdate is left untouched.
	//
	// UpdateWorkflowMeta 一次更新写 lifecycle/concurrency/attention 列（不升版本）。MetaUpdate
	// 中 nil 字段保持不动。
	UpdateWorkflowMeta(ctx context.Context, workflowID string, upd MetaUpdate) error

	// MarkInactiveIfDraining flips a draining workflow to inactive (+ active=false), conditionally on
	// it still being draining — the scheduler calls this when a draining workflow's last in-flight run
	// settles (graceful-drain complete). A no-op if the workflow already moved off draining (the user
	// re-activated it, or it was never draining). Idempotent; not an error if 0 rows match.
	// MarkInactiveIfDraining 把 draining 的 workflow 翻成 inactive（+ active=false），条件是它仍 draining
	// ——调度器在 draining workflow 最后一个在途 run 结算时调（优雅排空完成）。若 workflow 已离开 draining
	// （用户重新激活、或本就非 draining）则 no-op。幂等；0 行匹配不算错。
	MarkInactiveIfDraining(ctx context.Context, workflowID string) error

	// --- versions (immutable, cap-trimmed) ---

	SaveVersion(ctx context.Context, v *Version) error
	GetVersion(ctx context.Context, versionID string) (*Version, error)
	GetVersionByNumber(ctx context.Context, workflowID string, versionN int) (*Version, error)
	ListVersions(ctx context.Context, workflowID string, filter VersionListFilter) ([]*Version, string, error)

	// MaxVersionNumber returns the highest version number for a workflow (0 if none) — the
	// next write is MaxVersionNumber+1.
	//
	// MaxVersionNumber 返某 workflow 的最大版本号（无则 0）——下一次写入为 +1。
	MaxVersionNumber(ctx context.Context, workflowID string) (int, error)

	// TrimOldestVersions hard-deletes versions beyond the newest `keep`, never deleting the
	// workflow's current active version.
	//
	// TrimOldestVersions 硬删超出最新 keep 个的版本，绝不删 workflow 当前 active 版本。
	TrimOldestVersions(ctx context.Context, workflowID string, keep int) error
}
