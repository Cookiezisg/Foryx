// Package db is the generic SQLite gateway: connection, migration, and escape-hatch SQL.
//
// Package db 是通用 SQLite 网关：连接、迁移与 AutoMigrate 兜不住的 SQL。
package db

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// Config opens the DB; zero value = in-memory + quiet logger (test default).
//
// Config 打开 DB 的配置；零值为内存 DB + 静音 logger（测试默认）。
type Config struct {
	DataDir  string
	LogLevel gormlogger.LogLevel
}

// Open returns a SQLite *gorm.DB with WAL, foreign_keys and prepared-stmt caching enabled.
//
// Open 返回启用 WAL、FK、prepared-stmt 缓存的 SQLite *gorm.DB。
func Open(cfg Config) (*gorm.DB, error) {
	dsn, err := buildDSN(cfg.DataDir)
	if err != nil {
		return nil, err
	}

	logLevel := cfg.LogLevel
	if logLevel == 0 {
		logLevel = gormlogger.Warn
	}

	// Lookup-or-default patterns (middleware user resolver, optional model lookups)
	// intentionally probe by id and tolerate ErrRecordNotFound. The default GORM
	// logger surfaces those as Warnings → dev-log noise. Suppress just that one
	// category; everything else stays at the configured LogLevel.
	//
	// 查不到-就-默认 模式（middleware user resolver、可选 model 查询）按 id 探
	// 查，容忍 ErrRecordNotFound；默认 logger 把它当 Warning 打 → dev log 噪
	// 音。只静默这一类，其它走原 LogLevel。
	gormLog := gormlogger.New(
		log.New(os.Stdout, "\r\n", log.LstdFlags),
		gormlogger.Config{
			SlowThreshold:             200 * time.Millisecond,
			LogLevel:                  logLevel,
			IgnoreRecordNotFoundError: true,
			Colorful:                  false,
		},
	)

	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		NowFunc:     func() time.Time { return time.Now().UTC() },
		Logger:      gormLog,
		PrepareStmt: true,
	})
	if err != nil {
		return nil, fmt.Errorf("gorm open: %w", err)
	}

	// :memory: gives each sql conn its own empty DB — pin pool to 1 for shared state.
	// Pin to one connection for ALL database types:
	// - :memory: needs it for data isolation (each sql conn has its own empty DB).
	// - file DB needs it to prevent SQLITE_BUSY: WAL mode allows only one writer
	//   at a time. With multiple GORM connections, concurrent goroutines that open
	//   deferred write transactions race to upgrade the lock; the loser gets
	//   SQLITE_BUSY even after busy_timeout if the winner holds the lock too long
	//   (e.g. AppendEvent's SELECT+INSERT inside a transaction). A single connection
	//   serializes all writes at the Go level before they reach SQLite, eliminating
	//   the lock-upgrade race entirely. SQLite is single-writer regardless; the
	//   extra connections provided no concurrency benefit and only caused contention.
	//
	// 所有 DB 类型都锁 1 连接:
	// :memory: 需要隔离(每条 sql conn 独立 DB);file DB 需要防 SQLITE_BUSY——
	// WAL 单写者、多连接竞升锁导致 busy_timeout 后仍失败。
	sqlDB, err := db.DB()
	if err != nil {
		_ = Close(db)
		return nil, fmt.Errorf("gorm.DB(): %w", err)
	}
	sqlDB.SetMaxOpenConns(1)

	if err := verifyPragmas(db); err != nil {
		_ = Close(db)
		return nil, err
	}

	return db, nil
}

// Close releases the connection pool; nil-safe.
//
// Close 释放连接池；对 nil 安全。
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
