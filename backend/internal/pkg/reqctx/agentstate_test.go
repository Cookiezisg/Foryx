package reqctx

import (
	"context"
	"testing"

	agentstatepkg "github.com/sunweilin/forgify/backend/internal/pkg/agentstate"
)

func TestWithAgentState_RoundTrip(t *testing.T) {
	state := &agentstatepkg.AgentState{}
	ctx := WithAgentState(context.Background(), state)

	got, ok := GetAgentState(ctx)
	if !ok {
		t.Fatal("expected ok=true after WithAgentState")
	}
	if got != state {
		t.Errorf("expected same pointer, got different one")
	}
}

func TestGetAgentState_EmptyCtxReturnsFalse(t *testing.T) {
	_, ok := GetAgentState(context.Background())
	if ok {
		t.Error("expected ok=false on empty ctx")
	}
}

func TestGetAgentState_NilPointerReturnsFalse(t *testing.T) {
	ctx := WithAgentState(context.Background(), nil)
	_, ok := GetAgentState(ctx)
	if ok {
		t.Error("expected ok=false when nil pointer was stored")
	}
}

func TestGetAgentState_StateUsable(t *testing.T) {
	state := &agentstatepkg.AgentState{}
	ctx := WithAgentState(context.Background(), state)

	got, _ := GetAgentState(ctx)
	got.MarkRead("/abs/path", 1234)

	if sz, ok := state.WasRead("/abs/path"); !ok || sz != 1234 {
		t.Errorf("expected size=1234 ok=true, got size=%d ok=%v", sz, ok)
	}
}
