// Package handlers — HTTP delivery of user answers to AskUserQuestion.
//
// Package handlers — 把用户答案投递给 AskUserQuestion 工具。
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

func NewAnswerHandler(svc *askapp.Service, log *zap.Logger) *AnswerHandler {
	return &AnswerHandler{svc: svc, log: log}
}

func (h *AnswerHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/conversations/{id}/answers", h.Submit)
}

type answerRequest struct {
	ToolCallID string `json:"toolCallId"`
	Answer     string `json:"answer"`
	Skipped    bool   `json:"skipped,omitempty"`
}

// skippedAnswer is the sentinel fed to the agent when the user skips.
//
// skippedAnswer 用户 skip 时塞给 agent 的哨兵字符串。
const skippedAnswer = "(user skipped)"

// Submit returns 204; 400 if toolCallId missing; 404 if no pending question.
//
// Submit 返 204;缺 toolCallId 返 400;无 pending 返 404。
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
