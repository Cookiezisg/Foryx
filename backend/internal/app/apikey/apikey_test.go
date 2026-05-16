package apikey

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap/zaptest"

	apikeydomain "github.com/sunweilin/forgify/backend/internal/domain/apikey"
	cryptoinfra "github.com/sunweilin/forgify/backend/internal/infra/crypto"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

type fakeRepo struct {
	items map[string]*apikeydomain.APIKey
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{items: map[string]*apikeydomain.APIKey{}}
}

func (r *fakeRepo) Get(ctx context.Context, id string) (*apikeydomain.APIKey, error) {
	uid, _ := reqctxpkg.GetUserID(ctx)
	k, ok := r.items[id]
	if !ok || k.UserID != uid {
		return nil, apikeydomain.ErrNotFound
	}
	return k, nil
}

func (r *fakeRepo) List(ctx context.Context, filter apikeydomain.ListFilter) ([]*apikeydomain.APIKey, string, error) {
	uid, _ := reqctxpkg.GetUserID(ctx)
	out := []*apikeydomain.APIKey{}
	for _, k := range r.items {
		if k.UserID != uid {
			continue
		}
		if filter.Provider != "" && k.Provider != filter.Provider {
			continue
		}
		out = append(out, k)
	}
	return out, "", nil
}

func (r *fakeRepo) GetByProvider(ctx context.Context, provider string) (*apikeydomain.APIKey, error) {
	uid, _ := reqctxpkg.GetUserID(ctx)
	var best *apikeydomain.APIKey
	for _, k := range r.items {
		if k.UserID != uid || k.Provider != provider {
			continue
		}
		if best == nil ||
			(k.TestStatus == apikeydomain.TestStatusOK && best.TestStatus != apikeydomain.TestStatusOK) ||
			(k.TestStatus == best.TestStatus && k.CreatedAt.After(best.CreatedAt)) {
			best = k
		}
	}
	if best == nil {
		return nil, apikeydomain.ErrNotFoundForProvider
	}
	return best, nil
}

func (r *fakeRepo) Save(_ context.Context, k *apikeydomain.APIKey) error {
	r.items[k.ID] = k
	return nil
}

func (r *fakeRepo) Delete(_ context.Context, id string) error {
	if _, ok := r.items[id]; !ok {
		return apikeydomain.ErrNotFound
	}
	delete(r.items, id)
	return nil
}

func (r *fakeRepo) UpdateTestResult(_ context.Context, id, status, errMsg string, _ []string) error {
	k, ok := r.items[id]
	if !ok {
		return apikeydomain.ErrNotFound
	}
	k.TestStatus = status
	k.TestError = errMsg
	now := time.Now().UTC()
	k.LastTestedAt = &now
	return nil
}

type fakeTester struct {
	result *TestResult
	err    error
	calls  int
}

func (t *fakeTester) Test(_ context.Context, _, _, _, _ string) (*TestResult, error) {
	t.calls++
	return t.result, t.err
}

func newTestService(t *testing.T, tester ConnectivityTester) (*Service, *fakeRepo) {
	t.Helper()
	enc, err := cryptoinfra.NewAESGCMEncryptor(cryptoinfra.DeriveKey("service-test-fixture"))
	if err != nil {
		t.Fatalf("NewAESGCMEncryptor: %v", err)
	}
	repo := newFakeRepo()
	svc := NewService(repo, enc, tester, zaptest.NewLogger(t))
	return svc, repo
}

func ctxFor(userID string) context.Context {
	return reqctxpkg.SetUserID(context.Background(), userID)
}

func providersOf(ks []*apikeydomain.APIKey) []string {
	out := make([]string, len(ks))
	for i, k := range ks {
		out[i] = k.Provider
	}
	return out
}

func TestService_Create_Success(t *testing.T) {
	svc, repo := newTestService(t, &fakeTester{})
	ctx := ctxFor("u-alice")

	k, err := svc.Create(ctx, CreateInput{
		Provider:    "openai",
		DisplayName: "Main OpenAI",
		Key:         "sk-proj-abcdefg1234567890xyz",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if !strings.HasPrefix(k.ID, "aki_") {
		t.Errorf("ID = %q, want prefix aki_", k.ID)
	}
	if k.UserID != "u-alice" {
		t.Errorf("UserID = %q, want u-alice", k.UserID)
	}
	if k.KeyMasked != "sk-proj...0xyz" {
		t.Errorf("KeyMasked = %q, want sk-proj...0xyz", k.KeyMasked)
	}
	if k.KeyEncrypted == "" || k.KeyEncrypted == "sk-proj-abcdefg1234567890xyz" {
		t.Errorf("KeyEncrypted = %q, want non-empty ciphertext", k.KeyEncrypted)
	}
	if !strings.HasPrefix(k.KeyEncrypted, "v1:") {
		t.Errorf("KeyEncrypted = %q, want v1: prefix", k.KeyEncrypted)
	}
	if k.TestStatus != apikeydomain.TestStatusPending {
		t.Errorf("TestStatus = %q, want pending", k.TestStatus)
	}
	if _, ok := repo.items[k.ID]; !ok {
		t.Error("repo did not store the new key")
	}
}

func TestService_Create_ValidationErrors(t *testing.T) {
	cases := []struct {
		name string
		in   CreateInput
		want error
	}{
		{"unknown provider", CreateInput{Provider: "notreal", Key: "k"}, apikeydomain.ErrInvalidProvider},
		{"empty key", CreateInput{Provider: "openai", Key: "  "}, apikeydomain.ErrKeyRequired},
		{"ollama missing baseURL", CreateInput{Provider: "ollama", Key: "k"}, apikeydomain.ErrBaseURLRequired},
		{"custom missing apiFormat", CreateInput{Provider: "custom", Key: "k", BaseURL: "http://x"}, apikeydomain.ErrAPIFormatRequired},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			svc, repo := newTestService(t, &fakeTester{})
			_, err := svc.Create(ctxFor("u"), c.in)
			if !errors.Is(err, c.want) {
				t.Errorf("err = %v, want %v", err, c.want)
			}
			if len(repo.items) != 0 {
				t.Errorf("repo got %d items on validation error, want 0", len(repo.items))
			}
		})
	}
}

func TestService_Create_MissingUserID_Errors(t *testing.T) {
	svc, _ := newTestService(t, &fakeTester{})
	_, err := svc.Create(context.Background(), CreateInput{Provider: "openai", Key: "sk-x"})
	if err == nil {
		t.Fatal("want error when userID missing")
	}
	if !strings.Contains(err.Error(), "missing user id") {
		t.Errorf("err = %v, want message about missing user id", err)
	}
}

func TestService_Create_UsesCustomBaseURLAndAPIFormat(t *testing.T) {
	svc, _ := newTestService(t, &fakeTester{})
	k, err := svc.Create(ctxFor("u"), CreateInput{
		Provider:  "custom",
		Key:       "sk-x",
		BaseURL:   "https://proxy.example.com/v1",
		APIFormat: apikeydomain.APIFormatAnthropicCompatible,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if k.BaseURL != "https://proxy.example.com/v1" {
		t.Errorf("BaseURL = %q", k.BaseURL)
	}
	if k.APIFormat != apikeydomain.APIFormatAnthropicCompatible {
		t.Errorf("APIFormat = %q", k.APIFormat)
	}
}

func TestService_Update_PartialFields(t *testing.T) {
	svc, _ := newTestService(t, &fakeTester{})
	ctx := ctxFor("u")
	created, _ := svc.Create(ctx, CreateInput{Provider: "openai", DisplayName: "Old", Key: "sk-x"})

	newName := "New Display"
	updated, err := svc.Update(ctx, created.ID, UpdateInput{DisplayName: &newName})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.DisplayName != "New Display" {
		t.Errorf("DisplayName = %q, want New Display", updated.DisplayName)
	}
	if updated.BaseURL != created.BaseURL {
		t.Errorf("BaseURL changed unexpectedly")
	}
	if updated.UpdatedAt.Before(created.UpdatedAt) {
		t.Error("UpdatedAt did not advance")
	}
}

func TestService_Update_NotFound(t *testing.T) {
	svc, _ := newTestService(t, &fakeTester{})
	name := "x"
	_, err := svc.Update(ctxFor("u"), "nonexistent", UpdateInput{DisplayName: &name})
	if !errors.Is(err, apikeydomain.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestService_Delete_RemovesEntry(t *testing.T) {
	svc, repo := newTestService(t, &fakeTester{})
	ctx := ctxFor("u")
	k, _ := svc.Create(ctx, CreateInput{Provider: "openai", Key: "sk-x"})
	if err := svc.Delete(ctx, k.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, ok := repo.items[k.ID]; ok {
		t.Error("repo still has entry after Delete")
	}
}

func TestService_Delete_NotFound(t *testing.T) {
	svc, _ := newTestService(t, &fakeTester{})
	err := svc.Delete(ctxFor("u"), "nope")
	if !errors.Is(err, apikeydomain.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestService_Test_Success_WritesOKStatus(t *testing.T) {
	tester := &fakeTester{result: &TestResult{OK: true, Message: "ok", LatencyMs: 42}}
	svc, repo := newTestService(t, tester)
	ctx := ctxFor("u")
	k, _ := svc.Create(ctx, CreateInput{Provider: "openai", Key: "sk-x"})

	res, err := svc.Test(ctx, k.ID)
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	if !res.OK || res.LatencyMs != 42 {
		t.Errorf("result = %+v, want OK=true LatencyMs=42", res)
	}
	if tester.calls != 1 {
		t.Errorf("tester calls = %d, want 1", tester.calls)
	}
	stored := repo.items[k.ID]
	if stored.TestStatus != apikeydomain.TestStatusOK {
		t.Errorf("TestStatus = %q, want ok", stored.TestStatus)
	}
	if stored.LastTestedAt == nil {
		t.Error("LastTestedAt nil after successful test")
	}
}

func TestService_Test_Failure_WritesErrorStatus(t *testing.T) {
	tester := &fakeTester{result: &TestResult{OK: false, Message: "HTTP 401: invalid"}}
	svc, repo := newTestService(t, tester)
	ctx := ctxFor("u")
	k, _ := svc.Create(ctx, CreateInput{Provider: "openai", Key: "sk-x"})

	res, err := svc.Test(ctx, k.ID)
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	if res.OK {
		t.Error("result.OK = true, want false")
	}
	stored := repo.items[k.ID]
	if stored.TestStatus != apikeydomain.TestStatusError {
		t.Errorf("TestStatus = %q, want error", stored.TestStatus)
	}
	if stored.TestError != "HTTP 401: invalid" {
		t.Errorf("TestError = %q", stored.TestError)
	}
}

func TestService_Test_TesterProgrammerBug_RecordedAndPropagated(t *testing.T) {
	tester := &fakeTester{err: errors.New("unknown provider")}
	svc, repo := newTestService(t, tester)
	ctx := ctxFor("u")
	k, _ := svc.Create(ctx, CreateInput{Provider: "openai", Key: "sk-x"})

	_, err := svc.Test(ctx, k.ID)
	if err == nil {
		t.Fatal("want error when tester returns error")
	}
	if repo.items[k.ID].TestStatus != apikeydomain.TestStatusError {
		t.Error("test_status not recorded as error on programmer-bug path")
	}
}

func TestService_Test_NotFound(t *testing.T) {
	svc, _ := newTestService(t, &fakeTester{})
	_, err := svc.Test(ctxFor("u"), "nope")
	if !errors.Is(err, apikeydomain.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestService_List_FiltersByProvider(t *testing.T) {
	svc, _ := newTestService(t, &fakeTester{})
	ctx := ctxFor("u")
	_, _ = svc.Create(ctx, CreateInput{Provider: "openai", Key: "sk-x"})
	_, _ = svc.Create(ctx, CreateInput{Provider: "anthropic", Key: "sk-ant-y"})

	got, _, err := svc.List(ctx, apikeydomain.ListFilter{Provider: "openai"})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 1 || got[0].Provider != "openai" {
		t.Errorf("got %v, want exactly 1 openai", providersOf(got))
	}
}

func TestService_ResolveCredentials_DecryptsAndMergesDefaultBaseURL(t *testing.T) {
	svc, _ := newTestService(t, &fakeTester{})
	ctx := ctxFor("u")
	_, _ = svc.Create(ctx, CreateInput{Provider: "openai", Key: "sk-plaintext"})

	creds, err := svc.ResolveCredentials(ctx, "openai")
	if err != nil {
		t.Fatalf("ResolveCredentials: %v", err)
	}
	if creds.Key != "sk-plaintext" {
		t.Errorf("Key = %q, want sk-plaintext", creds.Key)
	}
	if creds.BaseURL != "https://api.openai.com/v1" {
		t.Errorf("BaseURL = %q, want provider default", creds.BaseURL)
	}
}

func TestService_ResolveCredentials_UserBaseURLOverridesDefault(t *testing.T) {
	svc, _ := newTestService(t, &fakeTester{})
	ctx := ctxFor("u")
	_, _ = svc.Create(ctx, CreateInput{Provider: "openai", Key: "sk-x", BaseURL: "https://custom.proxy/v1"})

	creds, err := svc.ResolveCredentials(ctx, "openai")
	if err != nil {
		t.Fatalf("ResolveCredentials: %v", err)
	}
	if creds.BaseURL != "https://custom.proxy/v1" {
		t.Errorf("BaseURL = %q, want user-supplied override", creds.BaseURL)
	}
}

func TestService_ResolveCredentials_NoKeyForProvider(t *testing.T) {
	svc, _ := newTestService(t, &fakeTester{})
	_, err := svc.ResolveCredentials(ctxFor("u"), "openai")
	if !errors.Is(err, apikeydomain.ErrNotFoundForProvider) {
		t.Errorf("err = %v, want ErrNotFoundForProvider", err)
	}
}

func TestService_MarkInvalid_UpdatesTestResult(t *testing.T) {
	svc, repo := newTestService(t, &fakeTester{})
	ctx := ctxFor("u")
	k, _ := svc.Create(ctx, CreateInput{Provider: "openai", Key: "sk-x"})

	if err := svc.MarkInvalid(ctx, "openai", "chat returned 401"); err != nil {
		t.Fatalf("MarkInvalid: %v", err)
	}
	stored := repo.items[k.ID]
	if stored.TestStatus != apikeydomain.TestStatusError {
		t.Errorf("TestStatus = %q, want error", stored.TestStatus)
	}
	if stored.TestError != "chat returned 401" {
		t.Errorf("TestError = %q", stored.TestError)
	}
}

func TestService_MarkInvalid_NoKeyForProvider(t *testing.T) {
	svc, _ := newTestService(t, &fakeTester{})
	err := svc.MarkInvalid(ctxFor("u"), "openai", "nope")
	if !errors.Is(err, apikeydomain.ErrNotFoundForProvider) {
		t.Errorf("err = %v, want ErrNotFoundForProvider", err)
	}
}

func TestNewService_NilLogger_Panics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("NewService did not panic on nil logger")
		}
	}()
	_ = NewService(newFakeRepo(), nil, &fakeTester{}, nil)
}

func TestMaskKey(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"", "****"},
		{"abc", "****"},
		{"short12", "****"},
		{"12345678", "123...5678"},
		{"AKIA1234567890ABCDEF", "AKI...CDEF"},
		{"sk-proj-abcdefg1234567890xyz", "sk-proj...0xyz"},
		{"sk-ant-api01-xxxxxxxxxxxxxxxxyyyy", "sk-ant-...yyyy"},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			if got := maskKey(c.in); got != c.want {
				t.Errorf("maskKey(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestMaskKey_NeverLeaksMiddle(t *testing.T) {
	secret := "sk-proj-MIDDLE_SECRET_PART_xyz9"
	masked := maskKey(secret)
	if strings.Contains(masked, "MIDDLE_SECRET_PART") {
		t.Errorf("mask leaked middle of key: %q", masked)
	}
}
