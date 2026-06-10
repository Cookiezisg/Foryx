package handlers

import (
	"net/http"
	"strconv"

	"go.uber.org/zap"

	notificationapp "github.com/sunweilin/forgify/backend/internal/app/notification"
	responsehttpapi "github.com/sunweilin/forgify/backend/internal/transport/httpapi/response"
)

// NotificationHandler serves the notification center's REST surface (list / unread-count /
// mark-read / mark-all-read) backed by the DB. The live notifications SSE subscription is served
// by StreamHandler alongside the other two streams (one place for all three, E1).
//
// NotificationHandler 提供通知中心的 REST 面（list / unread-count / mark-read / mark-all-read）走 DB。
// 实时 notifications SSE 订阅由 StreamHandler 与另两条流统一提供（三流一处，E1）。
type NotificationHandler struct {
	svc *notificationapp.Service
	log *zap.Logger
}

// NewNotificationHandler constructs the handler.
//
// NewNotificationHandler 构造 handler。
func NewNotificationHandler(svc *notificationapp.Service, log *zap.Logger) *NotificationHandler {
	if log == nil {
		log = zap.NewNop()
	}
	return &NotificationHandler{svc: svc, log: log.Named("handlers.notification")}
}

// Register wires the REST endpoints onto mux (the SSE stream is StreamHandler's).
//
// Register 把 REST 端点挂到 mux（SSE 流归 StreamHandler）。
func (h *NotificationHandler) Register(mux Registrar) {
	mux.HandleFunc("GET /api/v1/notifications", h.List)
	mux.HandleFunc("GET /api/v1/notifications/unread-count", h.UnreadCount)
	mux.HandleFunc("PUT /api/v1/notifications/{id}/read", h.MarkRead)
	mux.HandleFunc("POST /api/v1/notifications/read-all", h.MarkAllRead)
}

// List handles GET /api/v1/notifications — newest-first, keyset-paginated.
//
// List 处理 GET /api/v1/notifications —— 最新优先、keyset 分页。
func (h *NotificationHandler) List(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit := 0
	if raw := q.Get("limit"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			limit = n
		}
	}
	items, next, err := h.svc.List(r.Context(), q.Get("cursor"), limit)
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Paged(w, items, next, next != "")
}

// UnreadCount handles GET /api/v1/notifications/unread-count — the badge number.
//
// UnreadCount 处理 GET /api/v1/notifications/unread-count —— badge 数。
func (h *NotificationHandler) UnreadCount(w http.ResponseWriter, r *http.Request) {
	n, err := h.svc.CountUnread(r.Context())
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, map[string]int{"unread": n})
}

// MarkRead handles PUT /api/v1/notifications/{id}/read.
//
// MarkRead 处理 PUT /api/v1/notifications/{id}/read。
func (h *NotificationHandler) MarkRead(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.MarkRead(r.Context(), r.PathValue("id")); err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.NoContent(w)
}

// MarkAllRead handles POST /api/v1/notifications/read-all.
//
// MarkAllRead 处理 POST /api/v1/notifications/read-all。
func (h *NotificationHandler) MarkAllRead(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.MarkAllRead(r.Context()); err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.NoContent(w)
}
