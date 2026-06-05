// Package document is the orm-backed implementation of documentdomain.Repository:
// a workspace-scoped, soft-deleted markdown tree. Workspace isolation is automatic
// (orm fills/filters workspace_id from ctx), so no method hand-writes a workspace
// predicate. Tree walks (descendant BFS, ancestor chain) are plain Go loops over
// indexed parent_id queries.
//
// Package document 是 documentdomain.Repository 的 orm 实现：按 workspace、软删的 markdown
// 树。workspace 隔离自动（orm 据 ctx 填/过滤 workspace_id），故无方法手写 workspace 谓词。
// 树遍历（后裔 BFS、祖先链）是对 parent_id 索引查询的纯 Go 循环。
package document

import (
	"context"
	"errors"
	"fmt"

	documentdomain "github.com/sunweilin/forgify/backend/internal/domain/document"
	ormpkg "github.com/sunweilin/forgify/backend/internal/pkg/orm"
)

// Schema is the documents DDL, exported as ordered idempotent statements for
// cmd/server to collect and apply via db.Migrate. The UNIQUE index uses
// COALESCE(parent_id,”) so root-level (NULL parent) siblings are also name-unique
// — SQLite would otherwise treat each NULL as distinct and let root dups through.
// The partial WHERE deleted_at IS NULL frees a name once its doc is soft-deleted.
//
// Schema 是 documents 表 DDL，按序幂等语句导出，由 cmd/server 汇总经 db.Migrate 应用。
// UNIQUE 索引用 COALESCE(parent_id,”)，使根级（NULL 父）兄弟也按名唯一——否则 SQLite 把
// 每个 NULL 当作不同值、放过根级重名。partial WHERE deleted_at IS NULL 让软删后名可复用。
var Schema = []string{
	`CREATE TABLE IF NOT EXISTS documents (
		id           TEXT PRIMARY KEY,
		workspace_id TEXT NOT NULL,
		parent_id    TEXT,
		name         TEXT NOT NULL,
		description  TEXT NOT NULL DEFAULT '',
		content      TEXT NOT NULL DEFAULT '',
		tags         TEXT NOT NULL DEFAULT '[]',
		position     INTEGER NOT NULL DEFAULT 0,
		path         TEXT NOT NULL,
		size_bytes   INTEGER NOT NULL DEFAULT 0,
		created_at   DATETIME NOT NULL,
		updated_at   DATETIME NOT NULL,
		deleted_at   DATETIME
	)`,
	`CREATE UNIQUE INDEX IF NOT EXISTS idx_documents_ws_parent_name ON documents(workspace_id, COALESCE(parent_id, ''), name) WHERE deleted_at IS NULL`,
	`CREATE INDEX IF NOT EXISTS idx_documents_ws_parent ON documents(workspace_id, parent_id) WHERE deleted_at IS NULL`,
	`CREATE INDEX IF NOT EXISTS idx_documents_ws_path ON documents(workspace_id, path) WHERE deleted_at IS NULL`,
}

// Store implements documentdomain.Repository over pkg/orm.
//
// Store 基于 pkg/orm 实现 documentdomain.Repository。
type Store struct {
	db   *ormpkg.DB
	repo *ormpkg.Repo[documentdomain.Document]
}

// New constructs a Store bound to the documents table.
//
// New 构造绑定 documents 表的 Store。
func New(db *ormpkg.DB) *Store {
	return &Store{db: db, repo: ormpkg.For[documentdomain.Document](db, "documents")}
}

var _ documentdomain.Repository = (*Store)(nil)

func (s *Store) Insert(ctx context.Context, d *documentdomain.Document) error {
	if err := s.repo.Create(ctx, d); err != nil {
		if errors.Is(err, ormpkg.ErrConflict) {
			return documentdomain.ErrNameConflict
		}
		return fmt.Errorf("documentstore.Insert: %w", err)
	}
	return nil
}

func (s *Store) Get(ctx context.Context, id string) (*documentdomain.Document, error) {
	d, err := s.repo.Get(ctx, id)
	if errors.Is(err, ormpkg.ErrNotFound) {
		return nil, documentdomain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("documentstore.Get: %w", err)
	}
	return d, nil
}

func (s *Store) GetBatch(ctx context.Context, ids []string) ([]*documentdomain.Document, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	rows, err := s.repo.WhereIn("id", toAny(ids)...).Find(ctx)
	if err != nil {
		return nil, fmt.Errorf("documentstore.GetBatch: %w", err)
	}
	return rows, nil
}

func (s *Store) ListByParent(ctx context.Context, parentID *string) ([]*documentdomain.Document, error) {
	q := s.repo.Query()
	if parentID == nil {
		q = q.WhereNull("parent_id")
	} else {
		q = q.WhereEq("parent_id", *parentID)
	}
	rows, err := q.Order("position ASC, created_at ASC").Find(ctx)
	if err != nil {
		return nil, fmt.Errorf("documentstore.ListByParent: %w", err)
	}
	return rows, nil
}

func (s *Store) ListAll(ctx context.Context) ([]*documentdomain.Document, error) {
	rows, err := s.repo.Order("path ASC").Find(ctx)
	if err != nil {
		return nil, fmt.Errorf("documentstore.ListAll: %w", err)
	}
	return rows, nil
}

func (s *Store) Search(ctx context.Context, query string, limit int) ([]*documentdomain.Document, error) {
	if limit <= 0 {
		limit = 50
	}
	like := "%" + query + "%"
	rows, err := s.repo.
		Where("name LIKE ? OR description LIKE ?", like, like).
		Order("updated_at DESC").
		Limit(limit).
		Find(ctx)
	if err != nil {
		return nil, fmt.Errorf("documentstore.Search: %w", err)
	}
	return rows, nil
}

func (s *Store) Update(ctx context.Context, d *documentdomain.Document) error {
	if err := s.repo.Save(ctx, d); err != nil {
		if errors.Is(err, ormpkg.ErrConflict) {
			return documentdomain.ErrNameConflict
		}
		return fmt.Errorf("documentstore.Update: %w", err)
	}
	return nil
}

// UpdateBatch saves all docs in one transaction (path-cascade atomicity).
//
// UpdateBatch 在单事务内 save 所有 doc（path 级联原子性）。
func (s *Store) UpdateBatch(ctx context.Context, docs []*documentdomain.Document) error {
	if len(docs) == 0 {
		return nil
	}
	return s.db.Transaction(ctx, func(tx *ormpkg.DB) error {
		r := ormpkg.For[documentdomain.Document](tx, "documents")
		for _, d := range docs {
			if err := r.Save(ctx, d); err != nil {
				return fmt.Errorf("documentstore.UpdateBatch: id=%s: %w", d.ID, err)
			}
		}
		return nil
	})
}

// SoftDeleteSubtree collects descendants via BFS then soft-deletes all in one statement.
//
// SoftDeleteSubtree 经 BFS 收集后裔，再一次性软删（单 UPDATE deleted_at）。
func (s *Store) SoftDeleteSubtree(ctx context.Context, id string) (int64, error) {
	ids, err := s.collectDescendantIDs(ctx, id)
	if err != nil {
		return 0, fmt.Errorf("documentstore.SoftDeleteSubtree: %w", err)
	}
	if len(ids) == 0 {
		return 0, documentdomain.ErrNotFound
	}
	n, err := s.repo.WhereIn("id", toAny(ids)...).Delete(ctx)
	if err != nil {
		return 0, fmt.Errorf("documentstore.SoftDeleteSubtree: %w", err)
	}
	return n, nil
}

// collectDescendantIDs returns [id, ...all live descendants] via BFS; empty when root missing.
//
// collectDescendantIDs 经 BFS 返 [id, ...所有活跃后裔]；root 不存在返空。
func (s *Store) collectDescendantIDs(ctx context.Context, id string) ([]string, error) {
	if _, err := s.repo.Get(ctx, id); err != nil {
		if errors.Is(err, ormpkg.ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}
	ids := []string{id}
	frontier := []string{id}
	for len(frontier) > 0 {
		kids, err := s.repo.WhereIn("parent_id", toAny(frontier)...).Find(ctx)
		if err != nil {
			return nil, err
		}
		frontier = frontier[:0]
		for _, k := range kids {
			ids = append(ids, k.ID)
			frontier = append(frontier, k.ID)
		}
	}
	return ids, nil
}

// IsAncestor walks descendant's parent chain to detect whether candidate sits above it.
//
// IsAncestor 沿 descendant 的 parent 链向上爬，判断 candidate 是否在其祖先链上。
func (s *Store) IsAncestor(ctx context.Context, candidateAncestorID, descendantID string) (bool, error) {
	if candidateAncestorID == descendantID {
		return true, nil
	}
	cursor := descendantID
	for range 10_000 { // depth bound — bail on misshapen data instead of looping forever
		row, err := s.repo.Get(ctx, cursor)
		if errors.Is(err, ormpkg.ErrNotFound) {
			return false, nil
		}
		if err != nil {
			return false, fmt.Errorf("documentstore.IsAncestor: %w", err)
		}
		if row.ParentID == nil {
			return false, nil
		}
		if *row.ParentID == candidateAncestorID {
			return true, nil
		}
		cursor = *row.ParentID
	}
	return false, fmt.Errorf("documentstore.IsAncestor: parent chain exceeds depth bound (corrupt data?)")
}

func (s *Store) CountDescendants(ctx context.Context, id string) (int64, error) {
	ids, err := s.collectDescendantIDs(ctx, id)
	if err != nil {
		return 0, fmt.Errorf("documentstore.CountDescendants: %w", err)
	}
	if len(ids) == 0 {
		return 0, nil
	}
	return int64(len(ids) - 1), nil // minus the root itself
}

func (s *Store) MaxSiblingPosition(ctx context.Context, parentID *string) (int, error) {
	q := s.repo.Query()
	if parentID == nil {
		q = q.WhereNull("parent_id")
	} else {
		q = q.WhereEq("parent_id", *parentID)
	}
	rows, err := q.Order("position DESC").Limit(1).Find(ctx)
	if err != nil {
		return -1, fmt.Errorf("documentstore.MaxSiblingPosition: %w", err)
	}
	if len(rows) == 0 {
		return -1, nil
	}
	return rows[0].Position, nil
}

func (s *Store) ListSubtreeIDs(ctx context.Context, rootID string) ([]string, error) {
	return s.collectDescendantIDs(ctx, rootID)
}

// toAny widens a []string to []any for orm WhereIn variadic args.
//
// toAny 把 []string 拓宽为 []any 以喂 orm WhereIn 变长参数。
func toAny(ss []string) []any {
	out := make([]any, len(ss))
	for i, v := range ss {
		out[i] = v
	}
	return out
}
