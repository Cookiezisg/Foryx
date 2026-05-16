package shell

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
)


var (
	// ErrEmptyBashID: bash_id missing.
	//
	// ErrEmptyBashID：bash_id 缺失。
	ErrEmptyBashID = errors.New("bash_id is required")
)


const outputDescription = `Read new stdout/stderr from a background shell process started by Bash.

Usage:
- ` + "`bash_id`" + ` is the ID returned by a Bash call with run_in_background:true.
- Returns only output APPEARED SINCE THE LAST BashOutput call for that ID — successive polls don't repeat what you've already seen.
- Includes a status footer: "running", "exited (code N)", "killed", or "errored".
- ` + "`filter`" + ` (optional regex) keeps only matching lines from the new output. Useful for grepping a noisy log stream.
- Returns a not-found message if the bash_id is unknown (never started or already removed via KillShell).`

var outputSchema = json.RawMessage(`{
	"type": "object",
	"required": ["bash_id"],
	"properties": {
		"bash_id": {
			"type": "string",
			"description": "ID of the background shell process to poll (returned by Bash with run_in_background:true)."
		},
		"filter": {
			"type": "string",
			"description": "Optional regex; keep only matching lines from the new output."
		}
	}
}`)


// BashOutput implements the BashOutput system tool.
//
// BashOutput 是 BashOutput 系统工具的实现。
type BashOutput struct {
	mgr *ProcessManager
}

func (t *BashOutput) Name() string                { return "BashOutput" }
func (t *BashOutput) Description() string         { return outputDescription }
func (t *BashOutput) Parameters() json.RawMessage { return outputSchema }

func (t *BashOutput) IsReadOnly() bool        { return true }
func (t *BashOutput) NeedsReadFirst() bool    { return false }
func (t *BashOutput) RequiresWorkspace() bool { return false }

// ValidateInput rejects empty bash_id and invalid regex pre-Execute.
//
// ValidateInput 在 Execute 前拒绝空 bash_id 与非法 regex。
func (t *BashOutput) ValidateInput(args json.RawMessage) error {
	var a struct {
		BashID string `json:"bash_id"`
		Filter string `json:"filter"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return fmt.Errorf("BashOutput.ValidateInput: %w", err)
	}
	if strings.TrimSpace(a.BashID) == "" {
		return ErrEmptyBashID
	}
	if a.Filter != "" {
		if _, err := regexp.Compile(a.Filter); err != nil {
			return fmt.Errorf("BashOutput.ValidateInput: filter regex: %w", err)
		}
	}
	return nil
}

func (t *BashOutput) CheckPermissions(_ json.RawMessage, _ toolapp.PermissionMode) toolapp.PermissionResult {
	return toolapp.PermissionAllow
}

// Execute drains new bytes from the named process, optionally filters by
// regex, and emits them with a status footer.
//
// Execute 从命名进程取新字节，按可选 regex 过滤，附状态尾注返回。
func (t *BashOutput) Execute(_ context.Context, argsJSON string) (string, error) {
	var args struct {
		BashID string `json:"bash_id"`
		Filter string `json:"filter"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("BashOutput.Execute: %w", err)
	}

	proc, err := t.mgr.Get(args.BashID)
	if err != nil {
		return fmt.Sprintf("Background shell process not found: %s", args.BashID), nil
	}

	newBytes, dropped, status, exitCode := proc.drainNew()
	body := string(newBytes)

	if args.Filter != "" {
		re := regexp.MustCompile(args.Filter)
		body = filterLines(body, re)
	}

	return formatOutputResult(body, dropped, status, exitCode), nil
}

func filterLines(s string, re *regexp.Regexp) string {
	if s == "" {
		return ""
	}
	lines := strings.Split(s, "\n")
	out := lines[:0]
	for _, ln := range lines {
		if re.MatchString(ln) {
			out = append(out, ln)
		}
	}
	return strings.Join(out, "\n")
}

func formatOutputResult(body string, dropped int64, status Status, exitCode int) string {
	var sb strings.Builder
	if body != "" {
		sb.WriteString(body)
		if !strings.HasSuffix(body, "\n") {
			sb.WriteString("\n")
		}
	} else {
		sb.WriteString("(no new output since last poll)\n")
	}
	sb.WriteString("\n")
	if dropped > 0 {
		fmt.Fprintf(&sb, "[note: %d bytes dropped from buffer head before this poll due to ring overflow]\n", dropped)
	}
	switch status {
	case StatusRunning:
		sb.WriteString("[status: running]")
	case StatusExited:
		fmt.Fprintf(&sb, "[status: exited (code %d)]", exitCode)
	case StatusKilled:
		sb.WriteString("[status: killed]")
	case StatusErrored:
		sb.WriteString("[status: errored]")
	default:
		sb.WriteString("[status: unknown]")
	}
	return sb.String()
}


var _ toolapp.Tool = (*BashOutput)(nil)
