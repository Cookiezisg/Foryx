package shell

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"testing"
)

func newTestOutput() (*BashOutput, *ProcessManager) {
	mgr := NewProcessManager()
	return &BashOutput{mgr: mgr}, mgr
}


func TestBashOutput_IdentityMethods(t *testing.T) {
	tool, _ := newTestOutput()
	if tool.Name() != "BashOutput" {
		t.Errorf("Name = %q, want BashOutput", tool.Name())
	}
	if tool.Description() == "" {
		t.Error("Description empty")
	}
	if len(tool.Parameters()) == 0 {
		t.Error("Parameters empty")
	}
}

func TestBashOutput_StaticMetadata(t *testing.T) {
	tool, _ := newTestOutput()
	if !tool.IsReadOnly() {
		t.Error("BashOutput should be read-only")
	}
	if tool.NeedsReadFirst() {
		t.Error("BashOutput should not require Read first")
	}
}

func TestBashOutput_Schema_HasExpectedFields(t *testing.T) {
	var doc map[string]any
	if err := json.Unmarshal(outputSchema, &doc); err != nil {
		t.Fatalf("schema not valid: %v", err)
	}
	props := doc["properties"].(map[string]any)
	for _, want := range []string{"bash_id", "filter"} {
		if _, ok := props[want]; !ok {
			t.Errorf("schema missing %q", want)
		}
	}
}


func TestBashOutput_ValidateInput_RequiresBashID(t *testing.T) {
	tool, _ := newTestOutput()
	if err := tool.ValidateInput(json.RawMessage(`{}`)); !errors.Is(err, ErrEmptyBashID) {
		t.Errorf("want ErrEmptyBashID, got %v", err)
	}
	if err := tool.ValidateInput(json.RawMessage(`{"bash_id":"  "}`)); !errors.Is(err, ErrEmptyBashID) {
		t.Errorf("whitespace should fail: %v", err)
	}
}

func TestBashOutput_ValidateInput_RejectsBadRegex(t *testing.T) {
	tool, _ := newTestOutput()
	err := tool.ValidateInput(json.RawMessage(`{"bash_id":"x","filter":"(unclosed"}`))
	if err == nil {
		t.Error("invalid regex should fail")
	}
}

func TestBashOutput_ValidateInput_AcceptsValid(t *testing.T) {
	tool, _ := newTestOutput()
	if err := tool.ValidateInput(json.RawMessage(`{"bash_id":"bsh_x","filter":"^OK"}`)); err != nil {
		t.Errorf("unexpected: %v", err)
	}
}


func TestBashOutput_Execute_UnknownID_FriendlyMessage(t *testing.T) {
	tool, _ := newTestOutput()
	out, err := tool.Execute(context.Background(), `{"bash_id":"bsh_doesnotexist"}`)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "not found") {
		t.Errorf("expected not-found message, got: %q", out)
	}
}

func TestBashOutput_Execute_ReturnsNewBytesAndAdvancesCursor(t *testing.T) {
	tool, mgr := newTestOutput()
	proc := &BgProcess{Command: "echo", status: StatusRunning}
	mgr.Register(proc)
	proc.appendOutput([]byte("line one\nline two\n"))

	first := mustExecute(t, tool, fmt.Sprintf(`{"bash_id":%q}`, proc.ID))
	if !strings.Contains(first, "line one") || !strings.Contains(first, "line two") {
		t.Errorf("first poll missing initial output: %q", first)
	}
	if !strings.Contains(first, "[status: running]") {
		t.Errorf("first poll missing status footer: %q", first)
	}

	// Second poll with no new output → "no new output" + status.
	// 第二次轮询无新输出 → "no new output" + 状态。
	second := mustExecute(t, tool, fmt.Sprintf(`{"bash_id":%q}`, proc.ID))
	if !strings.Contains(second, "no new output") {
		t.Errorf("second poll should say no new output: %q", second)
	}
}

func TestBashOutput_Execute_FilterRetainsOnlyMatchingLines(t *testing.T) {
	tool, mgr := newTestOutput()
	proc := &BgProcess{Command: "echo", status: StatusRunning}
	mgr.Register(proc)
	proc.appendOutput([]byte("INFO startup\nDEBUG noise\nERROR something failed\n"))

	out := mustExecute(t, tool, fmt.Sprintf(`{"bash_id":%q,"filter":"^ERROR"}`, proc.ID))
	if !strings.Contains(out, "ERROR something failed") {
		t.Errorf("filter should retain ERROR line: %q", out)
	}
	if strings.Contains(out, "INFO startup") || strings.Contains(out, "DEBUG noise") {
		t.Errorf("filter should drop non-matching lines: %q", out)
	}
}

func TestBashOutput_Execute_StatusFooterReflectsExitCode(t *testing.T) {
	tool, mgr := newTestOutput()
	proc := &BgProcess{Command: "echo", status: StatusRunning}
	mgr.Register(proc)
	proc.markFinished(StatusExited, 42)

	out := mustExecute(t, tool, fmt.Sprintf(`{"bash_id":%q}`, proc.ID))
	if !strings.Contains(out, "exited (code 42)") {
		t.Errorf("expected exit code in status, got: %q", out)
	}
}

func TestBashOutput_Execute_KilledStatusReported(t *testing.T) {
	tool, mgr := newTestOutput()
	proc := &BgProcess{Command: "echo", status: StatusRunning}
	mgr.Register(proc)
	proc.markFinished(StatusKilled, -1)

	out := mustExecute(t, tool, fmt.Sprintf(`{"bash_id":%q}`, proc.ID))
	if !strings.Contains(out, "[status: killed]") {
		t.Errorf("expected killed status, got: %q", out)
	}
}


func TestFilterLines_KeepsMatching(t *testing.T) {
	re := regexp.MustCompile("foo")
	got := filterLines("alpha\nfoo bar\nbaz\nfoo\n", re)
	want := "foo bar\nfoo\n"
	if !strings.HasPrefix(got, "foo bar") {
		t.Errorf("filterLines lost first match: %q", got)
	}
	// Allow trailing newline difference.
	// 容许末尾换行差异。
	if strings.TrimSpace(got) != strings.TrimSpace(want) {
		t.Errorf("filterLines = %q, want similar to %q", got, want)
	}
}

func TestFilterLines_EmptyInput(t *testing.T) {
	re := regexp.MustCompile("foo")
	if got := filterLines("", re); got != "" {
		t.Errorf("empty input should return empty, got %q", got)
	}
}


func mustExecute(t *testing.T, tool *BashOutput, args string) string {
	t.Helper()
	out, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	return out
}
