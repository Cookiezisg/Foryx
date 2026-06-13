package handlers

import (
	"net/http"

	"go.uber.org/zap"

	aispawnapp "github.com/sunweilin/forgify/backend/internal/app/aispawn"
	triggerapp "github.com/sunweilin/forgify/backend/internal/app/trigger"
	mentiondomain "github.com/sunweilin/forgify/backend/internal/domain/mention"
	triggerdomain "github.com/sunweilin/forgify/backend/internal/domain/trigger"
	schemapkg "github.com/sunweilin/forgify/backend/internal/pkg/schema"
	responsehttpapi "github.com/sunweilin/forgify/backend/internal/transport/httpapi/response"
)

// TriggerHandler hosts the trigger HTTP endpoints. A trigger is a standalone signal source
// (cron / webhook / fsnotify / sensor) with no version model. Edit is a plain PATCH (config
// takes effect immediately); :fire manually fires it. The activation log (GET .../activations)
// answers "why didn't it fire?". Reference-counted listen lifecycle is driven by workflow
// activate/deactivate (波次 4), not exposed here.
//
// TriggerHandler 持 trigger HTTP 端点。trigger 是独立信号源（cron/webhook/fsnotify/sensor），无版本。
// Edit 是普通 PATCH（config 立即生效）；:fire 手动触发。activation 日志回答「为什么没触发」。
// 引用计数监听生命周期由 workflow 激活/停用驱动（波次 4），不在此暴露。
type TriggerHandler struct {
	svc     *triggerapp.Service
	aispawn *aispawnapp.Service
	log     *zap.Logger
}

func NewTriggerHandler(svc *triggerapp.Service, aispawn *aispawnapp.Service, log *zap.Logger) *TriggerHandler {
	if log == nil {
		log = zap.NewNop()
	}
	return &TriggerHandler{svc: svc, aispawn: aispawn, log: log.Named("handlers.trigger")}
}

func (h *TriggerHandler) Register(mux Registrar) {
	mux.HandleFunc("POST /api/v1/triggers", h.Create)
	mux.HandleFunc("GET /api/v1/triggers", h.List)
	mux.HandleFunc("GET /api/v1/triggers/{id}", h.Get)
	mux.HandleFunc("PATCH /api/v1/triggers/{id}", h.Edit)
	mux.HandleFunc("DELETE /api/v1/triggers/{id}", h.Delete)
	mux.HandleFunc("POST /api/v1/triggers/{idAction}", h.postOnTrigger)
	mux.HandleFunc("GET /api/v1/triggers/{id}/activations", h.ListActivations)
	mux.HandleFunc("GET /api/v1/triggers/{id}/firings", h.ListFirings)
	mux.HandleFunc("GET /api/v1/trigger-activations/{actId}", h.GetActivation)
}

func (h *TriggerHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string            `json:"name"`
		Description string            `json:"description"`
		Kind        string            `json:"kind"`
		Config      map[string]any    `json:"config"`
		Outputs     []schemapkg.Field `json:"outputs"`
	}
	if err := decodeJSON(r, &req); err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	t, err := h.svc.Create(r.Context(), triggerapp.CreateInput{
		Name: req.Name, Description: req.Description, Kind: req.Kind, Config: req.Config, Outputs: req.Outputs,
	})
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Created(w, map[string]any{"trigger": t})
}

func (h *TriggerHandler) List(w http.ResponseWriter, r *http.Request) {
	p, err := responsehttpapi.ParsePage(r)
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	items, next, err := h.svc.List(r.Context(), triggerdomain.ListFilter{Cursor: p.Cursor, Limit: p.Limit})
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Paged(w, items, next, next != "")
}

func (h *TriggerHandler) Get(w http.ResponseWriter, r *http.Request) {
	t, err := h.svc.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, t)
}

func (h *TriggerHandler) Edit(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        *string           `json:"name"`
		Description *string           `json:"description"`
		Config      map[string]any    `json:"config"`
		Outputs     []schemapkg.Field `json:"outputs"`
	}
	if err := decodeJSON(r, &req); err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	t, err := h.svc.Edit(r.Context(), r.PathValue("id"), triggerapp.EditInput{
		Name: req.Name, Description: req.Description, Config: req.Config, Outputs: req.Outputs,
	})
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, t)
}

func (h *TriggerHandler) Delete(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.Delete(r.Context(), r.PathValue("id")); err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.NoContent(w)
}

// postOnTrigger dispatches POST /triggers/{id}:<action> (only :fire today).
//
// postOnTrigger 派发 POST /triggers/{id}:<action>（目前仅 :fire）。
func (h *TriggerHandler) postOnTrigger(w http.ResponseWriter, r *http.Request) {
	id, action, ok := idAndAction(r, "idAction")
	if !ok {
		http.NotFound(w, r)
		return
	}
	switch action {
	case "fire":
		actID, err := h.svc.FireManual(r.Context(), id)
		if err != nil {
			responsehttpapi.FromDomainError(w, h.log, err)
			return
		}
		responsehttpapi.Success(w, http.StatusAccepted, map[string]any{"fired": true, "triggerId": id, "activationId": actID})
	case "iterate":
		iterateEntity(w, r, h.log, h.aispawn, mentiondomain.MentionTrigger, id)
	default:
		http.NotFound(w, r)
	}
}

func (h *TriggerHandler) ListActivations(w http.ResponseWriter, r *http.Request) {
	p, err := responsehttpapi.ParsePage(r)
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	acts, next, err := h.svc.SearchActivations(r.Context(), triggerdomain.ActivationFilter{
		TriggerID: r.PathValue("id"),
		FiredOnly: r.URL.Query().Get("firedOnly") == "true",
		Cursor:    p.Cursor,
		Limit:     p.Limit,
	})
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Paged(w, acts, next, next != "")
}

// ListFirings pages the trigger's firing inbox (?status=pending|started|skipped|superseded|shed) —
// the disposition surface behind "it fired, why didn't it run".
//
// ListFirings 分页 trigger 的 firing 收件箱（?status=…）——「触发了为什么没跑」的处置面。
func (h *TriggerHandler) ListFirings(w http.ResponseWriter, r *http.Request) {
	p, err := responsehttpapi.ParsePage(r)
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	rows, next, err := h.svc.SearchFirings(r.Context(), triggerdomain.FiringFilter{
		TriggerID: r.PathValue("id"),
		Status:    r.URL.Query().Get("status"),
		Cursor:    p.Cursor,
		Limit:     p.Limit,
	})
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Paged(w, rows, next, next != "")
}

func (h *TriggerHandler) GetActivation(w http.ResponseWriter, r *http.Request) {
	act, err := h.svc.GetActivation(r.Context(), r.PathValue("actId"))
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, act)
}
