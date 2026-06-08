package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"

	"go.uber.org/zap"

	functionapp "github.com/sunweilin/forgify/backend/internal/app/function"
	functiondomain "github.com/sunweilin/forgify/backend/internal/domain/function"
	schemapkg "github.com/sunweilin/forgify/backend/internal/pkg/schema"
	responsehttpapi "github.com/sunweilin/forgify/backend/internal/transport/httpapi/response"
)

// FunctionHandler hosts the function HTTP endpoints. The version model is linear with a
// free-moving active pointer — no pending/accept endpoints. The AI :iterate verb
// depends on askai (波次 6) and is added in that wave.
//
// FunctionHandler 持 function HTTP 端点。版本模型线性 + 可自由移动的 active 指针——无
// pending/accept 端点。AI :iterate 依赖 askai（波次 6），那轮加入。
type FunctionHandler struct {
	svc *functionapp.Service
	log *zap.Logger
}

// NewFunctionHandler constructs the handler.
//
// NewFunctionHandler 构造 handler。
func NewFunctionHandler(svc *functionapp.Service, log *zap.Logger) *FunctionHandler {
	if log == nil {
		log = zap.NewNop()
	}
	return &FunctionHandler{svc: svc, log: log.Named("handlers.function")}
}

// Register wires the endpoints onto mux.
//
// Register 把端点挂到 mux。
func (h *FunctionHandler) Register(mux Registrar) {
	mux.HandleFunc("POST /api/v1/functions", h.Create)
	mux.HandleFunc("GET /api/v1/functions", h.List)
	mux.HandleFunc("GET /api/v1/functions/{id}", h.Get)
	mux.HandleFunc("PATCH /api/v1/functions/{id}", h.UpdateMeta)
	mux.HandleFunc("DELETE /api/v1/functions/{id}", h.Delete)
	mux.HandleFunc("POST /api/v1/functions/{idAction}", h.postOnFunction)
	mux.HandleFunc("GET /api/v1/functions/{id}/versions", h.ListVersions)
	mux.HandleFunc("GET /api/v1/functions/{id}/versions/{version}", h.GetVersion)
	mux.HandleFunc("GET /api/v1/functions/{id}/executions", h.ListExecutions)
	mux.HandleFunc("GET /api/v1/function-executions/{execId}", h.GetExecution)
}

type createFunctionRequest struct {
	Name          string                         `json:"name"`
	Description   string                         `json:"description"`
	Code          string                         `json:"code"`
	Tags          []string          `json:"tags"`
	Inputs        []schemapkg.Field `json:"inputs"`
	Outputs       []schemapkg.Field `json:"outputs"`
	Dependencies  []string          `json:"dependencies"`
	PythonVersion string            `json:"pythonVersion"`
	ChangeReason  string            `json:"changeReason"`
}

func (h *FunctionHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req createFunctionRequest
	if err := decodeJSON(r, &req); err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	f, v, err := h.svc.CreateDirect(r.Context(), functionapp.DirectCreateInput{
		Name:          req.Name,
		Description:   req.Description,
		Code:          req.Code,
		Tags:          req.Tags,
		Inputs:        req.Inputs,
		Outputs:       req.Outputs,
		Dependencies:  req.Dependencies,
		PythonVersion: req.PythonVersion,
		ChangeReason:  req.ChangeReason,
	})
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Created(w, map[string]any{"function": f, "version": v})
}

func (h *FunctionHandler) List(w http.ResponseWriter, r *http.Request) {
	p, err := responsehttpapi.ParsePage(r)
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	items, next, err := h.svc.List(r.Context(), functiondomain.ListFilter{Cursor: p.Cursor, Limit: p.Limit})
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Paged(w, items, next, next != "")
}

func (h *FunctionHandler) Get(w http.ResponseWriter, r *http.Request) {
	f, err := h.svc.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, f)
}

func (h *FunctionHandler) UpdateMeta(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        *string   `json:"name"`
		Description *string   `json:"description"`
		Tags        *[]string `json:"tags"`
	}
	if err := decodeJSON(r, &req); err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	f, err := h.svc.UpdateMeta(r.Context(), functionapp.UpdateMetaInput{
		ID: r.PathValue("id"), Name: req.Name, Description: req.Description, Tags: req.Tags,
	})
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, f)
}

func (h *FunctionHandler) Delete(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.Delete(r.Context(), r.PathValue("id")); err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.NoContent(w)
}

// postOnFunction dispatches POST /functions/{id}:<action> (:run / :revert / :edit).
//
// postOnFunction 派发 POST /functions/{id}:<action>（:run / :revert / :edit）。
func (h *FunctionHandler) postOnFunction(w http.ResponseWriter, r *http.Request) {
	id, action, ok := idAndAction(r, "idAction")
	if !ok {
		http.NotFound(w, r)
		return
	}
	switch action {
	case "run":
		h.run(w, r, id)
	case "revert":
		h.revert(w, r, id)
	case "edit":
		h.edit(w, r, id)
	default:
		http.NotFound(w, r)
	}
}

func (h *FunctionHandler) run(w http.ResponseWriter, r *http.Request, id string) {
	var req struct {
		Args    map[string]any `json:"args"`
		Version int            `json:"version"`
	}
	if err := decodeJSON(r, &req); err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	versionID := ""
	if req.Version > 0 {
		v, err := h.svc.GetVersionByNumber(r.Context(), id, req.Version)
		if err != nil {
			responsehttpapi.FromDomainError(w, h.log, err)
			return
		}
		versionID = v.ID
	}
	res, err := h.svc.RunFunction(r.Context(), functionapp.RunInput{
		FunctionID:  id,
		VersionID:   versionID,
		Input:       req.Args,
		TriggeredBy: functiondomain.TriggeredByManual,
	})
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, res)
}

func (h *FunctionHandler) revert(w http.ResponseWriter, r *http.Request, id string) {
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

func (h *FunctionHandler) edit(w http.ResponseWriter, r *http.Request, id string) {
	var req struct {
		Ops          json.RawMessage `json:"ops"`
		ChangeReason string          `json:"changeReason"`
	}
	if err := decodeJSON(r, &req); err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	var ops []functionapp.Op
	if len(req.Ops) > 0 {
		parsed, err := functionapp.ParseOps(req.Ops)
		if err != nil {
			responsehttpapi.FromDomainError(w, h.log, err)
			return
		}
		ops = parsed
	}
	v, err := h.svc.Edit(r.Context(), functionapp.EditInput{ID: id, Ops: ops, ChangeReason: req.ChangeReason})
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, v)
}

func (h *FunctionHandler) ListVersions(w http.ResponseWriter, r *http.Request) {
	p, err := responsehttpapi.ParsePage(r)
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	rows, next, err := h.svc.ListVersions(r.Context(), r.PathValue("id"), functiondomain.VersionListFilter{Cursor: p.Cursor, Limit: p.Limit})
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Paged(w, rows, next, next != "")
}

// GetVersion accepts either an integer version number or a version id in {version}.
//
// GetVersion 的 {version} 接整数版本号或 version id。
func (h *FunctionHandler) GetVersion(w http.ResponseWriter, r *http.Request) {
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

func (h *FunctionHandler) ListExecutions(w http.ResponseWriter, r *http.Request) {
	p, err := responsehttpapi.ParsePage(r)
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	q := r.URL.Query()
	res, err := h.svc.SearchExecutions(r.Context(), functiondomain.ExecutionFilter{
		FunctionID:     r.PathValue("id"),
		VersionID:      q.Get("versionId"),
		Status:         q.Get("status"),
		TriggeredBy:    q.Get("triggeredBy"),
		ConversationID: q.Get("conversationId"),
		FlowrunID:      q.Get("flowrunId"),
		Cursor:         p.Cursor,
		Limit:          p.Limit,
	})
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, res)
}

func (h *FunctionHandler) GetExecution(w http.ResponseWriter, r *http.Request) {
	e, err := h.svc.GetExecution(r.Context(), r.PathValue("execId"))
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, e)
}
