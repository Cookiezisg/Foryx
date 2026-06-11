package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"go.uber.org/zap"

	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
	apikeydomain "github.com/sunweilin/forgify/backend/internal/domain/apikey"
	websearchdomain "github.com/sunweilin/forgify/backend/internal/domain/websearch"
	errorspkg "github.com/sunweilin/forgify/backend/internal/pkg/errors"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

// Sentinels with real HTTP semantics — upstream provider failures map to 502/429
// if ever surfaced via HTTP; today they reach the LLM as a tool-result message.
// errors.Is matches by Code.
//
// 带真 HTTP 语义的 sentinel——上游 provider 失败将来若经 HTTP 冒出即 502/429；今天经
// tool-result 给 LLM。errors.Is 按 Code 匹配。
var (
	ErrAuthFailed   = errorspkg.New(errorspkg.KindBadGateway, "WEBSEARCH_AUTH_FAILED", "search provider authentication failed")
	ErrRateLimited  = errorspkg.New(errorspkg.KindRateLimited, "WEBSEARCH_RATE_LIMITED", "search provider rate limited")
	ErrUpstreamHTTP = errorspkg.New(errorspkg.KindBadGateway, "WEBSEARCH_UPSTREAM_HTTP", "search provider upstream error")
)

const (
	searchTimeout      = 10 * time.Second
	defaultSearchLimit = 10
	maxSearchLimit     = 30
)

// ErrEmptyQuery: query missing or empty.
//
// ErrEmptyQuery：query 缺失或为空。
var ErrEmptyQuery = errorspkg.New(errorspkg.KindInvalid, "WEBSEARCH_EMPTY_QUERY", "query is required and must be non-empty")

// ErrNegativeLimit: limit must be >= 0.
//
// ErrNegativeLimit：limit 不可为负。
var ErrNegativeLimit = errorspkg.New(errorspkg.KindInvalid, "WEBSEARCH_NEGATIVE_LIMIT", "limit must be non-negative")

const searchDescription = `Web search via the workspace's configured search key (BYOK: Brave/Serper/Tavily/Bocha — one explicit key). Returns JSON {query,source,results:[{title,url,snippet}],truncated}. If no search key is configured, returns setup guidance.`

var searchSchema = json.RawMessage(`{
	"type": "object",
	"required": ["query"],
	"properties": {
		"query": {
			"type": "string",
			"description": "Search query string."
		},
		"limit": {
			"type": "number",
			"description": "Maximum results to return (default 10, hard max 30)."
		}
	}
}`)

type searchArgs struct {
	Query string `json:"query"`
	Limit int    `json:"limit"`
}

func (a *searchArgs) normalize() {
	if a.Limit == 0 {
		a.Limit = defaultSearchLimit
	}
	if a.Limit > maxSearchLimit {
		a.Limit = maxSearchLimit
	}
}

type searchResult struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet"`
}

type searchResponse struct {
	Query     string         `json:"query"`
	Source    string         `json:"source"`
	Results   []searchResult `json:"results"`
	Truncated bool           `json:"truncated"`
}

// WebSearch runs a query against the workspace's single configured search key
// (BYOK). No provider walk (the old SearchProviderPriority burned credits probing
// each), no MCP tier (a search MCP server exposes its own tool via tool/mcp).
//
// WebSearch 用 workspace 唯一配置的搜索 key（BYOK）跑查询。无 provider 遍历（旧
// SearchProviderPriority 挨个试烧钱），无 MCP tier（搜索 MCP server 经 tool/mcp 暴露自己的工具）。
type WebSearch struct {
	httpClient *http.Client
	keys       apikeydomain.KeyProvider
	searchKeys websearchdomain.SearchKeyPicker
	log        *zap.Logger
}

func (t *WebSearch) Name() string                { return "WebSearch" }
func (t *WebSearch) Description() string         { return searchDescription }
func (t *WebSearch) Parameters() json.RawMessage { return searchSchema }

// ValidateInput rejects empty queries and negative limits pre-Execute.
//
// ValidateInput 在 Execute 前拒绝空 query 与负 limit。
func (t *WebSearch) ValidateInput(args json.RawMessage) error {
	var a searchArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return fmt.Errorf("WebSearch.ValidateInput: %w", err)
	}
	if strings.TrimSpace(a.Query) == "" {
		return ErrEmptyQuery
	}
	if a.Limit < 0 {
		return ErrNegativeLimit
	}
	return nil
}

// Execute resolves the workspace's single search key and queries its provider;
// no key configured → actionable guidance. provider is implied by the key
// (Credentials.Provider). 401/403 marks the key invalid by id.
//
// Execute 解析 workspace 唯一搜索 key 并查其 provider；未配 → 可操作引导。provider 由 key
// 隐含（Credentials.Provider）。401/403 按 id 标该 key 失效。
func (t *WebSearch) Execute(ctx context.Context, argsJSON string) (string, error) {
	var args searchArgs
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("WebSearch.Execute: %w", err)
	}
	args.normalize()

	keyID, ok := t.searchKeys.DefaultSearchKeyID(ctx)
	if !ok {
		return noBackendMessage(args.Query), nil
	}
	creds, err := t.keys.ResolveCredentialsByID(ctx, keyID)
	if err != nil {
		// Key vanished (deleted) — surface guidance rather than an opaque error.
		// key 没了（被删）——返引导而非晦涩错误。
		t.warnf("WebSearch: resolve configured search key failed", err)
		return noBackendMessage(args.Query), nil
	}
	if !websearchdomain.IsProvider(creds.Provider) {
		return fmt.Sprintf(
			"The configured default search key is provider %q, which is not a search backend "+
				"(expected one of brave / serper / tavily / bocha). Point the workspace's default "+
				"search key at a search-category key.", creds.Provider), nil
	}

	baseURL := strings.TrimRight(creds.BaseURL, "/")
	if baseURL == "" {
		return fmt.Sprintf("Search provider %q has no base URL configured.", creds.Provider), nil
	}

	results, err := t.runProvider(ctx, creds.Provider, baseURL, creds.Key, args.Query, args.Limit)
	if err != nil {
		t.warnf(fmt.Sprintf("WebSearch %q failed", creds.Provider), err)
		t.markInvalidIfAuthErr(ctx, keyID, err)
		return fmt.Sprintf("Search via %s failed: %v", creds.Provider, err), nil
	}
	return marshalSearchResponse(args, creds.Provider, results)
}

// runProvider dispatches to the per-provider HTTP call (search_byok.go). The
// default branch is defensive — IsProvider already gated the provider.
//
// runProvider 分派到各 provider 的 HTTP 调用（search_byok.go）。default 分支是防御——
// IsProvider 已门控过 provider。
func (t *WebSearch) runProvider(ctx context.Context, provider, baseURL, key, query string, limit int) ([]searchResult, error) {
	switch provider {
	case websearchdomain.ProviderBrave:
		return t.searchBrave(ctx, baseURL, key, query, limit)
	case websearchdomain.ProviderSerper:
		return t.searchSerper(ctx, baseURL, key, query, limit)
	case websearchdomain.ProviderTavily:
		return t.searchTavily(ctx, baseURL, key, query, limit)
	case websearchdomain.ProviderBocha:
		return t.searchBocha(ctx, baseURL, key, query, limit)
	default:
		return nil, fmt.Errorf("unknown search provider %q", provider)
	}
}

// noBackendMessage guides the user to configure a search key OR install a search
// MCP server. The MCP mention is plain text — that path runs through tool/mcp,
// NOT through WebSearch (no proxying).
//
// noBackendMessage 引导用户配搜索 key 或装搜索 MCP server。MCP 提示是纯文字——那条路走
// tool/mcp、不经 WebSearch（不代理）。
func noBackendMessage(query string) string {
	return fmt.Sprintf("No search backend configured for %q.\n\n"+
		"To enable web search, do ONE of the following:\n"+
		"  • Configure a search-category API key (Brave / Serper / Tavily — international; Bocha — China; all have free tiers) and set it as the workspace's default search key.\n"+
		"  • Install a search MCP server (e.g. duckduckgo-search) — it exposes its own search tool you can call directly (no API key needed).",
		query)
}

// markInvalidIfAuthErr surfaces 401/403 to apikey domain so the UI status flips;
// best-effort, on a detached context that preserves the workspace id (§S9).
//
// markInvalidIfAuthErr 把 401/403 通知 apikey 域让 UI 翻转；best-effort，detached context
// 保留 workspace id（§S9）。
func (t *WebSearch) markInvalidIfAuthErr(ctx context.Context, keyID string, err error) {
	if !errors.Is(err, ErrAuthFailed) || t.keys == nil {
		return
	}
	mctx := ctx
	if wsID, ok := reqctxpkg.GetWorkspaceID(ctx); ok {
		mctx = reqctxpkg.SetWorkspaceID(context.Background(), wsID)
	}
	if merr := t.keys.MarkInvalidByID(mctx, keyID, err.Error()); merr != nil {
		t.debugf("MarkInvalidByID failed", merr)
	}
}

func (t *WebSearch) warnf(msg string, err error) {
	if t.log != nil {
		t.log.Warn(msg, zap.Error(err))
	}
}

func (t *WebSearch) debugf(msg string, err error) {
	if t.log != nil {
		t.log.Debug(msg, zap.Error(err))
	}
}

func marshalSearchResponse(args searchArgs, source string, results []searchResult) (string, error) {
	truncated := false
	if len(results) > args.Limit {
		results = results[:args.Limit]
		truncated = true
	}
	body, err := json.MarshalIndent(searchResponse{
		Query:     args.Query,
		Source:    source,
		Results:   results,
		Truncated: truncated,
	}, "", "  ")
	if err != nil {
		return "", fmt.Errorf("WebSearch.Execute: marshal: %w", err)
	}
	return string(body), nil
}

var _ toolapp.Tool = (*WebSearch)(nil)
