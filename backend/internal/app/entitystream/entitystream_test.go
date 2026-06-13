package entitystream

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	streamdomain "github.com/sunweilin/forgify/backend/internal/domain/stream"
)

type capBridge struct{ events []streamdomain.Event }

func (b *capBridge) Publish(_ context.Context, e streamdomain.Event) (streamdomain.Envelope, error) {
	b.events = append(b.events, e)
	return streamdomain.Envelope{}, nil
}
func (b *capBridge) Subscribe(_ context.Context, _ int64) (<-chan streamdomain.Envelope, func(), error) {
	return nil, func() {}, nil
}

func TestWriter_StreamsNodeScopedToEntity(t *testing.T) {
	b := &capBridge{}
	scope := streamdomain.Scope{Kind: streamdomain.KindFunction, ID: "fn_x"}
	w := New(context.Background(), b, scope, "forge", json.RawMessage(`{"op":"edit"}`))
	_, _ = w.Write([]byte("def main"))
	_, _ = w.Write([]byte("(x): return x"))
	w.Close("completed", json.RawMessage(`{"versionId":"fnv_1"}`))

	if len(b.events) != 4 {
		t.Fatalf("want 4 frames (open + 2 delta + close), got %d", len(b.events))
	}
	open, ok := b.events[0].Frame.(streamdomain.Open)
	if !ok || open.Node.Type != "forge" {
		t.Fatalf("frame[0] not a forge Open: %+v", b.events[0])
	}
	if b.events[0].Scope.Kind != streamdomain.KindFunction || b.events[0].Scope.ID != "fn_x" {
		t.Fatalf("not scoped to function:fn_x: %+v", b.events[0].Scope)
	}
	if string(open.Node.Content) != `{"op":"edit"}` {
		t.Fatalf("open content lost: %s", open.Node.Content)
	}
	if _, ok := b.events[1].Frame.(streamdomain.Delta); !ok {
		t.Fatalf("frame[1] not a Delta: %+v", b.events[1])
	}
	cl, ok := b.events[3].Frame.(streamdomain.Close)
	if !ok || cl.Result == nil || !strings.Contains(string(cl.Result.Content), "fnv_1") {
		t.Fatalf("frame[3] not a Close with result snapshot: %+v", b.events[3])
	}
	// every frame shares the same node id (open's mint).
	if b.events[1].ID != b.events[0].ID || b.events[3].ID != b.events[0].ID {
		t.Fatalf("frames not anchored to one node id")
	}
}

func TestWriter_NoOpWhenDisabled(t *testing.T) {
	// nil bridge.
	n, err := New(context.Background(), nil, streamdomain.Scope{Kind: "function", ID: "fn_x"}, "run", nil).Write([]byte("x"))
	if n != 1 || err != nil {
		t.Fatalf("disabled Write must report (1, nil); got (%d, %v)", n, err)
	}
	// empty scope id.
	b := &capBridge{}
	w := New(context.Background(), b, streamdomain.Scope{Kind: "function", ID: ""}, "run", nil)
	_, _ = w.Write([]byte("x"))
	w.Close("completed", nil)
	if len(b.events) != 0 {
		t.Fatalf("empty scope id → no frames, got %d", len(b.events))
	}
}

func TestSignal_PointNode(t *testing.T) {
	b := &capBridge{}
	scope := streamdomain.Scope{Kind: streamdomain.KindTrigger, ID: "trg_1"}
	Signal(context.Background(), b, scope, "fire", json.RawMessage(`{"flowrunId":"frn_9"}`), true)
	if len(b.events) != 1 {
		t.Fatalf("want 1 signal frame, got %d", len(b.events))
	}
	sig, ok := b.events[0].Frame.(streamdomain.Signal)
	if !ok || sig.Node.Type != "fire" || b.events[0].Scope.ID != "trg_1" {
		t.Fatalf("not a fire Signal scoped to the trigger: %+v", b.events[0])
	}
	// ephemeral=true → lossy, never buffered (E2/MD-sse1: the DB row is the reconnect truth).
	if !sig.Ephemeral || sig.Durable() {
		t.Fatalf("ephemeral signal must not be durable: %+v", sig)
	}
	// disabled.
	b2 := &capBridge{}
	Signal(context.Background(), nil, scope, "fire", nil, true)
	Signal(context.Background(), b2, streamdomain.Scope{Kind: "trigger", ID: ""}, "fire", nil, true)
	if len(b2.events) != 0 {
		t.Fatal("disabled Signal must emit nothing")
	}
}

func TestWithBridge_RoundTrip(t *testing.T) {
	if BridgeFrom(context.Background()) != nil {
		t.Fatal("no bridge → BridgeFrom nil")
	}
	b := &capBridge{}
	if BridgeFrom(WithBridge(context.Background(), b)) == nil {
		t.Fatal("WithBridge bridge not retrievable")
	}
	if BridgeFrom(WithBridge(context.Background(), nil)) != nil {
		t.Fatal("WithBridge(nil) must not set")
	}
}
