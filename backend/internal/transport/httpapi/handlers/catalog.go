package handlers

import (
	"net/http"

	"go.uber.org/zap"

	catalogapp "github.com/sunweilin/forgify/backend/internal/app/catalog"
	responsehttpapi "github.com/sunweilin/forgify/backend/internal/transport/httpapi/response"
)

// CatalogHandler hosts the 2 capability-catalog endpoints.
//
// CatalogHandler 持 2 个 catalog 端点。
type CatalogHandler struct {
	svc *catalogapp.Service
	log *zap.Logger
}

func NewCatalogHandler(svc *catalogapp.Service, log *zap.Logger) *CatalogHandler {
	if log == nil {
		log = zap.NewNop()
	}
	return &CatalogHandler{svc: svc, log: log.Named("handlers.catalog")}
}

func (h *CatalogHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/catalog", h.Get)
	mux.HandleFunc("POST /api/v1/catalog:refresh", h.Refresh)
}

// Get returns the current cached Catalog; null when cache not yet built.
//
// Get 返当前缓存 Catalog;未构造时返 null。
func (h *CatalogHandler) Get(w http.ResponseWriter, _ *http.Request) {
	responsehttpapi.Success(w, http.StatusOK, h.svc.Get())
}

// Refresh forces an immediate Service.Refresh and returns the new Catalog.
//
// Refresh 强制立即刷新并返新 Catalog。
func (h *CatalogHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.Refresh(r.Context()); err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, h.svc.Get())
}
