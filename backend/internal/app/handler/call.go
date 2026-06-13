package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"go.uber.org/zap"

	entitystreamapp "github.com/sunweilin/forgify/backend/internal/app/entitystream"
	loopapp "github.com/sunweilin/forgify/backend/internal/app/loop"
	handlerdomain "github.com/sunweilin/forgify/backend/internal/domain/handler"
	streamdomain "github.com/sunweilin/forgify/backend/internal/domain/stream"
	handlerinfra "github.com/sunweilin/forgify/backend/internal/infra/handler"
	idgenpkg "github.com/sunweilin/forgify/backend/internal/pkg/idgen"
	logtailpkg "github.com/sunweilin/forgify/backend/internal/pkg/logtail"
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

	// Resolve the method's spec up front: a miss fails with the precise domain error before any
	// spawn/RPC, and the spec's Timeout (ms) deadline-bounds THIS call — without it a wedged
	// method would block the resident instance's serial pipe (one mutexed stdio) indefinitely
	// when the caller carries no deadline of its own.
	//
	// 先解析 method 的 spec：未命中在任何 spawn/RPC 前以精确 domain 错误失败；spec 的 Timeout（ms）
	// 给本次调用加 deadline——没有它，卡死的 method 会在调用方自身无 deadline 时无限期堵住常驻实例的
	// 串行管道（单 mutex stdio）。
	active, err := s.repo.GetVersion(ctx, h.ActiveVersionID)
	if err != nil {
		return nil, fmt.Errorf("handlerapp.Call: %w", err)
	}
	var spec *handlerdomain.MethodSpec
	for i := range active.Methods {
		if active.Methods[i].Name == in.Method {
			spec = &active.Methods[i]
			break
		}
	}
	if spec == nil {
		return nil, fmt.Errorf("handlerapp.Call: %q: %w", in.Method, handlerdomain.ErrMethodNotFound)
	}
	if spec.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(spec.Timeout)*time.Millisecond)
		defer cancel()
	}

	inst, err := s.manager.Get(ctx, h.ID)
	if err != nil {
		return nil, fmt.Errorf("handlerapp.Call: %w", err)
	}

	// Normalize nil args to {} BEFORE the RPC: the driver does method(**args), and a nil map
	// marshals to JSON `null` → method(**None) → TypeError. A no-arg caller (sensor poll,
	// workflow node with no input wiring) must not crash a zero-arg method (same as function).
	// RPC 前把 nil args 归一成 {}：driver 做 method(**args)，nil map 序列化成 JSON `null`
	// → method(**None) → TypeError。无参调用方不该把零参 method 搞崩（同 function）。
	if in.Args == nil {
		in.Args = map[string]any{}
	}

	// Tee the method's yields onto the handler's entities run terminal (entity panel, all callers)
	// and the call's capped logtail (persisted on the call record), in addition to the caller's
	// progress sink (messages, chat). Always StreamCall — doCall is safe for a non-streaming method
	// (no yields → onProgress never fires → plain return).
	//
	// 把 method 的 yield tee 到 handler 的 entities run 终端（实体面板，全 caller）+ 本次调用的限长
	// logtail（随 call 记录落盘）+ 调用方的进度 sink（messages，chat）。一律 StreamCall——doCall 对
	// 非流式 method 安全（无 yield → onProgress 不触发 → 正常返回）。
	runTerm := entitystreamapp.New(ctx, s.entities, streamdomain.Scope{Kind: streamdomain.KindHandler, ID: h.ID}, entitystreamapp.NodeRun, nil)
	logs := logtailpkg.New(logtailpkg.DefaultCap)
	onProgress := func(v any) {
		if in.OnProgress != nil {
			in.OnProgress(v)
		}
		line := yieldBytes(v)
		_, _ = runTerm.Write(line)
		_, _ = logs.Write(line)
	}

	// Attach a per-call sink to the instance's stderr fan for the duration of the call: the
	// handler's print()/logging (stderr is its only reachable channel — the protocol owns stdout)
	// streams to chat progress + the run terminal and persists into the call's logs. Window
	// attribution: concurrent calls on the same instance each receive the window's lines.
	//
	// 调用存续期把 per-call sink 挂上实例 stderr 扇出：handler 的 print()/日志（stderr 是它唯一可达
	// 通道——协议占用 stdout）流到 chat 进度 + run 终端，并落盘进本次调用的 logs。窗口归属：同实例
	// 并发调用各收各窗口的行。
	prog := loopapp.ToolProgress(ctx)
	defer prog.Close()
	detach := inst.Stderr.attach(io.MultiWriter(prog, runTerm, logs))

	startedAt := time.Now().UTC()
	result, err := inst.Client.StreamCall(ctx, in.Method, in.Args, onProgress)
	endedAt := time.Now().UTC()
	// stderr grace before detach: stdout (the return frame) and stderr (the prints) are two
	// independent pipes read by independent goroutines — a print written BEFORE the return
	// can still arrive after it. A short quiesce keeps those lines inside this call's window.
	// detach 前的 stderr 宽限：stdout（return 帧）与 stderr（print）是两条独立管道、各自
	// goroutine 在读——先于 return 写出的 print 仍可能后到。短静默把这些行留在本调用窗口内。
	time.Sleep(stderrGrace)
	detach()
	if err != nil {
		runTerm.Close("error", nil)
	} else {
		runTerm.Close("completed", nil)
	}

	callErr := s.mapCallErr(ctx, err)
	s.recordCall(ctx, h, inst, in, startedAt, endedAt, result, logs.String(), callErr, ctx.Err())
	return result, callErr
}

// stderrGrace bounds how long a call waits for straggler stderr lines before closing its
// log window (pipe-ordering race, see Call).
//
// stderrGrace 限定调用收尾时等迟到 stderr 行多久（管道乱序竞态，见 Call）。
const stderrGrace = 30 * time.Millisecond

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

func (s *Service) recordCall(ctx context.Context, h *handlerdomain.Handler, inst *Instance, in CallInput, startedAt, endedAt time.Time, result any, logs string, callErr, runCtxErr error) {
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

	// Provenance comes off ctx: chat identity (conversation/message/toolCall) from the loop,
	// flowrun identity from the scheduler's dispatch injection — whichever path ran us.
	// 溯源取自 ctx：chat 身份（conversation/message/toolCall）来自 loop，flowrun 身份来自调度器
	// 派发注入——哪条路径跑的就带哪份。
	convID, _ := reqctxpkg.GetConversationID(ctx)
	msgID, _ := reqctxpkg.GetMessageID(ctx)
	toolCallID, _ := reqctxpkg.GetToolCallID(ctx)
	flowrunID, _ := reqctxpkg.GetFlowrunID(ctx)
	flowrunNodeID, _ := reqctxpkg.GetFlowrunNodeID(ctx)

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
		Logs:           logs,
		ElapsedMs:      endedAt.Sub(startedAt).Milliseconds(),
		StartedAt:      startedAt,
		EndedAt:        endedAt,
		InstanceID:     inst.ID,
		ConversationID: convID,
		MessageID:      msgID,
		ToolCallID:     toolCallID,
		FlowrunID:      flowrunID,
		FlowrunNodeID:  flowrunNodeID,
	}

	wsID, _ := reqctxpkg.GetWorkspaceID(ctx)
	detached := reqctxpkg.Detached(wsID)
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
