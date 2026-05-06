// Package skill is the service layer for Skill — Anthropic's Agent Skills
// abstraction. Owns skill discovery (~/.forgify/skills/* scan), the
// metadata cache, search ranking, Activate's body-load + placeholder
// substitution, and the fork-mode dispatch into SubagentService. fsnotify
// + the SSE 'skill' event live in watcher.go (D7-4).
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
// 替换、fork 模式 SubagentService 派发。fsnotify + SSE 'skill' 事件在
// watcher.go（D7-4）。
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

	"go.uber.org/zap"

	apikeydomain "github.com/sunweilin/forgify/backend/internal/domain/apikey"
	eventsdomain "github.com/sunweilin/forgify/backend/internal/domain/events"
	modeldomain "github.com/sunweilin/forgify/backend/internal/domain/model"
	skilldomain "github.com/sunweilin/forgify/backend/internal/domain/skill"
	subagentapp "github.com/sunweilin/forgify/backend/internal/app/subagent"
	llminfra "github.com/sunweilin/forgify/backend/internal/infra/llm"
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
// main.go; Scan must be called before any Get/Search/Activate (typically
// from main.go bootstrap, then again from the watcher).
//
// Service 把 disk 扫、元数据缓存、search/activate 派发、fork 模式
// subagent 集成串起来。main.go 一次构造；任何 Get/Search/Activate 前必须
// 至少调一次 Scan（典型：main.go bootstrap 调一次，watcher 后续按需调）。
type Service struct {
	skillsDir   string
	subagent    SubagentService
	bridge      eventsdomain.Bridge
	modelPicker modeldomain.ModelPicker
	keyProvider apikeydomain.KeyProvider
	llmFactory  *llminfra.Factory
	log         *zap.Logger

	mu     sync.RWMutex
	skills map[string]*skilldomain.Skill
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
	bridge eventsdomain.Bridge,
	modelPicker modeldomain.ModelPicker,
	keyProvider apikeydomain.KeyProvider,
	llmFactory *llminfra.Factory,
	log *zap.Logger,
) *Service {
	if log == nil {
		panic("skill.New: logger is nil")
	}
	return &Service{
		skillsDir:   skillsDir,
		subagent:    subagent,
		bridge:      bridge,
		modelPicker: modelPicker,
		keyProvider: keyProvider,
		llmFactory:  llmFactory,
		log:         log,
		skills:      map[string]*skilldomain.Skill{},
	}
}

// SkillsDir returns the absolute path the Service is scanning. Exposed
// for the watcher (which needs it to add fsnotify watches) and tests.
//
// SkillsDir 返 Service 在扫描的绝对路径。供 watcher（加 fsnotify watch）
// 与测试用。
func (s *Service) SkillsDir() string {
	return s.skillsDir
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

// snapshotLocked builds the SSE event payload from current skills cache.
// Caller MUST hold s.mu.RLock.
//
// snapshotLocked 从当前 skills 构造 SSE 事件载荷。调用方必须持 RLock。
func (s *Service) snapshotLocked() []*skilldomain.Skill {
	out := make([]*skilldomain.Skill, 0, len(s.skills))
	for _, sk := range s.skills {
		cp := *sk
		out = append(out, &cp)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// publishSnapshot fires the 'skill' SSE event with the current full
// snapshot. Per skill.md §10: publish whole-list snapshot (not single-
// skill delta) so the UI can replace local state in one go.
//
// publishSnapshot 发 'skill' SSE 事件携全 skill 快照。§10：发整快照（非
// 单 skill 增量）让 UI 一次性替换本地。
func (s *Service) publishSnapshot(ctx context.Context) {
	if s.bridge == nil {
		return
	}
	s.mu.RLock()
	skills := s.snapshotLocked()
	s.mu.RUnlock()
	s.bridge.Publish(ctx, "", eventsdomain.Skill{Skills: skills})
}
