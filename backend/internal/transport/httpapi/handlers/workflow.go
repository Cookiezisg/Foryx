package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"

	"go.uber.org/zap"

	askai "github.com/sunweilin/forgify/backend/internal/app/askai"
	workflowapp "github.com/sunweilin/forgify/backend/internal/app/workflow"
	workflowdomain "github.com/sunweilin/forgify/backend/internal/domain/workflow"
	paginationpkg "github.com/sunweilin/forgify/backend/internal/pkg/pagination"
	responsehttpapi "github.com/sunweilin/forgify/backend/internal/transport/httpapi/response"
)

// WorkflowHandler hosts the workflow HTTP routes.
//
// WorkflowHandler 持 workflow HTTP 路由。
type WorkflowHandler struct {
	svc     *workflowapp.Service
	flowrun *FlowRunHandler
	spawner *askai.Spawner // optional; nil disables :iterate
	log     *zap.Logger
}

func NewWorkflowHandler(svc *workflowapp.Service, log *zap.Logger) *WorkflowHandler {
	return &WorkflowHandler{svc: svc, log: log}
}

func (h *WorkflowHandler) SetSpawner(s *askai.Spawner) { h.spawner = s }

// AttachFlowRunHandler enables :trigger action + /triggers state.
//
// AttachFlowRunHandler 接入 :trigger + /triggers 路由。
func (h *WorkflowHandler) AttachFlowRunHandler(f *FlowRunHandler) {
	h.flowrun = f
}

func (h *WorkflowHandler) Register(mux Registrar) {
	mux.HandleFunc("POST /api/v1/workflows", h.Create)
	mux.HandleFunc("GET /api/v1/workflows", h.List)
	mux.HandleFunc("GET /api/v1/workflows/{id}", h.Get)
	mux.HandleFunc("PATCH /api/v1/workflows/{id}", h.UpdateMeta)
	mux.HandleFunc("DELETE /api/v1/workflows/{id}", h.Delete)
	mux.HandleFunc("POST /api/v1/workflows/{idAction}", h.postOnWorkflow)
	mux.HandleFunc("GET /api/v1/workflows/{id}/triggers", h.GetTriggers)
	mux.HandleFunc("GET /api/v1/workflows/{id}/versions", h.ListVersions)
	mux.HandleFunc("GET /api/v1/workflows/{id}/versions/{version}", h.GetVersion)
	mux.HandleFunc("GET /api/v1/workflows/{id}/pending", h.GetPending)
	mux.HandleFunc("POST /api/v1/workflows/{id}/pending:accept", h.AcceptPending)
	mux.HandleFunc("POST /api/v1/workflows/{id}/pending:reject", h.RejectPending)
}

// Create applies ops to an empty graph and persists workflow + auto-accepted v1.
//
// Create 把 ops 应用到空图,持久化 workflow + 自动 accept v1。
func (h *WorkflowHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Ops          json.RawMessage `json:"ops"`
		ChangeReason string          `json:"changeReason"`
	}
	if err := decodeJSON(r, &req); err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	ops, err := workflowapp.ParseOps(req.Ops)
	if err != nil {
		responsehttpapi.Error(w, http.StatusBadRequest, "WORKFLOW_OP_INVALID", err.Error(), nil)
		return
	}
	wf, v, err := h.svc.Create(r.Context(), workflowapp.CreateInput{
		Ops:          ops,
		ChangeReason: req.ChangeReason,
	})
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Created(w, map[string]any{"workflow": wf, "version": v})
}

func (h *WorkflowHandler) List(w http.ResponseWriter, r *http.Request) {
	p, err := paginationpkg.Parse(r)
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	enabledOnly := r.URL.Query().Get("enabled") == "true"
	items, next, err := h.svc.List(r.Context(), workflowdomain.ListFilter{
		Cursor:      p.Cursor,
		Limit:       p.Limit,
		EnabledOnly: enabledOnly,
	})
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Paged(w, items, next, next != "")
}

func (h *WorkflowHandler) Get(w http.ResponseWriter, r *http.Request) {
	wf, err := h.svc.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, wf)
}

func (h *WorkflowHandler) UpdateMeta(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name            *string   `json:"name"`
		Description     *string   `json:"description"`
		Tags            *[]string `json:"tags"`
		Enabled         *bool     `json:"enabled"`
		Concurrency     *string   `json:"concurrency"`
		NeedsAttention  *bool     `json:"needsAttention"`
		AttentionReason *string   `json:"attentionReason"`
	}
	if err := decodeJSON(r, &req); err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	wf, err := h.svc.UpdateMeta(r.Context(), workflowapp.UpdateMetaInput{
		ID:              r.PathValue("id"),
		Name:            req.Name,
		Description:     req.Description,
		Tags:            req.Tags,
		Enabled:         req.Enabled,
		Concurrency:     req.Concurrency,
		NeedsAttention:  req.NeedsAttention,
		AttentionReason: req.AttentionReason,
	})
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, wf)
}

func (h *WorkflowHandler) Delete(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.Delete(r.Context(), r.PathValue("id")); err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.NoContent(w)
}

// postOnWorkflow dispatches POST /api/v1/workflows/{id}:<action> (:revert/:trigger).
//
// postOnWorkflow 派发 :revert / :trigger;后者委派 FlowRunHandler。
func (h *WorkflowHandler) postOnWorkflow(w http.ResponseWriter, r *http.Request) {
	id, action, ok := idAndAction(r, "idAction")
	if !ok {
		http.NotFound(w, r)
		return
	}
	switch action {
	case "revert":
		h.Revert(w, r, id)
	case "edit":
		h.Edit(w, r, id)
	case "trigger":
		if h.flowrun == nil {
			responsehttpapi.Error(w, http.StatusServiceUnavailable, "SCHEDULER_NOT_AVAILABLE",
				"Plan 05 execution plane not wired", nil)
			return
		}
		h.flowrun.FireManual(w, r, id)
	case "capability-check":
		h.CapabilityCheck(w, r, id)
	case "iterate":
		h.Iterate(w, r, id)
	default:
		http.NotFound(w, r)
	}
}

// Iterate — see FunctionHandler.Iterate for semantics.
//
// Iterate —— 语义见 FunctionHandler.Iterate。
func (h *WorkflowHandler) Iterate(w http.ResponseWriter, r *http.Request, id string) {
	if h.spawner == nil {
		responsehttpapi.Error(w, http.StatusServiceUnavailable, "ASKAI_NOT_AVAILABLE",
			"askai spawner not wired", nil)
		return
	}
	var req struct {
		Prompt string `json:"prompt"`
	}
	if err := decodeJSON(r, &req); err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	sysPrompt, err := askai.BuildWorkflowContext(r.Context(), id, h.svc)
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	result, err := h.spawner.Spawn(r.Context(), askai.SpawnInput{
		SystemPrompt: sysPrompt,
		UserPrompt:   req.Prompt,
	})
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, result)
}

// CapabilityCheck runs pre-flight validation on the workflow's active version.
// Returns a report with ok=bool + issues list; HTTP 200 in both pass and fail
// cases — failure mode is body content not status.
//
// CapabilityCheck 跑 workflow active version 预检。
// 返 {ok, issues} 报告；通过 / 不通过都 HTTP 200，body 内容区分。
func (h *WorkflowHandler) CapabilityCheck(w http.ResponseWriter, r *http.Request, id string) {
	report, err := h.svc.CapabilityCheck(r.Context(), id)
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, report)
}

// GetTriggers returns per-trigger State; empty list when flowrun unwired.
//
// GetTriggers 返每个 trigger 状态;未接 flowrun 返空列表。
func (h *WorkflowHandler) GetTriggers(w http.ResponseWriter, r *http.Request) {
	if h.flowrun == nil {
		responsehttpapi.Success(w, http.StatusOK, []any{})
		return
	}
	responsehttpapi.Success(w, http.StatusOK, h.flowrun.TriggerStates(r.PathValue("id")))
}

func (h *WorkflowHandler) Revert(w http.ResponseWriter, r *http.Request, id string) {
	var req struct {
		TargetVersion int `json:"targetVersion"`
	}
	if err := decodeJSON(r, &req); err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	v, err := h.svc.Revert(r.Context(), id, req.TargetVersion)
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, v)
}

// Edit applies ops to a workflow, producing or iterating a pending version.
// Mirrors edit_workflow LLM tool semantics; consumer = future testend editor.
//
// Edit 给 workflow 应用 ops 产/迭代 pending 版本。镜像 edit_workflow 工具语义；
// HTTP 消费者是未来 testend 编辑器。
func (h *WorkflowHandler) Edit(w http.ResponseWriter, r *http.Request, id string) {
	var req struct {
		Ops          json.RawMessage `json:"ops"`
		ChangeReason string          `json:"changeReason"`
	}
	if err := decodeJSON(r, &req); err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	ops, err := workflowapp.ParseOps(req.Ops)
	if err != nil {
		responsehttpapi.Error(w, http.StatusBadRequest, "WORKFLOW_OP_INVALID", err.Error(), nil)
		return
	}
	v, err := h.svc.Edit(r.Context(), workflowapp.EditInput{
		ID:           id,
		Ops:          ops,
		ChangeReason: req.ChangeReason,
	})
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, v)
}

func (h *WorkflowHandler) ListVersions(w http.ResponseWriter, r *http.Request) {
	p, err := paginationpkg.Parse(r)
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	rows, next, err := h.svc.ListVersions(r.Context(), r.PathValue("id"), workflowdomain.VersionListFilter{
		Cursor: p.Cursor,
		Limit:  p.Limit,
		Status: r.URL.Query().Get("status"),
	})
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Paged(w, rows, next, next != "")
}

// GetVersion accepts an integer version number or a wfv_xxx version ID.
//
// GetVersion 兼容数字版本号或 wfv_xxx 版本 ID。
func (h *WorkflowHandler) GetVersion(w http.ResponseWriter, r *http.Request) {
	versionStr := r.PathValue("version")
	versionN, err := strconv.Atoi(versionStr)
	if err != nil {
		v, gerr := h.svc.GetVersion(r.Context(), versionStr)
		if gerr != nil {
			responsehttpapi.FromDomainError(w, h.log, gerr)
			return
		}
		responsehttpapi.Success(w, http.StatusOK, v)
		return
	}
	v, err := h.svc.GetVersionByNumber(r.Context(), r.PathValue("id"), versionN)
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, v)
}

func (h *WorkflowHandler) GetPending(w http.ResponseWriter, r *http.Request) {
	v, err := h.svc.GetPending(r.Context(), r.PathValue("id"))
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, v)
}

func (h *WorkflowHandler) AcceptPending(w http.ResponseWriter, r *http.Request) {
	v, err := h.svc.AcceptPending(r.Context(), r.PathValue("id"))
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, v)
}

func (h *WorkflowHandler) RejectPending(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.RejectPending(r.Context(), r.PathValue("id")); err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.NoContent(w)
}
