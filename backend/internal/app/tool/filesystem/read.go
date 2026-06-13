package filesystem

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"

	errorspkg "github.com/sunweilin/forgify/backend/internal/pkg/errors"
	limitspkg "github.com/sunweilin/forgify/backend/internal/pkg/limits"

	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
	fspathpkg "github.com/sunweilin/forgify/backend/internal/pkg/fspath"
	pathguardpkg "github.com/sunweilin/forgify/backend/internal/pkg/pathguard"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

const (
	defaultOffset = 1

	// maxScannerLineBytes caps a single line; bufio.Scanner errors past this
	// rather than silently truncating. 8 MiB matches what Claude Code uses and
	// is enough for almost any real source file while still bounding memory.
	//
	// maxScannerLineBytes 单行字节上限；bufio.Scanner 超出会报错而非静默截断。
	// 8 MiB 与 Claude Code 一致，足以容纳几乎任何真实源码文件，同时限内存。
	maxScannerLineBytes = 8 * 1024 * 1024
)

// Read tool errors are returned by ValidateInput and surface to the LLM as a
// tool-result string so it can fix its args. These are NOT bubbled through
// domain errmap (S20) — they never reach HTTP.
//
// Read 工具错误由 ValidateInput 返回、以 tool-result 字符串呈现给 LLM 让它修参数。
// 它们不走 domain errmap（S20）——永不到达 HTTP。
var (
	ErrEmptyFilePath  = errorspkg.New(errorspkg.KindInvalid, "FS_EMPTY_FILE_PATH", "file_path is required")
	ErrNegativeOffset = errorspkg.New(errorspkg.KindInvalid, "FS_NEGATIVE_OFFSET", "offset must be non-negative")
	ErrNegativeLimit  = errorspkg.New(errorspkg.KindInvalid, "FS_NEGATIVE_LIMIT", "limit must be non-negative")
)

const readDescription = `Read a file. Absolute path; cat -n output (line-num TAB content). Defaults to first 2000 lines; use offset+limit to page. For directory listing use Glob "*".`

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

// Read is the read-only filesystem tool. It also has a side effect: stamping
// path → size into AgentState.SeenFiles so subsequent Write / Edit can verify
// the file was seen and detect external drift.
//
// Read 是只读文件系统 tool。它有个副作用：把 path → size 盖章进 AgentState.SeenFiles，
// 使后续 Write / Edit 能验证文件已被看过、并检测外部漂移。
type Read struct {
	pathGuard pathguardpkg.PathGuard
}

func (t *Read) Name() string                { return "Read" }
func (t *Read) Description() string         { return readDescription }
func (t *Read) Parameters() json.RawMessage { return readSchema }

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
	if a.Offset < 0 {
		return ErrNegativeOffset
	}
	if a.Limit < 0 {
		return ErrNegativeLimit
	}
	return nil
}

// Execute reads the file under PathGuard.Allow, applies offset/limit, formats
// with cat -n, and stamps the path into AgentState.SeenFiles on success.
// Read-only — AgentState absent is tolerated (just skip the stamp); only Write/Edit
// are fail-closed on missing state.
//
// Execute 在 PathGuard.Allow 下读文件 / 应用 offset+limit / cat -n 格式化，成功后把
// path 盖章进 AgentState.SeenFiles。只读——AgentState 缺失可容忍（跳过盖章）；只有
// Write/Edit 对 state 缺失 fail-closed。
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
		args.Limit = limitspkg.Current().Tools.ReadDefaultLines
	}

	cleaned, err := fspathpkg.Expand(args.FilePath)
	if err != nil {
		return err.Error(), nil
	}
	if ok, reason := t.pathGuard.Allow(cleaned); !ok {
		return reason, nil
	}

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

	// Truncation marker: a successful one-more Scan after hitting the limit
	// means more content exists. This consumes the scanner but we no longer
	// need it.
	//
	// 截断标记：到 limit 后再次 Scan 成功说明有剩余。此调用耗尽 scanner，但此后无需再用。
	if written >= args.Limit && scanner.Scan() {
		fmt.Fprintf(&sb, "... [truncated at line %d; use offset+limit to read more]\n", lastEmittedLine)
	}

	markSeen(ctx, cleaned, info.Size())

	return sb.String(), nil
}

// statErrorMessage maps a stat() error to a human-readable tool-result. The LLM
// reads this string and decides what to do; no error bubbles up.
//
// statErrorMessage 把 stat() 错误映射成人读的 tool-result。LLM 读这段串自行决策；错误不冒泡。
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

// markSeen stamps path → size if AgentState is present; absent is tolerated for
// read-only operations (host may have skipped seeding in non-conversation runs).
//
// markSeen 在 AgentState 在场时盖章 path → size；只读操作容忍缺失（host 在非对话场景可能不 seed）。
func markSeen(ctx context.Context, path string, size int64) {
	if state, ok := reqctxpkg.GetAgentState(ctx); ok {
		state.MarkRead(path, size)
	}
}

var _ toolapp.Tool = (*Read)(nil)
