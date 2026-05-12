package handler

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"go.uber.org/zap"

	handlerdomain "github.com/sunweilin/forgify/backend/internal/domain/handler"
	notificationsdomain "github.com/sunweilin/forgify/backend/internal/domain/notifications"
	notificationspkg "github.com/sunweilin/forgify/backend/internal/pkg/notifications"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

// fakeEncryptor returns ciphertext = plaintext (identity). Pure for testing.
//
// fakeEncryptor 返密文 = 明文(恒等);测试用。
type fakeEncryptor struct{}

func (fakeEncryptor) Encrypt(_ context.Context, p []byte) ([]byte, error) { return p, nil }
func (fakeEncryptor) Decrypt(_ context.Context, c []byte) ([]byte, error) { return c, nil }

// fakeRepo is a partial Repository implementation just for config tests —
// only the 3 config methods + GetHandler are exercised here. Other methods
// panic if hit (forces test isolation).
//
// fakeRepo 仅实现 config 测试需要的 4 方法;其他方法 panic 保隔离。
type fakeRepo struct {
	mu     sync.Mutex
	cipher map[string]string // handlerID → ciphertext
	exists map[string]bool   // handlerID → row exists
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		cipher: make(map[string]string),
		exists: make(map[string]bool),
	}
}

func (r *fakeRepo) addHandler(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.exists[id] = true
}

func (r *fakeRepo) UpdateConfigEncrypted(_ context.Context, handlerID, ct string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.exists[handlerID] {
		return handlerdomain.ErrNotFound
	}
	r.cipher[handlerID] = ct
	return nil
}

func (r *fakeRepo) ClearConfig(_ context.Context, handlerID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.exists[handlerID] {
		return handlerdomain.ErrNotFound
	}
	r.cipher[handlerID] = ""
	return nil
}

func (r *fakeRepo) GetConfigEncrypted(_ context.Context, handlerID string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.exists[handlerID] {
		return "", handlerdomain.ErrNotFound
	}
	return r.cipher[handlerID], nil
}

// All other Repository methods panic — they should not be hit by config tests.
//
// 其他 Repository 方法 panic — config 测试不该碰到。
func (r *fakeRepo) SaveHandler(context.Context, *handlerdomain.Handler) error { panic("not implemented") }
func (r *fakeRepo) GetHandler(context.Context, string) (*handlerdomain.Handler, error) {
	panic("not implemented")
}
func (r *fakeRepo) GetHandlerByName(context.Context, string) (*handlerdomain.Handler, error) {
	panic("not implemented")
}
func (r *fakeRepo) GetHandlersByIDs(context.Context, []string) ([]*handlerdomain.Handler, error) {
	panic("not implemented")
}
func (r *fakeRepo) ListHandlers(context.Context, handlerdomain.ListFilter) ([]*handlerdomain.Handler, string, error) {
	panic("not implemented")
}
func (r *fakeRepo) ListAllHandlers(context.Context) ([]*handlerdomain.Handler, error) {
	panic("not implemented")
}
func (r *fakeRepo) DeleteHandler(context.Context, string) error           { panic("not implemented") }
func (r *fakeRepo) SetActiveVersion(context.Context, string, string) error { panic("not implemented") }
func (r *fakeRepo) SaveVersion(context.Context, *handlerdomain.Version) error {
	panic("not implemented")
}
func (r *fakeRepo) GetVersion(context.Context, string) (*handlerdomain.Version, error) {
	panic("not implemented")
}
func (r *fakeRepo) GetVersionByNumber(context.Context, string, int) (*handlerdomain.Version, error) {
	panic("not implemented")
}
func (r *fakeRepo) ListVersions(context.Context, string, handlerdomain.VersionListFilter) ([]*handlerdomain.Version, string, error) {
	panic("not implemented")
}
func (r *fakeRepo) GetPending(context.Context, string) (*handlerdomain.Version, error) {
	panic("not implemented")
}
func (r *fakeRepo) UpdateVersionStatus(context.Context, string, string, *int) error {
	panic("not implemented")
}
func (r *fakeRepo) UpdateVersionEnv(context.Context, string, string, string, string, string, *time.Time) error {
	panic("not implemented")
}
func (r *fakeRepo) HardDeleteOldestAccepted(context.Context, string, int) error {
	panic("not implemented")
}
func (r *fakeRepo) HardDeleteVersion(context.Context, string) error {
	panic("not implemented")
}

// Call-log methods (D22) — config tests never hit these.
func (r *fakeRepo) SaveCall(context.Context, *handlerdomain.Call) error { panic("not implemented") }
func (r *fakeRepo) GetCallByID(context.Context, string) (*handlerdomain.Call, error) {
	panic("not implemented")
}
func (r *fakeRepo) ListCalls(context.Context, handlerdomain.CallFilter) ([]*handlerdomain.Call, string, error) {
	panic("not implemented")
}
func (r *fakeRepo) ComputeCallAggregates(context.Context, handlerdomain.CallFilter) (handlerdomain.CallAggregates, error) {
	panic("not implemented")
}

// Trick: handlerdomain.Repository.UpdateVersionEnv has a *time.Time arg, not
// *struct{}. We need to satisfy the actual interface. Switch to embedding a
// nil Repository so unimplemented methods auto-panic. Above panics are
// scaffolding for compile-time interface satisfaction; the real check is
// embedding pattern.

// configTestSvc builds a Service with fakeEncryptor + fakeRepo + nil sandbox.
// Suitable for config-method tests that don't touch sandbox.
//
// configTestSvc 构造 fakeEncryptor + fakeRepo + nil sandbox 的 Service。
func configTestSvc(t *testing.T) (*Service, *fakeRepo) {
	t.Helper()
	repo := newFakeRepo()
	svc := NewService(
		repo,                        // satisfy Repository interface
		nil,                                      // sandbox not used by config methods
		DefaultClientFactory,                     // unused; required non-nil if you choose
		fakeEncryptor{},
		notificationspkg.New(silentBridge{}, zap.NewNop()),
		zap.NewNop(),
	)
	return svc, repo
}

// silentBridge implements notificationsdomain.Bridge as a no-op for tests.
//
// silentBridge 实现 notificationsdomain.Bridge 当 no-op 测试用。
type silentBridge struct{}

func (silentBridge) Publish(_ context.Context, e notificationsdomain.Event) (notificationsdomain.Envelope, error) {
	return notificationsdomain.Envelope{Event: e}, nil
}
func (silentBridge) Subscribe(_ context.Context, _ int64) (<-chan notificationsdomain.Envelope, func(), error) {
	ch := make(chan notificationsdomain.Envelope)
	return ch, func() {}, nil
}
func (silentBridge) Replay(_ context.Context, fromSeq int64) ([]notificationsdomain.Envelope, error) {
	return nil, nil
}

// fakeRepo IS the Repository for these tests (panic stubs everywhere except
// the 3 config methods).
//
// fakeRepo 直接当 Repository 用(panic stub 占位剩下方法)。

func ctxFor(userID string) context.Context {
	return reqctxpkg.SetUserID(context.Background(), userID)
}

// ── ConfigState ──────────────────────────────────────────────────────────────

func TestComputeConfigState_AllStates(t *testing.T) {
	svc, repo := configTestSvc(t)
	repo.addHandler("hd1")
	ctx := ctxFor("u-alice")

	schema := []handlerdomain.InitArgSpec{
		{Name: "dsn", Type: "string", Required: true, Sensitive: true},
		{Name: "schema", Type: "string", Required: false},
	}

	// 1. unconfigured
	state, missing, err := svc.ComputeConfigState(ctx, "hd1", schema)
	if err != nil {
		t.Fatalf("ComputeConfigState: %v", err)
	}
	if state != handlerdomain.ConfigStateUnconfigured {
		t.Errorf("state = %q, want unconfigured", state)
	}
	if len(missing) != 1 || missing[0] != "dsn" {
		t.Errorf("missing = %v, want [dsn]", missing)
	}

	// 2. partially_configured — add a non-required key
	if err := svc.UpdateConfig(ctx, "hd1", map[string]any{"schema": "public"}); err != nil {
		t.Fatalf("UpdateConfig: %v", err)
	}
	state, missing, _ = svc.ComputeConfigState(ctx, "hd1", schema)
	if state != handlerdomain.ConfigStateUnconfigured {
		// Note: schema is not required, so missing dsn means we're still
		// unconfigured (all required missing). Not "partial".
		t.Errorf("state with only non-required filled = %q, want unconfigured (dsn missing)", state)
	}

	// 3. ready — fill dsn
	if err := svc.UpdateConfig(ctx, "hd1", map[string]any{"dsn": "postgres://..."}); err != nil {
		t.Fatalf("UpdateConfig dsn: %v", err)
	}
	state, missing, _ = svc.ComputeConfigState(ctx, "hd1", schema)
	if state != handlerdomain.ConfigStateReady {
		t.Errorf("state = %q, want ready", state)
	}
	if len(missing) != 0 {
		t.Errorf("missing should be empty when ready; got %v", missing)
	}
}

func TestComputeConfigState_PartiallyConfigured(t *testing.T) {
	svc, repo := configTestSvc(t)
	repo.addHandler("hd1")
	ctx := ctxFor("u-alice")

	schema := []handlerdomain.InitArgSpec{
		{Name: "host", Type: "string", Required: true},
		{Name: "port", Type: "integer", Required: true},
	}
	_ = svc.UpdateConfig(ctx, "hd1", map[string]any{"host": "localhost"})
	state, missing, _ := svc.ComputeConfigState(ctx, "hd1", schema)
	if state != handlerdomain.ConfigStatePartiallyConfigured {
		t.Errorf("state = %q, want partially_configured", state)
	}
	if len(missing) != 1 || missing[0] != "port" {
		t.Errorf("missing = %v, want [port]", missing)
	}
}

// ── LoadConfig / UpdateConfig round-trip ─────────────────────────────────────

func TestLoadConfig_RoundTrip(t *testing.T) {
	svc, repo := configTestSvc(t)
	repo.addHandler("hd1")
	ctx := ctxFor("u-alice")

	// Empty initially.
	cfg, err := svc.LoadConfig(ctx, "hd1")
	if err != nil {
		t.Fatalf("LoadConfig empty: %v", err)
	}
	if cfg != nil {
		t.Errorf("expected nil for unconfigured; got %v", cfg)
	}

	// Update with patch.
	if err := svc.UpdateConfig(ctx, "hd1", map[string]any{"k1": "v1", "k2": 42}); err != nil {
		t.Fatalf("UpdateConfig: %v", err)
	}
	cfg, _ = svc.LoadConfig(ctx, "hd1")
	if cfg["k1"] != "v1" {
		t.Errorf("k1 = %v, want v1", cfg["k1"])
	}
	if cfg["k2"].(float64) != 42 {
		t.Errorf("k2 = %v, want 42", cfg["k2"])
	}
}

func TestUpdateConfig_MergeOverwrites(t *testing.T) {
	svc, repo := configTestSvc(t)
	repo.addHandler("hd1")
	ctx := ctxFor("u-alice")

	_ = svc.UpdateConfig(ctx, "hd1", map[string]any{"k1": "old", "k2": "keep"})
	_ = svc.UpdateConfig(ctx, "hd1", map[string]any{"k1": "new"}) // only k1
	cfg, _ := svc.LoadConfig(ctx, "hd1")
	if cfg["k1"] != "new" {
		t.Errorf("k1 = %v, want new", cfg["k1"])
	}
	if cfg["k2"] != "keep" {
		t.Errorf("k2 = %v, want keep", cfg["k2"])
	}
}

func TestUpdateConfig_NilDeletesKey(t *testing.T) {
	svc, repo := configTestSvc(t)
	repo.addHandler("hd1")
	ctx := ctxFor("u-alice")

	_ = svc.UpdateConfig(ctx, "hd1", map[string]any{"k1": "v", "k2": "keep"})
	// Map with nil value → mergePatch deletes the key.
	_ = svc.UpdateConfig(ctx, "hd1", map[string]any{"k1": nil})
	cfg, _ := svc.LoadConfig(ctx, "hd1")
	if _, ok := cfg["k1"]; ok {
		t.Errorf("k1 should be deleted, got %v", cfg["k1"])
	}
	if cfg["k2"] != "keep" {
		t.Errorf("k2 = %v, want keep", cfg["k2"])
	}
}

func TestClearConfig_BackToUnconfigured(t *testing.T) {
	svc, repo := configTestSvc(t)
	repo.addHandler("hd1")
	ctx := ctxFor("u-alice")

	_ = svc.UpdateConfig(ctx, "hd1", map[string]any{"k": "v"})
	_ = svc.ClearConfig(ctx, "hd1")
	cfg, _ := svc.LoadConfig(ctx, "hd1")
	if cfg != nil {
		t.Errorf("after Clear: expected nil, got %v", cfg)
	}
}

// ── MaskedConfig ─────────────────────────────────────────────────────────────

func TestMaskedConfig_SensitiveReplaced(t *testing.T) {
	svc, repo := configTestSvc(t)
	repo.addHandler("hd1")
	ctx := ctxFor("u-alice")

	_ = svc.UpdateConfig(ctx, "hd1", map[string]any{
		"dsn":    "postgres://user:secret@db",
		"schema": "public",
	})
	schema := []handlerdomain.InitArgSpec{
		{Name: "dsn", Sensitive: true},
		{Name: "schema", Sensitive: false},
	}
	masked, err := svc.MaskedConfig(ctx, "hd1", schema)
	if err != nil {
		t.Fatalf("MaskedConfig: %v", err)
	}
	if masked["dsn"] != "********" {
		t.Errorf("dsn not masked: %v", masked["dsn"])
	}
	if masked["schema"] != "public" {
		t.Errorf("non-sensitive masked: %v", masked["schema"])
	}
}

// ── Decrypt failure path ─────────────────────────────────────────────────────

// brokenEncryptor returns an error from Decrypt to exercise ErrConfigDecryptFailed.
//
// brokenEncryptor Decrypt 返错,触发 ErrConfigDecryptFailed。
type brokenEncryptor struct{}

func (brokenEncryptor) Encrypt(_ context.Context, p []byte) ([]byte, error) { return p, nil }
func (brokenEncryptor) Decrypt(_ context.Context, _ []byte) ([]byte, error) {
	return nil, errors.New("AES key mismatch")
}

func TestLoadConfig_DecryptFailed(t *testing.T) {
	repo := newFakeRepo()
	repo.addHandler("hd1")
	repo.cipher["hd1"] = "garbled-ciphertext"

	svc := NewService(
		repo,
		nil,
		DefaultClientFactory,
		brokenEncryptor{},
		notificationspkg.New(silentBridge{}, zap.NewNop()),
		zap.NewNop(),
	)
	ctx := ctxFor("u-alice")

	_, err := svc.LoadConfig(ctx, "hd1")
	if !errors.Is(err, handlerdomain.ErrConfigDecryptFailed) {
		t.Errorf("expected ErrConfigDecryptFailed, got %v", err)
	}
}

// Suppress json import warning in case test file moves.
var _ = json.RawMessage{}
