// Package db is the generic relational-database gateway: connection setup,
// schema application (Migrate), and escape-hatch SQL for features
// AutoMigrate can't express (FTS5, triggers, complex CHECKs).
//
// This package is domain-agnostic. Table-specific repositories live
// under internal/infra/store/<domain>/ and consume *gorm.DB from here.
//
// Package db 是通用的关系数据库网关：连接建立、schema 应用（Migrate）、
// 以及 AutoMigrate 表达不了的 SQL（FTS5、触发器、复杂 CHECK）。
//
// 本包与 domain 无关。表相关的 Repository 位于 internal/infra/store/<domain>/，
// 从本包消费 *gorm.DB。
package db

import (
	"fmt"
	"os"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// Config controls how the database is opened. Zero values are test-safe
// defaults (in-memory, quiet logger).
//
// Config 控制数据库的打开方式。零值为测试安全默认（内存 DB、静音 logger）。
type Config struct {
	// DataDir holds forgify.db. Empty = in-memory (for tests).
	//
	// DataDir 存放 forgify.db。空 = 内存数据库（测试用）。
	DataDir string

	// LogLevel controls GORM's internal SQL logger.
	//
	// LogLevel 控制 GORM 内部 SQL 日志。
	LogLevel gormlogger.LogLevel
}

// Open establishes a SQLite connection with WAL, FK, and prepared
// statement caching all enabled.
//
// Open 打开 SQLite 连接，启用 WAL、FK、prepared statement 缓存。
func Open(cfg Config) (*gorm.DB, error) {
	dsn, err := buildDSN(cfg.DataDir)
	if err != nil {
		return nil, err
	}

	logLevel := cfg.LogLevel
	if logLevel == 0 {
		logLevel = gormlogger.Warn
	}

	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		// UTC at rest; convert at the transport boundary.
		// 存 UTC，传输边界再转换。
		NowFunc:     func() time.Time { return time.Now().UTC() },
		Logger:      gormlogger.Default.LogMode(logLevel),
		PrepareStmt: true,
	})
	if err != nil {
		return nil, fmt.Errorf("gorm open: %w", err)
	}

	if err := verifyPragmas(db); err != nil {
		_ = Close(db)
		return nil, err
	}

	return db, nil
}

// Close releases the connection pool. Safe on nil.
//
// Close 释放连接池。对 nil 调用安全。
func Close(db *gorm.DB) error {
	if db == nil {
		return nil
	}
	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("gorm close: get underlying sql.DB: %w", err)
	}
	return sqlDB.Close()
}

func buildDSN(dataDir string) (string, error) {
	params := "_pragma=journal_mode(WAL)" +
		"&_pragma=busy_timeout(5000)" +
		"&_pragma=foreign_keys(on)" +
		"&_pragma=synchronous(NORMAL)"

	if dataDir == "" {
		return ":memory:?" + params, nil
	}
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return "", fmt.Errorf("mkdir %s: %w", dataDir, err)
	}
	return fmt.Sprintf("file:%s/forgify.db?%s", dataDir, params), nil
}

// verifyPragmas double-checks critical PRAGMAs took effect. Belt-and-suspenders
// for safety-critical settings like foreign_keys.
//
// verifyPragmas 二次确认关键 PRAGMA 生效。对 foreign_keys 这类安全关键项做双保险。
func verifyPragmas(db *gorm.DB) error {
	var fk int
	if err := db.Raw("PRAGMA foreign_keys").Scan(&fk).Error; err != nil {
		return fmt.Errorf("query foreign_keys pragma: %w", err)
	}
	if fk != 1 {
		return fmt.Errorf("foreign_keys pragma is %d, expected 1", fk)
	}
	return nil
}
