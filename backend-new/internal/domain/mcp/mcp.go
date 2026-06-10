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

	errorsdomain "github.com/sunweilin/forgify/backend/internal/domain/errors"
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

	Env        map[string]string
	TimeoutSec int

	Source     string // registry|manual|import
	RegistryID string // e.g. "com.microsoft/azure" — for "check updates"; empty for manual

	CreatedAt time.Time
	UpdatedAt time.Time
}

// IsRemote reports whether this is a remote endpoint (vs a stdio subprocess).
//
// IsRemote 报告这是否为远程端点（相对 stdio 子进程）。
func (s *Server) IsRemote() bool { return s.URL != "" }

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
	ErrServerNotFound        = errorsdomain.New(errorsdomain.KindNotFound, "MCP_SERVER_NOT_FOUND", "mcp server not found")
	ErrServerNotConnected    = errorsdomain.New(errorsdomain.KindUnavailable, "MCP_SERVER_DOWN", "mcp server not connected")
	ErrToolNotFound          = errorsdomain.New(errorsdomain.KindNotFound, "MCP_TOOL_NOT_FOUND", "mcp tool not found on server")
	ErrToolCallFailed        = errorsdomain.New(errorsdomain.KindBadGateway, "MCP_RPC_ERROR", "mcp tool call failed")
	ErrToolCallTimeout       = errorsdomain.New(errorsdomain.KindGatewayTimeout, "MCP_TOOL_TIMEOUT", "mcp tool call timed out")
	ErrNameConflict          = errorsdomain.New(errorsdomain.KindConflict, "MCP_NAME_CONFLICT", "mcp server name already exists")
	ErrInstallFailed         = errorsdomain.New(errorsdomain.KindBadGateway, "MCP_INSTALL_FAILED", "mcp server install failed")
	ErrEnvMissing            = errorsdomain.New(errorsdomain.KindUnprocessable, "MCP_ENV_MISSING", "required environment variables missing")
	ErrRegistryEntryNotFound = errorsdomain.New(errorsdomain.KindNotFound, "MCP_REGISTRY_NOT_FOUND", "mcp registry entry not found")
	ErrNoRunnablePackage     = errorsdomain.New(errorsdomain.KindUnprocessable, "MCP_NO_RUNNABLE_PACKAGE", "no package with a supported runtime (node/python/docker/dotnet) and no remote endpoint")
	ErrCallNotFound          = errorsdomain.New(errorsdomain.KindNotFound, "MCP_CALL_NOT_FOUND", "mcp call not found")
)
