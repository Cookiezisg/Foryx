//go:build pipeline

package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	mcpdomain "github.com/sunweilin/forgify/backend/internal/domain/mcp"
	th "github.com/sunweilin/forgify/backend/test/harness"
)

// curatedSmokeEnabled gates every test on FORGIFY_CURATED_SMOKE=1.
//
// curatedSmokeEnabled 用 FORGIFY_CURATED_SMOKE=1 门控本文件全部测试。
func curatedSmokeEnabled(t *testing.T) {
	t.Helper()
	if os.Getenv("FORGIFY_CURATED_SMOKE") != "1" {
		t.Skip("set FORGIFY_CURATED_SMOKE=1 to opt in (real npx/uvx installs)")
	}
}

// sharedSandboxDir returns FORGIFY_TEST_SANDBOX_DIR; lets mise/node/npm cache survive across runs.
//
// sharedSandboxDir 返 FORGIFY_TEST_SANDBOX_DIR；让缓存跨运行保留。
func sharedSandboxDir() string { return os.Getenv("FORGIFY_TEST_SANDBOX_DIR") }

// installTimeout caps install at 15min (worst case: mise + node + npm playwright).
const installTimeout = 15 * time.Minute

// curatedSmokeCase holds per-entry env/args for installing a curated entry.
//
// curatedSmokeCase 描述一条 entry 装机所需 env/args。
type curatedSmokeCase struct {
	name        string
	envFrom     []string
	envExtra    map[string]string
	args        map[string]string
	knownBroken string
}

// smokeCases drives TestCuratedMarketplace_AllSmoke; envFrom must mirror curated RequiredEnv.
var smokeCases = []curatedSmokeCase{
	{name: "playwright"},
	{name: "chrome-devtools"},
	{name: "duckduckgo"},
	{name: "context7"},
	{name: "memory"},

	{name: "tavily", envFrom: []string{"TAVILY_API_KEY"}},
	{name: "firecrawl", envFrom: []string{"FIRECRAWL_API_KEY"}},
	{name: "github", envFrom: []string{"GITHUB_PERSONAL_ACCESS_TOKEN"}},
	{name: "gitlab", envFrom: []string{"GITLAB_PERSONAL_ACCESS_TOKEN", "GITLAB_API_URL"}},
	{name: "sentry", envFrom: []string{"SENTRY_AUTH_TOKEN", "SENTRY_HOST"}},
	{name: "linear", envFrom: []string{"LINEAR_API_KEY"}},
	{name: "atlassian", envFrom: []string{"JIRA_URL", "JIRA_USERNAME", "JIRA_API_TOKEN", "CONFLUENCE_URL"}},
	{name: "notion", envFrom: []string{"NOTION_TOKEN"}},
	{name: "slack", envFrom: []string{"SLACK_BOT_TOKEN", "SLACK_TEAM_ID"}},
	{name: "figma", envFrom: []string{"FIGMA_API_KEY"}},
	{name: "e2b", envFrom: []string{"E2B_API_KEY"}},

	// ms365 ships shared Azure AD creds; google-workspace needs user-supplied OAuth.
	{name: "google-workspace", envFrom: []string{"GOOGLE_OAUTH_CLIENT_ID", "GOOGLE_OAUTH_CLIENT_SECRET"}},
	{name: "ms365"},

	{name: "dbhub", envFrom: []string{"DSN"}},
	{name: "mongodb", envFrom: []string{"MDB_MCP_CONNECTION_STRING"}},
	{name: "supabase", envFrom: []string{"SUPABASE_ACCESS_TOKEN", "SUPABASE_PROJECT_REF"}},
}

func TestCuratedMarketplace_AllSmoke(t *testing.T) {
	curatedSmokeEnabled(t)
	opts := []th.Option{th.WithCuratedRegistry()}
	if d := sharedSandboxDir(); d != "" {
		opts = append(opts, th.WithSandboxDataDir(d))
	}
	h := th.New(t, opts...)
	if !h.Sandbox.IsReady() {
		t.Skip("sandbox not ready (run `make resources` to embed mise)")
	}

	if got := len(smokeCases); got != 21 {
		t.Fatalf("smokeCases length = %d, want 21 (curated registry shape changed?)", got)
	}

	for _, tc := range smokeCases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.knownBroken != "" {
				t.Skipf("knownBroken: %s", tc.knownBroken)
			}

			env, hasRealCreds := collectEnv(t, tc)
			ctx, cancel := context.WithTimeout(context.Background(), installTimeout)
			defer cancel()

			if rmErr := h.MCP.RemoveServer(ctx, tc.name); rmErr != nil &&
				!errors.Is(rmErr, mcpdomain.ErrServerNotFound) {
				t.Logf("pre-clean remove %s: %v", tc.name, rmErr)
			}

			st, err := h.MCP.InstallFromRegistry(ctx, tc.name, env, tc.args)
			t.Cleanup(func() {
				cleanupCtx, cancelClean := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancelClean()
				if rmErr := h.MCP.RemoveServer(cleanupCtx, tc.name); rmErr != nil &&
					!errors.Is(rmErr, mcpdomain.ErrServerNotFound) {
					t.Logf("cleanup remove %s: %v", tc.name, rmErr)
				}
			})

			assertInstallOutcome(t, tc.name, st, err, hasRealCreds)
		})
	}
}

// assertInstallOutcome enforces strict ready check for real-creds mode; stub mode tolerates auth/conn errors.
//
// assertInstallOutcome 真凭证严格 ready；stub 模式容忍 auth/conn 错。
func assertInstallOutcome(t *testing.T, name string, st *mcpdomain.ServerStatus, err error, hasRealCreds bool) {
	t.Helper()

	if errors.Is(err, mcpdomain.ErrRequiredEnvMissing) ||
		errors.Is(err, mcpdomain.ErrRequiredArgsMissing) {
		t.Fatalf("%s: smokeCases envFrom/args drift from curated RequiredEnv — fix the test data: %v", name, err)
	}

	if !hasRealCreds {
		if err != nil {
			t.Logf("%s [stub-mode] install error after validation passed (acceptable): %v", name, err)
			return
		}
		switch st.Status {
		case mcpdomain.StatusReady:
			t.Logf("%s [stub-mode] reached ready with stub creds — server defers auth", name)
		default:
			t.Logf("%s [stub-mode] status=%q lastError=%q — install path OK, runtime auth/conn pending",
				name, st.Status, st.LastError)
		}
		return
	}

	if err != nil {
		t.Fatalf("%s [real-creds] InstallFromRegistry: %v", name, err)
	}
	if st == nil {
		t.Fatalf("%s [real-creds] nil status", name)
	}
	if st.Status != mcpdomain.StatusReady {
		t.Fatalf("%s [real-creds] status=%q lastError=%q — expected ready",
			name, st.Status, st.LastError)
	}
	if len(st.Tools) == 0 {
		t.Errorf("%s [real-creds] ready but tools/list empty", name)
	}
}

// envStub is the placeholder for an empty envFrom key; lets install + handshake proceed.
const envStub = "forgify-smoke-stub"

// collectEnv builds env from envFrom (real or envStub) + envExtra; returns hasRealCreds.
//
// collectEnv 用 envFrom（真值或 envStub）+ envExtra 拼 env，并返 hasRealCreds。
func collectEnv(t *testing.T, tc curatedSmokeCase) (env map[string]string, hasRealCreds bool) {
	t.Helper()
	out := map[string]string{}
	hasRealCreds = true
	for _, k := range tc.envFrom {
		v := os.Getenv(k)
		if v == "" {
			out[k] = envStub
			hasRealCreds = false
			continue
		}
		out[k] = v
	}
	for k, v := range tc.envExtra {
		out[k] = v
	}
	return out, hasRealCreds
}


func TestCuratedMarketplace_T0_Live_DuckDuckGo(t *testing.T) {
	curatedSmokeEnabled(t)
	st, h := installT0(t, "duckduckgo")
	requireToolListed(t, st, "search")

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	out, err := h.MCP.CallTool(ctx, "duckduckgo", "search",
		json.RawMessage(`{"query":"anthropic claude","max_results":3}`))
	if err != nil {
		t.Fatalf("CallTool search: %v", err)
	}
	if !strings.Contains(strings.ToLower(out), "anthropic") &&
		!strings.Contains(strings.ToLower(out), "claude") {
		t.Errorf("search result lacks expected term anthropic/claude: %s", trimForLog(out))
	}
}

func TestCuratedMarketplace_T0_Live_Context7(t *testing.T) {
	curatedSmokeEnabled(t)
	st, h := installT0(t, "context7")
	toolName := pickFirstTool(st, "resolve-library-id", "resolve_library_id", "search")
	if toolName == "" {
		t.Fatalf("context7 exposes no resolver tool; tools=%v", toolNames(st))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	// Send both query and libraryName — resolve-library-id required schema has shifted between npm versions.
	out, err := h.MCP.CallTool(ctx, "context7", toolName,
		json.RawMessage(`{"libraryName":"react","query":"react"}`))
	if err != nil {
		t.Fatalf("CallTool %s: %v", toolName, err)
	}
	if strings.TrimSpace(out) == "" {
		t.Errorf("context7 %s returned empty result", toolName)
	}
}

func TestCuratedMarketplace_T0_Live_Memory(t *testing.T) {
	curatedSmokeEnabled(t)
	st, h := installT0(t, "memory")
	createTool := pickFirstTool(st, "create_entities", "create-entities")
	readTool := pickFirstTool(st, "read_graph", "read-graph")
	if createTool == "" || readTool == "" {
		t.Fatalf("memory missing expected create/read tools; tools=%v", toolNames(st))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	createPayload := `{"entities":[{"name":"forgify-pipeline-marker","entityType":"test","observations":["smoke-test entity"]}]}`
	if _, err := h.MCP.CallTool(ctx, "memory", createTool, json.RawMessage(createPayload)); err != nil {
		t.Fatalf("CallTool %s: %v", createTool, err)
	}
	out, err := h.MCP.CallTool(ctx, "memory", readTool, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("CallTool %s: %v", readTool, err)
	}
	if !strings.Contains(out, "forgify-pipeline-marker") {
		t.Errorf("read_graph missing the entity we just created: %s", trimForLog(out))
	}
}

func TestCuratedMarketplace_T0_Live_Playwright(t *testing.T) {
	curatedSmokeEnabled(t)
	st, h := installT0(t, "playwright")
	navTool := pickFirstTool(st, "browser_navigate", "navigate")
	snapTool := pickFirstTool(st, "browser_snapshot", "snapshot", "browser_get_text")
	if navTool == "" || snapTool == "" {
		t.Fatalf("playwright missing expected nav/snapshot tools; tools=%v", toolNames(st))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	navPayload := `{"url":"https://example.com"}`
	if _, err := h.MCP.CallTool(ctx, "playwright", navTool, json.RawMessage(navPayload)); err != nil {
		t.Fatalf("CallTool %s: %v", navTool, err)
	}
	out, err := h.MCP.CallTool(ctx, "playwright", snapTool, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("CallTool %s: %v", snapTool, err)
	}
	if !strings.Contains(strings.ToLower(out), "example") {
		t.Errorf("snapshot of example.com lacks expected text: %s", trimForLog(out))
	}
}

func TestCuratedMarketplace_T0_Live_ChromeDevTools(t *testing.T) {
	curatedSmokeEnabled(t)
	st, h := installT0(t, "chrome-devtools")
	navTool := pickFirstTool(st, "navigate_page", "navigate", "page_navigate")
	if navTool == "" {
		t.Fatalf("chrome-devtools missing navigate tool; tools=%v", toolNames(st))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	out, err := h.MCP.CallTool(ctx, "chrome-devtools", navTool,
		json.RawMessage(`{"url":"https://example.com"}`))
	if err != nil {
		t.Fatalf("CallTool %s: %v", navTool, err)
	}
	if strings.TrimSpace(out) == "" {
		t.Errorf("navigate returned empty payload (protocol issue)")
	}
}

// installT0 installs a tier-0 entry and registers cleanup; returns ServerStatus + harness.
//
// installT0 装一条 T0 并挂 cleanup，返 ServerStatus + harness。
func installT0(t *testing.T, name string) (*mcpdomain.ServerStatus, *th.Harness) {
	t.Helper()
	opts := []th.Option{th.WithCuratedRegistry()}
	if d := sharedSandboxDir(); d != "" {
		opts = append(opts, th.WithSandboxDataDir(d))
	}
	h := th.New(t, opts...)
	if !h.Sandbox.IsReady() {
		t.Skip("sandbox not ready (run `make resources` to embed mise)")
	}

	ctx, cancel := context.WithTimeout(context.Background(), installTimeout)
	defer cancel()

	// Pre-clean — shared sandbox dir may carry leftover mcp.json from a crashed prior run.
	if rmErr := h.MCP.RemoveServer(ctx, name); rmErr != nil &&
		!errors.Is(rmErr, mcpdomain.ErrServerNotFound) {
		t.Logf("pre-clean remove %s: %v", name, rmErr)
	}

	st, err := h.MCP.InstallFromRegistry(ctx, name, nil, nil)
	t.Cleanup(func() {
		cleanCtx, cancelClean := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancelClean()
		if rmErr := h.MCP.RemoveServer(cleanCtx, name); rmErr != nil &&
			!errors.Is(rmErr, mcpdomain.ErrServerNotFound) {
			t.Logf("cleanup remove %s: %v", name, rmErr)
		}
	})
	if err != nil {
		t.Fatalf("InstallFromRegistry %s: %v", name, err)
	}
	if st.Status != mcpdomain.StatusReady {
		t.Fatalf("%s status=%q lastError=%q (want ready)", name, st.Status, st.LastError)
	}
	if len(st.Tools) == 0 {
		t.Fatalf("%s exposes no tools after install", name)
	}
	return st, h
}

// requireToolListed fatals if the named tool is not in st.Tools.
//
// requireToolListed 若 tool 不在 st.Tools 则 fatal。
func requireToolListed(t *testing.T, st *mcpdomain.ServerStatus, want string) {
	t.Helper()
	for _, td := range st.Tools {
		if td.Name == want {
			return
		}
	}
	t.Fatalf("tool %q not exposed; tools=%v", want, toolNames(st))
}

// pickFirstTool returns the first candidate in st.Tools, or "" if none match (handles renames).
//
// pickFirstTool 返 st.Tools 中第一个匹配的候选；容忍上游改名。
func pickFirstTool(st *mcpdomain.ServerStatus, candidates ...string) string {
	have := map[string]bool{}
	for _, td := range st.Tools {
		have[td.Name] = true
	}
	for _, c := range candidates {
		if have[c] {
			return c
		}
	}
	return ""
}

func toolNames(st *mcpdomain.ServerStatus) []string {
	out := make([]string, 0, len(st.Tools))
	for _, td := range st.Tools {
		out = append(out, td.Name)
	}
	return out
}

// trimForLog truncates tool result payloads so failures don't drown the test log.
//
// trimForLog 截断 tool 结果，避免日志被淹。
func trimForLog(s string) string {
	const max = 200
	if len(s) <= max {
		return s
	}
	return s[:max] + "...[truncated]"
}
