package relation

import (
	"errors"
	"testing"

	relationdomain "github.com/sunweilin/forgify/backend/internal/domain/relation"
)

// TestGetRelations_Wiring: group shape + kind/id required (reuses the relation domain
// sentinel).
//
// TestGetRelations_Wiring：组形状 + kind/id 必填（复用 relation 域 sentinel）。
func TestGetRelations_Wiring(t *testing.T) {
	tools := RelationTools(nil)
	if len(tools) != 1 || tools[0].Name() != "get_relations" {
		t.Fatalf("group wrong: %v", tools)
	}
	for _, bad := range []string{`{}`, `{"kind":"function"}`, `{"id":"fn_1"}`} {
		if err := tools[0].ValidateInput([]byte(bad)); !errors.Is(err, relationdomain.ErrInvalidRef) {
			t.Fatalf("args %s must reject: %v", bad, err)
		}
	}
	if err := tools[0].ValidateInput([]byte(`{"kind":"function","id":"fn_1"}`)); err != nil {
		t.Fatalf("valid args rejected: %v", err)
	}
}
