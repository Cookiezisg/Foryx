package todo

import (
	"errors"
	"testing"

	toolapp "github.com/sunweilin/anselm/backend/internal/app/tool"
	tododomain "github.com/sunweilin/anselm/backend/internal/domain/todo"
)

// TestTodoWrite_Wiring: the group exposes exactly todo_write with the 5-method interface,
// and ValidateInput enforces the items field (nil ≠ []: [] clears, absent is a mistake).
//
// TestTodoWrite_Wiring：组里恰是 todo_write（5 方法接口）；ValidateInput 强制 items 字段
// （nil ≠ []：[] 清空、缺省是错误）。
func TestTodoWrite_Wiring(t *testing.T) {
	tools := TodoTools(nil)
	var tw toolapp.Tool
	for _, tl := range tools {
		if tl.Name() == "todo_write" {
			tw = tl
		}
	}
	if tw == nil {
		t.Fatalf("todo_write missing from group: %v", tools)
	}
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

// TestTodoTools_Names: the group is resident {todo_write, todo_read} — todo_read must be present
// (and resident, not lazy) so the read-back path exists without a search_tools hop (F39).
//
// TestTodoTools_Names：组是常驻 {todo_write, todo_read}——todo_read 必须在（且常驻、非懒），使读回
// 路径无需 search_tools 跳转即存在（F39）。
func TestTodoTools_Names(t *testing.T) {
	tools := TodoTools(nil)
	got := map[string]bool{}
	for _, tl := range tools {
		got[tl.Name()] = true
	}
	if len(tools) != 2 || !got["todo_write"] || !got["todo_read"] {
		t.Fatalf("group must be {todo_write, todo_read}, got %d: %v", len(tools), got)
	}
}
