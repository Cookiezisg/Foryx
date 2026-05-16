package reqctx

import (
	"context"
	"testing"
)

func TestSetGetUserID_RoundTrip(t *testing.T) {
	ctx := SetUserID(context.Background(), "alice-123")

	id, ok := GetUserID(ctx)
	if !ok {
		t.Fatal("ok: got false, want true after SetUserID")
	}
	if id != "alice-123" {
		t.Errorf("id: got %q, want \"alice-123\"", id)
	}
}

func TestGetUserID_MissingReturnsFalse(t *testing.T) {
	id, ok := GetUserID(context.Background())
	if ok {
		t.Errorf("ok: got true for empty ctx, want false")
	}
	if id != "" {
		t.Errorf("id: got %q, want empty", id)
	}
}

func TestGetUserID_EmptyStringReturnsFalse(t *testing.T) {
	ctx := SetUserID(context.Background(), "")
	id, ok := GetUserID(ctx)
	if ok {
		t.Errorf("ok: got true for empty-string userID, want false")
	}
	if id != "" {
		t.Errorf("id: got %q, want empty", id)
	}
}

func TestGetUserID_PrivateKeyIsolation(t *testing.T) {
	//lint:ignore SA1029 intentional: simulating external code that uses a raw string key
	ctx := context.WithValue(context.Background(), "userID", "attacker")
	id, ok := GetUserID(ctx)
	if ok {
		t.Errorf("string-keyed value leaked into private key: got id=%q", id)
	}
}

func TestSetUserID_CopiesContext(t *testing.T) {
	parent := context.Background()
	_ = SetUserID(parent, "child")

	id, ok := GetUserID(parent)
	if ok || id != "" {
		t.Errorf("parent ctx was mutated: id=%q ok=%v", id, ok)
	}
}

func TestDefaultLocalUserID_IsNotEmpty(t *testing.T) {
	if DefaultLocalUserID == "" {
		t.Error("DefaultLocalUserID should never be empty")
	}
}
