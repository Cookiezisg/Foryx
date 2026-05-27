package handlers

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"

	mcpapp "github.com/sunweilin/forgify/backend/internal/app/mcp"
	mcpdomain "github.com/sunweilin/forgify/backend/internal/domain/mcp"
	responsehttpapi "github.com/sunweilin/forgify/backend/internal/transport/httpapi/response"
)

const importMaxBytes = 1 << 20

// MCPHandler serves /api/v1/mcp-* + /api/v1/mcp-registry routes.
//
// MCPHandler 提供 /api/v1/mcp-* + /api/v1/mcp-registry 路由。
type MCPHandler struct {
	svc *mcpapp.Service
	log *zap.Logger
}

func NewMCPHandler(svc *mcpapp.Service, log *zap.Logger) *MCPHandler {
	return &MCPHandler{svc: svc, log: log}
}

func (h *MCPHandler) Register(mux Registrar) {
	mux.HandleFunc("GET /api/v1/mcp-servers", h.ListServers)
	mux.HandleFunc("GET /api/v1/mcp-servers/{name}", h.GetServer)
	mux.HandleFunc("GET /api/v1/mcp-servers/{name}/stderr", h.GetServerStderr)
	mux.HandleFunc("GET /api/v1/mcp-servers/{name}/health-history", h.GetHealthHistory)
	mux.HandleFunc("PUT /api/v1/mcp-servers/{name}", h.PutServer)
	mux.HandleFunc("DELETE /api/v1/mcp-servers/{name}", h.DeleteServer)
	mux.HandleFunc("POST /api/v1/mcp-servers/{nameAction}", h.serverNameAction)
	mux.HandleFunc("POST /api/v1/mcp-servers/{name}/tools/{toolNameAction}", h.toolNameAction)
	mux.HandleFunc("POST /api/v1/mcp-servers:import", h.importServers)
	mux.HandleFunc("GET /api/v1/mcp-registry", h.ListRegistry)
	mux.HandleFunc("GET /api/v1/mcp-registry/{name}", h.GetRegistryEntry)
	mux.HandleFunc("POST /api/v1/mcp-registry/{nameAction}", h.registryNameAction)
}

// GetHealthHistory returns recent health snapshots for a server. ?sinceMinutes=N
// controls window (default 1440 = 24h, max 10080 = 7 days).
//
// GetHealthHistory 返 server 最近健康快照；?sinceMinutes=N 控时间窗
// （默认 1440 = 24h，最大 10080 = 7 天）。
func (h *MCPHandler) GetHealthHistory(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	sinceMin := 1440
	if raw := r.URL.Query().Get("sinceMinutes"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			sinceMin = n
		}
	}
	if sinceMin > 10080 {
		sinceMin = 10080
	}
	since := time.Now().UTC().Add(-time.Duration(sinceMin) * time.Minute)
	rows, err := h.svc.ListHealthHistory(r.Context(), name, since)
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, rows)
}

// toolNameAction dispatches POST /mcp-servers/{name}/tools/{tool}:invoke.
//
// toolNameAction 派发 POST /mcp-servers/{name}/tools/{tool}:invoke。
func (h *MCPHandler) toolNameAction(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	toolName, action := splitAction(r.PathValue("toolNameAction"))
	if toolName == "" {
		responsehttpapi.Error(w, http.StatusBadRequest, "INVALID_REQUEST",
			"missing tool name in path", nil)
		return
	}
	switch action {
	case "invoke":
		h.invokeTool(w, r, name, toolName)
	default:
		responsehttpapi.Error(w, http.StatusBadRequest, "INVALID_REQUEST",
			"unknown action "+action, nil)
	}
}

// invokeTool calls an MCP tool directly (bypasses chat/LLM); used by MCP detail
// page "试调用" button to let users smoke-test tools.
//
// invokeTool 直接调 MCP 工具（绕过 chat/LLM）；mcp 详情页"试调用"按钮用。
func (h *MCPHandler) invokeTool(w http.ResponseWriter, r *http.Request, serverName, toolName string) {
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
	res, err := h.svc.CallTool(r.Context(), serverName, toolName, req.Args)
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, map[string]any{"result": res})
}

func (h *MCPHandler) ListServers(w http.ResponseWriter, r *http.Request) {
	servers := h.svc.ListServers(r.Context())
	responsehttpapi.Success(w, http.StatusOK, servers)
}

// GetServerStderr returns the captured stderr ring-buffer of the subprocess.
//
// GetServerStderr 返子进程 stderr 环形缓冲。
func (h *MCPHandler) GetServerStderr(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	tail, err := h.svc.Stderr(name)
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, map[string]any{
		"name":   name,
		"stderr": tail,
		"size":   len(tail),
	})
}

func (h *MCPHandler) GetServer(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	st, err := h.svc.GetServer(r.Context(), name)
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, st)
}

// PutServer upserts ServerConfig and Connects; returns 200 with status row.
//
// PutServer 创建/更新 ServerConfig 并 Connect,返 200 + status row。
func (h *MCPHandler) PutServer(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	var body struct {
		Command    string            `json:"command"`
		Args       []string          `json:"args,omitempty"`
		Env        map[string]string `json:"env,omitempty"`
		TimeoutSec int               `json:"timeoutSec,omitempty"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, importMaxBytes)).Decode(&body); err != nil {
		responsehttpapi.Error(w, http.StatusBadRequest, "INVALID_REQUEST",
			"failed to parse request body: "+err.Error(), nil)
		return
	}
	if strings.TrimSpace(body.Command) == "" {
		responsehttpapi.Error(w, http.StatusBadRequest, "MCP_COMMAND_REQUIRED",
			"command field is required", nil)
		return
	}
	cfg := mcpdomain.ServerConfig{
		Name:       name,
		Command:    body.Command,
		Args:       body.Args,
		Env:        body.Env,
		TimeoutSec: body.TimeoutSec,
	}
	if err := h.svc.AddServer(r.Context(), cfg); err != nil {
		st, gErr := h.svc.GetServer(r.Context(), name)
		if gErr == nil && st != nil {
			h.log.Error("PUT mcp-server completed with connect issue (returned 200 + status row per mcp.md §10)",
				zap.String("name", name), zap.Error(err))
			responsehttpapi.Success(w, http.StatusOK, st)
			return
		}
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	st, err := h.svc.GetServer(r.Context(), name)
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, st)
}

func (h *MCPHandler) DeleteServer(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := h.svc.RemoveServer(r.Context(), name); err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// serverNameAction dispatches :reconnect / :health-check on a server name.
//
// serverNameAction 分派 :reconnect / :health-check。
func (h *MCPHandler) serverNameAction(w http.ResponseWriter, r *http.Request) {
	name, action := splitAction(r.PathValue("nameAction"))
	if name == "" {
		responsehttpapi.Error(w, http.StatusBadRequest, "INVALID_REQUEST",
			"missing server name in path", nil)
		return
	}
	switch action {
	case "reconnect":
		if err := h.svc.Reconnect(r.Context(), name); err != nil {
			responsehttpapi.FromDomainError(w, h.log, err)
			return
		}
		st, err := h.svc.GetServer(r.Context(), name)
		if err != nil {
			responsehttpapi.FromDomainError(w, h.log, err)
			return
		}
		responsehttpapi.Success(w, http.StatusOK, st)
	case "health-check":
		res, err := h.svc.HealthCheck(r.Context(), name)
		if err != nil {
			responsehttpapi.FromDomainError(w, h.log, err)
			return
		}
		responsehttpapi.Success(w, http.StatusOK, res)
	default:
		responsehttpapi.Error(w, http.StatusBadRequest, "INVALID_REQUEST",
			"unknown action "+action, nil)
	}
}

// importServers accepts JSON or multipart Claude-Desktop mcp.json fragment.
//
// importServers 接 JSON 或 multipart 上传的 Claude Desktop mcp.json 片段。
func (h *MCPHandler) importServers(w http.ResponseWriter, r *http.Request) {
	overwrite := r.URL.Query().Get("overwrite") == "true"
	contentType := r.Header.Get("Content-Type")

	var raw []byte
	if strings.HasPrefix(contentType, "multipart/form-data") {
		if err := r.ParseMultipartForm(importMaxBytes); err != nil {
			responsehttpapi.Error(w, http.StatusBadRequest, "INVALID_REQUEST",
				"failed to parse multipart form: "+err.Error(), nil)
			return
		}
		file, _, err := r.FormFile("config")
		if err != nil {
			responsehttpapi.Error(w, http.StatusBadRequest, "INVALID_REQUEST",
				"missing 'config' file part: "+err.Error(), nil)
			return
		}
		defer file.Close()
		var rerr error
		raw, rerr = io.ReadAll(io.LimitReader(file, importMaxBytes+1))
		if rerr != nil {
			responsehttpapi.Error(w, http.StatusBadRequest, "INVALID_REQUEST",
				"handlers.ImportServers: read multipart file: "+rerr.Error(), nil)
			return
		}
		if int64(len(raw)) > importMaxBytes {
			responsehttpapi.Error(w, http.StatusRequestEntityTooLarge, "INVALID_REQUEST",
				"config file exceeds 1MB limit", nil)
			return
		}
	} else {
		body := http.MaxBytesReader(w, r.Body, importMaxBytes)
		var err error
		raw, err = io.ReadAll(body)
		if err != nil {
			responsehttpapi.Error(w, http.StatusBadRequest, "INVALID_REQUEST",
				"failed to read body: "+err.Error(), nil)
			return
		}
	}

	var fragment struct {
		MCPServers map[string]struct {
			Command    string            `json:"command"`
			Args       []string          `json:"args,omitempty"`
			Env        map[string]string `json:"env,omitempty"`
			TimeoutSec int               `json:"timeoutSec,omitempty"`
		} `json:"mcpServers"`
	}
	if err := json.Unmarshal(raw, &fragment); err != nil {
		responsehttpapi.Error(w, http.StatusBadRequest, "INVALID_REQUEST",
			"failed to parse JSON: "+err.Error(), nil)
		return
	}
	if len(fragment.MCPServers) == 0 {
		responsehttpapi.Error(w, http.StatusBadRequest, "INVALID_REQUEST",
			"no servers found in payload (mcpServers map is empty or missing)", nil)
		return
	}

	configs := make(map[string]mcpdomain.ServerConfig, len(fragment.MCPServers))
	for name, e := range fragment.MCPServers {
		configs[name] = mcpdomain.ServerConfig{
			Name:       name,
			Command:    e.Command,
			Args:       e.Args,
			Env:        e.Env,
			TimeoutSec: e.TimeoutSec,
		}
	}

	res, err := h.svc.Import(r.Context(), configs, overwrite)
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, res)
}

func (h *MCPHandler) ListRegistry(w http.ResponseWriter, r *http.Request) {
	entries, err := h.svc.ListRegistry(r.Context())
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, entries)
}

func (h *MCPHandler) GetRegistryEntry(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	entry, err := h.svc.GetRegistryEntry(r.Context(), name)
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, entry)
}

// registryNameAction handles POST /api/v1/mcp-registry/{name}:install.
//
// registryNameAction 处理 POST /api/v1/mcp-registry/{name}:install。
func (h *MCPHandler) registryNameAction(w http.ResponseWriter, r *http.Request) {
	name, action := splitAction(r.PathValue("nameAction"))
	if name == "" {
		responsehttpapi.Error(w, http.StatusBadRequest, "INVALID_REQUEST",
			"missing entry name in path", nil)
		return
	}
	if action != "install" {
		responsehttpapi.Error(w, http.StatusBadRequest, "INVALID_REQUEST",
			"unknown action "+action, nil)
		return
	}

	var body struct {
		Env  map[string]string `json:"env,omitempty"`
		Args map[string]string `json:"args,omitempty"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, importMaxBytes)).Decode(&body); err != nil {
		if !errors.Is(err, io.EOF) {
			responsehttpapi.Error(w, http.StatusBadRequest, "INVALID_REQUEST",
				"handlers.InstallFromRegistry: decode body: "+err.Error(), nil)
			return
		}
	}

	st, err := h.svc.InstallFromRegistry(r.Context(), name, body.Env, body.Args)
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusCreated, st)
}
