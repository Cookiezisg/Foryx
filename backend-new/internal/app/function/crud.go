package function

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"

	envfixapp "github.com/sunweilin/forgify/backend/internal/app/envfix"
	functiondomain "github.com/sunweilin/forgify/backend/internal/domain/function"
	idgenpkg "github.com/sunweilin/forgify/backend/internal/pkg/idgen"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
	schemapkg "github.com/sunweilin/forgify/backend/internal/pkg/schema"
)

// CreateInput is the LLM-forge create payload; Progress (optional) streams env-fix attempts.
//
// CreateInput 是 LLM 锻造 create 载荷；Progress（可选）推 env-fix 尝试进度。
type CreateInput struct {
	Ops          []Op
	ChangeReason string
	Progress     envfixapp.Sink
}

// EditInput is the edit payload; empty Ops is the "rebuild active env" path.
//
// EditInput 是 edit 载荷；空 Ops 走「重建 active env」路径。
type EditInput struct {
	ID           string
	Ops          []Op
	ChangeReason string
	Progress     envfixapp.Sink
}

// DirectCreateInput is the flat HTTP create shape; CreateDirect rebuilds canonical ops.
//
// DirectCreateInput 是扁平 HTTP create 形状；CreateDirect 反推 canonical ops。
type DirectCreateInput struct {
	Name          string
	Description   string
	Code          string
	Tags          []string
	Inputs        []schemapkg.Field
	Outputs       []schemapkg.Field
	Dependencies  []string
	PythonVersion string
	ChangeReason  string
}

// UpdateMetaInput patches function metadata without a version bump; nil = unchanged.
//
// UpdateMetaInput 改 function 元数据不动版本；nil = 不变。
type UpdateMetaInput struct {
	ID          string
	Name        *string
	Description *string
	Tags        *[]string
}

// List returns a cursor page of live functions.
func (s *Service) List(ctx context.Context, filter functiondomain.ListFilter) ([]*functiondomain.Function, string, error) {
	return s.repo.ListFunctions(ctx, filter)
}

// ListAll returns every live function (no pagination).
func (s *Service) ListAll(ctx context.Context) ([]*functiondomain.Function, error) {
	return s.repo.ListAllFunctions(ctx)
}

// Search filters live functions by case-insensitive substring over name / description / tags.
//
// Search 按 name / description / tags 大小写不敏感子串过滤活跃 function。
func (s *Service) Search(ctx context.Context, query string) ([]*functiondomain.Function, error) {
	all, err := s.repo.ListAllFunctions(ctx)
	if err != nil {
		return nil, fmt.Errorf("functionapp.Search: %w", err)
	}
	if strings.TrimSpace(query) == "" {
		return all, nil
	}
	needle := strings.ToLower(query)
	out := make([]*functiondomain.Function, 0, len(all))
	for _, fn := range all {
		if strings.Contains(strings.ToLower(fn.Name), needle) || strings.Contains(strings.ToLower(fn.Description), needle) {
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

// Get returns one function with its active version attached (code + env state in one trip).
//
// Get 返单 function 并附上 active 版本（一趟拿到代码 + env 状态）。
func (s *Service) Get(ctx context.Context, id string) (*functiondomain.Function, error) {
	f, err := s.repo.GetFunction(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("functionapp.Get: %w", err)
	}
	if f.ActiveVersionID != "" {
		if v, verr := s.repo.GetVersion(ctx, f.ActiveVersionID); verr == nil {
			f.ActiveVersion = v
		}
	}
	return f, nil
}

// Create applies ops, persists Function + v1 (active), and materializes its env.
//
// Create 应用 ops、持久化 Function + v1（active）、物化其 env。
func (s *Service) Create(ctx context.Context, in CreateInput) (*functiondomain.Function, *functiondomain.Version, error) {
	if !s.runner.Ready() {
		return nil, nil, functiondomain.ErrSandboxUnavailable
	}
	draft, _, err := s.ApplyOps(nil, in.Ops)
	if err != nil {
		return nil, nil, fmt.Errorf("functionapp.Create: %w", err)
	}

	if _, derr := s.repo.GetFunctionByName(ctx, draft.Name); derr == nil {
		return nil, nil, functiondomain.ErrDuplicateName
	} else if !errors.Is(derr, functiondomain.ErrNotFound) {
		return nil, nil, fmt.Errorf("functionapp.Create: dup-check: %w", derr)
	}

	now := time.Now().UTC()
	fnID := idgenpkg.New("fn")
	versionID := idgenpkg.New("fnv")
	f := &functiondomain.Function{
		ID: fnID, Name: draft.Name, Description: draft.Description, Tags: draft.Tags,
		ActiveVersionID: versionID, CreatedAt: now, UpdatedAt: now,
	}
	v := newVersionFromDraft(versionID, fnID, 1, draft, in.ChangeReason, now)
	if convID, ok := reqctxpkg.GetConversationID(ctx); ok {
		v.ForgedInConversationID = &convID
	}

	if err := s.repo.SaveFunction(ctx, f); err != nil {
		return nil, nil, fmt.Errorf("functionapp.Create: %w", err)
	}
	if err := s.repo.SaveVersion(ctx, v); err != nil {
		return nil, nil, fmt.Errorf("functionapp.Create: %w", err)
	}
	s.publish(ctx, "created", fnID, map[string]any{"versionId": versionID, "version": 1})

	s.ensureEnv(ctx, v, in.Progress) // builds env + AI dep-fix loop; writes status/deps back onto v
	s.syncForgedEdge(ctx, fnID, v.ForgedInConversationID)

	f.ActiveVersion = v
	return f, v, nil
}

// CreateDirect builds canonical ops from a flat HTTP payload and delegates to Create.
//
// CreateDirect 从扁平 HTTP 载荷构 canonical ops 再委托 Create。
func (s *Service) CreateDirect(ctx context.Context, in DirectCreateInput) (*functiondomain.Function, *functiondomain.Version, error) {
	ops, err := buildOpsFromDirect(in)
	if err != nil {
		return nil, nil, fmt.Errorf("functionapp.CreateDirect: %w", err)
	}
	return s.Create(ctx, CreateInput{Ops: ops, ChangeReason: in.ChangeReason})
}

// Edit writes a new version (based on the active one) and moves the active pointer to
// it. Empty Ops re-provisions the active version's env (retry a failed build).
//
// Edit 写新版本（基于 active）并把 active 指针移到它。空 Ops 重建 active 版本的 env（重试失败构建）。
func (s *Service) Edit(ctx context.Context, in EditInput) (*functiondomain.Version, error) {
	if !s.runner.Ready() {
		return nil, functiondomain.ErrSandboxUnavailable
	}
	f, err := s.repo.GetFunction(ctx, in.ID)
	if err != nil {
		return nil, fmt.Errorf("functionapp.Edit: %w", err)
	}

	if len(in.Ops) == 0 {
		if f.ActiveVersionID == "" {
			return nil, fmt.Errorf("functionapp.Edit: %w", functiondomain.ErrNoActiveVersion)
		}
		active, err := s.repo.GetVersion(ctx, f.ActiveVersionID)
		if err != nil {
			return nil, fmt.Errorf("functionapp.Edit: %w", err)
		}
		s.ensureEnv(ctx, active, in.Progress)
		s.publish(ctx, "env_rebuilt", in.ID, map[string]any{"versionId": active.ID})
		return active, nil
	}

	base, err := s.activeAsDraft(ctx, f)
	if err != nil {
		return nil, fmt.Errorf("functionapp.Edit: %w", err)
	}
	draft, _, err := s.ApplyOps(base, in.Ops)
	if err != nil {
		return nil, fmt.Errorf("functionapp.Edit: %w", err)
	}

	nextN, err := s.nextVersionNumber(ctx, in.ID)
	if err != nil {
		return nil, fmt.Errorf("functionapp.Edit: %w", err)
	}
	now := time.Now().UTC()
	versionID := idgenpkg.New("fnv")
	v := newVersionFromDraft(versionID, in.ID, nextN, draft, in.ChangeReason, now)
	if convID, ok := reqctxpkg.GetConversationID(ctx); ok {
		v.ForgedInConversationID = &convID
	}

	if err := s.repo.SaveVersion(ctx, v); err != nil {
		return nil, fmt.Errorf("functionapp.Edit: %w", err)
	}
	if err := s.repo.SetActiveVersion(ctx, in.ID, versionID); err != nil {
		return nil, fmt.Errorf("functionapp.Edit: %w", err)
	}
	if err := s.repo.TrimOldestVersions(ctx, in.ID, functiondomain.VersionCap); err != nil {
		s.log.Warn("functionapp.Edit: trim versions failed", zap.String("functionId", in.ID), zap.Error(err))
	}
	s.publish(ctx, "edited", in.ID, map[string]any{"versionId": versionID, "version": nextN})

	s.ensureEnv(ctx, v, in.Progress)
	s.syncEditedEdge(ctx, in.ID)
	return v, nil
}

// Revert moves the active pointer to an existing version by number — a pure pointer
// op: no new version, no deletion of "newer" versions. The target's env is rebuilt
// lazily on the next run if it was reclaimed.
//
// Revert 按号把 active 指针移到一个已有版本——纯指针操作：不产生版本、不删「更新的」版本。
// target 的 env 若已被回收，下次 run 时懒重建。
func (s *Service) Revert(ctx context.Context, id string, targetVersion int) (*functiondomain.Version, error) {
	target, err := s.repo.GetVersionByNumber(ctx, id, targetVersion)
	if err != nil {
		return nil, fmt.Errorf("functionapp.Revert: %w", err)
	}
	if err := s.repo.SetActiveVersion(ctx, id, target.ID); err != nil {
		return nil, fmt.Errorf("functionapp.Revert: %w", err)
	}
	s.publish(ctx, "reverted", id, map[string]any{"versionId": target.ID, "version": targetVersion})
	s.syncEditedEdge(ctx, id)
	return target, nil
}

// UpdateMeta patches function metadata without creating a version.
func (s *Service) UpdateMeta(ctx context.Context, in UpdateMetaInput) (*functiondomain.Function, error) {
	f, err := s.repo.GetFunction(ctx, in.ID)
	if err != nil {
		return nil, fmt.Errorf("functionapp.UpdateMeta: %w", err)
	}
	if in.Name != nil {
		if !validNameRe.MatchString(*in.Name) {
			return nil, fmt.Errorf("functionapp.UpdateMeta: %w: name %q", functiondomain.ErrOpInvalid, *in.Name)
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
	s.publish(ctx, "updated", f.ID, nil)
	return f, nil
}

// Delete soft-deletes the function, destroys its sandbox envs, and purges relation edges.
//
// Delete 软删 function、销毁其 sandbox envs、清理 relation 边。
func (s *Service) Delete(ctx context.Context, id string) error {
	if err := s.repo.DeleteFunction(ctx, id); err != nil {
		return fmt.Errorf("functionapp.Delete: %w", err)
	}
	if err := s.runner.Destroy(ctx, id); err != nil {
		s.log.Warn("functionapp.Delete: sandbox destroy failed (best-effort)", zap.String("functionId", id), zap.Error(err))
	}
	s.publish(ctx, "deleted", id, nil)
	s.purgeRelations(ctx, id)
	return nil
}

// --- version reads ---

func (s *Service) ListVersions(ctx context.Context, functionID string, filter functiondomain.VersionListFilter) ([]*functiondomain.Version, string, error) {
	return s.repo.ListVersions(ctx, functionID, filter)
}

func (s *Service) GetVersion(ctx context.Context, versionID string) (*functiondomain.Version, error) {
	return s.repo.GetVersion(ctx, versionID)
}

func (s *Service) GetVersionByNumber(ctx context.Context, functionID string, versionN int) (*functiondomain.Version, error) {
	return s.repo.GetVersionByNumber(ctx, functionID, versionN)
}

// ActiveVersion returns the function's active version (or ErrNoActiveVersion).
func (s *Service) ActiveVersion(ctx context.Context, functionID string) (*functiondomain.Version, error) {
	f, err := s.repo.GetFunction(ctx, functionID)
	if err != nil {
		return nil, fmt.Errorf("functionapp.ActiveVersion: %w", err)
	}
	if f.ActiveVersionID == "" {
		return nil, fmt.Errorf("functionapp.ActiveVersion: %w", functiondomain.ErrNoActiveVersion)
	}
	return s.repo.GetVersion(ctx, f.ActiveVersionID)
}

// --- helpers ---

func newVersionFromDraft(versionID, functionID string, versionN int, d *VersionDraft, changeReason string, now time.Time) *functiondomain.Version {
	pyVer := d.PythonVersion
	if pyVer == "" {
		pyVer = functiondomain.DefaultPythonVersion
	}
	return &functiondomain.Version{
		ID: versionID, FunctionID: functionID, Version: versionN,
		Code: d.Code, Inputs: d.Inputs, Outputs: d.Outputs,
		Dependencies: d.Dependencies, PythonVersion: pyVer,
		EnvID: idgenpkg.New("fnenv"), EnvStatus: functiondomain.EnvStatusPending,
		ChangeReason: changeReason, CreatedAt: now, UpdatedAt: now,
	}
}

func (s *Service) nextVersionNumber(ctx context.Context, functionID string) (int, error) {
	max, err := s.repo.MaxVersionNumber(ctx, functionID)
	if err != nil {
		return 0, err
	}
	return max + 1, nil
}

func (s *Service) activeAsDraft(ctx context.Context, f *functiondomain.Function) (*VersionDraft, error) {
	d := &VersionDraft{Name: f.Name, Description: f.Description, Tags: append([]string(nil), f.Tags...)}
	if f.ActiveVersionID == "" {
		return d, nil
	}
	active, err := s.repo.GetVersion(ctx, f.ActiveVersionID)
	if err != nil {
		return nil, err
	}
	d.Code = active.Code
	d.Inputs = append([]schemapkg.Field(nil), active.Inputs...)
	d.Outputs = append([]schemapkg.Field(nil), active.Outputs...)
	d.Dependencies = append([]string(nil), active.Dependencies...)
	d.PythonVersion = active.PythonVersion
	return d, nil
}

func buildOpsFromDirect(in DirectCreateInput) ([]Op, error) {
	ops := make([]Op, 0, 6)
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
	if in.Code != "" {
		if err := add("set_code", map[string]any{"code": in.Code}); err != nil {
			return nil, err
		}
	}
	if len(in.Inputs) > 0 {
		if err := add("set_inputs", map[string]any{"inputs": in.Inputs}); err != nil {
			return nil, err
		}
	}
	if len(in.Outputs) > 0 {
		if err := add("set_outputs", map[string]any{"outputs": in.Outputs}); err != nil {
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
