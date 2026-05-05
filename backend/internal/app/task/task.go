// Package task (app layer) owns the Service that the LLM tool family
// (app/tool/task) drives: validation, conversation scoping, ID minting,
// and SSE event publication on every mutation.
//
// All three task packages (domain / app / store) declare `package task`;
// callers alias by role (taskdomain / taskapp / taskstore) per §S13.
//
// Package task（app 层）持有 LLM 工具家族（app/tool/task）驱动的 Service：
// 校验、conversation 作用域、ID 分配、每次变更发 SSE。
//
// 三个 task 包（domain / app / store）都声明 `package task`；调用方按 §S13
// 别名（taskdomain / taskapp / taskstore）。
package task

import (
	"context"
	"fmt"
	"strings"

	"go.uber.org/zap"

	eventsdomain "github.com/sunweilin/forgify/backend/internal/domain/events"
	taskdomain "github.com/sunweilin/forgify/backend/internal/domain/task"
	idgenpkg "github.com/sunweilin/forgify/backend/internal/pkg/idgen"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

// Service orchestrates task CRUD, scopes everything to the conversation
// in ctx, and broadcasts entity-state SSE events through the bridge.
//
// Service 编排 task CRUD、按 ctx 中的 conversation 作用域、并通过 bridge
// 广播 entity-state SSE。
type Service struct {
	repo   taskdomain.Repository
	bridge eventsdomain.Bridge
	log    *zap.Logger
}

// NewService wires Service. Panics on nil logger so the missing-init bug
// surfaces immediately rather than as a nil-deref later.
//
// NewService 装配 Service。nil logger 立即 panic，让漏接错误立刻暴露。
func NewService(repo taskdomain.Repository, bridge eventsdomain.Bridge, log *zap.Logger) *Service {
	if log == nil {
		panic("task.NewService: logger is nil")
	}
	return &Service{repo: repo, bridge: bridge, log: log}
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

// UpdateInput is the partial-update payload for Service.Update. Pointer
// fields encode "leave unchanged" (nil) vs "set to this value"
// (including empty string / nil slice).
//
// UpdateInput 是 Service.Update 的部分更新载荷；指针字段编码"不变"（nil）
// 与"设为该值"（含空字符串 / nil 切片）。
type UpdateInput struct {
	Subject     *string
	Description *string
	ActiveForm  *string
	Status      *string
	Owner       *string
	BlockedBy   *[]string
	Metadata    map[string]any
}

// Create inserts a new task scoped to the current conversation. ID is
// minted with the `tk_` prefix per §S15.
//
// Create 插入新任务，作用域为当前 conversation；ID 用 `tk_` 前缀（§S15）。
func (s *Service) Create(ctx context.Context, in CreateInput) (*taskdomain.Task, error) {
	convID, ok := reqctxpkg.GetConversationID(ctx)
	if !ok || convID == "" {
		return nil, fmt.Errorf("task.Service.Create: %w", reqctxpkg.ErrMissingConversationID)
	}
	subject := strings.TrimSpace(in.Subject)
	if subject == "" {
		return nil, taskdomain.ErrSubjectRequired
	}
	t := &taskdomain.Task{
		ID:             newID(),
		ConversationID: convID,
		Subject:        subject,
		Description:    strings.TrimSpace(in.Description),
		ActiveForm:     strings.TrimSpace(in.ActiveForm),
		Status:         taskdomain.StatusPending,
		BlockedBy:      in.BlockedBy,
		Metadata:       in.Metadata,
	}
	if err := s.repo.Create(ctx, t); err != nil {
		return nil, err
	}
	s.publish(ctx, t)
	s.log.Info("task created",
		zap.String("task_id", t.ID),
		zap.String("conversation_id", convID),
		zap.String("subject", subject))
	return t, nil
}

// Get returns a task by ID, scoped to the current conversation. A task
// belonging to another conversation is reported as ErrNotFound rather
// than ErrConversationMismatch — we don't want to leak existence across
// conversations.
//
// Get 按 ID 返任务，作用域为当前 conversation；属于另一对话的任务报
// ErrNotFound 而非 ErrConversationMismatch——不跨对话泄漏存在性。
func (s *Service) Get(ctx context.Context, id string) (*taskdomain.Task, error) {
	convID, ok := reqctxpkg.GetConversationID(ctx)
	if !ok || convID == "" {
		return nil, fmt.Errorf("task.Service.Get: %w", reqctxpkg.ErrMissingConversationID)
	}
	t, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if t.ConversationID != convID {
		return nil, taskdomain.ErrNotFound
	}
	return t, nil
}

// List returns all active tasks for the current conversation.
//
// List 返回当前 conversation 的所有活跃任务。
func (s *Service) List(ctx context.Context) ([]*taskdomain.Task, error) {
	convID, ok := reqctxpkg.GetConversationID(ctx)
	if !ok || convID == "" {
		return nil, fmt.Errorf("task.Service.List: %w", reqctxpkg.ErrMissingConversationID)
	}
	return s.repo.ListByConversation(ctx, convID)
}

// Update applies a partial update. Status transitions are validated
// against the whitelist; other fields are written verbatim. ConversationID
// cannot be changed (the input struct has no field for it).
//
// Update 应用部分更新；status 转换按白名单校验；其他字段原样写入。
// ConversationID 不可改（input 无该字段）。
func (s *Service) Update(ctx context.Context, id string, in UpdateInput) (*taskdomain.Task, error) {
	convID, ok := reqctxpkg.GetConversationID(ctx)
	if !ok || convID == "" {
		return nil, fmt.Errorf("task.Service.Update: %w", reqctxpkg.ErrMissingConversationID)
	}
	t, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if t.ConversationID != convID {
		// Same not-found semantics as Get.
		// 与 Get 同的 not-found 语义。
		return nil, taskdomain.ErrNotFound
	}
	if in.Subject != nil {
		subject := strings.TrimSpace(*in.Subject)
		if subject == "" {
			return nil, taskdomain.ErrSubjectRequired
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
		if !taskdomain.IsValidStatus(*in.Status) {
			return nil, taskdomain.ErrInvalidStatus
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
	s.log.Info("task updated",
		zap.String("task_id", t.ID),
		zap.String("conversation_id", convID),
		zap.String("status", t.Status))
	return t, nil
}

// Delete soft-deletes a task scoped to the current conversation. Returns
// ErrNotFound when absent or owned by another conversation. The final
// snapshot is published so subscribers can drop their local copy.
//
// Delete 软删任务（作用域当前 conversation）；不存在或属另一对话返
// ErrNotFound；发最终快照让订阅方丢本地拷贝。
func (s *Service) Delete(ctx context.Context, id string) error {
	convID, ok := reqctxpkg.GetConversationID(ctx)
	if !ok || convID == "" {
		return fmt.Errorf("task.Service.Delete: %w", reqctxpkg.ErrMissingConversationID)
	}
	t, err := s.repo.Get(ctx, id)
	if err != nil {
		return err
	}
	if t.ConversationID != convID {
		return taskdomain.ErrNotFound
	}
	if err := s.repo.SoftDelete(ctx, id); err != nil {
		return err
	}
	// Stamp the final status so subscribers see the deletion intent
	// (the row's deleted_at is set but not in the snapshot fields).
	// 把最终状态印到快照让订阅方看到删除意图（行的 deleted_at 已置但
	// 未在快照字段里）。
	t.Status = taskdomain.StatusDeleted
	s.publish(ctx, t)
	s.log.Info("task deleted",
		zap.String("task_id", t.ID),
		zap.String("conversation_id", convID))
	return nil
}

// publish sends a Task event on the bridge. Bridge.Publish is best-effort
// (slow subscribers drop events, never block) so there's nothing to
// return — we just call and move on.
//
// publish 在 bridge 发 Task 事件；Bridge.Publish 是 best-effort（慢订阅者
// 丢事件不阻塞），无错可返，调完即走。
func (s *Service) publish(ctx context.Context, t *taskdomain.Task) {
	if s.bridge == nil {
		return
	}
	s.bridge.Publish(ctx, t.ConversationID, eventsdomain.Task{Task: t})
}

func newID() string { return idgenpkg.New("tk") }
