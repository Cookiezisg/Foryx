package apikey

import (
	"context"
	"errors"
	"strings"
	"testing"

	"go.uber.org/zap"

	apikeydomain "github.com/sunweilin/forgify/backend/internal/domain/apikey"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

// --- fakes ---

type fakeRepo struct {
	items map[string]*apikeydomain.APIKey
}

func newFakeRepo() *fakeRepo { return &fakeRepo{items: map[string]*apikeydomain.APIKey{}} }

func (f *fakeRepo) Get(_ context.Context, id string) (*apikeydomain.APIKey, error) {
	k, ok := f.items[id]
	if !ok {
		return nil, apikeydomain.ErrNotFound
	}
	cp := *k
	return &cp, nil
}

func (f *fakeRepo) List(_ context.Context, filter apikeydomain.ListFilter) ([]*apikeydomain.APIKey, string, error) {
	out := []*apikeydomain.APIKey{}
	for _, k := range f.items {
		if filter.Provider != "" && k.Provider != filter.Provider {
			continue
		}
		cp := *k
		out = append(out, &cp)
	}
	return out, "", nil
}

func (f *fakeRepo) Save(_ context.Context, k *apikeydomain.APIKey) error {
	for id, ex := range f.items {
		if id != k.ID && ex.DisplayName == k.DisplayName {
			return apikeydomain.ErrDisplayNameConflict
		}
	}
	cp := *k
	f.items[k.ID] = &cp
	return nil
}

func (f *fakeRepo) Delete(_ context.Context, id string) error {
	if _, ok := f.items[id]; !ok {
		return apikeydomain.ErrNotFound
	}
	delete(f.items, id)
	return nil
}

func (f *fakeRepo) UpdateTestResult(_ context.Context, id, status, errMsg, response string) error {
	k, ok := f.items[id]
	if !ok {
		return apikeydomain.ErrNotFound
	}
	k.TestStatus, k.TestError, k.TestResponse = status, errMsg, response
	return nil
}

func (f *fakeRepo) ListProbed(_ context.Context) ([]apikeydomain.ProbedKey, error) {
	out := []apikeydomain.ProbedKey{}
	for _, k := range f.items {
		out = append(out, apikeydomain.ProbedKey{Provider: k.Provider, TestStatus: k.TestStatus, TestResponse: k.TestResponse})
	}
	return out, nil
}

// fakeEncryptor is a reversible non-crypto stand-in proving the boundary is exercised.
type fakeEncryptor struct{}

func (fakeEncryptor) Encrypt(_ context.Context, plain []byte) ([]byte, error) {
	return []byte("ENC:" + string(plain)), nil
}
func (fakeEncryptor) Decrypt(_ context.Context, ct []byte) ([]byte, error) {
	return []byte(strings.TrimPrefix(string(ct), "ENC:")), nil
}

type fakeTester struct {
	result *TestResult
	err    error
}

func (f fakeTester) Test(context.Context, string, string, string, string) (*TestResult, error) {
	return f.result, f.err
}

type fakeScanner struct{ used bool }

func (f fakeScanner) ReferencesAPIKey(context.Context, string) (bool, error) { return f.used, nil }

func newSvc(tester ConnectivityTester) (*Service, *fakeRepo) {
	repo := newFakeRepo()
	return NewService(repo, fakeEncryptor{}, tester, zap.NewNop()), repo
}

func ctxWS() context.Context { return reqctxpkg.SetWorkspaceID(context.Background(), "ws_1") }

// --- tests ---

func TestCreate_EncryptsAndMasks(t *testing.T) {
	s, _ := newSvc(nil)
	k, err := s.Create(ctxWS(), CreateInput{Provider: "openai", DisplayName: "main", Key: "sk-abcdefghijklmnop"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if k.KeyEncrypted != "ENC:sk-abcdefghijklmnop" {
		t.Errorf("not encrypted: %q", k.KeyEncrypted)
	}
	if k.KeyMasked == "" || strings.Contains(k.KeyMasked, "efghij") {
		t.Errorf("not masked: %q", k.KeyMasked)
	}
	if k.TestStatus != apikeydomain.TestStatusPending || !strings.HasPrefix(k.ID, "aki_") {
		t.Errorf("got %+v", k)
	}
}

func TestCreate_Validation(t *testing.T) {
	s, _ := newSvc(nil)
	cases := []struct {
		name string
		in   CreateInput
		want error
	}{
		{"unknown provider", CreateInput{Provider: "nope", Key: "k"}, apikeydomain.ErrInvalidProvider},
		{"empty key", CreateInput{Provider: "openai", Key: "  "}, apikeydomain.ErrKeyRequired},
		{"ollama needs baseURL", CreateInput{Provider: "ollama", Key: "k"}, apikeydomain.ErrBaseURLRequired},
		{"custom needs apiFormat", CreateInput{Provider: "custom", Key: "k", BaseURL: "http://x"}, apikeydomain.ErrAPIFormatRequired},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := s.Create(ctxWS(), c.in); !errors.Is(err, c.want) {
				t.Errorf("err = %v, want %v", err, c.want)
			}
		})
	}
}

func TestUpdate_KeyRotationResetsProbe(t *testing.T) {
	s, repo := newSvc(nil)
	k, _ := s.Create(ctxWS(), CreateInput{Provider: "openai", DisplayName: "m", Key: "sk-old1234567890"})
	repo.items[k.ID].TestStatus = apikeydomain.TestStatusOK
	repo.items[k.ID].TestResponse = `{"data":[]}`

	newKey := "sk-new1234567890"
	got, err := s.Update(ctxWS(), k.ID, UpdateInput{Key: &newKey})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if got.KeyEncrypted != "ENC:sk-new1234567890" {
		t.Errorf("key not rotated: %q", got.KeyEncrypted)
	}
	if got.TestStatus != apikeydomain.TestStatusPending || got.TestResponse != "" {
		t.Errorf("probe archive not reset: status=%q response=%q", got.TestStatus, got.TestResponse)
	}
}

func TestDelete_RefScannerBlocks(t *testing.T) {
	s, _ := newSvc(nil)
	k, _ := s.Create(ctxWS(), CreateInput{Provider: "openai", DisplayName: "m", Key: "sk-1234567890"})
	s.AddRefScanner(fakeScanner{used: true})
	if err := s.Delete(ctxWS(), k.ID); !errors.Is(err, apikeydomain.ErrInUse) {
		t.Errorf("err = %v, want ErrInUse", err)
	}
}

func TestDelete_OKWhenUnreferenced(t *testing.T) {
	s, _ := newSvc(nil)
	k, _ := s.Create(ctxWS(), CreateInput{Provider: "openai", DisplayName: "m", Key: "sk-1234567890"})
	s.AddRefScanner(fakeScanner{used: false})
	if err := s.Delete(ctxWS(), k.ID); err != nil {
		t.Errorf("delete: %v", err)
	}
}

func TestTest_OKPersistsRawResponse(t *testing.T) {
	raw := `{"data":[{"id":"gpt-5"}]}`
	s, repo := newSvc(fakeTester{result: &TestResult{OK: true, Message: "connected", RawResponse: raw}})
	k, _ := s.Create(ctxWS(), CreateInput{Provider: "openai", DisplayName: "m", Key: "sk-1234567890"})

	res, err := s.Test(ctxWS(), k.ID)
	if err != nil || !res.OK {
		t.Fatalf("test: res=%+v err=%v", res, err)
	}
	stored := repo.items[k.ID]
	if stored.TestStatus != apikeydomain.TestStatusOK || stored.TestResponse != raw {
		t.Errorf("raw not archived: status=%q response=%q", stored.TestStatus, stored.TestResponse)
	}
}

func TestTest_FailPersistsError(t *testing.T) {
	s, repo := newSvc(fakeTester{result: &TestResult{OK: false, Message: "HTTP 401"}})
	k, _ := s.Create(ctxWS(), CreateInput{Provider: "openai", DisplayName: "m", Key: "sk-1234567890"})

	if _, err := s.Test(ctxWS(), k.ID); err != nil {
		t.Fatalf("test: %v", err)
	}
	stored := repo.items[k.ID]
	if stored.TestStatus != apikeydomain.TestStatusError || stored.TestError != "HTTP 401" || stored.TestResponse != "" {
		t.Errorf("failure not persisted right: %+v", stored)
	}
}

func TestResolveCredentialsByID_DecryptsAndFallsBackBaseURL(t *testing.T) {
	s, _ := newSvc(nil)
	k, _ := s.Create(ctxWS(), CreateInput{Provider: "openai", DisplayName: "m", Key: "sk-secret123456"})

	creds, err := s.ResolveCredentialsByID(ctxWS(), k.ID)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if creds.Key != "sk-secret123456" {
		t.Errorf("not decrypted: %q", creds.Key)
	}
	if creds.BaseURL != "https://api.openai.com/v1" {
		t.Errorf("baseURL fallback failed: %q", creds.BaseURL)
	}
}

func TestMarkInvalidByID(t *testing.T) {
	s, repo := newSvc(nil)
	k, _ := s.Create(ctxWS(), CreateInput{Provider: "openai", DisplayName: "m", Key: "sk-1234567890"})
	if err := s.MarkInvalidByID(ctxWS(), k.ID, "401 from caller"); err != nil {
		t.Fatalf("mark: %v", err)
	}
	if repo.items[k.ID].TestStatus != apikeydomain.TestStatusError {
		t.Errorf("not marked invalid: %q", repo.items[k.ID].TestStatus)
	}
}
