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


var (
	// ErrEmptyPattern: pattern missing or empty.
	//
	// ErrEmptyPattern：pattern 缺失或为空。
	ErrEmptyPattern = errors.New("pattern is required and must be non-empty")

	// ErrInvalidOutputMode: output_mode not in supported enum.
	//
	// ErrInvalidOutputMode：output_mode 不在支持的枚举内。
	ErrInvalidOutputMode = errors.New(`output_mode must be one of "content", "files_with_matches", "count"`)
)


const (
	OutputModeContent          = "content"
	OutputModeFilesWithMatches = "files_with_matches"
	OutputModeCount            = "count"
)


const grepDescription = `Regex content search across files. Returns plain-text lines (mirrors ripgrep's --no-heading format); use Glob for path-only listings.

Usage:
- ALWAYS use Grep for content search. NEVER invoke ` + "`grep`" + ` or ` + "`rg`" + ` via Bash — Grep enforces safe path access.
- Full regex syntax (e.g. "log.*Error", "function\s+\w+"). Literal braces need escaping: ` + "`interface\\{\\}`" + `.
- Filter files with ` + "`glob`" + ` (e.g. "*.go", "**/*.tsx") or ` + "`type`" + ` (e.g. "go", "py", "js").
- Output modes: "content" (matching lines; ` + "`-n`" + ` for line numbers, ` + "`-A`" + `/` + "`-B`" + `/` + "`-C`" + ` for context), "files_with_matches" (paths only, default, cheapest), "count" (path:N per file).
- Multiline: set ` + "`multiline: true`" + ` for patterns crossing line boundaries (e.g. ` + "`struct \\{[\\s\\S]*?field`" + `).
- ` + "`-i`" + ` for case-insensitive; ` + "`head_limit`" + ` caps the result list.
- ` + "`path`" + ` (file or directory) must be absolute when provided; defaults to current working directory.
- Sensitive paths are blocked.`

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


// Grep implements the Grep system tool; rgPath empty → stdlib fallback.
//
// Grep 是 Grep 系统工具的实现；rgPath 为空时用 stdlib fallback。
type Grep struct {
	pathGuard pathguardpkg.PathGuard
	rgPath    string
	log       *zap.Logger
}

func (t *Grep) Name() string                { return "Grep" }
func (t *Grep) Description() string         { return grepDescription }
func (t *Grep) Parameters() json.RawMessage { return grepSchema }

func (t *Grep) IsReadOnly() bool        { return true }
func (t *Grep) NeedsReadFirst() bool    { return false }
func (t *Grep) RequiresWorkspace() bool { return true }

// ValidateInput checks pattern non-empty, output_mode in enum, numeric fields non-negative.
//
// ValidateInput 校验 pattern 非空、output_mode 合法、数字字段非负。
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

func (t *Grep) CheckPermissions(_ json.RawMessage, _ toolapp.PermissionMode) toolapp.PermissionResult {
	return toolapp.PermissionAllow
}

// Execute dispatches to rg or stdlib backend by rgPath; both share args / output / cap semantics.
//
// Execute 按 rgPath 分派到 rg 或 stdlib 后端；两者共享 args /
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
		if err != nil {
			t.log.Warn("Grep: rg backend failed; falling through to stdlib (results may differ for PCRE-only patterns)",
				zap.String("rg_path", t.rgPath), zap.Error(err))
			return t.execStdlib(ctx, args, info.IsDir())
		}
		return out, nil
	}
	return t.execStdlib(ctx, args, info.IsDir())
}


var _ toolapp.Tool = (*Grep)(nil)
