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
		table: "functions",
		stmts: []string{
			`CREATE UNIQUE INDEX IF NOT EXISTS idx_functions_user_name_active
				ON functions(user_id, name)
				WHERE deleted_at IS NULL`,
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
