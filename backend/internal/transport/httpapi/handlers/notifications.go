package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"go.uber.org/zap"

	notificationsdomain "github.com/sunweilin/forgify/backend/internal/domain/notifications"
	responsehttpapi "github.com/sunweilin/forgify/backend/internal/transport/httpapi/response"
)

// NotificationsHandler exposes /api/v1/notifications as SSE stream or REST snapshot.
//
// NotificationsHandler 将 /api/v1/notifications 暴露为 SSE 流或 REST 快照。
type NotificationsHandler struct {
	bridge notificationsdomain.Bridge
	log    *zap.Logger
}

func NewNotificationsHandler(bridge notificationsdomain.Bridge, log *zap.Logger) *NotificationsHandler {
	if log == nil {
		log = zap.NewNop()
	}
	return &NotificationsHandler{bridge: bridge, log: log.Named("notifications.handler")}
}

func (h *NotificationsHandler) Register(mux Registrar) {
	mux.HandleFunc("GET /api/v1/notifications", h.Handle)
}

// Handle routes to SSE stream or REST list based on Accept header.
//
// Handle 按 Accept header 分发 SSE 流或 REST 快照列表。
func (h *NotificationsHandler) Handle(w http.ResponseWriter, r *http.Request) {
	if strings.Contains(r.Header.Get("Accept"), "text/event-stream") {
		h.stream(w, r)
		return
	}
	h.list(w, r)
}

func (h *NotificationsHandler) stream(w http.ResponseWriter, r *http.Request) {
	var fromSeq int64
	if v := r.Header.Get("Last-Event-ID"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
			fromSeq = n
		}
	}

	ch, cancelSub, err := h.bridge.Subscribe(r.Context(), fromSeq)
	if err != nil {
		if errors.Is(err, notificationsdomain.ErrSeqTooOld) {
			responsehttpapi.Error(w, http.StatusGone, "SEQ_TOO_OLD",
				"requested Last-Event-ID has been evicted from the replay buffer; resubscribe without it",
				nil)
			return
		}
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	defer cancelSub()

	responsehttpapi.StreamSSE(w, r, nil, ch,
		func(out io.Writer, env notificationsdomain.Envelope) error {
			data, err := json.Marshal(env.Event)
			if err != nil {
				h.log.Warn("SSE marshal failed",
					zap.String("type", env.Event.Type),
					zap.Int64("seq", env.Seq),
					zap.Error(err))
				return err
			}
			_, err = fmt.Fprintf(out, "event: notification\nid: %d\ndata: %s\n\n",
				env.Seq, data)
			return err
		},
	)
}

// notifListItem is the JSON shape for each item in the REST snapshot list.
type notifListItem struct {
	Seq            int64  `json:"seq"`
	Type           string `json:"type"`
	ID             string `json:"id"`
	Data           any    `json:"data"`
	ConversationID string `json:"conversationId,omitempty"`
}

func (h *NotificationsHandler) list(w http.ResponseWriter, r *http.Request) {
	var fromSeq int64
	if v := r.URL.Query().Get("cursor"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
			fromSeq = n
		}
	}
	limit := 50
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	if limit > 200 {
		limit = 200
	}

	envelopes, hasMore, err := h.bridge.List(r.Context(), fromSeq, limit)
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}

	items := make([]notifListItem, len(envelopes))
	var lastSeq int64
	for i, env := range envelopes {
		items[i] = notifListItem{
			Seq:            env.Seq,
			Type:           env.Event.Type,
			ID:             env.Event.ID,
			Data:           env.Event.Data,
			ConversationID: env.Event.ConversationID,
		}
		lastSeq = env.Seq
	}

	var nextCursor string
	if hasMore {
		nextCursor = strconv.FormatInt(lastSeq, 10)
	}
	responsehttpapi.Paged(w, items, nextCursor, hasMore)
}
