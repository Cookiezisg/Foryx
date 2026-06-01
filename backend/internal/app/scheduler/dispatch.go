package scheduler

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"

	flowrundomain "github.com/sunweilin/forgify/backend/internal/domain/flowrun"
	triggerdomain "github.com/sunweilin/forgify/backend/internal/domain/trigger"
	workflowdomain "github.com/sunweilin/forgify/backend/internal/domain/workflow"
	idgenpkg "github.com/sunweilin/forgify/backend/internal/pkg/idgen"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

// SetFiringInbox wires the durable trigger-firings inbox (M5 dispatch; 17 §6, ADR-021).
func (s *Service) SetFiringInbox(inbox triggerdomain.FiringInbox) { s.firingInbox = inbox }

// buildRun validates the workflow and builds the FlowRun struct WITHOUT writing it. Shared by the
// direct StartRun path (writes via s.repo.Create) and the durable dispatch path (writes inside the
// single-tx claim). Returns the run, its pinned graph, and the run-level timeout seconds.
//
// buildRun 校验 workflow 并构造 FlowRun 结构(不落库);直接路径与单事务派发路径共用。
func (s *Service) buildRun(ctx context.Context, workflowID, triggerKind string, input map[string]any, dryRun bool) (*flowrundomain.FlowRun, *workflowdomain.Graph, int, error) {
	uid, err := reqctxpkg.RequireUserID(ctx)
	if err != nil {
		return nil, nil, 0, fmt.Errorf("schedulerapp.buildRun: %w", err)
	}
	wf, err := s.workflowRead.GetWorkflow(ctx, workflowID)
	if err != nil {
		if errors.Is(err, workflowdomain.ErrNotFound) {
			return nil, nil, 0, fmt.Errorf("schedulerapp.buildRun: %w", ErrWorkflowNotFound)
		}
		return nil, nil, 0, fmt.Errorf("schedulerapp.buildRun: GetWorkflow: %w", err)
	}
	if !wf.Enabled {
		return nil, nil, 0, fmt.Errorf("schedulerapp.buildRun: %w", ErrWorkflowDisabled)
	}
	if wf.NeedsAttention {
		return nil, nil, 0, fmt.Errorf("schedulerapp.buildRun: %w", ErrWorkflowNeedsAttention)
	}
	if wf.Concurrency == workflowdomain.ConcurrencySerial {
		running, cErr := s.repo.CountRunning(ctx, workflowID)
		if cErr != nil {
			return nil, nil, 0, fmt.Errorf("schedulerapp.buildRun: CountRunning: %w", cErr)
		}
		if running >= 1 {
			return nil, nil, 0, fmt.Errorf("schedulerapp.buildRun: %w", ErrConcurrencyLimit)
		}
	}
	version, err := s.workflowRead.GetActiveVersion(ctx, workflowID)
	if err != nil {
		return nil, nil, 0, fmt.Errorf("schedulerapp.buildRun: GetActiveVersion: %w", err)
	}
	run := &flowrundomain.FlowRun{
		ID:           idgenpkg.New("fr"),
		UserID:       uid,
		WorkflowID:   workflowID,
		VersionID:    version.ID,
		TriggerKind:  triggerKind,
		TriggerInput: input,
		Status:       flowrundomain.StatusRunning,
		StartedAt:    time.Now().UTC(),
		DryRun:       dryRun,
	}
	return run, version.GraphParsed, wf.TimeoutSec, nil
}

// spawnRun sets up the detached run ctx (timeout/cancel) + cancels map and launches executeRun.
//
// spawnRun 建 detached run ctx(timeout/cancel)+ 注册 cancel,起 goroutine 跑 executeRun。
func (s *Service) spawnRun(run *flowrundomain.FlowRun, graph *workflowdomain.Graph, timeoutSec int) {
	runCtx := reqctxpkg.SetUserID(context.Background(), run.UserID)
	var cancel context.CancelFunc
	if timeoutSec > 0 {
		runCtx, cancel = context.WithTimeout(runCtx, time.Duration(timeoutSec)*time.Second)
	} else {
		runCtx, cancel = context.WithCancel(runCtx)
	}
	s.cancelsMu.Lock()
	s.cancels[run.ID] = cancel
	s.cancelsMu.Unlock()
	s.runWG.Add(1)
	go func() {
		defer s.runWG.Done()
		defer s.releaseCancel(run.ID)
		defer func() {
			if r := recover(); r != nil {
				s.log.Error("scheduler.executeRun panic", zap.String("runID", run.ID), zap.Any("recover", r))
				_ = s.repo.UpdateStatus(runCtx, run.ID, flowrundomain.StatusFailed,
					nil, "INTERNAL_PANIC", fmt.Sprintf("%v", r), ptrNow(), 0)
			}
		}()
		s.ExecuteFn(runCtx, run, graph)
	}()
}

// DispatchPending drains pending firings via the single-tx claim (ADR-021): for each firing, build
// the run (validation), then claim (pending→claimed) + INSERT the flowrun in ONE tx, then spawn
// execution post-commit. A firing whose workflow is gone/disabled/at-limit is shed (terminal, never
// lost); a claim lost to a concurrent dispatcher is skipped. Crash-safe: a firing left pending is
// re-drained on the next call (boot catchup); a committed claim leaves status=started + the flowrun.
//
// DispatchPending 单事务认领 pending firing → 建 flowrun → 起执行;崩溃安全(pending 下次重排,无重复)。
func (s *Service) DispatchPending(ctx context.Context) {
	if s.firingInbox == nil {
		return
	}
	firings, err := s.firingInbox.ListPending(ctx, 50)
	if err != nil {
		s.log.Error("scheduler.DispatchPending: list", zap.Error(err))
		return
	}
	for i := range firings {
		f := firings[i]
		run, graph, timeoutSec, bErr := s.buildRun(ctx, f.WorkflowID, f.TriggerKind, f.Payload, false)
		if bErr != nil {
			if mErr := s.firingInbox.MarkOutcome(ctx, f.ID, triggerdomain.FiringShed); mErr != nil {
				s.log.Error("scheduler.DispatchPending: shed", zap.String("firingID", f.ID), zap.Error(mErr))
			}
			s.log.Warn("scheduler.DispatchPending: shed firing (workflow gone/disabled/at-limit)",
				zap.String("firingID", f.ID), zap.Error(bErr))
			continue
		}
		// Single-tx claim: claim the firing + create the flowrun atomically (ADR-021).
		_, cErr := s.firingInbox.ClaimFiring(ctx, f.ID, func(tx *gorm.DB) (string, error) {
			if cErr := tx.Create(run).Error; cErr != nil {
				return "", cErr
			}
			return run.ID, nil
		})
		if cErr != nil {
			if errors.Is(cErr, triggerdomain.ErrFiringNotPending) {
				continue // another claimant won the race
			}
			s.log.Error("scheduler.DispatchPending: claim", zap.String("firingID", f.ID), zap.Error(cErr))
			continue
		}
		s.spawnRun(run, graph, timeoutSec)
		s.publish(ctx, run.ID, f.WorkflowID, "started", map[string]any{"triggerKind": f.TriggerKind})
	}
}

// OnTriggerFired persists a firing (durable, persist-before-act) then drains the inbox via the
// single-tx claim (ADR-021). Implements triggerapp.SchedulerStarter's durable path. If no inbox is
// wired it falls back to the direct StartRun so triggers still run.
//
// OnTriggerFired 持久化 firing(先持久化再动作)然后单事务派发;无 inbox 时回退到直接 StartRun。
func (s *Service) OnTriggerFired(ctx context.Context, firing *triggerdomain.TriggerFiring) error {
	if s.firingInbox == nil {
		if _, err := s.StartRun(ctx, firing.WorkflowID, firing.TriggerKind, firing.Payload); err != nil {
			return fmt.Errorf("schedulerapp.OnTriggerFired: direct: %w", err)
		}
		return nil
	}
	if _, err := s.firingInbox.AppendFiring(ctx, firing); err != nil {
		return fmt.Errorf("schedulerapp.OnTriggerFired: append: %w", err)
	}
	s.DispatchPending(ctx)
	return nil
}
