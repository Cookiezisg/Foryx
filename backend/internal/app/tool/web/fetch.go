package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
	apikeydomain "github.com/sunweilin/forgify/backend/internal/domain/apikey"
	modeldomain "github.com/sunweilin/forgify/backend/internal/domain/model"
	llminfra "github.com/sunweilin/forgify/backend/internal/infra/llm"
	llmclientpkg "github.com/sunweilin/forgify/backend/internal/pkg/llmclient"
)


const (
	fetchTimeout  = 30 * time.Second
	maxFetchBytes = 1 << 20
)

// jinaEndpoint is the public Jina reader; var (not const) so tests can override.
//
// jinaEndpoint 是 Jina 公共 reader；用 var 是为测试可改写。
var jinaEndpoint = "https://r.jina.ai/"

var (
	// ErrEmptyURL: url missing or empty.
	//
	// ErrEmptyURL：url 缺失或为空。
	ErrEmptyURL = errors.New("url is required and must be non-empty")

	// ErrEmptyPrompt: prompt missing or empty.
	//
	// ErrEmptyPrompt：prompt 缺失或为空。
	ErrEmptyPrompt = errors.New("prompt is required and must be non-empty")

	// ErrUnsupportedScheme: only http and https allowed (SSRF surface).
	//
	// ErrUnsupportedScheme：仅允许 http/https（SSRF 面）。
	ErrUnsupportedScheme = errors.New("url must use http or https scheme")
)


const fetchDescription = `Fetches a URL and returns an LLM-generated summary tailored to your prompt.

Usage:
- ` + "`url`" + ` must be an absolute http or https URL.
- ` + "`prompt`" + ` describes what to extract or summarise from the page (e.g. "What does this paper conclude?", "List every API endpoint mentioned").
- The tool fetches the URL (Jina reader for clean markdown when available, direct HTTP GET fallback), caps content at 1 MB, then asks the configured summary model to answer your prompt against that content.
- Summarisation uses the "web_summary" model scenario if configured; otherwise falls back to the "chat" scenario.
- Private / loopback / link-local addresses are blocked (no fetching localhost or RFC 1918 ranges).
- Each fetch is capped at 30 seconds.`

var fetchSchema = json.RawMessage(`{
	"type": "object",
	"required": ["url", "prompt"],
	"properties": {
		"url": {
			"type": "string",
			"description": "Absolute http or https URL to fetch."
		},
		"prompt": {
			"type": "string",
			"description": "What to extract or summarise from the page content."
		}
	}
}`)


// WebFetch implements the WebFetch system tool.
//
// WebFetch 是 WebFetch 系统工具的实现。
type WebFetch struct {
	picker  modeldomain.ModelPicker
	keys    apikeydomain.KeyProvider
	factory *llminfra.Factory
}

func (t *WebFetch) Name() string                { return "WebFetch" }
func (t *WebFetch) Description() string         { return fetchDescription }
func (t *WebFetch) Parameters() json.RawMessage { return fetchSchema }

func (t *WebFetch) IsReadOnly() bool        { return true }
func (t *WebFetch) NeedsReadFirst() bool    { return false }
func (t *WebFetch) RequiresWorkspace() bool { return false }

// ValidateInput rejects empty url/prompt and non-http(s) schemes pre-Execute.
//
// ValidateInput 在 Execute 前拒绝空 url / prompt / 非 http(s) scheme。
func (t *WebFetch) ValidateInput(args json.RawMessage) error {
	var a struct {
		URL    string `json:"url"`
		Prompt string `json:"prompt"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return fmt.Errorf("WebFetch.ValidateInput: %w", err)
	}
	if strings.TrimSpace(a.URL) == "" {
		return ErrEmptyURL
	}
	if strings.TrimSpace(a.Prompt) == "" {
		return ErrEmptyPrompt
	}
	u, err := url.Parse(a.URL)
	if err != nil {
		return fmt.Errorf("WebFetch.ValidateInput: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return ErrUnsupportedScheme
	}
	return nil
}

func (t *WebFetch) CheckPermissions(_ json.RawMessage, _ toolapp.PermissionMode) toolapp.PermissionResult {
	return toolapp.PermissionAllow
}


// Execute runs SSRF check → two-tier fetch → cap → LLM summary; failures return friendly strings (not Go err).
//
// Execute 做 SSRF 检查 → 两段抓取 → 截断 → LLM 摘要；失败返友好字符串（非 Go err）。
func (t *WebFetch) Execute(ctx context.Context, argsJSON string) (string, error) {
	var args struct {
		URL    string `json:"url"`
		Prompt string `json:"prompt"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("WebFetch.Execute: %w", err)
	}

	parsed, err := url.Parse(args.URL)
	if err != nil {
		return fmt.Sprintf("Invalid URL %q: %v", args.URL, err), nil
	}
	if reason := guardHostname(parsed.Hostname()); reason != "" {
		return reason, nil
	}

	content, err := fetchContent(ctx, args.URL)
	if err != nil {
		return fmt.Sprintf("Failed to fetch %s: %v", args.URL, err), nil
	}
	if strings.TrimSpace(content) == "" {
		return fmt.Sprintf("Fetched %s but body was empty.", args.URL), nil
	}

	summary, err := t.summarise(ctx, args.URL, args.Prompt, content)
	if err != nil {
		return fmt.Sprintf("Summarisation failed (%v). Raw content (first 4 KB):\n\n%s",
			err, truncate(content, 4096)), nil
	}
	return summary, nil
}


// fetchClient is a process-wide http.Client with CheckRedirect re-running the SSRF guard on each hop.
//
// fetchClient 进程级 http.Client；CheckRedirect 在每跳重跑 SSRF 守卫。
var fetchClient = &http.Client{
	Timeout:       fetchTimeout,
	CheckRedirect: ssrfCheckRedirect,
}

// ssrfCheckRedirect rejects redirects to loopback / private / link-local / unspecified / multicast.
//
// ssrfCheckRedirect 拒绝跳到 loopback / 私网 / link-local / 未指定 / multicast 的目标。
func ssrfCheckRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= 10 {
		return errors.New("stopped after 10 redirects")
	}
	if reason := guardHostname(req.URL.Hostname()); reason != "" {
		return fmt.Errorf("redirect blocked: %s", reason)
	}
	return nil
}

// fetchContent tries Jina first, falls back to direct GET; returns capped body.
//
// fetchContent 先 Jina，失败 fallback 直 GET；返截断后正文。
func fetchContent(ctx context.Context, target string) (string, error) {
	if body, err := fetchViaJina(ctx, target); err == nil {
		return body, nil
	} else if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return "", err
	}
	return fetchDirect(ctx, target)
}

// fetchViaJina fetches via Jina reader; JINA_API_KEY enables higher rate limits.
//
// fetchViaJina 走 Jina reader 抓取；JINA_API_KEY 设了走高速率档。
func fetchViaJina(ctx context.Context, target string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, jinaEndpoint+target, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "text/markdown")
	req.Header.Set("User-Agent", "ForgifyWebFetch/1.0")
	if k := strings.TrimSpace(os.Getenv("JINA_API_KEY")); k != "" {
		req.Header.Set("Authorization", "Bearer "+k)
	}
	return doRequest(req)
}

// fetchDirect performs a plain HTTP GET as Jina fallback.
//
// fetchDirect 直接 HTTP GET，作为 Jina 兜底。
func fetchDirect(ctx context.Context, target string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "ForgifyWebFetch/1.0")
	return doRequest(req)
}

// doRequest sends req via fetchClient with byte cap; non-2xx becomes an error.
//
// doRequest 用 fetchClient 发 req（带字节封顶）；非 2xx 报错。
func doRequest(req *http.Request) (string, error) {
	resp, err := fetchClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("http status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxFetchBytes))
	if err != nil {
		return "", err
	}
	return string(body), nil
}


// guardHostname returns empty when host is safe; rejects any DNS answer in a denied range (anti-rebinding).
//
// guardHostname 安全则返空；任一 DNS 答案落入禁区即拒（防 DNS rebinding）。
func guardHostname(host string) string {
	if host == "" {
		return "URL has no host."
	}
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	if host == "localhost" || host == "ip6-localhost" || host == "ip6-loopback" {
		return "Refusing to fetch loopback host: " + host
	}
	if ip := net.ParseIP(host); ip != nil {
		if reason := classifyIP(ip); reason != "" {
			return reason
		}
		return ""
	}
	ips, err := net.LookupIP(host)
	if err != nil {
		return fmt.Sprintf("Cannot resolve host %s: %v", host, err)
	}
	for _, ip := range ips {
		if reason := classifyIP(ip); reason != "" {
			return reason
		}
	}
	return ""
}

// classifyIP returns a rejection message for denied ranges; empty for safe public addresses.
//
// classifyIP 禁区返拒绝消息；公网安全地址返空。
func classifyIP(ip net.IP) string {
	switch {
	case ip.IsLoopback():
		return "Refusing to fetch loopback address: " + ip.String()
	case ip.IsPrivate():
		return "Refusing to fetch private address: " + ip.String()
	case ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast():
		return "Refusing to fetch link-local address: " + ip.String()
	case ip.IsUnspecified():
		return "Refusing to fetch unspecified address: " + ip.String()
	case ip.IsMulticast():
		return "Refusing to fetch multicast address: " + ip.String()
	}
	return ""
}


// summarise resolves the web_summary LLM (chat fallback) and asks it to answer prompt against content.
//
// summarise 解析 web_summary LLM（chat 兜底），让它按 prompt 回答 content。
func (t *WebFetch) summarise(ctx context.Context, source, prompt, content string) (string, error) {
	bundle, err := llmclientpkg.ResolveForWebSummary(ctx, t.picker, t.keys, t.factory)
	if err != nil {
		return "", err
	}
	body := buildSummaryPrompt(source, prompt, content)
	out, err := llminfra.Generate(ctx, bundle.Client, llminfra.Request{
		ModelID:  bundle.ModelID,
		Key:      bundle.Key,
		BaseURL:  bundle.BaseURL,
		Messages: []llminfra.LLMMessage{{Role: llminfra.RoleUser, Content: body}},
	})
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

func buildSummaryPrompt(source, prompt, content string) string {
	return fmt.Sprintf(`You are summarising web content fetched on the user's behalf.

Source URL: %s

User's request: %s

Below is the fetched content (it may be markdown rendered by Jina or raw HTML):

<<<CONTENT_BEGIN>>>
%s
<<<CONTENT_END>>>

Answer the user's request directly based on the content above. If the content does not contain the requested information, say so clearly.`,
		source, prompt, content)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "\n\n...[truncated]"
}


var _ toolapp.Tool = (*WebFetch)(nil)
