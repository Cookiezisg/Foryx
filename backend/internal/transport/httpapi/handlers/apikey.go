// apikey.go — HTTP handler for /api/v1/api-keys/*. Thin: decode JSON →
// call apikeyapp.Service → envelope via response package. No business
// logic here.
//
// apikey.go — /api/v1/api-keys/* 的 HTTP handler。薄层：解 JSON →
// 调 apikeyapp.Service → 通过 response 包输出 envelope。不含业务逻辑。

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

// APIKeyHandler serves the 5 /api/v1/api-keys/* endpoints. Holds a
// Service pointer; routes are declarative in Register.
//
// APIKeyHandler 提供 /api/v1/api-keys/* 的 5 个端点。持有 Service 指针；
// 路由在 Register 中声明。
type APIKeyHandler struct {
	svc *apikeyapp.Service
	log *zap.Logger
}

// NewAPIKeyHandler wires the handler dependencies.
//
// NewAPIKeyHandler 装配 handler 依赖。
func NewAPIKeyHandler(svc *apikeyapp.Service, log *zap.Logger) *APIKeyHandler {
	return &APIKeyHandler{svc: svc, log: log}
}

// Register attaches apikey routes. The "POST /{idAction}" pattern
// catches `/{id}:test` because Go 1.22's {id} wildcard greedily matches
// any non-slash sequence, including colons — we then split `:test`
// in postOnID to dispatch actions.
//
// Register 挂载 apikey 路由。"POST /{idAction}" 模式能匹配 `/{id}:test`
// 因为 Go 1.22 的 {id} 通配符贪婪匹配任意非斜杠序列（含冒号）——在
// postOnID 内部再按 `:test` 拆分成 action 分派。
func (h *APIKeyHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/api-keys", h.Create)
	mux.HandleFunc("GET /api/v1/api-keys", h.List)
	mux.HandleFunc("PATCH /api/v1/api-keys/{id}", h.Update)
	mux.HandleFunc("DELETE /api/v1/api-keys/{id}", h.Delete)
	mux.HandleFunc("POST /api/v1/api-keys/{idAction}", h.postOnID)
}

// ---- request shapes / 请求形状 ----

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
}

// ---- endpoint handlers / 端点处理器 ----

// Create: POST /api/v1/api-keys → 201 with the created APIKey.
//
// Create：POST /api/v1/api-keys → 201 返回新建的 APIKey。
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

// List: GET /api/v1/api-keys?cursor=&limit=&provider= → 200 paged envelope.
//
// List：GET /api/v1/api-keys?cursor=&limit=&provider= → 200 分页 envelope。
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

// Update: PATCH /api/v1/api-keys/{id} → 200 with the updated APIKey.
//
// Update：PATCH /api/v1/api-keys/{id} → 200 返回更新后的 APIKey。
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
	})
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, k)
}

// Delete: DELETE /api/v1/api-keys/{id} → 204.
//
// Delete：DELETE /api/v1/api-keys/{id} → 204。
func (h *APIKeyHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := h.svc.Delete(r.Context(), id); err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.NoContent(w)
}

// postOnID dispatches POST requests that land on the `/{id}` segment —
// currently only `:test` is supported. Unknown actions → 404.
//
// postOnID 分派落在 `/{id}` 段上的 POST 请求——当前只支持 `:test`。
// 未知 action → 404。
func (h *APIKeyHandler) postOnID(w http.ResponseWriter, r *http.Request) {
	id, action, found := idAndAction(r, "idAction")
	if !found {
		// POST on bare /{id} has no semantics — spec reserves the `:action` form.
		// 裸 /{id} 上的 POST 无语义——规范保留 `:action` 形式。
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

// test: POST /api/v1/api-keys/{id}:test → 200 on OK, 422 on failed probe.
// 200 shape: {data: {ok, message, latencyMs, modelsFound}}.
// 422 shape: {error: {code: API_KEY_TEST_FAILED, message, details}}.
//
// test：POST /api/v1/api-keys/{id}:test → 连通 200，探测失败 422。
// 200：{data: {ok, message, latencyMs, modelsFound}}。
// 422：{error: {code: API_KEY_TEST_FAILED, message, details}}。
func (h *APIKeyHandler) test(w http.ResponseWriter, r *http.Request, id string) {
	res, err := h.svc.Test(r.Context(), id)
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	if !res.OK {
		// Non-OK outcome is a 422 (per N2: 422 = business-rule refusal).
		// Details carry the latency so the UI can show probe timing.
		//
		// 非 OK 结果是 422（N2：422 = 业务规则拒绝）。
		// details 带 latency，UI 可展示探测耗时。
		responsehttpapi.Error(w, http.StatusUnprocessableEntity,
			"API_KEY_TEST_FAILED", res.Message,
			map[string]any{"latencyMs": res.LatencyMs})
		return
	}
	// Model list must never be nil on the wire (JSON [] vs null).
	// 线上模型列表不能为 nil（JSON [] 与 null 区别）。
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

// decodeJSON reads JSON into v; returns a errorsdomain.ErrInvalidRequest-wrapped
// error on malformed input so errmap renders 400 INVALID_REQUEST uniformly.
//
// decodeJSON 把 JSON 读入 v；输入畸形时返回包裹 errorsdomain.ErrInvalidRequest
// 的错误，让 errmap 统一渲染为 400 INVALID_REQUEST。
func decodeJSON(r *http.Request, v any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("decode body: %w", joinInvalidRequest(err))
	}
	return nil
}

// joinInvalidRequest wraps err so errors.Is can match errorsdomain.ErrInvalidRequest.
//
// joinInvalidRequest 包装 err，让 errors.Is 能匹配到 errorsdomain.ErrInvalidRequest。
func joinInvalidRequest(err error) error {
	return errors.Join(err, errorsdomain.ErrInvalidRequest)
}
