// Package mcp (app/mcp) is the service layer for MCP integration.
// Owns server lifecycle (Start/Stop/Connect/Disconnect/Reconnect),
// tools/list cache, search ranking, tools/call dispatch with health
// monitoring, marketplace install via sandbox, and the SSE 'mcp' event.
//
// Concurrency model: a single RWMutex guards the configs/states/clients
// maps. Read APIs (ListServers/GetServer/ListTools) take RLock; mutating
// APIs (Add/Remove/Reconnect/CallTool result recording) take Lock.
// Per-call timeouts derive from §5.7 precedence (ServerConfig.TimeoutSec
// > global defaultCallTimeout 30s).
//
// Per mcp.md §3 design principles: stdio only V1, no auto-restart on
// subprocess crash (loud failure beats silent flap), self-contained
// boundary (only ~/.forgify/mcp.json — never Claude Desktop / Cursor
// app dirs), no enable/disable bit (in-config = enabled).
//
// Package mcp（app/mcp）是 MCP 集成的 service 层。持 server lifecycle
// （Start/Stop/Connect/Disconnect/Reconnect）、tools/list 缓存、search
// 排序、tools/call dispatch + 健康监控、经 sandbox 装 marketplace、SSE
// 'mcp' 事件。
//
// 并发模型：单 RWMutex 守 configs/states/clients map。读 API 取 RLock；
// 变更 API（Add/Remove/Reconnect/CallTool 结果记账）取 Lock。per-call
// 超时按 §5.7 precedence（ServerConfig.TimeoutSec > 全局
// defaultCallTimeout 30s）。
//
// 遵 mcp.md §3：仅 stdio；不自动重启（loud beats silent flap）；自包含
// （只读 ~/.forgify/mcp.json）；无 enable/disable（配置中即启用）。
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

// defaultCallTimeout is the §5.7 fallback when ServerConfig.TimeoutSec
// is unset (= 0). Single global ceiling so curated entries don't need
// to declare per-entry defaults.
//
// defaultCallTimeout 是 §5.7 兜底——ServerConfig.TimeoutSec 未设（= 0）时
// 用。单一全局上限，curated 条目无需 per-entry 默认。
const defaultCallTimeout = 30 * time.Second

// degradedThreshold is the consecutive-failure count that flips a server
// from ready to degraded (§5.6). Auto-heal back to ready on next success.
//
// degradedThreshold 是连续失败数阈值，触发 ready → degraded（§5.6）。
// 下次成功自动回 ready。
const degradedThreshold = 3

// addServerTimeout caps the AddServer connect path — the first
// install invokes `npx -y <pkg>` / `uvx <pkg>` which fetches the
// package on demand (5-60s on first run + handshake). Reconnect
// stays on the shorter initializeTimeout because the package is
// already cached locally by then.
//
// addServerTimeout 限定 AddServer 的 connect 路径——首次 install 会跑
// `npx -y <pkg>` / `uvx <pkg>` 现场拉包（首跑 5-60s + 握手）。Reconnect
// 仍用更短的 initializeTimeout——那时包已经本地缓存。
const addServerTimeout = 3 * time.Minute

// initializeTimeout caps the handshake. Long enough that slow servers
// (Java loading the JVM) succeed; short enough that broken commands
// fail fast at boot rather than hanging Start.
//
// initializeTimeout 限定握手。够长让慢 server（Java 起 JVM）成功；够短
// 让坏命令快速失败，不挂 Start。
const initializeTimeout = 30 * time.Second

// SandboxInstaller is the port mcpapp uses to lazy-install MCP server
// runtimes. Implemented by *sandboxapp.Service. The port shape matches
// EnsureEnv exactly so the wiring is direct (no adapter layer).
//
// SandboxInstaller 是 mcpapp 懒装 MCP server runtime 的 port。由
// *sandboxapp.Service 实现。形状与 EnsureEnv 完全一致，直接接（无 adapter）。
type SandboxInstaller interface {
	EnsureEnv(ctx context.Context, owner sandboxdomain.Owner, spec sandboxdomain.EnvSpec, stream sandboxdomain.ProgressFunc) (*sandboxdomain.Env, error)
}

// Service ties registry / config file / stdio Client / sandbox / LLM
// search together. Constructed once in main.go; Start/Stop bracket the
// process lifetime.
//
// Service 把 registry / 配置文件 / stdio Client / sandbox / LLM search 串
// 起来。main.go 一次构造；Start/Stop 包进程生命周期。
type Service struct {
	configPath  string
	source      mcpdomain.RegistrySource
	sandbox     SandboxInstaller
	modelPicker modeldomain.ModelPicker
	keyProvider apikeydomain.KeyProvider
	llmFactory  *llminfra.Factory
	notif       notificationspkg.Publisher
	log         *zap.Logger

	// callRepo persists D22 mcp_calls rows. Optional — nil means call log
	// is disabled (Service.CallTool still works, just no audit trail).
	// E11 introduces this.
	//
	// callRepo 持 mcp_calls 行(D22);nil 时禁日志,CallTool 照常 work。
	callRepo mcpdomain.CallRepository

	// newClient lets unit tests inject fake Clients. Production wires
	// mcpinfra.NewStdioClient.
	//
	// newClient 让单测注入 fake Client。生产接 mcpinfra.NewStdioClient。
	newClient func(cfg mcpdomain.ServerConfig, log *zap.Logger) mcpinfra.Client

	mu      sync.RWMutex
	configs map[string]mcpdomain.ServerConfig
	states  map[string]*mcpdomain.ServerStatus
	clients map[string]mcpinfra.Client
}

// New constructs a Service. Caller must invoke Start before any
// CallTool / Search to load mcp.json + Connect servers.
//
// New 构造 Service。调用方在任何 CallTool / Search 前必须调 Start
// 加载 mcp.json + Connect server。
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

// SetClientFactory swaps the Client constructor for tests. Production
// code should not call this — the default mcpinfra.NewStdioClient is
// what main.go wires.
//
// SetClientFactory 给测试换 Client 构造器。生产代码不该调；默认
// mcpinfra.NewStdioClient 是 main.go 接的。
func (s *Service) SetClientFactory(f func(cfg mcpdomain.ServerConfig, log *zap.Logger) mcpinfra.Client) {
	s.newClient = f
}

// SetCallRepo wires the D22 call log Repository. Optional — nil disables
// the audit trail. E15 main.go calls this after constructing the GORM-
// backed Store.
//
// SetCallRepo 接 D22 call log Repository;nil 禁日志;E15 main.go 装。
func (s *Service) SetCallRepo(r mcpdomain.CallRepository) {
	s.mu.Lock()
	s.callRepo = r
	s.mu.Unlock()
}

// ── Lifecycle ────────────────────────────────────────────────────────

// Start loads ~/.forgify/mcp.json and parallel-Connects every server.
// Per mcp.md §5.7 末段: a corrupt mcp.json is logged + treated as empty
// (no panic) so the user can fix it. Per-server Connect failure is
// captured on ServerStatus + does not block other servers.
//
// Start 加载 ~/.forgify/mcp.json + 并发 Connect 所有 server。mcp.md §5.7
// 末段：mcp.json 损坏 log + 当空（不 panic）让用户修。per-server Connect
// 失败记到 ServerStatus + 不挡其他 server。
func (s *Service) Start(ctx context.Context) error {
	configs, err := mcpinfra.Load(s.configPath)
	if err != nil {
		// Log + continue: bring the Service up empty so the rest of the
		// app boots. UI surfaces the error to nudge the user to fix.
		// log + 继续：让 Service 空启动让 app 整体起来。UI 暴露错误推用户修。
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

	// Parallel-connect every server. Each connectOne publishes its own
	// mcp_server notification on completion. wg ensures Start returns
	// only after every connect (success or failure) has settled.
	// 并发 connect 每个 server。每个 connectOne 完成后自己 publish
	// mcp_server 通知。wg 保证所有 connect（成功或失败）完成后 Start 才返。
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

// Stop closes every connected client. SDK CommandTransport handles
// SIGTERM → 5s → SIGKILL per spec.
//
// Stop 关每个 connected client。SDK CommandTransport 按 spec 走
// SIGTERM → 5s → SIGKILL。
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

// AddServer writes the new ServerConfig to mcp.json + Connects. If a
// server with the same name exists it's replaced (Disconnect old, Connect
// new). Atomic on success; on Connect failure the config row stays
// (so user can edit + Reconnect rather than losing the row entirely).
//
// AddServer 把新 ServerConfig 写 mcp.json + Connect。同名 server 替换
// （Disconnect 旧，Connect 新）。成功原子；Connect 失败保留 config 行
// （让用户编辑 + Reconnect，不至于丢配置）。
func (s *Service) AddServer(ctx context.Context, cfg mcpdomain.ServerConfig) error {
	if cfg.Name == "" {
		// Programming bug: every caller (PUT /mcp-servers/{name} handler
		// fills from URL path; InstallFromRegistry fills from registry
		// entry; Import iterates over named map keys) constructs cfg
		// with a non-empty Name. Empty here means a future call site
		// bypassed those — wiring bug, not user input. panic so dev
		// sees the stack immediately rather than the call landing as
		// a generic 500 "unmapped domain error". Same approach as
		// apikey.HTTPTester.Test default branch (calibration audit).
		//
		// 编程 bug：每个调用方都填了 Name（PUT /mcp-servers/{name} 从
		// URL path / InstallFromRegistry 从 registry entry / Import 从
		// 命名 map key 迭代）。空 = 未来某调用方绕过了这些——wiring bug
		// 不是用户输入。panic 让 dev 立刻看 stack，避免 caller 看到
		// generic 500 "unmapped domain error"。同 apikey.HTTPTester.Test
		// default 分支（calibration audit）。
		panic("mcpapp.Service.AddServer: cfg.Name is empty — caller wiring bug; every code path should fill Name (URL path / registry slug / Import map key)")
	}

	s.mu.Lock()
	if existing, ok := s.clients[cfg.Name]; ok {
		// Close orphan subprocess loudly per mcp.md §3 fail-loud intent.
		// 关闭孤儿子进程要响——按 mcp.md §3 fail-loud 设计意图。
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

// RemoveServer disconnects + removes the server from mcp.json. Idempotent
// on absent name (returns ErrServerNotFound).
//
// RemoveServer 断连 + 从 mcp.json 删 server。name 不存在返 ErrServerNotFound。
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

// Reconnect force-restarts the subprocess. Useful for failed/degraded
// recovery via UI button.
//
// Reconnect 强制重启子进程。失败/degraded 走 UI 按钮恢复用。
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

// publishStatus snapshots the ServerStatus for name and pushes a
// type=mcp_server notification. Best-effort; caller must NOT hold mu.
//
// publishStatus 拍 name 的 ServerStatus 快照推 type=mcp_server 通知。
// best-effort；调用方禁持 mu。
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

// ── connectOne (internal) ────────────────────────────────────────────

// connectOne builds a Client, runs Initialize, fetches tools/list, and
// updates state. Caller must NOT hold s.mu (this method takes the lock
// for short critical sections only). Failure leaves status=failed +
// LastError set; success → status=ready + Tools cached.
//
// connectOne 建 Client、跑 Initialize、取 tools/list、更新 state。调用方
// 不能持 s.mu（本方法自取短临界区锁）。失败：status=failed + LastError；
// 成功：status=ready + Tools 缓存。
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

// ── Read APIs ────────────────────────────────────────────────────────

// ListServers returns a stable-order snapshot of every configured server's
// current status. Sorted by name for deterministic UI rendering.
//
// ListServers 返每个配置的 server 的当前状态稳定快照。按 name 排序，UI 渲
// 染确定。
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
// GetServer 返单个 server 状态；不存在返 ErrServerNotFound。
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

// Stderr returns the captured stderr ring-buffer tail (≤ 256 KB) for the
// named MCP server's subprocess. Returns "" with ErrServerNotFound when no
// such server is configured, "" with nil when configured-but-not-connected
// (e.g. failed handshake before any stderr arrived).
//
// Stderr 返指定 MCP server 子进程的 stderr 环形缓冲尾部（≤ 256 KB）。
// 未配置返 ErrServerNotFound；配置了但未连接（如握手前失败）返 ""。
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

// ListTools flattens every connected server's cached tools/list into one
// stable-ordered slice (server alpha, then tool alpha). Used by Search
// when total tool count <= topK (skip ranking).
//
// ListTools 把每个 connected server 的 tools/list 缓存拍平为单个稳定排序
// slice（server 字母序，然后 tool 字母序）。Search 在总工具数 ≤ topK 时直
// 接全返用。
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

// ── Registry source passthrough ──────────────────────────────────────

// ListRegistry returns every entry in the curated marketplace catalog,
// sorted tier-asc + name-asc. V3 (2026-05-09) replaces the V2
// search-only contract — the curated catalog is small enough (~21
// entries) that listing all of them fits in the LLM's context, and
// per-call AND-match search proved too lossy in practice.
//
// ListRegistry 返 curated marketplace 全部条目（tier asc + name asc）。
// V3（2026-05-09）替代 V2 仅 Search——curated 目录 ~21 条全列入 LLM context
// 完全 OK，而 V2 关键词 AND-match 实测召回过低。
func (s *Service) ListRegistry(ctx context.Context) ([]mcpdomain.RegistryEntry, error) {
	entries, err := s.source.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("mcpapp.ListRegistry: %w", err)
	}
	return entries, nil
}

// GetRegistryEntry returns one entry by canonical short slug (e.g.
// "playwright"). Returns ErrRegistryEntryNotFound when truly absent.
//
// GetRegistryEntry 按规范短 slug 返单条；不可达返 ErrRegistryEntryNotFound。
func (s *Service) GetRegistryEntry(ctx context.Context, name string) (*mcpdomain.RegistryEntry, error) {
	e, err := s.source.Get(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("mcpapp.GetRegistryEntry %s: %w", name, err)
	}
	return e, nil
}

// ── helpers ──────────────────────────────────────────────────────────

// cloneConfigsLocked returns a copy of the configs map. Caller MUST hold
// s.mu. Used by Save callers so the file write happens without the lock.
//
// cloneConfigsLocked 返 configs map 拷贝。调用方必须持 s.mu。让 Save 在
// 不持锁的情况下写文件。
func (s *Service) cloneConfigsLocked() map[string]mcpdomain.ServerConfig {
	out := make(map[string]mcpdomain.ServerConfig, len(s.configs))
	for k, v := range s.configs {
		out[k] = v
	}
	return out
}
