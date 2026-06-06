package search

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"go.uber.org/zap"

	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
	fspathpkg "github.com/sunweilin/forgify/backend/internal/pkg/fspath"
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

	// ErrPathRequired: path missing — there is no current directory to default to.
	//
	// ErrPathRequired：path 缺失——没有当前目录可作默认。
	ErrPathRequired = errors.New("path is required (absolute or ~; the agent has no current directory)")
)

const (
	OutputModeContent          = "content"
	OutputModeFilesWithMatches = "files_with_matches"
	OutputModeCount            = "count"
)

const grepDescription = `Regex content search across files (ripgrep). Never call grep/rg via Bash. Root path required (absolute or ~); narrow it first (LS to look around) rather than grepping all of ~. Filter by glob or type; output_mode files_with_matches (default) | content | count.`

var grepSchema = json.RawMessage(`{
	"type": "object",
	"required": ["pattern", "path"],
	"properties": {
		"pattern": {
			"type": "string",
			"description": "The regex pattern to search for. Use full regex syntax. Literal braces need escaping (e.g. \"interface\\\\{\\\\}\")."
		},
		"path": {
			"type": "string",
			"description": "File or directory to search in: absolute path or ~ (e.g. \"~/projects\"). Required — the agent has no current directory. Keep it narrow."
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

// normalize fills the output_mode default and folds -C into -A/-B. Path has no
// default — there is no current directory; the agent must pass a root.
//
// normalize 补 output_mode 默认、把 -C 折进 -A/-B。Path 无默认——没有当前目录,agent 必须传根。
func (a *grepArgs) normalize() {
	if a.OutputMode == "" {
		a.OutputMode = OutputModeFilesWithMatches
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

// ValidateInput checks pattern + path non-empty, output_mode in enum, numeric fields non-negative.
//
// ValidateInput 校验 pattern + path 非空、output_mode 合法、数字字段非负。
func (t *Grep) ValidateInput(args json.RawMessage) error {
	var a grepArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return fmt.Errorf("Grep.ValidateInput: %w", err)
	}
	if strings.TrimSpace(a.Pattern) == "" {
		return ErrEmptyPattern
	}
	if strings.TrimSpace(a.Path) == "" {
		return ErrPathRequired
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
	return nil
}

// Execute resolves the root via fspath.Expand then dispatches to the rg or stdlib
// backend; both share args / output / cap semantics so the LLM sees consistent
// behaviour regardless of whether ripgrep is installed.
//
// Execute 经 fspath.Expand 解析根,再分派到 rg 或 stdlib 后端;两者共享 args /
// 输出 / cap 语义,使 LLM 不论是否装了 ripgrep 都看到一致行为。
func (t *Grep) Execute(ctx context.Context, argsJSON string) (string, error) {
	var args grepArgs
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("Grep.Execute: %w", err)
	}
	args.normalize()

	abs, err := fspathpkg.Expand(args.Path)
	if err != nil {
		return err.Error(), nil
	}
	if ok, reason := t.pathGuard.Allow(abs); !ok {
		return reason, nil
	}

	info, err := os.Stat(abs)
	if err != nil {
		if os.IsNotExist(err) {
			return "Search root not found: " + abs, nil
		}
		return fmt.Sprintf("Cannot access %s: %v", abs, err), nil
	}
	args.Path = abs

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
