// repository.go — Repository port for Workflow + WorkflowVersion. Mirrors
// the function / handler repository shapes (Plan 01/02 patterns) so
// Service code feels uniform across trinity.
//
// repository.go —— Workflow + WorkflowVersion 的 Repository port;形态跟
// function / handler 同(Plan 01/02 模式),Service 代码跨 trinity 统一。

package workflow

import "context"

// Repository is the storage contract for Workflow + WorkflowVersion.
// All methods scoped to ctx userID (caller must run InjectUserID middleware
// first; repo enforces ctx-scoped reads — cross-user reads return
// ErrNotFound).
//
// Implementation: infra/store/workflow.Store.
// Consumer: app/workflow.Service only (no cross-domain direct repo access).
//
// Repository 是 Workflow + WorkflowVersion 的存储契约。所有方法按 ctx userID
// 过滤(InjectUserID 中间件先注入)。
type Repository interface {
	// ── Workflow CRUD ─────────────────────────────────────────────────────

	// SaveWorkflow inserts or updates by primary key. On duplicate (user_id,
	// name) returns ErrDuplicateName.
	SaveWorkflow(ctx context.Context, w *Workflow) error

	// GetWorkflow fetches by id, scoped to current user. ErrNotFound if
	// absent / soft-deleted. Computed fields NOT populated; service layer's
	// attach hooks fill them.
	GetWorkflow(ctx context.Context, id string) (*Workflow, error)

	// GetWorkflowByName fetches by name, scoped to current user — for
	// create-time duplicate name check.
	GetWorkflowByName(ctx context.Context, name string) (*Workflow, error)

	// GetWorkflowsByIDs fetches multiple Workflows by ID slice, preserving
	// input order. Used by search after LLM returns ranked IDs.
	GetWorkflowsByIDs(ctx context.Context, ids []string) ([]*Workflow, error)

	// ListWorkflows returns a cursor-paginated page of live workflows for
	// current user. EnabledOnly=true filters out disabled.
	ListWorkflows(ctx context.Context, filter ListFilter) ([]*Workflow, string, error)

	// ListAllWorkflows returns all live workflows for current user
	// without pagination — used by SearchWorkflow (LLM ranking) and
	// future scheduler (ListEnabled).
	ListAllWorkflows(ctx context.Context) ([]*Workflow, error)

	// DeleteWorkflow soft-deletes a workflow by id, scoped to current user.
	DeleteWorkflow(ctx context.Context, id string) error

	// SetActiveVersion atomically updates Workflow.ActiveVersionID. Used by
	// accept-pending / revert.
	SetActiveVersion(ctx context.Context, workflowID, versionID string) error

	// SetNeedsAttention atomically updates NeedsAttention + AttentionReason.
	// Called by the D20 capability-deletion listener (Plan 05) and by
	// Service.AcceptPending (clears the flag after a successful edit).
	SetNeedsAttention(ctx context.Context, workflowID string, needs bool, reason string) error

	// ── Versions (including pending) ──────────────────────────────────────

	// SaveVersion inserts (or upserts on primary key) a WorkflowVersion.
	SaveVersion(ctx context.Context, v *Version) error

	// GetVersion fetches by version id. ErrVersionNotFound if absent.
	GetVersion(ctx context.Context, versionID string) (*Version, error)

	// GetVersionByNumber fetches an accepted version by workflow id +
	// integer. Used by revert flow.
	GetVersionByNumber(ctx context.Context, workflowID string, versionN int) (*Version, error)

	// ListVersions returns cursor-paginated versions for a workflow, newest
	// first. Filter.Status filters by 'pending' / 'accepted' / 'rejected'.
	ListVersions(ctx context.Context, workflowID string, filter VersionListFilter) ([]*Version, string, error)

	// GetPending returns the active pending version (at most one); returns
	// ErrPendingNotFound if none.
	GetPending(ctx context.Context, workflowID string) (*Version, error)

	// UpdateVersionStatus transitions a version's status (pending → accepted
	// / rejected). versionN must be non-nil when transitioning to accepted
	// (new sequential integer); nil otherwise.
	UpdateVersionStatus(ctx context.Context, versionID, status string, versionN *int) error

	// HardDeleteVersion physically deletes one Version row by ID
	// (workflow_versions has no soft-delete column). Called by
	// Service.RejectPending after destroying the pending — same pattern
	// as function / handler (D-redo-12).
	HardDeleteVersion(ctx context.Context, versionID string) error

	// HardDeleteOldestAccepted enforces the per-workflow accepted-version
	// cap (AcceptedVersionCap = 50) — service layer calls after each new
	// accept.
	HardDeleteOldestAccepted(ctx context.Context, workflowID string, keep int) error
}
