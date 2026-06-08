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

	errorsdomain "github.com/sunweilin/forgify/backend/internal/domain/errors"
	modeldomain "github.com/sunweilin/forgify/backend/internal/domain/model"
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
	Skill         string                `db:"skill"                       json:"skill,omitempty"` // 0-1 skill name to pre-activate
	Knowledge     []string              `db:"knowledge,json"              json:"knowledge"`       // document IDs attached as context
	Tools         []ToolRef             `db:"tools,json"                  json:"tools"`           // fn_/hd_/mcp refs (no ag_)
	Inputs        []schemapkg.Field     `db:"inputs,json"                 json:"inputs"`  // declared task inputs (workflow feeds these)
	Outputs       []schemapkg.Field     `db:"outputs,json"                json:"outputs"` // declared result fields (downstream reads these)
	ModelOverride *modeldomain.ModelRef `db:"model_override,json"         json:"modelOverride,omitempty"` // nil → default agent scenario model
	ChangeReason  string                `db:"change_reason"               json:"changeReason,omitempty"`

	// ForgedInConversationID records which conversation produced this version (relation forged/edited edges).
	//
	// ForgedInConversationID 记录哪个对话产出此版本（relation forged/edited 边）。
	ForgedInConversationID string `db:"forged_in_conversation_id" json:"forgedInConversationId,omitempty"`

	CreatedAt time.Time `db:"created_at,created" json:"createdAt"`
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
	ErrNotFound             = errorsdomain.New(errorsdomain.KindNotFound, "AGENT_NOT_FOUND", "agent not found")
	ErrNameConflict         = errorsdomain.New(errorsdomain.KindConflict, "AGENT_NAME_CONFLICT", "agent name already exists")
	ErrVersionNotFound      = errorsdomain.New(errorsdomain.KindNotFound, "AGENT_VERSION_NOT_FOUND", "agent version not found")
	ErrNoActiveVersion      = errorsdomain.New(errorsdomain.KindUnprocessable, "AGENT_NO_ACTIVE_VERSION", "agent has no active version to invoke")
	ErrToolsAgentRef        = errorsdomain.New(errorsdomain.KindUnprocessable, "AGENT_TOOLS_AGENT_REF", "agent tools cannot reference another agent (ag_ forbidden)")
	ErrToolRefBlank         = errorsdomain.New(errorsdomain.KindUnprocessable, "AGENT_TOOL_REF_BLANK", "agent tool ref must not be blank")
	ErrInvalidModelOverride = errorsdomain.New(errorsdomain.KindUnprocessable, "AGENT_INVALID_MODEL_OVERRIDE", "invalid modelOverride (apiKeyId and modelId both required)")
	ErrExecutionNotFound    = errorsdomain.New(errorsdomain.KindNotFound, "AGENT_EXECUTION_NOT_FOUND", "agent execution not found")
)

// Repository is the persistence port for the Agent domain. No GetPending / AcceptVersion —
// edits take effect immediately (the pending/accept machine is gone).
//
// Repository 是 Agent domain 的持久化端口。无 GetPending / AcceptVersion——编辑立即生效
// （pending/accept 机制已去除）。
type Repository interface {
	Create(ctx context.Context, a *Agent) error
	Get(ctx context.Context, id string) (*Agent, error)
	GetByName(ctx context.Context, name string) (*Agent, error)
	List(ctx context.Context, limit int, cursor string) ([]*Agent, string, error)
	ListAll(ctx context.Context) ([]*Agent, error)
	UpdateMeta(ctx context.Context, a *Agent) error                        // name/description/tags only — no version bump
	SetActiveVersion(ctx context.Context, agentID, versionID string) error // edit / revert: move the pointer
	SoftDelete(ctx context.Context, id string) error

	CreateVersion(ctx context.Context, v *Version) error // version pre-set to max+1 by the Service
	GetVersion(ctx context.Context, versionID string) (*Version, error)
	GetVersionByNumber(ctx context.Context, agentID string, version int) (*Version, error)
	ListVersions(ctx context.Context, agentID string) ([]*Version, error)
	NextVersionNumber(ctx context.Context, agentID string) (int, error) // max(version)+1
	TrimVersions(ctx context.Context, agentID string, keep int) error   // hard-delete oldest beyond the cap

	ExecutionRepository
}
