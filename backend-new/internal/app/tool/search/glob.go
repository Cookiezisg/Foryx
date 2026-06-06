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
	"strings"
	"time"

	"github.com/bmatcuk/doublestar/v4"

	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
	fspathpkg "github.com/sunweilin/forgify/backend/internal/pkg/fspath"
	pathguardpkg "github.com/sunweilin/forgify/backend/internal/pkg/pathguard"
)

const (
	defaultGlobLimit = 100
	maxGlobLimit     = 1000
)

const globDescription = `Find files by glob pattern (supports ** recursion) under a root, sorted by mtime desc; returns JSON {root,matches:[{path,type,size,mtime}],total,truncated}. Root is required (absolute or ~). Narrow the root first (LS to look around) rather than globbing all of ~ — that scans huge trees. Use Grep for content.`

var globSchema = json.RawMessage(`{
	"type": "object",
	"required": ["pattern", "path"],
	"properties": {
		"pattern": {
			"type": "string",
			"description": "Glob pattern (e.g. \"**/*.go\", \"src/**/*.tsx\", \"*.md\"). Use \"*\" to match a directory's immediate children, or prefer LS to list a directory."
		},
		"path": {
			"type": "string",
			"description": "Search root: absolute path or ~ (e.g. \"~/projects\"). Required — the agent has no current directory. Keep this narrow; do not glob the whole home dir."
		},
		"limit": {
			"type": "number",
			"description": "Max matches to return (default 100, hard max 1000). The truncated flag indicates whether more existed."
		}
	}
}`)

type globArgs struct {
	Pattern string `json:"pattern"`
	Path    string `json:"path"`
	Limit   int    `json:"limit"`
}

// normalize applies the limit default and hard cap. Path has no default — there
// is no current directory; the agent must pass a root.
//
// normalize 套 limit 默认与硬上限。Path 无默认——没有当前目录,agent 必须传根。
func (a *globArgs) normalize() {
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

// Glob finds files by name pattern under a root — the find-by-name primitive.
//
// Glob 在某根下按名字模式找文件——按名字找的原语。
type Glob struct {
	pathGuard pathguardpkg.PathGuard
}

func (t *Glob) Name() string                { return "Glob" }
func (t *Glob) Description() string         { return globDescription }
func (t *Glob) Parameters() json.RawMessage { return globSchema }

// ValidateInput requires non-empty pattern and path, non-negative limit; pattern
// syntax errors and ~ / absolute resolution are deferred to Execute.
//
// ValidateInput 要求 pattern / path 非空、limit 非负;pattern 语法错与 ~ / 绝对解析留 Execute。
func (t *Glob) ValidateInput(args json.RawMessage) error {
	var a globArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return fmt.Errorf("Glob.ValidateInput: %w", err)
	}
	if a.Pattern == "" {
		return ErrEmptyPattern
	}
	if strings.TrimSpace(a.Path) == "" {
		return ErrPathRequired
	}
	if a.Limit < 0 {
		return errors.New("limit must be non-negative")
	}
	return nil
}

// Execute resolves the root via fspath.Expand, runs doublestar.Glob over os.DirFS,
// stats matches, sorts mtime-desc, caps to limit, returns JSON.
//
// Execute 经 fspath.Expand 解析根,在 os.DirFS 上跑 doublestar.Glob,stat 每项,
// 按 mtime 降序并截断,返 JSON。
func (t *Glob) Execute(ctx context.Context, argsJSON string) (string, error) {
	var args globArgs
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("Glob.Execute: %w", err)
	}
	args.normalize()

	root, err := fspathpkg.Expand(args.Path)
	if err != nil {
		return err.Error(), nil
	}
	if ok, reason := t.pathGuard.Allow(root); !ok {
		return reason, nil
	}

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
			Type:  entryKind(st.Mode()),
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

var _ toolapp.Tool = (*Glob)(nil)
