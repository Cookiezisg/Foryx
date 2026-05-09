// spawn.go — Service.Spawn / Service.SpawnLongLived + Layer A
// (graceful-shutdown handle registry).
//
// Spawn methods translate Owner → env → resolved binary path / cwd / env
// vars, then delegate to infra/sandbox.SpawnOnce / SpawnLongLived for the
// actual exec.Cmd plumbing.
//
// Long-lived handles are tracked in activeHandles so Service.Shutdown can
// kill survivors at app exit (Layer A leak prevention). Per-call cleanup
// stays the caller's responsibility — Wait()/Kill() on the returned
// handle un-registers automatically.
//
// spawn.go ——Service.Spawn / Service.SpawnLongLived + 层 A
// （优雅退出 handle 注册表）。
//
// Spawn 方法把 Owner → env → 解析后的二进制路径 / cwd / env vars，再委托给
// infra/sandbox.SpawnOnce / SpawnLongLived 做真正的 exec.Cmd 编排。
//
// 长生命周期 handle 跟踪在 activeHandles 让 Service.Shutdown 在 app 退出时
// 杀残留（层 A leak 防御）。Per-call 清理仍是调用方责任——返回 handle 的
// Wait()/Kill() 自动反注册。

package sandbox

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	sandboxdomain "github.com/sunweilin/forgify/backend/internal/domain/sandbox"
	sandboxinfra "github.com/sunweilin/forgify/backend/internal/infra/sandbox"
)

// Spawn runs a one-shot command in the env owned by owner. The command
// binary is resolved via EnvManager.EnvBin (when opts.Cmd is a bare name
// like "python") or used as-is (when opts.Cmd is an absolute / relative
// path containing a separator). cwd defaults to EnvManager.EnvDir(envPath).
// Env vars overlay opts.Env onto the inherited os.Environ() (so callers
// can add PATH / overrides without losing the base).
//
// Spawn 在 owner 拥有的 env 内跑一次性命令。命令 binary 通过 EnvManager.EnvBin
// 解析（opts.Cmd 是裸名如 "python" 时）或原样用（opts.Cmd 含分隔符时）。
// cwd 默认 EnvManager.EnvDir(envPath)。Env vars 把 opts.Env 叠加到继承的
// os.Environ()（让调用方可加 PATH / overrides 不丢 base）。
func (s *Service) Spawn(ctx context.Context, owner sandboxdomain.Owner, opts sandboxdomain.SpawnOpts) (*sandboxdomain.ExecutionResult, error) {
	cmd, cwd, env, err := s.prepareSpawn(ctx, owner, opts)
	if err != nil {
		return nil, err
	}

	// Apply per-call timeout if the caller asked for one.
	// 调用方传了 timeout 就应用。
	spawnCtx := ctx
	if opts.Timeout > 0 {
		var cancel context.CancelFunc
		spawnCtx, cancel = context.WithTimeout(ctx, opts.Timeout)
		defer cancel()
	}

	return sandboxinfra.SpawnOnce(spawnCtx, sandboxinfra.SpawnOptions{
		Cmd:   cmd,
		Args:  opts.Args,
		Cwd:   cwd,
		Env:   env,
		Stdin: opts.Stdin,
	})
}

// SpawnLongLived starts a long-running process in owner's env and returns
// a tracked handle. The handle is registered in activeHandles so
// Service.Shutdown can kill it on app exit; Wait()/Kill() on the handle
// un-registers automatically.
//
// SpawnLongLived 在 owner env 起长生命周期进程返跟踪的 handle。Handle
// 注册到 activeHandles 让 Service.Shutdown 在 app 退出时杀；handle 上的
// Wait()/Kill() 自动反注册。
func (s *Service) SpawnLongLived(ctx context.Context, owner sandboxdomain.Owner, opts sandboxdomain.SpawnOpts) (sandboxdomain.LongLivedHandle, error) {
	cmd, cwd, env, err := s.prepareSpawn(ctx, owner, opts)
	if err != nil {
		return nil, err
	}

	inner, err := sandboxinfra.SpawnLongLived(ctx, sandboxinfra.SpawnOptions{
		Cmd:  cmd,
		Args: opts.Args,
		Cwd:  cwd,
		Env:  env,
	})
	if err != nil {
		return nil, err
	}

	id := s.nextHandleID.Add(1)
	envRow, lookupErr := s.repo.FindEnvByOwner(ctx, owner.Kind, owner.ID)
	envID := ""
	if lookupErr == nil {
		envID = envRow.ID
	}
	tracked := &trackedHandle{
		inner:   inner,
		id:      id,
		owner:   owner,
		envID:   envID,
		service: s,
	}
	s.activeHandles.Store(id, tracked)

	// Layer B leak prevention: record PID in manifest so a crash before
	// Wait/Kill leaves a trail for the next boot scan. Best-effort —
	// failures log but don't abort the spawn (the Layer A registry above
	// already protects graceful shutdown).
	//
	// 层 B leak 防御：把 PID 记 manifest，让 Wait/Kill 前 crash 给下次
	// 启动扫描留痕迹。Best-effort——失败 log 不中止 spawn（上面的层 A
	// 注册表已保护优雅 shutdown）。
	if envID != "" {
		if err := s.repo.SetEnvRunningPID(ctx, envID, inner.PID()); err != nil {
			s.log.Warn("sandbox: track running pid failed",
				zap.String("env_id", envID),
				zap.Int("pid", inner.PID()),
				zap.Error(err))
		}
	}
	return tracked, nil
}

// Shutdown kills all active LongLived handles. Called by main.go's
// SIGTERM/SIGINT shutdown hook (Layer A leak prevention). Best-effort:
// kill failures are logged but don't abort other handles. Blocks until
// all kills returned or ctx expires.
//
// Shutdown 杀所有 active LongLived handle。main.go 的 SIGTERM/SIGINT
// shutdown hook 调（层 A leak 防御）。Best-effort：单 kill 失败 log 但不
// 阻挡其他 handle。阻塞直到所有 kill 返或 ctx 过期。
func (s *Service) Shutdown(ctx context.Context) error {
	var wg sync.WaitGroup
	count := 0
	s.activeHandles.Range(func(_, v any) bool {
		t := v.(*trackedHandle)
		count++
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := t.inner.Kill(); err != nil {
				s.log.Warn("sandbox shutdown: kill handle failed",
					zap.Int("pid", t.inner.PID()),
					zap.String("owner_kind", t.owner.Kind),
					zap.String("owner_id", t.owner.ID),
					zap.Error(err))
			}
		}()
		return true
	})

	// Wait for all kills, but bail if ctx expires (preserves shutdown
	// deadline; OS will reap any survivors via Job Object on Windows or
	// PR_SET_PDEATHSIG on Linux when the process actually exits).
	//
	// 等所有 kill，但 ctx 过期就放手（保 shutdown deadline；OS 在进程实际
	// 退出时通过 Windows Job Object 或 Linux PR_SET_PDEATHSIG 收掉剩下的）。
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		s.log.Info("sandbox shutdown: all handles killed", zap.Int("count", count))
		return nil
	case <-ctx.Done():
		s.log.Warn("sandbox shutdown: deadline reached before all handles killed", zap.Int("count", count))
		return ctx.Err()
	}
}

// prepareSpawn does the shared owner/env/binary resolution for Spawn +
// SpawnLongLived. Returns the absolute command path, cwd, and merged
// env vars.
//
// prepareSpawn 做 Spawn + SpawnLongLived 共享的 owner/env/binary 解析。
// 返绝对命令路径、cwd、合并的 env vars。
func (s *Service) prepareSpawn(ctx context.Context, owner sandboxdomain.Owner, opts sandboxdomain.SpawnOpts) (cmd, cwd string, env []string, err error) {
	if !s.IsReady() {
		return "", "", nil, fmt.Errorf("sandboxapp.Spawn: %w", sandboxdomain.ErrSpawnFailed)
	}
	if opts.Cmd == "" {
		// Caller wiring bug: every internal Spawn caller (forge, future
		// workflow run) builds opts with a concrete Cmd. Empty here =
		// future code path bypassed. panic so dev sees the stack
		// rather than masking as 500 unmapped (same approach as
		// apikey.HTTPTester default + mcp.AddServer + sandbox.EnsureEnv).
		//
		// 调用方 wiring bug：每个内部 Spawn caller 都填了 Cmd。空 = 未来
		// 代码绕过——panic 让 dev 看 stack（同 apikey/mcp/sandbox 模式）。
		panic("sandboxapp.Spawn: opts.Cmd is empty — caller wiring bug")
	}

	envRow, err := s.repo.FindEnvByOwner(ctx, owner.Kind, owner.ID)
	if err != nil {
		return "", "", nil, fmt.Errorf("sandboxapp.Spawn: lookup env %s/%s: %w", owner.Kind, owner.ID, err)
	}
	if envRow.Status != sandboxdomain.EnvStatusReady {
		return "", "", nil, fmt.Errorf("sandboxapp.Spawn: env %s status=%s: %w", envRow.ID, envRow.Status, sandboxdomain.ErrSpawnFailed)
	}

	rt, err := s.repo.GetRuntime(ctx, envRow.RuntimeID)
	if err != nil {
		return "", "", nil, fmt.Errorf("sandboxapp.Spawn: lookup runtime %s: %w", envRow.RuntimeID, err)
	}

	s.regMu.RLock()
	em, ok := s.envManagers[rt.Kind]
	s.regMu.RUnlock()
	if !ok {
		return "", "", nil, fmt.Errorf("sandboxapp.Spawn: no env manager for kind %s: %w", rt.Kind, sandboxdomain.ErrRuntimeNotSupported)
	}

	envPath := filepath.Join(s.sandboxRoot, envRow.Path)
	cmd = resolveCmd(em, envPath, opts.Cmd)
	cwd = em.EnvDir(envPath)
	env = mergeEnv(opts.Env)

	// Touch last_used_at on Spawn so GC sees the env as recently active.
	// Best-effort.
	//
	// Spawn 时更新 last_used_at 让 GC 视 env 近期活跃。Best-effort。
	envRow.LastUsedAt = time.Now()
	if updateErr := s.repo.UpdateEnv(ctx, envRow); updateErr != nil {
		s.log.Warn("sandbox: spawn touch last_used_at failed",
			zap.String("env_id", envRow.ID),
			zap.Error(updateErr))
	}
	return cmd, cwd, env, nil
}

// resolveCmd returns the absolute command path. If cmd already looks
// like a path (contains separator or starts with /, ~, .) we use it
// as-is; otherwise we treat it as a bare binary name and look it up
// via EnvManager.EnvBin.
//
// resolveCmd 返绝对命令路径。cmd 已像路径（含分隔符或 / ~ . 起头）原样用；
// 否则当裸 binary 名通过 EnvManager.EnvBin 查。
func resolveCmd(em sandboxdomain.EnvManager, envPath, cmd string) string {
	if filepath.IsAbs(cmd) || strings.ContainsAny(cmd, "/\\") || strings.HasPrefix(cmd, "~") || strings.HasPrefix(cmd, ".") {
		return cmd
	}
	return em.EnvBin(envPath, cmd)
}

// mergeEnv overlays the (key → value) overrides onto the inherited
// process environment, returning a complete []string list suitable
// for exec.Cmd.Env. Existing entries are replaced; new entries appended.
//
// mergeEnv 把 (key → value) overrides 叠加到继承的进程 env，返完整的
// []string list 给 exec.Cmd.Env。已有项替换；新项追加。
func mergeEnv(overrides map[string]string) []string {
	base := os.Environ()
	if len(overrides) == 0 {
		return base
	}
	// Build index from base for O(1) replacement.
	// 给 base 建索引以 O(1) 替换。
	idx := make(map[string]int, len(base))
	for i, kv := range base {
		if eq := strings.IndexByte(kv, '='); eq > 0 {
			idx[kv[:eq]] = i
		}
	}
	out := append([]string(nil), base...)
	for k, v := range overrides {
		entry := k + "=" + v
		if i, ok := idx[k]; ok {
			out[i] = entry
		} else {
			out = append(out, entry)
		}
	}
	return out
}

// trackedHandle wraps a sandboxdomain.LongLivedHandle, un-registering
// itself from activeHandles when Wait/Kill returns. Idempotent — multiple
// Wait/Kill calls work correctly because sync.Map.Delete is a no-op for
// already-deleted keys.
//
// trackedHandle 包 sandboxdomain.LongLivedHandle，Wait/Kill 返时从
// activeHandles 反注册自己。幂等——Wait/Kill 多次调用 OK，因
// sync.Map.Delete 对已删 key 是 no-op。
type trackedHandle struct {
	inner   sandboxdomain.LongLivedHandle
	id      uint64
	owner   sandboxdomain.Owner
	envID   string // empty if envID lookup failed at SpawnLongLived time
	service *Service
}

func (t *trackedHandle) Stdin() io.WriteCloser { return t.inner.Stdin() }
func (t *trackedHandle) Stdout() io.ReadCloser { return t.inner.Stdout() }
func (t *trackedHandle) Stderr() io.ReadCloser { return t.inner.Stderr() }
func (t *trackedHandle) PID() int              { return t.inner.PID() }

func (t *trackedHandle) Wait() error {
	err := t.inner.Wait()
	t.unregister()
	return err
}

func (t *trackedHandle) Kill() error {
	err := t.inner.Kill()
	t.unregister()
	return err
}

// unregister drops the handle from activeHandles AND clears the manifest
// running_pid column (Layer B). Idempotent — sync.Map.Delete and
// ClearEnvRunningPID both no-op on already-cleared state.
//
// unregister 把 handle 从 activeHandles 移除 + 清 manifest running_pid 列
// （层 B）。幂等——sync.Map.Delete 和 ClearEnvRunningPID 对已清状态都 no-op。
func (t *trackedHandle) unregister() {
	t.service.activeHandles.Delete(t.id)
	if t.envID == "" {
		return
	}
	if err := t.service.repo.ClearEnvRunningPID(context.Background(), t.envID); err != nil {
		t.service.log.Warn("sandbox: clear running pid failed",
			zap.String("env_id", t.envID),
			zap.Error(err))
	}
}

