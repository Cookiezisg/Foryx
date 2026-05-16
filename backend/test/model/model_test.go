//go:build pipeline

// Package model runs end-to-end tests for /api/v1/model-configs/*.
//
// Package model 跑 /api/v1/model-configs/* 端到端测试。
package model

import (
	"net/http"
	"testing"

	th "github.com/sunweilin/forgify/backend/test/harness"
)

// putModelConfig is shorthand for PUT /api/v1/model-configs/{scenario}.
//
// putModelConfig 是 PUT /api/v1/model-configs/{scenario} 的简写。
func putModelConfig(t *testing.T, h *th.Harness, scenario, provider, modelID string, out any) int {
	t.Helper()
	return th.DoRequest(t, h, "PUT", "/api/v1/model-configs/"+scenario, map[string]any{
		"provider": provider,
		"modelId":  modelID,
	}, out)
}

func TestModel_UpsertAndList_Roundtrip(t *testing.T) {
	h := th.New(t)
	// model.Upsert now requires a matching api-key (green-save / red-runtime
	// guard); seed one before the PUT.
	// model.Upsert 现在要求有匹配 api-key；PUT 前种一个。
	h.SeedDeepSeek(t, "test-key")

	var resp struct {
		Data struct {
			ID       string `json:"id"`
			Scenario string `json:"scenario"`
			Provider string `json:"provider"`
			ModelID  string `json:"modelId"`
		} `json:"data"`
	}
	if s := putModelConfig(t, h, "chat", "deepseek", "deepseek-chat", &resp); s != http.StatusOK {
		t.Fatalf("PUT /model-configs/chat status=%d, want 200", s)
	}
	if resp.Data.ID == "" {
		t.Fatal("empty id in upsert response")
	}
	if resp.Data.Scenario != "chat" {
		t.Errorf("scenario=%q, want chat", resp.Data.Scenario)
	}
	if resp.Data.Provider != "deepseek" {
		t.Errorf("provider=%q, want deepseek", resp.Data.Provider)
	}
	if resp.Data.ModelID != "deepseek-chat" {
		t.Errorf("modelId=%q, want deepseek-chat", resp.Data.ModelID)
	}
	configID := resp.Data.ID

	var listResp struct {
		Data []struct {
			ID       string `json:"id"`
			Scenario string `json:"scenario"`
		} `json:"data"`
	}
	h.GetJSON("/api/v1/model-configs", &listResp)
	if len(listResp.Data) != 1 {
		t.Fatalf("list: got %d items, want 1", len(listResp.Data))
	}
	if listResp.Data[0].ID != configID {
		t.Errorf("list[0].id=%q, want %q", listResp.Data[0].ID, configID)
	}
}

func TestModel_Upsert_Idempotent_IDUnchanged(t *testing.T) {
	h := th.New(t)
	h.SeedDeepSeek(t, "test-key")

	first := struct {
		Data struct{ ID string `json:"id"` } `json:"data"`
	}{}
	if s := putModelConfig(t, h, "chat", "deepseek", "deepseek-chat", &first); s != http.StatusOK {
		t.Fatalf("first PUT status=%d", s)
	}

	second := struct {
		Data struct{ ID string `json:"id"` } `json:"data"`
	}{}
	if s := putModelConfig(t, h, "chat", "deepseek", "deepseek-reasoner", &second); s != http.StatusOK {
		t.Fatalf("second PUT status=%d", s)
	}

	if first.Data.ID != second.Data.ID {
		t.Errorf("ID changed: %q → %q", first.Data.ID, second.Data.ID)
	}
}

func TestModel_Upsert_InvalidScenario_Returns400(t *testing.T) {
	h := th.New(t)
	var errResp th.ErrEnvelope
	if s := putModelConfig(t, h, "not-a-real-scenario", "deepseek", "deepseek-chat", &errResp); s != http.StatusBadRequest {
		t.Errorf("status=%d, want 400", s)
	}
	if errResp.Error.Code != "INVALID_SCENARIO" {
		t.Errorf("error.code=%q, want INVALID_SCENARIO", errResp.Error.Code)
	}
}

func TestModel_Upsert_MissingProvider_Returns400(t *testing.T) {
	h := th.New(t)
	var errResp th.ErrEnvelope
	if s := putModelConfig(t, h, "chat", "", "deepseek-chat", &errResp); s != http.StatusBadRequest {
		t.Errorf("status=%d, want 400", s)
	}
	if errResp.Error.Code != "PROVIDER_REQUIRED" {
		t.Errorf("error.code=%q, want PROVIDER_REQUIRED", errResp.Error.Code)
	}
}
