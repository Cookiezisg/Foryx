package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
	apikeydomain "github.com/sunweilin/forgify/backend/internal/domain/apikey"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"

	"go.uber.org/zap"
)

// HTTP-classified sentinels enabling errors.Is matching (not substring).
//
// HTTP 状态分类的 sentinel，允许 errors.Is 匹配（非 substring）。
var (
	ErrAuthFailed   = errors.New("websearch: provider authentication failed")
	ErrRateLimited  = errors.New("websearch: provider rate limited")
	ErrUpstreamHTTP = errors.New("websearch: provider upstream HTTP error")
)

const (
	searchTimeout      = 10 * time.Second
	defaultSearchLimit = 10
	maxSearchLimit     = 30
)

var (
	// ErrEmptyQuery: query missing or empty.
	//
	// ErrEmptyQuery：query 缺失或为空。
	ErrEmptyQuery = errors.New("query is required and must be non-empty")
)


const searchDescription = `Web search. Routes to the first available source: a configured BYOK provider (Brave / Serper / Tavily / Bocha), then the duckduckgo-search MCP server (if installed). When neither is available the result includes a hint about how to enable one.

Usage:
- ` + "`query`" + ` is the search string (treated as one phrase by the upstream engine).
- Returns JSON: {"query","source","results":[{"title","url","snippet"}],"truncated"}.
- ` + "`source`" + ` indicates which backend produced the results: "brave" / "serper" / "tavily" / "bocha" / "mcp".
- ` + "`limit`" + ` caps the result count (default 10, hard max 30).
- Each backend has a 10-second budget; the tool falls through if a backend returns no results or errors.`

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

// WebSearch implements the WebSearch system tool with BYOK → MCP routing.
//
// WebSearch 是 WebSearch 系统工具的实现，BYOK → MCP 路由。
type WebSearch struct {
	httpClient *http.Client
	keys       apikeydomain.KeyProvider
	mcpRouter  MCPSearchRouter
	log        *zap.Logger
}

func (t *WebSearch) Name() string                { return "WebSearch" }
func (t *WebSearch) Description() string         { return searchDescription }
func (t *WebSearch) Parameters() json.RawMessage { return searchSchema }

func (t *WebSearch) IsReadOnly() bool        { return true }
func (t *WebSearch) NeedsReadFirst() bool    { return false }
func (t *WebSearch) RequiresWorkspace() bool { return false }

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
		return errors.New("limit must be non-negative")
	}
	return nil
}

func (t *WebSearch) CheckPermissions(_ json.RawMessage, _ toolapp.PermissionMode) toolapp.PermissionResult {
	return toolapp.PermissionAllow
}


// Execute walks BYOK → MCP routing and returns the first non-empty result list.
//
// Execute 走 BYOK → MCP 路由，返首个非空结果列表。
func (t *WebSearch) Execute(ctx context.Context, argsJSON string) (string, error) {
	var args searchArgs
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("WebSearch.Execute: %w", err)
	}
	args.normalize()

	if t.keys != nil {
		order := apikeydomain.SearchProviderPriority
		if def := t.keys.DefaultSearchProvider(ctx); def != "" {
			order = append([]string{def}, removeStr(apikeydomain.SearchProviderPriority, def)...)
		}
		for _, provider := range order {
			if ctx.Err() != nil {
				break
			}
			results, source, ok := t.tryBYOKProvider(ctx, provider, args.Query, args.Limit)
			if ok && len(results) > 0 {
				return marshalSearchResponse(args, source, results)
			}
		}
	}

	if ctx.Err() == nil && t.mcpRouter != nil {
		results, err := t.runMCPSearch(ctx, args.Query, args.Limit)
		switch {
		case errors.Is(err, ErrMCPSearchUnavailable):
		case err != nil:
			t.warnf("WebSearch MCP backend failed; falling through", err)
		case len(results) > 0:
			return marshalSearchResponse(args, "mcp", results)
		}
	}

	return fmt.Sprintf("No results for %q. No search backend is currently available.\n\n"+
		"To enable web search, do ONE of the following:\n"+
		"  • Configure a search-category API key (Brave / Serper / Tavily — international; Bocha — China; all have free tiers).\n"+
		"  • Install the duckduckgo-search MCP server from the marketplace (no API key needed; ~30s install).",
		args.Query), nil
}

// tryBYOKProvider tries one BYOK provider; ErrNotFoundForProvider is silent (user just hasn't configured it).
//
// tryBYOKProvider 试一个 BYOK provider；ErrNotFoundForProvider 静默（用户没配）。
func (t *WebSearch) tryBYOKProvider(ctx context.Context, provider, query string, limit int) ([]searchResult, string, bool) {
	creds, err := t.keys.ResolveCredentials(ctx, provider)
	if err != nil {
		if !errors.Is(err, apikeydomain.ErrNotFoundForProvider) {
			t.warnf(fmt.Sprintf("WebSearch BYOK %q ResolveCredentials failed (not 'no key configured')", provider), err)
		}
		return nil, "", false
	}

	baseURL := strings.TrimRight(creds.BaseURL, "/")
	if baseURL == "" {
		return nil, "", false
	}

	var (
		results []searchResult
		runErr  error
	)
	switch provider {
	case "brave":
		results, runErr = t.searchBrave(ctx, baseURL, creds.Key, query, limit)
	case "serper":
		results, runErr = t.searchSerper(ctx, baseURL, creds.Key, query, limit)
	case "tavily":
		results, runErr = t.searchTavily(ctx, baseURL, creds.Key, query, limit)
	case "bocha":
		results, runErr = t.searchBocha(ctx, baseURL, creds.Key, query, limit)
	default:
		return nil, "", false
	}
	if runErr != nil {
		t.warnf(fmt.Sprintf("WebSearch BYOK %q failed; falling through", provider), runErr)
		t.markInvalidIfAuthErr(ctx, provider, runErr)
		return nil, "", false
	}
	return results, provider, true
}

// markInvalidIfAuthErr surfaces 401/403 to apikey domain so the UI status flips; best-effort.
//
// markInvalidIfAuthErr 把 401/403 通知 apikey 域让 UI 翻转；best-effort。
func (t *WebSearch) markInvalidIfAuthErr(ctx context.Context, provider string, err error) {
	if !errors.Is(err, ErrAuthFailed) {
		return
	}
	if t.keys == nil {
		return
	}
	uid, _ := reqctxpkg.GetUserID(ctx)
	mctx := ctx
	if uid != "" {
		mctx = reqctxpkg.SetUserID(context.Background(), uid)
	}
	if merr := t.keys.MarkInvalid(mctx, provider, err.Error()); merr != nil {
		t.debugf(fmt.Sprintf("MarkInvalid for %q failed", provider), merr)
	}
}

func (t *WebSearch) warnf(msg string, err error) {
	if t.log == nil {
		return
	}
	t.log.Warn(msg, zap.Error(err))
}

func (t *WebSearch) debugf(msg string, err error) {
	if t.log == nil {
		return
	}
	t.log.Debug(msg, zap.Error(err))
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


// removeStr returns a copy of s with all occurrences of x removed.
//
// removeStr 返回去除所有 x 后的 s 副本。
func removeStr(s []string, x string) []string {
	out := make([]string, 0, len(s))
	for _, v := range s {
		if v != x {
			out = append(out, v)
		}
	}
	return out
}

var _ toolapp.Tool = (*WebSearch)(nil)
