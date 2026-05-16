package filesystem

import (
	"bufio"
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

const (
	defaultLimit  = 2000
	defaultOffset = 1
	// maxScannerLineBytes caps a single line; bufio.Scanner errors past this rather than truncating.
	//
	// maxScannerLineBytes 单行字节上限；bufio.Scanner 超出会报错而非静默截断。
	maxScannerLineBytes = 8 * 1024 * 1024
)

var (
	// ErrEmptyFilePath: file_path missing or empty.
	//
	// ErrEmptyFilePath：file_path 缺失或为空。
	ErrEmptyFilePath = errors.New("file_path is required")

	// ErrPathNotAbsolute: file_path must be absolute.
	//
	// ErrPathNotAbsolute：file_path 必须是绝对路径。
	ErrPathNotAbsolute = errors.New("file_path must be an absolute path (e.g. /Users/you/file.txt)")

	// ErrNegativeOffset: offset must be ≥ 0.
	//
	// ErrNegativeOffset：offset 必须 ≥ 0。
	ErrNegativeOffset = errors.New("offset must be non-negative")

	// ErrNegativeLimit: limit must be ≥ 0.
	//
	// ErrNegativeLimit：limit 必须 ≥ 0。
	ErrNegativeLimit = errors.New("limit must be non-negative")
)

const readDescription = `Reads a file from the local filesystem.

Usage:
- file_path must be an absolute path.
- Reads up to 2000 lines from the start by default; use offset+limit to page through larger files.
- Output uses cat -n format (5-digit right-padded line number, tab, content), 1-based.
- Only reads files; for directory listing use Glob with pattern "*".
- An empty file returns a system reminder; a missing file returns an error message.
- Sensitive paths (system directories, credential locations) are blocked.`

var readSchema = json.RawMessage(`{
	"type": "object",
	"required": ["file_path"],
	"properties": {
		"file_path": {
			"type": "string",
			"description": "The absolute path to the file to read"
		},
		"offset": {
			"type": "number",
			"description": "The line number to start reading from (1-based; default 1). Only provide if the file is too large to read at once."
		},
		"limit": {
			"type": "number",
			"description": "The number of lines to read (default 2000). Only provide if the file is too large to read at once."
		}
	}
}`)

// Read implements the Read system tool.
//
// Read 是 Read 系统工具的实现。
type Read struct {
	pathGuard pathguardpkg.PathGuard
}

func (t *Read) Name() string                { return "Read" }
func (t *Read) Description() string         { return readDescription }
func (t *Read) Parameters() json.RawMessage { return readSchema }

func (t *Read) IsReadOnly() bool        { return true }
func (t *Read) NeedsReadFirst() bool    { return false }
func (t *Read) RequiresWorkspace() bool { return true }

// ValidateInput checks structural correctness of file_path / offset / limit.
//
// ValidateInput 校验 file_path / offset / limit 的结构正确性。
func (t *Read) ValidateInput(args json.RawMessage) error {
	var a struct {
		FilePath string `json:"file_path"`
		Offset   int    `json:"offset"`
		Limit    int    `json:"limit"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return fmt.Errorf("Read.ValidateInput: %w", err)
	}
	if strings.TrimSpace(a.FilePath) == "" {
		return ErrEmptyFilePath
	}
	if !filepath.IsAbs(a.FilePath) {
		return ErrPathNotAbsolute
	}
	if a.Offset < 0 {
		return ErrNegativeOffset
	}
	if a.Limit < 0 {
		return ErrNegativeLimit
	}
	return nil
}

func (t *Read) CheckPermissions(_ json.RawMessage, _ toolapp.PermissionMode) toolapp.PermissionResult {
	return toolapp.PermissionAllow
}

// Execute reads the file, applies offset/limit, formats with cat -n, and marks the path in AgentState.SeenFiles.
//
// Execute 读文件 / 应用 offset+limit / cat -n 格式化 / 把 path 标进 AgentState.SeenFiles。
func (t *Read) Execute(ctx context.Context, argsJSON string) (string, error) {
	var args struct {
		FilePath string `json:"file_path"`
		Offset   int    `json:"offset"`
		Limit    int    `json:"limit"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("Read.Execute: %w", err)
	}

	if args.Offset == 0 {
		args.Offset = defaultOffset
	}
	if args.Limit == 0 {
		args.Limit = defaultLimit
	}

	if ok, reason := t.pathGuard.Allow(args.FilePath); !ok {
		return reason, nil
	}

	cleaned := filepath.Clean(args.FilePath)

	info, err := os.Stat(cleaned)
	if err != nil {
		return statErrorMessage(cleaned, err), nil
	}
	if info.IsDir() {
		return fmt.Sprintf("Path is a directory, not a file: %s. Use Glob with pattern \"*\" to list a directory.", cleaned), nil
	}

	if info.Size() == 0 {
		markSeen(ctx, cleaned, 0)
		return "<system-reminder>File exists but has empty contents.</system-reminder>", nil
	}

	f, err := os.Open(cleaned)
	if err != nil {
		return statErrorMessage(cleaned, err), nil
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), maxScannerLineBytes)

	var sb strings.Builder
	written := 0
	lastEmittedLine := 0
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		if lineNum < args.Offset {
			continue
		}
		if written >= args.Limit {
			break
		}
		fmt.Fprintf(&sb, "%5d\t%s\n", lineNum, scanner.Text())
		written++
		lastEmittedLine = lineNum
	}
	if err := scanner.Err(); err != nil {
		return fmt.Sprintf("Failed to read %s: %v", cleaned, err), nil
	}

	if written >= args.Limit && hasMoreLines(scanner) {
		fmt.Fprintf(&sb, "... [truncated at line %d; use offset+limit to read more]\n", lastEmittedLine)
	}

	markSeen(ctx, cleaned, info.Size())

	return sb.String(), nil
}

func statErrorMessage(path string, err error) string {
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return "File not found: " + path
	case errors.Is(err, fs.ErrPermission):
		return "Permission denied: " + path
	default:
		return fmt.Sprintf("Cannot access %s: %v", path, err)
	}
}

// hasMoreLines peeks one Scan; the scanner is consumed afterwards.
//
// hasMoreLines 多扫一行；调用后 scanner 即被消耗。
func hasMoreLines(scanner *bufio.Scanner) bool {
	return scanner.Scan()
}

func markSeen(ctx context.Context, path string, size int64) {
	if state, ok := reqctxpkg.GetAgentState(ctx); ok {
		state.MarkRead(path, size)
	}
}

var _ toolapp.Tool = (*Read)(nil)
