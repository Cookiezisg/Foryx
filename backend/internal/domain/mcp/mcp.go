// Package mcp is the domain layer for Model Context Protocol servers — the protocol
// bridge to external tool ecosystems (GitHub, Slack, Notion, ...). A server is a
// CONTAINER entity: it holds N callable tools, runs as a resident process (stdio) or a
// remote connection (HTTP/SSE), and is workspace-isolated like handler.
//
// Package mcp 是 MCP server 的 domain 层——接外部工具生态（GitHub/Slack/Notion…）的协议网桥。
// server 是容器实体：持有 N 个可调工具，以常驻进程（stdio）或远程连接（HTTP/SSE）运行，
// 像 handler 一样 workspace 隔离。
package mcp

import (
	"context"
	"encoding/json"
	"time"

	errorspkg "github.com/sunweilin/anselm/backend/internal/pkg/errors"
)

// Transport kinds. stdio = local subprocess; the other two = remote endpoints.
//
// 传输种类。stdio = 本地子进程；另两种 = 远程端点。
const (
	TransportStdio          = "stdio"
	TransportSSE            = "sse"
	TransportStreamableHTTP = "streamable-http"
)

// Runtime kinds for stdio servers — which sandbox env to provision (install) and which
// launcher to resolve (run). Remote servers have no runtime.
//
// stdio server 的 runtime 种类——决定装哪个 sandbox env（install）、解析哪个启动器（run）。
// remote server 无 runtime。
const (
	RuntimeNode   = "node"
	RuntimePython = "python"
	RuntimeDocker = "docker"
	RuntimeDotnet = "dotnet"
)

// Server status — the in-process runtime state; NEVER persisted (no health-history table).
//
// server 状态——进程内运行态；永不落盘（无 health-history 表）。
const (
	StatusDisconnected = "disconnected"
	StatusConnecting   = "connecting"
	StatusReady        = "ready"
	StatusDegraded     = "degraded"
	StatusFailed       = "failed"
)

// Source records how a server got installed (provenance, not behaviour).
//
// Source 记录 server 怎么装进来的（来源，不影响行为）。
const (
	SourceRegistry = "registry"
	SourceManual   = "manual"
	SourceImport   = "import"
)

// DegradedThreshold: consecutive call failures that flip a ready server to degraded.
//
// DegradedThreshold：连续调用失败多少次把 ready server 翻成 degraded。
const DegradedThreshold = 3

// IsCallable reports whether status permits tools/call (ready or degraded — degraded
// still serves, it's just a soft warning).
//
// IsCallable 报告 status 是否允许 tools/call（ready 或 degraded——degraded 仍可服务，只是软警告）。
func IsCallable(status string) bool {
	return status == StatusReady || status == StatusDegraded
}

// Server is one installed MCP server — the persisted row in mcp_servers. URL != "" means
// remote (no runtime/command); otherwise stdio (runtime + command launched via sandbox).
// Env + Headers carry secrets and are encrypted at rest (config_enc): the domain holds
// them in plaintext, the store encrypts on Save / decrypts on Get.
//
// Server 是一个已装 MCP server——mcp_servers 表的一行。URL != "" 为 remote（无 runtime/command），
// 否则 stdio（runtime + command 经 sandbox 启动）。Env + Headers 含 secret，落盘加密（config_enc）：
// domain 持明文，store 在 Save 时加密 / Get 时解密。
type Server struct {
	ID          string
	WorkspaceID string
	Name        string
	Description string

	Transport string   // stdio | sse | streamable-http
	Runtime   string   // node|python|docker|dotnet (stdio only)
	Command   string   // npx|uvx|dnx | image (stdio only; docker → image)
	Args      []string //

	URL     string            // remote only
	Headers map[string]string // remote only
	OAuth   *OAuthCredentials // remote only: set when the server authenticates via OAuth 2.1 (Tier 2)

	Env        map[string]string
	TimeoutSec int

	Source     string // registry|manual|import
	RegistryID string // e.g. "com.microsoft/azure" — for "check updates"; empty for manual

	CreatedAt time.Time
	UpdatedAt time.Time
}

// OAuthCredentials is one remote server's OAuth 2.1 grant — the DCR-registered client plus the
// live token pair, persisted (encrypted in config_enc, like Env/Headers). The access token is
// injected as a Bearer header; when it nears Expiry the refresh token mints a new one and the
// bundle is re-persisted. A blank RefreshToken with an expired AccessToken ⇒ re-authorization.
//
// OAuthCredentials 是一个 remote server 的 OAuth 2.1 授权——DCR 注册的客户端 + 实时 token 对，持久化
// （加密在 config_enc，同 Env/Headers）。access token 作 Bearer 注入；临 Expiry 时 refresh token 换新、
// 整束重存。RefreshToken 空且 AccessToken 过期 ⇒ 需重新授权。
type OAuthCredentials struct {
	Resource            string    `json:"resource"`            // RFC 8707 resource indicator = the server's canonical URL
	AuthorizationServer string    `json:"authorizationServer"` // discovered issuer
	TokenEndpoint       string    `json:"tokenEndpoint"`       // where refresh posts
	ClientID            string    `json:"clientId"`            // from DCR
	ClientSecret        string    `json:"clientSecret"`        // empty for a public (PKCE) client
	Scopes              []string  `json:"scopes,omitempty"`
	AccessToken         string    `json:"accessToken"`
	RefreshToken        string    `json:"refreshToken"`
	Expiry              time.Time `json:"expiry"` // absolute; zero = unknown
}

// Expired reports whether the access token is at/within skew of expiry (skew leaves refresh
// headroom). A zero Expiry means "unknown" → treated as not expired.
//
// Expired 报告 access token 是否到/进入 skew 内（skew 给刷新留余量）。零 Expiry = 未知 → 视为未过期。
func (c *OAuthCredentials) Expired(now time.Time, skew time.Duration) bool {
	if c.Expiry.IsZero() {
		return false
	}
	return !now.Add(skew).Before(c.Expiry)
}

// IsRemote reports whether this is a remote endpoint (vs a stdio subprocess).
//
// IsRemote 报告这是否为远程端点（相对 stdio 子进程）。
func (s *Server) IsRemote() bool { return s.URL != "" }

// IsOAuth reports whether this remote server authenticates via the OAuth 2.1 flow.
//
// IsOAuth 报告该 remote server 是否走 OAuth 2.1 流程认证。
func (s *Server) IsOAuth() bool { return s.OAuth != nil }

// ToolDef is one tool a server advertises (cached from tools/list). InputSchema is the
// MCP JSON Schema, reused VERBATIM as the wrapped tool's Parameters — we don't author it.
//
// ToolDef 是 server 通告的一个工具（tools/list 缓存）。InputSchema 是 MCP JSON Schema，
// 原样复用为包装工具的 Parameters——我们不造。
type ToolDef struct {
	ServerName  string          `json:"serverName"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

// ServerStatus is the live runtime state of one server (in-process map value, no DB row).
//
// ServerStatus 是单个 server 的实时运行态（进程内 map 值，无 DB 行）。
type ServerStatus struct {
	ID                  string     `json:"id"`
	Name                string     `json:"name"`
	Status              string     `json:"status"`
	ConnectedAt         *time.Time `json:"connectedAt,omitempty"`
	LastError           string     `json:"lastError,omitempty"`
	LastErrorAt         *time.Time `json:"lastErrorAt,omitempty"`
	ConsecutiveFailures int        `json:"consecutiveFailures"`
	TotalCalls          int64      `json:"totalCalls"`
	TotalFailures       int64      `json:"totalFailures"`
	Tools               []ToolDef  `json:"tools"`
}

// Repository persists mcp_servers + the mcp_calls log — workspace-scoped automatically via orm
// (D2). The store encrypts/decrypts Env + Headers around the server calls.
//
// Repository 持久化 mcp_servers + mcp_calls log——经 orm 自动 workspace 隔离（D2）。store 在 server
// 调用周围加解密 Env + Headers。
type Repository interface {
	Save(ctx context.Context, s *Server) error
	GetByID(ctx context.Context, id string) (*Server, error)
	GetByName(ctx context.Context, name string) (*Server, error)
	List(ctx context.Context) ([]*Server, error)
	Delete(ctx context.Context, id string) error
	CallRepository
}

// Error dictionary. KindBadGateway (502) for upstream MCP/install failures, KindUnavailable
// (503) for a down subprocess — same split handler uses.
//
// 错误字典。KindBadGateway（502）给上游 MCP/安装失败，KindUnavailable（503）给挂掉的子进程
// ——与 handler 同款划分。
var (
	ErrServerNotFound     = errorspkg.New(errorspkg.KindNotFound, "MCP_SERVER_NOT_FOUND", "mcp server not found")
	ErrServerNotConnected = errorspkg.New(errorspkg.KindUnavailable, "MCP_SERVER_DOWN", "mcp server not connected")
	// ErrInvalidCallStatus: a list filter passed a status outside CallStatuses — 422 with the allowed set
	// in Details so the caller self-corrects instead of silently getting an empty page (F168-M2).
	// ErrInvalidCallStatus：list 过滤传了 CallStatuses 外的状态——返 422、Details 带合法集自纠（F168-M2）。
	ErrInvalidCallStatus     = errorspkg.New(errorspkg.KindUnprocessable, "MCP_CALL_INVALID_STATUS", "mcp call status filter must be one of: ok, failed, cancelled, timeout")
	ErrToolNotFound          = errorspkg.New(errorspkg.KindNotFound, "MCP_TOOL_NOT_FOUND", "mcp tool not found on server")
	ErrToolCallFailed        = errorspkg.New(errorspkg.KindBadGateway, "MCP_RPC_ERROR", "mcp tool call failed")
	ErrToolCallTimeout       = errorspkg.New(errorspkg.KindGatewayTimeout, "MCP_TOOL_TIMEOUT", "mcp tool call timed out")
	ErrNameConflict          = errorspkg.New(errorspkg.KindConflict, "MCP_NAME_CONFLICT", "mcp server name already exists")
	ErrInstallFailed         = errorspkg.New(errorspkg.KindBadGateway, "MCP_INSTALL_FAILED", "mcp server install failed")
	ErrEnvMissing            = errorspkg.New(errorspkg.KindUnprocessable, "MCP_ENV_MISSING", "required environment variables missing")
	ErrRegistryEntryNotFound = errorspkg.New(errorspkg.KindNotFound, "MCP_REGISTRY_NOT_FOUND", "mcp registry entry not found")
	ErrNoRunnablePackage     = errorspkg.New(errorspkg.KindUnprocessable, "MCP_NO_RUNNABLE_PACKAGE", "no package with a supported runtime (node/python/docker/dotnet) and no remote endpoint")
	ErrCallNotFound          = errorspkg.New(errorspkg.KindNotFound, "MCP_CALL_NOT_FOUND", "mcp call not found")

	// OAuth (remote servers whose auth is an OAuth 2.1 + PKCE + DCR flow, Tier 2). Discovery /
	// registration / token / authorize are upstream-facing (502); NotSupported is a config dead-end
	// (422); ReauthRequired (401) tells the user a stored grant expired and they must authorize again.
	//
	// OAuth（认证走 OAuth 2.1 + PKCE + DCR 的 remote server，档 2）。发现/注册/token/授权 面向上游（502）；
	// NotSupported 是配置死路（422）；ReauthRequired（401）告诉用户已存授权过期、须重新授权。
	ErrOAuthDiscovery      = errorspkg.New(errorspkg.KindBadGateway, "MCP_OAUTH_DISCOVERY_FAILED", "mcp oauth discovery failed")
	ErrOAuthRegistration   = errorspkg.New(errorspkg.KindBadGateway, "MCP_OAUTH_REGISTRATION_FAILED", "mcp oauth dynamic client registration failed")
	ErrOAuthToken          = errorspkg.New(errorspkg.KindBadGateway, "MCP_OAUTH_TOKEN_FAILED", "mcp oauth token request failed")
	ErrOAuthAuthorize      = errorspkg.New(errorspkg.KindBadGateway, "MCP_OAUTH_AUTHORIZE_FAILED", "mcp oauth authorization flow failed")
	ErrOAuthNotSupported   = errorspkg.New(errorspkg.KindUnprocessable, "MCP_OAUTH_NOT_SUPPORTED", "mcp server requires oauth but its authorization server does not support dynamic client registration")
	ErrOAuthReauthRequired = errorspkg.New(errorspkg.KindUnauthorized, "MCP_OAUTH_REAUTH_REQUIRED", "mcp server oauth grant expired or revoked; re-authorization required")
)
