// Package agent is the orm-backed implementation of agentdomain.Repository: agents
// (soft-deleted) + agent_versions (append-only, cap-trimmed, immutable config snapshots) +
// agent_executions (append-only log). Workspace isolation is automatic (orm fills/filters
// workspace_id from ctx via the ,ws tag), so no method hand-writes a workspace predicate.
//
// Package agent 是 agentdomain.Repository 的 orm 实现：agents（软删）+ agent_versions（只增、
// 按上限裁剪、不可变配置快照）+ agent_executions（只增 log）。workspace 隔离自动（orm 据 ctx 经
// ,ws tag 填/过滤 workspace_id），故无方法手写 workspace 谓词。
package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	agentdomain "github.com/sunweilin/forgify/backend/internal/domain/agent"
	ormpkg "github.com/sunweilin/forgify/backend/internal/pkg/orm"
)

// Schema is the agent tables' DDL, exported as ordered idempotent statements for bootstrap to
// collect via db.Migrate. agents has a partial-UNIQUE name (freed on soft-delete); versions are
// UNIQUE(agent_id, version) and carry created_at/updated_at like every other entity's version
// table (ORM ,updated tag); executions are an append-only log (no deleted_at — D1) with
// CHECK-constrained status / triggered_by.
//
// Schema 是 agent 三表 DDL，按序幂等导出。agents 用 partial-UNIQUE name（软删后释放）；versions
// UNIQUE(agent_id, version) 且与其他实体的 version 表一样带 created_at/updated_at（ORM ,updated 标签）；
// executions 是只增 log（无 deleted_at——D1），
// status / triggered_by 带 CHECK。
var Schema = []string{
	`CREATE TABLE IF NOT EXISTS agents (
		id                TEXT PRIMARY KEY,
		workspace_id      TEXT NOT NULL,
		name              TEXT NOT NULL,
		description       TEXT NOT NULL DEFAULT '',
		tags              TEXT NOT NULL DEFAULT '[]',
		active_version_id TEXT NOT NULL DEFAULT '',
		created_at        DATETIME NOT NULL,
		updated_at        DATETIME NOT NULL,
		deleted_at        DATETIME
	)`,
	`CREATE UNIQUE INDEX IF NOT EXISTS idx_agents_ws_name ON agents(workspace_id, name) WHERE deleted_at IS NULL`,
	`CREATE INDEX IF NOT EXISTS idx_agents_ws_created ON agents(workspace_id, created_at DESC, id DESC) WHERE deleted_at IS NULL`,

	`CREATE TABLE IF NOT EXISTS agent_versions (
		id                        TEXT PRIMARY KEY,
		workspace_id              TEXT NOT NULL,
		agent_id                  TEXT NOT NULL,
		version                   INTEGER NOT NULL,
		prompt                    TEXT NOT NULL DEFAULT '',
		skill                     TEXT NOT NULL DEFAULT '',
		knowledge                 TEXT NOT NULL DEFAULT '[]',
		tools                     TEXT NOT NULL DEFAULT '[]',
		inputs                    TEXT NOT NULL DEFAULT '[]',
		outputs                   TEXT NOT NULL DEFAULT '[]',
		model_override            TEXT NOT NULL DEFAULT 'null',
		change_reason             TEXT NOT NULL DEFAULT '',
		forged_in_conversation_id TEXT NOT NULL DEFAULT '',
		created_at                DATETIME NOT NULL,
		updated_at                DATETIME NOT NULL
	)`,
	`CREATE UNIQUE INDEX IF NOT EXISTS idx_agv_agent_version ON agent_versions(agent_id, version)`,
	`CREATE INDEX IF NOT EXISTS idx_agv_agent_created ON agent_versions(agent_id, created_at DESC, id DESC)`,

	`CREATE TABLE IF NOT EXISTS agent_executions (
		id              TEXT PRIMARY KEY,
		workspace_id    TEXT NOT NULL,
		agent_id        TEXT NOT NULL,
		version_id      TEXT NOT NULL,
		model_id        TEXT NOT NULL DEFAULT '',
		status          TEXT NOT NULL CHECK (status IN ('ok','failed','cancelled','timeout')),
		triggered_by    TEXT NOT NULL CHECK (triggered_by IN ('chat','workflow','manual')),
		input           TEXT NOT NULL DEFAULT '{}',
		output          TEXT,
		transcript      TEXT NOT NULL DEFAULT '[]',
		error_message   TEXT NOT NULL DEFAULT '',
		elapsed_ms      INTEGER NOT NULL DEFAULT 0,
		started_at      DATETIME NOT NULL,
		ended_at        DATETIME NOT NULL,
		conversation_id TEXT NOT NULL DEFAULT '',
		message_id      TEXT NOT NULL DEFAULT '',
		tool_call_id    TEXT NOT NULL DEFAULT '',
		flowrun_id      TEXT NOT NULL DEFAULT '',
		flowrun_node_id TEXT NOT NULL DEFAULT '',
		created_at      DATETIME NOT NULL
	)`,
	`CREATE INDEX IF NOT EXISTS idx_agx_ws_agent ON agent_executions(workspace_id, agent_id, created_at DESC, id DESC)`,
	`CREATE INDEX IF NOT EXISTS idx_agx_ws_conversation ON agent_executions(workspace_id, conversation_id) WHERE conversation_id != ''`,
	`CREATE INDEX IF NOT EXISTS idx_agx_ws_flowrun ON agent_executions(workspace_id, flowrun_id) WHERE flowrun_id != ''`,
}

// Store implements agentdomain.Repository over pkg/orm.
//
// Store 基于 pkg/orm 实现 agentdomain.Repository。
type Store struct {
	db     *ormpkg.DB
	agents *ormpkg.Repo[agentdomain.Agent]
	vers   *ormpkg.Repo[agentdomain.Version]
	execs  *ormpkg.Repo[agentdomain.Execution]
}

// New constructs a Store bound to the three agent tables.
//
// New 构造绑定 agent 三表的 Store。
func New(db *ormpkg.DB) *Store {
	return &Store{
		db:     db,
		agents: ormpkg.For[agentdomain.Agent](db, "agents"),
		vers:   ormpkg.For[agentdomain.Version](db, "agent_versions"),
		execs:  ormpkg.For[agentdomain.Execution](db, "agent_executions"),
	}
}

var _ agentdomain.Repository = (*Store)(nil)

// --- agents ----------------------------------------------------------------

func (s *Store) Create(ctx context.Context, a *agentdomain.Agent) error {
	if err := s.agents.Save(ctx, a); err != nil {
		if errors.Is(err, ormpkg.ErrConflict) {
			return agentdomain.ErrNameConflict
		}
		return fmt.Errorf("agentstore.Create: %w", err)
	}
	return nil
}

func (s *Store) Get(ctx context.Context, id string) (*agentdomain.Agent, error) {
	a, err := s.agents.Get(ctx, id)
	if errors.Is(err, ormpkg.ErrNotFound) {
		return nil, agentdomain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("agentstore.Get: %w", err)
	}
	return a, nil
}

func (s *Store) GetByName(ctx context.Context, name string) (*agentdomain.Agent, error) {
	a, err := s.agents.WhereEq("name", name).First(ctx)
	if errors.Is(err, ormpkg.ErrNotFound) {
		return nil, agentdomain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("agentstore.GetByName: %w", err)
	}
	return a, nil
}

func (s *Store) ListAgents(ctx context.Context, filter agentdomain.ListFilter) ([]*agentdomain.Agent, string, error) {
	rows, next, err := s.agents.Query().Page(ctx, filter.Cursor, filter.Limit)
	if err != nil {
		return nil, "", fmt.Errorf("agentstore.ListAgents: %w", err)
	}
	return rows, next, nil
}

func (s *Store) ListAll(ctx context.Context) ([]*agentdomain.Agent, error) {
	rows, err := s.agents.Order("created_at DESC, id DESC").Find(ctx)
	if err != nil {
		return nil, fmt.Errorf("agentstore.ListAll: %w", err)
	}
	return rows, nil
}

// UpdateMeta updates name/description/tags only (no version bump). tags is JSON-marshalled here
// because Updates passes raw values straight to the driver (orm only serialises ,json fields on
// Create/Save).
//
// UpdateMeta 仅更新 name/description/tags（不升版本）。tags 在此手工 JSON 序列化（Updates 直送 driver）。
func (s *Store) UpdateMeta(ctx context.Context, a *agentdomain.Agent) error {
	tagsJSON, err := json.Marshal(a.Tags)
	if err != nil {
		return fmt.Errorf("agentstore.UpdateMeta: marshal tags: %w", err)
	}
	n, err := s.agents.WhereEq("id", a.ID).Updates(ctx, map[string]any{
		"name":        a.Name,
		"description": a.Description,
		"tags":        string(tagsJSON),
	})
	if err != nil {
		if errors.Is(err, ormpkg.ErrConflict) {
			return agentdomain.ErrNameConflict
		}
		return fmt.Errorf("agentstore.UpdateMeta: %w", err)
	}
	if n == 0 {
		return agentdomain.ErrNotFound
	}
	return nil
}

func (s *Store) SetActiveVersion(ctx context.Context, agentID, versionID string) error {
	n, err := s.agents.WhereEq("id", agentID).Update(ctx, "active_version_id", versionID)
	if err != nil {
		return fmt.Errorf("agentstore.SetActiveVersion: %w", err)
	}
	if n == 0 {
		return agentdomain.ErrNotFound
	}
	return nil
}

func (s *Store) SoftDelete(ctx context.Context, id string) error {
	ok, err := s.agents.Delete(ctx, id) // soft-delete (agents has deleted_at)
	if err != nil {
		return fmt.Errorf("agentstore.SoftDelete: %w", err)
	}
	if !ok {
		return agentdomain.ErrNotFound
	}
	return nil
}

// --- versions --------------------------------------------------------------

func (s *Store) CreateVersion(ctx context.Context, v *agentdomain.Version) error {
	if err := s.vers.Create(ctx, v); err != nil {
		return fmt.Errorf("agentstore.CreateVersion: %w", err)
	}
	return nil
}

func (s *Store) GetVersion(ctx context.Context, versionID string) (*agentdomain.Version, error) {
	v, err := s.vers.Get(ctx, versionID)
	if errors.Is(err, ormpkg.ErrNotFound) {
		return nil, agentdomain.ErrVersionNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("agentstore.GetVersion: %w", err)
	}
	return v, nil
}

func (s *Store) GetVersionByNumber(ctx context.Context, agentID string, version int) (*agentdomain.Version, error) {
	v, err := s.vers.WhereEq("agent_id", agentID).WhereEq("version", version).First(ctx)
	if errors.Is(err, ormpkg.ErrNotFound) {
		return nil, agentdomain.ErrVersionNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("agentstore.GetVersionByNumber: %w", err)
	}
	return v, nil
}

func (s *Store) ListVersions(ctx context.Context, agentID string, filter agentdomain.VersionListFilter) ([]*agentdomain.Version, string, error) {
	rows, next, err := s.vers.WhereEq("agent_id", agentID).Page(ctx, filter.Cursor, filter.Limit)
	if err != nil {
		return nil, "", fmt.Errorf("agentstore.ListVersions: %w", err)
	}
	return rows, next, nil
}

// NextVersionNumber returns max(version)+1 for the agent (1 for the first version).
//
// NextVersionNumber 返该 agent 的 max(version)+1（首版为 1）。
func (s *Store) NextVersionNumber(ctx context.Context, agentID string) (int, error) {
	var nums []int
	if err := s.vers.WhereEq("agent_id", agentID).Order("version DESC").Limit(1).Pluck(ctx, "version", &nums); err != nil {
		return 0, fmt.Errorf("agentstore.NextVersionNumber: %w", err)
	}
	if len(nums) == 0 {
		return 1, nil
	}
	return nums[0] + 1, nil
}

// TrimVersions hard-deletes versions below the keep-th newest, always sparing the active
// version (which may be old after a revert).
//
// TrimVersions 硬删低于第 keep 新的版本，始终放过 active 版本（revert 后它可能很老）。
func (s *Store) TrimVersions(ctx context.Context, agentID string, keep int) error {
	if keep <= 0 {
		keep = agentdomain.AcceptedVersionCap
	}
	var nums []int
	if err := s.vers.WhereEq("agent_id", agentID).Order("version DESC").Pluck(ctx, "version", &nums); err != nil {
		return fmt.Errorf("agentstore.TrimVersions: %w", err)
	}
	if len(nums) <= keep {
		return nil
	}
	cutoff := nums[keep-1] // keep versions with number >= cutoff
	a, err := s.agents.Get(ctx, agentID)
	if err != nil {
		return fmt.Errorf("agentstore.TrimVersions: load active: %w", err)
	}
	if _, err := s.vers.
		WhereEq("agent_id", agentID).
		Where("version < ?", cutoff).
		Where("id != ?", a.ActiveVersionID).
		Delete(ctx); err != nil { // hard-delete: agent_versions has no deleted_at
		return fmt.Errorf("agentstore.TrimVersions: %w", err)
	}
	return nil
}

// GetByIDs batch-loads agents by id (relation Namer hydration). Missing ids are skipped.
//
// GetByIDs 按 id 批量取 agent（relation Namer hydrate 用）。缺失 id 跳过。
func (s *Store) GetByIDs(ctx context.Context, ids []string) ([]*agentdomain.Agent, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	vals := make([]any, len(ids))
	for i, id := range ids {
		vals[i] = id
	}
	rows, err := s.agents.WhereIn("id", vals...).Find(ctx)
	if err != nil {
		return nil, fmt.Errorf("agentstore.GetByIDs: %w", err)
	}
	return rows, nil
}

// CreateWithVersion inserts the entity row and its v1 in ONE transaction: a
// create either fully lands or fully doesn't — no versionless entity row on a mid-write failure.
//
// CreateWithVersion 在单事务内插入实体行与其 v1：create 要么完整落地、要么完全不落
// ——中途失败不留无版本实体行。
func (s *Store) CreateWithVersion(ctx context.Context, e *agentdomain.Agent, v *agentdomain.Version) error {
	return s.db.Transaction(ctx, func(tx *ormpkg.DB) error {
		if err := ormpkg.For[agentdomain.Agent](tx, "agents").Create(ctx, e); err != nil {
			if errors.Is(err, ormpkg.ErrConflict) {
				return agentdomain.ErrNameConflict
			}
			return fmt.Errorf("agentstore.CreateWithVersion: entity: %w", err)
		}
		if err := ormpkg.For[agentdomain.Version](tx, "agent_versions").Create(ctx, v); err != nil {
			return fmt.Errorf("agentstore.CreateWithVersion: version: %w", err)
		}
		return nil
	})
}

// SaveVersionAndActivate inserts a new version and moves the active pointer in ONE transaction:
// an edit either fully lands or fully doesn't — no orphan version + stale pointer.
//
// SaveVersionAndActivate 在单事务内插入新版本并移动 active 指针：edit 要么完整生效、
// 要么完全不生效——不留孤儿版本 + 旧指针。
func (s *Store) SaveVersionAndActivate(ctx context.Context, v *agentdomain.Version, entityID string) error {
	return s.db.Transaction(ctx, func(tx *ormpkg.DB) error {
		if err := ormpkg.For[agentdomain.Version](tx, "agent_versions").Create(ctx, v); err != nil {
			return fmt.Errorf("agentstore.SaveVersionAndActivate: version: %w", err)
		}
		n, err := ormpkg.For[agentdomain.Agent](tx, "agents").
			WhereEq("id", entityID).Update(ctx, "active_version_id", v.ID)
		if err != nil {
			return fmt.Errorf("agentstore.SaveVersionAndActivate: pointer: %w", err)
		}
		if n == 0 {
			return agentdomain.ErrNotFound
		}
		return nil
	})
}
