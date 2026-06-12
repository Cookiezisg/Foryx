package conversation

import (
	"errors"
	"testing"

	searchdomain "github.com/sunweilin/forgify/backend/internal/domain/search"
)

// TestSearchConversations_Wiring: group shape + query required (reuses the search domain
// sentinel — same physical violation, same wire code).
//
// TestSearchConversations_Wiring：组形状 + query 必填（复用 search 域 sentinel——同一物理
// 违例、同一 wire code）。
func TestSearchConversations_Wiring(t *testing.T) {
	tools := ConversationTools(nil)
	if len(tools) != 1 || tools[0].Name() != "search_conversations" {
		t.Fatalf("group wrong: %v", tools)
	}
	if err := tools[0].ValidateInput([]byte(`{"query":"  "}`)); !errors.Is(err, searchdomain.ErrQueryRequired) {
		t.Fatalf("blank query must reject: %v", err)
	}
	if err := tools[0].ValidateInput([]byte(`{"query":"上次的方案"}`)); err != nil {
		t.Fatalf("valid query rejected: %v", err)
	}
}
