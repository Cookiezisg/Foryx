// Package document is the domain layer for the Notion-style tree document library:
// a per-workspace markdown tree (parent/children, ordered, path-addressed) that the
// user @-mentions, attaches to chats/workflows, and wikilinks between. Pure structs +
// the storage contract; the tree algorithms and cross-module adapters live in app.
//
// Package document 是 Notion-style 树状文档库的 domain 层：按 workspace 的 markdown 树
// （父子、有序、path 寻址），可被 @ 引用、挂载到对话/workflow、用 wikilink 互链。纯 struct
// + 存储契约；树算法与跨模块适配器在 app。
package document

import (
	"context"
	"time"

	errorsdomain "github.com/sunweilin/forgify/backend/internal/domain/errors"
)

// Document is one node in the workspace's markdown tree; ParentID nil = root-level.
//
// Document 是 workspace markdown 树的一个节点；ParentID nil = 根级。
type Document struct {
	ID          string     `db:"id,pk"              json:"id"`
	WorkspaceID string     `db:"workspace_id,ws"    json:"-"`
	ParentID    *string    `db:"parent_id"          json:"parentId,omitempty"`
	Name        string     `db:"name"               json:"name"`
	Description string     `db:"description"        json:"description"`
	Content     string     `db:"content"            json:"content"`
	Tags        []string   `db:"tags,json"          json:"tags"`
	Position    int        `db:"position"           json:"position"`
	Path        string     `db:"path"               json:"path"`
	SizeBytes   int64      `db:"size_bytes"         json:"sizeBytes"`
	CreatedAt   time.Time  `db:"created_at,created" json:"createdAt"`
	UpdatedAt   time.Time  `db:"updated_at,updated" json:"updatedAt"`
	DeletedAt   *time.Time `db:"deleted_at,deleted" json:"-"`
}

// MaxContentBytes caps a single document body; oversized payloads should split into child docs.
//
// MaxContentBytes 单文档 markdown 上限；超出应拆子文档。
const MaxContentBytes = 1 << 20 // 1 MB

// MaxNameLength caps the title.
//
// MaxNameLength 标题上限。
const MaxNameLength = 256

var (
	ErrNotFound        = errorsdomain.New(errorsdomain.KindNotFound, "DOCUMENT_NOT_FOUND", "document not found")
	ErrInvalidParent   = errorsdomain.New(errorsdomain.KindUnprocessable, "DOCUMENT_INVALID_PARENT", "invalid parent (cycle or self)")
	ErrNameConflict    = errorsdomain.New(errorsdomain.KindConflict, "DOCUMENT_NAME_CONFLICT", "name already exists under same parent")
	ErrContentTooLarge = errorsdomain.New(errorsdomain.KindTooLarge, "DOCUMENT_CONTENT_TOO_LARGE", "content exceeds 1 MB limit")
	ErrInvalidName     = errorsdomain.New(errorsdomain.KindInvalid, "DOCUMENT_INVALID_NAME", "invalid name (empty, too long, or contains '/')")
	ErrParentNotFound  = errorsdomain.New(errorsdomain.KindUnprocessable, "DOCUMENT_PARENT_NOT_FOUND", "parent not found")
)

// CreateInput is the write payload for a new document; WorkspaceID is filled by the
// orm layer from ctx.
//
// CreateInput 新建写入载荷；WorkspaceID 由 orm 层从 ctx 填。
type CreateInput struct {
	Name        string
	ParentID    *string
	Content     string
	Description string
	Tags        []string
}

// UpdateInput is a partial-update payload; nil pointers mean "leave alone".
//
// UpdateInput 部分更新载荷；nil 指针表示不动。
type UpdateInput struct {
	Name        *string
	Description *string
	Content     *string
	Tags        *[]string
}

// MoveInput identifies a relocation; nil ParentID moves to root; nil Position appends to end.
//
// MoveInput 描述一次移动；nil ParentID 移到根；nil Position 追加到末尾。
type MoveInput struct {
	ParentID *string
	Position *int
}

// AttachedDocument is one entry in a conversation / workflow node's attach list. The
// referenced document is injected verbatim — only that single doc, never its subtree.
// Subtree auto-injection was deliberately dropped: an attach must be explicit and
// bounded, not "attach one, drag in a whole tree" that blows the context budget.
//
// AttachedDocument 是 conversation / workflow 节点挂载列表的一项。被引文档原样注入——只那
// 一篇，绝不连子树。子树自动注入已刻意砍掉：挂载必须显式且有界，不能"挂一篇拖出一整棵树"
// 炸 context。
type AttachedDocument struct {
	DocumentID string `json:"documentId"`
}

// Repository is the storage contract; workspace isolation is applied by the orm layer
// from ctx, so methods take no workspace id. Soft-delete (deleted_at) — a deleted
// subtree keeps a tombstone, unlike sandbox's hard-deleted manifest.
//
// Repository 是存储契约；workspace 隔离由 orm 层据 ctx 施加，故方法不带 workspace id。
// 软删（deleted_at）——删除的子树留墓碑，不同于 sandbox manifest 的硬删。
type Repository interface {
	Insert(ctx context.Context, d *Document) error
	Get(ctx context.Context, id string) (*Document, error)
	GetBatch(ctx context.Context, ids []string) ([]*Document, error)
	ListByParent(ctx context.Context, parentID *string) ([]*Document, error)
	ListAll(ctx context.Context) ([]*Document, error)
	Search(ctx context.Context, query string, limit int) ([]*Document, error)
	Update(ctx context.Context, d *Document) error
	UpdateBatch(ctx context.Context, docs []*Document) error
	SoftDeleteSubtree(ctx context.Context, id string) (deletedCount int64, err error)
	IsAncestor(ctx context.Context, candidateAncestorID, descendantID string) (bool, error)
	CountDescendants(ctx context.Context, id string) (int64, error)
	MaxSiblingPosition(ctx context.Context, parentID *string) (int, error)

	// ListSubtreeIDs returns [rootID, ...all live descendant IDs] via BFS; empty when
	// root id not found. Used by Delete's relation-edge purge.
	//
	// ListSubtreeIDs 经 BFS 返 [rootID, ...所有活跃后裔 ID]；root id 不存在返空。供 Delete
	// 的 relation 边清理用。
	ListSubtreeIDs(ctx context.Context, rootID string) ([]string, error)
}
