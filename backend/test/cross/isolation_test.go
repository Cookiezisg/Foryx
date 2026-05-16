//go:build pipeline

package cross

import (
	"errors"
	"testing"

	apikeyapp "github.com/sunweilin/forgify/backend/internal/app/apikey"
	apikeydomain "github.com/sunweilin/forgify/backend/internal/domain/apikey"
	convdomain "github.com/sunweilin/forgify/backend/internal/domain/conversation"
	th "github.com/sunweilin/forgify/backend/test/harness"
)

func TestIsolation_Conversation_User2CannotDeleteUser1Conv(t *testing.T) {
	h := th.New(t)

	conv, err := h.Conversation.Create(th.LocalCtxAs("user-001"), "user-001 private conv")
	if err != nil {
		t.Fatalf("create as user-001: %v", err)
	}

	err = h.Conversation.Delete(th.LocalCtxAs("user-002"), conv.ID)
	if !errors.Is(err, convdomain.ErrNotFound) {
		t.Errorf("expected ErrNotFound for cross-user delete, got: %v", err)
	}
}

func TestIsolation_Conversation_User2ListSeesOnlyOwnData(t *testing.T) {
	h := th.New(t)

	ctx1 := th.LocalCtxAs("user-001")
	for i := range 2 {
		if _, err := h.Conversation.Create(ctx1, "conv"); err != nil {
			t.Fatalf("create conv %d: %v", i, err)
		}
	}

	items, _, err := h.Conversation.List(th.LocalCtxAs("user-002"), convdomain.ListFilter{Limit: 50})
	if err != nil {
		t.Fatalf("list as user-002: %v", err)
	}
	if len(items) != 0 {
		t.Errorf("user-002 sees %d conversations; should see 0 (all belong to user-001)",
			len(items))
	}
}

func TestIsolation_APIKey_User2ListSeesOnlyOwnData(t *testing.T) {
	h := th.New(t)

	if _, err := h.APIKey.Create(th.LocalCtxAs("user-001"), apikeyapp.CreateInput{
		Provider:    th.ProviderDeepSeek,
		DisplayName: "user-001 key",
		Key:         "sk-fake-u1",
	}); err != nil {
		t.Fatalf("create apikey as user-001: %v", err)
	}

	items, _, err := h.APIKey.List(th.LocalCtxAs("user-002"), apikeydomain.ListFilter{Limit: 50})
	if err != nil {
		t.Fatalf("list as user-002: %v", err)
	}
	if len(items) != 0 {
		t.Errorf("user-002 sees %d API keys; should see 0", len(items))
	}
}
