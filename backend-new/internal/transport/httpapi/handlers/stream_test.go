package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	streamdomain "github.com/sunweilin/forgify/backend/internal/domain/stream"
)

// fakeBridge serves a canned channel (or a canned Subscribe error) so the SSE plumbing can be
// exercised without a real workspace-partitioned Bus.
type fakeBridge struct {
	ch  chan streamdomain.Envelope
	err error
}

func (f *fakeBridge) Subscribe(_ context.Context, _ int64) (<-chan streamdomain.Envelope, func(), error) {
	if f.err != nil {
		return nil, nil, f.err
	}
	return f.ch, func() {}, nil
}
func (f *fakeBridge) Publish(_ context.Context, _ streamdomain.Event) (streamdomain.Envelope, error) {
	return streamdomain.Envelope{}, nil
}

func env(seq int64, convID, chunk string) streamdomain.Envelope {
	return streamdomain.Envelope{
		Seq: seq,
		Event: streamdomain.Event{
			Scope: streamdomain.Scope{Kind: streamdomain.KindConversation, ID: convID},
			ID:    "n1",
			Frame: streamdomain.Delta{Chunk: chunk},
		},
	}
}

// TestStreamHandler_StreamsVerbatimUntilClose: the handler streams every frame the bus delivers
// VERBATIM (no scope filter — the workspace is the only partition), then exits when the channel
// closes. Two different conversations' frames both reach the client; filtering is the client's job.
//
// TestStreamHandler_StreamsVerbatimUntilClose：handler 逐字流出总线送来的每帧（不按 scope 过滤——
// workspace 是唯一分区），channel 关闭即退出。两个不同对话的帧都到客户端；过滤是客户端的事。
func TestStreamHandler_StreamsVerbatimUntilClose(t *testing.T) {
	ch := make(chan streamdomain.Envelope, 2)
	ch <- env(1, "c1", "hello")
	ch <- env(2, "c2", "world") // a different conversation — still forwarded
	close(ch)

	h := NewStreamHandler(&fakeBridge{ch: ch}, &fakeBridge{}, &fakeBridge{}, nil)
	mux := http.NewServeMux()
	h.Register(mux)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/messages/stream", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("content-type = %q, want text/event-stream", ct)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "hello") || !strings.Contains(body, "world") {
		t.Fatalf("stream body missing frames (no client-side filter expected): %q", body)
	}
}

// TestStreamHandler_SeqTooOld410: a resume cursor evicted from the replay ring surfaces as 410 so
// the client refetches history and reconnects.
//
// TestStreamHandler_SeqTooOld410：被 replay 环淘汰的续传游标 → 410，客户端重取历史后重连。
func TestStreamHandler_SeqTooOld410(t *testing.T) {
	eb := &fakeBridge{err: streamdomain.ErrSeqTooOld} // same error bridge on all three streams
	h := NewStreamHandler(eb, eb, eb, nil)
	mux := http.NewServeMux()
	h.Register(mux)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/entities/stream", nil))

	if rec.Code != http.StatusGone {
		t.Fatalf("status = %d, want 410", rec.Code)
	}
}

func TestDecodeFromSeq(t *testing.T) {
	mk := func(header, query string) *http.Request {
		r := httptest.NewRequest(http.MethodGet, "/x?"+query, nil)
		if header != "" {
			r.Header.Set("Last-Event-ID", header)
		}
		return r
	}
	cases := []struct {
		header, query string
		want          int64
	}{
		{"42", "", 42},        // Last-Event-ID wins
		{"", "fromSeq=7", 7},  // query fallback
		{"9", "fromSeq=7", 9}, // header preferred over query
		{"", "", 0},           // absent → live-only
		{"junk", "", 0},       // invalid → 0
	}
	for _, c := range cases {
		if got := decodeFromSeq(mk(c.header, c.query)); got != c.want {
			t.Fatalf("decodeFromSeq(header=%q query=%q) = %d, want %d", c.header, c.query, got, c.want)
		}
	}
}
