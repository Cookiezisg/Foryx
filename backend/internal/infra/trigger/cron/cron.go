// Package cron is the cron-listener for the trigger domain (wraps robfig/cron).
//
// Package cron 是 trigger 域的 cron-listener（封装 robfig/cron）。
package cron

import (
	"fmt"
	"sync"
	"time"

	robfigcron "github.com/robfig/cron/v3"
	"go.uber.org/zap"

	triggerdomain "github.com/sunweilin/forgify/backend/internal/domain/trigger"
)

// OnFireFunc is invoked when a cron entry fires; caller wires it to scheduler.StartRun.
//
// OnFireFunc 在 cron entry 触发时调用；调用方接到 scheduler.StartRun。
type OnFireFunc func(workflowID, nodeID string, input map[string]any)

// Listener wraps robfig/cron with per-(workflowID,nodeID) entries + last-fired tracking.
//
// Listener 包 robfig/cron，按 (workflowID,nodeID) 跟 entry 与 last-fired。
type Listener struct {
	mu       sync.Mutex
	cron     *robfigcron.Cron
	entries  map[string]robfigcron.EntryID
	lastFire map[string]time.Time
	onFire   OnFireFunc
	log      *zap.Logger
}

// New constructs a Listener bound to time.Local; caller calls Start to begin scheduling.
//
// New 构造 Listener，时区锁 time.Local；调用方调 Start 才开始调度。
func New(log *zap.Logger, onFire OnFireFunc) *Listener {
	return &Listener{
		cron:     robfigcron.New(robfigcron.WithLocation(time.Local)),
		entries:  make(map[string]robfigcron.EntryID),
		lastFire: make(map[string]time.Time),
		onFire:   onFire,
		log:      log.Named("trigger.cron"),
	}
}

// Register adds or replaces a cron entry; missed runs fire one catch-up.
//
// Register 增加或替换一个 cron entry；漏跑过的会立即补一次。
func (l *Listener) Register(spec triggerdomain.Spec) error {
	expr, _ := spec.Config["expression"].(string)
	if expr == "" {
		return fmt.Errorf("triggercroninfra.Register: %w: empty expression", triggerdomain.ErrInvalidCronExpression)
	}

	schedule, parseErr := robfigcron.ParseStandard(expr)
	if parseErr != nil {
		return fmt.Errorf("triggercroninfra.Register: %w: %v", triggerdomain.ErrInvalidCronExpression, parseErr)
	}

	key := spec.WorkflowID + "/" + spec.NodeID

	l.mu.Lock()
	defer l.mu.Unlock()

	if existing, ok := l.entries[key]; ok {
		l.cron.Remove(existing)
		delete(l.entries, key)
	}

	if last, ok := l.lastFire[key]; ok {
		next := schedule.Next(last)
		if time.Now().After(next) {
			missedSince := last
			go l.onFire(spec.WorkflowID, spec.NodeID, map[string]any{
				"firedAt":     time.Now(),
				"missedSince": missedSince,
				"catchUp":     true,
			})
		}
	}

	id, addErr := l.cron.AddFunc(expr, func() {
		now := time.Now()
		l.mu.Lock()
		l.lastFire[key] = now
		l.mu.Unlock()
		// Recover so an onFire panic doesn't crash the global scheduler.
		// 用 recover 防 onFire panic 把整个 scheduler 拉崩。
		defer func() {
			if r := recover(); r != nil {
				l.log.Error("cron onFire panic",
					zap.String("workflowID", spec.WorkflowID),
					zap.String("nodeID", spec.NodeID),
					zap.Any("recover", r))
			}
		}()
		l.onFire(spec.WorkflowID, spec.NodeID, map[string]any{
			"firedAt": now,
		})
	})
	if addErr != nil {
		return fmt.Errorf("triggercroninfra.Register: %w: %v", triggerdomain.ErrInvalidCronExpression, addErr)
	}
	l.entries[key] = id
	return nil
}

// Unregister removes a cron entry; no-op on unknown key.
//
// Unregister 删 cron entry；未知 key 时 no-op。
func (l *Listener) Unregister(workflowID, nodeID string) {
	key := workflowID + "/" + nodeID
	l.mu.Lock()
	defer l.mu.Unlock()
	if id, ok := l.entries[key]; ok {
		l.cron.Remove(id)
		delete(l.entries, key)
	}
}

func (l *Listener) Start() { l.cron.Start() }

// Stop halts the scheduler and waits for in-flight fires to finish.
//
// Stop 停 scheduler 并等 in-flight fire 跑完。
func (l *Listener) Stop() {
	ctx := l.cron.Stop()
	<-ctx.Done()
}

// State returns the current state for one (workflowID,nodeID) trigger.
//
// State 返某 (workflowID,nodeID) 触发器的当前状态。
func (l *Listener) State(workflowID, nodeID string) triggerdomain.State {
	key := workflowID + "/" + nodeID
	l.mu.Lock()
	defer l.mu.Unlock()
	state := triggerdomain.State{
		WorkflowID: workflowID,
		NodeID:     nodeID,
		Kind:       triggerdomain.KindCron,
		Status:     triggerdomain.StateIdle,
	}
	if last, ok := l.lastFire[key]; ok {
		t := last
		state.LastFiredAt = &t
	}
	if id, ok := l.entries[key]; ok {
		entry := l.cron.Entry(id)
		next := entry.Next
		state.NextFireAt = &next
		state.Status = triggerdomain.StateActive
	}
	return state
}
