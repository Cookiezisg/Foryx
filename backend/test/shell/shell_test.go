//go:build pipeline

// Package shell_test runs pipeline tests for shell system tools (Bash/BashOutput/KillShell).
//
// Package shell_test 跑 shell 系统工具（Bash/BashOutput/KillShell）pipeline 测试。
package shell_test

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	chatdomain "github.com/sunweilin/forgify/backend/internal/domain/chat"
	th "github.com/sunweilin/forgify/backend/test/harness"
)

func TestShell_BashEchoForeground(t *testing.T) {
	fake := th.NewFakeLLMServer(t)
	fake.PushScript(th.ScriptSingleToolCall(
		"Bash", "call_fake_bash_001",
		`{"summary":"sanity echo","command":"echo hello-from-bash-pipeline"}`,
	))
	fake.PushScript(th.ScriptText("Echoed."))

	h := th.New(t, th.WithFakeLLMBaseURL(fake.URL()))
	h.SeedDeepSeek(t, "fake-test-key")
	conv := h.NewConversation(t, "shell-echo")
	sub := h.SubscribeSSE(t, conv.ID)

	th.PostMessage(t, h, conv.ID, "Echo a marker.")

	final := sub.WaitForAssistantTerminal(30 * time.Second)
	if final.Status != chatdomain.StatusCompleted {
		t.Fatalf("status=%q errCode=%q errMsg=%q\nraw:\n%s",
			final.Status, final.ErrorCode, final.ErrorMessage, sub.FormatRawEvents())
	}

	bashID, ok := th.ExtractToolCallByName(final.Blocks, "Bash")
	if !ok {
		t.Fatalf("no Bash tool_call in final blocks\nraw:\n%s", sub.FormatRawEvents())
	}
	res, ok := th.ExtractToolResultByCallID(final.Blocks, bashID)
	if !ok {
		t.Fatalf("no Bash tool_result for call %q", bashID)
	}
	if v, _ := res["ok"].(bool); !v {
		t.Errorf("Bash tool_result.ok = false; expected true. data: %v", res)
	}
	resultText, _ := res["result"].(string)
	if !strings.Contains(resultText, "hello-from-bash-pipeline") {
		t.Errorf("expected echoed marker in tool_result, got: %q", resultText)
	}
	if !strings.Contains(resultText, "[exit code: 0]") {
		t.Errorf("expected exit-code footer, got: %q", resultText)
	}
}

func TestShell_CdStateMachinePersistsAcrossCalls(t *testing.T) {
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("eval symlinks: %v", err)
	}

	fake := th.NewFakeLLMServer(t)
	fake.PushScript(th.ScriptSingleToolCall(
		"Bash", "call_fake_bash_cd",
		fmt.Sprintf(`{"summary":"cd to tmp","command":"cd %s"}`, dir),
	))
	fake.PushScript(th.ScriptSingleToolCall(
		"Bash", "call_fake_bash_pwd",
		`{"summary":"verify cwd","command":"pwd"}`,
	))
	fake.PushScript(th.ScriptText("Cwd verified."))

	h := th.New(t, th.WithFakeLLMBaseURL(fake.URL()))
	h.SeedDeepSeek(t, "fake-test-key")
	conv := h.NewConversation(t, "shell-cd")
	sub := h.SubscribeSSE(t, conv.ID)

	th.PostMessage(t, h, conv.ID, "Change directory then verify.")

	final := sub.WaitForAssistantTerminal(30 * time.Second)
	if final.Status != chatdomain.StatusCompleted {
		t.Fatalf("status=%q\nraw:\n%s", final.Status, sub.FormatRawEvents())
	}

	cdRes, ok := th.ExtractToolResultByCallID(final.Blocks, "call_fake_bash_cd")
	if !ok {
		t.Fatalf("no Bash tool_result for cd call\nraw:\n%s", sub.FormatRawEvents())
	}
	cdText, _ := cdRes["result"].(string)
	if !strings.Contains(cdText, "Changed working directory") {
		t.Errorf("first Bash result should be cd confirmation, got: %q", cdText)
	}

	pwdRes, ok := th.ExtractToolResultByCallID(final.Blocks, "call_fake_bash_pwd")
	if !ok {
		t.Fatalf("no Bash tool_result for pwd call")
	}
	pwdText, _ := pwdRes["result"].(string)
	if !strings.Contains(pwdText, dir) {
		t.Errorf("second Bash result (pwd) should contain %q, got: %q", dir, pwdText)
	}
}

func TestShell_BashOutputAndKillShellHandleUnknownID(t *testing.T) {
	fake := th.NewFakeLLMServer(t)
	fake.PushScript(th.ScriptSingleToolCall(
		"BashOutput", "call_fake_output",
		`{"summary":"poll unknown","bash_id":"bsh_doesnotexist"}`,
	))
	fake.PushScript(th.ScriptSingleToolCall(
		"KillShell", "call_fake_kill",
		`{"summary":"kill unknown","bash_id":"bsh_doesnotexist"}`,
	))
	fake.PushScript(th.ScriptText("Both reported unknown."))

	h := th.New(t, th.WithFakeLLMBaseURL(fake.URL()))
	h.SeedDeepSeek(t, "fake-test-key")
	conv := h.NewConversation(t, "shell-unknown")
	sub := h.SubscribeSSE(t, conv.ID)

	th.PostMessage(t, h, conv.ID, "Try poll and kill on a fake id.")

	final := sub.WaitForAssistantTerminal(30 * time.Second)
	if final.Status != chatdomain.StatusCompleted {
		t.Fatalf("status=%q\nraw:\n%s", final.Status, sub.FormatRawEvents())
	}

	outputID, ok := th.ExtractToolCallByName(final.Blocks, "BashOutput")
	if !ok {
		t.Fatalf("no BashOutput tool_call in final blocks")
	}
	outputRes, ok := th.ExtractToolResultByCallID(final.Blocks, outputID)
	if !ok {
		t.Fatal("no BashOutput tool_result")
	}
	outputText, _ := outputRes["result"].(string)
	if !strings.Contains(outputText, "not found") {
		t.Errorf("BashOutput result missing not-found message: %q", outputText)
	}

	killID, ok := th.ExtractToolCallByName(final.Blocks, "KillShell")
	if !ok {
		t.Fatalf("no KillShell tool_call in final blocks")
	}
	killRes, ok := th.ExtractToolResultByCallID(final.Blocks, killID)
	if !ok {
		t.Fatal("no KillShell tool_result")
	}
	killText, _ := killRes["result"].(string)
	if !strings.Contains(killText, "not found") {
		t.Errorf("KillShell result missing not-found message: %q", killText)
	}
}

