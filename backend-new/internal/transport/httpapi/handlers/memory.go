package handlers

import (
	"net/http"

	"go.uber.org/zap"

	memoryapp "github.com/sunweilin/forgify/backend/internal/app/memory"
	memorydomain "github.com/sunweilin/forgify/backend/internal/domain/memory"
	responsehttpapi "github.com/sunweilin/forgify/backend/internal/transport/httpapi/response"
)

// MemoryHandler serves the per-workspace memory management UI: list / get / upsert /
// delete + pin / unpin. Memories are markdown files (see domains/memory.md); the
// LLM-facing read/write/forget tools come later (波次 2/3).
//
// MemoryHandler 提供按 workspace 的记忆管理 UI：list / get / upsert / delete + pin /
// unpin。记忆是 markdown 文件（见 domains/memory.md）；LLM 用的 read/write/forget 工具
// 留波次 2/3。
type MemoryHandler struct {
	svc *memoryapp.Service
	log *zap.Logger
}

// NewMemoryHandler constructs the handler.
//
// NewMemoryHandler 构造 handler。
func NewMemoryHandler(svc *memoryapp.Service, log *zap.Logger) *MemoryHandler {
	if log == nil {
		log = zap.NewNop()
	}
	return &MemoryHandler{svc: svc, log: log.Named("handlers.memory")}
}

// Register wires the endpoints onto mux.
//
// Register 把端点挂到 mux。
func (h *MemoryHandler) Register(mux Registrar) {
	mux.HandleFunc("GET /api/v1/memories", h.List)
	mux.HandleFunc("GET /api/v1/memories/{name}", h.Get)
	mux.HandleFunc("PUT /api/v1/memories/{name}", h.Upsert)
	mux.HandleFunc("DELETE /api/v1/memories/{name}", h.Delete)
	mux.HandleFunc("POST /api/v1/memories/{name}/pin", h.Pin)
	mux.HandleFunc("POST /api/v1/memories/{name}/unpin", h.Unpin)
}

// List handles GET /api/v1/memories — the workspace's memories, optional ?pinned=.
//
// List 处理 GET /api/v1/memories —— 该 workspace 的记忆，可选 ?pinned=。
func (h *MemoryHandler) List(w http.ResponseWriter, r *http.Request) {
	var filter memorydomain.ListFilter
	if v := r.URL.Query().Get("pinned"); v != "" {
		b := v == "true"
		filter.Pinned = &b
	}
	items, err := h.svc.List(r.Context(), filter)
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, items)
}

// Get handles GET /api/v1/memories/{name} — one memory's full content.
//
// Get 处理 GET /api/v1/memories/{name} —— 一条记忆的全文。
func (h *MemoryHandler) Get(w http.ResponseWriter, r *http.Request) {
	m, err := h.svc.Get(r.Context(), r.PathValue("name"))
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, m)
}

type upsertMemoryRequest struct {
	Description string `json:"description"`
	Content     string `json:"content"`
	Pinned      bool   `json:"pinned"`
	Source      string `json:"source"`
}

// Upsert handles PUT /api/v1/memories/{name} — create or update.
//
// Upsert 处理 PUT /api/v1/memories/{name} —— 创建或更新。
func (h *MemoryHandler) Upsert(w http.ResponseWriter, r *http.Request) {
	var req upsertMemoryRequest
	if err := decodeJSON(r, &req); err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	m, err := h.svc.Upsert(r.Context(), memoryapp.UpsertInput{
		Name:        r.PathValue("name"),
		Description: req.Description,
		Content:     req.Content,
		Pinned:      req.Pinned,
		Source:      req.Source,
	})
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, m)
}

// Delete handles DELETE /api/v1/memories/{name}.
//
// Delete 处理 DELETE /api/v1/memories/{name}。
func (h *MemoryHandler) Delete(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.Delete(r.Context(), r.PathValue("name")); err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.NoContent(w)
}

// Pin handles POST /api/v1/memories/{name}/pin.
//
// Pin 处理 POST /api/v1/memories/{name}/pin。
func (h *MemoryHandler) Pin(w http.ResponseWriter, r *http.Request) {
	m, err := h.svc.Pin(r.Context(), r.PathValue("name"))
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, m)
}

// Unpin handles POST /api/v1/memories/{name}/unpin.
//
// Unpin 处理 POST /api/v1/memories/{name}/unpin。
func (h *MemoryHandler) Unpin(w http.ResponseWriter, r *http.Request) {
	m, err := h.svc.Unpin(r.Context(), r.PathValue("name"))
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, m)
}
