package workflow

import "context"

// Repository is the storage contract for Workflow + WorkflowVersion, scoped by ctx userID.
//
// Repository 是 Workflow + WorkflowVersion 的存储契约，按 ctx userID 过滤。
type Repository interface {
	SaveWorkflow(ctx context.Context, w *Workflow) error

	// GetWorkflow fetches by id; computed fields filled by service attach hooks.
	//
	// GetWorkflow 按 id 取；计算字段由 service attach 钩子填，不在 repo 层。
	GetWorkflow(ctx context.Context, id string) (*Workflow, error)

	GetWorkflowByName(ctx context.Context, name string) (*Workflow, error)
	GetWorkflowsByIDs(ctx context.Context, ids []string) ([]*Workflow, error)
	ListWorkflows(ctx context.Context, filter ListFilter) ([]*Workflow, string, error)
	ListAllWorkflows(ctx context.Context) ([]*Workflow, error)
	DeleteWorkflow(ctx context.Context, id string) error
	SetActiveVersion(ctx context.Context, workflowID, versionID string) error

	// SetNeedsAttention is called by the D20 capability-deletion listener and AcceptPending.
	//
	// SetNeedsAttention 由 D20 capability-deletion 监听器与 AcceptPending 调用。
	SetNeedsAttention(ctx context.Context, workflowID string, needs bool, reason string) error

	SaveVersion(ctx context.Context, v *Version) error
	GetVersion(ctx context.Context, versionID string) (*Version, error)
	GetVersionByNumber(ctx context.Context, workflowID string, versionN int) (*Version, error)
	ListVersions(ctx context.Context, workflowID string, filter VersionListFilter) ([]*Version, string, error)
	GetPending(ctx context.Context, workflowID string) (*Version, error)
	UpdateVersionStatus(ctx context.Context, versionID, status string, versionN *int) error
	HardDeleteVersion(ctx context.Context, versionID string) error
	HardDeleteOldestAccepted(ctx context.Context, workflowID string, keep int) error
}
