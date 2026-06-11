// Package fsnotify is the filesystem-watch source listener (wraps fsnotify v1), keyed by
// triggerID. It fires once per filtered file event with an empty dedupKey — each event is a
// genuinely distinct fire (the app derives a per-event key), unlike cron's scheduled tick.
//
// Package fsnotify 是文件系统监听 source listener（封装 fsnotify v1），按 triggerID 键。每条过滤后
// 的文件事件触发一次、dedupKey 传空——每个事件是真正独立的一次触发（app 自生成键），不像 cron 按刻度。
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

	triggerinfra "github.com/sunweilin/forgify/backend/internal/infra/trigger"
)

// Listener manages the fsnotify watcher + per-triggerID registrations.
//
// Listener 管理 fsnotify watcher 与 per-triggerID 注册表。
type Listener struct {
	mu      sync.Mutex
	watcher *notifyfsnotify.Watcher
	specs   map[string]watchSpec // key: triggerID
	report  triggerinfra.ReportFunc
	log     *zap.Logger
	stopCh  chan struct{}
	wg      sync.WaitGroup
}

type watchSpec struct {
	TriggerID string
	Path      string
	Pattern   string
	Events    []notifyfsnotify.Op
}

// New constructs a Listener (the watcher is built lazily on first Register).
//
// New 构造 Listener（watcher 在首次 Register 时延迟建）。
func New(log *zap.Logger, report triggerinfra.ReportFunc) *Listener {
	return &Listener{
		specs:  make(map[string]watchSpec),
		report: report,
		log:    log.Named("trigger.fsnotify"),
		stopCh: make(chan struct{}),
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

// Register adds a watch for triggerID. A missing path returns an error (the app maps it);
// the structural "path present" check already passed at create time (ValidateConfig).
//
// Register 为 triggerID 加 watch。path 缺失返 error（app 映射）；"path 非空" 结构校验 create 时已过。
func (l *Listener) Register(triggerID string, _ string, config map[string]any) error {
	path, _ := config["path"].(string)
	pattern, _ := config["pattern"].(string)
	eventsAny, _ := config["events"].([]any)
	if path == "" {
		return fmt.Errorf("fsnotify.Register %s: empty path", triggerID)
	}
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("fsnotify.Register %s: path %q: %w", triggerID, path, err)
	}
	events := parseEvents(eventsAny)

	l.mu.Lock()
	defer l.mu.Unlock()
	if err := l.ensureWatcher(); err != nil {
		return fmt.Errorf("fsnotify.Register %s: %w", triggerID, err)
	}
	if old, ok := l.specs[triggerID]; ok {
		_ = l.watcher.Remove(old.Path)
		delete(l.specs, triggerID)
	}
	if err := l.watcher.Add(path); err != nil {
		return fmt.Errorf("fsnotify.Register %s: watch add: %w", triggerID, err)
	}
	l.specs[triggerID] = watchSpec{TriggerID: triggerID, Path: path, Pattern: strings.ToLower(pattern), Events: events}
	return nil
}

// Unregister removes triggerID's watch; no-op on unknown key.
//
// Unregister 删 triggerID 的 watch；未知 key 时 no-op。
func (l *Listener) Unregister(triggerID string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if spec, ok := l.specs[triggerID]; ok {
		if l.watcher != nil {
			_ = l.watcher.Remove(spec.Path)
		}
		delete(l.specs, triggerID)
	}
}

// Start is a no-op — the event loop starts lazily in Register's ensureWatcher.
//
// Start 是 no-op——event loop 在 Register 的 ensureWatcher 里延迟启动。
func (l *Listener) Start() {}

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
	evBaseLower := strings.ToLower(filepath.Base(ev.Name))

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
			if matched, _ := filepath.Match(spec.Pattern, evBaseLower); !matched {
				continue
			}
		}
		// Dedup key: path + op + a second bucket — an editor's burst of duplicate events for one
		// save collapses per workflow; a later real change (next second on) fires again.
		// 去重键：path + op + 秒桶——编辑器一次保存的重复事件突发按 workflow 折叠；之后（下一秒起）
		// 的真实变化照常触发。
		l.fire(spec.TriggerID, map[string]any{
			"firedAt":   time.Now(),
			"path":      ev.Name,
			"eventKind": ev.Op.String(),
		}, ev.Name+"|"+ev.Op.String()+"|"+time.Now().UTC().Format("20060102150405"))
	}
}

// fire reports a fired action under a recover so a panicking handler doesn't kill the loop.
//
// fire 在 recover 下报告一次触发，handler panic 不拖垮 event loop。
func (l *Listener) fire(triggerID string, payload map[string]any, dedup string) {
	defer func() {
		if r := recover(); r != nil {
			l.log.Error("fsnotify report panic", zap.String("triggerID", triggerID), zap.Any("recover", r))
		}
	}()
	l.report(triggerID, triggerinfra.Activity{Fired: true, Payload: payload, DedupKey: dedup})
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

var _ triggerinfra.Listener = (*Listener)(nil)
