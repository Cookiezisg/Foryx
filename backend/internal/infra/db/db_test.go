package db

import (
	"os"
	"path/filepath"
	"testing"

	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := Open(Config{LogLevel: gormlogger.Silent})
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { _ = Close(db) })
	return db
}

func TestOpen_InMemoryDB(t *testing.T) {
	db := newTestDB(t)

	var got int
	if err := db.Raw("SELECT 1").Scan(&got).Error; err != nil {
		t.Fatalf("SELECT 1: %v", err)
	}
	if got != 1 {
		t.Errorf("got %d, want 1", got)
	}
}

func TestOpen_FileDB(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(Config{DataDir: dir, LogLevel: gormlogger.Silent})
	if err != nil {
		t.Fatalf("open file db: %v", err)
	}
	t.Cleanup(func() { _ = Close(db) })

	dbFile := filepath.Join(dir, "forgify.db")
	if _, err := os.Stat(dbFile); err != nil {
		t.Errorf("forgify.db not created: %v", err)
	}
}

func TestOpen_ForeignKeysEnabled(t *testing.T) {
	db := newTestDB(t)

	var fk int
	if err := db.Raw("PRAGMA foreign_keys").Scan(&fk).Error; err != nil {
		t.Fatalf("query pragma: %v", err)
	}
	if fk != 1 {
		t.Errorf("foreign_keys = %d, want 1", fk)
	}
}

func TestOpen_WALEnabled(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(Config{DataDir: dir, LogLevel: gormlogger.Silent})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = Close(db) })

	var mode string
	if err := db.Raw("PRAGMA journal_mode").Scan(&mode).Error; err != nil {
		t.Fatalf("query pragma: %v", err)
	}
	if mode != "wal" {
		t.Errorf("journal_mode = %q, want \"wal\"", mode)
	}
}

func TestOpen_InvalidDataDir(t *testing.T) {
	tmpfile, err := os.CreateTemp("", "notadir-*")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	defer os.Remove(tmpfile.Name())
	_ = tmpfile.Close()

	if _, err := Open(Config{DataDir: tmpfile.Name(), LogLevel: gormlogger.Silent}); err == nil {
		t.Errorf("expected error opening DB in path that's a file, got nil")
	}
}

func TestClose_NilSafe(t *testing.T) {
	if err := Close(nil); err != nil {
		t.Errorf("Close(nil) returned error: %v", err)
	}
}

type dummyModel struct {
	ID   string `gorm:"primaryKey;type:text"`
	Name string `gorm:"not null"`
}

func (dummyModel) TableName() string { return "dummy_models" }

func TestMigrate_CreatesTable(t *testing.T) {
	db := newTestDB(t)
	if err := Migrate(db, &dummyModel{}); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	if !db.Migrator().HasTable(&dummyModel{}) {
		t.Errorf("table dummy_models was not created")
	}
}

func TestMigrate_Idempotent(t *testing.T) {
	db := newTestDB(t)
	if err := Migrate(db, &dummyModel{}); err != nil {
		t.Fatalf("first migrate: %v", err)
	}
	if err := Migrate(db, &dummyModel{}); err != nil {
		t.Fatalf("second migrate (should be idempotent): %v", err)
	}
}

func TestMigrate_MultipleModels(t *testing.T) {
	type other struct {
		ID string `gorm:"primaryKey;type:text"`
	}
	db := newTestDB(t)
	if err := Migrate(db, &dummyModel{}, &other{}); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if !db.Migrator().HasTable(&dummyModel{}) {
		t.Errorf("dummy_models missing")
	}
	if !db.Migrator().HasTable(&other{}) {
		t.Errorf("others missing")
	}
}

func TestMigrate_NilDB(t *testing.T) {
	if err := Migrate(nil, &dummyModel{}); err == nil {
		t.Errorf("Migrate(nil, ...) should fail, got nil")
	}
}

func TestMigrate_EmptyModelsRuns(t *testing.T) {
	db := newTestDB(t)
	if err := Migrate(db); err != nil {
		t.Errorf("Migrate(db) with no models returned: %v", err)
	}
}
