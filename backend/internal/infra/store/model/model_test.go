package model

import (
	"context"
	"errors"
	"testing"

	gormlogger "gorm.io/gorm/logger"

	modeldomain "github.com/sunweilin/forgify/backend/internal/domain/model"
	dbinfra "github.com/sunweilin/forgify/backend/internal/infra/db"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

const (
	userAlice = "u-alice"
	userBob   = "u-bob"
)

func newStore(t *testing.T) *Store {
	t.Helper()
	database, err := dbinfra.Open(dbinfra.Config{LogLevel: gormlogger.Silent})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = dbinfra.Close(database) })
	if err := dbinfra.Migrate(database, &modeldomain.ModelConfig{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return New(database)
}

func ctxFor(userID string) context.Context {
	return reqctxpkg.SetUserID(context.Background(), userID)
}

func mkConfig(id, userID, scenario, apiKeyID, modelID string) *modeldomain.ModelConfig {
	return &modeldomain.ModelConfig{
		ID:       id,
		UserID:   userID,
		Scenario: scenario,
		APIKeyID: apiKeyID,
		ModelID:  modelID,
	}
}

func TestUpsert_NewRowCreated(t *testing.T) {
	s := newStore(t)
	ctx := ctxFor(userAlice)

	m := mkConfig("mc1", userAlice, modeldomain.ScenarioDialogue, "aki_openai", "gpt-4o")
	if err := s.Upsert(ctx, m); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	got, err := s.GetByScenario(ctx, modeldomain.ScenarioDialogue)
	if err != nil {
		t.Fatalf("GetByScenario: %v", err)
	}
	if got.APIKeyID != "aki_openai" || got.ModelID != "gpt-4o" {
		t.Errorf("got %+v, want aki_openai/gpt-4o", got)
	}
}

func TestUpsert_UpdatePreservesID(t *testing.T) {
	s := newStore(t)
	ctx := ctxFor(userAlice)

	m := mkConfig("mc1", userAlice, modeldomain.ScenarioDialogue, "aki_openai", "gpt-4o")
	if err := s.Upsert(ctx, m); err != nil {
		t.Fatalf("first Upsert: %v", err)
	}

	m.APIKeyID = "aki_anthropic"
	m.ModelID = "claude-3-5-sonnet-latest"
	if err := s.Upsert(ctx, m); err != nil {
		t.Fatalf("second Upsert: %v", err)
	}

	got, err := s.GetByScenario(ctx, modeldomain.ScenarioDialogue)
	if err != nil {
		t.Fatalf("GetByScenario: %v", err)
	}
	if got.ID != "mc1" {
		t.Errorf("ID changed: got %q, want %q", got.ID, "mc1")
	}
	if got.APIKeyID != "aki_anthropic" || got.ModelID != "claude-3-5-sonnet-latest" {
		t.Errorf("fields not updated: got %+v", got)
	}
}

func TestUpsert_UniqueConstraintBlocksDuplicate(t *testing.T) {
	s := newStore(t)
	ctx := ctxFor(userAlice)

	m1 := mkConfig("mc1", userAlice, modeldomain.ScenarioDialogue, "aki_openai", "gpt-4o")
	if err := s.Upsert(ctx, m1); err != nil {
		t.Fatalf("first Upsert: %v", err)
	}

	m2 := mkConfig("mc2", userAlice, modeldomain.ScenarioDialogue, "aki_anthropic", "claude-3-5-sonnet-latest")
	if err := s.Upsert(ctx, m2); err == nil {
		t.Error("second Upsert with same (user, scenario) different ID: got nil, want unique constraint error")
	}
}

func TestGetByScenario_NotFound(t *testing.T) {
	s := newStore(t)
	_, err := s.GetByScenario(ctxFor(userAlice), modeldomain.ScenarioDialogue)
	if !errors.Is(err, modeldomain.ErrNotConfigured) {
		t.Errorf("got %v, want ErrNotConfigured", err)
	}
}

func TestGetByScenario_CrossUserIsolation(t *testing.T) {
	s := newStore(t)

	m := mkConfig("mc1", userAlice, modeldomain.ScenarioDialogue, "aki_openai", "gpt-4o")
	if err := s.Upsert(ctxFor(userAlice), m); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	_, err := s.GetByScenario(ctxFor(userBob), modeldomain.ScenarioDialogue)
	if !errors.Is(err, modeldomain.ErrNotConfigured) {
		t.Errorf("Bob sees Alice's config: got %v, want ErrNotConfigured", err)
	}
}

func TestGetByScenario_MissingUserID(t *testing.T) {
	s := newStore(t)
	_, err := s.GetByScenario(context.Background(), modeldomain.ScenarioDialogue)
	if err == nil {
		t.Fatal("got nil, want wiring error")
	}
	if errors.Is(err, modeldomain.ErrNotConfigured) {
		t.Errorf("wiring bug leaked as ErrNotConfigured: %v", err)
	}
}

func TestList_EmptyReturnsNonNilSlice(t *testing.T) {
	s := newStore(t)
	rows, err := s.List(ctxFor(userAlice))
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if rows == nil {
		t.Error("List returned nil slice, want empty non-nil")
	}
	if len(rows) != 0 {
		t.Errorf("List returned %d rows, want 0", len(rows))
	}
}

func TestList_ReturnsActiveRows(t *testing.T) {
	s := newStore(t)
	ctx := ctxFor(userAlice)

	m := mkConfig("mc1", userAlice, modeldomain.ScenarioDialogue, "aki_openai", "gpt-4o")
	if err := s.Upsert(ctx, m); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	rows, err := s.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1", len(rows))
	}
	if rows[0].ID != "mc1" {
		t.Errorf("ID = %q, want %q", rows[0].ID, "mc1")
	}
}

func TestList_CrossUserIsolation(t *testing.T) {
	s := newStore(t)

	ma := mkConfig("mc-a", userAlice, modeldomain.ScenarioDialogue, "aki_openai", "gpt-4o")
	if err := s.Upsert(ctxFor(userAlice), ma); err != nil {
		t.Fatalf("Upsert Alice: %v", err)
	}
	mb := mkConfig("mc-b", userBob, modeldomain.ScenarioDialogue, "aki_anthropic", "claude-3-5-sonnet-latest")
	if err := s.Upsert(ctxFor(userBob), mb); err != nil {
		t.Fatalf("Upsert Bob: %v", err)
	}

	rows, err := s.List(ctxFor(userAlice))
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(rows) != 1 || rows[0].ID != "mc-a" {
		t.Errorf("Alice sees wrong rows: %+v", rows)
	}
}

func TestStore_AnyReferencesApiKey_True(t *testing.T) {
	s := newStore(t)
	ctx := ctxFor(userAlice)

	if err := s.Upsert(ctx, mkConfig("mc1", userAlice, modeldomain.ScenarioDialogue, "aki_x", "gpt-4o")); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	got, err := s.AnyReferencesApiKey(ctx, "aki_x")
	if err != nil {
		t.Fatalf("AnyReferencesApiKey: %v", err)
	}
	if !got {
		t.Error("got false, want true (model_config references aki_x)")
	}
}

func TestStore_AnyReferencesApiKey_False(t *testing.T) {
	s := newStore(t)
	got, err := s.AnyReferencesApiKey(ctxFor(userAlice), "aki_x")
	if err != nil {
		t.Fatalf("AnyReferencesApiKey: %v", err)
	}
	if got {
		t.Error("got true on empty store, want false")
	}
}

func TestStore_AnyReferencesApiKey_CrossUserIsolated(t *testing.T) {
	s := newStore(t)

	if err := s.Upsert(ctxFor(userAlice), mkConfig("mc-a", userAlice, modeldomain.ScenarioDialogue, "aki_x", "gpt-4o")); err != nil {
		t.Fatalf("Upsert Alice: %v", err)
	}
	got, err := s.AnyReferencesApiKey(ctxFor(userBob), "aki_x")
	if err != nil {
		t.Fatalf("AnyReferencesApiKey: %v", err)
	}
	if got {
		t.Error("got true: Bob sees Alice's reference, want false (cross-user isolated)")
	}
}
