package document

import (
	"testing"

	documentdomain "github.com/sunweilin/forgify/backend/internal/domain/document"
)

func TestTopLevelSegment(t *testing.T) {
	cases := map[string]string{
		"/Projects/2026/Q1": "Projects",
		"/Notes/daily":      "Notes",
		"/RootOnly":         "RootOnly",
		"/":                 "",
		"":                  "",
		"NoLeadingSlash":    "NoLeadingSlash",
	}
	for in, want := range cases {
		if got := topLevelSegment(in); got != want {
			t.Errorf("topLevelSegment(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCatalogSource_ListItems_ShapeAndCategory(t *testing.T) {
	s := newService(t)
	ctx := userCtx()
	root, _ := s.Create(ctx, documentdomain.CreateInput{
		Name:        "Projects",
		Description: "all projects",
	})
	_, _ = s.Create(ctx, documentdomain.CreateInput{
		Name:        "Q1-planning",
		ParentID:    &root.ID,
		Description: "quarterly plan",
	})
	_, _ = s.Create(ctx, documentdomain.CreateInput{
		Name:        "scratch",
		Description: "loose notes",
	})

	src := s.AsCatalogSource()
	if src.Name() != "document" {
		t.Errorf("Name() = %q, want %q", src.Name(), "document")
	}

	items, err := src.ListItems(ctx)
	if err != nil {
		t.Fatalf("ListItems: %v", err)
	}
	if len(items) != 3 {
		t.Fatalf("got %d items, want 3", len(items))
	}

	pathsByCat := map[string][]string{}
	for _, it := range items {
		if it.Source != "document" {
			t.Errorf("item Source = %q, want document", it.Source)
		}
		if it.ID == "" || it.Name == "" {
			t.Errorf("item missing ID or Name: %+v", it)
		}
		pathsByCat[it.Category] = append(pathsByCat[it.Category], it.Name)
	}
	// Items under "Projects" parent should share Category="Projects".
	//
	// "Projects" 父下 item 共享 Category="Projects"。
	if len(pathsByCat["Projects"]) != 2 {
		t.Errorf("Category=Projects should have 2 items; got %v", pathsByCat["Projects"])
	}
	if len(pathsByCat["scratch"]) != 1 {
		t.Errorf("root-only doc 'scratch' should self-group; got %v", pathsByCat)
	}
}

func TestCatalogSource_ItemDescription_FallsBackToTags(t *testing.T) {
	s := newService(t)
	ctx := userCtx()
	_, _ = s.Create(ctx, documentdomain.CreateInput{
		Name: "no-desc",
		Tags: []string{"a", "b"},
	})
	items, err := s.AsCatalogSource().ListItems(ctx)
	if err != nil {
		t.Fatalf("ListItems: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("got %d items", len(items))
	}
	if got := items[0].Description; got != "tags: a, b" {
		t.Errorf("tag fallback = %q", got)
	}
}

func TestCatalogSource_ItemDescription_FallsBackToParenthesized(t *testing.T) {
	s := newService(t)
	ctx := userCtx()
	_, _ = s.Create(ctx, documentdomain.CreateInput{Name: "naked"})
	items, _ := s.AsCatalogSource().ListItems(ctx)
	if items[0].Description != "(no description)" {
		t.Errorf("got %q", items[0].Description)
	}
}
