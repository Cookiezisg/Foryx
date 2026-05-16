//go:build pipeline

// Package search_test runs pipeline tests for search system tools (Grep/Glob).
//
// Package search_test 跑 search 系统工具（Grep/Glob）pipeline 测试。
package search_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	chatdomain "github.com/sunweilin/forgify/backend/internal/domain/chat"
	th "github.com/sunweilin/forgify/backend/test/harness"
)

func TestSearch_GrepFindsMatches(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"a.go":     "package main\nfunc TargetFunc() {}\n",
		"b.go":     "package main\nfunc OtherFunc() {}\n",
		"sub/c.go": "package sub\nfunc TargetFunc() {}\n",
	}
	for rel, body := range files {
		full := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatalf("seed %s: %v", rel, err)
		}
	}

	fake := th.NewFakeLLMServer(t)
	fake.PushScript(th.ScriptSingleToolCall(
		"Grep", "call_fake_grep_001",
		fmt.Sprintf(`{"summary":"finding TargetFunc","pattern":"TargetFunc","path":%q,"output_mode":"files_with_matches"}`, dir),
	))
	fake.PushScript(th.ScriptText("Found the matches."))

	h := th.New(t, th.WithFakeLLMBaseURL(fake.URL()))
	h.SeedDeepSeek(t, "fake-test-key")
	conv := h.NewConversation(t, "search-grep")
	sub := h.SubscribeSSE(t, conv.ID)

	th.PostMessage(t, h, conv.ID, "Find TargetFunc in "+dir)

	final := sub.WaitForAssistantTerminal(30 * time.Second)
	if final.Status != chatdomain.StatusCompleted {
		t.Fatalf("status=%q errCode=%q errMsg=%q\nraw:\n%s",
			final.Status, final.ErrorCode, final.ErrorMessage, sub.FormatRawEvents())
	}

	grepID, ok := th.ExtractToolCallByName(final.Blocks, "Grep")
	if !ok {
		t.Fatalf("no Grep tool_call in final blocks\nraw:\n%s", sub.FormatRawEvents())
	}
	res, ok := th.ExtractToolResultByCallID(final.Blocks, grepID)
	if !ok {
		t.Fatalf("no Grep tool_result for call %q", grepID)
	}
	if v, _ := res["ok"].(bool); !v {
		t.Errorf("Grep tool_result not ok: %v", res)
	}
	resultText, _ := res["result"].(string)
	wantPaths := []string{filepath.Join(dir, "a.go"), filepath.Join(dir, "sub", "c.go")}
	for _, p := range wantPaths {
		if !strings.Contains(resultText, p) {
			t.Errorf("Grep result missing %q:\n%s", p, resultText)
		}
	}
	if strings.Contains(resultText, filepath.Join(dir, "b.go")) {
		t.Errorf("Grep result should not list b.go (no TargetFunc):\n%s", resultText)
	}
}

func TestSearch_GlobListsDirectoryWithMetadata(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "alpha.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatalf("seed alpha: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "beta.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatalf("seed beta: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "subdir"), 0o755); err != nil {
		t.Fatalf("seed subdir: %v", err)
	}

	fake := th.NewFakeLLMServer(t)
	fake.PushScript(th.ScriptSingleToolCall(
		"Glob", "call_fake_glob_001",
		fmt.Sprintf(`{"summary":"listing dir","pattern":"*","path":%q}`, dir),
	))
	fake.PushScript(th.ScriptText("Done — listed the directory."))

	h := th.New(t, th.WithFakeLLMBaseURL(fake.URL()))
	h.SeedDeepSeek(t, "fake-test-key")
	conv := h.NewConversation(t, "search-glob")
	sub := h.SubscribeSSE(t, conv.ID)

	th.PostMessage(t, h, conv.ID, "List "+dir)

	final := sub.WaitForAssistantTerminal(30 * time.Second)
	if final.Status != chatdomain.StatusCompleted {
		t.Fatalf("status=%q\nraw:\n%s", final.Status, sub.FormatRawEvents())
	}

	globID, ok := th.ExtractToolCallByName(final.Blocks, "Glob")
	if !ok {
		t.Fatalf("no Glob tool_call in final blocks")
	}
	res, ok := th.ExtractToolResultByCallID(final.Blocks, globID)
	if !ok {
		t.Fatalf("no Glob tool_result")
	}
	if v, _ := res["ok"].(bool); !v {
		t.Errorf("Glob tool_result not ok: %v", res)
	}

	resultText, _ := res["result"].(string)
	var parsed struct {
		Root    string `json:"root"`
		Matches []struct {
			Path  string `json:"path"`
			Type  string `json:"type"`
			Size  int64  `json:"size"`
			MTime string `json:"mtime"`
		} `json:"matches"`
		Total     int  `json:"total"`
		Truncated bool `json:"truncated"`
	}
	if err := json.Unmarshal([]byte(resultText), &parsed); err != nil {
		t.Fatalf("Glob result is not parseable JSON: %v\nraw: %q", err, resultText)
	}
	if parsed.Total != 3 {
		t.Errorf("total = %d, want 3 (alpha.txt + beta.txt + subdir)", parsed.Total)
	}
	gotTypes := map[string]string{}
	for _, m := range parsed.Matches {
		gotTypes[filepath.Base(m.Path)] = m.Type
		if m.MTime == "" {
			t.Errorf("match %s missing mtime", m.Path)
		}
	}
	if gotTypes["subdir"] != "dir" {
		t.Errorf("subdir type = %q, want dir", gotTypes["subdir"])
	}
	if gotTypes["alpha.txt"] != "file" {
		t.Errorf("alpha.txt type = %q, want file", gotTypes["alpha.txt"])
	}
}

func TestSearch_GrepPathGuardDeniesSensitivePath(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home dir; PathGuard test needs ~ expansion")
	}
	denied := filepath.Join(home, ".ssh")

	fake := th.NewFakeLLMServer(t)
	fake.PushScript(th.ScriptSingleToolCall(
		"Grep", "call_fake_grep_002",
		fmt.Sprintf(`{"summary":"snooping keys","pattern":"BEGIN","path":%q}`, denied),
	))
	fake.PushScript(th.ScriptText("I cannot access that directory."))

	h := th.New(t, th.WithFakeLLMBaseURL(fake.URL()))
	h.SeedDeepSeek(t, "fake-test-key")
	conv := h.NewConversation(t, "search-pathguard")
	sub := h.SubscribeSSE(t, conv.ID)

	th.PostMessage(t, h, conv.ID, "Grep BEGIN under "+denied)

	final := sub.WaitForAssistantTerminal(30 * time.Second)
	if final.Status != chatdomain.StatusCompleted {
		t.Fatalf("status=%q\nraw:\n%s", final.Status, sub.FormatRawEvents())
	}

	grepID, ok := th.ExtractToolCallByName(final.Blocks, "Grep")
	if !ok {
		t.Fatal("no Grep tool_call in final blocks")
	}
	res, ok := th.ExtractToolResultByCallID(final.Blocks, grepID)
	if !ok {
		t.Fatal("no Grep tool_result")
	}
	resultText, _ := res["result"].(string)
	if !strings.Contains(resultText, "denied by safety guard") {
		t.Errorf("expected PathGuard denial in tool_result, got: %q", resultText)
	}
	if v, _ := res["ok"].(bool); !v {
		t.Errorf("Grep tool_result.ok = false; expected true (denial is a string). data: %v", res)
	}
}
