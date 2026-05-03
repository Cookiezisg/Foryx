// Package conversation (app layer) owns the Service: create, list, rename,
// and delete conversation threads.
//
// All three conversation packages (domain / app / store) declare
// `package conversation`; external callers alias at import.
//
// Package conversation（app 层）负责 Service：创建、列出、改名、删除对话线程。
package conversation

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"

	convdomain "github.com/sunweilin/forgify/backend/internal/domain/conversation"
	idgenpkg "github.com/sunweilin/forgify/backend/internal/pkg/idgen"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

// Service orchestrates conversation CRUD.
//
// Service 编排对话 CRUD。
type Service struct {
	repo convdomain.Repository
	log  *zap.Logger
}

// NewService wires Service dependencies. Panics on nil logger.
//
// NewService 装配依赖。nil logger 立刻 panic。
func NewService(repo convdomain.Repository, log *zap.Logger) *Service {
	if log == nil {
		panic("conversation.NewService: logger is nil")
	}
	return &Service{repo: repo, log: log}
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
	return c, nil
}

// List returns a paginated page of the current user's conversations.
//
// List 返回当前用户对话的一页（分页）。
func (s *Service) List(ctx context.Context, filter convdomain.ListFilter) ([]*convdomain.Conversation, string, error) {
	return s.repo.List(ctx, filter)
}

// Rename updates the title of a conversation.
//
// Rename 更新对话的 title。
func (s *Service) Rename(ctx context.Context, id, title string) (*convdomain.Conversation, error) {
	c, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	c.Title = strings.TrimSpace(title)
	c.UpdatedAt = time.Now().UTC()
	if err := s.repo.Save(ctx, c); err != nil {
		return nil, err
	}
	return c, nil
}

// Delete soft-deletes a conversation.
//
// Delete 软删除一个对话。
func (s *Service) Delete(ctx context.Context, id string) error {
	return s.repo.Delete(ctx, id)
}

func newID() string { return idgenpkg.New("cv") }
