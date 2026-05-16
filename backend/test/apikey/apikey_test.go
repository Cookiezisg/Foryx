//go:build pipeline

// Package apikey runs end-to-end tests for /api/v1/api-keys/* endpoints.
//
// Package apikey 跑 /api/v1/api-keys/* 端到端测试。
package apikey

import (
	"fmt"
	"net/http"
	"testing"

	th "github.com/sunweilin/forgify/backend/test/harness"
)

func TestAPIKey_CRUD_Roundtrip(t *testing.T) {
	h := th.New(t)

	var createResp struct {
		Data struct {
			ID          string `json:"id"`
			Provider    string `json:"provider"`
			DisplayName string `json:"displayName"`
			KeyMasked   string `json:"keyMasked"`
			TestStatus  string `json:"testStatus"`
		} `json:"data"`
	}
	h.PostJSON("/api/v1/api-keys", map[string]any{
		"provider":    "deepseek",
		"displayName": "my-key",
		"key":         "sk-fake-12345",
	}, &createResp)
	if createResp.Data.ID == "" {
		t.Fatal("create: empty id in response")
	}
	if createResp.Data.Provider != "deepseek" {
		t.Errorf("provider=%q, want deepseek", createResp.Data.Provider)
	}
	if createResp.Data.KeyMasked == "" {
		t.Error("keyMasked is empty; should be a masked representation")
	}
	if createResp.Data.TestStatus != "pending" {
		t.Errorf("testStatus=%q, want pending", createResp.Data.TestStatus)
	}
	keyID := createResp.Data.ID

	var listResp struct {
		Data    []struct{ ID string `json:"id"` } `json:"data"`
		HasMore *bool                              `json:"hasMore"`
	}
	h.GetJSON("/api/v1/api-keys", &listResp)
	if len(listResp.Data) != 1 {
		t.Fatalf("list: got %d items, want 1", len(listResp.Data))
	}
	if listResp.Data[0].ID != keyID {
		t.Errorf("list[0].id=%q, want %q", listResp.Data[0].ID, keyID)
	}
	if listResp.HasMore == nil || *listResp.HasMore {
		t.Error("hasMore should be false for a single-item list")
	}

	newName := "renamed-key"
	var updateResp struct {
		Data struct {
			DisplayName string `json:"displayName"`
		} `json:"data"`
	}
	h.PatchJSON("/api/v1/api-keys/"+keyID, map[string]any{"displayName": newName}, &updateResp)
	if updateResp.Data.DisplayName != newName {
		t.Errorf("displayName after PATCH=%q, want %q", updateResp.Data.DisplayName, newName)
	}

	h.Delete("/api/v1/api-keys/" + keyID)

	h.GetJSON("/api/v1/api-keys", &listResp)
	if len(listResp.Data) != 0 {
		t.Errorf("list after delete: got %d items, want 0", len(listResp.Data))
	}
}

func TestAPIKey_Create_InvalidProvider_Returns400(t *testing.T) {
	h := th.New(t)
	var errResp th.ErrEnvelope
	status := th.DoRequest(t, h, "POST", "/api/v1/api-keys", map[string]any{
		"provider": "alien-provider",
		"key":      "sk-fake",
	}, &errResp)
	if status != http.StatusBadRequest {
		t.Errorf("status=%d, want 400", status)
	}
	if errResp.Error.Code != "INVALID_PROVIDER" {
		t.Errorf("error.code=%q, want INVALID_PROVIDER", errResp.Error.Code)
	}
}

func TestAPIKey_Test_FakeServer_Success_Returns200(t *testing.T) {
	fake := th.NewFakeLLMServer(t)
	h := th.New(t)

	var createResp struct {
		Data struct{ ID string `json:"id"` } `json:"data"`
	}
	h.PostJSON("/api/v1/api-keys", map[string]any{
		"provider":    "deepseek",
		"displayName": "fake-conn-key",
		"key":         "sk-fake-conn",
		"baseUrl":     fake.URL(),
	}, &createResp)
	keyID := createResp.Data.ID

	var testResp struct {
		Data struct {
			OK          bool     `json:"ok"`
			Message     string   `json:"message"`
			ModelsFound []string `json:"modelsFound"`
		} `json:"data"`
	}
	h.PostJSON("/api/v1/api-keys/"+keyID+":test", nil, &testResp)
	if !testResp.Data.OK {
		t.Errorf("ok=false, want true; message=%q", testResp.Data.Message)
	}
	if len(testResp.Data.ModelsFound) == 0 {
		t.Error("modelsFound is empty; fake server should return 2 models")
	}
}

func TestAPIKey_Test_FakeServer_Auth401_Returns422(t *testing.T) {
	fake := th.NewFakeLLMServer(t)
	fake.SetModelsStatus(http.StatusUnauthorized)

	h := th.New(t)
	var createResp struct {
		Data struct{ ID string `json:"id"` } `json:"data"`
	}
	h.PostJSON("/api/v1/api-keys", map[string]any{
		"provider":    "deepseek",
		"displayName": "bad-key",
		"key":         "sk-bad",
		"baseUrl":     fake.URL(),
	}, &createResp)
	keyID := createResp.Data.ID

	var errResp th.ErrEnvelope
	status := th.DoRequest(t, h, "POST", "/api/v1/api-keys/"+keyID+":test", nil, &errResp)
	if status != http.StatusUnprocessableEntity {
		t.Errorf("status=%d, want 422", status)
	}
	if errResp.Error.Code != "API_KEY_TEST_FAILED" {
		t.Errorf("error.code=%q, want API_KEY_TEST_FAILED", errResp.Error.Code)
	}
}

func TestAPIKey_CursorPagination_ExhaustPages(t *testing.T) {
	h := th.New(t)

	for i := range 7 {
		h.PostJSON("/api/v1/api-keys", map[string]any{
			"provider":    "deepseek",
			"displayName": fmt.Sprintf("key-%02d", i),
			"key":         fmt.Sprintf("sk-fake-%02d", i),
		}, nil)
	}

	type pagedResp struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
		NextCursor *string `json:"nextCursor"`
		HasMore    *bool   `json:"hasMore"`
	}

	var cursor string
	totalSeen := 0
	for page := 0; ; page++ {
		url := "/api/v1/api-keys?limit=3"
		if cursor != "" {
			url += "&cursor=" + cursor
		}
		var resp pagedResp
		h.GetJSON(url, &resp)

		if len(resp.Data) == 0 {
			t.Fatalf("page %d: empty data before exhausting 7 items (total so far: %d)", page, totalSeen)
		}
		totalSeen += len(resp.Data)

		hasMore := resp.HasMore != nil && *resp.HasMore
		if hasMore {
			if resp.NextCursor == nil || *resp.NextCursor == "" {
				t.Fatalf("page %d: hasMore=true but nextCursor is empty", page)
			}
			cursor = *resp.NextCursor
		} else {
			break
		}
		if page > 5 {
			t.Fatal("pagination did not terminate after 5 pages (7 items, limit=3)")
		}
	}
	if totalSeen != 7 {
		t.Errorf("total items across pages=%d, want 7", totalSeen)
	}
}
