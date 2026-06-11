package shell

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

const killDescription = `Terminate a background Bash shell (bash_id). Idempotent — killing an already-finished or unknown id is harmless.`

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
type KillShell struct{ mgr *ProcessManager }

func (t *KillShell) Name() string                { return "KillShell" }
func (t *KillShell) Description() string         { return killDescription }
func (t *KillShell) Parameters() json.RawMessage { return killSchema }

// ValidateInput rejects empty bash_id.
//
// ValidateInput 拒绝空 bash_id。
func (t *KillShell) ValidateInput(args json.RawMessage) error {
	var a struct {
		BashID string `json:"bash_id"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return fmt.Errorf("KillShell: bad args: %w", err)
	}
	if strings.TrimSpace(a.BashID) == "" {
		return ErrEmptyBashID
	}
	return nil
}

// Execute kills the named process if running and removes it from the registry; idempotent.
//
// Execute 杀掉命名进程（若 running）并从注册表删除；幂等。
func (t *KillShell) Execute(_ context.Context, argsJSON string) (string, error) {
	var a struct {
		BashID string `json:"bash_id"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &a); err != nil {
		return "", fmt.Errorf("KillShell: %w", err)
	}

	proc, err := t.mgr.Get(a.BashID)
	if err != nil {
		return fmt.Sprintf("Background shell process not found: %s", a.BashID), nil
	}

	wasRunning := false
	if proc.Cmd != nil && proc.Cmd.Process != nil {
		if err := proc.Cmd.Process.Kill(); err == nil {
			wasRunning = true
		}
	}
	t.mgr.Remove(a.BashID)

	if wasRunning {
		return fmt.Sprintf("Killed background shell %s.", a.BashID), nil
	}
	return fmt.Sprintf("Background shell %s already finished; removed from registry.", a.BashID), nil
}
