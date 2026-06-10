package scheduler

import (
	"context"

	"go.uber.org/zap"

	flowrundomain "github.com/sunweilin/forgify/backend/internal/domain/flowrun"
)

// trackInflight wraps an advance with a cancellable context registered under the run id, so
// KillWorkflow can interrupt this run even while it is blocked deep in a node (a long agent's
// loop.Run, a slow function). Advance is synchronous and per-run single-goroutine, so there is at
// most one cancel per run. The returned release deregisters and cancels (freeing the ctx). On a
// normal finish release just cleans up; on a kill the cancel has already fired and release is a
// harmless second cancel.
//
// trackInflight 给一次 advance 包一个按 run id 注册的可取消 ctx，使 KillWorkflow 能打断这个 run——即便它
// 正卡在某节点深处（长 agent 的 loop.Run、慢 function）。Advance 同步且 per-run 单 goroutine，故每个 run
// 至多一个 cancel。返回的 release 注销并 cancel（释放 ctx）。正常结束时 release 只清理；被 kill 时 cancel
// 已先触发、release 是无害的第二次 cancel。
func (s *Service) trackInflight(ctx context.Context, flowrunID string) (context.Context, func()) {
	cctx, cancel := context.WithCancel(ctx)
	s.inflightMu.Lock()
	s.inflight[flowrunID] = cancel
	s.inflightMu.Unlock()
	return cctx, func() {
		s.inflightMu.Lock()
		delete(s.inflight, flowrunID)
		s.inflightMu.Unlock()
		cancel()
	}
}

// cancelInflight cancels a run's in-progress advance if one is registered (interrupting a blocked
// node). A no-op when the run is not actively advancing — e.g. a run parked on an approval has
// already returned from advance, so there is nothing to interrupt; KillWorkflow then just marks it
// cancelled in the store.
//
// cancelInflight 取消某 run 在途的 advance（若有注册）（打断阻塞的节点）。run 未在 advance 时 no-op——如
// park 在审批上的 run 早已从 advance 返回、无可打断；KillWorkflow 随即只在 store 里标 cancelled。
func (s *Service) cancelInflight(flowrunID string) {
	s.inflightMu.Lock()
	cancel := s.inflight[flowrunID]
	s.inflightMu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// KillWorkflow hard-stops a workflow's execution: every currently-running run is cancelled — its
// in-progress advance interrupted via ctx (so a long agent / function returns at once), then its
// header marked cancelled (first-wins; a run that finished in the same instant keeps its real
// terminal). Detaching the trigger listener and flipping lifecycle are the workflow service's job
// (it calls this after Detach). Returns how many runs were killed. Workspace-scoped.
//
// KillWorkflow 硬停一个 workflow 的执行：当前所有 running run 都被取消——经 ctx 打断其在途 advance（长
// agent / function 立即返回），再把头标 cancelled（first-wins；同一瞬间结束的 run 保留其真实终态）。摘 trigger
// 监听、翻 lifecycle 是 workflow service 的事（它在 Detach 后调本法）。返被杀 run 数。按 workspace 隔离。
func (s *Service) KillWorkflow(ctx context.Context, workflowID string) (int, error) {
	runs, err := s.runs.ListRunningByWorkflow(ctx, workflowID)
	if err != nil {
		return 0, err
	}
	for _, r := range runs {
		// Mark cancelled BEFORE cancelling the ctx: the interrupted advance's RunAgent/RunAction will
		// return ctx.Err(), which the interpreter would otherwise turn into a `failed` run via failNode.
		// Writing cancelled first (guarded WHERE running) makes cancelled win — failNode's later
		// mark-failed matches 0 rows and is a no-op. Order matters for the run's recorded terminal.
		//
		// 先标 cancelled 再 cancel ctx：被打断的 advance 的 RunAgent/RunAction 会返 ctx.Err()，否则解释器会
		// 经 failNode 把 run 变 `failed`。先写 cancelled（守卫 WHERE running）使 cancelled 赢——failNode 随后
		// 的 mark-failed 匹配 0 行 no-op。顺序决定 run 记录的终态。
		if err := s.runs.MarkRunTerminal(ctx, r.ID, flowrundomain.StatusCancelled, "killed by user"); err != nil {
			s.log.Warn("schedulerapp.KillWorkflow: mark cancelled", zap.String("flowrun", r.ID), zap.Error(err))
		}
		s.cancelInflight(r.ID)
	}
	return len(runs), nil
}

// CountRunning reports a workflow's in-flight run count (the workflow service's Runner port uses it
// to pick draining vs inactive on :deactivate). Workspace-scoped.
//
// CountRunning 报告一个 workflow 在途 run 数（workflow service 的 Runner 端口据此在 :deactivate 选
// draining vs inactive）。按 workspace 隔离。
func (s *Service) CountRunning(ctx context.Context, workflowID string) (int, error) {
	return s.runs.CountRunningByWorkflow(ctx, workflowID)
}

// markRunTerminal flips a run terminal then reconciles its workflow's drain state — the one
// chokepoint for a run reaching completed/failed (kill writes cancelled directly via the store and
// flips lifecycle itself). When a draining workflow's LAST in-flight run settles here, the workflow
// becomes inactive (graceful-drain complete).
//
// markRunTerminal 把 run 翻终态后结算其 workflow 排空——run 走到 completed/failed 的唯一收口（kill 经 store
// 直接写 cancelled、自己翻 lifecycle）。当一个 draining workflow 的**最后**一个在途 run 在此结算，该 workflow
// 变 inactive（优雅排空完成）。
func (s *Service) markRunTerminal(ctx context.Context, run *flowrundomain.FlowRun, status, msg string) error {
	if err := s.runs.MarkRunTerminal(ctx, run.ID, status, msg); err != nil {
		return err
	}
	s.afterRunSettled(ctx, run.WorkflowID)
	return nil
}

// afterRunSettled flips a draining workflow to inactive once it has no running runs left. Best-effort
// + nil-tolerant: a count/reconcile error is logged, never failing the run that just settled.
//
// afterRunSettled 在某 workflow 无 running run 后把 draining 翻 inactive。best-effort + nil-tolerant：
// count/reconcile 出错只记日志，绝不连累刚结算的 run。
func (s *Service) afterRunSettled(ctx context.Context, workflowID string) {
	if s.recon == nil {
		return
	}
	n, err := s.runs.CountRunningByWorkflow(ctx, workflowID)
	if err != nil {
		s.log.Warn("schedulerapp: count running for drain reconcile", zap.String("workflow", workflowID), zap.Error(err))
		return
	}
	if n > 0 {
		return
	}
	if err := s.recon.MarkInactiveIfDrained(ctx, workflowID); err != nil {
		s.log.Warn("schedulerapp: drain reconcile", zap.String("workflow", workflowID), zap.Error(err))
	}
}
