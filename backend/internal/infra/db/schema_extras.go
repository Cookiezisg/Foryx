package db

import (
	"fmt"

	"gorm.io/gorm"
)

// extraGroup bundles idempotent SQL statements depending on one table.
//
// extraGroup 把依赖同一张表的幂等 SQL 语句归为一组。
type extraGroup struct {
	table string
	stmts []string
}

var schemaExtraGroups = []extraGroup{
	{
		table: "api_keys",
		stmts: []string{
			// Empty displayName allowed to duplicate (server-defaulted); non-empty
			// must be unique per user. Soft-deleted rows ignored.
			//
			// 空 displayName 允许重复(服务端默认值);非空必 per-user 唯一。软删除行不参与。
			`CREATE UNIQUE INDEX IF NOT EXISTS idx_api_keys_user_displayname_active
				ON api_keys(user_id, display_name)
				WHERE deleted_at IS NULL AND display_name != ''`,
		},
	},
	{
		table: "functions",
		stmts: []string{
			`CREATE UNIQUE INDEX IF NOT EXISTS idx_functions_user_name_active
				ON functions(user_id, name)
				WHERE deleted_at IS NULL`,
		},
	},
	{
		table: "flowrun_events",
		stmts: []string{
			// record-once (ADR-018): result/waiting/agent-substep/control events are
			// deduped on dedup_key; attempt-class (node_started/node_failed) carry
			// dedup_key='' and are excluded here so they append freely (retry trail).
			//
			// record-once:结果/等待/agent 子步/控制事件按 dedup_key 去重;
			// attempt 类(node_started/node_failed)dedup_key='' 被此排除,自由 append。
			`CREATE UNIQUE INDEX IF NOT EXISTS idx_fre_record_once
				ON flowrun_events(flowrun_id, dedup_key)
				WHERE type NOT IN ('node_started','node_failed')`,
		},
	},
	{
		table: "handlers",
		stmts: []string{
			`CREATE UNIQUE INDEX IF NOT EXISTS idx_handlers_user_name_active
				ON handlers(user_id, name)
				WHERE deleted_at IS NULL`,
		},
	},
	{
		table: "workflows",
		stmts: []string{
			`CREATE UNIQUE INDEX IF NOT EXISTS idx_workflows_user_name_active
				ON workflows(user_id, name)
				WHERE deleted_at IS NULL`,
		},
	},
	{
		table: "memories",
		stmts: []string{
			`CREATE UNIQUE INDEX IF NOT EXISTS idx_memories_name_active
				ON memories(name)
				WHERE deleted_at IS NULL`,
			`CREATE INDEX IF NOT EXISTS idx_memories_type_pinned
				ON memories(type, pinned)`,
			`CREATE INDEX IF NOT EXISTS idx_memories_accessed
				ON memories(accessed_at DESC, access_count DESC)`,
		},
	},
	{
		table: "documents",
		stmts: []string{
			// COALESCE(parent_id, '') so two roots with the same name still collide
			// (SQLite treats NULL != NULL in plain UNIQUE indexes).
			//
			// COALESCE(parent_id, '') 让根级两个同名也撞 UNIQUE
			// (SQLite 普通 UNIQUE 视 NULL != NULL,不加这层保护根级会漏)。
			`CREATE UNIQUE INDEX IF NOT EXISTS idx_documents_parent_name_active
				ON documents(user_id, COALESCE(parent_id, ''), name)
				WHERE deleted_at IS NULL`,
		},
	},
	{
		table: "relations",
		stmts: []string{
			// Self-loop forbidden — GORM tag can't express tuple comparison cross-field;
			// trigger is the SQLite-compatible way to enforce.
			//
			// 禁止自环——GORM tag 无法表达跨字段 tuple 比较；SQLite 兼容做法走 trigger。
			`CREATE TRIGGER IF NOT EXISTS trg_relations_no_self_loop
				BEFORE INSERT ON relations
				WHEN NEW.from_kind = NEW.to_kind AND NEW.from_id = NEW.to_id
			BEGIN
				SELECT RAISE(ABORT, 'relations: self-loop forbidden');
			END`,
		},
	},
}

func applySchemaExtras(db *gorm.DB) error {
	for _, g := range schemaExtraGroups {
		if !db.Migrator().HasTable(g.table) {
			continue
		}
		if err := db.Transaction(func(tx *gorm.DB) error {
			for i, stmt := range g.stmts {
				if err := tx.Exec(stmt).Error; err != nil {
					return fmt.Errorf("schema extras group %q #%d: %w", g.table, i, err)
				}
			}
			return nil
		}); err != nil {
			return err
		}
	}
	return nil
}
