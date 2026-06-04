package handlers

import (
	"net/http"
	"strconv"

	"go.uber.org/zap"

	relationapp "github.com/sunweilin/forgify/backend/internal/app/relation"
	relationdomain "github.com/sunweilin/forgify/backend/internal/domain/relation"
	responsehttpapi "github.com/sunweilin/forgify/backend/internal/transport/httpapi/response"
)

// RelationHandler serves 3 read-only endpoints over cross-entity relations. Edges
// are derived data — written implicitly by source-domain hooks — so there is no
// POST/PATCH/DELETE here. Reads return hydrated views/snapshots (names filled).
//
// RelationHandler 提供 3 个只读跨实体关系端点。边是派生数据——由 source domain hook 隐式
// 写入——故此处无 POST/PATCH/DELETE。读返回 hydrate 后的视图/快照（已填名字）。
type RelationHandler struct {
	svc relationdomain.Service
	log *zap.Logger
}

// NewRelationHandler constructs the handler.
//
// NewRelationHandler 构造 handler。
func NewRelationHandler(svc *relationapp.Service, log *zap.Logger) *RelationHandler {
	if log == nil {
		log = zap.NewNop()
	}
	return &RelationHandler{svc: svc, log: log.Named("handlers.relation")}
}

// Register wires the 3 endpoints onto mux.
//
// Register 把 3 个端点挂到 mux。
func (h *RelationHandler) Register(mux Registrar) {
	mux.HandleFunc("GET /api/v1/relations", h.List)
	mux.HandleFunc("GET /api/v1/relations/neighborhood", h.Neighborhood)
	mux.HandleFunc("GET /api/v1/relgraph", h.Relgraph)
}

// List handles GET /api/v1/relations — filter on any of fromKind/fromId,
// toKind/toId, kind; keyset-paginated via cursor/limit.
//
// List 处理 GET /api/v1/relations —— fromKind/fromId、toKind/toId、kind 任意组合过滤；
// 经 cursor/limit keyset 分页。
func (h *RelationHandler) List(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	filter := relationdomain.Filter{
		FromKind: q.Get("fromKind"),
		FromID:   q.Get("fromId"),
		ToKind:   q.Get("toKind"),
		ToID:     q.Get("toId"),
		Kind:     q.Get("kind"),
	}
	limit := 0 // 0 → store default
	if raw := q.Get("limit"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			limit = n
		}
	}
	views, next, err := h.svc.List(r.Context(), filter, q.Get("cursor"), limit)
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Paged(w, views, next, next != "")
}

// Neighborhood handles GET /api/v1/relations/neighborhood?kind=&id=&depth= —
// every edge within `depth` hops (BFS) of the center entity. depth defaults to 2.
//
// Neighborhood 处理 GET /api/v1/relations/neighborhood —— 中心实体 depth 跳内的所有边
// （BFS）。depth 默认 2。
func (h *RelationHandler) Neighborhood(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	depth := 2
	if raw := q.Get("depth"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil {
			depth = n
		}
	}
	views, err := h.svc.Neighborhood(r.Context(), q.Get("kind"), q.Get("id"), depth)
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, views)
}

// Relgraph handles GET /api/v1/relgraph — the full hydrated snapshot for the
// insight tab (nodes + edges, no pagination).
//
// Relgraph 处理 GET /api/v1/relgraph —— 洞察 tab 的完整 hydrate 快照（节点 + 边，不分页）。
func (h *RelationHandler) Relgraph(w http.ResponseWriter, r *http.Request) {
	snap, err := h.svc.GetRelgraph(r.Context())
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, snap)
}
