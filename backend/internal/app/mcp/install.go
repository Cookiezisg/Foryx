package mcp

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"

	loopapp "github.com/sunweilin/anselm/backend/internal/app/loop"
	mcpdomain "github.com/sunweilin/anselm/backend/internal/domain/mcp"
	sandboxdomain "github.com/sunweilin/anselm/backend/internal/domain/sandbox"
	mcpinfra "github.com/sunweilin/anselm/backend/internal/infra/mcp"
	idgenpkg "github.com/sunweilin/anselm/backend/internal/pkg/idgen"
	reqctxpkg "github.com/sunweilin/anselm/backend/internal/pkg/reqctx"
)

const addServerTimeout = 3 * time.Minute // install: provision runtime + first connect (npm/pip fetch can be slow)

// InstallFromRegistry installs a curated entry: pick the best runnable package (or remote),
// provision its runtime env, persist the encrypted config, and connect. A connect failure
// still persists the server (recoverable via reconnect).
//
// InstallFromRegistry 安装一条 curated 条目：挑最优可跑 package（或 remote）、物化 runtime env、
// 存加密 config、连接。连接失败也会留住 server（可经 reconnect 恢复）。
func (s *Service) InstallFromRegistry(ctx context.Context, fullName string, userEnv map[string]string) (*mcpdomain.ServerStatus, error) {
	entry, err := s.registry.Get(ctx, fullName)
	if err != nil {
		return nil, fmt.Errorf("mcpapp.InstallFromRegistry %s: %w", fullName, err)
	}
	plan, ok := entry.Plan()
	if !ok {
		return nil, fmt.Errorf("mcpapp.InstallFromRegistry %s: %w", fullName, mcpdomain.ErrNoRunnablePackage)
	}
	if missing := missingEnv(plan.EnvVars, userEnv); len(missing) > 0 {
		// Carry the missing variable NAMES in Details, not as a `%s` tail after `%w` — the tail lives only
		// in the wrap chain's .Error() string, which errorspkg.Surface strips, so the LLM/HTTP error said
		// "required environment variables missing" with no clue WHICH (F169-M5). Details survives Surface
		// + the N1 envelope, so the agent learns exactly which keys to supply.
		// 缺失变量**名**进 Details、而非 `%w` 后的 `%s` 尾——尾只在包裹链 .Error() 串里、被 errorspkg.Surface 剥掉，
		// 故 LLM/HTTP 错误只说"缺必填环境变量"却不知**是哪些**（F169-M5）。Details 穿过 Surface + N1 envelope，使 agent 知道究竟补哪些键。
		return nil, mcpdomain.ErrEnvMissing.WithDetails(map[string]any{"missing": missing})
	}

	wsID, err := reqctxpkg.RequireWorkspaceID(ctx)
	if err != nil {
		return nil, fmt.Errorf("mcpapp.InstallFromRegistry: %w", err)
	}
	name := shortName(entry.Name)
	if _, err := s.repo.GetByName(ctx, name); err == nil {
		return nil, fmt.Errorf("mcpapp.InstallFromRegistry %s: %w", name, mcpdomain.ErrNameConflict)
	}

	srv := &mcpdomain.Server{
		ID:          idgenpkg.New("mcp"),
		WorkspaceID: wsID,
		Name:        name,
		Description: entry.Description,
		Source:      mcpdomain.SourceRegistry,
		RegistryID:  entry.Name,
		Env:         userEnv,
	}
	if plan.OAuth {
		// Interactive OAuth: discover → register → browser consent → token. Blocks until the user
		// authorizes (or times out); the grant is stored encrypted and refreshed on use. A templated
		// per-tenant URL (e.g. Glean) is resolved from the user-supplied env first.
		// 交互 OAuth：发现→注册→浏览器同意→token。阻塞到用户授权（或超时）；授权加密落盘、用时刷新。
		// 每租户模板 URL（如 Glean）先用用户给的 env 解析。
		url := expandPlaceholders(plan.URL, userEnv)
		clientID := userEnv[plan.OAuthClientIDEnv]         // "" for DCR servers (OAuthClientIDEnv == "")
		clientSecret := userEnv[plan.OAuthClientSecretEnv] // "" for public/PKCE clients
		creds, err := s.authorizeOAuth(ctx, url, clientID, clientSecret, plan.OAuthScopes)
		if err != nil {
			return nil, fmt.Errorf("mcpapp.InstallFromRegistry %s: %w", name, err)
		}
		srv.Transport = plan.Transport
		srv.URL = url
		srv.OAuth = creds
	} else if plan.Remote {
		srv.Transport = plan.Transport
		srv.URL = plan.URL
		srv.Headers = resolveHeaders(plan.Headers, userEnv)
	} else {
		srv.Transport = mcpdomain.TransportStdio
		srv.Runtime = plan.Runtime
		srv.Command = plan.Command
		srv.Args = expandArgs(plan.Args, userEnv) // fill {placeholder} credential args (launchdarkly/logfire)
		if err := s.ensureEnv(ctx, srv); err != nil {
			return nil, fmt.Errorf("mcpapp.InstallFromRegistry %s: %w: %w", name, mcpdomain.ErrInstallFailed, err)
		}
	}

	st, err := s.persistAndConnect(ctx, srv)
	if err == nil {
		s.emitNotif(ctx, "installed", srv.Name)
	}
	return st, err
}

// AddServer upserts a manually-configured server (PUT) and connects; same name replaces.
//
// AddServer 创建/更新手动配置的 server（PUT）并连接；同名替换。
func (s *Service) AddServer(ctx context.Context, srv *mcpdomain.Server) (*mcpdomain.ServerStatus, error) {
	wsID, err := reqctxpkg.RequireWorkspaceID(ctx)
	if err != nil {
		return nil, fmt.Errorf("mcpapp.AddServer: %w", err)
	}
	srv.WorkspaceID = wsID
	if srv.Source == "" {
		srv.Source = mcpdomain.SourceManual
	}
	if srv.IsRemote() {
		srv.Transport = orDefault(srv.Transport, mcpdomain.TransportStreamableHTTP)
	} else {
		srv.Transport = mcpdomain.TransportStdio
		if srv.Runtime == "" {
			srv.Runtime = inferRuntime(srv.Command)
		}
	}

	// Replace an existing same-name server: keep its id, drop its old connection.
	// 替换同名 server：保留 id、断旧连接。
	replaced := false
	if existing, _ := s.repo.GetByName(ctx, srv.Name); existing != nil {
		srv.ID = existing.ID
		s.closeOne(srv.ID)
		replaced = true
	} else if srv.ID == "" {
		srv.ID = idgenpkg.New("mcp")
	}

	if !srv.IsRemote() && srv.Runtime != "" {
		if err := s.ensureEnv(ctx, srv); err != nil {
			return nil, fmt.Errorf("mcpapp.AddServer %s: %w: %w", srv.Name, mcpdomain.ErrInstallFailed, err)
		}
	}
	st, err := s.persistAndConnect(ctx, srv)
	if err == nil {
		if replaced {
			s.emitNotif(ctx, "updated", srv.Name)
		} else {
			s.emitNotif(ctx, "installed", srv.Name)
		}
	}
	return st, err
}

// Import folds a Claude Desktop mcp.json fragment into the store; overwrite=false skips
// name collisions. Returns the imported + skipped names.
//
// Import 把 Claude Desktop mcp.json 片段折叠进存储；overwrite=false 跳过同名。返回 imported + skipped。
func (s *Service) Import(ctx context.Context, entries map[string]mcpinfra.ImportEntry, overwrite bool) (imported, skipped []string, err error) {
	for name, e := range entries {
		if existing, _ := s.repo.GetByName(ctx, name); existing != nil && !overwrite {
			skipped = append(skipped, name)
			continue
		}
		srv := &mcpdomain.Server{Name: name, Source: mcpdomain.SourceImport, Env: e.Env, TimeoutSec: e.TimeoutSec}
		if e.URL != "" {
			srv.URL = e.URL
		} else {
			srv.Command = e.Command
			srv.Args = e.Args
		}
		if _, aerr := s.AddServer(ctx, srv); aerr != nil {
			s.log.Warn("mcpapp.Import: add server failed", zap.String("server", name), zap.Error(aerr))
			continue
		}
		imported = append(imported, name)
	}
	return imported, skipped, nil
}

// RemoveServer disconnects, soft-deletes the row, drops live status, and purges relations.
//
// RemoveServer 断开、软删行、清实时状态、清 relation 边。
func (s *Service) RemoveServer(ctx context.Context, name string) error {
	srv, err := s.repo.GetByName(ctx, name)
	if err != nil {
		return fmt.Errorf("mcpapp.RemoveServer: %w", err)
	}
	s.closeOne(srv.ID)
	if err := s.repo.Delete(ctx, srv.ID); err != nil {
		return fmt.Errorf("mcpapp.RemoveServer: %w", err)
	}
	s.mu.Lock()
	delete(s.states, srv.ID)
	s.mu.Unlock()
	s.notifySearch(ctx, srv.Name)
	s.purgeRelations(ctx, srv.ID, srv.Name) // edges may be keyed by EITHER id or name (F166)
	s.emitNotif(ctx, "removed", srv.Name)
	return nil
}

// persistAndConnect saves the server then connects best-effort (failure → status=failed).
//
// persistAndConnect 存 server 再 best-effort 连接（失败 → status=failed）。
func (s *Service) persistAndConnect(ctx context.Context, srv *mcpdomain.Server) (*mcpdomain.ServerStatus, error) {
	if err := s.repo.Save(ctx, srv); err != nil {
		return nil, fmt.Errorf("mcpapp.persistAndConnect: save: %w", err)
	}
	s.notifySearch(ctx, srv.Name)
	s.initStatus(srv)
	cctx, cancel := context.WithTimeout(ctx, addServerTimeout)
	defer cancel()
	_ = s.connectOne(cctx, srv)
	return s.GetServer(ctx, srv.Name)
}

// ensureEnv provisions the server's runtime env via sandbox. For docker the image ref
// travels in RuntimeSpec.Version (= the server's Command).
//
// ensureEnv 经 sandbox 物化 server 的 runtime env。docker 的镜像 ref 走 RuntimeSpec.Version
// （= server 的 Command）。
func (s *Service) ensureEnv(ctx context.Context, srv *mcpdomain.Server) error {
	spec := sandboxdomain.EnvSpec{Runtime: sandboxdomain.RuntimeSpec{Kind: srv.Runtime}}
	if srv.Runtime == mcpdomain.RuntimeDocker {
		spec.Runtime.Version = srv.Command
	}
	// Stream each install stage live under the install_mcp_server tool_call so the user watches the
	// runtime (npx/uvx/docker pull) get provisioned. nil-safe off a streamed turn (REST / boot
	// reconnect) → a no-op, identical to passing nil.
	//
	// 把每个安装阶段实时流在 install_mcp_server tool_call 下，使用户看 runtime（npx/uvx/docker pull）被物化。
	// 非流式 turn（REST / boot 重连）→ no-op，等同传 nil。
	prog := loopapp.ToolProgress(ctx)
	defer prog.Close()
	_, err := s.sandbox.EnsureEnv(ctx, ownerFor(srv), spec, func(stage, message string, percent int) {
		if percent > 0 {
			prog.Print(fmt.Sprintf("[%s] %s (%d%%)\n", stage, message, percent))
			return
		}
		prog.Print(fmt.Sprintf("[%s] %s\n", stage, message))
	})
	return err
}

// ListRegistry returns every marketplace entry (global/public; HTTP + list_mcp_marketplace).
//
// ListRegistry 返回所有市场条目（全局公共；HTTP + list_mcp_marketplace）。
func (s *Service) ListRegistry(ctx context.Context) ([]mcpdomain.RegistryEntry, error) {
	return s.registry.List(ctx)
}

// --- helpers ---------------------------------------------------------------

// shortName takes the last path segment of a registry slug ("com.microsoft/azure" → "azure").
//
// shortName 取 registry slug 最后一段（"com.microsoft/azure" → "azure"）。
func shortName(full string) string {
	if i := strings.LastIndexByte(full, '/'); i >= 0 {
		return full[i+1:]
	}
	return full
}

// missingEnv reports which REQUIRED env vars the caller didn't supply (optional ones are passed
// through when given, never blocked) — so a server with many optional knobs installs with just its
// required credential.
//
// missingEnv 报告调用方没给的**必填** env（可选的给了就传、从不拦）——使有一堆可选旋钮的 server 只填必填
// 凭据即可装。
func missingEnv(envs []mcpdomain.EnvVar, supplied map[string]string) []string {
	var missing []string
	for _, ev := range envs {
		if !ev.Required {
			continue
		}
		if v, ok := supplied[ev.Name]; !ok || strings.TrimSpace(v) == "" {
			missing = append(missing, ev.Name)
		}
	}
	return missing
}

// resolveHeaders fills "{TOKEN}" placeholders in remote header values from userEnv; a header
// without a name defaults to Authorization.
//
// resolveHeaders 用 userEnv 填 remote header 值里的 "{TOKEN}" 占位；无名 header 默认 Authorization。
func resolveHeaders(headers []mcpdomain.Header, env map[string]string) map[string]string {
	out := make(map[string]string, len(headers))
	for _, h := range headers {
		name := h.Name
		if name == "" {
			name = "Authorization"
		}
		out[name] = expandPlaceholders(h.Value, env)
	}
	return out
}

// expandArgs fills "{X}" placeholders in each launch arg from the user env (credential args).
//
// expandArgs 用用户 env 填每个启动参数里的 "{X}" 占位（凭据参数）。
func expandArgs(args []string, env map[string]string) []string {
	if len(args) == 0 {
		return args
	}
	out := make([]string, len(args))
	for i, a := range args {
		out[i] = expandPlaceholders(a, env)
	}
	return out
}

func expandPlaceholders(s string, env map[string]string) string {
	for k, v := range env {
		s = strings.ReplaceAll(s, "{"+k+"}", v)
	}
	return s
}

// inferRuntime guesses the sandbox runtime from a manual command (npx → node, uvx → python…).
//
// inferRuntime 从手动 command 推断 sandbox runtime（npx → node、uvx → python…）。
func inferRuntime(command string) string {
	switch command {
	case "npx", "npm", "node":
		return mcpdomain.RuntimeNode
	case "uvx", "uv", "python", "python3", "pip", "pipx":
		return mcpdomain.RuntimePython
	case "docker":
		return mcpdomain.RuntimeDocker
	case "dnx", "dotnet":
		return mcpdomain.RuntimeDotnet
	}
	return ""
}

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}
