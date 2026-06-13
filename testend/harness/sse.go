// sse.go subscribes to the three SSE streams exactly like the frontend will (E1: messages
// / entities / notifications) and turns "did event X arrive" into assertions. Events are
// collected raw — what the wire carries is what scenarios judge.
//
// sse.go 像未来前端一样订阅三条 SSE 流（E1：messages/entities/notifications），把「事件 X
// 到了吗」变成断言。事件按原始线缆收集——场景评判的就是线缆上有什么。
package harness

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

// SSEEvent is one wire frame: the SSE event name + raw data payload.
//
// SSEEvent 是一帧线缆事件：SSE event 名 + 原始 data 载荷。
type SSEEvent struct {
	Event string
	Data  json.RawMessage
}

// SSE is a live subscription collecting events until the test ends.
//
// SSE 是一条活订阅，收集事件直到测试结束。
type SSE struct {
	mu     sync.Mutex
	events []SSEEvent
	cancel context.CancelFunc
}

// Subscribe opens one stream ("messages" | "entities" | "notifications") for the
// workspace and collects every frame in the background.
//
// Subscribe 打开一条流（messages|entities|notifications）并后台收集每一帧。
func (c *Client) Subscribe(t *testing.T, stream string) *SSE {
	t.Helper()
	return c.SubscribeFrom(t, stream, -1)
}

// SubscribeFrom opens a stream resuming from seq (the reconnect path: durable frames
// with seq > fromSeq replay, ephemeral deltas never do). fromSeq < 0 → live-only.
//
// SubscribeFrom 从 seq 续传打开一条流（重连路径：seq > fromSeq 的 durable 帧重放、
// ephemeral delta 永不重放）。fromSeq < 0 → 仅实时。
func (c *Client) SubscribeFrom(t *testing.T, stream string, fromSeq int64) *SSE {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	url := c.base + "/api/v1/" + stream + "/stream"
	if fromSeq >= 0 {
		url += fmt.Sprintf("?fromSeq=%d", fromSeq)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		cancel()
		t.Fatalf("sse: new request: %v", err)
	}
	req.Header.Set(HeaderWorkspace, c.ws)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		cancel()
		t.Fatalf("sse: connect %s: %v", stream, err)
	}
	if resp.StatusCode != http.StatusOK {
		cancel()
		t.Fatalf("sse: %s stream status %d", stream, resp.StatusCode)
	}
	s := &SSE{cancel: cancel}
	go func() {
		defer resp.Body.Close()
		sc := bufio.NewScanner(resp.Body)
		sc.Buffer(make([]byte, 64*1024), 4*1024*1024)
		var ev SSEEvent
		for sc.Scan() {
			line := sc.Text()
			switch {
			case strings.HasPrefix(line, "event:"):
				ev.Event = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			case strings.HasPrefix(line, "data:"):
				ev.Data = json.RawMessage(strings.TrimSpace(strings.TrimPrefix(line, "data:")))
			case line == "" && (ev.Event != "" || len(ev.Data) > 0):
				s.mu.Lock()
				s.events = append(s.events, ev)
				s.mu.Unlock()
				ev = SSEEvent{}
			}
		}
	}()
	t.Cleanup(s.Close)
	return s
}

func (s *SSE) Close() { s.cancel() }

// Snapshot returns the events collected so far.
//
// Snapshot 返回迄今收集的事件。
func (s *SSE) Snapshot() []SSEEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]SSEEvent, len(s.events))
	copy(out, s.events)
	return out
}

// WaitFor polls until an event whose raw frame contains all substrings arrives.
//
// WaitFor 轮询直到出现「原始帧包含全部子串」的事件。
func (s *SSE) WaitFor(t *testing.T, timeoutMS int, what string, substrs ...string) SSEEvent {
	t.Helper()
	deadline := time.Now().Add(time.Duration(timeoutMS) * time.Millisecond)
	for time.Now().Before(deadline) {
		for _, ev := range s.Snapshot() {
			all := true
			for _, sub := range substrs {
				if !strings.Contains(ev.Event+" "+string(ev.Data), sub) {
					all = false
					break
				}
			}
			if all {
				return ev
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("sse.WaitFor: %s — %v not seen within %dms (got %d events)", what, substrs, timeoutMS, len(s.Snapshot()))
	return SSEEvent{}
}

// Never asserts no matching event arrived within the window (negative assertion).
//
// Never 断言窗口内没有匹配事件（否定断言）。
func (s *SSE) Never(t *testing.T, windowMS int, what string, substrs ...string) {
	t.Helper()
	time.Sleep(time.Duration(windowMS) * time.Millisecond)
	for _, ev := range s.Snapshot() {
		all := true
		for _, sub := range substrs {
			if !strings.Contains(ev.Event+" "+string(ev.Data), sub) {
				all = false
				break
			}
		}
		if all {
			t.Fatalf("sse.Never: %s — unexpected event %s %s", what, ev.Event, ev.Data)
		}
	}
}
