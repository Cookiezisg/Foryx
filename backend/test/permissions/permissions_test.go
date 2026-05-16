//go:build pipeline

// permissions_test.go — pipeline tests for V1.2 §3 final-sweep: the
// permissions gate + Pre/PostToolUse hooks layered onto runOneTool.
//
// Scenarios:
//  1. DenyRule_BlocksTool — settings.json denies "TodoCreate"; LLM
//     calls it; expect tool_result error "BLOCKED_BY_RULE".
//  2. PreToolUseHook_DeniesViaExitCode2 — shell hook exits 2; expect
//     same block + reason from stderr.
//  3. PostToolUseHook_AppendsHint — hook prints injectIntoNextTurn;
//     expect tool_result content to contain "[hook] <hint>".
//
// permissions_test.go ——V1.2 §3 final-sweep pipeline 测试：permissions
// gate + Pre/PostToolUse hook 接到 runOneTool。
package permissions_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	chatdomain "github.com/sunweilin/forgify/backend/internal/domain/chat"
	th "github.com/sunweilin/forgify/backend/test/harness"
)

// writeSettings drops a settings.json into the harness's expected path
// and forces a reload so the next call sees them.
//
// writeSettings 把 settings.json 写到 harness 期望路径并强制 reload。
func writeSettings(t *testing.T, h *th.Harness, content string) {
	t.Helper()
	if err := os.WriteFile(h.SettingsPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write settings: %v", err)
	}
	if err := h.Settings.Reload(); err != nil {
		t.Fatalf("reload settings: %v", err)
	}
}

// ── 1. DenyRule_BlocksTool ──────────────────────────────────────────────

// TestPermissions_DenyRule_BlocksTool installs a deny rule for TodoCreate,
// scripts the fake LLM to call it, and asserts the tool_result block is
// a denial error.
//
// TestPermissions_DenyRule_BlocksTool 装 TodoCreate deny 规则，fake LLM
// 调它，断言 tool_result 是拒绝错误。
func TestPermissions_DenyRule_BlocksTool(t *testing.T) {
	fake := th.NewFakeLLMServer(t)
	fake.PushScript(th.ScriptSingleToolCall(
		"TodoCreate", "call_perm_deny_1",
		`{"summary":"create a todo","subject":"x","active_form":"working on x"}`,
	))
	fake.PushScript(th.ScriptText("got it"))
	fake.PushDefault(th.ScriptText("Title"))

	h := th.New(t, th.WithFakeLLMBaseURL(fake.URL()))
	h.SeedDeepSeek(t, "fake-test-key")

	writeSettings(t, h, `{
		"permissions": {
			"defaultMode": "allow",
			"deny": ["TodoCreate"]
		}
	}`)

	conv := h.NewConversation(t, "perm-deny")
	sub := h.SubscribeSSE(t, conv.ID)
	th.PostMessage(t, h, conv.ID, "make a todo")

	final := sub.WaitForAssistantTerminal(30 * time.Second)
	if final.Status != chatdomain.StatusCompleted {
		t.Fatalf("status=%q raw:\n%s", final.Status, sub.FormatRawEvents())
	}

	res, ok := th.ExtractToolResultByCallID(final.Blocks, "call_perm_deny_1")
	if !ok {
		t.Fatalf("no TodoCreate tool_result in final blocks\nraw:\n%s", sub.FormatRawEvents())
	}
	if v, _ := res["ok"].(bool); v {
		t.Errorf("expected ok=false on denied call, got ok=true: %v", res)
	}
	text, _ := res["result"].(string)
	if !strings.Contains(text, "permission denied") {
		t.Errorf("expected 'permission denied' in result text, got %q", text)
	}
}

// ── 2. PreToolUseHook_DeniesViaExitCode2 ─────────────────────────────────

// TestPermissions_PreToolUseHook_DeniesViaExitCode2 sets up a shell hook
// that prints "blocked by policy" to stderr and exits 2 for any TodoCreate
// call; expects the tool_result to be a denial with the hook's reason.
//
// TestPermissions_PreToolUseHook_DeniesViaExitCode2 装一个对 TodoCreate
// 调用 stderr 打 "blocked by policy" + exit 2 的 shell hook；断言
// tool_result 是拒绝 + 含 hook reason。
func TestPermissions_PreToolUseHook_DeniesViaExitCode2(t *testing.T) {
	fake := th.NewFakeLLMServer(t)
	fake.PushScript(th.ScriptSingleToolCall(
		"TodoCreate", "call_perm_hookdeny_1",
		`{"summary":"another todo","subject":"y","active_form":"working on y"}`,
	))
	fake.PushScript(th.ScriptText("ok"))
	fake.PushDefault(th.ScriptText("Title"))

	h := th.New(t, th.WithFakeLLMBaseURL(fake.URL()))
	h.SeedDeepSeek(t, "fake-test-key")

	// Write the hook script next to settings.json so the relative
	// reference resolves predictably.
	// hook 脚本和 settings.json 同目录，引用稳定。
	hookPath := filepath.Join(filepath.Dir(h.SettingsPath), "deny-todo.sh")
	hookBody := "#!/bin/sh\necho \"blocked by policy\" >&2\nexit 2\n"
	if err := os.WriteFile(hookPath, []byte(hookBody), 0o755); err != nil {
		t.Fatalf("write hook: %v", err)
	}

	writeSettings(t, h, `{
		"permissions": {"defaultMode": "allow"},
		"hooks": {
			"PreToolUse": [
				{"matcher": "TodoCreate", "command": "`+hookPath+`", "timeout": 5}
			]
		}
	}`)

	conv := h.NewConversation(t, "perm-hookdeny")
	sub := h.SubscribeSSE(t, conv.ID)
	th.PostMessage(t, h, conv.ID, "make a todo")

	final := sub.WaitForAssistantTerminal(30 * time.Second)
	if final.Status != chatdomain.StatusCompleted {
		t.Fatalf("status=%q raw:\n%s", final.Status, sub.FormatRawEvents())
	}

	res, ok := th.ExtractToolResultByCallID(final.Blocks, "call_perm_hookdeny_1")
	if !ok {
		t.Fatalf("no TodoCreate tool_result\nraw:\n%s", sub.FormatRawEvents())
	}
	if v, _ := res["ok"].(bool); v {
		t.Errorf("expected ok=false on hook-denied call, got ok=true: %v", res)
	}
	text, _ := res["result"].(string)
	if !strings.Contains(text, "blocked by policy") {
		t.Errorf("expected hook reason 'blocked by policy' in result, got %q", text)
	}
}

// ── 3. PostToolUseHook_AppendsHint ───────────────────────────────────────

// TestPermissions_PostToolUseHook_AppendsHint installs a PostToolUse hook
// that emits an injectIntoNextTurn string; asserts the tool_result content
// has the "[hook] <text>" appendix runOneTool concatenates.
//
// TestPermissions_PostToolUseHook_AppendsHint 装 PostToolUse hook 发
// injectIntoNextTurn 串；断言 tool_result content 有 runOneTool 拼的
// "[hook] <text>" 附录。
func TestPermissions_PostToolUseHook_AppendsHint(t *testing.T) {
	fake := th.NewFakeLLMServer(t)
	fake.PushScript(th.ScriptSingleToolCall(
		"TodoCreate", "call_perm_posthook_1",
		`{"summary":"third","subject":"z","active_form":"working on z"}`,
	))
	fake.PushScript(th.ScriptText("done"))
	fake.PushDefault(th.ScriptText("Title"))

	h := th.New(t, th.WithFakeLLMBaseURL(fake.URL()))
	h.SeedDeepSeek(t, "fake-test-key")

	hookPath := filepath.Join(filepath.Dir(h.SettingsPath), "post-hint.sh")
	hookBody := "#!/bin/sh\nprintf '%s' '{\"injectIntoNextTurn\":\"remember to run tests\"}'\n"
	if err := os.WriteFile(hookPath, []byte(hookBody), 0o755); err != nil {
		t.Fatalf("write hook: %v", err)
	}

	writeSettings(t, h, `{
		"permissions": {"defaultMode": "allow"},
		"hooks": {
			"PostToolUse": [
				{"matcher": "TodoCreate", "command": "`+hookPath+`", "timeout": 5}
			]
		}
	}`)

	conv := h.NewConversation(t, "perm-posthook")
	sub := h.SubscribeSSE(t, conv.ID)
	th.PostMessage(t, h, conv.ID, "make a todo")

	final := sub.WaitForAssistantTerminal(30 * time.Second)
	if final.Status != chatdomain.StatusCompleted {
		t.Fatalf("status=%q raw:\n%s", final.Status, sub.FormatRawEvents())
	}

	res, ok := th.ExtractToolResultByCallID(final.Blocks, "call_perm_posthook_1")
	if !ok {
		t.Fatalf("no TodoCreate tool_result\nraw:\n%s", sub.FormatRawEvents())
	}
	if v, _ := res["ok"].(bool); !v {
		t.Errorf("PostToolUse hook should not deny; got ok=false: %v", res)
	}
	text, _ := res["result"].(string)
	if !strings.Contains(text, "[hook] remember to run tests") {
		t.Errorf("expected '[hook] remember to run tests' in result, got %q", text)
	}
}
