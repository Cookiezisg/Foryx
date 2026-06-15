// Package agent (app layer) orchestrates the agent domain: forging config versions, running
// the ReAct loop (invoke), the execution-log surface, and the relation / catalog adapters.
//
// The version model is linear append-only with a free-moving ActiveVersionID pointer — no
// pending/accept. Create/edit write a new version (max+1) and take effect immediately; revert
// just moves the pointer. An agent writes no code, so there is NO sandbox dependency; instead
// invoke needs four injected ports (DIP): an LLM resolver, a mount resolver (synthesizes the
// version's fn_/hd_/mcp refs into bound tools), a skill-guide renderer, and a knowledge
// renderer — none of which the agent owns.
//
// Package agent（app 层）编排 agent domain：锻造配置版本、跑 ReAct loop（invoke）、execution-log
// 面、relation / catalog 适配器。版本模型线性只增 + 可移动 ActiveVersionID 指针——无 pending/accept。
// create/edit 写新版本（max+1）立即生效；revert 只移指针。agent 不写代码，故**无 sandbox 依赖**；
// invoke 需四个注入端口（DIP）：LLM resolver、mount resolver（把版本的 fn_/hd_/mcp ref 合成绑定
// 工具）、skill 指南渲染器、knowledge 渲染器——agent 自己都不拥有。
package agent

import (
	"context"

	"go.uber.org/zap"

	loopapp "github.com/sunweilin/forgify/backend/internal/app/loop"
	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
	agentdomain "github.com/sunweilin/forgify/backend/internal/domain/agent"
	modeldomain "github.com/sunweilin/forgify/backend/internal/domain/model"
	notificationdomain "github.com/sunweilin/forgify/backend/internal/domain/notification"
	relationdomain "github.com/sunweilin/forgify/backend/internal/domain/relation"
	searchdomain "github.com/sunweilin/forgify/backend/internal/domain/search"
	streamdomain "github.com/sunweilin/forgify/backend/internal/domain/stream"
	llminfra "github.com/sunweilin/forgify/backend/internal/infra/llm"
)

// LLMBundle is a ready-to-run LLM client + a pre-filled base Request (ModelID/Key/BaseURL/
// Thinking/Options). InvokeAgent fills System + lets loop.Run compose Messages from the host.
//
// LLMBundle 是即用的 LLM client + 预填的 base Request（ModelID/Key/BaseURL/Thinking/Options）。
// InvokeAgent 填 System，Messages 由 loop.Run 从 host 组装。
type LLMBundle struct {
	Client  llminfra.Client
	Request llminfra.Request
}

// LLMResolver turns a (nil = default agent scenario) model override into a runnable bundle.
// Implemented at boot over model-picker + apikey + llm-factory — the agent owns none of that.
//
// LLMResolver 把（nil = 默认 agent 场景）model 覆盖解析为可运行 bundle。boot 时基于 model-picker +
// apikey + llm-factory 实现——agent 自己都不拥有。
type LLMResolver interface {
	ResolveAgent(ctx context.Context, override *modeldomain.ModelRef) (LLMBundle, error)
}

// KnowledgeProvider renders the agent's attached document IDs into a prompt-prefix string.
//
// KnowledgeProvider 把 agent 挂的文档 ID 渲染成 prompt 前缀字符串。
type KnowledgeProvider interface {
	BuildKnowledgePrefix(ctx context.Context, docIDs []string) (string, error)
}

// MountResolver synthesizes the version's mounted ToolRefs (fn_/hd_…method/mcp:server/tool)
// into bound, callable tools — one per mount, named after the target, executing through the
// target's standard execution method. The agent NEVER sees the generic system-tool registry
// (no run_function / Read / Bash); its tool universe is exactly its mounts.
//
// MountResolver 把版本挂载的 ToolRef（fn_/hd_…method/mcp:server/tool）合成绑定可调工具——每挂载
// 一个、以目标命名、经目标标准执行方法执行。agent **永不**见通用系统工具表（无 run_function /
// Read / Bash）；其工具宇宙恰是其挂载。
type MountResolver interface {
	Resolve(ctx context.Context, refs []agentdomain.ToolRef) ([]toolapp.Tool, error)
	// CheckHealth resolves each mount independently (no fail-fast) for the on-demand mount-health
	// precheck — same resolution path as Resolve, but per-mount status instead of all-or-nothing.
	//
	// CheckHealth 独立解析每个挂载（不 fail-fast），给按需挂载健康预检——与 Resolve 同解析路径，但逐
	// 挂载状态而非全或无。
	CheckHealth(ctx context.Context, refs []agentdomain.ToolRef) []agentdomain.MountHealth
}

// SkillGuide renders the version's mounted skill into an execution-guide string injected into
// the system prompt (no active-skill state, no fork — see skillapp.Guide).
//
// SkillGuide 把版本挂载的 skill 渲染成注入 system prompt 的执行指南串（无 active-skill 状态、
// 不 fork——见 skillapp.Guide）。
type SkillGuide interface {
	Guide(ctx context.Context, name string) (string, error)
}

// InvokeDeps are the LLM-side dependencies InvokeAgent needs, injected post-construction (DIP).
// Each dep is consulted only when the version mounts the matching capability — but a needed-yet-
// nil dep is a wiring bug and fails the invoke loudly (a worker must not run silently degraded).
//
// InvokeDeps 是 InvokeAgent 需要的 LLM 侧依赖，构造后注入（DIP）。每个依赖仅在版本挂载对应能力时
// 被用到——但「需要却为 nil」是装配 bug、让 invoke 大声失败（worker 绝不静默降级运行）。
type InvokeDeps struct {
	Resolver  LLMResolver
	Mounts    MountResolver
	Skill     SkillGuide
	Knowledge KnowledgeProvider

	// EntitiesBridge (SSE-C, nil-tolerant): the agent run mirrors its ReAct trace (every block)
	// onto the entities stream scoped to the agent, so the agent panel shows the run live —
	// regardless of caller (chat / REST / workflow node). A stream, not the messages-table coupling
	// B5 deliberately avoided.
	//
	// EntitiesBridge（SSE-C，允许 nil）：agent run 把 ReAct 轨迹（每个 block）镜像到 agent scope 的 entities
	// 流，使 agent 面板实时显示运行——与谁触发无关（chat / REST / workflow 节点）。是流、非 B5 刻意回避的
	// messages 表耦合。
	EntitiesBridge streamdomain.Bridge
}

// RelationSyncer is the slice of relationapp.Service the agent consumes (nil-tolerant). Agents
// have both outgoing edges (the mounted skill/doc/fn/hd/mcp) and incoming edges (the
// conversation that forged/edited a version).
//
// RelationSyncer 是 agent 消费的 relationapp.Service 切片（允许 nil）。agent 有出边（挂载的
// skill/doc/fn/hd/mcp）也有入边（锻造/编辑某版本的对话）。
type RelationSyncer interface {
	SyncOutgoing(ctx context.Context, fromKind, fromID string, kindScope []string, edges []relationdomain.SyncEdge) error
	SyncIncoming(ctx context.Context, toKind, toID string, kindScope []string, edges []relationdomain.SyncEdge) error
	PurgeEntity(ctx context.Context, kind, id string) error
}

// Service orchestrates the agent domain.
//
// Service 编排 agent domain。
type Service struct {
	repo      agentdomain.Repository
	search    searchdomain.Notifier // nil → search indexing disabled. nil → 不接搜索索引。
	invoke    InvokeDeps
	relations RelationSyncer             // nil disables relation hooks
	notif     notificationdomain.Emitter // nil-tolerant
	log       *zap.Logger
}

// NewService wires the service; nil repo / log is a wiring bug. invoke deps + relations are
// injected later (SetInvokeDeps / SetRelationSyncer) to avoid init cycles.
//
// NewService 装配 service；nil repo / log 是装配 bug。invoke deps + relations 后注入（避 init 环）。
func NewService(repo agentdomain.Repository, notif notificationdomain.Emitter, log *zap.Logger) *Service {
	if repo == nil {
		panic("agentapp.NewService: repo is nil")
	}
	if log == nil {
		log = zap.NewNop()
	}
	return &Service{repo: repo, notif: notif, log: log}
}

// SetRelationSyncer installs the relation Service post-construction (avoids an init cycle).
//
// SetRelationSyncer 装配后注入 relation Service（避 init 环）。
func (s *Service) SetRelationSyncer(r RelationSyncer) { s.relations = r }

// SetInvokeDeps installs the LLM-side invoke dependencies (resolver / tools / knowledge).
// Until called, InvokeAgent returns an error — CRUD works without them.
//
// SetInvokeDeps 注入 LLM 侧 invoke 依赖。未注入前 InvokeAgent 报错——CRUD 不依赖它们。
func (s *Service) SetInvokeDeps(deps InvokeDeps) { s.invoke = deps }

// publish emits an agent lifecycle notification; nil emitter is a no-op.
//
// publish 发一条 agent 生命周期通知；nil emitter 为 no-op。
func (s *Service) publish(ctx context.Context, action, agentID string, extra map[string]any) {
	s.notifySearch(ctx, agentID)
	if s.notif == nil {
		return
	}
	payload := map[string]any{"agentId": agentID}
	for k, v := range extra {
		payload[k] = v
	}
	if err := s.notif.Emit(ctx, "agent."+action, payload); err != nil {
		s.log.Warn("agentapp.publish: emit failed", zap.String("action", action), zap.Error(err))
	}
}

// loopHostType pins the loop.Host interface so a compile error fires if agentHost drifts.
var _ loopapp.Host = (*agentHost)(nil)
