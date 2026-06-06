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
	fspathpkg "github.com/sunweilin/forgify/backend/internal/pkg/fspath"
	pathguardpkg "github.com/sunweilin/forgify/backend/internal/pkg/pathguard"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

const defaultFileMode os.FileMode = 0o644

const writeDescription = `Write a file, overwriting if it exists (atomic). Absolute path; parent dir must exist. Overwrite needs a prior Read this conversation. Prefer Edit for changes.`

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

// Write is the create-or-overwrite filesystem tool. It runs under three guards:
// PathGuard.AllowWrite (R0003 .git/.env/node_modules write extras), parent dir
// must exist, and overwrite requires the file to be in AgentState.SeenFiles —
// fail-closed when AgentState itself is missing, never silently allowed.
//
// Write 是创建或覆写的文件系统 tool。三重守卫：PathGuard.AllowWrite（R0003 .git/.env/
// node_modules 写专属 extras）、父目录必须存在、覆写要求文件在 AgentState.SeenFiles ——
// AgentState 缺失时 fail-closed，永不静默放行。
type Write struct {
	pathGuard pathguardpkg.PathGuard
}

func (t *Write) Name() string                { return "Write" }
func (t *Write) Description() string         { return writeDescription }
func (t *Write) Parameters() json.RawMessage { return writeSchema }

// ValidateInput requires file_path absolute and content key present. Empty
// content is explicitly allowed (create-empty-file is a legitimate use).
//
// ValidateInput 要求 file_path 为绝对路径且 content 字段存在。空 content 显式允许
// （创建空文件是合法用法）。
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
	if a.Content == nil {
		return errors.New("content field is required (use empty string to create an empty file)")
	}
	return nil
}

// Execute atomically writes content to file_path. Sequence: PathGuard.AllowWrite
// → parent-dir is a directory → must-Read-first guard (only when target exists)
// → write tmp + chmod (preserving original mode on overwrite) → rename to target
// → stamp new size into AgentState.
//
// Execute 原子写 content 到 file_path。顺序：PathGuard.AllowWrite → 父目录是目录 →
// 写前必读守卫（仅覆写）→ 写 tmp + chmod（覆写时保留原 mode）→ rename 到目标 → 把
// 新 size 盖章进 AgentState。
func (t *Write) Execute(ctx context.Context, argsJSON string) (string, error) {
	var args struct {
		FilePath string `json:"file_path"`
		Content  string `json:"content"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("Write.Execute: %w", err)
	}

	cleaned, err := fspathpkg.Expand(args.FilePath)
	if err != nil {
		return err.Error(), nil
	}
	if ok, reason := t.pathGuard.AllowWrite(cleaned); !ok {
		return reason, nil
	}

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
			// Fail-closed: silently allowing overwrite without state would defeat
			// the must-Read-first invariant. Better to surface a clear refusal so
			// either the LLM Reads first or the host learns to seed state.
			//
			// fail-closed：state 缺失时静默放过覆写会让写前必读不变式形同虚设。
			// 显式拒绝更好——要么 LLM 先 Read，要么 host 学会 seed state。
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

	// Explicit chmod: CreateTemp defaults to 0600 which would silently shrink
	// permissions on overwrite of e.g. a 0644 file. Preserve original mode.
	//
	// 显式 chmod：CreateTemp 默认 0600，覆写 0644 之类的文件会静默收紧权限。保留原 mode。
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
