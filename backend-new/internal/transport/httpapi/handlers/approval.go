package handlers

import (
	"net/http"
	"strconv"

	"go.uber.org/zap"

	aispawnapp "github.com/sunweilin/forgify/backend/internal/app/aispawn"
	approvalapp "github.com/sunweilin/forgify/backend/internal/app/approval"
	approvaldomain "github.com/sunweilin/forgify/backend/internal/domain/approval"
	mentiondomain "github.com/sunweilin/forgify/backend/internal/domain/mention"
	schemapkg "github.com/sunweilin/forgify/backend/internal/pkg/schema"
	responsehttpapi "github.com/sunweilin/forgify/backend/internal/transport/httpapi/response"
)

// ApprovalHandler hosts the approval-form HTTP endpoints. Linear version model with a
// free-moving active pointer — no pending/accept endpoints, no :run (an approval form is
// rendered + parked by the workflow interpreter, never invoked standalone). The :iterate
// verb (R0065) opens an AI conversation to edit this approval form via aispawn.
//
// ApprovalHandler 持审批表 HTTP 端点。线性版本 + 自由 active 指针——无 pending/accept 端点、无 :run
// （审批表由 workflow 解释器渲染 + park，绝不独立调用）。:iterate 动词（R0065）经 aispawn 开一个 AI 对话来编辑本审批表。
type ApprovalHandler struct {
	svc     *approvalapp.Service
	aispawn *aispawnapp.Service
	log     *zap.Logger
}

// NewApprovalHandler constructs the handler.
//
// NewApprovalHandler 构造 handler。
func NewApprovalHandler(svc *approvalapp.Service, aispawn *aispawnapp.Service, log *zap.Logger) *ApprovalHandler {
	if log == nil {
		log = zap.NewNop()
	}
	return &ApprovalHandler{svc: svc, aispawn: aispawn, log: log.Named("handlers.approval")}
}

// Register wires the endpoints onto mux.
//
// Register 把端点挂到 mux。
func (h *ApprovalHandler) Register(mux Registrar) {
	mux.HandleFunc("POST /api/v1/approvals", h.Create)
	mux.HandleFunc("GET /api/v1/approvals", h.List)
	mux.HandleFunc("GET /api/v1/approvals/{id}", h.Get)
	mux.HandleFunc("PATCH /api/v1/approvals/{id}", h.UpdateMeta)
	mux.HandleFunc("DELETE /api/v1/approvals/{id}", h.Delete)
	mux.HandleFunc("POST /api/v1/approvals/{idAction}", h.postOnApproval)
	mux.HandleFunc("GET /api/v1/approvals/{id}/versions", h.ListVersions)
	mux.HandleFunc("GET /api/v1/approvals/{id}/versions/{version}", h.GetVersion)
}

func (h *ApprovalHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name            string            `json:"name"`
		Description     string            `json:"description"`
		Inputs          []schemapkg.Field `json:"inputs"`
		Template        string            `json:"template"`
		AllowReason     bool              `json:"allowReason"`
		Timeout         string            `json:"timeout"`
		TimeoutBehavior string            `json:"timeoutBehavior"`
		ChangeReason    string            `json:"changeReason"`
	}
	if err := decodeJSON(r, &req); err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	f, v, err := h.svc.Create(r.Context(), approvalapp.CreateInput{
		Name:            req.Name,
		Description:     req.Description,
		Inputs:          req.Inputs,
		Template:        req.Template,
		AllowReason:     req.AllowReason,
		Timeout:         req.Timeout,
		TimeoutBehavior: req.TimeoutBehavior,
		ChangeReason:    req.ChangeReason,
	})
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Created(w, map[string]any{"approval": f, "version": v})
}

func (h *ApprovalHandler) List(w http.ResponseWriter, r *http.Request) {
	p, err := responsehttpapi.ParsePage(r)
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	items, next, err := h.svc.List(r.Context(), approvaldomain.ListFilter{Cursor: p.Cursor, Limit: p.Limit})
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Paged(w, items, next, next != "")
}

func (h *ApprovalHandler) Get(w http.ResponseWriter, r *http.Request) {
	f, err := h.svc.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, f)
}

func (h *ApprovalHandler) UpdateMeta(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        *string `json:"name"`
		Description *string `json:"description"`
	}
	if err := decodeJSON(r, &req); err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	f, err := h.svc.UpdateMeta(r.Context(), approvalapp.UpdateMetaInput{
		ID: r.PathValue("id"), Name: req.Name, Description: req.Description,
	})
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, f)
}

func (h *ApprovalHandler) Delete(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.Delete(r.Context(), r.PathValue("id")); err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.NoContent(w)
}

// postOnApproval dispatches POST /approvals/{id}:<action> (:edit / :revert). No :run.
//
// postOnApproval 派发 POST /approvals/{id}:<action>（:edit / :revert）。无 :run。
func (h *ApprovalHandler) postOnApproval(w http.ResponseWriter, r *http.Request) {
	id, action, ok := idAndAction(r, "idAction")
	if !ok {
		http.NotFound(w, r)
		return
	}
	switch action {
	case "edit":
		h.edit(w, r, id)
	case "revert":
		h.revert(w, r, id)
	case "iterate":
		iterateEntity(w, r, h.log, h.aispawn, mentiondomain.MentionApproval, id)
	default:
		http.NotFound(w, r)
	}
}

func (h *ApprovalHandler) edit(w http.ResponseWriter, r *http.Request, id string) {
	var req struct {
		Inputs          []schemapkg.Field `json:"inputs"`
		Template        string            `json:"template"`
		AllowReason     bool              `json:"allowReason"`
		Timeout         string            `json:"timeout"`
		TimeoutBehavior string            `json:"timeoutBehavior"`
		ChangeReason    string            `json:"changeReason"`
	}
	if err := decodeJSON(r, &req); err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	v, err := h.svc.Edit(r.Context(), approvalapp.EditInput{
		ID: id, Inputs: req.Inputs, Template: req.Template, AllowReason: req.AllowReason,
		Timeout: req.Timeout, TimeoutBehavior: req.TimeoutBehavior, ChangeReason: req.ChangeReason,
	})
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, v)
}

func (h *ApprovalHandler) revert(w http.ResponseWriter, r *http.Request, id string) {
	var req struct {
		Version int `json:"version"`
	}
	if err := decodeJSON(r, &req); err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	v, err := h.svc.Revert(r.Context(), id, req.Version)
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, v)
}

func (h *ApprovalHandler) ListVersions(w http.ResponseWriter, r *http.Request) {
	p, err := responsehttpapi.ParsePage(r)
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	rows, next, err := h.svc.ListVersions(r.Context(), r.PathValue("id"), approvaldomain.VersionListFilter{Cursor: p.Cursor, Limit: p.Limit})
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Paged(w, rows, next, next != "")
}

// GetVersion accepts either an integer version number or a version id in {version}.
//
// GetVersion 的 {version} 接整数版本号或 version id。
func (h *ApprovalHandler) GetVersion(w http.ResponseWriter, r *http.Request) {
	versionStr := r.PathValue("version")
	if n, err := strconv.Atoi(versionStr); err == nil {
		v, gerr := h.svc.GetVersionByNumber(r.Context(), r.PathValue("id"), n)
		if gerr != nil {
			responsehttpapi.FromDomainError(w, h.log, gerr)
			return
		}
		responsehttpapi.Success(w, http.StatusOK, v)
		return
	}
	v, err := h.svc.GetVersion(r.Context(), versionStr)
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, v)
}
