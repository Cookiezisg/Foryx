// Package user owns the local-profile CRUD Service.
//
// Package user 持本地多账号 profile 的 CRUD Service。
package user

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"go.uber.org/zap"

	userdomain "github.com/sunweilin/forgify/backend/internal/domain/user"
	idgenpkg "github.com/sunweilin/forgify/backend/internal/pkg/idgen"
)

// usernameRE constrains usernames to 1-32 [a-z0-9_-]; lowercased before save.
//
// usernameRE 限 username 为 1-32 [a-z0-9_-]，存前转小写。
var usernameRE = regexp.MustCompile(`^[a-z0-9_-]{1,32}$`)

// Service orchestrates User CRUD.
//
// Service 编排 User CRUD。
type Service struct {
	repo userdomain.Repository
	log  *zap.Logger
}

// NewService wires dependencies; panics on nil logger.
//
// NewService 装配依赖；nil logger panic。
func NewService(repo userdomain.Repository, log *zap.Logger) *Service {
	if log == nil {
		panic("user.NewService: logger is nil")
	}
	return &Service{repo: repo, log: log.Named("userapp")}
}

// CreateInput is the validated payload for Service.Create.
type CreateInput struct {
	Username    string
	DisplayName string
	AvatarColor string
	Language    string // optional; defaults to zh-CN
}

// UpdateInput is the partial-update payload; nil fields are skipped.
//
// UpdateInput 是部分更新载荷；nil 字段跳过。
type UpdateInput struct {
	DisplayName *string
	AvatarColor *string
	Language    *string
}

// Create makes a new user; username uniqueness + format validated.
//
// Create 创建新 user；username 唯一性 + 格式校验。
func (s *Service) Create(ctx context.Context, in CreateInput) (*userdomain.User, error) {
	username := strings.ToLower(strings.TrimSpace(in.Username))
	if username == "" {
		return nil, userdomain.ErrUsernameRequired
	}
	if !usernameRE.MatchString(username) {
		return nil, userdomain.ErrUsernameInvalid
	}
	now := time.Now().UTC()
	u := &userdomain.User{
		ID:          idgenpkg.New("u"),
		Username:    username,
		DisplayName: strings.TrimSpace(in.DisplayName),
		AvatarColor: strings.TrimSpace(in.AvatarColor),
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if u.DisplayName == "" {
		u.DisplayName = username
	}
	if in.Language != "" {
		if !userdomain.IsValidLanguage(in.Language) {
			return nil, userdomain.ErrLanguageInvalid
		}
		u.Language = in.Language
	} else {
		u.Language = userdomain.LanguageZhCN
	}
	if err := s.repo.Save(ctx, u); err != nil {
		return nil, err
	}
	s.log.Info("user created",
		zap.String("user_id", u.ID),
		zap.String("username", u.Username))
	return u, nil
}

// EnsureDefault creates the migration "default" user pinned to ID="local-user" when users table is empty.
// Called at boot to make existing single-user data discoverable as a profile.
//
// EnsureDefault 在 users 表空时创建迁移用 "default" user（ID 固定 "local-user"）。
// 启动时调，让老的单用户数据作为一个 profile 浮现。
func (s *Service) EnsureDefault(ctx context.Context) (*userdomain.User, error) {
	n, err := s.repo.Count(ctx)
	if err != nil {
		return nil, fmt.Errorf("user.EnsureDefault: count: %w", err)
	}
	if n > 0 {
		return nil, nil
	}
	now := time.Now().UTC()
	u := &userdomain.User{
		ID:          "local-user", // matches reqctxpkg.DefaultLocalUserID; existing rows already use this user_id
		Username:    userdomain.DefaultUsername,
		DisplayName: "Default",
		AvatarColor: "#4f46e5",
		Language:    userdomain.LanguageZhCN,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := s.repo.Save(ctx, u); err != nil {
		return nil, fmt.Errorf("user.EnsureDefault: save: %w", err)
	}
	s.log.Info("default user created (data migration)",
		zap.String("user_id", u.ID))
	return u, nil
}

// Get returns one user by id.
//
// Get 按 id 取 user。
func (s *Service) Get(ctx context.Context, id string) (*userdomain.User, error) {
	return s.repo.Get(ctx, id)
}

// GetByUsername returns one user by username (lowercased before lookup).
//
// GetByUsername 按 username 查（先转小写）。
func (s *Service) GetByUsername(ctx context.Context, username string) (*userdomain.User, error) {
	return s.repo.GetByUsername(ctx, strings.ToLower(strings.TrimSpace(username)))
}

// List returns all users (small set, no pagination needed).
//
// List 返所有 user（数量小，不分页）。
func (s *Service) List(ctx context.Context) ([]*userdomain.User, error) {
	return s.repo.List(ctx)
}

// Update applies partial fields to a user; nil = skip.
//
// Update 部分更新；nil 字段跳过。
func (s *Service) Update(ctx context.Context, id string, in UpdateInput) (*userdomain.User, error) {
	u, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if in.DisplayName != nil {
		u.DisplayName = strings.TrimSpace(*in.DisplayName)
	}
	if in.AvatarColor != nil {
		u.AvatarColor = strings.TrimSpace(*in.AvatarColor)
	}
	if in.Language != nil {
		if !userdomain.IsValidLanguage(*in.Language) {
			return nil, userdomain.ErrLanguageInvalid
		}
		u.Language = *in.Language
	}
	u.UpdatedAt = time.Now().UTC()
	if err := s.repo.Save(ctx, u); err != nil {
		return nil, err
	}
	return u, nil
}

// Delete removes a user; refuses to delete the last one.
//
// Delete 删 user；最后一个拒删。
func (s *Service) Delete(ctx context.Context, id string) error {
	n, err := s.repo.Count(ctx)
	if err != nil {
		return fmt.Errorf("user.Delete: count: %w", err)
	}
	if n <= 1 {
		return userdomain.ErrCannotDeleteLast
	}
	return s.repo.Delete(ctx, id)
}

// TouchLastUsed updates the user's last-used timestamp (called on session activate).
//
// TouchLastUsed 刷 last-used 时间戳（session 激活时调）。
func (s *Service) TouchLastUsed(ctx context.Context, id string) error {
	return s.repo.TouchLastUsed(ctx, id)
}
