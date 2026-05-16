// Package catalog is the service layer for the Capability Catalog.
//
// Package catalog 提供 Capability Catalog 的 service 层。
package catalog

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"

	catalogdomain "github.com/sunweilin/forgify/backend/internal/domain/catalog"
	notificationspkg "github.com/sunweilin/forgify/backend/internal/pkg/notifications"
)

const defaultPollInterval = 1 * time.Second

// Generator is the LLM-driven Summary builder; nil falls back to mechanical.
//
// Generator 是 LLM Summary 构建器；nil 时 fallback 到 mechanical。
type Generator interface {
	Generate(ctx context.Context, items []catalogdomain.Item, gMap map[string]catalogdomain.Granularity) (*catalogdomain.Catalog, error)
}

// Service ties sources, polling, cache, disk persistence, and Generator together.
//
// Service 串联 source、轮询、内存与磁盘 cache、以及 Generator。
type Service struct {
	cachePath    string
	pollInterval time.Duration
	notif        notificationspkg.Publisher
	log          *zap.Logger

	generator Generator

	sourcesMu sync.RWMutex
	sources   []catalogdomain.CatalogSource

	cache  atomic.Pointer[catalogdomain.Catalog]
	lastFP atomic.Value // string
	busy   atomic.Bool

	versionMu sync.Mutex
	version   int

	stopOnce   sync.Once
	stopCancel context.CancelFunc
	pollDone   chan struct{}
}

// New constructs a Service rooted at cachePath; Start must run before queries.
//
// New 以 cachePath 为根构造 Service；查询前必须先 Start。
func New(cachePath string, notif notificationspkg.Publisher, log *zap.Logger) *Service {
	if log == nil {
		panic("catalog.New: logger is nil")
	}
	if notif == nil {
		notif = notificationspkg.New(nil, log)
	}
	s := &Service{
		cachePath:    cachePath,
		pollInterval: defaultPollInterval,
		notif:        notif,
		log:          log,
	}
	s.lastFP.Store("")
	return s
}

// SetGenerator injects the LLM Generator post-construction.
//
// SetGenerator 在 New 之后注入 LLM Generator。
func (s *Service) SetGenerator(g Generator) {
	s.generator = g
}

// SetPollInterval overrides the default 1s tick (tests only).
//
// SetPollInterval 覆盖默认 1s tick（仅测试用）。
func (s *Service) SetPollInterval(d time.Duration) {
	if d > 0 {
		s.pollInterval = d
	}
}

// RegisterSource adds a source to the polling rotation; safe at any time.
//
// RegisterSource 把 source 加入轮询，任意时点调用都安全。
func (s *Service) RegisterSource(src catalogdomain.CatalogSource) {
	s.sourcesMu.Lock()
	defer s.sourcesMu.Unlock()
	s.sources = append(s.sources, src)
}

func (s *Service) snapshotSources() []catalogdomain.CatalogSource {
	s.sourcesMu.RLock()
	defer s.sourcesMu.RUnlock()
	out := make([]catalogdomain.CatalogSource, len(s.sources))
	copy(out, s.sources)
	return out
}

// Get returns the cached Catalog or nil before first Refresh; read-only.
//
// Get 返回缓存 Catalog（首次 Refresh 前为 nil），调用方视为只读。
func (s *Service) Get() *catalogdomain.Catalog {
	return s.cache.Load()
}

// GetForSystemPrompt returns the cached Summary text or "" before first build.
//
// GetForSystemPrompt 返回缓存 Summary 文本，构建前为空串。
func (s *Service) GetForSystemPrompt() string {
	cat := s.cache.Load()
	if cat == nil {
		return ""
	}
	return cat.Summary
}

func (s *Service) nextVersion() int {
	s.versionMu.Lock()
	defer s.versionMu.Unlock()
	s.version++
	return s.version
}
