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

// SandboxHandler serves /api/v1/sandbox/* + per-conversation scratch env routes.
//
// SandboxHandler 提供 /api/v1/sandbox/* + per-conversation scratch env 路由。
type SandboxHandler struct {
	svc *sandboxapp.Service
	log *zap.Logger
}

func NewSandboxHandler(svc *sandboxapp.Service, log *zap.Logger) *SandboxHandler {
	return &SandboxHandler{svc: svc, log: log}
}

func (h *SandboxHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/sandbox/runtimes", h.ListRuntimes)
	mux.HandleFunc("GET /api/v1/sandbox/envs", h.ListEnvs)
	mux.HandleFunc("GET /api/v1/sandbox/envs/{id}", h.GetEnv)
	mux.HandleFunc("GET /api/v1/sandbox/disk-usage", h.DiskUsage)
	mux.HandleFunc("GET /api/v1/sandbox/bootstrap-status", h.BootstrapStatus)
	mux.HandleFunc("GET /api/v1/conversations/{id}/sandbox-envs", h.ListConvEnvs)
	mux.HandleFunc("POST /api/v1/sandbox/envs/{idAction}", h.envAction)
	mux.HandleFunc("DELETE /api/v1/sandbox/envs/{id}", h.DestroyEnv)
	mux.HandleFunc("POST /api/v1/sandbox/runtimes/{idAction}", h.runtimeAction)
	mux.HandleFunc("POST /api/v1/sandbox/{action}", h.sandboxAction)
	mux.HandleFunc("POST /api/v1/conversations/{id}/sandbox-envs/{kindAction}", h.convEnvKindAction)
	mux.HandleFunc("POST /api/v1/conversations/{id}/sandbox-envs:reset-all", h.convEnvsAction)
}

func (h *SandboxHandler) ListRuntimes(w http.ResponseWriter, r *http.Request) {
	rows, err := h.svc.ListRuntimes(r.Context())
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, rows)
}

// validOwnerKinds whitelists OwnerKind inputs; unknown values return 400.
//
// validOwnerKinds 白名单,非法 OwnerKind 返 400 避免空 list 误读为"没数据"。
var validOwnerKinds = map[string]bool{
	sandboxdomain.OwnerKindFunction:     true,
	sandboxdomain.OwnerKindHandler:      true,
	sandboxdomain.OwnerKindMCP:          true,
	sandboxdomain.OwnerKindSkill:        true,
	sandboxdomain.OwnerKindConversation: true,
}

// ListEnvs requires ownerKind; empty or non-whitelisted returns 400.
//
// ListEnvs 要求 ownerKind;空或非白名单返 400。
func (h *SandboxHandler) ListEnvs(w http.ResponseWriter, r *http.Request) {
	ownerKind := r.URL.Query().Get("ownerKind")
	if ownerKind == "" {
		responsehttpapi.Error(w, http.StatusBadRequest, "OWNER_KIND_REQUIRED",
			"ownerKind query parameter is required", nil)
		return
	}
	if !validOwnerKinds[ownerKind] {
		responsehttpapi.Error(w, http.StatusBadRequest, "INVALID_OWNER_KIND",
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

func (h *SandboxHandler) GetEnv(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	env, err := h.svc.GetEnv(r.Context(), id)
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, env)
}

func (h *SandboxHandler) DiskUsage(w http.ResponseWriter, r *http.Request) {
	total, err := h.svc.TotalDiskUsage(r.Context())
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, map[string]int64{"totalBytes": total})
}

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

// ListConvEnvs filters conversation-kind envs by ownerID prefix "<convID>_".
//
// ListConvEnvs 按 ownerID 前缀 "<convID>_" 过滤 conversation-kind env。
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

func (h *SandboxHandler) envAction(w http.ResponseWriter, r *http.Request) {
	id, action := splitAction(r.PathValue("idAction"))
	if action != "destroy" {
		responsehttpapi.Error(w, http.StatusNotFound, "UNKNOWN_ACTION",
			"unknown env action: "+action, nil)
		return
	}
	h.destroyEnv(w, r, id)
}

// DestroyEnv handles `DELETE /api/v1/sandbox/envs/{id}` — RESTful alias for
// the `POST /envs/{id}:destroy` action. Same body, same response.
//
// DestroyEnv 是 `DELETE /api/v1/sandbox/envs/{id}` 的 RESTful 别名,
// 走与 `:destroy` action 完全一样的逻辑。
func (h *SandboxHandler) DestroyEnv(w http.ResponseWriter, r *http.Request) {
	h.destroyEnv(w, r, r.PathValue("id"))
}

func (h *SandboxHandler) destroyEnv(w http.ResponseWriter, r *http.Request, id string) {
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

// sandboxAction dispatches :gc / :retry-bootstrap / runtimes:install.
//
// sandboxAction 分派 :gc / :retry-bootstrap / runtimes:install。
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
	rt, err := h.svc.EnsureRuntime(r.Context(),
		sandboxdomain.RuntimeSpec{Kind: req.Kind, Version: req.Version}, nil)
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, rt)
}

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
		ID:   convID + "_" + kind,
	}
	if err := h.svc.Destroy(r.Context(), owner); err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.NoContent(w)
}

func (h *SandboxHandler) convEnvsAction(w http.ResponseWriter, r *http.Request) {
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

// splitAction parses "<id>:<action>"; no colon returns ("", input).
//
// splitAction 解析 "<id>:<action>";无冒号返 ("", input)。
func splitAction(idAction string) (id, action string) {
	if i := strings.LastIndexByte(idAction, ':'); i >= 0 {
		return idAction[:i], idAction[i+1:]
	}
	return "", idAction
}

