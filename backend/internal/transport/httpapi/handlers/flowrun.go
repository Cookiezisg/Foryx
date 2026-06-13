package handlers

import (
	"net/http"

	"go.uber.org/zap"

	schedulerapp "github.com/sunweilin/forgify/backend/internal/app/scheduler"
	flowrundomain "github.com/sunweilin/forgify/backend/internal/domain/flowrun"
	responsehttpapi "github.com/sunweilin/forgify/backend/internal/transport/httpapi/response"
)

// FlowrunHandler hosts the durable-execution HTTP surface: list/inspect runs, start one manually
// ("Run now"), replay a failed one, and decide a parked approval. A flowrun is a runtime record
// (no version, no forge) — so there is no Create-as-edit / :revert here; "create" is starting a run
// and its body is the entry trigger's declared Outputs (the manual payload form, doc 21 §4.6).
//
// FlowrunHandler 持持久化执行的 HTTP 面：列/查 run、手动起一个（「Run now」）、replay 失败的、决策
// parked 审批。flowrun 是运行时记录（无版本、无锻造）——故无 Create-as-edit/:revert；「create」就是起
// 一个 run，body 形如入口 trigger 声明的 Outputs（手动 payload 表单，doc 21 §4.6）。
type FlowrunHandler struct {
	svc *schedulerapp.Service
	log *zap.Logger
}

func NewFlowrunHandler(svc *schedulerapp.Service, log *zap.Logger) *FlowrunHandler {
	if log == nil {
		log = zap.NewNop()
	}
	return &FlowrunHandler{svc: svc, log: log.Named("handlers.flowrun")}
}

func (h *FlowrunHandler) Register(mux Registrar) {
	mux.HandleFunc("GET /api/v1/flowruns", h.List)
	mux.HandleFunc("POST /api/v1/flowruns", h.Start)
	mux.HandleFunc("GET /api/v1/flowrun-inbox", h.Inbox)
	mux.HandleFunc("GET /api/v1/flowruns/{id}", h.Get)
	mux.HandleFunc("POST /api/v1/flowruns/{idAction}", h.postOnRun)
	mux.HandleFunc("POST /api/v1/flowruns/{id}/approvals/{nodeAction}", h.postOnApproval)
}

// List pages a workspace's runs (newest-first), optionally filtered via ?workflowId and/or ?status (running|completed|failed|cancelled).
//
// List 分页一个 workspace 的 run（最新优先），可选 ?workflowId / ?status 过滤。
func (h *FlowrunHandler) List(w http.ResponseWriter, r *http.Request) {
	p, err := responsehttpapi.ParsePage(r)
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	runs, next, err := h.svc.ListRuns(r.Context(), flowrundomain.ListFilter{
		WorkflowID: r.URL.Query().Get("workflowId"),
		Status:     r.URL.Query().Get("status"),
		Cursor:     p.Cursor,
		Limit:      p.Limit,
	})
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Paged(w, runs, next, next != "")
}

// Start is the manual-trigger path ("Run now"): create + advance a run. The payload conforms to the
// entry trigger's Outputs; entryNode disambiguates a multi-trigger graph. Returns the run + nodes
// (which may already be completed, failed, or running-parked since advance is synchronous in v1).
//
// Start 是手动 trigger 路径（「Run now」）：建 + advance 一个 run。payload 形如入口 trigger 的 Outputs；
// entryNode 在多 trigger 图里消歧。返 run + 节点（v1 advance 同步，故可能已 completed/failed/running-parked）。
func (h *FlowrunHandler) Start(w http.ResponseWriter, r *http.Request) {
	var req struct {
		WorkflowID string         `json:"workflowId"`
		EntryNode  string         `json:"entryNode"`
		Payload    map[string]any `json:"payload"`
	}
	if err := decodeJSON(r, &req); err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	id, err := h.svc.StartRun(r.Context(), schedulerapp.StartInput{
		WorkflowID: req.WorkflowID,
		EntryNode:  req.EntryNode,
		Payload:    req.Payload,
	})
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	h.writeRun(w, r, id, responsehttpapi.Created)
}

// Get returns one run header + all its node rows (the full memoization).
//
// Get 返一个 run 头 + 它全部节点行（完整记忆化）。
func (h *FlowrunHandler) Get(w http.ResponseWriter, r *http.Request) {
	run, nodes, err := h.svc.GetRunWithNodes(r.Context(), r.PathValue("id"))
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, map[string]any{"flowrun": run, "nodes": nodes})
}

// Inbox returns every parked approval node in the workspace (the approval inbox).
//
// Inbox 返 workspace 内所有 parked approval 节点（审批收件箱）。
func (h *FlowrunHandler) Inbox(w http.ResponseWriter, r *http.Request) {
	parked, err := h.svc.ListInbox(r.Context())
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, map[string]any{"parked": parked})
}

// postOnRun dispatches POST /flowruns/{id}:<action> (only :replay today).
//
// postOnRun 派发 POST /flowruns/{id}:<action>（目前仅 :replay）。
func (h *FlowrunHandler) postOnRun(w http.ResponseWriter, r *http.Request) {
	id, action, ok := idAndAction(r, "idAction")
	if !ok {
		http.NotFound(w, r)
		return
	}
	switch action {
	case "replay":
		if err := h.svc.Replay(r.Context(), id); err != nil {
			responsehttpapi.FromDomainError(w, h.log, err)
			return
		}
		h.writeRun(w, r, id, func(w http.ResponseWriter, data any) { responsehttpapi.Success(w, http.StatusAccepted, data) })
	default:
		http.NotFound(w, r)
	}
}

// postOnApproval dispatches POST /flowruns/{id}/approvals/{nodeId}:decide with a {decision,reason} body.
//
// postOnApproval 派发 POST /flowruns/{id}/approvals/{nodeId}:decide，body {decision,reason}。
func (h *FlowrunHandler) postOnApproval(w http.ResponseWriter, r *http.Request) {
	nodeID, action, ok := idAndAction(r, "nodeAction")
	if !ok || action != "decide" {
		http.NotFound(w, r)
		return
	}
	var req struct {
		Decision string `json:"decision"`
		Reason   string `json:"reason"`
	}
	if err := decodeJSON(r, &req); err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	if err := h.svc.DecideApproval(r.Context(), r.PathValue("id"), nodeID, req.Decision, req.Reason); err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	h.writeRun(w, r, r.PathValue("id"), func(w http.ResponseWriter, data any) { responsehttpapi.Success(w, http.StatusAccepted, data) })
}

// writeRun re-reads the run + nodes and writes them with the given responder (so a caller sees the
// run's post-action state).
//
// writeRun 重读 run + 节点并用给定 responder 写出（使调用方见动作后的 run 态）。
func (h *FlowrunHandler) writeRun(w http.ResponseWriter, r *http.Request, id string, respond func(http.ResponseWriter, any)) {
	run, nodes, err := h.svc.GetRunWithNodes(r.Context(), id)
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	respond(w, map[string]any{"flowrun": run, "nodes": nodes})
}
