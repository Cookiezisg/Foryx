package search

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/bmatcuk/doublestar/v4"

	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
	pathguardpkg "github.com/sunweilin/forgify/backend/internal/pkg/pathguard"
)


const (
	defaultGlobLimit = 100
	maxGlobLimit     = 1000
)


const globDescription = `File finder: matches glob patterns, returns JSON with type / size / mtime per entry. Use this for path-only listings; use Grep for content search.

Usage:
- Supports any glob pattern, including ` + "`**`" + ` for recursive descent (e.g. "**/*.go", "src/**/*.tsx", "*.md").
- Pass pattern "*" with a directory ` + "`path`" + ` to list immediate children — Glob covers what a separate LS tool would.
- Output JSON: {"root", "matches":[{"path","type","size","mtime"}], "total", "truncated"}.
- ` + "`type`" + ` is "file", "dir", or "symlink"; ` + "`mtime`" + ` is RFC 3339.
- Matches are sorted mtime-descending so recently-edited files surface first.
- ` + "`path`" + ` (search root) defaults to the current working directory; must be absolute when provided.
- ` + "`limit`" + ` caps results (default 100, hard max 1000); the ` + "`truncated`" + ` flag indicates more matches exist.
- Sensitive paths are blocked.`

var globSchema = json.RawMessage(`{
	"type": "object",
	"required": ["pattern"],
	"properties": {
		"pattern": {
			"type": "string",
			"description": "Glob pattern (e.g. \"**/*.go\", \"src/**/*.tsx\", \"*.md\"). Use \"*\" with a directory path to list immediate children."
		},
		"path": {
			"type": "string",
			"description": "Search root (absolute path). Defaults to the current working directory."
		},
		"limit": {
			"type": "number",
			"description": "Max matches to return (default 100, hard max 1000). The truncated flag in the response indicates whether more matches existed."
		}
	}
}`)


type globArgs struct {
	Pattern string `json:"pattern"`
	Path    string `json:"path"`
	Limit   int    `json:"limit"`
}

// normalize fills cwd default for Path and applies the limit caps.
//
// normalize 把 Path 缺省补 cwd 并对 Limit 做默认/硬上限处理。
func (a *globArgs) normalize() {
	if a.Path == "" {
		if cwd, err := os.Getwd(); err == nil {
			a.Path = cwd
		}
	}
	if a.Limit == 0 {
		a.Limit = defaultGlobLimit
	}
	if a.Limit > maxGlobLimit {
		a.Limit = maxGlobLimit
	}
}


type globMatch struct {
	Path  string    `json:"path"`
	Type  string    `json:"type"`
	Size  int64     `json:"size"`
	MTime time.Time `json:"mtime"`
}

type globResult struct {
	Root      string      `json:"root"`
	Matches   []globMatch `json:"matches"`
	Total     int         `json:"total"`
	Truncated bool        `json:"truncated"`
}


// Glob implements the Glob system tool.
//
// Glob 是 Glob 系统工具的实现。
type Glob struct {
	pathGuard pathguardpkg.PathGuard
}

func (t *Glob) Name() string                { return "Glob" }
func (t *Glob) Description() string         { return globDescription }
func (t *Glob) Parameters() json.RawMessage { return globSchema }

func (t *Glob) IsReadOnly() bool        { return true }
func (t *Glob) NeedsReadFirst() bool    { return false }
func (t *Glob) RequiresWorkspace() bool { return true }

// ValidateInput rejects empty patterns, relative paths, and negative limits; pattern syntax errors deferred to Execute.
//
// ValidateInput 拒绝空 pattern / 相对 path / 负 limit；pattern 语法错留给 Execute 报。
func (t *Glob) ValidateInput(args json.RawMessage) error {
	var a globArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return fmt.Errorf("Glob.ValidateInput: %w", err)
	}
	if a.Pattern == "" {
		return ErrEmptyPattern
	}
	if a.Path != "" && !filepath.IsAbs(a.Path) {
		return errors.New("path must be absolute when provided")
	}
	if a.Limit < 0 {
		return errors.New("limit must be non-negative")
	}
	return nil
}

func (t *Glob) CheckPermissions(_ json.RawMessage, _ toolapp.PermissionMode) toolapp.PermissionResult {
	return toolapp.PermissionAllow
}


// Execute runs doublestar.Glob over os.DirFS, stats matches, sorts mtime-desc, caps to limit.
//
// Execute 用 doublestar.Glob 在 os.DirFS 上跑，stat 每项，按 mtime 降序并截断。
func (t *Glob) Execute(ctx context.Context, argsJSON string) (string, error) {
	var args globArgs
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("Glob.Execute: %w", err)
	}
	args.normalize()

	if ok, reason := t.pathGuard.Allow(args.Path); !ok {
		return reason, nil
	}

	root := filepath.Clean(args.Path)
	info, err := os.Stat(root)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "Search root not found: " + root, nil
		}
		return fmt.Sprintf("Cannot access %s: %v", root, err), nil
	}
	if !info.IsDir() {
		return "Search root must be a directory: " + root, nil
	}

	pattern := filepath.ToSlash(args.Pattern)
	relMatches, err := doublestar.Glob(os.DirFS(root), pattern)
	if err != nil {
		return fmt.Sprintf("Invalid glob pattern %q: %v", args.Pattern, err), nil
	}

	matches := make([]globMatch, 0, len(relMatches))
	for _, rel := range relMatches {
		if ctx.Err() != nil {
			break
		}
		full := filepath.Join(root, rel)
		st, err := os.Lstat(full)
		if err != nil {
			continue
		}
		matches = append(matches, globMatch{
			Path:  full,
			Type:  classifyType(st),
			Size:  st.Size(),
			MTime: st.ModTime(),
		})
	}

	sort.Slice(matches, func(i, j int) bool {
		if matches[i].MTime.Equal(matches[j].MTime) {
			return matches[i].Path < matches[j].Path
		}
		return matches[i].MTime.After(matches[j].MTime)
	})

	total := len(matches)
	truncated := false
	if total > args.Limit {
		matches = matches[:args.Limit]
		truncated = true
	}

	out := globResult{
		Root:      root,
		Matches:   matches,
		Total:     total,
		Truncated: truncated,
	}
	body, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return "", fmt.Errorf("Glob.Execute: marshal result: %w", err)
	}
	return string(body), nil
}

func classifyType(st os.FileInfo) string {
	mode := st.Mode()
	switch {
	case mode&os.ModeSymlink != 0:
		return "symlink"
	case mode.IsDir():
		return "dir"
	default:
		return "file"
	}
}


var _ toolapp.Tool = (*Glob)(nil)
