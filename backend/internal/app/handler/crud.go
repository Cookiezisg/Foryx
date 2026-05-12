// crud.go — Handler CRUD + version lifecycle (pending → accept / reject /
// revert) + metadata update + soft-delete at the Service layer.
//
// Mirrors function/crud.go structure. Handler-specific differences:
//   - Version has methods + init_args_schema (no whole-code-file)
//   - attachComputed also fills ConfigState by calling ComputeConfigState
//     against active version's InitArgsSchema (D-handler)
//   - Delete cascades to instance registry: DestroyAllOwnersOf the handler
//     (we don't have a per-handler scoped registry index in V1 — just iterate
//     the snapshot)
//
// crud.go —— Handler CRUD + 版本生命周期(pending → accept/reject/revert)+
// 元数据更新 + 软删。差异:Version 含 methods + init_args_schema;
// attachComputed 计算 ConfigState;Delete 时级联销毁所有 owner 持有的 instance。

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

// ── Input types ───────────────────────────────────────────────────────────────

// CreateInput is the request shape for Service.Create (LLM ops-driven path).
//
// CreateInput Service.Create 的请求形状(LLM ops 驱动路径)。
type CreateInput struct {
	Ops             []Op
	ChangeReason    string
	ProgressBlockID string
}

// EditInput is the request shape for Service.Edit (writes a pending version).
//
// EditInput Service.Edit 的请求形状(写 pending)。
type EditInput struct {
	ID              string
	Ops             []Op
	ChangeReason    string
	ProgressBlockID string
}

// UpdateMetaInput patches Handler-level metadata (no version side effects).
//
// UpdateMetaInput 改 Handler 元数据(不改版本)。
type UpdateMetaInput struct {
	ID          string
	Name        *string
	Description *string
	Tags        *[]string
}

// DirectCreateInput is the HTTP-friendly flat shape for POST /handlers
// (curl/UI use). Service.CreateDirect translates to canonical ops then
// delegates to Create.
//
// DirectCreateInput POST /handlers 的扁平形状;CreateDirect 转 ops 再 Create。
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

// ── Reads ─────────────────────────────────────────────────────────────────────

// List returns a paginated page of live handlers for current user.
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

// Get fetches one handler with all computed fields populated (Pending,
// active env state, ConfigState, LiveInstances).
//
// Get 返单 handler 含全部计算字段(Pending / active env / ConfigState /
// LiveInstances)。
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

// attachComputed fills Pending + Env* + ConfigState + LiveInstances on a
// Handler row. Best-effort: individual fetch failures log a warning but
// don't fail the parent call.
//
// attachComputed 填 Pending + Env* + ConfigState + LiveInstances。
// 单项失败 warn log,不挂主调用。
func (s *Service) attachComputed(ctx context.Context, h *handlerdomain.Handler) {
	if h == nil {
		return
	}
	// Pending
	pending, err := s.repo.GetPending(ctx, h.ID)
	if err == nil {
		h.Pending = pending
	} else if !errors.Is(err, handlerdomain.ErrPendingNotFound) {
		s.log.Warn("handlerapp.attachComputed: pending fetch", zap.String("id", h.ID), zap.Error(err))
	}

	// Active env mirror + ConfigState (depends on active version's schema).
	if h.ActiveVersionID == "" {
		// No active version → can't compute ConfigState meaningfully.
		// No active 版本 → ConfigState 没意义。
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

	// LiveInstances: count across all owners. Cheap iteration via Snapshot.
	// LiveInstances 跨 owner 数;Snapshot 一次扫。
	live := 0
	for _, om := range s.registry.Snapshot() {
		if _, ok := om[h.Name]; ok {
			live++
		}
	}
	h.LiveInstances = live
}

// ── Lifecycle ─────────────────────────────────────────────────────────────────

// Create applies ops to an empty draft + persists Handler + v1 auto-accepted
// version. ConfigEncrypted starts empty (config flow is separate).
//
// Create 应用 ops → 持久化 Handler + v1 auto-accepted 版本;ConfigEncrypted
// 起步空(config 流独立)。
func (s *Service) Create(ctx context.Context, in CreateInput) (*handlerdomain.Handler, *handlerdomain.Version, error) {
	uid, err := reqctxpkg.RequireUserID(ctx)
	if err != nil {
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
	envID := ComputeEnvID(draft.Dependencies, pyVer)

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
		EnvID:          envID,
		EnvStatus:      handlerdomain.EnvStatusPending,
		ChangeReason:   in.ChangeReason,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	if err := s.repo.SaveHandler(ctx, h); err != nil {
		return nil, nil, fmt.Errorf("handlerapp.Create: SaveHandler: %w", err)
	}
	if err := s.repo.SaveVersion(ctx, v); err != nil {
		return nil, nil, fmt.Errorf("handlerapp.Create: SaveVersion: %w", err)
	}

	s.publishHandlerEvent(ctx, hdID, "created", map[string]any{"handler": h, "version": v})
	return h, v, nil
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

// buildOpsFromDirect marshals direct fields to canonical ops:
// set_meta → set_imports → set_init → set_shutdown → set_init_args_schema →
// add_method × N → set_dependencies → set_python_version. Empty fields skipped.
//
// buildOpsFromDirect 转扁平字段为 canonical ops;空字段跳。
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
		raw, _ := json.Marshal(map[string]any{"init_body": in.InitBody})
		ops = append(ops, Op{Type: "set_init", Raw: raw})
	}
	if in.ShutdownBody != "" {
		raw, _ := json.Marshal(map[string]any{"shutdown_body": in.ShutdownBody})
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
		raw, _ := json.Marshal(map[string]any{"deps": in.Dependencies})
		ops = append(ops, Op{Type: "set_dependencies", Raw: raw})
	}
	if in.PythonVersion != "" {
		raw, _ := json.Marshal(map[string]any{"version": in.PythonVersion})
		ops = append(ops, Op{Type: "set_python_version", Raw: raw})
	}
	return ops, nil
}

// Edit writes a new pending version. Errors with ErrPendingConflict if a
// pending already exists (LLM/UI must accept or reject first).
//
// Edit 写新 pending;已有 pending 时返 ErrPendingConflict。
func (s *Service) Edit(ctx context.Context, in EditInput) (*handlerdomain.Version, error) {
	if _, err := reqctxpkg.RequireUserID(ctx); err != nil {
		return nil, fmt.Errorf("handlerapp.Edit: %w", err)
	}
	h, err := s.repo.GetHandler(ctx, in.ID)
	if err != nil {
		return nil, fmt.Errorf("handlerapp.Edit: %w", err)
	}
	if _, err := s.repo.GetPending(ctx, in.ID); err == nil {
		return nil, fmt.Errorf("handlerapp.Edit: %w", handlerdomain.ErrPendingConflict)
	} else if !errors.Is(err, handlerdomain.ErrPendingNotFound) {
		return nil, fmt.Errorf("handlerapp.Edit: pending-check: %w", err)
	}

	base, err := s.activeAsDraft(ctx, h)
	if err != nil {
		return nil, fmt.Errorf("handlerapp.Edit: %w", err)
	}
	draft, _, err := s.ApplyOps(ctx, base, in.Ops, in.ProgressBlockID)
	if err != nil {
		return nil, fmt.Errorf("handlerapp.Edit: %w", err)
	}

	now := time.Now().UTC()
	versionID := idgenpkg.New("hdv")
	pyVer := draft.PythonVersion
	if pyVer == "" {
		pyVer = handlerdomain.DefaultPythonVersion
	}
	envID := ComputeEnvID(draft.Dependencies, pyVer)

	v := &handlerdomain.Version{
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
		EnvID:          envID,
		EnvStatus:      handlerdomain.EnvStatusPending,
		ChangeReason:   in.ChangeReason,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := s.repo.SaveVersion(ctx, v); err != nil {
		return nil, fmt.Errorf("handlerapp.Edit: SaveVersion: %w", err)
	}
	s.publishHandlerEvent(ctx, in.ID, "pending_created", map[string]any{"version": v})
	return v, nil
}

// AcceptPending turns the active pending into a numbered accepted version,
// flips Handler.ActiveVersionID, enforces AcceptedVersionCap.
//
// AcceptPending 翻 pending 为带号 accepted + 翻 ActiveVersionID + 应用 cap。
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
	s.publishHandlerEvent(ctx, id, "version_accepted", map[string]any{"version": pending})
	return pending, nil
}

// RejectPending marks the active pending as rejected (no ActiveVersion change).
//
// RejectPending pending → rejected;不动 ActiveVersion。
func (s *Service) RejectPending(ctx context.Context, id string) error {
	if _, err := reqctxpkg.RequireUserID(ctx); err != nil {
		return fmt.Errorf("handlerapp.RejectPending: %w", err)
	}
	pending, err := s.repo.GetPending(ctx, id)
	if err != nil {
		return fmt.Errorf("handlerapp.RejectPending: %w", err)
	}
	if err := s.repo.UpdateVersionStatus(ctx, pending.ID, handlerdomain.StatusRejected, nil); err != nil {
		return fmt.Errorf("handlerapp.RejectPending: %w", err)
	}
	s.publishHandlerEvent(ctx, id, "pending_rejected", map[string]any{"versionId": pending.ID})
	return nil
}

// Revert flips ActiveVersionID to a target accepted version number.
//
// Revert 翻 ActiveVersionID 到指定 accepted 版本号。
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
	s.publishHandlerEvent(ctx, id, "reverted", map[string]any{"version": target})
	return target, nil
}

// UpdateMeta patches Handler metadata (name/description/tags) without
// creating a version. Enforces duplicate-name + name char-set.
//
// UpdateMeta 改元数据(name/description/tags)不创建版本;校验重名 + 字符集。
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
	s.publishHandlerEvent(ctx, h.ID, "updated", map[string]any{"handler": h})
	return h, nil
}

// Delete soft-deletes a handler + tears down any live instance across owners.
//
// Delete 软删 + 跨 owner 销毁所有 instance。
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
	// Tear down any live instances scoped to any owner.
	// 跨 owner 关掉活的 instance(直接 iterate snapshot 找此 handler.Name)。
	for owner, om := range s.registry.Snapshot() {
		if _, ok := om[h.Name]; ok {
			s.registry.DestroyOwner(ctx, owner)
		}
	}
	s.publishHandlerEvent(ctx, id, "deleted", nil)
	return nil
}

// ── Versions/Pending pass-throughs ───────────────────────────────────────────

// ListVersions returns paginated versions for a handler.
//
// ListVersions 返某 handler 版本分页。
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

// GetVersionByNumber fetches accepted version by integer.
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
// GetPending 返活动 pending。
func (s *Service) GetPending(ctx context.Context, handlerID string) (*handlerdomain.Version, error) {
	if _, err := reqctxpkg.RequireUserID(ctx); err != nil {
		return nil, fmt.Errorf("handlerapp.GetPending: %w", err)
	}
	return s.repo.GetPending(ctx, handlerID)
}

// ── Helpers ───────────────────────────────────────────────────────────────────

// activeAsDraft loads the handler's active version into a VersionDraft for
// Edit's base. Returns a name/desc/tags-only draft when no active version.
//
// activeAsDraft 把 active 版本加载为 Edit 的 base draft。
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

// nextVersionNumber returns max(accepted.version)+1 for the handler.
//
// nextVersionNumber 返某 handler 的 max(accepted.version)+1。
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

// validNameRe must match validateIncremental's regex (re-exported here so
// UpdateMeta can validate name before save).
//
// 注意:此处的 validNameRe 跟 validate.go 中的同义。UpdateMeta 用,validate 用。
var _ = regexp.MustCompile // keep import; the regex itself is declared in validate.go
