package handlers

import (
	"net/http"

	"go.uber.org/zap"

	apikeyapp "github.com/sunweilin/forgify/backend/internal/app/apikey"
	apikeydomain "github.com/sunweilin/forgify/backend/internal/domain/apikey"
	responsehttpapi "github.com/sunweilin/forgify/backend/internal/transport/httpapi/response"
)

// APIKeyHandler serves /api/v1/api-keys (CRUD + :test) and /api/v1/providers
// (the static provider catalog used during onboarding).
//
// APIKeyHandler 提供 /api/v1/api-keys（CRUD + :test）与 /api/v1/providers
// （onboarding 用的静态 provider 目录）。
type APIKeyHandler struct {
	svc *apikeyapp.Service
	log *zap.Logger
}

func NewAPIKeyHandler(svc *apikeyapp.Service, log *zap.Logger) *APIKeyHandler {
	if log == nil {
		log = zap.NewNop()
	}
	return &APIKeyHandler{svc: svc, log: log.Named("handlers.apikey")}
}

func (h *APIKeyHandler) Register(mux Registrar) {
	mux.HandleFunc("POST /api/v1/api-keys", h.Create)
	mux.HandleFunc("GET /api/v1/api-keys", h.List)
	mux.HandleFunc("PATCH /api/v1/api-keys/{id}", h.Update)
	mux.HandleFunc("DELETE /api/v1/api-keys/{id}", h.Delete)
	mux.HandleFunc("POST /api/v1/api-keys/{idAction}", h.postOnID)
	// Provider catalog — onboarding lists this before any key/workspace exists
	// (exempt from RequireWorkspace).
	// provider 目录——onboarding 在任何 key/workspace 前列它（豁免 RequireWorkspace）。
	mux.HandleFunc("GET /api/v1/providers", h.ListProviders)
}

type createRequest struct {
	Provider    string `json:"provider"`
	DisplayName string `json:"displayName"`
	Key         string `json:"key"`
	BaseURL     string `json:"baseUrl"`
	APIFormat   string `json:"apiFormat"`
}

type updateRequest struct {
	DisplayName *string `json:"displayName"`
	BaseURL     *string `json:"baseUrl"`
	Key         *string `json:"key"`
}

func (h *APIKeyHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req createRequest
	if err := decodeJSON(r, &req); err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	k, err := h.svc.Create(r.Context(), apikeyapp.CreateInput{
		Provider:    req.Provider,
		DisplayName: req.DisplayName,
		Key:         req.Key,
		BaseURL:     req.BaseURL,
		APIFormat:   req.APIFormat,
	})
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Created(w, k)
}

func (h *APIKeyHandler) List(w http.ResponseWriter, r *http.Request) {
	p, err := responsehttpapi.ParsePage(r)
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	items, next, err := h.svc.List(r.Context(), apikeydomain.ListFilter{
		Cursor:   p.Cursor,
		Limit:    p.Limit,
		Provider: r.URL.Query().Get("provider"),
	})
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Paged(w, items, next, next != "")
}

func (h *APIKeyHandler) Update(w http.ResponseWriter, r *http.Request) {
	var req updateRequest
	if err := decodeJSON(r, &req); err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	k, err := h.svc.Update(r.Context(), r.PathValue("id"), apikeyapp.UpdateInput{
		DisplayName: req.DisplayName,
		BaseURL:     req.BaseURL,
		Key:         req.Key,
	})
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, k)
}

func (h *APIKeyHandler) Delete(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.Delete(r.Context(), r.PathValue("id")); err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.NoContent(w)
}

func (h *APIKeyHandler) postOnID(w http.ResponseWriter, r *http.Request) {
	id, action, ok := idAndAction(r, "idAction")
	if !ok {
		responsehttpapi.Error(w, http.StatusNotFound, "NOT_FOUND", "route not found", nil)
		return
	}
	switch action {
	case "test":
		h.test(w, r, id)
	default:
		responsehttpapi.Error(w, http.StatusNotFound, "NOT_FOUND", "unknown action: "+action, nil)
	}
}

// test runs the dumb probe: 200 on live, 422 (API_KEY_TEST_FAILED) on a dead
// key. Returns only {ok, message, latencyMs} — "what models" lives in the model
// module's endpoint, not here.
//
// test 跑哑探针：活返 200，死返 422（API_KEY_TEST_FAILED）。只返 {ok, message, latencyMs}
// ——「有哪些模型」在 model 模块的端点，不在这。
func (h *APIKeyHandler) test(w http.ResponseWriter, r *http.Request, id string) {
	res, err := h.svc.Test(r.Context(), id)
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	if !res.OK {
		responsehttpapi.Error(w, http.StatusUnprocessableEntity,
			"API_KEY_TEST_FAILED", res.Message, map[string]any{"latencyMs": res.LatencyMs})
		return
	}
	responsehttpapi.Success(w, http.StatusOK, map[string]any{
		"ok":        true,
		"message":   res.Message,
		"latencyMs": res.LatencyMs,
	})
}

// ListProviders returns the static provider catalog (name/displayName/baseUrl
// requirement/category) for the onboarding key-config UI.
//
// ListProviders 返回静态 provider 目录，供 onboarding 配 key 的 UI。
func (h *APIKeyHandler) ListProviders(w http.ResponseWriter, r *http.Request) {
	responsehttpapi.Success(w, http.StatusOK, apikeyapp.ListProviders())
}
