// Package flowrun is the orm-backed flowrundomain.Repository: flowruns (header) +
// flowrun_nodes (the node-result memoization truth table). Both are Log tables — NO
// deleted_at (D1); the one permitted physical delete is DeleteFailedNodes (clearing a
// non-result for :replay). Record-once lives on idx_frn_once = UNIQUE(flowrun_id, node_id,
// iteration) (D3): InsertNodeResult is first-wins (a duplicate is silently ignored), and an
// approval decision is a conditional update gated on status='parked' (first-wins again).
// Workspace isolation is automatic (orm ,ws tag); ListRunningRuns deliberately crosses it
// (boot recovery scans every workspace's in-flight runs).
//
// Package flowrun 是 flowrundomain.Repository 的 orm 实现：flowruns（header）+ flowrun_nodes
// （节点结果记忆化真相表）。两张都是 Log 表——无 deleted_at（D1）；唯一允许的物理删是
// DeleteFailedNodes（清 :replay 的非结果行）。record-once 落在 idx_frn_once =
// UNIQUE(flowrun_id,node_id,iteration)（D3）：InsertNodeResult first-wins（重复静默忽略），
// approval 决策是 status='parked' 上的条件更新（同 first-wins）。workspace 隔离自动（orm ,ws）；
// ListRunningRuns 刻意跨 workspace（boot 恢复扫所有 workspace 的在途 run）。
package flowrun

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	flowrundomain "github.com/sunweilin/forgify/backend/internal/domain/flowrun"
	idgenpkg "github.com/sunweilin/forgify/backend/internal/pkg/idgen"
	ormpkg "github.com/sunweilin/forgify/backend/internal/pkg/orm"
)

// Table names, exported so the scheduler's firing-claim callback can bind a Repo on the
// trigger store's transaction (SeedRunOnTx) without re-deriving the strings.
//
// 表名导出，使 scheduler 的 firing-claim 回调能在 trigger store 的事务上绑 Repo（SeedRunOnTx），
// 不必重复字符串。
const (
	TableFlowRuns     = "flowruns"
	TableFlowRunNodes = "flowrun_nodes"
)

// Schema is the 2-table DDL (idempotent). flowruns is the run header; flowrun_nodes is the
// memoization truth table. Neither has deleted_at (Log, D1). idx_frn_once is the record-once
// key (D3, replaces the old journal idx_fre_record_once). idx_fr_running supports cross-ws
// boot recovery; idx_frn_parked supports the approval inbox.
//
// Schema 是 2 表 DDL（幂等）。flowruns 是 run 头；flowrun_nodes 是记忆化真相表。都无 deleted_at
// （Log，D1）。idx_frn_once 是 record-once 键（D3，取代旧 journal idx_fre_record_once）。
// idx_fr_running 支撑跨 ws boot 恢复；idx_frn_parked 支撑审批收件箱。
var Schema = []string{
	`CREATE TABLE IF NOT EXISTS flowruns (
		id            TEXT PRIMARY KEY,
		workspace_id  TEXT NOT NULL,
		workflow_id   TEXT NOT NULL,
		version_id    TEXT NOT NULL,
		pinned_refs   TEXT NOT NULL DEFAULT '{}',
		trigger_id    TEXT NOT NULL DEFAULT '',
		firing_id     TEXT NOT NULL DEFAULT '',
		status        TEXT NOT NULL CHECK (status IN ('running','completed','failed','cancelled')),
		replay_count  INTEGER NOT NULL DEFAULT 0,
		error         TEXT NOT NULL DEFAULT '',
		started_at    DATETIME NOT NULL,
		completed_at  DATETIME,
		updated_at    DATETIME NOT NULL
	)`,
	`CREATE INDEX IF NOT EXISTS idx_fr_ws_created ON flowruns(workspace_id, started_at DESC, id DESC)`,
	`CREATE INDEX IF NOT EXISTS idx_fr_ws_workflow ON flowruns(workspace_id, workflow_id, started_at DESC, id DESC)`,
	`CREATE INDEX IF NOT EXISTS idx_fr_running ON flowruns(status) WHERE status = 'running'`,

	`CREATE TABLE IF NOT EXISTS flowrun_nodes (
		id            TEXT PRIMARY KEY,
		workspace_id  TEXT NOT NULL,
		flowrun_id    TEXT NOT NULL,
		node_id       TEXT NOT NULL,
		iteration     INTEGER NOT NULL DEFAULT 0,
		kind          TEXT NOT NULL,
		ref           TEXT NOT NULL DEFAULT '',
		status        TEXT NOT NULL CHECK (status IN ('completed','failed','parked')),
		result        TEXT NOT NULL DEFAULT '{}',
		error         TEXT NOT NULL DEFAULT '',
		created_at    DATETIME NOT NULL,
		completed_at  DATETIME,
		updated_at    DATETIME NOT NULL
	)`,
	`CREATE UNIQUE INDEX IF NOT EXISTS idx_frn_once ON flowrun_nodes(flowrun_id, node_id, iteration)`,
	`CREATE INDEX IF NOT EXISTS idx_frn_run ON flowrun_nodes(flowrun_id)`,
	`CREATE INDEX IF NOT EXISTS idx_frn_parked ON flowrun_nodes(workspace_id, status) WHERE status = 'parked'`,
}

// Store implements flowrundomain.Repository over pkg/orm, plus the concrete run-creation
// methods (SeedRunOnTx / CreateRunWithTrigger) that span both tables in one transaction.
//
// Store 在 pkg/orm 上实现 flowrundomain.Repository，外加跨两表单事务的具体建-run 方法
// （SeedRunOnTx / CreateRunWithTrigger）。
type Store struct {
	db    *ormpkg.DB
	runs  *ormpkg.Repo[flowrundomain.FlowRun]
	nodes *ormpkg.Repo[flowrundomain.FlowRunNode]
}

// New constructs a Store bound to the two flowrun tables.
//
// New 构造绑定两张 flowrun 表的 Store。
func New(db *ormpkg.DB) *Store {
	return &Store{
		db:    db,
		runs:  ormpkg.For[flowrundomain.FlowRun](db, TableFlowRuns),
		nodes: ormpkg.For[flowrundomain.FlowRunNode](db, TableFlowRunNodes),
	}
}

var _ flowrundomain.Repository = (*Store)(nil)

// --- run creation (store-concrete, NOT in Repository: spans both tables atomically) -------

// SeedRunOnTx creates the run header + seeds its trigger node row on the GIVEN transaction —
// so the firing path can do it inside triggerstore.ClaimFiring's single tx (claim + run in one
// atom, ADR-021). Mints ids when empty. The trigger node IS the entry payload (its result), so
// a run never exists without its seed (no "ran nothing" ghost).
//
// SeedRunOnTx 在给定事务上建 run 头 + seed 它的 trigger 节点行——使 firing 路径能在
// triggerstore.ClaimFiring 的单事务内做（claim+建 run 一个原子，ADR-021）。id 空则铸。trigger
// 节点即入口 payload（它的 result），故 run 绝不无 seed 而存在（无「跑了个寂寞」幽灵）。
func (s *Store) SeedRunOnTx(ctx context.Context, tx *ormpkg.DB, run *flowrundomain.FlowRun, triggerNode *flowrundomain.FlowRunNode) error {
	if run.ID == "" {
		run.ID = idgenpkg.New("fr")
	}
	if run.Status == "" {
		run.Status = flowrundomain.StatusRunning
	}
	triggerNode.FlowRunID = run.ID
	if triggerNode.ID == "" {
		triggerNode.ID = idgenpkg.New("frn")
	}
	if triggerNode.Status == "" {
		triggerNode.Status = flowrundomain.NodeCompleted
	}
	if err := ormpkg.For[flowrundomain.FlowRun](tx, TableFlowRuns).Create(ctx, run); err != nil {
		return fmt.Errorf("flowrunstore.SeedRunOnTx run: %w", err)
	}
	if err := ormpkg.For[flowrundomain.FlowRunNode](tx, TableFlowRunNodes).Create(ctx, triggerNode); err != nil {
		return fmt.Errorf("flowrunstore.SeedRunOnTx trigger node: %w", err)
	}
	return nil
}

// CreateRunWithTrigger is the manual-trigger path: SeedRunOnTx in its own transaction (no firing
// to claim). Returns the run id.
//
// CreateRunWithTrigger 是手动 trigger 路径：在自有事务里 SeedRunOnTx（无 firing 可 claim）。返 run id。
func (s *Store) CreateRunWithTrigger(ctx context.Context, run *flowrundomain.FlowRun, triggerNode *flowrundomain.FlowRunNode) (string, error) {
	err := s.db.Transaction(ctx, func(tx *ormpkg.DB) error {
		return s.SeedRunOnTx(ctx, tx, run, triggerNode)
	})
	if err != nil {
		return "", fmt.Errorf("flowrunstore.CreateRunWithTrigger: %w", err)
	}
	return run.ID, nil
}

// --- flowruns --------------------------------------------------------------

func (s *Store) GetRun(ctx context.Context, id string) (*flowrundomain.FlowRun, error) {
	r, err := s.runs.Get(ctx, id)
	if errors.Is(err, ormpkg.ErrNotFound) {
		return nil, flowrundomain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("flowrunstore.GetRun: %w", err)
	}
	return r, nil
}

func (s *Store) ListRuns(ctx context.Context, filter flowrundomain.ListFilter) ([]*flowrundomain.FlowRun, string, error) {
	q := s.runs.Query()
	if filter.WorkflowID != "" {
		q = q.WhereEq("workflow_id", filter.WorkflowID)
	}
	if filter.Status != "" {
		q = q.WhereEq("status", filter.Status)
	}
	rows, next, err := q.Page(ctx, filter.Cursor, filter.Limit)
	if err != nil {
		return nil, "", fmt.Errorf("flowrunstore.ListRuns: %w", err)
	}
	return rows, next, nil
}

// ListRunningRuns crosses workspaces on purpose: boot recovery runs before any request ctx and
// must re-walk every in-flight run regardless of workspace (the scheduler then advances each in
// a ctx scoped to that run's WorkspaceID).
//
// ListRunningRuns 刻意跨 workspace：boot 恢复在任何请求 ctx 之前跑，须重走每个在途 run（不论
// workspace；scheduler 再在各 run 自己 WorkspaceID 的 ctx 里 advance）。
func (s *Store) ListRunningRuns(ctx context.Context) ([]*flowrundomain.FlowRun, error) {
	rows, err := s.runs.CrossWorkspace().WhereEq("status", flowrundomain.StatusRunning).Order("started_at ASC, id ASC").Find(ctx)
	if err != nil {
		return nil, fmt.Errorf("flowrunstore.ListRunningRuns: %w", err)
	}
	return rows, nil
}

// CountRunningByWorkflow counts a workflow's running runs in the current workspace (overlap input).
//
// CountRunningByWorkflow 数当前 workspace 内某 workflow 的 running run（overlap 输入）。
func (s *Store) CountRunningByWorkflow(ctx context.Context, workflowID string) (int, error) {
	n, err := s.runs.WhereEq("workflow_id", workflowID).WhereEq("status", flowrundomain.StatusRunning).Count(ctx)
	if err != nil {
		return 0, fmt.Errorf("flowrunstore.CountRunningByWorkflow: %w", err)
	}
	return int(n), nil
}

// ListRunningByWorkflow returns one workflow's running runs in the current workspace — the kill set.
//
// ListRunningByWorkflow 返当前 workspace 内某 workflow 的 running run——kill 集。
func (s *Store) ListRunningByWorkflow(ctx context.Context, workflowID string) ([]*flowrundomain.FlowRun, error) {
	rows, err := s.runs.WhereEq("workflow_id", workflowID).WhereEq("status", flowrundomain.StatusRunning).Find(ctx)
	if err != nil {
		return nil, fmt.Errorf("flowrunstore.ListRunningByWorkflow: %w", err)
	}
	return rows, nil
}

// MarkRunTerminal flips a run to a terminal status — GUARDED on it still being running (first-wins).
// kill, finalize (completed), and failRun can race on the same run; whoever updates first wins, the
// loser's UPDATE matches 0 rows and is a no-op (a completed run is never clobbered to cancelled, etc.).
//
// MarkRunTerminal 把 run 翻成终态——守卫在它仍 running（first-wins）。kill、finalize（completed）、
// failRun 可能撞同一 run；先 UPDATE 者赢，输家匹配 0 行 no-op（completed run 绝不被刷成 cancelled 等）。
func (s *Store) MarkRunTerminal(ctx context.Context, id, status, errMsg string) error {
	_, err := s.runs.WhereEq("id", id).WhereEq("status", flowrundomain.StatusRunning).Updates(ctx, map[string]any{
		"status":       status,
		"error":        errMsg,
		"completed_at": time.Now().UTC(),
	})
	if err != nil {
		return fmt.Errorf("flowrunstore.MarkRunTerminal: %w", err)
	}
	return nil
}

func (s *Store) ReopenForReplay(ctx context.Context, id string) error {
	run, err := s.GetRun(ctx, id) // ErrNotFound (ws-scoped) if missing
	if err != nil {
		return err
	}
	if run.Status != flowrundomain.StatusFailed {
		return flowrundomain.ErrNotReplayable
	}
	_, err = s.runs.WhereEq("id", id).Updates(ctx, map[string]any{
		"status":       flowrundomain.StatusRunning,
		"replay_count": run.ReplayCount + 1,
		"error":        "",
		"completed_at": nil,
	})
	if err != nil {
		return fmt.Errorf("flowrunstore.ReopenForReplay: %w", err)
	}
	return nil
}

// --- flowrun_nodes ---------------------------------------------------------

func (s *Store) InsertNodeResult(ctx context.Context, n *flowrundomain.FlowRunNode) (bool, error) {
	if n.ID == "" {
		n.ID = idgenpkg.New("frn")
	}
	if err := s.nodes.Create(ctx, n); err != nil {
		if errors.Is(err, ormpkg.ErrConflict) {
			return false, nil // record-once: the (run,node,iteration) row already exists — first writer won
		}
		return false, fmt.Errorf("flowrunstore.InsertNodeResult: %w", err)
	}
	return true, nil
}

func (s *Store) GetNodes(ctx context.Context, flowrunID string) ([]*flowrundomain.FlowRunNode, error) {
	rows, err := s.nodes.WhereEq("flowrun_id", flowrunID).Find(ctx)
	if err != nil {
		return nil, fmt.Errorf("flowrunstore.GetNodes: %w", err)
	}
	return rows, nil
}

func (s *Store) ResolveParkedNode(ctx context.Context, flowrunID, nodeID, status string, result map[string]any) (bool, error) {
	raw, err := json.Marshal(result)
	if err != nil {
		return false, fmt.Errorf("flowrunstore.ResolveParkedNode marshal: %w", err)
	}
	n, err := s.nodes.
		WhereEq("flowrun_id", flowrunID).
		WhereEq("node_id", nodeID).
		WhereEq("status", flowrundomain.NodeParked).
		Updates(ctx, map[string]any{
			"status":       status,
			"result":       string(raw),
			"completed_at": time.Now().UTC(),
		})
	if err != nil {
		return false, fmt.Errorf("flowrunstore.ResolveParkedNode: %w", err)
	}
	return n > 0, nil
}

func (s *Store) GetParkedNode(ctx context.Context, flowrunID, nodeID string) (*flowrundomain.FlowRunNode, error) {
	n, err := s.nodes.
		WhereEq("flowrun_id", flowrunID).
		WhereEq("node_id", nodeID).
		WhereEq("status", flowrundomain.NodeParked).
		First(ctx)
	if errors.Is(err, ormpkg.ErrNotFound) {
		return nil, flowrundomain.ErrNodeNotParked
	}
	if err != nil {
		return nil, fmt.Errorf("flowrunstore.GetParkedNode: %w", err)
	}
	return n, nil
}

func (s *Store) ListParkedNodes(ctx context.Context) ([]*flowrundomain.FlowRunNode, error) {
	rows, err := s.nodes.WhereEq("status", flowrundomain.NodeParked).Order("created_at ASC, id ASC").Find(ctx)
	if err != nil {
		return nil, fmt.Errorf("flowrunstore.ListParkedNodes: %w", err)
	}
	return rows, nil
}

// DeleteFailedNodes hard-deletes a run's failed rows (flowrun_nodes has no deleted column → the
// query Delete is a physical DELETE). The ONE permitted delete on a Log table: a failed row is a
// non-result, removing it to retry is not erasing history.
//
// DeleteFailedNodes 物理删一个 run 的 failed 行（flowrun_nodes 无 deleted 列 → 查询 Delete 即物理
// DELETE）。Log 表上唯一允许的删：failed 行是非结果，删它重试不是抹历史。
func (s *Store) DeleteFailedNodes(ctx context.Context, flowrunID string) (int, error) {
	n, err := s.nodes.WhereEq("flowrun_id", flowrunID).WhereEq("status", flowrundomain.NodeFailed).Delete(ctx)
	if err != nil {
		return 0, fmt.Errorf("flowrunstore.DeleteFailedNodes: %w", err)
	}
	return int(n), nil
}
