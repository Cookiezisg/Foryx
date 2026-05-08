// search_marketplace.go — search_mcp_marketplace system tool: discovers
// installable MCP servers in the official MCP Registry. LLM-reranked,
// no alpha-order fallback (consistent with forge.search and the post-
// 2026-05-08 mcp.calltool::Search fix). Pairs with install_mcp_server +
// uninstall_mcp_server for the full LLM-driven marketplace flow.
//
// search_marketplace.go ——search_mcp_marketplace 系统工具：发现可装的
// MCP server。LLM 重排，无字母序 fallback（与 forge.search + 2026-05-08
// 后 mcp.calltool::Search 修复一致）。与 install_mcp_server + uninstall_mcp_server
// 配对完成完整 LLM 驱动 marketplace 流程。
package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	mcpapp "github.com/sunweilin/forgify/backend/internal/app/mcp"
	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
	apikeydomain "github.com/sunweilin/forgify/backend/internal/domain/apikey"
	mcpdomain "github.com/sunweilin/forgify/backend/internal/domain/mcp"
	modeldomain "github.com/sunweilin/forgify/backend/internal/domain/model"
	llminfra "github.com/sunweilin/forgify/backend/internal/infra/llm"
	llmclientpkg "github.com/sunweilin/forgify/backend/internal/pkg/llmclient"
	llmparsepkg "github.com/sunweilin/forgify/backend/internal/pkg/llmparse"
)

// SearchMarketplaceMCP implements the search_mcp_marketplace system tool.
//
// SearchMarketplaceMCP 实现 search_mcp_marketplace 系统工具。
type SearchMarketplaceMCP struct {
	svc     *mcpapp.Service
	picker  modeldomain.ModelPicker
	keys    apikeydomain.KeyProvider
	factory *llminfra.Factory
}

const searchMarketplaceDescription = `Search Forgify's curated MCP marketplace for installable servers. Use this when an MCP capability is needed but no matching server is currently installed (search_mcp_tools returned nothing).

The catalog is HAND-PICKED — about 21 servers verified to install and run out of the box. Categories: browser (playwright, chrome-devtools), web-data (firecrawl, duckduckgo, tavily), code (context7, github, gitlab, sentry), database (dbhub, mongodb, supabase), project-mgmt (linear, jira+confluence), docs (notion, slack, figma, memory), email (gmail, ms365), sandbox (e2b).

Each result includes:
  - name: canonical id used by install_mcp_server
  - description, runtime (node/python), homepage
  - tier: 0=zero-config, 1=one API key, 2=OAuth device-code flow, 3=DB connection string
  - requiredEnv / requiredArgs: values to supply at install (each env carries a setupURL — pass that link to the user so they can grab the key)
  - notes: first-run gotchas (chromium downloads, Notion sharing rituals, OAuth flow expectations) — surface these to the user when proposing the install

After choosing a server, call install_mcp_server({name}) to begin the install flow. The first call returns "needs_confirmation" with details so you can use the ask tool to confirm with the user before the second call (with confirmed=true) actually installs.`

var searchMarketplaceSchema = json.RawMessage(`{
	"type": "object",
	"properties": {
		"query": {"type": "string", "description": "Natural-language description of the capability you need (e.g. 'web search no API key', 'PDF text extraction')."},
		"limit": {"type": "integer", "description": "Maximum results to return (default 5, max 20)."}
	},
	"required": ["query"]
}`)

// Identity --------------------------------------------------------------------
func (t *SearchMarketplaceMCP) Name() string                { return "search_mcp_marketplace" }
func (t *SearchMarketplaceMCP) Description() string         { return searchMarketplaceDescription }
func (t *SearchMarketplaceMCP) Parameters() json.RawMessage { return searchMarketplaceSchema }

// Static metadata -------------------------------------------------------------
func (t *SearchMarketplaceMCP) IsReadOnly() bool        { return true }
func (t *SearchMarketplaceMCP) NeedsReadFirst() bool    { return false }
func (t *SearchMarketplaceMCP) RequiresWorkspace() bool { return false }

// Args-dependent hooks --------------------------------------------------------
func (t *SearchMarketplaceMCP) ValidateInput(args json.RawMessage) error {
	var a struct {
		Query string `json:"query"`
		Limit int    `json:"limit"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return fmt.Errorf("search_mcp_marketplace: bad args: %w", err)
	}
	if strings.TrimSpace(a.Query) == "" {
		return errors.New("search_mcp_marketplace: query is required")
	}
	if a.Limit < 0 {
		return errors.New("search_mcp_marketplace: limit must be non-negative")
	}
	return nil
}

func (t *SearchMarketplaceMCP) CheckPermissions(json.RawMessage, toolapp.PermissionMode) toolapp.PermissionResult {
	return toolapp.PermissionAllow
}

// ── Execute ───────────────────────────────────────────────────────────

func (t *SearchMarketplaceMCP) Execute(ctx context.Context, argsJSON string) (string, error) {
	var args struct {
		Query string `json:"query"`
		Limit int    `json:"limit"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("search_mcp_marketplace: %w", err)
	}
	if args.Limit <= 0 {
		args.Limit = 5
	}
	if args.Limit > 20 {
		args.Limit = 20
	}

	all, err := t.svc.SearchRegistry(ctx, args.Query)
	if err != nil {
		// Surface ErrMarketplaceUnavailable as actionable text so LLM can
		// tell the user. Other errors bubble up too.
		// 把 ErrMarketplaceUnavailable 当 actionable 文本返让 LLM 告诉用户。
		// 其他错也冒泡。
		if errors.Is(err, mcpdomain.ErrMarketplaceUnavailable) {
			return fmt.Sprintf("Marketplace unavailable: %v. The official MCP registry is unreachable. The user can configure a search-category API key (Brave / Serper / Tavily / Bocha) for web search instead, or retry later.", err), nil
		}
		return "", fmt.Errorf("search_mcp_marketplace: search: %w", err)
	}
	if len(all) == 0 {
		out, _ := json.Marshal([]any{})
		return string(out), nil
	}

	// Short-circuit: total ≤ limit → return all (skip LLM call).
	// 短路：总数 ≤ limit → 全返（跳过 LLM 调用）。
	if len(all) <= args.Limit {
		return marshalMarketplaceResults(all), nil
	}

	// LLM rerank. Each candidate gets one numbered line: "<idx>. <name> — <desc snippet>".
	// LLM 重排。每候选一行编号："<idx>. <name> — <desc 片段>"。
	var sb strings.Builder
	fmt.Fprintf(&sb, "Query: %s\n\nMarketplace candidates:\n", args.Query)
	for i, e := range all {
		desc := e.Description
		if len(desc) > 200 {
			desc = desc[:200] + "..."
		}
		fmt.Fprintf(&sb, "%d. %s — %s\n", i, e.Name, desc)
	}
	fmt.Fprintf(&sb, "\nReturn the indices of the %d most relevant candidates as a JSON array, "+
		"most relevant first: [3, 7, 1, ...]\n"+
		"Respond with valid JSON only, no surrounding prose.", args.Limit)

	bundle, err := llmclientpkg.Resolve(ctx, t.picker, t.keys, t.factory)
	if err != nil {
		return "", fmt.Errorf("search_mcp_marketplace: resolve LLM: %w", err)
	}
	resp, err := llminfra.Generate(ctx, bundle.Client, llminfra.Request{
		ModelID:  bundle.ModelID,
		Key:      bundle.Key,
		BaseURL:  bundle.BaseURL,
		Messages: []llminfra.LLMMessage{{Role: llminfra.RoleUser, Content: sb.String()}},
	})
	if err != nil {
		return "", fmt.Errorf("search_mcp_marketplace: llm: %w", err)
	}

	jsonStr, ok := llmparsepkg.ExtractJSON(resp)
	if !ok {
		return "", fmt.Errorf("search_mcp_marketplace: LLM response contained no JSON; retry with a more specific query")
	}
	var indices []int
	if err := json.Unmarshal([]byte(jsonStr), &indices); err != nil {
		return "", fmt.Errorf("search_mcp_marketplace: parse ranking: %w; retry with a more specific query", err)
	}

	out := make([]mcpdomain.RegistryEntry, 0, args.Limit)
	for _, idx := range indices {
		if idx < 0 || idx >= len(all) {
			continue
		}
		out = append(out, all[idx])
		if len(out) >= args.Limit {
			break
		}
	}
	if len(out) == 0 {
		return "", fmt.Errorf("search_mcp_marketplace: ranking produced no valid indices; retry with a more specific query")
	}
	return marshalMarketplaceResults(out), nil
}

// marshalMarketplaceResults renders a slice of RegistryEntry as the JSON
// shape the LLM consumes — slimmer than the full RegistryEntry to avoid
// burning tokens on InstallCmd internals. LLM only needs name/description/
// version/runtime/homepage + the user-supplied requirements.
//
// marshalMarketplaceResults 把 RegistryEntry 切片渲染成 LLM 消费的 JSON ——
// 比完整 RegistryEntry 瘦避免在 InstallCmd 内部细节烧 token。LLM 只需
// name/description/version/runtime/homepage + 用户必填项。
func marshalMarketplaceResults(entries []mcpdomain.RegistryEntry) string {
	type result struct {
		Name         string                     `json:"name"`
		Description  string                     `json:"description"`
		Category     string                     `json:"category,omitempty"`
		Tier         int                        `json:"tier"`
		Runtime      string                     `json:"runtime"`
		Homepage     string                     `json:"homepage,omitempty"`
		RequiredEnv  []mcpdomain.EnvRequirement `json:"requiredEnv,omitempty"`
		RequiredArgs []mcpdomain.ArgRequirement `json:"requiredArgs,omitempty"`
		Notes        string                     `json:"notes,omitempty"`
	}
	out := make([]result, 0, len(entries))
	for _, e := range entries {
		out = append(out, result{
			Name:         e.Name,
			Description:  e.Description,
			Category:     e.Category,
			Tier:         e.Tier,
			Runtime:      e.Runtime,
			Homepage:     e.Homepage,
			RequiredEnv:  e.RequiredEnv,
			RequiredArgs: e.RequiredArgs,
			Notes:        e.Notes,
		})
	}
	b, _ := json.Marshal(out)
	return string(b)
}
