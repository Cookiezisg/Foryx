package handlers

import (
	"net/http"

	"go.uber.org/zap"

	workspaceapp "github.com/sunweilin/forgify/backend/internal/app/workspace"
	modeldomain "github.com/sunweilin/forgify/backend/internal/domain/model"
	responsehttpapi "github.com/sunweilin/forgify/backend/internal/transport/httpapi/response"
)

// WorkspacesHandler serves /api/v1/workspaces — local workspace (isolation root)
// management. Unauthenticated: these run before a workspace is selected (onboarding),
// so they are exempt from RequireWorkspace.
//
// WorkspacesHandler 提供 /api/v1/workspaces——本地 workspace（隔离根）管理。无需鉴权：
// 这些在选定 workspace 前运行（onboarding），故豁免 RequireWorkspace。
type WorkspacesHandler struct {
	svc *workspaceapp.Service
	log *zap.Logger
}

func NewWorkspacesHandler(svc *workspaceapp.Service, log *zap.Logger) *WorkspacesHandler {
	if log == nil {
		log = zap.NewNop()
	}
	return &WorkspacesHandler{svc: svc, log: log.Named("handlers.workspaces")}
}

func (h *WorkspacesHandler) Register(mux Registrar) {
	mux.HandleFunc("GET /api/v1/workspaces", h.List)
	mux.HandleFunc("POST /api/v1/workspaces", h.Create)
	mux.HandleFunc("GET /api/v1/workspaces/{id}", h.Get)
	mux.HandleFunc("PATCH /api/v1/workspaces/{id}", h.Update)
	mux.HandleFunc("DELETE /api/v1/workspaces/{id}", h.Delete)
	// Per-scenario default model selection — a workspace-scoped preference (alongside language).
	// 按 scenario 的默认模型选择——workspace 级偏好（与 language 并列）。
	mux.HandleFunc("PUT /api/v1/workspaces/{id}/default-models/{scenario}", h.SetDefaultModel)
	// Default search key — the single explicit api-key WebSearch uses (provider implied).
	// 默认搜索 key——WebSearch 用的唯一显式 api-key（provider 隐含）。
	mux.HandleFunc("PUT /api/v1/workspaces/{id}/default-search", h.SetDefaultSearch)
	mux.HandleFunc("DELETE /api/v1/workspaces/{id}/default-search", h.ClearDefaultSearch)
	// Go 1.22+ ServeMux disallows a literal `{id}:action` segment — capture the
	// whole `{idAction}` and split in postOnWorkspace.
	// Go 1.22+ ServeMux 禁止字面 `{id}:action` 分段——整体捕获 {idAction}，在 postOnWorkspace 拆分。
	mux.HandleFunc("POST /api/v1/workspaces/{idAction}", h.postOnWorkspace)
}

type createWorkspaceRequest struct {
	Name        string `json:"name"`
	AvatarColor string `json:"avatarColor"`
	Language    string `json:"language"`
}

type updateWorkspaceRequest struct {
	Name        *string `json:"name,omitempty"`
	AvatarColor *string `json:"avatarColor,omitempty"`
	Language    *string `json:"language,omitempty"`
}

type setDefaultModelRequest struct {
	APIKeyID string            `json:"apiKeyId"`
	ModelID  string            `json:"modelId"`
	Options  map[string]string `json:"options,omitempty"`
}

type setDefaultSearchRequest struct {
	APIKeyID string `json:"apiKeyId"`
}

func (h *WorkspacesHandler) List(w http.ResponseWriter, r *http.Request) {
	rows, err := h.svc.List(r.Context())
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, rows)
}

func (h *WorkspacesHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req createWorkspaceRequest
	if err := decodeJSON(r, &req); err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	ws, err := h.svc.Create(r.Context(), workspaceapp.CreateInput{
		Name:        req.Name,
		AvatarColor: req.AvatarColor,
		Language:    req.Language,
	})
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Created(w, ws)
}

func (h *WorkspacesHandler) Get(w http.ResponseWriter, r *http.Request) {
	ws, err := h.svc.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, ws)
}

func (h *WorkspacesHandler) Update(w http.ResponseWriter, r *http.Request) {
	var req updateWorkspaceRequest
	if err := decodeJSON(r, &req); err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	ws, err := h.svc.Update(r.Context(), r.PathValue("id"), workspaceapp.UpdateInput{
		Name:        req.Name,
		AvatarColor: req.AvatarColor,
		Language:    req.Language,
	})
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, ws)
}

func (h *WorkspacesHandler) Delete(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.Delete(r.Context(), r.PathValue("id")); err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.NoContent(w)
}

// postOnWorkspace dispatches `:action` suffix routes (currently just :activate).
//
// postOnWorkspace 分派 `:action` 后缀路由（目前仅 :activate）。
func (h *WorkspacesHandler) postOnWorkspace(w http.ResponseWriter, r *http.Request) {
	id, action, ok := idAndAction(r, "idAction")
	if !ok {
		responsehttpapi.Error(w, http.StatusNotFound, "NOT_FOUND", "unknown action", nil)
		return
	}
	switch action {
	case "activate":
		h.activate(w, r, id)
	default:
		responsehttpapi.Error(w, http.StatusNotFound, "NOT_FOUND", "unknown action: "+action, nil)
	}
}

// activate marks a workspace most-recently-used; the client calls it when switching.
//
// activate 标 workspace 为最近使用；切换时客户端调。
func (h *WorkspacesHandler) activate(w http.ResponseWriter, r *http.Request, id string) {
	if err := h.svc.TouchLastUsed(r.Context(), id); err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	ws, err := h.svc.Get(r.Context(), id)
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, ws)
}

// SetDefaultModel sets a workspace's default model for one scenario (dialogue/utility/agent). The
// body is a ModelRef ({apiKeyId, modelId, options}); both ids are required (validated downstream).
//
// SetDefaultModel 设置 workspace 某 scenario（dialogue/utility/agent）的默认模型。body 是 ModelRef
// （{apiKeyId, modelId, options}）；两个 id 必填（下游校验）。
func (h *WorkspacesHandler) SetDefaultModel(w http.ResponseWriter, r *http.Request) {
	var req setDefaultModelRequest
	if err := decodeJSON(r, &req); err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	ref := &modeldomain.ModelRef{APIKeyID: req.APIKeyID, ModelID: req.ModelID, Options: req.Options}
	ws, err := h.svc.SetDefault(r.Context(), r.PathValue("id"), r.PathValue("scenario"), ref)
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, ws)
}

// SetDefaultSearch sets the workspace's default search api-key (body {apiKeyId}) — the
// single explicit key WebSearch uses; its provider is implied by the key itself.
//
// SetDefaultSearch 设置 workspace 默认搜索 api-key（body {apiKeyId}）——WebSearch 用的唯一显式 key；
// 其 provider 由 key 自身隐含。
func (h *WorkspacesHandler) SetDefaultSearch(w http.ResponseWriter, r *http.Request) {
	var req setDefaultSearchRequest
	if err := decodeJSON(r, &req); err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	ws, err := h.svc.SetDefaultSearch(r.Context(), r.PathValue("id"), req.APIKeyID)
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, ws)
}

// ClearDefaultSearch clears the workspace's default search key (DELETE) — WebSearch then
// falls back to MCP / the actionable "configure a search backend" message.
//
// ClearDefaultSearch 清除 workspace 默认搜索 key（DELETE）——WebSearch 据此降级到 MCP / 可操作的
// "去配置搜索后端"提示。
func (h *WorkspacesHandler) ClearDefaultSearch(w http.ResponseWriter, r *http.Request) {
	ws, err := h.svc.SetDefaultSearch(r.Context(), r.PathValue("id"), "")
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, ws)
}
