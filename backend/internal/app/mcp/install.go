// install.go — Service.InstallFromRegistry + Service.Import. The two
// "user adds servers" entry points: marketplace install (one entry by
// name with user-supplied env/args) and drag-import (whole mcp.json
// fragment).
//
// Both end with the same Connect path (delegated to AddServer). The
// real work in InstallFromRegistry is validation + delegating runtime
// install to sandboxapp; the actual server subprocess starts only after
// EnsureEnv reports the env is ready.
//
// install.go ——Service.InstallFromRegistry + Service.Import。两个"用户加
// server"入口：marketplace 装（按 name 一项 + 用户填 env/args）+ 拖拽
// import（整 mcp.json 片段）。两者都最终走 Connect（委托 AddServer）。
// InstallFromRegistry 真正的工作是校验 + 委托 sandboxapp 装 runtime；
// server 子进程在 EnsureEnv 报 env ready 后才起。
package mcp

import (
	"context"
	"fmt"
	"strings"

	"go.uber.org/zap"

	eventlogdomain "github.com/sunweilin/forgify/backend/internal/domain/eventlog"
	mcpdomain "github.com/sunweilin/forgify/backend/internal/domain/mcp"
	sandboxdomain "github.com/sunweilin/forgify/backend/internal/domain/sandbox"
	mcpinfra "github.com/sunweilin/forgify/backend/internal/infra/mcp"
	eventlogpkg "github.com/sunweilin/forgify/backend/internal/pkg/eventlog"
)

// InstallFromRegistry installs a curated catalog entry: validates user-
// supplied env/args against RequiredEnv/RequiredArgs, asks sandboxapp
// to ensure the runtime + dependencies (one-time download for first
// install of each runtime kind), substitutes ${name} tokens in
// InstallCmd.Args, writes the resulting ServerConfig to mcp.json (using
// `name` directly as the key — the curated catalog gives every entry a
// short kebab-case Name, no separate alias concept), and Connects.
//
// Returns ErrAlreadyInstalled when name is already a key in mcp.json.
//
// InstallFromRegistry 装 curated 目录的一项：校 env/args、让 sandboxapp 确保
// runtime + deps、替换 ${name} token、写 mcp.json（直接拿 name 作 key——
// curated 给每条短 kebab-case Name，无独立 alias），Connect。name 已在
// mcp.json 返 ErrAlreadyInstalled。
func (s *Service) InstallFromRegistry(ctx context.Context, name string, env, args map[string]string) (*mcpdomain.ServerStatus, error) {
	entry, err := s.GetRegistryEntry(ctx, name)
	if err != nil {
		return nil, err
	}

	// Collision check before any install work — saves wasted runtime install.
	// 安装前先查冲突——省得 runtime install 白干。
	s.mu.RLock()
	_, collided := s.configs[name]
	s.mu.RUnlock()
	if collided {
		return nil, fmt.Errorf("mcpapp.InstallFromRegistry %s: %w",
			name, mcpdomain.ErrAlreadyInstalled)
	}

	// Validate RequiredEnv: every name listed in entry.RequiredEnv must
	// appear in the user-supplied env map (Secret-flag-checked at UI
	// layer; here we just check presence).
	// 校验 RequiredEnv：entry.RequiredEnv 里每项都必须在用户 env map 出现
	// （Secret 标记由 UI 校验；这里只查存在性）。
	if missing := missingEnvKeys(entry.RequiredEnv, env); len(missing) > 0 {
		return nil, fmt.Errorf("mcpapp.InstallFromRegistry %s: %w: %s",
			name, mcpdomain.ErrRequiredEnvMissing, strings.Join(missing, ", "))
	}
	if missing := missingArgKeys(entry.RequiredArgs, args); len(missing) > 0 {
		return nil, fmt.Errorf("mcpapp.InstallFromRegistry %s: %w: %s",
			name, mcpdomain.ErrRequiredArgsMissing, strings.Join(missing, ", "))
	}

	// Delegate runtime + dep install to sandboxapp. EnvSpec.Runtime
	// .Kind comes from entry.Runtime (node/python/binary); deps and
	// extras are encoded into the InstallCmd which sandboxapp doesn't
	// see directly — rather, the env layout is set up so when the MCP
	// subprocess runs `npx -y @scope/pkg`, the Node runtime that
	// sandboxapp installed is on PATH and the package fetches/runs.
	//
	// 委托 runtime + dep 装到 sandboxapp。EnvSpec.Runtime.Kind 来自
	// entry.Runtime（node/python/binary）；deps 与 extras 编进 InstallCmd
	// sandboxapp 不直接见——而是 env 布局设好后 MCP 子进程跑 `npx -y
	// @scope/pkg` 时 sandboxapp 装的 Node runtime 在 PATH 上，包就能 fetch+run。
	if s.sandbox != nil && entry.Runtime != "" && entry.Runtime != "binary" {
		owner := sandboxdomain.Owner{
			Kind: "mcp",
			ID:   entry.Name,
			Name: entry.Name,
		}
		spec := sandboxdomain.EnvSpec{
			Runtime: sandboxdomain.RuntimeSpec{Kind: entry.Runtime},
		}

		// Event-log Phase 3: surface sandbox install progress as a
		// streaming progress block. parent comes from ctx (runOneTool
		// stamps WithParentBlockID(tc.ID) before invoking install_mcp_server),
		// so this block hangs under the LLM's install_mcp_server tool_call.
		// Empty progID (no emitter / no parent) silently no-ops.
		//
		// 事件日志 Phase 3：sandbox 装包进度作 streaming progress block 推
		// 出。parent 来自 ctx（runOneTool 在 install_mcp_server 调用前打了
		// WithParentBlockID(tc.ID)），所以本 block 挂 LLM 的 install_mcp_server
		// tool_call 下。空 progID（无 emitter / 无父）静默 no-op。
		em := eventlogpkg.From(ctx)
		progID := em.StartBlock(ctx, eventlogdomain.BlockTypeProgress,
			map[string]any{
				"stage":   "installing",
				"runtime": entry.Runtime,
				"server":  entry.Name,
			})
		progress := func(stage, message string, percent int) {
			line := message
			if stage != "" {
				line = "[" + stage + "] " + line
			}
			if percent >= 0 {
				line = fmt.Sprintf("%s (%d%%)", line, percent)
			}
			em.DeltaBlock(ctx, progID, line+"\n")
		}

		_, ensureErr := s.sandbox.EnsureEnv(ctx, owner, spec, progress)
		ensureStatus := eventlogdomain.StatusCompleted
		if ensureErr != nil {
			ensureStatus = eventlogdomain.StatusError
		}
		em.StopBlock(ctx, progID, ensureStatus, ensureErr)

		if ensureErr != nil {
			return nil, fmt.Errorf("mcpapp.InstallFromRegistry %s: %w: %v",
				name, mcpdomain.ErrInstallFailed, ensureErr)
		}
	}

	// Substitute ${name} tokens in InstallCmd.Args using the user's
	// args map. Unsubstituted tokens are left literal — they'll surface
	// when the MCP subprocess receives them and fails, which is the
	// correct behavior (user sees the actual problem).
	// 用用户 args map 替换 InstallCmd.Args 的 ${name} token。未替换的留原样
	// ——MCP 子进程收到失败时暴露，让用户看到真问题（这是正确行为）。
	resolvedArgs := substituteArgs(entry.InstallCmd.Args, args)

	cfg := mcpdomain.ServerConfig{
		Name:    name, // mcp.json key is the curated entry's Name (short slug)
		Command: entry.InstallCmd.Command,
		Args:    resolvedArgs,
		Env:     env,
	}

	if err := s.AddServer(ctx, cfg); err != nil {
		return nil, fmt.Errorf("mcpapp.InstallFromRegistry %s: %w", name, err)
	}

	st, err := s.GetServer(ctx, name)
	if err != nil {
		return nil, err
	}
	return st, nil
}

// Import folds an external mcp.json fragment (e.g. a Claude Desktop
// config the user dragged into the UI) into ~/.forgify/mcp.json. By
// default existing entries are preserved (returned in MergeResult.
// Conflicts so the frontend can prompt for confirmation); pass
// overwrite=true to force-replace.
//
// Each newly-imported server is then Connected. Connect failures are
// captured per-server in ServerStatus + don't block other imports.
//
// Import 把外部 mcp.json 片段（如用户拖入 UI 的 Claude Desktop 配置）叠
// 加到 ~/.forgify/mcp.json。默认保留已存在条目（在 MergeResult.Conflicts
// 返回让前端弹确认）；overwrite=true 强制替换。
//
// 每个新导入的 server 然后 Connect。Connect 失败 per-server 记到
// ServerStatus + 不挡其他 import。
func (s *Service) Import(ctx context.Context, incoming map[string]mcpdomain.ServerConfig, overwrite bool) (mcpinfra.MergeResult, error) {
	s.mu.Lock()
	merged, res := mcpinfra.Merge(s.cloneConfigsLocked(), incoming, overwrite)
	// Mirror back into Service state for any newly-imported names; mark
	// them disconnected so the Connect loop has a state row to update.
	// 把新导入名同步回 Service state；标 disconnected 让 Connect 循环有
	// state 行可更新。
	for _, name := range res.Imported {
		s.configs[name] = merged[name]
		s.states[name] = &mcpdomain.ServerStatus{
			Name:   name,
			Status: mcpdomain.StatusDisconnected,
			Tools:  []mcpdomain.ToolDef{},
		}
		// If overwriting an existing connected server, drop the old
		// client first so connectOne can attach a fresh one.
		// 覆盖现有 connected server 时先扔旧 client，让 connectOne 接新的。
		if c, ok := s.clients[name]; ok {
			_ = c.Close()
			delete(s.clients, name)
		}
	}
	configsCopy := s.cloneConfigsLocked()
	s.mu.Unlock()

	if err := mcpinfra.Save(s.configPath, configsCopy); err != nil {
		return res, fmt.Errorf("mcpapp.Import: save mcp.json: %w", err)
	}

	// Connect imports in parallel; failures captured on ServerStatus.
	// 并发 Connect 导入项；失败记到 ServerStatus。
	for _, name := range res.Imported {
		go func(n string) {
			cctx, cancel := context.WithTimeout(ctx, initializeTimeout)
			defer cancel()
			if err := s.connectOne(cctx, n); err != nil {
				s.log.Warn("mcp imported server connect failed",
					zap.String("server", n), zap.Error(err))
			}
		}(name)
	}
	// We don't wait — let UI poll ServerStatus or watch SSE 'mcp' event
	// for transitions. Returning immediately keeps the import endpoint
	// responsive even when several servers each take seconds to handshake.
	// 不等——UI 轮 ServerStatus 或盯 SSE 'mcp' 事件看转换。立即返让 import
	// 端点不被多 server 各几秒握手拖慢。
	return res, nil
}

// ── helpers ──────────────────────────────────────────────────────────

// missingEnvKeys returns the names from required[] that are absent or
// empty in supplied. Stable sort for deterministic error messages.
//
// missingEnvKeys 返 required[] 中在 supplied 缺失或空的 name。稳定排序让
// 错误消息确定。
func missingEnvKeys(required []mcpdomain.EnvRequirement, supplied map[string]string) []string {
	var missing []string
	for _, req := range required {
		if v, ok := supplied[req.Name]; !ok || strings.TrimSpace(v) == "" {
			missing = append(missing, req.Name)
		}
	}
	return missing
}

// missingArgKeys is the equivalent for ArgRequirement[]. Args with a
// non-empty Default are considered satisfied even if absent.
//
// missingArgKeys 同上但用于 ArgRequirement[]。Default 非空即视为已满足
// 即便 absent。
func missingArgKeys(required []mcpdomain.ArgRequirement, supplied map[string]string) []string {
	var missing []string
	for _, req := range required {
		if v, ok := supplied[req.Name]; ok && strings.TrimSpace(v) != "" {
			continue
		}
		if strings.TrimSpace(req.Default) != "" {
			continue
		}
		missing = append(missing, req.Name)
	}
	return missing
}

// substituteArgs replaces every "${key}" in the args slice with the
// matching map value. Tokens with no match are left as-is.
//
// substituteArgs 把 args slice 中每个 "${key}" 替换为 map 对应值。
// 无匹配的 token 留原样。
func substituteArgs(args []string, vars map[string]string) []string {
	out := make([]string, len(args))
	for i, a := range args {
		out[i] = expandVars(a, vars)
	}
	return out
}


// expandVars does a single-pass scan replacing ${name} tokens. Doesn't
// support nested or escaped tokens — that's beyond what RegistryEntry
// install commands need.
//
// expandVars 单趟扫描替换 ${name} token。不支持嵌套或转义——超出
// RegistryEntry install 命令需要。
func expandVars(s string, vars map[string]string) string {
	var b strings.Builder
	i := 0
	for i < len(s) {
		if i+1 < len(s) && s[i] == '$' && s[i+1] == '{' {
			end := strings.IndexByte(s[i+2:], '}')
			if end >= 0 {
				key := s[i+2 : i+2+end]
				if v, ok := vars[key]; ok {
					b.WriteString(v)
					i += 2 + end + 1
					continue
				}
			}
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}
