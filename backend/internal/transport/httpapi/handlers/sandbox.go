// sandbox.go — HTTP handler for /api/v1/sandbox/* + the conversation
// scratch-env sub-routes under /api/v1/conversations/{id}/sandbox-envs/*.
// Thin: decode → service → envelope.
//
// Endpoints (per sandbox.md §11):
//
//	GET  /api/v1/sandbox/runtimes                            list installed runtimes
//	GET  /api/v1/sandbox/envs?ownerKind=...                  list envs (filtered)
//	GET  /api/v1/sandbox/envs/{id}                           single env detail
//	GET  /api/v1/sandbox/disk-usage                          total bytes
//	POST /api/v1/sandbox/envs/{id}:destroy                   force delete env (debug)
//	POST /api/v1/sandbox/runtimes/{id}:destroy               force delete runtime
//	POST /api/v1/sandbox:gc?olderThanDays=N                  user-triggered GC
//	GET  /api/v1/sandbox/bootstrap-status                    {ok, error, miseBin}
//	POST /api/v1/sandbox:retry-bootstrap                     re-run Bootstrap
//	POST /api/v1/sandbox/runtimes:install                    explicit install (debug)
//
//	GET  /api/v1/conversations/{id}/sandbox-envs             list this conv's scratch envs
//	POST /api/v1/conversations/{id}/sandbox-envs/{kind}:reset  destroy one
//	POST /api/v1/conversations/{id}/sandbox-envs:reset-all   destroy all for conv
//
// sandbox.go ——/api/v1/sandbox/* + /api/v1/conversations/{id}/sandbox-envs/*
// 子路由的 HTTP handler。薄层：decode → service → envelope。

package handlers

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"

	sandboxapp "github.com/sunweilin/forgify/backend/internal/app/sandbox"
	sandboxdomain "github.com/sunweilin/forgify/backend/internal/domain/sandbox"
	responsehttpapi "github.com/sunweilin/forgify/backend/internal/transport/httpapi/response"
)

// SandboxHandler serves the /api/v1/sandbox/* + per-conversation scratch
// env routes.
//
// SandboxHandler 提供 /api/v1/sandbox/* + per-conversation scratch env 路由。
type SandboxHandler struct {
	svc *sandboxapp.Service
	log *zap.Logger
}

// NewSandboxHandler wires the handler dependencies.
//
// NewSandboxHandler 装配 handler 依赖。
func NewSandboxHandler(svc *sandboxapp.Service, log *zap.Logger) *SandboxHandler {
	return &SandboxHandler{svc: svc, log: log}
}

// Register attaches sandbox routes. Action-style routes (:destroy / :gc /
// :retry-bootstrap / :reset / :install) use a single trailing-segment
// dispatch — the standard library's mux supports {var} patterns but not
// arbitrary action suffixes, so we route the verb-bearing prefix and
// dispatch on the action inside the handler.
//
// Register 挂 sandbox 路由。:action 风格路由（:destroy / :gc /
// :retry-bootstrap / :reset / :install）用单尾段派发——stdlib mux 支持
// {var} 但不支持任意 action 后缀，所以路由带动词前缀，handler 内按 action
// 分派。
func (h *SandboxHandler) Register(mux *http.ServeMux) {
	// Read endpoints
	mux.HandleFunc("GET /api/v1/sandbox/runtimes", h.ListRuntimes)
	mux.HandleFunc("GET /api/v1/sandbox/envs", h.ListEnvs)
	mux.HandleFunc("GET /api/v1/sandbox/envs/{id}", h.GetEnv)
	mux.HandleFunc("GET /api/v1/sandbox/disk-usage", h.DiskUsage)
	mux.HandleFunc("GET /api/v1/sandbox/bootstrap-status", h.BootstrapStatus)
	mux.HandleFunc("GET /api/v1/conversations/{id}/sandbox-envs", h.ListConvEnvs)

	// :action endpoints (POST). Use a wildcard pattern then strings.Cut to
	// split id from action — keeps the route table compact.
	// :action 端点（POST）。用通配 + strings.Cut 拆 id/action——路由表紧凑。
	mux.HandleFunc("POST /api/v1/sandbox/envs/{idAction}", h.envAction)
	mux.HandleFunc("POST /api/v1/sandbox/runtimes/{idAction}", h.runtimeAction)
	mux.HandleFunc("POST /api/v1/sandbox/{action}", h.sandboxAction)
	mux.HandleFunc("POST /api/v1/conversations/{id}/sandbox-envs/{kindAction}", h.convEnvKindAction)
	mux.HandleFunc("POST /api/v1/conversations/{id}/sandbox-envs", h.convEnvsAction)
}

// ── Read endpoints ────────────────────────────────────────────────────

// ListRuntimes: GET /api/v1/sandbox/runtimes → 200 [{kind, version, ...}].
func (h *SandboxHandler) ListRuntimes(w http.ResponseWriter, r *http.Request) {
	rows, err := h.svc.ListRuntimes(r.Context())
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, rows)
}

// ListEnvs: GET /api/v1/sandbox/envs?ownerKind=mcp → 200 [{...}].
// Empty ownerKind returns 400 — caller must scope the query (otherwise
// the response is unbounded across all owner kinds).
//
// ListEnvs: GET /api/v1/sandbox/envs?ownerKind=mcp → 200。空 ownerKind
// 返 400——调用方必须 scope 查询（否则跨所有 owner kind 无界）。
func (h *SandboxHandler) ListEnvs(w http.ResponseWriter, r *http.Request) {
	ownerKind := r.URL.Query().Get("ownerKind")
	if ownerKind == "" {
		responsehttpapi.Error(w, http.StatusBadRequest, "OWNER_KIND_REQUIRED",
			"ownerKind query parameter is required", nil)
		return
	}
	rows, err := h.svc.ListEnvs(r.Context(), ownerKind)
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, rows)
}

// GetEnv: GET /api/v1/sandbox/envs/{id} → 200 / 404.
func (h *SandboxHandler) GetEnv(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	env, err := h.svc.GetEnv(r.Context(), id)
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, env)
}

// DiskUsage: GET /api/v1/sandbox/disk-usage → 200 {totalBytes}.
func (h *SandboxHandler) DiskUsage(w http.ResponseWriter, r *http.Request) {
	total, err := h.svc.TotalDiskUsage(r.Context())
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, map[string]int64{"totalBytes": total})
}

// BootstrapStatus: GET /api/v1/sandbox/bootstrap-status → 200
// {ok: bool, error: string?, miseBin: string?}.
func (h *SandboxHandler) BootstrapStatus(w http.ResponseWriter, r *http.Request) {
	body := map[string]any{
		"ok":      h.svc.IsReady(),
		"miseBin": h.svc.MiseBin(),
	}
	if err := h.svc.BootstrapError(); err != nil {
		body["error"] = err.Error()
	}
	responsehttpapi.Success(w, http.StatusOK, body)
}

// ListConvEnvs: GET /api/v1/conversations/{id}/sandbox-envs → 200 [{...}].
// Filters all conversation-kind envs to those whose ownerID prefix matches
// "<convID>:" (matching sandbox.md §5 conversation owner-id convention
// "<conv_id>:<runtime_kind>").
//
// ListConvEnvs: GET /api/v1/conversations/{id}/sandbox-envs → 200。过滤所有
// conversation-kind env 找 ownerID 前缀 "<convID>:" 匹配的（按 sandbox.md
// §5 conversation owner-id 约定 "<conv_id>:<runtime_kind>"）。
func (h *SandboxHandler) ListConvEnvs(w http.ResponseWriter, r *http.Request) {
	convID := r.PathValue("id")
	all, err := h.svc.ListEnvs(r.Context(), sandboxdomain.OwnerKindConversation)
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	prefix := convID + ":"
	scoped := make([]*sandboxdomain.Env, 0)
	for _, e := range all {
		if strings.HasPrefix(e.OwnerID, prefix) {
			scoped = append(scoped, e)
		}
	}
	responsehttpapi.Success(w, http.StatusOK, scoped)
}

// ── :action dispatchers ──────────────────────────────────────────────

// envAction handles POST /api/v1/sandbox/envs/{id}:destroy.
//
// envAction 处理 POST /api/v1/sandbox/envs/{id}:destroy。
func (h *SandboxHandler) envAction(w http.ResponseWriter, r *http.Request) {
	id, action := splitAction(r.PathValue("idAction"))
	if action != "destroy" {
		responsehttpapi.Error(w, http.StatusNotFound, "UNKNOWN_ACTION",
			"unknown env action: "+action, nil)
		return
	}
	env, err := h.svc.GetEnv(r.Context(), id)
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	owner := sandboxdomain.Owner{Kind: env.OwnerKind, ID: env.OwnerID}
	if err := h.svc.Destroy(r.Context(), owner); err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.NoContent(w)
}

// runtimeAction handles POST /api/v1/sandbox/runtimes/{id}:destroy.
//
// runtimeAction 处理 POST /api/v1/sandbox/runtimes/{id}:destroy。
func (h *SandboxHandler) runtimeAction(w http.ResponseWriter, r *http.Request) {
	id, action := splitAction(r.PathValue("idAction"))
	if action != "destroy" {
		responsehttpapi.Error(w, http.StatusNotFound, "UNKNOWN_ACTION",
			"unknown runtime action: "+action, nil)
		return
	}
	if err := h.svc.DeleteRuntime(r.Context(), id); err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.NoContent(w)
}

// sandboxAction handles POST /api/v1/sandbox/{action} where action is
// one of "gc" / "retry-bootstrap" / "runtimes:install" (the last keeps
// its colon since it's namespaced under runtimes/).
//
// sandboxAction 处理 POST /api/v1/sandbox/{action}，action 取
// "gc" / "retry-bootstrap" / "runtimes:install"。
func (h *SandboxHandler) sandboxAction(w http.ResponseWriter, r *http.Request) {
	action := r.PathValue("action")
	switch {
	case action == ":gc":
		h.gc(w, r)
	case action == ":retry-bootstrap":
		h.retryBootstrap(w, r)
	case action == "runtimes:install":
		h.installRuntime(w, r)
	default:
		responsehttpapi.Error(w, http.StatusNotFound, "UNKNOWN_ACTION",
			"unknown sandbox action: "+action, nil)
	}
}

func (h *SandboxHandler) gc(w http.ResponseWriter, r *http.Request) {
	days := 30
	if v := r.URL.Query().Get("olderThanDays"); v != "" {
		if d, err := strconv.Atoi(v); err == nil && d > 0 {
			days = d
		}
	}
	removed, err := h.svc.GC(r.Context(), time.Duration(days)*24*time.Hour)
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, map[string]any{
		"removed":       removed,
		"olderThanDays": days,
	})
}

func (h *SandboxHandler) retryBootstrap(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.RetryBootstrap(r.Context()); err != nil {
		// Don't 500 — RetryBootstrap's failure is observable state, not an
		// HTTP error. Return the new status with the failure reason; UI
		// surfaces it as the bootstrap-status banner.
		//
		// 不 500——RetryBootstrap 失败是可观察状态非 HTTP 错。返新状态带
		// 失败原因；UI 暴露为 bootstrap-status banner。
		responsehttpapi.Success(w, http.StatusOK, map[string]any{
			"ok":    false,
			"error": err.Error(),
		})
		return
	}
	responsehttpapi.Success(w, http.StatusOK, map[string]any{"ok": true})
}

type installRuntimeRequest struct {
	Kind    string `json:"kind"`
	Version string `json:"version"`
}

func (h *SandboxHandler) installRuntime(w http.ResponseWriter, r *http.Request) {
	var req installRuntimeRequest
	if err := decodeJSON(r, &req); err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	if req.Kind == "" {
		responsehttpapi.Error(w, http.StatusBadRequest, "KIND_REQUIRED",
			"kind is required", nil)
		return
	}
	rt, err := h.svc.EnsureRuntime(r.Context(),
		sandboxdomain.RuntimeSpec{Kind: req.Kind, Version: req.Version}, nil)
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, rt)
}

// convEnvKindAction handles POST /api/v1/conversations/{id}/sandbox-envs/{kind}:reset.
//
// convEnvKindAction 处理 POST /api/v1/conversations/{id}/sandbox-envs/{kind}:reset。
func (h *SandboxHandler) convEnvKindAction(w http.ResponseWriter, r *http.Request) {
	convID := r.PathValue("id")
	kind, action := splitAction(r.PathValue("kindAction"))
	if action != "reset" {
		responsehttpapi.Error(w, http.StatusNotFound, "UNKNOWN_ACTION",
			"unknown conv-env action: "+action, nil)
		return
	}
	owner := sandboxdomain.Owner{
		Kind: sandboxdomain.OwnerKindConversation,
		ID:   convID + ":" + kind,
	}
	if err := h.svc.Destroy(r.Context(), owner); err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.NoContent(w)
}

// convEnvsAction handles POST /api/v1/conversations/{id}/sandbox-envs:reset-all.
//
// convEnvsAction 处理 POST /api/v1/conversations/{id}/sandbox-envs:reset-all。
func (h *SandboxHandler) convEnvsAction(w http.ResponseWriter, r *http.Request) {
	convID := r.PathValue("id")
	all, err := h.svc.ListEnvs(r.Context(), sandboxdomain.OwnerKindConversation)
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	prefix := convID + ":"
	removed := 0
	for _, e := range all {
		if !strings.HasPrefix(e.OwnerID, prefix) {
			continue
		}
		owner := sandboxdomain.Owner{Kind: e.OwnerKind, ID: e.OwnerID}
		if err := h.svc.Destroy(r.Context(), owner); err != nil {
			responsehttpapi.FromDomainError(w, h.log, err)
			return
		}
		removed++
	}
	responsehttpapi.Success(w, http.StatusOK, map[string]int{"removed": removed})
}

// splitAction parses "<id>:<action>" path tail. Returns ("", input) if
// no colon — used to dispatch the action; missing-colon falls through
// to the "unknown action" branch.
//
// splitAction 解析 "<id>:<action>" 路径尾部。无冒号返 ("", input)——用于
// action 派发；漏冒号落到 "unknown action" 分支。
func splitAction(idAction string) (id, action string) {
	if i := strings.LastIndexByte(idAction, ':'); i >= 0 {
		return idAction[:i], idAction[i+1:]
	}
	return "", idAction
}

