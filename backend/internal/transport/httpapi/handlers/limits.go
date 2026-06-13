package handlers

import (
	"encoding/json"
	"io"
	"net/http"

	"go.uber.org/zap"

	settingsapp "github.com/sunweilin/forgify/backend/internal/app/settings"
	responsehttpapi "github.com/sunweilin/forgify/backend/internal/transport/httpapi/response"
)

// LimitsHandler serves the user-tunable operational ceilings (settings.json "limits"
// block): GET returns the live values, PATCH merges a partial update, persists and
// hot-swaps — consumers see new values on their next read, no restart.
//
// LimitsHandler 提供用户可调运行上限（settings.json "limits" 段）：GET 返活动值，PATCH
// 合并部分更新、持久化并热换——消费方下一次读取即见新值，无需重启。
type LimitsHandler struct {
	svc *settingsapp.Service
	log *zap.Logger
}

func NewLimitsHandler(svc *settingsapp.Service, log *zap.Logger) *LimitsHandler {
	if log == nil {
		log = zap.NewNop()
	}
	return &LimitsHandler{svc: svc, log: log.Named("handlers.limits")}
}

func (h *LimitsHandler) Register(mux Registrar) {
	mux.HandleFunc("GET /api/v1/limits", h.Get)
	mux.HandleFunc("PATCH /api/v1/limits", h.Patch)
}

func (h *LimitsHandler) Get(w http.ResponseWriter, r *http.Request) {
	responsehttpapi.Success(w, http.StatusOK, h.svc.Limits())
}

func (h *LimitsHandler) Patch(w http.ResponseWriter, r *http.Request) {
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 64<<10))
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, settingsapp.ErrLimitsInvalid)
		return
	}
	cur, err := h.svc.PatchLimits(json.RawMessage(raw))
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, cur)
}
