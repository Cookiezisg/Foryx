// Package mcp is the service layer for MCP integration (stdio servers).
//
// Package mcp 是 MCP 集成（stdio servers）的服务层。
package mcp

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"go.uber.org/zap"

	apikeydomain "github.com/sunweilin/forgify/backend/internal/domain/apikey"
	mcpdomain "github.com/sunweilin/forgify/backend/internal/domain/mcp"
	modeldomain "github.com/sunweilin/forgify/backend/internal/domain/model"
	sandboxdomain "github.com/sunweilin/forgify/backend/internal/domain/sandbox"
	llminfra "github.com/sunweilin/forgify/backend/internal/infra/llm"
	mcpinfra "github.com/sunweilin/forgify/backend/internal/infra/mcp"
	notificationspkg "github.com/sunweilin/forgify/backend/internal/pkg/notifications"
)

const (
	defaultCallTimeout = 30 * time.Second
	degradedThreshold  = 3
	addServerTimeout   = 3 * time.Minute
	initializeTimeout  = 30 * time.Second
)

// SandboxInstaller is the port mcpapp uses to lazy-install runtimes.
//
// SandboxInstaller 是 mcpapp 懒装 runtime 的端口。
type SandboxInstaller interface {
	EnsureEnv(ctx context.Context, owner sandboxdomain.Owner, spec sandboxdomain.EnvSpec, stream sandboxdomain.ProgressFunc) (*sandboxdomain.Env, error)
}

// Service ties registry, config, stdio Client, sandbox, and LLM search together.
//
// Service 串联 registry、配置、stdio Client、sandbox 与 LLM search。
type Service struct {
	configPath  string
	source      mcpdomain.RegistrySource
	sandbox     SandboxInstaller
	modelPicker modeldomain.ModelPicker
	keyProvider apikeydomain.KeyProvider
	llmFactory  *llminfra.Factory
	notif       notificationspkg.Publisher
	log         *zap.Logger

	callRepo mcpdomain.CallRepository

	newClient func(cfg mcpdomain.ServerConfig, log *zap.Logger) mcpinfra.Client

	mu      sync.RWMutex
	configs map[string]mcpdomain.ServerConfig
	states  map[string]*mcpdomain.ServerStatus
	clients map[string]mcpinfra.Client
}

// New constructs a Service; caller must Start before CallTool/Search.
//
// New 构造 Service；CallTool/Search 之前必须先 Start。
func New(
	configPath string,
	source mcpdomain.RegistrySource,
	sandbox SandboxInstaller,
	modelPicker modeldomain.ModelPicker,
	keyProvider apikeydomain.KeyProvider,
	llmFactory *llminfra.Factory,
	notif notificationspkg.Publisher,
	log *zap.Logger,
) *Service {
	if log == nil {
		panic("mcp.New: logger is nil")
	}
	if source == nil {
		panic("mcp.New: registry source is nil")
	}
	if notif == nil {
		notif = notificationspkg.New(nil, log)
	}
	return &Service{
		configPath:  configPath,
		source:      source,
		sandbox:     sandbox,
		modelPicker: modelPicker,
		keyProvider: keyProvider,
		llmFactory:  llmFactory,
		notif:       notif,
		log:         log,
		newClient:   mcpinfra.NewStdioClient,
		configs:     map[string]mcpdomain.ServerConfig{},
		states:      map[string]*mcpdomain.ServerStatus{},
		clients:     map[string]mcpinfra.Client{},
	}
}

// SetClientFactory swaps the Client constructor (tests only).
//
// SetClientFactory 替换 Client 构造器（仅测试用）。
func (s *Service) SetClientFactory(f func(cfg mcpdomain.ServerConfig, log *zap.Logger) mcpinfra.Client) {
	s.newClient = f
}

// SetCallRepo wires the call log Repository; nil disables audit.
//
// SetCallRepo 注入 call log Repository，nil 禁用审计。
func (s *Service) SetCallRepo(r mcpdomain.CallRepository) {
	s.mu.Lock()
	s.callRepo = r
	s.mu.Unlock()
}

// Start loads mcp.json and parallel-connects every server.
//
// Start 加载 mcp.json 并并发连接每个 server。
func (s *Service) Start(ctx context.Context) error {
	configs, err := mcpinfra.Load(s.configPath)
	if err != nil {
		s.log.Error("mcp.json load failed; starting with no servers",
			zap.String("path", s.configPath), zap.Error(err))
		configs = map[string]mcpdomain.ServerConfig{}
	}

	s.mu.Lock()
	s.configs = configs
	for name := range configs {
		s.states[name] = &mcpdomain.ServerStatus{
			Name:   name,
			Status: mcpdomain.StatusDisconnected,
			Tools:  []mcpdomain.ToolDef{},
		}
	}
	s.mu.Unlock()

	var wg sync.WaitGroup
	for name := range configs {
		wg.Add(1)
		go func(n string) {
			defer wg.Done()
			cctx, cancel := context.WithTimeout(ctx, initializeTimeout)
			defer cancel()
			if err := s.connectOne(cctx, n); err != nil {
				s.log.Warn("mcp connect failed", zap.String("server", n), zap.Error(err))
			}
		}(name)
	}
	wg.Wait()

	return nil
}

// Stop closes every connected client (SDK handles SIGTERM → 5s → SIGKILL).
//
// Stop 关闭每个已连接 client（SDK 负责 SIGTERM → 5s → SIGKILL）。
func (s *Service) Stop(_ context.Context) error {
	s.mu.Lock()
	clients := s.clients
	s.clients = map[string]mcpinfra.Client{}
	s.mu.Unlock()

	for name, c := range clients {
		if err := c.Close(); err != nil {
			s.log.Warn("mcp close failed", zap.String("server", name), zap.Error(err))
		}
	}
	return nil
}

// AddServer writes ServerConfig to mcp.json and connects; same name replaces.
//
// AddServer 写入 ServerConfig 到 mcp.json 并连接；同名替换旧的。
func (s *Service) AddServer(ctx context.Context, cfg mcpdomain.ServerConfig) error {
	if cfg.Name == "" {
		panic("mcpapp.Service.AddServer: cfg.Name is empty — caller wiring bug; every code path should fill Name (URL path / registry slug / Import map key)")
	}

	s.mu.Lock()
	if existing, ok := s.clients[cfg.Name]; ok {
		if err := existing.Close(); err != nil {
			s.log.Warn("AddServer: replace-existing close failed; orphan subprocess may persist until parent exit",
				zap.String("server", cfg.Name), zap.Error(err))
		}
		delete(s.clients, cfg.Name)
	}
	s.configs[cfg.Name] = cfg
	s.states[cfg.Name] = &mcpdomain.ServerStatus{
		Name:   cfg.Name,
		Status: mcpdomain.StatusDisconnected,
		Tools:  []mcpdomain.ToolDef{},
	}
	configsCopy := s.cloneConfigsLocked()
	s.mu.Unlock()

	if err := mcpinfra.Save(s.configPath, configsCopy); err != nil {
		return fmt.Errorf("mcpapp.AddServer: save mcp.json: %w", err)
	}
	s.publishStatus(ctx, cfg.Name)

	cctx, cancel := context.WithTimeout(ctx, addServerTimeout)
	defer cancel()
	if err := s.connectOne(cctx, cfg.Name); err != nil {
		return fmt.Errorf("mcpapp.AddServer: connect: %w", err)
	}
	return nil
}

// RemoveServer disconnects and removes server from mcp.json.
//
// RemoveServer 断开并从 mcp.json 移除 server，name 缺失返 ErrServerNotFound。
func (s *Service) RemoveServer(ctx context.Context, name string) error {
	s.mu.Lock()
	if _, ok := s.configs[name]; !ok {
		s.mu.Unlock()
		return fmt.Errorf("mcpapp.RemoveServer: %w: %q", mcpdomain.ErrServerNotFound, name)
	}
	if c, ok := s.clients[name]; ok {
		if err := c.Close(); err != nil {
			s.log.Warn("RemoveServer: close failed; orphan subprocess may persist until parent exit",
				zap.String("server", name), zap.Error(err))
		}
		delete(s.clients, name)
	}
	delete(s.configs, name)
	delete(s.states, name)
	configsCopy := s.cloneConfigsLocked()
	s.mu.Unlock()

	if err := mcpinfra.Save(s.configPath, configsCopy); err != nil {
		return fmt.Errorf("mcpapp.RemoveServer: save mcp.json: %w", err)
	}
	s.notif.Publish(ctx, "mcp_server", name,
		map[string]any{"name": name, "deleted": true}, "")
	return nil
}

// Reconnect force-restarts the subprocess.
//
// Reconnect 强制重启子进程，常用于 failed/degraded 恢复。
func (s *Service) Reconnect(ctx context.Context, name string) error {
	s.mu.Lock()
	if _, ok := s.configs[name]; !ok {
		s.mu.Unlock()
		return fmt.Errorf("mcpapp.Reconnect: %w: %q", mcpdomain.ErrServerNotFound, name)
	}
	if c, ok := s.clients[name]; ok {
		if err := c.Close(); err != nil {
			s.log.Warn("Reconnect: close-existing failed; orphan subprocess may persist until parent exit",
				zap.String("server", name), zap.Error(err))
		}
		delete(s.clients, name)
	}
	if state, ok := s.states[name]; ok {
		state.Status = mcpdomain.StatusDisconnected
		state.LastError = ""
		state.ConsecutiveFailures = 0
	}
	s.mu.Unlock()

	cctx, cancel := context.WithTimeout(ctx, initializeTimeout)
	defer cancel()
	if err := s.connectOne(cctx, name); err != nil {
		return fmt.Errorf("mcpapp.Reconnect: %w", err)
	}
	return nil
}

func (s *Service) publishStatus(ctx context.Context, name string) {
	s.mu.RLock()
	state, ok := s.states[name]
	var snap mcpdomain.ServerStatus
	if ok {
		snap = *state
	}
	s.mu.RUnlock()
	if !ok {
		return
	}
	s.notif.Publish(ctx, "mcp_server", name, &snap, "")
}

// connectOne initializes a Client, caches tools/list, and updates status.
//
// connectOne 初始化 Client、缓存 tools/list、更新 status；调用方不可持锁。
func (s *Service) connectOne(ctx context.Context, name string) error {
	s.mu.RLock()
	cfg, ok := s.configs[name]
	state := s.states[name]
	s.mu.RUnlock()
	if !ok || state == nil {
		return fmt.Errorf("mcpapp.connectOne: %w: %q", mcpdomain.ErrServerNotFound, name)
	}

	s.mu.Lock()
	state.Status = mcpdomain.StatusConnecting
	s.mu.Unlock()

	client := s.newClient(cfg, s.log)
	if err := client.Initialize(ctx); err != nil {
		now := time.Now().UTC()
		s.mu.Lock()
		state.Status = mcpdomain.StatusFailed
		state.LastError = err.Error()
		state.LastErrorAt = &now
		s.mu.Unlock()
		if cerr := client.Close(); cerr != nil {
			s.log.Warn("mcpapp.connectOne: close after Initialize-fail also failed; orphan subprocess",
				zap.String("server", name), zap.Error(cerr))
		}
		s.publishStatus(ctx, name)
		return fmt.Errorf("mcpapp.connectOne: initialize: %w", err)
	}

	tools, err := client.ListTools(ctx)
	if err != nil {
		now := time.Now().UTC()
		s.mu.Lock()
		state.Status = mcpdomain.StatusFailed
		state.LastError = err.Error()
		state.LastErrorAt = &now
		s.mu.Unlock()
		if cerr := client.Close(); cerr != nil {
			s.log.Warn("mcpapp.connectOne: close after ListTools-fail also failed; orphan subprocess",
				zap.String("server", name), zap.Error(cerr))
		}
		s.publishStatus(ctx, name)
		return fmt.Errorf("mcpapp.connectOne: list tools: %w", err)
	}

	now := time.Now().UTC()
	s.mu.Lock()
	state.Status = mcpdomain.StatusReady
	state.ConnectedAt = &now
	state.LastError = ""
	state.LastErrorAt = nil
	state.Tools = tools
	s.clients[name] = client
	s.mu.Unlock()
	s.publishStatus(ctx, name)
	return nil
}

// ListServers returns every configured server's status, name-sorted.
//
// ListServers 返回每个 server 的状态，按 name 排序。
func (s *Service) ListServers(_ context.Context) []mcpdomain.ServerStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]mcpdomain.ServerStatus, 0, len(s.states))
	for _, st := range s.states {
		out = append(out, *st)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// GetServer returns one server's status; ErrServerNotFound when absent.
//
// GetServer 返回单个 server 状态，缺失返 ErrServerNotFound。
func (s *Service) GetServer(_ context.Context, name string) (*mcpdomain.ServerStatus, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	st, ok := s.states[name]
	if !ok {
		return nil, fmt.Errorf("mcpapp.GetServer: %w: %q", mcpdomain.ErrServerNotFound, name)
	}
	cp := *st
	return &cp, nil
}

// Stderr returns the stderr ring-buffer tail (≤ 256 KB) for the named server.
//
// Stderr 返回指定 server 的 stderr 环形缓冲尾部（≤ 256 KB）。
func (s *Service) Stderr(name string) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if _, ok := s.states[name]; !ok {
		return "", fmt.Errorf("mcpapp.Stderr: %w: %q", mcpdomain.ErrServerNotFound, name)
	}
	c, ok := s.clients[name]
	if !ok {
		return "", nil
	}
	return c.StderrTail(), nil
}

// ListTools flattens connected servers' cached tools into stable order.
//
// ListTools 把已连接 server 的 tools 缓存拍平为稳定排序的 slice。
func (s *Service) ListTools(_ context.Context) []mcpdomain.ToolDef {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []mcpdomain.ToolDef{}
	names := make([]string, 0, len(s.states))
	for n := range s.states {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		st := s.states[n]
		if st.Status != mcpdomain.StatusReady && st.Status != mcpdomain.StatusDegraded {
			continue
		}
		tools := append([]mcpdomain.ToolDef(nil), st.Tools...)
		sort.Slice(tools, func(i, j int) bool { return tools[i].Name < tools[j].Name })
		out = append(out, tools...)
	}
	return out
}

// ListRegistry returns every curated marketplace entry, tier+name sorted.
//
// ListRegistry 返回 curated marketplace 全部条目，按 tier+name 排序。
func (s *Service) ListRegistry(ctx context.Context) ([]mcpdomain.RegistryEntry, error) {
	entries, err := s.source.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("mcpapp.ListRegistry: %w", err)
	}
	return entries, nil
}

// GetRegistryEntry returns one entry by short slug; absent → ErrRegistryEntryNotFound.
//
// GetRegistryEntry 按短 slug 返回单条，缺失返 ErrRegistryEntryNotFound。
func (s *Service) GetRegistryEntry(ctx context.Context, name string) (*mcpdomain.RegistryEntry, error) {
	e, err := s.source.Get(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("mcpapp.GetRegistryEntry %s: %w", name, err)
	}
	return e, nil
}

func (s *Service) cloneConfigsLocked() map[string]mcpdomain.ServerConfig {
	out := make(map[string]mcpdomain.ServerConfig, len(s.configs))
	for k, v := range s.configs {
		out[k] = v
	}
	return out
}
