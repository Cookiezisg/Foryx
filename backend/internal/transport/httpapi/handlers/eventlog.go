// eventlog.go — SSE handler for the recursive event-log protocol.
// Phase 1 ships this alongside the legacy ChatHandler.EventsSSE
// endpoint (/api/v1/events) so frontends can migrate at their own
// pace; Phase 4 cutover deletes the legacy path.
//
// Wire format per event:
//
//	event: <type>          ← message_start | block_delta | ...
//	id: <seq>              ← per-conversation monotonic
//	data: <JSON of event>  ← payload struct as-is (no type/seq dup)
//
// Reconnect: client sends `Last-Event-ID: N` header; server replays
// buffered envelopes with seq > N, then live. Past the buffer's oldest
// entry the server returns 410 Gone — client must HTTP-fetch full
// state and resubscribe with the new tail seq.
//
// eventlog.go ——递归事件日志协议的 SSE handler。Phase 1 与 legacy
// ChatHandler.EventsSSE（/api/v1/events）共存，让前端按自己节奏迁移；
// Phase 4 cutover 删 legacy。
//
// 每条 wire 格式：
//
//	event: <type>          ← message_start | block_delta | ...
//	id: <seq>              ← per-conversation 单调
//	data: <JSON of event>  ← payload struct 原样（不重复 type/seq）
//
// 重连：客户端发 `Last-Event-ID: N` header；服务端 replay 缓存中 seq > N
// 的 envelope，再接实时。超过 buffer 最旧时返 410 Gone——客户端必须
// HTTP fetch 全态后用新 tail seq 重订阅。
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

// EventLogHandler exposes /api/v1/eventlog as an SSE stream backed by
// the recursive-event-log Bridge plus a /api/v1/conversations/{id}/eventlog
// HTTP endpoint that reconstructs events from DB for refetch-after-410.
//
// EventLogHandler 把 /api/v1/eventlog 暴露为递归事件日志 Bridge 支撑的
// SSE 流，加 /api/v1/conversations/{id}/eventlog HTTP 端点从 DB 重构
// 事件给 410 后的 refetch。
type EventLogHandler struct {
	bridge eventlogdomain.Bridge
	repo   chatdomain.Repository // optional; nil disables the refetch endpoint
	log    *zap.Logger
}

// NewEventLogHandler wires the handler dependencies. repo is optional —
// pass nil to disable the HTTP refetch endpoint (only the SSE stream
// will be served).
//
// NewEventLogHandler 装配 handler 依赖。repo 可选——传 nil 禁用 HTTP
// refetch 端点（只提供 SSE 流）。
func NewEventLogHandler(bridge eventlogdomain.Bridge, repo chatdomain.Repository, log *zap.Logger) *EventLogHandler {
	if log == nil {
		log = zap.NewNop()
	}
	return &EventLogHandler{bridge: bridge, repo: repo, log: log.Named("eventlog.handler")}
}

// Register attaches the SSE route + the HTTP refetch route.
//
// Register 挂 SSE 路由 + HTTP refetch 路由。
func (h *EventLogHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/eventlog", h.Stream)
	if h.repo != nil {
		mux.HandleFunc("GET /api/v1/conversations/{id}/eventlog", h.History)
	}
}

// Stream serves GET /api/v1/eventlog. The bridge keys by user_id from
// ctx (D-redo-2) — no query parameter. Clients receive every event for
// every conversation this user owns and demux on payload.conversationId.
//
// Stream 服务 GET /api/v1/eventlog。Bridge 按 ctx 中 user_id 订阅
// (D-redo-2),无 query 参;客户端按 payload.conversationId 分派 panel。
func (h *EventLogHandler) Stream(w http.ResponseWriter, r *http.Request) {
	// Last-Event-ID is the standard SSE reconnect header. Parse to
	// int64; absent / invalid → 0 (no replay, live only).
	//
	// Last-Event-ID 是标准 SSE 重连 header。解析为 int64;缺失/非法
	// → 0(无 replay 直接实时)。
	var fromSeq int64
	if v := r.Header.Get("Last-Event-ID"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
			fromSeq = n
		}
	}

	ch, cancelSub, err := h.bridge.Subscribe(r.Context(), fromSeq)
	if err != nil {
		if errors.Is(err, eventlogdomain.ErrSeqTooOld) {
			// 410 Gone signals "buffer evicted; refetch full state".
			// 410 Gone 表示"buffer 已淘汰；refetch 全态"。
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

// History serves GET /api/v1/conversations/{id}/eventlog?from=<seq>.
// Returns a JSON envelope of replayed block events from DB. Clients
// that received 410 Gone from the SSE replay buffer call this to
// refetch full state, then resubscribe with Last-Event-ID = the new
// tail seq from this response.
//
// History 服务 GET /api/v1/conversations/{id}/eventlog?from=<seq>。
// 从 DB 返 JSON 包装的回放 block 事件。客户端收到 SSE replay buffer 的
// 410 Gone 时调用本端点 refetch 全态，然后用响应中的 tail seq 作
// Last-Event-ID 重订阅。
func (h *EventLogHandler) History(w http.ResponseWriter, r *http.Request) {
	// Go 1.22 mux {id} pattern rejects empty path segments at the router
	// layer — an empty id can never reach this handler. Validation here
	// would be dead code (§6 反校验剧场).
	// Go 1.22 mux {id} 在路由层就拒空段——handler 永不会收到空 id，
	// 此处校验属死分支（§6 反校验剧场）。
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

// _ marker to keep chatdomain import live when repo is nil at compile.
var _ chatdomain.Repository = (chatdomain.Repository)(nil)
