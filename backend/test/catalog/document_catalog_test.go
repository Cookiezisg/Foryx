//go:build pipeline

package catalog

import (
	"context"
	"strings"
	"testing"

	documentapp "github.com/sunweilin/forgify/backend/internal/app/document"
	th "github.com/sunweilin/forgify/backend/test/harness"
)

// TestCatalog_DocumentSource_E2E — seed a small doc tree, refresh catalog,
// verify Coverage["document"] + Summary path-grouping.
//
// TestCatalog_DocumentSource_E2E —— 种小文档树,refresh catalog,
// 校验 Coverage["document"] + Summary 含 path 分组。
func TestCatalog_DocumentSource_E2E(t *testing.T) {
	h := th.New(t)
	ctx := h.LocalCtx()

	projects, err := h.Document.Create(ctx, documentapp.CreateInput{
		Name:        "Projects",
		Description: "Top-level project folder",
	})
	if err != nil {
		t.Fatalf("seed Projects: %v", err)
	}
	q1, err := h.Document.Create(ctx, documentapp.CreateInput{
		Name:        "Q1-planning",
		ParentID:    &projects.ID,
		Description: "Quarterly planning notes",
	})
	if err != nil {
		t.Fatalf("seed Q1: %v", err)
	}
	notes, err := h.Document.Create(ctx, documentapp.CreateInput{
		Name:        "scratchpad",
		Description: "Loose ideas",
	})
	if err != nil {
		t.Fatalf("seed scratchpad: %v", err)
	}

	if err := h.Catalog.Refresh(context.Background()); err != nil {
		t.Fatalf("Catalog.Refresh: %v", err)
	}
	cat := h.Catalog.Get()
	if cat == nil {
		t.Fatal("Catalog nil after Refresh")
	}

	docIDs := cat.Coverage["document"]
	if !contains(docIDs, projects.ID) || !contains(docIDs, q1.ID) || !contains(docIDs, notes.ID) {
		t.Errorf("Coverage[document]=%v missing one of the seeded IDs", docIDs)
	}

	if !strings.Contains(cat.Summary, "Projects") {
		t.Errorf("Summary missing Projects: %s", cat.Summary)
	}
	if !strings.Contains(cat.Summary, "Q1-planning") {
		t.Errorf("Summary missing Q1-planning: %s", cat.Summary)
	}
	if !strings.Contains(cat.Summary, "scratchpad") {
		t.Errorf("Summary missing scratchpad: %s", cat.Summary)
	}
}
