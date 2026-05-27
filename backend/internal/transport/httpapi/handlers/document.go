package handlers

import (
	"net/http"

	"go.uber.org/zap"

	askai "github.com/sunweilin/forgify/backend/internal/app/askai"
	documentapp "github.com/sunweilin/forgify/backend/internal/app/document"
	documentdomain "github.com/sunweilin/forgify/backend/internal/domain/document"
	responsehttpapi "github.com/sunweilin/forgify/backend/internal/transport/httpapi/response"
)

// DocumentHandler hosts the 7 document HTTP endpoints (tree CRUD + move).
//
// DocumentHandler 持文档树的 7 个 HTTP 端点(CRUD + move)。
type DocumentHandler struct {
	svc     *documentapp.Service
	spawner *askai.Spawner // optional; nil disables :iterate
	log     *zap.Logger
}

func NewDocumentHandler(svc *documentapp.Service, log *zap.Logger) *DocumentHandler {
	return &DocumentHandler{svc: svc, log: log}
}

func (h *DocumentHandler) SetSpawner(s *askai.Spawner) { h.spawner = s }

func (h *DocumentHandler) Register(mux Registrar) {
	mux.HandleFunc("GET /api/v1/documents", h.List)
	mux.HandleFunc("GET /api/v1/documents/tree", h.Tree)
	mux.HandleFunc("POST /api/v1/documents", h.Create)
	mux.HandleFunc("GET /api/v1/documents/{id}", h.Get)
	mux.HandleFunc("PATCH /api/v1/documents/{id}", h.Update)
	mux.HandleFunc("DELETE /api/v1/documents/{id}", h.Delete)
	mux.HandleFunc("POST /api/v1/documents/{idAction}", h.postOnDoc)
}

type createDocumentRequest struct {
	Name        string   `json:"name"`
	ParentID    *string  `json:"parentId,omitempty"`
	Description string   `json:"description,omitempty"`
	Content     string   `json:"content,omitempty"`
	Tags        []string `json:"tags,omitempty"`
}

type updateDocumentRequest struct {
	Name        *string   `json:"name,omitempty"`
	Description *string   `json:"description,omitempty"`
	Content     *string   `json:"content,omitempty"`
	Tags        *[]string `json:"tags,omitempty"`
}

type moveDocumentRequest struct {
	ParentID *string `json:"parentId,omitempty"` // null/omit means move to root
	Position *int    `json:"position,omitempty"` // omit means append to end
}

// List returns direct children of parentID (or root when parentId query param missing/empty).
//
// List 返 parentId 直接子节点(参数空 = root 级)。
func (h *DocumentHandler) List(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	var parentID *string
	if pid := q.Get("parentId"); pid != "" {
		parentID = &pid
	}
	rows, err := h.svc.ListByParent(r.Context(), parentID)
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, rows)
}

// Tree returns the entire tree metadata (no content) for the sidebar one-shot load.
//
// Tree 返整树 metadata(不含 content),给前端侧边栏一次拉满。
func (h *DocumentHandler) Tree(w http.ResponseWriter, r *http.Request) {
	rows, err := h.svc.ListAll(r.Context())
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	// Strip content + nil empty tags so payload stays small even with N docs.
	//
	// 抠掉 content + 空 tags 让多文档时 payload 不胖。
	out := make([]map[string]any, len(rows))
	for i, d := range rows {
		out[i] = map[string]any{
			"id":          d.ID,
			"parentId":    d.ParentID,
			"name":        d.Name,
			"description": d.Description,
			"path":        d.Path,
			"position":    d.Position,
			"sizeBytes":   d.SizeBytes,
			"tags":        d.Tags,
			"createdAt":   d.CreatedAt,
			"updatedAt":   d.UpdatedAt,
		}
	}
	responsehttpapi.Success(w, http.StatusOK, out)
}

func (h *DocumentHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req createDocumentRequest
	if err := decodeJSON(r, &req); err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	d, err := h.svc.Create(r.Context(), documentapp.CreateInput{
		Name:        req.Name,
		ParentID:    req.ParentID,
		Description: req.Description,
		Content:     req.Content,
		Tags:        req.Tags,
	})
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Created(w, d)
}

func (h *DocumentHandler) Get(w http.ResponseWriter, r *http.Request) {
	d, err := h.svc.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, d)
}

func (h *DocumentHandler) Update(w http.ResponseWriter, r *http.Request) {
	var req updateDocumentRequest
	if err := decodeJSON(r, &req); err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	d, err := h.svc.Update(r.Context(), r.PathValue("id"), documentapp.UpdateInput{
		Name:        req.Name,
		Description: req.Description,
		Content:     req.Content,
		Tags:        req.Tags,
	})
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, d)
}

func (h *DocumentHandler) Delete(w http.ResponseWriter, r *http.Request) {
	n, err := h.svc.Delete(r.Context(), r.PathValue("id"))
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, map[string]any{
		"id":           r.PathValue("id"),
		"deletedCount": n,
	})
}

// postOnDoc dispatches POST /api/v1/documents/{id}:move (and future :action verbs).
//
// postOnDoc 派发 POST /api/v1/documents/{id}:move(以及未来扩展的 :action)。
func (h *DocumentHandler) postOnDoc(w http.ResponseWriter, r *http.Request) {
	id, action, ok := idAndAction(r, "idAction")
	if !ok {
		responsehttpapi.Error(w, http.StatusNotFound, "NOT_FOUND", "unknown route", nil)
		return
	}
	switch action {
	case "move":
		h.move(w, r, id)
	case "iterate":
		h.Iterate(w, r, id)
	default:
		responsehttpapi.Error(w, http.StatusNotFound, "NOT_FOUND", "unknown action: "+action, nil)
	}
}

// Iterate — see FunctionHandler.Iterate for semantics.
//
// Iterate —— 语义见 FunctionHandler.Iterate。
func (h *DocumentHandler) Iterate(w http.ResponseWriter, r *http.Request, id string) {
	if h.spawner == nil {
		responsehttpapi.Error(w, http.StatusServiceUnavailable, "ASKAI_NOT_AVAILABLE",
			"askai spawner not wired", nil)
		return
	}
	var req struct {
		Prompt string `json:"prompt"`
	}
	if err := decodeJSON(r, &req); err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	sysPrompt, err := askai.BuildDocumentContext(r.Context(), id, h.svc)
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	result, err := h.spawner.Spawn(r.Context(), askai.SpawnInput{
		SystemPrompt: sysPrompt,
		UserPrompt:   req.Prompt,
	})
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, result)
}

func (h *DocumentHandler) move(w http.ResponseWriter, r *http.Request, id string) {
	var req moveDocumentRequest
	if err := decodeJSON(r, &req); err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	d, err := h.svc.Move(r.Context(), id, documentdomain.MoveInput{
		ParentID: req.ParentID,
		Position: req.Position,
	})
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, d)
}
