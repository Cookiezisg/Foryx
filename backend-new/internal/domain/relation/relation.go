// Package relation owns the cross-entity topology graph: directed edges between
// entities, the entity/edge kind vocabulary, and the id-prefix→kind routing
// inherited from idgen. Edges are derived data — written explicitly by source
// domains after they forge/equip/link, never authored directly — so there is no
// soft-delete (an entity's edges are hard-deleted with it) and the read side is
// pure projection. Display names are NOT stored: they are looked up fresh at read
// time so a renamed entity always shows current.
//
// Package relation 持有跨实体拓扑图：实体间有向边、实体/边类型词表、从 idgen 收编的
// id 前缀→类型路由。边是派生数据——由各 source domain 在 forge/equip/link 后显式写入、
// 从不直接编写——故无软删（实体的边随实体硬删），读侧是纯投影。显示名不入库：读时现查，
// 故改名后永远显示最新。
package relation

import (
	"context"
	"time"

	errorsdomain "github.com/sunweilin/forgify/backend/internal/domain/errors"
)

// Relation is one directed edge: (from_kind, from_id) --kind--> (to_kind, to_id),
// with optional attrs (e.g. an edit edge's version number). No name columns — see
// the package doc on read-time name lookup.
//
// Relation 是一条有向边：(from_kind, from_id) --kind--> (to_kind, to_id)，带可选 attrs
// （如 edit 边的版本号）。无 name 列——读时取名见包注释。
type Relation struct {
	ID          string         `db:"id,pk" json:"id"`
	WorkspaceID string         `db:"workspace_id,ws" json:"-"`
	Kind        string         `db:"kind" json:"kind"`
	FromKind    string         `db:"from_kind" json:"fromKind"`
	FromID      string         `db:"from_id" json:"fromId"`
	ToKind      string         `db:"to_kind" json:"toKind"`
	ToID        string         `db:"to_id" json:"toId"`
	Attrs       map[string]any `db:"attrs,json" json:"attrs,omitempty"`
	CreatedAt   time.Time      `db:"created_at,created" json:"createdAt"`
	UpdatedAt   time.Time      `db:"updated_at,updated" json:"updatedAt"`
}

// RelationView is an edge decorated with both endpoints' current display names,
// assembled in memory at read time. FromName/ToName fall back to the raw id when
// the entity is gone or not yet name-resolvable (skill/mcp today).
//
// RelationView 是带两端当前显示名的边，读时内存组装。实体已删或暂不可解析名字（当前的
// skill/mcp）时，FromName/ToName 回退为原始 id。
type RelationView struct {
	Relation
	FromName string `json:"fromName"`
	ToName   string `json:"toName"`
}

// Node is a graph node: a distinct entity referenced by some edge plus its current
// name. Isolated entities (no edges) do not appear — the graph shows relationships,
// not an inventory.
//
// Node 是图节点：被某条边引用的去重实体 + 其当前名字。孤立实体（无边）不出现——图展示
// 关系，而非清单。
type Node struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Snapshot is the /relgraph response: nodes deduped from edge endpoints + the
// hydrated edges.
//
// Snapshot 是 /relgraph 响应：从边端点去重的节点 + hydrate 后的边。
type Snapshot struct {
	Nodes []Node          `json:"nodes"`
	Edges []*RelationView `json:"edges"`
}

// SyncEdge declares one variable-side endpoint for a Sync* call. The fixed side is
// the caller's own entity; OtherKind/OtherID is the other end — TO for SyncOutgoing,
// FROM for SyncIncoming.
//
// SyncEdge 声明 Sync* 调用的可变端。固定端是调用方自身实体；OtherKind/OtherID 是另一端
// ——SyncOutgoing 时为 TO，SyncIncoming 时为 FROM。
type SyncEdge struct {
	OtherKind string
	OtherID   string
	Kind      string
	Attrs     map[string]any
}

// Filter is the List query input; an empty field means no filter on it. A kind/id
// pair must be supplied together or not at all (enforced by the Service).
//
// Filter 是 List 查询输入；空字段表示不过滤。kind/id 对必须成对给出或都不给（Service 强制）。
type Filter struct {
	FromKind string
	FromID   string
	ToKind   string
	ToID     string
	Kind     string
}

// Neighborhood BFS depth bounds.
//
// Neighborhood BFS 深度边界。
const (
	MinNeighborhoodDepth = 1
	MaxNeighborhoodDepth = 3
)

var (
	// ErrInvalidRef: source or target ref has an empty id or an unknown entity kind.
	// ErrInvalidRef：源或目标 ref 的 id 为空或实体类型未知。
	ErrInvalidRef = errorsdomain.New(errorsdomain.KindInvalid, "REL_INVALID_REF", "invalid entity ref (unknown kind or empty id)")

	// ErrInvalidKind: an edge kind is not one of the 4 verbs.
	// ErrInvalidKind：边类型不是 4 个动词之一。
	ErrInvalidKind = errorsdomain.New(errorsdomain.KindInvalid, "REL_INVALID_KIND", "invalid relation kind")

	// ErrSelfLoop: an entity may not reference itself.
	// ErrSelfLoop：实体不能引用自身。
	ErrSelfLoop = errorsdomain.New(errorsdomain.KindInvalid, "REL_SELF_LOOP", "self-loop forbidden (from == to)")

	// ErrDepthOutOfRange: neighborhood depth outside [Min,Max].
	// ErrDepthOutOfRange：neighborhood 深度超出 [Min,Max]。
	ErrDepthOutOfRange = errorsdomain.New(errorsdomain.KindInvalid, "REL_DEPTH_LIMIT", "neighborhood depth out of range")

	// ErrIncompleteFilter: a filter gave a kind without its id, or vice versa.
	// ErrIncompleteFilter：filter 给了 kind 却没给 id，或反之。
	ErrIncompleteFilter = errorsdomain.New(errorsdomain.KindInvalid, "REL_INCOMPLETE_FILTER", "incomplete filter (kind without id, or vice versa)")
)

// Service is the high-level relation API. The write side (Sync*/Purge) is called
// by source-domain hooks after they settle an active version; the read side
// (List/Neighborhood/GetRelgraph) backs the 3 read-only HTTP endpoints and returns
// hydrated views (names filled).
//
// Service 是高层 relation API。写侧（Sync*/Purge）由 source domain 在 active version
// 落定后调用；读侧（List/Neighborhood/GetRelgraph）支撑 3 个只读 HTTP 端点，返回 hydrate
// 后的视图（已填名字）。
type Service interface {
	// SyncOutgoing replaces all edges where (from) match AND kind ∈ kindScope with
	// the given edges (idempotent diff-sync). Used for equip / link.
	//
	// SyncOutgoing 把 (from) 匹配且 kind ∈ kindScope 的所有出边整组替换为给定边（幂等
	// diff-sync）。用于 equip / link。
	SyncOutgoing(ctx context.Context, fromKind, fromID string, kindScope []string, edges []SyncEdge) error

	// SyncIncoming mirrors SyncOutgoing on the (to) side. Used for create / edit,
	// which the forged entity declares about itself (the conversation does not know
	// what it forged).
	//
	// SyncIncoming 是 SyncOutgoing 在 (to) 侧的镜像。用于 create / edit，由被锻造实体
	// 自述（对话不知道自己锻造了什么）。
	SyncIncoming(ctx context.Context, toKind, toID string, kindScope []string, edges []SyncEdge) error

	// PurgeEntity hard-deletes every edge touching the entity (from OR to). Called
	// by the source domain's Delete in the same flow.
	//
	// PurgeEntity 硬删所有触及该实体的边（from 或 to）。由 source domain 的 Delete 同流程调。
	PurgeEntity(ctx context.Context, kind, id string) error

	// List returns hydrated edges matching filter, keyset-paginated; next == "" at end.
	//
	// List 返回匹配 filter 的 hydrate 边，keyset 分页；到底时 next == ""。
	List(ctx context.Context, filter Filter, cursor string, limit int) (edges []*RelationView, next string, err error)

	// Neighborhood returns hydrated edges within depth hops of the center (BFS).
	//
	// Neighborhood 返回中心实体 depth 跳内的 hydrate 边（BFS）。
	Neighborhood(ctx context.Context, kind, id string, depth int) ([]*RelationView, error)

	// GetRelgraph returns the full hydrated snapshot (nodes + edges, no pagination).
	//
	// GetRelgraph 返回完整 hydrate 快照（节点 + 边，无分页）。
	GetRelgraph(ctx context.Context) (*Snapshot, error)
}

// Repository is the storage contract; workspace isolation is applied by the orm
// layer from ctx, so methods take no userID.
//
// Repository 是存储契约；workspace 隔离由 orm 层据 ctx 施加，故方法不带 userID。
type Repository interface {
	// InsertBatch inserts rows in one statement, idempotent on the dedup unique index.
	// InsertBatch 一条语句插入多行，对 dedup 唯一索引幂等。
	InsertBatch(ctx context.Context, rels []*Relation) error

	// UpdateAttrs rewrites only the attrs of one row by id.
	// UpdateAttrs 仅按 id 重写一行的 attrs。
	UpdateAttrs(ctx context.Context, id string, attrs map[string]any) error

	// DeleteByIDs hard-deletes rows by id list.
	// DeleteByIDs 按 id 列表硬删。
	DeleteByIDs(ctx context.Context, ids []string) error

	// ListByFromAndKinds returns edges for (from) within kind scope; empty kinds = all.
	// ListByFromAndKinds 返回 (from) 在 kind 范围内的边；空 kinds = 全部。
	ListByFromAndKinds(ctx context.Context, fromKind, fromID string, kinds []string) ([]*Relation, error)

	// ListByToAndKinds is the mirror on the (to) side.
	// ListByToAndKinds 是 (to) 侧镜像。
	ListByToAndKinds(ctx context.Context, toKind, toID string, kinds []string) ([]*Relation, error)

	// List returns rows matching filter, keyset-paginated; next == "" at end.
	// List 返回匹配 filter 的行，keyset 分页；到底时 next == ""。
	List(ctx context.Context, filter Filter, cursor string, limit int) (rows []*Relation, next string, err error)

	// ListAll returns every edge for the workspace (relgraph snapshot, no limit).
	// ListAll 返回该 workspace 的所有边（relgraph 快照，无上限）。
	ListAll(ctx context.Context) ([]*Relation, error)

	// PurgeEntity hard-deletes rows where from OR to is (kind,id); returns count.
	// PurgeEntity 硬删 from 或 to 为 (kind,id) 的行；返回条数。
	PurgeEntity(ctx context.Context, kind, id string) (int64, error)
}
