// answers.go — HTTP handler for delivering user answers to questions
// the AskUserQuestion tool is currently waiting on. The route lives
// under the conversation namespace because answers are conceptually a
// conversation-scoped resource (one tool_call_id per conversation
// turn), but the actual rendezvous keying is by tool_call_id.
//
// Decision D11: no separate event family for asking — the question
// itself rides chat.message SSE; this handler only closes the loop
// from user answer back to the blocked tool.
//
// answers.go — 把用户答案投递给正在等的 AskUserQuestion 工具的 HTTP
// handler。路由放在 conversation 命名空间下（答案概念上 conv-scoped），
// 但实际会合按 tool_call_id 索引。
//
// 决策 D11：问题不新建事件家族——问题本身坐 chat.message SSE；本 handler
// 只负责把用户答案闭合回阻塞的工具。
package handlers

import (
	"net/http"

	"go.uber.org/zap"

	askapp "github.com/sunweilin/forgify/backend/internal/app/ask"
	responsehttpapi "github.com/sunweilin/forgify/backend/internal/transport/httpapi/response"
)

// AnswerHandler serves POST /api/v1/conversations/{id}/answers.
//
// AnswerHandler 提供 POST /api/v1/conversations/{id}/answers。
type AnswerHandler struct {
	svc *askapp.Service
	log *zap.Logger
}

// NewAnswerHandler wires the handler dependencies.
//
// NewAnswerHandler 装配 handler 依赖。
func NewAnswerHandler(svc *askapp.Service, log *zap.Logger) *AnswerHandler {
	return &AnswerHandler{svc: svc, log: log}
}

// Register attaches the answer route.
//
// Register 挂载路由。
func (h *AnswerHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/conversations/{id}/answers", h.Submit)
}

// answerRequest is the POST body shape. ConversationID is taken from
// the URL path; the body carries the tool_call_id the answer is for,
// the answer text, and an optional `skipped` flag (2026-05 #6: user
// explicitly chose to skip — backend substitutes a sentinel answer so
// the agent can decide its own default behaviour).
//
// answerRequest POST 体。conversationID 从 URL;body 带 toolCallId + answer
// + skipped 标志(2026-05 #6:用户显式 skip;backend 用哨兵答案让 agent 自决)。
type answerRequest struct {
	ToolCallID string `json:"toolCallId"`
	Answer     string `json:"answer"`
	Skipped    bool   `json:"skipped,omitempty"`
}

// skippedAnswer is the sentinel string fed to the agent when the user
// clicks "skip" on the frontend. The agent's tool_result will see this
// literal text and decide a reasonable default. Documented in the
// AskUserQuestion tool's Description so the LLM knows to interpret it.
//
// skippedAnswer 用户 skip 时塞给 agent 的哨兵字符串。Tool description 已
// 说明,LLM 看到这串就走"用合理 default 继续"。
const skippedAnswer = "(user skipped)"

// Submit: POST /api/v1/conversations/{id}/answers → 204.
// Body: {"toolCallId": "...", "answer": "..."} or {"toolCallId": "...", "skipped": true}.
//
// Errors:
//   - 400 INVALID_REQUEST — body missing toolCallId
//   - 404 ASK_NO_PENDING_QUESTION — toolCallId not waiting (or already answered)
//
// Submit:POST /api/v1/conversations/{id}/answers → 204。
//
// 错误:400 缺字段;404 toolCallId 无 pending(或已答)。
func (h *AnswerHandler) Submit(w http.ResponseWriter, r *http.Request) {
	var req answerRequest
	if err := decodeJSON(r, &req); err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	if req.ToolCallID == "" {
		responsehttpapi.Error(w, http.StatusBadRequest, "INVALID_REQUEST",
			"toolCallId is required", nil)
		return
	}
	// If skipped: substitute sentinel so the agent sees a meaningful tool_result
	// (instead of an empty answer, which is indistinguishable from "user typed nothing").
	// skipped: 用哨兵替空 answer,让 agent 知道是"主动 skip"而非"空答"。
	answer := req.Answer
	if req.Skipped {
		answer = skippedAnswer
	}
	if err := h.svc.Resolve(req.ToolCallID, answer); err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.NoContent(w)
}
