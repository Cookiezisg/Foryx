package loop

import (
	"context"
	"strings"
	"sync"
	"testing"

	messagesdomain "github.com/sunweilin/forgify/backend/internal/domain/messages"
	streamdomain "github.com/sunweilin/forgify/backend/internal/domain/stream"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

// captureBridge records every published Event so the progress frame sequence can be asserted.
type captureBridge struct {
	mu     sync.Mutex
	events []streamdomain.Event
}

func (b *captureBridge) Publish(_ context.Context, e streamdomain.Event) (streamdomain.Envelope, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.events = append(b.events, e)
	return streamdomain.Envelope{}, nil
}
func (b *captureBridge) Subscribe(_ context.Context, _ int64) (<-chan streamdomain.Envelope, func(), error) {
	return nil, func() {}, nil
}

// streamCtx seeds the ctx the way the loop does before a tool runs: messages Bridge + conversation
// anchor + the tool_call id this tool is executing under.
func streamCtx(b streamdomain.Bridge) context.Context {
	ctx := WithBridge(context.Background(), b)
	ctx = reqctxpkg.SetConversationID(ctx, "c1")
	return reqctxpkg.SetToolCallID(ctx, "tc1")
}

func TestToolProgress_StreamsUnderToolCall(t *testing.T) {
	b := &captureBridge{}
	prog := ToolProgress(streamCtx(b))
	prog.Print("installing deps...\n")
	prog.Print("retry 1\n")
	prog.Close()

	if len(b.events) != 4 {
		t.Fatalf("want 4 frames (open + 2 delta + close), got %d", len(b.events))
	}
	// open: a progress block nested under the tool_call, scoped to the conversation.
	open, ok := b.events[0].Frame.(streamdomain.Open)
	if !ok || open.ParentID != "tc1" || open.Node.Type != messagesdomain.BlockTypeProgress {
		t.Fatalf("frame[0] not a progress Open under tc1: %+v", b.events[0])
	}
	if b.events[0].Scope.Kind != streamdomain.KindConversation || b.events[0].Scope.ID != "c1" {
		t.Fatalf("progress not scoped to conversation:c1: %+v", b.events[0].Scope)
	}
	// the two writes are deltas under the same block id.
	if _, ok := b.events[1].Frame.(streamdomain.Delta); !ok {
		t.Fatalf("frame[1] not a Delta: %+v", b.events[1])
	}
	if b.events[1].ID != open.ParentID && b.events[1].ID == "" {
		t.Fatalf("delta not anchored to the progress block id")
	}
	// close carries the full snapshot (deltas are lossy; the snapshot is reconnect truth).
	cl, ok := b.events[3].Frame.(streamdomain.Close)
	if !ok || cl.Result == nil {
		t.Fatalf("frame[3] not a Close with snapshot: %+v", b.events[3])
	}
	if snap := string(cl.Result.Content); !strings.Contains(snap, "installing deps") || !strings.Contains(snap, "retry 1") {
		t.Fatalf("close snapshot missing accumulated text: %s", snap)
	}
}

// TestToolProgress_NoOpWhenNotStreaming: a tool that always calls ToolProgress stays correct when
// there is no messages Bridge or no tool_call anchor (REST / tests / non-streaming hosts) — every
// method is a silent no-op, and Write still satisfies io.Writer (full length, nil error).
func TestToolProgress_NoOpWhenNotStreaming(t *testing.T) {
	// No bridge in ctx → disabled.
	noBridge := reqctxpkg.SetToolCallID(reqctxpkg.SetConversationID(context.Background(), "c1"), "tc1")
	n, err := ToolProgress(noBridge).Write([]byte("x"))
	if n != 1 || err != nil {
		t.Fatalf("disabled Write must report (1, nil) for io.Writer; got (%d, %v)", n, err)
	}

	// Bridge present but no tool_call id → nothing to anchor under → no frames.
	b := &captureBridge{}
	noTC := reqctxpkg.SetConversationID(WithBridge(context.Background(), b), "c1")
	prog := ToolProgress(noTC)
	prog.Print("y")
	prog.Close()
	if len(b.events) != 0 {
		t.Fatalf("no tool_call id → no frames, got %d", len(b.events))
	}
}
