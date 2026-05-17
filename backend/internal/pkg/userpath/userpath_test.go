package userpath

import (
	"os"
	"path/filepath"
	"testing"
)

func TestUserHome_CreatesDir(t *testing.T) {
	root := t.TempDir()
	dir, err := UserHome(root, "u_alice")
	if err != nil {
		t.Fatalf("UserHome: %v", err)
	}
	want := filepath.Join(root, "users", "u_alice")
	if dir != want {
		t.Errorf("dir = %q, want %q", dir, want)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Errorf("dir not created: %v", err)
	}
}

func TestMigrateLegacy_MovesExisting(t *testing.T) {
	root := t.TempDir()
	legacy := filepath.Join(root, "mcp.json")
	if err := os.WriteFile(legacy, []byte(`{"servers":[]}`), 0o644); err != nil {
		t.Fatalf("seed legacy: %v", err)
	}
	if err := MigrateLegacy(root, "local-user", "mcp.json"); err != nil {
		t.Fatalf("MigrateLegacy: %v", err)
	}
	target := filepath.Join(root, "users", "local-user", "mcp.json")
	if _, err := os.Stat(target); err != nil {
		t.Errorf("target not present: %v", err)
	}
	if _, err := os.Stat(legacy); err == nil {
		t.Errorf("legacy should be moved, still present")
	}
}

func TestMigrateLegacy_SkipWhenTargetExists(t *testing.T) {
	root := t.TempDir()
	legacy := filepath.Join(root, "skills")
	if err := os.MkdirAll(legacy, 0o755); err != nil {
		t.Fatalf("seed legacy: %v", err)
	}
	if err := os.WriteFile(filepath.Join(legacy, "old.md"), []byte("legacy"), 0o644); err != nil {
		t.Fatalf("seed legacy file: %v", err)
	}
	// Pre-existing target → MigrateLegacy must NOT overwrite.
	// 目标已存在 → MigrateLegacy 不可覆盖。
	target := filepath.Join(root, "users", "local-user", "skills")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatalf("seed target: %v", err)
	}
	if err := os.WriteFile(filepath.Join(target, "new.md"), []byte("new"), 0o644); err != nil {
		t.Fatalf("seed target file: %v", err)
	}
	if err := MigrateLegacy(root, "local-user", "skills"); err != nil {
		t.Fatalf("MigrateLegacy: %v", err)
	}
	// Target stays as-is.
	// Target 保持原样。
	if b, _ := os.ReadFile(filepath.Join(target, "new.md")); string(b) != "new" {
		t.Errorf("target overwritten unexpectedly")
	}
	// Legacy still exists (skip mode).
	// Legacy 仍在（skip 模式）。
	if _, err := os.Stat(legacy); err != nil {
		t.Errorf("legacy should stay when target exists: %v", err)
	}
}

func TestMigrateLegacy_NoSourceIsNoop(t *testing.T) {
	root := t.TempDir()
	if err := MigrateLegacy(root, "local-user", "missing.json"); err != nil {
		t.Errorf("MigrateLegacy noop should succeed: %v", err)
	}
}
