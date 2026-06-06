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

var (
	ErrEmptyOldString = errors.New("old_string is required and must be non-empty")
	ErrEditNoOp       = errors.New("old_string and new_string must be different")
)

const editDescription = `Exact literal string replace in a file (not regex; whitespace/case matter). Read the file first. old_string must be unique unless replace_all; strip Read's line-num prefix.`

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

// Edit is the in-place literal-substring replacement tool. It runs under four
// guards: PathGuard.AllowWrite, file exists, must-Read-first via AgentState,
// and size match against the size stamped at last Read (cheap external-drift
// detector — same-size content swaps still slip through, accepted v1 trade-off).
//
// Edit 是原地字面量替换 tool。四重守卫：PathGuard.AllowWrite、文件存在、通过 AgentState
// 的写前必读、以及对最近 Read 时盖章 size 的匹配（廉价外部漂移检测——同 size 内容互换
// 仍会漏，v1 接受的取舍）。
type Edit struct {
	pathGuard pathguardpkg.PathGuard
}

func (t *Edit) Name() string                { return "Edit" }
func (t *Edit) Description() string         { return editDescription }
func (t *Edit) Parameters() json.RawMessage { return editSchema }

// ValidateInput rejects empty old_string and the no-op old_string == new_string.
// new_string may be empty (delete-the-matched-text is a legitimate use).
//
// ValidateInput 拒绝空 old_string 和 old_string == new_string 的空操作。
// new_string 可为空（删匹配文本是合法用法）。
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

// Execute applies the literal-substring replacement. Sequence: PathGuard.AllowWrite
// → file exists & is a file → must-Read-first → size-drift check → match count
// (zero / multiple-without-replace_all surface clear LLM-actionable strings) →
// atomic tmp + chmod + rename → re-stamp size into AgentState.
//
// Execute 应用字面量替换。顺序：PathGuard.AllowWrite → 文件存在且为文件 → 写前必读 →
// size 漂移检测 → 命中数（0 / 多匹配无 replace_all 各返清晰可让 LLM 行动的串）→ 原子
// tmp + chmod + rename → 重新盖章 size 进 AgentState。
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

	cleaned, err := fspathpkg.Expand(args.FilePath)
	if err != nil {
		return err.Error(), nil
	}
	if ok, reason := t.pathGuard.AllowWrite(cleaned); !ok {
		return reason, nil
	}

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
