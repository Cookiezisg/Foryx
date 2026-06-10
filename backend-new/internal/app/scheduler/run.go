package scheduler

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.uber.org/zap"

	approvaldomain "github.com/sunweilin/forgify/backend/internal/domain/approval"
	flowrundomain "github.com/sunweilin/forgify/backend/internal/domain/flowrun"
	triggerdomain "github.com/sunweilin/forgify/backend/internal/domain/trigger"
	workflowdomain "github.com/sunweilin/forgify/backend/internal/domain/workflow"
	ormpkg "github.com/sunweilin/forgify/backend/internal/pkg/orm"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

// StartInput parameterises a new run. WorkflowID is required; the rest are optional. EntryNode picks
// the entry trigger node when a graph has several (manual path); TriggerID picks it by referenced
// trg_ (firing path). Payload becomes the trigger node's result — the data the workflow reads as
// `<triggerNode>.field`.
//
// StartInput 参数化一个新 run。WorkflowID 必填，其余可选。EntryNode 在多 trigger 图里选入口（手动）；
// TriggerID 按引用的 trg_ 选（firing）。Payload 成为 trigger 节点的 result——workflow 读 `<trigger>.字段`。
type StartInput struct {
	WorkflowID string
	EntryNode  string // explicit entry node id (manual, multi-trigger)
	TriggerID  string // entry by referenced trg_ (firing path)
	Payload    map[string]any
	FiringID   string // source firing (firing path); "" for manual
}

// StartRun is the manual-trigger path (UI/API "Run now"): build the run + seed its trigger node in
// one transaction, then advance. No firing claim (a human asked once — nothing to dedup). Returns
// the new flowrun id.
//
// StartRun 是手动 trigger 路径（UI/API「Run now」）：单事务建 run + seed trigger 节点，再 advance。
// 无 firing claim（人明确点一次、没东西可去重）。返新 flowrun id。
func (s *Service) StartRun(ctx context.Context, in StartInput) (string, error) {
	run, trig, err := s.buildRun(ctx, in)
	if err != nil {
		return "", err
	}
	if _, err := s.runs.CreateRunWithTrigger(ctx, run, trig); err != nil {
		return "", fmt.Errorf("schedulerapp.StartRun: %w", err)
	}
	if err := s.Advance(ctx, run.ID); err != nil {
		return run.ID, err // run exists; surface the advance error but keep the id
	}
	return run.ID, nil
}

// buildRun resolves the workflow's active version, pins its referenced entities, finds the entry
// trigger node, and assembles the (run header, seed trigger node) pair — all READS, done outside any
// claim transaction (the firing path then writes them in the claim tx via SeedRunOnTx).
//
// buildRun 解析 workflow 的 active 版本、pin 其引用实体、找入口 trigger 节点、组装 (run 头, seed
// trigger 节点)——全是读，在任何 claim 事务之外做（firing 路径再经 SeedRunOnTx 在 claim 事务里写）。
func (s *Service) buildRun(ctx context.Context, in StartInput) (*flowrundomain.FlowRun, *flowrundomain.FlowRunNode, error) {
	ver, err := s.workflows.GetActiveVersion(ctx, in.WorkflowID)
	if err != nil {
		return nil, nil, fmt.Errorf("schedulerapp.buildRun: active version: %w", err)
	}
	graph, err := decodeGraph(ver.Graph)
	if err != nil {
		return nil, nil, err
	}
	entry, err := resolveEntry(graph, in.EntryNode, in.TriggerID)
	if err != nil {
		return nil, nil, err
	}
	pins, err := s.workflows.BuildPinClosure(ctx, graph)
	if err != nil {
		return nil, nil, fmt.Errorf("schedulerapp.buildRun: pin closure: %w", err)
	}
	payload := in.Payload
	if payload == nil {
		payload = map[string]any{}
	}
	run := &flowrundomain.FlowRun{
		WorkflowID: in.WorkflowID,
		VersionID:  ver.ID,
		PinnedRefs: pins,
		TriggerID:  in.TriggerID,
		FiringID:   in.FiringID,
		Status:     flowrundomain.StatusRunning,
	}
	trig := &flowrundomain.FlowRunNode{
		NodeID: entry.ID,
		Kind:   workflowdomain.NodeKindTrigger,
		Ref:    entry.Ref,
		Status: flowrundomain.NodeCompleted,
		Result: payload,
	}
	return run, trig, nil
}

// resolveEntry picks the entry trigger node: an explicit entryNode id (manual, multi-trigger) wins;
// else the node referencing triggerRef (firing path); else the sole trigger node. Ambiguity (many
// triggers, no selector) or a bad selector is ErrInvalidEntry.
//
// resolveEntry 选入口 trigger 节点：显式 entryNode id（手动、多 trigger）优先；否则引用 triggerRef 的
// 节点（firing）；否则唯一的 trigger 节点。歧义（多 trigger 无选择器）或选择器错 = ErrInvalidEntry。
func resolveEntry(graph *workflowdomain.Graph, entryNode, triggerRef string) (*workflowdomain.Node, error) {
	if entryNode != "" {
		for i := range graph.Nodes {
			n := &graph.Nodes[i]
			if n.ID == entryNode {
				if n.Kind != workflowdomain.NodeKindTrigger {
					return nil, flowrundomain.ErrInvalidEntry.WithDetails(map[string]any{"reason": fmt.Sprintf("entry node %q is kind %q, not a trigger", entryNode, n.Kind)})
				}
				return n, nil
			}
		}
		return nil, flowrundomain.ErrInvalidEntry.WithDetails(map[string]any{"reason": fmt.Sprintf("entry node %q not found", entryNode)})
	}
	if triggerRef != "" {
		for i := range graph.Nodes {
			n := &graph.Nodes[i]
			if n.Kind == workflowdomain.NodeKindTrigger && n.Ref == triggerRef {
				return n, nil
			}
		}
		return nil, flowrundomain.ErrInvalidEntry.WithDetails(map[string]any{"reason": fmt.Sprintf("no trigger node references %q", triggerRef)})
	}
	var sole *workflowdomain.Node
	count := 0
	for i := range graph.Nodes {
		if graph.Nodes[i].Kind == workflowdomain.NodeKindTrigger {
			sole = &graph.Nodes[i]
			count++
		}
	}
	switch count {
	case 0:
		return nil, flowrundomain.ErrInvalidEntry.WithDetails(map[string]any{"reason": "graph has no trigger node"})
	case 1:
		return sole, nil
	default:
		return nil, flowrundomain.ErrInvalidEntry.WithDetails(map[string]any{"reason": "graph has multiple trigger nodes; specify entryNode"})
	}
}

// DrainFirings claims every pending firing and turns it into a run (the automatic path). No-op when
// no inbox is wired (manual-only deployments). Per-firing failures are logged and skipped — one bad
// firing must not stall the queue.
//
// DrainFirings claim 每条 pending firing 转成 run（自动路径）。无 inbox 时 no-op（纯手动部署）。逐条
// 失败记日志跳过——一条坏 firing 不该卡住队列。
func (s *Service) DrainFirings(ctx context.Context) error {
	if s.inbox == nil {
		return nil
	}
	firings, err := s.inbox.ListPendingFirings(ctx, 100)
	if err != nil {
		return fmt.Errorf("schedulerapp.DrainFirings: list pending: %w", err)
	}
	for _, f := range firings {
		if err := s.consumeFiring(ctx, f); err != nil {
			s.log.Warn("schedulerapp: consume firing failed", zap.String("firing", f.ID), zap.Error(err))
		}
	}
	return nil
}

// consumeFiring turns one firing into a run, honoring the workflow's overlap policy, in the firing's
// own workspace context. The single-tx claim (ClaimFiring) does pending→claimed + SeedRunOnTx + the
// started backfill atomically (ADR-021) so a crash can never leave a claimed-but-no-run strand. The
// run is then advanced outside the claim tx.
//
// consumeFiring 把一条 firing 转成 run，遵守 workflow overlap 策略，在 firing 自己的 workspace ctx 里。
// 单事务 claim（ClaimFiring）原子做 pending→claimed + SeedRunOnTx + started 回填（ADR-021），崩溃绝不
// 留 claimed-但-无-run 残留。run 在 claim 事务外 advance。
func (s *Service) consumeFiring(ctx context.Context, f *triggerdomain.Firing) error {
	fctx := reqctxpkg.SetWorkspaceID(ctx, f.WorkspaceID)

	action, outcome, err := s.overlapDecision(fctx, f)
	if err != nil {
		return err
	}
	switch action {
	case overlapDefer:
		return nil // serial + a run already in flight → leave pending, re-drained later
	case overlapSkip:
		return s.inbox.MarkFiringOutcome(fctx, f.ID, outcome)
	}

	// reads outside the tx (active version + pin + entry resolution).
	run, trig, err := s.buildRun(fctx, StartInput{
		WorkflowID: f.WorkflowID,
		TriggerID:  f.TriggerID,
		Payload:    f.Payload,
		FiringID:   f.ID,
	})
	if err != nil {
		return err
	}

	runID, err := s.inbox.ClaimFiring(fctx, f.ID, func(tx *ormpkg.DB) (string, error) {
		if err := s.runs.SeedRunOnTx(fctx, tx, run, trig); err != nil {
			return "", err
		}
		return run.ID, nil
	})
	if err != nil {
		if errors.Is(err, triggerdomain.ErrFiringNotPending) {
			return nil // lost the claim race (already consumed) — fine
		}
		return fmt.Errorf("schedulerapp.consumeFiring: claim: %w", err)
	}
	return s.Advance(fctx, runID)
}

type overlapAction int

const (
	overlapRun overlapAction = iota
	overlapSkip
	overlapDefer
)

// overlapDecision applies the workflow's concurrency policy to a new firing. v1 implements serial
// (defer while a run is in flight), Skip (drop), and AllowAll (always run). BufferOne/BufferAll are
// v2 — treated as AllowAll so firings are never silently lost.
//
// overlapDecision 对新 firing 应用 workflow 并发策略。v1 实现 serial（有 run 在途则推迟）、Skip（丢）、
// AllowAll（总跑）。BufferOne/BufferAll 是 v2——按 AllowAll 处理使 firing 绝不静默丢失。
func (s *Service) overlapDecision(ctx context.Context, f *triggerdomain.Firing) (overlapAction, string, error) {
	w, err := s.workflows.GetWorkflow(ctx, f.WorkflowID)
	if err != nil {
		return overlapRun, "", fmt.Errorf("schedulerapp.overlapDecision: %w", err)
	}
	switch w.Concurrency {
	case workflowdomain.ConcurrencySerial, workflowdomain.ConcurrencySkip:
		running, err := s.runs.CountRunningByWorkflow(ctx, f.WorkflowID)
		if err != nil {
			return overlapRun, "", err
		}
		if running > 0 {
			if w.Concurrency == workflowdomain.ConcurrencySkip {
				return overlapSkip, triggerdomain.FiringSkipped, nil
			}
			return overlapDefer, "", nil // serial: wait, stays pending
		}
		return overlapRun, "", nil
	default:
		return overlapRun, "", nil // AllowAll / BufferOne / BufferAll(v2) → run
	}
}

// Recover re-walks every still-running flowrun across all workspaces (boot crash recovery): each
// advance copies completed rows and re-runs whatever the crash interrupted (at-least-once). Each run
// advances in a context scoped to its own workspace.
//
// Recover 重走所有 workspace 中每个仍 running 的 flowrun（boot 崩溃恢复）：每次 advance 抄 completed
// 行、重跑崩溃打断的（at-least-once）。每个 run 在其自己 workspace 的 ctx 里 advance。
func (s *Service) Recover(ctx context.Context) error {
	running, err := s.runs.ListRunningRuns(ctx)
	if err != nil {
		return fmt.Errorf("schedulerapp.Recover: %w", err)
	}
	for _, run := range running {
		rctx := reqctxpkg.SetWorkspaceID(ctx, run.WorkspaceID)
		if err := s.Advance(rctx, run.ID); err != nil {
			s.log.Warn("schedulerapp: recover advance failed", zap.String("flowrun", run.ID), zap.Error(err))
		}
	}
	return nil
}

// DecideApproval resolves a parked approval node with a human decision and re-drives the run. The
// conditional update is first-wins: a decision that loses to an earlier decision/timeout returns
// ErrNodeNotParked (a clean 422), never corrupting the recorded outcome.
//
// DecideApproval 用人工决策落定一个 parked approval 节点并重驱 run。条件更新 first-wins：输给更早
// 决策/超时的决策返 ErrNodeNotParked（干净 422），绝不污染已记结果。
func (s *Service) DecideApproval(ctx context.Context, flowrunID, nodeID, decision, reason string) error {
	if decision != workflowdomain.ApprovalPortYes && decision != workflowdomain.ApprovalPortNo {
		return flowrundomain.ErrInvalidDecision
	}
	won, err := s.runs.ResolveParkedNode(ctx, flowrunID, nodeID, flowrundomain.NodeCompleted, flowrundomain.ApprovalDecision(decision, reason))
	if err != nil {
		return fmt.Errorf("schedulerapp.DecideApproval: %w", err)
	}
	if !won {
		return flowrundomain.ErrNodeNotParked // already decided / timed out — first-wins loser
	}
	return s.Advance(ctx, flowrunID)
}

// CheckTimeouts settles parked approvals whose deadline has passed (the one durable timer, doc 21
// §4.5). For each parked node it resolves the pinned form's Timeout/TimeoutBehavior; reject→no,
// approve→yes, fail→fail the run. first-wins guards against racing a human decision. Workspace-scoped
// (the caller ticks it per workspace; for a single-user app that is the one workspace).
//
// CheckTimeouts 落定到期的 parked approval（唯一 durable timer，doc 21 §4.5）。对每个 parked 节点解析
// pin 表单的 Timeout/TimeoutBehavior；reject→no、approve→yes、fail→run 失败。first-wins 防与人工决策
// 竞争。按 workspace 隔离（调用方逐 workspace tick；单用户即一个 workspace）。
func (s *Service) CheckTimeouts(ctx context.Context, now time.Time) error {
	parked, err := s.runs.ListParkedNodes(ctx)
	if err != nil {
		return fmt.Errorf("schedulerapp.CheckTimeouts: %w", err)
	}
	for _, p := range parked {
		run, err := s.runs.GetRun(ctx, p.FlowRunID)
		if err != nil {
			s.log.Warn("schedulerapp.CheckTimeouts: get run", zap.String("flowrun", p.FlowRunID), zap.Error(err))
			continue
		}
		form, err := s.approval.Resolve(ctx, p.Ref, run.PinnedRefs[entityIDOf(p.Ref)])
		if err != nil {
			s.log.Warn("schedulerapp.CheckTimeouts: resolve form", zap.String("ref", p.Ref), zap.Error(err))
			continue
		}
		d, err := approvaldomain.ParseTimeout(form.Timeout)
		if err != nil || d == 0 {
			continue // unparseable or never-times-out
		}
		if p.CreatedAt.Add(d).After(now) {
			continue // not yet due
		}
		if err := s.settleTimeout(ctx, run, p, form.TimeoutBehavior); err != nil {
			s.log.Warn("schedulerapp.CheckTimeouts: settle", zap.String("flowrun", p.FlowRunID), zap.Error(err))
		}
	}
	return nil
}

// settleTimeout resolves one timed-out parked node per its behavior (first-wins), then re-drives or
// fails the run.
//
// settleTimeout 按 behavior 落定一个到期 parked 节点（first-wins），再重驱或失败 run。
func (s *Service) settleTimeout(ctx context.Context, run *flowrundomain.FlowRun, p *flowrundomain.FlowRunNode, behavior string) error {
	if behavior == approvaldomain.TimeoutFail {
		won, err := s.runs.ResolveParkedNode(ctx, p.FlowRunID, p.NodeID, flowrundomain.NodeFailed, map[string]any{"reason": "approval timed out"})
		if err != nil || !won {
			return err
		}
		return s.markRunTerminal(ctx, run, flowrundomain.StatusFailed, fmt.Sprintf("approval %s timed out", p.NodeID))
	}
	decision := workflowdomain.ApprovalPortNo
	if behavior == approvaldomain.TimeoutApprove {
		decision = workflowdomain.ApprovalPortYes
	}
	won, err := s.runs.ResolveParkedNode(ctx, p.FlowRunID, p.NodeID, flowrundomain.NodeCompleted, flowrundomain.ApprovalDecision(decision, "timeout"))
	if err != nil || !won {
		return err
	}
	return s.Advance(ctx, p.FlowRunID)
}

// Replay fixes a failed run: clear its failed node rows (a non-result), reopen the run to running +
// bump replay_count, then re-walk — completed rows are reused, the cleared steps re-run. ErrNotReplayable
// if the run is not failed.
//
// Replay 修复失败的 run：清其 failed 节点行（非结果）、把 run 翻回 running + replay_count++、再重走——
// completed 行复用、被清的步骤重跑。run 非 failed 则 ErrNotReplayable。
func (s *Service) Replay(ctx context.Context, flowrunID string) error {
	run, err := s.runs.GetRun(ctx, flowrunID)
	if err != nil {
		return err
	}
	if run.Status != flowrundomain.StatusFailed {
		return flowrundomain.ErrNotReplayable
	}
	if _, err := s.runs.DeleteFailedNodes(ctx, flowrunID); err != nil {
		return fmt.Errorf("schedulerapp.Replay: %w", err)
	}
	if err := s.runs.ReopenForReplay(ctx, flowrunID); err != nil {
		return err
	}
	return s.Advance(ctx, flowrunID)
}
