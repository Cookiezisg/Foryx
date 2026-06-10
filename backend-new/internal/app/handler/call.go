package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"go.uber.org/zap"

	entitystreamapp "github.com/sunweilin/forgify/backend/internal/app/entitystream"
	handlerdomain "github.com/sunweilin/forgify/backend/internal/domain/handler"
	streamdomain "github.com/sunweilin/forgify/backend/internal/domain/stream"
	handlerinfra "github.com/sunweilin/forgify/backend/internal/infra/handler"
	idgenpkg "github.com/sunweilin/forgify/backend/internal/pkg/idgen"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

// CallInput is the request shape for Service.Call. TriggeredBy is the execution body;
// empty is derived from ctx (subagent → agent, else chat). HTTP passes manual.
//
// CallInput 是 Service.Call 的请求形状。TriggeredBy 是执行体；空则按 ctx 推（有 subagent → agent，
// 否则 chat）。HTTP 传 manual。
type CallInput struct {
	HandlerID   string
	HandlerName string
	Method      string
	Args        map[string]any
	TriggeredBy string
	OnProgress  func(any)
}

// Call dispatches a method on the handler's resident instance (spawning it if needed),
// records one Call audit row, and maps crash / timeout to domain errors.
//
// Call 在 handler 的常驻实例上派发方法调用（需要则起实例）、写一行 Call 审计、把 crash / timeout
// 映射成 domain 错误。
func (s *Service) Call(ctx context.Context, in CallInput) (any, error) {
	h, err := s.resolveHandler(ctx, in.HandlerID, in.HandlerName)
	if err != nil {
		return nil, fmt.Errorf("handlerapp.Call: %w", err)
	}
	if h.ActiveVersionID == "" {
		return nil, fmt.Errorf("handlerapp.Call: %w", handlerdomain.ErrNoActiveVersion)
	}

	inst, err := s.manager.Get(ctx, h.ID)
	if err != nil {
		return nil, fmt.Errorf("handlerapp.Call: %w", err)
	}

	// Tee the method's yields onto the handler's entities run terminal (entity panel, all callers)
	// in addition to the caller's progress sink (messages, chat). Always StreamCall — doCall is safe
	// for a non-streaming method (no yields → onProgress never fires → plain return).
	//
	// 把 method 的 yield tee 到 handler 的 entities run 终端（实体面板，全 caller）+ 调用方的进度 sink
	// （messages，chat）。一律 StreamCall——doCall 对非流式 method 安全（无 yield → onProgress 不触发 → 正常返回）。
	runTerm := entitystreamapp.New(ctx, s.entities, streamdomain.Scope{Kind: streamdomain.KindHandler, ID: h.ID}, entitystreamapp.NodeRun, nil)
	onProgress := func(v any) {
		if in.OnProgress != nil {
			in.OnProgress(v)
		}
		_, _ = runTerm.Write(yieldBytes(v))
	}
	startedAt := time.Now().UTC()
	result, err := inst.Client.StreamCall(ctx, in.Method, in.Args, onProgress)
	endedAt := time.Now().UTC()
	if err != nil {
		runTerm.Close("error", nil)
	} else {
		runTerm.Close("completed", nil)
	}

	callErr := s.mapCallErr(ctx, err)
	s.recordCall(ctx, h, inst, in, startedAt, endedAt, result, callErr, ctx.Err())
	return result, callErr
}

// yieldBytes renders a streaming method's yield (any) as one line for the entities run terminal:
// a string goes verbatim, anything else as compact JSON.
//
// yieldBytes 把流式 method 的 yield（any）渲成 entities run 终端的一行：string 原样，其余 compact JSON。
func yieldBytes(v any) []byte {
	if s, ok := v.(string); ok {
		return []byte(s + "\n")
	}
	b, _ := json.Marshal(v)
	return append(b, '\n')
}

// mapCallErr maps infra client errors to domain errors for HTTP status mapping.
//
// mapCallErr 把 infra client 错误映射成 domain 错误，以便 HTTP 状态码映射。
func (s *Service) mapCallErr(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return fmt.Errorf("%w: %v", handlerdomain.ErrInstanceRPCTimeout, err)
	}
	if errors.Is(err, handlerinfra.ErrCrashed) {
		return fmt.Errorf("%w: %v", handlerdomain.ErrInstanceCrashed, err)
	}
	return err // ErrCallFailed (the method raised) — passes through with the Python traceback
}

func (s *Service) resolveHandler(ctx context.Context, id, name string) (*handlerdomain.Handler, error) {
	switch {
	case id != "":
		return s.repo.GetHandler(ctx, id)
	case name != "":
		return s.repo.GetHandlerByName(ctx, name)
	default:
		return nil, fmt.Errorf("handlerName or handlerID required")
	}
}

func (s *Service) recordCall(ctx context.Context, h *handlerdomain.Handler, inst *Instance, in CallInput, startedAt, endedAt time.Time, result any, callErr, runCtxErr error) {
	status := handlerdomain.CallStatusOK
	errMsg := ""
	if callErr != nil {
		status = handlerdomain.CallStatusFailed
		errMsg = callErr.Error()
		if errors.Is(runCtxErr, context.DeadlineExceeded) {
			status = handlerdomain.CallStatusTimeout
		} else if errors.Is(runCtxErr, context.Canceled) {
			status = handlerdomain.CallStatusCancelled
		}
	}

	triggeredBy := in.TriggeredBy
	if !handlerdomain.IsValidTrigger(triggeredBy) {
		triggeredBy = triggerFromCtx(ctx)
	}
	input := in.Args
	if input == nil {
		input = map[string]any{}
	}

	convID, _ := reqctxpkg.GetConversationID(ctx)
	msgID, _ := reqctxpkg.GetMessageID(ctx)

	call := &handlerdomain.Call{
		ID:             idgenpkg.New("hcl"),
		HandlerID:      h.ID,
		VersionID:      h.ActiveVersionID,
		Method:         in.Method,
		Status:         status,
		TriggeredBy:    triggeredBy,
		Input:          input,
		Output:         result,
		ErrorMessage:   errMsg,
		ElapsedMs:      endedAt.Sub(startedAt).Milliseconds(),
		StartedAt:      startedAt,
		EndedAt:        endedAt,
		InstanceID:     inst.ID,
		ConversationID: convID,
		MessageID:      msgID,
	}

	wsID, _ := reqctxpkg.GetWorkspaceID(ctx)
	detached := reqctxpkg.SetWorkspaceID(context.Background(), wsID)
	if err := s.repo.SaveCall(detached, call); err != nil {
		s.log.Warn("handlerapp.recordCall: save failed (best-effort)",
			zap.String("handlerId", h.ID), zap.String("method", in.Method), zap.Error(err))
	}
}

// triggerFromCtx derives the execution body: a subagent context means an agent run,
// otherwise a chat turn. (Workflow / manual callers set TriggeredBy explicitly.)
//
// triggerFromCtx 按 ctx 推执行体：有 subagent 即 agent，否则 chat。（workflow / manual 显式设。）
func triggerFromCtx(ctx context.Context) string {
	if _, ok := reqctxpkg.GetSubagentID(ctx); ok {
		return handlerdomain.TriggeredByAgent
	}
	return handlerdomain.TriggeredByChat
}
