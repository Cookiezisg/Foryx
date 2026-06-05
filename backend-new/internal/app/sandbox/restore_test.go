package sandbox

import (
	"context"
	"os/exec"
	"runtime"
	"testing"
)

// TestRestoreOnBoot_KillsSurvivorAndClearsManifest: a long-lived process recorded
// in the manifest from a prior run is killed and its running_pid cleared.
//
// manifest 里上次运行记录的长生命周期进程被杀掉、running_pid 被清。
func TestRestoreOnBoot_KillsSurvivorAndClearsManifest(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sleep + POSIX liveness probe")
	}
	svc, _ := newServiceWithEnv(t, "fake-py")
	ctx := context.Background()

	// Start a real survivor and record its PID in the manifest.
	// 起一个真的残留进程，把其 PID 记进 manifest。
	cmd := exec.Command("sleep", "30")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start sleep: %v", err)
	}
	if err := svc.repo.SetEnvRunningPID(ctx, "se_test", cmd.Process.Pid); err != nil {
		t.Fatalf("set pid: %v", err)
	}

	svc.RestoreOrCleanupOnBoot(ctx)

	// If the survivor was SIGKILL'd, Wait returns promptly with an error. Had the
	// boot scan failed to kill it, Wait would block until sleep exits and return
	// nil — so a prompt non-nil error confirms the kill.
	// 残留被 SIGKILL 则 Wait 立即返错；若没杀则阻塞到 sleep 结束（返回 nil）——故立即
	// 返回非 nil 错误即确认杀成功。
	if err := cmd.Wait(); err == nil {
		t.Error("survivor exited cleanly — boot scan did not kill it")
	}

	live, err := svc.repo.ListEnvsWithRunningPID(ctx)
	if err != nil {
		t.Fatalf("list running: %v", err)
	}
	if len(live) != 0 {
		t.Errorf("running_pid not cleared after boot scan: %+v", live)
	}
}
