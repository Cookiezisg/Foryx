package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestProvidersHandler_List_AllCategories(t *testing.T) {
	h := NewProvidersHandler()
	mux := http.NewServeMux()
	h.Register(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/providers", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	var env struct {
		Data []providerInfo `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(env.Data) != 16 {
		t.Errorf("len = %d, want 16", len(env.Data))
	}
	hasLLM := false
	hasSearch := false
	for _, p := range env.Data {
		if p.Category == "llm" {
			hasLLM = true
		}
		if p.Category == "search" {
			hasSearch = true
		}
	}
	if !hasLLM || !hasSearch {
		t.Errorf("missing category coverage: hasLLM=%v hasSearch=%v", hasLLM, hasSearch)
	}
}

func TestProvidersHandler_List_FilterSearchOnly(t *testing.T) {
	h := NewProvidersHandler()
	mux := http.NewServeMux()
	h.Register(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/providers?category=search", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	var env struct {
		Data []providerInfo `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(env.Data) != 4 {
		t.Errorf("len = %d, want 4 (brave/serper/tavily/bocha)", len(env.Data))
	}
	for _, p := range env.Data {
		if p.Category != "search" {
			t.Errorf("non-search provider %q in search-filtered response", p.Name)
		}
	}
}

func TestProvidersHandler_List_FilterUnknownCategory_ReturnsEmpty(t *testing.T) {
	h := NewProvidersHandler()
	mux := http.NewServeMux()
	h.Register(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/providers?category=mystery", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	var env struct {
		Data []providerInfo `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(env.Data) != 0 {
		t.Errorf("len = %d, want 0 (unknown category)", len(env.Data))
	}
}

func TestProvidersHandler_List_StableAlphaOrder(t *testing.T) {
	h := NewProvidersHandler()
	mux := http.NewServeMux()
	h.Register(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/providers", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	var env struct {
		Data []providerInfo `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	for i := 1; i < len(env.Data); i++ {
		if env.Data[i-1].Name > env.Data[i].Name {
			t.Errorf("order broken at idx %d: %q > %q", i, env.Data[i-1].Name, env.Data[i].Name)
		}
	}
}
