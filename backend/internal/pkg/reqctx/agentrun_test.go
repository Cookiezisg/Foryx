package reqctx

import (
	"context"
	"testing"
)

func TestSetGetConversationID_RoundTrip(t *testing.T) {
	ctx := WithConversationID(context.Background(), "cv_abc123")

	id, ok := GetConversationID(ctx)
	if !ok {
		t.Fatal("ok: got false, want true after WithConversationID")
	}
	if id != "cv_abc123" {
		t.Errorf("id: got %q, want \"cv_abc123\"", id)
	}
}

func TestGetConversationID_MissingReturnsFalse(t *testing.T) {
	id, ok := GetConversationID(context.Background())
	if ok {
		t.Errorf("ok: got true for empty ctx, want false")
	}
	if id != "" {
		t.Errorf("id: got %q, want empty", id)
	}
}

func TestGetConversationID_EmptyStringReturnsFalse(t *testing.T) {
	ctx := WithConversationID(context.Background(), "")
	id, ok := GetConversationID(ctx)
	if ok {
		t.Errorf("ok: got true for empty-string convID, want false")
	}
	if id != "" {
		t.Errorf("id: got %q, want empty", id)
	}
}

func TestSetGetMessageID_RoundTrip(t *testing.T) {
	ctx := WithMessageID(context.Background(), "msg_xyz")
	id, ok := GetMessageID(ctx)
	if !ok || id != "msg_xyz" {
		t.Errorf("got %q ok=%v, want \"msg_xyz\" ok=true", id, ok)
	}
}

func TestGetMessageID_MissingReturnsFalse(t *testing.T) {
	id, ok := GetMessageID(context.Background())
	if ok || id != "" {
		t.Errorf("got %q ok=%v, want empty/false", id, ok)
	}
}

func TestSetGetToolCallID_RoundTrip(t *testing.T) {
	ctx := WithToolCallID(context.Background(), "tc_001")
	id, ok := GetToolCallID(ctx)
	if !ok || id != "tc_001" {
		t.Errorf("got %q ok=%v, want \"tc_001\" ok=true", id, ok)
	}
}

func TestGetToolCallID_MissingReturnsFalse(t *testing.T) {
	id, ok := GetToolCallID(context.Background())
	if ok || id != "" {
		t.Errorf("got %q ok=%v, want empty/false", id, ok)
	}
}

func TestSetGetParentBlockID_RoundTrip(t *testing.T) {
	ctx := WithParentBlockID(context.Background(), "blk_42")
	id, ok := GetParentBlockID(ctx)
	if !ok || id != "blk_42" {
		t.Errorf("got %q ok=%v, want \"blk_42\" ok=true", id, ok)
	}
}

func TestGetParentBlockID_MissingReturnsFalse(t *testing.T) {
	id, ok := GetParentBlockID(context.Background())
	if ok || id != "" {
		t.Errorf("got %q ok=%v, want empty/false", id, ok)
	}
}

func TestAgentRunIDs_KeyIsolation(t *testing.T) {
	ctx := WithConversationID(context.Background(), "cv_only")

	if _, ok := GetMessageID(ctx); ok {
		t.Error("conversationID leaked into messageID slot")
	}
	if _, ok := GetToolCallID(ctx); ok {
		t.Error("conversationID leaked into toolCallID slot")
	}
	if _, ok := GetParentBlockID(ctx); ok {
		t.Error("conversationID leaked into parentBlockID slot")
	}
}

func TestAgentRunIDs_StackedRoundTrip(t *testing.T) {
	ctx := WithConversationID(context.Background(), "cv_1")
	ctx = WithMessageID(ctx, "msg_2")
	ctx = WithToolCallID(ctx, "tc_3")

	if id, _ := GetConversationID(ctx); id != "cv_1" {
		t.Errorf("convID: got %q, want \"cv_1\"", id)
	}
	if id, _ := GetMessageID(ctx); id != "msg_2" {
		t.Errorf("msgID: got %q, want \"msg_2\"", id)
	}
	if id, _ := GetToolCallID(ctx); id != "tc_3" {
		t.Errorf("tcID: got %q, want \"tc_3\"", id)
	}
}

func TestAgentRunIDs_PrivateKeyIsolation(t *testing.T) {
	//lint:ignore SA1029 intentional: simulating external code that uses a raw string key
	ctx := context.WithValue(context.Background(), "conversationID", "attacker")
	if _, ok := GetConversationID(ctx); ok {
		t.Error("string-keyed value leaked into private conversationID key")
	}
}

func TestSetWithConversationID_CopiesContext(t *testing.T) {
	parent := context.Background()
	_ = WithConversationID(parent, "child")

	if _, ok := GetConversationID(parent); ok {
		t.Error("parent ctx was mutated by WithConversationID")
	}
}
