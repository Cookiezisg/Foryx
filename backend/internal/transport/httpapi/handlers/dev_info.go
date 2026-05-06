// dev_info.go — /dev/info + /dev/forgify-home (TE-9). Dev-only
// endpoints surfacing backend boot info (version / uptime / paths)
// and the ~/.forgify/ directory tree (so testers can see at a
// glance what's been written: which skills exist, mcp.json content,
// catalog cache size, etc.) without dropping to a shell.

package handlers

import (
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	responsehttpapi "github.com/sunweilin/forgify/backend/internal/transport/httpapi/response"
)

// startTime is captured at first call to ensure it's set even if the
// dev handler is constructed lazily; production main.go could pass an
// explicit start time, but this is dev-only + good enough.
//
// startTime 在首次调用时捕获，保证即便 dev handler 懒构造也设了；生产
// main.go 可传显式启动时间，但 dev-only 够了。
var startTime = time.Now()

// Info handles GET /dev/info. Returns boot metadata + paths.
//
// Info 处理 GET /dev/info。返 boot 元数据 + 路径。
func (h *DevHandler) Info(w http.ResponseWriter, r *http.Request) {
	home, _ := os.UserHomeDir()
	resp := map[string]any{
		"version":          "v1.2-dev",
		"startedAt":        startTime.UTC(),
		"uptimeSeconds":    int(time.Since(startTime).Seconds()),
		"port":             h.port,
		"integrationDir":   h.integrationDir,
		"collectionsDir":   h.collectionsDir,
		"toolCount":        len(h.tools),
		"home":             home,
		"forgifyHome":      filepath.Join(home, ".forgify"),
		// Conventional paths the major subsystems use — exposed so
		// testers can verify them rather than grep code.
		// 各子系统约定路径——让测试不用 grep 代码即可验证。
		"mcpConfigPath":     filepath.Join(home, ".forgify", "mcp.json"),
		"skillsDir":         filepath.Join(home, ".forgify", "skills"),
		"catalogCachePath":  filepath.Join(home, ".forgify", ".catalog.json"),
	}
	responsehttpapi.Success(w, http.StatusOK, resp)
}

// homeEntry is one node in the ~/.forgify/ tree response.
//
// homeEntry 是 ~/.forgify/ 树响应里的一个节点。
type homeEntry struct {
	Name     string      `json:"name"`
	Path     string      `json:"path"` // relative to ~/.forgify/
	IsDir    bool        `json:"isDir"`
	Size     int64       `json:"size"` // file size; for dirs sum of children
	ModTime  time.Time   `json:"modTime"`
	Children []homeEntry `json:"children,omitempty"`
}

// ForgifyHome handles GET /dev/forgify-home. Walks ~/.forgify/ and
// returns the tree (max depth 4, max ~500 entries to bound payload).
// Hidden files starting with '.' included — that's where the
// interesting state lives (.catalog.json etc.).
//
// ForgifyHome 处理 GET /dev/forgify-home。走 ~/.forgify/ 返树（最深 4
// 层 / 上限 ~500 条目限载荷）。含 '.' 开头隐藏文件——好东西在那
// （.catalog.json 等）。
func (h *DevHandler) ForgifyHome(w http.ResponseWriter, r *http.Request) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		responsehttpapi.Error(w, http.StatusInternalServerError, "INTERNAL_ERROR",
			"could not resolve user home", nil)
		return
	}
	root := filepath.Join(home, ".forgify")
	count := 0
	tree, err := walkHomeTree(root, "", 0, 4, &count, 500)
	if err != nil {
		// Missing dir is benign — tester just hasn't generated any
		// state yet. Return empty tree with the path so UI shows
		// "(empty) — would be at <path>".
		// 缺目录良性——测试还没生成 state。返空树+路径让 UI 显示
		// "(empty) — would be at <path>"。
		if os.IsNotExist(err) {
			responsehttpapi.Success(w, http.StatusOK, map[string]any{
				"root":  root,
				"empty": true,
			})
			return
		}
		responsehttpapi.Error(w, http.StatusInternalServerError, "INTERNAL_ERROR",
			"walk failed: "+err.Error(), nil)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, map[string]any{
		"root":    root,
		"entries": tree,
		"count":   count,
	})
}

// walkHomeTree recursively descends path, building the tree. Bounded
// by maxDepth (relative to start) + maxEntries (across the whole
// walk). Files sized via Stat; dirs sized as sum of immediate children
// (not recursively — that would be a separate disk-usage call).
//
// walkHomeTree 递归向下走 path 建树。受 maxDepth（相对起点）+ maxEntries
// （全树）限。文件 Stat 取大小；dir 大小取直接子项之和（不递归——那是
// 另一个 disk-usage 调用）。
func walkHomeTree(path, rel string, depth, maxDepth int, count *int, maxEntries int) ([]homeEntry, error) {
	if depth > maxDepth || *count >= maxEntries {
		return nil, nil
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}
	out := make([]homeEntry, 0, len(entries))
	for _, ent := range entries {
		if *count >= maxEntries {
			break
		}
		*count++
		full := filepath.Join(path, ent.Name())
		relChild := filepath.Join(rel, ent.Name())
		info, err := ent.Info()
		if err != nil {
			continue
		}
		e := homeEntry{
			Name:    ent.Name(),
			Path:    relChild,
			IsDir:   info.IsDir(),
			Size:    info.Size(),
			ModTime: info.ModTime().UTC(),
		}
		if info.IsDir() && depth < maxDepth {
			children, _ := walkHomeTree(full, relChild, depth+1, maxDepth, count, maxEntries)
			e.Children = children
			// dir size = sum of immediate children file sizes
			// dir 大小 = 直接子文件大小之和
			var sum int64
			for _, c := range children {
				sum += c.Size
			}
			e.Size = sum
		}
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].IsDir != out[j].IsDir {
			return out[i].IsDir
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	// Suppress unused fs import on platforms where ReadDir doesn't surface fs.PathError directly.
	// 抑制未用 fs import 在 ReadDir 不直接出 fs.PathError 的平台。
	_ = fs.ErrNotExist
	return out, nil
}
