// subagent.go — HTTP handler for /api/v1/subagent-* + the per-conversation
// /api/v1/conversations/{id}/subagent-runs sub-route. Thin: decode →
// service → envelope. The Subagent system tool is what the LLM uses to
// SPAWN runs; these endpoints are the OBSERVABILITY surface (UI lists,
// transcript replay, type catalog).
//
// Endpoints (per subagent.md §11):
//
//	GET /api/v1/conversations/{id}/subagent-runs   list runs of one conv (UI history)
//	GET /api/v1/subagent-runs/{id}                 single run detail
//	GET /api/v1/subagent-runs/{id}/messages        run transcript (replay)
//	GET /api/v1/subagent-types                     built-in registry catalog
//
// V1 omits POST :cancel — main-chat Cancel cascades to active sub-runs
// via parent ctx (see Service.Spawn ctx wiring). A future explicit
// per-run cancel endpoint is straightforward but unneeded for V1 demo.
//
// subagent.go ——/api/v1/subagent-* + per-conversation /api/v1/conversations/
// {id}/subagent-runs 子路由 HTTP handler。薄层：decode → service → envelope。
// LLM 用 Subagent 系统工具 SPAWN run；这些端点是 UI 观测面（列表、回放、
// 类型目录）。V1 不出 POST :cancel——主对话 cancel 经父 ctx 级联到活跃
// sub-run；future 可加显式 per-run cancel，V1 demo 不需要。
package handlers

import (
	"errors"
	"net/http"

	"go.uber.org/zap"
	"gorm.io/gorm"

	subagentapp "github.com/sunweilin/forgify/backend/internal/app/subagent"
	subagentdomain "github.com/sunweilin/forgify/backend/internal/domain/subagent"
	responsehttpapi "github.com/sunweilin/forgify/backend/internal/transport/httpapi/response"
)

// SubagentHandler serves the /api/v1/subagent-* + per-conversation routes.
//
// SubagentHandler 提供 /api/v1/subagent-* + per-conversation 路由。
type SubagentHandler struct {
	svc *subagentapp.Service
	log *zap.Logger
}

// NewSubagentHandler wires the handler dependencies.
//
// NewSubagentHandler 装配 handler 依赖。
func NewSubagentHandler(svc *subagentapp.Service, log *zap.Logger) *SubagentHandler {
	return &SubagentHandler{svc: svc, log: log}
}

// Register attaches the four GET routes. No :action endpoints — V1 is
// observability-only.
//
// Register 挂四个 GET 路由。无 :action 端点——V1 仅观测。
func (h *SubagentHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/conversations/{id}/subagent-runs", h.ListRunsByConversation)
	mux.HandleFunc("GET /api/v1/subagent-runs/{id}", h.GetRun)
	mux.HandleFunc("GET /api/v1/subagent-runs/{id}/messages", h.ListMessages)
	mux.HandleFunc("GET /api/v1/subagent-types", h.ListTypes)
}

// ── Endpoints ────────────────────────────────────────────────────────

// ListRunsByConversation: GET /api/v1/conversations/{id}/subagent-runs
//
// Returns runs for one conversation, newest-first (Service order).
//
// ListRunsByConversation: 返某对话的 run 列表，最新优先（Service 排序）。
func (h *SubagentHandler) ListRunsByConversation(w http.ResponseWriter, r *http.Request) {
	convID := r.PathValue("id")
	rows, err := h.svc.ListByConversation(r.Context(), convID)
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, rows)
}

// GetRun: GET /api/v1/subagent-runs/{id}
//
// Returns the single run with its terminal status / token totals. The
// transient lastTool* fields are zero on a fetched (post-restart) row —
// they only ride live SSE frames during the run itself.
//
// GetRun: 返单 run 含终态 status / token 累计。瞬时 lastTool* 字段在拉取
// （重启后）行上为零——仅 run 本身活跃时才坐 SSE 帧。
func (h *SubagentHandler) GetRun(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	row, err := h.svc.Get(r.Context(), id)
	if err != nil {
		// gorm.ErrRecordNotFound surfaces as a 500 by default through
		// FromDomainError because no errmap row matches it. Map to 404
		// here so the frontend shows the right status.
		// gorm.ErrRecordNotFound 默认走 FromDomainError 落 500（errmap 无对应行）；
		// 在此映射成 404 让前端拿到正确状态。
		if errors.Is(err, gorm.ErrRecordNotFound) {
			responsehttpapi.Error(w, http.StatusNotFound, "SUBAGENT_RUN_NOT_FOUND",
				"subagent run not found", nil)
			return
		}
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, row)
}

// ListMessages: GET /api/v1/subagent-runs/{id}/messages
//
// Returns the run's full message transcript ordered by Seq — used by the
// SubagentRun-detail UI to replay what the sub-runner did. Empty result
// is a 200 with [] (run exists but produced no messages, e.g. cancelled
// before LoadHistory completed).
//
// ListMessages: 返 run 全部消息按 Seq——SubagentRun 详情 UI 回放 sub-runner
// 做了什么。空结果走 200 []（run 存在但未产消息，例如 LoadHistory 完成前
// 被 cancel）。
func (h *SubagentHandler) ListMessages(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	rows, err := h.svc.ListMessages(r.Context(), id)
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, rows)
}

// ListTypes: GET /api/v1/subagent-types
//
// Returns the built-in SubagentType registry (V1: Explore + Plan +
// general-purpose) in stable alphabetic order so the UI can render a
// deterministic dropdown.
//
// ListTypes: 返内置 SubagentType 注册表（V1：Explore + Plan + general-purpose）
// 按字母序，UI 渲染确定性下拉。
func (h *SubagentHandler) ListTypes(w http.ResponseWriter, r *http.Request) {
	types := h.svc.ListTypes()
	responsehttpapi.Success(w, http.StatusOK, types)
}

// Compile-time keep-alive — silences unused-import lint when the file
// trims down (subagentdomain is referenced via Service return types
// transitively; this nudge keeps the import explicit for future direct use).
//
// 编译期保活——文件 trim 时静默 unused-import lint（subagentdomain 经 Service
// 返值类型间接被用；显式保 import 给未来直接用）。
var _ = subagentdomain.StatusRunning
