package settings

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	limitspkg "github.com/sunweilin/forgify/backend/internal/pkg/limits"
)

// TestLoad_AbsentFileIsDefaults: first boot has no settings.json — pure defaults, no file created.
//
// TestLoad_AbsentFileIsDefaults：首启无 settings.json——纯默认、不建文件。
func TestLoad_AbsentFileIsDefaults(t *testing.T) {
	defer limitspkg.SetProvider(limitspkg.Default)
	dir := t.TempDir()
	s, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if s.Limits() != limitspkg.Default() {
		t.Fatalf("absent file must mean defaults: %+v", s.Limits())
	}
	if _, err := os.Stat(filepath.Join(dir, "settings.json")); !os.IsNotExist(err) {
		t.Fatal("Load must not create the file")
	}
}

// TestPatch_PersistsAndHotSwaps: a patch survives reload and limits.Current() sees it
// immediately (the hot-swap consumers rely on).
//
// TestPatch_PersistsAndHotSwaps：patch 经得起重载，limits.Current() 立即可见（消费方依赖的热换）。
func TestPatch_PersistsAndHotSwaps(t *testing.T) {
	defer limitspkg.SetProvider(limitspkg.Default)
	dir := t.TempDir()
	s, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	got, err := s.PatchLimits(json.RawMessage(`{"agent":{"maxSteps":40},"timeout":{"mcpCallSec":300}}`))
	if err != nil {
		t.Fatalf("PatchLimits: %v", err)
	}
	if got.Agent.MaxSteps != 40 || got.Timeout.MCPCallSec != 300 || got.Agent.InvokeMaxTurns != 10 {
		t.Fatalf("merge wrong: %+v", got)
	}
	if limitspkg.Current().Agent.MaxSteps != 40 {
		t.Fatal("hot-swap did not land in limits.Current()")
	}
	// reload from disk sees the same values
	s2, err := Load(dir)
	if err != nil {
		t.Fatalf("re-Load: %v", err)
	}
	if s2.Limits().Agent.MaxSteps != 40 || s2.Limits().Timeout.MCPCallSec != 300 {
		t.Fatalf("persisted values lost: %+v", s2.Limits())
	}
}

// TestPatch_RejectsOutOfRange: negative ceilings and out-of-(0,1) ratio reject without
// touching the live values or the file.
//
// TestPatch_RejectsOutOfRange：负上限与 (0,1) 外 ratio 被拒，活动值与文件不动。
func TestPatch_RejectsOutOfRange(t *testing.T) {
	defer limitspkg.SetProvider(limitspkg.Default)
	dir := t.TempDir()
	s, _ := Load(dir)
	for _, patch := range []string{
		`{"agent":{"maxSteps":-1}}`,
		`{"context":{"triggerRatio":1.5}}`,
		`{"agent":{"maxSteps":`, // malformed JSON
	} {
		if _, err := s.PatchLimits(json.RawMessage(patch)); !errors.Is(err, ErrLimitsInvalid) {
			t.Fatalf("patch %q: want ErrLimitsInvalid, got %v", patch, err)
		}
	}
	if limitspkg.Current() != limitspkg.Default() {
		t.Fatal("rejected patch leaked into live values")
	}
}

// TestLoad_MalformedFileFails: a hand-edited broken file must fail boot loudly, not be
// silently ignored.
//
// TestLoad_MalformedFileFails：手编坏文件必须把 boot 喊停，不得静默忽略。
func TestLoad_MalformedFileFails(t *testing.T) {
	defer limitspkg.SetProvider(limitspkg.Default)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(dir); err == nil || !strings.Contains(err.Error(), "parse") {
		t.Fatalf("want parse error, got %v", err)
	}
}
