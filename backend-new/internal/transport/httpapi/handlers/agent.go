package handlers

import (
	"net/http"
	"strconv"

	"go.uber.org/zap"

	agentapp "github.com/sunweilin/forgify/backend/internal/app/agent"
	agentdomain "github.com/sunweilin/forgify/backend/internal/domain/agent"
	modeldomain "github.com/sunweilin/forgify/backend/internal/domain/model"
	schemapkg "github.com/sunweilin/forgify/backend/internal/pkg/schema"
	responsehttpapi "github.com/sunweilin/forgify/backend/internal/transport/httpapi/response"
)

// AgentHandler hosts the agent HTTP endpoints. The version model is linear with a free-moving
// active pointer — no pending/accept endpoints. The AI :iterate verb depends on askai (波次 6)
// and is added in that wave.
//
// AgentHandler 持 agent HTTP 端点。版本模型线性 + 可自由移动的 active 指针——无 pending/accept 端点。
// AI :iterate 依赖 askai（波次 6），那轮加入。
type AgentHandler struct {
	svc *agentapp.Service
	log *zap.Logger
}

// NewAgentHandler constructs the handler.
//
// NewAgentHandler 构造 handler。
func NewAgentHandler(svc *agentapp.Service, log *zap.Logger) *AgentHandler {
	if log == nil {
		log = zap.NewNop()
	}
	return &AgentHandler{svc: svc, log: log.Named("handlers.agent")}
}

// Register wires the endpoints onto mux.
//
// Register 把端点挂到 mux。
func (h *AgentHandler) Register(mux Registrar) {
	mux.HandleFunc("POST /api/v1/agents", h.Create)
	mux.HandleFunc("GET /api/v1/agents", h.List)
	mux.HandleFunc("GET /api/v1/agents/{id}", h.Get)
	mux.HandleFunc("PATCH /api/v1/agents/{id}", h.UpdateMeta)
	mux.HandleFunc("DELETE /api/v1/agents/{id}", h.Delete)
	mux.HandleFunc("POST /api/v1/agents/{idAction}", h.postOnAgent)
	mux.HandleFunc("GET /api/v1/agents/{id}/versions", h.ListVersions)
	mux.HandleFunc("GET /api/v1/agents/{id}/versions/{version}", h.GetVersion)
	mux.HandleFunc("GET /api/v1/agents/{id}/executions", h.ListExecutions)
	mux.HandleFunc("GET /api/v1/agent-executions/{execId}", h.GetExecution)
	mux.HandleFunc("POST /api/v1/agent-executions/{execId}/interactions/{toolCallId}", h.ResolveExecution)
}

// agentConfigRequest is the mounted config carried by create/edit HTTP bodies.
//
// agentConfigRequest 是 create/edit HTTP body 携带的挂载配置。
type agentConfigRequest struct {
	Prompt        string                `json:"prompt"`
	Skill         string                `json:"skill"`
	Knowledge     []string              `json:"knowledge"`
	Tools         []agentdomain.ToolRef `json:"tools"`
	Inputs        []schemapkg.Field     `json:"inputs"`
	Outputs       []schemapkg.Field     `json:"outputs"`
	ModelOverride *modeldomain.ModelRef `json:"modelOverride"`
	ChangeReason  string                `json:"changeReason"`
}

func (c agentConfigRequest) toConfig() agentapp.Config {
	return agentapp.Config{
		Prompt: c.Prompt, Skill: c.Skill, Knowledge: c.Knowledge, Tools: c.Tools,
		Inputs: c.Inputs, Outputs: c.Outputs, ModelOverride: c.ModelOverride, ChangeReason: c.ChangeReason,
	}
}

func (h *AgentHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string   `json:"name"`
		Description string   `json:"description"`
		Tags        []string `json:"tags"`
		agentConfigRequest
	}
	if err := decodeJSON(r, &req); err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	ag, v, err := h.svc.Create(r.Context(), agentapp.CreateInput{
		Name: req.Name, Description: req.Description, Tags: req.Tags, Config: req.agentConfigRequest.toConfig(),
	})
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Created(w, map[string]any{"agent": ag, "version": v})
}

func (h *AgentHandler) List(w http.ResponseWriter, r *http.Request) {
	p, err := responsehttpapi.ParsePage(r)
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	items, next, err := h.svc.List(r.Context(), p.Limit, p.Cursor)
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Paged(w, items, next, next != "")
}

func (h *AgentHandler) Get(w http.ResponseWriter, r *http.Request) {
	ag, err := h.svc.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, ag)
}

func (h *AgentHandler) UpdateMeta(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        *string   `json:"name"`
		Description *string   `json:"description"`
		Tags        *[]string `json:"tags"`
	}
	if err := decodeJSON(r, &req); err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	ag, err := h.svc.UpdateMeta(r.Context(), agentapp.UpdateMetaInput{
		ID: r.PathValue("id"), Name: req.Name, Description: req.Description, Tags: req.Tags,
	})
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, ag)
}

func (h *AgentHandler) Delete(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.Delete(r.Context(), r.PathValue("id")); err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.NoContent(w)
}

// postOnAgent dispatches POST /agents/{id}:<action> (:invoke / :revert / :edit).
//
// postOnAgent 派发 POST /agents/{id}:<action>（:invoke / :revert / :edit）。
func (h *AgentHandler) postOnAgent(w http.ResponseWriter, r *http.Request) {
	id, action, ok := idAndAction(r, "idAction")
	if !ok {
		http.NotFound(w, r)
		return
	}
	switch action {
	case "invoke":
		h.invoke(w, r, id)
	case "revert":
		h.revert(w, r, id)
	case "edit":
		h.edit(w, r, id)
	default:
		http.NotFound(w, r)
	}
}

func (h *AgentHandler) invoke(w http.ResponseWriter, r *http.Request, id string) {
	var req struct {
		Input   map[string]any `json:"input"`
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
	res, err := h.svc.InvokeAgent(r.Context(), agentapp.InvokeInput{
		AgentID:     id,
		VersionID:   versionID,
		Input:       req.Input,
		TriggeredBy: agentdomain.TriggeredByManual,
	})
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, res)
}

func (h *AgentHandler) revert(w http.ResponseWriter, r *http.Request, id string) {
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

func (h *AgentHandler) edit(w http.ResponseWriter, r *http.Request, id string) {
	var req struct {
		agentConfigRequest
	}
	if err := decodeJSON(r, &req); err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	v, err := h.svc.Edit(r.Context(), agentapp.EditInput{ID: id, Config: req.agentConfigRequest.toConfig()})
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, v)
}

func (h *AgentHandler) ListVersions(w http.ResponseWriter, r *http.Request) {
	rows, err := h.svc.ListVersions(r.Context(), r.PathValue("id"))
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, rows)
}

// GetVersion accepts either an integer version number or a version id in {version}.
//
// GetVersion 的 {version} 接整数版本号或 version id。
func (h *AgentHandler) GetVersion(w http.ResponseWriter, r *http.Request) {
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

func (h *AgentHandler) ListExecutions(w http.ResponseWriter, r *http.Request) {
	p, err := responsehttpapi.ParsePage(r)
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	q := r.URL.Query()
	res, err := h.svc.SearchExecutions(r.Context(), agentdomain.ExecutionFilter{
		AgentID:        r.PathValue("id"),
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

func (h *AgentHandler) GetExecution(w http.ResponseWriter, r *http.Request) {
	e, err := h.svc.GetExecutionDetail(r.Context(), r.PathValue("execId"))
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, e)
}

// ResolveExecution resolves a parked agent run's pending interaction (R0064): danger approve | deny,
// ask accept | decline, or cancel — body {"action": "...", "answer": "..."?}. The run resumes
// synchronously (the agent loop is not queued like chat); the response carries its next state.
//
// ResolveExecution 决议一个 parked agent 运行的待决交互（R0064）：danger approve|deny、ask accept|decline、
// 或 cancel——body {"action":"...","answer":"..."?}。运行同步恢复（agent loop 不像 chat 那样排队）；响应带下一态。
func (h *AgentHandler) ResolveExecution(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Action string `json:"action"`
		Answer string `json:"answer"`
	}
	if err := decodeJSON(r, &req); err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	res, err := h.svc.ResumeExecution(r.Context(), r.PathValue("execId"), r.PathValue("toolCallId"), req.Action, req.Answer)
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, res)
}
