// Package workflow (app layer) orchestrates the workflow graph domain: forging graph
// versions from ops, validating + CEL-compiling each version at create/edit time, pinning a
// graph's referenced entities to their active versions, and the relation / catalog adapters.
// The version model is a linear, append-only history with a free-moving ActiveVersionID
// pointer — no pending/accept. Create/edit write a new version and take effect immediately;
// revert just moves the pointer.
//
// This module STORES + VALIDATES + PINS the graph; it does NOT execute it. The durable
// interpreter / scheduler (later wave) consumes WorkflowReader + BuildPinClosure and the
// pure domain helpers (ValidateGraph / BackEdges). Ref resolution (does fn_… exist? does its
// active version expose this method?) is delegated to an injected RefResolver port, faked in
// tests and wired to the real capability catalog at assembly (M7).
//
// Package workflow（app 层）编排 workflow 图 domain：从 ops 锻造图版本、create/edit 时校验 +
// 编译每版本的 CEL、把图引用的实体 pin 到其 active 版本、relation / catalog 适配器。版本模型线性、
// 只增 + 可自由移动的 ActiveVersionID 指针——无 pending/accept。create/edit 写新版本并立即生效；
// revert 只移指针。
//
// 本模块 STORE + VALIDATE + PIN 图；不执行它。durable 解释器 / 调度器（后续波次）消费
// WorkflowReader + BuildPinClosure 与纯 domain helper（ValidateGraph / BackEdges）。ref 解析
// （fn_… 存在吗？active 版本暴露此方法吗？）委托给注入的 RefResolver 端口，测试里 fake、装配时
// （M7）接真能力 catalog。
package workflow

import (
	"context"

	"go.uber.org/zap"

	notificationdomain "github.com/sunweilin/forgify/backend/internal/domain/notification"
	relationdomain "github.com/sunweilin/forgify/backend/internal/domain/relation"
	workflowdomain "github.com/sunweilin/forgify/backend/internal/domain/workflow"
)

// RefInfo is what a RefResolver reports about a resolved node ref: its entity kind (one of
// the relation EntityKind* vocab), whether it currently has an active version (a graph that
// references a version-less entity cannot run), and — for the two structured refs — the
// branch port names of a control logic / the method names of a handler. The pin id is the
// entity's active version id, used by BuildPinClosure. AgentCallables lists the fn_/hd_
// callables an agent mounts (for the depth-2 pin recursion).
//
// RefInfo 是 RefResolver 对某解析 node ref 的报告：实体 kind（relation EntityKind* 词表之一）、
// 当前是否有 active 版本（引用无版本实体的图不能跑）、以及——对两种结构化 ref——control 逻辑的分支
// 端口名 / handler 的方法名。pin id 是实体 active 版本 id，供 BuildPinClosure 用。AgentCallables
// 列出 agent 挂载的 fn_/hd_ 可调用项（供深度 2 的 pin 递归）。
type RefInfo struct {
	Kind             string   // relationdomain.EntityKind*
	HasActiveVersion bool     // false → graph references a version-less entity
	ActiveVersionID  string   // the pin target (entity_id → this)
	BranchPorts      []string // control only: the ctl_ active version's branch port names
	MethodNames      []string // handler only: the hd_ active version's method names
	AgentCallables   []string // agent only: the fn_/hd_ refs this agent mounts (for pin recursion)
}

// WorkflowReader is the read surface the future durable scheduler depends on (DIP: the
// scheduler imports this interface, not the concrete Service). Implemented by *Service.
//
// WorkflowReader 是未来 durable 调度器依赖的读面（DIP：调度器 import 此接口、非具体 Service）。
// 由 *Service 实现。
type WorkflowReader interface {
	// GetActiveVersion returns a workflow's active version with its graph decoded.
	// GetActiveVersion 返 workflow 的 active 版本并解码其图。
	GetActiveVersion(ctx context.Context, id string) (*workflowdomain.Version, error)

	// GetWorkflow returns the bare workflow header.
	// GetWorkflow 返裸 workflow 头。
	GetWorkflow(ctx context.Context, id string) (*workflowdomain.Workflow, error)

	// ListActive returns every live workflow with active=true (the scheduler candidate set).
	// ListActive 返所有 active=true 的活跃 workflow（调度器候选集）。
	ListActive(ctx context.Context) ([]*workflowdomain.Workflow, error)
}

// RefResolver resolves a node ref (trg_/fn_/hd_<id>.method/mcp:server/tool/ag_/ctl_/apf_) to
// its RefInfo. A miss returns workflowdomain.ErrRefNotFound. The implementation lives outside
// this module (the capability catalog, M7); it is faked in tests. The Service tolerates a nil
// resolver: CapabilityCheck then runs structural-only and says so.
//
// RefResolver 把 node ref 解析为 RefInfo。未命中返 workflowdomain.ErrRefNotFound。实现在本模块外
// （能力 catalog，M7）；测试里 fake。Service 容忍 nil resolver：CapabilityCheck 届时仅结构校验并说明。
type RefResolver interface {
	Resolve(ctx context.Context, ref string) (RefInfo, error)
}

// RelationSyncer is the slice of relationapp.Service workflow consumes (nil-tolerant).
//
// RelationSyncer 是 workflow 消费的 relationapp.Service 切片（允许 nil）。
type RelationSyncer interface {
	SyncOutgoing(ctx context.Context, fromKind, fromID string, kindScope []string, edges []relationdomain.SyncEdge) error
	SyncIncoming(ctx context.Context, toKind, toID string, kindScope []string, edges []relationdomain.SyncEdge) error
	PurgeEntity(ctx context.Context, kind, id string) error
}

// Binder is the trigger listen-registry slice the execution-lifecycle actions drive (DIP →
// *triggerapp.Service): Attach engages a continuous listener (activate), AttachOnce a one-shot
// (stage), Detach disengages (deactivate / kill). Keyed by the entry trigger entity ref (trg_).
//
// Binder 是执行生命周期动作驱动的 trigger 监听注册切片（DIP → *triggerapp.Service）：Attach 挂持续监听
// （激活）、AttachOnce 挂一次性（试运行）、Detach 摘（关掉激活 / 杀掉）。按入口 trigger 实体 ref（trg_）键。
type Binder interface {
	Attach(ctx context.Context, triggerID, workflowID string) error
	AttachOnce(ctx context.Context, triggerID, workflowID string) error
	Detach(triggerID, workflowID string)
}

// Runner is the durable-scheduler slice the execution-lifecycle actions drive (DIP →
// *schedulerapp.Service via a bootstrap adapter): StartRun fires one run now (trigger), KillWorkflow
// hard-stops every in-flight run (kill). Defined with primitive params (not scheduler.StartInput) so
// this package never imports the scheduler.
//
// Runner 是执行生命周期动作驱动的 durable 调度器切片（DIP → 经 bootstrap adapter 接 *schedulerapp.Service）：
// StartRun 立即跑一次（触发）、KillWorkflow 硬停所有在途 run（杀掉）。用原生参数（非 scheduler.StartInput）
// 定义，使本包绝不 import 调度器。
type Runner interface {
	StartRun(ctx context.Context, workflowID string, payload map[string]any) (string, error)
	KillWorkflow(ctx context.Context, workflowID string) (int, error)
	// CountRunning reports a workflow's in-flight run count — deactivate uses it to pick draining
	// (runs still finishing) vs inactive (clean stop).
	// CountRunning 报告 workflow 在途 run 数——deactivate 据此选 draining（还在收尾）vs inactive（干净停）。
	CountRunning(ctx context.Context, workflowID string) (int, error)
}

// Service orchestrates the workflow graph domain.
//
// Service 编排 workflow 图 domain。
type Service struct {
	repo      workflowdomain.Repository
	resolver  RefResolver                // nil → CapabilityCheck/pin run structural-only
	notif     notificationdomain.Emitter // nil-tolerant
	relations RelationSyncer             // nil disables relation hooks
	binder    Binder                     // nil → execution-lifecycle actions unavailable
	runner    Runner                     // nil → execution-lifecycle actions unavailable
	log       *zap.Logger
}

// NewService wires the service. repo + log are required (nil log defaults to nop). resolver
// is optional and may be installed later via SetResolver (avoids an init cycle with the
// capability catalog, which itself depends on every entity service).
//
// NewService 装配 service。repo + log 必填（nil log 默认 nop）。resolver 可选，可经 SetResolver
// 后装（避与能力 catalog 的 init 环——catalog 本身依赖每个实体 service）。
func NewService(repo workflowdomain.Repository, resolver RefResolver, notif notificationdomain.Emitter, log *zap.Logger) *Service {
	if repo == nil {
		panic("workflowapp.NewService: repo is nil")
	}
	if log == nil {
		log = zap.NewNop()
	}
	return &Service{repo: repo, resolver: resolver, notif: notif, log: log}
}

// SetResolver installs the ref resolver post-construction (avoids an init cycle).
//
// SetResolver 装配后注入 ref resolver（避 init 环）。
func (s *Service) SetResolver(r RefResolver) { s.resolver = r }

// SetRelationSyncer installs the relation Service post-construction (avoids an init cycle).
//
// SetRelationSyncer 装配后注入 relation Service（避 init 环）。
func (s *Service) SetRelationSyncer(r RelationSyncer) { s.relations = r }

// SetExecutionPorts installs the trigger binder + scheduler runner post-construction — the two
// collaborators the execution-lifecycle actions (activate/stage/deactivate/trigger/kill) need.
// Wired at assembly (M7) after both services exist; avoids the workflow ↔ scheduler/trigger DI cycle.
//
// SetExecutionPorts 装配后注入 trigger binder + scheduler runner——执行生命周期动作
// （activate/stage/deactivate/trigger/kill）所需的两个协作者。装配时（M7）两服务齐备后接；避 workflow ↔
// scheduler/trigger 的 DI 环。
func (s *Service) SetExecutionPorts(b Binder, r Runner) { s.binder, s.runner = b, r }

// publish emits a workflow lifecycle notification; nil emitter is a no-op.
//
// publish 发一条 workflow 生命周期通知；nil emitter 为 no-op。
func (s *Service) publish(ctx context.Context, action, workflowID string, extra map[string]any) {
	if s.notif == nil {
		return
	}
	payload := map[string]any{"workflowId": workflowID}
	for k, v := range extra {
		payload[k] = v
	}
	if err := s.notif.Emit(ctx, "workflow."+action, payload); err != nil {
		s.log.Warn("workflowapp.publish: emit failed", zap.String("action", action), zap.Error(err))
	}
}
