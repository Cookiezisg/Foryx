package scheduler

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

// RehydrateOnBoot scans paused FlowRuns and re-registers cancel handles
// for Service.Cancel. Caller MUST pass a non-empty userID — typically by
// iterating userService.List at boot (see cmd/server/main.go). No
// magic-id fallback.
//
// RehydrateOnBoot 扫 paused FlowRun 并重注册 cancel 句柄。
// 调用方必须传非空 userID(主 main.go 会遍历 users.List 调用)。
func (s *Service) RehydrateOnBoot(ctx context.Context, userID string) error {
	if userID == "" {
		return fmt.Errorf("schedulerapp.RehydrateOnBoot: %w", reqctxpkg.ErrMissingUserID)
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
