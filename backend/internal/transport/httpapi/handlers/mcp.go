// mcp.go — HTTP handler for /api/v1/mcp-* server CRUD/import/reconnect/
// health-check + /api/v1/mcp-registry list/get/install. Thin: decode →
// service → envelope. The search_mcp + call_mcp SYSTEM TOOLS are what
// LLMs use at runtime; these endpoints are the UI's configuration +
// observability surface.
//
// Endpoints (per mcp.md §10):
//
//	GET    /api/v1/mcp-servers                       list all (status + tools + health)
//	GET    /api/v1/mcp-servers/{name}                single server detail
//	PUT    /api/v1/mcp-servers/{name}                upsert config + Connect
//	DELETE /api/v1/mcp-servers/{name}                disconnect + remove
//	POST   /api/v1/mcp-servers:import                drag-import (multipart or JSON body)
//	POST   /api/v1/mcp-servers/{name}:reconnect      force restart subprocess
//	POST   /api/v1/mcp-servers/{name}:health-check   active probe (tools/list)
//
//	GET    /api/v1/mcp-registry                      built-in marketplace catalog
//	GET    /api/v1/mcp-registry/{name}               single entry detail
//	POST   /api/v1/mcp-registry/{name}:install       fill env+args → write mcp.json → Connect
//
// Action-style routes (:reconnect / :health-check / :install / :import)
// use the {nameAction} wildcard + splitAction helper, same pattern as
// sandbox.go — stdlib mux supports {var} but not arbitrary action
// suffixes, so we route the verb-bearing prefix and dispatch on the
// suffix inside the handler.
//
// mcp.go ——/api/v1/mcp-* + /api/v1/mcp-registry HTTP handler。薄层：decode
// → service → envelope。LLM 用 search_mcp + call_mcp 系统工具运行时调；这些
// 端点是 UI 配置+观测面。:action 路由用 {nameAction} 通配 + splitAction，
// 同 sandbox.go——stdlib mux 不支持任意 action 后缀。
package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"go.uber.org/zap"

	mcpapp "github.com/sunweilin/forgify/backend/internal/app/mcp"
	mcpdomain "github.com/sunweilin/forgify/backend/internal/domain/mcp"
	mcpinfra "github.com/sunweilin/forgify/backend/internal/infra/mcp"
	responsehttpapi "github.com/sunweilin/forgify/backend/internal/transport/httpapi/response"
)

// importMaxBytes caps the upload size for :import to prevent runaway
// memory growth from a malicious / accidental huge JSON file. 1MB is
// generous (Claude Desktop configs are typically <10KB).
//
// importMaxBytes 限定 :import 上传大小防恶意/意外大 JSON 文件爆内存。
// 1MB 宽裕（Claude Desktop 配置通常 <10KB）。
const importMaxBytes = 1 << 20

// MCPHandler serves the /api/v1/mcp-* + /api/v1/mcp-registry routes.
//
// MCPHandler 提供 /api/v1/mcp-* + /api/v1/mcp-registry 路由。
type MCPHandler struct {
	svc *mcpapp.Service
	log *zap.Logger
}

// NewMCPHandler wires the handler dependencies.
//
// NewMCPHandler 装配 handler 依赖。
func NewMCPHandler(svc *mcpapp.Service, log *zap.Logger) *MCPHandler {
	return &MCPHandler{svc: svc, log: log}
}

// Register attaches the 10 MCP routes to mux.
//
// Register 把 10 个 MCP 路由挂到 mux。
func (h *MCPHandler) Register(mux *http.ServeMux) {
	// Server CRUD/lifecycle
	mux.HandleFunc("GET /api/v1/mcp-servers", h.ListServers)
	mux.HandleFunc("GET /api/v1/mcp-servers/{name}", h.GetServer)
	mux.HandleFunc("GET /api/v1/mcp-servers/{name}/stderr", h.GetServerStderr)
	mux.HandleFunc("PUT /api/v1/mcp-servers/{name}", h.PutServer)
	mux.HandleFunc("DELETE /api/v1/mcp-servers/{name}", h.DeleteServer)

	// Server :action endpoints. POST /mcp-servers:import has no name in
	// the path, so it goes through serversAction which dispatches by
	// action. POST /mcp-servers/{nameAction}:action uses splitAction.
	//
	// :action 端点。POST /mcp-servers:import 路径无 name，走 serversAction
	// 按 action 派发。POST /mcp-servers/{nameAction}:action 用 splitAction。
	mux.HandleFunc("POST /api/v1/mcp-servers/{nameAction}", h.serverNameAction)
	// :import has no name in the path; stdlib mux can't wildcard-match
	// when there's no '/' before the action, so register the literal.
	// :import 路径无 name；stdlib mux 在 action 前无 '/' 时通配不到，直接
	// 登记字面 path。
	mux.HandleFunc("POST /api/v1/mcp-servers:import", h.importServers)

	// Registry
	mux.HandleFunc("GET /api/v1/mcp-registry", h.SearchRegistry)
	mux.HandleFunc("GET /api/v1/mcp-registry/{name}", h.GetRegistryEntry)
	mux.HandleFunc("POST /api/v1/mcp-registry/{nameAction}", h.registryNameAction)
}

// ── Server reads ─────────────────────────────────────────────────────

// ListServers: GET /api/v1/mcp-servers → 200 [{ServerStatus...}].
//
// ListServers: GET /api/v1/mcp-servers → 200 [{ServerStatus...}]。
func (h *MCPHandler) ListServers(w http.ResponseWriter, r *http.Request) {
	servers := h.svc.ListServers(r.Context())
	responsehttpapi.Success(w, http.StatusOK, servers)
}

// GetServerStderr: GET /api/v1/mcp-servers/{name}/stderr → 200 with the
// captured stderr ring-buffer (≤ 256 KB) of the named server's subprocess.
// Empty string when configured-but-not-connected. 404 when no such
// server is configured. Used by testend's MCP tab to debug connection
// failures (handshake errors, missing executables, runtime crashes —
// all surface in stderr).
//
// GetServerStderr: GET /api/v1/mcp-servers/{name}/stderr → 200 返子进程
// stderr 环形缓冲（≤ 256 KB）。配置了未连接返空。未配置 404。testend
// MCP tab 用来调试连接失败。
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

// GetServer: GET /api/v1/mcp-servers/{name} → 200 ServerStatus / 404.
//
// GetServer: GET /api/v1/mcp-servers/{name} → 200 / 404。
func (h *MCPHandler) GetServer(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	st, err := h.svc.GetServer(r.Context(), name)
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, st)
}

// ── Server CRUD ──────────────────────────────────────────────────────

// PutServer: PUT /api/v1/mcp-servers/{name} — upsert + Connect. Body
// is a partial ServerConfig (Name is taken from the path; mcp.md §5
// says Name is the map key on disk, not duplicated in the value).
// Returns 200 with the resulting ServerStatus regardless of whether
// the underlying connect succeeded — caller checks status field.
//
// PutServer: PUT /api/v1/mcp-servers/{name} 创建/更新 + Connect。Body 是
// 部分 ServerConfig（Name 从 path 取；mcp.md §5：Name 是 disk 的 map key
// 不在 value 重复）。返 200 + ServerStatus 不论 connect 是否成功——调用方
// 看 status 字段。
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
		// AddServer can fail at mcp.json save (filesystem) or at Connect
		// (subprocess spawn). Both should still surface a status row to
		// the caller — fetch and return it alongside the warning.
		// AddServer 可能 mcp.json 保存（文件系统）失败或 Connect（子进程 spawn）失败。
		// 都该把状态行返给调用方——拉一下随告警一起返。
		st, gErr := h.svc.GetServer(r.Context(), name)
		if gErr == nil && st != nil {
			h.log.Warn("PUT mcp-server completed with connect issue",
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

// DeleteServer: DELETE /api/v1/mcp-servers/{name} → 204 / 404.
//
// DeleteServer: DELETE /api/v1/mcp-servers/{name} → 204 / 404。
func (h *MCPHandler) DeleteServer(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := h.svc.RemoveServer(r.Context(), name); err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ── Server :action dispatch ──────────────────────────────────────────

// serverNameAction handles POST /api/v1/mcp-servers/{name}:reconnect
// and POST /api/v1/mcp-servers/{name}:health-check. nameAction is the
// "<name>:<action>" path tail.
//
// serverNameAction 处理 POST /api/v1/mcp-servers/{name}:reconnect 和
// POST /api/v1/mcp-servers/{name}:health-check。nameAction 是
// "<name>:<action>" 路径尾。
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

// importServers handles POST /api/v1/mcp-servers:import. Accepts:
//   - JSON body: `{"mcpServers": {...}}` Claude Desktop fragment
//     (preferred — easy for the testend console + UI clipboard paste)
//   - multipart form-data: file field "config" carrying mcp.json
//
// Query param ?overwrite=true forces replace of conflicts; default
// keeps existing entries and returns them in MergeResult.Conflicts so
// the UI can prompt for confirmation.
//
// importServers 处理 POST /api/v1/mcp-servers:import。接受 JSON body
// （Claude Desktop 片段，testend 控制台 + UI 剪贴板粘贴方便）或 multipart
// form-data 上传 mcp.json 文件。?overwrite=true 强制覆盖；默认保留已存在
// 并在 MergeResult.Conflicts 返让 UI 弹确认。
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
		// Already capped by ParseMultipartForm; an additional read with
		// MaxBytesReader guards against weird filesystem-sourced files.
		// 已被 ParseMultipartForm 限上限；再加 MaxBytesReader 防奇怪文件来源。
		raw = make([]byte, 0, 4096)
		buf := make([]byte, 4096)
		for {
			n, rerr := file.Read(buf)
			if n > 0 {
				raw = append(raw, buf[:n]...)
				if int64(len(raw)) > importMaxBytes {
					responsehttpapi.Error(w, http.StatusRequestEntityTooLarge, "INVALID_REQUEST",
						"config file exceeds 1MB limit", nil)
					return
				}
			}
			if rerr != nil {
				break
			}
		}
	} else {
		// JSON body path. Cap at importMaxBytes for symmetry with
		// multipart.
		// JSON body 路径。importMaxBytes 与 multipart 对称。
		body := http.MaxBytesReader(w, r.Body, importMaxBytes)
		var err error
		raw, err = readAll(body)
		if err != nil {
			responsehttpapi.Error(w, http.StatusBadRequest, "INVALID_REQUEST",
				"failed to read body: "+err.Error(), nil)
			return
		}
	}

	// Parse the Claude Desktop fragment into our config shape.
	// 解 Claude Desktop 片段为我们的 config 形状。
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

// ── Registry ─────────────────────────────────────────────────────────

// SearchRegistry: GET /api/v1/mcp-registry?search=<query> → 200
// [{RegistryEntry...}]. Empty / missing search returns 400 — the marketplace
// has 5000+ entries; full listing is disallowed (callers must search by
// keyword). Server-side filter on the upstream registry; multi-word
// queries are tokenized client-side.
//
// SearchRegistry: GET /api/v1/mcp-registry?search=<query> → 200。空 / 缺
// search 返 400——marketplace 5000+ 条，全列禁止（必须按关键词搜）。
// 上游 server-side 过滤；多词 query 客户端拆词。
func (h *MCPHandler) SearchRegistry(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("search")
	entries, err := h.svc.SearchRegistry(r.Context(), query)
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, entries)
}

// GetRegistryEntry: GET /api/v1/mcp-registry/{name} → 200 / 404.
// Returns Hidden entries too — the install flow needs them.
//
// GetRegistryEntry: GET /api/v1/mcp-registry/{name} → 200 / 404。
// 含 Hidden——install 流需要。
func (h *MCPHandler) GetRegistryEntry(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	entry, err := h.svc.GetRegistryEntry(r.Context(), name)
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, entry)
}

// registryNameAction handles POST /api/v1/mcp-registry/{name}:install
// (currently the only action). Body is the install payload:
// `{"env": {...}, "args": {...}}`. Validates env+args against the
// entry's RequiredEnv/RequiredArgs, delegates runtime install to
// sandboxapp, writes the resulting ServerConfig to mcp.json, Connects.
//
// registryNameAction 处理 POST /api/v1/mcp-registry/{name}:install
// （当前唯一 action）。Body 是 install 载荷 {env, args}。校验 env+args
// 后委托 sandboxapp 装 runtime，写 ServerConfig 到 mcp.json，Connect。
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
		// Empty body is OK — entry might have no RequiredEnv/RequiredArgs.
		// Treat any decode error as "no body provided".
		// 空 body OK——entry 可能无 RequiredEnv/RequiredArgs。任何 decode 错
		// 视作"未提供 body"。
		if !errors.Is(err, errEmptyBody) {
			body.Env = nil
			body.Args = nil
		}
	}

	// Curated catalog has no separate alias — name doubles as the
	// mcp.json key.
	// curated 目录无独立 alias —— name 直接作 mcp.json key。
	st, err := h.svc.InstallFromRegistry(r.Context(), name, body.Env, body.Args)
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusCreated, st)
}

// ── helpers ──────────────────────────────────────────────────────────

// readAll reads the body in one shot. Wraps the standard pattern so
// the caller stays a single line.
//
// readAll 一次读完 body。包标准模式让调用方保持单行。
func readAll(r interface{ Read([]byte) (int, error) }) ([]byte, error) {
	out := make([]byte, 0, 4096)
	buf := make([]byte, 4096)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			out = append(out, buf[:n]...)
		}
		if err != nil {
			if err.Error() == "EOF" {
				return out, nil
			}
			return out, err
		}
	}
}

// errEmptyBody is the sentinel returned for missing/empty bodies in
// the install path so we can distinguish "user provided no env/args"
// from "decode failure on bad JSON". Currently unused (the install
// path treats any decode failure as no-body); kept so a future strict
// mode can switch on it.
//
// errEmptyBody 是 install 路径中 missing/empty body 的 sentinel，让我们
// 区分"用户没传 env/args"与"JSON 解析失败"。当前未用（install 路径把任
// 何解析失败当无 body）；保留给将来严格模式 switch。
var errEmptyBody = errors.New("empty body")

// Compile-time keep-alive — silences unused-import lint when refactors
// trim the file.
//
// 编译期保活——重构 trim 文件时静默 unused-import lint。
var _ = mcpinfra.MergeResult{}
