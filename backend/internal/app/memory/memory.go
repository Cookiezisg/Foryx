// Package memory owns the global memory Service (CRUD + system-prompt injection + notifications).
//
// Package memory 持有全局 memory Service（CRUD + system prompt 注入 + 通知）。
package memory

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"

	errorsdomain "github.com/sunweilin/forgify/backend/internal/domain/errors"
	memorydomain "github.com/sunweilin/forgify/backend/internal/domain/memory"
	idgenpkg "github.com/sunweilin/forgify/backend/internal/pkg/idgen"
	notificationspkg "github.com/sunweilin/forgify/backend/internal/pkg/notifications"
)

// Service orchestrates memory CRUD + chat-side injection.
//
// Service 编排 memory CRUD + chat 侧注入。
type Service struct {
	repo  memorydomain.Repository
	notif notificationspkg.Publisher
	log   *zap.Logger
}

// New wires dependencies; panics on nil logger; nil notif → noop fallback.
//
// New 装配依赖；nil logger panic；nil notif → noop 兜底。
func New(repo memorydomain.Repository, notif notificationspkg.Publisher, log *zap.Logger) *Service {
	if log == nil {
		panic("memoryapp.New: logger is nil")
	}
	if notif == nil {
		notif = notificationspkg.New(nil, log)
	}
	return &Service{repo: repo, notif: notif, log: log}
}

// Get fetches a memory by name and bumps access stats (best-effort); returns ErrNotFound when absent.
//
// Get 按 name 取 memory 并尽力更新访问统计；缺失返 ErrNotFound。
func (s *Service) Get(ctx context.Context, name string) (*memorydomain.Memory, error) {
	m, err := s.repo.GetByName(ctx, name)
	if err != nil {
		return nil, err
	}
	if err := s.repo.MarkAccessed(ctx, name); err != nil {
		s.log.Warn("memoryapp.Get: MarkAccessed failed", zap.String("name", name), zap.Error(err))
	}
	return m, nil
}

// GetByID fetches by ID (used by HTTP path-matched handlers).
//
// GetByID 按 ID 查（HTTP handler 用）。
func (s *Service) GetByID(ctx context.Context, id string) (*memorydomain.Memory, error) {
	return s.repo.GetByID(ctx, id)
}

// List returns live memories optionally filtered by type/pinned.
//
// List 返活跃 memory，可按 type/pinned 过滤。
func (s *Service) List(ctx context.Context, filter memorydomain.ListFilter) ([]*memorydomain.Memory, error) {
	return s.repo.List(ctx, filter)
}

// ListPinned returns pinned memories in stable order for system-prompt injection.
//
// ListPinned 返 pinned memory（稳定顺序）供 system prompt 注入。
func (s *Service) ListPinned(ctx context.Context) ([]*memorydomain.Memory, error) {
	return s.repo.ListPinned(ctx)
}

const MaxIndexLines = 200

var _ memorydomain.SystemPromptProvider = (*Service)(nil)

// ForSystemPrompt renders pinned content + memory index for system prompt; "" when empty.
//
// ForSystemPrompt 渲染 pinned 全文 + memory 索引供 system prompt；空时返 ""。
func (s *Service) ForSystemPrompt(ctx context.Context) string {
	var sb strings.Builder
	pinned, err := s.repo.ListPinned(ctx)
	if err != nil {
		s.log.Warn("memoryapp.ForSystemPrompt: ListPinned failed", zap.Error(err))
	} else if len(pinned) > 0 {
		sb.WriteString("──── Pinned memories ────\n")
		for _, m := range pinned {
			fmt.Fprintf(&sb, "\n## %s (type=%s)\n%s\n", m.Name, m.Type, m.Content)
		}
	}
	idx, err := s.ListIndex(ctx, MaxIndexLines)
	if err != nil {
		s.log.Warn("memoryapp.ForSystemPrompt: ListIndex failed", zap.Error(err))
	} else if idx != "" {
		if sb.Len() > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString("──── Memory index ────\n")
		sb.WriteString(idx)
		sb.WriteString("\nUse read_memory(name) to load a specific entry when relevant.\n")
		sb.WriteString("Use write_memory(...) when you learn something worth keeping across conversations.\n")
	}
	return sb.String()
}

// ListIndex builds the markdown index for system prompt (excludes pinned); "" when none.
//
// ListIndex 拼 markdown 索引段（排除 pinned）；无非 pinned 返 ""。
func (s *Service) ListIndex(ctx context.Context, limit int) (string, error) {
	if limit <= 0 {
		limit = MaxIndexLines
	}
	rows, err := s.repo.ListForIndex(ctx, limit)
	if err != nil {
		return "", err
	}
	if len(rows) == 0 {
		return "", nil
	}
	var sb strings.Builder
	for _, m := range rows {
		fmt.Fprintf(&sb, "- [%s] %s: %s\n", m.Type, m.Name, m.Description)
	}
	return sb.String(), nil
}

// Create inserts a new memory; returns ErrNameConflict on duplicate name.
//
// Create 创建新 memory；name 重复返 ErrNameConflict。
func (s *Service) Create(ctx context.Context, in UpsertInput) (*memorydomain.Memory, error) {
	if err := validateUpsert(in); err != nil {
		return nil, err
	}
	if _, err := s.repo.GetByName(ctx, in.Name); err == nil {
		return nil, memorydomain.ErrNameConflict
	} else if !errors.Is(err, memorydomain.ErrNotFound) {
		return nil, fmt.Errorf("memoryapp.Create: lookup: %w", err)
	}
	return s.Upsert(ctx, in)
}

type UpsertInput = memorydomain.UpsertInput

// Upsert creates or updates a memory matched by Name; validates Name/Type/Source/Description.
//
// Upsert 创建或更新 memory（按 Name 匹配）；校验 Name / Type / Source / Description。
func (s *Service) Upsert(ctx context.Context, in UpsertInput) (*memorydomain.Memory, error) {
	if err := validateUpsert(in); err != nil {
		return nil, err
	}

	existing, err := s.repo.GetByName(ctx, in.Name)
	switch {
	case err == nil:
		existing.Type = in.Type
		existing.Description = strings.TrimSpace(in.Description)
		existing.Content = in.Content
		if in.Pinned != nil {
			existing.Pinned = *in.Pinned
		}
		// Source intentionally not updated — preserves original authorship.
		existing.Metadata = in.Metadata
		existing.UpdatedAt = time.Now().UTC()
		if err := s.repo.Save(ctx, existing); err != nil {
			return nil, fmt.Errorf("memoryapp.Upsert: %w", err)
		}
		s.publish(ctx, existing.ID, "updated", existing.Name, existing.Type, existing.Source)
		s.log.Info("memory updated",
			zap.String("memory_id", existing.ID),
			zap.String("name", existing.Name),
			zap.String("type", existing.Type),
			zap.String("source", existing.Source))
		return existing, nil

	case errors.Is(err, memorydomain.ErrNotFound):
		now := time.Now().UTC()
		pinned := false
		if in.Pinned != nil {
			pinned = *in.Pinned
		}
		m := &memorydomain.Memory{
			ID:          newID(),
			Name:        in.Name,
			Type:        in.Type,
			Description: strings.TrimSpace(in.Description),
			Content:     in.Content,
			Pinned:      pinned,
			Source:      in.Source,
			Metadata:    in.Metadata,
			CreatedAt:   now,
			UpdatedAt:   now,
		}
		if err := s.repo.Save(ctx, m); err != nil {
			return nil, fmt.Errorf("memoryapp.Upsert: %w", err)
		}
		s.publish(ctx, m.ID, "created", m.Name, m.Type, m.Source)
		s.log.Info("memory created",
			zap.String("memory_id", m.ID),
			zap.String("name", m.Name),
			zap.String("type", m.Type),
			zap.String("source", m.Source))
		return m, nil

	default:
		return nil, fmt.Errorf("memoryapp.Upsert: lookup: %w", err)
	}
}

func (s *Service) Pin(ctx context.Context, name string) (*memorydomain.Memory, error) {
	return s.setPinned(ctx, name, true, "pinned")
}

func (s *Service) Unpin(ctx context.Context, name string) (*memorydomain.Memory, error) {
	return s.setPinned(ctx, name, false, "unpinned")
}

func (s *Service) setPinned(ctx context.Context, name string, pinned bool, action string) (*memorydomain.Memory, error) {
	m, err := s.repo.GetByName(ctx, name)
	if err != nil {
		return nil, err
	}
	if m.Pinned == pinned {
		return m, nil
	}
	m.Pinned = pinned
	m.UpdatedAt = time.Now().UTC()
	if err := s.repo.Save(ctx, m); err != nil {
		return nil, fmt.Errorf("memoryapp.setPinned: %w", err)
	}
	s.publish(ctx, m.ID, action, m.Name, m.Type, m.Source)
	return m, nil
}

// Delete soft-deletes the memory by name.
//
// Delete 按 name 软删 memory。
func (s *Service) Delete(ctx context.Context, name string) error {
	m, err := s.repo.GetByName(ctx, name)
	if err != nil {
		return err
	}
	if err := s.repo.Delete(ctx, name); err != nil {
		return fmt.Errorf("memoryapp.Delete: %w", err)
	}
	s.publish(ctx, m.ID, "deleted", m.Name, m.Type, m.Source)
	s.log.Info("memory deleted",
		zap.String("memory_id", m.ID),
		zap.String("name", m.Name))
	return nil
}

func validateUpsert(in UpsertInput) error {
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return fmt.Errorf("memoryapp.validateUpsert: name required: %w", errorsdomain.ErrInvalidRequest)
	}
	if !memorydomain.NameRegex.MatchString(name) {
		return fmt.Errorf("memoryapp.validateUpsert: name %q: %w", name, memorydomain.ErrInvalidName)
	}
	if !memorydomain.IsValidType(in.Type) {
		return fmt.Errorf("memoryapp.validateUpsert: type %q invalid: %w", in.Type, errorsdomain.ErrInvalidRequest)
	}
	if !memorydomain.IsValidSource(in.Source) {
		return fmt.Errorf("memoryapp.validateUpsert: source %q invalid: %w", in.Source, errorsdomain.ErrInvalidRequest)
	}
	if strings.TrimSpace(in.Description) == "" {
		return fmt.Errorf("memoryapp.validateUpsert: description required: %w", errorsdomain.ErrInvalidRequest)
	}
	return nil
}

func (s *Service) publish(ctx context.Context, id, action, name, ty, source string) {
	s.notif.Publish(ctx, "memory", id, map[string]any{
		"action":  action,
		"name":    name,
		"memType": ty,
		"source":  source,
	}, "")
}

func newID() string { return idgenpkg.New("mem") }
