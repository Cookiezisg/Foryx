package mcp

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"go.uber.org/zap"

	mcpdomain "github.com/sunweilin/anselm/backend/internal/domain/mcp"
)

const defaultRegistryEndpoint = "https://api.mcp.github.com/v0/servers?limit=100"

// embeddedSnapshot is a trimmed snapshot of the GitHub MCP Registry (name/description/
// packages/remotes, no readmes), baked in so the marketplace works fully offline on first run.
//
// embeddedSnapshot 是 GitHub MCP Registry 的精简快照（name/description/packages/remotes，
// 无 readme），打进二进制使市场首次离线也能用。
//
//go:embed registry_snapshot.json
var embeddedSnapshot []byte

// GitHubRegistrySource serves the marketplace from the GitHub MCP Registry over HTTP, with
// an on-disk cache and the embedded snapshot as offline fallbacks (local-first: never assume
// network). Global/public — not workspace-scoped.
//
// GitHubRegistrySource 从 GitHub MCP Registry 经 HTTP 提供市场，带磁盘缓存 + 内嵌 snapshot
// 兜底离线（本地优先：永不假设有网）。全局公共——不按 workspace 隔离。
type GitHubRegistrySource struct {
	endpoint   string
	cacheFile  string
	httpClient *http.Client
	log        *zap.Logger

	mu     sync.RWMutex
	cached []mcpdomain.RegistryEntry
}

// NewGitHubRegistrySource builds the source; cacheDir holds the refreshed registry JSON
// (e.g. ~/.anselm/cache). No network on construction.
//
// NewGitHubRegistrySource 构造 source；cacheDir 存刷新后的 registry JSON（如 ~/.anselm/cache）。
// 构造时不联网。
func NewGitHubRegistrySource(cacheDir string, log *zap.Logger) *GitHubRegistrySource {
	if log == nil {
		log = zap.NewNop()
	}
	return &GitHubRegistrySource{
		endpoint:   defaultRegistryEndpoint,
		cacheFile:  filepath.Join(cacheDir, "mcp-registry.json"),
		httpClient: &http.Client{Timeout: 15 * time.Second},
		log:        log.Named("mcp.registry"),
	}
}

var _ mcpdomain.RegistrySource = (*GitHubRegistrySource)(nil)

// List loads once (HTTP → disk cache → embed) and caches in memory for the process lifetime.
//
// List 加载一次（HTTP → 磁盘缓存 → embed）并在进程内缓存。
func (g *GitHubRegistrySource) List(ctx context.Context) ([]mcpdomain.RegistryEntry, error) {
	g.mu.RLock()
	out := g.cached
	g.mu.RUnlock()
	if out != nil {
		return out, nil
	}

	entries := g.loadEntries(ctx)
	g.mu.Lock()
	g.cached = entries
	g.mu.Unlock()
	return entries, nil
}

// Get returns one entry by its full registry name (e.g. "com.microsoft/azure").
//
// Get 按完整 registry 名（如 "com.microsoft/azure"）返回一条。
func (g *GitHubRegistrySource) Get(ctx context.Context, name string) (*mcpdomain.RegistryEntry, error) {
	entries, err := g.List(ctx)
	if err != nil {
		return nil, err
	}
	for i := range entries {
		if entries[i].Name == name {
			cp := entries[i]
			return &cp, nil
		}
	}
	return nil, fmt.Errorf("mcpregistry.Get %s: %w", name, mcpdomain.ErrRegistryEntryNotFound)
}

// Refresh drops the in-memory cache so the next List re-fetches.
//
// Refresh 清内存缓存，使下次 List 重新拉取。
func (g *GitHubRegistrySource) Refresh() {
	g.mu.Lock()
	g.cached = nil
	g.mu.Unlock()
}

// loadEntries tries HTTP (writing the disk cache on success), then the disk cache, then the
// embedded snapshot. Always returns a usable list — the embed guarantees non-empty.
//
// loadEntries 先试 HTTP（成功写磁盘缓存），再磁盘缓存，再内嵌 snapshot。总返回可用列表
// ——embed 保证非空。
func (g *GitHubRegistrySource) loadEntries(ctx context.Context) []mcpdomain.RegistryEntry {
	if raw, err := g.fetch(ctx); err == nil {
		if entries, perr := parseGitHub(raw); perr == nil && len(entries) > 0 {
			g.writeCache(raw)
			return entries
		}
	} else {
		g.log.Info("mcp registry HTTP fetch failed; using cache/embed", zap.Error(err))
	}
	if raw, err := os.ReadFile(g.cacheFile); err == nil {
		if entries, perr := parseGitHub(raw); perr == nil && len(entries) > 0 {
			return entries
		}
	}
	entries, _ := parseGitHub(embeddedSnapshot)
	return entries
}

func (g *GitHubRegistrySource) fetch(ctx context.Context) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, g.endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := g.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("mcpregistry.fetch: status %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

func (g *GitHubRegistrySource) writeCache(raw []byte) {
	if err := os.MkdirAll(filepath.Dir(g.cacheFile), 0o755); err != nil {
		g.log.Warn("mcp registry cache mkdir failed", zap.Error(err))
		return
	}
	tmp := g.cacheFile + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		g.log.Warn("mcp registry cache write failed", zap.Error(err))
		return
	}
	if err := os.Rename(tmp, g.cacheFile); err != nil {
		_ = os.Remove(tmp)
		g.log.Warn("mcp registry cache rename failed", zap.Error(err))
	}
}

// --- GitHub registry JSON shape (api.mcp.github.com/v0/servers) → domain mapping ---

type ghResponse struct {
	Servers []struct {
		Server ghServer `json:"server"`
	} `json:"servers"`
}
type ghServer struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Packages    []ghPackage `json:"packages"`
	Remotes     []ghRemote  `json:"remotes"`
}
type ghPackage struct {
	Name                 string  `json:"name"`
	RuntimeHint          string  `json:"runtime_hint"`
	Version              string  `json:"version"`
	PackageArguments     []ghArg `json:"package_arguments"`
	EnvironmentVariables []ghEnv `json:"environment_variables"`
}
type ghArg struct {
	Value string `json:"value"`
}
type ghEnv struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	IsSecret    bool   `json:"is_secret"`
	IsRequired  bool   `json:"is_required"`
}
type ghRemote struct {
	TransportType string     `json:"transport_type"`
	URL           string     `json:"url"`
	Headers       []ghHeader `json:"headers"`
}
type ghHeader struct {
	Name        string `json:"name"`
	Value       string `json:"value"`
	Description string `json:"description"`
	IsSecret    bool   `json:"is_secret"`
}

// parseGitHub maps the registry response to domain RegistryEntries. Entry.Name is the full
// registry slug (e.g. "com.microsoft/azure"); install derives a short display name from it.
//
// parseGitHub 把 registry 响应映射成 domain RegistryEntry。Entry.Name 是完整 registry slug
// （如 "com.microsoft/azure"）；install 时派生短展示名。
func parseGitHub(raw []byte) ([]mcpdomain.RegistryEntry, error) {
	var r ghResponse
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, fmt.Errorf("mcpregistry.parse: %w", err)
	}
	out := make([]mcpdomain.RegistryEntry, 0, len(r.Servers))
	for _, s := range r.Servers {
		e := mcpdomain.RegistryEntry{Name: s.Server.Name, Description: s.Server.Description}
		for _, p := range s.Server.Packages {
			pkg := mcpdomain.Package{Name: p.Name, RuntimeHint: p.RuntimeHint, Version: p.Version}
			for _, a := range p.PackageArguments {
				if a.Value != "" {
					pkg.Args = append(pkg.Args, a.Value)
				}
			}
			for _, ev := range p.EnvironmentVariables {
				pkg.EnvVars = append(pkg.EnvVars, mcpdomain.EnvVar{Name: ev.Name, Description: ev.Description, IsSecret: ev.IsSecret, Required: ev.IsRequired})
			}
			e.Packages = append(e.Packages, pkg)
		}
		for _, rm := range s.Server.Remotes {
			rem := mcpdomain.Remote{Transport: rm.TransportType, URL: rm.URL}
			for _, h := range rm.Headers {
				rem.Headers = append(rem.Headers, mcpdomain.Header{Name: h.Name, Value: h.Value, IsSecret: h.IsSecret})
			}
			e.Remotes = append(e.Remotes, rem)
		}
		out = append(out, e)
	}
	return out, nil
}
