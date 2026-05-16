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

// RunInput is the request shape for Service.RunFunction.
//
// RunInput 是 Service.RunFunction 的请求形状。
type RunInput struct {
	FunctionID  string
	VersionID   string
	Input       map[string]any
	TriggeredBy string
}

// RunFunction synchronously executes a function, ensuring env is ready and writing one Execution row.
//
// RunFunction 同步执行 function，先确保 env ready，并写一行 Execution。
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
		Input:      in.Input,
	}
	startedAt := time.Now().UTC()
	res, sandboxErr := s.sandbox.Run(ctx, runRequest)
	// Lazy rebuild if env was destroyed externally; retry once.
	// 外部销毁导致 env 找不到时按存档重建并重试一次。
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

// syncEnvSync materializes the venv synchronously and writes terminal EnvStatus to DB + v in place.
//
// syncEnvSync 同步物化 venv，终态写 DB 并镜像到 v。
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
