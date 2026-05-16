// Package settings loads + watches ~/.forgify/settings.json (V1.2 §3
// final-sweep). The Service holds the parsed Settings behind an
// atomic.Value snapshot so the gate's hot path reads without locks.
// Bad JSON is non-fatal — log + keep the last good snapshot.
//
// Package settings 加载 + watch ~/.forgify/settings.json（V1.2 §3）。
// Service 把解析后的 Settings 放 atomic.Value 快照，gate 热路径无锁读。
// 坏 JSON 不致命——log + 保留上次好快照。
package settings

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fsnotify/fsnotify"
	"go.uber.org/zap"

	permdomain "github.com/sunweilin/forgify/backend/internal/domain/permissions"
)

// Service loads + watches settings.json. New() reads once + spawns a
// goroutine that fsnotify-watches the parent dir; Close() stops the
// goroutine. GetRules() returns the latest snapshot atomically.
//
// Service 加载 + watch settings.json。New() 读一次 + spawn 一个
// goroutine fsnotify-watch 父目录；Close() 停 goroutine。GetRules()
// 原子返最新快照。
type Service struct {
	path     string
	log      *zap.Logger
	current  atomic.Pointer[permdomain.Settings]

	mu       sync.Mutex
	watcher  *fsnotify.Watcher
	stopCh   chan struct{}
	stopOnce sync.Once

	// debounce groups rapid file events (most editors trigger a write +
	// chmod + rename burst). 100ms wait after last event before reparse.
	// debounce 组合多次连续事件（编辑器通常 write+chmod+rename）。
	// 最后一次事件 100ms 后 reparse。
	debounceWait time.Duration

	// pollInterval is the fsnotify safety net. Set to 0 to disable
	// (tests). Production = 5s — catches macOS edge cases where
	// fsnotify silently drops events (atomic-rename editors).
	// pollInterval 是 fsnotify 兜底。0 关（测试）。生产 5s——抓 macOS
	// fsnotify 丢事件的 edge case（atomic-rename 编辑器）。
	pollInterval time.Duration

	// lastMod tracks file mtime; poll-detected changes only trigger if it differs.
	// lastMod 记 mtime；poll 检测的改动仅 mtime 变才触发。
	lastMod time.Time
}

// New constructs a Service. path is the absolute settings.json location
// (e.g. ~/.forgify/settings.json — caller resolves ~). Errors loading
// the initial file are non-fatal: an empty Settings is published and
// the watcher still starts so a later valid write picks up.
//
// New 构造 Service。path 是 settings.json 的绝对路径（caller 解 ~）。
// 初次加载失败非致命：发布空 Settings + watcher 仍启动，让后续合法写
// 入被捕获。
func New(path string, log *zap.Logger) *Service {
	if log == nil {
		log = zap.NewNop()
	}
	s := &Service{
		path:         path,
		log:          log.Named("settings"),
		stopCh:       make(chan struct{}),
		debounceWait: 100 * time.Millisecond,
		pollInterval: 5 * time.Second,
	}
	// Publish empty initial snapshot so GetRules() never returns nil.
	// 发布空初始快照让 GetRules() 永不返 nil。
	empty := permdomain.Settings{}
	s.current.Store(&empty)
	if err := s.loadOnce(); err != nil {
		s.log.Warn("initial settings load failed (using empty defaults)",
			zap.String("path", path), zap.Error(err))
	}
	return s
}

// Start spawns the watcher goroutine. Idempotent — second call is no-op.
//
// Start spawn watcher goroutine。幂等——二次调用 no-op。
func (s *Service) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.watcher != nil {
		return nil
	}
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("settings.Start: fsnotify: %w", err)
	}
	// Watch the PARENT dir (file may not exist yet, and atomic-rename
	// editors recreate the inode — watching the file directly misses
	// these). Filter on basename in the loop.
	// watch 父目录（文件可能不存在，atomic-rename 编辑器重建 inode——
	// 直接 watch 文件会漏）。循环里按 basename 过滤。
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		w.Close()
		return fmt.Errorf("settings.Start: mkdir %q: %w", dir, err)
	}
	if err := w.Add(dir); err != nil {
		w.Close()
		return fmt.Errorf("settings.Start: add watch %q: %w", dir, err)
	}
	s.watcher = w
	go s.watchLoop(ctx)
	return nil
}

// Close stops the watcher. Safe to call multiple times.
//
// Close 停 watcher。多次调用安全。
func (s *Service) Close() {
	s.stopOnce.Do(func() {
		close(s.stopCh)
	})
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.watcher != nil {
		s.watcher.Close()
		s.watcher = nil
	}
}

// GetRules returns the latest parsed Settings snapshot. Never nil —
// New() seeds an empty Settings on construction.
//
// GetRules 返最新解析的 Settings 快照。永不 nil——New() 初始化空 Settings。
func (s *Service) GetRules() *permdomain.Settings {
	return s.current.Load()
}

// Reload forces an immediate file re-read. Used by POST /:reload and
// tests. Returns parse / validation errors so callers can surface them.
//
// Reload 强制立即重读文件。POST /:reload 和测试用。返解析 / 校验错让
// caller 暴露。
func (s *Service) Reload() error {
	return s.loadOnce()
}

// loadOnce reads + parses + validates the file, publishing the result
// on success. Failures leave the current snapshot untouched.
//
// loadOnce 读 + 解析 + 校验文件，成功发布。失败保留当前快照。
func (s *Service) loadOnce() error {
	raw, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			// Missing file = use defaults. Don't error.
			// 文件缺 = 用默认。不报错。
			empty := permdomain.Settings{}
			s.current.Store(&empty)
			return nil
		}
		return fmt.Errorf("settings.loadOnce: read %q: %w", s.path, err)
	}
	if len(raw) == 0 {
		empty := permdomain.Settings{}
		s.current.Store(&empty)
		return nil
	}
	var next permdomain.Settings
	if err := json.Unmarshal(raw, &next); err != nil {
		return fmt.Errorf("settings.loadOnce: parse: %w", err)
	}
	if err := next.Validate(); err != nil {
		return fmt.Errorf("settings.loadOnce: validate: %w", err)
	}
	s.current.Store(&next)
	if fi, err := os.Stat(s.path); err == nil {
		s.lastMod = fi.ModTime()
	}
	s.log.Debug("settings reloaded",
		zap.Int("denyRules", len(next.Permissions.Deny)),
		zap.Int("askRules", len(next.Permissions.Ask)),
		zap.Int("allowRules", len(next.Permissions.Allow)),
		zap.Int("preToolUseHooks", len(next.Hooks.PreToolUse)),
		zap.Int("postToolUseHooks", len(next.Hooks.PostToolUse)),
		zap.Int("stopHooks", len(next.Hooks.Stop)))
	return nil
}

// watchLoop debounces fsnotify events + polls every pollInterval as a
// safety net. Exits when stopCh closes or ctx cancels.
//
// watchLoop debounce fsnotify 事件 + pollInterval 兜底轮询。stopCh
// 关或 ctx 取消时退出。
func (s *Service) watchLoop(ctx context.Context) {
	var (
		debounceTimer *time.Timer
		pollTicker    *time.Ticker
	)
	if s.pollInterval > 0 {
		pollTicker = time.NewTicker(s.pollInterval)
		defer pollTicker.Stop()
	}
	defer func() {
		if debounceTimer != nil {
			debounceTimer.Stop()
		}
	}()

	want := filepath.Base(s.path)
	scheduleReload := func() {
		if debounceTimer != nil {
			debounceTimer.Stop()
		}
		debounceTimer = time.AfterFunc(s.debounceWait, func() {
			if err := s.loadOnce(); err != nil {
				s.log.Warn("settings reload failed (keeping last good snapshot)",
					zap.Error(err))
			}
		})
	}

	for {
		var pollCh <-chan time.Time
		if pollTicker != nil {
			pollCh = pollTicker.C
		}
		s.mu.Lock()
		w := s.watcher
		s.mu.Unlock()
		if w == nil {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-s.stopCh:
			return
		case ev, ok := <-w.Events:
			if !ok {
				return
			}
			if filepath.Base(ev.Name) != want {
				continue
			}
			scheduleReload()
		case err, ok := <-w.Errors:
			if !ok {
				return
			}
			s.log.Warn("fsnotify error", zap.Error(err))
		case <-pollCh:
			// Poll-only fallback: check mtime; on change schedule reload.
			// 轮询兜底：看 mtime；变了 schedule reload。
			if fi, err := os.Stat(s.path); err == nil && !fi.ModTime().Equal(s.lastMod) {
				scheduleReload()
			}
		}
	}
}

// SetDebounceWait overrides the default debounce window. For tests only.
//
// SetDebounceWait 覆写默认 debounce 窗口，仅测试用。
func (s *Service) SetDebounceWait(d time.Duration) {
	s.debounceWait = d
}

// SetPollInterval overrides the default poll fallback. For tests only.
//
// SetPollInterval 覆写默认 poll 兜底，仅测试用。
func (s *Service) SetPollInterval(d time.Duration) {
	s.pollInterval = d
}
