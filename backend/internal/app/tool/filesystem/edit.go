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

var (
	// ErrEmptyOldString: old_string missing or empty.
	//
	// ErrEmptyOldString：old_string 缺失或为空。
	ErrEmptyOldString = errors.New("old_string is required and must be non-empty")

	// ErrEditNoOp: old_string == new_string would be a no-op edit.
	//
	// ErrEditNoOp：old_string == new_string 是空操作。
	ErrEditNoOp = errors.New("old_string and new_string must be different")
)

const editDescription = `Performs exact string replacement in an existing file.

Usage:
- file_path must be an absolute path.
- The file must have been Read in this conversation first.
- Matching is exact literal (NOT regex); whitespace, indentation, and case all matter.
- old_string must appear exactly once unless replace_all: true. Include enough context to make it unique.
- old_string and new_string must differ (no-op edits are rejected).
- Writes are atomic (tmp + rename). Result reports the actual replacement count.
- When editing text from Read output, preserve indentation AFTER the line-number prefix; never include the prefix itself.
- Sensitive paths (system directories, credential locations) are blocked.`

var editSchema = json.RawMessage(`{
	"type": "object",
	"required": ["file_path", "old_string", "new_string"],
	"properties": {
		"file_path": {
			"type": "string",
			"description": "The absolute path to the file to edit (must be absolute)"
		},
		"old_string": {
			"type": "string",
			"description": "The text to replace. Must be non-empty and present in the file. Include enough surrounding context to make the match unique unless replace_all is true."
		},
		"new_string": {
			"type": "string",
			"description": "The text to replace it with. Must differ from old_string."
		},
		"replace_all": {
			"type": "boolean",
			"default": false,
			"description": "If true, replace every occurrence of old_string in the file (e.g. variable rename). If false (default), the call fails when old_string is not unique."
		}
	}
}`)

// Edit implements the Edit system tool.
//
// Edit 是 Edit 系统工具的实现。
type Edit struct {
	pathGuard pathguardpkg.PathGuard
}

func (t *Edit) Name() string                { return "Edit" }
func (t *Edit) Description() string         { return editDescription }
func (t *Edit) Parameters() json.RawMessage { return editSchema }

func (t *Edit) IsReadOnly() bool        { return false }
func (t *Edit) NeedsReadFirst() bool    { return true }
func (t *Edit) RequiresWorkspace() bool { return true }

// ValidateInput rejects empty old_string and no-op old_string == new_string.
//
// ValidateInput 拒绝空 old_string 和 old_string == new_string 的空操作。
func (t *Edit) ValidateInput(args json.RawMessage) error {
	var a struct {
		FilePath  string  `json:"file_path"`
		OldString *string `json:"old_string"`
		NewString *string `json:"new_string"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return fmt.Errorf("Edit.ValidateInput: %w", err)
	}
	if strings.TrimSpace(a.FilePath) == "" {
		return ErrEmptyFilePath
	}
	if !filepath.IsAbs(a.FilePath) {
		return ErrPathNotAbsolute
	}
	if a.OldString == nil || *a.OldString == "" {
		return ErrEmptyOldString
	}
	if a.NewString == nil {
		return errors.New("new_string field is required (use empty string to delete the matched text)")
	}
	if *a.OldString == *a.NewString {
		return ErrEditNoOp
	}
	return nil
}

func (t *Edit) CheckPermissions(_ json.RawMessage, _ toolapp.PermissionMode) toolapp.PermissionResult {
	return toolapp.PermissionAllow
}

// Execute applies literal-substring replacement under guards: PathGuard, file exists, must-Read-first, size matches.
//
// Execute 在守卫（PathGuard / 文件存在 / must-Read-first / size 匹配）下做字面量替换并原子写入。
func (t *Edit) Execute(ctx context.Context, argsJSON string) (string, error) {
	var args struct {
		FilePath   string `json:"file_path"`
		OldString  string `json:"old_string"`
		NewString  string `json:"new_string"`
		ReplaceAll bool   `json:"replace_all"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("Edit.Execute: %w", err)
	}

	if ok, reason := t.pathGuard.Allow(args.FilePath); !ok {
		return reason, nil
	}

	cleaned := filepath.Clean(args.FilePath)

	info, err := os.Stat(cleaned)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "File not found: " + cleaned + ". Edit can only modify existing files; use Write to create new ones.", nil
		}
		return fmt.Sprintf("Cannot access %s: %v", cleaned, err), nil
	}
	if info.IsDir() {
		return "Path is a directory, not a file: " + cleaned, nil
	}

	state, hasState := reqctxpkg.GetAgentState(ctx)
	if !hasState {
		return "Cannot verify Read-first guard: agent state missing. Read the file first.", nil
	}
	seenSize, seen := state.WasRead(cleaned)
	if !seen {
		return "File must be read first before editing: " + cleaned + ". Use the Read tool first.", nil
	}

	// Size-only external-modification check; same-size content swaps slip through (v1 trade-off).
	// 仅 size 的外部修改检测；同 size 内容互换会漏（v1 取舍）。
	if info.Size() != seenSize {
		return fmt.Sprintf(
			"File has been modified since last read (current size %d, expected %d): %s. Read it again before editing.",
			info.Size(), seenSize, cleaned,
		), nil
	}

	raw, err := os.ReadFile(cleaned)
	if err != nil {
		return fmt.Sprintf("Cannot read %s: %v", cleaned, err), nil
	}
	content := string(raw)

	occurrences := strings.Count(content, args.OldString)
	switch {
	case occurrences == 0:
		return "old_string not found in the file. Verify the exact text (whitespace and case matter).", nil
	case occurrences > 1 && !args.ReplaceAll:
		return fmt.Sprintf(
			"Found %d matches of old_string in %s, but replace_all is false. Either provide more surrounding context to make old_string unique, or set replace_all: true.",
			occurrences, cleaned,
		), nil
	}

	var newContent string
	var replaced int
	if args.ReplaceAll {
		newContent = strings.ReplaceAll(content, args.OldString, args.NewString)
		replaced = occurrences
	} else {
		newContent = strings.Replace(content, args.OldString, args.NewString, 1)
		replaced = 1
	}

	// Atomic write: tmp + rename, preserve original mode.
	parent := filepath.Dir(cleaned)
	tmpFile, err := os.CreateTemp(parent, ".forgify-edit-*")
	if err != nil {
		return fmt.Sprintf("Edit failed (cannot create temp): %v", err), nil
	}
	tmpPath := tmpFile.Name()
	cleanup := func() { _ = os.Remove(tmpPath) }

	if _, err := tmpFile.WriteString(newContent); err != nil {
		_ = tmpFile.Close()
		cleanup()
		return fmt.Sprintf("Edit failed (writing temp): %v", err), nil
	}
	if err := tmpFile.Close(); err != nil {
		cleanup()
		return fmt.Sprintf("Edit failed (closing temp): %v", err), nil
	}
	if err := os.Chmod(tmpPath, info.Mode().Perm()); err != nil {
		cleanup()
		return fmt.Sprintf("Edit failed (chmod temp): %v", err), nil
	}
	if err := os.Rename(tmpPath, cleaned); err != nil {
		cleanup()
		return fmt.Sprintf("Edit failed (rename to target): %v", err), nil
	}

	state.MarkRead(cleaned, int64(len(newContent)))

	if replaced == 1 {
		return fmt.Sprintf("Replaced 1 occurrence in %s.", cleaned), nil
	}
	return fmt.Sprintf("Replaced %d occurrences in %s.", replaced, cleaned), nil
}

var _ toolapp.Tool = (*Edit)(nil)
