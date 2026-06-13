package handlers

import (
	"net/http"

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
	// 非 CRUD 状态变更用 :action(N5/MD5):实体级 {id}:mark-read、集合级 :mark-all-read。
	mux.HandleFunc("POST /api/v1/notifications/{idAction}", h.postOnNotification)
	mux.HandleFunc("POST /api/v1/notifications:mark-all-read", h.MarkAllRead)
}

// postOnNotification dispatches the single entity-level action POST /notifications/{id}:mark-read.
//
// postOnNotification 派发唯一的实体级动作 POST /notifications/{id}:mark-read。
func (h *NotificationHandler) postOnNotification(w http.ResponseWriter, r *http.Request) {
	id, action, ok := idAndAction(r, "idAction")
	if !ok || action != "mark-read" {
		http.NotFound(w, r)
		return
	}
	h.markRead(w, r, id)
}

// List handles GET /api/v1/notifications — newest-first, keyset-paginated.
//
// List 处理 GET /api/v1/notifications —— 最新优先、keyset 分页。
func (h *NotificationHandler) List(w http.ResponseWriter, r *http.Request) {
	p, err := responsehttpapi.ParsePage(r)
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	items, next, err := h.svc.List(r.Context(), p.Cursor, p.Limit)
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

// markRead marks one notification read (POST /notifications/{id}:mark-read).
//
// markRead 把一条通知标已读（POST /notifications/{id}:mark-read）。
func (h *NotificationHandler) markRead(w http.ResponseWriter, r *http.Request, id string) {
	if err := h.svc.MarkRead(r.Context(), id); err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.NoContent(w)
}

// MarkAllRead handles POST /api/v1/notifications:mark-all-read.
//
// MarkAllRead 处理 POST /api/v1/notifications:mark-all-read。
func (h *NotificationHandler) MarkAllRead(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.MarkAllRead(r.Context()); err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.NoContent(w)
}
