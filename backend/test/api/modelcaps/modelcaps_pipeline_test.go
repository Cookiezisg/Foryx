//go:build pipeline

// Package modelcaps runs end-to-end tests for /api/v1/model-capabilities/*.
//
// Package modelcaps 跑 /api/v1/model-capabilities/* 端到端测试。
package modelcaps

import (
	"net/http"
	"testing"

	th "github.com/sunweilin/forgify/backend/test/harness"
)

// seedVerifiedDeepSeek seeds a deepseek api key with TestStatus=ok and
// ModelsFound=["deepseek-v4-pro"] so the capabilities List endpoint returns
// resolved entries. SeedDeepSeek only creates the key (TestStatus=pending);
// this helper promotes it.
//
// seedVerifiedDeepSeek 把 SeedDeepSeek 种入的 key 标记为 ok + 设 modelsFound,
// 让 capabilities List 能返回解析后的条目。
func seedVerifiedDeepSeek(t *testing.T, h *th.Harness) string {
	t.Helper()
	keyID := h.SeedDeepSeek(t, "test-key")
	if err := h.DB.Exec(
		`UPDATE api_keys SET test_status = 'ok', models_found = '["deepseek-v4-pro"]'
		 WHERE id = ?`,
		keyID,
	).Error; err != nil {
		t.Fatalf("seedVerifiedDeepSeek: update test_status: %v", err)
	}
	return keyID
}

// covers: GET /api/v1/model-capabilities
func TestModelCaps_List_VerifiedKeyReturnsStaticCap(t *testing.T) {
	h := th.New(t)
	seedVerifiedDeepSeek(t, h)

	var resp struct {
		Data []struct {
			Provider      string   `json:"provider"`
			ModelID       string   `json:"modelId"`
			ThinkingShape string   `json:"thinkingShape"`
			EffortValues  []string `json:"effortValues"`
			ContextWindow int      `json:"contextWindow"`
			MaxOutput     int      `json:"maxOutput"`
		} `json:"data"`
	}
	h.GetJSON("/api/v1/model-capabilities", &resp)

	if len(resp.Data) != 1 {
		t.Fatalf("list: got %d items, want 1", len(resp.Data))
	}
	item := resp.Data[0]
	if item.Provider != "deepseek" {
		t.Errorf("provider=%q, want deepseek", item.Provider)
	}
	if item.ModelID != "deepseek-v4-pro" {
		t.Errorf("modelId=%q, want deepseek-v4-pro", item.ModelID)
	}
	// deepseek-v4-pro matches prefix "deepseek-v4" → ShapeEffort
	if item.ThinkingShape != "effort" {
		t.Errorf("thinkingShape=%q, want effort", item.ThinkingShape)
	}
	// Static cap: contextWindow=1_000_000
	if item.ContextWindow != 1_000_000 {
		t.Errorf("contextWindow=%d, want 1000000", item.ContextWindow)
	}
}

// covers: PUT /api/v1/model-capabilities/{provider}/{modelId}
// covers: GET /api/v1/model-capabilities
func TestModelCaps_Override_SetAndListShowsOverriddenValues(t *testing.T) {
	h := th.New(t)
	seedVerifiedDeepSeek(t, h)

	newContextWindow := 256000
	newThinkingShape := "none"
	var putResp struct {
		Data struct {
			ThinkingShape *string `json:"thinkingShape"`
			ContextWindow *int    `json:"contextWindow"`
		} `json:"data"`
	}
	putStatus := th.DoRequest(t, h, "PUT",
		"/api/v1/model-capabilities/deepseek/deepseek-v4-pro",
		map[string]any{
			"thinkingShape": newThinkingShape,
			"contextWindow": newContextWindow,
		},
		&putResp,
	)
	if putStatus != http.StatusOK {
		t.Fatalf("PUT status=%d, want 200", putStatus)
	}
	if putResp.Data.ThinkingShape == nil || *putResp.Data.ThinkingShape != "none" {
		t.Errorf("PUT response thinkingShape=%v, want none", putResp.Data.ThinkingShape)
	}
	if putResp.Data.ContextWindow == nil || *putResp.Data.ContextWindow != 256000 {
		t.Errorf("PUT response contextWindow=%v, want 256000", putResp.Data.ContextWindow)
	}

	// GET must reflect the override.
	var getResp struct {
		Data []struct {
			ThinkingShape string `json:"thinkingShape"`
			ContextWindow int    `json:"contextWindow"`
		} `json:"data"`
	}
	h.GetJSON("/api/v1/model-capabilities", &getResp)
	if len(getResp.Data) != 1 {
		t.Fatalf("list after PUT: got %d items, want 1", len(getResp.Data))
	}
	item := getResp.Data[0]
	if item.ThinkingShape != "none" {
		t.Errorf("thinkingShape after override=%q, want none", item.ThinkingShape)
	}
	if item.ContextWindow != 256000 {
		t.Errorf("contextWindow after override=%d, want 256000", item.ContextWindow)
	}
}

// covers: DELETE /api/v1/model-capabilities/{provider}/{modelId}
func TestModelCaps_Delete_ClearsOverrideRestoresStatic(t *testing.T) {
	h := th.New(t)
	seedVerifiedDeepSeek(t, h)

	// Seed an override first.
	putStatus := th.DoRequest(t, h, "PUT",
		"/api/v1/model-capabilities/deepseek/deepseek-v4-pro",
		map[string]any{"thinkingShape": "none", "contextWindow": 256000},
		nil,
	)
	if putStatus != http.StatusOK {
		t.Fatalf("PUT status=%d, want 200", putStatus)
	}

	// DELETE clears it.
	delStatus := th.DoRequest(t, h, "DELETE",
		"/api/v1/model-capabilities/deepseek/deepseek-v4-pro",
		nil, nil,
	)
	if delStatus != http.StatusNoContent {
		t.Fatalf("DELETE status=%d, want 204", delStatus)
	}

	// GET must return static capability again.
	var getResp struct {
		Data []struct {
			ThinkingShape string `json:"thinkingShape"`
			ContextWindow int    `json:"contextWindow"`
		} `json:"data"`
	}
	h.GetJSON("/api/v1/model-capabilities", &getResp)
	if len(getResp.Data) != 1 {
		t.Fatalf("list after DELETE: got %d items, want 1", len(getResp.Data))
	}
	item := getResp.Data[0]
	if item.ThinkingShape != "effort" {
		t.Errorf("thinkingShape after delete=%q, want effort (static)", item.ThinkingShape)
	}
	if item.ContextWindow != 1_000_000 {
		t.Errorf("contextWindow after delete=%d, want 1000000 (static)", item.ContextWindow)
	}
}

// covers: PUT /api/v1/model-capabilities/{provider}/{modelId}
// INVALID_THINKING_SHAPE is an inline error (not in errTable); only the endpoint is annotated.
func TestModelCaps_Override_InvalidThinkingShape_Returns400(t *testing.T) {
	h := th.New(t)

	var errResp th.ErrEnvelope
	status := th.DoRequest(t, h, "PUT",
		"/api/v1/model-capabilities/deepseek/deepseek-v4-pro",
		map[string]any{"thinkingShape": "bogus"},
		&errResp,
	)
	if status != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400", status)
	}
	if errResp.Error.Code != "INVALID_THINKING_SHAPE" {
		t.Errorf("error.code=%q, want INVALID_THINKING_SHAPE", errResp.Error.Code)
	}
}
