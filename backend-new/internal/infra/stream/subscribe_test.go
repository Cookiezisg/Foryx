package stream

import (
	"context"
	"errors"
	"testing"

	streamdomain "github.com/sunweilin/forgify/backend/internal/domain/stream"
)

func TestSubscribeLiveFanout(t *testing.T) {
	b := New(16)
	ctx := wsCtx("ws1")
	ch, cancel, err := b.Subscribe(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer cancel()
	want, _ := b.Publish(ctx, durableEvent())
	got := <-ch
	if got.Seq != want.Seq {
		t.Errorf("fanout seq = %d, want %d", got.Seq, want.Seq)
	}
}

func TestSubscribeReplaysFromSeq(t *testing.T) {
	b := New(16)
	ctx := wsCtx("ws1")
	b.Publish(ctx, durableEvent()) // seq 1
	b.Publish(ctx, durableEvent()) // seq 2
	b.Publish(ctx, durableEvent()) // seq 3
	ch, cancel, err := b.Subscribe(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer cancel()
	if e := <-ch; e.Seq != 2 {
		t.Errorf("first replay seq = %d, want 2", e.Seq)
	}
	if e := <-ch; e.Seq != 3 {
		t.Errorf("second replay seq = %d, want 3", e.Seq)
	}
}

func TestSubscribeSeqTooOld(t *testing.T) {
	b := New(2) // tiny ring keeps only the last 2 durable frames
	ctx := wsCtx("ws1")
	for i := 0; i < 5; i++ {
		b.Publish(ctx, durableEvent()) // ring ends up holding seq 4,5
	}
	if _, _, err := b.Subscribe(ctx, 1); !errors.Is(err, streamdomain.ErrSeqTooOld) {
		t.Errorf("err = %v, want ErrSeqTooOld", err)
	}
}

func TestSubscribeRequiresWorkspace(t *testing.T) {
	b := New(16)
	if _, _, err := b.Subscribe(context.Background(), 0); err == nil {
		t.Error("Subscribe with no workspace: want error, got nil")
	}
}

func TestCancelIdempotent(t *testing.T) {
	b := New(16)
	_, cancel, err := b.Subscribe(wsCtx("ws1"), 0)
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	cancel() // second cancel must not panic or double-close
}
