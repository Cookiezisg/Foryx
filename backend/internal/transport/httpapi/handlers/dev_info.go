package handlers

import (
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	responsehttpapi "github.com/sunweilin/forgify/backend/internal/transport/httpapi/response"
)

var startTime = time.Now()

// Info handles GET /dev/info, returning boot metadata + paths.
//
// Info 处理 GET /dev/info,返 boot 元数据 + 路径。
func (h *DevHandler) Info(w http.ResponseWriter, r *http.Request) {
	home, _ := os.UserHomeDir()
	resp := map[string]any{
		"version":          "v1.2-dev",
		"startedAt":        startTime.UTC(),
		"uptimeSeconds":    int(time.Since(startTime).Seconds()),
		"port":             h.port,
		"integrationDir":   h.integrationDir,
		"toolCount":        len(h.tools),
		"home":             home,
		"forgifyHome":      h.forgifyHome,
		"mcpConfigPath":    filepath.Join(h.forgifyHome, "mcp.json"),
		"skillsDir":        filepath.Join(h.forgifyHome, "skills"),
		"catalogCachePath": filepath.Join(h.forgifyHome, ".catalog.json"),
	}
	responsehttpapi.Success(w, http.StatusOK, resp)
}

type homeEntry struct {
	Name     string      `json:"name"`
	Path     string      `json:"path"`
	IsDir    bool        `json:"isDir"`
	Size     int64       `json:"size"`
	ModTime  time.Time   `json:"modTime"`
	Children []homeEntry `json:"children,omitempty"`
}

// ForgifyHome walks ~/.forgify/ and returns the tree (depth 4, ~500 entries).
//
// ForgifyHome 走 ~/.forgify/ 返树(最深 4 层,上限 ~500 条)。
func (h *DevHandler) ForgifyHome(w http.ResponseWriter, r *http.Request) {
	root := h.forgifyHome
	count := 0
	tree, err := walkHomeTree(root, "", 0, 4, &count, 500)
	if err != nil {
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
	return out, nil
}
