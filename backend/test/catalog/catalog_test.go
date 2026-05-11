//go:build pipeline

// catalog_test.go — pipeline tests for the Capability Catalog. Three offline
// scenarios drive the full registration → poll → fingerprint → mechanical-
// fallback path with real function / skill / mcp services wired through the
// harness's CatalogSource adapters (D8-4).
//
// Scenarios per catalog.md §11:
//
//  1. AllSourcesCovered_E2E
//     Seed 1 function + 1 skill via the harness services → call
//     Catalog.Refresh → assert Coverage map includes IDs from both sources +
//     Summary contains both names + chat's next system prompt carries the
//     catalog block.
//
//  2. FunctionDescriptionChange_TriggersRegen
//     Seed function with description 'v1' → Refresh (Version=1) → update
//     function.Description to 'v2' → Refresh again → assert Version=2,
//     fingerprint changed, Summary now contains 'v2'.
//
//  3. NoLLMKey_FallsBackToMechanical
//     Harness wires LLMGenerator but no apikey is seeded → Generator fails
//     LLM resolve → Service.Refresh switches to mechanical fallback → assert
//     GeneratedBy='mechanical-fallback' + lastFP still updates (catalog.md §3
//     'user-activity-driven retry' invariant — next tick won't re-call LLM
//     until source data actually changes).
//
// All three are offline (no LLM / no network).
//
// catalog_test.go —— Capability Catalog pipeline 测试。3 个离线场景驱动
// 注册 → 轮询 → fingerprint → mechanical-fallback 全路径,经 harness
// CatalogSource 适配器接 function / skill / mcp 真服务。
package catalog

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	functionapp "github.com/sunweilin/forgify/backend/internal/app/function"
	th "github.com/sunweilin/forgify/backend/test/harness"
)

// seedSkillForCatalog writes a SKILL.md to harness's SkillsDir and triggers
// Service.Scan so the Skill source's ListItems returns it on the next catalog
// Refresh.
//
// seedSkillForCatalog 写 SKILL.md 到 harness SkillsDir + 调 Service.Scan 让
// Skill source 下次 catalog Refresh 时返。
func seedSkillForCatalog(t *testing.T, h *th.Harness, name, desc string) {
	t.Helper()
	dir := filepath.Join(h.Skill.SkillsDir(), name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir skill dir: %v", err)
	}
	content := "---\nname: " + name + "\ndescription: " + desc + "\n---\n# Body\nrun.\n"
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("write SKILL.md: %v", err)
	}
	if err := h.Skill.Scan(context.Background()); err != nil {
		t.Fatalf("Skill.Scan: %v", err)
	}
}

// ── 1. all 3 sources end-to-end ─────────────────────────────────────

func TestCatalog_AllSourcesCovered_E2E(t *testing.T) {
	h := th.New(t)

	fn := h.NewFunction(t, "csv-clean", "def csv_clean(args):\n    return args\n")
	seedSkillForCatalog(t, h, "deploy", "Deploy via internal CI")

	if err := h.Catalog.Refresh(context.Background()); err != nil {
		t.Fatalf("Catalog.Refresh: %v", err)
	}

	cat := h.Catalog.Get()
	if cat == nil {
		t.Fatal("Catalog still nil after Refresh")
	}
	if cat.GeneratedBy != "mechanical-fallback" && cat.GeneratedBy != "llm" {
		t.Errorf("unexpected GeneratedBy=%q", cat.GeneratedBy)
	}

	functionIDs := cat.Coverage["function"]
	skillIDs := cat.Coverage["skill"]
	if !contains(functionIDs, fn.ID) {
		t.Errorf("Coverage[function]=%v missing function ID %q", functionIDs, fn.ID)
	}
	if !contains(skillIDs, "deploy") {
		t.Errorf("Coverage[skill]=%v missing 'deploy'", skillIDs)
	}

	if !strings.Contains(cat.Summary, "csv-clean") {
		t.Errorf("Summary missing function name: %q", cat.Summary)
	}
	if !strings.Contains(cat.Summary, "deploy") {
		t.Errorf("Summary missing skill name: %q", cat.Summary)
	}

	if got := h.Catalog.GetForSystemPrompt(); got != cat.Summary {
		t.Errorf("GetForSystemPrompt mismatch with Get().Summary")
	}
}

// ── 2. description change triggers regen ────────────────────────────

func TestCatalog_FunctionDescriptionChange_TriggersRegen(t *testing.T) {
	h := th.New(t)

	fn := h.NewFunction(t, "describe-me", "def describe_me(a):\n    return a\n")

	if err := h.Catalog.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh #1: %v", err)
	}
	first := h.Catalog.Get()
	versionFirst := first.Version
	fpFirst := first.Fingerprint

	newDesc := "VERSION-TWO description for fingerprint test"
	updated, err := h.Function.UpdateMeta(h.LocalCtx(), functionapp.UpdateMetaInput{
		ID:          fn.ID,
		Description: &newDesc,
	})
	if err != nil {
		t.Fatalf("Function.UpdateMeta: %v", err)
	}
	if updated.Description != newDesc {
		t.Fatalf("Description not updated; got %q", updated.Description)
	}

	if err := h.Catalog.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh #2: %v", err)
	}
	second := h.Catalog.Get()
	if second.Version <= versionFirst {
		t.Errorf("Version after #2 = %d, want > %d (description change should bust fingerprint)",
			second.Version, versionFirst)
	}
	if second.Fingerprint == fpFirst {
		t.Errorf("Fingerprint unchanged after description edit: %q", second.Fingerprint)
	}
	if !strings.Contains(second.Summary, "VERSION-TWO") {
		t.Errorf("Summary did not pick up new description; got %q", second.Summary)
	}
}

// ── 3. mechanical fallback when LLM unavailable ────────────────────

func TestCatalog_NoLLMKey_FallsBackToMechanical(t *testing.T) {
	h := th.New(t)
	h.NewFunction(t, "alpha", "def alpha(a):\n    return a\n")

	if err := h.Catalog.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	cat := h.Catalog.Get()
	if cat == nil {
		t.Fatal("Catalog nil after Refresh; mechanical fallback should have produced one")
	}
	if cat.GeneratedBy != "mechanical-fallback" {
		t.Errorf("GeneratedBy=%q, want mechanical-fallback (LLM unavailable)", cat.GeneratedBy)
	}
	if cat.Fingerprint == "" {
		t.Errorf("Fingerprint empty; lastFP didn't update")
	}
	if !strings.Contains(cat.Summary, "alpha") {
		t.Errorf("mechanical Summary missing seeded function name: %q", cat.Summary)
	}

	versionAfterFirst := cat.Version
	if err := h.Catalog.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh #2: %v", err)
	}
	if h.Catalog.Get().Version != versionAfterFirst {
		t.Errorf("Version after no-op Refresh #2 = %d, want %d (lastFP short-circuit)",
			h.Catalog.Get().Version, versionAfterFirst)
	}
}

// ── helpers ─────────────────────────────────────────────────────────

func contains(xs []string, want string) bool {
	for _, x := range xs {
		if x == want {
			return true
		}
	}
	return false
}
