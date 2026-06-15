package handlers

import (
	"net/http"
	"strconv"

	"go.uber.org/zap"

	agentapp "github.com/sunweilin/forgify/backend/internal/app/agent"
	aispawnapp "github.com/sunweilin/forgify/backend/internal/app/aispawn"
	agentdomain "github.com/sunweilin/forgify/backend/internal/domain/agent"
	mentiondomain "github.com/sunweilin/forgify/backend/internal/domain/mention"
	modeldomain "github.com/sunweilin/forgify/backend/internal/domain/model"
	errorspkg "github.com/sunweilin/forgify/backend/internal/pkg/errors"
	schemapkg "github.com/sunweilin/forgify/backend/internal/pkg/schema"
	responsehttpapi "github.com/sunweilin/forgify/backend/internal/transport/httpapi/response"
)

// AgentHandler hosts the agent HTTP endpoints. The version model is linear with a free-moving
// active pointer — no pending/accept endpoints. The :iterate verb opens an AI conversation
// to edit this agent via aispawn.
//
// AgentHandler 持 agent HTTP 端点。版本模型线性 + 可自由移动的 active 指针——无 pending/accept 端点。
// :iterate 动词经 aispawn 开一个 AI 对话来编辑本 agent。
type AgentHandler struct {
	svc     *agentapp.Service
	aispawn *aispawnapp.Service
	log     *zap.Logger
}

// NewAgentHandler constructs the handler.
//
// NewAgentHandler 构造 handler。
func NewAgentHandler(svc *agentapp.Service, aispawn *aispawnapp.Service, log *zap.Logger) *AgentHandler {
	if log == nil {
		log = zap.NewNop()
	}
	return &AgentHandler{svc: svc, aispawn: aispawn, log: log.Named("handlers.agent")}
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
	mux.HandleFunc("GET /api/v1/agents/{id}/mount-health", h.MountHealth)
	mux.HandleFunc("GET /api/v1/agents/{id}/versions", h.ListVersions)
	mux.HandleFunc("GET /api/v1/agents/{id}/versions/{version}", h.GetVersion)
	mux.HandleFunc("GET /api/v1/agents/{id}/executions", h.ListExecutions)
	mux.HandleFunc("GET /api/v1/agent-executions/{id}", h.GetExecution) // Log 单读路径变量统一 {id}(MD-id4)
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
	ag.ActiveVersion = v // 裸实体 + 内嵌 activeVersion,与 GET 同形
	responsehttpapi.Created(w, ag)
}

func (h *AgentHandler) List(w http.ResponseWriter, r *http.Request) {
	p, err := responsehttpapi.ParsePage(r)
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	items, next, err := h.svc.List(r.Context(), agentdomain.ListFilter{Cursor: p.Cursor, Limit: p.Limit})
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

// MountHealth handles GET /agents/{id}/mount-health — the on-demand precheck of whether the agent's
// active-version mounts (fn/hd/mcp) still resolve, so the UI can warn before invoke.
//
// MountHealth 处理 GET /agents/{id}/mount-health——按需预检 agent active 版本的挂载（fn/hd/mcp）是否
// 仍可解析，使 UI 在 invoke 前预警。
func (h *AgentHandler) MountHealth(w http.ResponseWriter, r *http.Request) {
	report, err := h.svc.MountHealth(r.Context(), r.PathValue("id"))
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, report)
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

// postOnAgent dispatches POST /agents/{id}:<action> (:invoke / :revert / :edit / :iterate).
//
// postOnAgent 派发 POST /agents/{id}:<action>（:invoke / :revert / :edit / :iterate）。
func (h *AgentHandler) postOnAgent(w http.ResponseWriter, r *http.Request) {
	id, action, ok := idAndAction(r, "idAction")
	if !ok {
		responsehttpapi.FromDomainError(w, h.log, errorspkg.ErrNotFound)
		return
	}
	switch action {
	case "invoke":
		h.invoke(w, r, id)
	case "revert":
		h.revert(w, r, id)
	case "edit":
		h.edit(w, r, id)
	case "iterate":
		iterateEntity(w, r, h.log, h.aispawn, mentiondomain.MentionAgent, id)
	default:
		responsehttpapi.FromDomainError(w, h.log, errorspkg.ErrNotFound)
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
	p, err := responsehttpapi.ParsePage(r)
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	rows, next, err := h.svc.ListVersions(r.Context(), r.PathValue("id"), agentdomain.VersionListFilter{Cursor: p.Cursor, Limit: p.Limit})
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Paged(w, rows, next, next != "")
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
	// 分页坐标恒顶层(Paged);aggregates 作 list 元数据进 data 子对象。
	responsehttpapi.Paged(w, map[string]any{"executions": res.Executions, "aggregates": res.Aggregates}, res.NextCursor, res.HasMore)
}

func (h *AgentHandler) GetExecution(w http.ResponseWriter, r *http.Request) {
	e, err := h.svc.GetExecutionDetail(r.Context(), r.PathValue("id"))
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, e)
}
