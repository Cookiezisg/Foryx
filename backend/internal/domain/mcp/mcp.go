// Package mcp is the domain layer for Model Context Protocol integration.
// MCP lets external processes expose tools over stdio JSON-RPC; Forgify
// connects to user-configured servers (per ~/.forgify/mcp.json) and
// surfaces them through the search_mcp + call_mcp system tools rather
// than flat-registering every server's tools (avoids the 70k-token
// startup hit Claude Code etc. take).
//
// V1 scope (mcp.md §3):
//   - stdio transport only (no Streamable HTTP)
//   - official modelcontextprotocol/go-sdk v1.x
//   - configuration source = single ~/.forgify/mcp.json (no project-level)
//   - "in mcp.json = enabled"; no separate enable/disable bit
//   - no auto-restart on subprocess crash (loud failure beats silent flap)
//   - no OAuth (stdio spec doesn't recommend it; env carries secrets)
//
// Layering (per CLAUDE.md §S13):
//
//	internal/domain/mcp/                — entities + 10 sentinels (this file)
//	internal/domain/mcp/registry.go     — RegistryEntry value types (D5-2)
//	internal/app/mcp/registry.go        — built-in 6 entries + Get/List + GOOS filter (D5-2)
//	internal/infra/mcp/config.go        — ~/.forgify/mcp.json Load/Save/Merge (D5-3)
//	internal/infra/mcp/                 — stdio Client wrapper (D6)
//	internal/app/mcp/mcp.go             — Service: Connect/Disconnect/Search/CallTool (D6)
//	internal/app/tool/mcp/              — search_mcp + call_mcp system tools (D6)
//
// Aliases:
//
//	mcpdomain "…/internal/domain/mcp"
//	mcpapp    "…/internal/app/mcp"
//	mcpinfra  "…/internal/infra/mcp"
//	mcptool   "…/internal/app/tool/mcp"
//
// Package mcp 是 Model Context Protocol 集成的 domain 层。MCP 让外部进程
// 通过 stdio JSON-RPC 暴露工具；Forgify 按 ~/.forgify/mcp.json 连用户配置
// 的 server，通过 search_mcp + call_mcp 两个系统工具暴露给 LLM——而非
// flat 注册每个 server 的工具（避开 Claude Code 等 70k token 启动开销）。
//
// V1 范围（mcp.md §3）：仅 stdio；官方 go-sdk；单一 ~/.forgify/mcp.json；
// "在配置中 = 启用"；不自动重启；不走 OAuth。
package mcp

import (
	"encoding/json"
	"errors"
	"time"
)

// ── Status enum ──────────────────────────────────────────────────────

// ServerStatus.Status five-state machine. ready is the only "fully usable"
// state; degraded means recent tools/call failures ≥ 3 (still callable but
// UI warns — auto-recovers on next success); failed means we cannot call
// at all (handshake never completed, or subprocess exited).
//
// ServerStatus.Status 五态。ready 是唯一"完全可用"；degraded 表示近期
// tools/call 连续失败 ≥ 3（仍可调，UI 警示——下次成功自动恢复）；
// failed 完全不可用（握手没成 / 子进程退出）。
const (
	StatusDisconnected = "disconnected"
	StatusConnecting   = "connecting"
	StatusReady        = "ready"
	StatusDegraded     = "degraded"
	StatusFailed       = "failed"
)

// IsCallable reports whether a server's status permits tools/call. Both
// ready and degraded qualify — degraded servers still serve, just with
// the "this might be flaky" warning attached.
//
// IsCallable 报告 status 是否允许 tools/call。ready 与 degraded 都允许
// ——degraded 仍服务，只是带"可能抽风"的警示。
func IsCallable(status string) bool {
	return status == StatusReady || status == StatusDegraded
}

// ── ServerConfig ─────────────────────────────────────────────────────

// ServerConfig is one entry in ~/.forgify/mcp.json (Claude Desktop-
// compatible schema). Name is the unique key within the file. Command +
// Args are the subprocess invocation; Env injects per-server secrets
// (GitHub PAT, etc.). TimeoutSec overrides the per-call default 30s
// (mcp.md §5.7 — high-priority over RegistryEntry.DefaultTimeoutSec
// over the global 30s fallback).
//
// ServerConfig 是 ~/.forgify/mcp.json 的一条（Claude Desktop 兼容 schema）。
// Name 文件内唯一；Command+Args 是子进程命令；Env 注入 per-server secret
// （GitHub PAT 等）；TimeoutSec 覆盖 per-call 默认 30s（mcp.md §5.7：
// 用户配置 > Registry 默认 > 全局 30s 兜底）。
type ServerConfig struct {
	Name       string            `json:"name"`
	Command    string            `json:"command"`
	Args       []string          `json:"args,omitempty"`
	Env        map[string]string `json:"env,omitempty"`
	TimeoutSec int               `json:"timeoutSec,omitempty"`
}

// ── ToolDef ──────────────────────────────────────────────────────────

// ToolDef is one tool advertised by an MCP server (cached from tools/list).
// Forwarded verbatim to the LLM by search_mcp; LLM uses InputSchema to
// build the args for the subsequent call_mcp invocation. Name has no
// "mcp__" prefix here — namespacing only happens at the LLM-facing
// search_mcp result + at call_mcp dispatch time.
//
// ToolDef 是 MCP server 通告的一个工具（tools/list 缓存）。search_mcp 原样
// 转发给 LLM；LLM 用 InputSchema 构造下一步 call_mcp 的 args。这里 Name
// 不带 "mcp__" 前缀——命名空间仅在 search_mcp 结果 + call_mcp 派发时加。
type ToolDef struct {
	ServerName  string          `json:"serverName"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

// ── ServerStatus ─────────────────────────────────────────────────────

// ServerStatus is the live runtime state of one MCP server. Persisted
// only in-process (no DB); the SSE event family ships the whole snapshot
// so the UI replaces local state on every change. Health-monitoring
// counters (ConsecutiveFailures / TotalCalls / TotalFailures) drive the
// degraded transition (mcp.md §5.6).
//
// ServerStatus 是单个 MCP server 的实时运行态。仅进程内（无 DB）；SSE
// 事件家族发整快照，UI 收到全量替换。健康监控计数（ConsecutiveFailures
// / TotalCalls / TotalFailures）驱动 degraded 转换（mcp.md §5.6）。
type ServerStatus struct {
	Name                string     `json:"name"`
	Status              string     `json:"status"`
	PID                 int        `json:"pid,omitempty"`
	ConnectedAt         *time.Time `json:"connectedAt,omitempty"`
	LastError           string     `json:"lastError,omitempty"`
	LastErrorAt         *time.Time `json:"lastErrorAt,omitempty"`
	LastSuccessAt       *time.Time `json:"lastSuccessAt,omitempty"`
	ConsecutiveFailures int        `json:"consecutiveFailures"`
	TotalCalls          int64      `json:"totalCalls"`
	TotalFailures       int64      `json:"totalFailures"`
	Tools               []ToolDef  `json:"tools"`
}

// ── HealthResult ─────────────────────────────────────────────────────

// HealthResult is what Service.HealthCheck returns. Returned to a UI
// "Test Connection" button without mutating ServerStatus (so probing
// doesn't accidentally trip the degraded transition). LatencyMs measures
// the tools/list RTT; ToolCount is the size of the response.
//
// HealthResult 是 Service.HealthCheck 的返回。给 UI "Test Connection"
// 按钮用，不改 ServerStatus（避免探测触发 degraded 误判）。LatencyMs 是
// tools/list 往返时间；ToolCount 是响应里的 tool 数量。
type HealthResult struct {
	ServerName string    `json:"serverName"`
	Healthy    bool      `json:"healthy"`
	LatencyMs  int       `json:"latencyMs"`
	ToolCount  int       `json:"toolCount"`
	Error      string    `json:"error,omitempty"`
	CheckedAt  time.Time `json:"checkedAt"`
}

// ── Sentinels (10) ───────────────────────────────────────────────────

// Sentinel errors per mcp.md §4 + §11. Five "runtime" sentinels (Server*
// / Tool*) cover live-call failure paths; five "Registry" sentinels cover
// install-flow validation (RequiredEnv / RequiredArgs / runtime missing
// / install command failed / unknown registry entry).
//
// Sentinel 错误（mcp.md §4 + §11）。5 个 "runtime" sentinel（Server* /
// Tool*）覆盖运行调用失败路径；5 个 "Registry" sentinel 覆盖安装流校验
// （RequiredEnv / RequiredArgs / runtime 缺 / install 命令失败 / 未知
// registry entry）。
var (
	ErrServerNotFound        = errors.New("mcp: server not found")
	ErrServerNotConnected    = errors.New("mcp: server not connected")
	ErrToolNotFound          = errors.New("mcp: tool not found on server")
	ErrToolCallFailed        = errors.New("mcp: tool call failed")
	ErrToolCallTimeout       = errors.New("mcp: tool call timeout")

	ErrRegistryEntryNotFound = errors.New("mcp: registry entry not found")
	ErrRuntimeMissing        = errors.New("mcp: runtime (node/python) not available")
	ErrRequiredEnvMissing    = errors.New("mcp: required env variables not provided")
	ErrRequiredArgsMissing   = errors.New("mcp: required args not provided")
	ErrInstallFailed         = errors.New("mcp: install command failed")
)
