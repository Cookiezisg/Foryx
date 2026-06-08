package control

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"

	controldomain "github.com/sunweilin/forgify/backend/internal/domain/control"
	celpkg "github.com/sunweilin/forgify/backend/internal/pkg/cel"
	idgenpkg "github.com/sunweilin/forgify/backend/internal/pkg/idgen"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
	schemapkg "github.com/sunweilin/forgify/backend/internal/pkg/schema"
)

// CreateInput is the create payload: full metadata + the ordered branch set. No ops —
// branches are an atomic whole, so create/edit pass the complete set.
//
// CreateInput 是 create 载荷：完整元数据 + 有序 branch 组。无 ops——branches 是整体，create/edit
// 直接传完整组。
type CreateInput struct {
	Name         string
	Description  string
	InputSchema  []schemapkg.Field
	Branches     []controldomain.Branch
	ChangeReason string
}

// EditInput writes a new version from a fresh branch set (a non-nil whole-set replace),
// and moves the active pointer to it.
//
// EditInput 用一组新 branches 写新版本（整组替换）并把 active 指针移到它。
type EditInput struct {
	ID           string
	InputSchema  []schemapkg.Field
	Branches     []controldomain.Branch
	ChangeReason string
}

// UpdateMetaInput patches control metadata without a version bump; nil = unchanged.
//
// UpdateMetaInput 改 control 元数据不动版本；nil = 不变。
type UpdateMetaInput struct {
	ID          string
	Name        *string
	Description *string
}

// Create validates + persists a ControlLogic + v1 (active). Workspace UNIQUE name is
// enforced by the store (SaveControl maps a conflict to ErrDuplicateName) so a duplicate
// never leaves an orphan version (control row is written first).
//
// Create 校验 + 持久化 ControlLogic + v1（active）。workspace UNIQUE name 由 store 保证
// （SaveControl 把冲突映射为 ErrDuplicateName），故重名绝不留孤儿版本（先写 control 行）。
func (s *Service) Create(ctx context.Context, in CreateInput) (*controldomain.ControlLogic, *controldomain.Version, error) {
	if strings.TrimSpace(in.Name) == "" {
		return nil, nil, controldomain.ErrInvalidName
	}
	if err := s.validateBranches(in.Branches); err != nil {
		return nil, nil, err
	}
	now := time.Now().UTC()
	ctlID := idgenpkg.New("ctl")
	versionID := idgenpkg.New("ctlv")
	c := &controldomain.ControlLogic{
		ID: ctlID, Name: in.Name, Description: in.Description,
		ActiveVersionID: versionID, CreatedAt: now, UpdatedAt: now,
	}
	v := &controldomain.Version{
		ID: versionID, ControlID: ctlID, Version: 1, InputSchema: in.InputSchema, Branches: in.Branches,
		ChangeReason: in.ChangeReason, CreatedAt: now, UpdatedAt: now,
	}
	if convID, ok := reqctxpkg.GetConversationID(ctx); ok {
		v.ForgedInConversationID = &convID
	}

	if err := s.repo.SaveControl(ctx, c); err != nil { // UNIQUE name → ErrDuplicateName here
		return nil, nil, fmt.Errorf("controlapp.Create: %w", err)
	}
	if err := s.repo.SaveVersion(ctx, v); err != nil {
		return nil, nil, fmt.Errorf("controlapp.Create: %w", err)
	}
	s.publish(ctx, "created", ctlID, map[string]any{"versionId": versionID, "version": 1})
	s.syncForgedEdge(ctx, ctlID, v.ForgedInConversationID)

	c.ActiveVersion = v
	return c, v, nil
}

// Edit writes a new version (a whole new branch set) and moves the active pointer to it.
//
// Edit 写新版本（整组新 branches）并把 active 指针移到它。
func (s *Service) Edit(ctx context.Context, in EditInput) (*controldomain.Version, error) {
	if _, err := s.repo.GetControl(ctx, in.ID); err != nil {
		return nil, fmt.Errorf("controlapp.Edit: %w", err)
	}
	if err := s.validateBranches(in.Branches); err != nil {
		return nil, err
	}
	max, err := s.repo.MaxVersionNumber(ctx, in.ID)
	if err != nil {
		return nil, fmt.Errorf("controlapp.Edit: %w", err)
	}
	now := time.Now().UTC()
	versionID := idgenpkg.New("ctlv")
	v := &controldomain.Version{
		ID: versionID, ControlID: in.ID, Version: max + 1, InputSchema: in.InputSchema, Branches: in.Branches,
		ChangeReason: in.ChangeReason, CreatedAt: now, UpdatedAt: now,
	}
	if convID, ok := reqctxpkg.GetConversationID(ctx); ok {
		v.ForgedInConversationID = &convID
	}
	if err := s.repo.SaveVersion(ctx, v); err != nil {
		return nil, fmt.Errorf("controlapp.Edit: %w", err)
	}
	if err := s.repo.SetActiveVersion(ctx, in.ID, versionID); err != nil {
		return nil, fmt.Errorf("controlapp.Edit: %w", err)
	}
	if err := s.repo.TrimOldestVersions(ctx, in.ID, controldomain.VersionCap); err != nil {
		s.log.Warn("controlapp.Edit: trim versions failed", zap.String("controlId", in.ID), zap.Error(err))
	}
	s.publish(ctx, "edited", in.ID, map[string]any{"versionId": versionID, "version": max + 1})
	s.syncEditedEdge(ctx, in.ID)
	return v, nil
}

// UpdateMeta patches control metadata (name/description) without creating a version.
//
// UpdateMeta 改 control 元数据（name/description）不产版本。
func (s *Service) UpdateMeta(ctx context.Context, in UpdateMetaInput) (*controldomain.ControlLogic, error) {
	c, err := s.repo.GetControl(ctx, in.ID)
	if err != nil {
		return nil, fmt.Errorf("controlapp.UpdateMeta: %w", err)
	}
	if in.Name != nil {
		if strings.TrimSpace(*in.Name) == "" {
			return nil, controldomain.ErrInvalidName
		}
		c.Name = *in.Name
	}
	if in.Description != nil {
		c.Description = *in.Description
	}
	if err := s.repo.SaveControl(ctx, c); err != nil {
		return nil, fmt.Errorf("controlapp.UpdateMeta: %w", err)
	}
	s.publish(ctx, "updated", c.ID, nil)
	return c, nil
}

// Revert moves the active pointer to an existing version by number — a pure pointer op:
// no new version, no deletion of "newer" versions.
//
// Revert 按号把 active 指针移到一个已有版本——纯指针操作：不产生版本、不删「更新的」版本。
func (s *Service) Revert(ctx context.Context, id string, targetVersion int) (*controldomain.Version, error) {
	target, err := s.repo.GetVersionByNumber(ctx, id, targetVersion)
	if err != nil {
		return nil, fmt.Errorf("controlapp.Revert: %w", err)
	}
	if err := s.repo.SetActiveVersion(ctx, id, target.ID); err != nil {
		return nil, fmt.Errorf("controlapp.Revert: %w", err)
	}
	s.publish(ctx, "reverted", id, map[string]any{"versionId": target.ID, "version": targetVersion})
	s.syncEditedEdge(ctx, id)
	return target, nil
}

// Get returns one control logic with its active version attached (branches in one trip).
//
// Get 返单 control 逻辑并附上 active 版本（一趟拿到 branches）。
func (s *Service) Get(ctx context.Context, id string) (*controldomain.ControlLogic, error) {
	c, err := s.repo.GetControl(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("controlapp.Get: %w", err)
	}
	if c.ActiveVersionID != "" {
		if v, verr := s.repo.GetVersion(ctx, c.ActiveVersionID); verr == nil {
			c.ActiveVersion = v
		}
	}
	return c, nil
}

// List returns a cursor page of live control logics.
func (s *Service) List(ctx context.Context, filter controldomain.ListFilter) ([]*controldomain.ControlLogic, string, error) {
	return s.repo.ListControls(ctx, filter)
}

// ListAll returns every live control logic (catalog source).
func (s *Service) ListAll(ctx context.Context) ([]*controldomain.ControlLogic, error) {
	return s.repo.ListAllControls(ctx)
}

// Search filters live control logics by case-insensitive substring over name / description.
//
// Search 按 name / description 大小写不敏感子串过滤活跃 control 逻辑。
func (s *Service) Search(ctx context.Context, query string) ([]*controldomain.ControlLogic, error) {
	all, err := s.repo.ListAllControls(ctx)
	if err != nil {
		return nil, fmt.Errorf("controlapp.Search: %w", err)
	}
	if strings.TrimSpace(query) == "" {
		return all, nil
	}
	needle := strings.ToLower(query)
	out := make([]*controldomain.ControlLogic, 0, len(all))
	for _, c := range all {
		if strings.Contains(strings.ToLower(c.Name), needle) || strings.Contains(strings.ToLower(c.Description), needle) {
			out = append(out, c)
		}
	}
	return out, nil
}

// Delete soft-deletes the control logic and purges its relation edges.
//
// Delete 软删 control 逻辑并清理 relation 边。
func (s *Service) Delete(ctx context.Context, id string) error {
	if err := s.repo.DeleteControl(ctx, id); err != nil {
		return fmt.Errorf("controlapp.Delete: %w", err)
	}
	s.publish(ctx, "deleted", id, nil)
	s.purgeRelations(ctx, id)
	return nil
}

// --- version reads ---

func (s *Service) ListVersions(ctx context.Context, controlID string, filter controldomain.VersionListFilter) ([]*controldomain.Version, string, error) {
	return s.repo.ListVersions(ctx, controlID, filter)
}

func (s *Service) GetVersion(ctx context.Context, versionID string) (*controldomain.Version, error) {
	return s.repo.GetVersion(ctx, versionID)
}

func (s *Service) GetVersionByNumber(ctx context.Context, controlID string, versionN int) (*controldomain.Version, error) {
	return s.repo.GetVersionByNumber(ctx, controlID, versionN)
}

// Resolve returns a version's branches for the durable interpreter (波次 4). versionID==""
// means the active version. The interpreter compiles + evals the CEL (pinned per flowrun).
//
// Resolve 返某版本的 branches 供 durable 解释器（波次 4）。versionID 空 = active。解释器自行
// 编译+求值（按 flowrun pin）。
func (s *Service) Resolve(ctx context.Context, id, versionID string) ([]controldomain.Branch, error) {
	c, err := s.repo.GetControl(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("controlapp.Resolve: %w", err)
	}
	vid := versionID
	if vid == "" {
		if c.ActiveVersionID == "" {
			return nil, controldomain.ErrNoActiveVersion
		}
		vid = c.ActiveVersionID
	}
	v, err := s.repo.GetVersion(ctx, vid)
	if err != nil {
		return nil, fmt.Errorf("controlapp.Resolve: %w", err)
	}
	return v.Branches, nil
}

// validateBranches runs domain structural checks (non-empty, ports unique, catch-all)
// then compiles every when/emit via pkg/cel (domain can't import cel-go, 原则 #3) — a
// compile error maps to ErrInvalidCEL. Fast-fail at create/edit so authoring errors never
// reach the interpreter.
//
// validateBranches 跑 domain 结构校验（非空、port 唯一、兜底），再用 pkg/cel 编译每个 when/emit
// （domain 不能 import cel-go，原则 #3）——编译错映射 ErrInvalidCEL。create/edit 期快速失败，
// 编写错绝不流到解释器。
func (s *Service) validateBranches(branches []controldomain.Branch) error {
	if err := controldomain.ValidateBranches(branches); err != nil {
		return err
	}
	for _, b := range branches {
		if _, err := celpkg.Compile(b.When); err != nil {
			return controldomain.ErrInvalidCEL
		}
		for _, expr := range b.Emit {
			if _, err := celpkg.Compile(expr); err != nil {
				return controldomain.ErrInvalidCEL
			}
		}
	}
	return nil
}
