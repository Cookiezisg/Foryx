package shell

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
)

const killDescription = `Terminate a background shell process started by Bash.

Usage:
- ` + "`bash_id`" + ` is the ID returned by a Bash call with run_in_background:true.
- Sends SIGKILL on Unix; the process is removed from the registry whether or not it was still running.
- Idempotent: killing an already-finished or unknown ID returns a clear message instead of failing.`

var killSchema = json.RawMessage(`{
	"type": "object",
	"required": ["bash_id"],
	"properties": {
		"bash_id": {
			"type": "string",
			"description": "ID of the background shell process to terminate (returned by Bash with run_in_background:true)."
		}
	}
}`)

// KillShell implements the KillShell system tool.
//
// KillShell 是 KillShell 系统工具的实现。
type KillShell struct {
	mgr *ProcessManager
}

func (t *KillShell) Name() string                { return "KillShell" }
func (t *KillShell) Description() string         { return killDescription }
func (t *KillShell) Parameters() json.RawMessage { return killSchema }

func (t *KillShell) IsReadOnly() bool        { return false }
func (t *KillShell) NeedsReadFirst() bool    { return false }
func (t *KillShell) RequiresWorkspace() bool { return false }

// ValidateInput rejects empty bash_id.
//
// ValidateInput 拒绝空 bash_id。
func (t *KillShell) ValidateInput(args json.RawMessage) error {
	var a struct {
		BashID string `json:"bash_id"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return fmt.Errorf("KillShell.ValidateInput: %w", err)
	}
	if strings.TrimSpace(a.BashID) == "" {
		return errors.New("bash_id is required")
	}
	return nil
}

func (t *KillShell) CheckPermissions(_ json.RawMessage, _ toolapp.PermissionMode) toolapp.PermissionResult {
	return toolapp.PermissionAllow
}

// Execute kills the named process if running and removes it from the registry; idempotent.
//
// Execute 杀掉命名进程（若 running）并从注册表删除；幂等。
func (t *KillShell) Execute(_ context.Context, argsJSON string) (string, error) {
	var args struct {
		BashID string `json:"bash_id"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("KillShell.Execute: %w", err)
	}

	proc, err := t.mgr.Get(args.BashID)
	if err != nil {
		return fmt.Sprintf("Background shell process not found: %s", args.BashID), nil
	}

	wasRunning := false
	if proc.Cmd != nil && proc.Cmd.Process != nil {
		if err := proc.Cmd.Process.Kill(); err == nil {
			wasRunning = true
		}
	}
	t.mgr.Remove(args.BashID)

	if wasRunning {
		return fmt.Sprintf("Killed background shell %s.", args.BashID), nil
	}
	return fmt.Sprintf("Background shell %s already finished; removed from registry.", args.BashID), nil
}


var _ toolapp.Tool = (*KillShell)(nil)
