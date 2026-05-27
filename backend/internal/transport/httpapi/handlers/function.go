package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"

	"go.uber.org/zap"

	askai "github.com/sunweilin/forgify/backend/internal/app/askai"
	functionapp "github.com/sunweilin/forgify/backend/internal/app/function"
	functiondomain "github.com/sunweilin/forgify/backend/internal/domain/function"
	paginationpkg "github.com/sunweilin/forgify/backend/internal/pkg/pagination"
	responsehttpapi "github.com/sunweilin/forgify/backend/internal/transport/httpapi/response"
)

// FunctionHandler hosts the function HTTP routes.
//
// FunctionHandler 持 function HTTP 路由。
type FunctionHandler struct {
	svc     *functionapp.Service
	spawner *askai.Spawner // optional; nil disables :iterate
	log     *zap.Logger
}

func NewFunctionHandler(svc *functionapp.Service, log *zap.Logger) *FunctionHandler {
	return &FunctionHandler{svc: svc, log: log}
}

// SetSpawner installs the askai Spawner post-construction; nil disables :iterate.
//
// SetSpawner 装配后注入 askai Spawner；nil 关闭 :iterate。
func (h *FunctionHandler) SetSpawner(s *askai.Spawner) { h.spawner = s }

func (h *FunctionHandler) Register(mux Registrar) {
	mux.HandleFunc("POST /api/v1/functions", h.Create)
	mux.HandleFunc("GET /api/v1/functions", h.List)
	mux.HandleFunc("GET /api/v1/functions/{id}", h.Get)
	mux.HandleFunc("PATCH /api/v1/functions/{id}", h.UpdateMeta)
	mux.HandleFunc("DELETE /api/v1/functions/{id}", h.Delete)
	mux.HandleFunc("POST /api/v1/functions/{idAction}", h.postOnFunction)
	mux.HandleFunc("GET /api/v1/functions/{id}/versions", h.ListVersions)
	mux.HandleFunc("GET /api/v1/functions/{id}/versions/{version}", h.GetVersion)
	mux.HandleFunc("GET /api/v1/functions/{id}/pending", h.GetPending)
	mux.HandleFunc("POST /api/v1/functions/{id}/pending:accept", h.AcceptPending)
	mux.HandleFunc("POST /api/v1/functions/{id}/pending:reject", h.RejectPending)
	mux.HandleFunc("GET /api/v1/functions/{id}/executions", h.ListExecutions)
	mux.HandleFunc("GET /api/v1/function-executions/{execId}", h.GetExecution)
}

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

// postOnFunction dispatches POST /api/v1/functions/{id}:<action> (:run / :revert / :edit).
//
// postOnFunction 派发 POST /api/v1/functions/{id}:<action>(:run / :revert / :edit)。
func (h *FunctionHandler) postOnFunction(w http.ResponseWriter, r *http.Request) {
	id, action, ok := idAndAction(r, "idAction")
	if !ok {
		http.NotFound(w, r)
		return
	}
	switch action {
	case "run":
		h.Run(w, r, id)
	case "revert":
		h.Revert(w, r, id)
	case "edit":
		h.Edit(w, r, id)
	case "iterate":
		h.Iterate(w, r, id)
	default:
		http.NotFound(w, r)
	}
}

// Iterate spawns an AI-driven editing conversation for this function: builds
// a system prompt with current state + user's request, returns conversationId
// for frontend to subscribe to eventlog + forge stream.
//
// Iterate 起一个 AI 编辑对话：拼当前状态 + 用户请求做 system prompt，返
// conversationId 让前端订阅 eventlog + forge stream 看 AI 推理 + pending 落地。
func (h *FunctionHandler) Iterate(w http.ResponseWriter, r *http.Request, id string) {
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
	sysPrompt, err := askai.BuildFunctionContext(r.Context(), id, h.svc)
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

// Edit applies ops to a function, producing or iterating a pending version.
// Mirrors edit_function LLM tool semantics; no forge SSE emit (HTTP path —
// future testend editor UI is the consumer, not chat).
//
// Edit 给 function 应用 ops 产/迭代 pending 版本。镜像 edit_function 工具语义；
// HTTP 路径不发 forge SSE（消费者是未来 testend 编辑器 UI,不是 chat 流）。
func (h *FunctionHandler) Edit(w http.ResponseWriter, r *http.Request, id string) {
	var req struct {
		Ops          json.RawMessage `json:"ops"`
		ChangeReason string          `json:"changeReason"`
	}
	if err := decodeJSON(r, &req); err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	ops, err := functionapp.ParseOps(req.Ops)
	if err != nil {
		responsehttpapi.Error(w, http.StatusBadRequest, "FUNCTION_OP_INVALID", err.Error(), nil)
		return
	}
	v, err := h.svc.Edit(r.Context(), functionapp.EditInput{
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
