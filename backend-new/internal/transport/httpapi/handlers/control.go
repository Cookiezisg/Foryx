package handlers

import (
	"net/http"
	"strconv"

	"go.uber.org/zap"

	aispawnapp "github.com/sunweilin/forgify/backend/internal/app/aispawn"
	controlapp "github.com/sunweilin/forgify/backend/internal/app/control"
	controldomain "github.com/sunweilin/forgify/backend/internal/domain/control"
	mentiondomain "github.com/sunweilin/forgify/backend/internal/domain/mention"
	schemapkg "github.com/sunweilin/forgify/backend/internal/pkg/schema"
	responsehttpapi "github.com/sunweilin/forgify/backend/internal/transport/httpapi/response"
)

// ControlHandler hosts the control-logic HTTP endpoints. The version model is linear
// with a free-moving active pointer — no pending/accept endpoints, no :run (a control
// logic is evaluated by the workflow interpreter, never invoked standalone). The :iterate
// verb (R0065) opens an AI conversation to edit this control logic via aispawn.
//
// ControlHandler 持 control 逻辑 HTTP 端点。版本模型线性 + 可自由移动的 active 指针——无
// pending/accept 端点、无 :run（control 逻辑由 workflow 解释器求值，绝不独立调用）。:iterate 动词
// （R0065）经 aispawn 开一个 AI 对话来编辑本 control 逻辑。
type ControlHandler struct {
	svc     *controlapp.Service
	aispawn *aispawnapp.Service
	log     *zap.Logger
}

// NewControlHandler constructs the handler.
//
// NewControlHandler 构造 handler。
func NewControlHandler(svc *controlapp.Service, aispawn *aispawnapp.Service, log *zap.Logger) *ControlHandler {
	if log == nil {
		log = zap.NewNop()
	}
	return &ControlHandler{svc: svc, aispawn: aispawn, log: log.Named("handlers.control")}
}

// Register wires the endpoints onto mux.
//
// Register 把端点挂到 mux。
func (h *ControlHandler) Register(mux Registrar) {
	mux.HandleFunc("POST /api/v1/controls", h.Create)
	mux.HandleFunc("GET /api/v1/controls", h.List)
	mux.HandleFunc("GET /api/v1/controls/{id}", h.Get)
	mux.HandleFunc("PATCH /api/v1/controls/{id}", h.UpdateMeta)
	mux.HandleFunc("DELETE /api/v1/controls/{id}", h.Delete)
	mux.HandleFunc("POST /api/v1/controls/{idAction}", h.postOnControl)
	mux.HandleFunc("GET /api/v1/controls/{id}/versions", h.ListVersions)
	mux.HandleFunc("GET /api/v1/controls/{id}/versions/{version}", h.GetVersion)
}

// controlBranchInput is the HTTP JSON shape of one routing branch.
//
// controlBranchInput 是一条路由分支的 HTTP JSON 形状。
type controlBranchInput struct {
	Port string            `json:"port"`
	When string            `json:"when"`
	Emit map[string]string `json:"emit"`
}

func controlBranches(in []controlBranchInput) []controldomain.Branch {
	out := make([]controldomain.Branch, len(in))
	for i, b := range in {
		out[i] = controldomain.Branch{Port: b.Port, When: b.When, Emit: b.Emit}
	}
	return out
}

func (h *ControlHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name         string               `json:"name"`
		Description  string               `json:"description"`
		Inputs       []schemapkg.Field    `json:"inputs"`
		Branches     []controlBranchInput `json:"branches"`
		ChangeReason string               `json:"changeReason"`
	}
	if err := decodeJSON(r, &req); err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	c, v, err := h.svc.Create(r.Context(), controlapp.CreateInput{
		Name:         req.Name,
		Description:  req.Description,
		Inputs:       req.Inputs,
		Branches:     controlBranches(req.Branches),
		ChangeReason: req.ChangeReason,
	})
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Created(w, map[string]any{"control": c, "version": v})
}

func (h *ControlHandler) List(w http.ResponseWriter, r *http.Request) {
	p, err := responsehttpapi.ParsePage(r)
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	items, next, err := h.svc.List(r.Context(), controldomain.ListFilter{Cursor: p.Cursor, Limit: p.Limit})
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Paged(w, items, next, next != "")
}

func (h *ControlHandler) Get(w http.ResponseWriter, r *http.Request) {
	c, err := h.svc.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, c)
}

func (h *ControlHandler) UpdateMeta(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        *string `json:"name"`
		Description *string `json:"description"`
	}
	if err := decodeJSON(r, &req); err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	c, err := h.svc.UpdateMeta(r.Context(), controlapp.UpdateMetaInput{
		ID: r.PathValue("id"), Name: req.Name, Description: req.Description,
	})
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, c)
}

func (h *ControlHandler) Delete(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.Delete(r.Context(), r.PathValue("id")); err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.NoContent(w)
}

// postOnControl dispatches POST /controls/{id}:<action> (:edit / :revert). No :run.
//
// postOnControl 派发 POST /controls/{id}:<action>（:edit / :revert）。无 :run。
func (h *ControlHandler) postOnControl(w http.ResponseWriter, r *http.Request) {
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
		iterateEntity(w, r, h.log, h.aispawn, mentiondomain.MentionControl, id)
	default:
		http.NotFound(w, r)
	}
}

func (h *ControlHandler) edit(w http.ResponseWriter, r *http.Request, id string) {
	var req struct {
		Inputs       []schemapkg.Field    `json:"inputs"`
		Branches     []controlBranchInput `json:"branches"`
		ChangeReason string               `json:"changeReason"`
	}
	if err := decodeJSON(r, &req); err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	v, err := h.svc.Edit(r.Context(), controlapp.EditInput{
		ID: id, Inputs: req.Inputs, Branches: controlBranches(req.Branches), ChangeReason: req.ChangeReason,
	})
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, v)
}

func (h *ControlHandler) revert(w http.ResponseWriter, r *http.Request, id string) {
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

func (h *ControlHandler) ListVersions(w http.ResponseWriter, r *http.Request) {
	p, err := responsehttpapi.ParsePage(r)
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	rows, next, err := h.svc.ListVersions(r.Context(), r.PathValue("id"), controldomain.VersionListFilter{Cursor: p.Cursor, Limit: p.Limit})
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Paged(w, rows, next, next != "")
}

// GetVersion accepts either an integer version number or a version id in {version}.
//
// GetVersion 的 {version} 接整数版本号或 version id。
func (h *ControlHandler) GetVersion(w http.ResponseWriter, r *http.Request) {
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
