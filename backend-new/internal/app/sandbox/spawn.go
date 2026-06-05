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

// Spawn runs a one-shot command in the owner's env.
//
// Spawn 在 owner env 中执行一次性命令。
func (s *Service) Spawn(ctx context.Context, owner sandboxdomain.Owner, opts sandboxdomain.SpawnOpts) (*sandboxdomain.ExecutionResult, error) {
	cmd, args, cwd, env, _, err := s.prepareSpawn(ctx, owner, opts)
	if err != nil {
		return nil, err
	}

	spawnCtx := ctx
	if opts.Timeout > 0 {
		var cancel context.CancelFunc
		spawnCtx, cancel = context.WithTimeout(ctx, opts.Timeout)
		defer cancel()
	}

	return sandboxinfra.SpawnOnce(spawnCtx, sandboxinfra.SpawnOptions{
		Cmd:   cmd,
		Args:  args,
		Cwd:   cwd,
		Env:   env,
		Stdin: opts.Stdin,
	})
}

// SpawnLongLived starts a long-running process; the handle auto-unregisters on Wait/Kill.
//
// SpawnLongLived 启动长生命周期进程；返回的 handle 在 Wait/Kill 时自动反注册。
func (s *Service) SpawnLongLived(ctx context.Context, owner sandboxdomain.Owner, opts sandboxdomain.SpawnOpts) (sandboxdomain.LongLivedHandle, error) {
	cmd, args, cwd, env, envID, err := s.prepareSpawn(ctx, owner, opts)
	if err != nil {
		return nil, err
	}

	inner, err := sandboxinfra.SpawnLongLived(ctx, sandboxinfra.SpawnOptions{
		Cmd:  cmd,
		Args: args,
		Cwd:  cwd,
		Env:  env,
	})
	if err != nil {
		return nil, fmt.Errorf("sandboxapp.SpawnLongLived: %w", err)
	}

	id := s.nextHandleID.Add(1)
	tracked := &trackedHandle{
		inner:   inner,
		id:      id,
		owner:   owner,
		envID:   envID,
		service: s,
	}
	s.activeHandles.Store(id, tracked)

	if err := s.repo.SetEnvRunningPID(ctx, envID, inner.PID()); err != nil {
		s.log.Warn("sandbox: track running pid failed",
			zap.String("env_id", envID),
			zap.Int("pid", inner.PID()),
			zap.Error(err))
	}
	return tracked, nil
}

// Shutdown kills all active LongLived handles; blocks until done or ctx expires.
//
// Shutdown 杀掉所有活跃 LongLived handle，阻塞直到完成或 ctx 过期。
func (s *Service) Shutdown(ctx context.Context) error {
	var wg sync.WaitGroup
	count := 0
	s.activeHandles.Range(func(_, v any) bool {
		t := v.(*trackedHandle)
		count++
		wg.Go(func() {
			if err := t.inner.Kill(); err != nil {
				s.log.Warn("sandbox shutdown: kill handle failed",
					zap.Int("pid", t.inner.PID()),
					zap.String("owner_kind", t.owner.Kind),
					zap.String("owner_id", t.owner.ID),
					zap.Error(err))
			}
		})
		return true
	})

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
		return fmt.Errorf("sandboxapp.Shutdown: %w", ctx.Err())
	}
}

// prepareSpawn resolves owner → env → host command/args/cwd/env vars. The
// EnvManager assembles cmd+args (a venv binary for python/node, a `docker run`
// wrapper for docker), so spawn.go itself holds no runtime knowledge.
//
// prepareSpawn 把 owner 解析为 env → 宿主 命令/参数/cwd/env vars。EnvManager 组装 cmd+args
// （python/node 为 venv binary，docker 为 `docker run` 包装），故 spawn 本身不持 runtime 知识。
func (s *Service) prepareSpawn(ctx context.Context, owner sandboxdomain.Owner, opts sandboxdomain.SpawnOpts) (cmd string, args []string, cwd string, env []string, envID string, err error) {
	if !s.IsReady() {
		return "", nil, "", nil, "", fmt.Errorf("sandboxapp.Spawn: %w", sandboxdomain.ErrSpawnFailed)
	}
	if opts.Cmd == "" {
		return "", nil, "", nil, "", fmt.Errorf("sandboxapp.Spawn: %w", sandboxdomain.ErrCmdRequired)
	}

	envRow, err := s.repo.FindEnvByOwner(ctx, owner.Kind, owner.ID)
	if err != nil {
		return "", nil, "", nil, "", fmt.Errorf("sandboxapp.Spawn: lookup env %s/%s: %w", owner.Kind, owner.ID, err)
	}
	if envRow.Status != sandboxdomain.EnvStatusReady {
		return "", nil, "", nil, "", fmt.Errorf("sandboxapp.Spawn: env %s status=%s: %w", envRow.ID, envRow.Status, sandboxdomain.ErrSpawnFailed)
	}

	rt, err := s.repo.GetRuntime(ctx, envRow.RuntimeID)
	if err != nil {
		return "", nil, "", nil, "", fmt.Errorf("sandboxapp.Spawn: lookup runtime %s: %w", envRow.RuntimeID, err)
	}

	s.regMu.RLock()
	em, ok := s.envManagers[rt.Kind]
	s.regMu.RUnlock()
	if !ok {
		return "", nil, "", nil, "", fmt.Errorf("sandboxapp.Spawn: no env manager for kind %s: %w", rt.Kind, sandboxdomain.ErrRuntimeNotSupported)
	}

	// rt.Path is passed verbatim as runtimeRef: python/node ignore it (the venv
	// pins the interpreter); docker uses it as the image ref.
	// rt.Path 原样作 runtimeRef：python/node 忽略（venv 钉死解释器）；docker 用作镜像 ref。
	envPath := filepath.Join(s.sandboxRoot, envRow.Path)
	cmd, args, cwd = em.ResolveExec(rt.Path, envPath, opts)
	env = mergeEnv(opts.Env)
	envID = envRow.ID

	envRow.LastUsedAt = time.Now()
	if updateErr := s.repo.UpdateEnv(ctx, envRow); updateErr != nil {
		s.log.Warn("sandbox: spawn touch last_used_at failed",
			zap.String("env_id", envRow.ID),
			zap.Error(updateErr))
	}
	return cmd, args, cwd, env, envID, nil
}

// mergeEnv overlays overrides onto os.Environ(); existing keys are replaced.
//
// mergeEnv 把 overrides 叠加到 os.Environ()，同 key 替换，新 key 追加。
func mergeEnv(overrides map[string]string) []string {
	base := os.Environ()
	if len(overrides) == 0 {
		return base
	}
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

// trackedHandle wraps a LongLivedHandle and auto-unregisters on Wait/Kill.
//
// trackedHandle 包装 LongLivedHandle，Wait/Kill 时自动反注册（幂等）。
type trackedHandle struct {
	inner   sandboxdomain.LongLivedHandle
	id      uint64
	owner   sandboxdomain.Owner
	envID   string
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

func (t *trackedHandle) unregister() {
	t.service.activeHandles.Delete(t.id)
	if err := t.service.repo.ClearEnvRunningPID(context.Background(), t.envID); err != nil {
		t.service.log.Warn("sandbox: clear running pid failed",
			zap.String("env_id", t.envID),
			zap.Error(err))
	}
}
