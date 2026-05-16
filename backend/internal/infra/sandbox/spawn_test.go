package sandbox

import (
	"context"
	"errors"
	"io"
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

func TestSpawnOnce_StartFailure_WrapsErrSpawnFailed(t *testing.T) {
	res, err := SpawnOnce(context.Background(), SpawnOptions{
		Cmd: "/nonexistent/binary/path",
	})
	if err == nil {
		t.Fatal("want error for nonexistent binary, got nil")
	}
	if !errors.Is(err, sandboxdomain.ErrSpawnFailed) {
		t.Errorf("err must wrap ErrSpawnFailed, got %v", err)
	}
	if res == nil {
		t.Error("result should be non-nil even on infrastructure failure")
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
	_, err := SpawnLongLived(context.Background(), SpawnOptions{
		Cmd: "/nonexistent/binary/path",
	})
	if err == nil {
		t.Fatal("want error for nonexistent binary, got nil")
	}
	if !errors.Is(err, sandboxdomain.ErrSpawnFailed) {
		t.Errorf("err must wrap ErrSpawnFailed, got %v", err)
	}
}
