package function

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.uber.org/zap"

	functiondomain "github.com/sunweilin/anselm/backend/internal/domain/function"
	sandboxdomain "github.com/sunweilin/anselm/backend/internal/domain/sandbox"
	idgenpkg "github.com/sunweilin/anselm/backend/internal/pkg/idgen"
	limitspkg "github.com/sunweilin/anselm/backend/internal/pkg/limits"
	reqctxpkg "github.com/sunweilin/anselm/backend/internal/pkg/reqctx"
)

// RunInput is the request shape for RunFunction. TriggeredBy is the execution body
// (chat / agent / workflow / manual); empty defaults to manual.
//
// RunInput 是 RunFunction 的请求形状。TriggeredBy 是执行体（chat / agent / workflow /
// manual）；空默认 manual。
type RunInput struct {
	FunctionID  string
	VersionID   string // empty → active version
	Input       map[string]any
	TriggeredBy string
}

// RunFunction synchronously runs a function: ensure its env is ready (rebuilding on
// demand if it was reclaimed), spawn the code, and write one Execution audit row.
//
// RunFunction 同步运行 function：确保 env 就绪（被回收则按需重建）、spawn 代码、写一行
// Execution 审计。
func (s *Service) RunFunction(ctx context.Context, in RunInput) (*functiondomain.ExecutionResult, error) {
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
		if ready, errMsg := s.ensureEnv(ctx, v, nil); !ready {
			return nil, fmt.Errorf("functionapp.RunFunction: %s: %w", errMsg, functiondomain.ErrEnvNotReady)
		}
	}

	// Normalize nil input to {} BEFORE the runner: the driver does f(**input), and a nil
	// map marshals to JSON `null` → f(**None) → TypeError. A no-arg caller (sensor poll,
	// a workflow node with no input wiring) must not crash a zero-arg function.
	// 在 runner 前把 nil input 归一成 {}：driver 做 f(**input)，nil map 序列化成 JSON `null`
	// → f(**None) → TypeError。无参调用方（sensor 轮询、无 input 接线的 workflow 节点）不该
	// 把零参函数搞崩。
	input := in.Input
	if input == nil {
		input = map[string]any{}
	}
	owner := envOwner(v.FunctionID, v.EnvID)
	// Bound the run's wall clock so a runaway / infinite-loop function can't pin a worker (esp. a
	// workflow node, which has no client to navigate away and cancel) — the deadline descends into the
	// sandbox exec ctx (pgroup-SIGKILL) and surfaces as the durable ExecutionStatusTimeout. Mirrors
	// handler/call.go + mcp/calltool.go. (Bounding the OUTER ctx, not SpawnOpts.Timeout, is what makes
	// ctx.Err()==DeadlineExceeded reachable in recordExecution — a child Spawn deadline would not.)
	// 限运行墙钟，使失控/死循环 function 不钉死 worker（尤其 workflow 节点：无客户端可取消）——deadline 下沉到
	// sandbox exec ctx（pgroup-SIGKILL）、记为 durable ExecutionStatusTimeout。对标 handler/call.go + mcp/calltool.go。
	cctx, cancel := context.WithTimeout(ctx, time.Duration(limitspkg.Current().Timeout.FunctionRunSec)*time.Second)
	defer cancel()
	startedAt := time.Now().UTC()
	res, sandboxErr := s.runner.Run(cctx, owner, v.FunctionID, v.ID, v.Code, input)

	// Env reclaimed externally (GC / manual cleanup): rebuild from the version snapshot
	// and retry once.
	// env 被外部回收（GC / 手工清理）：按版本快照重建并重试一次。
	if sandboxErr != nil && errors.Is(sandboxErr, sandboxdomain.ErrEnvNotFound) {
		s.log.Info("function env reclaimed; rebuilding then retrying",
			zap.String("functionId", v.FunctionID), zap.String("versionId", v.ID))
		if ready, _ := s.ensureEnv(ctx, v, nil); ready {
			res, sandboxErr = s.runner.Run(cctx, owner, v.FunctionID, v.ID, v.Code, input)
		}
	}
	endedAt := time.Now().UTC()

	s.recordExecution(ctx, in, v, startedAt, endedAt, res, sandboxErr, cctx.Err())

	if sandboxErr != nil {
		return nil, fmt.Errorf("functionapp.RunFunction: %w", sandboxErr)
	}
	return res, nil
}

// recordExecution writes one terminal Execution row (best-effort, on a detached context
// preserving the workspace so a cancelled run's record still lands).
//
// recordExecution 写一行终态 Execution（best-effort，用保留 workspace 的 detached ctx，
// 使被取消的运行记录仍落库）。
func (s *Service) recordExecution(ctx context.Context, in RunInput, v *functiondomain.Version, startedAt, endedAt time.Time, res *functiondomain.ExecutionResult, sandboxErr, runCtxErr error) {
	status := functiondomain.ExecutionStatusOK
	errMsg := ""
	var output any
	switch {
	case sandboxErr != nil:
		status = functiondomain.ExecutionStatusFailed
		errMsg = sandboxErr.Error()
		if errors.Is(runCtxErr, context.DeadlineExceeded) {
			status = functiondomain.ExecutionStatusTimeout
			// A clear run-duration message: the raw sandbox "spawn process timeout" connotes a process
			// LAUNCH failure and mis-leads :triage into chasing a phantom env/cold-start problem (F105;
			// the status itself is already correct, F97). Mirrors handler "instance RPC timeout" / mcp.
			//
			// 清晰的运行时长消息：裸 sandbox "spawn process timeout" 暗示进程**启动**失败，误导 :triage 去
			// 追幻象的 env/冷启动问题（F105；status 本身已对，F97）。镜像 handler "instance RPC timeout" / mcp。
			errMsg = fmt.Sprintf("function run exceeded the %ds wall-clock limit", limitspkg.Current().Timeout.FunctionRunSec)
		} else if errors.Is(runCtxErr, context.Canceled) {
			status = functiondomain.ExecutionStatusCancelled
			errMsg = "function run cancelled"
		}
	case res != nil:
		if !res.OK {
			status = functiondomain.ExecutionStatusFailed
			errMsg = res.ErrorMsg
		}
		output = res.Output
	}
	logs := ""
	if res != nil {
		logs = res.Logs
	}

	triggeredBy := in.TriggeredBy
	if !functiondomain.IsValidTrigger(triggeredBy) {
		triggeredBy = functiondomain.TriggeredByManual
	}
	input := in.Input
	if input == nil {
		input = map[string]any{}
	}

	// Provenance comes off ctx: chat identity (conversation/message/toolCall) from the loop,
	// flowrun identity from the scheduler's dispatch injection — whichever path ran us.
	// 溯源取自 ctx：chat 身份（conversation/message/toolCall）来自 loop，flowrun 身份来自调度器
	// 派发注入——哪条路径跑的就带哪份。
	convID, _ := reqctxpkg.GetConversationID(ctx)
	msgID, _ := reqctxpkg.GetMessageID(ctx)
	toolCallID, _ := reqctxpkg.GetToolCallID(ctx)
	flowrunID, _ := reqctxpkg.GetFlowrunID(ctx)
	flowrunNodeID, _ := reqctxpkg.GetFlowrunNodeID(ctx)

	exec := &functiondomain.Execution{
		ID:             idgenpkg.New("fne"),
		FunctionID:     v.FunctionID,
		VersionID:      v.ID,
		Status:         status,
		TriggeredBy:    triggeredBy,
		Input:          input,
		Output:         output,
		ErrorMessage:   errMsg,
		Logs:           logs,
		ElapsedMs:      endedAt.Sub(startedAt).Milliseconds(),
		StartedAt:      startedAt,
		EndedAt:        endedAt,
		ConversationID: convID,
		MessageID:      msgID,
		ToolCallID:     toolCallID,
		FlowrunID:      flowrunID,
		FlowrunNodeID:  flowrunNodeID,
	}

	wsID, _ := reqctxpkg.GetWorkspaceID(ctx)
	detached := reqctxpkg.Detached(wsID)
	if err := s.repo.SaveExecution(detached, exec); err != nil {
		s.log.Warn("functionapp.recordExecution: save failed (best-effort)",
			zap.String("functionId", v.FunctionID), zap.String("versionId", v.ID), zap.Error(err))
	}
}
