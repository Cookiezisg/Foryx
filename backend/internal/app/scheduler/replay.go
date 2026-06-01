package scheduler

import (
	"context"
	"errors"
	"fmt"

	"go.uber.org/zap"

	flowrundomain "github.com/sunweilin/forgify/backend/internal/domain/flowrun"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

// ErrNotReplayable means the flowrun is not in a replayable (terminal-failed) state.
var ErrNotReplayable = errors.New("schedulerapp: flowrun not replayable")

// ReplayRun re-runs a failed flowrun under a fresh generation (ADR-019 `:replay`): validate the run
// is terminal-failed, bump generation + flip running, journal replay_started, then re-drive the
// interpreter — which copy-hits the still-succeeded steps (their node_completed survives) and re-runs
// the failed one (+ everything downstream) at the new generation. The journal is the truth; the prior
// generation's events stay for audit, distinguished by `generation`.
//
// ReplayRun 在新一代下重跑失败的 flowrun(ADR-019):成功步抄、失败步及其下游在新代重跑。
func (s *Service) ReplayRun(ctx context.Context, runID string) error {
	run, err := s.repo.Get(ctx, runID)
	if err != nil {
		return fmt.Errorf("schedulerapp.ReplayRun: %w", err)
	}
	if run.Status != flowrundomain.StatusFailed {
		return fmt.Errorf("schedulerapp.ReplayRun: %w (status=%s)", ErrNotReplayable, run.Status)
	}
	graph, err := s.loadFrozenGraph(ctx, run)
	if err != nil {
		return fmt.Errorf("schedulerapp.ReplayRun: load graph: %w", err)
	}
	newGen, err := s.repo.BumpGeneration(ctx, runID)
	if err != nil {
		return fmt.Errorf("schedulerapp.ReplayRun: bump: %w", err)
	}
	run.Generation = newGen
	run.Status = flowrundomain.StatusRunning
	if s.journal != nil {
		if _, jErr := s.journal.AppendEvent(ctx, &flowrundomain.FlowRunEvent{
			FlowrunID: runID, Type: flowrundomain.EventReplayStarted, Generation: newGen,
		}); jErr != nil {
			s.log.Warn("ReplayRun: journal replay_started failed", zap.String("runID", runID), zap.Error(jErr))
		}
	}
	s.publish(ctx, runID, run.WorkflowID, "replaying", map[string]any{"generation": newGen})

	runCtx := reqctxpkg.SetUserID(context.Background(), run.UserID)
	runCtx, cancel := context.WithCancel(runCtx)
	s.cancelsMu.Lock()
	s.cancels[runID] = cancel
	s.cancelsMu.Unlock()
	s.runWG.Add(1)
	go func() {
		defer s.runWG.Done()
		defer s.releaseCancel(runID)
		defer func() {
			if r := recover(); r != nil {
				s.log.Error("ReplayRun: continuation panic", zap.String("runID", runID), zap.Any("recover", r))
			}
		}()
		s.ExecuteFn(runCtx, run, graph)
	}()
	return nil
}
