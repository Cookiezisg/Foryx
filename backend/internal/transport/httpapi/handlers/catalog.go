package handlers

import (
	"net/http"

	"go.uber.org/zap"

	catalogapp "github.com/sunweilin/forgify/backend/internal/app/catalog"
	responsehttpapi "github.com/sunweilin/forgify/backend/internal/transport/httpapi/response"
)

// CatalogHandler hosts the catalog inspection endpoint.
//
// CatalogHandler 持 catalog 巡检端点。
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

func (h *CatalogHandler) Register(mux Registrar) {
	mux.HandleFunc("GET /api/v1/catalog", h.Get)
}

// Get builds and returns the current capability catalog for the request user.
//
// Get 按需构建并返回当前用户的能力清单。
func (h *CatalogHandler) Get(w http.ResponseWriter, r *http.Request) {
	cat, err := h.svc.Get(r.Context())
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, cat)
}
