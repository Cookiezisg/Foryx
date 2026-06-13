package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"

	workflowdomain "github.com/sunweilin/forgify/backend/internal/domain/workflow"
	celpkg "github.com/sunweilin/forgify/backend/internal/pkg/cel"
	idgenpkg "github.com/sunweilin/forgify/backend/internal/pkg/idgen"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

// CreateInput is the create payload: identity (name/description/tags) + the ops that build
// v1's graph. Empty Ops yields an empty-graph v1 (a draft the author wires up via :edit
// later) — but an empty graph fails ValidateGraph (no trigger), so an empty create is only
// accepted when the caller has not asked to validate. To keep create strict-by-default we
// always validate: a create therefore needs at least a trigger node in its ops.
//
// CreateInput 是 create 载荷：身份（name/description/tags）+ 构 v1 图的 ops。空 Ops 得空图 v1
// （作者后续 :edit 接线的草稿）——但空图过不了 ValidateGraph（无 trigger），故空 create 仅在调用方
// 不要求校验时接受。为保 create 默认严格我们总校验：故 create 至少需 ops 里一个 trigger 节点。
type CreateInput struct {
	Name         string
	Description  string
	Tags         []string
	Ops          []workflowdomain.Op
	ChangeReason string
}

// EditInput applies ops on top of the active graph, producing vN+1 (immediately active).
//
// EditInput 在 active 图上套 ops，产 vN+1（立即 active）。
type EditInput struct {
	ID           string
	Ops          []workflowdomain.Op
	ChangeReason string
}

// UpdateMetaInput patches workflow identity without a version bump; nil = unchanged.
//
// UpdateMetaInput 改 workflow 身份不动版本；nil = 不变。
type UpdateMetaInput struct {
	ID          string
	Name        *string
	Description *string
	Tags        *[]string
	// Concurrency switches the overlap policy (serial|skip|buffer_one|buffer_all|allow_all)
	// — a runtime header knob, not version content; takes effect on the NEXT firing drain.
	// Concurrency 切换 overlap 政策——运行时头部旋钮、非版本内容；下一次 firing drain 生效。
	Concurrency *string
}

// Create applies ops → validates the graph → compiles every node.Input CEL → persists a
// Workflow + v1 (active). A new workflow starts parked: active=false, lifecycle=inactive
// (the author activates it explicitly once the graph is sound).
//
// Create 应用 ops → 校验图 → 编译每个 node.Input CEL → 持久化 Workflow + v1（active）。新
// workflow 起始停泊：active=false、lifecycle=inactive（作者待图无误后显式激活）。
func (s *Service) Create(ctx context.Context, in CreateInput) (*workflowdomain.Workflow, *workflowdomain.Version, error) {
	if strings.TrimSpace(in.Name) == "" {
		return nil, nil, workflowdomain.ErrInvalidOps.WithDetails(map[string]any{"reason": "name is required"})
	}
	graph, err := s.buildGraph(nil, in.Ops)
	if err != nil {
		return nil, nil, fmt.Errorf("workflowapp.Create: %w", err)
	}
	// set_meta ops project onto the HEADER (name/description/tags/concurrency) — an explicit
	// op wins over the flat payload fields. Without this projection a set_meta was a silent
	// no-op (shape-checked, applied nowhere).
	// set_meta op 投影到**头部**（name/description/tags/concurrency）——显式 op 赢过扁平字段。
	// 没有这步投影，set_meta 曾是静默 no-op（查形状、无处生效）。
	meta, err := workflowdomain.ExtractMeta(in.Ops)
	if err != nil {
		return nil, nil, fmt.Errorf("workflowapp.Create: %w", err)
	}
	name, desc, tags := in.Name, in.Description, in.Tags
	if meta.Name != nil {
		name = *meta.Name
	}
	if meta.Description != nil {
		desc = *meta.Description
	}
	if meta.Tags != nil {
		tags = *meta.Tags
	}
	concurrency := workflowdomain.ConcurrencySerial
	if meta.Concurrency != nil {
		if !workflowdomain.IsValidConcurrency(*meta.Concurrency) {
			return nil, nil, workflowdomain.ErrInvalidOps.WithDetails(map[string]any{"reason": "invalid concurrency policy"})
		}
		concurrency = *meta.Concurrency
	}
	in.Name = name

	if _, derr := s.repo.GetWorkflowByName(ctx, in.Name); derr == nil {
		return nil, nil, workflowdomain.ErrDuplicateName
	} else if !errors.Is(derr, workflowdomain.ErrNotFound) {
		return nil, nil, fmt.Errorf("workflowapp.Create: dup-check: %w", derr)
	}

	now := time.Now().UTC()
	wfID := idgenpkg.New("wf")
	versionID := idgenpkg.New("wfv")
	w := &workflowdomain.Workflow{
		ID: wfID, Name: in.Name, Description: desc, Tags: orEmpty(tags),
		Active: false, LifecycleState: workflowdomain.LifecycleInactive,
		Concurrency: concurrency, LastActionBy: workflowdomain.ActorUser,
		ActiveVersionID: versionID, CreatedAt: now, UpdatedAt: now,
	}
	v := newVersion(versionID, wfID, 1, graph, in.ChangeReason, now)
	if convID, ok := reqctxpkg.GetConversationID(ctx); ok {
		v.ForgedInConversationID = &convID
	}

	if err := s.repo.SaveWorkflow(ctx, w); err != nil { // UNIQUE name → ErrDuplicateName here
		return nil, nil, fmt.Errorf("workflowapp.Create: %w", err)
	}
	if err := s.repo.SaveVersion(ctx, v); err != nil {
		return nil, nil, fmt.Errorf("workflowapp.Create: %w", err)
	}
	s.publish(ctx, "created", wfID, map[string]any{"versionId": versionID, "version": 1})
	s.syncRelations(ctx, w, v, graph)

	v.GraphParsed = graph
	w.ActiveVersion = v
	return w, v, nil
}

// Edit applies ops on top of the active graph, validates + compiles, writes vN+1 and moves
// the active pointer to it. Empty Ops is rejected (an edit must change something — unlike
// function, workflow has no "rebuild env" empty-ops path).
//
// Edit 在 active 图上套 ops，校验 + 编译，写 vN+1 并把 active 指针移到它。空 Ops 被拒（edit 须
// 改动——与 function 不同，workflow 无「重建 env」空 ops 路径）。
func (s *Service) Edit(ctx context.Context, in EditInput) (*workflowdomain.Version, error) {
	w, err := s.repo.GetWorkflow(ctx, in.ID)
	if err != nil {
		return nil, fmt.Errorf("workflowapp.Edit: %w", err)
	}
	if len(in.Ops) == 0 {
		return nil, workflowdomain.ErrInvalidOps.WithDetails(map[string]any{"reason": "edit requires at least one op"})
	}

	base, err := s.activeGraph(ctx, w)
	if err != nil {
		return nil, fmt.Errorf("workflowapp.Edit: %w", err)
	}
	graph, err := s.buildGraph(base, in.Ops)
	if err != nil {
		return nil, fmt.Errorf("workflowapp.Edit: %w", err)
	}

	max, err := s.repo.MaxVersionNumber(ctx, in.ID)
	if err != nil {
		return nil, fmt.Errorf("workflowapp.Edit: %w", err)
	}
	now := time.Now().UTC()
	versionID := idgenpkg.New("wfv")
	v := newVersion(versionID, in.ID, max+1, graph, in.ChangeReason, now)
	if convID, ok := reqctxpkg.GetConversationID(ctx); ok {
		v.ForgedInConversationID = &convID
	}

	if err := s.repo.SaveVersion(ctx, v); err != nil {
		return nil, fmt.Errorf("workflowapp.Edit: %w", err)
	}
	if err := s.repo.SetActiveVersion(ctx, in.ID, versionID); err != nil {
		return nil, fmt.Errorf("workflowapp.Edit: %w", err)
	}
	// set_meta ops project onto the header (previously a silent no-op — shape-checked,
	// applied nowhere). Name change re-checks uniqueness via SaveWorkflow's UNIQUE.
	// set_meta op 投影到头部（此前是静默 no-op——查形状、无处生效）。改名经 SaveWorkflow 的
	// UNIQUE 重查唯一性。
	if meta, merr := workflowdomain.ExtractMeta(in.Ops); merr == nil {
		dirty := false
		if meta.Name != nil && strings.TrimSpace(*meta.Name) != "" {
			w.Name, dirty = *meta.Name, true
		}
		if meta.Description != nil {
			w.Description, dirty = *meta.Description, true
		}
		if meta.Tags != nil {
			w.Tags, dirty = orEmpty(*meta.Tags), true
		}
		if meta.Concurrency != nil && workflowdomain.IsValidConcurrency(*meta.Concurrency) {
			w.Concurrency, dirty = *meta.Concurrency, true
		}
		if dirty {
			if err := s.repo.SaveWorkflow(ctx, w); err != nil {
				return nil, fmt.Errorf("workflowapp.Edit: meta: %w", err)
			}
		}
	}
	// A live (active) workflow whose edit changed the entry trigger refs must rebind NOW:
	// the binder still holds the old graph's refs (see rebindIfListening).
	// 活（active）workflow 的编辑若改了入口 trigger ref，必须立刻重绑：binder 还挂着旧图的 ref
	// （见 rebindIfListening）。
	s.rebindIfListening(ctx, w, base, graph)
	if err := s.repo.TrimOldestVersions(ctx, in.ID, workflowdomain.VersionCap); err != nil {
		s.log.Warn("workflowapp.Edit: trim versions failed", zap.String("workflowId", in.ID), zap.Error(err))
	}
	s.publish(ctx, "edited", in.ID, map[string]any{"versionId": versionID, "version": max + 1})

	w.ActiveVersionID = versionID
	s.syncRelations(ctx, w, v, graph)
	v.GraphParsed = graph
	return v, nil
}

// Revert moves the active pointer to an existing version by number — a pure pointer op: no
// new version, no deletion of "newer" versions.
//
// Revert 按号把 active 指针移到一个已有版本——纯指针操作：不产生版本、不删「更新的」版本。
func (s *Service) Revert(ctx context.Context, id string, targetVersion int) (*workflowdomain.Version, error) {
	target, err := s.repo.GetVersionByNumber(ctx, id, targetVersion)
	if err != nil {
		return nil, fmt.Errorf("workflowapp.Revert: %w", err)
	}
	// Snapshot the OLD active graph before the pointer moves — a revert can change the entry
	// trigger refs just like an edit, and a live listener must rebind (see rebindIfListening).
	// 在指针移动前快照**旧** active 图——revert 与 edit 一样可能换入口 trigger ref，活监听必须重绑
	// （见 rebindIfListening）。
	w, err := s.repo.GetWorkflow(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("workflowapp.Revert: %w", err)
	}
	oldGraph, _ := s.activeGraph(ctx, w)
	if err := s.repo.SetActiveVersion(ctx, id, target.ID); err != nil {
		return nil, fmt.Errorf("workflowapp.Revert: %w", err)
	}
	s.publish(ctx, "reverted", id, map[string]any{"versionId": target.ID, "version": targetVersion})

	w.ActiveVersionID = target.ID
	if g, perr := decodeGraph(target.Graph); perr == nil {
		s.rebindIfListening(ctx, w, oldGraph, g)
		s.syncRelations(ctx, w, target, g)
		target.GraphParsed = g
	}
	return target, nil
}

// UpdateMeta patches workflow identity (name/description/tags) without creating a version.
//
// UpdateMeta 改 workflow 身份（name/description/tags）不产版本。
func (s *Service) UpdateMeta(ctx context.Context, in UpdateMetaInput) (*workflowdomain.Workflow, error) {
	w, err := s.repo.GetWorkflow(ctx, in.ID)
	if err != nil {
		return nil, fmt.Errorf("workflowapp.UpdateMeta: %w", err)
	}
	if in.Name != nil {
		if strings.TrimSpace(*in.Name) == "" {
			return nil, workflowdomain.ErrInvalidOps.WithDetails(map[string]any{"reason": "name cannot be empty"})
		}
		w.Name = *in.Name
	}
	if in.Description != nil {
		w.Description = *in.Description
	}
	if in.Tags != nil {
		w.Tags = orEmpty(*in.Tags)
	}
	if in.Concurrency != nil {
		if !workflowdomain.IsValidConcurrency(*in.Concurrency) {
			return nil, workflowdomain.ErrInvalidOps.WithDetails(map[string]any{"reason": "invalid concurrency policy"})
		}
		w.Concurrency = *in.Concurrency
	}
	if err := s.repo.SaveWorkflow(ctx, w); err != nil {
		return nil, fmt.Errorf("workflowapp.UpdateMeta: %w", err) // ErrDuplicateName
	}
	s.publish(ctx, "updated", w.ID, nil)
	return w, nil
}

// SetLifecycle transitions the workflow's lifecycle state (active|draining|inactive) and
// mirrors active = (state == active). actionBy records whether a user or the system drove
// the change. Any state is reachable from any other (the scheduler may force-drain or park
// from anywhere); only the value itself is validated.
//
// SetLifecycle 转换 workflow lifecycle 状态（active|draining|inactive）并镜像 active = (state
// == active)。actionBy 记录用户或系统驱动。任意状态可从任意状态到达（调度器可从任何处强制 drain 或
// 停泊）；只校验值本身。
func (s *Service) SetLifecycle(ctx context.Context, id, state, actionBy string) (*workflowdomain.Workflow, error) {
	if !workflowdomain.IsValidLifecycle(state) {
		return nil, workflowdomain.ErrInvalidLifecycle.WithDetails(map[string]any{"reason": fmt.Sprintf("unknown lifecycle state %q", state)})
	}
	if actionBy == "" {
		actionBy = workflowdomain.ActorUser
	}
	active := state == workflowdomain.LifecycleActive
	if err := s.repo.UpdateWorkflowMeta(ctx, id, workflowdomain.MetaUpdate{
		Active: &active, LifecycleState: &state, LastActionBy: &actionBy,
	}); err != nil {
		return nil, fmt.Errorf("workflowapp.SetLifecycle: %w", err)
	}
	s.publish(ctx, "lifecycle_changed", id, map[string]any{"lifecycleState": state, "active": active})
	return s.repo.GetWorkflow(ctx, id)
}

// SetNeedsAttention flags (or clears) the workflow's needs-attention banner with a reason
// (cleared when needs=false). The scheduler raises this when a run fails non-retryably;
// surfaced as a system action.
//
// SetNeedsAttention 置（或清）workflow 的 needs-attention 横幅及原因（needs=false 时清空）。调度器
// 在运行不可重试地失败时拉起；作为系统动作上呈。
func (s *Service) SetNeedsAttention(ctx context.Context, id string, needs bool, reason string) (*workflowdomain.Workflow, error) {
	if !needs {
		reason = ""
	}
	actionBy := workflowdomain.ActorSystem
	if err := s.repo.UpdateWorkflowMeta(ctx, id, workflowdomain.MetaUpdate{
		NeedsAttention: &needs, AttentionReason: &reason, LastActionBy: &actionBy,
	}); err != nil {
		return nil, fmt.Errorf("workflowapp.SetNeedsAttention: %w", err)
	}
	s.publish(ctx, "attention_changed", id, map[string]any{"needsAttention": needs, "attentionReason": reason})
	return s.repo.GetWorkflow(ctx, id)
}

// MarkRunAttention is the scheduler-facing slice of SetNeedsAttention: idempotent (no
// write, no event when the flag already matches) and error-only — failed runs light the
// banner, a completed run clears it (self-healing semantics, no acknowledge endpoint).
//
// MarkRunAttention 是面向调度器的 SetNeedsAttention 切面：幂等（旗标已一致则不写不发事件）、
// 只返 error——失败 run 点灯、completed run 熄灯（自愈语义，无需 acknowledge 端点）。
func (s *Service) MarkRunAttention(ctx context.Context, id string, needs bool, reason string) error {
	w, err := s.repo.GetWorkflow(ctx, id)
	if err != nil {
		return fmt.Errorf("workflowapp.MarkRunAttention: %w", err)
	}
	if w.NeedsAttention == needs && (!needs || w.AttentionReason == reason) {
		return nil
	}
	_, err = s.SetNeedsAttention(ctx, id, needs, reason)
	return err
}

// Get returns one workflow with its active version + decoded graph attached (graph in one
// round-trip).
//
// Get 返单 workflow 并附上 active 版本 + 解码图（一趟拿到图）。
func (s *Service) Get(ctx context.Context, id string) (*workflowdomain.Workflow, error) {
	w, err := s.repo.GetWorkflow(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("workflowapp.Get: %w", err)
	}
	if w.ActiveVersionID != "" {
		if v, verr := s.repo.GetVersion(ctx, w.ActiveVersionID); verr == nil {
			if g, perr := decodeGraph(v.Graph); perr == nil {
				v.GraphParsed = g
			}
			w.ActiveVersion = v
		}
	}
	return w, nil
}

// List returns a cursor page of live workflows.
func (s *Service) List(ctx context.Context, filter workflowdomain.ListFilter) ([]*workflowdomain.Workflow, string, error) {
	return s.repo.ListWorkflows(ctx, filter)
}

// ListAll returns every live workflow (catalog source).
func (s *Service) ListAll(ctx context.Context) ([]*workflowdomain.Workflow, error) {
	return s.repo.ListAllWorkflows(ctx)
}

// Search filters live workflows by case-insensitive substring over name / description / tags.
//
// Search 按 name / description / tags 大小写不敏感子串过滤活跃 workflow。
func (s *Service) Search(ctx context.Context, query string) ([]*workflowdomain.Workflow, error) {
	all, err := s.repo.ListAllWorkflows(ctx)
	if err != nil {
		return nil, fmt.Errorf("workflowapp.Search: %w", err)
	}
	if strings.TrimSpace(query) == "" {
		return all, nil
	}
	needle := strings.ToLower(query)
	out := make([]*workflowdomain.Workflow, 0, len(all))
	for _, w := range all {
		if strings.Contains(strings.ToLower(w.Name), needle) || strings.Contains(strings.ToLower(w.Description), needle) {
			out = append(out, w)
			continue
		}
		for _, tag := range w.Tags {
			if strings.Contains(strings.ToLower(tag), needle) {
				out = append(out, w)
				break
			}
		}
	}
	return out, nil
}

// Delete soft-deletes the workflow and purges its relation edges.
//
// Delete 软删 workflow 并清理 relation 边。
func (s *Service) Delete(ctx context.Context, id string) error {
	if err := s.repo.DeleteWorkflow(ctx, id); err != nil {
		return fmt.Errorf("workflowapp.Delete: %w", err)
	}
	s.publish(ctx, "deleted", id, nil)
	s.purgeRelations(ctx, id)
	return nil
}

// --- version reads ---

// GetActiveVersion returns the workflow's active version with its graph decoded
// (ErrNoActiveVersion if unset). Part of WorkflowReader (the scheduler's read surface).
//
// GetActiveVersion 返 workflow 的 active 版本并解码其图（未设则 ErrNoActiveVersion）。属
// WorkflowReader（调度器读面）。
func (s *Service) GetActiveVersion(ctx context.Context, id string) (*workflowdomain.Version, error) {
	w, err := s.repo.GetWorkflow(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("workflowapp.GetActiveVersion: %w", err)
	}
	if w.ActiveVersionID == "" {
		return nil, fmt.Errorf("workflowapp.GetActiveVersion: %w", workflowdomain.ErrNoActiveVersion)
	}
	v, err := s.repo.GetVersion(ctx, w.ActiveVersionID)
	if err != nil {
		return nil, fmt.Errorf("workflowapp.GetActiveVersion: %w", err)
	}
	if g, perr := decodeGraph(v.Graph); perr == nil {
		v.GraphParsed = g
	}
	return v, nil
}

// GetWorkflow returns the bare workflow header (no active version attached). Part of
// WorkflowReader.
//
// GetWorkflow 返裸 workflow 头（不附 active 版本）。属 WorkflowReader。
func (s *Service) GetWorkflow(ctx context.Context, id string) (*workflowdomain.Workflow, error) {
	return s.repo.GetWorkflow(ctx, id)
}

// ListActive returns every live workflow with active=true. Part of WorkflowReader (the
// scheduler's candidate set).
//
// ListActive 返所有 active=true 的活跃 workflow。属 WorkflowReader（调度器候选集）。
func (s *Service) ListActive(ctx context.Context) ([]*workflowdomain.Workflow, error) {
	return s.repo.ListActiveWorkflows(ctx)
}

func (s *Service) ListVersions(ctx context.Context, workflowID string, filter workflowdomain.VersionListFilter) ([]*workflowdomain.Version, string, error) {
	return s.repo.ListVersions(ctx, workflowID, filter)
}

func (s *Service) GetVersion(ctx context.Context, versionID string) (*workflowdomain.Version, error) {
	v, err := s.repo.GetVersion(ctx, versionID)
	if err != nil {
		return nil, err
	}
	if g, perr := decodeGraph(v.Graph); perr == nil {
		v.GraphParsed = g
	}
	return v, nil
}

func (s *Service) GetVersionByNumber(ctx context.Context, workflowID string, versionN int) (*workflowdomain.Version, error) {
	v, err := s.repo.GetVersionByNumber(ctx, workflowID, versionN)
	if err != nil {
		return nil, err
	}
	if g, perr := decodeGraph(v.Graph); perr == nil {
		v.GraphParsed = g
	}
	return v, nil
}

// --- graph build pipeline ---

// buildGraph is the shared create/edit core: apply ops to base → ValidateGraph (structural)
// → compile every node.Input CEL via pkg/cel (domain can't import cel-go, 原则 #3). A compile
// error maps to ErrInvalidGraph with the offending node/field in details. It does NOT resolve
// refs — that is CapabilityCheck's job (needs the catalog).
//
// Node Input CEL is compiled per-node against an ANCESTOR-scoped env (compileGraphCEL), enforcing
// the visibility lint: a node may read only the results of nodes guaranteed to have completed before
// it (its ancestors), never an arbitrary existing node — caught at create/edit, not at run time.
//
// buildGraph 是 create/edit 共享内核：套 ops 到 base → ValidateGraph（结构）→ 用 pkg/cel 编译
// 每个 node.Input CEL（domain 不能 import cel-go，原则 #3）。编译错映射 ErrInvalidGraph，违例
// 节点/字段在 details。它不解析 ref——那是 CapabilityCheck 的事（需 catalog）。
//
// 节点 Input CEL 逐节点用**祖先作根**的 ScopedEnv 编译（compileGraphCEL），落实可见性 lint：一个节点只能
// 读保证在它之前已完成的节点（其祖先）的 result，不能读任意存在节点——在 create/edit 当场拒、非运行时。
func (s *Service) buildGraph(base *workflowdomain.Graph, ops []workflowdomain.Op) (*workflowdomain.Graph, error) {
	graph, err := workflowdomain.ApplyOps(base, ops)
	if err != nil {
		return nil, err
	}
	if err := workflowdomain.ValidateGraph(graph); err != nil {
		return nil, err
	}
	if err := compileGraphCEL(graph); err != nil {
		return nil, err
	}
	return graph, nil
}

// compileGraphCEL compiles every node's Input wiring and enforces the ancestor-visibility lint.
// Node Input reads upstream results by node id (`reviewer.score`). Each node is compiled against an
// env whose roots are exactly ITS ancestors (+ the always-present ctx), so:
//   - a syntax error, or a reference to a name that is no node at all → "invalid CEL";
//   - a reference to an existing but NON-ANCESTOR node → "references a non-ancestor node".
//
// The two-tier check (full env first, ancestor env second) is only to tell those two apart for a
// clear authoring message — the ancestor env is what actually enforces visibility. No CEL-AST walk
// is needed: an out-of-scope identifier simply fails to compile.
//
// compileGraphCEL 编译每条 Input 接线并落实祖先可见性 lint。节点 Input 按 node id 读上游结果
// （`reviewer.score`）。每个节点用「**恰为其祖先** + 恒在的 ctx」作根的 env 编译，故：语法错 / 引用根本不存在
// 的名字 →「invalid CEL」；引用存在但**非祖先**的节点 →「references a non-ancestor node」。两段检查（先全图
// env、再祖先 env）只为把这两类区分开给出清晰提示——真正强制可见性的是祖先 env。无需走 CEL AST：越界标识符直接
// 编译失败。
func compileGraphCEL(g *workflowdomain.Graph) error {
	allRoots := make([]string, len(g.Nodes))
	for i := range g.Nodes {
		allRoots[i] = g.Nodes[i].ID
	}
	fullEnv, err := celpkg.NewScopedEnv(allRoots)
	if err != nil {
		return workflowdomain.ErrInvalidGraph.WithDetails(map[string]any{"reason": fmt.Sprintf("cel scope: %v", err)})
	}
	for i := range g.Nodes {
		n := &g.Nodes[i]
		if len(n.Input) == 0 {
			continue // trigger (and any input-less node) reads nothing
		}
		anc := workflowdomain.Ancestors(g, n.ID)
		ancEnv, err := celpkg.NewScopedEnv(anc)
		if err != nil {
			return workflowdomain.ErrInvalidGraph.WithDetails(map[string]any{"reason": fmt.Sprintf("cel scope: %v", err)})
		}
		for field, expr := range n.Input {
			if _, err := fullEnv.Compile(expr); err != nil {
				return workflowdomain.ErrInvalidGraph.WithDetails(map[string]any{
					"reason": fmt.Sprintf("node %q input %q has invalid CEL: %v", n.ID, field, err),
				})
			}
			if _, err := ancEnv.Compile(expr); err != nil {
				return workflowdomain.ErrInvalidGraph.WithDetails(map[string]any{
					"reason": fmt.Sprintf("node %q input %q references a non-ancestor node (only these upstream nodes are visible: %v): %v", n.ID, field, anc, err),
				})
			}
		}
	}
	return nil
}

// activeGraph decodes the workflow's active version's graph (empty graph if no active
// version — the edit then builds from scratch).
//
// activeGraph 解码 workflow active 版本的图（无 active 版本则空图——edit 从零构）。
func (s *Service) activeGraph(ctx context.Context, w *workflowdomain.Workflow) (*workflowdomain.Graph, error) {
	if w.ActiveVersionID == "" {
		return &workflowdomain.Graph{}, nil
	}
	v, err := s.repo.GetVersion(ctx, w.ActiveVersionID)
	if err != nil {
		return nil, err
	}
	return decodeGraph(v.Graph)
}

// --- helpers ---

func newVersion(versionID, workflowID string, versionN int, g *workflowdomain.Graph, changeReason string, now time.Time) *workflowdomain.Version {
	blob, _ := json.Marshal(g) // g is well-formed (built in-process); marshal cannot fail
	return &workflowdomain.Version{
		ID: versionID, WorkflowID: workflowID, Version: versionN,
		Graph: string(blob), ChangeReason: changeReason, CreatedAt: now, UpdatedAt: now,
	}
}

// decodeGraph parses a stored graph blob. An empty/absent blob decodes to an empty graph.
//
// decodeGraph 解析存储的图 blob。空/缺 blob 解为空图。
func decodeGraph(blob string) (*workflowdomain.Graph, error) {
	if strings.TrimSpace(blob) == "" {
		return &workflowdomain.Graph{}, nil
	}
	var g workflowdomain.Graph
	if err := json.Unmarshal([]byte(blob), &g); err != nil {
		return nil, fmt.Errorf("workflowapp: decode graph: %w", err)
	}
	return &g, nil
}

func orEmpty(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
