// forge.go — HTTP handler for /api/v1/forges/*. Thin: decode → service → envelope.
//
// forge.go — /api/v1/forges/* 的 HTTP handler。薄层：解码 → service → envelope。
package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"

	"go.uber.org/zap"

	forgeapp "github.com/sunweilin/forgify/backend/internal/app/forge"
	forgedomain "github.com/sunweilin/forgify/backend/internal/domain/forge"
	paginationpkg "github.com/sunweilin/forgify/backend/internal/pkg/pagination"
	responsehttpapi "github.com/sunweilin/forgify/backend/internal/transport/httpapi/response"
)

// ForgeHandler serves the /api/v1/forges/* endpoints.
//
// ForgeHandler 提供 /api/v1/forges/* 端点。
type ForgeHandler struct {
	svc *forgeapp.Service
	log *zap.Logger
}

// NewForgeHandler wires handler dependencies.
//
// NewForgeHandler 装配 handler 依赖。
func NewForgeHandler(svc *forgeapp.Service, log *zap.Logger) *ForgeHandler {
	return &ForgeHandler{svc: svc, log: log}
}

// Register attaches all tool routes to mux.
//
// Register 把所有工具路由挂载到 mux。
func (h *ForgeHandler) Register(mux *http.ServeMux) {
	// Collection
	mux.HandleFunc("POST /api/v1/forges", h.Create)
	mux.HandleFunc("GET /api/v1/forges", h.List)
	mux.HandleFunc("POST /api/v1/forges:import", h.Import)

	// Resource
	mux.HandleFunc("GET /api/v1/forges/{id}", h.Get)
	mux.HandleFunc("PATCH /api/v1/forges/{id}", h.Update)
	mux.HandleFunc("DELETE /api/v1/forges/{id}", h.Delete)

	// Resource actions (:run, :export, :revert, :test, :generate-test-cases)
	mux.HandleFunc("POST /api/v1/forges/{idAction}", h.postOnForge)

	// Versions
	mux.HandleFunc("GET /api/v1/forges/{id}/versions", h.ListVersions)
	mux.HandleFunc("GET /api/v1/forges/{id}/versions/{version}", h.GetVersion)

	// Pending
	mux.HandleFunc("GET /api/v1/forges/{id}/pending", h.GetPending)
	mux.HandleFunc("POST /api/v1/forges/{id}/pending:accept", h.AcceptPending)
	mux.HandleFunc("POST /api/v1/forges/{id}/pending:reject", h.RejectPending)

	// Test cases
	mux.HandleFunc("GET /api/v1/forges/{id}/test-cases", h.ListTestCases)
	mux.HandleFunc("POST /api/v1/forges/{id}/test-cases", h.CreateTestCase)
	mux.HandleFunc("DELETE /api/v1/forges/{id}/test-cases/{tcId}", h.DeleteTestCase)
	mux.HandleFunc("POST /api/v1/forges/{id}/test-cases/{tcIdAction}", h.postOnTestCase)

	// Executions (unified run + test history)
	mux.HandleFunc("GET /api/v1/forges/{id}/executions", h.ListExecutions)
}

// ── CRUD ──────────────────────────────────────────────────────────────────────

func (h *ForgeHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string   `json:"name"`
		Description string   `json:"description"`
		Code        string   `json:"code"`
		Tags        []string `json:"tags"`
	}
	if err := decodeJSON(r, &req); err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	t, err := h.svc.Create(r.Context(), forgeapp.CreateInput{
		Name: req.Name, Description: req.Description,
		Code: req.Code, Tags: req.Tags,
	})
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Created(w, t)
}

func (h *ForgeHandler) List(w http.ResponseWriter, r *http.Request) {
	p, err := paginationpkg.Parse(r)
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	items, next, err := h.svc.List(r.Context(), forgedomain.ListFilter{Cursor: p.Cursor, Limit: p.Limit})
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Paged(w, items, next, next != "")
}

func (h *ForgeHandler) Get(w http.ResponseWriter, r *http.Request) {
	t, err := h.svc.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, t)
}

func (h *ForgeHandler) Update(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        *string   `json:"name"`
		Description *string   `json:"description"`
		Tags        *[]string `json:"tags"`
		Code        *string   `json:"code"`
	}
	if err := decodeJSON(r, &req); err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	t, err := h.svc.Update(r.Context(), r.PathValue("id"), forgeapp.UpdateInput{
		Name: req.Name, Description: req.Description, Tags: req.Tags, Code: req.Code,
	})
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, t)
}

func (h *ForgeHandler) Delete(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.Delete(r.Context(), r.PathValue("id")); err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.NoContent(w)
}

// ── Import / Export ───────────────────────────────────────────────────────────

func (h *ForgeHandler) Import(w http.ResponseWriter, r *http.Request) {
	var data json.RawMessage
	if err := decodeJSON(r, &data); err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	t, err := h.svc.Import(r.Context(), []byte(data))
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Created(w, t)
}

// ── Resource action dispatcher ────────────────────────────────────────────────

// postOnForge dispatches POST /api/v1/forges/{idAction} based on the action suffix.
//
// postOnForge 按 action 后缀分派 POST /api/v1/forges/{idAction}。
func (h *ForgeHandler) postOnForge(w http.ResponseWriter, r *http.Request) {
	id, action, ok := idAndAction(r, "idAction")
	if !ok {
		responsehttpapi.Error(w, http.StatusNotFound, "NOT_FOUND", "unknown action", nil)
		return
	}
	switch action {
	case "run":
		h.Run(w, r, id)
	case "export":
		h.Export(w, r, id)
	case "revert":
		h.Revert(w, r, id)
	case "test":
		h.RunAllTests(w, r, id)
	case "generate-test-cases":
		h.GenerateTestCases(w, r, id)
	default:
		responsehttpapi.Error(w, http.StatusNotFound, "NOT_FOUND", "unknown action: "+action, nil)
	}
}

func (h *ForgeHandler) Run(w http.ResponseWriter, r *http.Request, id string) {
	var req struct {
		Input map[string]any `json:"input"`
	}
	if err := decodeJSON(r, &req); err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	result, err := h.svc.RunForge(r.Context(), id, req.Input)
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, result)
}

func (h *ForgeHandler) Export(w http.ResponseWriter, r *http.Request, id string) {
	data, err := h.svc.Export(r.Context(), id)
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	// Late write error means client disconnected mid-response; status
	// already sent, intentionally ignored.
	// 写出错通常是客户端中途断开，状态码已发出无可挽回，故意忽略。
	_, _ = w.Write(data)
}

func (h *ForgeHandler) Revert(w http.ResponseWriter, r *http.Request, id string) {
	var req struct {
		Version int `json:"version"`
	}
	if err := decodeJSON(r, &req); err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	t, err := h.svc.RevertToVersion(r.Context(), id, req.Version)
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, t)
}

func (h *ForgeHandler) RunAllTests(w http.ResponseWriter, r *http.Request, id string) {
	results, err := h.svc.RunAllTests(r.Context(), id)
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	total, passed := len(results), 0
	for _, r := range results {
		if r.Pass != nil && *r.Pass {
			passed++
		}
	}
	responsehttpapi.Success(w, http.StatusOK, map[string]any{
		"total": total, "passed": passed, "failed": total - passed, "results": results,
	})
}

// GenerateTestCases returns AI-generated test cases as a single JSON batch.
//
// GenerateTestCases 一次性返回 AI 生成的测试用例（JSON）。
func (h *ForgeHandler) GenerateTestCases(w http.ResponseWriter, r *http.Request, id string) {
	count := 5
	if s := r.URL.Query().Get("count"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 && n <= 20 {
			count = n
		}
	}
	result, err := h.svc.GenerateTestCases(r.Context(), id, count)
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, result)
}

// ── Versions ──────────────────────────────────────────────────────────────────

func (h *ForgeHandler) ListVersions(w http.ResponseWriter, r *http.Request) {
	versions, err := h.svc.ListVersions(r.Context(), r.PathValue("id"))
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, versions)
}

func (h *ForgeHandler) GetVersion(w http.ResponseWriter, r *http.Request) {
	v, err := strconv.Atoi(r.PathValue("version"))
	if err != nil {
		responsehttpapi.Error(w, http.StatusBadRequest, "INVALID_REQUEST", "version must be an integer", nil)
		return
	}
	version, err := h.svc.GetVersion(r.Context(), r.PathValue("id"), v)
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, version)
}

// ── Pending ───────────────────────────────────────────────────────────────────

func (h *ForgeHandler) GetPending(w http.ResponseWriter, r *http.Request) {
	pending, err := h.svc.GetActivePending(r.Context(), r.PathValue("id"))
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, pending)
}

func (h *ForgeHandler) AcceptPending(w http.ResponseWriter, r *http.Request) {
	t, err := h.svc.AcceptPending(r.Context(), r.PathValue("id"))
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, t)
}

func (h *ForgeHandler) RejectPending(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.RejectPending(r.Context(), r.PathValue("id")); err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.NoContent(w)
}

// ── Test cases ────────────────────────────────────────────────────────────────

func (h *ForgeHandler) ListTestCases(w http.ResponseWriter, r *http.Request) {
	cases, err := h.svc.ListTestCases(r.Context(), r.PathValue("id"))
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, cases)
}

func (h *ForgeHandler) CreateTestCase(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name           string `json:"name"`
		InputData      string `json:"inputData"`
		ExpectedOutput string `json:"expectedOutput"`
	}
	if err := decodeJSON(r, &req); err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	tc, err := h.svc.CreateTestCase(r.Context(), r.PathValue("id"), forgeapp.TestCaseInput{
		Name: req.Name, InputData: req.InputData, ExpectedOutput: req.ExpectedOutput,
	})
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Created(w, tc)
}

func (h *ForgeHandler) DeleteTestCase(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.DeleteTestCase(r.Context(), r.PathValue("tcId")); err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.NoContent(w)
}

// postOnTestCase dispatches POST /api/v1/forges/{id}/test-cases/{tcIdAction}.
//
// postOnTestCase 分派 POST /api/v1/forges/{id}/test-cases/{tcIdAction}。
func (h *ForgeHandler) postOnTestCase(w http.ResponseWriter, r *http.Request) {
	tcID, action, ok := idAndAction(r, "tcIdAction")
	if !ok || action != "run" {
		responsehttpapi.Error(w, http.StatusNotFound, "NOT_FOUND", "unknown action", nil)
		return
	}
	result, err := h.svc.RunTestCase(r.Context(), tcID, "")
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, result)
}

// ── Executions (unified run + test history) ───────────────────────────────────

// ListExecutions returns a cursor-paginated page of executions for a forge.
// Supports filtering by kind (run|test) and batchId (single test batch).
// Single endpoint replaces the previous /run-history + /test-history split.
//
// ListExecutions 返回 forge 执行记录的 cursor 分页结果。支持按 kind（run|test）
// 和 batchId（单次 test 批次）过滤。单端点取代原 /run-history + /test-history。
func (h *ForgeHandler) ListExecutions(w http.ResponseWriter, r *http.Request) {
	p, err := paginationpkg.Parse(r)
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	filter := forgedomain.ExecutionFilter{
		ForgeID: r.PathValue("id"),
		Kind:    r.URL.Query().Get("kind"),
		BatchID: r.URL.Query().Get("batchId"),
		Cursor:  p.Cursor,
		Limit:   p.Limit,
	}
	items, next, err := h.svc.ListExecutions(r.Context(), filter)
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Paged(w, items, next, next != "")
}
