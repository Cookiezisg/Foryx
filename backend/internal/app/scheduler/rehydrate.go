package scheduler

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

// RehydrateOnBoot scans paused FlowRuns and re-registers cancel handles for Service.Cancel.
//
// RehydrateOnBoot 扫 paused FlowRun 并重新注册 cancel 句柄。
func (s *Service) RehydrateOnBoot(ctx context.Context, userID string) error {
	if userID == "" {
		userID = reqctxpkg.DefaultLocalUserID
	}
	scopedCtx := reqctxpkg.SetUserID(ctx, userID)
	rows, err := s.repo.ListPaused(scopedCtx)
	if err != nil {
		return fmt.Errorf("schedulerapp.RehydrateOnBoot: %w", err)
	}
	s.log.Info("rehydrating paused flowruns", zap.Int("count", len(rows)))
	for _, run := range rows {
		s.cancelsMu.Lock()
		s.cancels[run.ID] = func() {
			s.log.Info("cancel called on pre-resume paused run",
				zap.String("runID", run.ID))
		}
		s.cancelsMu.Unlock()
	}
	return nil
}
