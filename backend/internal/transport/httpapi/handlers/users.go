package handlers

import (
	"net/http"

	"go.uber.org/zap"

	userapp "github.com/sunweilin/forgify/backend/internal/app/user"
	responsehttpapi "github.com/sunweilin/forgify/backend/internal/transport/httpapi/response"
)

// UsersHandler serves /api/v1/users — local profile management (no auth, just identity switching).
//
// UsersHandler 提供 /api/v1/users 端点——本地多 profile 管理（无 auth，仅身份切换）。
type UsersHandler struct {
	svc *userapp.Service
	log *zap.Logger
}

func NewUsersHandler(svc *userapp.Service, log *zap.Logger) *UsersHandler {
	if log == nil {
		log = zap.NewNop()
	}
	return &UsersHandler{svc: svc, log: log.Named("handlers.users")}
}

func (h *UsersHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/users", h.List)
	mux.HandleFunc("POST /api/v1/users", h.Create)
	mux.HandleFunc("GET /api/v1/users/{id}", h.Get)
	mux.HandleFunc("PATCH /api/v1/users/{id}", h.Update)
	mux.HandleFunc("DELETE /api/v1/users/{id}", h.Delete)
	mux.HandleFunc("POST /api/v1/users/{id}:activate", h.Activate)
}

type createUserRequest struct {
	Username    string `json:"username"`
	DisplayName string `json:"displayName"`
	AvatarColor string `json:"avatarColor"`
	Language    string `json:"language"`
}

type updateUserRequest struct {
	DisplayName *string `json:"displayName,omitempty"`
	AvatarColor *string `json:"avatarColor,omitempty"`
	Language    *string `json:"language,omitempty"`
}

func (h *UsersHandler) List(w http.ResponseWriter, r *http.Request) {
	rows, err := h.svc.List(r.Context())
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, rows)
}

func (h *UsersHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req createUserRequest
	if err := decodeJSON(r, &req); err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	u, err := h.svc.Create(r.Context(), userapp.CreateInput{
		Username:    req.Username,
		DisplayName: req.DisplayName,
		AvatarColor: req.AvatarColor,
		Language:    req.Language,
	})
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Created(w, u)
}

func (h *UsersHandler) Get(w http.ResponseWriter, r *http.Request) {
	u, err := h.svc.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, u)
}

func (h *UsersHandler) Update(w http.ResponseWriter, r *http.Request) {
	var req updateUserRequest
	if err := decodeJSON(r, &req); err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	u, err := h.svc.Update(r.Context(), r.PathValue("id"), userapp.UpdateInput{
		DisplayName: req.DisplayName,
		AvatarColor: req.AvatarColor,
		Language:    req.Language,
	})
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, u)
}

func (h *UsersHandler) Delete(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.Delete(r.Context(), r.PathValue("id")); err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.NoContent(w)
}

// Activate marks a user as most-recently-used; client uses this when switching profiles.
//
// Activate 标用户为最近使用；切换 profile 时客户端调。
func (h *UsersHandler) Activate(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := h.svc.TouchLastUsed(r.Context(), id); err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	u, err := h.svc.Get(r.Context(), id)
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, u)
}
