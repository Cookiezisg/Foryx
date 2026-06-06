package search

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"sort"
	"strings"
	"time"

	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
	fspathpkg "github.com/sunweilin/forgify/backend/internal/pkg/fspath"
	pathguardpkg "github.com/sunweilin/forgify/backend/internal/pkg/pathguard"
)

const (
	defaultLSLimit = 200
	maxLSLimit     = 1000
)

const lsDescription = `List a directory's immediate contents (non-recursive), directories first. Absolute path or ~ (e.g. "~/Downloads"). The primitive for looking inside a folder and drilling into subdirectories — like opening it in a file browser. Use Glob to find files by name pattern, Grep to find by content.`

var lsSchema = json.RawMessage(`{
	"type": "object",
	"required": ["path"],
	"properties": {
		"path": {
			"type": "string",
			"description": "Absolute path (or ~ / ~/sub) of the directory to list. There is no current directory — pass a full path."
		},
		"limit": {
			"type": "number",
			"description": "Max entries to return (default 200, hard max 1000). The footer indicates whether more existed."
		}
	}
}`)

type lsArgs struct {
	Path  string `json:"path"`
	Limit int    `json:"limit"`
}

func (a *lsArgs) normalize() {
	if a.Limit == 0 {
		a.Limit = defaultLSLimit
	}
	if a.Limit > maxLSLimit {
		a.Limit = maxLSLimit
	}
}

// LS lists one directory level, directories first — the look-inside-a-folder
// primitive of file navigation.
//
// LS 列一层目录,目录优先——文件导航里"打开文件夹看一眼"的原语。
type LS struct {
	pathGuard pathguardpkg.PathGuard
}

func (t *LS) Name() string                { return "LS" }
func (t *LS) Description() string         { return lsDescription }
func (t *LS) Parameters() json.RawMessage { return lsSchema }

// ValidateInput requires a non-empty path and non-negative limit; ~ / absolute
// resolution is deferred to Execute via fspath.Expand.
//
// ValidateInput 要求 path 非空、limit 非负;~ / 绝对解析留给 Execute 的 fspath.Expand。
func (t *LS) ValidateInput(args json.RawMessage) error {
	var a lsArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return fmt.Errorf("LS.ValidateInput: %w", err)
	}
	if strings.TrimSpace(a.Path) == "" {
		return ErrPathRequired
	}
	if a.Limit < 0 {
		return errors.New("limit must be non-negative")
	}
	return nil
}

type lsEntry struct {
	name  string
	kind  string
	size  int64
	mtime time.Time
	isDir bool
}

// Execute lists abs (a directory) one level deep, directories first then name
// order, capped to limit. Output is line-oriented text for the LLM to read and
// decide what to drill into.
//
// Execute 列 abs(目录)一层,目录优先再按名字序,截断到 limit。输出按行文本供 LLM
// 读取并决定下钻哪个。
func (t *LS) Execute(ctx context.Context, argsJSON string) (string, error) {
	var args lsArgs
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("LS.Execute: %w", err)
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
		if errors.Is(err, fs.ErrNotExist) {
			return "Directory not found: " + abs, nil
		}
		return fmt.Sprintf("Cannot access %s: %v", abs, err), nil
	}
	if !info.IsDir() {
		return "Not a directory (use Read for a file): " + abs, nil
	}

	dirents, err := os.ReadDir(abs)
	if err != nil {
		return fmt.Sprintf("Cannot read directory %s: %v", abs, err), nil
	}

	entries := make([]lsEntry, 0, len(dirents))
	for _, d := range dirents {
		fi, err := d.Info()
		if err != nil {
			continue
		}
		entries = append(entries, lsEntry{
			name:  d.Name(),
			kind:  entryKind(fi.Mode()),
			size:  fi.Size(),
			mtime: fi.ModTime(),
			isDir: fi.IsDir(),
		})
	}

	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].isDir != entries[j].isDir {
			return entries[i].isDir // directories first
		}
		return entries[i].name < entries[j].name
	})

	total := len(entries)
	truncated := false
	if total > args.Limit {
		entries = entries[:args.Limit]
		truncated = true
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "%s (%d entries)\n", abs, total)
	if total == 0 {
		sb.WriteString("  (empty)\n")
		return sb.String(), nil
	}
	for _, e := range entries {
		switch e.kind {
		case "dir":
			fmt.Fprintf(&sb, "  dir   %s\n", e.name)
		case "link":
			fmt.Fprintf(&sb, "  link  %s\n", e.name)
		default:
			fmt.Fprintf(&sb, "  file  %s   %s   %s\n", e.name, humanBytes(e.size), e.mtime.Format("2006-01-02 15:04"))
		}
	}
	if truncated {
		fmt.Fprintf(&sb, "  ... showing %d of %d entries; raise limit to see more\n", args.Limit, total)
	}
	return sb.String(), nil
}

// entryKind classifies a file mode as dir / link / file. Shared with Glob.
//
// entryKind 把文件 mode 分类成 dir / link / file。与 Glob 共用。
func entryKind(mode fs.FileMode) string {
	switch {
	case mode&fs.ModeSymlink != 0:
		return "link"
	case mode.IsDir():
		return "dir"
	default:
		return "file"
	}
}

// humanBytes renders a byte count as a compact human-readable string (B/KB/MB/…).
//
// humanBytes 把字节数渲染成紧凑的人读字符串(B/KB/MB/…)。
func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for nn := n / unit; nn >= unit; nn /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGTPE"[exp])
}

var _ toolapp.Tool = (*LS)(nil)
