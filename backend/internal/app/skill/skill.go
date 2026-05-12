// Package skill is the service layer for Skill — Anthropic's Agent Skills
// abstraction. Owns skill discovery (~/.forgify/skills/* scan), the
// metadata cache, search ranking, Activate's body-load + placeholder
// substitution, and the fork-mode dispatch into SubagentService. The 1s
// polling loop + SSE 'skill' event live in polling.go.
//
// Per skill.md design:
//   - L1 (metadata) injected eagerly at startup via Scan
//   - L2 (body) loaded on demand when LLM calls activate_skill
//   - L3 (resources) loaded by the LLM itself via Read/Bash with the
//     skill's allowed-tools pre-approved (handled by framework dispatch
//     querying agentstate.IsToolPreApprovedBySkill)
//
// Concurrency: a single RWMutex guards the skills map. Reads (Get / List /
// Search / Activate metadata lookup) take RLock; mutations (Scan rebuild)
// take Lock. Body re-reads (Activate) hit disk every call to avoid stale
// reads under user-edit (skill.md §9.5).
//
// Package skill（app/skill）是 Skill 服务层。持 skill 发现（~/.forgify/
// skills/* 扫描）、元数据缓存、search 排序、Activate 的 body 加载 + 占位
// 替换、fork 模式 SubagentService 派发。1s 轮询 + SSE 'skill' 事件在
// polling.go。
//
// 设计：L1 元数据启动时 Scan eager 注入；L2 body LLM 调 activate_skill
// 时按需加载；L3 资源 LLM 自取（Read/Bash），允许 tool 由 framework
// dispatch 查 agentstate.IsToolPreApprovedBySkill 预授权。
//
// 并发：单 RWMutex 守 skills map。读 RLock，Scan 重建 Lock。Activate 每
// 次重读 disk 防用户编辑期错版（§9.5）。
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

// SubagentService is the port skill uses to dispatch fork-mode skills
// (frontmatter.context: fork). Implemented by *subagentapp.Service.
// Interface (not direct dep on the service struct) so tests can mock
// without spinning up subagent infrastructure.
//
// SubagentService 是 skill 调 fork 模式（frontmatter.context: fork）用的
// port。由 *subagentapp.Service 实现。接口而非直接依赖让测试能 mock。
type SubagentService interface {
	Spawn(ctx context.Context, typeName, prompt string, opts subagentapp.SpawnOpts) (*subagentapp.SpawnResult, error)
}

// Service ties the disk scan, metadata cache, search/activate dispatch,
// and fork-mode subagent integration together. Constructed once in
// main.go; Start kicks off the bootstrap Scan + 1s polling goroutine.
//
// Service 把 disk 扫、元数据缓存、search/activate 派发、fork 模式
// subagent 集成串起来。main.go 一次构造；Start 触发 bootstrap Scan + 1s
// 轮询 goroutine。
type Service struct {
	skillsDir   string
	subagent    SubagentService
	modelPicker modeldomain.ModelPicker
	keyProvider apikeydomain.KeyProvider
	llmFactory  *llminfra.Factory
	notif       notificationspkg.Publisher
	log         *zap.Logger

	// execRepo persists D22 skill_executions rows. Optional — nil means
	// the audit trail is disabled (Activate still works). E11 set the
	// pattern for mcp_calls; E12 mirrors it here.
	//
	// execRepo 持 skill_executions(D22)。nil 时禁日志,Activate 照常 work。
	execRepo skilldomain.ExecutionRepository

	mu     sync.RWMutex
	skills map[string]*skilldomain.Skill

	// lastFP holds the fingerprint of the most recent Scan. Lets the
	// polling loop short-circuit notification publishes when nothing
	// user-visible changed (~99% of ticks). string via atomic.Value.
	//
	// lastFP 存最近一次 Scan 的指纹；轮询时让 notification publish 在用户
	// 可见内容未变时（~99% tick）短路。string 经 atomic.Value 存。
	lastFP atomic.Value

	// stopCancel + stopOnce + pollDone manage the polling goroutine
	// lifecycle (set by Start, drained by Stop).
	//
	// stopCancel + stopOnce + pollDone 管轮询 goroutine 生命周期
	// （Start 设置，Stop 排空）。
	stopCancel context.CancelFunc
	stopOnce   sync.Once
	pollDone   chan struct{}
}

// New constructs a Service. skillsDir is typically ~/.forgify/skills/;
// tests pass a t.TempDir(). subagent may be nil if no fork-mode skills
// are expected (Activate of a fork skill panics with a clear error then;
// production main.go always wires it).
//
// New 构造 Service。skillsDir 典型 ~/.forgify/skills/；测试传 t.TempDir()。
// subagent 若 nil 则 fork 模式 Activate 会清晰报错（生产 main.go 永远接）。
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

// SkillsDir returns the absolute path the Service is scanning. Exposed
// for tests + handlers that need to know the on-disk root.
//
// SkillsDir 返 Service 在扫描的绝对路径。供测试与需知磁盘根的 handler 用。
func (s *Service) SkillsDir() string {
	return s.skillsDir
}

// SetExecRepo wires the D22 skill_executions Repository. Optional —
// nil disables the audit trail. E15 main.go calls this after building
// the GORM-backed Store.
//
// SetExecRepo 接 D22 skill_executions Repository。nil 禁日志;E15 装。
func (s *Service) SetExecRepo(r skilldomain.ExecutionRepository) {
	s.mu.Lock()
	s.execRepo = r
	s.mu.Unlock()
}

// ── Read APIs ────────────────────────────────────────────────────────

// Get returns one skill's metadata; ErrSkillNotFound when absent.
//
// Get 返单个 skill 元数据；不存在返 ErrSkillNotFound。
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
// List 按 name 字母序返每个已加载 skill。
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

