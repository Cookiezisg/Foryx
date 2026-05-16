// Package mcp is the domain layer for Model Context Protocol integration.
//
// Package mcp 是 Model Context Protocol 集成的 domain 层。
package mcp

import (
	"encoding/json"
	"errors"
	"time"
)

const (
	StatusDisconnected = "disconnected"
	StatusConnecting   = "connecting"
	StatusReady        = "ready"
	StatusDegraded     = "degraded"
	StatusFailed       = "failed"
)

// IsCallable reports whether status permits tools/call (ready or degraded).
//
// IsCallable 报告 status 是否允许 tools/call（ready 或 degraded）。
func IsCallable(status string) bool {
	return status == StatusReady || status == StatusDegraded
}

// ServerConfig is one entry in ~/.forgify/mcp.json (Claude Desktop compatible schema).
//
// ServerConfig 是 ~/.forgify/mcp.json 的一条（Claude Desktop 兼容 schema）。
type ServerConfig struct {
	Name       string            `json:"name"`
	Command    string            `json:"command"`
	Args       []string          `json:"args,omitempty"`
	Env        map[string]string `json:"env,omitempty"`
	TimeoutSec int               `json:"timeoutSec,omitempty"`
}

// ToolDef is one tool advertised by an MCP server (cached from tools/list).
//
// ToolDef 是 MCP server 通告的一个工具（tools/list 缓存）。
type ToolDef struct {
	ServerName  string          `json:"serverName"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

// ServerStatus is the live runtime state of one MCP server (in-process only).
//
// ServerStatus 是单个 MCP server 的实时运行态（仅进程内，无 DB）。
type ServerStatus struct {
	Name                string     `json:"name"`
	Status              string     `json:"status"`
	ConnectedAt         *time.Time `json:"connectedAt,omitempty"`
	LastError           string     `json:"lastError,omitempty"`
	LastErrorAt         *time.Time `json:"lastErrorAt,omitempty"`
	LastSuccessAt       *time.Time `json:"lastSuccessAt,omitempty"`
	ConsecutiveFailures int        `json:"consecutiveFailures"`
	TotalCalls          int64      `json:"totalCalls"`
	TotalFailures       int64      `json:"totalFailures"`
	Tools               []ToolDef  `json:"tools"`
}

// HealthResult is what Service.HealthCheck returns without mutating ServerStatus.
//
// HealthResult 是 Service.HealthCheck 的返回，不改 ServerStatus 状态。
type HealthResult struct {
	ServerName string    `json:"serverName"`
	Healthy    bool      `json:"healthy"`
	LatencyMs  int       `json:"latencyMs"`
	ToolCount  int       `json:"toolCount"`
	Error      string    `json:"error,omitempty"`
	CheckedAt  time.Time `json:"checkedAt"`
}

var (
	ErrServerNotFound     = errors.New("mcp: server not found")
	ErrServerNotConnected = errors.New("mcp: server not connected")
	ErrToolNotFound       = errors.New("mcp: tool not found on server")
	ErrToolCallFailed     = errors.New("mcp: tool call failed")
	ErrToolCallTimeout    = errors.New("mcp: tool call timeout")

	ErrRegistryEntryNotFound = errors.New("mcp: registry entry not found")
	ErrRequiredEnvMissing    = errors.New("mcp: required env variables not provided")
	ErrRequiredArgsMissing   = errors.New("mcp: required args not provided")
	ErrInstallFailed         = errors.New("mcp: install command failed")
)
