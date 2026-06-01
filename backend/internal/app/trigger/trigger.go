// Package trigger (app layer) integrates the four trigger listener kinds behind one Service.
//
// Package trigger（app 层）把 4 种 trigger listener 整合到一个 Service。
package trigger

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	"go.uber.org/zap"

	workflowapp "github.com/sunweilin/forgify/backend/internal/app/workflow"
	triggerdomain "github.com/sunweilin/forgify/backend/internal/domain/trigger"
	croninfra "github.com/sunweilin/forgify/backend/internal/infra/trigger/cron"
	fsnotifyinfra "github.com/sunweilin/forgify/backend/internal/infra/trigger/fsnotify"
	pollinginfra "github.com/sunweilin/forgify/backend/internal/infra/trigger/polling"
	webhookinfra "github.com/sunweilin/forgify/backend/internal/infra/trigger/webhook"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

// SchedulerStarter is the port Service uses to dispatch fires to the scheduler.
//
// SchedulerStarter 是 Service 派发 fire 到 scheduler 的端口。
type SchedulerStarter interface {
	StartRun(ctx context.Context, workflowID string, triggerKind string, input map[string]any) (string, error)
	// OnTriggerFired persists a firing (durable, persist-before-act) then drains the inbox via the
	// single-tx claim (ADR-021). The durable trigger path; StartRun stays for manual / dry-run.
	OnTriggerFired(ctx context.Context, firing *triggerdomain.TriggerFiring) error
}

// PollingCursorStore extends ScheduleStore with cursor persistence for polling triggers.
// Implemented by infra/store/trigger.Store.
//
// PollingCursorStore 扩展 ScheduleStore，加 polling cursor 持久化。
type PollingCursorStore interface {
	pollinginfra.CursorStore
}

// ScheduleStore is the port Service uses to persist trigger schedules (lastFiredAt, upsert, failure tracking).
// Implemented by infra/store/trigger.Store; nil disables persistence (in-memory only, legacy).
//
// ScheduleStore 是 Service 持久化 trigger schedule 的端口。nil = 仅内存(旧行为)。
type ScheduleStore interface {
	UpsertSchedule(ctx context.Context, sched *triggerdomain.TriggerSchedule) error
	GetSchedule(ctx context.Context, workflowID, nodeID string) (*triggerdomain.TriggerSchedule, error)
	UpdateLastFiredAt(ctx context.Context, workflowID, nodeID string, t time.Time) error
	// IncrementConsecutiveFailures atomically increments the consecutive failure counter and returns the new value.
	IncrementConsecutiveFailures(ctx context.Context, workflowID, nodeID string) (int, error)
	// ResetConsecutiveFailures resets the counter to 0 after a successful fire.
	ResetConsecutiveFailures(ctx context.Context, workflowID, nodeID string) error
}

// WorkflowDeactivator is the port trigger.Service uses to mark a workflow as needing attention
// after repeated trigger failures (trigger exhausted → workflow.needs_attention=true).
//
// WorkflowDeactivator 是 trigger.Service 在触发器多次失败后标记 needs_attention 的端口。
type WorkflowDeactivator interface {
	SetNeedsAttention(ctx context.Context, workflowID string, reason string) error
}

// maxConsecutiveTriggerFailures is the threshold after which a trigger is considered exhausted.
// The workflow is then flagged as needs_attention so the user knows to investigate.
const maxConsecutiveTriggerFailures = 5

// Service is the unified trigger surface.
//
// Service 是统一的 trigger 入口。
type Service struct {
	mu               sync.RWMutex
	cron             *croninfra.Listener
	fsnotify         *fsnotifyinfra.Listener
	webhook          *webhookinfra.Listener
	polling          *pollinginfra.Listener
	specs            map[string]map[string]triggerdomain.Spec
	scheduler        SchedulerStarter
	scheduleStore    ScheduleStore
	pollingCursor    PollingCursorStore
	workflowDeact    WorkflowDeactivator
	onFire           func(workflowID, nodeID string, input map[string]any, dedupKey string)
	log              *zap.Logger
}

// SetWorkflowDeactivator wires the workflow deactivator post-construction.
//
// SetWorkflowDeactivator 构造后挂 workflow deactivator。
func (s *Service) SetWorkflowDeactivator(d WorkflowDeactivator) {
	s.mu.Lock()
	s.workflowDeact = d
	s.mu.Unlock()
}

// SetScheduleStore attaches the schedule store post-construction for cross-restart lastFiredAt.
//
// SetScheduleStore 构造后挂 schedule store，供跨重启 lastFiredAt 持久化。
func (s *Service) SetScheduleStore(ss ScheduleStore) {
	s.mu.Lock()
	s.scheduleStore = ss
	s.mu.Unlock()
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

	onFire := func(workflowID, nodeID string, input map[string]any, dedupKey string) {
		s.mu.RLock()
		sched := s.scheduler
		spec, ok := s.specs[workflowID][nodeID]
		s.mu.RUnlock()
		if sched == nil {
			s.log.Warn("trigger fired before scheduler attached — drop",
				zap.String("workflowID", workflowID),
				zap.String("nodeID", nodeID))
			return
		}
		// §multi-user: workflow owner comes from the registered Spec. No
		// fallback — every Spec is registered with an explicit UserID.
		// Missing = wiring bug: log + drop the trigger; running under a
		// magic default would silently attribute the run to the wrong user.
		//
		// §multi-user: workflow owner 来自注册的 Spec,没有兜底。
		// 缺 UserID = 接线 bug,记日志后丢弃这次触发。
		if !ok || spec.UserID == "" {
			s.log.Error("trigger fired but workflow spec has no owner — drop",
				zap.String("workflowID", workflowID),
				zap.String("nodeID", nodeID))
			return
		}
		ctx := reqctxpkg.SetUserID(context.Background(), spec.UserID)
		kind := kindForNode(s, workflowID, nodeID)
		// Persist-before-act: write a durable firing, then the scheduler drains it via the single-tx
		// claim (ADR-021). The listener supplies its natural idempotency key: cron keys on the SCHEDULED
		// TICK so a missed-tick catch-up that re-materializes an already-fired tick dedups against it;
		// fsnotify/webhook pass "" (each event/request is a distinct fire → per-event wall-clock key).
		if dedupKey == "" {
			dedupKey = workflowID + "|" + nodeID + "|" + strconv.FormatInt(time.Now().UnixNano(), 10)
		}
		firing := &triggerdomain.TriggerFiring{
			WorkflowID:    workflowID,
			TriggerNodeID: nodeID,
			TriggerKind:   kind,
			Payload:       input,
			DedupKey:      dedupKey,
		}
		if err := sched.OnTriggerFired(ctx, firing); err != nil {
			s.log.Error("scheduler.OnTriggerFired failed",
				zap.String("workflowID", workflowID),
				zap.String("nodeID", nodeID),
				zap.Error(err))
			// Track consecutive failures; if threshold exceeded, flag workflow as needs_attention.
			s.mu.RLock()
			ss := s.scheduleStore
			deact := s.workflowDeact
			s.mu.RUnlock()
			if ss != nil {
				n, incErr := ss.IncrementConsecutiveFailures(context.Background(), workflowID, nodeID)
				if incErr == nil && n >= maxConsecutiveTriggerFailures && deact != nil {
					s.log.Warn("trigger: exhausted consecutive failures, flagging workflow needs_attention",
						zap.String("workflowID", workflowID), zap.Int("failures", n))
					if dErr := deact.SetNeedsAttention(context.Background(), workflowID,
						fmt.Sprintf("trigger node %s failed %d times in a row", nodeID, n)); dErr != nil {
						s.log.Warn("trigger: SetNeedsAttention failed", zap.Error(dErr))
					}
				}
			}
			return
		}
		// Persist lastFiredAt so cron can detect missed ticks across process restarts.
		// Best-effort: a store failure only costs the cross-restart catch-up, not the firing itself.
		s.mu.RLock()
		ss := s.scheduleStore
		s.mu.RUnlock()
		if ss != nil {
			now := time.Now().UTC()
			if uErr := ss.UpdateLastFiredAt(context.Background(), workflowID, nodeID, now); uErr != nil {
				s.log.Warn("trigger: UpdateLastFiredAt failed (cross-restart catch-up may miss this tick)",
					zap.String("workflowID", workflowID), zap.String("nodeID", nodeID), zap.Error(uErr))
			}
			// Reset failure counter on success.
			_ = ss.ResetConsecutiveFailures(context.Background(), workflowID, nodeID)
		}
		s.log.Info("trigger fired (durable)",
			zap.String("workflowID", workflowID),
			zap.String("nodeID", nodeID))
	}

	s.onFire = onFire
	s.cron = croninfra.New(s.log, onFire)
	s.fsnotify = fsnotifyinfra.New(s.log, onFire)
	s.webhook = webhookinfra.New(mux, s.log, onFire)
	// polling is nil until SetPollingCallable is called — polling triggers can only be registered
	// after a FunctionCaller is wired. A nil polling listener skips polling registrations gracefully.
	s.cron.Start()
	return s
}

// SetPollingCallable wires the function executor and cursor store for polling triggers.
// Must be called before any KindPolling trigger is registered; no-op if already set.
//
// SetPollingCallable 挂载 polling trigger 所需的 function executor 和 cursor store。
func (s *Service) SetPollingCallable(callable FunctionCaller, cursor PollingCursorStore) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.polling != nil {
		return // already set
	}
	s.pollingCursor = cursor
	s.polling = pollinginfra.New(callable, cursor, s.log, s.onFire)
}

// SetScheduler attaches the scheduler after construction to avoid a ctor cycle.
//
// SetScheduler 构造后挂 scheduler，避免构造循环。
func (s *Service) SetScheduler(starter SchedulerStarter) {
	s.mu.Lock()
	s.scheduler = starter
	s.mu.Unlock()
}

// SyncWorkflowTriggers implements workflowapp.TriggerSync — the workflow→trigger wire invoked on
// :activate / :deactivate. It clears any stale registrations (idempotent), then on enable registers
// each trigger node's listener. Owner userID comes from ctx (set by the activate request or boot loop).
// A bad listener spec (e.g. invalid cron) is returned as firstErr but does not abort the others —
// fail-soft, matching the listener Register contract; the trigger State endpoint surfaces the error.
//
// SyncWorkflowTriggers 实现 workflowapp.TriggerSync —— :activate/:deactivate 的 workflow→trigger 接线。
func (s *Service) SyncWorkflowTriggers(ctx context.Context, workflowID string, enabled bool, triggers []workflowapp.TriggerNodeInfo) error {
	s.UnregisterByWorkflow(workflowID) // idempotent: drop stale listeners from a prior version/activate
	if !enabled {
		return nil
	}
	uid, _ := reqctxpkg.GetUserID(ctx)
	var firstErr error
	for _, t := range triggers {
		err := s.RegisterTrigger(triggerdomain.Spec{
			WorkflowID: workflowID,
			UserID:     uid,
			NodeID:     t.NodeID,
			Kind:       t.Kind,
			Config:     t.Config,
		})
		if err != nil {
			s.log.Warn("triggerapp.SyncWorkflowTriggers: RegisterTrigger failed",
				zap.String("workflowId", workflowID), zap.String("nodeId", t.NodeID),
				zap.String("kind", t.Kind), zap.Error(err))
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

// RegisterTrigger registers a trigger spec to its underlying listener.
//
// RegisterTrigger 把 trigger spec 注册到对应 listener。
func (s *Service) RegisterTrigger(spec triggerdomain.Spec) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Seed cron.lastFire from the persisted TriggerSchedule.LastFiredAt so missed-tick catch-up
	// survives process restarts. We do this BEFORE Register() so the loaded lastFire is in place
	// when the cron listener checks it during registration.
	if spec.Kind == triggerdomain.KindCron && s.scheduleStore != nil {
		if row, gErr := s.scheduleStore.GetSchedule(context.Background(), spec.WorkflowID, spec.NodeID); gErr == nil && row != nil && row.LastFiredAt != nil {
			spec.LastFiredAt = row.LastFiredAt
		}
	}

	var err error
	switch spec.Kind {
	case triggerdomain.KindCron:
		err = s.cron.RegisterWithLastFire(spec)
	case triggerdomain.KindFsnotify:
		err = s.fsnotify.Register(spec)
	case triggerdomain.KindWebhook:
		err = s.webhook.Register(spec)
	case triggerdomain.KindPolling:
		if s.polling == nil {
			return fmt.Errorf("triggerapp.RegisterTrigger: polling listener not configured (call SetPollingCallable first)")
		}
		err = s.polling.Register(spec)
	case triggerdomain.KindManual:
	default:
		return fmt.Errorf("triggerapp.RegisterTrigger: unknown kind %q", spec.Kind)
	}

	// Persist schedule registration (upsert idempotent). Best-effort.
	if s.scheduleStore != nil && err == nil {
		row := &triggerdomain.TriggerSchedule{
			WorkflowID:    spec.WorkflowID,
			TriggerNodeID: spec.NodeID,
			Kind:          spec.Kind,
			Spec:          spec.Config,
		}
		if uErr := s.scheduleStore.UpsertSchedule(context.Background(), row); uErr != nil {
			s.log.Warn("trigger: UpsertSchedule failed (non-fatal)", zap.Error(uErr))
		}
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
		case triggerdomain.KindPolling:
			if s.polling != nil {
				s.polling.Unregister(workflowID, nodeID)
			}
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
		case triggerdomain.KindPolling:
			if s.polling != nil {
				if ps := s.polling.State(workflowID, nodeID); ps != nil {
					st = *ps
				}
			}
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
	if s.polling != nil {
		s.polling.Stop()
	}
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

// FunctionCaller is the port for polling-trigger function invocation.
// Implemented by a thin adapter wrapping app/function.Service in main.go.
//
// FunctionCaller 是 polling trigger 调 function 的端口；main.go 中的薄适配器实现。
type FunctionCaller interface {
	CallFunction(ctx context.Context, userID, functionID string, args map[string]any) (map[string]any, error)
}
