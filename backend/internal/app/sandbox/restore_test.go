package sandbox

import (
	"context"
	"os/exec"
	"runtime"
	"syscall"
	"testing"
	"time"
)

func TestRestoreOrCleanupOnBoot_KillsStaleProcessAndClearsPID(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sleep + posix signal probe")
	}
	svc, owner := newServiceWithEnv(t, "fake-py")
	ctx := context.Background()

	sleepBin, err := exec.LookPath("sleep")
	if err != nil {
		t.Fatalf("look up sleep: %v", err)
	}
	cmd := exec.Command(sleepBin, "30")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start sleep: %v", err)
	}
	stalePID := cmd.Process.Pid

	envRow, err := svc.repo.FindEnvByOwner(ctx, owner.Kind, owner.ID)
	if err != nil {
		t.Fatalf("find env: %v", err)
	}
	if err := svc.repo.SetEnvRunningPID(ctx, envRow.ID, stalePID); err != nil {
		t.Fatalf("seed running PID: %v", err)
	}

	if err := cmd.Process.Signal(syscall.Signal(0)); err != nil {
		t.Fatalf("seeded process not alive before scan: %v", err)
	}

	svc.RestoreOrCleanupOnBoot(ctx)

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Errorf("stale process %d not reapable 2s after boot scan (kill failed)", stalePID)
		_ = cmd.Process.Kill()
		<-done
	}

	envRow, err = svc.repo.FindEnvByOwner(ctx, owner.Kind, owner.ID)
	if err != nil {
		t.Fatalf("re-find env: %v", err)
	}
	if envRow.RunningPID != 0 {
		t.Errorf("running_pid not cleared: got %d, want 0", envRow.RunningPID)
	}
}

func TestRestoreOrCleanupOnBoot_NoOpWhenNoStalePIDs(t *testing.T) {
	svc, _ := newServiceWithEnv(t, "fake-py")
	svc.RestoreOrCleanupOnBoot(context.Background())
}

func TestRestoreOrCleanupOnBoot_HandlesAlreadyDeadPID(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses /bin/true + posix signal probe")
	}
	svc, owner := newServiceWithEnv(t, "fake-py")
	ctx := context.Background()

	trueBin, err := exec.LookPath("true")
	if err != nil {
		t.Fatalf("look up true: %v", err)
	}
	cmd := exec.Command(trueBin)
	if err := cmd.Run(); err != nil {
		t.Fatalf("run true: %v", err)
	}
	deadPID := cmd.Process.Pid

	envRow, err := svc.repo.FindEnvByOwner(ctx, owner.Kind, owner.ID)
	if err != nil {
		t.Fatalf("find env: %v", err)
	}
	if err := svc.repo.SetEnvRunningPID(ctx, envRow.ID, deadPID); err != nil {
		t.Fatalf("seed dead PID: %v", err)
	}

	svc.RestoreOrCleanupOnBoot(ctx)
	envRow, _ = svc.repo.FindEnvByOwner(ctx, owner.Kind, owner.ID)
	if envRow.RunningPID != 0 {
		t.Errorf("dead PID column not cleared: got %d, want 0", envRow.RunningPID)
	}
}
