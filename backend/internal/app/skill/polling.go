package skill

import (
	"context"
	"time"

	"go.uber.org/zap"
)

const pollInterval = 1 * time.Second

// Start runs a synchronous Scan then launches the polling goroutine.
//
// Start 先同步 Scan 一次，再启动 polling goroutine。
func (s *Service) Start(ctx context.Context) error {
	if err := s.Scan(ctx); err != nil {
		s.log.Warn("skill initial scan failed (continuing with empty cache)",
			zap.Error(err))
	}

	pollCtx, pollCancel := context.WithCancel(ctx)
	s.stopCancel = pollCancel
	s.pollDone = make(chan struct{})
	go func() {
		defer close(s.pollDone)
		s.pollLoop(pollCtx)
	}()
	return nil
}

// Stop cancels the polling goroutine and blocks until exit; idempotent.
//
// Stop 取消 polling goroutine 并阻塞到退出，幂等。
func (s *Service) Stop() {
	s.stopOnce.Do(func() {
		if s.stopCancel != nil {
			s.stopCancel()
		}
		if s.pollDone != nil {
			<-s.pollDone
		}
	})
}

func (s *Service) pollLoop(ctx context.Context) {
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.Scan(ctx); err != nil {
				s.log.Warn("skill rescan failed", zap.Error(err))
			}
		}
	}
}
