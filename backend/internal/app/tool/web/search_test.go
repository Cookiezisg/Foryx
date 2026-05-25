package web

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	apikeydomain "github.com/sunweilin/forgify/backend/internal/domain/apikey"
)


func TestWebSearch_IdentityMethods(t *testing.T) {
	tool := newTestSearch(t)
	if tool.Name() != "WebSearch" {
		t.Errorf("Name = %q, want WebSearch", tool.Name())
	}
	if tool.Description() == "" {
		t.Error("Description should not be empty")
	}
	if len(tool.Parameters()) == 0 {
		t.Error("Parameters should not be empty")
	}
}

func TestWebSearch_StaticMetadata(t *testing.T) {
	tool := newTestSearch(t)
	if !tool.IsReadOnly() {
		t.Error("WebSearch should be read-only")
	}
	if tool.NeedsReadFirst() {
		t.Error("WebSearch should not require Read first")
	}
	if tool.RequiresWorkspace() {
		t.Error("WebSearch should not require workspace (network tool)")
	}
}

func TestWebSearch_Schema_IsParsableObject(t *testing.T) {
	var doc map[string]any
	if err := json.Unmarshal(searchSchema, &doc); err != nil {
		t.Fatalf("schema is not valid JSON: %v", err)
	}
	if doc["type"] != "object" {
		t.Errorf("schema type = %v", doc["type"])
	}
	props := doc["properties"].(map[string]any)
	for _, want := range []string{"query", "limit"} {
		if _, ok := props[want]; !ok {
			t.Errorf("schema missing property %q", want)
		}
	}
}


func TestWebSearch_ValidateInput_RequiresQuery(t *testing.T) {
	tool := newTestSearch(t)
	if err := tool.ValidateInput(json.RawMessage(`{}`)); !errors.Is(err, ErrEmptyQuery) {
		t.Fatalf("want ErrEmptyQuery, got %v", err)
	}
	if err := tool.ValidateInput(json.RawMessage(`{"query":"   "}`)); !errors.Is(err, ErrEmptyQuery) {
		t.Fatalf("whitespace query should fail, got %v", err)
	}
}

func TestWebSearch_ValidateInput_RejectsNegativeLimit(t *testing.T) {
	tool := newTestSearch(t)
	if err := tool.ValidateInput(json.RawMessage(`{"query":"x","limit":-1}`)); err == nil {
		t.Fatal("expected error for negative limit")
	}
}


func TestSearchArgs_NormalizeFillsDefaults(t *testing.T) {
	a := searchArgs{}
	a.normalize()
	if a.Limit != defaultSearchLimit {
		t.Errorf("Limit default = %d, want %d", a.Limit, defaultSearchLimit)
	}
}

func TestSearchArgs_NormalizeCapsHardLimit(t *testing.T) {
	a := searchArgs{Limit: 10_000}
	a.normalize()
	if a.Limit != maxSearchLimit {
		t.Errorf("Limit hard cap = %d, want %d", a.Limit, maxSearchLimit)
	}
}


func TestExecute_BYOKBrave_ReturnsResults(t *testing.T) {
	srv := newBraveServer(t, []searchResult{
		{Title: "Go", URL: "https://go.dev", Snippet: "Build simple, secure systems."},
	})
	defer srv.Close()

	tool := newTestSearchWith(t, &fakeKeys{
		creds: map[string]apikeydomain.Credentials{"brave": {Key: "test-key", BaseURL: srv.URL}},
	}, nil)
	out := runSearch(t, tool, `{"query":"golang"}`)

	if out.Source != "brave" {
		t.Errorf("source = %q, want brave", out.Source)
	}
	if len(out.Results) != 1 || out.Results[0].URL != "https://go.dev" {
		t.Errorf("unexpected results: %+v", out.Results)
	}
}

func TestExecute_BYOKBrave_FallsThroughOnError(t *testing.T) {
	dead := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "down", http.StatusInternalServerError)
	}))
	defer dead.Close()

	mcpResults := `{"results":[{"title":"hit","url":"https://example.com","snippet":"via MCP"}]}`
	tool := newTestSearchWith(t, &fakeKeys{
		creds: map[string]apikeydomain.Credentials{"brave": {Key: "k", BaseURL: dead.URL}},
	}, &fakeMCPRouter{result: mcpResults})

	out := runSearch(t, tool, `{"query":"go"}`)
	if out.Source != "mcp" {
		t.Errorf("source = %q, want mcp (Brave failed → fall through)", out.Source)
	}
}

func TestExecute_PrioritisesBraveOverSerper(t *testing.T) {
	brave := newBraveServer(t, []searchResult{{Title: "from-brave", URL: "https://x", Snippet: "b"}})
	defer brave.Close()
	serper := newSerperServer(t, []searchResult{{Title: "from-serper", URL: "https://y", Snippet: "s"}})
	defer serper.Close()

	tool := newTestSearchWith(t, &fakeKeys{
		creds: map[string]apikeydomain.Credentials{
			"brave":  {Key: "bk", BaseURL: brave.URL},
			"serper": {Key: "sk", BaseURL: serper.URL},
		},
	}, nil)
	out := runSearch(t, tool, `{"query":"q"}`)
	if out.Source != "brave" {
		t.Errorf("source = %q, want brave (priority list head)", out.Source)
	}
	if out.Results[0].Title != "from-brave" {
		t.Errorf("results from wrong backend: %+v", out.Results)
	}
}

func TestExecute_MCPRoute_FiresWhenNoBYOK(t *testing.T) {
	mcpResults := `{"results":[{"title":"DDG hit","url":"https://example.com","snippet":"via MCP"}]}`
	tool := newTestSearchWith(t, &fakeKeys{}, &fakeMCPRouter{result: mcpResults})

	out := runSearch(t, tool, `{"query":"x"}`)
	if out.Source != "mcp" {
		t.Errorf("source = %q, want mcp", out.Source)
	}
	if len(out.Results) != 1 || out.Results[0].Title != "DDG hit" {
		t.Errorf("unexpected MCP results: %+v", out.Results)
	}
}

func TestExecute_MCP_UnavailableFallsThroughToFriendlyMessage(t *testing.T) {
	tool := newTestSearchWith(t, &fakeKeys{}, &fakeMCPRouter{err: ErrMCPSearchUnavailable})

	body, err := tool.Execute(context.Background(), `{"query":"x"}`)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(body, "No results") || !strings.Contains(body, "API key") || !strings.Contains(body, "duckduckgo-search") {
		t.Errorf("expected friendly message guiding key + MCP install, got: %q", body)
	}
}

func TestExecute_NoBackendsConfigured_FriendlyMessage(t *testing.T) {
	tool := newTestSearchWith(t, &fakeKeys{}, nil)
	body, err := tool.Execute(context.Background(), `{"query":"unrecoverable"}`)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(body, "No results") || !strings.Contains(body, "API key") {
		t.Errorf("expected friendly message guiding key setup, got: %q", body)
	}
}

func TestExecute_AppliesLimitAndSetsTruncated(t *testing.T) {
	many := make([]searchResult, 0, 8)
	for i := 0; i < 8; i++ {
		many = append(many, searchResult{Title: "t", URL: "https://example.com/", Snippet: "s"})
	}
	srv := newBraveServer(t, many)
	defer srv.Close()

	tool := newTestSearchWith(t, &fakeKeys{
		creds: map[string]apikeydomain.Credentials{"brave": {Key: "k", BaseURL: srv.URL}},
	}, nil)
	out := runSearch(t, tool, `{"query":"x","limit":3}`)
	if !out.Truncated {
		t.Error("truncated should be true (8 > 3)")
	}
	if len(out.Results) != 3 {
		t.Errorf("results len = %d, want 3", len(out.Results))
	}
}

func TestExecute_HonoursContextCancellation(t *testing.T) {
	tool := newTestSearchWith(t, &fakeKeys{}, nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	body, err := tool.Execute(ctx, `{"query":"x"}`)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	// On cancelled ctx Execute either short-circuits (no-results path) or
	// reports the failure; both are acceptable as long as it returned.
	// 取消的 ctx 下要么走 no-results 短路要么报失败；只要 return 即可。
	if body == "" {
		t.Error("expected some response body even on cancellation")
	}
}


func TestParseMCPSearchResults_KeyedShape(t *testing.T) {
	raw := `{"results":[{"title":"A","url":"https://a","snippet":"sa"},{"name":"B","link":"https://b","content":"sb"}]}`
	got, err := parseMCPSearchResults(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if got[0].Title != "A" || got[0].URL != "https://a" || got[0].Snippet != "sa" {
		t.Errorf("[0] = %+v", got[0])
	}
	// Field-name union: name → Title, link → URL, content → Snippet.
	// 字段名取并集：name → Title、link → URL、content → Snippet。
	if got[1].Title != "B" || got[1].URL != "https://b" || got[1].Snippet != "sb" {
		t.Errorf("[1] = %+v", got[1])
	}
}

func TestParseMCPSearchResults_BareArray(t *testing.T) {
	raw := `[{"title":"X","url":"https://x","snippet":"sx"}]`
	got, err := parseMCPSearchResults(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(got) != 1 || got[0].Title != "X" {
		t.Errorf("got %+v", got)
	}
}

func TestParseMCPSearchResults_PlainTextFallback(t *testing.T) {
	got, err := parseMCPSearchResults("just some plain text from MCP server")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(got) != 1 || !strings.Contains(got[0].Snippet, "plain text") {
		t.Errorf("plain-text fallback failed: %+v", got)
	}
}


func newTestSearch(t *testing.T) *WebSearch {
	t.Helper()
	return newTestSearchWith(t, &fakeKeys{}, nil)
}

func newTestSearchWith(t *testing.T, keys apikeydomain.KeyProvider, mcp MCPSearchRouter) *WebSearch {
	t.Helper()
	return &WebSearch{
		httpClient: &http.Client{Timeout: 2 * time.Second},
		keys:       keys,
		mcpRouter:  mcp,
		log:        nil,
	}
}

// fakeKeys implements apikeydomain.KeyProvider with a static map.
//
// fakeKeys 用静态 map 实现 apikeydomain.KeyProvider。
type fakeKeys struct {
	creds map[string]apikeydomain.Credentials
}

func (f *fakeKeys) ResolveCredentials(_ context.Context, provider string) (apikeydomain.Credentials, error) {
	c, ok := f.creds[provider]
	if !ok {
		return apikeydomain.Credentials{}, apikeydomain.ErrNotFoundForProvider
	}
	return c, nil
}

func (f *fakeKeys) MarkInvalid(_ context.Context, _ string, _ string) error { return nil }
func (f *fakeKeys) HasKeyForProvider(_ context.Context, provider string) (bool, error) {
	_, ok := f.creds[provider]
	return ok, nil
}
func (f *fakeKeys) DefaultSearchProvider(_ context.Context) string { return "" }

// fakeMCPRouter implements MCPSearchRouter.
//
// fakeMCPRouter 实现 MCPSearchRouter。
type fakeMCPRouter struct {
	result string
	err    error
}

func (f *fakeMCPRouter) CallSearchTool(_ context.Context, _ string, _ int) (string, error) {
	return f.result, f.err
}

// newBraveServer returns a Brave-shaped JSON server.
//
// newBraveServer 返 Brave-shape JSON 服务器。
func newBraveServer(t *testing.T, results []searchResult) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		shaped := struct {
			Web struct {
				Results []struct {
					Title       string `json:"title"`
					URL         string `json:"url"`
					Description string `json:"description"`
				} `json:"results"`
			} `json:"web"`
		}{}
		for _, r := range results {
			shaped.Web.Results = append(shaped.Web.Results, struct {
				Title       string `json:"title"`
				URL         string `json:"url"`
				Description string `json:"description"`
			}{Title: r.Title, URL: r.URL, Description: r.Snippet})
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(shaped)
	}))
}

// newSerperServer returns a Serper-shaped JSON server.
//
// newSerperServer 返 Serper-shape JSON 服务器。
func newSerperServer(t *testing.T, results []searchResult) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		shaped := struct {
			Organic []struct {
				Title   string `json:"title"`
				Link    string `json:"link"`
				Snippet string `json:"snippet"`
			} `json:"organic"`
		}{}
		for _, r := range results {
			shaped.Organic = append(shaped.Organic, struct {
				Title   string `json:"title"`
				Link    string `json:"link"`
				Snippet string `json:"snippet"`
			}{Title: r.Title, Link: r.URL, Snippet: r.Snippet})
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(shaped)
	}))
}

func runSearch(t *testing.T, tool *WebSearch, args string) searchResponse {
	t.Helper()
	body, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	var out searchResponse
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatalf("decode response (raw=%q): %v", body, err)
	}
	return out
}
