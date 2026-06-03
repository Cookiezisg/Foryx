package stream

import (
	"context"
	"errors"
	"testing"

	streamdomain "github.com/sunweilin/forgify/backend/internal/domain/stream"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

func wsCtx(id string) context.Context {
	return reqctxpkg.SetWorkspaceID(context.Background(), id)
}

// durableEvent is an open frame (Durable). ephemeralEvent is a delta (lossy).
//
// durableEvent 是 open 帧（durable）。ephemeralEvent 是 delta（可丢）。
func durableEvent() streamdomain.Event {
	return streamdomain.Event{
		Scope: streamdomain.Scope{Kind: streamdomain.KindConversation, ID: "c1"},
		ID:    "n1",
		Frame: streamdomain.Open{Node: streamdomain.Node{Type: "text"}},
	}
}

func ephemeralEvent() streamdomain.Event {
	return streamdomain.Event{
		Scope: streamdomain.Scope{Kind: streamdomain.KindConversation, ID: "c1"},
		ID:    "n1",
		Frame: streamdomain.Delta{Chunk: "x"},
	}
}

func TestPublishDurableAssignsMonotonicSeq(t *testing.T) {
	b := New(16)
	ctx := wsCtx("ws1")
	e1, err := b.Publish(ctx, durableEvent())
	if err != nil {
		t.Fatal(err)
	}
	e2, _ := b.Publish(ctx, durableEvent())
	if e1.Seq != 1 || e2.Seq != 2 {
		t.Errorf("durable seq = %d, %d; want 1, 2", e1.Seq, e2.Seq)
	}
}

func TestPublishEphemeralSeqZeroNotBuffered(t *testing.T) {
	b := New(16)
	ctx := wsCtx("ws1")
	ev, err := b.Publish(ctx, ephemeralEvent())
	if err != nil {
		t.Fatal(err)
	}
	if ev.Seq != 0 {
		t.Errorf("ephemeral seq = %d, want 0", ev.Seq)
	}
	if got, _, _ := b.List(ctx, 0, 10); len(got) != 0 {
		t.Errorf("ephemeral entered the ring: List len = %d, want 0", len(got))
	}
	// Ephemeral must not advance the durable seq counter.
	if d, _ := b.Publish(ctx, durableEvent()); d.Seq != 1 {
		t.Errorf("durable seq after ephemeral = %d, want 1", d.Seq)
	}
}

func TestPublishRequiresWorkspace(t *testing.T) {
	b := New(16)
	_, err := b.Publish(context.Background(), durableEvent())
	if !errors.Is(err, reqctxpkg.ErrMissingWorkspaceID) {
		t.Errorf("err = %v, want ErrMissingWorkspaceID", err)
	}
}

func TestPublishValidates(t *testing.T) {
	b := New(16)
	bad := streamdomain.Event{Scope: streamdomain.Scope{Kind: "bogus", ID: "x"}, ID: "n1", Frame: streamdomain.Delta{}}
	if _, err := b.Publish(wsCtx("ws1"), bad); !errors.Is(err, streamdomain.ErrInvalidEvent) {
		t.Errorf("err = %v, want ErrInvalidEvent", err)
	}
}

func TestWorkspaceIsolation(t *testing.T) {
	b := New(16)
	a1, _ := b.Publish(wsCtx("ws1"), durableEvent())
	a2, _ := b.Publish(wsCtx("ws1"), durableEvent())
	c1, _ := b.Publish(wsCtx("ws2"), durableEvent())
	if a1.Seq != 1 || a2.Seq != 2 || c1.Seq != 1 {
		t.Errorf("seqs = %d,%d,%d; want 1,2,1 (per-workspace)", a1.Seq, a2.Seq, c1.Seq)
	}
}
