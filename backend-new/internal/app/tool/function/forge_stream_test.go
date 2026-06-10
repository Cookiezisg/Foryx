package function

import (
	"context"
	"strings"
	"testing"

	envfixapp "github.com/sunweilin/forgify/backend/internal/app/envfix"
	loopapp "github.com/sunweilin/forgify/backend/internal/app/loop"
	streamdomain "github.com/sunweilin/forgify/backend/internal/domain/stream"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

type capBridge struct{ events []streamdomain.Event }

func (b *capBridge) Publish(_ context.Context, e streamdomain.Event) (streamdomain.Envelope, error) {
	b.events = append(b.events, e)
	return streamdomain.Envelope{}, nil
}
func (b *capBridge) Subscribe(_ context.Context, _ int64) (<-chan streamdomain.Envelope, func(), error) {
	return nil, func() {}, nil
}

// TestForgeSink_AccumulatesAndStreams: the env-fix sink both folds attempts into the create/edit
// result (for the LLM) AND streams the install/repair narrative live as a progress block under the
// tool_call (for the user) — the same sink serves both, exactly as the package doc promised.
//
// TestForgeSink_AccumulatesAndStreams：env-fix sink 既把尝试折进 create/edit 结果（给 LLM），又把
// 装环境/修复叙事实时流成 tool_call 下的 progress 块（给用户）——同一 sink 两用，正如包注释所诺。
func TestForgeSink_AccumulatesAndStreams(t *testing.T) {
	b := &capBridge{}
	ctx := reqctxpkg.SetToolCallID(reqctxpkg.SetConversationID(loopapp.WithBridge(context.Background(), b), "c1"), "tc_create")

	sink := newForgeSink(ctx)
	sink.OnFixing(1)
	sink.OnAttempt(envfixapp.Attempt{Number: 1, OK: false, Error: "no module foo"})
	sink.OnAttempt(envfixapp.Attempt{Number: 2, OK: true})
	sink.Close()

	// accumulated for the result the LLM reads.
	if len(sink.attempts) != 2 {
		t.Fatalf("attempts not accumulated for result: got %d", len(sink.attempts))
	}
	// streamed under the tool_call for the user.
	open, ok := b.events[0].Frame.(streamdomain.Open)
	if !ok || open.ParentID != "tc_create" || open.Node.Type != "progress" {
		t.Fatalf("first frame not a progress Open under the tool_call: %+v", b.events[0])
	}
	var narrative strings.Builder
	for _, e := range b.events {
		if d, ok := e.Frame.(streamdomain.Delta); ok {
			narrative.WriteString(d.Chunk)
		}
	}
	s := narrative.String()
	if !strings.Contains(s, "revising deps") || !strings.Contains(s, "attempt 1 failed") || !strings.Contains(s, "env ready") {
		t.Fatalf("streamed progress missing the env-fix narrative: %q", s)
	}
}
