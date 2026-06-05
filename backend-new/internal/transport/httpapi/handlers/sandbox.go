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

// SandboxHandler serves /api/v1/sandbox/* (runtime + env management, disk audit,
// bootstrap status, GC) plus per-conversation scratch-env routes.
//
// SandboxHandler 提供 /api/v1/sandbox/*（runtime + env 管理、磁盘审计、bootstrap 状态、
// GC）及 per-conversation scratch-env 路由。
type SandboxHandler struct {
	svc *sandboxapp.Service
	log *zap.Logger
}

// NewSandboxHandler constructs the handler.
//
// NewSandboxHandler 构造 handler。
func NewSandboxHandler(svc *sandboxapp.Service, log *zap.Logger) *SandboxHandler {
	if log == nil {
		log = zap.NewNop()
	}
	return &SandboxHandler{svc: svc, log: log.Named("handlers.sandbox")}
}

// Register wires the endpoints onto mux.
//
// Register 把端点挂到 mux。
func (h *SandboxHandler) Register(mux Registrar) {
	mux.HandleFunc("GET /api/v1/sandbox/runtimes", h.ListRuntimes)
	mux.HandleFunc("POST /api/v1/sandbox/runtimes", h.InstallRuntime)
	mux.HandleFunc("DELETE /api/v1/sandbox/runtimes/{id}", h.DeleteRuntime)
	mux.HandleFunc("GET /api/v1/sandbox/envs", h.ListEnvs)
	mux.HandleFunc("GET /api/v1/sandbox/envs/{id}", h.GetEnv)
	mux.HandleFunc("DELETE /api/v1/sandbox/envs/{id}", h.DestroyEnv)
	mux.HandleFunc("GET /api/v1/sandbox/disk-usage", h.DiskUsage)
	mux.HandleFunc("GET /api/v1/sandbox/bootstrap-status", h.BootstrapStatus)
	mux.HandleFunc("POST /api/v1/sandbox:gc", h.GC)
	mux.HandleFunc("POST /api/v1/sandbox:retry-bootstrap", h.RetryBootstrap)
	mux.HandleFunc("GET /api/v1/conversations/{id}/sandbox-envs", h.ListConvEnvs)
	mux.HandleFunc("POST /api/v1/conversations/{id}/sandbox-envs/{kindAction}", h.ConvEnvReset)
	mux.HandleFunc("POST /api/v1/conversations/{id}/sandbox-envs:reset-all", h.ConvEnvsResetAll)
}

// ListRuntimes handles GET /api/v1/sandbox/runtimes.
//
// ListRuntimes 处理 GET /api/v1/sandbox/runtimes。
func (h *SandboxHandler) ListRuntimes(w http.ResponseWriter, r *http.Request) {
	rows, err := h.svc.ListRuntimes(r.Context())
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, rows)
}

type installRuntimeRequest struct {
	Kind    string `json:"kind"`
	Version string `json:"version"`
}

// InstallRuntime handles POST /api/v1/sandbox/runtimes — lazily install (kind, version).
//
// InstallRuntime 处理 POST /api/v1/sandbox/runtimes —— 懒装 (kind, version)。
func (h *SandboxHandler) InstallRuntime(w http.ResponseWriter, r *http.Request) {
	var req installRuntimeRequest
	if err := decodeJSON(r, &req); err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	rt, err := h.svc.EnsureRuntime(r.Context(),
		sandboxdomain.RuntimeSpec{Kind: req.Kind, Version: req.Version}, nil)
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Created(w, rt)
}

// DeleteRuntime handles DELETE /api/v1/sandbox/runtimes/{id}; 409 if any env refs it.
//
// DeleteRuntime 处理 DELETE /api/v1/sandbox/runtimes/{id}；有 env 引用则 409。
func (h *SandboxHandler) DeleteRuntime(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.DeleteRuntime(r.Context(), r.PathValue("id")); err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.NoContent(w)
}

// validOwnerKinds whitelists the ownerKind query value; an unknown value returns
// 400 so an empty list is never misread as "no data".
//
// validOwnerKinds 白名单 ownerKind，非法值返 400，避免空 list 被误读为"没数据"。
var validOwnerKinds = map[string]bool{
	sandboxdomain.OwnerKindFunction:     true,
	sandboxdomain.OwnerKindHandler:      true,
	sandboxdomain.OwnerKindMCP:          true,
	sandboxdomain.OwnerKindSkill:        true,
	sandboxdomain.OwnerKindConversation: true,
}

// ListEnvs handles GET /api/v1/sandbox/envs?ownerKind=... (ownerKind required).
//
// ListEnvs 处理 GET /api/v1/sandbox/envs?ownerKind=...（ownerKind 必填）。
func (h *SandboxHandler) ListEnvs(w http.ResponseWriter, r *http.Request) {
	ownerKind := r.URL.Query().Get("ownerKind")
	if ownerKind == "" {
		responsehttpapi.Error(w, http.StatusBadRequest, "SANDBOX_OWNER_KIND_REQUIRED",
			"ownerKind query parameter is required", nil)
		return
	}
	if !validOwnerKinds[ownerKind] {
		responsehttpapi.Error(w, http.StatusBadRequest, "SANDBOX_INVALID_OWNER_KIND",
			"ownerKind must be one of: function, handler, mcp, skill, conversation", nil)
		return
	}
	rows, err := h.svc.ListEnvs(r.Context(), ownerKind)
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, rows)
}

// GetEnv handles GET /api/v1/sandbox/envs/{id}.
//
// GetEnv 处理 GET /api/v1/sandbox/envs/{id}。
func (h *SandboxHandler) GetEnv(w http.ResponseWriter, r *http.Request) {
	env, err := h.svc.GetEnv(r.Context(), r.PathValue("id"))
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, env)
}

// DestroyEnv handles DELETE /api/v1/sandbox/envs/{id} (DB row + on-disk dir).
//
// DestroyEnv 处理 DELETE /api/v1/sandbox/envs/{id}（DB 行 + 磁盘目录）。
func (h *SandboxHandler) DestroyEnv(w http.ResponseWriter, r *http.Request) {
	env, err := h.svc.GetEnv(r.Context(), r.PathValue("id"))
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

// DiskUsage handles GET /api/v1/sandbox/disk-usage.
//
// DiskUsage 处理 GET /api/v1/sandbox/disk-usage。
func (h *SandboxHandler) DiskUsage(w http.ResponseWriter, r *http.Request) {
	total, err := h.svc.TotalDiskUsage(r.Context())
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, map[string]int64{"totalBytes": total})
}

// BootstrapStatus handles GET /api/v1/sandbox/bootstrap-status.
//
// BootstrapStatus 处理 GET /api/v1/sandbox/bootstrap-status。
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

// GC handles POST /api/v1/sandbox:gc?olderThanDays=N (default 30).
//
// GC 处理 POST /api/v1/sandbox:gc?olderThanDays=N（默认 30）。
func (h *SandboxHandler) GC(w http.ResponseWriter, r *http.Request) {
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

// RetryBootstrap handles POST /api/v1/sandbox:retry-bootstrap; reports the new
// state in-band (a failed re-bootstrap is degraded mode, not an HTTP error).
//
// RetryBootstrap 处理 POST /api/v1/sandbox:retry-bootstrap；带内返回新状态（重 bootstrap
// 失败是 degraded 而非 HTTP 错误）。
func (h *SandboxHandler) RetryBootstrap(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.RetryBootstrap(r.Context()); err != nil {
		responsehttpapi.Success(w, http.StatusOK, map[string]any{
			"ok":    false,
			"error": err.Error(),
		})
		return
	}
	responsehttpapi.Success(w, http.StatusOK, map[string]any{"ok": true})
}

// ListConvEnvs handles GET /api/v1/conversations/{id}/sandbox-envs — the
// conversation's scratch envs, filtered by ownerID prefix "<convID>_".
//
// ListConvEnvs 处理 GET /api/v1/conversations/{id}/sandbox-envs —— 该对话的 scratch env，
// 按 ownerID 前缀 "<convID>_" 过滤。
func (h *SandboxHandler) ListConvEnvs(w http.ResponseWriter, r *http.Request) {
	convID := r.PathValue("id")
	all, err := h.svc.ListEnvs(r.Context(), sandboxdomain.OwnerKindConversation)
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	prefix := convID + "_"
	scoped := make([]*sandboxdomain.Env, 0)
	for _, e := range all {
		if strings.HasPrefix(e.OwnerID, prefix) {
			scoped = append(scoped, e)
		}
	}
	responsehttpapi.Success(w, http.StatusOK, scoped)
}

// ConvEnvReset handles POST /api/v1/conversations/{id}/sandbox-envs/{kind}:reset
// — destroy the conversation's per-kind scratch env.
//
// ConvEnvReset 处理 POST /api/v1/conversations/{id}/sandbox-envs/{kind}:reset ——
// 销毁该对话某 kind 的 scratch env。
func (h *SandboxHandler) ConvEnvReset(w http.ResponseWriter, r *http.Request) {
	convID := r.PathValue("id")
	kind, action, _ := idAndAction(r, "kindAction")
	if action != "reset" {
		responsehttpapi.Error(w, http.StatusNotFound, "SANDBOX_UNKNOWN_ACTION",
			"unknown conv-env action: "+action, nil)
		return
	}
	owner := sandboxdomain.Owner{
		Kind: sandboxdomain.OwnerKindConversation,
		ID:   convID + "_" + kind,
	}
	if err := h.svc.Destroy(r.Context(), owner); err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.NoContent(w)
}

// ConvEnvsResetAll handles POST /api/v1/conversations/{id}/sandbox-envs:reset-all
// — destroy every scratch env owned by the conversation.
//
// ConvEnvsResetAll 处理 POST /api/v1/conversations/{id}/sandbox-envs:reset-all ——
// 销毁该对话拥有的所有 scratch env。
func (h *SandboxHandler) ConvEnvsResetAll(w http.ResponseWriter, r *http.Request) {
	convID := r.PathValue("id")
	all, err := h.svc.ListEnvs(r.Context(), sandboxdomain.OwnerKindConversation)
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	prefix := convID + "_"
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
