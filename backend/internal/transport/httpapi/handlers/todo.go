package handlers

import (
	"net/http"

	"go.uber.org/zap"

	todoapp "github.com/sunweilin/forgify/backend/internal/app/todo"
	tododomain "github.com/sunweilin/forgify/backend/internal/domain/todo"
	responsehttpapi "github.com/sunweilin/forgify/backend/internal/transport/httpapi/response"
)

// TodoHandler exposes the read-only task-board endpoint. Writes are LLM-only (the
// TodoWrite tool, 波次 2/3) — the frontend never edits the agent's plan, it observes it:
// fetch the current list on conversation open, then live-update from the messages stream's
// "todo" signal.
//
// TodoHandler 暴露只读任务看板端点。写入是 LLM 专属（TodoWrite 工具，波次 2/3）——前端从不
// 编辑 agent 的计划，只观察它：对话打开时拉当前清单，之后由 messages 流的 "todo" signal 实时更新。
type TodoHandler struct {
	svc *todoapp.Service
	log *zap.Logger
}

// NewTodoHandler constructs the handler.
//
// NewTodoHandler 构造 handler。
func NewTodoHandler(svc *todoapp.Service, log *zap.Logger) *TodoHandler {
	if log == nil {
		log = zap.NewNop()
	}
	return &TodoHandler{svc: svc, log: log.Named("handlers.todo")}
}

// Register wires the read endpoint.
//
// Register 挂只读端点。
func (h *TodoHandler) Register(mux Registrar) {
	mux.HandleFunc("GET /api/v1/conversations/{conversationId}/todos", h.List) // 路径占位 camelCase(N3)
}

// List returns the checklist for a conversation, or for a subagent run within it when
// ?subagentId= is given. An empty array (not null) when the scope has no todos yet.
//
// List 返某对话的清单，给了 ?subagentId= 则返其中某 subagent run 的。作用域尚无 todo 时返空数组（非 null）。
func (h *TodoHandler) List(w http.ResponseWriter, r *http.Request) {
	conv := r.PathValue("conversationId")
	var sub *string
	if sid := r.URL.Query().Get("subagentId"); sid != "" {
		sub = &sid
	}
	items, err := h.svc.GetForScope(r.Context(), conv, sub)
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	if items == nil {
		items = []tododomain.Item{}
	}
	responsehttpapi.Success(w, http.StatusOK, map[string]any{
		"conversationId": conv,
		"subagentId":     sub,
		"todos":          items,
	})
}
