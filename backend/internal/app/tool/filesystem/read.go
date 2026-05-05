// read.go — Read system tool: reads a file from the local filesystem and
// returns its contents formatted with cat -n line numbers.
//
// v1 scope: plain text files only. Image / PDF / Jupyter notebook support
// (mentioned in CC's description) is deferred.
//
// read.go — Read 系统工具：读本地文件并以 cat -n 行号格式返回内容。
//
// v1 范围：仅纯文本文件。CC description 提到的图片 / PDF / Jupyter notebook
// 支持暂未实现。
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

// ── Defaults & limits ─────────────────────────────────────────────────────────

const (
	// defaultLimit caps the number of lines returned when the LLM does not
	// specify `limit`. Matches CC's documented 2000.
	//
	// defaultLimit 是 LLM 不指定 limit 时返回的行数上限。匹配 CC 公开的 2000。
	defaultLimit = 2000

	// defaultOffset starts reading from line 1 (cat -n is 1-based).
	//
	// defaultOffset 从第 1 行开始（cat -n 是 1-based）。
	defaultOffset = 1

	// maxScannerLineBytes caps a single line's byte length. Beyond this
	// bufio.Scanner returns an error rather than silently truncating; we
	// surface that to the LLM. 8 MiB covers minified JS, JSON dumps, etc.
	//
	// maxScannerLineBytes 限制单行字节长度。超出后 bufio.Scanner 报错而不是
	// 静默截断；我们把错误暴露给 LLM。8 MiB 够 minified JS / JSON dump 等。
	maxScannerLineBytes = 8 * 1024 * 1024
)

// ── Validation sentinels ──────────────────────────────────────────────────────

var (
	// ErrEmptyFilePath: file_path missing or empty.
	// ErrEmptyFilePath：file_path 缺失或为空。
	ErrEmptyFilePath = errors.New("file_path is required")

	// ErrPathNotAbsolute: file_path must be absolute (starts with /).
	// ErrPathNotAbsolute：file_path 必须是绝对路径。
	ErrPathNotAbsolute = errors.New("file_path must be an absolute path (e.g. /Users/you/file.txt)")

	// ErrNegativeOffset: offset must be ≥ 0 (0 = use default).
	// ErrNegativeOffset：offset 必须 ≥ 0（0 = 使用默认）。
	ErrNegativeOffset = errors.New("offset must be non-negative")

	// ErrNegativeLimit: limit must be ≥ 0 (0 = use default).
	// ErrNegativeLimit：limit 必须 ≥ 0（0 = 使用默认）。
	ErrNegativeLimit = errors.New("limit must be non-negative")
)

// ── Description & schema (LLM-facing) ─────────────────────────────────────────

// readDescription is the text shown to the LLM. v1 omits image/PDF/notebook
// claims since those are not yet implemented; lying here would only confuse
// the model. CC's wording is preserved for the parts we do implement.
//
// readDescription 是给 LLM 的描述文本。v1 不写图片/PDF/notebook 相关条目
// （未实现）；写了反而误导模型。已实现部分保留 CC 的措辞。
const readDescription = `Reads a file from the local filesystem.

Assume this tool is able to read most files on the machine. If the User provides a path to a file assume that path is valid. It is okay to read a file that does not exist; an error message will be returned.

Usage:
- The file_path parameter must be an absolute path, not a relative path
- By default, it reads up to 2000 lines starting from the beginning of the file
- When you already know which part of the file you need, only read that part using offset and limit. This can be important for larger files
- Results are returned using cat -n format (5-digit right-padded line number, tab, content), with line numbers starting at 1
- This tool can only read files, not directories. To list files in a directory, use the Glob tool with pattern "*"
- If you read a file that exists but has empty contents you will receive a system reminder warning in place of file contents
- Some sensitive paths (system directories, credential locations like ~/.ssh) are blocked for safety; you will receive a denial message if you try to read one`

// readSchema is the LLM-facing JSON Schema (without the framework-injected
// summary / destructive / execution_group fields).
//
// readSchema 是给 LLM 的 JSON Schema（不含 framework 注入的 summary /
// destructive / execution_group 字段）。
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

// ── Tool struct & 9 methods ───────────────────────────────────────────────────

// Read implements the Read system tool.
//
// Read struct 是 Read 系统工具。pathGuard 是路径黑名单守卫；AgentState 通过
// ctx 注入（不持有为字段，保持 stateless 跟现有 forge tool 一致）。
type Read struct {
	pathGuard pathguardpkg.PathGuard
}

// Identity --------------------------------------------------------------------

func (t *Read) Name() string                { return "Read" }
func (t *Read) Description() string         { return readDescription }
func (t *Read) Parameters() json.RawMessage { return readSchema }

// Static metadata -------------------------------------------------------------

func (t *Read) IsReadOnly() bool        { return true }
func (t *Read) NeedsReadFirst() bool    { return false }
func (t *Read) RequiresWorkspace() bool { return true }

// Args-dependent hooks --------------------------------------------------------

// ValidateInput checks structural correctness of file_path / offset / limit
// before Execute. Failures become the LLM-facing tool_result error string;
// the LLM should retry with corrected args.
//
// ValidateInput 在 Execute 前校验 file_path / offset / limit 的结构正确性。
// 失败会作为 LLM 看到的 tool_result 错误字符串；LLM 应据此重试。
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

// CheckPermissions always allows. Read is intrinsically safe; access control
// happens via PathGuard inside Execute.
//
// CheckPermissions 始终允许。Read 本质安全；访问控制由 Execute 内的
// PathGuard 完成。
func (t *Read) CheckPermissions(_ json.RawMessage, _ toolapp.PermissionMode) toolapp.PermissionResult {
	return toolapp.PermissionAllow
}

// ── Execute ───────────────────────────────────────────────────────────────────

// Execute reads the file, applies offset/limit, formats with cat -n, and
// stamps the path into AgentState.SeenFiles so subsequent Edit/Write
// invocations on the same path can pass their must-Read-first guard.
//
// Filesystem-level errors (not found, permission denied) are returned as
// LLM-facing strings (not Go errors) so the LLM can recover; only truly
// internal failures (JSON unmarshal of args after ValidateInput passed,
// scanner panic) bubble up as Go errors.
//
// Execute 读文件、应用 offset/limit、cat -n 格式化、并把 path 标进
// AgentState.SeenFiles，让后续对同 path 的 Edit/Write 能通过 must-Read-first
// 守卫。
//
// 文件系统层面的错误（不存在、权限不足）作为 LLM 能看到的字符串返回（不返
// Go error），让 LLM 可以恢复；仅真正的内部失败（ValidateInput 通过后 args
// 解析又失败、scanner panic）才作为 Go error 上抛。
func (t *Read) Execute(ctx context.Context, argsJSON string) (string, error) {
	var args struct {
		FilePath string `json:"file_path"`
		Offset   int    `json:"offset"`
		Limit    int    `json:"limit"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("Read.Execute: %w", err)
	}

	// Normalise defaults.
	// 规范默认值。
	if args.Offset == 0 {
		args.Offset = defaultOffset
	}
	if args.Limit == 0 {
		args.Limit = defaultLimit
	}

	// PathGuard: deny known-sensitive paths.
	// PathGuard：拒绝已知敏感路径。
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

	// Empty file → mark as seen + return system reminder.
	// 空文件 → 标记为已读 + 返回 system reminder。
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
		// Common case: a single line exceeds maxScannerLineBytes.
		// 常见情况：单行超过 maxScannerLineBytes。
		return fmt.Sprintf("Failed to read %s: %v", cleaned, err), nil
	}

	// Truncation hint: only when we hit the limit AND there are more lines.
	// 截断提示：仅在命中 limit 且后面还有内容时追加。
	if written >= args.Limit && hasMoreLines(scanner) {
		fmt.Fprintf(&sb, "... [truncated at line %d; use offset+limit to read more]\n", lastEmittedLine)
	}

	// Mark file as Read in conversation-scoped AgentState so Edit/Write
	// can pass the must-Read-first guard later. Missing AgentState in ctx
	// means the chat layer didn't wire it (server-side bug); we don't fail
	// the read for that — the file content is still useful, just future
	// Edit/Write of this path will be re-asked.
	//
	// 在 conversation 级 AgentState 标记此文件已 Read，让 Edit/Write 后续能
	// 通过 must-Read-first 守卫。ctx 缺 AgentState 意味着 chat 层未注入
	// （服务端 bug）；我们不为此让 Read 失败——文件内容仍有用，只是后续对
	// 该 path 的 Edit/Write 会被要求重新 Read。
	markSeen(ctx, cleaned, info.Size())

	return sb.String(), nil
}

// ── Helpers ───────────────────────────────────────────────────────────────────

// statErrorMessage maps os errors to LLM-friendly messages.
//
// statErrorMessage 把 os 错误映射成 LLM 友好消息。
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

// hasMoreLines peeks one more Scan to decide whether to emit the truncation
// hint. The scanner is consumed regardless; any further reads after this
// would only return false. Safe to call after the truncation-causing break.
//
// hasMoreLines 多扫一行判断是否还有后续行——以决定是否输出截断提示。
// 调用后 scanner 被消耗；进一步读取会返 false。可在截断 break 后安全调用。
func hasMoreLines(scanner *bufio.Scanner) bool {
	return scanner.Scan()
}

// markSeen records the file in AgentState.SeenFiles when present in ctx.
// No-op when AgentState is missing (chat layer did not inject) — Read still
// succeeds, but future Edit/Write of this path will need a fresh Read.
//
// markSeen 在 ctx 含 AgentState 时把文件记进 SeenFiles。
// AgentState 缺失（chat 层未注入）时 no-op——Read 仍成功，但后续对该 path
// 的 Edit/Write 需重新 Read。
func markSeen(ctx context.Context, path string, size int64) {
	if state, ok := reqctxpkg.GetAgentState(ctx); ok {
		state.MarkRead(path, size)
	}
}

// ── Compile-time checks ───────────────────────────────────────────────────────

var _ toolapp.Tool = (*Read)(nil)
