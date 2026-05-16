package catalog

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestCatalog_JSONRoundTrip(t *testing.T) {
	in := Catalog{
		Summary:     "## Available capabilities\n- 5 forges...",
		Coverage:    map[string][]string{"forge": {"f_a", "f_b"}, "mcp": {"github"}},
		Fingerprint: "abc123",
		GeneratedAt: time.Date(2026, 5, 6, 13, 42, 0, 0, time.UTC),
		Version:     17,
		SourcesAt:   map[string]time.Time{"forge": time.Date(2026, 5, 6, 13, 42, 0, 0, time.UTC)},
		GeneratedBy: "llm",
	}
	b, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	wire := string(b)
	for _, want := range []string{
		`"summary":`, `"coverage":`, `"fingerprint":`,
		`"generatedAt":`, `"version":`, `"sourcesAt":`, `"generatedBy":`,
	} {
		if !strings.Contains(wire, want) {
			t.Errorf("wire missing field %s\nwire: %s", want, wire)
		}
	}

	var out Catalog
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if out.Version != 17 || out.GeneratedBy != "llm" {
		t.Errorf("round-trip lost data: %+v", out)
	}
	if len(out.Coverage["forge"]) != 2 || out.Coverage["mcp"][0] != "github" {
		t.Errorf("Coverage round-trip mangled: %v", out.Coverage)
	}
}

func TestGranularity_String(t *testing.T) {
	cases := []struct {
		g    Granularity
		want string
	}{
		{PerItem, "PerItem"},
		{PerServer, "PerServer"},
		{Granularity(99), "Unknown"},
	}
	for _, tc := range cases {
		if got := tc.g.String(); got != tc.want {
			t.Errorf("Granularity(%d).String() = %q, want %q", tc.g, got, tc.want)
		}
	}
}

func TestGranularity_EnumValuesStable(t *testing.T) {
	// PerItem must stay 0 so zero-value defaults to most permissive merging.
	// PerItem 必须保持 0，让零值落到最宽松合并选项。
	if PerItem != 0 {
		t.Errorf("PerItem = %d, want 0 (zero-value default)", PerItem)
	}
	if PerServer != 1 {
		t.Errorf("PerServer = %d, want 1", PerServer)
	}
}

func TestItem_JSONRoundTrip(t *testing.T) {
	in := Item{
		Source:      "forge",
		ID:          "f_abc",
		Name:        "csv-clean",
		Description: "Strip BOMs",
		Category:    "data",
	}
	b, _ := json.Marshal(in)
	var out Item
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if out != in {
		t.Errorf("round-trip mismatch: %+v vs %+v", out, in)
	}

	noCategory := Item{Source: "skill", ID: "x", Name: "y", Description: "z"}
	wire, _ := json.Marshal(noCategory)
	if strings.Contains(string(wire), `"category"`) {
		t.Errorf("empty Category should be omitted; wire = %s", wire)
	}
}

func TestSentinels_Distinct(t *testing.T) {
	if ErrCoverageIncomplete == nil || ErrGenerationFailed == nil {
		t.Fatal("nil sentinel")
	}
	if ErrCoverageIncomplete == ErrGenerationFailed {
		t.Errorf("sentinels collapsed; errors.Is at Service.Refresh switch-arm would be ambiguous")
	}
	for _, e := range []error{ErrCoverageIncomplete, ErrGenerationFailed} {
		if !strings.HasPrefix(e.Error(), "catalog: ") {
			t.Errorf("sentinel %q lacks 'catalog: ' prefix", e.Error())
		}
	}
}
