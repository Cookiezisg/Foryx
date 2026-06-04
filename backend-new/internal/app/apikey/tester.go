package apikey

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	apikeydomain "github.com/sunweilin/forgify/backend/internal/domain/apikey"
)

// TestResult is the outcome of one connectivity probe. It answers ONLY "is this
// key live" + how long it took, and carries the upstream's RawResponse verbatim.
// It does NOT parse models — that's the model module's job (it reads RawResponse).
//
// TestResult 是一次连通探测的结果。只回答「这把钥匙活没活」+ 耗时，并原样携带上游 RawResponse。
// 它不解析模型——那是 model 模块的活（它读 RawResponse）。
type TestResult struct {
	OK          bool
	Message     string
	LatencyMs   int64
	RawResponse string // upstream body verbatim on success; parsed downstream, never here
}

// ConnectivityTester is Service's port for probing a credential upstream; mockable in tests.
//
// ConnectivityTester 是 Service 探测凭证的端口，测试可 mock。
type ConnectivityTester interface {
	Test(ctx context.Context, provider, key, baseURL, apiFormat string) (*TestResult, error)
}

// HTTPTester dispatches by TestMethod to knock on each provider with its auth
// style, then stores the raw answer. A dumb probe: 200 = live, body archived as-is.
//
// HTTPTester 按 TestMethod 用各家认证方式敲门，再存原始回信。哑探针：200=活，body 原样存档。
type HTTPTester struct {
	client *http.Client
}

// NewHTTPTester installs a default 10s-timeout client when given nil.
//
// NewHTTPTester 传 nil 时装默认 10s 超时 client。
func NewHTTPTester(client *http.Client) *HTTPTester {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	return &HTTPTester{client: client}
}

var _ ConnectivityTester = (*HTTPTester)(nil)

// Test dispatches by the provider's TestMethod; a misconfigured provider returns
// error, transport/auth outcomes ride in TestResult.
//
// Test 按 provider 的 TestMethod 分派；配置错误返 error，传输/认证结果走 TestResult。
func (t *HTTPTester) Test(ctx context.Context, provider, key, baseURL, apiFormat string) (*TestResult, error) {
	meta, ok := GetProviderMeta(provider)
	if !ok {
		return nil, fmt.Errorf("apikey.HTTPTester.Test: unknown provider %q: %w", provider, apikeydomain.ErrInvalidProvider)
	}
	effective := strings.TrimRight(baseURL, "/")
	if effective == "" {
		effective = strings.TrimRight(meta.DefaultBaseURL, "/")
	}
	if effective == "" && meta.TestMethod != TestMethodAlwaysOK {
		return nil, fmt.Errorf("apikey.HTTPTester.Test: baseURL required for %q: %w", provider, apikeydomain.ErrBaseURLRequired)
	}

	switch meta.TestMethod {
	case TestMethodAlwaysOK:
		return &TestResult{OK: true, Message: "mock provider — always ok"}, nil
	case TestMethodGetModels:
		return t.probeGet(ctx, effective+"/models", bearer(key)), nil
	case TestMethodAnthropicModels:
		return t.probeAnthropicModels(ctx, effective, key), nil
	case TestMethodGoogleListModels:
		return t.probeGoogleListModels(ctx, effective, key), nil
	case TestMethodOllamaTags:
		return t.probeGet(ctx, strings.TrimSuffix(effective, "/v1")+"/api/tags", nil), nil
	case TestMethodCustom:
		if apiFormat == apikeydomain.APIFormatAnthropicCompatible {
			return t.probeAnthropicModels(ctx, effective, key), nil
		}
		return t.probeGet(ctx, effective+"/models", bearer(key)), nil
	case TestMethodSearchPing:
		return t.probeSearchPing(ctx, provider, effective, key), nil
	default:
		panic(fmt.Sprintf("apikey.HTTPTester.Test: TestMethod %q for provider %q has no dispatch branch", meta.TestMethod, provider))
	}
}

func bearer(key string) http.Header {
	h := http.Header{}
	h.Set("Authorization", "Bearer "+key)
	return h
}

// probeGet issues a GET with optional headers and judges connectivity by status.
//
// probeGet 发带可选 header 的 GET，按状态码判连通。
func (t *HTTPTester) probeGet(ctx context.Context, fullURL string, headers http.Header) *TestResult {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fullURL, nil)
	if err != nil {
		return &TestResult{OK: false, Message: "build request: " + err.Error()}
	}
	for k, vs := range headers {
		for _, v := range vs {
			req.Header.Set(k, v)
		}
	}
	return t.send(req)
}

// probeAnthropicModels hits GET /v1/models with x-api-key — Anthropic's models endpoint both
// proves connectivity and returns the model catalog, archived verbatim for the model module to parse.
//
// probeAnthropicModels 打 GET /v1/models（x-api-key）——Anthropic 的 models 端点既证连通又返回
// 模型目录，原样存档供 model 模块解析。
func (t *HTTPTester) probeAnthropicModels(ctx context.Context, baseURL, key string) *TestResult {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/v1/models", nil)
	if err != nil {
		return &TestResult{OK: false, Message: "build request: " + err.Error()}
	}
	req.Header.Set("x-api-key", key)
	req.Header.Set("anthropic-version", "2023-06-01")
	return t.send(req)
}

// probeGoogleListModels hits /v1beta/models with the key in the query string.
//
// probeGoogleListModels 打 /v1beta/models，key 在 query。
func (t *HTTPTester) probeGoogleListModels(ctx context.Context, baseURL, key string) *TestResult {
	root := strings.TrimSuffix(baseURL, "/v1beta/openai")
	root = strings.TrimSuffix(root, "/v1beta")
	u := fmt.Sprintf("%s/v1beta/models?key=%s", root, url.QueryEscape(key))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return &TestResult{OK: false, Message: "build request: " + err.Error()}
	}
	return t.send(req)
}

// probeSearchPing fires a 1-result probe search per search provider's auth style.
//
// probeSearchPing 按各搜索 provider 的认证方式发 1 条结果的探测搜索。
func (t *HTTPTester) probeSearchPing(ctx context.Context, provider, baseURL, key string) *TestResult {
	const q = "test"
	var req *http.Request
	var err error
	switch provider {
	case "brave":
		req, err = http.NewRequestWithContext(ctx, http.MethodGet,
			fmt.Sprintf("%s/web/search?q=%s&count=1", baseURL, url.QueryEscape(q)), nil)
		if err == nil {
			req.Header.Set("X-Subscription-Token", key)
			req.Header.Set("Accept", "application/json")
		}
	case "serper":
		req, err = http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/search",
			bytes.NewReader(fmt.Appendf(nil, `{"q":%q,"num":1}`, q)))
		if err == nil {
			req.Header.Set("X-API-KEY", key)
			req.Header.Set("Content-Type", "application/json")
		}
	case "tavily": // key goes in the JSON body, not a header
		req, err = http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/search",
			bytes.NewReader(fmt.Appendf(nil, `{"api_key":%q,"query":%q,"max_results":1}`, key, q)))
		if err == nil {
			req.Header.Set("Content-Type", "application/json")
		}
	case "bocha":
		req, err = http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/web-search",
			bytes.NewReader(fmt.Appendf(nil, `{"query":%q,"count":1}`, q)))
		if err == nil {
			req.Header.Set("Authorization", "Bearer "+key)
			req.Header.Set("Content-Type", "application/json")
		}
	default:
		return &TestResult{OK: false, Message: fmt.Sprintf("unknown search provider %q", provider)}
	}
	if err != nil {
		return &TestResult{OK: false, Message: "build request: " + err.Error()}
	}
	return t.send(req)
}

// send executes req and folds the outcome into a TestResult — 200 is live, the
// raw body is archived verbatim, non-200 / transport errors are failures.
//
// send 执行 req 并折叠成 TestResult —— 200 为活、原始 body 原样存档，非 200 / 传输错误为失败。
func (t *HTTPTester) send(req *http.Request) *TestResult {
	start := time.Now()
	resp, err := t.client.Do(req)
	latency := time.Since(start).Milliseconds()
	if err != nil {
		return &TestResult{OK: false, Message: "connection failed: " + err.Error(), LatencyMs: latency}
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if resp.StatusCode != http.StatusOK {
		return &TestResult{OK: false, Message: formatHTTPError(resp.StatusCode, body), LatencyMs: latency}
	}
	return &TestResult{OK: true, Message: "connected", LatencyMs: latency, RawResponse: string(body)}
}

func formatHTTPError(status int, body []byte) string {
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		return fmt.Sprintf("HTTP %d", status)
	}
	return fmt.Sprintf("HTTP %d: %s", status, truncate(trimmed, 200))
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
