package handlers

import (
	"net/http"

	"go.uber.org/zap"

	aispawnapp "github.com/sunweilin/forgify/backend/internal/app/aispawn"
	documentapp "github.com/sunweilin/forgify/backend/internal/app/document"
	documentdomain "github.com/sunweilin/forgify/backend/internal/domain/document"
	mentiondomain "github.com/sunweilin/forgify/backend/internal/domain/mention"
	errorspkg "github.com/sunweilin/forgify/backend/internal/pkg/errors"
	responsehttpapi "github.com/sunweilin/forgify/backend/internal/transport/httpapi/response"
)

// DocumentHandler hosts the document-tree HTTP endpoints (tree CRUD + move). The :iterate verb
// opens an AI conversation to edit this document via aispawn.
//
// DocumentHandler 持文档树的 HTTP 端点（树 CRUD + move）。:iterate 动词经 aispawn 开一个 AI 对话来编辑本文档。
type DocumentHandler struct {
	svc     *documentapp.Service
	aispawn *aispawnapp.Service
	log     *zap.Logger
}

// NewDocumentHandler constructs the handler.
//
// NewDocumentHandler 构造 handler。
func NewDocumentHandler(svc *documentapp.Service, aispawn *aispawnapp.Service, log *zap.Logger) *DocumentHandler {
	if log == nil {
		log = zap.NewNop()
	}
	return &DocumentHandler{svc: svc, aispawn: aispawn, log: log.Named("handlers.document")}
}

// Register wires the endpoints onto mux.
//
// Register 把端点挂到 mux。
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
	ParentID *string `json:"parentId,omitempty"` // null/omit = move to root
	Position *int    `json:"position,omitempty"` // omit = append to end
}

// List returns direct children of parentId (or root when the query param is empty).
//
// List 返 parentId 直接子节点（参数空 = root 级）。
func (h *DocumentHandler) List(w http.ResponseWriter, r *http.Request) {
	var parentID *string
	if pid := r.URL.Query().Get("parentId"); pid != "" {
		parentID = &pid
	}
	rows, err := h.svc.ListByParent(r.Context(), parentID)
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, rows)
}

// Tree returns the whole tree's metadata (no content) for a one-shot sidebar load.
//
// Tree 返整树 metadata（不含 content），给前端侧边栏一次拉满。
func (h *DocumentHandler) Tree(w http.ResponseWriter, r *http.Request) {
	rows, err := h.svc.ListAll(r.Context())
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
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
	// 级联删子树;DELETE 统一 204(被删数可由删后 list 推得)。
	if _, err := h.svc.Delete(r.Context(), r.PathValue("id")); err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.NoContent(w)
}

// postOnDoc dispatches POST /api/v1/documents/{id}:move / :iterate / :duplicate.
//
// postOnDoc 派发 POST /api/v1/documents/{id}:move / :iterate / :duplicate。
func (h *DocumentHandler) postOnDoc(w http.ResponseWriter, r *http.Request) {
	id, action, ok := idAndAction(r, "idAction")
	if !ok {
		responsehttpapi.FromDomainError(w, h.log, errorspkg.ErrNotFound)
		return
	}
	switch action {
	case "iterate":
		iterateEntity(w, r, h.log, h.aispawn, mentiondomain.MentionDocument, id)
	case "move":
		var req moveDocumentRequest
		if err := decodeJSON(r, &req); err != nil {
			responsehttpapi.FromDomainError(w, h.log, err)
			return
		}
		d, err := h.svc.Move(r.Context(), id, documentdomain.MoveInput{ParentID: req.ParentID, Position: req.Position})
		if err != nil {
			responsehttpapi.FromDomainError(w, h.log, err)
			return
		}
		responsehttpapi.Success(w, http.StatusOK, d)
	case "duplicate":
		// Optional body {parentId}: null/omit → copy lands as a sibling of the source.
		// 可选 body {parentId}：null/缺省 → 副本落为源的兄弟。
		var req struct {
			ParentID *string `json:"parentId,omitempty"`
		}
		if r.ContentLength != 0 {
			if err := decodeJSON(r, &req); err != nil {
				responsehttpapi.FromDomainError(w, h.log, err)
				return
			}
		}
		d, err := h.svc.Duplicate(r.Context(), id, req.ParentID)
		if err != nil {
			responsehttpapi.FromDomainError(w, h.log, err)
			return
		}
		responsehttpapi.Created(w, d) // a new subtree → 201 bare entity (the new root)
	default:
		responsehttpapi.FromDomainError(w, h.log, errorspkg.ErrNotFound)
	}
}
