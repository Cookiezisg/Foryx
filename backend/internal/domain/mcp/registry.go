package mcp

import (
	"context"
	"strings"
)

// RegistryEntry is one marketplace listing, mapped from the registry's server.json.
// Packages is an ORDERED list of install options (npm/pypi/oci/nuget…) — install picks
// the first whose runtime we support. Remotes are HTTP/SSE endpoints (no runtime needed).
//
// RegistryEntry 是市场一条，从 registry 的 server.json 映射。Packages 是有序的安装选项
// （npm/pypi/oci/nuget…）——安装挑第一个 runtime 我们支持的。Remotes 是 HTTP/SSE 端点（无需 runtime）。
type RegistryEntry struct {
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Packages    []Package `json:"packages,omitempty"`
	Remotes     []Remote  `json:"remotes,omitempty"`
}

// Package is one install option. RuntimeHint (npx/uvx/docker/dnx) maps to a sandbox
// runtime; Name is the package or image; Args are appended after the launch command.
//
// Package 是一个安装选项。RuntimeHint（npx/uvx/docker/dnx）映射 sandbox runtime；
// Name 是包名或镜像；Args 拼在启动命令之后。
type Package struct {
	RuntimeHint string   `json:"runtimeHint"`
	Name        string   `json:"name"`
	Version     string   `json:"version,omitempty"`
	Args        []string `json:"args,omitempty"`
	EnvVars     []EnvVar `json:"envVars,omitempty"`
}

// Remote is one remote endpoint. Headers may carry "{TOKEN}" placeholders filled at install.
//
// Remote 是一个远程端点。Headers 可含 "{TOKEN}" 占位符，安装时填。
type Remote struct {
	Transport string   `json:"transport"` // sse | streamable-http
	URL       string   `json:"url"`
	Auth      string   `json:"auth,omitempty"`   // "" static header | "oauth" (OAuth 2.1 + PKCE + DCR flow)
	URLEnv    *EnvVar  `json:"urlEnv,omitempty"` // set when URL is templated ("{X}") and the user supplies it (per-tenant endpoints)
	Headers   []Header `json:"headers,omitempty"`
}

// AuthOAuth marks a remote endpoint as OAuth-authenticated (vs a static header).
//
// AuthOAuth 标记 remote 端点走 OAuth 认证（相对静态 header）。
const AuthOAuth = "oauth"

// EnvVar is one env var the user must supply before install; IsSecret drives UI masking.
//
// EnvVar 是用户安装前必填的一个 env 变量；IsSecret 驱动 UI 打码。
type EnvVar struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	IsSecret    bool   `json:"isSecret"`
}

// Header is one remote header; Value may contain a "{TOKEN}" placeholder.
//
// Header 是一个 remote header；Value 可含 "{TOKEN}" 占位符。
type Header struct {
	Name     string `json:"name"`
	Value    string `json:"value"`
	IsSecret bool   `json:"isSecret"`
}

// RegistrySource is the marketplace data source (GitHub registry over HTTP, with an
// embedded snapshot + on-disk cache for offline). Global/public — not workspace-scoped.
//
// RegistrySource 是市场数据源（GitHub registry HTTP，带内嵌 snapshot + 本地缓存兜底离线）。
// 全局公共——不按 workspace 隔离。
type RegistrySource interface {
	List(ctx context.Context) ([]RegistryEntry, error)
	Get(ctx context.Context, name string) (*RegistryEntry, error)
}

// InstallPlan is the resolved way to run an entry: stdio (Runtime/Command/Args) or remote
// (Transport/URL/Headers). EnvVars are the required values the caller must collect.
//
// InstallPlan 是装一条 entry 的解析结果：stdio（Runtime/Command/Args）或 remote
// （Transport/URL/Headers）。EnvVars 是调用方要收集的必填值。
type InstallPlan struct {
	Remote    bool
	OAuth     bool     // remote: authenticate via the OAuth 2.1 flow instead of collecting a static token
	Runtime   string   // stdio: node|python|docker|dotnet
	Command   string   // stdio: npx|uvx|dnx | image
	Args      []string // stdio
	Transport string   // remote: sse|streamable-http
	URL       string   // remote
	Headers   []Header // remote
	EnvVars   []EnvVar // env to fill (from the chosen package, or remote headers)
}

// Plan picks the package with the BEST supported runtime (node>python>docker>dotnet —
// lighter first, so an npm copy wins over a NuGet one and we never pull a .NET SDK we can
// avoid); else the first remote; else ok=false. Centralized + pure so install and tests
// agree on which package wins.
//
// Plan 挑 runtime 最优的 package（node>python>docker>dotnet——轻量优先，有 npm 版就压过 NuGet 版、
// 能不拉 .NET SDK 就不拉）；否则第一个 remote；都没有返 ok=false。集中且纯，使 install 与测试一致。
func (e *RegistryEntry) Plan() (InstallPlan, bool) {
	best := -1
	var plan InstallPlan
	for _, p := range e.Packages {
		rt, ok := runtimeForHint(p.RuntimeHint, p.Name)
		if !ok {
			continue
		}
		if pr := runtimePriority(rt); best == -1 || pr < best {
			cmd, args := launchCommand(rt, p)
			best, plan = pr, InstallPlan{Runtime: rt, Command: cmd, Args: args, EnvVars: p.EnvVars}
		}
	}
	if best != -1 {
		return plan, true
	}
	if len(e.Remotes) > 0 {
		r := e.Remotes[0]
		t := r.Transport
		if t == "" {
			t = TransportStreamableHTTP // registry default when transport_type is blank
		}
		// OAuth endpoints collect no static token — the interactive flow mints one at install. A
		// per-tenant endpoint (templated URL) still needs the user to supply the URL (URLEnv).
		// OAuth 端点不收静态 token——安装时由交互流程铸一个。每租户端点（模板 URL）仍需用户给出 URL（URLEnv）。
		if r.Auth == AuthOAuth {
			var envs []EnvVar
			if r.URLEnv != nil {
				envs = []EnvVar{*r.URLEnv}
			}
			return InstallPlan{Remote: true, OAuth: true, Transport: t, URL: r.URL, EnvVars: envs}, true
		}
		// remote env requirements live in headers' {TOKEN} placeholders → surfaced as EnvVars
		envs := make([]EnvVar, 0, len(r.Headers))
		for _, h := range r.Headers {
			if name := placeholderName(h.Value); name != "" {
				envs = append(envs, EnvVar{Name: name, Description: h.Name, IsSecret: h.IsSecret})
			}
		}
		return InstallPlan{Remote: true, Transport: t, URL: r.URL, Headers: r.Headers, EnvVars: envs}, true
	}
	return InstallPlan{}, false
}

// runtimeForHint maps a registry runtime_hint to a sandbox runtime; falls back to package
// name patterns when the hint is blank (GitHub registry leaves runtime_hint empty often).
//
// runtimeForHint 把 registry runtime_hint 映射到 sandbox runtime；hint 空时按包名模式兜底
// （GitHub registry 常留空 runtime_hint）。
func runtimeForHint(hint, pkgName string) (string, bool) {
	switch hint {
	case "npx", "npm", "node":
		return RuntimeNode, true
	case "uvx", "uv", "pip", "pipx", "python":
		return RuntimePython, true
	case "docker", "oci":
		return RuntimeDocker, true
	case "dnx", "dotnet":
		return RuntimeDotnet, true
	}
	return runtimeForName(pkgName)
}

// runtimeForName infers a runtime from a package name when runtime_hint is blank. Best-effort
// ordering: docker/dotnet markers are unambiguous; npm scoped/-mcp names are the common case.
//
// runtimeForName 在 runtime_hint 空时从包名推断 runtime。尽力的判定序：docker/dotnet 标记无歧义；
// npm scoped/-mcp 名是常见情形。
func runtimeForName(name string) (string, bool) {
	switch {
	case name == "":
		return "", false
	case strings.Contains(name, "ghcr.io"), strings.Contains(name, "docker.io"), strings.HasPrefix(name, "mcr.microsoft.com"):
		return RuntimeDocker, true
	case strings.Contains(name, ".Mcp"), strings.Contains(name, "NuGet"):
		return RuntimeDotnet, true
	case strings.HasPrefix(name, "@"), strings.HasSuffix(name, "-mcp"), strings.HasSuffix(name, "-mcp-server"):
		return RuntimeNode, true
	}
	return "", false
}

// launchCommand builds the (command, args) for a runtime. node/python/dotnet use their
// auto-fetch launchers (npx/uvx/dnx); docker uses the bare image (sandbox wraps `docker run`).
//
// launchCommand 为某 runtime 构造 (command, args)。node/python/dotnet 用各自的自动拉取启动器
// （npx/uvx/dnx）；docker 用裸镜像名（sandbox 负责包 `docker run`）。
func launchCommand(runtime string, p Package) (string, []string) {
	switch runtime {
	case RuntimeNode:
		return "npx", append([]string{"-y", p.Name}, p.Args...)
	case RuntimePython:
		return "uvx", append([]string{p.Name}, p.Args...)
	case RuntimeDotnet:
		return "dnx", append([]string{p.Name, "--yes"}, p.Args...)
	case RuntimeDocker:
		return p.Name, append([]string(nil), p.Args...) // image; sandbox docker wraps it
	}
	return "", nil
}

// placeholderName extracts "X" from a header value "...{X}..." (the env var a remote needs).
//
// placeholderName 从 header 值 "...{X}..." 提取 "X"（remote 需要的 env 变量名）。
func placeholderName(v string) string {
	i := strings.IndexByte(v, '{')
	if i < 0 {
		return ""
	}
	j := strings.IndexByte(v[i+1:], '}')
	if j < 0 {
		return ""
	}
	return v[i+1 : i+1+j]
}

// runtimePriority ranks runtimes for Plan: lower = preferred. node/python (no daemon, fast
// fetch) beat docker (needs a daemon) beat dotnet (heavy SDK).
//
// runtimePriority 给 Plan 排 runtime 优先级：小=优先。node/python（无 daemon、拉取快）优于
// docker（要 daemon）优于 dotnet（重 SDK）。
func runtimePriority(rt string) int {
	switch rt {
	case RuntimeNode:
		return 1
	case RuntimePython:
		return 2
	case RuntimeDocker:
		return 3
	case RuntimeDotnet:
		return 4
	}
	return 99
}
