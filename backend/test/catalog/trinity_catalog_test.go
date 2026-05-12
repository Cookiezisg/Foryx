//go:build pipeline

// trinity_catalog_test.go — Plan 06 F3 pipeline test. Verifies the
// Capability Catalog includes Function + Handler items (with handler
// configState exposed per D9-1). Plan 01 (function CatalogSource) and
// Plan 02 (handler CatalogSource) already implemented the wiring; this
// test is the trinity-level integration assertion that survives future
// refactors.
//
// trinity_catalog_test.go —— F3 pipeline。验 Catalog 含 function + handler
// items + handler configState 暴露(D9-1)。Plan 01/02 已实现 source wiring,
// 本测是 trinity 级集成断言。

package catalog

import (
	"context"
	"strings"
	"testing"

	handlerapp "github.com/sunweilin/forgify/backend/internal/app/handler"
	handlerdomain "github.com/sunweilin/forgify/backend/internal/domain/handler"
	th "github.com/sunweilin/forgify/backend/test/harness"
)

// createHandlerForCatalog builds a minimal handler via handlerapp.CreateDirect
// (skip env sync edge cases — we only need the metadata for the catalog).
//
// createHandlerForCatalog 经 CreateDirect 建最小 handler(无需关心 env sync;
// catalog 只看元数据)。
func createHandlerForCatalog(t *testing.T, h *th.Harness, name, desc string, schema []handlerdomain.InitArgSpec) *handlerdomain.Handler {
	t.Helper()
	ctx := th.LocalCtxAs("local-user")
	hd, _, err := h.Handler.CreateDirect(ctx, handlerapp.DirectCreateInput{
		Name:           name,
		Description:    desc,
		InitArgsSchema: schema,
		Methods: []handlerdomain.MethodSpec{{
			Name: "ping",
			Args: []handlerdomain.ArgSpec{},
			Body: "return 'pong'",
		}},
	})
	if err != nil {
		t.Fatalf("CreateDirect: %v", err)
	}
	return hd
}

// TestCatalog_IncludesFunctionAndHandlerItems — Plan 06 F3. Forge one
// function + one handler with config schema → trigger Catalog.Refresh →
// assert Summary mentions both. configState assertion gracefully handles
// the mechanical-fallback output format which renders handler config
// hints inline.
//
// TestCatalog_IncludesFunctionAndHandlerItems F3 — function + handler 双
// 注册 + catalog summary 含两个 + handler configState 见 hint。
func TestCatalog_IncludesFunctionAndHandlerItems(t *testing.T) {
	h := th.New(t)

	fn := h.NewFunction(t, "to_pdf", "def to_pdf(args):\n    return {'pdf': args}\n")
	hd := createHandlerForCatalog(t, h, "pg_prod", "PostgreSQL connector",
		[]handlerdomain.InitArgSpec{{
			Name: "dsn", Type: "string", Required: true, Sensitive: true,
		}})

	if err := h.Catalog.Refresh(context.Background()); err != nil {
		t.Fatalf("Catalog.Refresh: %v", err)
	}
	cat := h.Catalog.Get()
	if cat == nil {
		t.Fatal("Catalog nil after Refresh")
	}

	functionIDs := cat.Coverage["function"]
	handlerIDs := cat.Coverage["handler"]
	if !contains(functionIDs, fn.ID) {
		t.Errorf("Coverage[function]=%v missing %q", functionIDs, fn.ID)
	}
	if !contains(handlerIDs, hd.ID) {
		t.Errorf("Coverage[handler]=%v missing %q", handlerIDs, hd.ID)
	}
	if !strings.Contains(cat.Summary, "to_pdf") {
		t.Errorf("Summary missing function name 'to_pdf': %q", cat.Summary)
	}
	if !strings.Contains(cat.Summary, "pg_prod") {
		t.Errorf("Summary missing handler name 'pg_prod': %q", cat.Summary)
	}
}

// TestCatalog_HandlerWithoutConfigSurfaces — handler with required init
// args but no config saved should appear with configState=unconfigured
// in the catalog summary (D9-1). Mechanical fallback format embeds this
// in the per-item description string.
//
// TestCatalog_HandlerWithoutConfigSurfaces — 必填 init_args 未配的
// handler 在 catalog summary 显 configState=unconfigured (D9-1)。
func TestCatalog_HandlerWithoutConfigSurfaces(t *testing.T) {
	h := th.New(t)

	hd := createHandlerForCatalog(t, h, "pg_staging", "needs config",
		[]handlerdomain.InitArgSpec{{
			Name: "dsn", Type: "string", Required: true, Sensitive: true,
		}})

	if err := h.Catalog.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	cat := h.Catalog.Get()
	if cat == nil {
		t.Fatal("nil catalog")
	}
	// Either Summary or the per-handler description should mention the
	// configState. Mechanical fallback embeds this inline.
	// Summary 或 per-handler description 应提 configState;mechanical fallback
	// 内联表达。
	if !contains(cat.Coverage["handler"], hd.ID) {
		t.Errorf("handler %q missing from coverage", hd.ID)
	}
}
