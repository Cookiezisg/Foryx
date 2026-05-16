package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	mcpapp "github.com/sunweilin/forgify/backend/internal/app/mcp"
	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
	mcpdomain "github.com/sunweilin/forgify/backend/internal/domain/mcp"
)

// InstallMCPServer implements the install_mcp_server system tool.
//
// InstallMCPServer 是 install_mcp_server 系统工具的实现。
type InstallMCPServer struct {
	svc *mcpapp.Service
}

const installMCPServerDescription = `Install an MCP server from the curated marketplace.

The tool runs in two calls. The first call (omit ` + "`confirmed`" + `) returns ` + "`{status:\"needs_confirmation\", suggested_question, required_env, required_args, notes}`" + ` — relay the question and notes to the user via the ask tool to collect any required values. The second call (` + "`confirmed: true`" + ` plus collected ` + "`env`" + ` / ` + "`arguments`" + `) performs the install and returns either a ServerStatus envelope or a structured error (codes: already_installed / missing_required_args / install_failed / handshake_failed).

` + "`name`" + ` is the curated catalog's short slug (pick from list_mcp_marketplace).`

var installMCPServerSchema = json.RawMessage(`{
	"type": "object",
	"properties": {
		"name":      {"type": "string", "description": "Curated catalog short slug (e.g. 'playwright', 'notion'). Pick from list_mcp_marketplace."},
		"confirmed": {"type": "boolean", "description": "Set to true on the second call after user has consented. Phase-1 calls omit this."},
		"env":       {"type": "object", "description": "Map of env-var values for required env entries. Phase 2 only."},
		"arguments": {"type": "object", "description": "Map of arg values for required args. Phase 2 only."}
	},
	"required": ["name"]
}`)

func (t *InstallMCPServer) Name() string                { return "install_mcp_server" }
func (t *InstallMCPServer) Description() string         { return installMCPServerDescription }
func (t *InstallMCPServer) Parameters() json.RawMessage { return installMCPServerSchema }

func (t *InstallMCPServer) IsReadOnly() bool        { return false }
func (t *InstallMCPServer) NeedsReadFirst() bool    { return false }
func (t *InstallMCPServer) RequiresWorkspace() bool { return false }

func (t *InstallMCPServer) ValidateInput(args json.RawMessage) error {
	var a installArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return fmt.Errorf("install_mcp_server: bad args: %w", err)
	}
	if strings.TrimSpace(a.Name) == "" {
		return errors.New("install_mcp_server: name is required")
	}
	return nil
}

// Permission stays Allow — the LLM-driven ask flow provides real user
// consent. Framework-level Ask would pop a UI dialog out-of-band, breaking
// the in-LLM orchestration we want here.
//
// 权限留 Allow —— LLM 驱动的 ask 流程提供真用户同意。框架级 Ask 会弹带外
// UI 对话框，打破我们想要的 in-LLM 编排。
func (t *InstallMCPServer) CheckPermissions(json.RawMessage, toolapp.PermissionMode) toolapp.PermissionResult {
	return toolapp.PermissionAllow
}

type installArgs struct {
	Name      string            `json:"name"`
	Confirmed bool              `json:"confirmed"`
	Env       map[string]string `json:"env"`
	Arguments map[string]string `json:"arguments"`
}

func (t *InstallMCPServer) Execute(ctx context.Context, argsJSON string) (string, error) {
	var args installArgs
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("install_mcp_server: %w", err)
	}

	entry, err := t.svc.GetRegistryEntry(ctx, args.Name)
	if err != nil {
		if errors.Is(err, mcpdomain.ErrRegistryEntryNotFound) {
			return errorJSON("not_in_registry",
				fmt.Sprintf("Server %q not found in marketplace. Use list_mcp_marketplace to discover available servers.", args.Name)), nil
		}
		return "", fmt.Errorf("install_mcp_server: %w", err)
	}

	// Phase 1: no confirmed → return needs_confirmation envelope.
	// 阶段 1：没 confirmed → 返 needs_confirmation 信封。
	if !args.Confirmed {
		return phase1Envelope(entry), nil
	}

	// Phase 2: do the install. The catalog's short Name doubles as the
	// mcp.json key (no separate alias).
	// 阶段 2：真装。catalog 的短 Name 直接作 mcp.json key（无 alias）。
	st, err := t.svc.InstallFromRegistry(ctx, args.Name, args.Env, args.Arguments)
	switch {
	case err == nil:
		return successJSON(st, args.Name), nil
	case errors.Is(err, mcpdomain.ErrAlreadyInstalled):
		return errorJSON("already_installed",
			fmt.Sprintf("A server named %q is already configured. Uninstall it first via uninstall_mcp_server.", args.Name)), nil
	case errors.Is(err, mcpdomain.ErrRequiredEnvMissing):
		return errorJSON("missing_required_args",
			fmt.Sprintf("Missing required env: %s. Ask the user for these values, then retry with env={...}.", err.Error())), nil
	case errors.Is(err, mcpdomain.ErrRequiredArgsMissing):
		return errorJSON("missing_required_args",
			fmt.Sprintf("Missing required args: %s. Ask the user for these values, then retry with arguments={...}.", err.Error())), nil
	case errors.Is(err, mcpdomain.ErrInstallFailed):
		return errorJSON("install_failed", fmt.Sprintf("Install failed: %s", err.Error())), nil
	default:
		return errorJSON("install_failed", err.Error()), nil
	}
}

// phase1Envelope builds the "needs_confirmation" response with a
// human-readable summary + suggested question for the LLM to feed into ask.
//
// phase1Envelope 构造 "needs_confirmation" 响应，带可读 summary + 给 LLM 喂
// 入 ask 的建议问句。
func phase1Envelope(entry *mcpdomain.RegistryEntry) string {
	// Summary line that the LLM can read to understand what it's about to do.
	// LLM 读懂将要做啥的 summary 行。
	summary := fmt.Sprintf("Install %s: %s", entry.Name, entry.Description)
	if entry.Runtime != "" {
		summary += fmt.Sprintf(" [runtime: %s]", entry.Runtime)
	}

	// Build the question the LLM should ask the user.
	// 建 LLM 该问用户的问句。
	var qb strings.Builder
	fmt.Fprintf(&qb, "Install the MCP server %q?\n\n%s",
		entry.Name, entry.Description)
	if len(entry.RequiredEnv) > 0 {
		qb.WriteString("\n\nIt needs the following environment variables:")
		for _, e := range entry.RequiredEnv {
			qb.WriteString(fmt.Sprintf("\n  - %s: %s", e.Name, e.Description))
			if e.SetupURL != "" {
				qb.WriteString(fmt.Sprintf(" (get one at %s)", e.SetupURL))
			}
		}
	}
	if entry.Notes != "" {
		qb.WriteString("\n\nNotes: " + entry.Notes)
	}
	if len(entry.RequiredArgs) > 0 {
		qb.WriteString("\n\nIt needs the following arguments:")
		for _, a := range entry.RequiredArgs {
			qb.WriteString(fmt.Sprintf("\n  - %s: %s", a.Name, a.Description))
			if a.Default != "" {
				qb.WriteString(fmt.Sprintf(" (default: %s)", a.Default))
			}
		}
	}
	qb.WriteString("\n\nProceed?")

	envelope := map[string]any{
		"status":             "needs_confirmation",
		"summary":            summary,
		"suggested_question": qb.String(),
		"required_env":       entry.RequiredEnv,
		"required_args":      entry.RequiredArgs,
		"notes":              entry.Notes,
		"tier":               entry.Tier,
	}
	b, _ := json.Marshal(envelope)
	return string(b)
}

// successJSON renders the post-install ServerStatus response. Envelope is
// the single source of truth (status / name / server.Status); no human
// "message" field — the LLM reads structured fields directly.
//
// successJSON 渲染装后 ServerStatus 响应。envelope 是单一事实源
// （status / name / server.Status）；不附 human "message"——LLM 直接读
// 结构化字段。
func successJSON(st *mcpdomain.ServerStatus, name string) string {
	envelope := map[string]any{
		"status": "installed",
		"name":   name,
		"server": st,
	}
	b, _ := json.Marshal(envelope)
	return string(b)
}

// errorJSON renders a structured error response the LLM can parse and act on.
//
// errorJSON 渲染 LLM 能解析 + 行动的结构化错误响应。
func errorJSON(code, message string) string {
	envelope := map[string]any{
		"status":  "error",
		"error":   code,
		"message": message,
	}
	b, _ := json.Marshal(envelope)
	return string(b)
}
