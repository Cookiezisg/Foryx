package flowrun

import "context"

// ListFilter paginates a workspace's flowruns (newest-first). An optional WorkflowID narrows to
// one workflow's history.
//
// ListFilter 分页一个 workspace 的 flowrun（最新优先）。可选 WorkflowID 收窄到单个 workflow 历史。
type ListFilter struct {
	WorkflowID string
	Cursor     string
	Limit      int
}

// Repository persists the three flowrun tables. All three are Log tables (D1: never deleted).
// The single-tx firing claim (pending→claimed + flowrun INSERT) is NOT here — it lives on the
// trigger store (it spans trigger_firings) and is handed the flowrun INSERT as a create callback;
// see triggerstore.ClaimFiring. The scheduler writes the claimed run's header + seed trigger node
// through that callback, then uses this Repository for everything after.
//
// Repository 持久化 flowrun 三表。三张都是 Log 表（D1：绝不删）。单事务 firing claim
// （pending→claimed + 建 flowrun）不在此——它住在 trigger store（跨 trigger_firings），以 create 回调
// 接住 flowrun 的 INSERT，见 triggerstore.ClaimFiring。scheduler 经该回调写 claim 后 run 的头 + seed
// trigger 节点，之后一切用本 Repository。
type Repository interface {
	// --- flowruns ---

	// GetRun loads a run header by id; ErrNotFound on miss.
	// GetRun 按 id 取 run 头；未命中 ErrNotFound。
	GetRun(ctx context.Context, id string) (*FlowRun, error)

	// ListRuns pages a workspace's runs newest-first (optionally one workflow's).
	// ListRuns 分页一个 workspace 的 run（最新优先，可限定单 workflow）。
	ListRuns(ctx context.Context, filter ListFilter) ([]*FlowRun, string, error)

	// ListRunningRuns returns every run still in StatusRunning — the boot-recovery candidate set
	// (re-walk each; memoized rows skip, parked rows stay).
	// ListRunningRuns 返所有仍 StatusRunning 的 run——boot 恢复候选集（逐个重走；记忆化行跳过、parked 留）。
	ListRunningRuns(ctx context.Context) ([]*FlowRun, error)

	// CountRunningByWorkflow counts a workflow's currently-running runs (overlap-policy input: serial
	// defers / Skip drops a new firing when this is > 0). Workspace-scoped.
	// CountRunningByWorkflow 数一个 workflow 当前 running 的 run（overlap 策略输入：>0 时 serial 推迟 /
	// Skip 丢弃新 firing）。按 workspace 隔离。
	CountRunningByWorkflow(ctx context.Context, workflowID string) (int, error)

	// ListRunningByWorkflow returns a workflow's currently-running runs — the kill set (kill_workflow
	// cancels each, interrupting any in-flight advance via ctx then marking it cancelled). Workspace-scoped.
	// ListRunningByWorkflow 返一个 workflow 当前 running 的 run——kill 集（kill_workflow 逐个取消：经 ctx
	// 打断在途 advance、再标 cancelled）。按 workspace 隔离。
	ListRunningByWorkflow(ctx context.Context, workflowID string) ([]*FlowRun, error)

	// MarkRunTerminal sets a run's terminal status (completed/failed) + error + completed_at.
	// MarkRunTerminal 置 run 终态（completed/failed）+ error + completed_at。
	MarkRunTerminal(ctx context.Context, id, status, errMsg string) error

	// ReopenForReplay flips a failed run back to running + increments replay_count + clears error
	// (the :replay header half; clearing failed node rows is DeleteFailedNodes). Returns ErrNotReplayable
	// if the run is not currently failed.
	// ReopenForReplay 把 failed run 翻回 running + replay_count++ + 清 error（:replay 的头那半；清 failed
	// 节点行是 DeleteFailedNodes）。run 非 failed 时返 ErrNotReplayable。
	ReopenForReplay(ctx context.Context, id string) error

	// --- flowrun_nodes (record-once truth table) ---

	// InsertNodeResult writes a terminal/parked node row with first-wins semantics: a duplicate on
	// UNIQUE(flowrun_id,node_id,iteration) is silently ignored (inserted=false), never an error.
	// This is the record-once / replay-skip / approval-park-once mechanism.
	// InsertNodeResult 以 first-wins 写一条终态/parked 节点行：UNIQUE(flowrun_id,node_id,iteration) 上的
	// 重复被静默忽略（inserted=false），绝不报错。这是 record-once / 重放跳过 / approval park-once 机制。
	InsertNodeResult(ctx context.Context, n *FlowRunNode) (inserted bool, err error)

	// GetNodes returns all node rows of a run (the full memoization the interpreter re-derives state
	// from). Order is unspecified; the scheduler indexes by (node_id, iteration) in memory.
	// GetNodes 返一个 run 的全部节点行（解释器据以重推状态的全部记忆化）。顺序不定；scheduler 内存按
	// (node_id, iteration) 索引。
	GetNodes(ctx context.Context, flowrunID string) ([]*FlowRunNode, error)

	// ResolveParkedNode flips a parked approval row to a terminal status + result, conditionally on
	// it still being parked — won=false means another writer (human vs timeout) already resolved it
	// (approval first-wins). The race loser is a no-op, not an error.
	// ResolveParkedNode 把一条 parked approval 行翻成终态 + result，条件是它仍 parked——won=false 表示
	// 另一写者（人 vs 超时）已抢先落定（approval first-wins）。竞争输家是 no-op、非错误。
	ResolveParkedNode(ctx context.Context, flowrunID, nodeID, status string, result map[string]any) (won bool, err error)

	// GetParkedNode loads the currently-parked row of (run, node) for the decide path; ErrNodeNotParked
	// if none is awaiting a decision.
	// GetParkedNode 取 (run,node) 当前 parked 行供决策路径；无在等的返 ErrNodeNotParked。
	GetParkedNode(ctx context.Context, flowrunID, nodeID string) (*FlowRunNode, error)

	// ListParkedNodes returns every parked node row in the workspace — the approval inbox (no separate
	// projection table; parked rows ARE the inbox).
	// ListParkedNodes 返 workspace 内所有 parked 节点行——审批收件箱（无独立投影表；parked 行即收件箱）。
	ListParkedNodes(ctx context.Context) ([]*FlowRunNode, error)

	// DeleteFailedNodes hard-deletes a run's failed node rows (the :replay node half — clears the
	// failures so a re-walk re-runs them; completed rows stay memoized). Returns rows removed. This is
	// the ONE permitted physical delete on a Log table: a failed row is a non-result (the activity did
	// not durably complete), so removing it to retry is not erasing history.
	// DeleteFailedNodes 物理删一个 run 的 failed 节点行（:replay 的节点那半——清掉失败让重走重跑；
	// completed 行留作记忆化）。返删除行数。这是 Log 表上唯一允许的物理删：failed 行是非结果（activity
	// 没 durable 完成），删它重试不是抹历史。
	DeleteFailedNodes(ctx context.Context, flowrunID string) (int, error)
}
