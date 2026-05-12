package db

import (
	"fmt"

	"gorm.io/gorm"
)

// extraGroup bundles SQL statements that all depend on the same table.
// Statements are skipped if the required table does not yet exist — they will
// be applied on the next Migrate call that includes that table.
// Every statement MUST be idempotent (CREATE … IF NOT EXISTS).
//
// extraGroup 把依赖同一张表的 SQL 语句归为一组。
// 若所需表尚不存在则跳过——下次包含该表的 Migrate 调用时自动补上。
// 每条语句必须幂等（CREATE … IF NOT EXISTS）。
type extraGroup struct {
	table string
	stmts []string
}

// schemaExtraGroups lists all SQL that AutoMigrate cannot express, grouped
// by the table they depend on. Add a new extraGroup here whenever a domain
// needs partial indexes, triggers, FTS5 virtual tables, or CHECK constraints.
//
// Driver: pure Go via modernc.org/sqlite (FTS5 included by default,
// no build flags required).
//
// schemaExtraGroups 按依赖表分组列出 AutoMigrate 表达不了的 SQL。
// 每当某 domain 需要部分索引、触发器、FTS5 虚拟表或 CHECK 约束时，在此追加一个 extraGroup。
// 驱动：modernc.org/sqlite 纯 Go 实现，FTS5 内置，无需编译标志。
//
// NOTE: FTS5 full-text search on messages was removed during the chat infra
// refactor (2026-04-27). The old index was built on messages.content which no
// longer exists. FTS5 will be re-added later targeting message_blocks.data.
//
// 注意：messages 的 FTS5 全文搜索在 chat 基础设施重构（2026-04-27）时移除。
// 旧索引基于已删除的 messages.content 列。后续将基于 message_blocks.data 重建。
var schemaExtraGroups = []extraGroup{
	{
		// functions — partial UNIQUE index so that soft-deleted functions do
		// not block re-creation of a function with the same name. A regular
		// GORM uniqueIndex would include deleted rows.
		//
		// functions — partial UNIQUE 索引,软删行不阻塞同名重建。
		// GORM 普通 uniqueIndex 会覆盖软删行,不符合需求。
		table: "functions",
		stmts: []string{
			`CREATE UNIQUE INDEX IF NOT EXISTS idx_functions_user_name_active
				ON functions(user_id, name)
				WHERE deleted_at IS NULL`,
		},
	},
	{
		// handlers — same partial UNIQUE pattern (Plan 02 trinity).
		//
		// handlers — partial UNIQUE 索引(Plan 02 trinity)。
		table: "handlers",
		stmts: []string{
			`CREATE UNIQUE INDEX IF NOT EXISTS idx_handlers_user_name_active
				ON handlers(user_id, name)
				WHERE deleted_at IS NULL`,
		},
	},
}

// applySchemaExtras runs each extraGroup whose required table already exists.
// Groups whose table is absent are silently skipped; they will be applied on
// the next Migrate call that creates the missing table.
//
// applySchemaExtras 对每个所需表已存在的 extraGroup 执行其 SQL。
// 所需表缺失的组静默跳过，待下次含该表的 Migrate 调用时补上。
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
