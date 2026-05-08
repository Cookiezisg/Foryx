// curated_registry.go — production RegistrySource backed by a
// hand-picked, hardcoded list of 21 high-value MCP servers. Replaces
// the "fetch the entire upstream registry" approach (5000+ entries of
// mostly broken / API-key-required / abandoned servers) with a
// curated catalog where every entry has been verified to install + run
// out of the box.
//
// Curation criteria (all entries must satisfy):
//   - Runtime is npm or pypi (no docker / go / rust / java).
//   - Either zero-config, or one API key with a clear free-tier signup,
//     or OAuth device-code flow handled by the package itself.
//   - Listed on a major registry, actively maintained.
//   - Either an official vendor server or widely-recognised third-party.
//
// Each entry's Name is a short kebab-case slug we choose — used as the
// LLM's install lookup id AND the mcp.json key (no separate alias). The
// real npm/pypi package identifier lives in InstallCmd.Args.
//
// curated_registry.go ——硬编码 21 条精选 MCP 的 RegistrySource。每条 Name
// 是我们写好的短 slug——同时是 LLM install lookup id 和 mcp.json key（无
// 独立 alias 概念）；真包名留在 InstallCmd.Args。
package mcp

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"

	mcpdomain "github.com/sunweilin/forgify/backend/internal/domain/mcp"
)

// CuratedRegistrySource implements mcpdomain.RegistrySource against the
// hardcoded curated catalog (see curatedEntries below).
//
// CuratedRegistrySource 用 hardcoded curated 目录实现 RegistrySource。
type CuratedRegistrySource struct {
	mu     sync.RWMutex
	byName map[string]mcpdomain.RegistryEntry
	all    []mcpdomain.RegistryEntry
}

// NewCuratedRegistrySource constructs the source. No I/O, never fails.
//
// NewCuratedRegistrySource 构造 source。无 I/O，永不失败。
func NewCuratedRegistrySource() *CuratedRegistrySource {
	src := &CuratedRegistrySource{
		byName: make(map[string]mcpdomain.RegistryEntry, len(curatedEntries)),
		all:    make([]mcpdomain.RegistryEntry, 0, len(curatedEntries)),
	}
	for _, e := range curatedEntries {
		src.byName[e.Name] = e
		src.all = append(src.all, e)
	}
	// Stable sort by tier then name so Search results have predictable
	// "easiest first" ordering when relevance ties.
	//
	// 按 tier+name 稳排，让 Search tied 时按"易上手优先"返。
	sort.Slice(src.all, func(i, j int) bool {
		if src.all[i].Tier != src.all[j].Tier {
			return src.all[i].Tier < src.all[j].Tier
		}
		return src.all[i].Name < src.all[j].Name
	})
	return src
}

// Search returns entries whose name / description / category / notes
// contains EVERY whitespace-separated token in query (AND match,
// case-insensitive). Empty query returns ErrQueryRequired.
//
// Search 返 name / description / category / notes 上 AND 匹配每个空白拆词
// 条目。空 query 返 ErrQueryRequired。
func (c *CuratedRegistrySource) Search(_ context.Context, query string) ([]mcpdomain.RegistryEntry, error) {
	tokens := tokenizeLower(query)
	if len(tokens) == 0 {
		return nil, mcpdomain.ErrQueryRequired
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	var out []mcpdomain.RegistryEntry
	for _, e := range c.all {
		hay := strings.ToLower(strings.Join([]string{
			e.Name, e.Description, e.Category, e.Notes,
		}, " "))
		matched := true
		for _, t := range tokens {
			if !strings.Contains(hay, t) {
				matched = false
				break
			}
		}
		if matched {
			out = append(out, e)
		}
	}
	return out, nil
}

// Get returns the entry matching name; ErrRegistryEntryNotFound when
// the name isn't in the curated list.
//
// Get 返 name 对应条目；不在 curated 列表返 ErrRegistryEntryNotFound。
func (c *CuratedRegistrySource) Get(_ context.Context, name string) (*mcpdomain.RegistryEntry, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	e, ok := c.byName[name]
	if !ok {
		return nil, fmt.Errorf("curated: %w: %q", mcpdomain.ErrRegistryEntryNotFound, name)
	}
	cp := e
	return &cp, nil
}

func tokenizeLower(query string) []string {
	var out []string
	for _, t := range strings.Fields(strings.ToLower(query)) {
		t = strings.TrimSpace(t)
		if t != "" {
			out = append(out, t)
		}
	}
	return out
}

// Compile-time check.
//
// 编译期检查。
var _ mcpdomain.RegistrySource = (*CuratedRegistrySource)(nil)

// ── Curated entries ──────────────────────────────────────────────────
//
// Order doesn't matter (Search re-sorts). Group by category for human
// review. Every entry MUST set Name / Description / Runtime / InstallCmd
// / Category / Tier. SetupURL on env requirements is mandatory when
// Tier >= 1. Notes is mandatory whenever there's a non-obvious gotcha
// (first-run download, OAuth flow, version dep).
//
// 顺序无所谓（Search 自己排）。按 category 人读分组。每条必填 Name /
// Description / Runtime / InstallCmd / Category / Tier。Tier >= 1 的 env
// 必带 SetupURL。有"陷阱"必填 Notes。

var curatedEntries = []mcpdomain.RegistryEntry{
	// ── Browser & web automation ─────────────────────────────────
	{
		Name:        "playwright",
		Description: "Browser automation: click, fill forms, screenshot, scrape JS-rendered pages. Microsoft official, the de-facto standard MCP for browser control.",
		Homepage:    "https://github.com/microsoft/playwright-mcp",
		Runtime:     "node",
		InstallCmd:  mcpdomain.InstallCmd{Command: "npx", Args: []string{"-y", "@playwright/mcp@latest"}},
		Category:    "browser",
		Tier:        0,
		Notes:       "First run downloads Chromium (~150MB). The first tool call may take 30s+ while Playwright sets up; subsequent calls are fast. No API key, no signup.",
	},
	{
		Name:        "chrome-devtools",
		Description: "Drive Chrome via DevTools Protocol: capture network requests, run performance traces, inspect console logs, debug live pages.",
		Homepage:    "https://github.com/ChromeDevTools/chrome-devtools-mcp",
		Runtime:     "node",
		InstallCmd:  mcpdomain.InstallCmd{Command: "npx", Args: []string{"-y", "chrome-devtools-mcp@latest"}},
		Category:    "browser",
		Tier:        0,
		Notes:       "Requires Chrome (any version) installed locally. The MCP attaches to your existing Chrome via DevTools Protocol; if Chrome isn't running it spawns one.",
	},
	{
		Name:        "duckduckgo",
		Description: "Web search + content fetching via DuckDuckGo. Zero API key required.",
		Homepage:    "https://github.com/nickclyde/duckduckgo-mcp-server",
		Runtime:     "python",
		InstallCmd:  mcpdomain.InstallCmd{Command: "uvx", Args: []string{"duckduckgo-mcp-server"}},
		Category:    "web-data",
		Tier:        0,
		Notes:       "Includes both search and webpage content fetch tools. Subject to DuckDuckGo's rate limits — for heavy use prefer brave or tavily.",
	},
	{
		Name:        "tavily",
		Description: "Web search optimised for LLM context — concise, deduplicated results suitable for direct ingestion.",
		Homepage:    "https://docs.tavily.com/docs/integrations/mcp",
		Runtime:     "node",
		InstallCmd:  mcpdomain.InstallCmd{Command: "npx", Args: []string{"-y", "tavily-mcp"}},
		Category:    "web-data",
		Tier:        1,
		RequiredEnv: []mcpdomain.EnvRequirement{
			{Name: "TAVILY_API_KEY", Description: "Tavily API key.", SetupURL: "https://app.tavily.com/home", Secret: true},
		},
		Notes: "Free tier: 1000 searches / month, no credit card required.",
	},
	{
		Name:        "firecrawl",
		Description: "Scrape, crawl, and extract structured data from websites; supports persistent browser sessions and AI-driven content extraction.",
		Homepage:    "https://docs.firecrawl.dev/mcp-server",
		Runtime:     "node",
		InstallCmd:  mcpdomain.InstallCmd{Command: "npx", Args: []string{"-y", "firecrawl-mcp"}},
		Category:    "web-data",
		Tier:        1,
		RequiredEnv: []mcpdomain.EnvRequirement{
			{Name: "FIRECRAWL_API_KEY", Description: "Firecrawl API key.", SetupURL: "https://www.firecrawl.dev/app/api-keys", Secret: true},
		},
		Notes: "Free tier: 500 credits (each scrape costs 1, each crawl costs 1 per page). Heavy crawling burns credits fast.",
	},

	// ── Code, version control, error tracking ────────────────────
	{
		Name:        "context7",
		Description: "Fetch the latest documentation for any library by name (e.g. 'show me current React docs'). Solves the LLM-stuck-on-old-training-data problem.",
		Homepage:    "https://github.com/upstash/context7",
		Runtime:     "node",
		InstallCmd:  mcpdomain.InstallCmd{Command: "npx", Args: []string{"-y", "@upstash/context7-mcp"}},
		Category:    "code",
		Tier:        0,
		Notes:       "Coding-workflow staple — pairs well with playwright for verifying generated code against current API. No key.",
	},
	{
		Name:        "github",
		Description: "GitHub via REST API: issues, PRs, code search, commits, releases, file contents.",
		Homepage:    "https://github.com/modelcontextprotocol/servers/tree/main/src/github",
		Runtime:     "node",
		InstallCmd:  mcpdomain.InstallCmd{Command: "npx", Args: []string{"-y", "@modelcontextprotocol/server-github"}},
		Category:    "vcs",
		Tier:        1,
		RequiredEnv: []mcpdomain.EnvRequirement{
			{Name: "GITHUB_PERSONAL_ACCESS_TOKEN", Description: "GitHub Personal Access Token (classic or fine-grained).", SetupURL: "https://github.com/settings/tokens", Secret: true},
		},
		Notes: "Use a fine-grained token scoped to the repos you want to expose. Public-repo-only access works without granting any repo scopes.",
	},
	{
		Name:        "gitlab",
		Description: "GitLab full coverage: projects, merge requests, issues, pipelines, wiki, releases, branches.",
		Homepage:    "https://github.com/zereight/gitlab-mcp",
		Runtime:     "node",
		InstallCmd:  mcpdomain.InstallCmd{Command: "npx", Args: []string{"-y", "@zereight/mcp-gitlab"}},
		Category:    "vcs",
		Tier:        1,
		RequiredEnv: []mcpdomain.EnvRequirement{
			{Name: "GITLAB_PERSONAL_ACCESS_TOKEN", Description: "GitLab Personal Access Token (api scope).", SetupURL: "https://gitlab.com/-/user_settings/personal_access_tokens", Secret: true},
			{Name: "GITLAB_API_URL", Description: "GitLab instance API base URL. Default https://gitlab.com/api/v4 — only set this for self-hosted GitLab.", SetupURL: "https://docs.gitlab.com/ee/api/", Secret: false},
		},
		Notes: "For gitlab.com leave GITLAB_API_URL as default. For self-hosted set to https://your-gitlab/api/v4.",
	},
	{
		Name:        "sentry",
		Description: "Sentry official: AI-powered natural-language search across events / issues, plus issue management and trace inspection.",
		Homepage:    "https://github.com/getsentry/sentry-mcp",
		Runtime:     "node",
		InstallCmd:  mcpdomain.InstallCmd{Command: "npx", Args: []string{"-y", "@sentry/mcp-server"}},
		Category:    "error-tracking",
		Tier:        1,
		RequiredEnv: []mcpdomain.EnvRequirement{
			{Name: "SENTRY_AUTH_TOKEN", Description: "Sentry user auth token (org:read, project:read, event:read scopes).", SetupURL: "https://sentry.io/settings/account/api/auth-tokens/", Secret: true},
			{Name: "SENTRY_HOST", Description: "Sentry host (default sentry.io). Set to your-org.sentry.io for self-hosted.", SetupURL: "https://docs.sentry.io/", Secret: false},
		},
		Notes: "AI-powered search tools (search_events / search_issues) require an LLM provider configured on the Sentry side. Non-search tools work without it.",
	},

	// ── Databases ────────────────────────────────────────────────
	{
		Name:        "dbhub",
		Description: "Token-efficient database MCP supporting PostgreSQL, MySQL, SQL Server, SQLite, and MariaDB through a single server.",
		Homepage:    "https://github.com/bytebase/dbhub",
		Runtime:     "node",
		InstallCmd:  mcpdomain.InstallCmd{Command: "npx", Args: []string{"-y", "@bytebase/dbhub"}},
		Category:    "database",
		Tier:        3,
		RequiredEnv: []mcpdomain.EnvRequirement{
			{Name: "DSN", Description: "Database connection string. Examples: postgres://user:pw@host:5432/db | mysql://user:pw@host:3306/db | sqlite:///path/to/file.db", SetupURL: "https://github.com/bytebase/dbhub#configuration", Secret: true},
		},
		Notes: "DSN scheme picks the driver. For SQLite the DSN points at a local file. For production DBs use a read-only credential — DBHub honours the connection's permissions.",
	},
	{
		Name:        "mongodb",
		Description: "MongoDB official server — query, aggregate, manage collections and indexes.",
		Homepage:    "https://github.com/mongodb-js/mongodb-mcp-server",
		Runtime:     "node",
		InstallCmd:  mcpdomain.InstallCmd{Command: "npx", Args: []string{"-y", "mongodb-mcp-server"}},
		Category:    "database",
		Tier:        3,
		RequiredEnv: []mcpdomain.EnvRequirement{
			{Name: "MDB_MCP_CONNECTION_STRING", Description: "MongoDB connection string (mongodb://... or mongodb+srv://...).", SetupURL: "https://www.mongodb.com/docs/manual/reference/connection-string/", Secret: true},
		},
		Notes: "Maintained by mongodb-js. Use a read-only role for safety on production clusters.",
	},
	{
		Name:        "supabase",
		Description: "Supabase official: query Postgres, manage auth users, list storage buckets, inspect functions — full backend-as-a-service stack.",
		Homepage:    "https://github.com/supabase-community/supabase-mcp",
		Runtime:     "node",
		InstallCmd:  mcpdomain.InstallCmd{Command: "npx", Args: []string{"-y", "@supabase/mcp-server-supabase"}},
		Category:    "database",
		Tier:        2,
		RequiredEnv: []mcpdomain.EnvRequirement{
			{Name: "SUPABASE_ACCESS_TOKEN", Description: "Personal Access Token from your Supabase account.", SetupURL: "https://app.supabase.com/account/tokens", Secret: true},
			{Name: "SUPABASE_PROJECT_REF", Description: "Project reference (the slug from your Supabase project URL).", SetupURL: "https://app.supabase.com/projects", Secret: false},
		},
		Notes: "PAT scopes the MCP to your Supabase account; project ref pins to one project. The token is account-wide — treat it like a password.",
	},

	// ── Project management ───────────────────────────────────────
	{
		Name:        "linear",
		Description: "Linear: issues, projects, cycles, comments — the dev-team tracker most YC startups use.",
		Homepage:    "https://github.com/jerhadf/linear-mcp-server",
		Runtime:     "node",
		InstallCmd:  mcpdomain.InstallCmd{Command: "npx", Args: []string{"-y", "linear-mcp-server"}},
		Category:    "project-mgmt",
		Tier:        1,
		RequiredEnv: []mcpdomain.EnvRequirement{
			{Name: "LINEAR_API_KEY", Description: "Linear API key (Settings → API).", SetupURL: "https://linear.app/settings/api", Secret: true},
		},
		Notes: "API key is workspace-wide. For multi-workspace users, install separate copies under different aliases is currently not supported in this curated catalog.",
	},
	{
		Name:        "atlassian",
		Description: "Atlassian: Jira issues + Confluence pages in one server. Enterprise project-management workhorse.",
		Homepage:    "https://github.com/sooperset/mcp-atlassian",
		Runtime:     "python",
		InstallCmd:  mcpdomain.InstallCmd{Command: "uvx", Args: []string{"mcp-atlassian"}},
		Category:    "project-mgmt",
		Tier:        2,
		RequiredEnv: []mcpdomain.EnvRequirement{
			{Name: "JIRA_URL", Description: "Jira instance URL (e.g. https://your-org.atlassian.net).", SetupURL: "https://www.atlassian.com/software/jira", Secret: false},
			{Name: "JIRA_USERNAME", Description: "Atlassian account email.", SetupURL: "https://id.atlassian.com/manage-profile/profile-and-visibility", Secret: false},
			{Name: "JIRA_API_TOKEN", Description: "Atlassian API token (used for both Jira and Confluence).", SetupURL: "https://id.atlassian.com/manage-profile/security/api-tokens", Secret: true},
			{Name: "CONFLUENCE_URL", Description: "Confluence instance URL (often same domain as Jira). Leave empty to disable Confluence tools.", SetupURL: "https://www.atlassian.com/software/confluence", Secret: false},
		},
		Notes: "One API token covers both Jira and Confluence. For Atlassian Cloud the URL is https://<org>.atlassian.net (Jira) and https://<org>.atlassian.net/wiki (Confluence).",
	},

	// ── Docs, collaboration, design ──────────────────────────────
	{
		Name:        "notion",
		Description: "Notion official: pages, databases, comments, blocks. 22 tools covering most of the Notion API.",
		Homepage:    "https://github.com/makenotion/notion-mcp-server",
		Runtime:     "node",
		InstallCmd:  mcpdomain.InstallCmd{Command: "npx", Args: []string{"-y", "@notionhq/notion-mcp-server"}},
		Category:    "docs",
		Tier:        1,
		RequiredEnv: []mcpdomain.EnvRequirement{
			{Name: "NOTION_TOKEN", Description: "Notion internal-integration token. After creating an integration, share each page/database you want accessible with the integration.", SetupURL: "https://www.notion.so/profile/integrations", Secret: true},
		},
		Notes: "Create an internal integration in Notion → copy the token here → SHARE each page or database with the integration via Notion's UI (otherwise the MCP can't see them). Share once at the parent level to cover all children.",
	},
	{
		Name:        "slack",
		Description: "Slack workspace: send messages to channels / DMs, list channels, read recent messages, post to threads.",
		Homepage:    "https://github.com/modelcontextprotocol/servers/tree/main/src/slack",
		Runtime:     "node",
		InstallCmd:  mcpdomain.InstallCmd{Command: "npx", Args: []string{"-y", "@modelcontextprotocol/server-slack"}},
		Category:    "docs",
		Tier:        2,
		RequiredEnv: []mcpdomain.EnvRequirement{
			{Name: "SLACK_BOT_TOKEN", Description: "Slack Bot User OAuth Token (xoxb-...). Create a Slack App at api.slack.com/apps, install it to your workspace.", SetupURL: "https://api.slack.com/apps", Secret: true},
			{Name: "SLACK_TEAM_ID", Description: "Slack Workspace / Team ID (starts with T...). Found in Slack admin or via /team-info API.", SetupURL: "https://api.slack.com/methods/team.info", Secret: false},
		},
		Notes: "Setup is the most involved — workspace admin must approve a Slack App with relevant scopes (channels:read, chat:write, etc). Bot tokens are workspace-scoped; don't reuse across orgs.",
	},
	{
		Name:        "figma",
		Description: "Framelink Figma context: read frames, components, styles. Lets the LLM implement Figma designs in code one-shot.",
		Homepage:    "https://github.com/GLips/Figma-Context-MCP",
		Runtime:     "node",
		InstallCmd:  mcpdomain.InstallCmd{Command: "npx", Args: []string{"-y", "figma-developer-mcp"}},
		Category:    "design",
		Tier:        1,
		RequiredEnv: []mcpdomain.EnvRequirement{
			{Name: "FIGMA_API_KEY", Description: "Figma personal access token (file_read scope).", SetupURL: "https://www.figma.com/settings", Secret: true},
		},
		Notes: "Free for any Figma plan including Starter. Token grants read-access to all your files — keep it secret.",
	},
	{
		Name:        "memory",
		Description: "Persistent knowledge graph across conversations. The LLM can record entities + relations and retrieve them later — Forgify's only built-in cross-conversation memory.",
		Homepage:    "https://github.com/modelcontextprotocol/servers/tree/main/src/memory",
		Runtime:     "node",
		InstallCmd:  mcpdomain.InstallCmd{Command: "npx", Args: []string{"-y", "@modelcontextprotocol/server-memory"}},
		Category:    "memory",
		Tier:        0,
		Notes:       "Stores graph in a JSON file on disk (default: package's working dir). For long-lived state, configure MEMORY_FILE_PATH to a stable location like ~/.forgify/memory/graph.json.",
	},
	{
		Name:        "e2b",
		Description: "Run arbitrary code (Python / Node / Bash) in a fresh cloud Linux VM. Useful when local execution is risky or requires installing system packages.",
		Homepage:    "https://github.com/e2b-dev/mcp-server",
		Runtime:     "node",
		InstallCmd:  mcpdomain.InstallCmd{Command: "npx", Args: []string{"-y", "@e2b/mcp-server"}},
		Category:    "sandbox",
		Tier:        1,
		RequiredEnv: []mcpdomain.EnvRequirement{
			{Name: "E2B_API_KEY", Description: "E2B API key.", SetupURL: "https://e2b.dev/dashboard?tab=keys", Secret: true},
		},
		Notes: "Free tier: 100 hours of sandbox compute / month. Each sandbox is ephemeral — state doesn't persist across calls unless you mount data via E2B's filesystem APIs.",
	},

	// ── Email / office ───────────────────────────────────────────
	{
		Name:        "gmail",
		Description: "Gmail: read inbox, send mail, manage labels / drafts. Uses Google's OAuth device-code flow — first run prints a login URL.",
		Homepage:    "https://github.com/GongRzhe/Gmail-MCP-Server",
		Runtime:     "node",
		InstallCmd:  mcpdomain.InstallCmd{Command: "npx", Args: []string{"-y", "@gongrzhe/server-gmail-autoauth-mcp"}},
		Category:    "email",
		Tier:        2,
		Notes:       "First run: the server prints a Google device-login URL + code to stderr. Open the URL, sign in to your Gmail account, paste the code → tokens are saved locally. The package ships its own OAuth client credentials; for production-grade isolation register your own Google Cloud OAuth app.",
	},
	{
		Name:        "ms365",
		Description: "Microsoft 365 via Graph API: 200+ tools covering Outlook mail, Calendar events, OneDrive files, Excel spreadsheets, Teams.",
		Homepage:    "https://github.com/softeria/ms-365-mcp-server",
		Runtime:     "node",
		InstallCmd:  mcpdomain.InstallCmd{Command: "npx", Args: []string{"-y", "@softeria/ms-365-mcp-server"}},
		Category:    "email",
		Tier:        2,
		Notes:       "First run: the server prints https://microsoft.com/devicelogin + a code to stderr. Visit it, sign in to your Microsoft account, paste the code. Tokens saved to OS keychain (keytar) on macOS / Windows; on headless Linux falls back to file. Ships with Softeria's shared Azure AD app; for production register your own and set MS365_MCP_CLIENT_ID + MS365_MCP_TENANT_ID.",
	},
}
