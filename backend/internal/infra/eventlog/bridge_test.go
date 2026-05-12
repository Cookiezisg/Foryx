package eventlog

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	eventlogdomain "github.com/sunweilin/forgify/backend/internal/domain/eventlog"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

// msgStart makes a valid MessageStart for test brevity. ConversationID
// is still carried in the event payload (clients demux on it) but the
// bridge keys by user_id from ctx (D-redo-2).
//
// msgStart 给测试方便造合法 MessageStart;payload 仍带 ConversationID 给
// client demux,但 bridge 按 ctx user_id key(D-redo-2)。
func msgStart(convID, msgID string) eventlogdomain.MessageStart {
	return eventlogdomain.MessageStart{
		ConversationID: convID,
		ID:             msgID,
		Role:           "assistant",
	}
}

// ctxFor returns a ctx with user_id set. The infra Bridge reads user_id
// via reqctxpkg.RequireUserID.
//
// ctxFor 返带 user_id 的 ctx。
func ctxFor(uid string) context.Context {
	return reqctxpkg.SetUserID(context.Background(), uid)
}

func TestPublish_AssignsMonotonicSeq(t *testing.T) {
	b := NewBridge(nil)
	ctx := ctxFor("u1")
	for i := 1; i <= 5; i++ {
		env, err := b.Publish(ctx, msgStart("cv_1", "m1"))
		if err != nil {
			t.Fatalf("publish #%d: %v", i, err)
		}
		if env.Seq != int64(i) {
			t.Errorf("seq #%d: want %d, got %d", i, i, env.Seq)
		}
	}
}

// TestPublish_PerUserSeq — different users have independent seq counters
// (D-redo-2). One user's traffic does not bump another user's seq.
func TestPublish_PerUserSeq(t *testing.T) {
	b := NewBridge(nil)
	ctxA := ctxFor("user_a")
	ctxB := ctxFor("user_b")
	envA1, _ := b.Publish(ctxA, msgStart("cv_a", "m1"))
	envB1, _ := b.Publish(ctxB, msgStart("cv_b", "m1"))
	envA2, _ := b.Publish(ctxA, msgStart("cv_a", "m2"))
	envB2, _ := b.Publish(ctxB, msgStart("cv_b", "m2"))
	if envA1.Seq != 1 || envA2.Seq != 2 {
		t.Errorf("user_a seq: got %d,%d want 1,2", envA1.Seq, envA2.Seq)
	}
	if envB1.Seq != 1 || envB2.Seq != 2 {
		t.Errorf("user_b seq: got %d,%d want 1,2", envB1.Seq, envB2.Seq)
	}
}

// TestPublish_PerUserSeqAcrossConversations — same user, multiple convs:
// seq is monotonic across the user's whole stream (not per-conv).
func TestPublish_PerUserSeqAcrossConversations(t *testing.T) {
	b := NewBridge(nil)
	ctx := ctxFor("user_a")
	env1, _ := b.Publish(ctx, msgStart("cv_a", "m1"))
	env2, _ := b.Publish(ctx, msgStart("cv_b", "m1"))
	env3, _ := b.Publish(ctx, msgStart("cv_a", "m2"))
	if env1.Seq != 1 || env2.Seq != 2 || env3.Seq != 3 {
		t.Errorf("cross-conv seq: got %d,%d,%d want 1,2,3", env1.Seq, env2.Seq, env3.Seq)
	}
}

func TestPublish_RejectsMissingUserID(t *testing.T) {
	b := NewBridge(nil)
	_, err := b.Publish(context.Background(), msgStart("cv_1", "m1"))
	if !errors.Is(err, reqctxpkg.ErrMissingUserID) {
		t.Errorf("want ErrMissingUserID, got %v", err)
	}
}

func TestPublish_RejectsInvalidPayload(t *testing.T) {
	b := NewBridge(nil)
	_, err := b.Publish(ctxFor("u1"), eventlogdomain.MessageStart{
		ConversationID: "cv_1", ID: "", Role: "user",
	})
	if !errors.Is(err, eventlogdomain.ErrInvalidEvent) {
		t.Errorf("want ErrInvalidEvent, got %v", err)
	}
}

func TestSubscribe_LiveDelivery(t *testing.T) {
	b := NewBridge(nil)
	ctx, cancel := context.WithCancel(ctxFor("u1"))
	defer cancel()

	ch, cancelSub, err := b.Subscribe(ctx, 0)
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	defer cancelSub()

	for i := 0; i < 3; i++ {
		if _, err := b.Publish(ctx, msgStart("cv_1", "m")); err != nil {
			t.Fatalf("publish: %v", err)
		}
	}

	for i := int64(1); i <= 3; i++ {
		select {
		case env := <-ch:
			if env.Seq != i {
				t.Errorf("delivery #%d: want seq %d, got %d", i, i, env.Seq)
			}
		case <-time.After(time.Second):
			t.Fatalf("timeout waiting for env %d", i)
		}
	}
}

func TestSubscribe_ReplayFromSeq(t *testing.T) {
	b := NewBridge(nil)
	ctx := ctxFor("u1")

	for i := 0; i < 5; i++ {
		b.Publish(ctx, msgStart("cv_1", "m"))
	}

	// Subscribe asking for replay from seq=2 (so we want seq 3,4,5).
	ch, cancelSub, err := b.Subscribe(ctx, 2)
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	defer cancelSub()

	for want := int64(3); want <= 5; want++ {
		select {
		case env := <-ch:
			if env.Seq != want {
				t.Errorf("replay: want seq %d, got %d", want, env.Seq)
			}
		case <-time.After(time.Second):
			t.Fatalf("timeout waiting for replay seq %d", want)
		}
	}

	// Publish a 6th event live; should arrive after replay.
	b.Publish(ctx, msgStart("cv_1", "m"))
	select {
	case env := <-ch:
		if env.Seq != 6 {
			t.Errorf("post-replay live: want seq 6, got %d", env.Seq)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for live event after replay")
	}
}

func TestSubscribe_FromSeqZeroSkipsReplay(t *testing.T) {
	b := NewBridge(nil)
	ctx := ctxFor("u1")
	for i := 0; i < 3; i++ {
		b.Publish(ctx, msgStart("cv_1", "m"))
	}
	ch, cancelSub, err := b.Subscribe(ctx, 0)
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	defer cancelSub()
	select {
	case env := <-ch:
		t.Errorf("fromSeq=0 should skip replay; got seq %d", env.Seq)
	case <-time.After(50 * time.Millisecond):
		// expected silence
	}
}

func TestSubscribe_TooOldReturnsErrSeqTooOld(t *testing.T) {
	b := NewBridge(nil)
	ctx := ctxFor("u1")

	// Fill buffer past replayBufferSize so old seqs get evicted.
	const total = replayBufferSize + 100
	for i := 0; i < total; i++ {
		b.Publish(ctx, msgStart("cv_1", "m"))
	}

	// Ask for replay from seq=10 (long evicted).
	_, _, err := b.Subscribe(ctx, 10)
	if !errors.Is(err, eventlogdomain.ErrSeqTooOld) {
		t.Errorf("want ErrSeqTooOld, got %v", err)
	}

	// But asking for seq within buffer should succeed.
	from := int64(total - 50)
	ch, cancelSub, err := b.Subscribe(ctx, from)
	if err != nil {
		t.Fatalf("subscribe near tail: %v", err)
	}
	defer cancelSub()
	// Should receive 50 envelopes.
	for want := from + 1; want <= int64(total); want++ {
		select {
		case env := <-ch:
			if env.Seq != want {
				t.Errorf("replay tail: want seq %d, got %d", want, env.Seq)
			}
		case <-time.After(time.Second):
			t.Fatalf("timeout at seq %d", want)
		}
	}
}

func TestSubscribe_CancelStopsDelivery(t *testing.T) {
	b := NewBridge(nil)
	ctx := ctxFor("u1")
	ch, cancelSub, _ := b.Subscribe(ctx, 0)
	cancelSub()
	cancelSub() // idempotent — should not panic

	// Publish after cancel; subscriber should NOT block publisher.
	done := make(chan struct{})
	go func() {
		b.Publish(ctx, msgStart("cv_1", "m"))
		close(done)
	}()
	select {
	case <-done:
		// good — publisher returned
	case <-time.After(time.Second):
		t.Fatal("publisher blocked after subscriber cancelled")
	}

	// Drain any in-flight events.
	go func() {
		for range ch {
		}
	}()
}

func TestSubscribe_CtxCancelStopsDelivery(t *testing.T) {
	b := NewBridge(nil)
	ctx, cancelCtx := context.WithCancel(ctxFor("u1"))
	ch, cancelSub, _ := b.Subscribe(ctx, 0)
	defer cancelSub()

	cancelCtx()
	// Allow goroutine to see ctx.Done.
	time.Sleep(10 * time.Millisecond)

	// Publish should not block (sub must have been auto-removed). Use
	// a fresh ctx with user_id (the cancelled one would refuse publish).
	done := make(chan struct{})
	go func() {
		b.Publish(ctxFor("u1"), msgStart("cv_1", "m"))
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("publisher blocked after ctx cancelled")
	}

	// Drain any pending.
	go func() {
		for range ch {
		}
	}()
}

func TestPublish_BlockOnSlowSubscriber(t *testing.T) {
	b := NewBridge(nil)
	ctx := ctxFor("u1")
	_, cancelSub, _ := b.Subscribe(ctx, 0)
	defer cancelSub()
	// Don't drain the channel — buffer fills.

	// Publish subscriberBufferSize+1 events; the +1 should block.
	pubDone := make(chan struct{})
	go func() {
		for i := 0; i < subscriberBufferSize+1; i++ {
			b.Publish(ctx, msgStart("cv_1", "m"))
		}
		close(pubDone)
	}()

	select {
	case <-pubDone:
		t.Fatal("publisher should have blocked on full subscriber buffer")
	case <-time.After(100 * time.Millisecond):
		// good — blocked as expected
	}
	// Cancel sub to unblock the goroutine.
	cancelSub()
	select {
	case <-pubDone:
	case <-time.After(time.Second):
		t.Fatal("publisher did not unblock after cancel")
	}
}

func TestPublish_ConcurrentMonotonicity(t *testing.T) {
	b := NewBridge(nil)
	ctx := ctxFor("u1")

	const N = 200
	var wg sync.WaitGroup
	seen := make(chan int64, N)
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			env, err := b.Publish(ctx, msgStart("cv_1", "m"))
			if err != nil {
				t.Errorf("publish: %v", err)
				return
			}
			seen <- env.Seq
		}()
	}
	wg.Wait()
	close(seen)

	got := make(map[int64]int)
	for s := range seen {
		got[s]++
	}
	for i := int64(1); i <= N; i++ {
		if got[i] != 1 {
			t.Errorf("seq %d: occurred %d times (want 1)", i, got[i])
		}
	}
}
