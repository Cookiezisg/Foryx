// Package relation is the domain layer for cross-entity edges (relgraph data backbone).
//
// Package relation 是跨实体边的 domain 层（relgraph 数据底座）。
package relation

import (
	"context"
	"errors"
	"time"
)

// Relation is one directed edge from one entity to another, carrying a kind + attrs.
// Live-derived from source domain state; no soft-delete (hard-delete on entity removal).
//
// Relation 是一条有向边，从一个实体指向另一个，带 kind + attrs。
// 由 source domain 状态派生，无软删（实体删时硬删边）。
type Relation struct {
	ID     string `gorm:"primaryKey;type:text" json:"id"`
	UserID string `gorm:"not null;type:text;index:idx_rel_fwd,priority:1;index:idx_rel_rev,priority:1;index:idx_rel_user_kind,priority:1;uniqueIndex:uq_rel,priority:1" json:"userId"`

	FromKind string `gorm:"not null;type:text;index:idx_rel_fwd,priority:2;uniqueIndex:uq_rel,priority:2" json:"fromKind"`
	FromID   string `gorm:"not null;type:text;index:idx_rel_fwd,priority:3;uniqueIndex:uq_rel,priority:3" json:"fromId"`
	ToKind   string `gorm:"not null;type:text;index:idx_rel_rev,priority:2;uniqueIndex:uq_rel,priority:4" json:"toKind"`
	ToID     string `gorm:"not null;type:text;index:idx_rel_rev,priority:3;uniqueIndex:uq_rel,priority:5" json:"toId"`

	// check constraint enumerates all valid kinds; update this AND IsValidKind when adding new kinds.
	Kind string `gorm:"not null;type:text;index:idx_rel_user_kind,priority:2;uniqueIndex:uq_rel,priority:6;check:kind IN ('conversation_forged_entity','conversation_edited_entity','workflow_uses_function','workflow_uses_handler','workflow_uses_mcp','workflow_uses_skill','workflow_uses_document','document_links_entity','workflow_uses_agent','agent_uses_function','agent_uses_handler','agent_uses_mcp','agent_uses_document','agent_uses_skill')" json:"kind"`

	Attrs map[string]any `gorm:"serializer:json;type:text;default:'{}'" json:"attrs,omitempty"`

	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

func (Relation) TableName() string { return "relations" }

// Closed edge kind enumeration (14 kinds). DB CHECK + service validation both reference these constants.
// Note: agent_uses_agent is intentionally absent — agents cannot call other agents (员工思维, doc 00).
//
// 14 种闭合边类型枚举。注意：无 agent_uses_agent（员工思维，agent 不能调 agent）。
const (
	KindConversationForgedEntity = "conversation_forged_entity"
	KindConversationEditedEntity = "conversation_edited_entity"
	KindWorkflowUsesFunction     = "workflow_uses_function"
	KindWorkflowUsesHandler      = "workflow_uses_handler"
	KindWorkflowUsesMCP          = "workflow_uses_mcp"
	KindWorkflowUsesSkill        = "workflow_uses_skill"
	KindWorkflowUsesDocument     = "workflow_uses_document"
	KindDocumentLinksEntity      = "document_links_entity"
	// Agent relation kinds (doc 11 §S3 / doc 09 quadrinity). No agent_uses_agent.
	KindWorkflowUsesAgent = "workflow_uses_agent"
	KindAgentUsesFunction = "agent_uses_function"
	KindAgentUsesHandler  = "agent_uses_handler"
	KindAgentUsesMCP      = "agent_uses_mcp"
	KindAgentUsesDocument = "agent_uses_document"
	KindAgentUsesSkill    = "agent_uses_skill"
)

// IsValidKind reports whether k is one of the 14 closed enum values.
//
// IsValidKind 报告 k 是否 14 种闭合枚举之一。
func IsValidKind(k string) bool {
	switch k {
	case KindConversationForgedEntity, KindConversationEditedEntity,
		KindWorkflowUsesFunction, KindWorkflowUsesHandler,
		KindWorkflowUsesMCP, KindWorkflowUsesSkill, KindWorkflowUsesDocument,
		KindDocumentLinksEntity,
		KindWorkflowUsesAgent,
		KindAgentUsesFunction, KindAgentUsesHandler, KindAgentUsesMCP,
		KindAgentUsesDocument, KindAgentUsesSkill:
		return true
	}
	return false
}

// Entity kind constants for from_kind / to_kind fields.
//
// from_kind / to_kind 的实体类型常量。
const (
	EntityKindWorkflow     = "workflow"
	EntityKindFunction     = "function"
	EntityKindHandler      = "handler"
	EntityKindDocument     = "document"
	EntityKindConversation = "conversation"
	EntityKindSkill        = "skill"
	EntityKindMCP          = "mcp"
	EntityKindAgent        = "agent" // quadrinity 4th member (doc 09)
)

// IsValidEntityKind reports whether k is one of the 8 entity kinds that can appear in from_kind/to_kind.
//
// IsValidEntityKind 报告 k 是否 8 种可出现在 from_kind/to_kind 的实体类型之一。
func IsValidEntityKind(k string) bool {
	switch k {
	case EntityKindWorkflow, EntityKindFunction, EntityKindHandler,
		EntityKindDocument, EntityKindConversation, EntityKindSkill, EntityKindMCP,
		EntityKindAgent:
		return true
	}
	return false
}

// Neighborhood depth bounds.
const (
	MinNeighborhoodDepth = 1
	MaxNeighborhoodDepth = 3
)

var (
	ErrInvalidEntityRef  = errors.New("relation: invalid entity ref (kind or id)")
	ErrInvalidKind       = errors.New("relation: invalid relation kind")
	ErrDepthOutOfRange   = errors.New("relation: neighborhood depth out of range")
	ErrIncompleteFilter  = errors.New("relation: incomplete filter (kind without id, or vice versa)")
	ErrSelfLoop          = errors.New("relation: self-loop forbidden (from == to)")
)

// SyncEdge is the input shape callers use to declare an outgoing/incoming edge.
// The fixed side is implicit (the caller's entity passed to Sync* method);
// OtherKind/OtherID declare the variable side:
//   - SyncOutgoing: Other* = TO side (target the caller's entity references)
//   - SyncIncoming: Other* = FROM side (source that references the caller's entity)
//
// SyncEdge 是调用方声明出/入边时用的输入形状。
// 固定端由 Sync* 方法的调用方传入隐式提供；OtherKind/OtherID 声明可变端：
//   - SyncOutgoing：Other* = TO 端（调用方实体引用的目标）
//   - SyncIncoming：Other* = FROM 端（引用调用方实体的源头）
type SyncEdge struct {
	OtherKind string
	OtherID   string
	Kind      string
	Attrs     map[string]any
}

// Filter is the query input for List; empty fields mean "no filter on that field".
//
// Filter 是 List 的查询输入；空字段表示对该字段不过滤。
type Filter struct {
	FromKind string
	FromID   string
	ToKind   string
	ToID     string
	Kind     string
}

// GraphNode is one node in the relgraph snapshot; label/sub populated by Service via reader ports.
//
// GraphNode 是 relgraph 快照里的一个节点；label/sub 由 Service 通过 reader port 填充。
type GraphNode struct {
	Kind  string `json:"kind"`
	ID    string `json:"id"`
	Label string `json:"label"`
	Sub   string `json:"sub,omitempty"`
}

// EntityMeta is the slim shape source domains return for relgraph node assembly.
// Lives in domain package so consumer domains can implement reader ports without
// importing the relation app package (avoiding circular imports).
//
// EntityMeta 是 source domain 为 relgraph 节点组装提供的精简形状。
// 放在 domain 包，让消费者 domain 实现 reader port 时无需 import relation app 包
// （避免循环依赖）。
type EntityMeta struct {
	ID    string
	Label string
	Sub   string
}

// Snapshot is the /relgraph response shape.
//
// Snapshot 是 /relgraph 端点的响应形状。
type Snapshot struct {
	Nodes []GraphNode `json:"nodes"`
	Edges []*Relation `json:"edges"`
}

// Service is the high-level relation API consumed by source-domain hooks and HTTP handlers.
//
// Service 是 source domain hook 和 HTTP handler 消费的高层 relation API。
type Service interface {
	// SyncOutgoing replaces all edges where (from_kind, from_id) match AND kind ∈ kindScope,
	// with the given edges. Diff & sync idempotent.
	//
	// SyncOutgoing 把 (from_kind, from_id) 在 kindScope 范围内的所有出向边整组替换。
	SyncOutgoing(ctx context.Context, fromKind, fromID string,
		kindScope []string, edges []SyncEdge) error

	// SyncIncoming replaces all edges where (to_kind, to_id) match AND kind ∈ kindScope.
	// Used for "at most 1 forged + 1 edited" semantics on trinity entities.
	//
	// SyncIncoming 把 (to_kind, to_id) 在 kindScope 范围内的所有入向边整组替换。
	// 用于 trinity 实体的 "至多 1 forged + 1 edited" 语义。
	SyncIncoming(ctx context.Context, toKind, toID string,
		kindScope []string, edges []SyncEdge) error

	// PurgeEntity hard-deletes all edges where from_id=id OR to_id=id.
	// Called by source-domain Delete() services in the same transaction.
	//
	// PurgeEntity 硬删所有 from_id=id 或 to_id=id 的边。由 source domain Delete() 同事务调。
	PurgeEntity(ctx context.Context, kind, id string) error

	// List returns relations matching filter, paginated.
	//
	// List 按 filter 返边，分页。
	List(ctx context.Context, filter Filter, cursor string, limit int) ([]*Relation, string, bool, error)

	// Neighborhood returns all edges within depth hops of the center entity (BFS).
	//
	// Neighborhood 返中心实体 depth 跳内的所有边（BFS）。
	Neighborhood(ctx context.Context, kind, id string, depth int) ([]*Relation, error)

	// GetRelgraph returns the full relgraph snapshot (no pagination, no limit).
	//
	// GetRelgraph 返完整 relgraph 快照（无分页，无上限）。
	GetRelgraph(ctx context.Context) (*Snapshot, error)
}

// Repository is the storage contract; UserID-scoped. Service calls these via diff-sync algorithms.
//
// Repository 是存储契约，按 UserID 作用域。Service 通过 diff-sync 算法调用。
type Repository interface {
	// Insert adds a single relation.
	Insert(ctx context.Context, r *Relation) error

	// InsertBatch adds multiple relations in one transaction (idempotent on uq_rel conflict).
	InsertBatch(ctx context.Context, rels []*Relation) error

	// UpdateAttrs updates only the Attrs JSON of an existing row by ID.
	UpdateAttrs(ctx context.Context, id string, attrs map[string]any) error

	// DeleteByIDs hard-deletes rows by ID list.
	DeleteByIDs(ctx context.Context, ids []string) error

	// ListByFromAndKinds returns existing edges for (user, from_kind, from_id) AND kind ∈ kinds.
	// Used by SyncOutgoing to compute diff against incoming edges.
	ListByFromAndKinds(ctx context.Context, userID, fromKind, fromID string, kinds []string) ([]*Relation, error)

	// ListByToAndKinds is the mirror of ListByFromAndKinds for SyncIncoming.
	ListByToAndKinds(ctx context.Context, userID, toKind, toID string, kinds []string) ([]*Relation, error)

	// List returns rows matching filter; nextCursor is opaque; hasMore signals more pages.
	List(ctx context.Context, userID string, filter Filter, cursor string, limit int) (rows []*Relation, nextCursor string, hasMore bool, err error)

	// ListAll returns every edge for the user (used by relgraph snapshot).
	ListAll(ctx context.Context, userID string) ([]*Relation, error)

	// PurgeEntity hard-deletes all rows where from=(kind,id) OR to=(kind,id).
	PurgeEntity(ctx context.Context, userID, kind, id string) (int64, error)
}
