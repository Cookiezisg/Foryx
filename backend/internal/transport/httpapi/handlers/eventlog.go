package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"go.uber.org/zap"

	chatdomain "github.com/sunweilin/forgify/backend/internal/domain/chat"
	eventlogdomain "github.com/sunweilin/forgify/backend/internal/domain/eventlog"
	responsehttpapi "github.com/sunweilin/forgify/backend/internal/transport/httpapi/response"
)

// EventLogHandler exposes /api/v1/eventlog SSE + per-conversation refetch.
//
// EventLogHandler 暴露 /api/v1/eventlog SSE + per-conversation 重取端点。
type EventLogHandler struct {
	bridge eventlogdomain.Bridge
	repo   chatdomain.Repository
	log    *zap.Logger
}

func NewEventLogHandler(bridge eventlogdomain.Bridge, repo chatdomain.Repository, log *zap.Logger) *EventLogHandler {
	if log == nil {
		log = zap.NewNop()
	}
	return &EventLogHandler{bridge: bridge, repo: repo, log: log.Named("eventlog.handler")}
}

func (h *EventLogHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/eventlog", h.Stream)
	if h.repo != nil {
		mux.HandleFunc("GET /api/v1/conversations/{id}/eventlog", h.History)
	}
}

// Stream serves GET /api/v1/eventlog; subscribed per-user via ctx user_id.
//
// Stream 服务 GET /api/v1/eventlog;按 ctx user_id 订阅。
func (h *EventLogHandler) Stream(w http.ResponseWriter, r *http.Request) {
	var fromSeq int64
	if v := r.Header.Get("Last-Event-ID"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
			fromSeq = n
		}
	}

	ch, cancelSub, err := h.bridge.Subscribe(r.Context(), fromSeq)
	if err != nil {
		if errors.Is(err, eventlogdomain.ErrSeqTooOld) {
			responsehttpapi.Error(w, http.StatusGone, "SEQ_TOO_OLD",
				"requested Last-Event-ID has been evicted from the replay buffer; refetch full state",
				nil)
			return
		}
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	defer cancelSub()

	responsehttpapi.StreamSSE(w, r, nil, ch,
		func(out io.Writer, env eventlogdomain.Envelope) error {
			data, err := json.Marshal(env.Event)
			if err != nil {
				h.log.Warn("SSE marshal failed",
					zap.String("type", env.Event.EventType()),
					zap.Int64("seq", env.Seq),
					zap.Error(err))
				return err
			}
			_, err = fmt.Fprintf(out, "event: %s\nid: %d\ndata: %s\n\n",
				env.Event.EventType(), env.Seq, data)
			return err
		},
	)
}

// History returns replayed events from DB for clients hit by 410 Gone.
//
// History 从 DB 返回回放事件,供 410 Gone 后 refetch。
func (h *EventLogHandler) History(w http.ResponseWriter, r *http.Request) {
	conversationID := r.PathValue("id")
	var fromSeq int64
	if v := r.URL.Query().Get("from"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n >= 0 {
			fromSeq = n
		}
	}

	envelopes, err := h.repo.ReplayEventsAfter(r.Context(), conversationID, fromSeq)
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	tailSeq := fromSeq
	if n := len(envelopes); n > 0 {
		tailSeq = envelopes[n-1].Seq
	}
	responsehttpapi.Success(w, http.StatusOK, map[string]any{
		"events":  envelopes,
		"tailSeq": tailSeq,
		"count":   len(envelopes),
	})
}

var _ chatdomain.Repository = (chatdomain.Repository)(nil)
