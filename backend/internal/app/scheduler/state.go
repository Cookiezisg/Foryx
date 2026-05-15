// state.go — ExecutionContext + topo state + per-run loop. Owns the
// in-flight mutable state for one FlowRun and drives the DAG: pick
// ready nodes → dispatch in parallel → advance → repeat until done,
// cancelled, or failed (per onError stop policy).
//
// state.go —— ExecutionContext + topo 状态 + per-run 主循环。管一个
// FlowRun 的 in-flight 可变状态;DAG 推进:pick ready → 并发 dispatch →
// advance → 直到完成 / 取消 / failed-stop。

package scheduler

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"

	flowrundomain "github.com/sunweilin/forgify/backend/internal/domain/flowrun"
	workflowdomain "github.com/sunweilin/forgify/backend/internal/domain/workflow"
	idgenpkg "github.com/sunweilin/forgify/backend/internal/pkg/idgen"
)

// ExecutionContext is the per-run mutable state. Persisted as PausedState
// JSON when an approval/wait node pauses (E10);rehydrated at boot.
//
// ExecutionContext 是 per-run 可变状态;approval/wait 暂停时整体序列化
// 到 PausedState JSON;boot 时 rehydrate。
type ExecutionContext struct {
	Run       *flowrundomain.FlowRun
	Graph     *workflowdomain.Graph
	Variables map[string]any                    // workflow-level vars
	Outputs   map[string]map[string]any         // nodeID → output by port
	Done      map[string]bool                   // nodeID → completed?
	Failed    map[string]string                 // nodeID → error message (if any)
	Attempts  map[string]int                    // nodeID → retry attempts so far
	NextPort  map[string]string                 // condition routing (nodeID → port)
}

// newExecutionContext builds an ExecutionContext seeded with trigger input
// in the workflow Variables (under reserved key "trigger").
//
// newExecutionContext 用 trigger input 初始化 workflow Variables(reserved
// key "trigger")。
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
	}
}

// topoState tracks in-degree per node + downstream edges for the
// pick-ready-after-advance loop. Built once at executeRun start;mutated
// as nodes complete.
//
// topoState 跟踪每节点 in-degree + 下游边;executeRun 起跑时建,节点完成
// 时变更。
type topoState struct {
	inDegree   map[string]int
	downstream map[string][]workflowdomain.EdgeSpec // nodeID → outgoing edges
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

// initialReady returns nodes with in-degree 0 (entry points — should be
// trigger nodes after validate.go's ErrNoTrigger gate, but any orphan
// node also lands here).
//
// initialReady 返 in-degree=0 的入口节点。
func (t *topoState) initialReady() []string {
	out := make([]string, 0)
	for id, deg := range t.inDegree {
		if deg == 0 {
			out = append(out, id)
		}
	}
	return out
}

// advance decrements downstream in-degrees of done and returns the new
// ready set. The dispatcher-chosen nextPort selects which downstream
// edges to follow on branching nodes (approval/condition/loop). Edges
// from non-branching nodes always pass (their FromPort is required to
// be empty by validate). Unmatched branch edges are parked (in-degree
// dropped without enqueuing) so the unselected branch never runs.
//
// advance 减下游 in-degree 返新 ready;dispatcher 给的 nextPort 过滤
// 分叉节点的出边。非分叉节点的出边 FromPort 强制为空(validate 保证),
// 总是通过。不匹配的分叉边 park 掉(in-degree 减但不 enqueue),不选的
// 分支永不跑。
func (t *topoState) advance(done string, nextPort string) []string {
	ready := make([]string, 0)
	doneNode := t.byID[done]
	branching := workflowdomain.IsBranchingNode(doneNode.Type)
	for _, e := range t.downstream[done] {
		t.inDegree[e.To]--
		if branching && e.FromPort != nextPort {
			// Branching node + this edge's fromPort doesn't match chosen
			// branch — parked. (in-degree already decremented above so
			// the To-node stays blocked even if other edges feed it.)
			// 分叉节点 + fromPort 不匹配 → park 这条边。
			continue
		}
		if t.inDegree[e.To] == 0 {
			ready = append(ready, e.To)
		}
	}
	return ready
}

// executeRun is the real Service.ExecuteFn (set by NewService). Builds
// the ExecutionContext + initial ready set and delegates the main loop
// to driveLoop (shared with ResumeApproval's continueRun).
//
// executeRun 是 Service.ExecuteFn 的真实实现;建 ExecutionContext + 初始
// ready 后委派 driveLoop(跟 ResumeApproval 的 continueRun 共享)。
func (s *Service) executeRun(ctx context.Context, run *flowrundomain.FlowRun, graph *workflowdomain.Graph) {
	if graph == nil || len(graph.Nodes) == 0 {
		s.finalizeRun(ctx, run, flowrundomain.StatusCompleted, map[string]any{"empty": true}, "", "")
		return
	}
	execCtx := newExecutionContext(run, graph)
	topo := buildTopo(graph)
	ready := topo.initialReady()
	s.driveLoop(ctx, run, graph, execCtx, topo, ready)
}

// dispatchResult bundles one node's input + output for sequential
// post-processing after the parallel dispatch batch.
//
// dispatchResult 把一节点输入输出绑一起,并发批后串行处理。
type dispatchResult struct {
	Node      workflowdomain.NodeSpec
	Input     map[string]any
	Output    DispatchOutput
	StartedAt time.Time
	EndedAt   time.Time
}

// dispatchBatch runs `nodes` in parallel via goroutines.
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
			// dispatchWithPolicies wraps router.Dispatch in retry + per-
			// attempt timeout layers (E9). Plain router.Dispatch is also
			// fine for tests that don't exercise those — retry MaxAttempts
			// ≤ 1 + Timeout = 0 means policy layers are no-ops.
			// dispatchWithPolicies 在 router.Dispatch 外套 retry + per-
			// attempt timeout 层;tests 不触这些时 policy 层 no-op。
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

// buildNodeInput resolves the node's input port data from upstream
// outputs + workflow variables. V1 minimal: passes upstream "out" outputs
// merged + trigger payload. E7-E8 dispatchers may template / project
// further per their needs.
//
// buildNodeInput 从上游 output + workflow Variables 拼节点输入;V1 最小化。
func buildNodeInput(_ workflowdomain.NodeSpec, _ *ExecutionContext) map[string]any {
	// V1 passes empty input map — each dispatcher reads what it needs from
	// node.Config + execCtx.Outputs + execCtx.Variables directly. This
	// keeps the framework simple;E7-E8 dispatchers do per-type resolution.
	// V1 不通用解析输入;dispatcher 各自从 node.Config + execCtx 读所需。
	return map[string]any{}
}

// recordNode writes one terminal flowrun_nodes row (best-effort — failure
// logs but does not abort the run). Uses detached ctx via runCtx variant
// since the caller ctx may be cancelled during finalization.
//
// recordNode 写 flowrun_nodes 终态(best-effort 失败 log 不挂 run);用
// caller ctx(已 detached from HTTP)所以可写。
func (s *Service) recordNode(ctx context.Context, run *flowrundomain.FlowRun, res dispatchResult, execCtx *ExecutionContext) {
	status := flowrundomain.NodeStatusOK
	if res.Output.Error != nil {
		status = flowrundomain.NodeStatusFailed
	}
	if ctx.Err() != nil && res.Output.Error == nil {
		// Run was cancelled — anything that hadn't reported its own error
		// is marked cancelled (it likely propagated ctx.Done internally
		// but didn't return an Error before our select picked it up).
		// 运行已取消 — 未自报错的节点标 cancelled。
		status = flowrundomain.NodeStatusCancelled
	}

	row := &flowrundomain.Node{
		ID:          idgenpkg.New("frn"),
		UserID:      run.UserID,
		Status:      status,
		TriggeredBy: flowrundomain.TriggerKindCron, // overridden below
		Input:       res.Input,
		Output:      res.Output.Outputs,
		StartedAt:   res.StartedAt,
		EndedAt:     res.EndedAt,
		ElapsedMs:   res.EndedAt.Sub(res.StartedAt).Milliseconds(),
		FlowrunID:   run.ID,
		NodeID:      res.Node.ID,
		NodeType:    res.Node.Type,
		Attempts:    maxInt(1, execCtx.Attempts[res.Node.ID]),
	}
	row.TriggeredBy = "workflow" // Node executions are workflow-scoped (see 08-executions §2)
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

// finalizeRun writes the FlowRun terminal status + applies retention
// pruning (§6.7) + publishes a notification.
//
// finalizeRun 写 FlowRun 终态 + 保留策略剪 + 推通知。
func (s *Service) finalizeRun(ctx context.Context, run *flowrundomain.FlowRun, status string, output any, errCode, errMsg string) {
	endedAt := time.Now().UTC()
	elapsedMs := endedAt.Sub(run.StartedAt).Milliseconds()
	if err := s.repo.UpdateStatus(ctx, run.ID, status, output, errCode, errMsg, &endedAt, elapsedMs); err != nil {
		s.log.Error("scheduler.finalizeRun: UpdateStatus failed",
			zap.String("runID", run.ID), zap.Error(err))
	}
	// Retention prune (best-effort §6.7).
	if err := s.repo.HardDeleteOldest(ctx, run.WorkflowID, flowrundomain.DefaultRetentionLimit); err != nil {
		s.log.Warn("scheduler.finalizeRun: HardDeleteOldest failed",
			zap.String("workflowID", run.WorkflowID), zap.Error(err))
	}
	s.publish(ctx, run.ID, run.WorkflowID, status, map[string]any{
		"elapsedMs": elapsedMs,
	})
}

// nodeOnError reads the NodeSpec's OnError policy. Empty → stop.
//
// nodeOnError 读 OnError 策略;空 → stop。
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
