//go:build pipeline

package catalog

import (
	"context"
	"strings"
	"testing"

	handlerapp "github.com/sunweilin/forgify/backend/internal/app/handler"
	handlerdomain "github.com/sunweilin/forgify/backend/internal/domain/handler"
	th "github.com/sunweilin/forgify/backend/test/harness"
)

// createHandlerForCatalog builds a minimal handler via CreateDirect (metadata only).
//
// createHandlerForCatalog 经 CreateDirect 建最小 handler，只为元数据。
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
	if !contains(cat.Coverage["handler"], hd.ID) {
		t.Errorf("handler %q missing from coverage", hd.ID)
	}
}
