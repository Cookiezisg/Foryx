package scheduler

import (
	"context"
	"encoding/json"
	"fmt"

	entitystreamapp "github.com/sunweilin/forgify/backend/internal/app/entitystream"
	flowrundomain "github.com/sunweilin/forgify/backend/internal/domain/flowrun"
	streamdomain "github.com/sunweilin/forgify/backend/internal/domain/stream"
	workflowdomain "github.com/sunweilin/forgify/backend/internal/domain/workflow"
	celpkg "github.com/sunweilin/forgify/backend/internal/pkg/cel"
)

// Advance is the idempotent heart of the engine: walk the run's FROZEN graph against its memoized
// frn rows until no (node, iteration) is ready, then finalize. Calling it repeatedly — including
// after a crash, on the same run — converges to the same state, because completed rows are copied
// (record-once), never re-run. A batch of ready nodes is run, then the rows are re-read and the
// walk recomputed (a freshly-completed node can unblock its successors). The loop ends when nothing
// is ready (→ finalize: completed, or still-running if a node is parked) or a node fails (fail-fast).
//
// Advance 是引擎的幂等核心：照 run 冻结的图、对其记忆化 frn 行走，直到无 (节点,轮次) ready，再
// finalize。反复调用（含崩溃后、同一 run）收敛到同一状态，因为 completed 行被抄（record-once）、绝不
// 重跑。跑一批 ready 节点后重读行、重算 walk（刚完成的节点可解锁后继）。循环在无人 ready（→ finalize：
// completed 或有 parked 则仍 running）或某节点失败（fail-fast）时结束。
func (s *Service) Advance(ctx context.Context, flowrunID string) error {
	run, err := s.runs.GetRun(ctx, flowrunID)
	if err != nil {
		return err
	}
	if run.Status != flowrundomain.StatusRunning {
		return nil // already terminal — nothing to do
	}
	// Register a cancellable child ctx for this drive so KillWorkflow can interrupt a node blocked
	// mid-flight (a long agent). On a normal finish release() just deregisters.
	// 为本次驱动注册可取消子 ctx，使 KillWorkflow 能打断卡在节点中的运行（长 agent）。正常结束时 release() 只注销。
	ctx, release := s.trackInflight(ctx, flowrunID)
	defer release()
	ver, err := s.workflows.GetVersion(ctx, run.VersionID)
	if err != nil {
		return fmt.Errorf("schedulerapp.Advance: pinned version %s: %w", run.VersionID, err)
	}
	graph, err := decodeGraph(ver.Graph)
	if err != nil {
		return err
	}
	senv, err := celScopedEnv(graph)
	if err != nil {
		return fmt.Errorf("schedulerapp.Advance: cel env: %w", err)
	}

	for {
		// Bail if this drive was interrupted (KillWorkflow cancelled our ctx, or the app is shutting
		// down). Not an error: the durable state is authoritative — kill already marked the run
		// cancelled; a shutdown leaves it running for the next boot's Recover to re-walk.
		// 若本次驱动被打断（KillWorkflow 取消了 ctx，或 app 关停）则退出。非错误：durable 状态为准——kill
		// 已标 run cancelled；shutdown 留 run running 待下次 boot 的 Recover 重走。
		if ctx.Err() != nil {
			return nil
		}
		rows, err := s.runs.GetNodes(ctx, flowrunID)
		if err != nil {
			return err
		}
		w := newWalk(graph, rows)
		ready, overflow := w.computeReady()
		if overflow != "" {
			return s.failRun(ctx, run, fmt.Sprintf("loop exceeded MaxIterations (%d) at node %q", MaxIterations, overflow))
		}
		if len(ready) == 0 {
			break
		}
		advanced := false
		for _, rn := range ready {
			status, err := s.runNode(ctx, run, senv, w, rn)
			if err != nil {
				return err
			}
			s.emitNodeProgress(ctx, run, rn, status) // SSE-C: workflow panel run terminal
			if status == flowrundomain.NodeFailed {
				return nil // fail-fast: the run was already marked failed inside failNode
			}
			if status == flowrundomain.NodeCompleted {
				advanced = true
			}
		}
		if !advanced {
			break // every ready node this batch parked → yield, await external signals
		}
	}
	return s.finalize(ctx, run, flowrunID)
}

// emitNodeProgress streams one node's terminal status onto the entities stream scoped to the
// workflow, so the workflow panel shows a flowrun progressing node by node (SSE-C). A point Signal
// keyed by flowrunId; the durable record is flowrun_nodes. nil bridge → no-op.
//
// emitNodeProgress 把一个节点的终态流到 workflow scope 的 entities 流，使 workflow 面板逐节点显示 flowrun
// 推进（SSE-C）。按 flowrunId 的点 Signal；耐久记录是 flowrun_nodes。nil bridge → no-op。
func (s *Service) emitNodeProgress(ctx context.Context, run *flowrundomain.FlowRun, rn readyNode, status string) {
	if s.entities == nil {
		return
	}
	content, _ := json.Marshal(map[string]any{
		"flowrunId": run.ID,
		"nodeId":    rn.node.ID,
		"iteration": rn.iter,
		"status":    status,
	})
	entitystreamapp.Signal(ctx, s.entities, streamdomain.Scope{Kind: streamdomain.KindWorkflow, ID: run.WorkflowID}, entitystreamapp.NodeRun, content)
}

// finalize settles a run that has no ready nodes left: still-running if any node is parked (awaiting
// a human/timeout signal), otherwise completed. (Failure is handled inline by failNode/failRun.)
//
// finalize 结算无 ready 节点的 run：有 parked 节点则仍 running（等人/超时信号），否则 completed。
// （失败由 failNode/failRun 内联处理。）
func (s *Service) finalize(ctx context.Context, run *flowrundomain.FlowRun, flowrunID string) error {
	rows, err := s.runs.GetNodes(ctx, flowrunID)
	if err != nil {
		return err
	}
	for _, r := range rows {
		if r.Status == flowrundomain.NodeParked {
			return nil // waiting on a signal — the run stays running
		}
	}
	if err := s.markRunTerminal(ctx, run, flowrundomain.StatusCompleted, ""); err != nil {
		return fmt.Errorf("schedulerapp.finalize: %w", err)
	}
	return nil
}

// failRun marks the whole run failed (used for engine-level failures like a loop overflow; node
// activity failures go through failNode which also writes the failed node row).
//
// failRun 把整个 run 标 failed（用于引擎级失败如循环溢出；节点 activity 失败走 failNode，它还写
// failed 节点行）。
func (s *Service) failRun(ctx context.Context, run *flowrundomain.FlowRun, msg string) error {
	if err := s.markRunTerminal(ctx, run, flowrundomain.StatusFailed, msg); err != nil {
		return fmt.Errorf("schedulerapp.failRun: %w", err)
	}
	return nil
}

// celScopedEnv builds the CEL environment whose roots are the graph's node ids — so a node's Input
// CEL can address ancestors by node id (model B). The same env is reused for every node in the run.
//
// celScopedEnv 构建以图 node id 为根的 CEL 环境——使节点 Input CEL 能按 node id 寻址祖先（model B）。
// 同一 env 复用于该 run 的每个节点。
func celScopedEnv(graph *workflowdomain.Graph) (*celpkg.ScopedEnv, error) {
	roots := make([]string, len(graph.Nodes))
	for i := range graph.Nodes {
		roots[i] = graph.Nodes[i].ID
	}
	return celpkg.NewScopedEnv(roots)
}
