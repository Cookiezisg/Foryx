package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"go.uber.org/zap"

	handlerdomain "github.com/sunweilin/forgify/backend/internal/domain/handler"
	idgenpkg "github.com/sunweilin/forgify/backend/internal/pkg/idgen"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

// CreateInput is the request shape for Service.Create (LLM ops-driven path).
//
// CreateInput 是 Service.Create 的请求形状（LLM ops 驱动）。
type CreateInput struct {
	Ops             []Op
	ChangeReason    string
	ProgressBlockID string
}

// EditInput is the request shape for Service.Edit (writes a pending version).
//
// EditInput 是 Service.Edit 的请求形状（写 pending）。
type EditInput struct {
	ID              string
	Ops             []Op
	ChangeReason    string
	ProgressBlockID string
}

// UpdateMetaInput patches Handler metadata without a version bump.
//
// UpdateMetaInput 改 Handler 元数据不动版本。
type UpdateMetaInput struct {
	ID          string
	Name        *string
	Description *string
	Tags        *[]string
}

// DirectCreateInput is the flat HTTP shape for POST /handlers; CreateDirect rebuilds the canonical ops.
//
// DirectCreateInput 是 POST /handlers 的扁平形状，CreateDirect 反推 canonical ops。
type DirectCreateInput struct {
	Name           string
	Description    string
	Tags           []string
	Imports        string
	InitBody       string
	ShutdownBody   string
	Methods        []handlerdomain.MethodSpec
	InitArgsSchema []handlerdomain.InitArgSpec
	Dependencies   []string
	PythonVersion  string
	ChangeReason   string
}

// List returns a paginated page of live handlers for the current user.
//
// List 返当前用户活跃 handler 的 cursor 分页。
func (s *Service) List(ctx context.Context, filter handlerdomain.ListFilter) ([]*handlerdomain.Handler, string, error) {
	if _, err := reqctxpkg.RequireUserID(ctx); err != nil {
		return nil, "", fmt.Errorf("handlerapp.List: %w", err)
	}
	rows, next, err := s.repo.ListHandlers(ctx, filter)
	if err != nil {
		return nil, "", fmt.Errorf("handlerapp.List: %w", err)
	}
	return rows, next, nil
}

// ListAll returns every live handler for current user (no pagination).
//
// ListAll 返当前用户全部活跃 handler(无分页)。
func (s *Service) ListAll(ctx context.Context) ([]*handlerdomain.Handler, error) {
	if _, err := reqctxpkg.RequireUserID(ctx); err != nil {
		return nil, fmt.Errorf("handlerapp.ListAll: %w", err)
	}
	return s.repo.ListAllHandlers(ctx)
}

// Search returns handlers whose name / description / tags match query
// (case-insensitive substring). V1 simple impl;LLM tool re-ranks.
//
// Search 名/描述/tag 子串匹配(忽略大小写)。
func (s *Service) Search(ctx context.Context, query string) ([]*handlerdomain.Handler, error) {
	all, err := s.ListAll(ctx)
	if err != nil {
		return nil, err
	}
	if query == "" {
		return all, nil
	}
	needle := strings.ToLower(query)
	out := make([]*handlerdomain.Handler, 0, len(all))
	for _, h := range all {
		if strings.Contains(strings.ToLower(h.Name), needle) ||
			strings.Contains(strings.ToLower(h.Description), needle) {
			out = append(out, h)
			continue
		}
		for _, tag := range h.Tags {
			if strings.Contains(strings.ToLower(tag), needle) {
				out = append(out, h)
				break
			}
		}
	}
	return out, nil
}

// Get fetches one handler with all computed fields populated.
//
// Get 返单 handler 含全部计算字段。
func (s *Service) Get(ctx context.Context, id string) (*handlerdomain.Handler, error) {
	if _, err := reqctxpkg.RequireUserID(ctx); err != nil {
		return nil, fmt.Errorf("handlerapp.Get: %w", err)
	}
	h, err := s.repo.GetHandler(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("handlerapp.Get: %w", err)
	}
	s.attachComputed(ctx, h)
	return h, nil
}

// GetByName fetches one handler by name without computed fields.
//
// GetByName 按 name 查 handler，不填计算字段。
func (s *Service) GetByName(ctx context.Context, name string) (*handlerdomain.Handler, error) {
	if _, err := reqctxpkg.RequireUserID(ctx); err != nil {
		return nil, fmt.Errorf("handlerapp.GetByName: %w", err)
	}
	h, err := s.repo.GetHandlerByName(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("handlerapp.GetByName: %w", err)
	}
	return h, nil
}

func (s *Service) attachComputed(ctx context.Context, h *handlerdomain.Handler) {
	if h == nil {
		return
	}
	pending, err := s.repo.GetPending(ctx, h.ID)
	if err == nil {
		h.Pending = pending
	} else if !errors.Is(err, handlerdomain.ErrPendingNotFound) {
		s.log.Warn("handlerapp.attachComputed: pending fetch", zap.String("id", h.ID), zap.Error(err))
	}

	if h.ActiveVersionID == "" {
		h.ConfigState = ""
	} else {
		active, err := s.repo.GetVersion(ctx, h.ActiveVersionID)
		if err != nil {
			s.log.Warn("handlerapp.attachComputed: active fetch", zap.String("id", h.ID), zap.Error(err))
		} else {
			h.EnvStatus = active.EnvStatus
			h.EnvError = active.EnvError
			h.EnvSyncedAt = active.EnvSyncedAt
			h.EnvSyncStage = active.EnvSyncStage
			h.EnvSyncDetail = active.EnvSyncDetail
			state, _, err := s.ComputeConfigState(ctx, h.ID, active.InitArgsSchema)
			if err != nil {
				s.log.Warn("handlerapp.attachComputed: configState", zap.String("id", h.ID), zap.Error(err))
			} else {
				h.ConfigState = state
			}
		}
	}

	live := 0
	for _, om := range s.registry.Snapshot() {
		if _, ok := om[h.Name]; ok {
			live++
		}
	}
	h.LiveInstances = live
}

// Create applies ops, persists Handler + auto-accepted v1, and synchronously syncs the venv.
//
// Create 应用 ops、持久化 Handler + 自动 accept 的 v1、同步装 venv。
func (s *Service) Create(ctx context.Context, in CreateInput) (*handlerdomain.Handler, *handlerdomain.Version, error) {
	uid, err := reqctxpkg.RequireUserID(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("handlerapp.Create: %w", err)
	}
	if err := s.checkSandbox(); err != nil {
		return nil, nil, fmt.Errorf("handlerapp.Create: %w", err)
	}
	draft, _, err := s.ApplyOps(ctx, nil, in.Ops, in.ProgressBlockID)
	if err != nil {
		return nil, nil, fmt.Errorf("handlerapp.Create: %w", err)
	}
	existing, err := s.repo.GetHandlerByName(ctx, draft.Name)
	if err != nil && !errors.Is(err, handlerdomain.ErrNotFound) {
		return nil, nil, fmt.Errorf("handlerapp.Create: dup-check: %w", err)
	}
	if existing != nil {
		return nil, nil, handlerdomain.ErrDuplicateName
	}

	now := time.Now().UTC()
	hdID := idgenpkg.New("hd")
	versionID := idgenpkg.New("hdv")
	versionN := 1
	pyVer := draft.PythonVersion
	if pyVer == "" {
		pyVer = handlerdomain.DefaultPythonVersion
	}

	h := &handlerdomain.Handler{
		ID:              hdID,
		UserID:          uid,
		Name:            draft.Name,
		Description:     draft.Description,
		Tags:            draft.Tags,
		ActiveVersionID: versionID,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	v := &handlerdomain.Version{
		ID:             versionID,
		HandlerID:      hdID,
		Status:         handlerdomain.StatusAccepted,
		Version:        &versionN,
		Imports:        draft.Imports,
		InitBody:       draft.InitBody,
		ShutdownBody:   draft.ShutdownBody,
		Methods:        draft.Methods,
		InitArgsSchema: draft.InitArgsSchema,
		Dependencies:   draft.Dependencies,
		PythonVersion:  pyVer,
		EnvID:          idgenpkg.New("hdenv"),
		EnvStatus:      handlerdomain.EnvStatusPending,
		ChangeReason:   in.ChangeReason,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if convID, ok := reqctxpkg.GetConversationID(ctx); ok {
		v.ForgedInConversationID = &convID
	}

	if err := s.repo.SaveHandler(ctx, h); err != nil {
		return nil, nil, fmt.Errorf("handlerapp.Create: SaveHandler: %w", err)
	}
	if err := s.repo.SaveVersion(ctx, v); err != nil {
		return nil, nil, fmt.Errorf("handlerapp.Create: SaveVersion: %w", err)
	}

	s.publishHandlerEvent(ctx, hdID, "created", map[string]any{"versionId": v.ID, "versionNumber": versionN})

	if err := s.syncEnv(ctx, v); err != nil {
		s.log.Warn("handlerapp.Create: env sync failed",
			zap.String("handlerId", hdID), zap.String("versionId", versionID), zap.Error(err))
	}

	s.syncRelationsAfterCreate(ctx, hdID, v.ForgedInConversationID)
	s.syncRelationsAfterActiveVersionChange(ctx, hdID)

	return h, v, nil
}

// checkSandbox is a fast availability ping; PythonPath()=="" signals bootstrap failure.
//
// checkSandbox 快速 ping sandbox 可用性，PythonPath()=="" 表 bootstrap 失败。
// (D-redo-20)。Create/Edit 在 DB 写入前先调,失败硬拒。
func (s *Service) checkSandbox() error {
	if s.sandbox.PythonPath() == "" {
		return handlerdomain.ErrSandboxUnavailable
	}
	return nil
}

// CreateDirect builds ops from a flat definition + delegates to Create.
//
// CreateDirect 从扁平定义构 ops 再委托 Create。
func (s *Service) CreateDirect(ctx context.Context, in DirectCreateInput) (*handlerdomain.Handler, *handlerdomain.Version, error) {
	ops, err := buildOpsFromDirect(in)
	if err != nil {
		return nil, nil, fmt.Errorf("handlerapp.CreateDirect: %w", err)
	}
	return s.Create(ctx, CreateInput{Ops: ops, ChangeReason: in.ChangeReason})
}

func buildOpsFromDirect(in DirectCreateInput) ([]Op, error) {
	ops := make([]Op, 0, 8+len(in.Methods))

	raw, err := json.Marshal(map[string]any{
		"name":        in.Name,
		"description": in.Description,
		"tags":        in.Tags,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal set_meta: %w", err)
	}
	ops = append(ops, Op{Type: "set_meta", Raw: raw})

	if in.Imports != "" {
		raw, _ := json.Marshal(map[string]any{"imports": in.Imports})
		ops = append(ops, Op{Type: "set_imports", Raw: raw})
	}
	if in.InitBody != "" {
		raw, _ := json.Marshal(map[string]any{"initBody": in.InitBody})
		ops = append(ops, Op{Type: "set_init", Raw: raw})
	}
	if in.ShutdownBody != "" {
		raw, _ := json.Marshal(map[string]any{"shutdownBody": in.ShutdownBody})
		ops = append(ops, Op{Type: "set_shutdown", Raw: raw})
	}
	if len(in.InitArgsSchema) > 0 {
		raw, _ := json.Marshal(map[string]any{"args": in.InitArgsSchema})
		ops = append(ops, Op{Type: "set_init_args_schema", Raw: raw})
	}
	for _, m := range in.Methods {
		raw, err := json.Marshal(map[string]any{"method": m})
		if err != nil {
			return nil, fmt.Errorf("marshal add_method %q: %w", m.Name, err)
		}
		ops = append(ops, Op{Type: "add_method", Raw: raw})
	}
	if len(in.Dependencies) > 0 {
		raw, _ := json.Marshal(map[string]any{"dependencies": in.Dependencies})
		ops = append(ops, Op{Type: "set_dependencies", Raw: raw})
	}
	if in.PythonVersion != "" {
		raw, _ := json.Marshal(map[string]any{"version": in.PythonVersion})
		ops = append(ops, Op{Type: "set_python_version", Raw: raw})
	}
	return ops, nil
}

// Edit produces or iterates a pending version; ops=[] is the force-rebuild-env path.
//
// Edit 产出或迭代 pending 版本；ops=[] 走强制重建 env 路径。
func (s *Service) Edit(ctx context.Context, in EditInput) (*handlerdomain.Version, error) {
	if _, err := reqctxpkg.RequireUserID(ctx); err != nil {
		return nil, fmt.Errorf("handlerapp.Edit: %w", err)
	}
	if err := s.checkSandbox(); err != nil {
		return nil, fmt.Errorf("handlerapp.Edit: %w", err)
	}
	h, err := s.repo.GetHandler(ctx, in.ID)
	if err != nil {
		return nil, fmt.Errorf("handlerapp.Edit: %w", err)
	}

	pending, perr := s.repo.GetPending(ctx, in.ID)
	switch {
	case perr == nil:
	case errors.Is(perr, handlerdomain.ErrPendingNotFound):
		pending = nil
	default:
		return nil, fmt.Errorf("handlerapp.Edit: pending-check: %w", perr)
	}

	if len(in.Ops) == 0 {
		var target *handlerdomain.Version
		if pending != nil {
			target = pending
		} else if h.ActiveVersionID != "" {
			target, err = s.repo.GetVersion(ctx, h.ActiveVersionID)
			if err != nil {
				return nil, fmt.Errorf("handlerapp.Edit: load active: %w", err)
			}
		} else {
			return nil, fmt.Errorf("handlerapp.Edit: %w", handlerdomain.ErrNoActiveVersion)
		}
		_ = s.sandbox.DestroyEnv(ctx, in.ID, target.EnvID)
		target.EnvStatus = handlerdomain.EnvStatusPending
		target.EnvError = ""
		target.EnvSyncedAt = nil
		target.EnvSyncStage = ""
		target.EnvSyncDetail = ""
		target.UpdatedAt = time.Now().UTC()
		if err := s.syncEnv(ctx, target); err != nil {
			s.log.Warn("handlerapp.Edit: rebuild env failed",
				zap.String("handlerId", in.ID), zap.String("versionId", target.ID), zap.Error(err))
		}
		s.publishHandlerEvent(ctx, in.ID, "env_rebuilt", map[string]any{"versionId": target.ID})
		return target, nil
	}

	var base *VersionDraft
	if pending != nil {
		base = versionToDraft(h, pending)
	} else {
		base, err = s.activeAsDraft(ctx, h)
		if err != nil {
			return nil, fmt.Errorf("handlerapp.Edit: %w", err)
		}
	}
	draft, _, err := s.ApplyOps(ctx, base, in.Ops, in.ProgressBlockID)
	if err != nil {
		return nil, fmt.Errorf("handlerapp.Edit: %w", err)
	}

	now := time.Now().UTC()
	pyVer := draft.PythonVersion
	if pyVer == "" {
		pyVer = handlerdomain.DefaultPythonVersion
	}

	var v *handlerdomain.Version
	if pending != nil {
		_ = s.sandbox.DestroyEnv(ctx, in.ID, pending.EnvID)
		pending.Imports = draft.Imports
		pending.InitBody = draft.InitBody
		pending.ShutdownBody = draft.ShutdownBody
		pending.Methods = draft.Methods
		pending.InitArgsSchema = draft.InitArgsSchema
		pending.Dependencies = draft.Dependencies
		pending.PythonVersion = pyVer
		pending.EnvStatus = handlerdomain.EnvStatusPending
		pending.EnvError = ""
		pending.EnvSyncedAt = nil
		pending.EnvSyncStage = ""
		pending.EnvSyncDetail = ""
		pending.ChangeReason = in.ChangeReason
		pending.UpdatedAt = now
		if convID, ok := reqctxpkg.GetConversationID(ctx); ok {
			pending.ForgedInConversationID = &convID
		}
		v = pending
	} else {
		versionID := idgenpkg.New("hdv")
		v = &handlerdomain.Version{
			ID:             versionID,
			HandlerID:      in.ID,
			Status:         handlerdomain.StatusPending,
			Imports:        draft.Imports,
			InitBody:       draft.InitBody,
			ShutdownBody:   draft.ShutdownBody,
			Methods:        draft.Methods,
			InitArgsSchema: draft.InitArgsSchema,
			Dependencies:   draft.Dependencies,
			PythonVersion:  pyVer,
			EnvID:          idgenpkg.New("hdenv"),
			EnvStatus:      handlerdomain.EnvStatusPending,
			ChangeReason:   in.ChangeReason,
			CreatedAt:      now,
			UpdatedAt:      now,
		}
		if convID, ok := reqctxpkg.GetConversationID(ctx); ok {
			v.ForgedInConversationID = &convID
		}
	}
	if err := s.repo.SaveVersion(ctx, v); err != nil {
		return nil, fmt.Errorf("handlerapp.Edit: SaveVersion: %w", err)
	}
	if err := s.syncEnv(ctx, v); err != nil {
		s.log.Warn("handlerapp.Edit: env sync failed",
			zap.String("handlerId", in.ID), zap.String("versionId", v.ID), zap.Error(err))
	}
	s.publishHandlerEvent(ctx, in.ID, "pending_created", map[string]any{"versionId": v.ID})
	return v, nil
}

func versionToDraft(h *handlerdomain.Handler, v *handlerdomain.Version) *VersionDraft {
	return &VersionDraft{
		Name:           h.Name,
		Description:    h.Description,
		Tags:           append([]string(nil), h.Tags...),
		Imports:        v.Imports,
		InitBody:       v.InitBody,
		ShutdownBody:   v.ShutdownBody,
		Methods:        append([]handlerdomain.MethodSpec(nil), v.Methods...),
		InitArgsSchema: append([]handlerdomain.InitArgSpec(nil), v.InitArgsSchema...),
		Dependencies:   append([]string(nil), v.Dependencies...),
		PythonVersion:  v.PythonVersion,
	}
}

// AcceptPending promotes the pending version to a numbered accepted version.
//
// AcceptPending 把 pending 翻为带号 accepted。
func (s *Service) AcceptPending(ctx context.Context, id string) (*handlerdomain.Version, error) {
	if _, err := reqctxpkg.RequireUserID(ctx); err != nil {
		return nil, fmt.Errorf("handlerapp.AcceptPending: %w", err)
	}
	pending, err := s.repo.GetPending(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("handlerapp.AcceptPending: %w", err)
	}
	nextN, err := s.nextVersionNumber(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("handlerapp.AcceptPending: nextN: %w", err)
	}
	if err := s.repo.UpdateVersionStatus(ctx, pending.ID, handlerdomain.StatusAccepted, &nextN); err != nil {
		return nil, fmt.Errorf("handlerapp.AcceptPending: UpdateStatus: %w", err)
	}
	if err := s.repo.SetActiveVersion(ctx, id, pending.ID); err != nil {
		return nil, fmt.Errorf("handlerapp.AcceptPending: SetActive: %w", err)
	}
	if err := s.repo.HardDeleteOldestAccepted(ctx, id, handlerdomain.AcceptedVersionCap); err != nil {
		s.log.Warn("handlerapp.AcceptPending: trim oldest", zap.String("id", id), zap.Error(err))
	}

	pending.Status = handlerdomain.StatusAccepted
	pending.Version = &nextN
	s.publishHandlerEvent(ctx, id, "version_accepted", map[string]any{"versionId": pending.ID, "versionNumber": nextN})
	s.syncRelationsAfterActiveVersionChange(ctx, id)
	return pending, nil
}

// RejectPending destroys the pending venv and hard-deletes the pending Version row.
//
// RejectPending 销 pending 的 venv 并物理删 Version 行。
func (s *Service) RejectPending(ctx context.Context, id string) error {
	if _, err := reqctxpkg.RequireUserID(ctx); err != nil {
		return fmt.Errorf("handlerapp.RejectPending: %w", err)
	}
	pending, err := s.repo.GetPending(ctx, id)
	if err != nil {
		return fmt.Errorf("handlerapp.RejectPending: %w", err)
	}
	if err := s.sandbox.DestroyEnv(ctx, id, pending.EnvID); err != nil {
		s.log.Warn("handlerapp.RejectPending: DestroyEnv failed (best-effort)",
			zap.String("handlerId", id), zap.String("versionId", pending.ID), zap.Error(err))
	}
	if err := s.repo.HardDeleteVersion(ctx, pending.ID); err != nil {
		return fmt.Errorf("handlerapp.RejectPending: %w", err)
	}
	s.publishHandlerEvent(ctx, id, "pending_rejected", map[string]any{"versionId": pending.ID})
	return nil
}

// Revert flips ActiveVersionID to an accepted version identified by its integer number.
//
// Revert 把 ActiveVersionID 翻到指定整数号的 accepted 版本。
func (s *Service) Revert(ctx context.Context, id string, targetVersion int) (*handlerdomain.Version, error) {
	if _, err := reqctxpkg.RequireUserID(ctx); err != nil {
		return nil, fmt.Errorf("handlerapp.Revert: %w", err)
	}
	target, err := s.repo.GetVersionByNumber(ctx, id, targetVersion)
	if err != nil {
		return nil, fmt.Errorf("handlerapp.Revert: %w", err)
	}
	if err := s.repo.SetActiveVersion(ctx, id, target.ID); err != nil {
		return nil, fmt.Errorf("handlerapp.Revert: %w", err)
	}
	revertedNum := 0
	if target.Version != nil {
		revertedNum = *target.Version
	}
	s.publishHandlerEvent(ctx, id, "reverted", map[string]any{"versionId": target.ID, "versionNumber": revertedNum})
	s.syncRelationsAfterActiveVersionChange(ctx, id)
	return target, nil
}

// UpdateMeta patches Handler metadata without creating a version.
//
// UpdateMeta 改 Handler 元数据不创建版本。
func (s *Service) UpdateMeta(ctx context.Context, in UpdateMetaInput) (*handlerdomain.Handler, error) {
	if _, err := reqctxpkg.RequireUserID(ctx); err != nil {
		return nil, fmt.Errorf("handlerapp.UpdateMeta: %w", err)
	}
	h, err := s.repo.GetHandler(ctx, in.ID)
	if err != nil {
		return nil, fmt.Errorf("handlerapp.UpdateMeta: %w", err)
	}
	if in.Name != nil {
		if !validNameRe.MatchString(*in.Name) {
			return nil, fmt.Errorf("handlerapp.UpdateMeta: name %q invalid: %w", *in.Name, handlerdomain.ErrOpInvalid)
		}
		if *in.Name != h.Name {
			existing, err := s.repo.GetHandlerByName(ctx, *in.Name)
			if err != nil && !errors.Is(err, handlerdomain.ErrNotFound) {
				return nil, fmt.Errorf("handlerapp.UpdateMeta: dup-check: %w", err)
			}
			if existing != nil && existing.ID != h.ID {
				return nil, handlerdomain.ErrDuplicateName
			}
		}
		h.Name = *in.Name
	}
	if in.Description != nil {
		h.Description = *in.Description
	}
	if in.Tags != nil {
		h.Tags = *in.Tags
	}
	if err := s.repo.SaveHandler(ctx, h); err != nil {
		return nil, fmt.Errorf("handlerapp.UpdateMeta: %w", err)
	}
	s.publishHandlerEvent(ctx, h.ID, "updated", nil)
	return h, nil
}

// Delete soft-deletes a handler and tears down live instances across owners.
//
// Delete 软删 handler 并销毁所有 owner 的活跃 instance。
func (s *Service) Delete(ctx context.Context, id string) error {
	if _, err := reqctxpkg.RequireUserID(ctx); err != nil {
		return fmt.Errorf("handlerapp.Delete: %w", err)
	}
	h, err := s.repo.GetHandler(ctx, id)
	if err != nil {
		return fmt.Errorf("handlerapp.Delete: %w", err)
	}
	if err := s.repo.DeleteHandler(ctx, id); err != nil {
		return fmt.Errorf("handlerapp.Delete: %w", err)
	}
	for owner, om := range s.registry.Snapshot() {
		if _, ok := om[h.Name]; ok {
			s.registry.DestroyOwner(ctx, owner)
		}
	}
	s.publishHandlerEvent(ctx, id, "deleted", nil)
	s.purgeRelations(ctx, id)
	return nil
}

// ListVersions returns paginated versions for a handler.
//
// ListVersions 返回某 handler 的版本分页。
func (s *Service) ListVersions(ctx context.Context, handlerID string, filter handlerdomain.VersionListFilter) ([]*handlerdomain.Version, string, error) {
	if _, err := reqctxpkg.RequireUserID(ctx); err != nil {
		return nil, "", fmt.Errorf("handlerapp.ListVersions: %w", err)
	}
	return s.repo.ListVersions(ctx, handlerID, filter)
}

// GetVersion fetches one version by id.
//
// GetVersion 按 id 取版本。
func (s *Service) GetVersion(ctx context.Context, versionID string) (*handlerdomain.Version, error) {
	if _, err := reqctxpkg.RequireUserID(ctx); err != nil {
		return nil, fmt.Errorf("handlerapp.GetVersion: %w", err)
	}
	return s.repo.GetVersion(ctx, versionID)
}

// GetVersionByNumber fetches an accepted version by integer number.
//
// GetVersionByNumber 按整数号取 accepted 版本。
func (s *Service) GetVersionByNumber(ctx context.Context, handlerID string, versionN int) (*handlerdomain.Version, error) {
	if _, err := reqctxpkg.RequireUserID(ctx); err != nil {
		return nil, fmt.Errorf("handlerapp.GetVersionByNumber: %w", err)
	}
	return s.repo.GetVersionByNumber(ctx, handlerID, versionN)
}

// GetPending returns the active pending version.
//
// GetPending 返回活动 pending。
func (s *Service) GetPending(ctx context.Context, handlerID string) (*handlerdomain.Version, error) {
	if _, err := reqctxpkg.RequireUserID(ctx); err != nil {
		return nil, fmt.Errorf("handlerapp.GetPending: %w", err)
	}
	return s.repo.GetPending(ctx, handlerID)
}

func (s *Service) activeAsDraft(ctx context.Context, h *handlerdomain.Handler) (*VersionDraft, error) {
	d := &VersionDraft{
		Name:        h.Name,
		Description: h.Description,
		Tags:        append([]string(nil), h.Tags...),
	}
	if h.ActiveVersionID == "" {
		return d, nil
	}
	active, err := s.repo.GetVersion(ctx, h.ActiveVersionID)
	if err != nil {
		return nil, err
	}
	d.Imports = active.Imports
	d.InitBody = active.InitBody
	d.ShutdownBody = active.ShutdownBody
	d.Methods = append([]handlerdomain.MethodSpec(nil), active.Methods...)
	d.InitArgsSchema = append([]handlerdomain.InitArgSpec(nil), active.InitArgsSchema...)
	d.Dependencies = append([]string(nil), active.Dependencies...)
	d.PythonVersion = active.PythonVersion
	return d, nil
}

func (s *Service) nextVersionNumber(ctx context.Context, handlerID string) (int, error) {
	rows, _, err := s.repo.ListVersions(ctx, handlerID, handlerdomain.VersionListFilter{
		Status: handlerdomain.StatusAccepted,
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

var _ = regexp.MustCompile
