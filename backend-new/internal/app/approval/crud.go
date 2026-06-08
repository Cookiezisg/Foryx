package approval

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"

	approvaldomain "github.com/sunweilin/forgify/backend/internal/domain/approval"
	celpkg "github.com/sunweilin/forgify/backend/internal/pkg/cel"
	idgenpkg "github.com/sunweilin/forgify/backend/internal/pkg/idgen"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

// CreateInput is the create payload: full metadata + the prompt template + decision rules.
// No ops — a form is an atomic whole, so create/edit pass the complete set.
//
// CreateInput 是 create 载荷：完整元数据 + prompt 模板 + 决策规则。无 ops——表是整体，create/edit
// 直接传完整组。
type CreateInput struct {
	Name            string
	Description     string
	Template        string
	AllowReason     bool
	Timeout         string
	TimeoutBehavior string
	ChangeReason    string
}

// EditInput writes a new version from a fresh template + rules set and moves the active pointer.
//
// EditInput 用一组新 template + 规则写新版本并把 active 指针移到它。
type EditInput struct {
	ID              string
	Template        string
	AllowReason     bool
	Timeout         string
	TimeoutBehavior string
	ChangeReason    string
}

// UpdateMetaInput patches form metadata without a version bump; nil = unchanged.
//
// UpdateMetaInput 改表元数据不动版本；nil = 不变。
type UpdateMetaInput struct {
	ID          string
	Name        *string
	Description *string
}

// Create validates + persists an ApprovalForm + v1 (active).
//
// Create 校验 + 持久化 ApprovalForm + v1（active）。
func (s *Service) Create(ctx context.Context, in CreateInput) (*approvaldomain.ApprovalForm, *approvaldomain.Version, error) {
	if strings.TrimSpace(in.Name) == "" {
		return nil, nil, approvaldomain.ErrInvalidName
	}
	if err := s.validateForm(in.Template, in.Timeout, in.TimeoutBehavior); err != nil {
		return nil, nil, err
	}
	now := time.Now().UTC()
	formID := idgenpkg.New("apf")
	versionID := idgenpkg.New("apfv")
	f := &approvaldomain.ApprovalForm{
		ID: formID, Name: in.Name, Description: in.Description,
		ActiveVersionID: versionID, CreatedAt: now, UpdatedAt: now,
	}
	v := newVersion(versionID, formID, 1, in.Template, in.AllowReason, in.Timeout, in.TimeoutBehavior, in.ChangeReason, now)
	if convID, ok := reqctxpkg.GetConversationID(ctx); ok {
		v.ForgedInConversationID = &convID
	}

	if err := s.repo.SaveForm(ctx, f); err != nil { // UNIQUE name → ErrDuplicateName here
		return nil, nil, fmt.Errorf("approvalapp.Create: %w", err)
	}
	if err := s.repo.SaveVersion(ctx, v); err != nil {
		return nil, nil, fmt.Errorf("approvalapp.Create: %w", err)
	}
	s.publish(ctx, "created", formID, map[string]any{"versionId": versionID, "version": 1})
	s.syncForgedEdge(ctx, formID, v.ForgedInConversationID)

	f.ActiveVersion = v
	return f, v, nil
}

// Edit writes a new version (a whole new template + rules set) and moves the active pointer.
//
// Edit 写新版本（整组新 template + 规则）并把 active 指针移到它。
func (s *Service) Edit(ctx context.Context, in EditInput) (*approvaldomain.Version, error) {
	if _, err := s.repo.GetForm(ctx, in.ID); err != nil {
		return nil, fmt.Errorf("approvalapp.Edit: %w", err)
	}
	if err := s.validateForm(in.Template, in.Timeout, in.TimeoutBehavior); err != nil {
		return nil, err
	}
	max, err := s.repo.MaxVersionNumber(ctx, in.ID)
	if err != nil {
		return nil, fmt.Errorf("approvalapp.Edit: %w", err)
	}
	now := time.Now().UTC()
	versionID := idgenpkg.New("apfv")
	v := newVersion(versionID, in.ID, max+1, in.Template, in.AllowReason, in.Timeout, in.TimeoutBehavior, in.ChangeReason, now)
	if convID, ok := reqctxpkg.GetConversationID(ctx); ok {
		v.ForgedInConversationID = &convID
	}
	if err := s.repo.SaveVersion(ctx, v); err != nil {
		return nil, fmt.Errorf("approvalapp.Edit: %w", err)
	}
	if err := s.repo.SetActiveVersion(ctx, in.ID, versionID); err != nil {
		return nil, fmt.Errorf("approvalapp.Edit: %w", err)
	}
	if err := s.repo.TrimOldestVersions(ctx, in.ID, approvaldomain.VersionCap); err != nil {
		s.log.Warn("approvalapp.Edit: trim versions failed", zap.String("approvalId", in.ID), zap.Error(err))
	}
	s.publish(ctx, "edited", in.ID, map[string]any{"versionId": versionID, "version": max + 1})
	s.syncEditedEdge(ctx, in.ID)
	return v, nil
}

// UpdateMeta patches form metadata (name/description) without creating a version.
//
// UpdateMeta 改表元数据（name/description）不产版本。
func (s *Service) UpdateMeta(ctx context.Context, in UpdateMetaInput) (*approvaldomain.ApprovalForm, error) {
	f, err := s.repo.GetForm(ctx, in.ID)
	if err != nil {
		return nil, fmt.Errorf("approvalapp.UpdateMeta: %w", err)
	}
	if in.Name != nil {
		if strings.TrimSpace(*in.Name) == "" {
			return nil, approvaldomain.ErrInvalidName
		}
		f.Name = *in.Name
	}
	if in.Description != nil {
		f.Description = *in.Description
	}
	if err := s.repo.SaveForm(ctx, f); err != nil {
		return nil, fmt.Errorf("approvalapp.UpdateMeta: %w", err)
	}
	s.publish(ctx, "updated", f.ID, nil)
	return f, nil
}

// Revert moves the active pointer to an existing version by number — a pure pointer op.
//
// Revert 按号把 active 指针移到一个已有版本——纯指针操作。
func (s *Service) Revert(ctx context.Context, id string, targetVersion int) (*approvaldomain.Version, error) {
	target, err := s.repo.GetVersionByNumber(ctx, id, targetVersion)
	if err != nil {
		return nil, fmt.Errorf("approvalapp.Revert: %w", err)
	}
	if err := s.repo.SetActiveVersion(ctx, id, target.ID); err != nil {
		return nil, fmt.Errorf("approvalapp.Revert: %w", err)
	}
	s.publish(ctx, "reverted", id, map[string]any{"versionId": target.ID, "version": targetVersion})
	s.syncEditedEdge(ctx, id)
	return target, nil
}

// Get returns one approval form with its active version attached.
//
// Get 返单审批表并附上 active 版本。
func (s *Service) Get(ctx context.Context, id string) (*approvaldomain.ApprovalForm, error) {
	f, err := s.repo.GetForm(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("approvalapp.Get: %w", err)
	}
	if f.ActiveVersionID != "" {
		if v, verr := s.repo.GetVersion(ctx, f.ActiveVersionID); verr == nil {
			f.ActiveVersion = v
		}
	}
	return f, nil
}

// List returns a cursor page of live approval forms.
func (s *Service) List(ctx context.Context, filter approvaldomain.ListFilter) ([]*approvaldomain.ApprovalForm, string, error) {
	return s.repo.ListForms(ctx, filter)
}

// ListAll returns every live approval form (catalog source).
func (s *Service) ListAll(ctx context.Context) ([]*approvaldomain.ApprovalForm, error) {
	return s.repo.ListAllForms(ctx)
}

// Search filters live approval forms by case-insensitive substring over name / description.
//
// Search 按 name / description 大小写不敏感子串过滤活跃审批表。
func (s *Service) Search(ctx context.Context, query string) ([]*approvaldomain.ApprovalForm, error) {
	all, err := s.repo.ListAllForms(ctx)
	if err != nil {
		return nil, fmt.Errorf("approvalapp.Search: %w", err)
	}
	if strings.TrimSpace(query) == "" {
		return all, nil
	}
	needle := strings.ToLower(query)
	out := make([]*approvaldomain.ApprovalForm, 0, len(all))
	for _, f := range all {
		if strings.Contains(strings.ToLower(f.Name), needle) || strings.Contains(strings.ToLower(f.Description), needle) {
			out = append(out, f)
		}
	}
	return out, nil
}

// Delete soft-deletes the approval form and purges its relation edges.
//
// Delete 软删审批表并清理 relation 边。
func (s *Service) Delete(ctx context.Context, id string) error {
	if err := s.repo.DeleteForm(ctx, id); err != nil {
		return fmt.Errorf("approvalapp.Delete: %w", err)
	}
	s.publish(ctx, "deleted", id, nil)
	s.purgeRelations(ctx, id)
	return nil
}

// --- version reads ---

func (s *Service) ListVersions(ctx context.Context, formID string, filter approvaldomain.VersionListFilter) ([]*approvaldomain.Version, string, error) {
	return s.repo.ListVersions(ctx, formID, filter)
}

func (s *Service) GetVersion(ctx context.Context, versionID string) (*approvaldomain.Version, error) {
	return s.repo.GetVersion(ctx, versionID)
}

func (s *Service) GetVersionByNumber(ctx context.Context, formID string, versionN int) (*approvaldomain.Version, error) {
	return s.repo.GetVersionByNumber(ctx, formID, versionN)
}

// Resolve returns a version (template + decision rules) for the durable interpreter (波次 4).
// versionID=="" means the active version. The interpreter compiles the template + Renders it
// over the pinned payload, then parks (approvals runtime table).
//
// Resolve 返某版本（template + 决策规则）供 durable 解释器（波次 4）。versionID 空 = active。解释器
// 编译 template + 对 pin 的 payload Render，然后 park（approvals 运行时表）。
func (s *Service) Resolve(ctx context.Context, id, versionID string) (*approvaldomain.Version, error) {
	f, err := s.repo.GetForm(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("approvalapp.Resolve: %w", err)
	}
	vid := versionID
	if vid == "" {
		if f.ActiveVersionID == "" {
			return nil, approvaldomain.ErrNoActiveVersion
		}
		vid = f.ActiveVersionID
	}
	v, err := s.repo.GetVersion(ctx, vid)
	if err != nil {
		return nil, fmt.Errorf("approvalapp.Resolve: %w", err)
	}
	return v, nil
}

// validateForm runs domain structural checks (template non-empty, timeout↔behavior) then
// compiles the template's `{{ CEL }}` spans via pkg/cel (domain can't import cel-go, 原则 #3)
// — a compile error maps to ErrInvalidTemplate. Fast-fail at create/edit.
//
// validateForm 跑 domain 结构校验（template 非空、timeout↔behavior），再用 pkg/cel 编译模板的
// `{{ CEL }}` 段（domain 不能 import cel-go，原则 #3）——编译错映射 ErrInvalidTemplate。create/edit
// 期快速失败。
func (s *Service) validateForm(template, timeout, timeoutBehavior string) error {
	if err := approvaldomain.ValidateForm(template, timeout, timeoutBehavior); err != nil {
		return err
	}
	if _, err := celpkg.CompileTemplate(template); err != nil {
		return approvaldomain.ErrInvalidTemplate
	}
	return nil
}

func newVersion(id, formID string, n int, template string, allowReason bool, timeout, timeoutBehavior, changeReason string, now time.Time) *approvaldomain.Version {
	return &approvaldomain.Version{
		ID: id, ApprovalID: formID, Version: n,
		Template: template, AllowReason: allowReason, Timeout: timeout, TimeoutBehavior: timeoutBehavior,
		ChangeReason: changeReason, CreatedAt: now, UpdatedAt: now,
	}
}
