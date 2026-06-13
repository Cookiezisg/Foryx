// Package scheduler is the durable workflow interpreter (M4.3): it walks a flowrun's PINNED
// graph and drives it to completion, crash-recoverably, off the node-result memoization in
// flowrun_nodes. There is no entity here — it is pure orchestration. The whole engine is one
// idempotent advance() function (advance.go): read the run's frn rows + the frozen graph →
// compute which (node, iteration) are ready → run / inline-evaluate them → upsert frn → repeat
// until none ready. A crash just means advance() runs again; completed rows are copied, never
// re-executed (record-once). This deliberately replaces the old event-sourcing engine (doc 17):
// no event journal, no generations, no 14-dispatcher fan-out — see doc 21.
//
// Package scheduler 是 durable workflow 解释器（M4.3）：照 flowrun 钉死的图走、驱动到完成、可崩溃
// 恢复，全靠 flowrun_nodes 的节点结果记忆化。这里无实体——纯编排。整个引擎是一个幂等的 advance()
// （advance.go）：读 run 的 frn 行 + 冻结的图 → 算哪些 (节点,轮次) ready → 跑 / 内联求值 → upsert frn
// → 直到无人 ready。崩溃 = advance() 再跑一遍；completed 行被抄、绝不重跑（record-once）。本包刻意
// 取代旧事件溯源引擎（doc 17）：无事件日志、无 generation、无 14-dispatcher 扇出——见 doc 21。
package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"go.uber.org/zap"

	approvaldomain "github.com/sunweilin/forgify/backend/internal/domain/approval"
	controldomain "github.com/sunweilin/forgify/backend/internal/domain/control"
	flowrundomain "github.com/sunweilin/forgify/backend/internal/domain/flowrun"
	notificationdomain "github.com/sunweilin/forgify/backend/internal/domain/notification"
	streamdomain "github.com/sunweilin/forgify/backend/internal/domain/stream"
	triggerdomain "github.com/sunweilin/forgify/backend/internal/domain/trigger"
	workflowdomain "github.com/sunweilin/forgify/backend/internal/domain/workflow"
	ormpkg "github.com/sunweilin/forgify/backend/internal/pkg/orm"
)

// MaxIterations caps how many loop turns a single back edge may drive before the run is failed —
// a runaway control (always picking its loop port) would otherwise grow frn rows without bound.
// A real loop is bounded by its own CEL guard (e.g. attempt < 3); this is the engine's backstop.
//
// MaxIterations 封顶单条回边能驱动多少轮循环，超出则 run 失败——失控的 control（总选循环 port）否则
// 会无界增长 frn 行。真实循环由自身 CEL guard（如 attempt<3）约束；这是引擎的安全帽。
const MaxIterations = 1000

// Dispatcher runs the two execution-unit node kinds. BOTH are coarse activities: they run to a
// final result and return it; a crash mid-run re-runs the whole unit (at-least-once, doc 21 §8).
// agent has no per-turn sink (resume-mid-agent is v2 — needs a durable loop.Run). M7 adapts the
// function/handler/mcp + agent Services onto this; tests inject a fake.
//
// pinnedVersionID is the run's pin-closure entry for the node's entity ("" when unpinned).
// function/agent execute that frozen version; handler (resident instance = active class code) and
// mcp (unversioned external server) are live-binding and ignore it.
//
// Dispatcher 跑两类执行单元节点。两者都是粗粒度 activity：跑到最终 result 返回；中途崩溃整体重跑
// （at-least-once，doc 21 §8）。agent 无逐轮 sink（resume-mid-agent 是 v2）。M7 把
// function/handler/mcp + agent Service 适配进来；测试注 fake。
//
// pinnedVersionID 是该节点实体在 run pin 闭包里的版本（未 pin 为 ""）。function/agent 执行该冻结
// 版本；handler（常驻实例 = active 类代码）与 mcp（无版本的外部 server）活态绑定、忽略之。
type Dispatcher interface {
	RunAction(ctx context.Context, ref, pinnedVersionID string, input map[string]any) (map[string]any, error)
	RunAgent(ctx context.Context, ref, pinnedVersionID string, input map[string]any) (map[string]any, error)
}

// WorkflowReader is the read surface the interpreter needs. GetVersion(pinnedID) reads the FROZEN
// topology a run executes against (NOT the active version — that may have moved); GetActiveVersion
// + BuildPinClosure are for StartRun's pin step. Satisfied by *workflowapp.Service.
//
// WorkflowReader 是解释器需要的读面。GetVersion(pinnedID) 读 run 执行所依的冻结拓扑（不是 active
// 版本——它可能已移）；GetActiveVersion + BuildPinClosure 供 StartRun 的 pin 步。由
// *workflowapp.Service 实现。
type WorkflowReader interface {
	GetWorkflow(ctx context.Context, id string) (*workflowdomain.Workflow, error)
	GetActiveVersion(ctx context.Context, id string) (*workflowdomain.Version, error)
	GetVersion(ctx context.Context, versionID string) (*workflowdomain.Version, error)
	BuildPinClosure(ctx context.Context, g *workflowdomain.Graph) (map[string]string, error)
}

// ControlResolver / ApprovalResolver resolve a control/approval node's pinned logic for inline
// evaluation. Satisfied by *controlapp.Service / *approvalapp.Service (defined here for DIP).
//
// ControlResolver / ApprovalResolver 解析 control/approval 节点的 pin 逻辑供内联求值。由
// *controlapp.Service / *approvalapp.Service 实现（在此定义以 DIP）。
type ControlResolver interface {
	Resolve(ctx context.Context, id, versionID string) ([]controldomain.Branch, error)
}
type ApprovalResolver interface {
	Resolve(ctx context.Context, id, versionID string) (*approvaldomain.Version, error)
}

// FiringInbox is the trigger firings surface the scheduler drains. ClaimFiring is the single-tx
// claim (ADR-021): it claims a pending firing AND builds the flowrun in ONE transaction via the
// create callback (so there is never a claimed-but-no-run strand). Satisfied by *triggerstore.Store.
// nil-tolerant: a manual-only deployment (or a test) wires no inbox.
//
// FiringInbox 是 scheduler 排空的 trigger firings 面。ClaimFiring 是单事务 claim（ADR-021）：在一个
// 事务内 claim pending firing + 经 create 回调建 flowrun（无 claimed-但-无-run 残留）。由
// *triggerstore.Store 实现。允许 nil：纯手动部署（或测试）不接 inbox。
type FiringInbox interface {
	ListPendingFirings(ctx context.Context, limit int) ([]*triggerdomain.Firing, error)
	ClaimFiring(ctx context.Context, firingID string, create func(tx *ormpkg.DB) (string, error)) (string, error)
	MarkFiringOutcome(ctx context.Context, firingID, status string) error
}

// RunStore is the flowrun persistence the scheduler needs: the domain Repository plus the two
// store-concrete atomic creation methods (they span flowruns + flowrun_nodes in one tx, so they
// are not domain-port material). Satisfied by *flowrunstore.Store.
//
// RunStore 是 scheduler 需要的 flowrun 持久化：domain Repository 加两个 store 具体的原子建-run 方法
// （跨 flowruns + flowrun_nodes 单事务，故非 domain 端口材料）。由 *flowrunstore.Store 实现。
type RunStore interface {
	flowrundomain.Repository
	CreateRunWithTrigger(ctx context.Context, run *flowrundomain.FlowRun, trig *flowrundomain.FlowRunNode) (string, error)
	SeedRunOnTx(ctx context.Context, tx *ormpkg.DB, run *flowrundomain.FlowRun, trig *flowrundomain.FlowRunNode) error
}

// LifecycleReconciler lets the interpreter settle a workflow's graceful-drain when its last in-flight
// run ends: a workflow the user :deactivated while runs were still flying sits in `draining`; when
// the final run settles the interpreter flips it to inactive. nil → no reconcile (manual/test). DIP:
// satisfied by *workflowapp.Service.
//
// LifecycleReconciler 让解释器在某 workflow 最后一个在途 run 结束时结算其优雅排空：用户在仍有 run 在飞时
// :deactivate 的 workflow 处于 draining；最后一个 run 结算时解释器把它翻成 inactive。nil → 不 reconcile
// （手动/测试）。DIP：由 *workflowapp.Service 实现。
type LifecycleReconciler interface {
	MarkInactiveIfDrained(ctx context.Context, workflowID string) error
	// MarkRunAttention lights (failed run) or clears (completed run) the workflow's
	// needs-attention banner. Idempotent on the workflow side.
	// MarkRunAttention 点亮（失败 run）或熄灭（completed run）workflow 的 needs-attention
	// 横幅。workflow 侧幂等。
	MarkRunAttention(ctx context.Context, workflowID string, needs bool, reason string) error
}

// Service is the durable interpreter. inbox may be nil (manual-only). log defaults to nop. inflight
// holds a CancelFunc per actively-advancing run so KillWorkflow can interrupt a run blocked mid-node
// (e.g. inside a long agent) — see kill.go.
//
// Service 是 durable 解释器。inbox 可空（纯手动）。log 默认 nop。inflight 为每个正在 advance 的 run 持一个
// CancelFunc，使 KillWorkflow 能打断卡在节点中（如长 agent）的 run——见 kill.go。
type Service struct {
	runs      RunStore
	workflows WorkflowReader
	control   ControlResolver
	approval  ApprovalResolver
	dispatch  Dispatcher
	inbox     FiringInbox
	entities  streamdomain.Bridge        // entities stream (SSE-C); nil → no workflow-panel run terminal
	recon     LifecycleReconciler        // nil → no drain reconcile
	notif     notificationdomain.Emitter // nil → no run notifications. nil → 无运行通知。
	log       *zap.Logger

	inflightMu sync.Mutex
	inflight   map[string]context.CancelFunc // flowrunID → cancel its in-progress advance
}

// SetEntitiesBridge installs the entities stream post-construction (SSE-C): Advance emits a node
// progress signal per node so the workflow panel shows a run progressing live.
//
// SetEntitiesBridge 装配后装入 entities 流（SSE-C）：Advance 每节点发一条进度信号，使 workflow 面板实时显示运行推进。
func (s *Service) SetEntitiesBridge(b streamdomain.Bridge) { s.entities = b }

// SetLifecycleReconciler installs the drain reconciler post-construction (avoids a DI cycle:
// workflow ← scheduler). nil-tolerant — left unset in manual-only/test wiring.
//
// SetLifecycleReconciler 构造后装入排空 reconciler（避开 DI 环：workflow ← scheduler）。nil-tolerant
// ——手动/测试装配不设。
func (s *Service) SetLifecycleReconciler(r LifecycleReconciler) { s.recon = r }

// SetNotifier installs the notifications emitter post-construction: a failed run and a
// parked approval are the two asynchronous events a user must be summoned back for —
// the panel signal alone only reaches whoever is already watching.
//
// SetNotifier 构造后装入通知发射器：失败 run 与 parked 审批是两类必须把用户**唤回**的
// 异步事件——面板信号只够到正在看的人。
func (s *Service) SetNotifier(e notificationdomain.Emitter) { s.notif = e }

// notify emits one run notification best-effort (a notification must never fail a run).
//
// notify best-effort 发一条运行通知（通知绝不连累 run）。
func (s *Service) notify(ctx context.Context, eventType string, payload map[string]any) {
	if s.notif == nil {
		return
	}
	if err := s.notif.Emit(ctx, eventType, payload); err != nil {
		s.log.Warn("schedulerapp: notify failed (best-effort)", zap.String("event", eventType), zap.Error(err))
	}
}

// NewService wires the interpreter. runs/workflows/control/approval/dispatch are required; inbox is
// optional (nil = manual-only); log nil → nop.
//
// NewService 装配解释器。runs/workflows/control/approval/dispatch 必填；inbox 可选（nil=纯手动）；log nil → nop。
func NewService(runs RunStore, workflows WorkflowReader, control ControlResolver, approval ApprovalResolver, dispatch Dispatcher, inbox FiringInbox, log *zap.Logger) *Service {
	if runs == nil || workflows == nil || control == nil || approval == nil || dispatch == nil {
		panic("schedulerapp.NewService: runs/workflows/control/approval/dispatch are required")
	}
	if log == nil {
		log = zap.NewNop()
	}
	return &Service{runs: runs, workflows: workflows, control: control, approval: approval, dispatch: dispatch, inbox: inbox, log: log, inflight: map[string]context.CancelFunc{}}
}

// decodeGraph parses a version's Graph JSON blob into the typed graph the interpreter walks.
//
// decodeGraph 把版本的 Graph JSON blob 解析成解释器要走的 typed 图。
func decodeGraph(raw string) (*workflowdomain.Graph, error) {
	var g workflowdomain.Graph
	if err := json.Unmarshal([]byte(raw), &g); err != nil {
		return nil, fmt.Errorf("schedulerapp: decode graph: %w", err)
	}
	return &g, nil
}

// entityIDOf strips a node ref to the pin-closure key (the entity id): fn_/ag_/ctl_/apf_/trg_ pass
// through; hd_<id>.method drops the method; mcp:server/tool maps to the server. Mirrors the
// workflow module's pin key derivation so PinnedRefs lookups line up.
//
// entityIDOf 把 node ref 削成 pin 闭包键（实体 id）：fn_/ag_/ctl_/apf_/trg_ 直通；hd_<id>.method 去
// 方法；mcp:server/tool 映射到 server。与 workflow 模块的 pin 键派生一致，使 PinnedRefs 查得上。
func entityIDOf(ref string) string {
	ref = strings.TrimSpace(ref)
	switch {
	case strings.HasPrefix(ref, workflowdomain.RefPrefixHandler):
		if i := strings.IndexByte(ref, '.'); i > 0 {
			return ref[:i]
		}
		return ref
	case strings.HasPrefix(ref, workflowdomain.RefPrefixMCP):
		server := strings.TrimPrefix(ref, workflowdomain.RefPrefixMCP)
		if i := strings.IndexByte(server, '/'); i > 0 {
			return server[:i]
		}
		return server
	default:
		return ref
	}
}
