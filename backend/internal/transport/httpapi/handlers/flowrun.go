package handlers

import (
	"net/http"

	"go.uber.org/zap"

	askai "github.com/sunweilin/forgify/backend/internal/app/askai"
	schedulerapp "github.com/sunweilin/forgify/backend/internal/app/scheduler"
	triggerapp "github.com/sunweilin/forgify/backend/internal/app/trigger"
	workflowapp "github.com/sunweilin/forgify/backend/internal/app/workflow"
	flowrundomain "github.com/sunweilin/forgify/backend/internal/domain/flowrun"
	paginationpkg "github.com/sunweilin/forgify/backend/internal/pkg/pagination"
	responsehttpapi "github.com/sunweilin/forgify/backend/internal/transport/httpapi/response"
)

// FlowRunHandler hosts execution-plane HTTP routes.
//
// FlowRunHandler 持执行 plane HTTP 路由。
type FlowRunHandler struct {
	repo        flowrundomain.Repository
	scheduler   *schedulerapp.Service
	trigger     *triggerapp.Service
	spawner     *askai.Spawner       // V1.2 §17 — :triage action; nil disables
	workflowSvc *workflowapp.Service // for triage context (workflow graph lookup)
	log         *zap.Logger
}

func NewFlowRunHandler(repo flowrundomain.Repository, scheduler *schedulerapp.Service, trigger *triggerapp.Service, log *zap.Logger) *FlowRunHandler {
	return &FlowRunHandler{repo: repo, scheduler: scheduler, trigger: trigger, log: log}
}

// SetAskAI wires the askai Spawner + workflow service post-construction.
//
// SetAskAI 装配后注入 askai Spawner + workflow service。
func (h *FlowRunHandler) SetAskAI(s *askai.Spawner, wf *workflowapp.Service) {
	h.spawner = s
	h.workflowSvc = wf
}

func (h *FlowRunHandler) Register(mux Registrar) {
	mux.HandleFunc("GET /api/v1/flowruns", h.List)
	mux.HandleFunc("GET /api/v1/flowruns/{id}", h.Get)
	mux.HandleFunc("GET /api/v1/flowruns/{id}/nodes", h.ListNodes)
	mux.HandleFunc("DELETE /api/v1/flowruns/{id}", h.Cancel)
	mux.HandleFunc("POST /api/v1/flowruns/{id}/approvals/{nodeId}", h.Approve)
	mux.HandleFunc("POST /api/v1/flowruns/{idAction}", h.postOnFlowRun)
}

// postOnFlowRun dispatches POST /api/v1/flowruns/{id}:<action> (:triage).
//
// postOnFlowRun 派发 POST /api/v1/flowruns/{id}:<action>(:triage)。
func (h *FlowRunHandler) postOnFlowRun(w http.ResponseWriter, r *http.Request) {
	id, action, ok := idAndAction(r, "idAction")
	if !ok {
		http.NotFound(w, r)
		return
	}
	switch action {
	case "triage":
		h.Triage(w, r, id)
	default:
		http.NotFound(w, r)
	}
}

// Triage spawns an AI-driven debugging conversation for this flowrun: builds
// a system prompt with run state + workflow graph + (optional) user hint,
// returns conversationId for frontend to subscribe to eventlog + forge stream.
//
// Triage 起 AI 调试对话：拼 run 状态 + workflow graph + 可选 user hint 做
// system prompt，返 conversationId 让前端订阅流。
func (h *FlowRunHandler) Triage(w http.ResponseWriter, r *http.Request, id string) {
	if h.spawner == nil {
		responsehttpapi.Error(w, http.StatusServiceUnavailable, "ASKAI_NOT_AVAILABLE",
			"askai spawner not wired", nil)
		return
	}
	var req struct {
		Prompt string `json:"prompt,omitempty"`
	}
	// Body is optional — Triage is meaningful even with no user hint.
	_ = decodeJSON(r, &req)
	sysPrompt, err := askai.BuildTriageContext(r.Context(), id, h.repo, h.workflowSvc)
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	userPrompt := req.Prompt
	if userPrompt == "" {
		userPrompt = "Please analyze this flowrun and tell me what went wrong and how to fix it."
	}
	result, err := h.spawner.Spawn(r.Context(), askai.SpawnInput{
		SystemPrompt: sysPrompt,
		UserPrompt:   userPrompt,
	})
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, result)
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
	// ?dryRun=true bypasses trigger.FireManual and starts a preview run via scheduler directly.
	// ?dryRun=true 跳 trigger.FireManual，直接经 scheduler 起预览 run。
	dryRun := r.URL.Query().Get("dryRun") == "true" || r.URL.Query().Get("dryRun") == "1"
	if dryRun {
		if h.scheduler == nil {
			responsehttpapi.Error(w, http.StatusServiceUnavailable, "SCHEDULER_NOT_AVAILABLE",
				"scheduler not wired", nil)
			return
		}
		runID, err := h.scheduler.StartRunWithOptions(r.Context(), workflowID,
			"manual", req.Input, schedulerapp.StartRunOptions{DryRun: true})
		if err != nil {
			responsehttpapi.FromDomainError(w, h.log, err)
			return
		}
		responsehttpapi.Created(w, map[string]any{"runId": runID, "dryRun": true})
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
	if err := h.scheduler.ResumeApproval(r.Context(), r.PathValue("id"), r.PathValue("nodeId"), req.Decision, req.Reason); err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusAccepted, map[string]any{"resumed": true})
}
