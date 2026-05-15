// run.go — Service.RunFunction + syncEnvSync helper.
//
// Per D-redo-9 (forge_redesign 2026-05-12) there is no async env sync path —
// env materialization happens synchronously inside Service.Create / Edit (and
// in-flight here in RunFunction when an evicted version is invoked). The
// prior SyncEnvForVersion fire-and-forget goroutine + Service.Resync entry
// have been removed; rebuild-env is now done via Edit with empty ops.
//
// RunFunction is the synchronous "execute version X with inputs Y" entry
// called by the run_function LLM tool and HTTP :run endpoint. It ensures the
// env is ready (in-flight sync if the version's env was evicted between
// accept and run) then delegates to Sandbox.Run. Every call writes one
// terminal Execution row (D22).
//
// EnvStatus state machine: pending → syncing → ready / failed.
// `evicted` is set by Sandbox GC (not here); in-flight sync re-builds it.
//
// run.go —— Service.RunFunction + syncEnvSync。env sync 同步发生(D-redo-9),
// 无后台 goroutine。RunFunction 调时若版本 env 被 evict,这里 in-flight 重建。

package function

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.uber.org/zap"

	functiondomain "github.com/sunweilin/forgify/backend/internal/domain/function"
	sandboxdomain "github.com/sunweilin/forgify/backend/internal/domain/sandbox"
	idgenpkg "github.com/sunweilin/forgify/backend/internal/pkg/idgen"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

// RunInput is the request shape for Service.RunFunction. Cancellation is
// caller-driven only — HTTP client disconnect / LLM tool ctx cancel both
// propagate through r.Context() to kill the sandbox process tree. No
// per-call timeout knob (forge_redesign decision 2026-05-12).
//
// RunInput 是 Service.RunFunction 的请求形状。取消只走 caller ctx(HTTP
// 断连 / LLM 工具 ctx cancel 一路传到 sandbox),无 per-call timeout。
type RunInput struct {
	FunctionID  string
	VersionID   string         // optional;empty = use Function.ActiveVersionID
	Input       map[string]any // kwargs passed to the user's def
	TriggeredBy string         // chat / workflow / http / test;default "http"
}

// RunFunction synchronously executes a function. Ensures env is ready first
// (kicks off a synchronous Sync if EnvStatus != ready), then delegates to
// Sandbox.Run. Always writes one terminal Execution row (D22) to
// function_executions; record write uses detached ctx (§S9) so caller cancel
// doesn't lose the log.
//
// RunFunction 同步执行 function。先确保 env ready,再委托 Sandbox.Run。
// 终态(成功/失败/timeout/cancel)写一行 Execution 到 function_executions
// (D22),用 detached ctx(§S9)防 cancel 丢日志。
func (s *Service) RunFunction(ctx context.Context, in RunInput) (*functiondomain.ExecutionResult, error) {
	uid, err := reqctxpkg.RequireUserID(ctx)
	if err != nil {
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

	runRequest := RunRequest{
		FunctionID: in.FunctionID,
		VersionID:  versionID,
		EnvID:      v.EnvID,
		Code:       v.Code,
		// EntryFunction left empty — SandboxAdapter.Run extracts `def name`
		// from the code; the function's user-facing Name (kebab-case allowed)
		// doesn't have to equal the Python identifier.
		Input: in.Input,
	}
	startedAt := time.Now().UTC()
	res, sandboxErr := s.sandbox.Run(ctx, runRequest)
	// #15 lazy rebuild: if env was destroyed externally (admin :destroy or
	// disk wipe), v.EnvStatus is still "ready" in DB but Spawn finds no row.
	// Rebuild from stored spec and retry once. Repeat NOT_FOUND after rebuild
	// is a real bug — let it surface so we don't loop forever.
	// #15 lazy rebuild: 外部销毁(admin :destroy / 磁盘清)后 v.EnvStatus
	// 仍记 ready,Spawn 找不到行;按存档 spec 重建并重试一次。
	if sandboxErr != nil && errors.Is(sandboxErr, sandboxdomain.ErrEnvNotFound) {
		s.log.Info("function env evicted externally; rebuilding then retrying run",
			zap.String("functionId", in.FunctionID),
			zap.String("versionId", versionID),
			zap.String("envId", v.EnvID))
		if err := s.syncEnvSync(ctx, v); err != nil {
			return nil, fmt.Errorf("functionapp.RunFunction: rebuild after evict: %w", err)
		}
		runRequest.EnvID = v.EnvID
		res, sandboxErr = s.sandbox.Run(ctx, runRequest)
	}
	endedAt := time.Now().UTC()

	s.recordExecution(ctx, uid, in, v, startedAt, endedAt, res, sandboxErr, ctx.Err())

	if sandboxErr != nil {
		return nil, fmt.Errorf("functionapp.RunFunction: %w", sandboxErr)
	}
	return res, nil
}

// recordExecution writes one terminal Execution row capturing the outcome.
// Best-effort: errors are logged but do not bubble to the caller (a failed
// log row shouldn't surface as a function failure). Uses detached ctx so
// caller cancel doesn't lose the write.
//
// recordExecution 写一行 Execution(详 D22)。best-effort——写失败仅 log;
// 用 detached ctx 防 cancel 丢日志。
func (s *Service) recordExecution(
	ctx context.Context,
	uid string,
	in RunInput,
	v *functiondomain.Version,
	startedAt, endedAt time.Time,
	res *functiondomain.ExecutionResult,
	sandboxErr error,
	runCtxErr error,
) {
	status := functiondomain.ExecutionStatusOK
	errorMessage := ""
	var output any
	if sandboxErr != nil {
		status = functiondomain.ExecutionStatusFailed
		errorMessage = sandboxErr.Error()
		if errors.Is(runCtxErr, context.DeadlineExceeded) {
			status = functiondomain.ExecutionStatusTimeout
		} else if errors.Is(runCtxErr, context.Canceled) {
			status = functiondomain.ExecutionStatusCancelled
		}
	} else if res != nil {
		if !res.OK {
			status = functiondomain.ExecutionStatusFailed
			errorMessage = res.ErrorMsg
		}
		output = res.Output
	}

	triggeredBy := in.TriggeredBy
	if triggeredBy == "" {
		triggeredBy = functiondomain.TriggeredByHTTP
	}

	convID, _ := reqctxpkg.GetConversationID(ctx)
	msgID, _ := reqctxpkg.GetMessageID(ctx)
	toolCallID, _ := reqctxpkg.GetToolCallID(ctx)

	exec := &functiondomain.Execution{
		ID:             idgenpkg.New("fne"),
		UserID:         uid,
		Status:         status,
		TriggeredBy:    triggeredBy,
		Input:          in.Input,
		Output:         output,
		ErrorCode:      "",
		ErrorMessage:   errorMessage,
		ElapsedMs:      endedAt.Sub(startedAt).Milliseconds(),
		StartedAt:      startedAt,
		EndedAt:        endedAt,
		ConversationID: convID,
		MessageID:      msgID,
		ToolCallID:     toolCallID,
		FunctionID:     in.FunctionID,
		VersionID:      v.ID,
		PythonVersion:  v.PythonVersion,
	}

	detached := reqctxpkg.SetUserID(context.Background(), uid)
	if err := s.repo.SaveExecution(detached, exec); err != nil {
		s.log.Warn("functionapp.recordExecution: SaveExecution failed (best-effort)",
			zap.String("functionId", in.FunctionID),
			zap.String("versionId", v.ID),
			zap.Error(err))
	}
}

// syncEnvSync runs the venv materialization synchronously, writes terminal
// EnvStatus to DB, mutates v in place so the caller sees the latest state
// without re-reading. Env terminal state is surfaced via the returned
// v.EnvStatus / v.EnvError + the LLM tool's tool_result (D-redo-7 removed
// env_synced / env_failed notification actions — UI fetches via GET).
//
// syncEnvSync 同步物化 venv,终态写 DB + 镜像到 v(调用方不必 re-read);
// 终态信息走 v.EnvStatus/EnvError + LLM tool_result(D-redo-7 删 env_synced/
// env_failed 通知,UI 经 GET 拉)。
func (s *Service) syncEnvSync(ctx context.Context, v *functiondomain.Version) error {
	now := time.Now().UTC()
	_ = s.repo.UpdateVersionEnv(ctx, v.ID,
		functiondomain.EnvStatusSyncing, "", "starting", "", nil)
	v.EnvStatus = functiondomain.EnvStatusSyncing

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
		v.EnvStatus = functiondomain.EnvStatusFailed
		v.EnvError = stderr
		v.EnvSyncStage = "failed"
		v.EnvSyncDetail = ""
		v.EnvSyncedAt = &now
		// env_failed / env_synced notifications were removed in C3 (D-redo-7);
		// the env terminal state is carried by the LLM tool_result and the
		// UI fetches via GET /functions/{id}. Keeping notification action
		// surface lean (D-redo-6 slim payload model).
		// env_failed / env_synced 通知在 C3 删除(D-redo-7);终态走 LLM
		// tool_result + UI GET 拉取。
		return fmt.Errorf("sandbox.Sync: %w", err)
	}

	syncedAt := time.Now().UTC()
	if err := s.repo.UpdateVersionEnv(ctx, v.ID,
		functiondomain.EnvStatusReady, "", "ready", "", &syncedAt); err != nil {
		return fmt.Errorf("UpdateVersionEnv ready: %w", err)
	}
	v.EnvStatus = functiondomain.EnvStatusReady
	v.EnvError = ""
	v.EnvSyncStage = "ready"
	v.EnvSyncDetail = ""
	v.EnvSyncedAt = &syncedAt
	return nil
}
