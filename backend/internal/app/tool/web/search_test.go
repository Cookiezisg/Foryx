package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"

	apikeydomain "github.com/sunweilin/anselm/backend/internal/domain/apikey"
)

// fakeKeys implements apikeydomain.KeyProvider (shared with fetch_test).
type fakeKeys struct {
	creds  apikeydomain.Credentials
	err    error
	marked []string
}

func (f *fakeKeys) ResolveCredentialsByID(_ context.Context, _ string) (apikeydomain.Credentials, error) {
	return f.creds, f.err
}

func (f *fakeKeys) MarkInvalidByID(_ context.Context, id, _ string) error {
	f.marked = append(f.marked, id)
	return nil
}

// fakeSearchKeys implements websearchdomain.SearchKeyPicker.
type fakeSearchKeys struct {
	id string
	ok bool
}

func (f *fakeSearchKeys) DefaultSearchKeyID(_ context.Context) (string, bool) { return f.id, f.ok }

func newWS(searchKeys *fakeSearchKeys, keys *fakeKeys) *WebSearch {
	return &WebSearch{
		httpClient: &http.Client{Timeout: 5 * time.Second},
		keys:       keys,
		searchKeys: searchKeys,
		log:        zap.NewNop(),
	}
}

func TestWebSearch_ValidateInput(t *testing.T) {
	ws := &WebSearch{}
	if err := ws.ValidateInput([]byte(`{"query":""}`)); err != ErrEmptyQuery {
		t.Fatalf("empty query: %v", err)
	}
	if err := ws.ValidateInput([]byte(`{"query":"go","limit":-1}`)); err == nil {
		t.Fatalf("negative limit: want error")
	}
	if err := ws.ValidateInput([]byte(`{"query":"go"}`)); err != nil {
		t.Fatalf("happy: %v", err)
	}
}

func TestWebSearch_NoKey_Guidance(t *testing.T) {
	ws := newWS(&fakeSearchKeys{ok: false}, &fakeKeys{})
	out, err := ws.Execute(context.Background(), `{"query":"golang"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "No search backend") {
		t.Fatalf("expected guidance, got %q", out)
	}
}

func TestWebSearch_NonSearchProvider_Rejected(t *testing.T) {
	ws := newWS(&fakeSearchKeys{id: "ak", ok: true}, &fakeKeys{creds: apikeydomain.Credentials{Provider: "openai"}})
	out, err := ws.Execute(context.Background(), `{"query":"golang"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "not a search backend") {
		t.Fatalf("expected non-search-provider rejection, got %q", out)
	}
}

func TestWebSearch_Brave_Happy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Subscription-Token") != "k" {
			t.Errorf("brave: missing auth header")
		}
		_, _ = w.Write([]byte(`{"web":{"results":[{"title":"T1","url":"http://a","description":"D1"}]}}`))
	}))
	defer srv.Close()

	ws := newWS(&fakeSearchKeys{id: "ak", ok: true},
		&fakeKeys{creds: apikeydomain.Credentials{Provider: "brave", Key: "k", BaseURL: srv.URL}})
	out, err := ws.Execute(context.Background(), `{"query":"golang"}`)
	if err != nil {
		t.Fatal(err)
	}
	var resp searchResponse
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("output not JSON: %v\n%s", err, out)
	}
	if resp.Source != "brave" || len(resp.Results) != 1 || resp.Results[0].Title != "T1" || resp.Results[0].Snippet != "D1" {
		t.Fatalf("unexpected response: %+v", resp)
	}
}

func TestWebSearch_Tavily_Happy(t *testing.T) {
	// Tavily auth is api_key in the JSON body (POST), not a header.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["api_key"] != "k" {
			t.Errorf("tavily: api_key not in body, got %v", body["api_key"])
		}
		_, _ = w.Write([]byte(`{"results":[{"title":"T2","url":"http://b","content":"C2"}]}`))
	}))
	defer srv.Close()

	ws := newWS(&fakeSearchKeys{id: "ak", ok: true},
		&fakeKeys{creds: apikeydomain.Credentials{Provider: "tavily", Key: "k", BaseURL: srv.URL}})
	out, err := ws.Execute(context.Background(), `{"query":"golang"}`)
	if err != nil {
		t.Fatal(err)
	}
	var resp searchResponse
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out)
	}
	if resp.Source != "tavily" || len(resp.Results) != 1 || resp.Results[0].Snippet != "C2" {
		t.Fatalf("unexpected: %+v", resp)
	}
}

func TestWebSearch_AuthError_MarksInvalid(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"bad key"}`))
	}))
	defer srv.Close()

	keys := &fakeKeys{creds: apikeydomain.Credentials{Provider: "brave", Key: "bad", BaseURL: srv.URL}}
	ws := newWS(&fakeSearchKeys{id: "ak_bad", ok: true}, keys)
	out, err := ws.Execute(context.Background(), `{"query":"golang"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "failed") {
		t.Fatalf("expected failure message, got %q", out)
	}
	// 401 → MarkInvalidByID called with the key id.
	if len(keys.marked) != 1 || keys.marked[0] != "ak_bad" {
		t.Fatalf("expected MarkInvalidByID(ak_bad), got %v", keys.marked)
	}
}

func TestWebSearch_LimitTruncates(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"web":{"results":[
			{"title":"1","url":"u1","description":"d"},
			{"title":"2","url":"u2","description":"d"},
			{"title":"3","url":"u3","description":"d"}
		]}}`))
	}))
	defer srv.Close()
	ws := newWS(&fakeSearchKeys{id: "ak", ok: true},
		&fakeKeys{creds: apikeydomain.Credentials{Provider: "brave", Key: "k", BaseURL: srv.URL}})
	out, _ := ws.Execute(context.Background(), `{"query":"go","limit":2}`)
	var resp searchResponse
	_ = json.Unmarshal([]byte(out), &resp)
	if len(resp.Results) != 2 || !resp.Truncated {
		t.Fatalf("want 2 results + truncated, got len=%d truncated=%v", len(resp.Results), resp.Truncated)
	}
}

// TestNoBackendMessage_NoPhantomKeylessMCP — F54: the no-search-backend guidance must not advertise a
// keyless search MCP the marketplace can't deliver (it named "duckduckgo-search ... no API key needed",
// absent from the registry — only Tavily, which needs a key), which made the agent chase a dead end.
func TestNoBackendMessage_NoPhantomKeylessMCP(t *testing.T) {
	msg := noBackendMessage("anything")
	if strings.Contains(strings.ToLower(msg), "duckduckgo") {
		t.Error("guidance must not name duckduckgo-search (not installable from the registry)")
	}
	if strings.Contains(msg, "no API key needed") {
		t.Error("guidance must not claim a keyless search MCP — search MCPs need their own key")
	}
	if !strings.Contains(msg, "WebFetch") {
		t.Error("guidance should point at the genuinely-keyless WebFetch fallback")
	}
}
