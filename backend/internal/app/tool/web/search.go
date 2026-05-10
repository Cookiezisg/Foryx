// search.go — WebSearch system tool: 2-tier routing (BYOK → MCP).
//
// Routing priority:
//  1. BYOK: iterate apikeydomain.SearchProviderPriority (brave, serper,
//     tavily, bocha) — first configured key whose call returns non-empty
//     wins. Per-provider failures fall through with warn log.
//  2. MCP: if user installed the duckduckgo-search MCP server (V1
//     marketplace entry), route the query through it. Connection failures
//     fall through with warn log; "not configured" falls through silently.
//
// When both tiers return empty / fail, the tool surfaces a clear LLM-
// actionable hint: install duckduckgo-search via install_mcp_server tool,
// or configure a search BYOK key. (We deliberately removed the previous
// Bing CN HTML scrape "fallback" — modern Bing/Bing CN renders results
// via JavaScript, so HTML scraping returns 0 hits regardless of UA. See
// progress-record.md 屎山拯救计划 #4 follow-up.)
//
// search.go ——WebSearch 系统工具：2 层路由（BYOK → MCP）。
//
// 路由优先级：
//  1. BYOK：按 apikeydomain.SearchProviderPriority 顺序遍历（brave / serper /
//     tavily / bocha）—— 第一个配了 key 且调用返非空的胜出。per-provider 失败
//     log warn 并降级。
//  2. MCP：用户装了 duckduckgo-search MCP server（V1 marketplace 条目）就
//     路由过去。连接失败 warn log 降级；未配置静默降级。
//
// 两层都空/失败时给 LLM 一条 actionable 提示：用 install_mcp_server 装
// duckduckgo-search，或者配 search BYOK key。（之前的 Bing CN HTML 抓取
// "兜底" 故意删了——现代 Bing 搜索结果完全 JS 渲染，HTML 抓取无论用啥 UA
// 都返 0 命中，是个假兜底。详见 progress-record.md 屎山拯救计划 #4 后续。）
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

// ── Sentinel errors ───────────────────────────────────────────────────────────
//
// HTTP-status-classified errors from BYOK provider calls. Wrapping with
// these sentinels lets markInvalidIfAuthErr (and any future caller) use
// errors.Is instead of fragile substring matching on err.Error(). Mirrors
// the llm.ErrAuthFailed / ErrRateLimited pattern from infra/llm (commit
// 363b084) — separate sentinel set because BYOK search providers
// (Brave / Serper / Tavily / Bocha) are not LLM transports.
//
// BYOK 调用按 HTTP 状态分类的 sentinel。markInvalidIfAuthErr（及未来调用方）
// 用 errors.Is 替代 err.Error() substring 匹配。同 infra/llm 的
// llm.ErrAuthFailed / ErrRateLimited 模式（commit 363b084）——独立
// sentinel 因为 BYOK search provider 不是 LLM transport。
var (
	ErrAuthFailed    = errors.New("websearch: provider authentication failed")
	ErrRateLimited   = errors.New("websearch: provider rate limited")
	ErrUpstreamHTTP  = errors.New("websearch: provider upstream HTTP error")
)

// ── Limits & defaults ─────────────────────────────────────────────────────────

const (
	// searchTimeout caps a single backend call. Two backends × 10s = 20s
	// worst case which fits comfortably inside the chat tool budget.
	//
	// searchTimeout 限制单后端调用；2 后端 × 10s = 20s 最坏，适配 chat 工具预算。
	searchTimeout = 10 * time.Second

	// defaultSearchLimit is the result count when LLM does not specify.
	// Matches what most search APIs return on a default query.
	//
	// defaultSearchLimit 是 LLM 不指定时的结果数；与多数搜索 API 默认一致。
	defaultSearchLimit = 10

	// maxSearchLimit hard cap so the LLM cannot ask for 1000 results.
	//
	// maxSearchLimit 硬上限，防 LLM 索取上千条。
	maxSearchLimit = 30
)

// ── Validation sentinels ──────────────────────────────────────────────────────

var (
	// ErrEmptyQuery: query missing or empty.
	// ErrEmptyQuery：query 缺失或为空。
	ErrEmptyQuery = errors.New("query is required and must be non-empty")
)

// ── Description & schema ──────────────────────────────────────────────────────

const searchDescription = `Web search. Routes to the first available source: configured BYOK provider (Brave / Serper / Tavily / Bocha), then duckduckgo-search MCP server (if installed). When neither is available the tool returns a clear hint — call list_mcp_marketplace to discover the duckduckgo-search backend, then install_mcp_server({name:"duckduckgo-search"}) to add it (~30s, no key needed).

Usage:
- ` + "`query`" + ` is the search string (treated as one phrase by the upstream engine).
- Returns JSON: {"query","source","results":[{"title","url","snippet"}],"truncated"}.
- ` + "`source`" + ` tells you which backend produced the results: "brave" / "serper" / "tavily" / "bocha" / "mcp".
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

// ── Args ──────────────────────────────────────────────────────────────────────

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

// ── Output ────────────────────────────────────────────────────────────────────

// searchResult is one hit. Field shapes match what an LLM expects from a
// search API; we never leak engine-specific extras.
//
// searchResult 是一条命中；字段形态对齐 LLM 对搜索 API 的期望，不漏后端内部。
type searchResult struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet"`
}

type searchResponse struct {
	Query     string         `json:"query"`
	Source    string         `json:"source"` // "brave" / "serper" / "tavily" / "bocha" / "mcp"
	Results   []searchResult `json:"results"`
	Truncated bool           `json:"truncated"`
}

// ── Tool struct & 9 methods ───────────────────────────────────────────────────

// WebSearch implements the WebSearch system tool. Carries:
//   - httpClient: short-timeout client shared by all backends
//   - keys: BYOK lookup for search-category providers (apikey domain)
//   - mcpRouter: optional port to delegate to a connected MCP search server
//   - log: structured logger for per-tier fall-through traces
//
// WebSearch struct 是 WebSearch 系统工具；持短超时 httpClient、apikey 域的
// BYOK 查询 keys、可选 MCP 路由 mcpRouter、log 用于 per-tier 降级追踪。
type WebSearch struct {
	httpClient *http.Client
	keys       apikeydomain.KeyProvider
	mcpRouter  MCPSearchRouter
	log        *zap.Logger
}

// Identity --------------------------------------------------------------------

func (t *WebSearch) Name() string                { return "WebSearch" }
func (t *WebSearch) Description() string         { return searchDescription }
func (t *WebSearch) Parameters() json.RawMessage { return searchSchema }

// Static metadata -------------------------------------------------------------

func (t *WebSearch) IsReadOnly() bool        { return true }
func (t *WebSearch) NeedsReadFirst() bool    { return false }
func (t *WebSearch) RequiresWorkspace() bool { return false }

// Args-dependent hooks --------------------------------------------------------

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

// ── Execute ───────────────────────────────────────────────────────────────────

// Execute walks the BYOK → MCP routing ladder. Returns the first non-empty
// result list as JSON. Per-tier failures + zero-result responses both
// trigger the next tier; warns are logged for "tried and failed" but not
// for "not configured" cases (the silent BYOK miss is the normal path for
// users who never set a key).
//
// Execute 走 BYOK → MCP 路由阶梯。第一个非空结果作 JSON 返。per-tier 失败
// + 零结果都触发下一层；"试了挂"走 warn log，"未配置"静默（用户从没配 key
// 时这是正常路径）。
func (t *WebSearch) Execute(ctx context.Context, argsJSON string) (string, error) {
	var args searchArgs
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("WebSearch.Execute: %w", err)
	}
	args.normalize()

	// Tier 1: BYOK iterate.
	if t.keys != nil {
		for _, provider := range apikeydomain.SearchProviderPriority {
			if ctx.Err() != nil {
				break
			}
			results, source, ok := t.tryBYOKProvider(ctx, provider, args.Query, args.Limit)
			if ok && len(results) > 0 {
				return marshalSearchResponse(args, source, results)
			}
		}
	}

	// Tier 2: MCP duckduckgo-search.
	if ctx.Err() == nil && t.mcpRouter != nil {
		results, err := t.runMCPSearch(ctx, args.Query, args.Limit)
		switch {
		case errors.Is(err, ErrMCPSearchUnavailable):
			// Silent fall-through — MCP search server simply not installed.
			// 静默降级——MCP 搜索 server 未装。
		case err != nil:
			t.warnf("WebSearch MCP backend failed; falling through", err)
		case len(results) > 0:
			return marshalSearchResponse(args, "mcp", results)
		}
	}

	return fmt.Sprintf("No results for %q. No search backend is currently available.\n\n"+
		"To enable web search, do ONE of the following:\n"+
		"  • Configure a search-category API key in Settings → API Keys "+
		"(Brave / Serper / Tavily — international; Bocha — China). All have free tiers.\n"+
		"  • Install the duckduckgo-search MCP server from the marketplace "+
		"(no API key needed; ~30s install). The user can do this from the MCP tab.\n\n"+
		"(The previous Bing CN HTML scrape fallback was removed because Bing now "+
		"renders results via JavaScript, making server-side HTML scraping return 0 results.)",
		args.Query), nil
}

// tryBYOKProvider attempts one BYOK search call. Returns (results, source,
// true) on success; (nil, "", false) when the provider has no configured
// key OR the call failed (latter logged at warn). source is the provider
// name on success so the response payload tells the LLM which backend
// produced the results.
//
// tryBYOKProvider 试调一个 BYOK 搜索。成功 (results, source, true)；provider
// 无 key 或调用失败 (nil, "", false)；后者 warn log。source 是成功时的 provider
// 名让响应载荷告诉 LLM 是哪个后端给的结果。
func (t *WebSearch) tryBYOKProvider(ctx context.Context, provider, query string, limit int) ([]searchResult, string, bool) {
	creds, err := t.keys.ResolveCredentials(ctx, provider)
	if err != nil {
		// ErrNotFoundForProvider IS the silent path — user just hasn't
		// configured this provider, fall through to next tier without
		// noise. Other errors (decryption fail, store fail, ctx canceled)
		// are NOT user-recoverable from "no key configured" — they
		// indicate something concrete is broken (corrupt encryption /
		// DB issue). Log loudly so operator sees the difference.
		// Same defect-class lesson as B2 bash auto-route silent fallback.
		//
		// ErrNotFoundForProvider 是静默路径——用户没配，无声降级到下层。
		// 其他错误（解密失败、store 失败、ctx 取消）不是"没配"——是有真问
		// 题。高声 log 让 operator 看出区别。同 B2 bash auto-route 经验。
		if !errors.Is(err, apikeydomain.ErrNotFoundForProvider) {
			t.warnf(fmt.Sprintf("WebSearch BYOK %q ResolveCredentials failed (not 'no key configured')", provider), err)
		}
		return nil, "", false
	}

	baseURL := strings.TrimRight(creds.BaseURL, "/")
	if baseURL == "" {
		// Defensive — meta.DefaultBaseURL should be merged by the
		// keyProvider. Fall through.
		// 防御——meta.DefaultBaseURL 应由 keyProvider 合并。降级。
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
		// Defensive — providers list and switch must stay in sync.
		// 防御——providers 列表与 switch 必须同步。
		return nil, "", false
	}
	if runErr != nil {
		t.warnf(fmt.Sprintf("WebSearch BYOK %q failed; falling through", provider), runErr)
		// Surface 401/403 to apikey domain so the UI badge flips invalid.
		// 把 401/403 通知 apikey 域让 UI 徽章翻 invalid。
		t.markInvalidIfAuthErr(ctx, provider, runErr)
		return nil, "", false
	}
	return results, provider, true
}

// markInvalidIfAuthErr surfaces 401/403 errors from BYOK calls back to the
// apikey domain so the UI status flips. Best-effort: failure to mark
// is logged at debug only.
//
// markInvalidIfAuthErr 把 BYOK 401/403 通知 apikey 域让 UI 状态翻转。
// best-effort：marker 失败 debug log。
func (t *WebSearch) markInvalidIfAuthErr(ctx context.Context, provider string, err error) {
	// errors.Is(ErrAuthFailed) catches both 401 + 403 (sentinel covers
	// both). Replaced fragile string match `Contains(msg, "HTTP 401")`
	// — see audit doc app-tool-web/search.go.md site #12.
	// errors.Is(ErrAuthFailed) 同时捕获 401 + 403（sentinel 覆盖两者）。
	// 替代脆 substring 匹配——见 audit doc。
	if !errors.Is(err, ErrAuthFailed) {
		return
	}
	if t.keys == nil {
		return
	}
	// MarkInvalid expects ctx with userID; reqctx middleware always stamps
	// it for HTTP-driven calls. detached context retains the user ID so
	// background invocations work too.
	// MarkInvalid 期望 ctx 含 userID；HTTP 路径走 middleware；detached ctx 留
	// userID 让后台调用也能 mark。
	uid, _ := reqctxpkg.GetUserID(ctx)
	mctx := ctx
	if uid != "" {
		mctx = reqctxpkg.SetUserID(context.Background(), uid)
	}
	if merr := t.keys.MarkInvalid(mctx, provider, err.Error()); merr != nil {
		t.debugf(fmt.Sprintf("MarkInvalid for %q failed", provider), merr)
	}
}

// warnf logs at warn level when t.log is non-nil; nil log = silent (tests).
//
// warnf 当 t.log 非空时 warn log；nil log 静默（测试）。
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

// marshalSearchResponse caps to args.Limit, sets the truncated flag, and
// JSON-encodes the response.
//
// marshalSearchResponse 截到 args.Limit、置 truncated、JSON 编码。
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

// ── Compile-time checks ───────────────────────────────────────────────────────

var _ toolapp.Tool = (*WebSearch)(nil)
