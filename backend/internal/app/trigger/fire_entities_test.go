package trigger

import (
	"context"
	"strings"
	"testing"

	streamdomain "github.com/sunweilin/forgify/backend/internal/domain/stream"
)

type recBridge struct{ events []streamdomain.Event }

func (b *recBridge) Publish(_ context.Context, e streamdomain.Event) (streamdomain.Envelope, error) {
	b.events = append(b.events, e)
	return streamdomain.Envelope{}, nil
}
func (b *recBridge) Subscribe(_ context.Context, _ int64) (<-chan streamdomain.Envelope, func(), error) {
	return nil, func() {}, nil
}

// TestFanOut_EmitsFireSignal: every fan-out (FireManual here; cron/webhook/fsnotify/sensor go
// through the same chokepoint) emits one fire Signal scoped to the trigger (SSE-C C5), carrying the
// activation id + fired/firingCount so the trigger panel shows activity live.
//
// TestFanOut_EmitsFireSignal：每次扇出（此处 FireManual；cron/webhook/fsnotify/sensor 走同一咽喉）发一条
// trigger scope 的 fire Signal（SSE-C C5），带 activation id + fired/firingCount，使 trigger 面板实时显示活动。
func TestFanOut_EmitsFireSignal(t *testing.T) {
	s, _ := newTestService(t)
	ent := &recBridge{}
	s.SetEntitiesBridge(ent)
	ctx := ctxWS("ws_1")
	tr := mkCron(t, s, ctx, "daily")

	if _, err := s.FireManual(ctx, tr.ID); err != nil {
		t.Fatalf("FireManual: %v", err)
	}

	if len(ent.events) != 1 {
		t.Fatalf("want 1 fire signal, got %d", len(ent.events))
	}
	e := ent.events[0]
	if e.Scope.Kind != streamdomain.KindTrigger || e.Scope.ID != tr.ID {
		t.Fatalf("fire not scoped to trigger:%s: %+v", tr.ID, e.Scope)
	}
	sig, ok := e.Frame.(streamdomain.Signal)
	if !ok || sig.Node.Type != "fire" {
		t.Fatalf("frame not a fire Signal: %+v", e.Frame)
	}
	body := string(sig.Node.Content)
	if !strings.Contains(body, `"fired":true`) || !strings.Contains(body, `"activationId":"tra_`) {
		t.Fatalf("fire content missing activation/fired: %s", body)
	}
}
