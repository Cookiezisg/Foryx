package scenarios

import (
	"testing"

	"github.com/sunweilin/anselm/testend/harness"
)

// Regression for F6 (iteration loop, 2026-06-18): an :edit carrying a set_meta op must update the
// entity ROW's name/description/tags — not only the version. Pre-fix, function/handler Edit moved
// the version pointer but silently dropped the rename (the agent "renamed" it, the backend didn't).
// Zero-token, deterministic. (workflow already did this right; agent/control/approval have no
// set_meta build op, so this pair is the whole bug surface.)
//
// F6 回归：带 set_meta 的 :edit 必须更新实体行的 name/description/tags、非只版本。修前 function/handler
// 的 Edit 移了版本指针却静默丢了改名。零 token、确定性。

func TestFunction_EditPersistsMeta(t *testing.T) {
	srv := harness.Start(t)
	c := srv.Client(t)
	ws := c.POST("/api/v1/workspaces", map[string]any{"name": "fn-editmeta"}).OK(t, nil)
	wc := c.WS(ws.Field(t, "id"))
	fnID := fnCreate(t, wc, "before_name", "def f() -> dict:\n    return {\"v\": 1}\n")

	wc.POST("/api/v1/functions/"+fnID+":edit", map[string]any{
		"ops": []map[string]any{
			{"op": "set_meta", "name": "after_name", "description": "new desc", "tags": []string{"renamed"}},
		},
	}).OK(t, nil)

	var got struct {
		Name        string   `json:"name"`
		Description string   `json:"description"`
		Tags        []string `json:"tags"`
	}
	wc.GET("/api/v1/functions/" + fnID).OK(t, &got)
	if got.Name != "after_name" {
		t.Fatalf("edit set_meta must rename the function row: name=%q want after_name", got.Name)
	}
	if got.Description != "new desc" {
		t.Errorf("edit set_meta must update description: got %q", got.Description)
	}
	if len(got.Tags) != 1 || got.Tags[0] != "renamed" {
		t.Errorf("edit set_meta must update tags: got %+v", got.Tags)
	}
}

func TestHandler_EditPersistsMeta(t *testing.T) {
	srv := harness.Start(t)
	c := srv.Client(t)
	ws := c.POST("/api/v1/workspaces", map[string]any{"name": "hd-editmeta"}).OK(t, nil)
	wc := c.WS(ws.Field(t, "id"))
	hdID := hdCreate(t, wc, "before_hd", map[string]any{
		"initBody": "self.x = 0",
		"methods":  []map[string]any{{"name": "ping", "inputs": []any{}, "body": "return {\"ok\": True}"}},
	})

	wc.POST("/api/v1/handlers/"+hdID+":edit", map[string]any{
		"ops": []map[string]any{
			{"op": "set_meta", "name": "after_hd", "description": "new hd desc"},
		},
	}).OK(t, nil)

	var got struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	wc.GET("/api/v1/handlers/" + hdID).OK(t, &got)
	if got.Name != "after_hd" {
		t.Fatalf("edit set_meta must rename the handler row: name=%q want after_hd", got.Name)
	}
	if got.Description != "new hd desc" {
		t.Errorf("edit set_meta must update description: got %q", got.Description)
	}
}
