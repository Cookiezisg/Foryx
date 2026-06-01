package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"go.uber.org/zap"

	workflowdomain "github.com/sunweilin/forgify/backend/internal/domain/workflow"
	idgenpkg "github.com/sunweilin/forgify/backend/internal/pkg/idgen"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

var validNameRe = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_\-]{0,63}$`)

// CreateInput is the request shape for Service.Create.
//
// CreateInput 是 Service.Create 的请求形状。
type CreateInput struct {
	Ops             []Op
	ChangeReason    string
	ProgressBlockID string
	// OnOpApplied is called after each op is applied (optional); used to emit ForgeOpApplied SSE.
	OnOpApplied OnOpApplied
}

// EditInput is the request shape for Service.Edit.
//
// EditInput 是 Service.Edit 的请求形状。
type EditInput struct {
	ID              string
	Ops             []Op
	ChangeReason    string
	ProgressBlockID string
	OnOpApplied     OnOpApplied
}

// UpdateMetaInput patches Workflow metadata without a version bump.
//
// UpdateMetaInput 改 Workflow 元数据，不动版本。
type UpdateMetaInput struct {
	ID             string
	Name           *string
	Description    *string
	Tags           *[]string
	Enabled        *bool
	Concurrency    *string
	NeedsAttention *bool
	AttentionReason *string
}

// List returns a paginated page of live workflows; computed fields are not populated.
//
// List 返当前用户活跃 workflow 分页，计算字段不填。
func (s *Service) List(ctx context.Context, filter workflowdomain.ListFilter) ([]*workflowdomain.Workflow, string, error) {
	if _, err := reqctxpkg.RequireUserID(ctx); err != nil {
		return nil, "", fmt.Errorf("workflowapp.List: %w", err)
	}
	rows, next, err := s.repo.ListWorkflows(ctx, filter)
	if err != nil {
		return nil, "", fmt.Errorf("workflowapp.List: %w", err)
	}
	return rows, next, nil
}

// ListAll returns every live workflow for the current user (no pagination).
//
// ListAll 返当前用户全部活跃 workflow（无分页）。
func (s *Service) ListAll(ctx context.Context) ([]*workflowdomain.Workflow, error) {
	if _, err := reqctxpkg.RequireUserID(ctx); err != nil {
		return nil, fmt.Errorf("workflowapp.ListAll: %w", err)
	}
	rows, err := s.repo.ListAllWorkflows(ctx)
	if err != nil {
		return nil, fmt.Errorf("workflowapp.ListAll: %w", err)
	}
	return rows, nil
}

// Search returns workflows whose name / description / tags contain query (case-insensitive substring).
//
// Search 返 name/description/tags 含 query 子串的 workflow（忽略大小写）。
func (s *Service) Search(ctx context.Context, query string) ([]*workflowdomain.Workflow, error) {
	all, err := s.ListAll(ctx)
	if err != nil {
		return nil, err
	}
	if query == "" {
		return all, nil
	}
	needle := strings.ToLower(query)
	out := make([]*workflowdomain.Workflow, 0, len(all))
	for _, w := range all {
		if strings.Contains(strings.ToLower(w.Name), needle) ||
			strings.Contains(strings.ToLower(w.Description), needle) {
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

// Get fetches one workflow with computed fields populated. attachComputed
// loads pending; LiveRuns / LastFiredAt / NextFireAt are Plan 05 territory.
//
// Get 返单 workflow 含计算字段(pending)。LiveRuns 等留 Plan 05。
func (s *Service) Get(ctx context.Context, id string) (*workflowdomain.Workflow, error) {
	if _, err := reqctxpkg.RequireUserID(ctx); err != nil {
		return nil, fmt.Errorf("workflowapp.Get: %w", err)
	}
	w, err := s.repo.GetWorkflow(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("workflowapp.Get: %w", err)
	}
	s.attachComputed(ctx, w)
	return w, nil
}

// ListVersions paginates a workflow's versions.
//
// ListVersions 返回某 workflow 的版本分页。
func (s *Service) ListVersions(ctx context.Context, workflowID string, filter workflowdomain.VersionListFilter) ([]*workflowdomain.Version, string, error) {
	if _, err := reqctxpkg.RequireUserID(ctx); err != nil {
		return nil, "", fmt.Errorf("workflowapp.ListVersions: %w", err)
	}
	rows, next, err := s.repo.ListVersions(ctx, workflowID, filter)
	if err != nil {
		return nil, "", fmt.Errorf("workflowapp.ListVersions: %w", err)
	}
	for _, v := range rows {
		s.attachGraph(v)
	}
	return rows, next, nil
}

// GetVersion fetches one version by id with GraphParsed populated.
//
// GetVersion 按 id 取版本并填 GraphParsed。
func (s *Service) GetVersion(ctx context.Context, versionID string) (*workflowdomain.Version, error) {
	if _, err := reqctxpkg.RequireUserID(ctx); err != nil {
		return nil, fmt.Errorf("workflowapp.GetVersion: %w", err)
	}
	v, err := s.repo.GetVersion(ctx, versionID)
	if err != nil {
		return nil, fmt.Errorf("workflowapp.GetVersion: %w", err)
	}
	s.attachGraph(v)
	return v, nil
}

// GetVersionByNumber fetches an accepted version by integer number.
//
// GetVersionByNumber 按整数号取 accepted 版本。
func (s *Service) GetVersionByNumber(ctx context.Context, workflowID string, versionN int) (*workflowdomain.Version, error) {
	if _, err := reqctxpkg.RequireUserID(ctx); err != nil {
		return nil, fmt.Errorf("workflowapp.GetVersionByNumber: %w", err)
	}
	v, err := s.repo.GetVersionByNumber(ctx, workflowID, versionN)
	if err != nil {
		return nil, fmt.Errorf("workflowapp.GetVersionByNumber: %w", err)
	}
	s.attachGraph(v)
	return v, nil
}

// GetPending returns the active pending with GraphParsed populated; ErrPendingNotFound if absent.
//
// GetPending 返活动 pending 并填 GraphParsed；无 pending 返 ErrPendingNotFound。
func (s *Service) GetPending(ctx context.Context, workflowID string) (*workflowdomain.Version, error) {
	if _, err := reqctxpkg.RequireUserID(ctx); err != nil {
		return nil, fmt.Errorf("workflowapp.GetPending: %w", err)
	}
	v, err := s.repo.GetPending(ctx, workflowID)
	if err != nil {
		return nil, fmt.Errorf("workflowapp.GetPending: %w", err)
	}
	s.attachGraph(v)
	return v, nil
}

// GetActiveVersion returns the frozen active graph for the scheduler to execute.
//
// GetActiveVersion 返回给 scheduler 执行的冻结活动图。
func (s *Service) GetActiveVersion(ctx context.Context, workflowID string) (*workflowdomain.Version, error) {
	if _, err := reqctxpkg.RequireUserID(ctx); err != nil {
		return nil, fmt.Errorf("workflowapp.GetActiveVersion: %w", err)
	}
	w, err := s.repo.GetWorkflow(ctx, workflowID)
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
	s.attachGraph(v)
	return v, nil
}

// GetWorkflow returns a workflow without computed fields (for schedulers).
//
// GetWorkflow 返 workflow 不填计算字段（scheduler 用）。
func (s *Service) GetWorkflow(ctx context.Context, workflowID string) (*workflowdomain.Workflow, error) {
	if _, err := reqctxpkg.RequireUserID(ctx); err != nil {
		return nil, fmt.Errorf("workflowapp.GetWorkflow: %w", err)
	}
	return s.repo.GetWorkflow(ctx, workflowID)
}

// ListEnabled returns enabled live workflows for the trigger domain to register listeners.
//
// ListEnabled 返已启用的 workflow，供 trigger 域注册 listener。
func (s *Service) ListEnabled(ctx context.Context) ([]*workflowdomain.Workflow, error) {
	if _, err := reqctxpkg.RequireUserID(ctx); err != nil {
		return nil, fmt.Errorf("workflowapp.ListEnabled: %w", err)
	}
	rows, _, err := s.repo.ListWorkflows(ctx, workflowdomain.ListFilter{EnabledOnly: true, Limit: 200})
	if err != nil {
		return nil, fmt.Errorf("workflowapp.ListEnabled: %w", err)
	}
	return rows, nil
}

// Create applies ops and persists Workflow + auto-accepted v1.
//
// Create 应用 ops 并持久化 Workflow + 自动 accept 的 v1。
func (s *Service) Create(ctx context.Context, in CreateInput) (*workflowdomain.Workflow, *workflowdomain.Version, error) {
	uid, err := reqctxpkg.RequireUserID(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("workflowapp.Create: %w", err)
	}

	graph, err := ApplyOps(ctx, nil, in.Ops, in.ProgressBlockID, s.keyProvider, in.OnOpApplied)
	if err != nil {
		return nil, nil, fmt.Errorf("workflowapp.Create: %w", err)
	}
	if graph.Name == "" {
		return nil, nil, fmt.Errorf("workflowapp.Create: %w: graph name is required (use set_meta op)", workflowdomain.ErrOpInvalid)
	}
	if !validNameRe.MatchString(graph.Name) {
		return nil, nil, fmt.Errorf("workflowapp.Create: %w: invalid name %q", workflowdomain.ErrOpInvalid, graph.Name)
	}
	if err := ValidateGraph(ctx, graph, s.checker); err != nil {
		return nil, nil, fmt.Errorf("workflowapp.Create: %w", err)
	}
	existing, err := s.repo.GetWorkflowByName(ctx, graph.Name)
	if err != nil && !errors.Is(err, workflowdomain.ErrNotFound) {
		return nil, nil, fmt.Errorf("workflowapp.Create: dup-check: %w", err)
	}
	if existing != nil {
		return nil, nil, workflowdomain.ErrDuplicateName
	}

	now := time.Now().UTC()
	wfID := idgenpkg.New("wf")
	versionID := idgenpkg.New("wfv")
	versionN := 1
	graphJSON, err := json.Marshal(graph)
	if err != nil {
		return nil, nil, fmt.Errorf("workflowapp.Create: marshal graph: %w", err)
	}

	w := &workflowdomain.Workflow{
		ID:              wfID,
		UserID:          uid,
		Name:            graph.Name,
		Description:     graph.Description,
		Tags:            append([]string(nil), graph.Tags...),
		Enabled:         true,
		Concurrency:     workflowdomain.ConcurrencySerial,
		NeedsAttention:  false,
		ActiveVersionID: versionID,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	v := &workflowdomain.Version{
		ID:           versionID,
		WorkflowID:   wfID,
		Status:       workflowdomain.StatusAccepted,
		Version:      &versionN,
		Graph:        string(graphJSON),
		ChangeReason: in.ChangeReason,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	// Tag the originating conversation if one is present in ctx (set by create_forge tool).
	// Manual HTTP create leaves ForgedInConversationID = nil.
	if convID, ok := reqctxpkg.GetConversationID(ctx); ok {
		v.ForgedInConversationID = &convID
	}
	if err := s.repo.SaveWorkflow(ctx, w); err != nil {
		return nil, nil, fmt.Errorf("workflowapp.Create: SaveWorkflow: %w", err)
	}
	if err := s.repo.SaveVersion(ctx, v); err != nil {
		return nil, nil, fmt.Errorf("workflowapp.Create: SaveVersion: %w", err)
	}
	s.attachGraph(v)
	s.publish(ctx, wfID, "created", map[string]any{"versionId": v.ID, "versionNumber": versionN})

	// Relation hooks: forged edge (from origin conv) + outgoing uses_* + edited (suppressed on Create).
	s.syncRelationsAfterCreate(ctx, wfID, v.ForgedInConversationID)
	s.syncRelationsAfterActiveVersionChange(ctx, wfID)
	// Create auto-accepts v1 + enables (Enabled:true above), so register its trigger listeners now —
	// otherwise an enabled workflow's listeners wouldn't exist until the next boot.
	s.syncActiveTriggers(ctx, wfID)
	return w, v, nil
}

// Edit produces or iterates a pending; empty ops is rejected.
//
// Edit 产出或迭代 pending；ops 为空时返 ErrOpInvalid。
func (s *Service) Edit(ctx context.Context, in EditInput) (*workflowdomain.Version, error) {
	if _, err := reqctxpkg.RequireUserID(ctx); err != nil {
		return nil, fmt.Errorf("workflowapp.Edit: %w", err)
	}
	if len(in.Ops) == 0 {
		return nil, fmt.Errorf("workflowapp.Edit: %w: ops is empty", workflowdomain.ErrOpInvalid)
	}
	w, err := s.repo.GetWorkflow(ctx, in.ID)
	if err != nil {
		return nil, fmt.Errorf("workflowapp.Edit: %w", err)
	}

	pending, perr := s.repo.GetPending(ctx, in.ID)
	switch {
	case perr == nil:
	case errors.Is(perr, workflowdomain.ErrPendingNotFound):
		pending = nil
	default:
		return nil, fmt.Errorf("workflowapp.Edit: pending-check: %w", perr)
	}

	var base *workflowdomain.Graph
	if pending != nil {
		s.attachGraph(pending)
		base = pending.GraphParsed
	} else if w.ActiveVersionID != "" {
		active, err := s.repo.GetVersion(ctx, w.ActiveVersionID)
		if err != nil {
			return nil, fmt.Errorf("workflowapp.Edit: load active: %w", err)
		}
		s.attachGraph(active)
		base = active.GraphParsed
	}

	draft, err := ApplyOps(ctx, base, in.Ops, in.ProgressBlockID, s.keyProvider, in.OnOpApplied)
	if err != nil {
		return nil, fmt.Errorf("workflowapp.Edit: %w", err)
	}
	if err := ValidateGraph(ctx, draft, s.checker); err != nil {
		return nil, fmt.Errorf("workflowapp.Edit: %w", err)
	}
	graphJSON, err := json.Marshal(draft)
	if err != nil {
		return nil, fmt.Errorf("workflowapp.Edit: marshal graph: %w", err)
	}

	now := time.Now().UTC()
	var v *workflowdomain.Version
	if pending != nil {
		pending.Graph = string(graphJSON)
		pending.ChangeReason = in.ChangeReason
		pending.UpdatedAt = now
		if convID, ok := reqctxpkg.GetConversationID(ctx); ok {
			pending.ForgedInConversationID = &convID
		}
		v = pending
	} else {
		v = &workflowdomain.Version{
			ID:           idgenpkg.New("wfv"),
			WorkflowID:   in.ID,
			Status:       workflowdomain.StatusPending,
			Graph:        string(graphJSON),
			ChangeReason: in.ChangeReason,
			CreatedAt:    now,
			UpdatedAt:    now,
		}
		if convID, ok := reqctxpkg.GetConversationID(ctx); ok {
			v.ForgedInConversationID = &convID
		}
	}
	if err := s.repo.SaveVersion(ctx, v); err != nil {
		return nil, fmt.Errorf("workflowapp.Edit: SaveVersion: %w", err)
	}
	s.attachGraph(v)
	s.publish(ctx, in.ID, "pending_created", map[string]any{"versionId": v.ID})
	return v, nil
}

// AcceptPending promotes pending → numbered accepted, flips active_version_id, clears NeedsAttention.
//
// AcceptPending 把 pending 翻为带号 accepted，翻 active_version_id，清 NeedsAttention。
func (s *Service) AcceptPending(ctx context.Context, id string) (*workflowdomain.Version, error) {
	if _, err := reqctxpkg.RequireUserID(ctx); err != nil {
		return nil, fmt.Errorf("workflowapp.AcceptPending: %w", err)
	}
	pending, err := s.repo.GetPending(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("workflowapp.AcceptPending: %w", err)
	}
	nextN, err := s.nextVersionNumber(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("workflowapp.AcceptPending: nextN: %w", err)
	}
	if err := s.repo.UpdateVersionStatus(ctx, pending.ID, workflowdomain.StatusAccepted, &nextN); err != nil {
		return nil, fmt.Errorf("workflowapp.AcceptPending: UpdateStatus: %w", err)
	}
	if err := s.repo.SetActiveVersion(ctx, id, pending.ID); err != nil {
		return nil, fmt.Errorf("workflowapp.AcceptPending: SetActive: %w", err)
	}
	if err := s.repo.SetNeedsAttention(ctx, id, false, ""); err != nil {
		s.log.Warn("workflowapp.AcceptPending: clear needs_attention failed", zap.String("id", id), zap.Error(err))
	}
	if err := s.repo.HardDeleteOldestAccepted(ctx, id, workflowdomain.AcceptedVersionCap); err != nil {
		s.log.Warn("workflowapp.AcceptPending: trim oldest failed", zap.Any("err", err), zap.Any("workflowId", id))
	}
	pending.Status = workflowdomain.StatusAccepted
	pending.Version = &nextN
	s.publish(ctx, id, "version_accepted", map[string]any{"versionId": pending.ID, "versionNumber": nextN})

	// Relation hooks: active_version_id flipped, recompute outgoing + edited
	s.syncRelationsAfterActiveVersionChange(ctx, id)
	// Active graph changed → re-register listeners so they track the new version's trigger set
	// (a new accepted version may add/remove/retune trigger nodes).
	s.syncActiveTriggers(ctx, id)
	return pending, nil
}

// RejectPending hard-deletes the pending Version row.
//
// RejectPending 物理删 pending 行。
func (s *Service) RejectPending(ctx context.Context, id string) error {
	if _, err := reqctxpkg.RequireUserID(ctx); err != nil {
		return fmt.Errorf("workflowapp.RejectPending: %w", err)
	}
	pending, err := s.repo.GetPending(ctx, id)
	if err != nil {
		return fmt.Errorf("workflowapp.RejectPending: %w", err)
	}
	if err := s.repo.HardDeleteVersion(ctx, pending.ID); err != nil {
		return fmt.Errorf("workflowapp.RejectPending: %w", err)
	}
	s.publish(ctx, id, "pending_rejected", map[string]any{"versionId": pending.ID})
	return nil
}

// Revert flips active_version_id to an accepted version identified by its integer number.
//
// Revert 把 active_version_id 翻到指定整数号的 accepted 版本。
func (s *Service) Revert(ctx context.Context, id string, targetVersion int) (*workflowdomain.Version, error) {
	if _, err := reqctxpkg.RequireUserID(ctx); err != nil {
		return nil, fmt.Errorf("workflowapp.Revert: %w", err)
	}
	target, err := s.repo.GetVersionByNumber(ctx, id, targetVersion)
	if err != nil {
		return nil, fmt.Errorf("workflowapp.Revert: %w", err)
	}
	if err := s.repo.SetActiveVersion(ctx, id, target.ID); err != nil {
		return nil, fmt.Errorf("workflowapp.Revert: %w", err)
	}
	s.attachGraph(target)
	s.publish(ctx, id, "reverted", map[string]any{"versionId": target.ID, "versionNumber": targetVersion})

	// Relation hooks: active_version_id flipped backward, recompute outgoing + edited
	s.syncRelationsAfterActiveVersionChange(ctx, id)
	return target, nil
}

// UpdateMeta patches Workflow metadata without creating a new version.
//
// UpdateMeta 改 Workflow 元数据不创建新版本。
func (s *Service) UpdateMeta(ctx context.Context, in UpdateMetaInput) (*workflowdomain.Workflow, error) {
	if _, err := reqctxpkg.RequireUserID(ctx); err != nil {
		return nil, fmt.Errorf("workflowapp.UpdateMeta: %w", err)
	}
	w, err := s.repo.GetWorkflow(ctx, in.ID)
	if err != nil {
		return nil, fmt.Errorf("workflowapp.UpdateMeta: %w", err)
	}
	if in.Name != nil {
		if !validNameRe.MatchString(*in.Name) {
			return nil, fmt.Errorf("workflowapp.UpdateMeta: invalid name %q", *in.Name)
		}
		if *in.Name != w.Name {
			existing, err := s.repo.GetWorkflowByName(ctx, *in.Name)
			if err != nil && !errors.Is(err, workflowdomain.ErrNotFound) {
				return nil, fmt.Errorf("workflowapp.UpdateMeta: dup-check: %w", err)
			}
			if existing != nil && existing.ID != w.ID {
				return nil, workflowdomain.ErrDuplicateName
			}
		}
		w.Name = *in.Name
	}
	if in.Description != nil {
		w.Description = *in.Description
	}
	if in.Tags != nil {
		w.Tags = *in.Tags
	}
	if in.Enabled != nil {
		w.Enabled = *in.Enabled
	}
	if in.Concurrency != nil {
		w.Concurrency = *in.Concurrency
	}
	if in.NeedsAttention != nil {
		w.NeedsAttention = *in.NeedsAttention
	}
	if in.AttentionReason != nil {
		w.AttentionReason = *in.AttentionReason
	}
	if err := s.repo.SaveWorkflow(ctx, w); err != nil {
		return nil, fmt.Errorf("workflowapp.UpdateMeta: %w", err)
	}
	// :activate / :deactivate — register or tear down the workflow's trigger listeners. Before this
	// hook the listeners never registered (the activate handler only flipped `enabled`).
	if in.Enabled != nil {
		s.syncActiveTriggers(ctx, w.ID)
	}
	s.publish(ctx, w.ID, "updated", nil)
	return w, nil
}

// Delete soft-deletes a workflow.
//
// Delete 软删 workflow。
func (s *Service) Delete(ctx context.Context, id string) error {
	if _, err := reqctxpkg.RequireUserID(ctx); err != nil {
		return fmt.Errorf("workflowapp.Delete: %w", err)
	}
	if err := s.repo.DeleteWorkflow(ctx, id); err != nil {
		return fmt.Errorf("workflowapp.Delete: %w", err)
	}
	s.publish(ctx, id, "deleted", nil)
	// Relation hook: cascade purge all edges involving this workflow.
	s.purgeRelations(ctx, id)
	return nil
}

func (s *Service) attachComputed(ctx context.Context, w *workflowdomain.Workflow) {
	if w == nil {
		return
	}
	pending, err := s.repo.GetPending(ctx, w.ID)
	if err == nil {
		s.attachGraph(pending)
		w.Pending = pending
	} else if !errors.Is(err, workflowdomain.ErrPendingNotFound) {
		s.log.Warn("workflowapp.Get: attach pending failed", zap.Any("err", err))
	}
}

func (s *Service) attachGraph(v *workflowdomain.Version) {
	if v == nil || v.Graph == "" {
		return
	}
	var g workflowdomain.Graph
	if err := json.Unmarshal([]byte(v.Graph), &g); err != nil {
		s.log.Warn("workflowapp.attachGraph: unmarshal failed",
			zap.String("versionId", v.ID), zap.Error(err))
		return
	}
	v.GraphParsed = &g
}

func (s *Service) nextVersionNumber(ctx context.Context, workflowID string) (int, error) {
	rows, _, err := s.repo.ListVersions(ctx, workflowID, workflowdomain.VersionListFilter{
		Status: workflowdomain.StatusAccepted,
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

// SetNeedsAttention flags a workflow as needing attention (e.g. trigger exhausted).
// Implements trigger.WorkflowDeactivator.
//
// SetNeedsAttention 标 workflow 需关注（如触发器耗尽）。实现 trigger.WorkflowDeactivator。
func (s *Service) SetNeedsAttention(ctx context.Context, workflowID string, reason string) error {
	if err := s.repo.SetNeedsAttention(ctx, workflowID, true, reason); err != nil {
		return fmt.Errorf("workflowapp.SetNeedsAttention: %w", err)
	}
	s.publish(ctx, workflowID, "trigger_exhausted", map[string]any{"reason": reason})
	return nil
}

func (s *Service) publish(ctx context.Context, workflowID, action string, data map[string]any) {
	envelope := map[string]any{"action": action}
	for k, v := range data {
		envelope[k] = v
	}
	s.notif.Publish(ctx, "workflow", workflowID, envelope, "")
}
