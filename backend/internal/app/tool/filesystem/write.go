package filesystem

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
	pathguardpkg "github.com/sunweilin/forgify/backend/internal/pkg/pathguard"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

const defaultFileMode os.FileMode = 0o644

const writeDescription = `Writes a file to the local filesystem. Overwrites if the file exists.

Usage:
- file_path must be an absolute path.
- Existing files require a prior Read in this conversation (must-Read-first guard prevents accidental clobbering).
- Prefer Edit for modifying existing files — Edit sends only the diff.
- Parent directory must exist; use Bash 'mkdir -p' first if needed.
- Writes are atomic (tmp file + rename); readers never see a half-written file.
- Sensitive paths (system directories, credential locations) are blocked.`

var writeSchema = json.RawMessage(`{
	"type": "object",
	"required": ["file_path", "content"],
	"properties": {
		"file_path": {
			"type": "string",
			"description": "The absolute path to the file to write (must be absolute)"
		},
		"content": {
			"type": "string",
			"description": "The content to write to the file (may be empty to create an empty file)"
		}
	}
}`)

// Write implements the Write system tool.
//
// Write 是 Write 系统工具的实现。
type Write struct {
	pathGuard pathguardpkg.PathGuard
}

func (t *Write) Name() string                { return "Write" }
func (t *Write) Description() string         { return writeDescription }
func (t *Write) Parameters() json.RawMessage { return writeSchema }

func (t *Write) IsReadOnly() bool        { return false }
func (t *Write) NeedsReadFirst() bool    { return true }
func (t *Write) RequiresWorkspace() bool { return true }

// ValidateInput requires file_path absolute and content key present (empty allowed).
//
// ValidateInput 要求 file_path 为绝对路径且 content 字段存在（可空）。
func (t *Write) ValidateInput(args json.RawMessage) error {
	var a struct {
		FilePath string  `json:"file_path"`
		Content  *string `json:"content"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return fmt.Errorf("Write.ValidateInput: %w", err)
	}
	if strings.TrimSpace(a.FilePath) == "" {
		return ErrEmptyFilePath
	}
	if !filepath.IsAbs(a.FilePath) {
		return ErrPathNotAbsolute
	}
	if a.Content == nil {
		return errors.New("content field is required (use empty string to create an empty file)")
	}
	return nil
}

func (t *Write) CheckPermissions(_ json.RawMessage, _ toolapp.PermissionMode) toolapp.PermissionResult {
	return toolapp.PermissionAllow
}

// Execute atomically writes content to file_path under PathGuard / parent-exists / must-Read-first-on-overwrite guards.
//
// Execute 在 PathGuard / 父目录存在 / 覆写时 must-Read-first 守卫下原子写 content 到 file_path。
func (t *Write) Execute(ctx context.Context, argsJSON string) (string, error) {
	var args struct {
		FilePath string `json:"file_path"`
		Content  string `json:"content"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("Write.Execute: %w", err)
	}

	if ok, reason := t.pathGuard.Allow(args.FilePath); !ok {
		return reason, nil
	}

	cleaned := filepath.Clean(args.FilePath)
	parent := filepath.Dir(cleaned)

	parentInfo, err := os.Stat(parent)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "Parent directory does not exist: " + parent + ". Use Bash 'mkdir -p' to create it first.", nil
		}
		return fmt.Sprintf("Cannot access parent directory %s: %v", parent, err), nil
	}
	if !parentInfo.IsDir() {
		return "Parent path exists but is not a directory: " + parent, nil
	}

	existingInfo, statErr := os.Stat(cleaned)
	exists := statErr == nil
	if exists && existingInfo.IsDir() {
		return "Path is a directory, not a file: " + cleaned, nil
	}
	if exists {
		state, hasState := reqctxpkg.GetAgentState(ctx)
		if !hasState {
			// Refuse overwrite when AgentState missing — silently allowing would defeat must-Read-first.
			// AgentState 缺失时拒绝覆写——静默放过会让 must-Read-first 形同虚设。
			return "Cannot verify Read-first guard: agent state missing. Read the file first.", nil
		}
		if _, seen := state.WasRead(cleaned); !seen {
			return "File must be read first before overwriting: " + cleaned + ". Use the Read tool first.", nil
		}
	}

	tmpFile, err := os.CreateTemp(parent, ".forgify-write-*")
	if err != nil {
		return fmt.Sprintf("Cannot create temp file in %s: %v", parent, err), nil
	}
	tmpPath := tmpFile.Name()
	cleanup := func() { _ = os.Remove(tmpPath) }

	if _, err := tmpFile.WriteString(args.Content); err != nil {
		_ = tmpFile.Close()
		cleanup()
		return fmt.Sprintf("Write failed (writing temp): %v", err), nil
	}
	if err := tmpFile.Close(); err != nil {
		cleanup()
		return fmt.Sprintf("Write failed (closing temp): %v", err), nil
	}

	// Explicit chmod: CreateTemp defaults to 0600 which would silently shrink permissions on overwrite.
	// 显式 chmod：CreateTemp 默认 0600，覆写时会静默收紧权限。
	mode := defaultFileMode
	if exists {
		mode = existingInfo.Mode().Perm()
	}
	if err := os.Chmod(tmpPath, mode); err != nil {
		cleanup()
		return fmt.Sprintf("Write failed (chmod temp): %v", err), nil
	}

	if err := os.Rename(tmpPath, cleaned); err != nil {
		cleanup()
		return fmt.Sprintf("Write failed (rename to target): %v", err), nil
	}

	if state, ok := reqctxpkg.GetAgentState(ctx); ok {
		state.MarkRead(cleaned, int64(len(args.Content)))
	}

	return "Wrote " + cleaned, nil
}

var _ toolapp.Tool = (*Write)(nil)
