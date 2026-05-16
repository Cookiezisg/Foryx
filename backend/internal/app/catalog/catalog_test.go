package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/zap/zaptest"

	catalogdomain "github.com/sunweilin/forgify/backend/internal/domain/catalog"
)

type fakeSource struct {
	mu        sync.Mutex
	name      string
	gran      catalogdomain.Granularity
	items     []catalogdomain.Item
	listErr   error
	callCount int32
}

func (f *fakeSource) Name() string                            { return f.name }
func (f *fakeSource) Granularity() catalogdomain.Granularity  { return f.gran }
func (f *fakeSource) ListItems(_ context.Context) ([]catalogdomain.Item, error) {
	atomic.AddInt32(&f.callCount, 1)
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.listErr != nil {
		return nil, f.listErr
	}
	out := make([]catalogdomain.Item, len(f.items))
	copy(out, f.items)
	return out, nil
}

func (f *fakeSource) setItems(items []catalogdomain.Item) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.items = items
}

func (f *fakeSource) setErr(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.listErr = err
}

type stubGenerator struct {
	mu       sync.Mutex
	called   int
	returnErr error
	summary  string
}

func (g *stubGenerator) Generate(_ context.Context, items []catalogdomain.Item, _ map[string]catalogdomain.Granularity) (*catalogdomain.Catalog, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.called++
	if g.returnErr != nil {
		return nil, g.returnErr
	}
	cov := map[string][]string{}
	for _, it := range items {
		cov[it.Source] = append(cov[it.Source], it.ID)
	}
	return &catalogdomain.Catalog{
		Summary:     g.summary,
		Coverage:    cov,
		GeneratedBy: "llm",
	}, nil
}

func newServiceForTest(t *testing.T) *Service {
	t.Helper()
	return New(filepath.Join(t.TempDir(), ".catalog.json"), nil, zaptest.NewLogger(t))
}

func TestService_NewHasEmptyCacheAndPrompt(t *testing.T) {
	s := newServiceForTest(t)
	if got := s.Get(); got != nil {
		t.Errorf("Get on fresh service = %v, want nil", got)
	}
	if got := s.GetForSystemPrompt(); got != "" {
		t.Errorf("GetForSystemPrompt on fresh service = %q, want empty", got)
	}
}

func TestService_RegisterSourceConcurrent(t *testing.T) {
	s := newServiceForTest(t)
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			s.RegisterSource(&fakeSource{name: "src", gran: catalogdomain.PerItem})
		}(i)
	}
	wg.Wait()
	if got := len(s.snapshotSources()); got != 10 {
		t.Errorf("registered = %d, want 10", got)
	}
}

func TestRefresh_NoSources_NoOp(t *testing.T) {
	s := newServiceForTest(t)
	if err := s.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if got := s.Get(); got != nil {
		t.Errorf("Get after no-source Refresh = %v, want nil", got)
	}
}

func TestRefresh_NilGenerator_UsesMechanicalFallback(t *testing.T) {
	s := newServiceForTest(t)
	src := &fakeSource{name: "forge", gran: catalogdomain.PerItem, items: []catalogdomain.Item{
		{Source: "forge", ID: "f_a", Name: "csv-clean", Description: "Strip BOMs"},
	}}
	s.RegisterSource(src)
	if err := s.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	cat := s.Get()
	if cat == nil {
		t.Fatal("Get after Refresh = nil")
	}
	if cat.GeneratedBy != "mechanical-fallback" {
		t.Errorf("GeneratedBy = %q, want mechanical-fallback (no generator wired)", cat.GeneratedBy)
	}
	if !strings.Contains(cat.Summary, "csv-clean") {
		t.Errorf("Summary lacks item name: %q", cat.Summary)
	}
	if got, want := cat.Version, 1; got != want {
		t.Errorf("Version = %d, want %d", got, want)
	}
	if cat.SourcesAt["forge"].IsZero() {
		t.Error("SourcesAt[forge] should be non-zero after successful poll")
	}
}

func TestRefresh_GeneratorWired_UsesGenerator(t *testing.T) {
	s := newServiceForTest(t)
	gen := &stubGenerator{summary: "## LLM-Generated Summary"}
	s.SetGenerator(gen)
	s.RegisterSource(&fakeSource{name: "forge", gran: catalogdomain.PerItem, items: []catalogdomain.Item{
		{Source: "forge", ID: "f_a", Name: "csv-clean", Description: "Strip BOMs"},
	}})
	if err := s.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	cat := s.Get()
	if cat.GeneratedBy != "llm" {
		t.Errorf("GeneratedBy = %q, want llm", cat.GeneratedBy)
	}
	if !strings.Contains(cat.Summary, "LLM-Generated") {
		t.Errorf("Summary doesn't carry stub marker: %q", cat.Summary)
	}
	if gen.called != 1 {
		t.Errorf("generator.called = %d, want 1", gen.called)
	}
}

func TestRefresh_GeneratorErrors_FallsBackToMechanical(t *testing.T) {
	s := newServiceForTest(t)
	gen := &stubGenerator{returnErr: errors.New("LLM unavailable")}
	s.SetGenerator(gen)
	s.RegisterSource(&fakeSource{name: "skill", gran: catalogdomain.PerItem, items: []catalogdomain.Item{
		{Source: "skill", ID: "deploy", Name: "deploy", Description: "Deploy via CI"},
	}})
	if err := s.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	cat := s.Get()
	if cat.GeneratedBy != "mechanical-fallback" {
		t.Errorf("GeneratedBy = %q, want mechanical-fallback after generator err", cat.GeneratedBy)
	}
	if !strings.Contains(cat.Summary, "deploy") {
		t.Errorf("mechanical Summary lost item: %q", cat.Summary)
	}
}

func TestRefresh_AllSourcesFail_KeepsCache(t *testing.T) {
	s := newServiceForTest(t)
	src := &fakeSource{name: "forge", gran: catalogdomain.PerItem, items: []catalogdomain.Item{
		{Source: "forge", ID: "f_a", Name: "csv-clean", Description: "Strip BOMs"},
	}}
	s.RegisterSource(src)
	if err := s.Refresh(context.Background()); err != nil {
		t.Fatalf("first Refresh: %v", err)
	}
	prev := s.Get()
	if prev == nil {
		t.Fatal("first Refresh produced nil cache")
	}

	src.setErr(errors.New("transient"))
	err := s.Refresh(context.Background())
	if err == nil {
		t.Fatal("all-sources-failed Refresh should err")
	}
	if !strings.Contains(err.Error(), "all 1 sources failed") {
		t.Errorf("err message lacks count signal: %q", err.Error())
	}
	if got := s.Get(); got != prev {
		t.Errorf("cache changed after all-sources-failed Refresh; was %p, now %p", prev, got)
	}
}

func TestRefresh_PartialFailure_OtherSourcesContinue(t *testing.T) {
	s := newServiceForTest(t)
	bad := &fakeSource{name: "bad", gran: catalogdomain.PerItem, listErr: errors.New("kaboom")}
	good := &fakeSource{name: "good", gran: catalogdomain.PerItem, items: []catalogdomain.Item{
		{Source: "good", ID: "g_1", Name: "good-item", Description: "still here"},
	}}
	s.RegisterSource(bad)
	s.RegisterSource(good)
	if err := s.Refresh(context.Background()); err != nil {
		t.Fatalf("partial-failure Refresh should not err; got %v", err)
	}
	cat := s.Get()
	if cat == nil {
		t.Fatal("partial-failure Refresh produced nil cache")
	}
	if !strings.Contains(cat.Summary, "good-item") {
		t.Errorf("good source dropped from Summary: %q", cat.Summary)
	}
	if _, ok := cat.SourcesAt["bad"]; ok {
		t.Errorf("SourcesAt should not include failed source")
	}
	if cat.SourcesAt["good"].IsZero() {
		t.Errorf("SourcesAt[good] missing")
	}
}

func TestRefresh_FingerprintShortCircuits_SecondCall(t *testing.T) {
	s := newServiceForTest(t)
	gen := &stubGenerator{summary: "## llm"}
	s.SetGenerator(gen)
	s.RegisterSource(&fakeSource{name: "forge", gran: catalogdomain.PerItem, items: []catalogdomain.Item{
		{Source: "forge", ID: "f", Name: "x", Description: "y"},
	}})

	if err := s.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh #1: %v", err)
	}
	if err := s.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh #2: %v", err)
	}
	if gen.called != 1 {
		t.Errorf("generator.called = %d after 2 Refresh w/ unchanged items; want 1 (fingerprint short-circuit)", gen.called)
	}
}

func TestRefresh_DescriptionChange_TriggersRegen(t *testing.T) {
	s := newServiceForTest(t)
	gen := &stubGenerator{summary: "## llm"}
	s.SetGenerator(gen)
	src := &fakeSource{name: "forge", gran: catalogdomain.PerItem, items: []catalogdomain.Item{
		{Source: "forge", ID: "f", Name: "x", Description: "v1"},
	}}
	s.RegisterSource(src)

	_ = s.Refresh(context.Background())
	src.setItems([]catalogdomain.Item{{Source: "forge", ID: "f", Name: "x", Description: "v2"}})
	_ = s.Refresh(context.Background())

	if gen.called != 2 {
		t.Errorf("generator.called = %d, want 2 (description change should bust fingerprint)", gen.called)
	}
}

func TestPollLoop_FiresAtLeastOnce(t *testing.T) {
	s := newServiceForTest(t)
	s.SetPollInterval(20 * time.Millisecond)
	src := &fakeSource{name: "forge", gran: catalogdomain.PerItem, items: []catalogdomain.Item{
		{Source: "forge", ID: "f", Name: "x", Description: "y"},
	}}
	s.RegisterSource(src)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := s.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if atomic.LoadInt32(&src.callCount) >= 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if got := atomic.LoadInt32(&src.callCount); got < 1 {
		t.Fatalf("source.callCount = %d after Start, want ≥ 1 (poll did not fire)", got)
	}
	if s.Get() == nil {
		t.Error("cache still nil after pollLoop tick")
	}
}

func TestTryRefresh_BusyGuard_SkipsConcurrent(t *testing.T) {
	s := newServiceForTest(t)
	release := make(chan struct{})
	gen := &slowGenerator{release: release, summary: "## llm", entered: make(chan struct{})}
	s.SetGenerator(gen)
	s.RegisterSource(&fakeSource{name: "forge", gran: catalogdomain.PerItem, items: []catalogdomain.Item{
		{Source: "forge", ID: "f", Name: "x", Description: "y"},
	}})

	go s.tryRefresh(context.Background())
	select {
	case <-gen.entered:
	case <-time.After(time.Second):
		t.Fatal("slow generator never entered")
	}

	s.tryRefresh(context.Background())

	close(release)
	time.Sleep(50 * time.Millisecond)

	if got := atomic.LoadInt32(&gen.entered2); got != 0 {
		t.Errorf("generator entered %d times after busy-guard skip; want 0 second entries", got)
	}
}

type slowGenerator struct {
	release  chan struct{}
	entered  chan struct{} // signals first entry
	entered2 int32         // count of additional entries
	mu       sync.Mutex
	first    bool
	summary  string
}

func (g *slowGenerator) Generate(_ context.Context, items []catalogdomain.Item, _ map[string]catalogdomain.Granularity) (*catalogdomain.Catalog, error) {
	g.mu.Lock()
	if !g.first {
		g.first = true
		close(g.entered)
	} else {
		atomic.AddInt32(&g.entered2, 1)
	}
	g.mu.Unlock()
	if g.release != nil {
		<-g.release
	}
	cov := map[string][]string{}
	for _, it := range items {
		cov[it.Source] = append(cov[it.Source], it.ID)
	}
	return &catalogdomain.Catalog{Summary: g.summary, Coverage: cov, GeneratedBy: "llm"}, nil
}

func TestStart_LoadsExistingCache(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".catalog.json")
	pre := &catalogdomain.Catalog{
		Summary:     "## from-disk",
		Coverage:    map[string][]string{"forge": {"f_pre"}},
		Fingerprint: "preFP",
		Version:     7,
		GeneratedBy: "llm",
	}
	raw, _ := json.Marshal(pre)
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatalf("seed disk cache: %v", err)
	}

	s := New(path, nil, zaptest.NewLogger(t))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := s.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	got := s.Get()
	if got == nil || got.Summary != "## from-disk" {
		t.Fatalf("cold-start cache lost; got %+v", got)
	}
	if got.Version != 7 {
		t.Errorf("Version = %d, want 7 (preserved from disk)", got.Version)
	}
	if v := s.nextVersion(); v != 8 {
		t.Errorf("nextVersion after disk load = %d, want 8", v)
	}
}

func TestStart_CorruptCacheMovedToBak(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".catalog.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
		t.Fatalf("write corrupt cache: %v", err)
	}

	s := New(path, nil, zaptest.NewLogger(t))
	if err := s.Start(context.Background()); err != nil {
		t.Fatalf("Start should not err on corrupt cache; got %v", err)
	}
	if got := s.Get(); got != nil {
		t.Errorf("cache should be nil after corrupt load; got %+v", got)
	}
	if _, err := os.Stat(path + ".bak"); err != nil {
		t.Errorf("corrupt file should have been moved to .bak; stat err = %v", err)
	}
}

func TestRefresh_PersistsToDisk(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".catalog.json")
	s := New(path, nil, zaptest.NewLogger(t))
	s.RegisterSource(&fakeSource{name: "forge", gran: catalogdomain.PerItem, items: []catalogdomain.Item{
		{Source: "forge", ID: "f", Name: "x", Description: "y"},
	}})
	if err := s.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read persisted catalog: %v", err)
	}
	var disk catalogdomain.Catalog
	if err := json.Unmarshal(raw, &disk); err != nil {
		t.Fatalf("persisted catalog malformed: %v", err)
	}
	if disk.Fingerprint == "" || !strings.Contains(disk.Summary, "x") {
		t.Errorf("persisted catalog incomplete: %+v", disk)
	}
}

func TestFingerprint_StableUnderShuffle(t *testing.T) {
	a := []catalogdomain.Item{
		{Source: "forge", ID: "f1", Name: "alpha", Description: "x"},
		{Source: "skill", ID: "s1", Name: "beta", Description: "y"},
	}
	b := []catalogdomain.Item{
		{Source: "skill", ID: "s1", Name: "beta", Description: "y"},
		{Source: "forge", ID: "f1", Name: "alpha", Description: "x"},
	}
	if fingerprint(a) != fingerprint(b) {
		t.Errorf("fingerprint should be stable under input order")
	}
}

func TestFingerprint_ChangesOnDescription(t *testing.T) {
	a := []catalogdomain.Item{{Source: "forge", ID: "f", Name: "x", Description: "v1"}}
	b := []catalogdomain.Item{{Source: "forge", ID: "f", Name: "x", Description: "v2"}}
	if fingerprint(a) == fingerprint(b) {
		t.Errorf("fingerprint should change when description changes (otherwise regen never triggers)")
	}
}

func TestFingerprint_IgnoresIDOnly(t *testing.T) {
	a := []catalogdomain.Item{{Source: "forge", ID: "f1", Name: "x", Description: "y"}}
	b := []catalogdomain.Item{{Source: "forge", ID: "f2", Name: "x", Description: "y"}}
	if fingerprint(a) != fingerprint(b) {
		t.Errorf("fingerprint should ignore ID-only changes")
	}
}
