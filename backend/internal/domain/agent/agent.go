// Package agent is the domain layer for Agent entities — the 4th Quadrinity element: a
// configured LLM worker. An Agent writes NO code itself; it mounts capabilities BY REFERENCE
// (a skill name, document IDs, fn_·hd_·mcp tool refs, a model override) and runs as a ReAct
// loop. Like function/handler it owns an append-only line of Versions; ActiveVersionID is a
// free-moving pointer. There is NO pending/accept state machine — every edit writes a new
// version (max+1) and takes effect immediately; revert just moves the pointer.
//
// Package agent 是 Agent 实体的 domain 层——Quadrinity 第四元：配置好的 LLM worker。Agent 自己
// **不写代码**，靠**按引用挂载**能力（skill 名 / 文档 IDs / fn_·hd_·mcp 工具 ref / model 覆盖），
// 以 ReAct loop 运行。像 function/handler 持一条只增 Version 线；ActiveVersionID 是可自由移动指针。
// **无 pending/accept 状态机**——每次编辑写新版本（max+1）并立即生效；revert 只移指针。
package agent

import (
	"context"
	"strings"
	"time"

	modeldomain "github.com/sunweilin/forgify/backend/internal/domain/model"
	errorspkg "github.com/sunweilin/forgify/backend/internal/pkg/errors"
	schemapkg "github.com/sunweilin/forgify/backend/internal/pkg/schema"
)

// Agent is the top-level entity (ag_ prefix); its mutable config lives on the active Version.
//
// Agent 是顶层实体（ag_ 前缀）；可变配置在 active Version 上。
type Agent struct {
	ID              string     `db:"id,pk"              json:"id"`
	WorkspaceID     string     `db:"workspace_id,ws"    json:"-"`
	Name            string     `db:"name"               json:"name"`
	Description     string     `db:"description"        json:"description"`
	Tags            []string   `db:"tags,json"          json:"tags"`
	ActiveVersionID string     `db:"active_version_id"  json:"activeVersionId"`
	CreatedAt       time.Time  `db:"created_at,created" json:"createdAt"`
	UpdatedAt       time.Time  `db:"updated_at,updated" json:"updatedAt"`
	DeletedAt       *time.Time `db:"deleted_at,deleted" json:"-"`

	// ActiveVersion is a computed (non-column) field attached by Service.Get so a reader sees
	// the agent's current config in one round-trip.
	//
	// ActiveVersion 是计算字段（非列），由 Service.Get 附上，使读者一趟拿到当前配置。
	ActiveVersion *Version `db:"-" json:"activeVersion,omitempty"`
}

// ToolRef is a callable the agent may use. Only fn_ / hd_…method / mcp:server/tool are valid —
// an ag_ ref is forbidden (an agent cannot invoke another agent).
//
// ToolRef 是 agent 可用的 callable。只允许 fn_ / hd_…method / mcp:server/tool——禁 ag_（员工不调员工）。
type ToolRef struct {
	Ref  string `json:"ref"`
	Name string `json:"name"` // display name (resolved at runtime)
}

// MountHealth is one mount's resolvability at a point in time — the on-demand pre-invoke check (a
// deleted function / offline mcp server surfaces as Healthy=false BEFORE the user invokes, instead
// of only when the invoke fails). Same resolution path as invoke, so a broken mount here is exactly
// what an invoke would reject.
//
// MountHealth 是某挂载此刻的可解析性——按需的 invoke 前预检（被删 function / 离线 mcp server 在用户
// invoke 前就以 Healthy=false 暴露，而非等 invoke 失败才知）。与 invoke 同一解析路径，故此处坏的挂载
// 正是 invoke 会拒的那个。
type MountHealth struct {
	Ref     string `json:"ref"`
	Name    string `json:"name,omitempty"` // resolved display name when healthy
	Healthy bool   `json:"healthy"`
	Error   string `json:"error,omitempty"` // why it's broken (deleted / offline / invalid ref)
}

// Version is one immutable snapshot of an agent's config. Version is a monotonic counter
// assigned at write time (max+1) — never reassigned, never renumbered.
//
// Version 是 agent 配置的不可变快照。Version 是写入时分配的单调号（max+1）——绝不重分配、重排号。
type Version struct {
	ID            string                `db:"id,pk"                       json:"id"`
	WorkspaceID   string                `db:"workspace_id,ws"             json:"-"`
	AgentID       string                `db:"agent_id"                    json:"agentId"`
	Version       int                   `db:"version"                     json:"version"`
	Prompt        string                `db:"prompt"                      json:"prompt"`
	Skill         string                `db:"skill"                       json:"skill,omitempty"`         // 0-1 skill name to pre-activate
	Knowledge     []string              `db:"knowledge,json"              json:"knowledge"`               // document IDs attached as context
	Tools         []ToolRef             `db:"tools,json"                  json:"tools"`                   // fn_/hd_/mcp refs (no ag_)
	Inputs        []schemapkg.Field     `db:"inputs,json"                 json:"inputs"`                  // declared task inputs (workflow feeds these)
	Outputs       []schemapkg.Field     `db:"outputs,json"                json:"outputs"`                 // declared result fields (downstream reads these)
	ModelOverride *modeldomain.ModelRef `db:"model_override,json"         json:"modelOverride,omitempty"` // nil → default agent scenario model
	ChangeReason  string                `db:"change_reason"               json:"changeReason,omitempty"`

	// ForgedInConversationID records which conversation produced this version (relation forged/edited edges).
	//
	// ForgedInConversationID 记录哪个对话产出此版本（relation forged/edited 边）。
	ForgedInConversationID string `db:"forged_in_conversation_id" json:"forgedInConversationId,omitempty"`

	CreatedAt time.Time `db:"created_at,created" json:"createdAt"`
	UpdatedAt time.Time `db:"updated_at,updated" json:"updatedAt"`
}

// AcceptedVersionCap bounds retained versions per agent (trimmed on write).
//
// AcceptedVersionCap 限制每 agent 保留版本数（写入时裁剪）。
const AcceptedVersionCap = 50

// ValidateTools rejects an ag_ ref (an agent cannot mount another agent) and any blank ref.
//
// ValidateTools 拒绝 ag_ ref（员工不挂员工）与空 ref。
func (v *Version) ValidateTools() error {
	for _, t := range v.Tools {
		ref := strings.TrimSpace(t.Ref)
		if ref == "" {
			return ErrToolRefBlank
		}
		if strings.HasPrefix(ref, "ag_") {
			return ErrToolsAgentRef
		}
	}
	return nil
}

// Domain errors (wire codes are stable; Kind → HTTP status).
//
// Domain 错误（wire code 稳定；Kind → HTTP status）。
var (
	ErrNotFound             = errorspkg.New(errorspkg.KindNotFound, "AGENT_NOT_FOUND", "agent not found")
	ErrNameConflict         = errorspkg.New(errorspkg.KindConflict, "AGENT_NAME_CONFLICT", "agent name already exists")
	ErrVersionNotFound      = errorspkg.New(errorspkg.KindNotFound, "AGENT_VERSION_NOT_FOUND", "agent version not found")
	ErrNoActiveVersion      = errorspkg.New(errorspkg.KindUnprocessable, "AGENT_NO_ACTIVE_VERSION", "agent has no active version to invoke")
	ErrToolsAgentRef        = errorspkg.New(errorspkg.KindUnprocessable, "AGENT_TOOLS_AGENT_REF", "agent tools cannot reference another agent (ag_ forbidden)")
	ErrToolRefBlank         = errorspkg.New(errorspkg.KindUnprocessable, "AGENT_TOOL_REF_BLANK", "agent tool ref must not be blank")
	ErrMountInvalid         = errorspkg.New(errorspkg.KindUnprocessable, "AGENT_MOUNT_INVALID", "agent mounted tool ref is invalid or unresolvable")
	ErrInvalidModelOverride = errorspkg.New(errorspkg.KindUnprocessable, "AGENT_INVALID_MODEL_OVERRIDE", "invalid modelOverride (apiKeyId and modelId both required)")
	ErrExecutionNotFound    = errorspkg.New(errorspkg.KindNotFound, "AGENT_EXECUTION_NOT_FOUND", "agent execution not found")
)

// Repository is the persistence port for the Agent domain. No GetPending / AcceptVersion —
// edits take effect immediately (there is no pending/accept state machine).
//
// Repository 是 Agent domain 的持久化端口。无 GetPending / AcceptVersion——编辑立即生效
// （无 pending/accept 状态机）。
// VersionListFilter is a cursor page request for one agent's versions (N4 — same shape as
// function/handler).
//
// VersionListFilter 是单 agent 版本的 cursor 分页请求（N4——与 function/handler 同形）。
type VersionListFilter struct {
	Cursor string
	Limit  int
}

// ListFilter is a cursor page request for agents (same shape as function/handler/control).
//
// ListFilter 是 agent 的 cursor 分页请求（与 function/handler/control 同形）。
type ListFilter struct {
	Cursor string
	Limit  int
}

type Repository interface {
	Create(ctx context.Context, a *Agent) error
	Get(ctx context.Context, id string) (*Agent, error)
	GetByIDs(ctx context.Context, ids []string) ([]*Agent, error)
	GetByName(ctx context.Context, name string) (*Agent, error)
	ListAgents(ctx context.Context, filter ListFilter) ([]*Agent, string, error)
	ListAll(ctx context.Context) ([]*Agent, error)
	UpdateMeta(ctx context.Context, a *Agent) error                                // name/description/tags only — no version bump
	SetActiveVersion(ctx context.Context, agentID, versionID string) error         // edit / revert: move the pointer
	CreateWithVersion(ctx context.Context, e *Agent, v *Version) error             // create + v1, one tx
	SaveVersionAndActivate(ctx context.Context, v *Version, entityID string) error // new version + pointer move, one tx
	SoftDelete(ctx context.Context, id string) error

	CreateVersion(ctx context.Context, v *Version) error // version pre-set to max+1 by the Service
	GetVersion(ctx context.Context, versionID string) (*Version, error)
	GetVersionByNumber(ctx context.Context, agentID string, version int) (*Version, error)
	ListVersions(ctx context.Context, agentID string, filter VersionListFilter) ([]*Version, string, error)
	NextVersionNumber(ctx context.Context, agentID string) (int, error) // max(version)+1
	TrimVersions(ctx context.Context, agentID string, keep int) error   // hard-delete oldest beyond the cap

	ExecutionRepository
}
