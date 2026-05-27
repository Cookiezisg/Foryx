package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"go.uber.org/zap"

	apikeyapp "github.com/sunweilin/forgify/backend/internal/app/apikey"
	apikeydomain "github.com/sunweilin/forgify/backend/internal/domain/apikey"
	errorsdomain "github.com/sunweilin/forgify/backend/internal/domain/errors"
	paginationpkg "github.com/sunweilin/forgify/backend/internal/pkg/pagination"
	responsehttpapi "github.com/sunweilin/forgify/backend/internal/transport/httpapi/response"
)

// APIKeyHandler serves the 5 /api/v1/api-keys/* endpoints.
//
// APIKeyHandler 提供 /api/v1/api-keys/* 的 5 个端点。
type APIKeyHandler struct {
	svc *apikeyapp.Service
	log *zap.Logger
}

func NewAPIKeyHandler(svc *apikeyapp.Service, log *zap.Logger) *APIKeyHandler {
	return &APIKeyHandler{svc: svc, log: log}
}

// Register uses POST /{idAction} so `/{id}:test` is captured for action dispatch.
//
// Register 用 POST /{idAction} 把 `/{id}:test` 收上来在 postOnID 内分派。
func (h *APIKeyHandler) Register(mux Registrar) {
	mux.HandleFunc("POST /api/v1/api-keys", h.Create)
	mux.HandleFunc("GET /api/v1/api-keys", h.List)
	mux.HandleFunc("PATCH /api/v1/api-keys/{id}", h.Update)
	mux.HandleFunc("DELETE /api/v1/api-keys/{id}", h.Delete)
	mux.HandleFunc("POST /api/v1/api-keys/{idAction}", h.postOnID)
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
	IsDefault   *bool   `json:"isDefault"`
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
	p, err := paginationpkg.Parse(r)
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
	id := r.PathValue("id")
	var req updateRequest
	if err := decodeJSON(r, &req); err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	k, err := h.svc.Update(r.Context(), id, apikeyapp.UpdateInput{
		DisplayName: req.DisplayName,
		BaseURL:     req.BaseURL,
		Key:         req.Key,
		IsDefault:   req.IsDefault,
	})
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, k)
}

func (h *APIKeyHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := h.svc.Delete(r.Context(), id); err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.NoContent(w)
}

func (h *APIKeyHandler) postOnID(w http.ResponseWriter, r *http.Request) {
	id, action, found := idAndAction(r, "idAction")
	if !found {
		responsehttpapi.Error(w, http.StatusNotFound, "NOT_FOUND", "route not found", nil)
		return
	}
	switch action {
	case "test":
		h.test(w, r, id)
	default:
		responsehttpapi.Error(w, http.StatusNotFound, "NOT_FOUND",
			fmt.Sprintf("unknown action %q", action), nil)
	}
}

// test returns 200 on probe OK, 422 (API_KEY_TEST_FAILED) on probe failure.
//
// test 探测成功返 200,失败返 422(API_KEY_TEST_FAILED)。
func (h *APIKeyHandler) test(w http.ResponseWriter, r *http.Request, id string) {
	res, err := h.svc.Test(r.Context(), id)
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	if !res.OK {
		responsehttpapi.Error(w, http.StatusUnprocessableEntity,
			"API_KEY_TEST_FAILED", res.Message,
			map[string]any{"latencyMs": res.LatencyMs})
		return
	}
	models := res.ModelsFound
	if models == nil {
		models = []string{}
	}
	responsehttpapi.Success(w, http.StatusOK, map[string]any{
		"ok":          true,
		"message":     res.Message,
		"latencyMs":   res.LatencyMs,
		"modelsFound": models,
	})
}

// decodeJSON joins ErrInvalidRequest so errmap renders 400 uniformly.
//
// decodeJSON 包入 ErrInvalidRequest,让 errmap 统一渲染 400。
func decodeJSON(r *http.Request, v any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("handlers.decodeJSON: %w", joinInvalidRequest(err))
	}
	return nil
}

func joinInvalidRequest(err error) error {
	return errors.Join(err, errorsdomain.ErrInvalidRequest)
}
