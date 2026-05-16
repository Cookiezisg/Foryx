// Package permissionsgate is V1.2 §3 final-sweep's runtime permission
// + hook evaluation. levels.go holds the static (toolName → DangerLevel)
// registry; rules.go evaluates settings.json deny/ask/allow; gate.go
// wires hook calls into the dispatch path.
//
// Package permissionsgate 是 V1.2 §3 final-sweep 的运行时权限 + hook
// 评估。levels.go 持静态 (toolName → DangerLevel) registry；rules.go
// 评估 settings.json deny/ask/allow；gate.go 把 hook 调用接到派发路径。
package permissionsgate

import (
	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
	permdomain "github.com/sunweilin/forgify/backend/internal/domain/permissions"
)

// toolLevels is the canonical (toolName → DangerLevel) table. Every
// tool registered with chat MUST appear here; the contract test in
// levels_test.go enforces this at CI time.
//
// Classification rationale:
//   - ReadOnly: pure read or query, zero side effects on disk / remote
//   - WorkspaceWrite: mutates local state (DB / FS / sandbox) but no
//     arbitrary shell or remote effect; sandbox-bounded
//   - DangerFullAccess: arbitrary shell or process control; only Bash
//     family currently — call_handler/run_function/Subagent stay
//     WorkspaceWrite because they're sandboxed (Python venv / sub-agent
//     constrained tool list)
//
// toolLevels 是 canonical (toolName → DangerLevel) 表。注册到 chat 的
// 每个 tool 必须在此登记；levels_test.go 的契约测试 CI 强制。
//
// 分类原则：
//   ReadOnly = 纯读 / 查询，磁盘 / 远程 0 副作用
//   WorkspaceWrite = 改本地状态（DB / FS / sandbox），无任意 shell / 远
//     程效果；sandbox 边界内
//   DangerFullAccess = 任意 shell / 进程控制；目前仅 Bash 家族——call_handler
//     / run_function / Subagent 是 WorkspaceWrite（sandbox / sub-agent
//     tool list 约束）
var toolLevels = map[string]permdomain.DangerLevel{
	// ── ReadOnly ──────────────────────────────────────────────────────
	"AskUserQuestion":            permdomain.LevelReadOnly,
	"BashOutput":                 permdomain.LevelReadOnly,
	"get_function":               permdomain.LevelReadOnly,
	"get_function_execution":     permdomain.LevelReadOnly,
	"get_handler":                permdomain.LevelReadOnly,
	"get_handler_call":           permdomain.LevelReadOnly,
	"get_mcp_call":               permdomain.LevelReadOnly,
	"get_skill_execution":        permdomain.LevelReadOnly,
	"get_workflow":               permdomain.LevelReadOnly,
	"get_workflow_execution":     permdomain.LevelReadOnly,
	"Glob":                       permdomain.LevelReadOnly,
	"Grep":                       permdomain.LevelReadOnly,
	"list_mcp_marketplace":       permdomain.LevelReadOnly,
	"Read":                       permdomain.LevelReadOnly,
	"read_memory":                permdomain.LevelReadOnly,
	"search_function":            permdomain.LevelReadOnly,
	"search_function_executions": permdomain.LevelReadOnly,
	"search_handler":             permdomain.LevelReadOnly,
	"search_handler_calls":       permdomain.LevelReadOnly,
	"search_mcp_calls":           permdomain.LevelReadOnly,
	"search_mcp_tools":           permdomain.LevelReadOnly,
	"search_skill_executions":    permdomain.LevelReadOnly,
	"search_skills":              permdomain.LevelReadOnly,
	"search_workflow":            permdomain.LevelReadOnly,
	"search_workflow_executions": permdomain.LevelReadOnly,
	"TodoGet":                    permdomain.LevelReadOnly,
	"TodoList":                   permdomain.LevelReadOnly,
	"WebFetch":                   permdomain.LevelReadOnly,
	"WebSearch":                  permdomain.LevelReadOnly,

	// ── WorkspaceWrite ────────────────────────────────────────────────
	"activate_skill":         permdomain.LevelWorkspaceWrite,
	"call_handler":           permdomain.LevelWorkspaceWrite,
	"call_mcp_tool":          permdomain.LevelWorkspaceWrite,
	"create_function":        permdomain.LevelWorkspaceWrite,
	"create_handler":         permdomain.LevelWorkspaceWrite,
	"create_workflow":        permdomain.LevelWorkspaceWrite,
	"delete_function":        permdomain.LevelWorkspaceWrite,
	"delete_handler":         permdomain.LevelWorkspaceWrite,
	"delete_workflow":        permdomain.LevelWorkspaceWrite,
	"Edit":                   permdomain.LevelWorkspaceWrite,
	"edit_function":          permdomain.LevelWorkspaceWrite,
	"edit_handler":           permdomain.LevelWorkspaceWrite,
	"edit_workflow":          permdomain.LevelWorkspaceWrite,
	"forget_memory":          permdomain.LevelWorkspaceWrite,
	"install_mcp_server":     permdomain.LevelWorkspaceWrite,
	"revert_function":        permdomain.LevelWorkspaceWrite,
	"revert_handler":         permdomain.LevelWorkspaceWrite,
	"revert_workflow":        permdomain.LevelWorkspaceWrite,
	"run_function":           permdomain.LevelWorkspaceWrite,
	"Subagent":               permdomain.LevelWorkspaceWrite,
	"TodoCreate":             permdomain.LevelWorkspaceWrite,
	"TodoUpdate":             permdomain.LevelWorkspaceWrite,
	"uninstall_mcp_server":   permdomain.LevelWorkspaceWrite,
	"update_handler_config":  permdomain.LevelWorkspaceWrite,
	"Write":                  permdomain.LevelWorkspaceWrite,
	"write_memory":           permdomain.LevelWorkspaceWrite,

	// ── DangerFullAccess ──────────────────────────────────────────────
	"Bash":      permdomain.LevelDangerFullAccess,
	"KillShell": permdomain.LevelDangerFullAccess,
}

// LookupLevel returns the registered DangerLevel for toolName, falling
// back via tool.IsReadOnly() when not in the registry (handles MCP tools
// injected dynamically: read-only flag flips to LevelReadOnly, anything
// else defaults to WorkspaceWrite — never DangerFullAccess, since we
// can't know if a dynamic tool is shell-equivalent).
//
// LookupLevel 返 toolName 登记的 DangerLevel；未登记时按 tool.IsReadOnly()
// 兜底（处理 MCP 动态注入的 tool）：read-only → LevelReadOnly，其余默认
// WorkspaceWrite——绝不默认 DangerFullAccess，因为我们不知道动态 tool 是
// 否等同 shell。
func LookupLevel(toolName string, t toolapp.Tool) permdomain.DangerLevel {
	if l, ok := toolLevels[toolName]; ok {
		return l
	}
	if t != nil && t.IsReadOnly() {
		return permdomain.LevelReadOnly
	}
	return permdomain.LevelWorkspaceWrite
}

// RegisteredTools returns the names of all tools in the static table.
// Used by the contract test + GET /api/v1/permissions/tools endpoint.
//
// RegisteredTools 返静态表中所有 tool 名。契约测试 + GET 端点用。
func RegisteredTools() []string {
	out := make([]string, 0, len(toolLevels))
	for n := range toolLevels {
		out = append(out, n)
	}
	return out
}
