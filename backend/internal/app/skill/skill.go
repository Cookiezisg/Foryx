// Package skill is the service layer for Anthropic Agent Skills.
//
// Package skill 是 Agent Skills 的服务层。
package skill

import (
	"context"
	"sort"
	"sync"
	"sync/atomic"

	"go.uber.org/zap"

	subagentapp "github.com/sunweilin/forgify/backend/internal/app/subagent"
	apikeydomain "github.com/sunweilin/forgify/backend/internal/domain/apikey"
	modeldomain "github.com/sunweilin/forgify/backend/internal/domain/model"
	skilldomain "github.com/sunweilin/forgify/backend/internal/domain/skill"
	llminfra "github.com/sunweilin/forgify/backend/internal/infra/llm"
	notificationspkg "github.com/sunweilin/forgify/backend/internal/pkg/notifications"
)

// SubagentService is the port skill uses to dispatch fork-mode skills.
//
// SubagentService 是 skill 派发 fork 模式时用的端口。
type SubagentService interface {
	Spawn(ctx context.Context, typeName, prompt string, opts subagentapp.SpawnOpts) (*subagentapp.SpawnResult, error)
}

// Service ties scan, cache, search/activate, and fork-mode dispatch together.
//
// Service 串联磁盘扫描、元数据缓存、search/activate 与 fork 模式派发。
type Service struct {
	skillsDir   string
	subagent    SubagentService
	modelPicker modeldomain.ModelPicker
	keyProvider apikeydomain.KeyProvider
	llmFactory  *llminfra.Factory
	notif       notificationspkg.Publisher
	log         *zap.Logger

	execRepo skilldomain.ExecutionRepository

	mu     sync.RWMutex
	skills map[string]*skilldomain.Skill

	lastFP atomic.Value

	stopCancel context.CancelFunc
	stopOnce   sync.Once
	pollDone   chan struct{}
}

// New constructs a Service rooted at skillsDir.
//
// New 以 skillsDir 为根构造 Service。
func New(
	skillsDir string,
	subagent SubagentService,
	modelPicker modeldomain.ModelPicker,
	keyProvider apikeydomain.KeyProvider,
	llmFactory *llminfra.Factory,
	notif notificationspkg.Publisher,
	log *zap.Logger,
) *Service {
	if log == nil {
		panic("skill.New: logger is nil")
	}
	if notif == nil {
		notif = notificationspkg.New(nil, log)
	}
	return &Service{
		skillsDir:   skillsDir,
		subagent:    subagent,
		modelPicker: modelPicker,
		keyProvider: keyProvider,
		llmFactory:  llmFactory,
		notif:       notif,
		log:         log,
		skills:      map[string]*skilldomain.Skill{},
	}
}

// SkillsDir returns the absolute scan path.
//
// SkillsDir 返回 Service 扫描的绝对路径。
func (s *Service) SkillsDir() string {
	return s.skillsDir
}

// SetExecRepo wires the skill_executions Repository; nil disables audit.
//
// SetExecRepo 注入 skill_executions Repository，nil 禁用审计。
func (s *Service) SetExecRepo(r skilldomain.ExecutionRepository) {
	s.mu.Lock()
	s.execRepo = r
	s.mu.Unlock()
}

// Get returns one skill's metadata; ErrSkillNotFound when absent.
//
// Get 返回单个 skill 元数据，缺失返 ErrSkillNotFound。
func (s *Service) Get(_ context.Context, name string) (*skilldomain.Skill, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sk, ok := s.skills[name]
	if !ok {
		return nil, skilldomain.ErrSkillNotFound
	}
	cp := *sk
	return &cp, nil
}

// List returns every loaded skill in stable name order.
//
// List 按 name 字典序返回所有已加载 skill。
func (s *Service) List(_ context.Context) []*skilldomain.Skill {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*skilldomain.Skill, 0, len(s.skills))
	for _, sk := range s.skills {
		cp := *sk
		out = append(out, &cp)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

