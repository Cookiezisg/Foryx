package handlers

import (
	"net/http"

	"go.uber.org/zap"

	memoryapp "github.com/sunweilin/forgify/backend/internal/app/memory"
	memorydomain "github.com/sunweilin/forgify/backend/internal/domain/memory"
	responsehttpapi "github.com/sunweilin/forgify/backend/internal/transport/httpapi/response"
)

// MemoryHandler hosts the 7 memory HTTP endpoints.
//
// MemoryHandler 持 memory 的 7 个 HTTP 端点。
type MemoryHandler struct {
	svc *memoryapp.Service
	log *zap.Logger
}

func NewMemoryHandler(svc *memoryapp.Service, log *zap.Logger) *MemoryHandler {
	return &MemoryHandler{svc: svc, log: log}
}

func (h *MemoryHandler) Register(mux Registrar) {
	mux.HandleFunc("GET /api/v1/memories", h.List)
	mux.HandleFunc("POST /api/v1/memories", h.Create)
	mux.HandleFunc("GET /api/v1/memories/{name}", h.Get)
	mux.HandleFunc("PATCH /api/v1/memories/{name}", h.Update)
	mux.HandleFunc("DELETE /api/v1/memories/{name}", h.Delete)
	mux.HandleFunc("POST /api/v1/memories/{nameAction}", h.postOnMemory)
}

type createMemoryRequest struct {
	Name        string         `json:"name"`
	Type        string         `json:"type"`
	Description string         `json:"description"`
	Content     string         `json:"content"`
	Pinned      *bool          `json:"pinned,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

type updateMemoryRequest struct {
	Type        *string        `json:"type,omitempty"`
	Description *string        `json:"description,omitempty"`
	Content     *string        `json:"content,omitempty"`
	Pinned      *bool          `json:"pinned,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

func (h *MemoryHandler) List(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	filter := memorydomain.ListFilter{}
	if t := q.Get("type"); t != "" {
		filter.Type = t
	}
	if p := q.Get("pinned"); p != "" {
		val := p == "true" || p == "1"
		filter.Pinned = &val
	}
	items, err := h.svc.List(r.Context(), filter)
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, items)
}

// Create persists a memory with source = SourceUser.
//
// Create 落库 memory,source 自动为 SourceUser。
func (h *MemoryHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req createMemoryRequest
	if err := decodeJSON(r, &req); err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	m, err := h.svc.Create(r.Context(), memoryapp.UpsertInput{
		Name:        req.Name,
		Type:        req.Type,
		Description: req.Description,
		Content:     req.Content,
		Pinned:      req.Pinned,
		Source:      memorydomain.SourceUser,
		Metadata:    req.Metadata,
	})
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Created(w, m)
}

func (h *MemoryHandler) Get(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	m, err := h.svc.Get(r.Context(), name)
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, m)
}

// Update is a partial PATCH; absent fields keep current values; source is fixed.
//
// Update 部分 PATCH;缺失字段保持原值;source 永不变。
func (h *MemoryHandler) Update(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	var req updateMemoryRequest
	if err := decodeJSON(r, &req); err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	existing, err := h.svc.Get(r.Context(), name)
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	in := memoryapp.UpsertInput{
		Name:        existing.Name,
		Type:        existing.Type,
		Description: existing.Description,
		Content:     existing.Content,
		Pinned:      req.Pinned,
		Source:      existing.Source,
		Metadata:    existing.Metadata,
	}
	if req.Type != nil {
		in.Type = *req.Type
	}
	if req.Description != nil {
		in.Description = *req.Description
	}
	if req.Content != nil {
		in.Content = *req.Content
	}
	if req.Metadata != nil {
		in.Metadata = req.Metadata
	}
	m, err := h.svc.Upsert(r.Context(), in)
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, m)
}

func (h *MemoryHandler) Delete(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.Delete(r.Context(), r.PathValue("name")); err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.NoContent(w)
}

// postOnMemory dispatches POST /api/v1/memories/{name}:pin and :unpin.
//
// postOnMemory 派发 POST /api/v1/memories/{name}:pin 与 :unpin。
func (h *MemoryHandler) postOnMemory(w http.ResponseWriter, r *http.Request) {
	name, action, ok := idAndAction(r, "nameAction")
	if !ok {
		responsehttpapi.Error(w, http.StatusNotFound, "NOT_FOUND", "unknown route", nil)
		return
	}
	var (
		m   *memorydomain.Memory
		err error
	)
	switch action {
	case "pin":
		m, err = h.svc.Pin(r.Context(), name)
	case "unpin":
		m, err = h.svc.Unpin(r.Context(), name)
	default:
		responsehttpapi.Error(w, http.StatusNotFound, "NOT_FOUND", "unknown action: "+action, nil)
		return
	}
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, m)
}
