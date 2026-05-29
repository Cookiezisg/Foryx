package model

import (
	"context"
	"errors"
	"testing"

	"go.uber.org/zap"

	apikeydomain "github.com/sunweilin/forgify/backend/internal/domain/apikey"
	modeldomain "github.com/sunweilin/forgify/backend/internal/domain/model"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

// fakeKeys is a minimal KeyProvider for model unit tests; byID drives
// ResolveCredentialsByID (unknown id => ErrNotFound, satisfying Upsert F1).
//
// fakeKeys 是 model 单测用的最小 KeyProvider 桩；byID 驱动 ResolveCredentialsByID
// (未知 id 返 ErrNotFound,Upsert F1 校验依赖此行为)。
type fakeKeys struct {
	byID map[string]apikeydomain.Credentials
}

func (f *fakeKeys) ResolveCredentials(context.Context, string) (apikeydomain.Credentials, error) {
	return apikeydomain.Credentials{}, nil
}
func (f *fakeKeys) ResolveCredentialsByID(_ context.Context, id string) (apikeydomain.Credentials, error) {
	if f.byID == nil {
		return apikeydomain.Credentials{}, apikeydomain.ErrNotFound
	}
	c, ok := f.byID[id]
	if !ok {
		return apikeydomain.Credentials{}, apikeydomain.ErrNotFound
	}
	return c, nil
}
func (f *fakeKeys) MarkInvalid(context.Context, string, string) error { return nil }
func (f *fakeKeys) DefaultSearchProvider(context.Context) string      { return "" }

type fakeRepo struct {
	rows      map[string]*modeldomain.ModelConfig // keyed by ID
	upsertErr error
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{rows: make(map[string]*modeldomain.ModelConfig)}
}

func (r *fakeRepo) GetByScenario(ctx context.Context, scenario string) (*modeldomain.ModelConfig, error) {
	uid, _ := reqctxpkg.GetUserID(ctx)
	for _, m := range r.rows {
		if m.UserID == uid && m.Scenario == scenario {
			cp := *m
			return &cp, nil
		}
	}
	return nil, modeldomain.ErrNotConfigured
}

func (r *fakeRepo) List(ctx context.Context) ([]*modeldomain.ModelConfig, error) {
	uid, _ := reqctxpkg.GetUserID(ctx)
	var out []*modeldomain.ModelConfig
	for _, m := range r.rows {
		if m.UserID == uid {
			cp := *m
			out = append(out, &cp)
		}
	}
	return out, nil
}

func (r *fakeRepo) Upsert(_ context.Context, m *modeldomain.ModelConfig) error {
	if r.upsertErr != nil {
		return r.upsertErr
	}
	cp := *m
	r.rows[m.ID] = &cp
	return nil
}

func newSvc(t *testing.T, repo modeldomain.Repository) *Service {
	t.Helper()
	keys := &fakeKeys{byID: map[string]apikeydomain.Credentials{
		"aki_test": {Provider: "anthropic", Key: "sk-test", BaseURL: ""},
	}}
	return NewService(repo, keys, zap.NewNop())
}

func ctxAlice() context.Context {
	return reqctxpkg.SetUserID(context.Background(), "u-alice")
}

func TestNewService_NilLogger_Panics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic on nil logger, got none")
		}
	}()
	NewService(newFakeRepo(), &fakeKeys{}, nil)
}

func TestUpsert_InvalidScenario(t *testing.T) {
	svc := newSvc(t, newFakeRepo())
	_, err := svc.Upsert(ctxAlice(), "nonexistent", UpsertInput{APIKeyID: "aki_test", ModelID: "gpt-4o"})
	if !errors.Is(err, modeldomain.ErrInvalidScenario) {
		t.Errorf("got %v, want ErrInvalidScenario", err)
	}
}

func TestUpsert_APIKeyIDRequired(t *testing.T) {
	svc := newSvc(t, newFakeRepo())
	_, err := svc.Upsert(ctxAlice(), modeldomain.ScenarioDialogue, UpsertInput{APIKeyID: "  ", ModelID: "gpt-4o"})
	if !errors.Is(err, modeldomain.ErrAPIKeyIDRequired) {
		t.Errorf("got %v, want ErrAPIKeyIDRequired", err)
	}
}

func TestUpsert_ModelIDRequired(t *testing.T) {
	svc := newSvc(t, newFakeRepo())
	_, err := svc.Upsert(ctxAlice(), modeldomain.ScenarioDialogue, UpsertInput{APIKeyID: "aki_test", ModelID: ""})
	if !errors.Is(err, modeldomain.ErrModelIDRequired) {
		t.Errorf("got %v, want ErrModelIDRequired", err)
	}
}

func TestUpsert_NewScenario_CreatesRow(t *testing.T) {
	svc := newSvc(t, newFakeRepo())
	got, err := svc.Upsert(ctxAlice(), modeldomain.ScenarioDialogue, UpsertInput{APIKeyID: "aki_test", ModelID: "gpt-4o"})
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	if got.APIKeyID != "aki_test" || got.ModelID != "gpt-4o" {
		t.Errorf("wrong fields: %+v", got)
	}
	if got.ID == "" {
		t.Error("ID must not be empty")
	}
}

func TestUpsert_ExistingScenario_PreservesID(t *testing.T) {
	svc := newSvc(t, newFakeRepo())

	first, err := svc.Upsert(ctxAlice(), modeldomain.ScenarioDialogue, UpsertInput{APIKeyID: "aki_test", ModelID: "gpt-4o"})
	if err != nil {
		t.Fatalf("first Upsert: %v", err)
	}

	second, err := svc.Upsert(ctxAlice(), modeldomain.ScenarioDialogue, UpsertInput{APIKeyID: "aki_test", ModelID: "claude-3-5-sonnet-latest"})
	if err != nil {
		t.Fatalf("second Upsert: %v", err)
	}
	if second.ID != first.ID {
		t.Errorf("ID changed: first=%q second=%q", first.ID, second.ID)
	}
	if second.ModelID != "claude-3-5-sonnet-latest" {
		t.Errorf("ModelID not updated: got %q", second.ModelID)
	}
}

func TestUpsert_TrimsWhitespace(t *testing.T) {
	svc := newSvc(t, newFakeRepo())
	got, err := svc.Upsert(ctxAlice(), modeldomain.ScenarioDialogue, UpsertInput{APIKeyID: "  aki_test  ", ModelID: " gpt-4o "})
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	if got.APIKeyID != "aki_test" || got.ModelID != "gpt-4o" {
		t.Errorf("whitespace not trimmed: apiKeyID=%q modelID=%q", got.APIKeyID, got.ModelID)
	}
}

func TestList_Empty(t *testing.T) {
	svc := newSvc(t, newFakeRepo())
	rows, err := svc.List(ctxAlice())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("got %d rows, want 0", len(rows))
	}
}

func TestList_AfterUpsert(t *testing.T) {
	svc := newSvc(t, newFakeRepo())
	if _, err := svc.Upsert(ctxAlice(), modeldomain.ScenarioDialogue, UpsertInput{APIKeyID: "aki_test", ModelID: "gpt-4o"}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	rows, err := svc.List(ctxAlice())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(rows) != 1 {
		t.Errorf("got %d rows, want 1", len(rows))
	}
}

func TestService_PickForDialogue(t *testing.T) {
	svc := newSvc(t, newFakeRepo())
	ctx := ctxAlice()
	if _, _, _, err := svc.PickForDialogue(ctx); !errors.Is(err, modeldomain.ErrNotConfigured) {
		t.Fatalf("unconfigured PickForDialogue: want ErrNotConfigured, got %v", err)
	}
	if _, err := svc.Upsert(ctx, modeldomain.ScenarioDialogue, UpsertInput{
		APIKeyID: "aki_test", ModelID: "claude-sonnet-4-5",
	}); err != nil {
		t.Fatal(err)
	}
	id, m, _, err := svc.PickForDialogue(ctx)
	if err != nil || id != "aki_test" || m != "claude-sonnet-4-5" {
		t.Fatalf("PickForDialogue=(%q,%q,%v), want (aki_test, claude-sonnet-4-5, nil)", id, m, err)
	}
}

func TestService_PickForUtility(t *testing.T) {
	svc := newSvc(t, newFakeRepo())
	ctx := ctxAlice()
	if _, _, _, err := svc.PickForUtility(ctx); !errors.Is(err, modeldomain.ErrNotConfigured) {
		t.Fatal("want ErrNotConfigured")
	}
	if _, err := svc.Upsert(ctx, modeldomain.ScenarioUtility, UpsertInput{
		APIKeyID: "aki_test", ModelID: "claude-haiku-4-5",
	}); err != nil {
		t.Fatal(err)
	}
	id, m, _, _ := svc.PickForUtility(ctx)
	if id != "aki_test" || m != "claude-haiku-4-5" {
		t.Fatalf("PickForUtility=(%q,%q), want (aki_test, claude-haiku-4-5)", id, m)
	}
}

func TestService_PickForAgent(t *testing.T) {
	svc := newSvc(t, newFakeRepo())
	ctx := ctxAlice()
	if _, _, _, err := svc.PickForAgent(ctx); !errors.Is(err, modeldomain.ErrNotConfigured) {
		t.Fatal("want ErrNotConfigured")
	}
	if _, err := svc.Upsert(ctx, modeldomain.ScenarioAgent, UpsertInput{
		APIKeyID: "aki_test", ModelID: "deepseek-chat",
	}); err != nil {
		t.Fatal(err)
	}
	id, m, _, _ := svc.PickForAgent(ctx)
	if id != "aki_test" || m != "deepseek-chat" {
		t.Fatalf("PickForAgent=(%q,%q), want (aki_test, deepseek-chat)", id, m)
	}
}

func TestService_Upsert_UnknownAPIKeyID_Returns404(t *testing.T) {
	svc := newSvc(t, newFakeRepo())
	_, err := svc.Upsert(ctxAlice(), modeldomain.ScenarioDialogue, UpsertInput{
		APIKeyID: "aki_nonexistent", ModelID: "claude-sonnet-4-5",
	})
	if !errors.Is(err, apikeydomain.ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestUpsert_WithThinking_PersistedAndReturned(t *testing.T) {
	svc := newSvc(t, newFakeRepo())
	spec := &modeldomain.ThinkingSpec{Mode: "on", Effort: "high"}
	got, err := svc.Upsert(ctxAlice(), modeldomain.ScenarioDialogue, UpsertInput{
		APIKeyID: "aki_test", ModelID: "claude-sonnet-4-5", Thinking: spec,
	})
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	if got.Thinking == nil {
		t.Fatal("Thinking is nil, want non-nil")
	}
	if got.Thinking.Mode != "on" || got.Thinking.Effort != "high" {
		t.Errorf("Thinking = %+v, want {Mode:on Effort:high}", got.Thinking)
	}
}

func TestUpsert_WithoutThinking_ThinkingIsNil(t *testing.T) {
	svc := newSvc(t, newFakeRepo())
	got, err := svc.Upsert(ctxAlice(), modeldomain.ScenarioDialogue, UpsertInput{
		APIKeyID: "aki_test", ModelID: "gpt-4o",
	})
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	if got.Thinking != nil {
		t.Errorf("Thinking = %+v, want nil", got.Thinking)
	}
}
