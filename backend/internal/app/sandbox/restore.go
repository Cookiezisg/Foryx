package sandbox

import (
	"context"
	"os"
	"runtime"
	"syscall"

	"go.uber.org/zap"
)

// RestoreOrCleanupOnBoot kills survivor PIDs from prior runs; best-effort.
//
// RestoreOrCleanupOnBoot 杀掉上次运行残留的 PID 并清 manifest，best-effort。
func (s *Service) RestoreOrCleanupOnBoot(ctx context.Context) {
	envs, err := s.repo.ListEnvsWithRunningPID(ctx)
	if err != nil {
		s.log.Warn("sandbox boot scan: list envs with running pid failed (skipping cleanup)",
			zap.Error(err))
		return
	}
	if len(envs) == 0 {
		return
	}

	killed, alreadyDead := 0, 0
	for _, e := range envs {
		alive := killIfAlive(e.RunningPID)
		if alive {
			killed++
			s.log.Info("sandbox boot scan: killed stale process",
				zap.String("env_id", e.ID),
				zap.String("owner_kind", e.OwnerKind),
				zap.String("owner_id", e.OwnerID),
				zap.Int("pid", e.RunningPID))
		} else {
			alreadyDead++
		}
		if err := s.repo.ClearEnvRunningPID(ctx, e.ID); err != nil {
			s.log.Warn("sandbox boot scan: clear running_pid failed",
				zap.String("env_id", e.ID),
				zap.Error(err))
		}
	}
	s.log.Info("sandbox boot scan complete",
		zap.Int("scanned", len(envs)),
		zap.Int("killed", killed),
		zap.Int("already_dead", alreadyDead))
}

func killIfAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	if runtime.GOOS != "windows" {
		if err := p.Signal(syscall.Signal(0)); err != nil {
			return false
		}
	}
	if err := p.Kill(); err != nil {
		return false
	}
	return true
}
