package shell

import (
	"errors"
	"strings"
	"testing"
)

func TestProcessManager_RegisterAssignsID(t *testing.T) {
	mgr := NewProcessManager()
	p := &BgProcess{Command: "echo hi"}
	mgr.Register(p)
	if p.ID == "" {
		t.Error("Register should assign an ID when none provided")
	}
	if !strings.HasPrefix(p.ID, "bsh_") {
		t.Errorf("ID should have bsh_ prefix, got %q", p.ID)
	}
}

func TestProcessManager_GetReturnsErrNotFoundForUnknown(t *testing.T) {
	mgr := NewProcessManager()
	_, err := mgr.Get("bsh_nonexistent")
	if !errors.Is(err, ErrProcessNotFound) {
		t.Errorf("want ErrProcessNotFound, got %v", err)
	}
}

func TestProcessManager_RegisterThenGetThenRemove(t *testing.T) {
	mgr := NewProcessManager()
	p := &BgProcess{Command: "ls"}
	mgr.Register(p)

	got, err := mgr.Get(p.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != p {
		t.Error("Get should return the registered pointer")
	}

	mgr.Remove(p.ID)
	if _, err := mgr.Get(p.ID); !errors.Is(err, ErrProcessNotFound) {
		t.Errorf("after Remove, want ErrProcessNotFound, got %v", err)
	}
}

func TestBgProcess_AppendOutput_BelowCap_NoDrop(t *testing.T) {
	p := &BgProcess{}
	p.appendOutput([]byte("hello"))
	p.appendOutput([]byte(" world"))
	if string(p.buf) != "hello world" {
		t.Errorf("buf = %q, want %q", p.buf, "hello world")
	}
	if p.dropped != 0 {
		t.Errorf("dropped = %d, want 0", p.dropped)
	}
}

func TestBgProcess_AppendOutput_OverflowRingDropsHead(t *testing.T) {
	p := &BgProcess{}
	// Fill exactly to cap.
	// 灌满到 cap。
	full := strings.Repeat("X", bgBufferBytes)
	p.appendOutput([]byte(full))
	if p.dropped != 0 {
		t.Errorf("at-cap drop = %d, want 0", p.dropped)
	}
	// Add 100 more bytes — should drop 100 from head.
	// 再加 100 字节——应丢头部 100。
	p.appendOutput([]byte(strings.Repeat("Y", 100)))
	if len(p.buf) != bgBufferBytes {
		t.Errorf("len = %d, want %d", len(p.buf), bgBufferBytes)
	}
	if p.dropped != 100 {
		t.Errorf("dropped = %d, want 100", p.dropped)
	}
	// Tail should now be all Y's.
	// 尾部应都是 Y。
	if !strings.HasSuffix(string(p.buf), "YYYYYYYYY") {
		t.Errorf("tail not Y's: %q", string(p.buf[len(p.buf)-20:]))
	}
}

func TestBgProcess_DrainNew_AdvancesCursor(t *testing.T) {
	p := &BgProcess{status: StatusRunning}
	p.appendOutput([]byte("first chunk\n"))
	got, _, _, _ := p.drainNew()
	if string(got) != "first chunk\n" {
		t.Errorf("drain1 = %q", got)
	}
	// Second call before any new output should return empty.
	// 第二次调用且无新输出应返空。
	got, _, _, _ = p.drainNew()
	if len(got) != 0 {
		t.Errorf("drain2 = %q, want empty", got)
	}
	// New output then drains as expected.
	// 新输出后正常 drain。
	p.appendOutput([]byte("second chunk\n"))
	got, _, _, _ = p.drainNew()
	if string(got) != "second chunk\n" {
		t.Errorf("drain3 = %q", got)
	}
}

func TestBgProcess_DrainNew_OverflowRewindsCursor(t *testing.T) {
	// Cursor pointed inside dropped region must snap to start of buffer.
	// 游标指在被丢区域必须贴齐到缓冲头。
	p := &BgProcess{}
	p.appendOutput(make([]byte, bgBufferBytes-100))
	_, _, _, _ = p.drainNew() // cursor at end
	p.appendOutput(make([]byte, 500))
	// Now ring overflowed: ~400 bytes were dropped, cursor should not
	// land past the new head.
	// 环形溢出：约 400 字节被丢；游标不应越过新头。
	got, _, _, _ := p.drainNew()
	if len(got) > 500 {
		t.Errorf("drain leaked = %d bytes; want ≤ 500", len(got))
	}
}

func TestBgProcess_MarkFinished_RecordsExit(t *testing.T) {
	p := &BgProcess{status: StatusRunning}
	p.markFinished(StatusExited, 7)
	_, _, status, code := p.drainNew()
	if status != StatusExited {
		t.Errorf("status = %s", status)
	}
	if code != 7 {
		t.Errorf("exit code = %d, want 7", code)
	}
}

func TestProcessManager_Stop_DoesNotPanic_OnEmpty(t *testing.T) {
	NewProcessManager().Stop()
	NewProcessManager().Stop() // second call also fine
}
