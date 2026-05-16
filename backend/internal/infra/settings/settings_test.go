// settings_test.go — load + hot-reload + bad-JSON resilience.
//
// settings_test.go ——加载 + 热加载 + 坏 JSON 容忍。
package settings

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"go.uber.org/zap/zaptest"

	permdomain "github.com/sunweilin/forgify/backend/internal/domain/permissions"
)

func writeSettings(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

func TestNew_MissingFile_EmptyDefaults(t *testing.T) {
	dir := t.TempDir()
	s := New(filepath.Join(dir, "settings.json"), zaptest.NewLogger(t))
	defer s.Close()
	r := s.GetRules()
	if r == nil {
		t.Fatal("GetRules returned nil")
	}
	if r.EffectiveDefaultMode() != permdomain.DefaultModeAsk {
		t.Errorf("missing file should fall back to ask, got %q", r.EffectiveDefaultMode())
	}
}

func TestNew_LoadsValidFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	writeSettings(t, path, `{
		"permissions": {
			"defaultMode": "allow",
			"deny": ["Bash(rm -rf *)"]
		}
	}`)
	s := New(path, zaptest.NewLogger(t))
	defer s.Close()
	r := s.GetRules()
	if r.EffectiveDefaultMode() != permdomain.DefaultModeAllow {
		t.Errorf("defaultMode = %q, want allow", r.EffectiveDefaultMode())
	}
	if len(r.Permissions.Deny) != 1 || r.Permissions.Deny[0] != "Bash(rm -rf *)" {
		t.Errorf("deny rules unexpected: %+v", r.Permissions.Deny)
	}
}

func TestReload_BadJSON_KeepsLastGood(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	writeSettings(t, path, `{"permissions":{"defaultMode":"allow"}}`)
	s := New(path, zaptest.NewLogger(t))
	defer s.Close()
	if s.GetRules().EffectiveDefaultMode() != permdomain.DefaultModeAllow {
		t.Fatal("initial load failed")
	}
	writeSettings(t, path, `{not json at all}`)
	err := s.Reload()
	if err == nil {
		t.Error("Reload of bad JSON should error")
	}
	if s.GetRules().EffectiveDefaultMode() != permdomain.DefaultModeAllow {
		t.Errorf("after bad-JSON reload, last good rules should persist; got %q",
			s.GetRules().EffectiveDefaultMode())
	}
}

func TestReload_InvalidSchema_KeepsLastGood(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	writeSettings(t, path, `{"permissions":{"defaultMode":"allow"}}`)
	s := New(path, zaptest.NewLogger(t))
	defer s.Close()
	writeSettings(t, path, `{"permissions":{"defaultMode":"wild-west"}}`)
	err := s.Reload()
	if err == nil {
		t.Error("Reload of invalid schema should error")
	}
	if s.GetRules().EffectiveDefaultMode() != permdomain.DefaultModeAllow {
		t.Errorf("after invalid reload, last good should persist; got %q",
			s.GetRules().EffectiveDefaultMode())
	}
}

func TestWatch_HotReloadAfterEdit(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	writeSettings(t, path, `{"permissions":{"defaultMode":"ask"}}`)
	s := New(path, zaptest.NewLogger(t))
	defer s.Close()
	s.SetDebounceWait(10 * time.Millisecond)
	s.SetPollInterval(50 * time.Millisecond)
	if err := s.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Mutate the file; expect snapshot to update within ~500ms.
	// 改文件；预期 ~500ms 内快照更新。
	time.Sleep(20 * time.Millisecond) // settle watcher
	writeSettings(t, path, `{"permissions":{"defaultMode":"allow"}}`)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if s.GetRules().EffectiveDefaultMode() == permdomain.DefaultModeAllow {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Errorf("hot reload did not pick up file change within 2s; current = %q",
		s.GetRules().EffectiveDefaultMode())
}
