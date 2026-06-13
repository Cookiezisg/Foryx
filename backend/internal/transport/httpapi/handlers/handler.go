package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"

	"go.uber.org/zap"

	aispawnapp "github.com/sunweilin/forgify/backend/internal/app/aispawn"
	handlerapp "github.com/sunweilin/forgify/backend/internal/app/handler"
	handlerdomain "github.com/sunweilin/forgify/backend/internal/domain/handler"
	mentiondomain "github.com/sunweilin/forgify/backend/internal/domain/mention"
	responsehttpapi "github.com/sunweilin/forgify/backend/internal/transport/httpapi/response"
)

// HandlerHandler hosts the handler HTTP endpoints. The version model is linear with a
// free-moving active pointer (no pending/accept). The resident instance is MCP-style
// (boot / restart / shutdown); :restart resets it. :iterate (R0065) opens an AI conversation
// to edit this handler via aispawn.
//
// HandlerHandler 持 handler HTTP 端点。版本模型线性 + 自由 active 指针（无 pending/accept）。
// 常驻实例 MCP 式（boot / restart / shutdown）；:restart 重置它。:iterate（R0065）经 aispawn 开 AI 对话编辑本 handler。
type HandlerHandler struct {
	svc     *handlerapp.Service
	aispawn *aispawnapp.Service
	log     *zap.Logger
}

func NewHandlerHandler(svc *handlerapp.Service, aispawn *aispawnapp.Service, log *zap.Logger) *HandlerHandler {
	if log == nil {
		log = zap.NewNop()
	}
	return &HandlerHandler{svc: svc, aispawn: aispawn, log: log.Named("handlers.handler")}
}

func (h *HandlerHandler) Register(mux Registrar) {
	mux.HandleFunc("POST /api/v1/handlers", h.Create)
	mux.HandleFunc("GET /api/v1/handlers", h.List)
	mux.HandleFunc("GET /api/v1/handlers/{id}", h.Get)
	mux.HandleFunc("PATCH /api/v1/handlers/{id}", h.UpdateMeta)
	mux.HandleFunc("DELETE /api/v1/handlers/{id}", h.Delete)
	mux.HandleFunc("POST /api/v1/handlers/{idAction}", h.postOnHandler)
	mux.HandleFunc("GET /api/v1/handlers/{id}/versions", h.ListVersions)
	mux.HandleFunc("GET /api/v1/handlers/{id}/versions/{version}", h.GetVersion)
	mux.HandleFunc("GET /api/v1/handlers/{id}/config", h.GetConfig)
	mux.HandleFunc("PUT /api/v1/handlers/{id}/config", h.UpdateConfig)
	mux.HandleFunc("DELETE /api/v1/handlers/{id}/config", h.ClearConfig)
	mux.HandleFunc("GET /api/v1/handlers/{id}/calls", h.ListCalls)
	mux.HandleFunc("GET /api/v1/handler-calls/{callId}", h.GetCall)
}

type createHandlerRequest struct {
	Name           string                      `json:"name"`
	Description    string                      `json:"description"`
	Tags           []string                    `json:"tags"`
	Imports        string                      `json:"imports"`
	InitBody       string                      `json:"initBody"`
	ShutdownBody   string                      `json:"shutdownBody"`
	Methods        []handlerdomain.MethodSpec  `json:"methods"`
	InitArgsSchema []handlerdomain.InitArgSpec `json:"initArgsSchema"`
	Dependencies   []string                    `json:"dependencies"`
	PythonVersion  string                      `json:"pythonVersion"`
	ChangeReason   string                      `json:"changeReason"`
}

func (h *HandlerHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req createHandlerRequest
	if err := decodeJSON(r, &req); err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	hd, v, err := h.svc.CreateDirect(r.Context(), handlerapp.DirectCreateInput{
		Name: req.Name, Description: req.Description, Tags: req.Tags,
		Imports: req.Imports, InitBody: req.InitBody, ShutdownBody: req.ShutdownBody,
		Methods: req.Methods, InitArgsSchema: req.InitArgsSchema,
		Dependencies: req.Dependencies, PythonVersion: req.PythonVersion, ChangeReason: req.ChangeReason,
	})
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	hd.ActiveVersion = v // 裸实体 + 内嵌 activeVersion,与 GET 同形(MD1)
	responsehttpapi.Created(w, hd)
}

func (h *HandlerHandler) List(w http.ResponseWriter, r *http.Request) {
	p, err := responsehttpapi.ParsePage(r)
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	items, next, err := h.svc.List(r.Context(), handlerdomain.ListFilter{Cursor: p.Cursor, Limit: p.Limit})
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Paged(w, items, next, next != "")
}

func (h *HandlerHandler) Get(w http.ResponseWriter, r *http.Request) {
	hd, err := h.svc.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, hd)
}

func (h *HandlerHandler) UpdateMeta(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        *string   `json:"name"`
		Description *string   `json:"description"`
		Tags        *[]string `json:"tags"`
	}
	if err := decodeJSON(r, &req); err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	hd, err := h.svc.UpdateMeta(r.Context(), handlerapp.UpdateMetaInput{
		ID: r.PathValue("id"), Name: req.Name, Description: req.Description, Tags: req.Tags,
	})
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, hd)
}

func (h *HandlerHandler) Delete(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.Delete(r.Context(), r.PathValue("id")); err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.NoContent(w)
}

// postOnHandler dispatches POST /handlers/{id}:<action> (:call / :restart / :revert / :edit).
//
// postOnHandler 派发 POST /handlers/{id}:<action>（:call / :restart / :revert / :edit）。
func (h *HandlerHandler) postOnHandler(w http.ResponseWriter, r *http.Request) {
	id, action, ok := idAndAction(r, "idAction")
	if !ok {
		http.NotFound(w, r)
		return
	}
	switch action {
	case "call":
		h.call(w, r, id)
	case "restart":
		h.restart(w, r, id)
	case "revert":
		h.revert(w, r, id)
	case "edit":
		h.edit(w, r, id)
	case "iterate":
		iterateEntity(w, r, h.log, h.aispawn, mentiondomain.MentionHandler, id)
	default:
		http.NotFound(w, r)
	}
}

func (h *HandlerHandler) call(w http.ResponseWriter, r *http.Request, id string) {
	var req struct {
		Method string         `json:"method"`
		Args   map[string]any `json:"args"`
	}
	if err := decodeJSON(r, &req); err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	res, err := h.svc.Call(r.Context(), handlerapp.CallInput{
		HandlerID: id, Method: req.Method, Args: req.Args, TriggeredBy: handlerdomain.TriggeredByManual,
	})
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, map[string]any{"result": res})
}

func (h *HandlerHandler) restart(w http.ResponseWriter, r *http.Request, id string) {
	state, err := h.svc.Restart(r.Context(), id)
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, map[string]any{"id": id, "runtimeState": state})
}

func (h *HandlerHandler) revert(w http.ResponseWriter, r *http.Request, id string) {
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

func (h *HandlerHandler) edit(w http.ResponseWriter, r *http.Request, id string) {
	var req struct {
		Ops          json.RawMessage `json:"ops"`
		ChangeReason string          `json:"changeReason"`
	}
	if err := decodeJSON(r, &req); err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	var ops []handlerapp.Op
	if len(req.Ops) > 0 {
		parsed, err := handlerapp.ParseOps(req.Ops)
		if err != nil {
			responsehttpapi.FromDomainError(w, h.log, err)
			return
		}
		ops = parsed
	}
	v, err := h.svc.Edit(r.Context(), handlerapp.EditInput{ID: id, Ops: ops, ChangeReason: req.ChangeReason})
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, v)
}

// GetConfig returns the masked config + config state for the handler's active version.
//
// GetConfig 返 handler active 版本的掩码 config + config 状态。
func (h *HandlerHandler) GetConfig(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	hd, err := h.svc.Get(r.Context(), id)
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	var schema []handlerdomain.InitArgSpec
	if hd.ActiveVersion != nil {
		schema = hd.ActiveVersion.InitArgsSchema
	}
	masked, err := h.svc.MaskedConfig(r.Context(), id, schema)
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, map[string]any{
		"config": masked, "configState": hd.ConfigState, "missingConfig": hd.MissingConfig, "schema": schema,
	})
}

func (h *HandlerHandler) UpdateConfig(w http.ResponseWriter, r *http.Request) {
	var req map[string]any
	if err := decodeJSON(r, &req); err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	if err := h.svc.UpdateConfig(r.Context(), r.PathValue("id"), req); err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.NoContent(w)
}

func (h *HandlerHandler) ClearConfig(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.ClearConfig(r.Context(), r.PathValue("id")); err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.NoContent(w)
}

func (h *HandlerHandler) ListVersions(w http.ResponseWriter, r *http.Request) {
	p, err := responsehttpapi.ParsePage(r)
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	rows, next, err := h.svc.ListVersions(r.Context(), r.PathValue("id"), handlerdomain.VersionListFilter{Cursor: p.Cursor, Limit: p.Limit})
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Paged(w, rows, next, next != "")
}

func (h *HandlerHandler) GetVersion(w http.ResponseWriter, r *http.Request) {
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

func (h *HandlerHandler) ListCalls(w http.ResponseWriter, r *http.Request) {
	p, err := responsehttpapi.ParsePage(r)
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	q := r.URL.Query()
	res, err := h.svc.SearchCalls(r.Context(), handlerdomain.CallFilter{
		HandlerID:      r.PathValue("id"),
		VersionID:      q.Get("versionId"),
		Method:         q.Get("method"),
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

func (h *HandlerHandler) GetCall(w http.ResponseWriter, r *http.Request) {
	c, err := h.svc.GetCall(r.Context(), r.PathValue("callId"))
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, c)
}
