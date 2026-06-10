package handlers

import (
	"net/http"
	"strconv"

	"go.uber.org/zap"

	streamdomain "github.com/sunweilin/forgify/backend/internal/domain/stream"
	responsehttpapi "github.com/sunweilin/forgify/backend/internal/transport/httpapi/response"
)

// StreamHandler is the one place that serves all three (and only three, E1) SSE subscriptions the
// frontend keeps open for the whole session: messages (chat block lifecycle — assistant
// text/reasoning + tool_call + tool_result), entities (forge pipeline progress), notifications
// (durable inbox). Every stream is workspace-scoped and UNFILTERED: the backend always streams the
// complete delta feed for the workspace; the client filters by conversation/entity itself. The
// plumbing (resume cursor → Bridge.Subscribe → StreamSSE → 410 on ErrSeqTooOld) is identical for
// all three. (Notification REST — list/read — stays in NotificationHandler.)
//
// StreamHandler 是统一提供全部三条（且仅三条，E1）SSE 订阅的唯一处：messages（chat block 生命周期——
// assistant 文本/reasoning + tool_call + tool_result）、entities（锻造流水线进度）、notifications
// （持久收件箱）。每条 workspace 级且**不过滤**：后端始终发该 workspace 的完整 delta 流，客户端自滤。
// 三条管道（续传游标 → Bridge.Subscribe → StreamSSE → ErrSeqTooOld 转 410）完全一致。
type StreamHandler struct {
	messages      streamdomain.Bridge
	entities      streamdomain.Bridge
	notifications streamdomain.Bridge
	log           *zap.Logger
}

// NewStreamHandler wires the three buses as the three SSE subscriptions.
//
// NewStreamHandler 把三条总线装成三条 SSE 订阅。
func NewStreamHandler(messages, entities, notifications streamdomain.Bridge, log *zap.Logger) *StreamHandler {
	if log == nil {
		log = zap.NewNop()
	}
	return &StreamHandler{messages: messages, entities: entities, notifications: notifications, log: log.Named("handlers.stream")}
}

func (h *StreamHandler) Register(mux Registrar) {
	mux.HandleFunc("GET /api/v1/messages/stream", func(w http.ResponseWriter, r *http.Request) { h.subscribe(w, r, h.messages) })
	mux.HandleFunc("GET /api/v1/entities/stream", func(w http.ResponseWriter, r *http.Request) { h.subscribe(w, r, h.entities) })
	mux.HandleFunc("GET /api/v1/notifications/stream", func(w http.ResponseWriter, r *http.Request) { h.subscribe(w, r, h.notifications) })
}

// subscribe is the shared SSE plumbing: resume from the cursor, stream every frame verbatim (no
// scope filter — the workspace is the only partition; the client filters), surface ErrSeqTooOld
// as 410 so the client refetches history and reconnects.
//
// subscribe 是共享 SSE 管道：从游标续传，逐字流出每帧（不过滤 scope——workspace 是唯一分区、客户端
// 自滤），ErrSeqTooOld → 410 让客户端重取历史后重连。
func (h *StreamHandler) subscribe(w http.ResponseWriter, r *http.Request, bridge streamdomain.Bridge) {
	ch, cancel, err := bridge.Subscribe(r.Context(), decodeFromSeq(r))
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	defer cancel()
	responsehttpapi.StreamSSE(w, r, nil, ch, responsehttpapi.WriteStreamEnvelope)
}

// decodeFromSeq reads the SSE resume cursor: Last-Event-ID (the browser re-sends it on reconnect),
// else ?fromSeq. Absent/invalid → 0 (live-only, no replay).
//
// decodeFromSeq 读 SSE 续传游标：Last-Event-ID（浏览器重连自动带），否则 ?fromSeq。缺/坏 → 0（仅实时）。
func decodeFromSeq(r *http.Request) int64 {
	v := r.Header.Get("Last-Event-ID")
	if v == "" {
		v = r.URL.Query().Get("fromSeq")
	}
	n, _ := strconv.ParseInt(v, 10, 64)
	return n
}
