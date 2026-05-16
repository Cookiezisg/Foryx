package shell

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"os/exec"
)

func newTestKill() (*KillShell, *ProcessManager) {
	mgr := NewProcessManager()
	return &KillShell{mgr: mgr}, mgr
}


func TestKillShell_IdentityMethods(t *testing.T) {
	tool, _ := newTestKill()
	if tool.Name() != "KillShell" {
		t.Errorf("Name = %q, want KillShell", tool.Name())
	}
	if tool.Description() == "" {
		t.Error("Description empty")
	}
	if len(tool.Parameters()) == 0 {
		t.Error("Parameters empty")
	}
}

func TestKillShell_StaticMetadata(t *testing.T) {
	tool, _ := newTestKill()
	if tool.IsReadOnly() {
		t.Error("KillShell should not be read-only (it terminates)")
	}
}

func TestKillShell_Schema_HasBashID(t *testing.T) {
	var doc map[string]any
	if err := json.Unmarshal(killSchema, &doc); err != nil {
		t.Fatalf("schema not valid: %v", err)
	}
	props := doc["properties"].(map[string]any)
	if _, ok := props["bash_id"]; !ok {
		t.Error("schema missing shell_id")
	}
}


func TestKillShell_ValidateInput_RequiresBashID(t *testing.T) {
	tool, _ := newTestKill()
	err := tool.ValidateInput(json.RawMessage(`{}`))
	if err == nil || !strings.Contains(err.Error(), "bash_id") {
		t.Errorf("want shell_id error, got %v", err)
	}
}


func TestKillShell_Execute_UnknownID_FriendlyMessage(t *testing.T) {
	tool, _ := newTestKill()
	out, err := tool.Execute(context.Background(), `{"bash_id":"bsh_unknown"}`)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "not found") {
		t.Errorf("expected not-found message, got: %q", out)
	}
}

func TestKillShell_Execute_KillsRunningProcess(t *testing.T) {
	tool, mgr := newTestKill()
	cmd := exec.Command("sleep", "10")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start sleep: %v", err)
	}
	proc := &BgProcess{Command: "sleep 10", Cmd: cmd, status: StatusRunning}
	mgr.Register(proc)

	out, err := tool.Execute(context.Background(), `{"bash_id":"`+proc.ID+`"}`)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "Killed background shell") {
		t.Errorf("expected kill confirmation, got: %q", out)
	}
	// Reaper goroutine doesn't run in unit tests; manually wait so the
	// child doesn't linger as a zombie under `go test`.
	// 单测里没 reaper goroutine；手动 wait 防 zombie。
	done := make(chan struct{})
	go func() { _ = cmd.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Error("child did not exit after kill within 2s")
	}
	// Registry entry should be gone.
	// 注册表条目应已删除。
	if _, err := mgr.Get(proc.ID); !errors.Is(err, ErrProcessNotFound) {
		t.Errorf("expected ErrProcessNotFound after kill, got %v", err)
	}
}

func TestKillShell_Execute_AlreadyFinishedProcess_StillRemovesEntry(t *testing.T) {
	tool, mgr := newTestKill()
	// Simulate a finished process: Cmd has no Process attached.
	// 模拟已结束进程：Cmd 没附 Process。
	proc := &BgProcess{Command: "echo done", status: StatusExited}
	mgr.Register(proc)

	out, err := tool.Execute(context.Background(), `{"bash_id":"`+proc.ID+`"}`)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "already finished") {
		t.Errorf("expected already-finished message, got: %q", out)
	}
	if _, err := mgr.Get(proc.ID); !errors.Is(err, ErrProcessNotFound) {
		t.Errorf("registry entry should be removed; got %v", err)
	}
}

func TestKillShell_Execute_IsIdempotent(t *testing.T) {
	tool, _ := newTestKill()
	args := `{"bash_id":"bsh_neverexisted"}`
	out1, _ := tool.Execute(context.Background(), args)
	out2, _ := tool.Execute(context.Background(), args)
	if out1 != out2 {
		t.Errorf("expected idempotent unknown-id behaviour:\n  call1: %q\n  call2: %q", out1, out2)
	}
}
