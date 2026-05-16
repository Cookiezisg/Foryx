// Package todo (app layer) owns the Service driving conversation-scoped todo CRUD with SSE publish.
//
// Package todo（app 层）提供 conversation 作用域的 todo CRUD 与 SSE 发布 Service。
package todo

import (
	"context"
	"fmt"
	"strings"

	"go.uber.org/zap"

	tododomain "github.com/sunweilin/forgify/backend/internal/domain/todo"
	idgenpkg "github.com/sunweilin/forgify/backend/internal/pkg/idgen"
	notificationspkg "github.com/sunweilin/forgify/backend/internal/pkg/notifications"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

// Service orchestrates todo CRUD scoped to ctx conversation and publishes "todo" notifications.
//
// Service 编排 conversation 作用域的 todo CRUD 并发布 "todo" 通知。
type Service struct {
	repo          tododomain.Repository
	notifications notificationspkg.Publisher
	log           *zap.Logger
}

// NewService wires Service; notifications may be nil for a no-op fallback; nil logger panics.
//
// NewService 装配 Service；notifications 可 nil 走 no-op；nil logger 立即 panic。
func NewService(repo tododomain.Repository, notifications notificationspkg.Publisher, log *zap.Logger) *Service {
	if log == nil {
		panic("todo.NewService: logger is nil")
	}
	if notifications == nil {
		notifications = notificationspkg.New(nil, log)
	}
	return &Service{repo: repo, notifications: notifications, log: log}
}

// CreateInput is the validated payload for Service.Create.
//
// CreateInput 是 Service.Create 的已校验载荷。
type CreateInput struct {
	Subject     string
	Description string
	ActiveForm  string
	BlockedBy   []string
	Metadata    map[string]any
}

// UpdateInput is the partial-update payload; nil pointer = leave unchanged.
//
// UpdateInput 是部分更新载荷；nil 指针表示「不变」。
type UpdateInput struct {
	Subject     *string
	Description *string
	ActiveForm  *string
	Status      *string
	Owner       *string
	BlockedBy   *[]string
	Metadata    map[string]any
}

// Create inserts a new todo scoped to the current conversation.
//
// Create 插入新 todo，作用域为当前 conversation。
func (s *Service) Create(ctx context.Context, in CreateInput) (*tododomain.Todo, error) {
	convID, ok := reqctxpkg.GetConversationID(ctx)
	if !ok || convID == "" {
		return nil, fmt.Errorf("todo.Service.Create: %w", reqctxpkg.ErrMissingConversationID)
	}
	subject := strings.TrimSpace(in.Subject)
	if subject == "" {
		return nil, tododomain.ErrSubjectRequired
	}
	t := &tododomain.Todo{
		ID:             newID(),
		ConversationID: convID,
		Subject:        subject,
		Description:    strings.TrimSpace(in.Description),
		ActiveForm:     strings.TrimSpace(in.ActiveForm),
		Status:         tododomain.StatusPending,
		BlockedBy:      in.BlockedBy,
		Metadata:       in.Metadata,
	}
	if err := s.repo.Create(ctx, t); err != nil {
		return nil, err
	}
	s.publish(ctx, t)
	s.log.Info("todo created",
		zap.String("todo_id", t.ID),
		zap.String("conversation_id", convID),
		zap.String("subject", subject))
	return t, nil
}

// Get returns a todo by ID scoped to the current conversation; cross-conversation lookups return ErrNotFound.
//
// Get 按 ID 返 todo 作用域当前 conversation；跨对话查询返 ErrNotFound。
func (s *Service) Get(ctx context.Context, id string) (*tododomain.Todo, error) {
	convID, ok := reqctxpkg.GetConversationID(ctx)
	if !ok || convID == "" {
		return nil, fmt.Errorf("todo.Service.Get: %w", reqctxpkg.ErrMissingConversationID)
	}
	t, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if t.ConversationID != convID {
		return nil, tododomain.ErrNotFound
	}
	return t, nil
}

// List returns all active todos for the current conversation.
//
// List 返回当前 conversation 的所有活跃 todo。
func (s *Service) List(ctx context.Context) ([]*tododomain.Todo, error) {
	convID, ok := reqctxpkg.GetConversationID(ctx)
	if !ok || convID == "" {
		return nil, fmt.Errorf("todo.Service.List: %w", reqctxpkg.ErrMissingConversationID)
	}
	return s.repo.ListByConversation(ctx, convID)
}

// Update applies a partial update; status is validated against the whitelist.
//
// Update 应用部分更新；status 按白名单校验。
func (s *Service) Update(ctx context.Context, id string, in UpdateInput) (*tododomain.Todo, error) {
	convID, ok := reqctxpkg.GetConversationID(ctx)
	if !ok || convID == "" {
		return nil, fmt.Errorf("todo.Service.Update: %w", reqctxpkg.ErrMissingConversationID)
	}
	t, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if t.ConversationID != convID {
		return nil, tododomain.ErrNotFound
	}
	if in.Subject != nil {
		subject := strings.TrimSpace(*in.Subject)
		if subject == "" {
			return nil, tododomain.ErrSubjectRequired
		}
		t.Subject = subject
	}
	if in.Description != nil {
		t.Description = strings.TrimSpace(*in.Description)
	}
	if in.ActiveForm != nil {
		t.ActiveForm = strings.TrimSpace(*in.ActiveForm)
	}
	if in.Status != nil {
		if !tododomain.IsValidStatus(*in.Status) {
			return nil, tododomain.ErrInvalidStatus
		}
		t.Status = *in.Status
	}
	if in.Owner != nil {
		t.Owner = strings.TrimSpace(*in.Owner)
	}
	if in.BlockedBy != nil {
		t.BlockedBy = *in.BlockedBy
	}
	if in.Metadata != nil {
		t.Metadata = in.Metadata
	}
	if err := s.repo.Update(ctx, t); err != nil {
		return nil, err
	}
	s.publish(ctx, t)
	s.log.Info("todo updated",
		zap.String("todo_id", t.ID),
		zap.String("conversation_id", convID),
		zap.String("status", t.Status))
	return t, nil
}

// Delete soft-deletes a todo scoped to the current conversation and publishes the final snapshot.
//
// Delete 软删 conversation 作用域内的 todo 并发布最终快照。
func (s *Service) Delete(ctx context.Context, id string) error {
	convID, ok := reqctxpkg.GetConversationID(ctx)
	if !ok || convID == "" {
		return fmt.Errorf("todo.Service.Delete: %w", reqctxpkg.ErrMissingConversationID)
	}
	t, err := s.repo.Get(ctx, id)
	if err != nil {
		return err
	}
	if t.ConversationID != convID {
		return tododomain.ErrNotFound
	}
	if err := s.repo.SoftDelete(ctx, id); err != nil {
		return err
	}
	t.Status = tododomain.StatusDeleted
	s.publish(ctx, t)
	s.log.Info("todo deleted",
		zap.String("todo_id", t.ID),
		zap.String("conversation_id", convID))
	return nil
}

func (s *Service) publish(ctx context.Context, t *tododomain.Todo) {
	s.notifications.Publish(ctx, "todo", t.ID, t, t.ConversationID)
}

func newID() string { return idgenpkg.New("td") }
