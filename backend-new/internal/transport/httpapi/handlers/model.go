package handlers

import (
	"net/http"

	"go.uber.org/zap"

	modelapp "github.com/sunweilin/forgify/backend/internal/app/model"
	modeldomain "github.com/sunweilin/forgify/backend/internal/domain/model"
	responsehttpapi "github.com/sunweilin/forgify/backend/internal/transport/httpapi/response"
)

// ModelCapabilitiesHandler serves GET /api/v1/model-capabilities — the aggregated "what models can
// I use, and how is each configured" list, built from the current workspace's probed keys.
//
// ModelCapabilitiesHandler 提供 GET /api/v1/model-capabilities——从当前 workspace 已探测的 key
// 聚合的「我能用哪些模型、每个怎么配」列表。
type ModelCapabilitiesHandler struct {
	svc *modelapp.CapabilityService
	log *zap.Logger
}

func NewModelCapabilitiesHandler(svc *modelapp.CapabilityService, log *zap.Logger) *ModelCapabilitiesHandler {
	if log == nil {
		log = zap.NewNop()
	}
	return &ModelCapabilitiesHandler{svc: svc, log: log.Named("handlers.modelcap")}
}

func (h *ModelCapabilitiesHandler) Register(mux Registrar) {
	mux.HandleFunc("GET /api/v1/model-capabilities", h.List)
}

// List returns every usable (key, model) pair with its capability specs and native knobs.
//
// List 返回每个可用的 (key, model) 对及其能力规格与原生旋钮。
func (h *ModelCapabilitiesHandler) List(w http.ResponseWriter, r *http.Request) {
	items, err := h.svc.List(r.Context())
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, items)
}

// ScenariosHandler serves GET /api/v1/scenarios — the fixed scenario whitelist (dialogue/utility/
// agent). Static metadata, exempt from RequireWorkspace so onboarding can render the model-config
// tab before any workspace exists.
//
// ScenariosHandler 提供 GET /api/v1/scenarios——固定 scenario 白名单（dialogue/utility/agent）。
// 静态元数据，豁免 RequireWorkspace，使 onboarding 在任何 workspace 前可渲染模型配置页。
type ScenariosHandler struct{}

func NewScenariosHandler() *ScenariosHandler { return &ScenariosHandler{} }

func (h *ScenariosHandler) Register(mux Registrar) {
	mux.HandleFunc("GET /api/v1/scenarios", h.List)
}

type scenarioInfo struct {
	Name string `json:"name"`
}

// List returns the full scenario whitelist; no pagination, no filter.
//
// List 返完整 scenario 白名单；不分页、不过滤。
func (h *ScenariosHandler) List(w http.ResponseWriter, r *http.Request) {
	names := modeldomain.ListScenarios()
	out := make([]scenarioInfo, 0, len(names))
	for _, n := range names {
		out = append(out, scenarioInfo{Name: n})
	}
	responsehttpapi.Success(w, http.StatusOK, out)
}
