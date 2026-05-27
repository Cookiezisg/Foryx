// Package handlers — per-resource HTTP handlers, attached to mux via Register.
//
// Package handlers — 按资源组织的 HTTP handler,通过 Register 挂到 mux。
package handlers

import (
	"net/http"

	responsehttpapi "github.com/sunweilin/forgify/backend/internal/transport/httpapi/response"
)

// HealthHandler serves /api/v1/health for boot-readiness probing.
//
// HealthHandler 提供 /api/v1/health 给启动就绪探测。
type HealthHandler struct{}

func NewHealthHandler() *HealthHandler {
	return &HealthHandler{}
}

func (h *HealthHandler) Register(mux Registrar) {
	mux.HandleFunc("GET /api/v1/health", h.Get)
}

func (h *HealthHandler) Get(w http.ResponseWriter, _ *http.Request) {
	responsehttpapi.Success(w, http.StatusOK, map[string]string{"status": "ok"})
}
