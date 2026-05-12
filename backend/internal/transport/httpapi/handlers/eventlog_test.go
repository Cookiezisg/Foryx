package handlers

import (
	"bufio"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	eventlogdomain "github.com/sunweilin/forgify/backend/internal/domain/eventlog"
	eventloginfra "github.com/sunweilin/forgify/backend/internal/infra/eventlog"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
	middlewarehttpapi "github.com/sunweilin/forgify/backend/internal/transport/httpapi/middleware"
)

// newEventLogServer builds an httptest server hosting the eventlog SSE
// endpoint wrapped with the InjectUserID middleware. The middleware stamps
// the default local user_id into request ctx so the bridge's per-user
// keying (D-redo-2) gets a key.
//
// newEventLogServer 建带 InjectUserID 中间件的 httptest server;中间件把
// 默认本地 user_id 塞进 ctx,Bridge per-user keying(D-redo-2)能拿到 key。
func newEventLogServer(t *testing.T) (*httptest.Server, *eventloginfra.Bridge) {
	t.Helper()
	bridge := eventloginfra.NewBridge(nil)
	mux := http.NewServeMux()
	NewEventLogHandler(bridge, nil, nil).Register(mux)
	srv := httptest.NewServer(middlewarehttpapi.InjectUserID(mux))
	t.Cleanup(srv.Close)
	return srv, bridge
}

// publishCtx returns a Background ctx with the default local user_id
// stamped so test publishers can use Bridge.Publish.
//
// publishCtx 返带默认本地 user_id 的 ctx 给测试 publisher 用。
func publishCtx() context.Context {
	return reqctxpkg.SetUserID(context.Background(), reqctxpkg.DefaultLocalUserID)
}

func TestEventLog_StreamDeliversLiveEvents(t *testing.T) {
	srv, bridge := newEventLogServer(t)

	// Open SSE connection — no query parameter required after D-redo-2.
	// 开 SSE 连接;D-redo-2 后无 query 参。
	req, _ := http.NewRequest("GET", srv.URL+"/api/v1/eventlog", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: got %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Errorf("content-type: got %q, want text/event-stream", ct)
	}

	// Subscribe will register; allow it to settle, then publish.
	time.Sleep(50 * time.Millisecond)
	bridge.Publish(publishCtx(), eventlogdomain.MessageStart{
		ConversationID: "cv_1", ID: "msg_1", Role: "assistant",
	})

	got := readSSE(t, resp.Body, 1, 2*time.Second)
	if len(got) != 1 {
		t.Fatalf("want 1 event, got %d", len(got))
	}
	ev := got[0]
	if ev.event != "message_start" {
		t.Errorf("event line: got %q, want message_start", ev.event)
	}
	if ev.id != "1" {
		t.Errorf("id line: got %q, want 1", ev.id)
	}
	if !strings.Contains(ev.data, `"id":"msg_1"`) {
		t.Errorf("data missing msg_1: %q", ev.data)
	}
	// Per-user stream covers all conversations — payload still carries
	// conversationId for client-side demux.
	// per-user 流跨对话;payload 仍带 conversationId 给 client demux。
	if !strings.Contains(ev.data, `"conversationId":"cv_1"`) {
		t.Errorf("data missing conversationId payload: %q", ev.data)
	}
}

func TestEventLog_LastEventIDReplays(t *testing.T) {
	srv, bridge := newEventLogServer(t)

	ctx := publishCtx()
	for i := 0; i < 3; i++ {
		bridge.Publish(ctx, eventlogdomain.MessageStart{
			ConversationID: "cv_1", ID: "msg", Role: "assistant",
		})
	}

	// Subscribe with Last-Event-ID: 1 → should receive seq 2 + 3.
	req, _ := http.NewRequest("GET", srv.URL+"/api/v1/eventlog", nil)
	req.Header.Set("Last-Event-ID", "1")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: got %d, want 200", resp.StatusCode)
	}
	got := readSSE(t, resp.Body, 2, 2*time.Second)
	if len(got) != 2 {
		t.Fatalf("want 2 replay events, got %d", len(got))
	}
	if got[0].id != "2" || got[1].id != "3" {
		t.Errorf("ids: got %s,%s want 2,3", got[0].id, got[1].id)
	}
}

func TestEventLog_LastEventIDTooOldReturns410(t *testing.T) {
	srv, bridge := newEventLogServer(t)

	ctx := publishCtx()
	// Fill buffer past replay capacity so old seqs evict.
	for i := 0; i < 4096+50; i++ {
		bridge.Publish(ctx, eventlogdomain.MessageStart{
			ConversationID: "cv_1", ID: "msg", Role: "assistant",
		})
	}

	req, _ := http.NewRequest("GET", srv.URL+"/api/v1/eventlog", nil)
	req.Header.Set("Last-Event-ID", "1") // long evicted
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusGone {
		t.Errorf("status: got %d, want 410 Gone", resp.StatusCode)
	}
}

// ── SSE wire parsing helper ──────────────────────────────────────────

type sseRecord struct {
	event string
	id    string
	data  string
}

// readSSE reads up to want events from rdr or fails after deadline.
func readSSE(t *testing.T, body interface {
	Read([]byte) (int, error)
}, want int, deadline time.Duration) []sseRecord {
	t.Helper()

	type result struct {
		recs []sseRecord
	}
	resCh := make(chan result, 1)

	go func() {
		sc := bufio.NewScanner(struct{ readerWrapper }{readerWrapper{body}})
		var (
			recs []sseRecord
			cur  sseRecord
		)
		for sc.Scan() {
			line := sc.Text()
			switch {
			case line == "":
				if cur.event != "" {
					recs = append(recs, cur)
					if len(recs) >= want {
						resCh <- result{recs: recs}
						return
					}
				}
				cur = sseRecord{}
			case strings.HasPrefix(line, "event: "):
				cur.event = strings.TrimPrefix(line, "event: ")
			case strings.HasPrefix(line, "id: "):
				cur.id = strings.TrimPrefix(line, "id: ")
			case strings.HasPrefix(line, "data: "):
				cur.data = strings.TrimPrefix(line, "data: ")
			}
		}
		resCh <- result{recs: recs}
	}()

	select {
	case r := <-resCh:
		return r.recs
	case <-time.After(deadline):
		t.Fatalf("timeout waiting for %d SSE events", want)
		return nil
	}
}

type readerWrapper struct {
	r interface {
		Read([]byte) (int, error)
	}
}

func (rw readerWrapper) Read(p []byte) (int, error) { return rw.r.Read(p) }
