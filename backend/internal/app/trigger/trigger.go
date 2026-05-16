// Package trigger (app layer) integrates the four trigger listener kinds behind one Service.
//
// Package trigger（app 层）把 4 种 trigger listener 整合到一个 Service。
package trigger

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"

	"go.uber.org/zap"

	triggerdomain "github.com/sunweilin/forgify/backend/internal/domain/trigger"
	croninfra "github.com/sunweilin/forgify/backend/internal/infra/trigger/cron"
	fsnotifyinfra "github.com/sunweilin/forgify/backend/internal/infra/trigger/fsnotify"
	webhookinfra "github.com/sunweilin/forgify/backend/internal/infra/trigger/webhook"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

// SchedulerStarter is the port Service uses to dispatch fires to the scheduler.
//
// SchedulerStarter 是 Service 派发 fire 到 scheduler 的端口。
type SchedulerStarter interface {
	StartRun(ctx context.Context, workflowID string, triggerKind string, input map[string]any) (string, error)
}

// Service is the unified trigger surface.
//
// Service 是统一的 trigger 入口。
type Service struct {
	mu        sync.RWMutex
	cron      *croninfra.Listener
	fsnotify  *fsnotifyinfra.Listener
	webhook   *webhookinfra.Listener
	specs     map[string]map[string]triggerdomain.Spec
	scheduler SchedulerStarter
	log       *zap.Logger
}

// New constructs Service; scheduler may be nil and attached later via SetScheduler.
//
// New 构造 Service；scheduler 可为 nil，构造后用 SetScheduler 补。
func New(mux *http.ServeMux, log *zap.Logger) *Service {
	if log == nil {
		panic("triggerapp.New: nil log")
	}
	if mux == nil {
		panic("triggerapp.New: nil mux")
	}
	s := &Service{
		specs: make(map[string]map[string]triggerdomain.Spec),
		log:   log.Named("triggerapp"),
	}

	onFire := func(workflowID, nodeID string, input map[string]any) {
		s.mu.RLock()
		sched := s.scheduler
		s.mu.RUnlock()
		if sched == nil {
			s.log.Warn("trigger fired before scheduler attached — drop",
				zap.String("workflowID", workflowID),
				zap.String("nodeID", nodeID))
			return
		}
		ctx := reqctxpkg.SetUserID(context.Background(), reqctxpkg.DefaultLocalUserID)
		kind := kindForNode(s, workflowID, nodeID)
		runID, err := sched.StartRun(ctx, workflowID, kind, input)
		if err != nil {
			s.log.Error("scheduler.StartRun failed",
				zap.String("workflowID", workflowID),
				zap.String("nodeID", nodeID),
				zap.Error(err))
			return
		}
		s.log.Info("trigger fired",
			zap.String("workflowID", workflowID),
			zap.String("nodeID", nodeID),
			zap.String("runID", runID))
	}

	s.cron = croninfra.New(s.log, onFire)
	s.fsnotify = fsnotifyinfra.New(s.log, onFire)
	s.webhook = webhookinfra.New(mux, s.log, onFire)
	s.cron.Start()
	return s
}

// SetScheduler attaches the scheduler after construction to avoid a ctor cycle.
//
// SetScheduler 构造后挂 scheduler，避免构造循环。
func (s *Service) SetScheduler(starter SchedulerStarter) {
	s.mu.Lock()
	s.scheduler = starter
	s.mu.Unlock()
}

// RegisterTrigger registers a trigger spec to its underlying listener.
//
// RegisterTrigger 把 trigger spec 注册到对应 listener。
func (s *Service) RegisterTrigger(spec triggerdomain.Spec) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	var err error
	switch spec.Kind {
	case triggerdomain.KindCron:
		err = s.cron.Register(spec)
	case triggerdomain.KindFsnotify:
		err = s.fsnotify.Register(spec)
	case triggerdomain.KindWebhook:
		err = s.webhook.Register(spec)
	case triggerdomain.KindManual:
	default:
		return fmt.Errorf("triggerapp.RegisterTrigger: unknown kind %q", spec.Kind)
	}

	if s.specs[spec.WorkflowID] == nil {
		s.specs[spec.WorkflowID] = make(map[string]triggerdomain.Spec)
	}
	s.specs[spec.WorkflowID][spec.NodeID] = spec
	return err
}

// UnregisterByWorkflow removes all triggers for a workflow.
//
// UnregisterByWorkflow 撤掉一个 workflow 关联的所有 trigger。
func (s *Service) UnregisterByWorkflow(workflowID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for nodeID, spec := range s.specs[workflowID] {
		switch spec.Kind {
		case triggerdomain.KindCron:
			s.cron.Unregister(workflowID, nodeID)
		case triggerdomain.KindFsnotify:
			s.fsnotify.Unregister(workflowID, nodeID)
		case triggerdomain.KindWebhook:
			s.webhook.Unregister(workflowID, nodeID)
		}
	}
	delete(s.specs, workflowID)
}

// State returns every registered trigger's state for a workflow.
//
// State 返回某 workflow 下所有已注册 trigger 的状态。
func (s *Service) State(workflowID string) []triggerdomain.State {
	s.mu.RLock()
	specs := s.specs[workflowID]
	s.mu.RUnlock()
	out := make([]triggerdomain.State, 0, len(specs))
	for nodeID, spec := range specs {
		var st triggerdomain.State
		switch spec.Kind {
		case triggerdomain.KindCron:
			st = s.cron.State(workflowID, nodeID)
		case triggerdomain.KindFsnotify:
			st = s.fsnotify.State(workflowID, nodeID)
		case triggerdomain.KindWebhook:
			st = s.webhook.State(workflowID, nodeID)
		case triggerdomain.KindManual:
			st = triggerdomain.State{
				WorkflowID: workflowID, NodeID: nodeID,
				Kind: triggerdomain.KindManual, Status: triggerdomain.StateIdle,
			}
		}
		out = append(out, st)
	}
	return out
}

// Shutdown stops listeners; call at process exit.
//
// Shutdown 停止 listener，进程退出前调用。
func (s *Service) Shutdown() {
	s.cron.Stop()
	s.fsnotify.Stop()
}

func kindForNode(s *Service, workflowID, nodeID string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if m, ok := s.specs[workflowID]; ok {
		if spec, ok := m[nodeID]; ok {
			return spec.Kind
		}
	}
	return triggerdomain.KindManual
}

// ErrSchedulerNotAttached is returned by manual fire when no scheduler has been wired yet.
//
// ErrSchedulerNotAttached 是 scheduler 未挂时 manual fire 返回的错误。
var ErrSchedulerNotAttached = errors.New("triggerapp: scheduler not attached")

// FireManual forwards a manual trigger directly to the scheduler.
//
// FireManual 把手动触发直接转发到 scheduler。
func (s *Service) FireManual(ctx context.Context, workflowID string, input map[string]any) (string, error) {
	s.mu.RLock()
	sched := s.scheduler
	s.mu.RUnlock()
	if sched == nil {
		return "", ErrSchedulerNotAttached
	}
	return sched.StartRun(ctx, workflowID, triggerdomain.KindManual, input)
}
