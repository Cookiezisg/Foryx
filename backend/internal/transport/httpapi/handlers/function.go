// function.go — HTTP handlers for the function domain (forge_redesign Plan 01
// Phase 5). 13 endpoints per spec/02-function.md §8:
//
//   POST   /api/v1/functions                          create (direct definition)
//   GET    /api/v1/functions                          list (paginated)
//   GET    /api/v1/functions/{id}                     detail (with pending + env)
//   PATCH  /api/v1/functions/{id}                     update metadata
//   DELETE /api/v1/functions/{id}                     soft-delete
//   POST   /api/v1/functions/{idAction}               :run / :resync / :revert
//   GET    /api/v1/functions/{id}/versions            list versions
//   GET    /api/v1/functions/{id}/versions/{v}        version detail
//   GET    /api/v1/functions/{id}/pending             fetch pending
//   POST   /api/v1/functions/{id}/pending:accept      accept pending
//   POST   /api/v1/functions/{id}/pending:reject      reject pending
//
// LLM-driven authoring (ops streams) goes through the LLM tool path
// (create_function / edit_function), NOT POST /functions. The HTTP shape is
// the direct definition for curl / UI / scripts.
//
// function.go —— function domain 的 HTTP handler(forge_redesign Plan 01
// Phase 5)。13 端点 per 02-function.md §8。LLM 走 ops 流(create_function /
// edit_function 工具);HTTP 走扁平 definition 给 curl/UI/script。

package handlers

import (
	"net/http"
	"strconv"

	"go.uber.org/zap"

	functionapp "github.com/sunweilin/forgify/backend/internal/app/function"
	functiondomain "github.com/sunweilin/forgify/backend/internal/domain/function"
	paginationpkg "github.com/sunweilin/forgify/backend/internal/pkg/pagination"
	responsehttpapi "github.com/sunweilin/forgify/backend/internal/transport/httpapi/response"
)

// FunctionHandler hosts the function HTTP routes.
//
// FunctionHandler 持 function HTTP 路由。
type FunctionHandler struct {
	svc *functionapp.Service
	log *zap.Logger
}

// NewFunctionHandler wires handler dependencies.
//
// NewFunctionHandler 装配 handler 依赖。
func NewFunctionHandler(svc *functionapp.Service, log *zap.Logger) *FunctionHandler {
	return &FunctionHandler{svc: svc, log: log}
}

// Register mounts every function route on mux.
//
// Register 把所有 function 路由挂到 mux。
func (h *FunctionHandler) Register(mux *http.ServeMux) {
	// Collection
	mux.HandleFunc("POST /api/v1/functions", h.Create)
	mux.HandleFunc("GET /api/v1/functions", h.List)

	// Resource
	mux.HandleFunc("GET /api/v1/functions/{id}", h.Get)
	mux.HandleFunc("PATCH /api/v1/functions/{id}", h.UpdateMeta)
	mux.HandleFunc("DELETE /api/v1/functions/{id}", h.Delete)

	// Resource actions (:run / :resync / :revert)
	mux.HandleFunc("POST /api/v1/functions/{idAction}", h.postOnFunction)

	// Versions
	mux.HandleFunc("GET /api/v1/functions/{id}/versions", h.ListVersions)
	mux.HandleFunc("GET /api/v1/functions/{id}/versions/{version}", h.GetVersion)

	// Pending
	mux.HandleFunc("GET /api/v1/functions/{id}/pending", h.GetPending)
	mux.HandleFunc("POST /api/v1/functions/{id}/pending:accept", h.AcceptPending)
	mux.HandleFunc("POST /api/v1/functions/{id}/pending:reject", h.RejectPending)

	// Execution log (D22)
	mux.HandleFunc("GET /api/v1/functions/{id}/executions", h.ListExecutions)
	mux.HandleFunc("GET /api/v1/function-executions/{execId}", h.GetExecution)
}

// ── CRUD ──────────────────────────────────────────────────────────────────────

func (h *FunctionHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name          string                          `json:"name"`
		Description   string                          `json:"description"`
		Code          string                          `json:"code"`
		Tags          []string                        `json:"tags"`
		Parameters    []functiondomain.ParameterSpec  `json:"parameters"`
		ReturnSchema  map[string]any                  `json:"returnSchema"`
		Dependencies  []string                        `json:"dependencies"`
		PythonVersion string                          `json:"pythonVersion"`
		ChangeReason  string                          `json:"changeReason"`
	}
	if err := decodeJSON(r, &req); err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	f, v, err := h.svc.CreateDirect(r.Context(), functionapp.DirectCreateInput{
		Name:          req.Name,
		Description:   req.Description,
		Code:          req.Code,
		Tags:          req.Tags,
		Parameters:    req.Parameters,
		ReturnSchema:  req.ReturnSchema,
		Dependencies:  req.Dependencies,
		PythonVersion: req.PythonVersion,
		ChangeReason:  req.ChangeReason,
	})
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	// Kick off env sync in background.
	h.svc.SyncEnvForVersion(r.Context(), v.ID)
	responsehttpapi.Created(w, map[string]any{"function": f, "version": v})
}

func (h *FunctionHandler) List(w http.ResponseWriter, r *http.Request) {
	p, err := paginationpkg.Parse(r)
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
		ID:          r.PathValue("id"),
		Name:        req.Name,
		Description: req.Description,
		Tags:        req.Tags,
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

// postOnFunction dispatches POST /api/v1/functions/{id}:<action>. Currently
// supports :run, :resync, :revert.
//
// postOnFunction 派发 POST /api/v1/functions/{id}:<action>。支持 :run / :resync /
// :revert。
func (h *FunctionHandler) postOnFunction(w http.ResponseWriter, r *http.Request) {
	id, action, ok := idAndAction(r, "idAction")
	if !ok {
		http.NotFound(w, r)
		return
	}
	switch action {
	case "run":
		h.Run(w, r, id)
	case "resync":
		h.Resync(w, r, id)
	case "revert":
		h.Revert(w, r, id)
	default:
		http.NotFound(w, r)
	}
}

func (h *FunctionHandler) Run(w http.ResponseWriter, r *http.Request, id string) {
	var req struct {
		Args    map[string]any `json:"args"`
		Version string         `json:"version"`
	}
	if err := decodeJSON(r, &req); err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	res, err := h.svc.RunFunction(r.Context(), functionapp.RunInput{
		FunctionID: id,
		VersionID:  req.Version,
		Input:      req.Args,
		TriggeredBy: functiondomain.TriggeredByHTTP,
	})
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, res)
}

func (h *FunctionHandler) Resync(w http.ResponseWriter, r *http.Request, id string) {
	if err := h.svc.Resync(r.Context(), id); err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusAccepted, map[string]any{"resyncing": true})
}

func (h *FunctionHandler) Revert(w http.ResponseWriter, r *http.Request, id string) {
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

// ── Versions ──────────────────────────────────────────────────────────────────

func (h *FunctionHandler) ListVersions(w http.ResponseWriter, r *http.Request) {
	p, err := paginationpkg.Parse(r)
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	rows, next, err := h.svc.ListVersions(r.Context(), r.PathValue("id"), functiondomain.VersionListFilter{
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

func (h *FunctionHandler) GetVersion(w http.ResponseWriter, r *http.Request) {
	versionStr := r.PathValue("version")
	versionN, err := strconv.Atoi(versionStr)
	if err != nil {
		// Treat as version ID (fnv_xxx) if not an integer.
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

// ── Pending ───────────────────────────────────────────────────────────────────

func (h *FunctionHandler) GetPending(w http.ResponseWriter, r *http.Request) {
	v, err := h.svc.GetPending(r.Context(), r.PathValue("id"))
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, v)
}

func (h *FunctionHandler) AcceptPending(w http.ResponseWriter, r *http.Request) {
	v, err := h.svc.AcceptPending(r.Context(), r.PathValue("id"))
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, v)
}

func (h *FunctionHandler) RejectPending(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.RejectPending(r.Context(), r.PathValue("id")); err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.NoContent(w)
}

// ── Execution log (D22) ───────────────────────────────────────────────────────

func (h *FunctionHandler) ListExecutions(w http.ResponseWriter, r *http.Request) {
	p, err := paginationpkg.Parse(r)
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	q := r.URL.Query()
	filter := functiondomain.ExecutionFilter{
		FunctionID:     r.PathValue("id"),
		VersionID:      q.Get("versionId"),
		Status:         q.Get("status"),
		ConversationID: q.Get("conversationId"),
		FlowrunID:      q.Get("flowrunId"),
		Cursor:         p.Cursor,
		Limit:          p.Limit,
	}
	res, err := h.svc.SearchExecutions(r.Context(), filter)
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, res)
}

func (h *FunctionHandler) GetExecution(w http.ResponseWriter, r *http.Request) {
	detail, err := h.svc.GetExecutionDetail(r.Context(), r.PathValue("execId"))
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, detail)
}
