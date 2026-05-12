// crud.go — Function CRUD + version lifecycle (pending → accept / reject /
// revert) at the Service layer. Each method is ctx-scoped to the current user
// via reqctxpkg.RequireUserID; cross-user reads return ErrNotFound by repo.
//
// Notifications: every state change publishes a `function` entity event via
// notif.Publish (conversationID == "" → global broadcast). UI subscribes to
// /api/v1/notifications and refreshes the function list / detail panel on each
// matching envelope.
//
// Sandbox env sync (writing code files + materializing the venv) is wired in
// Task 12 via sandbox_adapter.go. For now Create / Edit / AcceptPending leave
// EnvStatus == "pending" — the adapter (Task 12) starts a background goroutine
// after each accept that runs Sync + writes EnvStatus = ready/failed.
//
// crud.go —— Function CRUD + 版本生命周期(pending → accept / reject /
// revert)在 Service 层。每方法按 ctx userID 过滤;跨用户读由 repo 返
// ErrNotFound。
//
// 通知:每次状态变更经 notif.Publish 推 `function` entity 事件(全局广播)。
// UI 订阅 /api/v1/notifications,刷列表/详情。
//
// Sandbox env sync 在 Task 12 sandbox_adapter.go 接驳。本任务 Create / Edit /
// AcceptPending 留 EnvStatus="pending",adapter 在 accept 后起后台 goroutine
// 跑 Sync 写终态。

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

// ── Input types ───────────────────────────────────────────────────────────────

// CreateInput is the request shape for Service.Create. Name + Description come
// from explicit fields (used for duplicate-name check before applying ops);
// Ops carry the full editable surface (code / parameters / dependencies / etc).
//
// CreateInput 是 Service.Create 的请求形状。Name + Description 是显式字段(
// 用于 ops 应用前查重),Ops 携带其余可编辑面(代码 / 参数 / 依赖等)。
type CreateInput struct {
	Ops             []Op
	ChangeReason    string
	ProgressBlockID string // optional eventlog block id for progress deltas
}

// EditInput is the request shape for Service.Edit (writes a pending version).
//
// EditInput 是 Service.Edit 的请求形状(写 pending 版本)。
type EditInput struct {
	ID              string
	Ops             []Op
	ChangeReason    string
	ProgressBlockID string
}

// DirectCreateInput is the HTTP-friendly shape for POST /functions (flat
// definition instead of an ops list — easier for curl / UI / scripts than
// constructing the ops array). Service.CreateDirect translates these into
// the canonical ops sequence and delegates to Service.Create.
//
// DirectCreateInput 是 POST /functions 用的扁平定义形状(curl/UI/script 比
// ops 数组好用)。Service.CreateDirect 转为 canonical ops 再委托 Create。
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

// UpdateMetaInput patches Function metadata (no version side effects). nil
// fields are unchanged.
//
// UpdateMetaInput 改 Function 元数据(不改版本)。nil 字段不变。
type UpdateMetaInput struct {
	ID          string
	Name        *string
	Description *string
	Tags        *[]string
}

// ── Reads ─────────────────────────────────────────────────────────────────────

// List returns a paginated page of live functions for the current user.
// Computed Pending / Env* fields are NOT populated — caller uses Get for detail.
//
// List 返当前用户活跃 function 的 cursor 分页;计算字段不填,详情用 Get。
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
// Used by CatalogSource.ListItems + the search_function LLM tool.
//
// ListAll 返当前用户全部活跃 function(无分页);CatalogSource + search_function
// tool 用。
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

// Search returns functions whose name / description / tags contain query (case-
// insensitive substring). V1 implementation;V1.5 will let the LLM tool layer
// re-rank semantically.
//
// Search 返 name / description / tags 含 query 子串(忽略大小写)的 function。
// V1 实现;V1.5 由 LLM tool 层再语义排序。
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

// Get fetches one function with its computed fields populated (active version's
// env state mirrored onto Function;pending version attached if present).
//
// Get 返单 function 含计算字段(active version 的 env 状态镜像到 Function;
// 有 pending 时挂上)。
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

// attachComputed populates Function.Pending + Function.Env* from the pending
// version (if any) + active version. Errors fetching either are non-fatal —
// the function row is still usable, just without those decorations.
//
// attachComputed 把 pending + active 版本的状态填到 Function 计算字段。
// 单独失败不影响主返回(降级,只是少装饰)。
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

// ── Lifecycle ─────────────────────────────────────────────────────────────────

// Create builds a new Function from ops + auto-accepts the resulting version
// as v1 (first-create auto-accept — aligns with forge's TE-15 pattern).
//
// Per D-redo-9 (forge_redesign 2026-05-12) env sync is **synchronous** here;
// the caller's tool returns only after the venv is built (or terminally
// failed). Failure does NOT roll back the entity rows — v.EnvStatus is set to
// `failed` + v.EnvError captured, caller checks the returned Version to
// decide next step (typically retry via edit_function with new deps, the
// env-fix loop lives in the LLM tool layer, see C2).
//
// Per D-redo-20 a sandbox ping precedes the DB writes: if the sandbox is
// unavailable (mise binary missing / data dir not writable / etc.) we hard-
// reject with ErrSandboxUnavailable and create no entity.
//
// Create 应用 ops → 持久化 Function + Version1(自动 accept)。
//
// 按 D-redo-9,env sync 同步在此发生,工具返前必装完(或终态失败);失败
// **不回滚** entity 行,v.EnvStatus=failed + v.EnvError 写入,调用方检查
// Version 自行决定下一步(LLM tool env-fix loop 见 C2)。
// 按 D-redo-20,DB 写入前先 sandbox ping;不可用则硬拒,不建 entity。
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
		EnvID:         idgenpkg.New("fnenv"), // D-redo-8: each Version owns a venv keyed by a fresh fnenv_ id (decoupled from versionID)
		EnvStatus:     functiondomain.EnvStatusPending,
		ChangeReason:  in.ChangeReason,
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	if err := s.repo.SaveFunction(ctx, f); err != nil {
		return nil, nil, fmt.Errorf("functionapp.Create: SaveFunction: %w", err)
	}
	if err := s.repo.SaveVersion(ctx, v); err != nil {
		return nil, nil, fmt.Errorf("functionapp.Create: SaveVersion: %w", err)
	}

	s.publish(ctx, fnID, "created", map[string]any{"versionId": v.ID, "versionNumber": versionN})

	// Sync env synchronously (D-redo-9). Failure marks v.EnvStatus=failed +
	// v.EnvError via syncEnvSync (which writes to DB + mutates v in place);
	// entity rows kept. Caller checks v.EnvStatus to react.
	if err := s.syncEnvSync(ctx, v); err != nil {
		s.log.Warn("functionapp.Create: env sync failed",
			zap.String("functionId", fnID), zap.String("versionId", versionID), zap.Error(err))
	}

	return f, v, nil
}

// checkSandbox runs a fast availability check against the Sandbox port. It
// uses PythonPath()=="" as the failure signal (sandbox bootstrap failure
// leaves the bundled python path empty). D-redo-20: hard-reject Create/Edit
// before any DB writes when the sandbox is unavailable.
//
// checkSandbox 对 Sandbox 端口跑快速可用性 ping;PythonPath()=="" 表示
// bootstrap 失败(D-redo-20)。Create/Edit 在 DB 写入前先调,失败硬拒。
func (s *Service) checkSandbox() error {
	if s.sandbox.PythonPath() == "" {
		return functiondomain.ErrSandboxUnavailable
	}
	return nil
}

// CreateDirect builds an ops list from a flat definition and delegates to
// Create. HTTP POST /functions uses this; LLM create_function tool uses Create
// directly with its own ops.
//
// CreateDirect 从扁平定义构 ops 再委托 Create。HTTP POST /functions 用;LLM
// create_function 直接走 Create 用自己的 ops。
func (s *Service) CreateDirect(ctx context.Context, in DirectCreateInput) (*functiondomain.Function, *functiondomain.Version, error) {
	ops, err := buildOpsFromDirect(in)
	if err != nil {
		return nil, nil, fmt.Errorf("functionapp.CreateDirect: %w", err)
	}
	return s.Create(ctx, CreateInput{Ops: ops, ChangeReason: in.ChangeReason})
}

// buildOpsFromDirect marshals direct definition fields into a canonical ops
// sequence: set_meta → set_code → set_parameters → set_return_schema →
// set_dependencies → set_python_version. Empty fields are skipped (no-op);
// only set_code is required (apply final validation enforces).
//
// buildOpsFromDirect 把扁平字段 marshal 为 canonical ops 序列。空字段跳;
// 仅 set_code 必填(final 校验保证)。
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
		raw, err := json.Marshal(map[string]any{"deps": in.Dependencies})
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

// Edit produces a pending version under D-redo-11 "iterate same pending"
// semantics:
//   - No pending → ApplyOps on top of active → new pending Version row + sync env.
//   - Pending exists → ApplyOps on top of pending → **rewrite same pending row
//     in place** (keep ID, destroy old env, sync new env). No ErrPendingConflict.
//   - ops=[] with no pending → D-redo-22 "force rebuild env": no draft change,
//     destroy + re-sync the active version's env (returns the active version).
//   - ops=[] with pending → re-sync the pending row's env (no field change).
//
// Per D-redo-9 the env sync is synchronous; failure marks v.EnvStatus=failed +
// v.EnvError, entity rows kept. Per D-redo-20 a sandbox ping precedes work.
//
// Edit 按 D-redo-11 "iterate same pending":
//   - 无 pending → 在 active 上 ApplyOps → 新建 pending + 装 env
//   - 有 pending → 在 pending 上 ApplyOps → 重写同 ID pending(销旧 env + 装新 env)
//   - ops=[] 无 pending → D-redo-22 强制重建 active version 的 env
//   - ops=[] 有 pending → 重装 pending 的 env(字段不变)
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
		// pending exists → iterate same row
	case errors.Is(perr, functiondomain.ErrPendingNotFound):
		pending = nil
	default:
		return nil, fmt.Errorf("functionapp.Edit: pending-check: %w", perr)
	}

	// D-redo-22: ops=[] is the "force rebuild env" path — destroy + re-sync
	// the existing row (pending if present, else active). No draft change.
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

	// ApplyOps on top of pending (if any) else active.
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
		// Rewrite same pending row (keep ID + EnvID == ID). Destroy old venv
		// since deps/python may have changed; re-sync below.
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
			EnvID:         idgenpkg.New("fnenv"), // D-redo-8: fresh per-version env id, decoupled from versionID
			EnvStatus:     functiondomain.EnvStatusPending,
			ChangeReason:  in.ChangeReason,
			CreatedAt:     now,
			UpdatedAt:     now,
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

// versionToDraft converts an existing Version row to a VersionDraft (base for
// ApplyOps when iterating a pending row).
//
// versionToDraft 把已有 Version 行转 VersionDraft(iterate pending 时作 ApplyOps 起点)。
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

// AcceptPending turns the active pending into a numbered accepted version and
// flips Function.ActiveVersionID. Enforces the per-function accepted-version
// cap (functiondomain.AcceptedVersionCap).
//
// AcceptPending 把 pending 翻为带号 accepted + 翻 ActiveVersionID;
// 应用 per-function accepted 上限。
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
	return pending, nil
}

// RejectPending destroys the pending venv and hard-deletes the pending Version
// row (per D-redo-12). UI/LLM can immediately Edit again to create a fresh
// pending. No state change to ActiveVersion.
//
// RejectPending 销 pending 的 venv + 物理删 Version 行(D-redo-12);
// 不动 ActiveVersion;UI/LLM 可立即重新 Edit。
func (s *Service) RejectPending(ctx context.Context, id string) error {
	if _, err := reqctxpkg.RequireUserID(ctx); err != nil {
		return fmt.Errorf("functionapp.RejectPending: %w", err)
	}
	pending, err := s.repo.GetPending(ctx, id)
	if err != nil {
		return fmt.Errorf("functionapp.RejectPending: %w", err)
	}
	if err := s.sandbox.DestroyEnv(ctx, id, pending.EnvID); err != nil {
		// best-effort; venv cleanup failure shouldn't block the reject decision
		// 尽力清理 venv;失败仅 log,不阻 reject
		s.log.Warn("functionapp.RejectPending: DestroyEnv failed (best-effort)",
			zap.String("functionId", id), zap.String("versionId", pending.ID), zap.Error(err))
	}
	if err := s.repo.HardDeleteVersion(ctx, pending.ID); err != nil {
		return fmt.Errorf("functionapp.RejectPending: %w", err)
	}
	s.publish(ctx, id, "pending_rejected", map[string]any{"versionId": pending.ID})
	return nil
}

// Revert flips ActiveVersionID to a target accepted version. Returns
// ErrVersionNotFound if no accepted version with that number exists.
//
// Revert 把 ActiveVersionID 翻到指定 accepted 版本号;无则 ErrVersionNotFound。
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
	return target, nil
}

// UpdateMeta patches Function metadata without creating a new version. Used
// by the PATCH /functions/{id} endpoint for direct UI edits to name /
// description / tags. Code / parameters / dependencies changes go through
// Edit (pending version flow).
//
// UpdateMeta 改 Function 元数据不创建新版本。UI PATCH 端点用,改 code/
// parameters/deps 必须走 Edit(pending 流程)。
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
	// D-redo-6: slim payload — UI does GET to fetch updated meta.
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

// Delete soft-deletes a function. Publishes a deletion notification — the
// workflow domain subscribes to mark referencing workflows as needs_attention
// (per forge_redesign D20).
//
// Delete 软删 function。发删除通知——workflow domain 订阅后把引用此 function
// 的 workflow 标 needs_attention(D20)。
func (s *Service) Delete(ctx context.Context, id string) error {
	if _, err := reqctxpkg.RequireUserID(ctx); err != nil {
		return fmt.Errorf("functionapp.Delete: %w", err)
	}
	if err := s.repo.DeleteFunction(ctx, id); err != nil {
		return fmt.Errorf("functionapp.Delete: %w", err)
	}
	s.publish(ctx, id, "deleted", nil)
	return nil
}

// ── Helpers ───────────────────────────────────────────────────────────────────

// activeAsDraft loads the function's active version and returns it as a
// VersionDraft suitable as base for ApplyOps. If ActiveVersionID is empty
// (draft state) returns a zero-value draft preserving function name/desc/tags.
//
// activeAsDraft 把 active 版本加载为 VersionDraft 作为 Edit 的 base。
// ActiveVersionID 空时返保留 function 元数据的空 draft。
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

// nextVersionNumber returns max(accepted.version)+1 for the function. First
// accepted gets 1. Walks ListVersions accepted page (size 1) to find current
// max.
//
// nextVersionNumber 返该 function 下 max(accepted.version)+1。首个 accepted
// 返 1。
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

// publish emits a `function` entity notification. data may be nil for purely
// state-transition events (e.g. deleted).
//
// publish 推 `function` entity 通知;data 可为 nil(纯状态变更事件)。
func (s *Service) publish(ctx context.Context, functionID, action string, data map[string]any) {
	envelope := map[string]any{"action": action}
	for k, v := range data {
		envelope[k] = v
	}
	s.notif.Publish(ctx, "function", functionID, envelope, "")
}
