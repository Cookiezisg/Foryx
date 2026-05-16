// Package conversation owns the conversation CRUD Service (create / list / rename / delete).
//
// Package conversation 持有对话 CRUD Service（创建 / 列表 / 改名 / 删除）。
package conversation

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"

	convdomain "github.com/sunweilin/forgify/backend/internal/domain/conversation"
	idgenpkg "github.com/sunweilin/forgify/backend/internal/pkg/idgen"
	notificationspkg "github.com/sunweilin/forgify/backend/internal/pkg/notifications"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

// Service orchestrates conversation CRUD.
//
// Service 编排对话 CRUD。
type Service struct {
	repo  convdomain.Repository
	notif notificationspkg.Publisher
	log   *zap.Logger
}

// NewService wires dependencies; panics on nil logger; nil notif → no-op.
//
// NewService 装配依赖；nil logger panic；nil notif → no-op 兜底。
func NewService(repo convdomain.Repository, notif notificationspkg.Publisher, log *zap.Logger) *Service {
	if log == nil {
		panic("conversation.NewService: logger is nil")
	}
	if notif == nil {
		notif = notificationspkg.New(nil, log)
	}
	return &Service{repo: repo, notif: notif, log: log}
}

// Create makes a new conversation with the given title (may be empty).
//
// Create 创建一个新对话，title 可为空。
func (s *Service) Create(ctx context.Context, title string) (*convdomain.Conversation, error) {
	uid, err := reqctxpkg.RequireUserID(ctx)
	if err != nil {
		return nil, fmt.Errorf("conversation.Service.Create: %w", err)
	}
	now := time.Now().UTC()
	c := &convdomain.Conversation{
		ID:        newID(),
		UserID:    uid,
		Title:     strings.TrimSpace(title),
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := s.repo.Save(ctx, c); err != nil {
		return nil, err
	}
	s.log.Info("conversation created",
		zap.String("conversation_id", c.ID),
		zap.String("user_id", uid))
	s.notif.Publish(ctx, "conversation", c.ID,
		map[string]any{"action": "created", "title": c.Title}, c.ID)
	return c, nil
}

// List returns a paginated page of conversations.
//
// List 返回对话的一页。
func (s *Service) List(ctx context.Context, filter convdomain.ListFilter) ([]*convdomain.Conversation, string, error) {
	return s.repo.List(ctx, filter)
}

// Get fetches one conversation by id, scoped to ctx user.
//
// Get 按 id 取对话，按 ctx 用户过滤。
func (s *Service) Get(ctx context.Context, id string) (*convdomain.Conversation, error) {
	return s.repo.Get(ctx, id)
}

// Rename updates the conversation title.
//
// Rename 更新对话标题。
func (s *Service) Rename(ctx context.Context, id, title string) (*convdomain.Conversation, error) {
	return s.Update(ctx, id, &title, nil)
}

// Update applies a PATCH (nil = skip, &"" = clear).
//
// Update 部分更新（nil 跳过、&"" 清空）。
func (s *Service) Update(ctx context.Context, id string, title, systemPrompt *string) (*convdomain.Conversation, error) {
	c, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if title != nil {
		c.Title = strings.TrimSpace(*title)
	}
	if systemPrompt != nil {
		c.SystemPrompt = *systemPrompt
	}
	c.UpdatedAt = time.Now().UTC()
	if err := s.repo.Save(ctx, c); err != nil {
		return nil, err
	}
	s.notif.Publish(ctx, "conversation", c.ID,
		map[string]any{"action": "updated", "title": c.Title}, c.ID)
	return c, nil
}

// Delete soft-deletes a conversation.
//
// Delete 软删除对话。
func (s *Service) Delete(ctx context.Context, id string) error {
	if err := s.repo.Delete(ctx, id); err != nil {
		return err
	}
	s.notif.Publish(ctx, "conversation", id,
		map[string]any{"action": "deleted"}, id)
	return nil
}

func newID() string { return idgenpkg.New("cv") }
