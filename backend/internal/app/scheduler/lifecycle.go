package scheduler

import (
	"context"
	"time"

	"go.uber.org/zap"
)

// Drain waits for in-flight run goroutines to finish, up to ctx's deadline (graceful shutdown, M6).
// On deadline it cancels all in-flight runs (so their executeRun ctx.Err path writes a terminal
// status) and waits a final moment — so a clean stop doesn't leave `running` zombies for the next
// boot's reconciliation to mop up.
//
// Drain 等 in-flight run goroutine 结束(到 ctx deadline);超时则取消全部(写终态)再等片刻,优雅关停。
func (s *Service) Drain(ctx context.Context) {
	done := make(chan struct{})
	go func() {
		s.runWG.Wait()
		close(done)
	}()
	select {
	case <-done:
		s.log.Info("scheduler.Drain: all in-flight runs finished")
		return
	case <-ctx.Done():
	}
	// Deadline hit: cancel in-flight runs so they finalize (cancelled/timeout), then wait briefly.
	s.cancelsMu.Lock()
	n := len(s.cancels)
	for _, cancel := range s.cancels {
		cancel()
	}
	s.cancelsMu.Unlock()
	s.log.Warn("scheduler.Drain: deadline — cancelled in-flight runs", zap.Int("count", n))
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		s.log.Warn("scheduler.Drain: some runs did not finalize within the grace period")
	}
}
