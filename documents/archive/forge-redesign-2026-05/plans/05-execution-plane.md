# Execution Plane Implementation Plan — Scheduler + Trigger + FlowRun

> ✅ **COMPLETED 2026-05-13** — 17 commits E1-E17 直推 main(a273de1 / 72a5142 / 9821b45 / 97d4ee3 / 82be1f9 / 553440f / 5d9f80e / f03ef57 / 9da6b5a / dd5b7c9 / e175e9a / 6603f77 / 0afc548 / 174a9d6 / 6d73362 + E16/E17)。
> 见 [`../../service-design-documents/{flowrun,trigger,scheduler}.md`](../../service-design-documents/) 完整成品 spec;[`progress-record.md`](../../../progress-record.md) 2026-05-13 dev log。
>
> 14 项 hardening item 全覆盖 — 单测(cron missed-policy / fsnotify fail-soft / webhook secret / node timeout / panic recover / paused rehydrate / retention 200/wf / cron TZ)+ 7 pipeline E2E(workflow disabled gate / serial concurrency / cancellation cleanup / trigger states observable / happy :trigger / flowrun GET + BootSmoke)。loop body subgraph + parallel branches subgraph V1 留 Plan 06(显式 sentinel)。

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 实现 Workflow 执行那一面 — 3 个 sibling domain(scheduler / trigger / flowrun)+ **14 项生产级 V1 必做项**。**前置依赖**:Plan 01-04 已 merge。

**Architecture:** 三 domain 单向依赖链 trigger → scheduler → flowrun(只写)。Scheduler 是中间编排层(读 Workflow active version + 起 FlowRun + dispatch 13 种节点 + 处理 retry / onError / approval / cancellation)。Trigger 注册 4 种监听器(cron / fsnotify / webhook / manual)。FlowRun 持久化执行记录(`flowruns` + `flowrun_nodes` 2 表)。所有事件经 eventlog `flowrun:fr_xxx` scope 流式;chat-triggered run 由 trigger_workflow 工具内部 forward 到 conversation scope。

**Tech Stack:** Go 1.25,GORM,modernc.org/sqlite,`github.com/robfig/cron/v3`(cron 调度,首次引入)、`fsnotify`(已有 indirect dep,提为 direct)、`net/http`(webhook 注册)。

**关联**:[`05-execution-plane.md`](../05-execution-plane.md) 完整 spec / [`04-workflow.md`](../04-workflow.md)(scheduler 接 workflow active version)/ Plan 04 的 `SchedulerForwarder` interface 待实现。

---

## Phase 0:Branch + Prereqs

### Task 1:Branch + 添加新依赖

- [ ] **Step 1: 验证 prereq**

```bash
cd /Users/SP14921/Documents/Personal/PersonalCodeBase/Forgify
git checkout main && git pull origin main
ls backend/internal/domain/workflow/  # Plan 04 应已落
grep "SchedulerForwarder" backend/internal/app/workflow/  # 占位 interface 应在
```

- [ ] **Step 2: 创建分支**

```bash
git checkout -b feature/execution-plane
```

- [ ] **Step 3: 加新依赖**

```bash
cd backend
go get github.com/robfig/cron/v3
# fsnotify 已 indirect,提为 direct
go get github.com/fsnotify/fsnotify
go mod tidy
```

- [ ] **Step 4: Commit go.mod 改动**

```bash
git add backend/go.mod backend/go.sum
git commit -m "feat(execution): add robfig/cron + fsnotify dependencies"
git push origin feature/execution-plane
```

---

## Phase 1:FlowRun Domain

### Task 2:FlowRun + FlowRunNode entities + sentinels

**Files:** Create `backend/internal/domain/flowrun/flowrun.go`

```go
package flowrun

import (
	"errors"
	"time"

	"gorm.io/gorm"
)

const (
	// FlowRun (workflow run 整体) status — 5 值
	// V1 没有 run-level 总超时,**不含 timeout** 状态。节点 timeout 致 run 终止时,
	// run.status = failed,error_code 标 NODE_TIMEOUT。
	StatusRunning   = "running"
	StatusPaused    = "paused" // approval / wait 节点
	StatusCompleted = "completed"
	StatusFailed    = "failed"
	StatusCancelled = "cancelled"

	// FlowRunNode (单节点执行) status — 5 值
	// 每节点 execution 终态 — 含 timeout(节点级超时是真状态,对应 spec 08-executions
	// shared schema 模板的 status enum)。
	NodeStatusPending   = "pending"
	NodeStatusRunning   = "running"
	NodeStatusCompleted = "completed"
	NodeStatusFailed    = "failed"
	NodeStatusSkipped   = "skipped"
	NodeStatusTimeout   = "timeout"
)

type FlowRun struct {
	ID            string `gorm:"primaryKey"`
	WorkflowID    string `gorm:"index;not null"`
	VersionID     string `gorm:"not null"` // 锁哪个 active version
	TriggerKind   string // cron | fsnotify | webhook | manual
	TriggerInput  map[string]any `gorm:"serializer:json"`
	Status        string `gorm:"not null;check:status IN ('running','paused','completed','failed','cancelled')"`
	StartedAt     time.Time
	EndedAt       *time.Time
	ElapsedMs     int
	Output        map[string]any `gorm:"serializer:json"`
	ErrorCode     string
	ErrorMessage  string
	PausedState   *PausedState `gorm:"serializer:json"` // approval / wait 时持久化
	CreatedAt     time.Time
	DeletedAt     gorm.DeletedAt `gorm:"index"`
}

// PausedState 是 approval/wait 暂停时持久化的 ExecutionContext 快照(rehydrate 用,V1 §6.1)。
//
// PausedState is the persisted ExecutionContext snapshot for approval/wait
// (used to rehydrate at process restart per V1 production-hardening §6.1).
type PausedState struct {
	NodeID    string                            `json:"nodeId"`
	Variables map[string]any                    `json:"variables"`
	Outputs   map[string]map[string]any         `json:"outputs"` // nodeID → output
	Position  []string                          `json:"position"` // 当前 ready node IDs
	PausedAt  time.Time                         `json:"pausedAt"`
}

// FlowRunNode 是 spec 08-executions §4.5 定义的 5 张 execution log 表之一。
// 字段补齐到共享 schema 模板(详 08 §2)— 含 status / triggered_by / 
// conv 上下文 / error_code / elapsed_ms / 等通用字段 + flowrun-specific
// 字段 attempts / node_id / node_type。
//
// FlowRunNode is one of the 5 execution log tables defined in
// spec 08-executions §4.5. Fields aligned to the shared schema template.
type FlowRunNode struct {
	ID             string `gorm:"primaryKey"`  // frn_<16hex>
	UserID         string `gorm:"index"`
	FlowRunID      string `gorm:"index;not null"`
	NodeID         string // graph 内的 node id(如 "filter_cond")
	NodeType       string // function / handler / mcp / skill / llm / condition / loop / parallel / approval / wait / variable
	Status         string `gorm:"check:status IN ('ok','failed','cancelled','timeout')"`
	TriggeredBy    string `gorm:"check:triggered_by IN ('chat','workflow','http','test')"` // workflow 节点几乎总是 'workflow'
	Input          string // JSON
	Output         string // JSON (NULL on non-ok)
	ErrorCode      string
	ErrorMessage   string
	ElapsedMs      int
	StartedAt      time.Time
	EndedAt        time.Time
	ConversationID string `gorm:"index"`
	MessageID      string
	ToolCallID     string
	Attempts       int `gorm:"default:1"` // retry 次数(per-node retry policy)
	CreatedAt      time.Time
}

var (
	ErrNotFound                = errors.New("flowrun: not found")
	ErrNotCancellable          = errors.New("flowrun: not cancellable")
	ErrNotPaused               = errors.New("flowrun: not paused")
	ErrApprovalNodeNotFound    = errors.New("flowrun: approval node not found in paused state")
	ErrApprovalDecisionInvalid = errors.New("flowrun: approval decision invalid")
)
```

- [ ] **Step 1-3:**写 + 编译 + commit

---

### Task 3:Repository 接口 + Filter

参考 Plan 01 Task 4。新增 method:

- `Create(ctx, *FlowRun) error`
- `UpdateStatus(ctx, runID, status string, output map[string]any, errCode, errMsg string) error`
- `SetPausedState(ctx, runID string, ps *PausedState) error`
- `ClearPausedState(ctx, runID string) error`
- `CreateNode(ctx, *FlowRunNode) error`
- `UpdateNode(ctx, ...) error`
- `ListByWorkflow(ctx, workflowID, filter) ([]*FlowRun, *Cursor, error)`
- `ListPausedRuns(ctx) ([]*FlowRun, error)` — boot 时 rehydrate 用
- `PruneOlderThan(ctx, workflowID string, keep int) error` — V1 默认 200 (§6.7)

- [ ] Step 1-3

---

### Task 4:Domain 单测

- [ ] Step 1-3

---

## Phase 2:FlowRun Store

### Task 5:GORM Repository + AutoMigrate hook

参考 Plan 01 Task 6。索引:`(workflow_id, status, started_at)` 复合;`flowrun_nodes` 索引 `(flowrun_id, started_at)`。

- [ ] Step 1-3

### Task 6:Store 集成测试

- [ ] Step 1-3

---

## Phase 3:Trigger Domain

### Task 7:Trigger 类型 + sentinels

**Files:** Create `backend/internal/domain/trigger/trigger.go`

```go
package trigger

import "errors"

const (
	KindCron     = "cron"
	KindFsnotify = "fsnotify"
	KindWebhook  = "webhook"
	KindManual   = "manual"
)

// Spec 是 trigger 节点 config 在 trigger domain 的 normalized 表示。
//
// Spec is trigger node config normalized within trigger domain.
type Spec struct {
	WorkflowID string         `json:"workflowId"`
	NodeID     string         `json:"nodeId"`
	Kind       string         `json:"kind"`
	Config     map(string)any `json:"config"` // kind-specific
}

// State 是 listener 运行时状态(in-memory)。
//
// State is listener runtime state (in-memory).
type State struct {
	WorkflowID  string
	NodeID      string
	Kind        string
	Status      string // active | idle | error
	LastFiredAt *time.Time
	NextFireAt  *time.Time // cron only
	LastError   string
}

var (
	ErrPathNotExist           = errors.New("trigger: fsnotify path not exist")
	ErrPathConflict           = errors.New("trigger: webhook path conflict")
	ErrWebhookSecretMismatch  = errors.New("trigger: webhook secret mismatch")
	ErrInvalidCronExpression  = errors.New("trigger: invalid cron expression")
)
```

- [ ] Step 1-3

---

## Phase 4:Trigger Infra(4 实现)

### Task 8:Cron impl(`infra/trigger/cron/`)

**Files:** Create `backend/internal/infra/trigger/cron/cron.go` + tests

```go
package cron

import (
	"context"
	"sync"
	"time"

	robfigcron "github.com/robfig/cron/v3"
	"go.uber.org/zap"

	triggerdomain "github.com/sunweilin/forgify/backend/internal/domain/trigger"
)

type Listener struct {
	mu       sync.Mutex
	cron     *robfigcron.Cron
	entries  map[string]robfigcron.EntryID // (workflowID + nodeID) → cron entry
	lastFire map[string]time.Time          // last fired time(per spec)
	onFire   func(workflowID, nodeID string, input map[string]any)
	log      *zap.Logger
}

func New(log *zap.Logger, onFire func(string, string, map[string]any)) *Listener {
	return &Listener{
		cron:     robfigcron.New(robfigcron.WithLocation(time.Local)), // §6.10 锁本地时区
		entries:  make(map[string]robfigcron.EntryID),
		lastFire: make(map[string]time.Time),
		onFire:   onFire,
		log:      log.Named("trigger.cron"),
	}
}

// Register adds a cron entry. spec.Config["expression"] is the cron string.
//
// Register 加一个 cron entry。
func (l *Listener) Register(spec triggerdomain.Spec) error {
	expr, _ := spec.Config["expression"].(string)
	if expr == "" {
		return triggerdomain.ErrInvalidCronExpression
	}
	key := spec.WorkflowID + "/" + spec.NodeID

	l.mu.Lock()
	defer l.mu.Unlock()

	// missedPolicy 默认 runOnce — boot 时算 missed(§6.2)
	if last, ok := l.lastFire[key]; ok {
		schedule, err := robfigcron.ParseStandard(expr)
		if err != nil { return triggerdomain.ErrInvalidCronExpression }
		next := schedule.Next(last)
		if time.Now().After(next) {
			// 漏了 — 立刻补 1 次(runOnce 默认)
			go l.onFire(spec.WorkflowID, spec.NodeID, map[string]any{
				"missedSince": last,
				"firedAt":     time.Now(),
			})
		}
	}

	id, err := l.cron.AddFunc(expr, func() {
		now := time.Now()
		l.mu.Lock()
		l.lastFire[key] = now
		l.mu.Unlock()
		l.onFire(spec.WorkflowID, spec.NodeID, map[string]any{
			"firedAt": now,
		})
	})
	if err != nil { return triggerdomain.ErrInvalidCronExpression }
	l.entries[key] = id
	return nil
}

// Unregister 删一个 cron entry。
func (l *Listener) Unregister(workflowID, nodeID string) {
	key := workflowID + "/" + nodeID
	l.mu.Lock()
	defer l.mu.Unlock()
	if id, ok := l.entries[key]; ok {
		l.cron.Remove(id)
		delete(l.entries, key)
	}
}

// Start 启动 cron scheduler。
func (l *Listener) Start() { l.cron.Start() }

// Stop 停 cron scheduler。
func (l *Listener) Stop() { l.cron.Stop() }

// State 返指定 trigger 当前状态。
func (l *Listener) State(workflowID, nodeID string) triggerdomain.State {
	key := workflowID + "/" + nodeID
	l.mu.Lock()
	defer l.mu.Unlock()
	state := triggerdomain.State{
		WorkflowID: workflowID,
		NodeID:     nodeID,
		Kind:       triggerdomain.KindCron,
		Status:     "idle",
	}
	if last, ok := l.lastFire[key]; ok {
		state.LastFiredAt = &last
	}
	if id, ok := l.entries[key]; ok {
		entry := l.cron.Entry(id)
		state.NextFireAt = &entry.Next
		state.Status = "active"
	}
	return state
}
```

- [ ] **Step 1-3:**写 + 单测(missedPolicy / cron parse error / register-unregister)+ commit

---

### Task 9:Fsnotify impl(`infra/trigger/fsnotify/`)

**Files:** Create `backend/internal/infra/trigger/fsnotify/fsnotify.go` + tests

```go
// per spec §6.11 — 路径不存在 fail-soft(state=error,通过 notification 告知用户)。
//
// per spec §6.11 — path-not-exist fail-soft (state=error, notify user).
```

实现要点:
- 用 `fsnotify v1.9` watcher
- 启动时校验 path 存在(不存在 → state=error,不阻塞);path exist 后再 register
- 收到 event → match pattern(若有)+ event kind 过滤(create/modify/delete)→ onFire

- [ ] Step 1-3

---

### Task 10:Webhook impl(`infra/trigger/webhook/`)

**Files:** Create `backend/internal/infra/trigger/webhook/webhook.go` + tests

```go
// per spec §6.6 — webhook secret 防滥用。
// 注册到 httpapi router 的 /api/v1/webhooks/{wfId}/{path} 子路径。
//
// per spec §6.6 — webhook secret防滥用。Registers under
// /api/v1/webhooks/{wfId}/{path} subpath of httpapi router.
```

实现要点:
- 接 `*http.ServeMux`(从 transport router 注入),动态 register handler at path
- POST 接 body(默认 10MB cap),反序列化 → onFire
- secret 校验:`X-Webhook-Secret` header 或 `?token=` 参对比
- path 冲突注册 → 返 ErrPathConflict

- [ ] Step 1-3

---

### Task 11:Manual impl + Trigger app Service

**Files:**
- Create: `backend/internal/app/trigger/service.go`

manual 实现极简(无 listener,直接由 HTTP / LLM tool 调 scheduler.StartRun)。Service 整合 4 种:

```go
package trigger

import (
	"context"
	"sync"

	"go.uber.org/zap"

	triggerdomain "github.com/sunweilin/forgify/backend/internal/domain/trigger"
	cronlistener "github.com/sunweilin/forgify/backend/internal/infra/trigger/cron"
	fsnotifylistener "github.com/sunweilin/forgify/backend/internal/infra/trigger/fsnotify"
	webhooklistener "github.com/sunweilin/forgify/backend/internal/infra/trigger/webhook"
)

type Service struct {
	mu       sync.RWMutex
	cron     *cronlistener.Listener
	fsnotify *fsnotifylistener.Listener
	webhook  *webhooklistener.Listener
	specs    map[string]map[string]triggerdomain.Spec // workflowID → nodeID → spec
	scheduler SchedulerStarter
	log      *zap.Logger
}

type SchedulerStarter interface {
	StartRun(ctx context.Context, workflowID string, input map[string]any) (runID string, err error)
}

func New(scheduler SchedulerStarter, mux *http.ServeMux, log *zap.Logger) *Service {
	s := &Service{
		specs:     make(map[string]map[string]triggerdomain.Spec),
		scheduler: scheduler,
		log:       log.Named("trigger"),
	}

	onFire := func(workflowID, nodeID string, input map[string]any) {
		ctx := reqctxpkg.SetUserID(context.Background(), reqctxpkg.DefaultLocalUserID)
		runID, err := scheduler.StartRun(ctx, workflowID, input)
		if err != nil {
			s.log.Error("trigger fired but StartRun failed", zap.Error(err))
			return
		}
		s.log.Info("trigger fired", zap.String("workflowID", workflowID), zap.String("runID", runID))
	}

	s.cron = cronlistener.New(log, onFire)
	s.fsnotify = fsnotifylistener.New(log, onFire)
	s.webhook = webhooklistener.New(mux, log, onFire)

	s.cron.Start()
	return s
}

// RegisterTrigger 注册 trigger spec。Boot 时扫所有 active workflow,Active version
// 翻新时也调。
//
// RegisterTrigger registers a trigger spec. Called at boot and on
// active-version-change.
func (s *Service) RegisterTrigger(spec triggerdomain.Spec) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch spec.Kind {
	case triggerdomain.KindCron:
		if err := s.cron.Register(spec); err != nil { return err }
	case triggerdomain.KindFsnotify:
		if err := s.fsnotify.Register(spec); err != nil { return err }
	case triggerdomain.KindWebhook:
		if err := s.webhook.Register(spec); err != nil { return err }
	case triggerdomain.KindManual:
		// no listener
	}
	if s.specs[spec.WorkflowID] == nil {
		s.specs[spec.WorkflowID] = make(map[string]triggerdomain.Spec)
	}
	s.specs[spec.WorkflowID][spec.NodeID] = spec
	return nil
}

// UnregisterByWorkflow 撤所有 workflow 关联的 trigger(disable / delete / version 翻时)。
//
// UnregisterByWorkflow removes all triggers for a workflow.
func (s *Service) UnregisterByWorkflow(workflowID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for nodeID, spec := range s.specs[workflowID] {
		switch spec.Kind {
		case triggerdomain.KindCron:
			s.cron.Unregister(workflowID, nodeID)
		case triggerdomain.KindFsnotify:
			s.fsnotify.Unregister(workflowID, nodeID)
		case triggerdomain.KindWebhook:
			s.webhook.Unregister(workflowID, nodeID)
		}
	}
	delete(s.specs, workflowID)
}

// State 返某 workflow 所有 trigger 状态。
func (s *Service) State(workflowID string) []triggerdomain.State { ... }
```

- [ ] Step 1-3

---

## Phase 5:Scheduler

### Task 12:Scheduler app Service + StartRun + cancellation

**Files:** Create `backend/internal/app/scheduler/scheduler.go`

```go
package scheduler

import (
	"context"
	"sync"
	"time"

	"go.uber.org/zap"

	flowrunapp "github.com/sunweilin/forgify/backend/internal/app/flowrun"
	workflowapp "github.com/sunweilin/forgify/backend/internal/app/workflow"
	flowrundomain "github.com/sunweilin/forgify/backend/internal/domain/flowrun"
	workflowdomain "github.com/sunweilin/forgify/backend/internal/domain/workflow"
)

type Service struct {
	flowrunSvc   *flowrunapp.Service
	workflowSvc  WorkflowReader
	dispatcher   *Dispatcher
	notif        *notificationspkg.Publisher

	cancelsMu sync.RWMutex
	cancels   map[string]context.CancelFunc // runID → cancel func

	log *zap.Logger
}

type WorkflowReader interface {
	GetActiveVersion(ctx context.Context, workflowID string) (*workflowdomain.Version, error)
	GetWorkflow(ctx context.Context, workflowID string) (*workflowdomain.Workflow, error)
}

func New(...) *Service { ... }

// StartRun spawns a new FlowRun for a workflow trigger.
//
// StartRun 起一个 workflow 的新 FlowRun。
func (s *Service) StartRun(ctx context.Context, workflowID string, triggerInput map[string]any) (string, error) {
	wf, err := s.workflowSvc.GetWorkflow(ctx, workflowID)
	if err != nil { return "", err }

	// V1 §6.5 disabled check
	if !wf.Enabled {
		return "", workflowdomain.ErrDisabled
	}
	// V1 §6.x needs_attention check
	if wf.NeedsAttention {
		return "", workflowdomain.ErrNeedsAttention
	}

	// V1 §6.3 concurrency check
	if wf.Concurrency == "serial" {
		running, err := s.flowrunSvc.CountRunning(ctx, workflowID)
		if err != nil { return "", err }
		if running >= 1 {
			return "", schedulerdomain.ErrConcurrencyLimit // skip
		}
	}

	version, err := s.workflowSvc.GetActiveVersion(ctx, workflowID)
	if err != nil { return "", err }

	run := &flowrundomain.FlowRun{
		ID:           idgenpkg.New("fr"),
		WorkflowID:   workflowID,
		VersionID:    version.ID,
		TriggerInput: triggerInput,
		Status:       flowrundomain.StatusRunning,
		StartedAt:    time.Now(),
	}
	if err := s.flowrunSvc.Create(ctx, run); err != nil { return "", err }

	// 注册 cancel func
	runCtx, cancel := context.WithCancel(reqctxpkg.SetUserID(context.Background(), reqctxpkg.DefaultLocalUserID))
	s.cancelsMu.Lock()
	s.cancels[run.ID] = cancel
	s.cancelsMu.Unlock()

	// 异步执行
	go s.executeRun(runCtx, run, &version.Graph)

	return run.ID, nil
}

// Cancel 取消运行中 / paused FlowRun。
//
// Cancel cancels a running or paused FlowRun.
func (s *Service) Cancel(ctx context.Context, runID string) error {
	s.cancelsMu.RLock()
	cancel, ok := s.cancels[runID]
	s.cancelsMu.RUnlock()
	if !ok {
		return flowrundomain.ErrNotCancellable
	}
	cancel() // ctx.Done() 串到 dispatcher 的所有节点
	return nil
}
```

- [ ] Step 1-3

---

### Task 13:executeRun + state machine

**Files:** Modify `backend/internal/app/scheduler/scheduler.go` + create `state.go`

按 spec 05-execution §3.1 的伪代码实现:

- topo sort + ready set
- per-node dispatch with retry / onError / timeout
- handle approval / wait → persist PausedState + 释放 goroutine
- 终态 → cleanup(Handler instances cascade destroy)+ finalize FlowRun + publish notification

- [ ] Step 1-3

---

### Task 14:Dispatcher — 13 nodeDispatcher

**Files:**
- Create: `backend/internal/app/scheduler/dispatch.go`
- Create: `backend/internal/app/scheduler/dispatchers/{trigger,function,handler,mcp,skill,llm,http,condition,loop,parallel,approval,wait,variable}.go`

每节点一个 dispatcher,~50-150 行各自:

```go
type Dispatcher struct {
	functionSvc *functionapp.Service
	handlerSvc  *handlerapp.Service
	mcpSvc      *mcpapp.Service
	skillSvc    *skillapp.Service
	llmFactory  *llminfra.Factory
	httpClient  *http.Client
	flowrunSvc  *flowrunapp.Service // for variable get/set
}

type DispatchInput struct {
	Node    workflowdomain.NodeSpec
	NodeIn  map[string]any // input port data
	ExecCtx *ExecutionContext
}

type DispatchOutput struct {
	Outputs   map[string]any // by port name
	Error     error
}

// Dispatch 是顶层入口,按 node.Type 派给对应 dispatcher。
func (d *Dispatcher) Dispatch(ctx context.Context, in DispatchInput) DispatchOutput {
	switch in.Node.Type {
	case workflowdomain.NodeTypeTrigger:
		return d.dispatchTrigger(ctx, in)
	case workflowdomain.NodeTypeFunction:
		return d.dispatchFunction(ctx, in)
	case workflowdomain.NodeTypeHandler:
		return d.dispatchHandler(ctx, in)
	// ... 13 case
	}
}
```

- [ ] **Task 14a:** trigger / function / handler dispatchers
- [ ] **Task 14b:** mcp / skill / llm / http dispatchers
- [ ] **Task 14c:** condition / loop / parallel dispatchers(loop / parallel 调用 sub-dispatcher 处理 body)
- [ ] **Task 14d:** approval / wait / variable dispatchers

每个 dispatcher 完成一份单测(mock 各 service)。

---

### Task 15:Retry / OnError / Timeout(per-node config)

**Files:** Create `backend/internal/app/scheduler/retry.go` + `error_policy.go`

per spec §6.8 / §3.1:

```go
// withRetry wraps a dispatch call with the node's retry policy.
//
// withRetry 用节点 retry 策略包一层 dispatch 调用。
func withRetry(ctx context.Context, retry *workflowdomain.RetryPolicy, fn func() (any, error)) (any, error) {
	if retry == nil {
		return fn()
	}
	var lastErr error
	delay := parseDuration(retry.Delay, time.Second)
	for attempt := 0; attempt < retry.MaxAttempts; attempt++ {
		out, err := fn()
		if err == nil { return out, nil }
		lastErr = err
		if attempt < retry.MaxAttempts-1 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
			}
			switch retry.Backoff {
			case "exponential":
				delay *= 2
			case "linear":
				delay += parseDuration(retry.Delay, time.Second)
			}
		}
	}
	return nil, lastErr
}

// handleError applies onError policy.
func handleError(err error, onError string) ErrorAction {
	switch onError {
	case "continue":
		return ActionContinueWithNull
	case "branch":
		return ActionBranchToError
	default: // "stop" or empty
		return ActionStopRun
	}
}
```

- [ ] Step 1-3

---

### Task 16:Approval / wait — paused state persist + rehydrate

**Files:**
- Create: `backend/internal/app/scheduler/pause.go`
- Create: `backend/internal/app/scheduler/rehydrate.go`

per spec §3.5 + §6.1:

- Approval 节点 → persist ExecutionContext to flowruns.paused_state JSON,设 status=paused,释放 goroutine
- HTTP `POST /flowruns/{id}/approvals/{nodeId}` → 唤醒
- Boot 时 `Service.RehydrateOnBoot()` 扫 paused FlowRun → rehydrate ExecutionContext → 继续执行
- Wait 节点 → 类似;timer 起来到时唤醒

- [ ] Step 1-3:实现 + 单测(rehydrate / approve / reject / timeout)

---

## Phase 6:Workflow → Scheduler Wire-up(替换 Plan 04 占位)

### Task 17:替换 Plan 04 SchedulerForwarder 占位

**Files:** Modify `backend/cmd/server/main.go`

```go
// 之前 Plan 04 留的:
// workflowSvc := workflowapp.NewService(... nil /* SchedulerForwarder */ ...)

// 现在 Plan 05:
schedulerSvc := schedulerapp.New(...)
workflowSvc.SetScheduler(schedulerSvc) // 加 setter
```

- [ ] Step 1-3

---

### Task 18:Workflow accept/revert/delete → trigger service register/unregister

`workflowSvc.OnActiveVersionChange(...)` listener pattern:Workflow accept pending / revert / delete 时,trigger service unregister 老 + register 新。

- [ ] Step 1-3

---

## Phase 6.5:其他 2 张 Execution Log 表(D22 — mcp_calls + skill_executions)

### Task 16a:`mcp_calls` 表 + Service.CallTool 写入

**Files:**
- Modify: `backend/internal/domain/mcp/`(加 Call entity per spec 08 §4.3)
- Modify: `backend/internal/infra/store/mcp/`(GORM impl + 集成测试)
- Modify: `backend/internal/app/mcp/calltool.go`(终态写 mcp_calls row)
- Add HTTP `GET /api/v1/mcp-servers/{name}/calls`

参考 Plan 01 Task 23a-d 同套路。MCP-specific 字段:`server_name` / `tool_name` / `server_version`。ID 前缀 `mcl_<16hex>`。

- [ ] Step 1-3

### Task 16b:`skill_executions` 表 + Service.Execute 写入

**Files:**
- Modify: `backend/internal/domain/skill/`(加 Execution entity per spec 08 §4.4)
- Modify: `backend/internal/infra/store/skill/`(GORM impl)
- Modify: `backend/internal/app/skill/skill.go::Execute`(终态写 skill_executions row)
- Add HTTP `GET /api/v1/skills/{name}/executions`

Skill-specific:`skill_name` / `skill_version`(SHA256)/ `fork_depth` / `substitutions` JSON。ID 前缀 `ske_<16hex>`。

- [ ] Step 1-3

### Task 16c:scheduler.dispatchNode 写 flowrun_nodes(共享 schema 模板)

**Files:** Modify `backend/internal/app/scheduler/scheduler.go::dispatchNode`

per spec 08 §4.5 + Task 2 已更新的 FlowRunNode struct。每节点 dispatch 完成写一行。

**重要**:capability 节点(function/handler/mcp/skill)dispatch 时**也触发对应 entity execution 写入**(via 各 service Run/Call/Execute)— 跨表 ID 通过 `flowrun_node_id` 字段链接(详 spec 08 §4.5 重要标注)。

- [ ] Step 1-3:实现 + 单测覆盖 cross-table linking

### Task 16d:LLM 工具 — workflow / mcp / skill 执行日志(per-entity)

**Files:**
- Create: `backend/internal/app/tool/workflow/search_executions.go` + `get_execution.go`(workflow_executions 查 flowrun_nodes 表)
- Create: `backend/internal/app/tool/mcp/search_executions.go` + `get_execution.go`(mcp_calls 表)
- Create: `backend/internal/app/tool/skill/search_executions.go` + `get_execution.go`(skill_executions 表)

参考 Plan 01 Task 23e。3 域 × 2 工具 = **6 个 LLM 工具**。各自 kind-specific filter:
- workflow:`workflowId / flowrunId / nodeType`
- mcp:`serverName / toolName`
- skill:`skillName / forkDepth`

- [ ] Step 1: 3 × 2 = 6 工具实现(每个 ~80 行,共 ~480 行)
- [ ] Step 2: 各 factory 包加进 tools slice(workflowtool / mcptool / skilltool)
- [ ] Step 3: 单测覆盖 + main.go 装配 + commit

---

## Phase 7:14 Production Hardening Items 验证

### Task 19-32:14 项 V1 必做项的 unit + pipeline 测试

每条 spec §6 列的项写一个独立 pipeline test 验证:

- [ ] **Task 19:** 进程重启 paused run rehydrate(§6.1)
- [ ] **Task 20:** Cron 漏触发 missedPolicy=runOnce(§6.2)
- [ ] **Task 21:** 同 wf 多 run 默认 serial(§6.3)
- [ ] **Task 22:** 节点级 timeout(§6.8)
- [ ] **Task 23:** Approval timeout(§6.9)
- [ ] **Task 24:** 失败 run notification(§6.4)
- [ ] **Task 25:** Workflow enabled 开关(§6.5)
- [ ] **Task 26:** Webhook secret(§6.6)
- [ ] **Task 27:** FlowRun 保留 200/wf(§6.7)
- [ ] **Task 28:** Cron 时区锁本地(§6.10)
- [ ] **Task 29:** Fsnotify 路径不存在 fail-soft(§6.11)
- [ ] **Task 30:** Trigger 状态可见 GET /workflows/{id}/triggers(§6.12)
- [ ] **Task 31:** Trigger panic recover(§6.13)
- [ ] **Task 32:** Run cancellation cleanup(§6.14)

每项一个 pipeline test 文件 / 一个 commit。

---

## Phase 8:HTTP API

### Task 33:HTTP handlers for execution plane

**Files:** 多个新 handler 文件

per spec §7:

```
POST   /api/v1/workflows/{id}:trigger              手动触发(转 scheduler)
GET    /api/v1/flowruns                            列表
GET    /api/v1/flowruns/{id}                       详情
GET    /api/v1/flowruns/{id}/nodes                 node 执行记录
DELETE /api/v1/flowruns/{id}                       取消
POST   /api/v1/flowruns/{id}/approvals/{nodeId}    approval 签收
POST   /api/v1/webhooks/{wfId}/{path}              webhook 入口(动态)
GET    /api/v1/workflows/{id}/triggers             trigger 状态(§6.12)
```

- [ ] Step 1-3

---

## Phase 9:Cross-platform + Doc Sync

### Task 34:三平台 + staticcheck + doc sync

参考 Plan 01 Task 25/26。新增 service-design-documents:scheduler.md / trigger.md / flowrun.md(各一份)+ 4 contract docs sync + progress + backend-design。

- [ ] Step 1-6

---

## Phase 10:PR + Merge

### Task 35:Open PR

```bash
gh pr create --title "feat(execution): scheduler + trigger + flowrun + 14 production-grade hardening" --body "$(cat <<'EOF'
## Summary
- 3 sibling domains (scheduler / trigger / flowrun) for workflow execution
- Trigger 4 kinds: cron (robfig/cron) / fsnotify / webhook (httpapi sub-path) / manual
- Scheduler StartRun + executeRun + dispatch (13 node dispatchers) + retry/onError/timeout
- FlowRun + FlowRunNode persistence
- Approval / wait paused state persist + boot-time rehydrate (§6.1)
- 14 V1 production hardening items all tested:
  - Cron missedPolicy runOnce / serial concurrency / per-node timeout
  - Workflow enabled gate / Webhook secret / Fsnotify path fail-soft
  - Approval 7d default + onTimeout / Cron Local TZ / Trigger panic recover
  - FlowRun retention 200/wf / Trigger state observability / Run cancel cleanup
- Plan 04 placeholder SchedulerForwarder replaced with real scheduler

## Test plan
- [x] make test-unit + make test-pipeline (full execution flow + 14 hardening tests)
- [x] manual: cron trigger → run → completion notification
- [x] manual: webhook curl → run → flowrun queryable
- [x] manual: kill -KILL backend with paused run, restart → rehydrate works
- [x] 三平台 cross-compile / staticcheck 0 / S14 doc sync

## Related
- spec: 05-execution-plane.md
- plan: plans/05-execution-plane.md
EOF
)"
```

---

## Acceptance criteria

1. ✅ 35 task done
2. ✅ All 4 trigger types fire correctly
3. ✅ All 13 node dispatchers handle their type
4. ✅ All 14 production hardening items have passing pipeline test
5. ✅ Plan 04 trigger_workflow LLM tool now works(scheduler 装好)
6. ✅ S14 doc sync(scheduler.md / trigger.md / flowrun.md + contract + progress + backend-design)
7. ✅ PR merge to main + push

完工后,Plan 06(Subagent + Catalog ext + E2E)接力 — 最后一份。

---

(本 plan 完)
