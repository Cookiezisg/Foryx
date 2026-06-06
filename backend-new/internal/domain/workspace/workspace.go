// Package workspace is the domain layer for the local isolation root. A named
// workspace is the unit every other entity is scoped to (its workspace_id), and
// is itself the one table with no workspace_id column — it IS the workspace.
// Switching workspaces gives each its own agents, documents, api keys and runs,
// isolated in the database; application-level resources (mcp / skills / settings)
// stay shared on disk, so a workspace is a data boundary, not a filesystem bucket.
//
// Package workspace 是本地隔离根的 domain 层。一个具名 workspace 是其它所有实体的隔离单元
// （它们的 workspace_id），而它自己是唯一不带 workspace_id 列的表——它就是 workspace。
// 切换 workspace 让各自拥有独立的 agent/document/api key/run，在数据库层隔离；应用级资源
// （mcp/skills/settings）在磁盘共享——故 workspace 是数据边界，不是文件系统分桶。
package workspace

import (
	"context"
	"time"

	errorsdomain "github.com/sunweilin/forgify/backend/internal/domain/errors"
	modeldomain "github.com/sunweilin/forgify/backend/internal/domain/model"
)

// Workspace is one local isolation root. Name is a free-form display label,
// unique per machine; Language is a per-workspace UI preference (the first of a
// future set of workspace-scoped preferences). Unlike every other entity it
// carries no workspace_id.
//
// Workspace 是一个本地隔离根。Name 是自由展示名，全机唯一；Language 是 workspace 级 UI 偏好
// （未来一组 workspace 偏好的第一个）。与其它所有实体不同，它不带 workspace_id。
type Workspace struct {
	ID          string `db:"id,pk" json:"id"`
	Name        string `db:"name" json:"name"`
	AvatarColor string `db:"avatar_color" json:"avatarColor,omitempty"`
	Language    string `db:"language" json:"language"`
	// Per-scenario default model selections — workspace-scoped preferences alongside Language,
	// stored as JSON; nil = not configured for that scenario.
	// 按 scenario 的默认模型选择——与 Language 并列的 workspace 级偏好，JSON 存；nil = 该 scenario 未配置。
	DefaultDialogue *modeldomain.ModelRef `db:"default_dialogue,json" json:"defaultDialogue,omitempty"`
	DefaultUtility  *modeldomain.ModelRef `db:"default_utility,json" json:"defaultUtility,omitempty"`
	DefaultAgent    *modeldomain.ModelRef `db:"default_agent,json" json:"defaultAgent,omitempty"`
	// DefaultSearchKeyID is the api-key id chosen for WebSearch (provider implied by the
	// key). "" = unconfigured. A single explicit choice, not a priority list — the agent
	// never burns credits probing providers. Implements websearch.SearchKeyPicker via Service.
	// DefaultSearchKeyID 是为 WebSearch 选定的 api-key id（provider 由 key 隐含）。"" = 未配置。
	// 单一显式选择、非优先级列表——agent 永不试 provider 乱烧钱。经 Service 实现 websearch.SearchKeyPicker。
	DefaultSearchKeyID string     `db:"default_search_key_id" json:"defaultSearchKeyId,omitempty"`
	LastUsedAt         *time.Time `db:"last_used_at" json:"lastUsedAt,omitempty"`
	CreatedAt          time.Time  `db:"created_at,created" json:"createdAt"`
	UpdatedAt          time.Time  `db:"updated_at,updated" json:"updatedAt"`
	DeletedAt          *time.Time `db:"deleted_at,deleted" json:"-"`
}

// Supported UI languages; Language is CHECK-constrained to this set in the DDL.
//
// 支持的 UI 语言；Language 在 DDL 里被 CHECK 约束到此集合。
const (
	LanguageZhCN = "zh-CN"
	LanguageEn   = "en"
)

// MaxNameLen bounds a workspace name in runes — free-form display text, not a slug.
//
// MaxNameLen 按 rune 限制 workspace 名长度——自由展示文本，非 slug。
const MaxNameLen = 64

// IsValidLanguage reports whether l is a supported UI language.
//
// IsValidLanguage 报告 l 是否为支持的 UI 语言。
func IsValidLanguage(l string) bool {
	return l == LanguageZhCN || l == LanguageEn
}

// DefaultFor returns the default ModelRef for a scenario, or nil if unconfigured / unknown scenario.
//
// DefaultFor 返回某 scenario 的默认 ModelRef；未配置 / 未知 scenario 返 nil。
func (w *Workspace) DefaultFor(scenario string) *modeldomain.ModelRef {
	switch scenario {
	case modeldomain.ScenarioDialogue:
		return w.DefaultDialogue
	case modeldomain.ScenarioUtility:
		return w.DefaultUtility
	case modeldomain.ScenarioAgent:
		return w.DefaultAgent
	}
	return nil
}

// SetDefaultFor sets (or clears with nil) the default ModelRef for a scenario; an unknown scenario
// is a no-op (callers validate first).
//
// SetDefaultFor 设置（nil 则清除）某 scenario 的默认 ModelRef；未知 scenario 为 no-op（caller 先校验）。
func (w *Workspace) SetDefaultFor(scenario string, ref *modeldomain.ModelRef) {
	switch scenario {
	case modeldomain.ScenarioDialogue:
		w.DefaultDialogue = ref
	case modeldomain.ScenarioUtility:
		w.DefaultUtility = ref
	case modeldomain.ScenarioAgent:
		w.DefaultAgent = ref
	}
}

// Domain sentinels — built via errorsdomain.New so transport reads Kind/Code
// directly (§S20); wire codes align with error-codes.md.
//
// domain sentinel——经 errorsdomain.New 构造，使 transport 直接读 Kind/Code（§S20）；
// wire code 对齐 error-codes.md。
var (
	ErrNotFound         = errorsdomain.New(errorsdomain.KindNotFound, "WORKSPACE_NOT_FOUND", "workspace not found")
	ErrNameRequired     = errorsdomain.New(errorsdomain.KindInvalid, "WORKSPACE_NAME_REQUIRED", "workspace name is required")
	ErrNameTooLong      = errorsdomain.New(errorsdomain.KindInvalid, "WORKSPACE_NAME_TOO_LONG", "workspace name exceeds the length limit")
	ErrNameConflict     = errorsdomain.New(errorsdomain.KindConflict, "WORKSPACE_NAME_CONFLICT", "workspace name already exists")
	ErrCannotDeleteLast = errorsdomain.New(errorsdomain.KindUnprocessable, "CANNOT_DELETE_LAST_WORKSPACE", "cannot delete the last workspace")
	ErrLanguageInvalid  = errorsdomain.New(errorsdomain.KindInvalid, "WORKSPACE_LANGUAGE_INVALID", "language must be one of zh-CN, en")
)

// Repository is the storage contract for Workspace. Like the entity it is not
// workspace-scoped — these are the only queries that span all workspaces.
//
// Repository 是 Workspace 的存储契约。与实体一样不按 workspace 隔离——这是唯一跨所有 workspace 的查询。
type Repository interface {
	Save(ctx context.Context, w *Workspace) error
	Get(ctx context.Context, id string) (*Workspace, error)
	List(ctx context.Context) ([]*Workspace, error)
	Delete(ctx context.Context, id string) error
	Count(ctx context.Context) (int, error)
	TouchLastUsed(ctx context.Context, id string) error
}
