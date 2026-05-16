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
	workflowdomain "github.com/sunweilin/forgify/backend/internal/domain/workflow"
	idgenpkg "github.com/sunweilin/forgify/backend/internal/pkg/idgen"
	notificationspkg "github.com/sunweilin/forgify/backend/internal/pkg/notifications"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
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
	workflowRead WorkflowReader
	notif        notificationspkg.Publisher
	router       *Router
	log          *zap.Logger

	cancelsMu sync.RWMutex
	cancels   map[string]context.CancelFunc

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

// StartRun spawns a new FlowRun for a workflow trigger; implements SchedulerStarter.
//
// StartRun 为 workflow trigger 启动新 FlowRun，实现 SchedulerStarter。
func (s *Service) StartRun(ctx context.Context, workflowID, triggerKind string, triggerInput map[string]any) (string, error) {
	uid, err := reqctxpkg.RequireUserID(ctx)
	if err != nil {
		return "", fmt.Errorf("schedulerapp.StartRun: %w", err)
	}

	wf, err := s.workflowRead.GetWorkflow(ctx, workflowID)
	if err != nil {
		if errors.Is(err, workflowdomain.ErrNotFound) {
			return "", fmt.Errorf("schedulerapp.StartRun: %w", ErrWorkflowNotFound)
		}
		return "", fmt.Errorf("schedulerapp.StartRun: GetWorkflow: %w", err)
	}

	if !wf.Enabled {
		return "", fmt.Errorf("schedulerapp.StartRun: %w", ErrWorkflowDisabled)
	}
	if wf.NeedsAttention {
		return "", fmt.Errorf("schedulerapp.StartRun: %w", ErrWorkflowNeedsAttention)
	}

	if wf.Concurrency == workflowdomain.ConcurrencySerial {
		running, err := s.repo.CountRunning(ctx, workflowID)
		if err != nil {
			return "", fmt.Errorf("schedulerapp.StartRun: CountRunning: %w", err)
		}
		if running >= 1 {
			return "", fmt.Errorf("schedulerapp.StartRun: %w", ErrConcurrencyLimit)
		}
	}

	version, err := s.workflowRead.GetActiveVersion(ctx, workflowID)
	if err != nil {
		return "", fmt.Errorf("schedulerapp.StartRun: GetActiveVersion: %w", err)
	}

	now := time.Now().UTC()
	run := &flowrundomain.FlowRun{
		ID:           idgenpkg.New("fr"),
		UserID:       uid,
		WorkflowID:   workflowID,
		VersionID:    version.ID,
		TriggerKind:  triggerKind,
		TriggerInput: triggerInput,
		Status:       flowrundomain.StatusRunning,
		StartedAt:    now,
	}
	if err := s.repo.Create(ctx, run); err != nil {
		return "", fmt.Errorf("schedulerapp.StartRun: Create: %w", err)
	}

	runCtx := reqctxpkg.SetUserID(context.Background(), uid)
	runCtx, cancel := context.WithCancel(runCtx)
	s.cancelsMu.Lock()
	s.cancels[run.ID] = cancel
	s.cancelsMu.Unlock()

	graph := version.GraphParsed
	go func() {
		defer s.releaseCancel(run.ID)
		defer func() {
			if r := recover(); r != nil {
				s.log.Error("scheduler.executeRun panic",
					zap.String("runID", run.ID), zap.Any("recover", r))
				_ = s.repo.UpdateStatus(runCtx, run.ID, flowrundomain.StatusFailed,
					nil, "INTERNAL_PANIC", fmt.Sprintf("%v", r),
					ptrNow(), 0)
			}
		}()
		s.ExecuteFn(runCtx, run, graph)
	}()

	s.publish(ctx, run.ID, workflowID, "started", map[string]any{
		"triggerKind": triggerKind,
	})
	return run.ID, nil
}

// Cancel cancels a running or paused FlowRun; cleanup runs in executeRun's deferred path.
//
// Cancel 取消运行中或 paused FlowRun；清理在 executeRun defer 路径。
func (s *Service) Cancel(_ context.Context, runID string) error {
	s.cancelsMu.RLock()
	cancel, ok := s.cancels[runID]
	s.cancelsMu.RUnlock()
	if !ok {
		return fmt.Errorf("schedulerapp.Cancel: %w", flowrundomain.ErrNotCancellable)
	}
	cancel()
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
