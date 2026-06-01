// Package scheduler orchestrates workflow execution: read active Version → persist FlowRun → drive DAG.
//
// Package scheduler 编排 workflow 执行：读 active Version → 持久化 FlowRun → 推 DAG。
package scheduler

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"

	flowrundomain "github.com/sunweilin/forgify/backend/internal/domain/flowrun"
	triggerdomain "github.com/sunweilin/forgify/backend/internal/domain/trigger"
	workflowdomain "github.com/sunweilin/forgify/backend/internal/domain/workflow"
	notificationspkg "github.com/sunweilin/forgify/backend/internal/pkg/notifications"
)

// WorkflowReader is the read-only contract the scheduler consumes from workflowapp.
//
// WorkflowReader 是 scheduler 从 workflowapp 消费的只读契约。
type WorkflowReader interface {
	GetActiveVersion(ctx context.Context, workflowID string) (*workflowdomain.Version, error)
	GetWorkflow(ctx context.Context, workflowID string) (*workflowdomain.Workflow, error)
	ListEnabled(ctx context.Context) ([]*workflowdomain.Workflow, error)
}

// Service orchestrates FlowRun execution; StartRun is the only entry point.
//
// Service 编排 FlowRun 执行；StartRun 是唯一入口。
type Service struct {
	repo         flowrundomain.Repository
	journal      flowrundomain.JournalRepository
	approvals    flowrundomain.ApprovalRepository
	firingInbox  triggerdomain.FiringInbox
	workflowRead WorkflowReader
	notif        notificationspkg.Publisher
	router       *Router
	log          *zap.Logger

	cancelsMu sync.RWMutex
	cancels   map[string]context.CancelFunc
	runWG     sync.WaitGroup // tracks in-flight run goroutines for graceful Drain (lifecycle, M6)

	ExecuteFn func(ctx context.Context, run *flowrundomain.FlowRun, graph *workflowdomain.Graph)
}

// NewService constructs Service; panics on nil log/notif.
//
// NewService 构造 Service；nil log/notif 直接 panic。
func NewService(
	repo flowrundomain.Repository,
	workflowRead WorkflowReader,
	notif notificationspkg.Publisher,
	log *zap.Logger,
) *Service {
	if log == nil {
		panic("schedulerapp.NewService: log is nil")
	}
	if notif == nil {
		panic("schedulerapp.NewService: notif is nil")
	}
	s := &Service{
		repo:         repo,
		workflowRead: workflowRead,
		notif:        notif,
		router:       NewRouter(),
		log:          log.Named("schedulerapp"),
		cancels:      make(map[string]context.CancelFunc),
	}
	s.ExecuteFn = s.executeRun
	return s
}

// SetRouter swaps the Router after constructing production dispatchers.
//
// SetRouter 装配 dispatcher 后替换 Router。
func (s *Service) SetRouter(r *Router) { s.router = r }

// SetJournal injects the durable journal store (ADR-016); executeRun drives the interpreter on it.
//
// SetJournal 注入 durable journal store;executeRun 在其上跑 interpreter。
func (s *Service) SetJournal(j flowrundomain.JournalRepository) { s.journal = j }

// SetApprovals wires the approvals projection store (UI inbox + audit; 17 §9).
func (s *Service) SetApprovals(a flowrundomain.ApprovalRepository) { s.approvals = a }

// ListParkedApprovals returns the ctx user's currently-parked approvals (frontend inbox; 17 §9).
//
// ListParkedApprovals 返当前用户所有 parked approval(前端 inbox)。
func (s *Service) ListParkedApprovals(ctx context.Context) ([]*flowrundomain.Approval, error) {
	if s.approvals == nil {
		return nil, nil
	}
	return s.approvals.ListParked(ctx)
}

// RouterRef returns the current router for test helpers and observability.
//
// RouterRef 返回当前 router，供测试与观测使用。
func (s *Service) RouterRef() *Router { return s.router }

var (
	ErrWorkflowDisabled       = errors.New("scheduler: workflow disabled")
	ErrWorkflowNeedsAttention = errors.New("scheduler: workflow needs attention")
	ErrConcurrencyLimit       = errors.New("scheduler: concurrency limit reached (skipped)")
	ErrWorkflowNotFound       = errors.New("scheduler: workflow not found")
)

// StartRunOptions tweaks one-off behaviour for a single run (dry-run, override timeout, etc).
//
// StartRunOptions 调整单次 run 的临时行为（dry-run、覆盖 timeout 等）。
type StartRunOptions struct {
	DryRun bool // skip side-effect dispatchers; return mock outputs
}

// StartRun spawns a new FlowRun for a workflow trigger; implements SchedulerStarter.
//
// StartRun 为 workflow trigger 启动新 FlowRun，实现 SchedulerStarter。
func (s *Service) StartRun(ctx context.Context, workflowID, triggerKind string, triggerInput map[string]any) (string, error) {
	return s.StartRunWithOptions(ctx, workflowID, triggerKind, triggerInput, StartRunOptions{})
}

// StartRunWithOptions is the full-arg variant; StartRun delegates.
//
// StartRunWithOptions 是带 options 的全参数版本；StartRun 委派。
func (s *Service) StartRunWithOptions(ctx context.Context, workflowID, triggerKind string, triggerInput map[string]any, opts StartRunOptions) (string, error) {
	run, graph, timeoutSec, err := s.buildRun(ctx, workflowID, triggerKind, triggerInput, opts.DryRun)
	if err != nil {
		return "", err
	}
	if err := s.repo.Create(ctx, run); err != nil {
		return "", fmt.Errorf("schedulerapp.StartRun: Create: %w", err)
	}
	s.spawnRun(run, graph, timeoutSec)
	s.publish(ctx, run.ID, workflowID, "started", map[string]any{
		"triggerKind": triggerKind,
	})
	return run.ID, nil
}

// Cancel cancels a running or paused FlowRun. Running: signal the goroutine
// via cancel(); cleanup writes terminal state in executeRun's deferred path.
// Paused: no goroutine exists, so write status=cancelled directly to DB and
// publish a notification ourselves.
//
// Cancel 取消运行中或 paused FlowRun。运行中：发 cancel() 信号，终态写在
// executeRun defer 路径。Paused：没有 goroutine,直接 DB 转 cancelled +
// 自己发通知（v2 burn-in fix #27）。
func (s *Service) Cancel(ctx context.Context, runID string) error {
	s.cancelsMu.RLock()
	cancel, ok := s.cancels[runID]
	s.cancelsMu.RUnlock()
	if ok {
		cancel()
		return nil
	}
	run, err := s.repo.Get(ctx, runID)
	if err != nil {
		// Unknown run / soft-deleted → not cancellable (preserves the
		// pre-#27 contract; ErrNotFound was previously hidden inside the
		// cancels-map miss path).
		//
		// 未知 run / 软删 → not cancellable（保 #27 之前契约：先前 ErrNotFound
		// 被 cancels map miss 路径掩盖）。
		return fmt.Errorf("schedulerapp.Cancel: %w", flowrundomain.ErrNotCancellable)
	}
	// Both legacy paused (old loop body) and awaiting_signal (interpreter approval park) are
	// cancellable parked states. R2: the interpreter parks at awaiting_signal, so the old
	// StatusPaused-only gate made every approval-parked run an uncancellable zombie.
	if run.Status != flowrundomain.StatusPaused && run.Status != flowrundomain.StatusAwaitingSignal {
		return fmt.Errorf("schedulerapp.Cancel: %w", flowrundomain.ErrNotCancellable)
	}
	// Journal the cancellation so a crash-replay observes it (EventFlowrunCancelled was declared but
	// never produced before this).
	if s.journal != nil {
		if _, jErr := s.journal.AppendEvent(ctx, &flowrundomain.FlowRunEvent{
			FlowrunID: runID, Type: flowrundomain.EventFlowrunCancelled,
		}); jErr != nil {
			s.log.Warn("journal flowrun_cancelled failed", zap.String("runId", runID), zap.Error(jErr))
		}
	}
	// Flip any still-parked approval rows to cancelled (best-effort projection; journal is truth).
	if s.approvals != nil {
		if aErr := s.approvals.CancelParked(ctx, runID); aErr != nil {
			s.log.Warn("cancel parked approvals failed", zap.String("runId", runID), zap.Error(aErr))
		}
	}
	now := time.Now().UTC()
	elapsed := now.Sub(run.StartedAt).Milliseconds()
	if err := s.repo.UpdateStatus(ctx, runID, flowrundomain.StatusCancelled, nil, "CANCELLED", "cancelled while parked", &now, elapsed); err != nil {
		return fmt.Errorf("schedulerapp.Cancel: parked→cancelled: %w", err)
	}
	if err := s.repo.ClearPausedState(ctx, runID); err != nil {
		s.log.Warn("clear pausedState after cancel failed", zap.String("runId", runID), zap.Error(err))
	}
	s.publish(ctx, runID, run.WorkflowID, "cancelled", map[string]any{"fromParked": true})
	return nil
}

func (s *Service) releaseCancel(runID string) {
	s.cancelsMu.Lock()
	delete(s.cancels, runID)
	s.cancelsMu.Unlock()
}

func (s *Service) publish(ctx context.Context, runID, workflowID, action string, extra map[string]any) {
	payload := map[string]any{"action": action, "workflowId": workflowID}
	for k, v := range extra {
		payload[k] = v
	}
	s.notif.Publish(ctx, "flowrun", runID, payload, "")
}

func ptrNow() *time.Time {
	t := time.Now().UTC()
	return &t
}
