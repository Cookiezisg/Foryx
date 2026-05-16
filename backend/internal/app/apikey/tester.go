package apikey

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	apikeydomain "github.com/sunweilin/forgify/backend/internal/domain/apikey"
)

// TestResult is the outcome of one connectivity probe; Message is UI-safe.
//
// TestResult 是一次连通性探测的结果；Message 可直接展示。
type TestResult struct {
	OK          bool
	Message     string
	LatencyMs   int64
	ModelsFound []string
}

// ConnectivityTester is Service's port for verifying credentials upstream.
//
// ConnectivityTester 是 Service 验证凭证的端口，可在测试中 mock。
type ConnectivityTester interface {
	Test(ctx context.Context, provider, key, baseURL, apiFormat string) (*TestResult, error)
}

// HTTPTester dispatches by TestMethod to probe upstream with its auth style.
//
// HTTPTester 按 TestMethod 分派，用 provider 期望的认证方式调上游。
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

// Test dispatches by TestMethod; programming bugs return error, probes via TestResult.
//
// Test 按 TestMethod 分派；配置 bug 返 error，传输/认证结果走 TestResult。
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
		return nil, fmt.Errorf("apikey.HTTPTester.Test: baseURL required for provider %q: %w", provider, apikeydomain.ErrBaseURLRequired)
	}

	switch meta.TestMethod {
	case TestMethodAlwaysOK:
		return &TestResult{OK: true, Message: "mock provider — always ok", ModelsFound: []string{"mock-model"}}, nil
	case TestMethodGetModels:
		return t.testGetModels(ctx, effective, key), nil
	case TestMethodAnthropicPing:
		return t.testAnthropicPing(ctx, effective, key), nil
	case TestMethodGoogleListModels:
		return t.testGoogleListModels(ctx, effective, key), nil
	case TestMethodOllamaTags:
		return t.testOllamaTags(ctx, effective), nil
	case TestMethodCustom:
		if apiFormat == apikeydomain.APIFormatAnthropicCompatible {
			return t.testAnthropicPing(ctx, effective, key), nil
		}
		return t.testGetModels(ctx, effective, key), nil
	case TestMethodSearchPing:
		return t.testSearchPing(ctx, provider, effective, key), nil
	default:
		panic(fmt.Sprintf("apikey.HTTPTester.Test: TestMethod %q registered for provider %q has no dispatcher branch; add it to the switch above", meta.TestMethod, provider))
	}
}

func (t *HTTPTester) testSearchPing(ctx context.Context, provider, baseURL, key string) *TestResult {
	const probeQuery = "test"
	switch provider {
	case "brave":
		return t.testBravePing(ctx, baseURL, key, probeQuery)
	case "serper":
		return t.testSerperPing(ctx, baseURL, key, probeQuery)
	case "tavily":
		return t.testTavilyPing(ctx, baseURL, key, probeQuery)
	case "bocha":
		return t.testBochaPing(ctx, baseURL, key, probeQuery)
	default:
		return &TestResult{OK: false, Message: fmt.Sprintf("unknown search provider %q", provider)}
	}
}

func (t *HTTPTester) testBravePing(ctx context.Context, baseURL, key, query string) *TestResult {
	u := fmt.Sprintf("%s/web/search?q=%s&count=1", baseURL, url.QueryEscape(query))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return &TestResult{OK: false, Message: "build request: " + err.Error()}
	}
	req.Header.Set("X-Subscription-Token", key)
	req.Header.Set("Accept", "application/json")
	body, status, latency, err := t.do(req)
	if err != nil {
		return &TestResult{OK: false, Message: "connection failed: " + err.Error(), LatencyMs: latency}
	}
	if status != http.StatusOK {
		return &TestResult{OK: false, Message: formatHTTPError(status, body), LatencyMs: latency}
	}
	return &TestResult{OK: true, Message: "connected (1-result probe)", LatencyMs: latency}
}

func (t *HTTPTester) testSerperPing(ctx context.Context, baseURL, key, query string) *TestResult {
	payload := []byte(fmt.Sprintf(`{"q":%q,"num":1}`, query))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/search", bytes.NewReader(payload))
	if err != nil {
		return &TestResult{OK: false, Message: "build request: " + err.Error()}
	}
	req.Header.Set("X-API-KEY", key)
	req.Header.Set("Content-Type", "application/json")
	body, status, latency, err := t.do(req)
	if err != nil {
		return &TestResult{OK: false, Message: "connection failed: " + err.Error(), LatencyMs: latency}
	}
	if status != http.StatusOK {
		return &TestResult{OK: false, Message: formatHTTPError(status, body), LatencyMs: latency}
	}
	return &TestResult{OK: true, Message: "connected (1-result probe)", LatencyMs: latency}
}

// testTavilyPing passes the key inside the JSON body, not a header.
//
// testTavilyPing 把 key 放 JSON body 而不是 header。
func (t *HTTPTester) testTavilyPing(ctx context.Context, baseURL, key, query string) *TestResult {
	payload := []byte(fmt.Sprintf(`{"api_key":%q,"query":%q,"max_results":1}`, key, query))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/search", bytes.NewReader(payload))
	if err != nil {
		return &TestResult{OK: false, Message: "build request: " + err.Error()}
	}
	req.Header.Set("Content-Type", "application/json")
	body, status, latency, err := t.do(req)
	if err != nil {
		return &TestResult{OK: false, Message: "connection failed: " + err.Error(), LatencyMs: latency}
	}
	if status != http.StatusOK {
		return &TestResult{OK: false, Message: formatHTTPError(status, body), LatencyMs: latency}
	}
	return &TestResult{OK: true, Message: "connected (1-result probe)", LatencyMs: latency}
}

func (t *HTTPTester) testBochaPing(ctx context.Context, baseURL, key, query string) *TestResult {
	payload := []byte(fmt.Sprintf(`{"query":%q,"count":1}`, query))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/web-search", bytes.NewReader(payload))
	if err != nil {
		return &TestResult{OK: false, Message: "build request: " + err.Error()}
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")
	body, status, latency, err := t.do(req)
	if err != nil {
		return &TestResult{OK: false, Message: "connection failed: " + err.Error(), LatencyMs: latency}
	}
	if status != http.StatusOK {
		return &TestResult{OK: false, Message: formatHTTPError(status, body), LatencyMs: latency}
	}
	return &TestResult{OK: true, Message: "connected (1-result probe)", LatencyMs: latency}
}

func (t *HTTPTester) testGetModels(ctx context.Context, baseURL, key string) *TestResult {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/models", nil)
	if err != nil {
		return &TestResult{OK: false, Message: "build request: " + err.Error()}
	}
	req.Header.Set("Authorization", "Bearer "+key)
	body, status, latency, err := t.do(req)
	if err != nil {
		return &TestResult{OK: false, Message: "connection failed: " + err.Error(), LatencyMs: latency}
	}
	if status != http.StatusOK {
		return &TestResult{OK: false, Message: formatHTTPError(status, body), LatencyMs: latency}
	}
	models := parseOpenAIModels(body)
	return &TestResult{
		OK:          true,
		Message:     fmt.Sprintf("connected, %d models available", len(models)),
		LatencyMs:   latency,
		ModelsFound: models,
	}
}

// testAnthropicPing uses a 1-token /v1/messages call since /models is absent.
//
// testAnthropicPing 用 1-token /v1/messages 探测——Anthropic 无 /models 端点。
func (t *HTTPTester) testAnthropicPing(ctx context.Context, baseURL, key string) *TestResult {
	payload := []byte(`{"model":"claude-3-5-haiku-latest","max_tokens":1,"messages":[{"role":"user","content":"hi"}]}`)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/v1/messages", bytes.NewReader(payload))
	if err != nil {
		return &TestResult{OK: false, Message: "build request: " + err.Error()}
	}
	req.Header.Set("x-api-key", key)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("content-type", "application/json")
	body, status, latency, err := t.do(req)
	if err != nil {
		return &TestResult{OK: false, Message: "connection failed: " + err.Error(), LatencyMs: latency}
	}
	if status != http.StatusOK {
		return &TestResult{OK: false, Message: formatHTTPError(status, body), LatencyMs: latency}
	}
	return &TestResult{OK: true, Message: "connected (1-token ping)", LatencyMs: latency}
}

func (t *HTTPTester) testGoogleListModels(ctx context.Context, baseURL, key string) *TestResult {
	u := fmt.Sprintf("%s/v1beta/models?key=%s", baseURL, url.QueryEscape(key))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return &TestResult{OK: false, Message: "build request: " + err.Error()}
	}
	body, status, latency, err := t.do(req)
	if err != nil {
		return &TestResult{OK: false, Message: "connection failed: " + err.Error(), LatencyMs: latency}
	}
	if status != http.StatusOK {
		return &TestResult{OK: false, Message: formatHTTPError(status, body), LatencyMs: latency}
	}
	models := parseModelsByName(body)
	return &TestResult{
		OK:          true,
		Message:     fmt.Sprintf("connected, %d models available", len(models)),
		LatencyMs:   latency,
		ModelsFound: models,
	}
}

func (t *HTTPTester) testOllamaTags(ctx context.Context, baseURL string) *TestResult {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/api/tags", nil)
	if err != nil {
		return &TestResult{OK: false, Message: "build request: " + err.Error()}
	}
	body, status, latency, err := t.do(req)
	if err != nil {
		return &TestResult{OK: false, Message: "connection failed: " + err.Error(), LatencyMs: latency}
	}
	if status != http.StatusOK {
		return &TestResult{OK: false, Message: formatHTTPError(status, body), LatencyMs: latency}
	}
	models := parseModelsByName(body)
	return &TestResult{
		OK:          true,
		Message:     fmt.Sprintf("connected, %d local models installed", len(models)),
		LatencyMs:   latency,
		ModelsFound: models,
	}
}

// do sends req and returns (body, status, latencyMs, err); body capped at 64 KiB.
//
// do 发送请求并返回 (body, status, latencyMs, err)；body 上限 64 KiB。
func (t *HTTPTester) do(req *http.Request) ([]byte, int, int64, error) {
	start := time.Now()
	resp, err := t.client.Do(req)
	latency := time.Since(start).Milliseconds()
	if err != nil {
		return nil, 0, latency, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		return nil, resp.StatusCode, latency, err
	}
	return body, resp.StatusCode, latency, nil
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

func parseOpenAIModels(body []byte) []string {
	var resp struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil
	}
	out := make([]string, 0, len(resp.Data))
	for _, m := range resp.Data {
		if m.ID != "" {
			out = append(out, m.ID)
		}
	}
	return out
}

// parseModelsByName extracts names from {"models":[{"name":"..."}]} (Google + Ollama).
//
// parseModelsByName 从 {"models":[{"name":"..."}]} 提取名字（Google / Ollama 共用）。
func parseModelsByName(body []byte) []string {
	var resp struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil
	}
	out := make([]string, 0, len(resp.Models))
	for _, m := range resp.Models {
		if m.Name != "" {
			out = append(out, m.Name)
		}
	}
	return out
}
