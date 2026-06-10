package workflow

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	errorsdomain "github.com/sunweilin/forgify/backend/internal/domain/errors"
	workflowdomain "github.com/sunweilin/forgify/backend/internal/domain/workflow"
)

// execution.go is the workflow EXECUTION-LIFECYCLE surface (D1): the five actions that drive a
// workflow's runtime — trigger (run now), stage (arm for one real fire), activate (go live),
// deactivate (graceful off), kill (hard stop). They coordinate two injected collaborators: the
// trigger Binder (engage/disengage the listener) and the scheduler Runner (start/stop runs).
// "Store/validate/pin the graph" lives in crud.go; this is "drive the graph's execution".
//
// execution.go 是 workflow 执行生命周期入口（D1）：驱动 workflow 运行时的五个动作——trigger（现在跑）、
// stage（待命接一次真实触发）、activate（上线）、deactivate（优雅下线）、kill（硬停）。它们协调两个注入的
// 协作者：trigger Binder（挂/摘监听）+ scheduler Runner（起/停 run）。「存/校验/pin 图」在 crud.go；这里是
// 「驱动图的执行」。

// errExecUnavailable guards the five actions when the execution ports were never wired (a
// misconfiguration — in production SetExecutionPorts always runs at assembly). Surfaced as a 500.
//
// errExecUnavailable 在执行端口未接线（配置错误——生产中 SetExecutionPorts 必在装配时跑）时守卫五个动作。以 500 上呈。
var errExecUnavailable = errorsdomain.New(errorsdomain.KindInternal, "WORKFLOW_EXECUTION_UNAVAILABLE", "workflow execution lifecycle is unavailable (engine not wired)")

// Trigger fires one run now with a payload shaped like the entry trigger's signal (the LLM/UI
// "run now"). It does not touch listening state — it works whatever the lifecycle. Returns the new
// flowrun id. A missing active version / trigger entry surfaces from the scheduler as a clean 422.
//
// Trigger 用形如入口 trigger 信号的 payload 立即跑一次（LLM/UI「现在跑」）。不碰监听状态——任何 lifecycle 下
// 都能跑。返新 flowrun id。无 active 版本 / trigger 入口由调度器以干净 422 上呈。
func (s *Service) Trigger(ctx context.Context, id string, payload map[string]any) (string, error) {
	if s.runner == nil {
		return "", errExecUnavailable
	}
	return s.runner.StartRun(ctx, id, payload)
}

// Stage arms the workflow for exactly ONE run on its next real trigger fire, then auto-disarms (the
// "trial run"). It does not change lifecycle — the workflow stays inactive but armed. ErrAlreadyActive
// if it is already continuously listening (deactivate first); ErrNoTriggerEntry if it has no trigger node.
//
// Stage 给 workflow 待命，使其在下一次真实触发时恰跑一次、随即自动撤防（「试运行」）。不改 lifecycle——workflow
// 保持 inactive 但已待命。已在持续监听 → ErrAlreadyActive（先 deactivate）；无 trigger 节点 → ErrNoTriggerEntry。
func (s *Service) Stage(ctx context.Context, id string) error {
	if s.binder == nil {
		return errExecUnavailable
	}
	w, err := s.repo.GetWorkflow(ctx, id)
	if err != nil {
		return err
	}
	if w.Active {
		return workflowdomain.ErrAlreadyActive
	}
	refs, err := s.entryTriggerRefsOf(ctx, w)
	if err != nil {
		return err
	}
	for _, ref := range refs {
		if err := s.binder.AttachOnce(ctx, ref, id); err != nil {
			return fmt.Errorf("workflowapp.Stage: attach-once %s: %w", ref, err)
		}
	}
	return nil
}

// Activate brings the workflow online: start listening on each entry trigger, then flip lifecycle to
// active. Idempotent (re-activating re-attaches). ErrNoTriggerEntry if the active graph has no trigger
// node to listen on (a manual-only workflow can only be :trigger-ed).
//
// Activate 让 workflow 上线：在每个入口 trigger 上开始监听，再把 lifecycle 翻 active。幂等（重激活=重挂）。
// active 图无 trigger 节点可监听 → ErrNoTriggerEntry（纯手动 workflow 只能 :trigger）。
func (s *Service) Activate(ctx context.Context, id string) (*workflowdomain.Workflow, error) {
	if s.binder == nil {
		return nil, errExecUnavailable
	}
	refs, err := s.entryTriggerRefs(ctx, id)
	if err != nil {
		return nil, err
	}
	for _, ref := range refs {
		if err := s.binder.Attach(ctx, ref, id); err != nil {
			return nil, fmt.Errorf("workflowapp.Activate: attach %s: %w", ref, err)
		}
	}
	return s.SetLifecycle(ctx, id, workflowdomain.LifecycleActive, workflowdomain.ActorUser)
}

// Deactivate takes the workflow offline gracefully: stop listening (Detach each entry trigger) and
// flip lifecycle to inactive — or draining if runs are still in flight, which the scheduler flips to
// inactive when the last one settles. In-flight runs are NOT killed (that is kill's job). Detach is
// best-effort over whatever entry refs resolve (a since-deleted graph still deactivates).
//
// Deactivate 优雅下线：停监听（Detach 每个入口 trigger）+ 翻 lifecycle 为 inactive——或仍有 run 在飞则 draining，
// 由调度器在最后一个结算时翻 inactive。在途 run **不杀**（那是 kill 的事）。Detach 对能解析的入口 ref 尽力而为
// （图已删也能下线）。
func (s *Service) Deactivate(ctx context.Context, id string) (*workflowdomain.Workflow, error) {
	if s.binder == nil || s.runner == nil {
		return nil, errExecUnavailable
	}
	if refs, err := s.entryTriggerRefs(ctx, id); err == nil {
		for _, ref := range refs {
			s.binder.Detach(ref, id)
		}
	}
	state := workflowdomain.LifecycleInactive
	if n, err := s.runner.CountRunning(ctx, id); err == nil && n > 0 {
		state = workflowdomain.LifecycleDraining
	}
	return s.SetLifecycle(ctx, id, state, workflowdomain.ActorUser)
}

// Kill hard-stops the workflow: stop listening (Detach), cancel every in-flight run (interrupting a
// run blocked mid-node), and flip lifecycle to inactive. Returns how many runs were killed.
//
// Kill 硬停 workflow：停监听（Detach）、取消所有在途 run（打断卡在节点中的 run）、翻 lifecycle 为 inactive。
// 返被杀 run 数。
func (s *Service) Kill(ctx context.Context, id string) (int, error) {
	if s.binder == nil || s.runner == nil {
		return 0, errExecUnavailable
	}
	if refs, err := s.entryTriggerRefs(ctx, id); err == nil {
		for _, ref := range refs {
			s.binder.Detach(ref, id)
		}
	}
	killed, err := s.runner.KillWorkflow(ctx, id)
	if err != nil {
		return 0, err
	}
	if _, err := s.SetLifecycle(ctx, id, workflowdomain.LifecycleInactive, workflowdomain.ActorUser); err != nil {
		return killed, err
	}
	return killed, nil
}

// MarkInactiveIfDrained flips a draining workflow to inactive — the scheduler's drain reconcile
// (called when a draining workflow's last in-flight run settles). Conditional + idempotent in the store.
//
// MarkInactiveIfDrained 把 draining 的 workflow 翻 inactive——调度器的排空 reconcile（draining workflow 最后
// 一个在途 run 结算时调）。store 层条件 + 幂等。
func (s *Service) MarkInactiveIfDrained(ctx context.Context, workflowID string) error {
	return s.repo.MarkInactiveIfDraining(ctx, workflowID)
}

// ReattachActive re-engages the trigger listener for every active workflow — called at boot, because
// the listen registry is in-memory and empty after a restart (this is the "replay every active
// reference on boot" the trigger lifecycle expects). Per-workflow failures are logged and skipped.
// No-op without a binder. Workspace-scoped by ctx.
//
// ReattachActive 为每个 active workflow 重挂 trigger 监听——boot 时调，因监听注册表是内存的、重启后为空
// （即 trigger 生命周期期待的「boot 重放每个 active 引用」）。逐 workflow 失败记日志跳过。无 binder 则 no-op。
// 按 ctx 的 workspace 隔离。
func (s *Service) ReattachActive(ctx context.Context) error {
	if s.binder == nil {
		return nil
	}
	active, err := s.repo.ListActiveWorkflows(ctx)
	if err != nil {
		return fmt.Errorf("workflowapp.ReattachActive: %w", err)
	}
	for _, w := range active {
		refs, err := s.entryTriggerRefsOf(ctx, w)
		if err != nil {
			s.log.Warn("workflowapp.ReattachActive: skip", zap.String("workflow", w.ID), zap.Error(err))
			continue
		}
		for _, ref := range refs {
			if err := s.binder.Attach(ctx, ref, w.ID); err != nil {
				s.log.Warn("workflowapp.ReattachActive: attach", zap.String("workflow", w.ID), zap.String("trigger", ref), zap.Error(err))
			}
		}
	}
	return nil
}

// entryTriggerRefs loads the workflow and returns its entry trigger entity refs (the trg_ each
// listener binds to). ErrNotFound (workflow), ErrNoActiveVersion (no graph), or ErrNoTriggerEntry
// (graph has no trigger node) bubble up.
//
// entryTriggerRefs 载入 workflow 并返其入口 trigger 实体 ref（每个 listener 绑的 trg_）。ErrNotFound
// （workflow）、ErrNoActiveVersion（无图）、ErrNoTriggerEntry（图无 trigger 节点）冒泡。
func (s *Service) entryTriggerRefs(ctx context.Context, id string) ([]string, error) {
	w, err := s.repo.GetWorkflow(ctx, id)
	if err != nil {
		return nil, err
	}
	return s.entryTriggerRefsOf(ctx, w)
}

// entryTriggerRefsOf collects the deduped entity refs of a workflow's trigger nodes from its active
// graph. Empty (no active version / no trigger node) is an error, not a silent no-op — :activate /
// :stage on an unlistenable workflow must tell the caller why.
//
// entryTriggerRefsOf 从 workflow 的 active 图收集其 trigger 节点的去重实体 ref。空（无 active 版本 / 无 trigger
// 节点）是错误、非静默 no-op——对不可监听的 workflow 调 :activate / :stage 须告知调用方原因。
func (s *Service) entryTriggerRefsOf(ctx context.Context, w *workflowdomain.Workflow) ([]string, error) {
	if w.ActiveVersionID == "" {
		return nil, workflowdomain.ErrNoActiveVersion
	}
	g, err := s.activeGraph(ctx, w)
	if err != nil {
		return nil, err
	}
	var refs []string
	seen := map[string]bool{}
	for i := range g.Nodes {
		n := &g.Nodes[i]
		if n.Kind == workflowdomain.NodeKindTrigger && n.Ref != "" && !seen[n.Ref] {
			seen[n.Ref] = true
			refs = append(refs, n.Ref)
		}
	}
	if len(refs) == 0 {
		return nil, workflowdomain.ErrNoTriggerEntry
	}
	return refs, nil
}
