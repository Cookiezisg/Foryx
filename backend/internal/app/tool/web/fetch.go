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

	loopapp "github.com/sunweilin/forgify/backend/internal/app/loop"
	modelclientapp "github.com/sunweilin/forgify/backend/internal/app/modelclient"
	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
	apikeydomain "github.com/sunweilin/forgify/backend/internal/domain/apikey"
	modeldomain "github.com/sunweilin/forgify/backend/internal/domain/model"
	workspacedomain "github.com/sunweilin/forgify/backend/internal/domain/workspace"
	llminfra "github.com/sunweilin/forgify/backend/internal/infra/llm"
	errorspkg "github.com/sunweilin/forgify/backend/internal/pkg/errors"
)

const (
	fetchTimeout  = 30 * time.Second
	maxFetchBytes = 1 << 20
)

// jinaEndpoint is the public Jina reader; var (not const) so tests can override.
//
// jinaEndpoint 是 Jina 公共 reader；用 var 是为测试可改写。
var jinaEndpoint = "https://r.jina.ai/"

// fetchDirectFn is the direct-GET seam; var so tests can stub the network out
// (the SSRF guard only passes public addresses, so a hermetic test cannot host one).
//
// fetchDirectFn 是直 GET 的测试缝；用 var 是因为 SSRF 守卫只放行公网地址，封闭测试无法
// 自架一个，故测试需替换掉网络。
var fetchDirectFn = fetchDirect

var (
	// ErrEmptyURL: url missing or empty.
	//
	// ErrEmptyURL：url 缺失或为空。
	ErrEmptyURL = errorspkg.New(errorspkg.KindInvalid, "WEB_EMPTY_URL", "url is required and must be non-empty")

	// ErrEmptyPrompt: prompt missing or empty.
	//
	// ErrEmptyPrompt：prompt 缺失或为空。
	ErrEmptyPrompt = errorspkg.New(errorspkg.KindInvalid, "WEB_EMPTY_PROMPT", "prompt is required and must be non-empty")

	// ErrUnsupportedScheme: only http and https allowed (SSRF surface).
	//
	// ErrUnsupportedScheme：仅允许 http/https（SSRF 面）。
	ErrUnsupportedScheme = errorspkg.New(errorspkg.KindInvalid, "WEB_UNSUPPORTED_SCHEME", "url must use http or https scheme")
)

const fetchDescription = `Fetch a URL and return an LLM summary answering prompt. Absolute http/https only; private/loopback addresses blocked. Retrieval method (local direct GET vs Jina reader) follows the workspace webFetchMode setting.`

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

// FetchModePicker resolves the workspace's web-fetch mode (workspacedomain.WebFetchMode*,
// already defaulted — never ""). Satisfied by *workspaceapp.Service.
//
// FetchModePicker 解析 workspace 的抓取模式（workspacedomain.WebFetchMode*，已兜底——永不为 ""）。
// 由 *workspaceapp.Service 实现。
type FetchModePicker interface {
	WebFetchMode(ctx context.Context) string
}

// WebFetch fetches a URL (SSRF-guarded) and returns a utility-model summary
// answering the prompt — it does not return raw HTML, so a huge page never
// floods the context window. How the page is retrieved is a workspace setting
// (PD-4 C): "local" = direct GET only (no URL leaves the machine); "jina" =
// Jina reader first, direct GET fallback.
//
// WebFetch 抓 URL（SSRF 守卫）并返回 utility 模型按 prompt 的摘要——不返原始 HTML，
// 使超大页面永不灌爆上下文窗口。抓取方式是 workspace 配置（PD-4 C）："local" = 仅本机
// 直接 GET（URL 不出本机）；"jina" = Jina reader 优先、直 GET 兜底。
type WebFetch struct {
	picker  modeldomain.ModelPicker
	keys    apikeydomain.KeyProvider
	factory *llminfra.Factory
	mode    FetchModePicker // nil → local (fail-closed on privacy)
}

func (t *WebFetch) Name() string                { return "WebFetch" }
func (t *WebFetch) Description() string         { return fetchDescription }
func (t *WebFetch) Parameters() json.RawMessage { return fetchSchema }

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

// Execute runs SSRF check → two-tier fetch → cap → LLM summary; failures return
// friendly strings (not Go err). Summary failure (incl. no utility model
// configured) degrades to returning the raw content truncated.
//
// Execute 做 SSRF 检查 → 两段抓取 → 截断 → LLM 摘要；失败返友好字符串（非 Go err）。
// 摘要失败（含未配 utility 模型）降级为返原始内容截断。
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

	content, err := t.fetchContent(ctx, args.URL)
	if err != nil {
		return fmt.Sprintf("Failed to fetch %s: %v", args.URL, err), nil
	}
	if strings.TrimSpace(content) == "" {
		return fmt.Sprintf("Fetched %s but body was empty.", args.URL), nil
	}

	summary, err := t.summarise(ctx, args.URL, args.Prompt, content)
	if err != nil {
		return fmt.Sprintf("Summarisation unavailable (%v). Raw content (first 4 KB):\n\n%s",
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

// fetchContent retrieves the page per the workspace's web-fetch mode: local = direct GET only
// (the URL never leaves this machine); jina = Jina reader first, direct GET fallback. A nil
// picker fails closed to local — privacy degradation is never silent.
//
// fetchContent 按 workspace 抓取模式取页面：local = 仅直接 GET（URL 不出本机）；jina = Jina
// 优先、直 GET 兜底。picker 为 nil 时收敛到 local——隐私降级绝不静默发生。
func (t *WebFetch) fetchContent(ctx context.Context, target string) (string, error) {
	mode := workspacedomain.WebFetchModeLocal
	if t.mode != nil {
		mode = t.mode.WebFetchMode(ctx)
	}
	if mode != workspacedomain.WebFetchModeJina {
		return fetchDirectFn(ctx, target)
	}
	if body, err := fetchViaJina(ctx, target); err == nil {
		return body, nil
	} else if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return "", err
	}
	return fetchDirectFn(ctx, target)
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

// summarise resolves the utility model, builds a client, and asks it to answer
// prompt against content. Resolution goes through modelclient — the one shared
// chain (a hand-rolled copy here once miswired base URL into the wire model id,
// AC-26); no Thinking — knobs ride in ModelRef.Options.
//
// summarise 解析 utility 模型、构造 client、让它按 prompt 回答 content。解析走
// modelclient——唯一共享链（这里曾手抄一份并把 base URL 误接进线缆 model id，
// AC-26）；无 Thinking——旋钮走 ModelRef.Options。
func (t *WebFetch) summarise(ctx context.Context, source, prompt, content string) (string, error) {
	client, req, _, err := modelclientapp.Resolve(ctx, modeldomain.ScenarioUtility, nil, t.picker, t.keys, t.factory)
	if err != nil {
		return "", err
	}
	// Consume the raw stream (NOT Generate — its retry would re-emit the partial summary on retry;
	// the llm package doc forbids retry for emitting callers) and tee each text delta to a live
	// `progress` block under the tool_call, so the user watches the summary get written token by
	// token. nil-safe off a streamed turn (no frames, identical return).
	//
	// 直接消费裸 stream（不用 Generate——它的 retry 会在重试时重发已出的半截摘要；llm 包注释禁止 emit 调用方
	// 套 retry），把每个 text delta 实时 tee 到 tool_call 下的 `progress` 块，使用户逐字看摘要被写出来。非流式
	// turn 下 nil 安全（无帧、返回一致）。
	prog := loopapp.ToolProgress(ctx)
	defer prog.Close()
	req.Messages = []llminfra.LLMMessage{{Role: llminfra.RoleUser, Content: buildSummaryPrompt(source, prompt, content)}}
	var sb strings.Builder
	for event := range client.Stream(ctx, req) {
		switch event.Type {
		case llminfra.EventText:
			sb.WriteString(event.Delta)
			prog.Print(event.Delta)
		case llminfra.EventError:
			return "", event.Err
		}
	}
	return strings.TrimSpace(sb.String()), nil
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
