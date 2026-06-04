// Package relation is the orm-backed implementation of relationdomain.Repository
// plus the relations table DDL. Edges are hard-deleted (no soft-delete column) and
// workspace isolation is applied automatically by the orm layer from ctx.
//
// Package relation 是 relationdomain.Repository 的 orm 实现 + relations 表 DDL。
// 边硬删（无软删列），workspace 隔离由 orm 层据 ctx 自动施加。
package relation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	relationdomain "github.com/sunweilin/forgify/backend/internal/domain/relation"
	ormpkg "github.com/sunweilin/forgify/backend/internal/pkg/orm"
)

// Schema is the relations DDL. No deleted_at — an entity's edges are hard-deleted
// with it. idx_rel_dedup makes (from,to,kind) unique per workspace so re-syncs are
// idempotent; the from/to indexes back the directional neighborhood walks.
//
// Schema 是 relations 表 DDL。无 deleted_at——边随实体硬删。idx_rel_dedup 使每 workspace
// 下 (from,to,kind) 唯一，故重同步幂等；from/to 索引支撑方向性邻域遍历。
var Schema = []string{
	`CREATE TABLE IF NOT EXISTS relations (
		id           TEXT PRIMARY KEY,
		workspace_id TEXT NOT NULL,
		kind         TEXT NOT NULL CHECK (kind IN ('create','edit','equip','link')),
		from_kind    TEXT NOT NULL,
		from_id      TEXT NOT NULL,
		to_kind      TEXT NOT NULL,
		to_id        TEXT NOT NULL,
		attrs        TEXT NOT NULL DEFAULT '{}',
		created_at   DATETIME NOT NULL,
		updated_at   DATETIME NOT NULL
	)`,
	`CREATE UNIQUE INDEX IF NOT EXISTS idx_rel_dedup ON relations(workspace_id, from_id, to_id, kind)`,
	`CREATE INDEX IF NOT EXISTS idx_rel_from ON relations(workspace_id, from_id)`,
	`CREATE INDEX IF NOT EXISTS idx_rel_to ON relations(workspace_id, to_id)`,
}

// Store implements relationdomain.Repository over pkg/orm.
//
// Store 基于 pkg/orm 实现 relationdomain.Repository。
type Store struct {
	repo *ormpkg.Repo[relationdomain.Relation]
}

// New builds a Store bound to the relations table.
//
// New 构造绑定 relations 表的 Store。
func New(db *ormpkg.DB) *Store {
	return &Store{repo: ormpkg.For[relationdomain.Relation](db, "relations")}
}

var _ relationdomain.Repository = (*Store)(nil)

// InsertBatch inserts each edge; a dedup-index conflict means the edge already
// exists, so it is skipped — making re-sync idempotent without an upsert.
//
// InsertBatch 逐条插入；dedup 索引冲突表示边已存在，跳过——使重同步幂等而无需 upsert。
func (s *Store) InsertBatch(ctx context.Context, rels []*relationdomain.Relation) error {
	for _, rel := range rels {
		err := s.repo.Create(ctx, rel)
		if errors.Is(err, ormpkg.ErrConflict) {
			continue
		}
		if err != nil {
			return fmt.Errorf("relationstore.InsertBatch: %w", err)
		}
	}
	return nil
}

// UpdateAttrs rewrites one row's attrs. The json column is marshalled by hand —
// orm serializes json columns only on Create/Save, not on a column Update.
//
// UpdateAttrs 重写一行的 attrs。json 列手工 marshal——orm 仅在 Create/Save 序列化 json 列，
// 列 Update 不走序列化。
func (s *Store) UpdateAttrs(ctx context.Context, id string, attrs map[string]any) error {
	if attrs == nil {
		attrs = map[string]any{}
	}
	raw, err := json.Marshal(attrs)
	if err != nil {
		return fmt.Errorf("relationstore.UpdateAttrs: %w", err)
	}
	if _, err := s.repo.WhereEq("id", id).Update(ctx, "attrs", string(raw)); err != nil {
		return fmt.Errorf("relationstore.UpdateAttrs: %w", err)
	}
	return nil
}

// DeleteByIDs hard-deletes rows by id (relations has no soft-delete column).
//
// DeleteByIDs 按 id 硬删（relations 无软删列）。
func (s *Store) DeleteByIDs(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	if _, err := s.repo.WhereIn("id", toAny(ids)...).Delete(ctx); err != nil {
		return fmt.Errorf("relationstore.DeleteByIDs: %w", err)
	}
	return nil
}

// ListByFromAndKinds returns edges for (from) within kind scope; empty kinds = all.
//
// ListByFromAndKinds 返回 (from) 在 kind 范围内的边；空 kinds = 全部。
func (s *Store) ListByFromAndKinds(ctx context.Context, fromKind, fromID string, kinds []string) ([]*relationdomain.Relation, error) {
	q := s.repo.WhereEq("from_kind", fromKind).WhereEq("from_id", fromID)
	if len(kinds) > 0 {
		q = q.WhereIn("kind", toAny(kinds)...)
	}
	rows, err := q.Find(ctx)
	if err != nil {
		return nil, fmt.Errorf("relationstore.ListByFromAndKinds: %w", err)
	}
	return rows, nil
}

// ListByToAndKinds is the mirror on the (to) side.
//
// ListByToAndKinds 是 (to) 侧镜像。
func (s *Store) ListByToAndKinds(ctx context.Context, toKind, toID string, kinds []string) ([]*relationdomain.Relation, error) {
	q := s.repo.WhereEq("to_kind", toKind).WhereEq("to_id", toID)
	if len(kinds) > 0 {
		q = q.WhereIn("kind", toAny(kinds)...)
	}
	rows, err := q.Find(ctx)
	if err != nil {
		return nil, fmt.Errorf("relationstore.ListByToAndKinds: %w", err)
	}
	return rows, nil
}

// List applies the filter and keyset-paginates via orm Page; next == "" at end.
//
// List 套用 filter 并经 orm Page keyset 分页；到底时 next == ""。
func (s *Store) List(ctx context.Context, filter relationdomain.Filter, cursor string, limit int) ([]*relationdomain.Relation, string, error) {
	q := s.repo.Query()
	if filter.FromKind != "" {
		q = q.WhereEq("from_kind", filter.FromKind)
	}
	if filter.FromID != "" {
		q = q.WhereEq("from_id", filter.FromID)
	}
	if filter.ToKind != "" {
		q = q.WhereEq("to_kind", filter.ToKind)
	}
	if filter.ToID != "" {
		q = q.WhereEq("to_id", filter.ToID)
	}
	if filter.Kind != "" {
		q = q.WhereEq("kind", filter.Kind)
	}
	rows, next, err := q.Page(ctx, cursor, limit)
	if err != nil {
		return nil, "", fmt.Errorf("relationstore.List: %w", err)
	}
	return rows, next, nil
}

// ListAll returns every edge for the workspace, newest-first (relgraph snapshot).
//
// ListAll 返回该 workspace 的所有边，最新优先（relgraph 快照）。
func (s *Store) ListAll(ctx context.Context) ([]*relationdomain.Relation, error) {
	rows, err := s.repo.Order("created_at DESC, id DESC").Find(ctx)
	if err != nil {
		return nil, fmt.Errorf("relationstore.ListAll: %w", err)
	}
	return rows, nil
}

// PurgeEntity hard-deletes every edge whose from OR to is (kind,id); returns count.
//
// PurgeEntity 硬删 from 或 to 为 (kind,id) 的所有边；返回条数。
func (s *Store) PurgeEntity(ctx context.Context, kind, id string) (int64, error) {
	n, err := s.repo.
		Where("(from_kind = ? AND from_id = ?) OR (to_kind = ? AND to_id = ?)", kind, id, kind, id).
		Delete(ctx)
	if err != nil {
		return 0, fmt.Errorf("relationstore.PurgeEntity: %w", err)
	}
	return n, nil
}

// toAny widens a string slice for the variadic WhereIn.
//
// toAny 把字符串切片转为 WhereIn 变长参数所需的 []any。
func toAny(ss []string) []any {
	out := make([]any, len(ss))
	for i, s := range ss {
		out[i] = s
	}
	return out
}
