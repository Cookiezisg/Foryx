// install_server.go — install_mcp_server system tool: LLM-driven flow
// for installing an MCP server from the marketplace.
//
// Two-phase contract:
//
//   Phase 1: LLM calls install_mcp_server({name}) without `confirmed`.
//            Tool reads RegistryEntry, derives alias, checks for collision,
//            returns a "needs_confirmation" JSON containing a suggested
//            question for the user (incl. required env/args + alias).
//   Phase 2: LLM uses ask tool to get user consent + any required values.
//            LLM calls install_mcp_server({name, confirmed:true, alias?,
//            env?, arguments?}). Tool runs Service.InstallFromRegistry.
//
// This puts consent + parameter collection in LLM hands ("everything in
// LLM" per project preference) rather than having the framework's
// PermissionAsk pop up an out-of-band UI dialog. Permission at framework
// level is Allow because real consent flows through the LLM-driven ask.
//
// install_server.go ——install_mcp_server 系统工具：LLM 驱动的从 marketplace
// 装 MCP server 流程。
//
// 两阶段契约：
//   阶段 1: LLM 调 install_mcp_server({name}) 不带 confirmed。工具读
//           RegistryEntry、派生 alias、检冲突，返 "needs_confirmation"
//           JSON 含给用户的建议问句（含必填 env/args + alias）。
//   阶段 2: LLM 用 ask 工具拿用户同意 + 必填值。LLM 调 install_mcp_server(
//           {name, confirmed:true, alias?, env?, arguments?})。工具调
//           Service.InstallFromRegistry。
//
// 把同意 + 参数收集放 LLM 手里（与项目"everything in LLM"偏好一致）而非
// 让框架 PermissionAsk 弹带外 UI 对话框。框架级权限 = Allow，真正同意走
// LLM 驱动的 ask。
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
// InstallMCPServer 实现 install_mcp_server 系统工具。
type InstallMCPServer struct {
	svc *mcpapp.Service
}

const installMCPServerDescription = `Install an MCP server from Forgify's curated marketplace. Two-phase flow:

PHASE 1 (discovery): Call install_mcp_server({name: "<short-name>"}). Tool returns {status:"needs_confirmation", suggested_question, required_env, required_args, notes}. Use ` + "`ask`" + ` to relay the question to the user, then collect any required env / args values. Always relay the entry's notes to the user — they describe first-run gotchas (chromium downloads, OAuth flows, etc).

PHASE 2 (commit): Call install_mcp_server({name, confirmed: true, env?: {KEY:"value"}, arguments?: {key:"value"}}). Tool installs + connects the server. On success returns the new ServerStatus; on failure returns a structured error (already_installed / missing_required_args / install_failed / handshake_failed) with hints for recovery.

Notes:
- name is the curated catalog's short slug (e.g. "playwright", "notion", "ms365"). Pick from search_mcp_marketplace results.
- name doubles as the mcp.json key — no separate alias.
- already_installed means this name is already a configured server; uninstall first via uninstall_mcp_server.`

var installMCPServerSchema = json.RawMessage(`{
	"type": "object",
	"properties": {
		"name":      {"type": "string", "description": "Curated catalog short slug (e.g. 'playwright', 'notion'). Pick from search_mcp_marketplace."},
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
				fmt.Sprintf("Server %q not found in marketplace. Use search_mcp_marketplace to discover available servers.", args.Name)), nil
		}
		if errors.Is(err, mcpdomain.ErrMarketplaceUnavailable) {
			return errorJSON("marketplace_unavailable",
				"Marketplace is unreachable. Suggest the user retry later or configure a search-category API key as a workaround."), nil
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
			fmt.Sprintf("Missing required env: %v. Ask the user for these values, then retry with env={...}.", err.Error())), nil
	case errors.Is(err, mcpdomain.ErrRequiredArgsMissing):
		return errorJSON("missing_required_args",
			fmt.Sprintf("Missing required args: %v. Ask the user for these values, then retry with arguments={...}.", err.Error())), nil
	case errors.Is(err, mcpdomain.ErrInstallFailed):
		return errorJSON("install_failed", fmt.Sprintf("Install failed: %v", err)), nil
	case errors.Is(err, mcpdomain.ErrHandshakeFailed):
		return errorJSON("handshake_failed", fmt.Sprintf("Server installed but handshake failed: %v. The server is in mcp.json with status=failed; user can fix and reconnect via UI, or uninstall.", err)), nil
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
	fmt.Fprintf(&qb, "I'd like to install the MCP server %q.\n\n%s",
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

// successJSON renders the post-install ServerStatus response.
//
// successJSON 渲染装后 ServerStatus 响应。
func successJSON(st *mcpdomain.ServerStatus, name string) string {
	envelope := map[string]any{
		"status":  "installed",
		"name":    name,
		"server":  st,
		"message": fmt.Sprintf("Server %q installed and connected (status=%s).", name, st.Status),
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
