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

// pauseRun persists ExecutionContext as PausedState and flips status=paused. Still used by the old
// loop-body path (runReadyLoop → subdag), deleted with the 14→5 collapse.
//
// pauseRun 把 ExecutionContext 持久化为 PausedState 并把 status 翻为 paused（旧 loop body 路径）。
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
	if err := s.repo.UpdateStatus(ctx, run.ID, flowrundomain.StatusPaused, nil, "", "", nil, 0); err != nil {
		return fmt.Errorf("schedulerapp.pauseRun: UpdateStatus: %w", err)
	}
	s.publish(ctx, run.ID, run.WorkflowID, "paused", map[string]any{
		"nodeID": pausedNodeID,
	})
	return nil
}

// ResumeApproval records the user's decision as a durable journal signal (signal_received) and
// re-drives the interpreter, which copy-hits the signal at the parked approval and routes via the
// yes/no port (ADR-016). Crash-safe by construction: the decision is a journaled event, so the
// continuation survives a restart. reason is an optional audit note carried in the event.
//
// ResumeApproval 把用户决策记成 durable journal 信号(signal_received)并重驱解释器;
// 解释器在 parked approval 命中信号、按 yes/no 端口续走。崩溃安全(决策是已记账事件)。
func (s *Service) ResumeApproval(ctx context.Context, runID, nodeID, decision, reason string) error {
	if decision != "approved" && decision != "rejected" {
		return fmt.Errorf("schedulerapp.ResumeApproval: %w", flowrundomain.ErrApprovalDecisionInvalid)
	}
	run, err := s.repo.Get(ctx, runID)
	if err != nil {
		return fmt.Errorf("schedulerapp.ResumeApproval: %w", err)
	}
	if run.Status != flowrundomain.StatusAwaitingSignal {
		return fmt.Errorf("schedulerapp.ResumeApproval: %w (status=%s)", flowrundomain.ErrNotPaused, run.Status)
	}
	iter, ok := s.approvalParkedIter(ctx, runID, nodeID)
	if !ok {
		return fmt.Errorf("schedulerapp.ResumeApproval: %w (no parked approval at node %s)", flowrundomain.ErrApprovalNodeNotFound, nodeID)
	}
	graph, err := s.loadFrozenGraph(ctx, run)
	if err != nil {
		return fmt.Errorf("schedulerapp.ResumeApproval: load graph: %w", err)
	}

	// approved → yes port, rejected → no port (17 §7 ports yes/no; approvals.status approved/rejected).
	port := "no"
	if decision == "approved" {
		port = "yes"
	}
	if _, err := s.journal.AppendEvent(ctx, &flowrundomain.FlowRunEvent{
		FlowrunID: runID, Type: flowrundomain.EventSignalReceived, NodeID: nodeID, IterationKey: iter,
		Result: map[string]any{"decision": port, "reason": reason},
	}); err != nil {
		return fmt.Errorf("schedulerapp.ResumeApproval: journal signal: %w", err)
	}
	if err := s.repo.UpdateStatus(ctx, runID, flowrundomain.StatusRunning, nil, "", "", nil, 0); err != nil {
		return fmt.Errorf("schedulerapp.ResumeApproval: flip running: %w", err)
	}
	s.publish(ctx, runID, run.WorkflowID, "resumed", map[string]any{"nodeID": nodeID, "decision": decision})

	// Detached ctx (same as StartRun) so the continuation survives HTTP caller-cancel.
	runCtx := reqctxpkg.SetUserID(context.Background(), run.UserID)
	runCtx, cancel := context.WithCancel(runCtx)
	s.cancelsMu.Lock()
	s.cancels[runID] = cancel
	s.cancelsMu.Unlock()
	go func() {
		defer s.releaseCancel(runID)
		defer func() {
			if r := recover(); r != nil {
				s.log.Error("ResumeApproval: continuation panic", zap.String("runID", runID), zap.Any("recover", r))
			}
		}()
		s.executeRun(runCtx, run, graph)
	}()
	return nil
}

// approvalParkedIter finds the iteration_key of nodeID's latest signal_awaited not yet answered by
// a signal_received — i.e. the approval currently parked (handles an approval inside a loop).
//
// approvalParkedIter 找 nodeID 最新一条未被 signal_received 应答的 signal_awaited 的 iteration_key。
func (s *Service) approvalParkedIter(ctx context.Context, runID, nodeID string) (int, bool) {
	evs, err := s.journal.LoadJournal(ctx, runID)
	if err != nil {
		return 0, false
	}
	awaited := -1
	answered := map[int]bool{}
	for _, e := range evs {
		if e.NodeID != nodeID {
			continue
		}
		switch e.Type {
		case flowrundomain.EventSignalAwaited:
			if e.IterationKey > awaited {
				awaited = e.IterationKey
			}
		case flowrundomain.EventSignalReceived:
			answered[e.IterationKey] = true
		}
	}
	if awaited >= 0 && !answered[awaited] {
		return awaited, true
	}
	return 0, false
}

// runReadyLoop is the old ready-set evaluator; still used by the loop body (subdag) until the
// 14→5 collapse. Returns (status, errCode, errMsg, paused).
//
// runReadyLoop 是旧 ready-set 循环主体；loop body(subdag)仍复用，14→5 折叠时删。
func (s *Service) runReadyLoop(ctx context.Context, run *flowrundomain.FlowRun, execCtx *ExecutionContext, topo *topoState, ready []string) (status, errCode, errMsg string, paused bool) {
	status = flowrundomain.StatusCompleted

	for len(ready) > 0 {
		select {
		case <-ctx.Done():
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				status = flowrundomain.StatusFailed
				errCode = "RUN_TIMEOUT"
				errMsg = ctx.Err().Error()
			} else {
				status = flowrundomain.StatusCancelled
				errMsg = ctx.Err().Error()
			}
			return
		default:
		}

		nodes := make([]workflowdomain.NodeSpec, 0, len(ready))
		for _, id := range ready {
			nodes = append(nodes, topo.byID[id])
		}
		results := s.dispatchBatch(ctx, nodes, execCtx)

		nextReady := make([]string, 0)
		for _, res := range results {
			if errors.Is(res.Output.Error, ErrApprovalRequired) {
				position := make([]string, 0, len(ready))
				position = append(position, ready...)
				if err := s.pauseRun(ctx, run, execCtx, res.Node.ID, position); err != nil {
					s.log.Error("runReadyLoop: pause failed",
						zap.String("runID", run.ID), zap.Error(err))
					status = flowrundomain.StatusFailed
					errCode = "PAUSE_FAILED"
					errMsg = err.Error()
					return
				}
				paused = true
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
					status = flowrundomain.StatusFailed
					errCode = "NODE_FAILED"
					errMsg = fmt.Sprintf("node %q: %v", res.Node.ID, res.Output.Error)
					return
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
	return
}

// loadFrozenGraph fetches the Version graph the FlowRun is pinned to (V1 fast-path via
// GetActiveVersion; matching run.VersionID exactly when active flips is a later enhancement).
//
// loadFrozenGraph 取 run 锁定的 Version 的冻结图。
func (s *Service) loadFrozenGraph(ctx context.Context, run *flowrundomain.FlowRun) (*workflowdomain.Graph, error) {
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
