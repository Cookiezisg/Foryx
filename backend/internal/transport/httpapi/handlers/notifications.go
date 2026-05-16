package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"go.uber.org/zap"

	notificationsdomain "github.com/sunweilin/forgify/backend/internal/domain/notifications"
	responsehttpapi "github.com/sunweilin/forgify/backend/internal/transport/httpapi/response"
)

// NotificationsHandler exposes /api/v1/notifications global SSE stream.
//
// NotificationsHandler 把 /api/v1/notifications 暴露为全局 SSE 流。
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

func (h *NotificationsHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/notifications", h.Stream)
}

func (h *NotificationsHandler) Stream(w http.ResponseWriter, r *http.Request) {
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
