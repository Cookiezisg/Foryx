package scheduler

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"

	workflowapp "github.com/sunweilin/forgify/backend/internal/app/workflow"
	flowrundomain "github.com/sunweilin/forgify/backend/internal/domain/flowrun"
	workflowdomain "github.com/sunweilin/forgify/backend/internal/domain/workflow"
	idgenpkg "github.com/sunweilin/forgify/backend/internal/pkg/idgen"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

// ExecutionContext is the per-run mutable state, persisted as PausedState JSON when paused.
//
// ExecutionContext 是 per-run 可变状态，暂停时序列化为 PausedState JSON。
type ExecutionContext struct {
	Run       *flowrundomain.FlowRun
	Graph     *workflowdomain.Graph
	Variables map[string]any
	Outputs   map[string]map[string]any
	Done      map[string]bool
	Failed    map[string]string
	Attempts  map[string]int
	NextPort  map[string]string

	// §5.1 Loop body — set on iteration-scoped sub-execContexts; nil on top-level.
	// §5.1 Loop body — 迭代级 sub-execContext 设置；顶层为 nil/空。
	Loop             *workflowapp.LoopContext
	ParentLoopNodeID string
	IterationIndex   int

	// DryRun — true mirrors FlowRun.DryRun; side-effect dispatchers return mock outputs.
	// DryRun — 跟 FlowRun.DryRun 同步；有副作用的 dispatcher 返 mock outputs。
	DryRun bool
}

func newExecutionContext(run *flowrundomain.FlowRun, graph *workflowdomain.Graph) *ExecutionContext {
	vars := make(map[string]any, len(graph.Variables)+1)
	for _, v := range graph.Variables {
		if v.Default != nil {
			vars[v.Name] = v.Default
		}
	}
	vars["trigger"] = run.TriggerInput
	return &ExecutionContext{
		Run:       run,
		Graph:     graph,
		Variables: vars,
		Outputs:   make(map[string]map[string]any),
		Done:      make(map[string]bool),
		Failed:    make(map[string]string),
		Attempts:  make(map[string]int),
		NextPort:  make(map[string]string),
		DryRun:    run.DryRun,
	}
}

type topoState struct {
	inDegree   map[string]int
	downstream map[string][]workflowdomain.EdgeSpec
	byID       map[string]workflowdomain.NodeSpec
}

func buildTopo(graph *workflowdomain.Graph) *topoState {
	t := &topoState{
		inDegree:   make(map[string]int),
		downstream: make(map[string][]workflowdomain.EdgeSpec),
		byID:       make(map[string]workflowdomain.NodeSpec),
	}
	for _, n := range graph.Nodes {
		t.byID[n.ID] = n
		if _, ok := t.inDegree[n.ID]; !ok {
			t.inDegree[n.ID] = 0
		}
	}
	for _, e := range graph.Edges {
		t.inDegree[e.To]++
		t.downstream[e.From] = append(t.downstream[e.From], e)
	}
	return t
}

func (t *topoState) initialReady() []string {
	out := make([]string, 0)
	for id, deg := range t.inDegree {
		if deg == 0 {
			out = append(out, id)
		}
	}
	return out
}

// advance decrements downstream in-degrees and returns the new ready set; nextPort filters branching edges.
//
// advance 减下游 in-degree 并返新 ready；nextPort 过滤分叉边。
func (t *topoState) advance(done string, nextPort string) []string {
	ready := make([]string, 0)
	doneNode := t.byID[done]
	branching := workflowdomain.IsBranchingNode(doneNode.Type)
	for _, e := range t.downstream[done] {
		t.inDegree[e.To]--
		if branching && e.FromPort != nextPort {
			continue
		}
		if t.inDegree[e.To] == 0 {
			ready = append(ready, e.To)
		}
	}
	return ready
}

// executeRun is Service.ExecuteFn: drive the durable interpreter over the pinned graph, then
// write the single terminal status (ADR-016, M2). The old topo-walk (buildTopo/driveLoop) is
// retained for the approval-resume path (continueRun) + loop body (subdag) until M3/M4 collapse
// those nodes onto the interpreter; StartRun's execution now goes through the journal.
//
// executeRun 是 Service.ExecuteFn:在 pinned 图上跑 durable interpreter,写一次终态。
// 旧 topo-walk 暂留给 approval-resume(continueRun)+ loop body(subdag),M3/M4 折叠后删。
func (s *Service) executeRun(ctx context.Context, run *flowrundomain.FlowRun, graph *workflowdomain.Graph) {
	if graph == nil || len(graph.Nodes) == 0 {
		s.finalizeRun(ctx, run, flowrundomain.StatusCompleted, map[string]any{"empty": true}, "", "")
		return
	}
	parked, err := New(s.journal, s.router).Run(ctx, run.ID, *graph, run.TriggerInput)
	if err != nil {
		// Cancel/timeout is a distinct terminal from a node failure (concurrency-error-edges-2).
		// Finalize on a fresh detached ctx — the run ctx is already cancelled, so its DB write fails.
		if ctxErr := ctx.Err(); ctxErr != nil {
			fctx := reqctxpkg.SetUserID(context.Background(), run.UserID)
			if errors.Is(ctxErr, context.DeadlineExceeded) {
				s.finalizeRun(fctx, run, flowrundomain.StatusFailed, nil, "RUN_TIMEOUT", ctxErr.Error())
			} else {
				s.finalizeRun(fctx, run, flowrundomain.StatusCancelled, nil, "CANCELLED", ctxErr.Error())
			}
			return
		}
		s.finalizeRun(ctx, run, flowrundomain.StatusFailed, nil, "NODE_FAILED", err.Error())
		return
	}
	if parked {
		// approval suspended the run; status → awaiting_signal (not terminal). ResumeApproval
		// journals the decision and re-drives the interpreter.
		now := time.Now().UTC()
		elapsed := now.Sub(run.StartedAt).Milliseconds()
		if uErr := s.repo.UpdateStatus(ctx, run.ID, flowrundomain.StatusAwaitingSignal, nil, "", "", nil, elapsed); uErr != nil {
			s.log.Error("scheduler.executeRun: park UpdateStatus failed", zap.String("runID", run.ID), zap.Error(uErr))
		}
		return
	}
	s.finalizeRun(ctx, run, flowrundomain.StatusCompleted, nil, "", "")
}

type dispatchResult struct {
	Node      workflowdomain.NodeSpec
	Input     map[string]any
	Output    DispatchOutput
	StartedAt time.Time
	EndedAt   time.Time
}

// dispatchBatch runs nodes in parallel via goroutines.
//
// dispatchBatch 并发 dispatch 一批 ready 节点。
func (s *Service) dispatchBatch(ctx context.Context, nodes []workflowdomain.NodeSpec, execCtx *ExecutionContext) []dispatchResult {
	results := make([]dispatchResult, len(nodes))
	var wg sync.WaitGroup
	for i, n := range nodes {
		wg.Add(1)
		go func(idx int, node workflowdomain.NodeSpec) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					results[idx].Output = DispatchOutput{
						Error: fmt.Errorf("dispatcher panic: %v", r),
					}
					results[idx].EndedAt = time.Now().UTC()
					s.log.Error("dispatcher panic",
						zap.String("nodeID", node.ID),
						zap.String("nodeType", node.Type),
						zap.Any("recover", r))
				}
			}()
			input := buildNodeInput(node, execCtx)
			start := time.Now().UTC()
			out := s.dispatchWithPolicies(ctx, node, input, execCtx)
			results[idx] = dispatchResult{
				Node:      node,
				Input:     input,
				Output:    out,
				StartedAt: start,
				EndedAt:   time.Now().UTC(),
			}
		}(i, n)
	}
	wg.Wait()
	return results
}

func buildNodeInput(_ workflowdomain.NodeSpec, _ *ExecutionContext) map[string]any {
	return map[string]any{}
}

func (s *Service) recordNode(ctx context.Context, run *flowrundomain.FlowRun, res dispatchResult, execCtx *ExecutionContext) {
	status := flowrundomain.NodeStatusOK
	if res.Output.Error != nil {
		status = flowrundomain.NodeStatusFailed
	}
	if ctx.Err() != nil && res.Output.Error == nil {
		status = flowrundomain.NodeStatusCancelled
	}

	row := &flowrundomain.Node{
		ID:             idgenpkg.New("frn"),
		UserID:         run.UserID,
		Status:         status,
		TriggeredBy:    flowrundomain.TriggerKindCron,
		Input:          res.Input,
		Output:         res.Output.Outputs,
		StartedAt:      res.StartedAt,
		EndedAt:        res.EndedAt,
		ElapsedMs:      res.EndedAt.Sub(res.StartedAt).Milliseconds(),
		FlowrunID:      run.ID,
		NodeID:         res.Node.ID,
		NodeType:       res.Node.Type,
		Attempts:       maxInt(1, execCtx.Attempts[res.Node.ID]),
		ParentLoopNode: execCtx.ParentLoopNodeID,
		IterationIndex: execCtx.IterationIndex,
	}
	row.TriggeredBy = "workflow"
	if res.Output.Error != nil {
		row.ErrorMessage = res.Output.Error.Error()
		row.ErrorCode = "NODE_FAILED"
	}
	if err := s.repo.CreateNode(ctx, row); err != nil {
		s.log.Warn("scheduler.recordNode: create failed",
			zap.String("runID", run.ID),
			zap.String("nodeID", res.Node.ID),
			zap.Error(err))
	}
}

// finalizeRun writes terminal status, prunes per retention, and publishes a notification.
//
// finalizeRun 写终态、按保留策略剪除、并推通知。
func (s *Service) finalizeRun(ctx context.Context, run *flowrundomain.FlowRun, status string, output any, errCode, errMsg string) {
	endedAt := time.Now().UTC()
	elapsedMs := endedAt.Sub(run.StartedAt).Milliseconds()
	if err := s.repo.UpdateStatus(ctx, run.ID, status, output, errCode, errMsg, &endedAt, elapsedMs); err != nil {
		s.log.Error("scheduler.finalizeRun: UpdateStatus failed",
			zap.String("runID", run.ID), zap.Error(err))
	}
	if err := s.repo.HardDeleteOldest(ctx, run.WorkflowID, flowrundomain.DefaultRetentionLimit); err != nil {
		s.log.Warn("scheduler.finalizeRun: HardDeleteOldest failed",
			zap.String("workflowID", run.WorkflowID), zap.Error(err))
	}
	s.publish(ctx, run.ID, run.WorkflowID, status, map[string]any{
		"elapsedMs": elapsedMs,
	})
}

func nodeOnError(n workflowdomain.NodeSpec) string {
	if n.OnError == "" {
		return workflowdomain.OnErrorStop
	}
	return n.OnError
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
