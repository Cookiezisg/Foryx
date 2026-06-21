package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	mcpapp "github.com/sunweilin/anselm/backend/internal/app/mcp"
	toolapp "github.com/sunweilin/anselm/backend/internal/app/tool"
	mcpdomain "github.com/sunweilin/anselm/backend/internal/domain/mcp"
)

// --- list_mcp_marketplace --------------------------------------------------

// ListMarketplace lets the LLM browse the GitHub MCP Registry. Returns each installable
// server's name + description + runtime kind + required env (so the LLM knows what to supply).
//
// ListMarketplace 让 LLM 逛 GitHub MCP Registry。返回每个可装 server 的 名 + 描述 + runtime 类型
// + 必填 env（使 LLM 知道要提供什么）。
type ListMarketplace struct{ svc *mcpapp.Service }

func (t *ListMarketplace) Name() string { return "list_mcp_marketplace" }
func (t *ListMarketplace) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"query":{"type":"string","description":"Optional case-insensitive capability filter on server name + description. Multiple words are matched independently and the results are RANKED by how many of your words a server matches — a server matching ALL your words ranks first, but servers matching only some still appear (so \"fetch web page\" won't return empty just because no server contains all three words). Phrase it as a capability (\"send notification\", \"database query\"). STRONGLY prefer it when you want a specific capability — the catalog is large and an unfiltered list dumps every server. Omit to browse everything."}}}`)
}
func (t *ListMarketplace) ValidateInput(json.RawMessage) error { return nil }
func (t *ListMarketplace) Description() string {
	return "Browse the MCP server marketplace (the GitHub MCP Registry). Returns installable servers — each with its full name, description, runtime, and its `env` vars. Each env var carries a `required` flag: required:true MUST be supplied or the server won't start; required:false is OPTIONAL (the server runs without it) — do NOT tell the user an optional var is mandatory. Pass a `query` to filter by capability/name (preferred — the catalog is large); omit it to list everything. To install one, call install_mcp_server with its name."
}

type marketView struct {
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Runtime     string    `json:"runtime"`       // node|python|docker|dotnet|remote
	Env         []envView `json:"env,omitempty"` // the server's env vars; each carries its OWN required flag
}

type envView struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	// Required preserves the registry's per-var required/optional distinction — the field used to be
	// erased (every env var projected under a `requiredEnv` key), so the agent could not tell a mandatory
	// key from an optional one and falsely told users an OPTIONAL key was mandatory (F169).
	// Required 保留 registry 每个变量的必填/可选区分——此前被抹（所有 env var 都塞进 `requiredEnv` 键），故 agent
	// 分不清必填与可选、错告用户**可选**键是必填（F169）。
	Required bool `json:"required"`
}

func (t *ListMarketplace) Execute(ctx context.Context, argsJSON string) (string, error) {
	// query is an optional capability filter — the registry is large, so an unfiltered dump bloats the
	// agent's context (a single call returned ~96 servers when it only needed a notifier).
	//
	// query 是可选能力过滤——registry 很大，无过滤倾倒会撑爆 agent 上下文（一次调用返 ~96 个 server，而它只要一个通知器）。
	var a struct {
		Query string `json:"query"`
	}
	if strings.TrimSpace(argsJSON) != "" {
		_ = json.Unmarshal([]byte(argsJSON), &a) // optional arg; a parse error just means no filter
	}
	q := strings.ToLower(strings.TrimSpace(a.Query))

	entries, err := t.svc.ListRegistry(ctx)
	if err != nil {
		return "", fmt.Errorf("list_mcp_marketplace: %w", err)
	}
	views := filterMarketViews(entries, q)
	return toolapp.ToJSON(map[string]any{"servers": views, "count": len(views)}), nil
}

// filterMarketViews turns installable registry entries into views, hiding the un-installable
// (Plan ok=false) and, when q (already lowercased) is non-empty, those whose name+description miss
// any query WORD (whitespace-split, AND-matched). Pure so the capability filter is unit-testable
// without a live registry.
//
// filterMarketViews 把可装 registry 条目转成 view，隐去不可装的（Plan ok=false）；q（已小写）非空时
// 再隐去 name+description 缺任一查询词（按空白切、逐词 AND）的。纯函数，使能力过滤无需 live registry 即可单测。
func filterMarketViews(entries []mcpdomain.RegistryEntry, q string) []marketView {
	// Whitespace-split + OR-match RANKED BY MATCH COUNT: a server matching MORE query words ranks
	// higher, and any server matching ≥1 word is included. AND-of-all-tokens (F91) was too strict —
	// a natural capability phrase ("fetch web page", "send notification") has a word no server's
	// name+description literally contains, so AND silently returned 0 (F113-andtoostrict). OR-ranked
	// keeps the all-words match on top while still surfacing the relevant partial matches. Empty
	// query → no tokens → all servers, registry order.
	// 按空白切词 + OR 匹配、**按命中词数排序**：命中更多查询词的 server 排更前、命中 ≥1 词的都纳入。AND（F91）
	// 太严——自然能力短语总有某词不在任何 server 的 name+description 里、AND 静默返 0。OR-排序让全词命中仍居顶、
	// 同时浮出相关的部分命中。空查询→无 token→全部 server、registry 序。
	tokens := strings.Fields(q)
	type scoredView struct {
		view  marketView
		score int
	}
	scored := make([]scoredView, 0, len(entries))
	for _, e := range entries {
		plan, ok := e.Plan()
		if !ok {
			continue // unsupported runtime + no remote → can't install, hide it
		}
		score := tokenMatchCount(e.Name, e.Description, tokens)
		if len(tokens) > 0 && score == 0 {
			continue // matches no query word — exclude
		}
		v := marketView{Name: e.Name, Description: e.Description, Runtime: plan.Runtime}
		if plan.Remote {
			v.Runtime = "remote"
		}
		for _, ev := range plan.EnvVars {
			v.Env = append(v.Env, envView{Name: ev.Name, Description: ev.Description, Required: ev.Required})
		}
		scored = append(scored, scoredView{view: v, score: score})
	}
	// More matched words first; stable so equal-score servers keep registry order.
	sort.SliceStable(scored, func(i, j int) bool { return scored[i].score > scored[j].score })
	views := make([]marketView, len(scored))
	for i, s := range scored {
		views[i] = s.view
	}
	return views
}

// tokenMatchCount returns how many of tokens appear (case-insensitive substring) in name+description.
// No tokens (empty query) → 0 (the caller treats an empty query as "match everything").
//
// tokenMatchCount 返回 tokens 中有几个出现在 name+description 合串里（大小写不敏感子串）。无 token（空查询）
// →0（调用方把空查询当"全匹配"）。
func tokenMatchCount(name, description string, tokens []string) int {
	if len(tokens) == 0 {
		return 0
	}
	hay := strings.ToLower(name + " " + description)
	n := 0
	for _, tok := range tokens {
		if strings.Contains(hay, tok) {
			n++
		}
	}
	return n
}

// --- install_mcp_server ----------------------------------------------------

// InstallServer installs a server from the marketplace and connects it. Returns the live
// status + the tools it now exposes (so the LLM can immediately search_tools for them).
//
// InstallServer 从市场装一个 server 并连接。返回实时状态 + 它暴露的工具（使 LLM 可立即 search_tools）。
type InstallServer struct{ svc *mcpapp.Service }

func (t *InstallServer) Name() string { return "install_mcp_server" }
func (t *InstallServer) Description() string {
	return "Install an MCP server from the marketplace by its full name (from list_mcp_marketplace), supplying any required environment variables (API keys). On success the server's tools become available — find them with search_tools. By product design Anselm connects MARKETPLACE (registry) servers ONLY — there is no custom self-hosted server (a local stdio command or a private SSE/HTTP url). If a user asks to connect their own server, explain that only servers from the marketplace catalog are supported."
}
func (t *InstallServer) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"required": ["name"],
		"properties": {
			"name": {"type": "string", "description": "Full registry name from list_mcp_marketplace (e.g. io.github.upstash/context7)."},
			"env": {"type": "object", "description": "Required environment variables (API keys etc.) as name→value.", "additionalProperties": {"type": "string"}}
		}
	}`)
}

type installArgs struct {
	Name string            `json:"name"`
	Env  map[string]string `json:"env"`
}

func (t *InstallServer) ValidateInput(args json.RawMessage) error {
	var a installArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return fmt.Errorf("install_mcp_server: %w", err)
	}
	if strings.TrimSpace(a.Name) == "" {
		return ErrNameRequired
	}
	return nil
}

func (t *InstallServer) Execute(ctx context.Context, argsJSON string) (string, error) {
	var a installArgs
	if err := json.Unmarshal([]byte(argsJSON), &a); err != nil {
		return "", fmt.Errorf("install_mcp_server: %w", err)
	}
	status, err := t.svc.InstallFromRegistry(ctx, a.Name, a.Env)
	if err != nil {
		return "", err
	}
	return toolapp.ToJSON(status), nil
}

// --- uninstall_mcp_server --------------------------------------------------

// UninstallServer removes an installed server by name (stops it + deletes its config).
//
// UninstallServer 按 name 移除已装 server（停进程 + 删 config）。
type UninstallServer struct{ svc *mcpapp.Service }

func (t *UninstallServer) Name() string { return "uninstall_mcp_server" }
func (t *UninstallServer) Description() string {
	return "Uninstall an MCP server by name: stop its process and delete its configuration. Its tools become unavailable."
}
func (t *UninstallServer) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object","required":["name"],"properties":{"name":{"type":"string","description":"Installed server name (e.g. context7)."}}}`)
}

type nameArg struct {
	Name string `json:"name"`
}

func (t *UninstallServer) ValidateInput(args json.RawMessage) error { return requireName(args) }

func (t *UninstallServer) Execute(ctx context.Context, argsJSON string) (string, error) {
	var a nameArg
	if err := json.Unmarshal([]byte(argsJSON), &a); err != nil {
		return "", fmt.Errorf("uninstall_mcp_server: %w", err)
	}
	if err := t.svc.RemoveServer(ctx, a.Name); err != nil {
		return "", err
	}
	return fmt.Sprintf("Uninstalled MCP server %q.", a.Name), nil
}

// --- reconnect_mcp ---------------------------------------------------------

// ReconnectMCP restarts a server's connection — the reset button for a server that's
// connected but misbehaving (stale session / hung). Mirrors restart_handler.
//
// ReconnectMCP 重启一个 server 的连接——「重置按钮」，救连着但状态坏了的 server（stale session /
// 卡住）。镜像 restart_handler。
type ReconnectMCP struct{ svc *mcpapp.Service }

func (t *ReconnectMCP) Name() string { return "reconnect_mcp" }
func (t *ReconnectMCP) Description() string {
	return "Restart an installed MCP server's connection — the reset button for a server that's connected but misbehaving (stale session, hung). Returns the refreshed status."
}
func (t *ReconnectMCP) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object","required":["name"],"properties":{"name":{"type":"string","description":"Installed server name to reconnect."}}}`)
}

func (t *ReconnectMCP) ValidateInput(args json.RawMessage) error { return requireName(args) }

func (t *ReconnectMCP) Execute(ctx context.Context, argsJSON string) (string, error) {
	var a nameArg
	if err := json.Unmarshal([]byte(argsJSON), &a); err != nil {
		return "", fmt.Errorf("reconnect_mcp: %w", err)
	}
	status, err := t.svc.Reconnect(ctx, a.Name)
	if err != nil {
		return "", err
	}
	return toolapp.ToJSON(status), nil
}

func requireName(args json.RawMessage) error {
	var a nameArg
	if err := json.Unmarshal(args, &a); err != nil {
		return fmt.Errorf("tool/mcp: %w", err)
	}
	if strings.TrimSpace(a.Name) == "" {
		return fmt.Errorf("tool/mcp: name is required")
	}
	return nil
}
