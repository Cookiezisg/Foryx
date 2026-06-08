package approval

import "context"

// VersionCap bounds how many versions one approval form retains; edits beyond this trim
// the oldest — but never the active version (it can be old after a revert).
//
// VersionCap 限制单审批表保留的版本数；超出裁最老的——但绝不裁 active 版本（revert 后它可能很老）。
const VersionCap = 50

// ListFilter is a cursor page request for approval forms.
//
// ListFilter 是审批表的 cursor 分页请求。
type ListFilter struct {
	Cursor string
	Limit  int
}

// VersionListFilter is a cursor page request for one approval form's versions.
//
// VersionListFilter 是单审批表版本的 cursor 分页请求。
type VersionListFilter struct {
	Cursor string
	Limit  int
}

// Repository is the storage contract for ApprovalForm + Version. Workspace isolation is
// applied by the orm layer from ctx (the ,ws column tag), so methods take no workspace id.
//
// Repository 是 ApprovalForm + Version 的存储契约。workspace 隔离由 orm 层据 ctx 施加（,ws 列
// tag），故方法不带 workspace id。
type Repository interface {
	SaveForm(ctx context.Context, f *ApprovalForm) error
	GetForm(ctx context.Context, id string) (*ApprovalForm, error)
	GetFormsByIDs(ctx context.Context, ids []string) ([]*ApprovalForm, error)
	ListForms(ctx context.Context, filter ListFilter) ([]*ApprovalForm, string, error)
	ListAllForms(ctx context.Context) ([]*ApprovalForm, error)
	DeleteForm(ctx context.Context, id string) error // soft-delete (tombstone)
	SetActiveVersion(ctx context.Context, formID, versionID string) error

	SaveVersion(ctx context.Context, v *Version) error
	GetVersion(ctx context.Context, versionID string) (*Version, error)
	GetVersionByNumber(ctx context.Context, formID string, versionN int) (*Version, error)
	ListVersions(ctx context.Context, formID string, filter VersionListFilter) ([]*Version, string, error)

	// MaxVersionNumber returns the highest version number for a form (0 if none) — the
	// next write is MaxVersionNumber+1.
	//
	// MaxVersionNumber 返某审批表的最大版本号（无则 0）——下一次写入为 +1。
	MaxVersionNumber(ctx context.Context, formID string) (int, error)

	// TrimOldestVersions hard-deletes versions beyond the newest `keep`, never deleting the
	// form's current active version.
	//
	// TrimOldestVersions 硬删超出最新 keep 个的版本，绝不删审批表当前 active 版本。
	TrimOldestVersions(ctx context.Context, formID string, keep int) error
}
