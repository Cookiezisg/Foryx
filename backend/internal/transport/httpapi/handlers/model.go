package handlers

import (
	"net/http"

	"go.uber.org/zap"

	modelapp "github.com/sunweilin/forgify/backend/internal/app/model"
	responsehttpapi "github.com/sunweilin/forgify/backend/internal/transport/httpapi/response"
)

// ModelConfigHandler serves /api/v1/model-configs/* endpoints.
//
// ModelConfigHandler 提供 /api/v1/model-configs/* 端点。
type ModelConfigHandler struct {
	svc *modelapp.Service
	log *zap.Logger
}

func NewModelConfigHandler(svc *modelapp.Service, log *zap.Logger) *ModelConfigHandler {
	return &ModelConfigHandler{svc: svc, log: log}
}

func (h *ModelConfigHandler) Register(mux Registrar) {
	mux.HandleFunc("GET /api/v1/model-configs", h.List)
	mux.HandleFunc("GET /api/v1/model-configs/{scenario}", h.Get)
	mux.HandleFunc("PUT /api/v1/model-configs/{scenario}", h.Upsert)
}

type upsertModelRequest struct {
	Provider string `json:"provider"`
	ModelID  string `json:"modelId"`
}

func (h *ModelConfigHandler) List(w http.ResponseWriter, r *http.Request) {
	items, err := h.svc.List(r.Context())
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, items)
}

func (h *ModelConfigHandler) Get(w http.ResponseWriter, r *http.Request) {
	scenario := r.PathValue("scenario")
	m, err := h.svc.GetByScenario(r.Context(), scenario)
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, m)
}

func (h *ModelConfigHandler) Upsert(w http.ResponseWriter, r *http.Request) {
	scenario := r.PathValue("scenario")
	var req upsertModelRequest
	if err := decodeJSON(r, &req); err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	m, err := h.svc.Upsert(r.Context(), scenario, modelapp.UpsertInput{
		Provider: req.Provider,
		ModelID:  req.ModelID,
	})
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, m)
}
