package sandbox

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"

	sandboxdomain "github.com/sunweilin/forgify/backend/internal/domain/sandbox"
)

func echoBin(t *testing.T, msg string) (string, []string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("echo test depends on unix shell semantics; D14 Windows pipeline covers spawn behaviour separately")
	}
	bin, err := exec.LookPath("echo")
	if err != nil {
		t.Fatalf("look up echo: %v", err)
	}
	return bin, []string{msg}
}

func TestSpawnOnce_HappyPath(t *testing.T) {
	bin, args := echoBin(t, "hello sandbox")
	res, err := SpawnOnce(context.Background(), SpawnOptions{
		Cmd:  bin,
		Args: args,
	})
	if err != nil {
		t.Fatalf("SpawnOnce: %v", err)
	}
	if !res.Ok {
		t.Errorf("Ok = false (exit %d, stderr %q)", res.ExitCode, res.Stderr)
	}
	if got := strings.TrimSpace(string(res.Stdout)); got != "hello sandbox" {
		t.Errorf("stdout = %q, want %q", got, "hello sandbox")
	}
	if res.Duration <= 0 {
		t.Errorf("Duration = %v, want > 0", res.Duration)
	}
}

func TestSpawnOnce_NonZeroExit_ReturnsOkFalseNotError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses 'false' command")
	}
	bin, err := exec.LookPath("false")
	if err != nil {
		t.Fatalf("look up false: %v", err)
	}
	res, err := SpawnOnce(context.Background(), SpawnOptions{Cmd: bin})
	if err != nil {
		t.Fatalf("SpawnOnce returned Go error for non-zero exit: %v", err)
	}
	if res.Ok {
		t.Error("Ok = true for non-zero exit; want false")
	}
	if res.ExitCode == 0 {
		t.Error("ExitCode = 0; want non-zero")
	}
}

func TestSpawnOnce_StartFailure_AbsoluteMissingWrapsErrEnvNotFound(t *testing.T) {
	// §11.5 pre-§14: absolute nonexistent → ErrSpawnFailed (cmd.Start ENOENT).
	// §11.5 post: absolute nonexistent → ErrEnvNotFound (pre-check) so app
	// layer can lazy-rebuild. Result is nil pre-spawn (didn't reach exec).
	//
	// §11.5 之前:绝对路径不存在 → cmd.Start ENOENT → ErrSpawnFailed。
	// §11.5 之后:预检拦下,返 ErrEnvNotFound 给 app 层 lazy rebuild。
	// 此时 result=nil(还没到 exec)。
	res, err := SpawnOnce(context.Background(), SpawnOptions{
		Cmd: "/nonexistent/binary/path",
	})
	if err == nil {
		t.Fatal("want error for nonexistent binary, got nil")
	}
	if !errors.Is(err, sandboxdomain.ErrEnvNotFound) {
		t.Errorf("err must wrap ErrEnvNotFound (§11.5 pre-check), got %v", err)
	}
	if res != nil {
		t.Errorf("result should be nil when pre-check fails before exec; got %+v", res)
	}
}

func TestSpawnOnce_StdinPiped(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses 'cat' command")
	}
	bin, err := exec.LookPath("cat")
	if err != nil {
		t.Fatalf("look up cat: %v", err)
	}
	const payload = "stdin payload\n"
	res, err := SpawnOnce(context.Background(), SpawnOptions{
		Cmd:   bin,
		Stdin: []byte(payload),
	})
	if err != nil {
		t.Fatalf("SpawnOnce: %v", err)
	}
	if got := string(res.Stdout); got != payload {
		t.Errorf("cat stdout = %q, want %q", got, payload)
	}
}

func TestSpawnOnce_CtxCancelKillsProcess(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses 'sleep' command")
	}
	bin, err := exec.LookPath("sleep")
	if err != nil {
		t.Fatalf("look up sleep: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, _ = SpawnOnce(ctx, SpawnOptions{
		Cmd:  bin,
		Args: []string{"30"},
	})
	elapsed := time.Since(start)

	if elapsed > 5*time.Second {
		t.Errorf("ctx-cancel did not kill subprocess: elapsed %v", elapsed)
	}
}

func TestSpawnLongLived_StdinStdoutEcho(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses 'cat' command")
	}
	bin, err := exec.LookPath("cat")
	if err != nil {
		t.Fatalf("look up cat: %v", err)
	}
	handle, err := SpawnLongLived(context.Background(), SpawnOptions{Cmd: bin})
	if err != nil {
		t.Fatalf("SpawnLongLived: %v", err)
	}

	if handle.PID() == 0 {
		t.Error("PID = 0 after Start; want non-zero")
	}

	go func() {
		_, _ = handle.Stdin().Write([]byte("ping\n"))
		_ = handle.Stdin().Close()
	}()

	out, err := io.ReadAll(handle.Stdout())
	if err != nil {
		t.Fatalf("read stdout: %v", err)
	}
	if !strings.HasPrefix(string(out), "ping") {
		t.Errorf("stdout = %q, want prefix 'ping'", out)
	}

	// cat exits when stdin closes; Wait reaps.
	// cat 在 stdin 关时退出；Wait reap。
	if err := handle.Wait(); err != nil {
		t.Errorf("Wait: %v", err)
	}
}

func TestSpawnLongLived_KillTerminates(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses 'sleep' command")
	}
	bin, err := exec.LookPath("sleep")
	if err != nil {
		t.Fatalf("look up sleep: %v", err)
	}
	handle, err := SpawnLongLived(context.Background(), SpawnOptions{
		Cmd:  bin,
		Args: []string{"30"},
	})
	if err != nil {
		t.Fatalf("SpawnLongLived: %v", err)
	}

	if err := handle.Kill(); err != nil {
		t.Errorf("Kill: %v", err)
	}

	done := make(chan error, 1)
	go func() { done <- handle.Wait() }()

	select {
	case <-done:
		// Wait returned within timeout — Kill worked.
		// Wait 在 timeout 内返——Kill 工作。
	case <-time.After(5 * time.Second):
		t.Error("Wait did not return after Kill within 5s")
	}
}

func TestSpawnLongLived_StartFailure(t *testing.T) {
	// §11.5: absolute path pre-check classifies missing binary as
	// ErrEnvNotFound (lazy-rebuild trigger) — was ErrSpawnFailed pre-§11.5.
	//
	// §11.5:绝对路径预检把缺失 binary 归为 ErrEnvNotFound(触发 lazy rebuild),
	// §11.5 之前是 ErrSpawnFailed。
	_, err := SpawnLongLived(context.Background(), SpawnOptions{
		Cmd: "/nonexistent/binary/path",
	})
	if err == nil {
		t.Fatal("want error for nonexistent binary, got nil")
	}
	if !errors.Is(err, sandboxdomain.ErrEnvNotFound) {
		t.Errorf("err must wrap ErrEnvNotFound (§11.5 pre-check), got %v", err)
	}
}

// TestSpawnOnce_DanglingSymlink — §11.5 core scenario: venv .venv/bin/python
// is a symlink to a runtime that mise relocated. os.Stat follows the link
// and returns ENOENT; pre-check surfaces ErrEnvNotFound so the app-layer
// lazy rebuild kicks in.
//
// TestSpawnOnce_DanglingSymlink —— §11.5 核心场景:venv 内 python 是
// 指向已被 mise 重定位的 runtime 的 symlink。os.Stat 跟 link 返 ENOENT,
// 预检上抛 ErrEnvNotFound 让 app 层 lazy rebuild 接管。
func TestSpawnOnce_DanglingSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink semantics differ on Windows; production lazy rebuild path covered by integration tests")
	}
	tmp := t.TempDir()
	link := tmp + "/dangling-python"
	if err := os.Symlink("/does/not/exist/python", link); err != nil {
		t.Fatalf("create dangling symlink: %v", err)
	}

	_, err := SpawnOnce(context.Background(), SpawnOptions{Cmd: link})
	if err == nil {
		t.Fatal("expected error spawning via dangling symlink, got nil")
	}
	if !errors.Is(err, sandboxdomain.ErrEnvNotFound) {
		t.Errorf("dangling symlink should wrap ErrEnvNotFound (§11.5), got %v", err)
	}
}

// TestCheckBinaryExists_PathResolvedSkipped — bare command names go through
// $PATH lookup; pre-check skips them so they reach exec normally.
//
// TestCheckBinaryExists_PathResolvedSkipped —— 裸命令走 $PATH 解析,
// 预检跳过让 exec 正常处理。
func TestCheckBinaryExists_PathResolvedSkipped(t *testing.T) {
	for _, cmd := range []string{"", "echo", "./script.sh", "../relative"} {
		if err := checkBinaryExists(cmd); err != nil {
			t.Errorf("checkBinaryExists(%q) should pass through, got %v", cmd, err)
		}
	}
}
