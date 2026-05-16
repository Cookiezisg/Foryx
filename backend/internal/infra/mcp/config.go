package mcp

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	mcpdomain "github.com/sunweilin/forgify/backend/internal/domain/mcp"
)

const configFileMode os.FileMode = 0o600

// ErrConfigCorrupt wraps a JSON parse failure of ~/.forgify/mcp.json.
//
// ErrConfigCorrupt 包装 ~/.forgify/mcp.json 的 JSON 解析失败。
var ErrConfigCorrupt = errors.New("mcp.json: corrupt JSON")

type fileSchema struct {
	MCPServers map[string]serverEntry `json:"mcpServers"`
}

type serverEntry struct {
	Command    string            `json:"command"`
	Args       []string          `json:"args,omitempty"`
	Env        map[string]string `json:"env,omitempty"`
	TimeoutSec int               `json:"timeoutSec,omitempty"`
}

// Load returns parsed server configs by name; file-not-found returns empty map.
//
// Load 读 path 返回按 server 名 key 的 configs；文件不存在返回空 map。
func Load(path string) (map[string]mcpdomain.ServerConfig, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return map[string]mcpdomain.ServerConfig{}, nil
		}
		return nil, fmt.Errorf("mcp.config.Load: read %s: %w", path, err)
	}
	if len(raw) == 0 {
		return map[string]mcpdomain.ServerConfig{}, nil
	}
	var fs fileSchema
	if err := json.Unmarshal(raw, &fs); err != nil {
		return nil, fmt.Errorf("mcp.config.Load: %w: %v", ErrConfigCorrupt, err)
	}
	out := make(map[string]mcpdomain.ServerConfig, len(fs.MCPServers))
	for name, entry := range fs.MCPServers {
		out[name] = mcpdomain.ServerConfig{
			Name:       name,
			Command:    entry.Command,
			Args:       entry.Args,
			Env:        entry.Env,
			TimeoutSec: entry.TimeoutSec,
		}
	}
	return out, nil
}

// Save writes configs atomically (tmp+rename) at mode 0600, pretty + sorted.
//
// Save 原子写 configs（tmp+rename），mode 0600，pretty + 按名排序。
func Save(path string, configs map[string]mcpdomain.ServerConfig) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mcp.config.Save: mkdir %s: %w", dir, err)
	}

	names := make([]string, 0, len(configs))
	for n := range configs {
		names = append(names, n)
	}
	sort.Strings(names)

	servers := make(map[string]serverEntry, len(configs))
	for _, n := range names {
		c := configs[n]
		servers[n] = serverEntry{
			Command:    c.Command,
			Args:       c.Args,
			Env:        c.Env,
			TimeoutSec: c.TimeoutSec,
		}
	}

	body, err := json.MarshalIndent(fileSchema{MCPServers: servers}, "", "  ")
	if err != nil {
		return fmt.Errorf("mcp.config.Save: marshal: %w", err)
	}
	body = append(body, '\n')

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, body, configFileMode); err != nil {
		return fmt.Errorf("mcp.config.Save: write tmp: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("mcp.config.Save: rename %s → %s: %w", tmp, path, err)
	}
	return nil
}

// MergeResult reports a drag-import outcome: Imported added/replaced; Conflicts skipped (existing).
//
// MergeResult 拖拽导入结果：Imported 新增/替换，Conflicts 因已存在被跳过。
type MergeResult struct {
	Imported  []string `json:"imported"`
	Conflicts []string `json:"conflicts"`
}

// Merge folds incoming into existing; overwrite=false skips collisions into Conflicts.
//
// Merge 把 incoming 折叠进 existing；overwrite=false 时冲突项进 Conflicts。
func Merge(existing, incoming map[string]mcpdomain.ServerConfig, overwrite bool) (map[string]mcpdomain.ServerConfig, MergeResult) {
	if existing == nil {
		existing = make(map[string]mcpdomain.ServerConfig, len(incoming))
	}
	res := MergeResult{
		Imported:  make([]string, 0, len(incoming)),
		Conflicts: make([]string, 0),
	}
	for name, cfg := range incoming {
		cfg.Name = name
		if _, exists := existing[name]; exists && !overwrite {
			res.Conflicts = append(res.Conflicts, name)
			continue
		}
		existing[name] = cfg
		res.Imported = append(res.Imported, name)
	}
	sort.Strings(res.Imported)
	sort.Strings(res.Conflicts)
	return existing, res
}
