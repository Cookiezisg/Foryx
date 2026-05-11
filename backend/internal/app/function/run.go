// run.go — Service.RunFunction + SyncEnv helpers (Task 12).
//
// Two responsibilities:
//
//  1. SyncEnvForVersion — fire-and-forget background job that materializes the
//     venv for a FunctionVersion, streams stage progress to UpdateVersionEnv,
//     publishes env_synced / env_failed notifications.
//
//  2. RunFunction — synchronous "execute version X with inputs Y" entry called
//     by the run_function LLM tool and HTTP :run endpoint. Ensures the env is
//     ready first (synchronous Sync if not), then delegates to Sandbox.Run.
//
// EnvStatus state machine (per spec):
//   pending → syncing → ready
//                    → failed
//                    → evicted (set by Sandbox GC, not here)
//
// run.go —— Service.RunFunction + SyncEnv helpers(Task 12)。
//
// 1) SyncEnvForVersion 后台 fire-and-forget,物化版本 venv,流式回写 env 字段
//    + 推 env_synced / env_failed 通知。
// 2) RunFunction 同步入口,先确保 env ready(否则就地 sync)再委托 Sandbox.Run。

package function

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.uber.org/zap"

	functiondomain "github.com/sunweilin/forgify/backend/internal/domain/function"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

// RunInput is the request shape for Service.RunFunction.
//
// RunInput 是 Service.RunFunction 的请求形状。
type RunInput struct {
	FunctionID    string
	VersionID     string         // optional;empty = use Function.ActiveVersionID
	Input         map[string]any // kwargs passed to the user's def
	Timeout       time.Duration  // 0 = no per-call timeout (sandbox / ctx cancel only)
}

// RunFunction synchronously executes a function. Ensures env is ready first
// (kicks off a synchronous Sync if EnvStatus != ready), then delegates to
// Sandbox.Run. Returns the ExecutionResult unchanged.
//
// RunFunction 同步执行 function。先确保 env ready(否则 in-flight Sync),
// 再委托 Sandbox.Run,直接返 ExecutionResult。
func (s *Service) RunFunction(ctx context.Context, in RunInput) (*functiondomain.ExecutionResult, error) {
	if _, err := reqctxpkg.RequireUserID(ctx); err != nil {
		return nil, fmt.Errorf("functionapp.RunFunction: %w", err)
	}
	f, err := s.repo.GetFunction(ctx, in.FunctionID)
	if err != nil {
		return nil, fmt.Errorf("functionapp.RunFunction: %w", err)
	}
	versionID := in.VersionID
	if versionID == "" {
		versionID = f.ActiveVersionID
	}
	if versionID == "" {
		return nil, fmt.Errorf("functionapp.RunFunction: %w", functiondomain.ErrNoActiveVersion)
	}
	v, err := s.repo.GetVersion(ctx, versionID)
	if err != nil {
		return nil, fmt.Errorf("functionapp.RunFunction: %w", err)
	}

	if v.EnvStatus != functiondomain.EnvStatusReady {
		if err := s.syncEnvSync(ctx, v); err != nil {
			return nil, fmt.Errorf("functionapp.RunFunction: %w", err)
		}
	}

	runCtx := ctx
	if in.Timeout > 0 {
		var cancel context.CancelFunc
		runCtx, cancel = context.WithTimeout(ctx, in.Timeout)
		defer cancel()
	}

	res, err := s.sandbox.Run(runCtx, RunRequest{
		FunctionID: in.FunctionID,
		VersionID:  versionID,
		EnvID:      v.EnvID,
		Code:       v.Code,
		// EntryFunction left empty — SandboxAdapter.Run extracts `def name`
		// from the code; the function's user-facing Name (kebab-case allowed)
		// doesn't have to equal the Python identifier.
		Input: in.Input,
	})
	if err != nil {
		return nil, fmt.Errorf("functionapp.RunFunction: %w", err)
	}
	return res, nil
}

// SyncEnvForVersion kicks off a background goroutine that materializes the
// venv for a FunctionVersion. Returns immediately. Intended for the post-
// accept hook (HTTP handler / LLM tool calls this after CreatePending /
// AcceptPending so the user sees progress streaming without blocking the
// API response).
//
// Uses a detached context (per §S9) so the caller's request ctx cancellation
// does not abort the sync — the env materialization should outlive the
// request that initiated it.
//
// SyncEnvForVersion 后台起 goroutine 物化版本 venv,立即返。给 accept 后钩用
// (HTTP handler / LLM tool 调用后用户看进度不阻塞 API 响应)。用 detached
// context(per §S9)让 caller request cancel 不杀同步——env 物化应活过发起
// 请求。
func (s *Service) SyncEnvForVersion(ctx context.Context, versionID string) {
	uid, err := reqctxpkg.RequireUserID(ctx)
	if err != nil {
		s.log.Warn("functionapp.SyncEnvForVersion: no user id in ctx", zap.String("versionId", versionID))
		return
	}
	v, err := s.repo.GetVersion(ctx, versionID)
	if err != nil {
		s.log.Warn("functionapp.SyncEnvForVersion: GetVersion failed", zap.String("versionId", versionID), zap.Error(err))
		return
	}
	go func() {
		detached := reqctxpkg.SetUserID(context.Background(), uid)
		if err := s.syncEnvSync(detached, v); err != nil {
			s.log.Warn("functionapp.SyncEnvForVersion: sync failed", zap.String("versionId", versionID), zap.Error(err))
		}
	}()
}

// syncEnvSync runs the venv materialization synchronously and writes terminal
// EnvStatus + publishes env_synced / env_failed notification on completion.
// Stage progress is streamed via OnProgress → UpdateVersionEnv.
//
// syncEnvSync 同步跑 venv 物化,终态写 EnvStatus + 推通知;stage 进度经
// OnProgress 写 UpdateVersionEnv。
func (s *Service) syncEnvSync(ctx context.Context, v *functiondomain.Version) error {
	now := time.Now().UTC()
	_ = s.repo.UpdateVersionEnv(ctx, v.ID,
		functiondomain.EnvStatusSyncing, "", "starting", "", nil)

	onProgress := func(stage, detail string) {
		_ = s.repo.UpdateVersionEnv(ctx, v.ID,
			functiondomain.EnvStatusSyncing, "", stage, detail, nil)
	}

	req := SyncRequest{
		FunctionID:    v.FunctionID,
		VersionID:     v.ID,
		EnvID:         v.EnvID,
		Dependencies:  v.Dependencies,
		PythonVersion: v.PythonVersion,
		OnProgress:    onProgress,
	}
	if err := s.sandbox.Sync(ctx, req); err != nil {
		stderr := err.Error()
		var syncErr *SyncError
		if errors.As(err, &syncErr) {
			stderr = syncErr.Stderr
		}
		_ = s.repo.UpdateVersionEnv(ctx, v.ID,
			functiondomain.EnvStatusFailed, stderr, "failed", "", &now)
		s.publish(ctx, v.FunctionID, "env_failed", map[string]any{"versionId": v.ID, "error": stderr})
		return fmt.Errorf("sandbox.Sync: %w", err)
	}

	syncedAt := time.Now().UTC()
	if err := s.repo.UpdateVersionEnv(ctx, v.ID,
		functiondomain.EnvStatusReady, "", "ready", "", &syncedAt); err != nil {
		return fmt.Errorf("UpdateVersionEnv ready: %w", err)
	}
	v.EnvStatus = functiondomain.EnvStatusReady
	v.EnvSyncedAt = &syncedAt
	s.publish(ctx, v.FunctionID, "env_synced", map[string]any{"versionId": v.ID})
	return nil
}
