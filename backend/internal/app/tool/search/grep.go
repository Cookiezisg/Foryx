// grep.go — Grep system tool: regex content search across files. Two
// backends:
//
//   - rg (ripgrep) when present in PATH — full feature set + fast.
//   - stdlib bufio+regexp fallback when rg is missing — same feature
//     surface but slower on large trees.
//
// Backend chosen at construction time (newGrep in search.go); rg path is
// cached on the struct so each Execute does not re-look-up.
//
// grep.go — Grep 系统工具：跨文件 regex 内容搜索。两个后端：
//
//   - rg（ripgrep）在 PATH 上时使用——全特性 + 快。
//   - stdlib bufio+regexp fallback 当 rg 缺失时——同样的特性面，
//     大树上较慢。
//
// 后端在构造时选定（search.go 的 newGrep）；rg 路径缓存在 struct，每次
// Execute 不重查。
package search

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go.uber.org/zap"

	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
	pathguardpkg "github.com/sunweilin/forgify/backend/internal/pkg/pathguard"
)

// ── Validation sentinels ──────────────────────────────────────────────────────

var (
	// ErrEmptyPattern: pattern missing or empty.
	// ErrEmptyPattern：pattern 缺失或为空。
	ErrEmptyPattern = errors.New("pattern is required and must be non-empty")

	// ErrInvalidOutputMode: output_mode is not one of the supported enum.
	// ErrInvalidOutputMode：output_mode 不在支持的枚举内。
	ErrInvalidOutputMode = errors.New(`output_mode must be one of "content", "files_with_matches", "count"`)
)

// ── Output mode constants ─────────────────────────────────────────────────────

const (
	OutputModeContent          = "content"
	OutputModeFilesWithMatches = "files_with_matches"
	OutputModeCount            = "count"
)

// ── Description & schema (LLM-facing) ─────────────────────────────────────────

const grepDescription = `A powerful content search tool, backed by ripgrep when available (fast) or a stdlib regex fallback when not.

Usage:
- ALWAYS use Grep for content search tasks. NEVER invoke ` + "`grep`" + ` or ` + "`rg`" + ` as a Bash command — Grep is optimized for safe path access.
- Supports full regex syntax (e.g. "log.*Error", "function\s+\w+")
- Filter files with the ` + "`glob`" + ` parameter (e.g. "*.go", "**/*.tsx") or the ` + "`type`" + ` parameter (e.g. "go", "py", "js")
- Output modes: "content" shows matching lines (use -n for line numbers, -A/-B/-C for context), "files_with_matches" returns matching file paths only (default; cheapest), "count" returns one path:N line per matching file.
- Pattern syntax: full RE2 (Go) / PCRE-ish (ripgrep). Literal braces need escaping: ` + "`interface\\{\\}`" + ` to find ` + "`interface{}`" + ` in Go code.
- Multiline: by default patterns match within single lines only. For cross-line patterns like ` + "`struct \\{[\\s\\S]*?field`" + `, set ` + "`multiline: true`" + `.
- ` + "`-i`" + ` for case-insensitive. ` + "`head_limit`" + ` caps the result list.
- The ` + "`path`" + ` parameter (file or directory) must be absolute when provided. Defaults to current working directory.
- Sensitive paths (system dirs, credential locations like ~/.ssh) are blocked for safety.`

// grepSchema is the LLM-facing JSON Schema. Field names mirror ripgrep CLI
// flags exactly (-A / -B / -C / -n / -i) — unusual JSON Schema style but
// matches Claude Code's tool surface, so LLM intuition transfers.
//
// grepSchema 是 LLM 面 JSON Schema。字段名精确镜像 ripgrep CLI flag
// （-A/-B/-C/-n/-i）——unusual JSON Schema 风格但与 Claude Code 对齐，
// 让 LLM 直觉直接迁移。
var grepSchema = json.RawMessage(`{
	"type": "object",
	"required": ["pattern"],
	"properties": {
		"pattern": {
			"type": "string",
			"description": "The regex pattern to search for. Use full regex syntax. Literal braces need escaping (e.g. \"interface\\\\{\\\\}\")."
		},
		"path": {
			"type": "string",
			"description": "File or directory to search in (absolute path). Defaults to current working directory."
		},
		"glob": {
			"type": "string",
			"description": "Glob pattern to filter files (e.g. \"*.go\", \"**/*.tsx\"). Combine with type for narrower scope."
		},
		"type": {
			"type": "string",
			"description": "Language type filter (e.g. \"go\", \"py\", \"js\", \"ts\", \"rust\"). Maps to extensions internally."
		},
		"output_mode": {
			"type": "string",
			"enum": ["content", "files_with_matches", "count"],
			"default": "files_with_matches",
			"description": "How to present matches: content (lines), files_with_matches (paths only, default, cheapest), count (path:N per file)."
		},
		"-A": {
			"type": "number",
			"description": "Lines of trailing context after each match (content mode only)."
		},
		"-B": {
			"type": "number",
			"description": "Lines of leading context before each match (content mode only)."
		},
		"-C": {
			"type": "number",
			"description": "Lines of context around each match (sets both -A and -B; content mode only)."
		},
		"-n": {
			"type": "boolean",
			"description": "Show line numbers in content mode."
		},
		"-i": {
			"type": "boolean",
			"description": "Case-insensitive matching."
		},
		"multiline": {
			"type": "boolean",
			"default": false,
			"description": "Allow patterns to match across line boundaries. Required for patterns containing \\n or [\\s\\S]."
		},
		"head_limit": {
			"type": "number",
			"description": "Cap result to first N matches (content mode) or N files (files_with_matches/count modes)."
		}
	}
}`)

// ── Args ──────────────────────────────────────────────────────────────────────

// grepArgs is the decoded form of the LLM-supplied parameters. Struct tags
// use the schema's literal field names (including the dashy ones).
//
// grepArgs 是 LLM 传入参数的解码形式。struct tag 用 schema 的字面字段名
// （含带短横线的）。
type grepArgs struct {
	Pattern    string `json:"pattern"`
	Path       string `json:"path"`
	Glob       string `json:"glob"`
	Type       string `json:"type"`
	OutputMode string `json:"output_mode"`
	After      int    `json:"-A"`
	Before     int    `json:"-B"`
	Around     int    `json:"-C"`
	ShowLines  bool   `json:"-n"`
	IgnoreCase bool   `json:"-i"`
	Multiline  bool   `json:"multiline"`
	HeadLimit  int    `json:"head_limit"`
}

// normalize fills in defaults and resolves -C into -A/-B (content mode).
//
// normalize 补默认值，把 -C 化解为 -A/-B（content 模式）。
func (a *grepArgs) normalize() {
	if a.OutputMode == "" {
		a.OutputMode = OutputModeFilesWithMatches
	}
	if a.Path == "" {
		if cwd, err := os.Getwd(); err == nil {
			a.Path = cwd
		}
	}
	if a.Around > 0 {
		if a.Before == 0 {
			a.Before = a.Around
		}
		if a.After == 0 {
			a.After = a.Around
		}
	}
}

// ── Tool struct & 9 methods ───────────────────────────────────────────────────

// Grep implements the Grep system tool.
//
// Grep struct 是 Grep 系统工具。pathGuard 守卫敏感路径；rgPath 非空时
// shell out 到 ripgrep，否则用 stdlib fallback。
type Grep struct {
	pathGuard pathguardpkg.PathGuard
	rgPath    string // empty = use stdlib fallback
	log       *zap.Logger
}

// Identity --------------------------------------------------------------------

func (t *Grep) Name() string                { return "Grep" }
func (t *Grep) Description() string         { return grepDescription }
func (t *Grep) Parameters() json.RawMessage { return grepSchema }

// Static metadata -------------------------------------------------------------

func (t *Grep) IsReadOnly() bool        { return true }
func (t *Grep) NeedsReadFirst() bool    { return false }
func (t *Grep) RequiresWorkspace() bool { return true }

// Args-dependent hooks --------------------------------------------------------

// ValidateInput checks the pattern is non-empty, output_mode is in enum, and
// numeric fields are non-negative. Pattern compilation is deferred to
// Execute (the rg backend won't compile via Go's regexp at all).
//
// ValidateInput 校验 pattern 非空、output_mode 合法、数字字段非负。pattern
// 编译延后到 Execute（rg 后端根本不走 Go regexp 编译）。
func (t *Grep) ValidateInput(args json.RawMessage) error {
	var a grepArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return fmt.Errorf("Grep.ValidateInput: %w", err)
	}
	if strings.TrimSpace(a.Pattern) == "" {
		return ErrEmptyPattern
	}
	if a.OutputMode != "" &&
		a.OutputMode != OutputModeContent &&
		a.OutputMode != OutputModeFilesWithMatches &&
		a.OutputMode != OutputModeCount {
		return ErrInvalidOutputMode
	}
	if a.After < 0 || a.Before < 0 || a.Around < 0 || a.HeadLimit < 0 {
		return errors.New("-A / -B / -C / head_limit must be non-negative")
	}
	if a.Path != "" && !filepath.IsAbs(a.Path) {
		return errors.New("path must be absolute when provided")
	}
	return nil
}

// CheckPermissions always allows. Path safety enforced via PathGuard inside
// Execute.
//
// CheckPermissions 始终允许。路径安全靠 Execute 内的 PathGuard。
func (t *Grep) CheckPermissions(_ json.RawMessage, _ toolapp.PermissionMode) toolapp.PermissionResult {
	return toolapp.PermissionAllow
}

// ── Execute ───────────────────────────────────────────────────────────────────

// Execute dispatches to the rg or stdlib backend depending on rgPath. Both
// backends share the same args / output / cap semantics so the LLM sees
// identical behavior regardless of which one ran.
//
// Execute 按 rgPath 是否非空分派到 rg 或 stdlib 后端。两后端共享 args /
// 输出 / cap 语义，让 LLM 看到一致行为。
func (t *Grep) Execute(ctx context.Context, argsJSON string) (string, error) {
	var args grepArgs
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("Grep.Execute: %w", err)
	}
	args.normalize()

	if ok, reason := t.pathGuard.Allow(args.Path); !ok {
		return reason, nil
	}

	cleaned := filepath.Clean(args.Path)
	info, err := os.Stat(cleaned)
	if err != nil {
		if os.IsNotExist(err) {
			return "Search root not found: " + cleaned, nil
		}
		return fmt.Sprintf("Cannot access %s: %v", cleaned, err), nil
	}
	args.Path = cleaned

	if t.rgPath != "" {
		out, err := t.execRg(ctx, args)
		// rg failed for unexpected reason — fall back to stdlib so the
		// search still succeeds (per pkg doc, stdlib has same surface).
		// Log loudly so operator sees rg backend rotting + the fallback
		// path becoming the de-facto backend (PCRE-only patterns silently
		// produce different results without this audit trail). Same
		// defect class lesson as B2 bash auto-route silent fallback fix.
		//
		// rg 异常失败 → fallback 到 stdlib（包文档保两后端同 surface）。
		// 高声 log 让 operator 看到 rg 后端腐化 / fallback 变事实后端
		// （PCRE-only 模式悄悄产不同结果无审计）。同 B2 bash auto-route
		// silent fallback 经验。
		if err != nil {
			t.log.Warn("Grep: rg backend failed; falling through to stdlib (results may differ for PCRE-only patterns)",
				zap.String("rg_path", t.rgPath), zap.Error(err))
			return t.execStdlib(ctx, args, info.IsDir())
		}
		return out, nil
	}
	return t.execStdlib(ctx, args, info.IsDir())
}

// ── Compile-time checks ───────────────────────────────────────────────────────

var _ toolapp.Tool = (*Grep)(nil)
