// Package fsnotify is the filesystem-watch trigger listener (wraps fsnotify v1).
//
// Package fsnotify 是文件系统监听 trigger（封装 fsnotify v1）。
package fsnotify

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	notifyfsnotify "github.com/fsnotify/fsnotify"
	"go.uber.org/zap"

	triggerdomain "github.com/sunweilin/forgify/backend/internal/domain/trigger"
)

// OnFireFunc fires once per filtered fsnotify event; caller wires to scheduler.StartRun.
//
// OnFireFunc 每条过滤后的 fsnotify 事件触发一次；调用方接 scheduler.StartRun。
type OnFireFunc func(workflowID, nodeID string, input map[string]any)

// Listener manages the fsnotify watcher + per-key registrations.
//
// Listener 管理 fsnotify watcher 与 per-key 注册表。
type Listener struct {
	mu       sync.Mutex
	watcher  *notifyfsnotify.Watcher
	specs    map[string]watchSpec
	lastFire map[string]time.Time
	states   map[string]triggerdomain.State
	onFire   OnFireFunc
	log      *zap.Logger
	stopCh   chan struct{}
	wg       sync.WaitGroup
}

type watchSpec struct {
	WorkflowID string
	NodeID     string
	Path       string
	Pattern    string
	Events     []notifyfsnotify.Op
}

// New constructs a Listener (watcher is built lazily at first Register).
//
// New 构造 Listener（watcher 在首次 Register 时延迟建）。
func New(log *zap.Logger, onFire OnFireFunc) *Listener {
	return &Listener{
		specs:    make(map[string]watchSpec),
		lastFire: make(map[string]time.Time),
		states:   make(map[string]triggerdomain.State),
		onFire:   onFire,
		log:      log.Named("trigger.fsnotify"),
		stopCh:   make(chan struct{}),
	}
}

func (l *Listener) ensureWatcher() error {
	if l.watcher != nil {
		return nil
	}
	w, err := notifyfsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("fsnotify: new watcher: %w", err)
	}
	l.watcher = w
	l.wg.Add(1)
	go l.runEventLoop()
	return nil
}

// Register adds a watch; path-not-exist flips state=error and returns ErrPathNotExist (fail-soft).
//
// Register 加 watch；path 不存在置 state=error 并返 ErrPathNotExist（fail-soft）。
func (l *Listener) Register(spec triggerdomain.Spec) error {
	path, _ := spec.Config["path"].(string)
	pattern, _ := spec.Config["pattern"].(string)
	eventsAny, _ := spec.Config["events"].([]any)

	key := spec.WorkflowID + "/" + spec.NodeID

	if path == "" {
		l.mu.Lock()
		l.states[key] = triggerdomain.State{
			WorkflowID: spec.WorkflowID, NodeID: spec.NodeID,
			Kind: triggerdomain.KindFsnotify, Status: triggerdomain.StateError,
			LastError: "path is empty",
		}
		l.mu.Unlock()
		return fmt.Errorf("triggerfsnotifyinfra.Register: %w: empty path", triggerdomain.ErrPathNotExist)
	}

	if _, err := os.Stat(path); err != nil {
		l.mu.Lock()
		l.states[key] = triggerdomain.State{
			WorkflowID: spec.WorkflowID, NodeID: spec.NodeID,
			Kind: triggerdomain.KindFsnotify, Status: triggerdomain.StateError,
			LastError: fmt.Sprintf("path %q stat: %v", path, err),
		}
		l.mu.Unlock()
		return fmt.Errorf("triggerfsnotifyinfra.Register: %w: %v", triggerdomain.ErrPathNotExist, err)
	}

	events := parseEvents(eventsAny)

	l.mu.Lock()
	defer l.mu.Unlock()

	if err := l.ensureWatcher(); err != nil {
		return fmt.Errorf("triggerfsnotifyinfra.Register: %w", err)
	}

	if old, ok := l.specs[key]; ok {
		_ = l.watcher.Remove(old.Path)
		delete(l.specs, key)
	}

	if err := l.watcher.Add(path); err != nil {
		l.states[key] = triggerdomain.State{
			WorkflowID: spec.WorkflowID, NodeID: spec.NodeID,
			Kind: triggerdomain.KindFsnotify, Status: triggerdomain.StateError,
			LastError: fmt.Sprintf("watch add: %v", err),
		}
		return fmt.Errorf("triggerfsnotifyinfra.Register: watch add: %w", err)
	}

	l.specs[key] = watchSpec{
		WorkflowID: spec.WorkflowID,
		NodeID:     spec.NodeID,
		Path:       path,
		Pattern:    strings.ToLower(pattern),
		Events:     events,
	}
	l.states[key] = triggerdomain.State{
		WorkflowID: spec.WorkflowID, NodeID: spec.NodeID,
		Kind: triggerdomain.KindFsnotify, Status: triggerdomain.StateActive,
	}
	return nil
}

// Unregister removes a watch; no-op on unknown key.
//
// Unregister 删 watch；未知 key 时 no-op。
func (l *Listener) Unregister(workflowID, nodeID string) {
	key := workflowID + "/" + nodeID
	l.mu.Lock()
	defer l.mu.Unlock()
	if spec, ok := l.specs[key]; ok {
		if l.watcher != nil {
			_ = l.watcher.Remove(spec.Path)
		}
		delete(l.specs, key)
	}
	delete(l.states, key)
}

// State returns the runtime status for one trigger.
//
// State 返某 trigger 的运行时状态。
func (l *Listener) State(workflowID, nodeID string) triggerdomain.State {
	key := workflowID + "/" + nodeID
	l.mu.Lock()
	defer l.mu.Unlock()
	st, ok := l.states[key]
	if !ok {
		return triggerdomain.State{
			WorkflowID: workflowID, NodeID: nodeID,
			Kind: triggerdomain.KindFsnotify, Status: triggerdomain.StateIdle,
		}
	}
	if last, ok := l.lastFire[key]; ok {
		t := last
		st.LastFiredAt = &t
	}
	return st
}

// Stop closes the watcher and waits for the event loop to exit.
//
// Stop 关 watcher 并等 event loop goroutine 退出。
func (l *Listener) Stop() {
	close(l.stopCh)
	if l.watcher != nil {
		_ = l.watcher.Close()
	}
	l.wg.Wait()
}

func (l *Listener) runEventLoop() {
	defer l.wg.Done()
	for {
		select {
		case <-l.stopCh:
			return
		case ev, ok := <-l.watcher.Events:
			if !ok {
				return
			}
			l.dispatch(ev)
		case err, ok := <-l.watcher.Errors:
			if !ok {
				return
			}
			l.log.Warn("fsnotify error", zap.Error(err))
		}
	}
}

func (l *Listener) dispatch(ev notifyfsnotify.Event) {
	l.mu.Lock()
	specs := make([]watchSpec, 0, len(l.specs))
	for _, s := range l.specs {
		specs = append(specs, s)
	}
	l.mu.Unlock()

	evDir := filepath.Dir(ev.Name)
	evBase := filepath.Base(ev.Name)
	evBaseLower := strings.ToLower(evBase)

	for _, spec := range specs {
		watchedAbs, _ := filepath.Abs(spec.Path)
		eventAbs, _ := filepath.Abs(ev.Name)
		if eventAbs != watchedAbs && evDir != watchedAbs && !strings.HasPrefix(eventAbs, watchedAbs+string(filepath.Separator)) {
			continue
		}

		if len(spec.Events) > 0 {
			match := false
			for _, op := range spec.Events {
				if ev.Op&op != 0 {
					match = true
					break
				}
			}
			if !match {
				continue
			}
		}

		if spec.Pattern != "" {
			matched, _ := filepath.Match(spec.Pattern, evBaseLower)
			if !matched {
				continue
			}
		}

		key := spec.WorkflowID + "/" + spec.NodeID
		now := time.Now()
		l.mu.Lock()
		l.lastFire[key] = now
		l.mu.Unlock()

		func(wf, node string, input map[string]any) {
			defer func() {
				if r := recover(); r != nil {
					l.log.Error("fsnotify onFire panic",
						zap.String("workflowID", wf),
						zap.String("nodeID", node),
						zap.Any("recover", r))
				}
			}()
			l.onFire(wf, node, input)
		}(spec.WorkflowID, spec.NodeID, map[string]any{
			"firedAt":   now,
			"path":      ev.Name,
			"eventKind": ev.Op.String(),
		})
	}
}

// parseEvents normalizes a config "events" array into fsnotify Op masks; empty = all.
//
// parseEvents 把 config events 数组转 fsnotify Op masks；空数组 = 全要。
func parseEvents(arr []any) []notifyfsnotify.Op {
	if len(arr) == 0 {
		return nil
	}
	out := make([]notifyfsnotify.Op, 0, len(arr))
	for _, raw := range arr {
		s, _ := raw.(string)
		switch strings.ToLower(s) {
		case "create":
			out = append(out, notifyfsnotify.Create)
		case "modify", "write":
			out = append(out, notifyfsnotify.Write)
		case "delete", "remove":
			out = append(out, notifyfsnotify.Remove)
		case "rename":
			out = append(out, notifyfsnotify.Rename)
		case "chmod":
			out = append(out, notifyfsnotify.Chmod)
		}
	}
	return out
}
