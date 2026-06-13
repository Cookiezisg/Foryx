package handlers

import (
	"encoding/json"
	"io"
	"net/http"

	"go.uber.org/zap"

	mcpapp "github.com/sunweilin/forgify/backend/internal/app/mcp"
	mcpdomain "github.com/sunweilin/forgify/backend/internal/domain/mcp"
	mcpinfra "github.com/sunweilin/forgify/backend/internal/infra/mcp"
	responsehttpapi "github.com/sunweilin/forgify/backend/internal/transport/httpapi/response"
)

const mcpImportMaxBytes = 1 << 20

// MCPHandler hosts the MCP HTTP endpoints: installed servers (list / get / stderr / put /
// delete / :reconnect / tools:invoke / :import) + the marketplace (list / :install). Servers
// are keyed by their short name (workspace-unique); the registry by full slug (in body, since
// slugs contain '/'). Health-history is gone (live ServerStatus is enough).
//
// MCPHandler 持 MCP HTTP 端点：已装 server（list/get/stderr/put/delete/:reconnect/tools:invoke/
// :import）+ 市场（list/:install）。server 用短名为键（workspace 唯一）；registry 用完整 slug
// （在 body，因 slug 含 '/'）。health-history 已删（实时 ServerStatus 够用）。
type MCPHandler struct {
	svc *mcpapp.Service
	log *zap.Logger
}

func NewMCPHandler(svc *mcpapp.Service, log *zap.Logger) *MCPHandler {
	if log == nil {
		log = zap.NewNop()
	}
	return &MCPHandler{svc: svc, log: log.Named("handlers.mcp")}
}

func (h *MCPHandler) Register(mux Registrar) {
	mux.HandleFunc("GET /api/v1/mcp-servers", h.ListServers)
	mux.HandleFunc("GET /api/v1/mcp-servers/{name}", h.GetServer)
	mux.HandleFunc("GET /api/v1/mcp-servers/{name}/stderr", h.GetStderr)
	mux.HandleFunc("GET /api/v1/mcp-servers/{name}/calls", h.ListCalls)
	mux.HandleFunc("PUT /api/v1/mcp-servers/{name}", h.PutServer)
	mux.HandleFunc("DELETE /api/v1/mcp-servers/{name}", h.DeleteServer)
	mux.HandleFunc("POST /api/v1/mcp-servers/{nameAction}", h.serverNameAction)
	mux.HandleFunc("POST /api/v1/mcp-servers/{name}/tools/{toolNameAction}", h.toolNameAction)
	mux.HandleFunc("POST /api/v1/mcp-servers:import", h.Import)
	mux.HandleFunc("GET /api/v1/mcp-calls/{id}", h.GetCall) // Log 单读路径变量统一 {id}(MD-id4)
	mux.HandleFunc("GET /api/v1/mcp-registry", h.ListRegistry)
	mux.HandleFunc("POST /api/v1/mcp-registry:install", h.Install)
}

// GetCall returns one call-log record — the only HTTP surface that carries the call's logs
// (progress notifications + stderr tail on failure); list rows omit them. Mirrors
// GET /handler-calls/{callId}.
//
// GetCall 返回单条调用日志——唯一携带 logs（进度通知 + 失败附 stderr 尾）的 HTTP 面；列表行
// 不带。对标 GET /handler-calls/{callId}。
func (h *MCPHandler) GetCall(w http.ResponseWriter, r *http.Request) {
	c, err := h.svc.GetCall(r.Context(), r.PathValue("id"))
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, c)
}

func (h *MCPHandler) ListServers(w http.ResponseWriter, r *http.Request) {
	servers, err := h.svc.ListServers(r.Context())
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, servers)
}

func (h *MCPHandler) GetServer(w http.ResponseWriter, r *http.Request) {
	st, err := h.svc.GetServer(r.Context(), r.PathValue("name"))
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, st)
}

// ListCalls pages a server's tool-call log (C4) — the entity panel's run history.
//
// ListCalls 分页 server 的工具调用日志（C4）——实体面板的运行历史。
func (h *MCPHandler) ListCalls(w http.ResponseWriter, r *http.Request) {
	p, err := responsehttpapi.ParsePage(r)
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	st, err := h.svc.GetServer(r.Context(), r.PathValue("name"))
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	res, err := h.svc.SearchCalls(r.Context(), mcpdomain.CallFilter{
		ServerID:       st.ID,
		Tool:           r.URL.Query().Get("tool"),
		Status:         r.URL.Query().Get("status"),
		TriggeredBy:    r.URL.Query().Get("triggeredBy"),
		ConversationID: r.URL.Query().Get("conversationId"),
		FlowrunID:      r.URL.Query().Get("flowrunId"),
		Cursor:         p.Cursor,
		Limit:          p.Limit,
	})
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, res)
}

func (h *MCPHandler) GetStderr(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	tail, err := h.svc.Stderr(r.Context(), name)
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, map[string]any{"name": name, "stderr": tail, "size": len(tail)})
}

// PutServer upserts a manually-configured server (stdio command/args or remote url) and connects.
//
// PutServer 创建/更新手动配置的 server（stdio command/args 或 remote url）并连接。
func (h *MCPHandler) PutServer(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Description string            `json:"description"`
		Runtime     string            `json:"runtime"`
		Command     string            `json:"command"`
		Args        []string          `json:"args"`
		URL         string            `json:"url"`
		Transport   string            `json:"transport"`
		Env         map[string]string `json:"env"`
		Headers     map[string]string `json:"headers"`
		TimeoutSec  int               `json:"timeoutSec"`
	}
	if err := decodeJSON(r, &req); err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	srv := &mcpdomain.Server{
		Name:        r.PathValue("name"),
		Description: req.Description,
		Runtime:     req.Runtime,
		Command:     req.Command,
		Args:        req.Args,
		URL:         req.URL,
		Transport:   req.Transport,
		Env:         req.Env,
		Headers:     req.Headers,
		TimeoutSec:  req.TimeoutSec,
	}
	st, err := h.svc.AddServer(r.Context(), srv)
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, st)
}

func (h *MCPHandler) DeleteServer(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.RemoveServer(r.Context(), r.PathValue("name")); err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// serverNameAction dispatches POST /mcp-servers/{name}:reconnect.
//
// serverNameAction 分派 POST /mcp-servers/{name}:reconnect。
func (h *MCPHandler) serverNameAction(w http.ResponseWriter, r *http.Request) {
	name, action, ok := idAndAction(r, "nameAction")
	if !ok || name == "" {
		responsehttpapi.Error(w, http.StatusBadRequest, "INVALID_REQUEST", "missing server name in path", nil)
		return
	}
	switch action {
	case "reconnect":
		st, err := h.svc.Reconnect(r.Context(), name)
		if err != nil {
			responsehttpapi.FromDomainError(w, h.log, err)
			return
		}
		responsehttpapi.Success(w, http.StatusOK, st)
	default:
		responsehttpapi.Error(w, http.StatusBadRequest, "INVALID_REQUEST", "unknown action "+action, nil)
	}
}

// toolNameAction dispatches POST /mcp-servers/{name}/tools/{tool}:invoke (direct smoke-test,
// bypassing chat/LLM).
//
// toolNameAction 分派 POST /mcp-servers/{name}/tools/{tool}:invoke（直接试调用，绕过 chat/LLM）。
func (h *MCPHandler) toolNameAction(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	tool, action, ok := idAndAction(r, "toolNameAction")
	if !ok || tool == "" || action != "invoke" {
		responsehttpapi.Error(w, http.StatusBadRequest, "INVALID_REQUEST", "expected tools/{tool}:invoke", nil)
		return
	}
	var req struct {
		Args json.RawMessage `json:"args"`
	}
	if err := decodeJSON(r, &req); err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	if len(req.Args) == 0 {
		req.Args = json.RawMessage("{}")
	}
	st, err := h.svc.GetServer(r.Context(), name)
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	res, err := h.svc.CallTool(r.Context(), st.ID, tool, req.Args, mcpdomain.CallTriggeredByManual)
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, res) // 裸结果,不裹 {result}(envelope 内层)
}

// Import folds a Claude Desktop mcp.json fragment into the store. ?overwrite=true replaces
// same-name servers.
//
// Import 把 Claude Desktop mcp.json 片段折叠进存储。?overwrite=true 替换同名 server。
func (h *MCPHandler) Import(w http.ResponseWriter, r *http.Request) {
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, mcpImportMaxBytes))
	if err != nil {
		responsehttpapi.Error(w, http.StatusBadRequest, "INVALID_REQUEST", "read body: "+err.Error(), nil)
		return
	}
	entries, err := mcpinfra.ParseImport(raw)
	if err != nil {
		responsehttpapi.Error(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), nil)
		return
	}
	imported, skipped, err := h.svc.Import(r.Context(), entries, r.URL.Query().Get("overwrite") == "true")
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, map[string]any{"imported": imported, "skipped": skipped})
}

func (h *MCPHandler) ListRegistry(w http.ResponseWriter, r *http.Request) {
	entries, err := h.svc.ListRegistry(r.Context())
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, entries)
}

// Install installs a marketplace entry by full name (in body, since slugs contain '/').
//
// Install 按完整名（在 body，因 slug 含 '/'）安装一个市场条目。
func (h *MCPHandler) Install(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string            `json:"name"`
		Env  map[string]string `json:"env"`
	}
	if err := decodeJSON(r, &req); err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	st, err := h.svc.InstallFromRegistry(r.Context(), req.Name, req.Env)
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusCreated, st)
}
