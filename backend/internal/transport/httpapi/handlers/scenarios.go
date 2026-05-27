package handlers

import (
	"net/http"

	modeldomain "github.com/sunweilin/forgify/backend/internal/domain/model"
	responsehttpapi "github.com/sunweilin/forgify/backend/internal/transport/httpapi/response"
)

// ScenariosHandler serves GET /api/v1/scenarios from the model whitelist.
// Static metadata; lives outside RequireUser so onboarding can render the
// Model tab's scenario set before any user exists.
//
// ScenariosHandler 提供 GET /api/v1/scenarios,数据源是 model 白名单。
// 静态元数据,exempt 自 RequireUser,让 onboarding 在 user 创建前可读。
type ScenariosHandler struct{}

func NewScenariosHandler() *ScenariosHandler {
	return &ScenariosHandler{}
}

func (h *ScenariosHandler) Register(mux Registrar) {
	mux.HandleFunc("GET /api/v1/scenarios", h.List)
}

type scenarioInfo struct {
	Name string `json:"name"`
}

// List returns the full scenario whitelist; no pagination, no filter.
//
// List 返完整 scenario 白名单;不分页、不过滤。
func (h *ScenariosHandler) List(w http.ResponseWriter, r *http.Request) {
	names := modeldomain.ListScenarios()
	out := make([]scenarioInfo, 0, len(names))
	for _, n := range names {
		out = append(out, scenarioInfo{Name: n})
	}
	responsehttpapi.Success(w, http.StatusOK, out)
}
