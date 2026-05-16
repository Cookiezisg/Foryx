package handler

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeDriver struct {
	t          *testing.T
	clientIn   *io.PipeWriter
	clientInR  *io.PipeReader
	clientOut  *io.PipeReader
	clientOutW *io.PipeWriter
	driverIn   *bufio.Reader
}

func newFakeDriver(t *testing.T) (Client, *fakeDriver) {
	t.Helper()
	pr1, pw1 := io.Pipe()
	pr2, pw2 := io.Pipe()

	fd := &fakeDriver{
		t:          t,
		clientIn:   pw1,
		clientInR:  pr1,
		clientOut:  pr2,
		clientOutW: pw2,
		driverIn:   bufio.NewReader(pr1),
	}
	t.Cleanup(func() {
		_ = pw1.Close()
		_ = pw2.Close()
		_ = pr1.Close()
		_ = pr2.Close()
	})
	c := New(writeCloser{pw1}, pr2, nil)
	return c, fd
}

type writeCloser struct{ w *io.PipeWriter }

func (w writeCloser) Write(p []byte) (int, error) { return w.w.Write(p) }
func (w writeCloser) Close() error                { return w.w.Close() }

func (fd *fakeDriver) readMsg() map[string]any {
	fd.t.Helper()
	line, err := fd.driverIn.ReadString('\n')
	if err != nil {
		fd.t.Fatalf("driver readMsg: %v (partial %q)", err, line)
	}
	var msg map[string]any
	if err := json.Unmarshal([]byte(line), &msg); err != nil {
		fd.t.Fatalf("driver readMsg: bad JSON %q: %v", line, err)
	}
	return msg
}

func (fd *fakeDriver) writeMsg(msg map[string]any) {
	fd.t.Helper()
	raw, err := json.Marshal(msg)
	if err != nil {
		fd.t.Fatalf("driver writeMsg: marshal: %v", err)
	}
	if _, err := fd.clientOutW.Write(append(raw, '\n')); err != nil {
		fd.t.Fatalf("driver writeMsg: write: %v", err)
	}
}

func (fd *fakeDriver) killSubprocess() {
	_ = fd.clientOutW.Close()
}

func TestInit_HappyPath(t *testing.T) {
	c, fd := newFakeDriver(t)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		msg := fd.readMsg()
		if msg["type"] != MsgInit {
			t.Errorf("driver got type=%v, want %s", msg["type"], MsgInit)
		}
		fd.writeMsg(map[string]any{"type": MsgReady})
	}()

	if err := c.Init(context.Background(), map[string]any{"dsn": "fake"}); err != nil {
		t.Fatalf("Init: %v", err)
	}
	wg.Wait()
}

func TestInit_InitError(t *testing.T) {
	c, fd := newFakeDriver(t)

	go func() {
		_ = fd.readMsg()
		fd.writeMsg(map[string]any{
			"type":  MsgInitError,
			"error": "ImportError: psycopg2",
			"trace": "Traceback...",
		})
	}()

	err := c.Init(context.Background(), map[string]any{})
	if !errors.Is(err, ErrInitFailed) {
		t.Errorf("expected ErrInitFailed, got %v", err)
	}
	if !strings.Contains(err.Error(), "psycopg2") {
		t.Errorf("error should preserve remote message; got %v", err)
	}
}

func TestInit_CrashedSubprocess(t *testing.T) {
	c, fd := newFakeDriver(t)

	go func() {
		_ = fd.readMsg()
		fd.killSubprocess()
	}()

	err := c.Init(context.Background(), map[string]any{})
	if !errors.Is(err, ErrCrashed) {
		t.Errorf("expected ErrCrashed after EOF, got %v", err)
	}
	if !c.Crashed() {
		t.Error("Crashed() should report true after EOF")
	}
}

func TestCall_HappyPath(t *testing.T) {
	c, fd := newFakeDriver(t)

	go func() {
		_ = fd.readMsg()
		fd.writeMsg(map[string]any{"type": MsgReady})

		msg := fd.readMsg()
		if msg["type"] != MsgCall {
			t.Errorf("expected call, got %v", msg["type"])
		}
		if msg["method"] != "do_query" {
			t.Errorf("method = %v, want do_query", msg["method"])
		}
		id := msg["id"]
		fd.writeMsg(map[string]any{
			"type": MsgReturn,
			"id":   id,
			"data": []any{"row1", "row2"},
		})
	}()

	_ = c.Init(context.Background(), nil)
	res, err := c.Call(context.Background(), "do_query", map[string]any{"sql": "SELECT 1"})
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	rows, ok := res.([]any)
	if !ok || len(rows) != 2 {
		t.Errorf("result = %v, want []any{row1,row2}", res)
	}
}

func TestCall_RemoteException(t *testing.T) {
	c, fd := newFakeDriver(t)

	go func() {
		_ = fd.readMsg()
		fd.writeMsg(map[string]any{"type": MsgReady})

		msg := fd.readMsg()
		fd.writeMsg(map[string]any{
			"type":  MsgError,
			"id":    msg["id"],
			"error": "ValueError: nope",
			"trace": "Traceback...",
		})
	}()

	_ = c.Init(context.Background(), nil)
	_, err := c.Call(context.Background(), "boom", nil)
	if !errors.Is(err, ErrCallFailed) {
		t.Errorf("expected ErrCallFailed, got %v", err)
	}
	if !strings.Contains(err.Error(), "ValueError") {
		t.Errorf("error should preserve remote message; got %v", err)
	}
}

func TestStreamCall_ProgressThenReturn(t *testing.T) {
	c, fd := newFakeDriver(t)

	go func() {
		_ = fd.readMsg()
		fd.writeMsg(map[string]any{"type": MsgReady})

		msg := fd.readMsg()
		id := msg["id"]
		fd.writeMsg(map[string]any{"type": MsgProgress, "id": id, "data": "chunk-1"})
		fd.writeMsg(map[string]any{"type": MsgProgress, "id": id, "data": "chunk-2"})
		fd.writeMsg(map[string]any{"type": MsgReturn, "id": id, "data": "final"})
	}()

	_ = c.Init(context.Background(), nil)
	var captured []any
	res, err := c.StreamCall(context.Background(), "stream", nil, func(p any) {
		captured = append(captured, p)
	})
	if err != nil {
		t.Fatalf("StreamCall: %v", err)
	}
	if res != "final" {
		t.Errorf("res = %v, want final", res)
	}
	if len(captured) != 2 || captured[0] != "chunk-1" || captured[1] != "chunk-2" {
		t.Errorf("progress captures = %v, want [chunk-1 chunk-2]", captured)
	}
}

func TestCall_CtxCancel(t *testing.T) {
	c, fd := newFakeDriver(t)

	go func() {
		_ = fd.readMsg()
		fd.writeMsg(map[string]any{"type": MsgReady})
		_ = fd.readMsg()
	}()
	_ = c.Init(context.Background(), nil)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	_, err := c.Call(ctx, "slow", nil)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("expected context.DeadlineExceeded, got %v", err)
	}
	if !c.Crashed() {
		t.Error("after ctx cancel mid-call, client should transition to crashed (no way to recover serialized state)")
	}
}

func TestShutdown_Idempotent(t *testing.T) {
	c, fd := newFakeDriver(t)
	_ = fd

	if err := c.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	if err := c.Shutdown(context.Background()); err != nil {
		t.Fatalf("second Shutdown: %v", err)
	}

	if _, e := c.Call(context.Background(), "x", nil); !errors.Is(e, ErrShutdownAlready) {
		t.Errorf("expected ErrShutdownAlready from Call after Shutdown; got %v", e)
	}
	if e := c.Init(context.Background(), nil); !errors.Is(e, ErrShutdownAlready) {
		t.Errorf("expected ErrShutdownAlready from Init after Shutdown; got %v", e)
	}
}
