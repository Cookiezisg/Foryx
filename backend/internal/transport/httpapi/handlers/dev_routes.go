// dev_routes.go — GET /dev/routes endpoint (TE-21). Returns a hand-curated
// dump of every HTTP route the backend registers, grouped by handler file.
// stdlib http.ServeMux doesn't expose its registered routes at runtime,
// so this is maintained manually. The Routes tab in testend uses it to
// give testers a quick "what endpoints exist" lookup with copy-as-curl.
//
// Maintenance: when adding/removing a mux.HandleFunc call in any *.go in
// this directory, update the matching slice below. Verifiable via:
//   grep -rEh 'mux\.HandleFunc\(' backend/internal/transport/httpapi/handlers/*.go \
//     | grep -v _test | wc -l
// should match len(devRoutes).
//
// dev_routes.go ——/dev/routes（TE-21）。返回所有注册路由的手工清单，
// 按 handler 文件分组。stdlib mux 运行时不暴露注册路由，故手维护。
// testend Routes tab 用此查"有哪些端点"+ 复制 curl 命令。
package handlers

import (
	"net/http"
	"strings"

	responsehttpapi "github.com/sunweilin/forgify/backend/internal/transport/httpapi/response"
)

type devRoute struct {
	Method  string `json:"method"`
	Path    string `json:"path"`
	Handler string `json:"handler"`
}

// devRoutes mirrors mux.HandleFunc registrations across all handlers in
// this package. Sorted by HTTP method then path within each handler group.
// Verify with:
//
//	grep -rEh 'mux\.HandleFunc\("' backend/internal/transport/httpapi/handlers/*.go \
//	  | grep -v _test
//
// devRoutes 镜像本包所有 mux.HandleFunc 注册。按 method 然后 path 排序。
var devRoutes = []devRoute{
	// ── health + providers (always wired)
	{"GET", "/api/v1/health", "health.Get"},
	{"GET", "/api/v1/providers", "providers.List"},

	// ── apikey
	{"POST", "/api/v1/api-keys", "apikey.Create"},
	{"GET", "/api/v1/api-keys", "apikey.List"},
	{"PATCH", "/api/v1/api-keys/{id}", "apikey.Update"},
	{"DELETE", "/api/v1/api-keys/{id}", "apikey.Delete"},
	{"POST", "/api/v1/api-keys/{id}:test", "apikey.postOnID dispatch → :test"},

	// ── model-configs
	{"GET", "/api/v1/model-configs", "model.List"},
	{"PUT", "/api/v1/model-configs/{scenario}", "model.Upsert"},

	// ── conversations + chat + attachments
	{"POST", "/api/v1/conversations", "conversation.Create"},
	{"GET", "/api/v1/conversations", "conversation.List"},
	{"GET", "/api/v1/conversations/{id}", "conversation.Get"},
	{"PATCH", "/api/v1/conversations/{id}", "conversation.Rename"},
	{"DELETE", "/api/v1/conversations/{id}", "conversation.Delete"},
	{"POST", "/api/v1/conversations/{id}/messages", "chat.SendMessage"},
	{"GET", "/api/v1/conversations/{id}/messages", "chat.ListMessages"},
	{"DELETE", "/api/v1/conversations/{id}/stream", "chat.CancelStream"},
	{"POST", "/api/v1/conversations/{id}/answers", "ask.SubmitAnswer"},
	{"POST", "/api/v1/attachments", "chat.UploadAttachment"},

	// ── SSE streams (3 per E1)
	{"GET", "/api/v1/eventlog", "eventlog.Stream (per-user)"},
	{"GET", "/api/v1/conversations/{id}/eventlog", "eventlog.History (replay)"},
	{"GET", "/api/v1/notifications", "notifications.Stream (per-user)"},
	{"GET", "/api/v1/forge", "forge.Stream (per-user)"},

	// ── functions (trinity)
	{"POST", "/api/v1/functions", "function.Create"},
	{"GET", "/api/v1/functions", "function.List"},
	{"GET", "/api/v1/functions/{id}", "function.Get"},
	{"PATCH", "/api/v1/functions/{id}", "function.UpdateMeta"},
	{"DELETE", "/api/v1/functions/{id}", "function.Delete"},
	{"POST", "/api/v1/functions/{id}:run", "function.postOnFunction → :run"},
	{"POST", "/api/v1/functions/{id}:revert", "function.postOnFunction → :revert"},
	{"GET", "/api/v1/functions/{id}/versions", "function.ListVersions"},
	{"GET", "/api/v1/functions/{id}/versions/{version}", "function.GetVersion"},
	{"GET", "/api/v1/functions/{id}/pending", "function.GetPending"},
	{"POST", "/api/v1/functions/{id}/pending:accept", "function.AcceptPending"},
	{"POST", "/api/v1/functions/{id}/pending:reject", "function.RejectPending"},
	{"GET", "/api/v1/functions/{id}/executions", "function.ListExecutions (D22)"},
	{"GET", "/api/v1/function-executions/{execId}", "function.GetExecution (D22)"},

	// ── handlers (trinity)
	{"POST", "/api/v1/handlers", "handler.Create"},
	{"GET", "/api/v1/handlers", "handler.List"},
	{"GET", "/api/v1/handlers/{id}", "handler.Get"},
	{"PATCH", "/api/v1/handlers/{id}", "handler.UpdateMeta"},
	{"DELETE", "/api/v1/handlers/{id}", "handler.Delete"},
	{"POST", "/api/v1/handlers/{id}:call", "handler.postOnHandler → :call"},
	{"POST", "/api/v1/handlers/{id}:revert", "handler.postOnHandler → :revert"},
	{"GET", "/api/v1/handlers/{id}/versions", "handler.ListVersions"},
	{"GET", "/api/v1/handlers/{id}/versions/{version}", "handler.GetVersion"},
	{"GET", "/api/v1/handlers/{id}/pending", "handler.GetPending"},
	{"POST", "/api/v1/handlers/{id}/pending:accept", "handler.AcceptPending"},
	{"POST", "/api/v1/handlers/{id}/pending:reject", "handler.RejectPending"},
	{"GET", "/api/v1/handlers/{id}/config", "handler.GetConfig"},
	{"POST", "/api/v1/handlers/{id}/config", "handler.UpdateConfig"},
	{"DELETE", "/api/v1/handlers/{id}/config", "handler.ClearConfig"},
	{"GET", "/api/v1/handlers/{id}/calls", "handler.ListCalls (D22)"},
	{"GET", "/api/v1/handler-calls/{callId}", "handler.GetCall (D22)"},

	// ── workflows (trinity)
	{"POST", "/api/v1/workflows", "workflow.Create"},
	{"GET", "/api/v1/workflows", "workflow.List"},
	{"GET", "/api/v1/workflows/{id}", "workflow.Get"},
	{"PATCH", "/api/v1/workflows/{id}", "workflow.UpdateMeta"},
	{"DELETE", "/api/v1/workflows/{id}", "workflow.Delete"},
	{"POST", "/api/v1/workflows/{id}:trigger", "workflow.postOnWorkflow → :trigger"},
	{"POST", "/api/v1/workflows/{id}:revert", "workflow.postOnWorkflow → :revert"},
	{"GET", "/api/v1/workflows/{id}/triggers", "workflow.GetTriggers"},
	{"GET", "/api/v1/workflows/{id}/versions", "workflow.ListVersions"},
	{"GET", "/api/v1/workflows/{id}/versions/{version}", "workflow.GetVersion"},
	{"GET", "/api/v1/workflows/{id}/pending", "workflow.GetPending"},
	{"POST", "/api/v1/workflows/{id}/pending:accept", "workflow.AcceptPending"},
	{"POST", "/api/v1/workflows/{id}/pending:reject", "workflow.RejectPending"},

	// ── flowruns (Plan 05 execution plane)
	{"GET", "/api/v1/flowruns", "flowrun.List"},
	{"GET", "/api/v1/flowruns/{id}", "flowrun.Get"},
	{"GET", "/api/v1/flowruns/{id}/nodes", "flowrun.ListNodes"},
	{"DELETE", "/api/v1/flowruns/{id}", "flowrun.Cancel"},
	{"POST", "/api/v1/flowruns/{id}/approvals/{nodeId}", "flowrun.Approve"},

	// ── catalog
	{"GET", "/api/v1/catalog", "catalog.Get"},
	{"POST", "/api/v1/catalog:refresh", "catalog.Refresh"},

	// ── skills
	{"GET", "/api/v1/skills", "skills.List"},
	{"POST", "/api/v1/skills", "skills.Create"},
	{"GET", "/api/v1/skills/{name}", "skills.Get"},
	{"GET", "/api/v1/skills/{name}/body", "skills.GetBody"},
	{"PUT", "/api/v1/skills/{name}", "skills.Replace"},
	{"DELETE", "/api/v1/skills/{name}", "skills.Delete"},
	{"POST", "/api/v1/skills/{name}:invoke", "skills.NameAction → :invoke"},
	{"POST", "/api/v1/skills:import", "skills.Import"},
	{"POST", "/api/v1/skills:refresh", "skills.Refresh"},

	// ── mcp
	{"GET", "/api/v1/mcp-servers", "mcp.ListServers"},
	{"GET", "/api/v1/mcp-servers/{name}", "mcp.GetServer"},
	{"GET", "/api/v1/mcp-servers/{name}/stderr", "mcp.GetServerStderr"},
	{"PUT", "/api/v1/mcp-servers/{name}", "mcp.PutServer"},
	{"DELETE", "/api/v1/mcp-servers/{name}", "mcp.DeleteServer"},
	{"POST", "/api/v1/mcp-servers/{name}:reconnect", "mcp.serverNameAction → :reconnect"},
	{"POST", "/api/v1/mcp-servers/{name}:health-check", "mcp.serverNameAction → :health-check"},
	{"POST", "/api/v1/mcp-servers:import", "mcp.ImportServers"},
	{"GET", "/api/v1/mcp-registry", "mcp.ListRegistry"},
	{"GET", "/api/v1/mcp-registry/{name}", "mcp.GetRegistryEntry"},
	{"POST", "/api/v1/mcp-registry/{name}:install", "mcp.registryNameAction → :install"},

	// ── sandbox
	{"GET", "/api/v1/sandbox/runtimes", "sandbox.ListRuntimes"},
	{"GET", "/api/v1/sandbox/envs", "sandbox.ListEnvs"},
	{"GET", "/api/v1/sandbox/envs/{id}", "sandbox.GetEnv"},
	{"GET", "/api/v1/sandbox/disk-usage", "sandbox.DiskUsage"},
	{"GET", "/api/v1/sandbox/bootstrap-status", "sandbox.BootstrapStatus"},
	{"GET", "/api/v1/conversations/{id}/sandbox-envs", "sandbox.ListConvEnvs"},
	{"POST", "/api/v1/sandbox/envs/{id}:destroy", "sandbox.envAction → :destroy"},
	{"POST", "/api/v1/sandbox/runtimes/{id}:destroy", "sandbox.runtimeAction → :destroy"},
	{"POST", "/api/v1/sandbox/{action}", "sandbox.Action (gc / retry-bootstrap)"},
	{"POST", "/api/v1/conversations/{id}/sandbox-envs/{kind}:reset", "sandbox.convEnvKindAction"},
	{"POST", "/api/v1/conversations/{id}/sandbox-envs:reset-all", "sandbox.convEnvsAction"},

	// ── dev (only when --dev)
	{"GET", "/dev/", "dev.ServeIndex (testend HTML)"},
	{"GET", "/dev/logs", "dev.StreamLogs (SSE)"},
	{"POST", "/dev/sql", "dev.QuerySQL"},
	{"GET", "/dev/schema", "dev.Schema"},
	{"GET", "/dev/collections", "dev.ListCollections"},
	{"GET", "/dev/tools", "dev.ListTools"},
	{"POST", "/dev/invoke", "dev.InvokeTool"},
	{"GET", "/dev/info", "dev.Info"},
	{"GET", "/dev/forgify-home", "dev.ForgifyHome"},
	{"GET", "/dev/runtime", "dev.Runtime"},
	{"GET", "/dev/routes", "dev.Routes (this endpoint)"},
	{"GET", "/dev/bash-processes", "dev.BashProcesses"},
	{"POST", "/dev/mock-llm/scripts", "dev.MockLLMPushScripts"},
	{"GET", "/dev/mock-llm/queue", "dev.MockLLMQueue"},
	{"DELETE", "/dev/mock-llm/scripts", "dev.MockLLMClear"},
	{"GET", "/dev/mock-llm/last-prompt", "dev.MockLLMLastPrompt"},
	{"GET", "/dev/llm-trace", "dev.LLMTrace"},
}

// Routes serves GET /dev/routes — the manifest above.
//
// Routes 服务 GET /dev/routes，返回上面的 manifest。
func (h *DevHandler) Routes(w http.ResponseWriter, r *http.Request) {
	out := make([]devRoute, len(devRoutes))
	copy(out, devRoutes)
	// Stable sort: by path, then method (so GET/POST on same path cluster).
	// 稳定排序：按 path 然后 method。
	sortRoutes(out)
	responsehttpapi.Success(w, http.StatusOK, out)
}

func sortRoutes(rs []devRoute) {
	// Insertion sort — small N, stable, no imports needed.
	// 插入排序——N 小、稳定、零依赖。
	for i := 1; i < len(rs); i++ {
		for j := i; j > 0 && lessRoute(rs[j], rs[j-1]); j-- {
			rs[j], rs[j-1] = rs[j-1], rs[j]
		}
	}
}

func lessRoute(a, b devRoute) bool {
	if a.Path != b.Path {
		return a.Path < b.Path
	}
	return strings.Compare(a.Method, b.Method) < 0
}
