package mcp

import (
	"context"
	"errors"
	"strings"
	"testing"

	mcpdomain "github.com/sunweilin/forgify/backend/internal/domain/mcp"
)

func TestCurated_ListCount(t *testing.T) {
	if got := len(curatedEntries); got != 21 {
		t.Errorf("curatedEntries count = %d, want 21 — update both the catalog and this guard if changing intentionally", got)
	}
}

func TestCurated_AllEntriesValid(t *testing.T) {
	seen := map[string]bool{}
	validRuntime := map[string]bool{"node": true, "python": true}

	for _, e := range curatedEntries {
		if e.Name == "" {
			t.Errorf("entry has empty Name: %+v", e)
			continue
		}
		if seen[e.Name] {
			t.Errorf("duplicate Name %q", e.Name)
		}
		seen[e.Name] = true

		if e.Description == "" {
			t.Errorf("%s: empty Description", e.Name)
		}
		if !validRuntime[e.Runtime] {
			t.Errorf("%s: invalid Runtime %q (want node or python)", e.Name, e.Runtime)
		}
		if e.InstallCmd.Command == "" {
			t.Errorf("%s: empty InstallCmd.Command", e.Name)
		}
		if len(e.InstallCmd.Args) == 0 {
			t.Errorf("%s: empty InstallCmd.Args", e.Name)
		}
		if e.Category == "" {
			t.Errorf("%s: empty Category", e.Name)
		}
		if e.Tier < 0 || e.Tier > 3 {
			t.Errorf("%s: Tier %d out of range [0,3]", e.Name, e.Tier)
		}
		for _, env := range e.RequiredEnv {
			if env.Name == "" {
				t.Errorf("%s: RequiredEnv has empty Name", e.Name)
			}
			if env.Description == "" {
				t.Errorf("%s: RequiredEnv %s has empty Description", e.Name, env.Name)
			}
			if e.Tier >= 1 && env.SetupURL == "" {
				t.Errorf("%s: Tier %d but RequiredEnv %s lacks SetupURL", e.Name, e.Tier, env.Name)
			}
		}
		if e.Tier == 2 && len(e.RequiredEnv) == 0 && e.Notes == "" {
			t.Errorf("%s: OAuth-tier entry lacks Notes describing the auth flow", e.Name)
		}
	}
}

func TestCurated_RuntimeMix(t *testing.T) {
	for _, e := range curatedEntries {
		if e.Runtime != "node" && e.Runtime != "python" {
			t.Errorf("%s: runtime %q breaks the node+python-only invariant", e.Name, e.Runtime)
		}
	}
}

func TestCurated_NewSourceWiresAllEntries(t *testing.T) {
	src := NewCuratedRegistrySource()
	if got := len(src.all); got != len(curatedEntries) {
		t.Errorf("src.all = %d, want %d", got, len(curatedEntries))
	}
	if got := len(src.byName); got != len(curatedEntries) {
		t.Errorf("src.byName = %d, want %d", got, len(curatedEntries))
	}
	for _, e := range curatedEntries {
		if _, ok := src.byName[e.Name]; !ok {
			t.Errorf("byName missing %q", e.Name)
		}
	}
}

func TestCurated_List_AllEntriesReturned(t *testing.T) {
	src := NewCuratedRegistrySource()
	got, err := src.List(context.Background())
	if err != nil {
		t.Fatalf("List err: %v", err)
	}
	if len(got) != len(curatedEntries) {
		t.Errorf("List len = %d, want %d", len(got), len(curatedEntries))
	}
	seen := make(map[string]int, len(got))
	for _, e := range got {
		seen[e.Name]++
	}
	for _, e := range curatedEntries {
		if seen[e.Name] != 1 {
			t.Errorf("entry %q appeared %d times, want 1", e.Name, seen[e.Name])
		}
	}
}

func TestCurated_List_SortedByTierThenName(t *testing.T) {
	src := NewCuratedRegistrySource()
	got, _ := src.List(context.Background())
	for i := 1; i < len(got); i++ {
		prev, cur := got[i-1], got[i]
		if prev.Tier > cur.Tier {
			t.Errorf("tier order broken: %s(tier=%d) before %s(tier=%d)",
				prev.Name, prev.Tier, cur.Name, cur.Tier)
		}
		if prev.Tier == cur.Tier && prev.Name > cur.Name {
			t.Errorf("name order broken within tier %d: %s before %s",
				prev.Tier, prev.Name, cur.Name)
		}
	}
}

func TestCurated_List_ReturnsCopy(t *testing.T) {
	src := NewCuratedRegistrySource()
	got, _ := src.List(context.Background())
	if len(got) == 0 {
		t.Fatal("List returned 0 entries")
	}
	original := got[0].Name
	got[0].Name = "tampered"
	got2, _ := src.List(context.Background())
	if got2[0].Name != original {
		t.Errorf("internal state mutated: got2[0].Name = %q, want %q", got2[0].Name, original)
	}
}

func TestCurated_Get_KnownAndUnknown(t *testing.T) {
	src := NewCuratedRegistrySource()
	e, err := src.Get(context.Background(), "playwright")
	if err != nil {
		t.Fatalf("Get(playwright): %v", err)
	}
	if e == nil || e.Name != "playwright" {
		t.Errorf("got %+v", e)
	}

	_, err = src.Get(context.Background(), "definitely-not-a-real-server")
	if !errors.Is(err, mcpdomain.ErrRegistryEntryNotFound) {
		t.Errorf("Get(unknown) err = %v, want ErrRegistryEntryNotFound", err)
	}
}

func TestCurated_NotesPresentForGotchas(t *testing.T) {
	src := NewCuratedRegistrySource()
	mustContain := map[string]string{
		"playwright":      "Chromium",
		"chrome-devtools": "Chrome",
		"notion":          "SHARE",
		"google-workspace": "Cloud Console",
		"ms365":            "devicelogin",
	}
	for name, want := range mustContain {
		e, err := src.Get(context.Background(), name)
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if !strings.Contains(strings.ToLower(e.Notes), strings.ToLower(want)) {
			t.Errorf("%s Notes does not contain %q. Notes=%q", name, want, e.Notes)
		}
	}
}
