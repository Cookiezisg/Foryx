package function

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"

	functiondomain "github.com/sunweilin/forgify/backend/internal/domain/function"
	idgenpkg "github.com/sunweilin/forgify/backend/internal/pkg/idgen"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

// CreateInput is the request shape for Service.Create.
//
// CreateInput 是 Service.Create 的请求形状。
type CreateInput struct {
	Ops             []Op
	ChangeReason    string
	ProgressBlockID string
}

// EditInput is the request shape for Service.Edit (writes a pending version).
//
// EditInput 是 Service.Edit 的请求形状（写 pending 版本）。
type EditInput struct {
	ID              string
	Ops             []Op
	ChangeReason    string
	ProgressBlockID string
}

// DirectCreateInput is the flat HTTP shape for POST /functions; CreateDirect rebuilds the canonical ops.
//
// DirectCreateInput 是 POST /functions 的扁平形状，CreateDirect 反推 canonical ops。
type DirectCreateInput struct {
	Name          string
	Description   string
	Code          string
	Tags          []string
	Parameters    []functiondomain.ParameterSpec
	ReturnSchema  map[string]any
	Dependencies  []string
	PythonVersion string
	ChangeReason  string
}

// UpdateMetaInput patches Function metadata without a version bump; nil fields are unchanged.
//
// UpdateMetaInput 改 Function 元数据不动版本，nil 字段不变。
type UpdateMetaInput struct {
	ID          string
	Name        *string
	Description *string
	Tags        *[]string
}

// List returns a paginated page of live functions for the current user.
//
// List 返当前用户活跃 function 的 cursor 分页。
func (s *Service) List(ctx context.Context, filter functiondomain.ListFilter) ([]*functiondomain.Function, string, error) {
	if _, err := reqctxpkg.RequireUserID(ctx); err != nil {
		return nil, "", fmt.Errorf("functionapp.List: %w", err)
	}
	rows, next, err := s.repo.ListFunctions(ctx, filter)
	if err != nil {
		return nil, "", fmt.Errorf("functionapp.List: %w", err)
	}
	return rows, next, nil
}

// ListAll returns every live function for the current user (no pagination).
//
// ListAll 返当前用户全部活跃 function（无分页）。
func (s *Service) ListAll(ctx context.Context) ([]*functiondomain.Function, error) {
	if _, err := reqctxpkg.RequireUserID(ctx); err != nil {
		return nil, fmt.Errorf("functionapp.ListAll: %w", err)
	}
	rows, err := s.repo.ListAllFunctions(ctx)
	if err != nil {
		return nil, fmt.Errorf("functionapp.ListAll: %w", err)
	}
	return rows, nil
}

// Search returns functions whose name / description / tags contain query (case-insensitive substring).
//
// Search 返 name / description / tags 含 query 子串（忽略大小写）的 function。
func (s *Service) Search(ctx context.Context, query string) ([]*functiondomain.Function, error) {
	all, err := s.ListAll(ctx)
	if err != nil {
		return nil, err
	}
	if query == "" {
		return all, nil
	}
	needle := strings.ToLower(query)
	out := make([]*functiondomain.Function, 0, len(all))
	for _, fn := range all {
		if strings.Contains(strings.ToLower(fn.Name), needle) ||
			strings.Contains(strings.ToLower(fn.Description), needle) {
			out = append(out, fn)
			continue
		}
		for _, tag := range fn.Tags {
			if strings.Contains(strings.ToLower(tag), needle) {
				out = append(out, fn)
				break
			}
		}
	}
	return out, nil
}

// Get fetches one function with its computed fields populated (active env state + pending if any).
//
// Get 返单 function 含计算字段（active env 状态 + 可能的 pending）。
func (s *Service) Get(ctx context.Context, id string) (*functiondomain.Function, error) {
	if _, err := reqctxpkg.RequireUserID(ctx); err != nil {
		return nil, fmt.Errorf("functionapp.Get: %w", err)
	}
	f, err := s.repo.GetFunction(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("functionapp.Get: %w", err)
	}
	s.attachComputed(ctx, f)
	return f, nil
}

func (s *Service) attachComputed(ctx context.Context, f *functiondomain.Function) {
	if f == nil {
		return
	}
	pending, err := s.repo.GetPending(ctx, f.ID)
	if err == nil {
		f.Pending = pending
	} else if !errors.Is(err, functiondomain.ErrPendingNotFound) {
		s.log.Warn("functionapp.Get: attach pending failed", zap.Any("err", err))
	}
	if f.ActiveVersionID == "" {
		return
	}
	active, err := s.repo.GetVersion(ctx, f.ActiveVersionID)
	if err != nil {
		s.log.Warn("functionapp.Get: attach active env failed", zap.Any("err", err))
		return
	}
	f.EnvStatus = active.EnvStatus
	f.EnvError = active.EnvError
	f.EnvSyncedAt = active.EnvSyncedAt
	f.EnvSyncStage = active.EnvSyncStage
	f.EnvSyncDetail = active.EnvSyncDetail
}

// Create applies ops, persists Function + auto-accepted v1, and synchronously syncs the venv.
//
// Create 应用 ops、持久化 Function + 自动 accept 的 v1、同步装 venv。
func (s *Service) Create(ctx context.Context, in CreateInput) (*functiondomain.Function, *functiondomain.Version, error) {
	uid, err := reqctxpkg.RequireUserID(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("functionapp.Create: %w", err)
	}
	if err := s.checkSandbox(); err != nil {
		return nil, nil, fmt.Errorf("functionapp.Create: %w", err)
	}
	draft, _, err := s.ApplyOps(ctx, nil, in.Ops, in.ProgressBlockID)
	if err != nil {
		return nil, nil, fmt.Errorf("functionapp.Create: %w", err)
	}
	existing, err := s.repo.GetFunctionByName(ctx, draft.Name)
	if err != nil && !errors.Is(err, functiondomain.ErrNotFound) {
		return nil, nil, fmt.Errorf("functionapp.Create: dup-check: %w", err)
	}
	if existing != nil {
		return nil, nil, functiondomain.ErrDuplicateName
	}

	now := time.Now().UTC()
	fnID := idgenpkg.New("fn")
	versionID := idgenpkg.New("fnv")
	versionN := 1
	pyVer := draft.PythonVersion
	if pyVer == "" {
		pyVer = functiondomain.DefaultPythonVersion
	}

	f := &functiondomain.Function{
		ID:              fnID,
		UserID:          uid,
		Name:            draft.Name,
		Description:     draft.Description,
		Tags:            draft.Tags,
		ActiveVersionID: versionID,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	v := &functiondomain.Version{
		ID:            versionID,
		FunctionID:    fnID,
		Status:        functiondomain.StatusAccepted,
		Version:       &versionN,
		Code:          draft.Code,
		Parameters:    draft.Parameters,
		ReturnSchema:  draft.ReturnSchema,
		Dependencies:  draft.Dependencies,
		PythonVersion: pyVer,
		EnvID:         idgenpkg.New("fnenv"),
		EnvStatus:     functiondomain.EnvStatusPending,
		ChangeReason:  in.ChangeReason,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if convID, ok := reqctxpkg.GetConversationID(ctx); ok {
		v.ForgedInConversationID = &convID
	}

	if err := s.repo.SaveFunction(ctx, f); err != nil {
		return nil, nil, fmt.Errorf("functionapp.Create: SaveFunction: %w", err)
	}
	if err := s.repo.SaveVersion(ctx, v); err != nil {
		return nil, nil, fmt.Errorf("functionapp.Create: SaveVersion: %w", err)
	}

	s.publish(ctx, fnID, "created", map[string]any{"versionId": v.ID, "versionNumber": versionN})

	if err := s.syncEnvSync(ctx, v); err != nil {
		s.log.Warn("functionapp.Create: env sync failed",
			zap.String("functionId", fnID), zap.String("versionId", versionID), zap.Error(err))
	}

	// Relation hooks: forged edge (from origin conv) + initial edited sync (suppressed on Create).
	s.syncRelationsAfterCreate(ctx, fnID, v.ForgedInConversationID)
	s.syncRelationsAfterActiveVersionChange(ctx, fnID)

	return f, v, nil
}

func (s *Service) checkSandbox() error {
	if s.sandbox.PythonPath() == "" {
		return functiondomain.ErrSandboxUnavailable
	}
	return nil
}

// CreateDirect builds an ops list from a flat definition and delegates to Create.
//
// CreateDirect 从扁平定义构 ops 再委托 Create。
func (s *Service) CreateDirect(ctx context.Context, in DirectCreateInput) (*functiondomain.Function, *functiondomain.Version, error) {
	ops, err := buildOpsFromDirect(in)
	if err != nil {
		return nil, nil, fmt.Errorf("functionapp.CreateDirect: %w", err)
	}
	return s.Create(ctx, CreateInput{Ops: ops, ChangeReason: in.ChangeReason})
}

func buildOpsFromDirect(in DirectCreateInput) ([]Op, error) {
	ops := make([]Op, 0, 6)
	raw, err := json.Marshal(map[string]any{
		"name":        in.Name,
		"description": in.Description,
		"tags":        in.Tags,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal set_meta: %w", err)
	}
	ops = append(ops, Op{Type: "set_meta", Raw: raw})

	if in.Code != "" {
		raw, err := json.Marshal(map[string]any{"code": in.Code})
		if err != nil {
			return nil, fmt.Errorf("marshal set_code: %w", err)
		}
		ops = append(ops, Op{Type: "set_code", Raw: raw})
	}
	if len(in.Parameters) > 0 {
		raw, err := json.Marshal(map[string]any{"parameters": in.Parameters})
		if err != nil {
			return nil, fmt.Errorf("marshal set_parameters: %w", err)
		}
		ops = append(ops, Op{Type: "set_parameters", Raw: raw})
	}
	if in.ReturnSchema != nil {
		raw, err := json.Marshal(map[string]any{"returnSchema": in.ReturnSchema})
		if err != nil {
			return nil, fmt.Errorf("marshal set_return_schema: %w", err)
		}
		ops = append(ops, Op{Type: "set_return_schema", Raw: raw})
	}
	if len(in.Dependencies) > 0 {
		raw, err := json.Marshal(map[string]any{"dependencies": in.Dependencies})
		if err != nil {
			return nil, fmt.Errorf("marshal set_dependencies: %w", err)
		}
		ops = append(ops, Op{Type: "set_dependencies", Raw: raw})
	}
	if in.PythonVersion != "" {
		raw, err := json.Marshal(map[string]any{"version": in.PythonVersion})
		if err != nil {
			return nil, fmt.Errorf("marshal set_python_version: %w", err)
		}
		ops = append(ops, Op{Type: "set_python_version", Raw: raw})
	}
	return ops, nil
}

// Edit produces or iterates a pending version; ops=[] is the force-rebuild-env path.
//
// Edit 产出或迭代 pending 版本；ops=[] 走强制重建 env 路径。
func (s *Service) Edit(ctx context.Context, in EditInput) (*functiondomain.Version, error) {
	if _, err := reqctxpkg.RequireUserID(ctx); err != nil {
		return nil, fmt.Errorf("functionapp.Edit: %w", err)
	}
	if err := s.checkSandbox(); err != nil {
		return nil, fmt.Errorf("functionapp.Edit: %w", err)
	}
	f, err := s.repo.GetFunction(ctx, in.ID)
	if err != nil {
		return nil, fmt.Errorf("functionapp.Edit: %w", err)
	}

	pending, perr := s.repo.GetPending(ctx, in.ID)
	switch {
	case perr == nil:
	case errors.Is(perr, functiondomain.ErrPendingNotFound):
		pending = nil
	default:
		return nil, fmt.Errorf("functionapp.Edit: pending-check: %w", perr)
	}

	if len(in.Ops) == 0 {
		var target *functiondomain.Version
		if pending != nil {
			target = pending
		} else if f.ActiveVersionID != "" {
			target, err = s.repo.GetVersion(ctx, f.ActiveVersionID)
			if err != nil {
				return nil, fmt.Errorf("functionapp.Edit: load active: %w", err)
			}
		} else {
			return nil, fmt.Errorf("functionapp.Edit: %w", functiondomain.ErrNoActiveVersion)
		}
		_ = s.sandbox.DestroyEnv(ctx, in.ID, target.EnvID)
		target.EnvStatus = functiondomain.EnvStatusPending
		target.EnvError = ""
		target.EnvSyncedAt = nil
		target.EnvSyncStage = ""
		target.EnvSyncDetail = ""
		target.UpdatedAt = time.Now().UTC()
		if err := s.syncEnvSync(ctx, target); err != nil {
			s.log.Warn("functionapp.Edit: rebuild env failed",
				zap.String("functionId", in.ID), zap.String("versionId", target.ID), zap.Error(err))
		}
		s.publish(ctx, in.ID, "env_rebuilt", map[string]any{"versionId": target.ID})
		return target, nil
	}

	var base *VersionDraft
	if pending != nil {
		base = versionToDraft(f, pending)
	} else {
		base, err = s.activeAsDraft(ctx, f)
		if err != nil {
			return nil, fmt.Errorf("functionapp.Edit: %w", err)
		}
	}
	draft, _, err := s.ApplyOps(ctx, base, in.Ops, in.ProgressBlockID)
	if err != nil {
		return nil, fmt.Errorf("functionapp.Edit: %w", err)
	}

	now := time.Now().UTC()
	pyVer := draft.PythonVersion
	if pyVer == "" {
		pyVer = functiondomain.DefaultPythonVersion
	}

	var v *functiondomain.Version
	if pending != nil {
		_ = s.sandbox.DestroyEnv(ctx, in.ID, pending.EnvID)
		pending.Code = draft.Code
		pending.Parameters = draft.Parameters
		pending.ReturnSchema = draft.ReturnSchema
		pending.Dependencies = draft.Dependencies
		pending.PythonVersion = pyVer
		pending.EnvStatus = functiondomain.EnvStatusPending
		pending.EnvError = ""
		pending.EnvSyncedAt = nil
		pending.EnvSyncStage = ""
		pending.EnvSyncDetail = ""
		pending.ChangeReason = in.ChangeReason
		pending.UpdatedAt = now
		// Latest editor: update if conv changed (different ctx vs original pending creator).
		if convID, ok := reqctxpkg.GetConversationID(ctx); ok {
			pending.ForgedInConversationID = &convID
		}
		v = pending
	} else {
		versionID := idgenpkg.New("fnv")
		v = &functiondomain.Version{
			ID:            versionID,
			FunctionID:    in.ID,
			Status:        functiondomain.StatusPending,
			Code:          draft.Code,
			Parameters:    draft.Parameters,
			ReturnSchema:  draft.ReturnSchema,
			Dependencies:  draft.Dependencies,
			PythonVersion: pyVer,
			EnvID:         idgenpkg.New("fnenv"),
			EnvStatus:     functiondomain.EnvStatusPending,
			ChangeReason:  in.ChangeReason,
			CreatedAt:     now,
			UpdatedAt:     now,
		}
		if convID, ok := reqctxpkg.GetConversationID(ctx); ok {
			v.ForgedInConversationID = &convID
		}
	}
	if err := s.repo.SaveVersion(ctx, v); err != nil {
		return nil, fmt.Errorf("functionapp.Edit: SaveVersion: %w", err)
	}
	if err := s.syncEnvSync(ctx, v); err != nil {
		s.log.Warn("functionapp.Edit: env sync failed",
			zap.String("functionId", in.ID), zap.String("versionId", v.ID), zap.Error(err))
	}
	s.publish(ctx, in.ID, "pending_created", map[string]any{"versionId": v.ID})
	return v, nil
}

func versionToDraft(f *functiondomain.Function, v *functiondomain.Version) *VersionDraft {
	return &VersionDraft{
		Name:          f.Name,
		Description:   f.Description,
		Tags:          append([]string(nil), f.Tags...),
		Code:          v.Code,
		Parameters:    append([]functiondomain.ParameterSpec(nil), v.Parameters...),
		ReturnSchema:  v.ReturnSchema,
		Dependencies:  append([]string(nil), v.Dependencies...),
		PythonVersion: v.PythonVersion,
	}
}

// AcceptPending promotes the pending version to a numbered accepted version and flips ActiveVersionID.
//
// AcceptPending 把 pending 翻为带号 accepted 并翻 ActiveVersionID。
func (s *Service) AcceptPending(ctx context.Context, id string) (*functiondomain.Version, error) {
	if _, err := reqctxpkg.RequireUserID(ctx); err != nil {
		return nil, fmt.Errorf("functionapp.AcceptPending: %w", err)
	}
	pending, err := s.repo.GetPending(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("functionapp.AcceptPending: %w", err)
	}

	nextN, err := s.nextVersionNumber(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("functionapp.AcceptPending: nextN: %w", err)
	}
	if err := s.repo.UpdateVersionStatus(ctx, pending.ID, functiondomain.StatusAccepted, &nextN); err != nil {
		return nil, fmt.Errorf("functionapp.AcceptPending: UpdateStatus: %w", err)
	}
	if err := s.repo.SetActiveVersion(ctx, id, pending.ID); err != nil {
		return nil, fmt.Errorf("functionapp.AcceptPending: SetActive: %w", err)
	}
	if err := s.repo.HardDeleteOldestAccepted(ctx, id, functiondomain.AcceptedVersionCap); err != nil {
		s.log.Warn("functionapp.AcceptPending: trim oldest failed", zap.Any("err", err), zap.Any("functionId", id))
	}

	pending.Status = functiondomain.StatusAccepted
	pending.Version = &nextN
	s.publish(ctx, id, "version_accepted", map[string]any{"versionId": pending.ID, "versionNumber": nextN})

	// Relation hook: ActiveVersionID flipped to new accepted; recompute edited edge.
	s.syncRelationsAfterActiveVersionChange(ctx, id)
	return pending, nil
}

// RejectPending destroys the pending venv and hard-deletes the pending Version row.
//
// RejectPending 销 pending 的 venv 并物理删 Version 行。
func (s *Service) RejectPending(ctx context.Context, id string) error {
	if _, err := reqctxpkg.RequireUserID(ctx); err != nil {
		return fmt.Errorf("functionapp.RejectPending: %w", err)
	}
	pending, err := s.repo.GetPending(ctx, id)
	if err != nil {
		return fmt.Errorf("functionapp.RejectPending: %w", err)
	}
	if err := s.sandbox.DestroyEnv(ctx, id, pending.EnvID); err != nil {
		s.log.Warn("functionapp.RejectPending: DestroyEnv failed (best-effort)",
			zap.String("functionId", id), zap.String("versionId", pending.ID), zap.Error(err))
	}
	if err := s.repo.HardDeleteVersion(ctx, pending.ID); err != nil {
		return fmt.Errorf("functionapp.RejectPending: %w", err)
	}
	s.publish(ctx, id, "pending_rejected", map[string]any{"versionId": pending.ID})
	return nil
}

// Revert flips ActiveVersionID to an accepted version identified by its integer number.
//
// Revert 把 ActiveVersionID 翻到指定整数号的 accepted 版本。
func (s *Service) Revert(ctx context.Context, id string, targetVersion int) (*functiondomain.Version, error) {
	if _, err := reqctxpkg.RequireUserID(ctx); err != nil {
		return nil, fmt.Errorf("functionapp.Revert: %w", err)
	}
	target, err := s.repo.GetVersionByNumber(ctx, id, targetVersion)
	if err != nil {
		return nil, fmt.Errorf("functionapp.Revert: %w", err)
	}
	if err := s.repo.SetActiveVersion(ctx, id, target.ID); err != nil {
		return nil, fmt.Errorf("functionapp.Revert: %w", err)
	}
	revertedNum := 0
	if target.Version != nil {
		revertedNum = *target.Version
	}
	s.publish(ctx, id, "reverted", map[string]any{"versionId": target.ID, "versionNumber": revertedNum})

	// Relation hook: ActiveVersionID flipped backward; recompute edited edge.
	s.syncRelationsAfterActiveVersionChange(ctx, id)
	return target, nil
}

// UpdateMeta patches Function metadata without creating a new version.
//
// UpdateMeta 改 Function 元数据不创建新版本。
func (s *Service) UpdateMeta(ctx context.Context, in UpdateMetaInput) (*functiondomain.Function, error) {
	if _, err := reqctxpkg.RequireUserID(ctx); err != nil {
		return nil, fmt.Errorf("functionapp.UpdateMeta: %w", err)
	}
	f, err := s.repo.GetFunction(ctx, in.ID)
	if err != nil {
		return nil, fmt.Errorf("functionapp.UpdateMeta: %w", err)
	}
	if in.Name != nil {
		if !validNameRe.MatchString(*in.Name) {
			return nil, fmt.Errorf("functionapp.UpdateMeta: name %q invalid", *in.Name)
		}
		if *in.Name != f.Name {
			existing, err := s.repo.GetFunctionByName(ctx, *in.Name)
			if err != nil && !errors.Is(err, functiondomain.ErrNotFound) {
				return nil, fmt.Errorf("functionapp.UpdateMeta: dup-check: %w", err)
			}
			if existing != nil && existing.ID != f.ID {
				return nil, functiondomain.ErrDuplicateName
			}
		}
		f.Name = *in.Name
	}
	if in.Description != nil {
		f.Description = *in.Description
	}
	if in.Tags != nil {
		f.Tags = *in.Tags
	}
	if err := s.repo.SaveFunction(ctx, f); err != nil {
		return nil, fmt.Errorf("functionapp.UpdateMeta: %w", err)
	}
	s.publish(ctx, f.ID, "updated", nil)
	return f, nil
}

// ListVersions returns a paginated page of versions for one function.
//
// ListVersions 返单 function 版本的 cursor 分页。
func (s *Service) ListVersions(ctx context.Context, functionID string, filter functiondomain.VersionListFilter) ([]*functiondomain.Version, string, error) {
	if _, err := reqctxpkg.RequireUserID(ctx); err != nil {
		return nil, "", fmt.Errorf("functionapp.ListVersions: %w", err)
	}
	return s.repo.ListVersions(ctx, functionID, filter)
}

// GetVersion fetches one version by id.
//
// GetVersion 按 id 取版本。
func (s *Service) GetVersion(ctx context.Context, versionID string) (*functiondomain.Version, error) {
	if _, err := reqctxpkg.RequireUserID(ctx); err != nil {
		return nil, fmt.Errorf("functionapp.GetVersion: %w", err)
	}
	return s.repo.GetVersion(ctx, versionID)
}

// GetVersionByNumber fetches one accepted version by integer number.
//
// GetVersionByNumber 按整数号取已 accepted 版本。
func (s *Service) GetVersionByNumber(ctx context.Context, functionID string, versionN int) (*functiondomain.Version, error) {
	if _, err := reqctxpkg.RequireUserID(ctx); err != nil {
		return nil, fmt.Errorf("functionapp.GetVersionByNumber: %w", err)
	}
	return s.repo.GetVersionByNumber(ctx, functionID, versionN)
}

// GetPending fetches the active pending version (or ErrPendingNotFound).
//
// GetPending 取活动 pending 版本(或 ErrPendingNotFound)。
func (s *Service) GetPending(ctx context.Context, functionID string) (*functiondomain.Version, error) {
	if _, err := reqctxpkg.RequireUserID(ctx); err != nil {
		return nil, fmt.Errorf("functionapp.GetPending: %w", err)
	}
	return s.repo.GetPending(ctx, functionID)
}

// Delete soft-deletes a function and publishes a deletion notification.
//
// Delete 软删 function 并推删除通知。
func (s *Service) Delete(ctx context.Context, id string) error {
	if _, err := reqctxpkg.RequireUserID(ctx); err != nil {
		return fmt.Errorf("functionapp.Delete: %w", err)
	}
	if err := s.repo.DeleteFunction(ctx, id); err != nil {
		return fmt.Errorf("functionapp.Delete: %w", err)
	}
	s.publish(ctx, id, "deleted", nil)
	s.purgeRelations(ctx, id)
	return nil
}

func (s *Service) activeAsDraft(ctx context.Context, f *functiondomain.Function) (*VersionDraft, error) {
	d := &VersionDraft{
		Name:        f.Name,
		Description: f.Description,
		Tags:        append([]string(nil), f.Tags...),
	}
	if f.ActiveVersionID == "" {
		return d, nil
	}
	active, err := s.repo.GetVersion(ctx, f.ActiveVersionID)
	if err != nil {
		return nil, err
	}
	d.Code = active.Code
	d.Parameters = append([]functiondomain.ParameterSpec(nil), active.Parameters...)
	d.ReturnSchema = active.ReturnSchema
	d.Dependencies = append([]string(nil), active.Dependencies...)
	d.PythonVersion = active.PythonVersion
	return d, nil
}

func (s *Service) nextVersionNumber(ctx context.Context, functionID string) (int, error) {
	rows, _, err := s.repo.ListVersions(ctx, functionID, functiondomain.VersionListFilter{
		Status: functiondomain.StatusAccepted,
		Limit:  1,
	})
	if err != nil {
		return 0, err
	}
	if len(rows) == 0 || rows[0].Version == nil {
		return 1, nil
	}
	return *rows[0].Version + 1, nil
}

func (s *Service) publish(ctx context.Context, functionID, action string, data map[string]any) {
	envelope := map[string]any{"action": action}
	for k, v := range data {
		envelope[k] = v
	}
	s.notif.Publish(ctx, "function", functionID, envelope, "")
}
