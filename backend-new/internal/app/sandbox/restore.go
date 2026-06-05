package sandbox

import (
	"context"
	"os"
	"runtime"
	"syscall"

	"go.uber.org/zap"
)

// RestoreOrCleanupOnBoot kills survivor PIDs recorded in the manifest from a
// prior run (a long-lived process that outlived a backend crash); best-effort.
//
// RestoreOrCleanupOnBoot 杀掉 manifest 里上次运行记录的残留 PID（熬过后端崩溃的长生命
// 周期进程）；best-effort。
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
		if killIfAlive(e.RunningPID) {
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

// killIfAlive reports whether pid was alive (and kills it). Signal(0) probes
// liveness without killing; on success Kill terminates the survivor.
//
// killIfAlive 报告 pid 是否存活（并杀掉）。Signal(0) 探活不杀；存活则 Kill 终结残留。
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
