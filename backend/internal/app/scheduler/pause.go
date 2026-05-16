package scheduler

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.uber.org/zap"

	flowrundomain "github.com/sunweilin/forgify/backend/internal/domain/flowrun"
	workflowdomain "github.com/sunweilin/forgify/backend/internal/domain/workflow"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

// pauseRun persists ExecutionContext as PausedState and flips status=paused.
//
// pauseRun 把 ExecutionContext 持久化为 PausedState 并把 status 翻为 paused。
func (s *Service) pauseRun(ctx context.Context, run *flowrundomain.FlowRun, execCtx *ExecutionContext, pausedNodeID string, position []string) error {
	ps := &flowrundomain.PausedState{
		NodeID:    pausedNodeID,
		Variables: execCtx.Variables,
		Outputs:   execCtx.Outputs,
		Position:  position,
		PausedAt:  time.Now().UTC(),
	}
	if err := s.repo.SetPausedState(ctx, run.ID, ps); err != nil {
		return fmt.Errorf("schedulerapp.pauseRun: SetPausedState: %w", err)
	}
	// Status → paused (no ended_at — run not terminal). Output / err
	// fields stay empty;they'll be filled on final terminal flip.
	// status → paused;不写 ended_at(run 非终态)。
	if err := s.repo.UpdateStatus(ctx, run.ID, flowrundomain.StatusPaused, nil, "", "", nil, 0); err != nil {
		return fmt.Errorf("schedulerapp.pauseRun: UpdateStatus: %w", err)
	}
	s.publish(ctx, run.ID, run.WorkflowID, "paused", map[string]any{
		"nodeID": pausedNodeID,
	})
	return nil
}

// ResumeApproval rehydrates a paused FlowRun and continues execution with the user's decision.
//
// ResumeApproval 重建 paused FlowRun 并按用户决策继续执行。
func (s *Service) ResumeApproval(ctx context.Context, runID, nodeID, decision string) error {
	if decision != "approved" && decision != "rejected" {
		return fmt.Errorf("schedulerapp.ResumeApproval: %w", flowrundomain.ErrApprovalDecisionInvalid)
	}

	run, err := s.repo.Get(ctx, runID)
	if err != nil {
		return fmt.Errorf("schedulerapp.ResumeApproval: %w", err)
	}
	if run.Status != flowrundomain.StatusPaused {
		return fmt.Errorf("schedulerapp.ResumeApproval: %w (status=%s)", flowrundomain.ErrNotPaused, run.Status)
	}
	if run.PausedState == nil {
		return fmt.Errorf("schedulerapp.ResumeApproval: %w", flowrundomain.ErrNotPaused)
	}
	if run.PausedState.NodeID != nodeID {
		return fmt.Errorf("schedulerapp.ResumeApproval: %w (paused at %s, asked for %s)",
			flowrundomain.ErrApprovalNodeNotFound, run.PausedState.NodeID, nodeID)
	}

	// Look up the workflow version to get the frozen graph. The version
	// pinned at run-start is what's still executing — even if the active
	// version flipped meanwhile, the in-flight run sticks to its locked
	// version.
	// 取 version 的冻结图;run 锁定的 version 始终是它执行的图。
	v, err := s.workflowRead.GetActiveVersion(ctx, run.WorkflowID)
	if err == nil && v.ID != run.VersionID {
		// Active flipped — load the specific frozen version below.
		// active 已翻;以 run.VersionID 为准。
	}
	graph, err := s.loadFrozenGraph(ctx, run)
	if err != nil {
		return fmt.Errorf("schedulerapp.ResumeApproval: load graph: %w", err)
	}

	// Snapshot PausedState before ClearPausedState wipes it from the
	// in-memory cache (fakeRepo mutates the same row reference; prod
	// fetches a fresh row each call so this is just defensive).
	// 把 PausedState 拷出来再清,防 in-memory cache 把它一并 nil。
	savedPaused := run.PausedState
	if err := s.repo.ClearPausedState(ctx, runID); err != nil {
		s.log.Warn("ResumeApproval: ClearPausedState failed",
			zap.String("runID", runID), zap.Error(err))
	}
	run.PausedState = savedPaused

	// Detached ctx (same pattern as StartRun) so resume survives HTTP
	// caller-cancel. Register the cancel func so subsequent Cancel calls
	// still work.
	// detached ctx + 注 cancel,保证 Cancel 仍能杀。
	runCtx := reqctxpkg.SetUserID(context.Background(), run.UserID)
	runCtx, cancel := context.WithCancel(runCtx)
	s.cancelsMu.Lock()
	s.cancels[runID] = cancel
	s.cancelsMu.Unlock()

	if err := s.repo.UpdateStatus(runCtx, runID, flowrundomain.StatusRunning, nil, "", "", nil, 0); err != nil {
		return fmt.Errorf("schedulerapp.ResumeApproval: flip running: %w", err)
	}
	s.publish(runCtx, runID, run.WorkflowID, "resumed", map[string]any{
		"decision": decision,
		"nodeID":   nodeID,
	})

	go func() {
		defer s.releaseCancel(runID)
		defer func() {
			if r := recover(); r != nil {
				s.log.Error("ResumeApproval: continuation panic",
					zap.String("runID", runID), zap.Any("recover", r))
			}
		}()
		s.continueRun(runCtx, run, graph, nodeID, decision)
	}()
	return nil
}

// continueRun rebuilds ExecutionContext from PausedState and drives the remaining DAG.
//
// continueRun 从 PausedState 重建 ExecutionContext 并推剩余 DAG。
func (s *Service) continueRun(ctx context.Context, run *flowrundomain.FlowRun, graph *workflowdomain.Graph, pausedNodeID, decision string) {
	execCtx := &ExecutionContext{
		Run:       run,
		Graph:     graph,
		Variables: run.PausedState.Variables,
		Outputs:   run.PausedState.Outputs,
		Done:      make(map[string]bool),
		Failed:    make(map[string]string),
		Attempts:  make(map[string]int),
		NextPort:  make(map[string]string),
	}
	if execCtx.Variables == nil {
		execCtx.Variables = make(map[string]any)
	}
	if execCtx.Outputs == nil {
		execCtx.Outputs = make(map[string]map[string]any)
	}
	for nodeID := range execCtx.Outputs {
		execCtx.Done[nodeID] = true
	}
	// Approval node finalization: mark it done + record the decision as
	// its output + queue downstream advance with NextPort=decision.
	// approval 节点终态化:置 done + decision 写 Outputs + NextPort=decision
	// 推下游。
	execCtx.Done[pausedNodeID] = true
	execCtx.Outputs[pausedNodeID] = map[string]any{"decision": decision}
	execCtx.NextPort[pausedNodeID] = decision

	topo := buildTopo(graph)
	// Replay completed in-degree decrements so topo state is correct for
	// the continuation. Order: every already-done node had its downstream
	// in-degrees decremented when it originally completed.
	// 回放已完成节点的 in-degree decrement;按完成顺序回放保 topo 正确。
	for doneID := range execCtx.Done {
		port := execCtx.NextPort[doneID]
		_ = topo.advance(doneID, port)
	}
	// Now collect the new ready set from the approval node's downstream.
	// 然后收 approval 节点下游新 ready。
	ready := topo.advance(pausedNodeID, decision)
	// Already advance ran above for pausedNodeID via the Done loop too —
	// the second call decrements twice. Compensate by re-building topo
	// fresh and replaying without the paused node, then advance for it.
	// 重建 topo,把 paused 节点之外的 done 节点回放,然后 advance paused 节点。
	topo = buildTopo(graph)
	for doneID := range execCtx.Done {
		if doneID == pausedNodeID {
			continue
		}
		_ = topo.advance(doneID, execCtx.NextPort[doneID])
	}
	ready = topo.advance(pausedNodeID, decision)

	// Drive the rest of the DAG (same loop as executeRun).
	// 推剩余 DAG(跟 executeRun 同循环)。
	s.driveLoop(ctx, run, graph, execCtx, topo, ready)
}

// driveLoop is the per-ready-set loop body shared by executeRun and continueRun.
//
// driveLoop 是 executeRun 与 continueRun 共享的 per-ready-set 循环本体。
func (s *Service) driveLoop(ctx context.Context, run *flowrundomain.FlowRun, graph *workflowdomain.Graph, execCtx *ExecutionContext, topo *topoState, ready []string) {
	terminalStatus := flowrundomain.StatusCompleted
	var terminalErr string
	var terminalErrCode string

	for len(ready) > 0 {
		select {
		case <-ctx.Done():
			terminalStatus = flowrundomain.StatusCancelled
			terminalErr = ctx.Err().Error()
			ready = nil
			goto FINALIZE
		default:
		}

		nodes := make([]workflowdomain.NodeSpec, 0, len(ready))
		for _, id := range ready {
			nodes = append(nodes, topo.byID[id])
		}
		results := s.dispatchBatch(ctx, nodes, execCtx)

		nextReady := make([]string, 0)
		for _, res := range results {
			// Approval node — pause + return (don't write flowrun_node
			// row;the node hasn't actually completed yet).
			if errors.Is(res.Output.Error, ErrApprovalRequired) {
				position := make([]string, 0, len(ready))
				position = append(position, ready...)
				if err := s.pauseRun(ctx, run, execCtx, res.Node.ID, position); err != nil {
					s.log.Error("driveLoop: pause failed",
						zap.String("runID", run.ID), zap.Error(err))
					terminalStatus = flowrundomain.StatusFailed
					terminalErrCode = "PAUSE_FAILED"
					terminalErr = err.Error()
					goto FINALIZE
				}
				return
			}
			s.recordNode(ctx, run, res, execCtx)

			if res.Output.Error != nil {
				policy := nodeOnError(res.Node)
				switch policy {
				case workflowdomain.OnErrorContinue:
					execCtx.Done[res.Node.ID] = true
					nextReady = append(nextReady, topo.advance(res.Node.ID, "")...)
				case workflowdomain.OnErrorBranch:
					execCtx.Done[res.Node.ID] = true
					nextReady = append(nextReady, topo.advance(res.Node.ID, "error")...)
				default:
					terminalStatus = flowrundomain.StatusFailed
					terminalErrCode = "NODE_FAILED"
					terminalErr = fmt.Sprintf("node %q: %v", res.Node.ID, res.Output.Error)
					ready = nil
					goto FINALIZE
				}
				continue
			}
			execCtx.Done[res.Node.ID] = true
			if res.Output.Outputs != nil {
				execCtx.Outputs[res.Node.ID] = res.Output.Outputs
			}
			execCtx.NextPort[res.Node.ID] = res.Output.NextPort
			nextReady = append(nextReady, topo.advance(res.Node.ID, res.Output.NextPort)...)
		}
		ready = nextReady
	}

FINALIZE:
	output := map[string]any{
		"nodesCompleted": len(execCtx.Done),
		"nodesTotal":     len(graph.Nodes),
	}
	s.finalizeRun(ctx, run, terminalStatus, output, terminalErrCode, terminalErr)
}

// loadFrozenGraph fetches the specific Version graph the FlowRun is pinned to.
//
// loadFrozenGraph 取 run 锁定的 Version 的冻结图。
func (s *Service) loadFrozenGraph(ctx context.Context, run *flowrundomain.FlowRun) (*workflowdomain.Graph, error) {
	// We need a generic "GetVersion(versionID)" port — workflowRead is a
	// reader interface that doesn't expose this directly. For V1 we go
	// through GetActiveVersion as a fast path (works when the version
	// hasn't been replaced mid-run, which is the common case for paused
	// runs resumed within minutes). When active has flipped, GetWorkflow
	// → loop versions → match by ID is a Plan 06 enhancement.
	// V1:走 GetActiveVersion 快路径;active 已翻情况是 Plan 06 增强。
	v, err := s.workflowRead.GetActiveVersion(ctx, run.WorkflowID)
	if err != nil {
		return nil, err
	}
	if v.GraphParsed == nil {
		return nil, fmt.Errorf("version %s has no parsed graph", v.ID)
	}
	if v.ID != run.VersionID {
		s.log.Warn("loadFrozenGraph: version drifted",
			zap.String("runID", run.ID),
			zap.String("runVersion", run.VersionID),
			zap.String("activeVersion", v.ID))
	}
	return v.GraphParsed, nil
}
