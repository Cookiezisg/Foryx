// tester.go — ConnectivityTester port and HTTPTester implementation.
// Error-vs-TestResult convention: outcomes (401/5xx/net-fail/ctx-cancel)
// surface in TestResult; `error` is reserved for programmer bugs
// (unknown provider, required baseURL missing).
//
// tester.go — ConnectivityTester 端口和 HTTPTester 实现。
// 错误约定：测试结果（401/5xx/网络故障/ctx 取消）通过 TestResult 返回；
// `error` 只用于程序 bug（未知 provider、必填 baseURL 缺失）。

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

// TestResult is the outcome of one connectivity probe. Message is safe to
// render in the UI; ModelsFound is populated only when the provider
// returns a model list.
//
// TestResult 是一次连通性探测的结果。Message 可直接展示给用户；
// ModelsFound 仅在 provider 返回模型列表时填充。
type TestResult struct {
	OK          bool
	Message     string
	LatencyMs   int64
	ModelsFound []string
}

// ConnectivityTester is Service's port for verifying credentials with the
// upstream provider. Mocked in Service tests.
//
// ConnectivityTester 是 Service 验证凭证的端口。Service 测试里可 mock。
type ConnectivityTester interface {
	Test(ctx context.Context, provider, key, baseURL, apiFormat string) (*TestResult, error)
}

// HTTPTester dispatches on ProviderMeta.TestMethod to call the upstream
// with its expected auth style.
//
// HTTPTester 按 ProviderMeta.TestMethod 分派，用 provider 期望的认证方式调上游。
type HTTPTester struct {
	client *http.Client
}

// NewHTTPTester installs a default 10s-timeout client when given nil.
//
// NewHTTPTester 传 nil 时装默认 10s 总超时 client。
func NewHTTPTester(client *http.Client) *HTTPTester {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	return &HTTPTester{client: client}
}

var _ ConnectivityTester = (*HTTPTester)(nil)

// Test dispatches by the provider's TestMethod. Unknown providers and
// missing-required-baseURL surface as errors (Service validates before
// this); transport/auth outcomes surface in TestResult.
//
// Test 按 provider 的 TestMethod 分派。未知 provider、必填 baseURL 缺失
// 以 error 返回（Service 调用前应已校验）；传输/认证结果通过 TestResult 返回。
func (t *HTTPTester) Test(ctx context.Context, provider, key, baseURL, apiFormat string) (*TestResult, error) {
	meta, ok := GetProviderMeta(provider)
	if !ok {
		return nil, fmt.Errorf("apikeytester: unknown provider %q: %w", provider, apikeydomain.ErrInvalidProvider)
	}
	effective := strings.TrimRight(baseURL, "/")
	if effective == "" {
		effective = strings.TrimRight(meta.DefaultBaseURL, "/")
	}
	if effective == "" {
		return nil, fmt.Errorf("apikeytester: baseURL required for provider %q: %w", provider, apikeydomain.ErrBaseURLRequired)
	}

	switch meta.TestMethod {
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
		// Empty APIFormat falls through to openai-compatible — the default.
		// 空 APIFormat 落入 openai-compatible——默认值。
		return t.testGetModels(ctx, effective, key), nil
	default:
		return nil, fmt.Errorf("apikeytester: unhandled test method %q", meta.TestMethod)
	}
}

// testGetModels: GET {baseURL}/models with Bearer auth (OpenAI-compatible).
//
// testGetModels：GET {baseURL}/models，Bearer 认证（OpenAI 兼容）。
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

// testAnthropicPing: POST {baseURL}/v1/messages, 1-token body.
// Anthropic has no /models endpoint; this is the cheapest probe (~$0.0001/call).
//
// testAnthropicPing：POST {baseURL}/v1/messages，1-token 请求体。
// Anthropic 无 /models 端点，这是最便宜的探测（约 $0.0001/次）。
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

// testGoogleListModels: GET {baseURL}/v1beta/models?key={key}. Google
// accepts auth via query param (used here) or x-goog-api-key header.
//
// testGoogleListModels：GET {baseURL}/v1beta/models?key={key}。
// Google 支持 query 参数（此处）或 x-goog-api-key 头。
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

// testOllamaTags: GET {baseURL}/api/tags, no auth (Ollama runs local).
//
// testOllamaTags：GET {baseURL}/api/tags，无认证（Ollama 本地运行）。
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

// do sends req and returns (body, status, latencyMs, err). Body is capped
// at 64 KiB — enough for model lists and truncated error snippets.
//
// do 发送 req 并返回 (body, status, latencyMs, err)。body 上限 64 KiB——
// 足够解析模型列表和截断的错误片段。
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

// formatHTTPError builds "HTTP {status}: {first 200B of body}" — safe for UI.
//
// formatHTTPError 组装 "HTTP {status}: {body 前 200 字节}"，可安全展示。
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

// parseOpenAIModels extracts IDs from {"data":[{"id":"..."}]}. Returns nil
// on malformed JSON — connectivity still reported as success.
//
// parseOpenAIModels 从 {"data":[{"id":"..."}]} 提取 ID。JSON 格式错返回 nil——
// 连通性仍报告成功。
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

// parseModelsByName extracts names from {"models":[{"name":"..."}]}.
// Both Google's /v1beta/models and Ollama's /api/tags use this shape.
// If the two ever diverge, fork this back into per-provider helpers.
//
// parseModelsByName 从 {"models":[{"name":"..."}]} 提取名字。
// Google 的 /v1beta/models 和 Ollama 的 /api/tags 都用这个形状。
// 哪天两边形状漂移再拆回各自 helper。
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
