// Package mcp is the service layer for MCP integration — a resident process pool that
// connects installed servers (stdio subprocess via sandbox, or remote SSE/HTTP), caches
// their tools, and routes tool calls. Mirrors handler's lifecycle: map[mcp_id] singleton,
// Boot per-workspace, reconnect (the "reset button"), graceful Shutdown.
//
// Package mcp 是 MCP 集成的服务层——一个常驻进程池：连接已装 server（stdio 子进程经 sandbox，或
// remote SSE/HTTP）、缓存其工具、路由 tool 调用。镜像 handler 生命周期：map[mcp_id] 单例、
// Boot per-workspace、reconnect（重置按钮）、优雅 Shutdown。
package mcp

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"go.uber.org/zap"

	mcpdomain "github.com/sunweilin/forgify/backend/internal/domain/mcp"
	notificationdomain "github.com/sunweilin/forgify/backend/internal/domain/notification"
	sandboxdomain "github.com/sunweilin/forgify/backend/internal/domain/sandbox"
	searchdomain "github.com/sunweilin/forgify/backend/internal/domain/search"
	streamdomain "github.com/sunweilin/forgify/backend/internal/domain/stream"
	mcpinfra "github.com/sunweilin/forgify/backend/internal/infra/mcp"
)

const (
	connectTimeout = 30 * time.Second
)

// SandboxPort is the subset of sandboxapp.Service mcp needs: provision a runtime env
// (install) and spawn the server subprocess (run). The process is owned by sandbox.
//
// SandboxPort 是 mcp 需要的 sandboxapp.Service 子集：物化 runtime env（install）+ 起 server
// 子进程（run）。进程归 sandbox 管。
type SandboxPort interface {
	EnsureEnv(ctx context.Context, owner sandboxdomain.Owner, spec sandboxdomain.EnvSpec, stream sandboxdomain.ProgressFunc) (*sandboxdomain.Env, error)
	SpawnLongLived(ctx context.Context, owner sandboxdomain.Owner, opts sandboxdomain.SpawnOpts) (sandboxdomain.LongLivedHandle, error)
}

// RelationSyncer is the subset of relationapp.Service mcp consumes (nil-tolerant).
//
// RelationSyncer 是 mcp 消费的 relationapp.Service 子集（容忍 nil）。
type RelationSyncer interface {
	PurgeEntity(ctx context.Context, kind, id string) error
}

// Service ties the repo, registry, sandbox, and per-server clients together.
//
// Service 串联 repo、registry、sandbox 与 per-server client。
type Service struct {
	repo      mcpdomain.Repository
	search    searchdomain.Notifier      // nil → search indexing disabled. nil → 不接搜索索引。
	notif     notificationdomain.Emitter // nil → mcp.* 通知族静默（events.md P4 契约）
	registry  mcpdomain.RegistrySource
	sandbox   SandboxPort
	relations RelationSyncer      // optional; nil disables relation hooks
	entities  streamdomain.Bridge // entities stream (SSE-C); nil → no server-panel run terminal
	log       *zap.Logger

	// newClient builds a Client from a spec; swappable in tests.
	newClient func(spec mcpinfra.ClientSpec, log *zap.Logger) mcpinfra.Client

	mu      sync.RWMutex
	states  map[string]*mcpdomain.ServerStatus       // mcp_id → live status
	clients map[string]mcpinfra.Client               // mcp_id → connected client
	handles map[string]sandboxdomain.LongLivedHandle // mcp_id → sandbox handle (stdio only)
}

// New constructs a Service; call Boot before serving.
//
// New 构造 Service；服务前先 Boot。
func New(repo mcpdomain.Repository, registry mcpdomain.RegistrySource, sandbox SandboxPort, log *zap.Logger) *Service {
	if log == nil {
		log = zap.NewNop()
	}
	return &Service{
		repo:      repo,
		registry:  registry,
		sandbox:   sandbox,
		log:       log,
		newClient: mcpinfra.NewClient,
		states:    map[string]*mcpdomain.ServerStatus{},
		clients:   map[string]mcpinfra.Client{},
		handles:   map[string]sandboxdomain.LongLivedHandle{},
	}
}

// SetRelationSyncer installs the relation Service post-construction.
func (s *Service) SetRelationSyncer(r RelationSyncer) { s.relations = r }

// SetNotifier installs the notification emitter post-construction — the
// mcp.{installed,updated,removed,reconnected} family events.md promises; the
// service shipped without this wire and the family never fired (AC-29).
//
// SetNotifier 装配后装入通知发射器——events.md 承诺的 mcp.{installed,updated,removed,
// reconnected} 族；此前缺这条线、整族从未发出（AC-29）。
func (s *Service) SetNotifier(n notificationdomain.Emitter) { s.notif = n }

// emitNotif fires one mcp.<action> notification (best-effort, nil-safe).
//
// emitNotif 发一条 mcp.<action> 通知（best-effort、nil 安全）。
func (s *Service) emitNotif(ctx context.Context, action, name string) {
	if s.notif == nil {
		return
	}
	if err := s.notif.Emit(ctx, "mcp."+action, map[string]any{"name": name}); err != nil {
		s.log.Warn("mcpapp.notify failed", zap.String("name", name), zap.String("action", action), zap.Error(err))
	}
}

// SetEntitiesBridge installs the entities stream post-construction (SSE-C): CallTool tees a tool
// call's progress notifications onto the server's run terminal for the entity panel.
//
// SetEntitiesBridge 装配后装入 entities 流（SSE-C）：CallTool 把工具调用的进度通知 tee 到 server 的 run
// 终端供实体面板。
func (s *Service) SetEntitiesBridge(b streamdomain.Bridge) { s.entities = b }

// SetClientFactory swaps the Client constructor (tests only).
func (s *Service) SetClientFactory(f func(spec mcpinfra.ClientSpec, log *zap.Logger) mcpinfra.Client) {
	s.newClient = f
}

// Boot loads the ctx workspace's servers and parallel-connects each (best-effort: a server
// that fails to connect stays failed, recoverable via reconnect).
//
// Boot 加载 ctx workspace 的 server 并并发连接（best-effort：连不上的留 failed，可经 reconnect 恢复）。
func (s *Service) Boot(ctx context.Context) {
	servers, err := s.repo.List(ctx)
	if err != nil {
		s.log.Warn("mcpapp.Boot: list servers failed", zap.Error(err))
		return
	}
	var wg sync.WaitGroup
	for _, srv := range servers {
		s.initStatus(srv)
		wg.Add(1)
		go func(srv *mcpdomain.Server) {
			defer wg.Done()
			cctx, cancel := context.WithTimeout(ctx, connectTimeout)
			defer cancel()
			if err := s.connectOne(cctx, srv); err != nil {
				s.log.Warn("mcpapp.Boot: connect failed", zap.String("server", srv.Name), zap.Error(err))
			}
		}(srv)
	}
	wg.Wait()
}

// Shutdown closes every client and kills every sandbox handle (app exit).
//
// Shutdown 关闭每个 client、杀掉每个 sandbox handle（退出软件）。
func (s *Service) Shutdown(_ context.Context) {
	s.mu.Lock()
	ids := make([]string, 0, len(s.clients))
	for id := range s.clients {
		ids = append(ids, id)
	}
	s.mu.Unlock()
	for _, id := range ids {
		s.closeOne(id)
	}
}

// Reconnect force-restarts a server: close client + kill handle, then connect fresh. The
// "reset button" for a process that's alive-but-broken (stale connection / expired session).
//
// Reconnect 强制重启一个 server：关 client + 杀 handle，再重新连接。「重置按钮」——救「活着但
// 状态坏了」（stale 连接 / session 过期），对齐 handler 的 restart。
func (s *Service) Reconnect(ctx context.Context, name string) (*mcpdomain.ServerStatus, error) {
	srv, err := s.repo.GetByName(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("mcpapp.Reconnect: %w", err)
	}
	s.closeOne(srv.ID)
	s.initStatus(srv)
	cctx, cancel := context.WithTimeout(ctx, connectTimeout)
	defer cancel()
	_ = s.connectOne(cctx, srv) // failure → status=failed; caller sees lastError
	s.emitNotif(ctx, "reconnected", srv.Name)
	return s.GetServer(ctx, name)
}

// connectOne spawns/connects a server, caches tools/list, and flips status to ready. Caller
// must NOT hold s.mu.
//
// connectOne 起/连一个 server、缓存 tools/list、status 翻 ready。调用方不可持 s.mu。
func (s *Service) connectOne(ctx context.Context, srv *mcpdomain.Server) error {
	s.setConnecting(srv.ID)

	var (
		spec   mcpinfra.ClientSpec
		handle sandboxdomain.LongLivedHandle
	)
	if srv.IsRemote() {
		spec = mcpinfra.ClientSpec{Name: srv.Name, URL: srv.URL, Transport: srv.Transport, Headers: srv.Headers}
	} else {
		h, err := s.sandbox.SpawnLongLived(ctx, ownerFor(srv), sandboxdomain.SpawnOpts{
			Cmd: srv.Command, Args: srv.Args, Env: srv.Env, LongLived: true,
		})
		if err != nil {
			s.setFailed(srv.ID, err)
			return fmt.Errorf("mcpapp.connectOne: spawn %s: %w", srv.Name, err)
		}
		handle = h
		spec = mcpinfra.ClientSpec{Name: srv.Name, Stdin: h.Stdin(), Stdout: h.Stdout(), Stderr: h.Stderr()}
	}

	client := s.newClient(spec, s.log)
	if err := client.Initialize(ctx); err != nil {
		if handle != nil {
			_ = handle.Kill()
		}
		s.setFailed(srv.ID, err)
		return fmt.Errorf("mcpapp.connectOne: initialize %s: %w", srv.Name, err)
	}
	tools, err := client.ListTools(ctx)
	if err != nil {
		_ = client.Close()
		if handle != nil {
			_ = handle.Kill()
		}
		s.setFailed(srv.ID, err)
		return fmt.Errorf("mcpapp.connectOne: list tools %s: %w", srv.Name, err)
	}

	now := time.Now().UTC()
	s.mu.Lock()
	// A concurrent connect (double-clicked Reconnect, or Reconnect racing Boot) may have
	// registered another live client for this server: swap it out and close it, or the loser's
	// process leaks as a zombie. Last-writer-wins matches Reconnect's "reset" semantics.
	//
	// 并发连接（双击 Reconnect、或 Reconnect 与 Boot 重叠）可能已为该 server 注册了另一个活
	// client：换出并关闭它，否则输家的进程泄漏成僵尸。后写者赢与 Reconnect 的「重置」语义一致。
	if old := s.clients[srv.ID]; old != nil {
		go func() { _ = old.Close() }()
	}
	if oldH := s.handles[srv.ID]; oldH != nil {
		go func() { _ = oldH.Kill() }()
	}
	delete(s.handles, srv.ID)
	s.clients[srv.ID] = client
	if handle != nil {
		s.handles[srv.ID] = handle
	}
	if st := s.states[srv.ID]; st != nil {
		st.Status = mcpdomain.StatusReady
		st.ConnectedAt = &now
		st.LastError = ""
		st.LastErrorAt = nil
		st.Tools = tools
	}
	s.mu.Unlock()
	s.notifySearch(ctx, srv.Name)
	return nil
}

// closeOne disconnects + kills a single server's process (reconnect / uninstall / shutdown).
//
// closeOne 断开 + 杀单个 server 的进程（reconnect / uninstall / shutdown）。
func (s *Service) closeOne(id string) {
	s.mu.Lock()
	client := s.clients[id]
	handle := s.handles[id]
	delete(s.clients, id)
	delete(s.handles, id)
	s.mu.Unlock()
	if client != nil {
		if err := client.Close(); err != nil {
			s.log.Warn("mcpapp.closeOne: client close failed", zap.String("id", id), zap.Error(err))
		}
	}
	if handle != nil {
		if err := handle.Kill(); err != nil {
			s.log.Warn("mcpapp.closeOne: kill handle failed", zap.String("id", id), zap.Error(err))
		}
	}
}

// --- queries ---------------------------------------------------------------

// ListServers returns the ctx workspace's servers with their live status, name-sorted.
//
// ListServers 返回 ctx workspace 的 server 及实时状态，按名排序。
func (s *Service) ListServers(ctx context.Context) ([]mcpdomain.ServerStatus, error) {
	servers, err := s.repo.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("mcpapp.ListServers: %w", err)
	}
	out := make([]mcpdomain.ServerStatus, 0, len(servers))
	s.mu.RLock()
	for _, srv := range servers {
		if st := s.states[srv.ID]; st != nil {
			out = append(out, *st)
		} else {
			out = append(out, mcpdomain.ServerStatus{ID: srv.ID, Name: srv.Name, Status: mcpdomain.StatusDisconnected})
		}
	}
	s.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// GetServer returns one server's live status by name (workspace-checked via repo).
//
// GetServer 按 name 返回单个 server 的实时状态（经 repo 校验 workspace）。
func (s *Service) GetServer(ctx context.Context, name string) (*mcpdomain.ServerStatus, error) {
	srv, err := s.repo.GetByName(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("mcpapp.GetServer: %w", err)
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if st := s.states[srv.ID]; st != nil {
		cp := *st
		return &cp, nil
	}
	return &mcpdomain.ServerStatus{ID: srv.ID, Name: srv.Name, Status: mcpdomain.StatusDisconnected}, nil
}

// Stderr returns a stdio server's captured stderr tail.
//
// Stderr 返 stdio server 捕获的 stderr 尾部。
func (s *Service) Stderr(ctx context.Context, name string) (string, error) {
	srv, err := s.repo.GetByName(ctx, name)
	if err != nil {
		return "", fmt.Errorf("mcpapp.Stderr: %w", err)
	}
	s.mu.RLock()
	c := s.clients[srv.ID]
	s.mu.RUnlock()
	if c == nil {
		return "", nil
	}
	return c.StderrTail(), nil
}

// ListTools flattens connected servers' cached tools (ctx workspace), stable order. Used by
// the host to build the lazy tool pool.
//
// ListTools 把 ctx workspace 内已连接 server 的工具缓存拍平（稳定排序）。host 用它建 lazy 工具池。
func (s *Service) ListTools(ctx context.Context) ([]mcpdomain.ToolDef, error) {
	servers, err := s.repo.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("mcpapp.ListTools: %w", err)
	}
	out := []mcpdomain.ToolDef{}
	s.mu.RLock()
	for _, srv := range servers {
		st := s.states[srv.ID]
		if st == nil || !mcpdomain.IsCallable(st.Status) {
			continue
		}
		out = append(out, st.Tools...)
	}
	s.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool {
		if out[i].ServerName != out[j].ServerName {
			return out[i].ServerName < out[j].ServerName
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

// --- status helpers (hold s.mu internally) ---------------------------------

func (s *Service) initStatus(srv *mcpdomain.Server) {
	s.mu.Lock()
	s.states[srv.ID] = &mcpdomain.ServerStatus{ID: srv.ID, Name: srv.Name, Status: mcpdomain.StatusDisconnected, Tools: []mcpdomain.ToolDef{}}
	s.mu.Unlock()
}

func (s *Service) setConnecting(id string) {
	s.mu.Lock()
	if st := s.states[id]; st != nil {
		st.Status = mcpdomain.StatusConnecting
	}
	s.mu.Unlock()
}

func (s *Service) setFailed(id string, err error) {
	now := time.Now().UTC()
	s.mu.Lock()
	if st := s.states[id]; st != nil {
		st.Status = mcpdomain.StatusFailed
		st.LastError = err.Error()
		st.LastErrorAt = &now
	}
	s.mu.Unlock()
}

// ownerFor builds the sandbox owner key for a server's runtime env.
//
// ownerFor 构造 server runtime env 的 sandbox owner key。
func ownerFor(srv *mcpdomain.Server) sandboxdomain.Owner {
	return sandboxdomain.Owner{Kind: sandboxdomain.OwnerKindMCP, ID: srv.ID, Name: srv.Name}
}

// DisconnectWorkspace closes every connected server belonging to the ctx workspace (client +
// sandbox process) — the workspace-delete reaper's mcp step.
//
// DisconnectWorkspace 关闭 ctx workspace 名下全部已连接 server（client + sandbox 进程）——
// workspace 删除 reaper 的 mcp 步。
func (s *Service) DisconnectWorkspace(ctx context.Context) {
	servers, err := s.repo.List(ctx)
	if err != nil {
		s.log.Warn("mcpapp.DisconnectWorkspace: list failed", zap.Error(err))
		return
	}
	for _, srv := range servers {
		s.closeOne(srv.ID)
	}
}
