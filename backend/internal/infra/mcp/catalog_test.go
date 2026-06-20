package mcp

import (
	"context"
	"errors"
	"strings"
	"testing"

	mcpdomain "github.com/sunweilin/anselm/backend/internal/domain/mcp"
)

// fakeRegistry serves the embedded snapshot with no network — the underlying source the catalog
// decorates in tests.
//
// fakeRegistry 无网地提供内嵌 snapshot——测试里 catalog 装饰的底层源。
type fakeRegistry struct{ entries []mcpdomain.RegistryEntry }

func newFakeRegistry(t *testing.T) *fakeRegistry {
	t.Helper()
	entries, err := parseGitHub(embeddedSnapshot)
	if err != nil {
		t.Fatalf("parse snapshot: %v", err)
	}
	return &fakeRegistry{entries: entries}
}

func (f *fakeRegistry) List(context.Context) ([]mcpdomain.RegistryEntry, error) {
	return f.entries, nil
}

func (f *fakeRegistry) Get(_ context.Context, name string) (*mcpdomain.RegistryEntry, error) {
	for i := range f.entries {
		if f.entries[i].Name == name {
			cp := f.entries[i]
			return &cp, nil
		}
	}
	return nil, mcpdomain.ErrRegistryEntryNotFound
}

// TestCuratedCatalog_EmbeddedParses guards the compiled-in catalog.json: it parses, is non-empty,
// and every slug exists in the snapshot — a typo would silently drop the server from the
// marketplace (List filters by an exact name match), so this fails loudly instead.
//
// TestCuratedCatalog_EmbeddedParses 守编译进的 catalog.json：可解析、非空、每个 slug 都在 snapshot 里
// ——拼错会让该 server 从市场静默消失（List 按精确名过滤），故此处大声失败而非静默丢。
func TestCuratedCatalog_EmbeddedParses(t *testing.T) {
	if len(parsedCatalog) == 0 {
		t.Fatal("embedded catalog is empty")
	}
	snap, err := parseGitHub(embeddedSnapshot)
	if err != nil {
		t.Fatalf("parse snapshot: %v", err)
	}
	known := make(map[string]bool, len(snap))
	for _, e := range snap {
		known[e.Name] = true
	}
	for _, e := range parsedCatalog {
		if !known[e.Slug] {
			t.Errorf("catalog slug %q is not in the registry snapshot (would be dropped from the marketplace)", e.Slug)
		}
	}
}

// TestCuratedCatalog_ListIsWhitelist verifies List returns exactly the whitelisted set (every
// returned name is in the catalog, count matches) and that a deliberately-excluded server (Figma,
// which needs a vendor allowlist) is NOT offered.
//
// TestCuratedCatalog_ListIsWhitelist 验证 List 恰好返回白名单集（每个返回名都在 catalog、数量一致），
// 且被有意排除的 server（Figma，需厂商 allowlist）不被提供。
func TestCuratedCatalog_ListIsWhitelist(t *testing.T) {
	cat := NewCuratedCatalog(newFakeRegistry(t))
	got, err := cat.List(context.Background())
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != len(parsedCatalog) {
		t.Fatalf("list returned %d entries, catalog has %d", len(got), len(parsedCatalog))
	}
	allowed := make(map[string]bool, len(parsedCatalog))
	for _, e := range parsedCatalog {
		allowed[e.Slug] = true
	}
	for _, e := range got {
		if !allowed[e.Name] {
			t.Errorf("List leaked non-whitelisted server %q", e.Name)
		}
		if e.Name == "com.figma.mcp/mcp" {
			t.Error("Figma must not be offered (needs vendor allowlist)")
		}
	}
}

// TestCuratedCatalog_DeterministicViaFallback proves the available set is stable even when the
// live registry feed returns nothing (offline / drifted): the embedded snapshot fallback still
// yields the full whitelist — a vetted server is never silently dropped from the marketplace.
//
// TestCuratedCatalog_DeterministicViaFallback 证明 live registry feed 返回空（离线/漂移）时可用集仍稳定：
// 内嵌 snapshot 兜底仍给出完整白名单——已核验 server 绝不从市场静默消失。
func TestCuratedCatalog_DeterministicViaFallback(t *testing.T) {
	cat := NewCuratedCatalog(&fakeRegistry{entries: nil}) // live feed yields nothing
	got, err := cat.List(context.Background())
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != len(parsedCatalog) {
		t.Fatalf("empty live feed: fallback should still yield all %d, got %d", len(parsedCatalog), len(got))
	}
	if _, err := cat.Get(context.Background(), parsedCatalog[0].Slug); err != nil {
		t.Fatalf("Get should fall back to the snapshot: %v", err)
	}
}

// TestCuratedCatalog_GetRejectsNonWhitelisted ensures an unvetted server cannot be resolved for
// install — the marketplace must never let one through.
//
// TestCuratedCatalog_GetRejectsNonWhitelisted 确保未核验 server 无法被解析安装——市场绝不放行。
func TestCuratedCatalog_GetRejectsNonWhitelisted(t *testing.T) {
	cat := NewCuratedCatalog(newFakeRegistry(t))
	for _, slug := range []string{"com.figma.mcp/mcp", "com.microsoft/sentinel-data-exploration", "io.github.microsoft/EnterpriseMCP"} {
		if _, err := cat.Get(context.Background(), slug); !errors.Is(err, mcpdomain.ErrRegistryEntryNotFound) {
			t.Errorf("Get(%q) = %v, want ErrRegistryEntryNotFound", slug, err)
		}
	}
}

// TestCuratedCatalog_AllEntriesPlannable verifies every whitelisted entry resolves to an install
// plan — a curated server that can't even be planned is a broken listing.
//
// TestCuratedCatalog_AllEntriesPlannable 验证每个白名单条目都能解析出安装计划——连 plan 都出不来的
// curated server 是坏条目。
func TestCuratedCatalog_AllEntriesPlannable(t *testing.T) {
	cat := NewCuratedCatalog(newFakeRegistry(t))
	got, err := cat.List(context.Background())
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	for _, e := range got {
		if _, ok := e.Plan(); !ok {
			t.Errorf("catalog entry %q is not plannable", e.Name)
		}
	}
}

// TestCuratedCatalog_OverlayServersRequireTheirToken is the core anti-silent-zero-auth guarantee:
// every entry carrying an auth overlay (the static-token servers) must, after the overlay is
// applied, surface its token as a REQUIRED env var — so install refuses to proceed unauthenticated.
//
// TestCuratedCatalog_OverlayServersRequireTheirToken 是反「静默零认证」的核心保证：每个带 auth 覆盖的
// 条目（静态 token server）套上覆盖后，必须把 token 暴露成必填 env——使安装拒绝无认证地进行。
func TestCuratedCatalog_OverlayServersRequireTheirToken(t *testing.T) {
	cat := NewCuratedCatalog(newFakeRegistry(t))
	for _, ce := range parsedCatalog {
		if ce.Auth == nil || ce.Auth.Transport == "oauth" {
			continue // oauth servers mint the token via the interactive flow, not a static env
		}
		entry, err := cat.Get(context.Background(), ce.Slug)
		if err != nil {
			t.Fatalf("Get(%q): %v", ce.Slug, err)
		}
		plan, ok := entry.Plan()
		if !ok {
			t.Errorf("%q: not plannable after overlay", ce.Slug)
			continue
		}
		if ce.Auth.Env == nil {
			t.Errorf("%q: auth overlay has no env (cannot require a token)", ce.Slug)
			continue
		}
		found := false
		for _, ev := range plan.EnvVars {
			if ev.Name == ce.Auth.Env.Name {
				found = true
			}
		}
		if !found {
			t.Errorf("%q: token env %q is not REQUIRED by the plan (silent-zero-auth risk)", ce.Slug, ce.Auth.Env.Name)
		}
	}
}

// TestCuratedCatalog_CurrentlyExcluded pins what stays OFF the whitelist: figma-remote needs Figma
// to allowlist ANSELM (a vendor step — never), and the two Microsoft Entra servers are user-self-
// serve (bring-your-own Entra app) but their Entra-specific scope/redirect isn't wired yet (pending).
//
// TestCuratedCatalog_CurrentlyExcluded 钉死仍不在白名单的：figma-remote 需 Figma 把 Anselm 加 allowlist
// （厂商步骤，永不）；两个微软 Entra server 是用户自助（自带 Entra app），但 Entra 专属 scope/redirect 未接（待办）。
func TestCuratedCatalog_CurrentlyExcluded(t *testing.T) {
	cat := NewCuratedCatalog(newFakeRegistry(t))
	excluded := []string{
		"com.figma.mcp/mcp",
		"io.github.microsoft/EnterpriseMCP", "com.microsoft/sentinel-data-exploration",
	}
	for _, slug := range excluded {
		if _, err := cat.Get(context.Background(), slug); !errors.Is(err, mcpdomain.ErrRegistryEntryNotFound) {
			t.Errorf("excluded server %q must not be installable, got err=%v", slug, err)
		}
	}
}

// TestCuratedCatalog_BoxBYOClient verifies Box is installable as an OAuth server using the user's OWN
// registered app: its plan is OAuth carrying the user-supplied client_id + client_secret as required
// env (the flow then skips DCR and uses them).
//
// TestCuratedCatalog_BoxBYOClient 验证 Box 作为「用户自带注册 app」的 OAuth server 可装：计划是 OAuth、
// 带用户给的 client_id + client_secret 必填 env（流程随即跳过 DCR、用它们）。
func TestCuratedCatalog_BoxBYOClient(t *testing.T) {
	cat := NewCuratedCatalog(newFakeRegistry(t))
	entry, err := cat.Get(context.Background(), "box/mcp-server-box-remote")
	if err != nil {
		t.Fatalf("Get(box): %v", err)
	}
	plan, ok := entry.Plan()
	if !ok || !plan.OAuth {
		t.Fatalf("box should plan as OAuth, got ok=%v plan=%+v", ok, plan)
	}
	if plan.OAuthClientIDEnv == "" || plan.OAuthClientSecretEnv == "" {
		t.Errorf("box must require a user-supplied client_id + client_secret, got idEnv=%q secretEnv=%q", plan.OAuthClientIDEnv, plan.OAuthClientSecretEnv)
	}
	names := map[string]bool{}
	for _, ev := range plan.EnvVars {
		names[ev.Name] = true
	}
	if !names[plan.OAuthClientIDEnv] || !names[plan.OAuthClientSecretEnv] {
		t.Errorf("client_id/secret envs must be surfaced as required inputs, got %v", plan.EnvVars)
	}
}

// TestCuratedCatalog_GleanOAuthWithURLEnv verifies Glean is installable as an OAuth server whose
// per-tenant endpoint is supplied by the user: its plan is OAuth with a templated URL and a single
// required URL env (so install resolves the instance URL, then runs the OAuth flow).
//
// TestCuratedCatalog_GleanOAuthWithURLEnv 验证 Glean 作为 OAuth server 可装、其每租户端点由用户给出：
// 计划是 OAuth + 模板 URL + 单个必填 URL env（安装先解析实例 URL 再走 OAuth 流程）。
func TestCuratedCatalog_GleanOAuthWithURLEnv(t *testing.T) {
	cat := NewCuratedCatalog(newFakeRegistry(t))
	entry, err := cat.Get(context.Background(), "com.glean/mcp")
	if err != nil {
		t.Fatalf("Get(glean): %v", err)
	}
	plan, ok := entry.Plan()
	if !ok || !plan.OAuth {
		t.Fatalf("glean should plan as OAuth, got ok=%v plan=%+v", ok, plan)
	}
	if len(plan.EnvVars) != 1 {
		t.Fatalf("glean must require exactly its URL env, got %v", plan.EnvVars)
	}
	if !strings.Contains(plan.URL, "{"+plan.EnvVars[0].Name+"}") {
		t.Errorf("glean URL %q must be templated on its URL env %q", plan.URL, plan.EnvVars[0].Name)
	}
}

// TestCuratedCatalog_OAuthServersPlanAsOAuth verifies the Tier-2 oauth-dcr servers are now offered
// and resolve to an OAuth install plan (remote + OAuth flag + URL, no static token) — so install
// runs the authorization flow rather than collecting a credential.
//
// TestCuratedCatalog_OAuthServersPlanAsOAuth 验证档 2 的 oauth-dcr server 现已上架并解析成 OAuth 安装
// 计划（remote + OAuth 标志 + URL、无静态 token）——故安装走授权流程而非收凭据。
func TestCuratedCatalog_OAuthServersPlanAsOAuth(t *testing.T) {
	cat := NewCuratedCatalog(newFakeRegistry(t))
	oauthServers := []string{
		"com.atlassian/atlassian-mcp-server", "com.webflow/mcp", "io.github.miroapp/mcp-server",
		"amplitude/mcp-server-guide", "com.stackoverflow.mcp/mcp", "com.wix/mcp",
		"intercom/intercom-mcp-server", "com.getguru/mcp-server", "io.github.oakallow/oakallow",
		"com.vercel/vercel-mcp", // re-verified: open DCR (POST register → 201), not allowlist-gated
	}
	for _, slug := range oauthServers {
		entry, err := cat.Get(context.Background(), slug)
		if err != nil {
			t.Fatalf("Get(%q): %v", slug, err)
		}
		plan, ok := entry.Plan()
		if !ok || !plan.OAuth || plan.URL == "" {
			t.Errorf("%q should plan as OAuth (remote, OAuth=true, URL set), got ok=%v plan=%+v", slug, ok, plan)
		}
		if len(plan.EnvVars) != 0 {
			t.Errorf("%q OAuth plan must collect no static env, got %v", slug, plan.EnvVars)
		}
	}
}
