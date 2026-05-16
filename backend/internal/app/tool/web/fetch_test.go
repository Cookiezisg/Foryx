package web

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)


func TestWebFetch_IdentityMethods(t *testing.T) {
	tool := &WebFetch{}
	if tool.Name() != "WebFetch" {
		t.Errorf("Name = %q, want WebFetch", tool.Name())
	}
	if tool.Description() == "" {
		t.Error("Description should not be empty")
	}
	if len(tool.Parameters()) == 0 {
		t.Error("Parameters should not be empty")
	}
}

func TestWebFetch_StaticMetadata(t *testing.T) {
	tool := &WebFetch{}
	if !tool.IsReadOnly() {
		t.Error("WebFetch should be read-only")
	}
	if tool.NeedsReadFirst() {
		t.Error("WebFetch should not require Read first")
	}
	if tool.RequiresWorkspace() {
		t.Error("WebFetch should not require workspace (network tool)")
	}
}

func TestWebFetch_Schema_IsParsableObject(t *testing.T) {
	var doc map[string]any
	if err := json.Unmarshal(fetchSchema, &doc); err != nil {
		t.Fatalf("schema is not valid JSON: %v", err)
	}
	if doc["type"] != "object" {
		t.Errorf("schema type = %v, want object", doc["type"])
	}
	props, ok := doc["properties"].(map[string]any)
	if !ok {
		t.Fatalf("schema properties not an object")
	}
	for _, want := range []string{"url", "prompt"} {
		if _, ok := props[want]; !ok {
			t.Errorf("schema missing property %q", want)
		}
	}
	required, _ := doc["required"].([]any)
	if len(required) != 2 {
		t.Errorf("required len = %d, want 2 (url + prompt)", len(required))
	}
}


func TestWebFetch_ValidateInput_RequiresURL(t *testing.T) {
	tool := &WebFetch{}
	err := tool.ValidateInput(json.RawMessage(`{"prompt":"summarise"}`))
	if !errors.Is(err, ErrEmptyURL) {
		t.Fatalf("want ErrEmptyURL, got %v", err)
	}
}

func TestWebFetch_ValidateInput_RequiresPrompt(t *testing.T) {
	tool := &WebFetch{}
	err := tool.ValidateInput(json.RawMessage(`{"url":"https://example.com"}`))
	if !errors.Is(err, ErrEmptyPrompt) {
		t.Fatalf("want ErrEmptyPrompt, got %v", err)
	}
}

func TestWebFetch_ValidateInput_RejectsNonHTTPScheme(t *testing.T) {
	tool := &WebFetch{}
	cases := []string{
		`{"url":"file:///etc/passwd","prompt":"x"}`,
		`{"url":"ftp://example.com","prompt":"x"}`,
		`{"url":"gopher://example.com","prompt":"x"}`,
	}
	for _, c := range cases {
		if err := tool.ValidateInput(json.RawMessage(c)); !errors.Is(err, ErrUnsupportedScheme) {
			t.Errorf("for %s: want ErrUnsupportedScheme, got %v", c, err)
		}
	}
}

func TestWebFetch_ValidateInput_RejectsWhitespaceOnly(t *testing.T) {
	tool := &WebFetch{}
	if err := tool.ValidateInput(json.RawMessage(`{"url":"   ","prompt":"x"}`)); !errors.Is(err, ErrEmptyURL) {
		t.Errorf("whitespace url should be empty, got %v", err)
	}
	if err := tool.ValidateInput(json.RawMessage(`{"url":"https://x.com","prompt":"  "}`)); !errors.Is(err, ErrEmptyPrompt) {
		t.Errorf("whitespace prompt should be empty, got %v", err)
	}
}

func TestWebFetch_ValidateInput_AcceptsValidArgs(t *testing.T) {
	tool := &WebFetch{}
	if err := tool.ValidateInput(json.RawMessage(`{"url":"https://example.com/x","prompt":"summarise"}`)); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
}


func TestClassifyIP_AllowsPublic(t *testing.T) {
	publics := []string{"8.8.8.8", "1.1.1.1", "104.16.0.1", "2001:4860:4860::8888"}
	for _, addr := range publics {
		ip := net.ParseIP(addr)
		if ip == nil {
			t.Fatalf("bad fixture: %s", addr)
		}
		if got := classifyIP(ip); got != "" {
			t.Errorf("classifyIP(%s) = %q, want empty (public)", addr, got)
		}
	}
}

func TestClassifyIP_RejectsLoopback(t *testing.T) {
	for _, addr := range []string{"127.0.0.1", "127.0.0.5", "::1"} {
		ip := net.ParseIP(addr)
		if got := classifyIP(ip); got == "" {
			t.Errorf("classifyIP(%s) returned empty; should be rejected", addr)
		} else if !strings.Contains(got, "loopback") {
			t.Errorf("classifyIP(%s) = %q, want loopback rejection", addr, got)
		}
	}
}

func TestClassifyIP_RejectsPrivate(t *testing.T) {
	for _, addr := range []string{"10.0.0.1", "172.16.0.1", "192.168.1.1", "fc00::1"} {
		ip := net.ParseIP(addr)
		if got := classifyIP(ip); got == "" {
			t.Errorf("classifyIP(%s) returned empty; should be rejected", addr)
		} else if !strings.Contains(got, "private") {
			t.Errorf("classifyIP(%s) = %q, want private rejection", addr, got)
		}
	}
}

func TestClassifyIP_RejectsLinkLocal(t *testing.T) {
	for _, addr := range []string{"169.254.169.254", "fe80::1"} {
		ip := net.ParseIP(addr)
		if got := classifyIP(ip); got == "" {
			t.Errorf("classifyIP(%s) returned empty; should be rejected", addr)
		} else if !strings.Contains(got, "link-local") {
			t.Errorf("classifyIP(%s) = %q, want link-local rejection", addr, got)
		}
	}
}

func TestClassifyIP_RejectsUnspecifiedAndMulticast(t *testing.T) {
	if got := classifyIP(net.ParseIP("0.0.0.0")); !strings.Contains(got, "unspecified") {
		t.Errorf("0.0.0.0 should be unspecified, got %q", got)
	}
	// 239.0.0.1 is administratively-scoped multicast (not link-local), so
	// it falls through to the generic IsMulticast() branch.
	// 239.0.0.1 是管理域 multicast（非 link-local），落入通用 IsMulticast 分支。
	if got := classifyIP(net.ParseIP("239.0.0.1")); !strings.Contains(got, "multicast") {
		t.Errorf("239.0.0.1 should be multicast, got %q", got)
	}
}

func TestGuardHostname_RejectsLocalhostNames(t *testing.T) {
	for _, h := range []string{"localhost", "LOCALHOST", "ip6-localhost", "ip6-loopback"} {
		if got := guardHostname(h); got == "" {
			t.Errorf("guardHostname(%q) returned empty; should be rejected", h)
		}
	}
}

func TestGuardHostname_RejectsBareLoopbackIP(t *testing.T) {
	if got := guardHostname("127.0.0.1"); got == "" {
		t.Error("guardHostname(127.0.0.1) should reject")
	}
}

func TestGuardHostname_AllowsPublicLiteralIP(t *testing.T) {
	if got := guardHostname("8.8.8.8"); got != "" {
		t.Errorf("guardHostname(8.8.8.8) = %q, want empty", got)
	}
}

func TestGuardHostname_EmptyHost(t *testing.T) {
	if got := guardHostname(""); got == "" {
		t.Error("guardHostname(\"\") should reject")
	}
}


// withJinaServer points jinaEndpoint at the given test server for the
// duration of the test, restoring the original value after. The returned
// counter records hits to the Jina path so callers can verify routing.
//
// withJinaServer 把 jinaEndpoint 临时指向 test server，结束后恢复；
// 返回的计数器记录 Jina 路径命中次数。
func withJinaServer(t *testing.T, srv *httptest.Server) {
	t.Helper()
	prev := jinaEndpoint
	jinaEndpoint = srv.URL + "/"
	t.Cleanup(func() { jinaEndpoint = prev })
}

func TestFetchContent_PrefersJina(t *testing.T) {
	var jinaHits, directHits int32

	jina := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&jinaHits, 1)
		w.Header().Set("Content-Type", "text/markdown")
		_, _ = w.Write([]byte("# Clean markdown via Jina\n"))
	}))
	defer jina.Close()
	withJinaServer(t, jina)

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&directHits, 1)
		_, _ = w.Write([]byte("<html>raw</html>"))
	}))
	defer target.Close()

	body, err := fetchContent(context.Background(), target.URL)
	if err != nil {
		t.Fatalf("fetchContent: %v", err)
	}
	if !strings.Contains(body, "Clean markdown via Jina") {
		t.Errorf("body should come from Jina, got: %q", body)
	}
	if atomic.LoadInt32(&jinaHits) != 1 {
		t.Errorf("jina hit count = %d, want 1", jinaHits)
	}
	if atomic.LoadInt32(&directHits) != 0 {
		t.Errorf("direct should not be hit when Jina succeeds; hits = %d", directHits)
	}
}

func TestFetchContent_FallsBackToDirectWhenJinaFails(t *testing.T) {
	var directHits int32

	jina := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "rate limited", http.StatusTooManyRequests)
	}))
	defer jina.Close()
	withJinaServer(t, jina)

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&directHits, 1)
		_, _ = w.Write([]byte("direct response body"))
	}))
	defer target.Close()

	body, err := fetchContent(context.Background(), target.URL)
	if err != nil {
		t.Fatalf("fetchContent: %v", err)
	}
	if !strings.Contains(body, "direct response body") {
		t.Errorf("body should come from direct fetch, got: %q", body)
	}
	if atomic.LoadInt32(&directHits) != 1 {
		t.Errorf("direct hit count = %d, want 1", directHits)
	}
}

func TestFetchContent_BothBackendsFail(t *testing.T) {
	jina := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "down", http.StatusInternalServerError)
	}))
	defer jina.Close()
	withJinaServer(t, jina)

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "down", http.StatusBadGateway)
	}))
	defer target.Close()

	if _, err := fetchContent(context.Background(), target.URL); err == nil {
		t.Fatal("expected error when both backends 5xx")
	}
}

func TestFetchContent_HonoursContextCancellation(t *testing.T) {
	jina := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Should not be reached because ctx is cancelled before the call.
		// 不应被命中——ctx 在调用前已取消。
		t.Error("Jina handler should not be invoked on cancelled ctx")
	}))
	defer jina.Close()
	withJinaServer(t, jina)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := fetchContent(ctx, jina.URL); err == nil {
		t.Error("expected error from cancelled ctx")
	}
}

func TestFetchContent_CapsBytes(t *testing.T) {
	huge := strings.Repeat("X", maxFetchBytes*2)
	jina := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(huge))
	}))
	defer jina.Close()
	withJinaServer(t, jina)

	body, err := fetchContent(context.Background(), jina.URL)
	if err != nil {
		t.Fatalf("fetchContent: %v", err)
	}
	if len(body) > maxFetchBytes {
		t.Errorf("body len = %d, want ≤ %d", len(body), maxFetchBytes)
	}
}


func TestExecute_RejectsLoopbackBeforeFetch(t *testing.T) {
	// Even with an LLM present, SSRF check must short-circuit before any
	// network call. We don't construct a real LLM stack here — if the
	// guard works, summarise() is never reached.
	//
	// 即使 LLM 存在，SSRF 检查必须在任何网络调用前 short-circuit；只要
	// 守卫正常，summarise() 永远不会被走到。
	tool := &WebFetch{} // nil deps are fine — guard runs first
	args := json.RawMessage(`{"url":"http://127.0.0.1/secret","prompt":"snoop"}`)
	out, err := tool.Execute(context.Background(), string(args))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "loopback") {
		t.Errorf("expected loopback rejection, got: %q", out)
	}
}

func TestExecute_RejectsPrivateRangeBeforeFetch(t *testing.T) {
	tool := &WebFetch{}
	args := json.RawMessage(`{"url":"http://10.0.0.1/internal","prompt":"snoop"}`)
	out, err := tool.Execute(context.Background(), string(args))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "private") {
		t.Errorf("expected private rejection, got: %q", out)
	}
}

func TestExecute_ReportsBadURL(t *testing.T) {
	tool := &WebFetch{}
	// %ZZ is invalid percent-encoding — url.Parse rejects.
	// %ZZ 是非法 percent-encoding——url.Parse 拒绝。
	args := json.RawMessage(`{"url":"http://example.com/%ZZ","prompt":"x"}`)
	out, err := tool.Execute(context.Background(), string(args))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "Invalid URL") {
		t.Errorf("expected Invalid URL message, got: %q", out)
	}
}


// Regression: pre-fix, fetchClient followed redirects without re-running
// guardHostname, so a public URL that 302'd to http://127.0.0.1 would fetch
// the loopback. We now reject every redirect whose target classifies as
// loopback / private / link-local etc.
//
// 回归：修复前 fetchClient 跟随重定向不会重跑 guardHostname，公网 URL 302
// 到 http://127.0.0.1 会去抓 loopback。修复后每次跳转都校验。
func TestFetchClient_CheckRedirect_BlocksLoopback(t *testing.T) {
	// Public-ish bait server that 302s to a loopback URL.
	// 看起来"公网"的诱饵 server，302 到 loopback URL。
	bait := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", "http://127.0.0.1:1/admin")
		w.WriteHeader(http.StatusFound)
	}))
	defer bait.Close()

	req, err := http.NewRequest(http.MethodGet, bait.URL, nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	resp, err := fetchClient.Do(req)
	if resp != nil {
		_ = resp.Body.Close()
	}
	if err == nil {
		t.Fatal("expected redirect rejection, got nil err")
	}
	if !strings.Contains(err.Error(), "redirect blocked") {
		t.Errorf("expected 'redirect blocked' in err, got: %v", err)
	}
	if !strings.Contains(err.Error(), "loopback") {
		t.Errorf("expected 'loopback' classification in err, got: %v", err)
	}
}

func TestFetchClient_CheckRedirect_BlocksPrivateRange(t *testing.T) {
	bait := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", "http://10.0.0.1/")
		w.WriteHeader(http.StatusFound)
	}))
	defer bait.Close()

	req, _ := http.NewRequest(http.MethodGet, bait.URL, nil)
	resp, err := fetchClient.Do(req)
	if resp != nil {
		_ = resp.Body.Close()
	}
	if err == nil || !strings.Contains(err.Error(), "private") {
		t.Errorf("want private-range rejection, got: %v", err)
	}
}

func TestFetchClient_CheckRedirect_AllowsPublicTarget(t *testing.T) {
	// A redirect to another public httptest server (also 127.0.0.1 in tests…
	// but httptest URLs are 127.0.0.1, so we can't test "real public" without
	// a real network. Instead verify that our guard short-circuits before
	// allowing the redirect — the bait response itself should still come back.
	//
	// 跳转到另一个公网（不打真网络情况下 httptest 也是 127.0.0.1，难以测“真公网”）。
	// 这里换个角度：验证不带 Location 头的 200 直接通过，证明守卫不会
	// 错误拦下非重定向流量。
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("hello"))
	}))
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodGet, srv.URL, nil)
	resp, err := fetchClient.Do(req)
	if err != nil {
		t.Fatalf("non-redirect request errored: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
}

func TestSSRFCheckRedirect_RejectsAfterTenHops(t *testing.T) {
	// Build 10 fake "via" requests pointing at 8.8.8.8 (allowed); the 11th
	// hop is to a public-looking host but the count cap should fire first.
	// 构造 10 个 via 请求，第 11 跳即便目标合法也应被 cap 拦下。
	via := make([]*http.Request, 10)
	for i := range via {
		via[i], _ = http.NewRequest(http.MethodGet, "https://8.8.8.8/", nil)
	}
	next, _ := http.NewRequest(http.MethodGet, "https://8.8.8.8/", nil)
	if err := ssrfCheckRedirect(next, via); err == nil || !strings.Contains(err.Error(), "10 redirects") {
		t.Errorf("expected 10-redirect cap, got: %v", err)
	}
}


func TestTruncate(t *testing.T) {
	if got := truncate("short", 100); got != "short" {
		t.Errorf("under-cap should pass through, got %q", got)
	}
	long := strings.Repeat("X", 200)
	got := truncate(long, 50)
	if !strings.HasPrefix(got, strings.Repeat("X", 50)) {
		t.Errorf("truncate did not retain prefix: %q", got)
	}
	if !strings.Contains(got, "[truncated]") {
		t.Errorf("truncate missing indicator: %q", got)
	}
}

func TestBuildSummaryPrompt_IncludesAllPieces(t *testing.T) {
	body := buildSummaryPrompt("https://example.com/x", "List endpoints", "GET /a\nPOST /b")
	for _, want := range []string{"https://example.com/x", "List endpoints", "GET /a", "POST /b", "<<<CONTENT_BEGIN>>>", "<<<CONTENT_END>>>"} {
		if !strings.Contains(body, want) {
			t.Errorf("summary prompt missing %q", want)
		}
	}
}
