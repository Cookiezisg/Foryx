package todo

import (
	"errors"
	"testing"

	tododomain "github.com/sunweilin/forgify/backend/internal/domain/todo"
)

// TestTodoWrite_Wiring: the group exposes exactly todo_write with the 5-method interface,
// and ValidateInput enforces the items field (nil ≠ []: [] clears, absent is a mistake).
//
// TestTodoWrite_Wiring：组里恰是 todo_write（5 方法接口）；ValidateInput 强制 items 字段
// （nil ≠ []：[] 清空、缺省是错误）。
func TestTodoWrite_Wiring(t *testing.T) {
	tools := TodoTools(nil)
	if len(tools) != 1 || tools[0].Name() != "todo_write" {
		t.Fatalf("group wrong: %v", tools)
	}
	tw := tools[0]
	if err := tw.ValidateInput([]byte(`{}`)); !errors.Is(err, tododomain.ErrItemsRequired) {
		t.Fatalf("absent items must reject: %v", err)
	}
	if err := tw.ValidateInput([]byte(`{"items":[]}`)); err != nil {
		t.Fatalf("[] must be valid (clears the list): %v", err)
	}
	if err := tw.ValidateInput([]byte(`{"items":[{"content":"x","status":"pending"}]}`)); err != nil {
		t.Fatalf("valid items rejected: %v", err)
	}
}
