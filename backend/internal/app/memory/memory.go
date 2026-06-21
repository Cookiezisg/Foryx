// Package memory (app layer) orchestrates the per-workspace memory file store: CRUD +
// pin + the two-section system-prompt projection (pinned full-text + non-pinned
// index). Every mutation raises a notification via the injected Emitter
// ("memory.created/updated/deleted"). Workspace scoping is handled by the file store
// (path derived from ctx workspace), so this layer passes no workspace id.
//
// Package memory（app 层）编排按 workspace 的记忆文件 store：CRUD + pin + 两段式
// system-prompt 投影（pinned 全文 + 非 pinned 目录）。每次变更经注入的 Emitter 发通知
// （"memory.created/updated/deleted"）。workspace 隔离由文件 store 处理（路径据 ctx
// workspace 推导），本层不传 workspace id。
package memory

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"go.uber.org/zap"

	memorydomain "github.com/sunweilin/anselm/backend/internal/domain/memory"
	notificationdomain "github.com/sunweilin/anselm/backend/internal/domain/notification"
	searchdomain "github.com/sunweilin/anselm/backend/internal/domain/search"
)

// Service is the memory CRUD + system-prompt provider.
//
// Service 是记忆 CRUD + system-prompt 提供器。
type Service struct {
	repo    memorydomain.Repository
	search  searchdomain.Notifier      // nil → search indexing disabled. nil → 不接搜索索引。
	emitter notificationdomain.Emitter // notifications; nil → no notify (still persisted)
	log     *zap.Logger
}

// NewService wires dependencies; repo + log required, emitter optional (nil → no
// notifications, wired at boot).
//
// NewService 装配依赖；repo + log 必填，emitter 可选（nil → 不发通知，boot 装配）。
func NewService(repo memorydomain.Repository, emitter notificationdomain.Emitter, log *zap.Logger) *Service {
	if repo == nil {
		panic("memoryapp.NewService: repo is nil")
	}
	if log == nil {
		panic("memoryapp.NewService: log is nil")
	}
	return &Service{repo: repo, emitter: emitter, log: log}
}

var _ memorydomain.SystemPromptProvider = (*Service)(nil)

// UpsertInput is the write payload (LLM write_memory / frontend editor).
//
// UpsertInput 是写入载荷（LLM write_memory / 前端编辑）。
type UpsertInput struct {
	Name        string
	Description string
	Content     string
	Pinned      bool
	Source      string
}

// Upsert creates or updates a memory by name; notifies created vs updated.
//
// Upsert 按 name 创建或更新；通知区分 created / updated。
func (s *Service) Upsert(ctx context.Context, in UpsertInput) (*memorydomain.Memory, error) {
	if err := validateUpsert(in); err != nil {
		return nil, err
	}
	existing, gerr := s.repo.Get(ctx, in.Name)
	if gerr != nil && !errors.Is(gerr, memorydomain.ErrNotFound) {
		return nil, fmt.Errorf("memoryapp.Upsert: %w", gerr)
	}
	exists := gerr == nil
	action := "created"
	m := &memorydomain.Memory{
		Name:        in.Name,
		Description: strings.TrimSpace(in.Description),
		Content:     in.Content,
		Pinned:      in.Pinned,
		Source:      in.Source,
	}
	if exists {
		// A content update PRESERVES the user's curation: Pinned is the user's choice (changed only via
		// the dedicated pin/unpin endpoints) and Source is immutable authorship. Without this, an LLM
		// write_memory — which always sends source=ai and never sets pinned — silently UN-PINS and
		// re-attributes a user's verbatim-injected rule on every edit, demoting a pinned safety rule to a
		// lazy index line (F147). Re-pinning / re-authoring is not a content-write concern.
		// 内容更新**保留**用户策展：Pinned 是用户的选择（仅经专用 pin/unpin 端点改）、Source 是不可变作者归属。
		// 否则 LLM 的 write_memory（永远发 source=ai、从不设 pinned）每次编辑都静默取消置顶 + 改归属，把置顶安全
		// 规则降级成懒加载目录行（F147）。重新置顶/改归属不归内容写入管。
		action = "updated"
		m.Pinned = existing.Pinned
		m.Source = existing.Source
	}
	if err := s.repo.Save(ctx, m); err != nil {
		return nil, fmt.Errorf("memoryapp.Upsert: %w", err)
	}
	s.notify(ctx, action, m.Name)
	return m, nil
}

// Get reads one memory's full content; ErrNotFound when absent.
//
// Get 读一条记忆全文；不存在返 ErrNotFound。
func (s *Service) Get(ctx context.Context, name string) (*memorydomain.Memory, error) {
	return s.repo.Get(ctx, name)
}

// List returns the workspace's memories (optionally filtered by pinned).
//
// List 返回该 workspace 的记忆（可按 pinned 过滤）。
func (s *Service) List(ctx context.Context, filter memorydomain.ListFilter) ([]*memorydomain.Memory, error) {
	return s.repo.List(ctx, filter)
}

// Delete removes a memory by name.
//
// Delete 按 name 删一条记忆。
func (s *Service) Delete(ctx context.Context, name string) error {
	if err := s.repo.Delete(ctx, name); err != nil {
		return err
	}
	s.notify(ctx, "deleted", name)
	return nil
}

func (s *Service) Pin(ctx context.Context, name string) (*memorydomain.Memory, error) {
	return s.setPinned(ctx, name, true)
}

func (s *Service) Unpin(ctx context.Context, name string) (*memorydomain.Memory, error) {
	return s.setPinned(ctx, name, false)
}

func (s *Service) setPinned(ctx context.Context, name string, pinned bool) (*memorydomain.Memory, error) {
	m, err := s.repo.Get(ctx, name)
	if err != nil {
		return nil, err
	}
	if m.Pinned == pinned {
		return m, nil
	}
	m.Pinned = pinned
	if err := s.repo.Save(ctx, m); err != nil {
		return nil, fmt.Errorf("memoryapp.setPinned: %w", err)
	}
	s.notify(ctx, "updated", name)
	return m, nil
}

// ForSystemPrompt renders the two-section memory block: pinned memories in full, then
// a name+description index of the rest (LLM loads one via read_memory). "" when empty.
//
// ForSystemPrompt 渲染两段式 memory 块：pinned 记忆全文，再是其余的 name+description 目录
// （LLM 经 read_memory 加载）。空时返 ""。
func (s *Service) ForSystemPrompt(ctx context.Context) string {
	all, err := s.repo.List(ctx, memorydomain.ListFilter{})
	if err != nil {
		s.log.Warn("memoryapp.ForSystemPrompt: list failed", zap.Error(err))
		return ""
	}
	var pinned, index []*memorydomain.Memory
	for _, m := range all {
		if m.Pinned {
			pinned = append(pinned, m)
		} else {
			index = append(index, m)
		}
	}
	if len(pinned) == 0 && len(index) == 0 {
		return ""
	}
	var b strings.Builder
	if len(pinned) > 0 {
		b.WriteString("## Memory (pinned)\n")
		for _, m := range pinned {
			fmt.Fprintf(&b, "\n### %s (source: %s)\n%s\n", m.Name, m.Source, m.Content)
		}
	}
	if len(index) > 0 {
		if b.Len() > 0 {
			b.WriteString("\n")
		}
		b.WriteString("## Memory index — read_memory(name) to load\n")
		for _, m := range index {
			fmt.Fprintf(&b, "- %s: %s\n", m.Name, m.Description)
		}
	}
	return b.String()
}

// notify raises a memory.<action> notification (best-effort; nil emitter = skip).
//
// notify 发一条 memory.<action> 通知（best-effort；emitter 为 nil 则跳过）。
func (s *Service) notify(ctx context.Context, action, name string) {
	s.notifySearch(ctx, name)
	if s.emitter == nil {
		return
	}
	if err := s.emitter.Emit(ctx, "memory."+action, map[string]any{"name": name}); err != nil {
		s.log.Warn("memoryapp.notify failed", zap.String("name", name), zap.Error(err))
	}
}

func validateUpsert(in UpsertInput) error {
	if !memorydomain.IsValidName(in.Name) {
		return memorydomain.ErrInvalidName
	}
	if !memorydomain.IsValidSource(in.Source) {
		return memorydomain.ErrInvalidSource
	}
	if strings.TrimSpace(in.Description) == "" || strings.TrimSpace(in.Content) == "" {
		return memorydomain.ErrInvalidInput
	}
	return nil
}
