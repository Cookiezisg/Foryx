//go:build pipeline

// Package filesystem_test runs pipeline tests for file-ops tools (Read/Write/Edit).
//
// Package filesystem_test 跑 file-ops 工具（Read/Write/Edit）pipeline 测试。
package filesystem_test

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

func TestFileOps_ReadEditClosedLoop(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "src.txt")
	if err := os.WriteFile(target, []byte("alpha line\n"), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}

	fake := th.NewFakeLLMServer(t)
	fake.PushScript(th.ScriptSingleToolCall(
		"Read", "call_fake_read_001",
		fmt.Sprintf(`{"summary":"reading src","file_path":%q}`, target),
	))
	fake.PushScript(th.ScriptSingleToolCall(
		"Edit", "call_fake_edit_001",
		fmt.Sprintf(`{"summary":"renaming alpha","file_path":%q,"old_string":"alpha","new_string":"beta"}`, target),
	))
	fake.PushScript(th.ScriptText("Done — replaced alpha with beta."))

	h := th.New(t, th.WithFakeLLMBaseURL(fake.URL()))
	h.SeedDeepSeek(t, "fake-test-key")
	conv := h.NewConversation(t, "file-ops")
	sub := h.SubscribeSSE(t, conv.ID)

	th.PostMessage(t, h, conv.ID, "Read "+target+" and rename alpha to beta.")

	final := sub.WaitForAssistantTerminal(30 * time.Second)
	if final.Status != chatdomain.StatusCompleted {
		t.Fatalf("status=%q errCode=%q errMsg=%q\nraw:\n%s",
			final.Status, final.ErrorCode, final.ErrorMessage, sub.FormatRawEvents())
	}

	readID, ok := th.ExtractToolCallByName(final.Blocks, "Read")
	if !ok {
		t.Fatalf("no Read tool_call in final blocks\nraw:\n%s", sub.FormatRawEvents())
	}
	readResult, ok := th.ExtractToolResultByCallID(final.Blocks, readID)
	if !ok {
		t.Fatalf("no Read tool_result for call %q", readID)
	}
	if v, _ := readResult["ok"].(bool); !v {
		t.Errorf("Read tool_result not ok: %v", readResult)
	}

	editID, ok := th.ExtractToolCallByName(final.Blocks, "Edit")
	if !ok {
		t.Fatalf("no Edit tool_call in final blocks")
	}
	editResult, ok := th.ExtractToolResultByCallID(final.Blocks, editID)
	if !ok {
		t.Fatalf("no Edit tool_result for call %q", editID)
	}
	if v, _ := editResult["ok"].(bool); !v {
		t.Errorf("Edit tool_result not ok: %v", editResult)
	}
	if got, _ := editResult["result"].(string); !strings.Contains(got, "Replaced 1 occurrence") {
		t.Errorf("Edit result message missing explicit count: %q", got)
	}

	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read final content: %v", err)
	}
	if string(got) != "beta line\n" {
		t.Errorf("file content = %q, want %q", got, "beta line\n")
	}

	if text := th.ExtractTextFromBlocks(final.Blocks); !strings.Contains(text, "Done") {
		t.Errorf("expected 'Done' in final text, got %q", text)
	}
}

func TestFileOps_WriteWithoutReadDenied(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "exists.txt")
	if err := os.WriteFile(target, []byte("original"), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}

	fake := th.NewFakeLLMServer(t)
	fake.PushScript(th.ScriptSingleToolCall(
		"Write", "call_fake_write_001",
		fmt.Sprintf(`{"summary":"overwriting","file_path":%q,"content":"new"}`, target),
	))
	fake.PushScript(th.ScriptText("I cannot overwrite the file without reading it first."))

	h := th.New(t, th.WithFakeLLMBaseURL(fake.URL()))
	h.SeedDeepSeek(t, "fake-test-key")
	conv := h.NewConversation(t, "guard")
	sub := h.SubscribeSSE(t, conv.ID)

	th.PostMessage(t, h, conv.ID, "Overwrite "+target+" with 'new'.")

	final := sub.WaitForAssistantTerminal(30 * time.Second)
	if final.Status != chatdomain.StatusCompleted {
		t.Fatalf("status=%q\nraw:\n%s", final.Status, sub.FormatRawEvents())
	}

	writeID, ok := th.ExtractToolCallByName(final.Blocks, "Write")
	if !ok {
		t.Fatal("no Write tool_call in final blocks")
	}
	writeResult, ok := th.ExtractToolResultByCallID(final.Blocks, writeID)
	if !ok {
		t.Fatal("no Write tool_result")
	}
	resultText, _ := writeResult["result"].(string)
	if !strings.Contains(resultText, "must be read first") {
		t.Errorf("expected must-Read-first denial in tool_result, got: %q", resultText)
	}

	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read post-test content: %v", err)
	}
	if string(got) != "original" {
		t.Errorf("file content = %q, want %q (overwrite should have been blocked)", got, "original")
	}
}

func TestFileOps_PathGuardDeniesSensitivePath(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no user home dir; skipping (PathGuard test needs ~ expansion)")
	}
	denied := filepath.Join(home, ".ssh", "id_rsa-pipeline-test-must-not-exist")

	fake := th.NewFakeLLMServer(t)
	fake.PushScript(th.ScriptSingleToolCall(
		"Read", "call_fake_read_002",
		fmt.Sprintf(`{"summary":"checking key","file_path":%q}`, denied),
	))
	fake.PushScript(th.ScriptText("I cannot read that path."))

	h := th.New(t, th.WithFakeLLMBaseURL(fake.URL()))
	h.SeedDeepSeek(t, "fake-test-key")
	conv := h.NewConversation(t, "guard-path")
	sub := h.SubscribeSSE(t, conv.ID)

	th.PostMessage(t, h, conv.ID, "Read "+denied)

	final := sub.WaitForAssistantTerminal(30 * time.Second)
	if final.Status != chatdomain.StatusCompleted {
		t.Fatalf("status=%q\nraw:\n%s", final.Status, sub.FormatRawEvents())
	}

	readID, ok := th.ExtractToolCallByName(final.Blocks, "Read")
	if !ok {
		t.Fatal("no Read tool_call in final blocks")
	}
	readResult, ok := th.ExtractToolResultByCallID(final.Blocks, readID)
	if !ok {
		t.Fatal("no Read tool_result")
	}
	resultText, _ := readResult["result"].(string)
	if !strings.Contains(resultText, "denied by safety guard") {
		t.Errorf("expected PathGuard denial in tool_result, got: %q", resultText)
	}
	if v, _ := readResult["ok"].(bool); !v {
		t.Errorf("Read tool_result.ok = false; expected true (denial is a friendly string, not tool error). data: %s", mustJSON(readResult))
	}
}

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}
