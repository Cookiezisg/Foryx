package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	mcpapp "github.com/sunweilin/anselm/backend/internal/app/mcp"
	toolapp "github.com/sunweilin/anselm/backend/internal/app/tool"
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
	return json.RawMessage(`{"type":"object","properties":{}}`)
}
func (t *ListMarketplace) ValidateInput(json.RawMessage) error { return nil }
func (t *ListMarketplace) Description() string {
	return "Browse the MCP server marketplace (the GitHub MCP Registry). Returns installable servers — each with its full name, description, runtime, and the environment variables you must provide. To install one, call install_mcp_server with its name."
}

type marketView struct {
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Runtime     string    `json:"runtime"` // node|python|docker|dotnet|remote
	RequiredEnv []envView `json:"requiredEnv,omitempty"`
}

type envView struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

func (t *ListMarketplace) Execute(ctx context.Context, _ string) (string, error) {
	entries, err := t.svc.ListRegistry(ctx)
	if err != nil {
		return "", fmt.Errorf("list_mcp_marketplace: %w", err)
	}
	views := make([]marketView, 0, len(entries))
	for _, e := range entries {
		plan, ok := e.Plan()
		if !ok {
			continue // unsupported runtime + no remote → can't install, hide it
		}
		v := marketView{Name: e.Name, Description: e.Description, Runtime: plan.Runtime}
		if plan.Remote {
			v.Runtime = "remote"
		}
		for _, ev := range plan.EnvVars {
			v.RequiredEnv = append(v.RequiredEnv, envView{Name: ev.Name, Description: ev.Description})
		}
		views = append(views, v)
	}
	return toolapp.ToJSON(map[string]any{"servers": views, "count": len(views)}), nil
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
