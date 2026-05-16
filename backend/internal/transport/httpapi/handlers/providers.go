package handlers

import (
	"net/http"

	apikeyapp "github.com/sunweilin/forgify/backend/internal/app/apikey"
	responsehttpapi "github.com/sunweilin/forgify/backend/internal/transport/httpapi/response"
)

// ProvidersHandler serves GET /api/v1/providers from the apikey registry.
//
// ProvidersHandler 提供 GET /api/v1/providers,数据源是 apikey registry。
type ProvidersHandler struct{}

func NewProvidersHandler() *ProvidersHandler {
	return &ProvidersHandler{}
}

func (h *ProvidersHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/providers", h.List)
}

type providerInfo struct {
	Name            string `json:"name"`
	DisplayName     string `json:"displayName"`
	Category        string `json:"category"`
	DefaultBaseURL  string `json:"defaultBaseUrl,omitempty"`
	BaseURLRequired bool   `json:"baseUrlRequired"`
}

// List supports optional ?category=llm|search filter; unknown = empty list.
//
// List 支持可选 ?category=llm|search 过滤;未知 category 返空。
func (h *ProvidersHandler) List(w http.ResponseWriter, r *http.Request) {
	wantCategory := r.URL.Query().Get("category")

	names := apikeyapp.ListProviders()
	out := make([]providerInfo, 0, len(names))
	for _, name := range names {
		meta, ok := apikeyapp.GetProviderMeta(name)
		if !ok {
			continue
		}
		if wantCategory != "" && string(meta.Category) != wantCategory {
			continue
		}
		out = append(out, providerInfo{
			Name:            meta.Name,
			DisplayName:     meta.DisplayName,
			Category:        string(meta.Category),
			DefaultBaseURL:  meta.DefaultBaseURL,
			BaseURLRequired: meta.BaseURLRequired,
		})
	}
	sortProviderInfos(out)
	responsehttpapi.Success(w, http.StatusOK, out)
}

func sortProviderInfos(s []providerInfo) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j-1].Name > s[j].Name; j-- {
			s[j-1], s[j] = s[j], s[j-1]
		}
	}
}
