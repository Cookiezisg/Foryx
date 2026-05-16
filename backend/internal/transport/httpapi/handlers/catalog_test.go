package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"go.uber.org/zap/zaptest"

	catalogapp "github.com/sunweilin/forgify/backend/internal/app/catalog"
	catalogdomain "github.com/sunweilin/forgify/backend/internal/domain/catalog"
	middlewarehttpapi "github.com/sunweilin/forgify/backend/internal/transport/httpapi/middleware"
)

type stubCatalogSource struct {
	name  string
	gran  catalogdomain.Granularity
	items []catalogdomain.Item
}

func (s *stubCatalogSource) Name() string                            { return s.name }
func (s *stubCatalogSource) Granularity() catalogdomain.Granularity  { return s.gran }
func (s *stubCatalogSource) ListItems(_ context.Context) ([]catalogdomain.Item, error) {
	return append([]catalogdomain.Item(nil), s.items...), nil
}

type catalogHandlerHarness struct {
	srv *httptest.Server
	svc *catalogapp.Service
}

func newCatalogTestServer(t *testing.T) *catalogHandlerHarness {
	t.Helper()
	log := zaptest.NewLogger(t)
	svc := catalogapp.New(filepath.Join(t.TempDir(), ".catalog.json"), nil, log)
	hd := NewCatalogHandler(svc, log)
	mux := http.NewServeMux()
	hd.Register(mux)
	srv := httptest.NewServer(middlewarehttpapi.InjectUserID(mux))
	t.Cleanup(srv.Close)
	return &catalogHandlerHarness{srv: srv, svc: svc}
}

func TestCatalog_Get_NoCacheReturnsNullData(t *testing.T) {
	h := newCatalogTestServer(t)
	resp, err := http.Get(h.srv.URL + "/api/v1/catalog")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	got := envOf[*catalogdomain.Catalog](t, resp.Body)
	if got != nil {
		t.Errorf("expected null data on empty cache; got %+v", got)
	}
}

func TestCatalog_Refresh_BuildsAndReturnsCatalog(t *testing.T) {
	h := newCatalogTestServer(t)
	h.svc.RegisterSource(&stubCatalogSource{
		name: "forge",
		gran: catalogdomain.PerItem,
		items: []catalogdomain.Item{
			{Source: "forge", ID: "f_a", Name: "csv-clean", Description: "Strip BOMs"},
			{Source: "forge", ID: "f_b", Name: "csv-merge", Description: "Concat CSVs"},
		},
	})

	resp, err := http.Post(h.srv.URL+"/api/v1/catalog:refresh", "application/json", nil)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	got := envOf[*catalogdomain.Catalog](t, resp.Body)
	if got == nil {
		t.Fatal("Refresh returned nil Catalog")
	}
	if got.GeneratedBy != "mechanical-fallback" {
		t.Errorf("GeneratedBy = %q, want mechanical-fallback (no Generator wired)", got.GeneratedBy)
	}
	if len(got.Coverage["forge"]) != 2 {
		t.Errorf("Coverage[forge] = %v, want 2 items", got.Coverage["forge"])
	}
	if !strings.Contains(got.Summary, "csv-clean") || !strings.Contains(got.Summary, "csv-merge") {
		t.Errorf("Summary missing item names: %q", got.Summary)
	}
	if got.Fingerprint == "" {
		t.Errorf("Fingerprint empty after Refresh")
	}
	if got.Version != 1 {
		t.Errorf("Version = %d, want 1 on first Refresh", got.Version)
	}
}

func TestCatalog_GetAfterRefresh_ReturnsCachedSnapshot(t *testing.T) {
	h := newCatalogTestServer(t)
	h.svc.RegisterSource(&stubCatalogSource{
		name: "skill",
		gran: catalogdomain.PerItem,
		items: []catalogdomain.Item{
			{Source: "skill", ID: "deploy", Name: "deploy", Description: "deploy steps"},
		},
	})
	if err := h.svc.Refresh(context.Background()); err != nil {
		t.Fatalf("seed Refresh: %v", err)
	}

	resp, _ := http.Get(h.srv.URL + "/api/v1/catalog")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	got := envOf[*catalogdomain.Catalog](t, resp.Body)
	if got == nil || !strings.Contains(got.Summary, "deploy") {
		t.Errorf("GET after Refresh did not return cached snapshot; got %+v", got)
	}
}

func TestCatalog_Refresh_ShortCircuitsWhenFingerprintUnchanged(t *testing.T) {
	h := newCatalogTestServer(t)
	h.svc.RegisterSource(&stubCatalogSource{
		name: "forge",
		gran: catalogdomain.PerItem,
		items: []catalogdomain.Item{
			{Source: "forge", ID: "f", Name: "x", Description: "y"},
		},
	})
	first, _ := http.Post(h.srv.URL+"/api/v1/catalog:refresh", "application/json", nil)
	got1 := envOf[*catalogdomain.Catalog](t, first.Body)
	second, _ := http.Post(h.srv.URL+"/api/v1/catalog:refresh", "application/json", nil)
	got2 := envOf[*catalogdomain.Catalog](t, second.Body)

	if got1.Version != 1 {
		t.Errorf("Version after Refresh #1 = %d, want 1", got1.Version)
	}
	if got2.Version != 1 {
		t.Errorf("Version after Refresh #2 = %d, want 1 (fingerprint short-circuit)", got2.Version)
	}
	if got1.Fingerprint != got2.Fingerprint {
		t.Errorf("Fingerprint changed across no-op Refreshes: %q vs %q", got1.Fingerprint, got2.Fingerprint)
	}
}
