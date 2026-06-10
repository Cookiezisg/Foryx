package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	entitystreamapp "github.com/sunweilin/forgify/backend/internal/app/entitystream"
	loopapp "github.com/sunweilin/forgify/backend/internal/app/loop"
	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
	agentdomain "github.com/sunweilin/forgify/backend/internal/domain/agent"
	messagesdomain "github.com/sunweilin/forgify/backend/internal/domain/messages"
	streamdomain "github.com/sunweilin/forgify/backend/internal/domain/stream"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

// ResumeExecution resolves one pending human interaction in a parked agent run and re-drives it
// (R0064): it fills the leaf tool_result per the action, and — once the parked transcript has no
// other pending interaction — replays the transcript through a fresh loop.Run to continue the
// ReAct (already-completed tool_results are memoized in the transcript, never re-executed). The
// run advances to its next state: completed / failed, or parked again at a later interaction. The
// returned InvokeResult mirrors InvokeAgent's so a caller (the chat cascade in D3b, or a standalone
// invoker) can react. Idempotent on the leaf: a second resolve of an already-filled leaf is a no-op
// (the pending status is gone), so a double POST can't double-run the approved tool.
//
// ResumeExecution 决议一个 parked agent 运行里的一条待决人机交互并重驱（R0064）：按动作填 leaf tool_result，待
// parked transcript 无其它待决交互时，把 transcript 经一次全新 loop.Run 重放以继续 ReAct（已完成的 tool_result
// 在 transcript 里记忆化、绝不重执行）。运行推进到下一态：completed / failed，或在更后的交互再次 parked。返回的
// InvokeResult 对标 InvokeAgent，使调用方（D3b 的 chat 级联，或独立调用方）可反应。对 leaf 幂等：二次决议已填的
// leaf 为 no-op（pending 状态已没），故重复 POST 不会双跑批准的工具。
func (s *Service) ResumeExecution(ctx context.Context, executionID, leafToolCallID, action, answer string) (*InvokeResult, error) {
	exec, err := s.repo.GetExecutionByID(ctx, executionID)
	if err != nil {
		return nil, fmt.Errorf("agentapp.ResumeExecution: %w", err)
	}
	if exec.Status != agentdomain.ExecutionStatusParked {
		return nil, agentdomain.ErrExecutionNotParked
	}

	var blocks []messagesdomain.Block
	if err := json.Unmarshal(exec.Transcript, &blocks); err != nil {
		return nil, fmt.Errorf("agentapp.ResumeExecution: decode transcript: %w", err)
	}
	tcBlock, pending := findInteractionInBlocks(blocks, leafToolCallID)
	if tcBlock == nil || pending == nil {
		return nil, agentdomain.ErrInteractionNotFound
	}
	a, err := s.repo.Get(ctx, exec.AgentID)
	if err != nil {
		return nil, fmt.Errorf("agentapp.ResumeExecution: agent: %w", err)
	}
	v, err := s.repo.GetVersion(ctx, exec.VersionID)
	if err != nil {
		return nil, fmt.Errorf("agentapp.ResumeExecution: version: %w", err)
	}

	startedAt := time.Now().UTC()

	// Cancel abandons the run: fill every pending leaf as cancelled, mark the execution failed, no
	// continuation (mirrors chat's cancel).
	//
	// Cancel 放弃运行：把每条 pending leaf 填为 cancelled、execution 标 failed、不续跑（镜像 chat cancel）。
	if action == loopapp.ResolveCancel {
		for i := range blocks {
			if b := &blocks[i]; b.Type == messagesdomain.BlockTypeToolResult && b.Status == messagesdomain.StatusPending {
				b.Content = "The user cancelled this run."
				b.Status = messagesdomain.StatusCancelled
			}
		}
		exec.Status = agentdomain.ExecutionStatusCancelled
		exec.Transcript = mustMarshalBlocks(blocks)
		exec.ErrorMessage = "cancelled by user"
		exec.EndedAt = startedAt
		_ = s.repo.UpdateExecution(detachedWS(ctx), exec)
		return &InvokeResult{ExecutionID: exec.ID, Status: agentdomain.ExecutionStatusCancelled, ErrorMsg: "cancelled by user"}, nil
	}

	kind, _ := pending.Attrs["park"].(string)
	content, status, errMsg, err := s.resolveLeaf(ctx, a, v, kind, action, answer, tcBlock)
	if err != nil {
		return nil, err
	}
	pending.Content = content
	pending.Status = status
	pending.Error = errMsg

	// Other pending leaves in this same step? Persist the partial fill, stay parked.
	//
	// 同一步还有其它 pending leaf？落部分填充、保持 parked。
	if hasOtherPendingInBlocks(blocks, pending.ID) {
		exec.Transcript = mustMarshalBlocks(blocks)
		if err := s.repo.UpdateExecution(detachedWS(ctx), exec); err != nil {
			return nil, err
		}
		return &InvokeResult{ExecutionID: exec.ID, Status: agentdomain.ExecutionStatusParked, Parked: true}, nil
	}

	// All leaves resolved → replay the filled transcript through a fresh run to continue.
	//
	// 全 leaf 决议 → 把填好的 transcript 经一次全新运行重放以继续。
	result, modelID, runErr := s.runLoop(ctx, a, v, InvokeInput{
		AgentID:     exec.AgentID,
		VersionID:   exec.VersionID,
		Input:       exec.Input,
		TriggeredBy: exec.TriggeredBy,
		ReplaySteps: []RecordedStep{{Assistant: blocks}},
	})
	endedAt := time.Now().UTC()

	res := &InvokeResult{ExecutionID: exec.ID, ElapsedMs: exec.ElapsedMs + endedAt.Sub(startedAt).Milliseconds()}
	applyLoopResult(res, result, runErr)
	_ = modelID

	// The persisted transcript is the filled prior blocks + the continuation's new blocks.
	//
	// 落库 transcript = 填好的先前 blocks + 续跑新 blocks。
	full := append(blocks, result.Blocks...)
	exec.Status = res.Status
	exec.Output = res.Output
	exec.Transcript = mustMarshalBlocks(full)
	exec.ErrorMessage = res.ErrorMsg
	exec.EndedAt = endedAt
	if err := s.repo.UpdateExecution(detachedWS(ctx), exec); err != nil {
		return nil, err
	}
	return res, nil
}

// resolveLeaf computes the (content, status, error) for one leaf interaction's tool_result. danger:
// approve runs the gated tool now (execute-then-record), deny feeds the denial back. ask: accept =
// the answer, decline = a re-route signal.
//
// resolveLeaf 算一条 leaf 交互 tool_result 的 (content, status, error)。danger：approve 此刻跑被门工具
// （execute-then-record），deny 把拒绝反馈回模型。ask：accept = 答案，decline = 改道信号。
func (s *Service) resolveLeaf(ctx context.Context, a *agentdomain.Agent, v *agentdomain.Version, kind, action, answer string, tcBlock *messagesdomain.Block) (content, status, errMsg string, err error) {
	switch kind {
	case loopapp.ParkKindDanger:
		switch action {
		case loopapp.ResolveApprove:
			out, exErr := s.executeApprovedTool(ctx, a, v, tcBlock)
			if exErr != "" {
				return out, messagesdomain.StatusError, exErr, nil
			}
			return out, messagesdomain.StatusCompleted, "", nil
		case loopapp.ResolveDeny:
			return "The user denied running this tool. Do not retry it unless the user explicitly asks.", messagesdomain.StatusCompleted, "", nil
		}
	case loopapp.ParkKindAsk:
		switch action {
		case loopapp.ResolveAccept:
			ans := strings.TrimSpace(answer)
			if ans == "" {
				ans = "(the user submitted an empty answer)"
			}
			return ans, messagesdomain.StatusCompleted, "", nil
		case loopapp.ResolveDecline:
			return "The user declined to answer this question. Proceed without it or ask differently.", messagesdomain.StatusCompleted, "", nil
		}
	}
	return "", "", "", agentdomain.ErrBadResolveAction
}

// executeApprovedTool runs the gated tool at resolve time, scoped to the agent's entities run
// terminal + nested under the leaf tool_call. The stripped args ride the persisted tool_call
// block's Content. Returns (output, errMsg); errMsg != "" → the call failed.
//
// executeApprovedTool 在 resolve 时跑被门工具，锚到 agent 的 entities run 终端 + 嵌在 leaf tool_call 下。
// 剥离后的 args 随持久化的 tool_call 块 Content。返 (output, errMsg)；errMsg != "" → 调用失败。
func (s *Service) executeApprovedTool(ctx context.Context, a *agentdomain.Agent, v *agentdomain.Version, tcBlock *messagesdomain.Block) (string, string) {
	name, _ := tcBlock.Attrs["tool"].(string)
	var allTools = []toolapp.Tool(nil)
	if s.invoke.Tools != nil {
		allTools = s.invoke.Tools()
	}
	whitelist := make([]string, 0, len(v.Tools))
	for _, t := range v.Tools {
		whitelist = append(whitelist, t.Ref)
	}
	var tool toolapp.Tool
	for _, t := range filterToolsByWhitelist(allTools, whitelist) {
		if t.Name() == name {
			tool = t
			break
		}
	}
	if tool == nil {
		return "", "approved tool " + name + " is not in this agent's toolset"
	}
	wsID, _ := reqctxpkg.GetWorkspaceID(ctx)
	ectx := reqctxpkg.SetWorkspaceID(context.Background(), wsID)
	ectx = reqctxpkg.SetToolCallID(ectx, tcBlock.ID)
	ectx = entitystreamapp.WithBridge(ectx, s.invoke.EntitiesBridge)
	ectx = entitystreamapp.WithRunScope(ectx, streamdomain.Scope{Kind: streamdomain.KindAgent, ID: a.ID})
	out, err := tool.Execute(ectx, tcBlock.Content)
	if err != nil {
		if out != "" {
			return out, err.Error()
		}
		return err.Error(), err.Error()
	}
	return out, ""
}

// applyLoopResult maps a loop.Result onto an InvokeResult (shared by InvokeAgent + ResumeExecution):
// runErr → failed; parked → carry the pending interactions; else ok/failed by loop status.
//
// applyLoopResult 把 loop.Result 映射到 InvokeResult（InvokeAgent + ResumeExecution 共用）：runErr → failed；
// parked → 携待决交互；否则按 loop status 判 ok/failed。
func applyLoopResult(res *InvokeResult, result loopapp.Result, runErr error) {
	switch {
	case runErr != nil:
		res.Status = agentdomain.ExecutionStatusFailed
		res.ErrorMsg = runErr.Error()
	case result.Status == messagesdomain.StatusParked:
		res.Status = agentdomain.ExecutionStatusParked
		res.Parked = true
		res.ParkRequests = result.Parks
		res.StopReason = result.StopReason
		res.Steps = result.Steps
		res.TokensIn = result.TokensIn
		res.TokensOut = result.TokensOut
	default:
		res.OK = result.Status != messagesdomain.StatusError
		if !res.OK {
			res.Status = agentdomain.ExecutionStatusFailed
			res.ErrorMsg = "agent loop error"
		} else {
			res.Status = agentdomain.ExecutionStatusOK
		}
		res.Output = result.LastMessage
		res.StopReason = result.StopReason
		res.Steps = result.Steps
		res.TokensIn = result.TokensIn
		res.TokensOut = result.TokensOut
	}
}

// findInteractionInBlocks locates a tool_call (id == toolCallID) and its pending tool_result child.
//
// findInteractionInBlocks 定位 tool_call（id == toolCallID）及其 pending tool_result 子块。
func findInteractionInBlocks(blocks []messagesdomain.Block, toolCallID string) (tcBlock, pending *messagesdomain.Block) {
	for i := range blocks {
		b := &blocks[i]
		if b.ID == toolCallID && b.Type == messagesdomain.BlockTypeToolCall {
			tcBlock = b
		}
		if b.ParentBlockID == toolCallID && b.Type == messagesdomain.BlockTypeToolResult && b.Status == messagesdomain.StatusPending {
			pending = b
		}
	}
	return
}

// hasOtherPendingInBlocks reports a pending tool_result other than exceptID.
//
// hasOtherPendingInBlocks 报告除 exceptID 外的 pending tool_result。
func hasOtherPendingInBlocks(blocks []messagesdomain.Block, exceptID string) bool {
	for i := range blocks {
		b := &blocks[i]
		if b.Type == messagesdomain.BlockTypeToolResult && b.Status == messagesdomain.StatusPending && b.ID != exceptID {
			return true
		}
	}
	return false
}

func mustMarshalBlocks(blocks []messagesdomain.Block) json.RawMessage {
	b, err := json.Marshal(blocks)
	if err != nil || len(b) == 0 {
		return json.RawMessage("[]")
	}
	return b
}

func detachedWS(ctx context.Context) context.Context {
	wsID, _ := reqctxpkg.GetWorkspaceID(ctx)
	return reqctxpkg.SetWorkspaceID(context.Background(), wsID)
}
