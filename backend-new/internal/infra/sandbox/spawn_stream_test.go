package sandbox

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

// TestSpawnOnce_TeesStderrToStreamErr: stderr is captured in the result AND teed live to StreamErr
// (the seam run_function uses to stream a function's print() output). stdout stays untouched.
//
// TestSpawnOnce_TeesStderrToStreamErr：stderr 既进结果、又实时 tee 到 StreamErr（run_function 据此流式推
// 函数 print() 输出的接缝）。stdout 不受影响。
func TestSpawnOnce_TeesStderrToStreamErr(t *testing.T) {
	var live bytes.Buffer
	res, err := SpawnOnce(context.Background(), SpawnOptions{
		Cmd:       "sh",
		Args:      []string{"-c", "echo to-stdout; echo to-stderr 1>&2"},
		StreamErr: &live,
	})
	if err != nil {
		t.Fatalf("SpawnOnce: %v", err)
	}
	if !strings.Contains(live.String(), "to-stderr") {
		t.Fatalf("StreamErr did not receive stderr live: %q", live.String())
	}
	if !strings.Contains(string(res.Stderr), "to-stderr") {
		t.Fatalf("result stderr lost the tee: %q", res.Stderr)
	}
	if !strings.Contains(string(res.Stdout), "to-stdout") || strings.Contains(live.String(), "to-stdout") {
		t.Fatalf("stdout leaked into StreamErr or was lost: out=%q live=%q", res.Stdout, live.String())
	}
}

// TestSpawnOnce_NilStreamErrCaptures: with no StreamErr the behaviour is unchanged — stderr is just
// captured.
//
// TestSpawnOnce_NilStreamErrCaptures：无 StreamErr 时行为不变——stderr 仅被捕获。
func TestSpawnOnce_NilStreamErrCaptures(t *testing.T) {
	res, err := SpawnOnce(context.Background(), SpawnOptions{Cmd: "sh", Args: []string{"-c", "echo e 1>&2"}})
	if err != nil || !strings.Contains(string(res.Stderr), "e") {
		t.Fatalf("nil StreamErr should still capture stderr: err=%v stderr=%q", err, res.Stderr)
	}
}
