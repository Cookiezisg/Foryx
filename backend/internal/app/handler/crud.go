package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"

	envfixapp "github.com/sunweilin/anselm/backend/internal/app/envfix"
	handlerdomain "github.com/sunweilin/anselm/backend/internal/domain/handler"
	idgenpkg "github.com/sunweilin/anselm/backend/internal/pkg/idgen"
	reqctxpkg "github.com/sunweilin/anselm/backend/internal/pkg/reqctx"
)

// CreateInput is the LLM-build create payload; Progress (optional) streams env-fix attempts.
type CreateInput struct {
	Ops          []Op
	ChangeReason string
	Progress     envfixapp.Sink
}

// EditInput is the edit payload; empty Ops rebuilds the active version's env + restarts.
type EditInput struct {
	ID           string
	Ops          []Op
	ChangeReason string
	Progress     envfixapp.Sink
}

// DirectCreateInput is the flat HTTP create shape; CreateDirect rebuilds canonical ops.
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

// UpdateMetaInput patches handler metadata without a version bump; nil = unchanged.
type UpdateMetaInput struct {
	ID          string
	Name        *string
	Description *string
	Tags        *[]string
}

func (s *Service) List(ctx context.Context, filter handlerdomain.ListFilter) ([]*handlerdomain.Handler, string, error) {
	return s.repo.ListHandlers(ctx, filter)
}

func (s *Service) ListAll(ctx context.Context) ([]*handlerdomain.Handler, error) {
	return s.repo.ListAllHandlers(ctx)
}

// Search filters live handlers by case-insensitive substring over name / description / tags.
func (s *Service) Search(ctx context.Context, query string) ([]*handlerdomain.Handler, error) {
	all, err := s.repo.ListAllHandlers(ctx)
	if err != nil {
		return nil, fmt.Errorf("handlerapp.Search: %w", err)
	}
	if strings.TrimSpace(query) == "" {
		return all, nil
	}
	needle := strings.ToLower(query)
	out := make([]*handlerdomain.Handler, 0, len(all))
	for _, h := range all {
		if strings.Contains(strings.ToLower(h.Name), needle) || strings.Contains(strings.ToLower(h.Description), needle) {
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

// Get returns one handler with active version + config state + runtime (instance) state.
func (s *Service) Get(ctx context.Context, id string) (*handlerdomain.Handler, error) {
	h, err := s.repo.GetHandler(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("handlerapp.Get: %w", err)
	}
	s.attach(ctx, h)
	return h, nil
}

func (s *Service) attach(ctx context.Context, h *handlerdomain.Handler) {
	if h.ActiveVersionID != "" {
		if v, verr := s.repo.GetVersion(ctx, h.ActiveVersionID); verr == nil {
			h.ActiveVersion = v
			if state, missing, cerr := s.ComputeConfigState(ctx, h.ID, v.InitArgsSchema); cerr == nil {
				h.ConfigState = state
				h.MissingConfig = missing
			}
		}
	}
	h.RuntimeState = s.manager.State(h.ID)
}

// Create applies ops, persists Handler + v1 (active), materializes its env. It does NOT
// spawn the resident instance — that happens when config is complete (UpdateConfig
// restart), at Boot, or lazily on first call.
//
// Create 应用 ops、持久化 Handler + v1（active）、物化 env。**不 spawn 常驻实例**——那发生在
// config 配齐（UpdateConfig 重启）、Boot、或首调懒起时。
func (s *Service) Create(ctx context.Context, in CreateInput) (*handlerdomain.Handler, *handlerdomain.Version, error) {
	if !s.runner.Ready() {
		return nil, nil, handlerdomain.ErrSandboxUnavailable
	}
	draft, _, err := s.ApplyOps(nil, in.Ops)
	if err != nil {
		return nil, nil, fmt.Errorf("handlerapp.Create: %w", err)
	}
	now := time.Now().UTC()
	hID := idgenpkg.New("hd")
	versionID := idgenpkg.New("hdv")
	h := &handlerdomain.Handler{
		ID: hID, Name: draft.Name, Description: draft.Description, Tags: draft.Tags,
		ActiveVersionID: versionID, CreatedAt: now, UpdatedAt: now,
	}
	v := newVersionFromDraft(versionID, hID, 1, draft, in.ChangeReason, now)
	if convID, ok := reqctxpkg.GetConversationID(ctx); ok {
		v.BuiltInConversationID = &convID
	}

	if err := s.repo.CreateWithVersion(ctx, h, v); err != nil {
		return nil, nil, fmt.Errorf("handlerapp.Create: %w", err)
	}
	s.publish(ctx, "created", hID, map[string]any{"versionId": versionID, "version": 1})

	s.ensureEnv(ctx, v, in.Progress)
	s.syncBuiltEdge(ctx, hID, v.BuiltInConversationID)

	h.ActiveVersion = v
	return h, v, nil
}

// CreateDirect builds canonical ops from a flat HTTP payload and delegates to Create.
func (s *Service) CreateDirect(ctx context.Context, in DirectCreateInput) (*handlerdomain.Handler, *handlerdomain.Version, error) {
	ops, err := buildOpsFromDirect(in)
	if err != nil {
		return nil, nil, fmt.Errorf("handlerapp.CreateDirect: %w", err)
	}
	return s.Create(ctx, CreateInput{Ops: ops, ChangeReason: in.ChangeReason})
}

// Edit writes a new version (based on active), moves the active pointer, rebuilds env,
// and restarts the resident instance so it runs the new code. Empty Ops just rebuilds
// the env + restarts (retry a failed install).
//
// Edit 写新版本（基于 active）、移指针、重建 env、重启常驻实例跑新代码。空 Ops 仅重建 env + 重启。
func (s *Service) Edit(ctx context.Context, in EditInput) (*handlerdomain.Version, error) {
	if !s.runner.Ready() {
		return nil, handlerdomain.ErrSandboxUnavailable
	}
	h, err := s.repo.GetHandler(ctx, in.ID)
	if err != nil {
		return nil, fmt.Errorf("handlerapp.Edit: %w", err)
	}

	if len(in.Ops) == 0 {
		if h.ActiveVersionID == "" {
			return nil, fmt.Errorf("handlerapp.Edit: %w", handlerdomain.ErrNoActiveVersion)
		}
		active, gerr := s.repo.GetVersion(ctx, h.ActiveVersionID)
		if gerr != nil {
			return nil, fmt.Errorf("handlerapp.Edit: %w", gerr)
		}
		s.ensureEnv(ctx, active, in.Progress)
		s.restart(ctx, in.ID)
		s.publish(ctx, "env_rebuilt", in.ID, map[string]any{"versionId": active.ID})
		return active, nil
	}

	base, err := s.activeAsDraft(ctx, h)
	if err != nil {
		return nil, fmt.Errorf("handlerapp.Edit: %w", err)
	}
	draft, _, err := s.ApplyOps(base, in.Ops)
	if err != nil {
		return nil, fmt.Errorf("handlerapp.Edit: %w", err)
	}
	nextN, err := s.nextVersionNumber(ctx, in.ID)
	if err != nil {
		return nil, fmt.Errorf("handlerapp.Edit: %w", err)
	}
	now := time.Now().UTC()
	versionID := idgenpkg.New("hdv")
	v := newVersionFromDraft(versionID, in.ID, nextN, draft, in.ChangeReason, now)
	if convID, ok := reqctxpkg.GetConversationID(ctx); ok {
		v.BuiltInConversationID = &convID
	}
	// Carry the draft's meta (a set_meta op in the edit) onto the handler row, persisted with the
	// version+pointer in one tx — otherwise an edit-time rename/redescribe is silently dropped.
	//
	// 把 draft 的 meta（edit 里的 set_meta）带回 handler 行，与版本+指针同事务落——否则 edit 改名/改述被静默丢。
	h.Name, h.Description, h.Tags = draft.Name, draft.Description, draft.Tags
	if err := s.repo.SaveVersionAndActivate(ctx, v, h); err != nil {
		return nil, fmt.Errorf("handlerapp.Edit: %w", err)
	}
	if err := s.repo.TrimOldestVersions(ctx, in.ID, handlerdomain.VersionCap); err != nil {
		s.log.Warn("handlerapp.Edit: trim versions failed", zap.String("handlerId", in.ID), zap.Error(err))
	}
	s.publish(ctx, "edited", in.ID, map[string]any{"versionId": versionID, "version": nextN})

	s.ensureEnv(ctx, v, in.Progress)
	s.restart(ctx, in.ID) // resident instance must reload the new class code
	s.syncEditedEdge(ctx, in.ID)
	return v, nil
}

// Revert moves the active pointer to an existing version by number (pure pointer op)
// and restarts the resident instance so it runs the reverted code.
//
// Revert 按号把 active 指针移到已有版本（纯指针）并重启常驻实例跑回退后的代码。
func (s *Service) Revert(ctx context.Context, id string, targetVersion int) (*handlerdomain.Version, error) {
	target, err := s.repo.GetVersionByNumber(ctx, id, targetVersion)
	if err != nil {
		return nil, fmt.Errorf("handlerapp.Revert: %w", err)
	}
	if err := s.repo.SetActiveVersion(ctx, id, target.ID); err != nil {
		return nil, fmt.Errorf("handlerapp.Revert: %w", err)
	}
	s.publish(ctx, "reverted", id, map[string]any{"versionId": target.ID, "version": targetVersion})
	s.restart(ctx, id)
	s.syncEditedEdge(ctx, id)
	return target, nil
}

// Restart manually restarts the resident instance (the LLM/user "this is broken, restart
// it" path). Returns the fresh runtime state.
//
// Restart 手动重启常驻实例（LLM/用户"坏了重启"路径）。返回新的运行态。
func (s *Service) Restart(ctx context.Context, id string) (string, error) {
	if _, err := s.repo.GetHandler(ctx, id); err != nil {
		return "", fmt.Errorf("handlerapp.Restart: %w", err)
	}
	if _, err := s.manager.Restart(ctx, id); err != nil {
		s.publish(ctx, "restarted", id, map[string]any{"ok": false})
		return s.manager.State(id), fmt.Errorf("handlerapp.Restart: %w", err)
	}
	s.publish(ctx, "restarted", id, map[string]any{"ok": true})
	return s.manager.State(id), nil
}

// restart is the internal best-effort restart used after edit/revert (logs, never fails the op).
//
// restart 是 edit/revert 后内部 best-effort 重启（log，绝不让操作失败）。
func (s *Service) restart(ctx context.Context, id string) {
	if _, err := s.manager.Restart(ctx, id); err != nil {
		s.log.Info("handlerapp: resident instance not restarted (likely needs config / env)", zap.String("handlerId", id), zap.Error(err))
	}
}

func (s *Service) UpdateMeta(ctx context.Context, in UpdateMetaInput) (*handlerdomain.Handler, error) {
	h, err := s.repo.GetHandler(ctx, in.ID)
	if err != nil {
		return nil, fmt.Errorf("handlerapp.UpdateMeta: %w", err)
	}
	if in.Name != nil {
		if !validNameRe.MatchString(*in.Name) {
			return nil, fmt.Errorf("handlerapp.UpdateMeta: %w: name %q", handlerdomain.ErrInvalidName, *in.Name)
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
	s.publish(ctx, "updated", h.ID, nil)
	return h, nil
}

// Delete stops the resident instance, soft-deletes the handler, destroys its envs, and
// purges relation edges.
//
// Delete 停常驻实例、软删 handler、销毁其 envs、清理 relation 边。
func (s *Service) Delete(ctx context.Context, id string) error {
	s.manager.Stop(ctx, id)
	if err := s.repo.DeleteHandler(ctx, id); err != nil {
		return fmt.Errorf("handlerapp.Delete: %w", err)
	}
	if err := s.runner.Destroy(ctx, id); err != nil {
		s.log.Warn("handlerapp.Delete: sandbox destroy failed (best-effort)", zap.String("handlerId", id), zap.Error(err))
	}
	s.publish(ctx, "deleted", id, nil)
	s.purgeRelations(ctx, id)
	return nil
}

// --- version reads ---

func (s *Service) ListVersions(ctx context.Context, handlerID string, filter handlerdomain.VersionListFilter) ([]*handlerdomain.Version, string, error) {
	return s.repo.ListVersions(ctx, handlerID, filter)
}

func (s *Service) GetVersion(ctx context.Context, versionID string) (*handlerdomain.Version, error) {
	return s.repo.GetVersion(ctx, versionID)
}

func (s *Service) GetVersionByNumber(ctx context.Context, handlerID string, versionN int) (*handlerdomain.Version, error) {
	return s.repo.GetVersionByNumber(ctx, handlerID, versionN)
}

// --- helpers ---

func newVersionFromDraft(versionID, handlerID string, versionN int, d *VersionDraft, changeReason string, now time.Time) *handlerdomain.Version {
	pyVer := d.PythonVersion
	if pyVer == "" {
		pyVer = handlerdomain.DefaultPythonVersion
	}
	return &handlerdomain.Version{
		ID: versionID, HandlerID: handlerID, Version: versionN,
		Imports: d.Imports, InitBody: d.InitBody, ShutdownBody: d.ShutdownBody,
		Methods: d.Methods, InitArgsSchema: d.InitArgsSchema,
		Dependencies: d.Dependencies, PythonVersion: pyVer,
		EnvID: idgenpkg.New("hdenv"), EnvStatus: handlerdomain.EnvStatusPending,
		ChangeReason: changeReason, CreatedAt: now, UpdatedAt: now,
	}
}

func (s *Service) nextVersionNumber(ctx context.Context, handlerID string) (int, error) {
	max, err := s.repo.MaxVersionNumber(ctx, handlerID)
	if err != nil {
		return 0, err
	}
	return max + 1, nil
}

func (s *Service) activeAsDraft(ctx context.Context, h *handlerdomain.Handler) (*VersionDraft, error) {
	d := &VersionDraft{Name: h.Name, Description: h.Description, Tags: append([]string(nil), h.Tags...)}
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

func buildOpsFromDirect(in DirectCreateInput) ([]Op, error) {
	ops := make([]Op, 0, 8)
	add := func(opType string, body map[string]any) error {
		raw, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal %s: %w", opType, err)
		}
		ops = append(ops, Op{Type: opType, Raw: raw})
		return nil
	}
	if err := add("set_meta", map[string]any{"name": in.Name, "description": in.Description, "tags": in.Tags}); err != nil {
		return nil, err
	}
	if in.Imports != "" {
		if err := add("set_imports", map[string]any{"imports": in.Imports}); err != nil {
			return nil, err
		}
	}
	if in.InitBody != "" {
		if err := add("set_init", map[string]any{"initBody": in.InitBody}); err != nil {
			return nil, err
		}
	}
	if in.ShutdownBody != "" {
		if err := add("set_shutdown", map[string]any{"shutdownBody": in.ShutdownBody}); err != nil {
			return nil, err
		}
	}
	if len(in.InitArgsSchema) > 0 {
		if err := add("set_init_args_schema", map[string]any{"args": in.InitArgsSchema}); err != nil {
			return nil, err
		}
	}
	for _, m := range in.Methods {
		if err := add("add_method", map[string]any{"method": m}); err != nil {
			return nil, err
		}
	}
	if len(in.Dependencies) > 0 {
		if err := add("set_dependencies", map[string]any{"dependencies": in.Dependencies}); err != nil {
			return nil, err
		}
	}
	if in.PythonVersion != "" {
		if err := add("set_python_version", map[string]any{"version": in.PythonVersion}); err != nil {
			return nil, err
		}
	}
	return ops, nil
}
