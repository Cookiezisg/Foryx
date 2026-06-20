package mcp

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"

	mcpdomain "github.com/sunweilin/anselm/backend/internal/domain/mcp"
)

// curatedCatalog is the whitelist of marketplace servers vetted to genuinely work with code
// alone (Tier 1: stdio static-env + remote static-token). Each entry's optional auth overlay
// PINS the verified install+auth config; servers needing a vendor business step (OAuth app
// registration / allowlist) are simply absent. See docs/working/mcp-oauth-support.
//
// curatedCatalog 是市场白名单：只列「纯代码即可真正连上用」的 server（档 1：stdio 静态 env + remote
// 静态 token）。每条的可选 auth 覆盖钉死已核验的安装/认证配置；需要厂商业务步骤（注册 OAuth app/
// 进 allowlist）的直接缺席。见 docs/working/mcp-oauth-support。
//
//go:embed catalog.json
var curatedCatalog []byte

// catalogEntry is one whitelisted server. Slug alone = available, install from the upstream
// registry row as-is (works-now). Auth != nil pins the verified auth the raw row lacks/got wrong
// (a static-token remote whose upstream row carries no header, or a stdio token env).
//
// catalogEntry 是一条白名单 server。仅 Slug = 可用、按上游 registry 行原样装（works-now）。Auth != nil
// 钉死原始行缺失/写错的已核验认证（无 header 的静态 token remote，或 stdio token env）。
type catalogEntry struct {
	Slug string       `json:"slug"`
	Auth *authOverlay `json:"auth,omitempty"`
}

// authOverlay is the pinned install+auth for one server. Transport "remote" rewrites the
// endpoint+header (and forces Plan onto the remote); "stdio" optionally pins the launch package
// (when the raw registry row's bare name doesn't resolve to a runtime) and/or marks the token env
// required.
//
// authOverlay 是一条 server 钉死的安装+认证。Transport "remote" 重写端点+header（并强制 Plan 走 remote）；
// "stdio" 可钉死启动 package（原始 registry 行的裸名解析不出 runtime 时）和/或把 token env 标为必填。
type authOverlay struct {
	Transport     string          `json:"transport"` // remote | stdio | oauth
	URL           string          `json:"url,omitempty"`
	TransportType string          `json:"transportType,omitempty"` // sse | streamable-http
	Header        *overlayHeader  `json:"header,omitempty"`
	Package       *overlayPackage `json:"package,omitempty"` // stdio: pin the launch package
	Env           *overlayEnv     `json:"env,omitempty"`     // static-token env, or oauth per-tenant URL input
	// oauth, non-DCR providers: the user-supplied OAuth client they registered themselves.
	// oauth、无 DCR 提供方：用户自行注册并给出的 OAuth 客户端。
	ClientIDEnv     *overlayEnv `json:"clientIdEnv,omitempty"`
	ClientSecretEnv *overlayEnv `json:"clientSecretEnv,omitempty"`
}

type overlayHeader struct {
	Name   string `json:"name"`
	Value  string `json:"value"` // carries a {ENVNAME} placeholder matching Env.Name
	Secret bool   `json:"secret"`
}

type overlayPackage struct {
	RuntimeHint string   `json:"runtimeHint"` // npx | uvx | docker | dnx
	Name        string   `json:"name"`        // package/image name
	Args        []string `json:"args,omitempty"`
}

type overlayEnv struct {
	Name        string `json:"name"`
	Description string `json:"description"` // shown at install — includes where the user creates the token
	Secret      bool   `json:"secret"`
}

// parsedCatalog is the embedded catalog parsed once. A malformed compiled-in asset is a build
// bug (TestCuratedCatalog_EmbeddedParses guards it) → panic, like template.Must.
//
// parsedCatalog 是内嵌 catalog 解析一次的结果。编译进二进制的资产坏掉 = 构建 bug（有测试守）→ panic，
// 同 template.Must。
var parsedCatalog = mustParseCatalog()

func mustParseCatalog() (entries []catalogEntry) {
	var data struct {
		Servers []catalogEntry `json:"servers"`
	}
	if err := json.Unmarshal(curatedCatalog, &data); err != nil {
		panic(fmt.Sprintf("mcp: embedded catalog.json is malformed: %v", err))
	}
	return data.Servers
}

// CuratedCatalog decorates a RegistrySource: it exposes ONLY whitelisted servers and applies
// each one's pinned auth overlay. This is the marketplace's source of truth — every listed
// server has been vetted to work with code alone.
//
// CuratedCatalog 装饰一个 RegistrySource：只暴露白名单 server 并套上各自钉死的 auth 覆盖。这是市场的
// 事实源——每个列出的 server 都已核验「纯代码即可用」。
type CuratedCatalog struct {
	under    mcpdomain.RegistrySource
	fallback map[string]mcpdomain.RegistryEntry // embedded snapshot, keyed by slug — pins presence
	bySlug   map[string]catalogEntry
	order    []string
}

// NewCuratedCatalog wraps under with the embedded whitelist. The whitelist + the embedded snapshot
// fallback are parsed at construction, so it never fails. The fallback makes the available set
// DETERMINISTIC: a whitelisted server stays installable even if the live registry feed drops it
// (the snapshot carries every vetted server) — the live feed is only preferred for freshness.
//
// NewCuratedCatalog 用内嵌白名单包住 under。白名单 + 内嵌 snapshot 兜底在构造时解析，故永不失败。兜底使可用集
// 确定：白名单 server 即使被 live registry feed 丢了也仍可装（snapshot 持有每个已核验 server）——live feed 只为保鲜优先。
func NewCuratedCatalog(under mcpdomain.RegistrySource) *CuratedCatalog {
	bySlug := make(map[string]catalogEntry, len(parsedCatalog))
	order := make([]string, 0, len(parsedCatalog))
	for _, e := range parsedCatalog {
		bySlug[e.Slug] = e
		order = append(order, e.Slug)
	}
	fallback := map[string]mcpdomain.RegistryEntry{}
	if snap, err := parseGitHub(embeddedSnapshot); err == nil {
		for _, e := range snap {
			fallback[e.Name] = e
		}
	}
	return &CuratedCatalog{under: under, fallback: fallback, bySlug: bySlug, order: order}
}

var _ mcpdomain.RegistrySource = (*CuratedCatalog)(nil)

// List returns only whitelisted entries (in catalog order), each with its auth overlay applied.
// An entry absent upstream is skipped — the embedded snapshot guarantees all are present, so
// this only bites if a future catalog adds a slug the registry feed doesn't carry.
//
// List 只返回白名单条目（按 catalog 顺序）、各套 auth 覆盖。上游缺失的条目跳过——内嵌 snapshot 保证全在，
// 故只有未来 catalog 加了 registry feed 没有的 slug 才触发。
func (c *CuratedCatalog) List(ctx context.Context) ([]mcpdomain.RegistryEntry, error) {
	idx := make(map[string]mcpdomain.RegistryEntry)
	if all, err := c.under.List(ctx); err == nil {
		for _, e := range all {
			idx[e.Name] = e
		}
	}
	out := make([]mcpdomain.RegistryEntry, 0, len(c.order))
	for _, slug := range c.order {
		raw, ok := idx[slug]
		if !ok {
			if raw, ok = c.fallback[slug]; !ok {
				continue
			}
		}
		applyOverlay(&raw, c.bySlug[slug])
		out = append(out, raw)
	}
	return out, nil
}

// Get returns a whitelisted entry with its overlay; a non-whitelisted slug is "not found" (the
// marketplace must not let an unvetted server be installed).
//
// Get 返回套了覆盖的白名单条目；非白名单 slug 即「未找到」（市场绝不让未核验 server 被装）。
func (c *CuratedCatalog) Get(ctx context.Context, name string) (*mcpdomain.RegistryEntry, error) {
	ce, ok := c.bySlug[name]
	if !ok {
		return nil, fmt.Errorf("mcpcatalog.Get %s: %w", name, mcpdomain.ErrRegistryEntryNotFound)
	}
	raw, err := c.under.Get(ctx, name)
	if err != nil || raw == nil {
		fb, ok := c.fallback[name]
		if !ok {
			return nil, fmt.Errorf("mcpcatalog.Get %s: %w", name, mcpdomain.ErrRegistryEntryNotFound)
		}
		raw = &fb
	}
	applyOverlay(raw, ce)
	return raw, nil
}

// applyOverlay pins the verified auth onto a raw registry entry. A static-token REMOTE rewrites
// Remotes to exactly our endpoint+header AND clears Packages (Plan prefers packages — we must
// force the vetted remote). A static-token STDIO marks the token env required on every package
// (so Plan surfaces it → missingEnv enforces it → no silent unauthenticated install). Membership-
// only entries (works-now) pass through untouched.
//
// applyOverlay 把已核验认证钉到原始 registry 条目上。静态 token REMOTE 把 Remotes 改写成精确的端点+header
// 并清空 Packages（Plan 偏好 package——必须强制走核验过的 remote）；静态 token STDIO 把 token env 标为
// 每个 package 必填（Plan 暴露它 → missingEnv 强制 → 不会静默无认证安装）。仅成员（works-now）原样透传。
func applyOverlay(e *mcpdomain.RegistryEntry, ce catalogEntry) {
	a := ce.Auth
	if a == nil {
		return
	}
	switch a.Transport {
	case "remote":
		tt := a.TransportType
		if tt == "" {
			tt = mcpdomain.TransportStreamableHTTP
		}
		rem := mcpdomain.Remote{Transport: tt, URL: a.URL}
		if a.Header != nil {
			rem.Headers = []mcpdomain.Header{{Name: a.Header.Name, Value: a.Header.Value, IsSecret: a.Header.Secret}}
		}
		e.Remotes = []mcpdomain.Remote{rem}
		e.Packages = nil
	case "oauth":
		// OAuth 2.1 + DCR endpoint — no static credential; install runs the interactive flow. An
		// Env overlay marks a per-tenant templated URL the user must supply (URLEnv).
		// OAuth 2.1 + DCR 端点——无静态凭据；安装走交互流程。Env 覆盖标记每租户模板 URL（URLEnv，用户填）。
		tt := a.TransportType
		if tt == "" {
			tt = mcpdomain.TransportStreamableHTTP
		}
		rem := mcpdomain.Remote{Transport: tt, URL: a.URL, Auth: mcpdomain.AuthOAuth}
		if a.Env != nil {
			rem.URLEnv = &mcpdomain.EnvVar{Name: a.Env.Name, Description: a.Env.Description, IsSecret: a.Env.Secret}
		}
		if a.ClientIDEnv != nil {
			rem.ClientIDEnv = &mcpdomain.EnvVar{Name: a.ClientIDEnv.Name, Description: a.ClientIDEnv.Description, IsSecret: a.ClientIDEnv.Secret}
		}
		if a.ClientSecretEnv != nil {
			rem.ClientSecretEnv = &mcpdomain.EnvVar{Name: a.ClientSecretEnv.Name, Description: a.ClientSecretEnv.Description, IsSecret: a.ClientSecretEnv.Secret}
		}
		e.Remotes = []mcpdomain.Remote{rem}
		e.Packages = nil
	case "stdio":
		var ev *mcpdomain.EnvVar
		if a.Env != nil {
			ev = &mcpdomain.EnvVar{Name: a.Env.Name, Description: a.Env.Description, IsSecret: a.Env.Secret}
		}
		// A pinned package replaces the raw row's package(s) — used when the upstream bare name
		// doesn't resolve to a runtime (e.g. the Snyk CLI's `snyk mcp -t stdio`).
		// 钉死的 package 替换原始行的 package——上游裸名解析不出 runtime 时用（如 Snyk CLI 的 `snyk mcp -t stdio`）。
		if a.Package != nil {
			pkg := mcpdomain.Package{RuntimeHint: a.Package.RuntimeHint, Name: a.Package.Name, Args: a.Package.Args}
			if ev != nil {
				pkg.EnvVars = []mcpdomain.EnvVar{*ev}
			}
			e.Packages = []mcpdomain.Package{pkg}
			e.Remotes = nil
			return
		}
		if ev == nil {
			return
		}
		for i := range e.Packages {
			if !hasEnv(e.Packages[i].EnvVars, ev.Name) {
				e.Packages[i].EnvVars = append(e.Packages[i].EnvVars, *ev)
			}
		}
	}
}

func hasEnv(evs []mcpdomain.EnvVar, name string) bool {
	for _, ev := range evs {
		if ev.Name == name {
			return true
		}
	}
	return false
}
