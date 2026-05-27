package handlers

import (
	"net/http"
	"strconv"

	"go.uber.org/zap"

	relationapp "github.com/sunweilin/forgify/backend/internal/app/relation"
	relationdomain "github.com/sunweilin/forgify/backend/internal/domain/relation"
	responsehttpapi "github.com/sunweilin/forgify/backend/internal/transport/httpapi/response"
)

// RelationHandler hosts 3 read-only HTTP endpoints for cross-entity relations.
// Relations are derived data — there is no POST/PATCH/DELETE; mutations happen
// implicitly via source-domain hooks.
//
// RelationHandler 持 3 个只读 HTTP 端点。relations 是派生数据——无 POST/PATCH/DELETE,
// 变更由 source domain hook 隐式触发。
type RelationHandler struct {
	svc relationdomain.Service
	log *zap.Logger
}

// NewRelationHandler constructs the handler.
//
// NewRelationHandler 构造 handler。
func NewRelationHandler(svc *relationapp.Service, log *zap.Logger) *RelationHandler {
	return &RelationHandler{svc: svc, log: log}
}

// Register wires the 3 endpoints onto mux.
func (h *RelationHandler) Register(mux Registrar) {
	mux.HandleFunc("GET /api/v1/relations", h.List)
	mux.HandleFunc("GET /api/v1/relations/neighborhood", h.Neighborhood)
	mux.HandleFunc("GET /api/v1/relgraph", h.Relgraph)
}

// List handles GET /api/v1/relations — paginated filter on any combination of
// fromKind/fromId, toKind/toId, kind.
//
// List 处理 GET /api/v1/relations —— fromKind/fromId、toKind/toId、kind 任意组合过滤、分页。
func (h *RelationHandler) List(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	filter := relationdomain.Filter{
		FromKind: q.Get("fromKind"),
		FromID:   q.Get("fromId"),
		ToKind:   q.Get("toKind"),
		ToID:     q.Get("toId"),
		Kind:     q.Get("kind"),
	}
	cursor := q.Get("cursor")
	limit := 200
	if raw := q.Get("limit"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			limit = n
		}
	}
	if limit > 500 {
		limit = 500
	}
	rows, nextCursor, hasMore, err := h.svc.List(r.Context(), filter, cursor, limit)
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Paged(w, rows, nextCursor, hasMore)
}

// Neighborhood handles GET /api/v1/relations/neighborhood?kind=&id=&depth= —
// returns all edges within `depth` hops (BFS) of the center entity.
//
// Neighborhood 处理 2-hop（或 1/3 hop）邻域查询，BFS 走图。
func (h *RelationHandler) Neighborhood(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	kind := q.Get("kind")
	id := q.Get("id")
	depth := 2
	if raw := q.Get("depth"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil {
			depth = n
		}
	}
	rows, err := h.svc.Neighborhood(r.Context(), kind, id, depth)
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, rows)
}

// Relgraph handles GET /api/v1/relgraph — full snapshot for the 洞察 tab.
//
// Relgraph 处理 GET /api/v1/relgraph —— 洞察 tab 的全图快照。
func (h *RelationHandler) Relgraph(w http.ResponseWriter, r *http.Request) {
	snap, err := h.svc.GetRelgraph(r.Context())
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, snap)
}
