//go:build pipeline

// filesystem_test.go — pipeline tests for the file-ops system tools
// (Read / Write / Edit) driving the full chat ReAct loop with a fake LLM.
//
// Scenarios:
//  1. ReadEditClosedLoop — fake LLM scripts Read → Edit → "Done"; verify
//     file content actually changed on disk and both tool_call/tool_result
//     pairs are persisted.
//  2. WriteWithoutReadDenied — fake LLM scripts Write directly on an
//     existing file (no Read first); verify must-Read-first guard fires
//     and original file is untouched.
//  3. PathGuardDeniesSensitivePath — fake LLM tries to Read a sensitive
//     path; verify PathGuard denies and tool_result reflects denial.
//
// filesystem_test.go — file-ops 工具（Read / Write / Edit）的 pipeline 测试，
// 用 fake LLM 驱动完整 chat ReAct 循环。
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

// ── 1. Read → Edit → Done closed loop ─────────────────────────────────────────

func TestFileOps_ReadEditClosedLoop(t *testing.T) {
	// Pre-create a file the LLM will Read then Edit.
	// 预创建一个 LLM 将要 Read 后 Edit 的文件。
	dir := t.TempDir()
	target := filepath.Join(dir, "src.txt")
	if err := os.WriteFile(target, []byte("alpha line\n"), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}

	fake := th.NewFakeLLMServer(t)
	// Round 1: LLM emits a Read tool call.
	// Round 1：LLM 发 Read tool call。
	fake.PushScript(th.ScriptSingleToolCall(
		"Read", "call_fake_read_001",
		fmt.Sprintf(`{"summary":"reading src","file_path":%q}`, target),
	))
	// Round 2: LLM emits an Edit tool call (alpha → beta).
	// Round 2：LLM 发 Edit tool call（alpha → beta）。
	fake.PushScript(th.ScriptSingleToolCall(
		"Edit", "call_fake_edit_001",
		fmt.Sprintf(`{"summary":"renaming alpha","file_path":%q,"old_string":"alpha","new_string":"beta"}`, target),
	))
	// Round 3: LLM wraps up with text.
	// Round 3：LLM 文本收尾。
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

	// Both tool_call+result pairs must be present.
	// 两对 tool_call+result 都必须在最终 blocks 里。
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

	// File on disk must reflect the edit.
	// 磁盘文件必须反映编辑结果。
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read final content: %v", err)
	}
	if string(got) != "beta line\n" {
		t.Errorf("file content = %q, want %q", got, "beta line\n")
	}

	// Final assistant text must contain the wrap-up.
	// 最终 assistant 文本必须含收尾消息。
	if text := th.ExtractTextFromBlocks(final.Blocks); !strings.Contains(text, "Done") {
		t.Errorf("expected 'Done' in final text, got %q", text)
	}
}

// ── 2. Write without Read first → must-Read-first guard fires ─────────────────

func TestFileOps_WriteWithoutReadDenied(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "exists.txt")
	if err := os.WriteFile(target, []byte("original"), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}

	fake := th.NewFakeLLMServer(t)
	// Round 1: LLM tries to Write directly without Read. Tool will refuse.
	// Round 1：LLM 直接 Write 而没 Read。Tool 会拒绝。
	fake.PushScript(th.ScriptSingleToolCall(
		"Write", "call_fake_write_001",
		fmt.Sprintf(`{"summary":"overwriting","file_path":%q,"content":"new"}`, target),
	))
	// Round 2: LLM acknowledges the failure.
	// Round 2：LLM 看到失败后收尾。
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

	// Write tool_result.ok must be true at the framework level (no Go error)
	// even though the operation was refused — refusal is a friendly string,
	// not a tool failure. Verify the result text reflects the guard.
	//
	// Write tool_result.ok 在 framework 层是 true（无 Go err），即便操作被拒
	// ——拒绝是友好字符串，不是 tool 失败。验证 result 文本反映守卫。
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

	// Original file content must be intact — guard prevented the overwrite.
	// 原文件内容必须保留——守卫阻止了覆写。
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read post-test content: %v", err)
	}
	if string(got) != "original" {
		t.Errorf("file content = %q, want %q (overwrite should have been blocked)", got, "original")
	}
}

// ── 3. PathGuard denies a sensitive path ──────────────────────────────────────

func TestFileOps_PathGuardDeniesSensitivePath(t *testing.T) {
	// Compute a path that's on PathGuard's default deny list (~/.ssh/...).
	// 取一个在 PathGuard 默认黑名单里的路径（~/.ssh/...）。
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
	// Defensive: ensure tool result data shape is what we expect (not Go error).
	// 防御：确认 tool_result 数据形状如预期（不是 Go error）。
	if v, _ := readResult["ok"].(bool); !v {
		t.Errorf("Read tool_result.ok = false; expected true (denial is a friendly string, not tool error). data: %s", mustJSON(readResult))
	}
}

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}
