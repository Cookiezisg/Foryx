package handlers

import (
	"net/http"

	"go.uber.org/zap"

	schedulerapp "github.com/sunweilin/forgify/backend/internal/app/scheduler"
	triggerapp "github.com/sunweilin/forgify/backend/internal/app/trigger"
	flowrundomain "github.com/sunweilin/forgify/backend/internal/domain/flowrun"
	paginationpkg "github.com/sunweilin/forgify/backend/internal/pkg/pagination"
	responsehttpapi "github.com/sunweilin/forgify/backend/internal/transport/httpapi/response"
)

// FlowRunHandler hosts execution-plane HTTP routes.
//
// FlowRunHandler 持执行 plane HTTP 路由。
type FlowRunHandler struct {
	repo      flowrundomain.Repository
	scheduler *schedulerapp.Service
	trigger   *triggerapp.Service
	log       *zap.Logger
}

func NewFlowRunHandler(repo flowrundomain.Repository, scheduler *schedulerapp.Service, trigger *triggerapp.Service, log *zap.Logger) *FlowRunHandler {
	return &FlowRunHandler{repo: repo, scheduler: scheduler, trigger: trigger, log: log}
}

func (h *FlowRunHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/flowruns", h.List)
	mux.HandleFunc("GET /api/v1/flowruns/{id}", h.Get)
	mux.HandleFunc("GET /api/v1/flowruns/{id}/nodes", h.ListNodes)
	mux.HandleFunc("DELETE /api/v1/flowruns/{id}", h.Cancel)
	mux.HandleFunc("POST /api/v1/flowruns/{id}/approvals/{nodeId}", h.Approve)
}

// FireManual is invoked from WorkflowHandler's :trigger action dispatch.
//
// FireManual 给 WorkflowHandler 派 :trigger action 调用。
func (h *FlowRunHandler) FireManual(w http.ResponseWriter, r *http.Request, workflowID string) {
	var req struct {
		Input map[string]any `json:"input"`
	}
	if err := decodeJSON(r, &req); err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	if h.trigger == nil {
		responsehttpapi.Error(w, http.StatusServiceUnavailable, "SCHEDULER_NOT_AVAILABLE",
			"trigger service not wired", nil)
		return
	}
	runID, err := h.trigger.FireManual(r.Context(), workflowID, req.Input)
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Created(w, map[string]any{"runId": runID})
}

// TriggerStates returns per-trigger states for WorkflowHandler dispatch.
//
// TriggerStates 给 WorkflowHandler 派 GET /workflows/{id}/triggers 调。
func (h *FlowRunHandler) TriggerStates(workflowID string) []any {
	if h.trigger == nil {
		return []any{}
	}
	states := h.trigger.State(workflowID)
	out := make([]any, len(states))
	for i, s := range states {
		out[i] = s
	}
	return out
}

func (h *FlowRunHandler) List(w http.ResponseWriter, r *http.Request) {
	p, err := paginationpkg.Parse(r)
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	q := r.URL.Query()
	filter := flowrundomain.ListFilter{
		WorkflowID:  q.Get("workflowId"),
		Status:      q.Get("status"),
		TriggerKind: q.Get("triggerKind"),
		Cursor:      p.Cursor,
		Limit:       p.Limit,
	}
	rows, next, err := h.repo.List(r.Context(), filter)
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Paged(w, rows, next, next != "")
}

func (h *FlowRunHandler) Get(w http.ResponseWriter, r *http.Request) {
	run, err := h.repo.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, run)
}

func (h *FlowRunHandler) ListNodes(w http.ResponseWriter, r *http.Request) {
	p, err := paginationpkg.Parse(r)
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	q := r.URL.Query()
	filter := flowrundomain.NodeFilter{
		FlowrunID: r.PathValue("id"),
		NodeType:  q.Get("nodeType"),
		Status:    q.Get("status"),
		Cursor:    p.Cursor,
		Limit:     p.Limit,
	}
	rows, next, err := h.repo.ListNodes(r.Context(), filter)
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Paged(w, rows, next, next != "")
}

func (h *FlowRunHandler) Cancel(w http.ResponseWriter, r *http.Request) {
	if h.scheduler == nil {
		responsehttpapi.Error(w, http.StatusServiceUnavailable, "SCHEDULER_NOT_AVAILABLE",
			"scheduler not wired", nil)
		return
	}
	if err := h.scheduler.Cancel(r.Context(), r.PathValue("id")); err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.NoContent(w)
}

func (h *FlowRunHandler) Approve(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Decision string `json:"decision"`
		Reason   string `json:"reason"`
	}
	if err := decodeJSON(r, &req); err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	if h.scheduler == nil {
		responsehttpapi.Error(w, http.StatusServiceUnavailable, "SCHEDULER_NOT_AVAILABLE",
			"scheduler not wired", nil)
		return
	}
	if err := h.scheduler.ResumeApproval(r.Context(), r.PathValue("id"), r.PathValue("nodeId"), req.Decision); err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusAccepted, map[string]any{"resumed": true})
}
