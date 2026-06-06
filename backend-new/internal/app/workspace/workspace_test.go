package workspace

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"

	modeldomain "github.com/sunweilin/forgify/backend/internal/domain/model"
	workspacedomain "github.com/sunweilin/forgify/backend/internal/domain/workspace"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

// fakeRepo is an in-memory workspacedomain.Repository. It mirrors the store's
// unique-name behavior so conflict paths are covered without a real DB.
//
// fakeRepo 是内存版 workspacedomain.Repository。镜像 store 的唯一名行为，使冲突路径无需真 DB。
type fakeRepo struct {
	items map[string]*workspacedomain.Workspace
}

func newFakeRepo() *fakeRepo { return &fakeRepo{items: map[string]*workspacedomain.Workspace{}} }

func (f *fakeRepo) Save(_ context.Context, w *workspacedomain.Workspace) error {
	for id, existing := range f.items {
		if id != w.ID && existing.Name == w.Name {
			return workspacedomain.ErrNameConflict
		}
	}
	cp := *w
	f.items[w.ID] = &cp
	return nil
}

func (f *fakeRepo) Get(_ context.Context, id string) (*workspacedomain.Workspace, error) {
	w, ok := f.items[id]
	if !ok {
		return nil, workspacedomain.ErrNotFound
	}
	cp := *w
	return &cp, nil
}

func (f *fakeRepo) List(_ context.Context) ([]*workspacedomain.Workspace, error) {
	out := make([]*workspacedomain.Workspace, 0, len(f.items))
	for _, w := range f.items {
		cp := *w
		out = append(out, &cp)
	}
	return out, nil
}

func (f *fakeRepo) Delete(_ context.Context, id string) error {
	if _, ok := f.items[id]; !ok {
		return workspacedomain.ErrNotFound
	}
	delete(f.items, id)
	return nil
}

func (f *fakeRepo) Count(_ context.Context) (int, error) { return len(f.items), nil }

func (f *fakeRepo) TouchLastUsed(_ context.Context, id string) error {
	w, ok := f.items[id]
	if !ok {
		return workspacedomain.ErrNotFound
	}
	now := time.Now().UTC()
	w.LastUsedAt = &now
	return nil
}

func newService() *Service { return NewService(newFakeRepo(), zap.NewNop()) }

func TestCreate_TrimsName_DefaultsLanguageAndID(t *testing.T) {
	w, err := newService().Create(context.Background(), CreateInput{Name: "  My Space  "})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if w.Name != "My Space" {
		t.Errorf("name = %q, want trimmed 'My Space'", w.Name)
	}
	if w.Language != workspacedomain.LanguageZhCN {
		t.Errorf("language = %q, want default zh-CN", w.Language)
	}
	if !strings.HasPrefix(w.ID, "ws_") {
		t.Errorf("id = %q, want ws_ prefix", w.ID)
	}
}

func TestCreate_EmptyName_ErrNameRequired(t *testing.T) {
	_, err := newService().Create(context.Background(), CreateInput{Name: "   "})
	if !errors.Is(err, workspacedomain.ErrNameRequired) {
		t.Errorf("err = %v, want ErrNameRequired", err)
	}
}

func TestCreate_TooLong_ErrNameTooLong(t *testing.T) {
	long := strings.Repeat("a", workspacedomain.MaxNameLen+1)
	_, err := newService().Create(context.Background(), CreateInput{Name: long})
	if !errors.Is(err, workspacedomain.ErrNameTooLong) {
		t.Errorf("err = %v, want ErrNameTooLong", err)
	}
}

func TestCreate_InvalidLanguage_ErrLanguageInvalid(t *testing.T) {
	_, err := newService().Create(context.Background(), CreateInput{Name: "X", Language: "fr"})
	if !errors.Is(err, workspacedomain.ErrLanguageInvalid) {
		t.Errorf("err = %v, want ErrLanguageInvalid", err)
	}
}

func TestCreate_DuplicateName_ErrNameConflict(t *testing.T) {
	s := newService()
	if _, err := s.Create(context.Background(), CreateInput{Name: "Dup"}); err != nil {
		t.Fatalf("first: %v", err)
	}
	_, err := s.Create(context.Background(), CreateInput{Name: "Dup"})
	if !errors.Is(err, workspacedomain.ErrNameConflict) {
		t.Errorf("err = %v, want ErrNameConflict", err)
	}
}

func TestUpdate_PartialRename(t *testing.T) {
	s := newService()
	w, _ := s.Create(context.Background(), CreateInput{Name: "Orig"})
	newName := "Renamed"
	got, err := s.Update(context.Background(), w.ID, UpdateInput{Name: &newName})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if got.Name != "Renamed" {
		t.Errorf("name = %q, want Renamed", got.Name)
	}
}

func TestUpdate_InvalidLanguage(t *testing.T) {
	s := newService()
	w, _ := s.Create(context.Background(), CreateInput{Name: "X"})
	bad := "de"
	_, err := s.Update(context.Background(), w.ID, UpdateInput{Language: &bad})
	if !errors.Is(err, workspacedomain.ErrLanguageInvalid) {
		t.Errorf("err = %v, want ErrLanguageInvalid", err)
	}
}

func TestDelete_LastRefused(t *testing.T) {
	s := newService()
	w, _ := s.Create(context.Background(), CreateInput{Name: "Only"})
	if err := s.Delete(context.Background(), w.ID); !errors.Is(err, workspacedomain.ErrCannotDeleteLast) {
		t.Errorf("err = %v, want ErrCannotDeleteLast", err)
	}
}

func TestDelete_OKWhenMoreThanOne(t *testing.T) {
	s := newService()
	_, _ = s.Create(context.Background(), CreateInput{Name: "A"})
	b, _ := s.Create(context.Background(), CreateInput{Name: "B"})
	if err := s.Delete(context.Background(), b.ID); err != nil {
		t.Errorf("delete with >1 workspace: %v", err)
	}
}

func TestValidate_ExistingAndMissing(t *testing.T) {
	s := newService()
	w, _ := s.Create(context.Background(), CreateInput{Name: "V"})
	if err := s.Validate(context.Background(), w.ID); err != nil {
		t.Errorf("validate existing: %v", err)
	}
	if err := s.Validate(context.Background(), "ws_missing"); !errors.Is(err, workspacedomain.ErrNotFound) {
		t.Errorf("validate missing: err = %v, want ErrNotFound", err)
	}
}

func TestSetDefault_AndPick(t *testing.T) {
	s := newService()
	w, _ := s.Create(context.Background(), CreateInput{Name: "WS"})
	ref := &modeldomain.ModelRef{APIKeyID: "aki_1", ModelID: "gpt-5.5", Options: map[string]string{"reasoning_effort": "high"}}
	if _, err := s.SetDefault(context.Background(), w.ID, modeldomain.ScenarioDialogue, ref); err != nil {
		t.Fatalf("set default: %v", err)
	}
	// Pick reads the current workspace (id from ctx) — the picker contract LLM callers use.
	// Pick 读当前 workspace（id 取自 ctx）——LLM caller 用的 picker 契约。
	ctx := reqctxpkg.SetWorkspaceID(context.Background(), w.ID)
	got, err := s.Pick(ctx, modeldomain.ScenarioDialogue)
	if err != nil {
		t.Fatalf("pick: %v", err)
	}
	if got.APIKeyID != "aki_1" || got.ModelID != "gpt-5.5" || got.Options["reasoning_effort"] != "high" {
		t.Errorf("pick = %+v, want the set default", got)
	}
}

func TestSetDefaultSearch_AndPick(t *testing.T) {
	s := newService()
	w, _ := s.Create(context.Background(), CreateInput{Name: "WS"})
	if _, err := s.SetDefaultSearch(context.Background(), w.ID, "aki_search"); err != nil {
		t.Fatalf("set default search: %v", err)
	}
	// DefaultSearchKeyID reads the current workspace (id from ctx) — the SearchKeyPicker contract.
	// DefaultSearchKeyID 读当前 workspace（id 取自 ctx）——SearchKeyPicker 契约。
	ctx := reqctxpkg.SetWorkspaceID(context.Background(), w.ID)
	id, ok := s.DefaultSearchKeyID(ctx)
	if !ok || id != "aki_search" {
		t.Fatalf("DefaultSearchKeyID = (%q,%v), want (aki_search,true)", id, ok)
	}
}

func TestDefaultSearchKeyID_Unconfigured(t *testing.T) {
	s := newService()
	w, _ := s.Create(context.Background(), CreateInput{Name: "WS"})
	ctx := reqctxpkg.SetWorkspaceID(context.Background(), w.ID)
	if id, ok := s.DefaultSearchKeyID(ctx); ok || id != "" {
		t.Fatalf(`DefaultSearchKeyID = (%q,%v), want ("",false)`, id, ok)
	}
}

func TestSetDefaultSearch_Clear(t *testing.T) {
	s := newService()
	w, _ := s.Create(context.Background(), CreateInput{Name: "WS"})
	if _, err := s.SetDefaultSearch(context.Background(), w.ID, "aki_search"); err != nil {
		t.Fatalf("set: %v", err)
	}
	if _, err := s.SetDefaultSearch(context.Background(), w.ID, ""); err != nil {
		t.Fatalf("clear: %v", err)
	}
	ctx := reqctxpkg.SetWorkspaceID(context.Background(), w.ID)
	if id, ok := s.DefaultSearchKeyID(ctx); ok || id != "" {
		t.Fatalf(`after clear = (%q,%v), want ("",false)`, id, ok)
	}
}

func TestDefaultSearchKeyID_NoWorkspaceInCtx(t *testing.T) {
	s := newService()
	if id, ok := s.DefaultSearchKeyID(context.Background()); ok || id != "" {
		t.Fatalf(`no ws in ctx = (%q,%v), want ("",false)`, id, ok)
	}
}

func TestPick_NotConfigured(t *testing.T) {
	s := newService()
	w, _ := s.Create(context.Background(), CreateInput{Name: "WS"})
	ctx := reqctxpkg.SetWorkspaceID(context.Background(), w.ID)
	if _, err := s.Pick(ctx, modeldomain.ScenarioUtility); !errors.Is(err, modeldomain.ErrNotConfigured) {
		t.Errorf("err = %v, want ErrNotConfigured", err)
	}
}

func TestSetDefault_InvalidRef(t *testing.T) {
	s := newService()
	w, _ := s.Create(context.Background(), CreateInput{Name: "WS"})
	_, err := s.SetDefault(context.Background(), w.ID, modeldomain.ScenarioAgent, &modeldomain.ModelRef{APIKeyID: "aki_1"})
	if !errors.Is(err, modeldomain.ErrRefInvalid) {
		t.Errorf("err = %v, want ErrRefInvalid", err)
	}
}

func TestSetDefault_InvalidScenario(t *testing.T) {
	s := newService()
	w, _ := s.Create(context.Background(), CreateInput{Name: "WS"})
	_, err := s.SetDefault(context.Background(), w.ID, "bogus", &modeldomain.ModelRef{APIKeyID: "a", ModelID: "m"})
	if !errors.Is(err, modeldomain.ErrScenarioInvalid) {
		t.Errorf("err = %v, want ErrScenarioInvalid", err)
	}
}
