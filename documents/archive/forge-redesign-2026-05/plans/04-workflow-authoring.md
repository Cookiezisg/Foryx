# Workflow Authoring Implementation Plan

> ✅ **COMPLETED 2026-05-12** — 9 commits W1-W9 直推 main(c5ccfc9 / 3cf96cc / 630d126 / c41738c / 0783a47 / 9d8892c / 1ddb96f / 2130e50 / 本 commit)。
> 见 [`../../service-design-documents/workflow.md`](../../service-design-documents/workflow.md) 完整成品 spec;[`progress-record.md`](../../../progress-record.md) 2026-05-12 dev log。
>
> 故意不含 ErrPendingConflict(iterate-same-pending D-redo-11);无 envfix loop(workflow 无 env)。Plan 05 territory:trigger / flowrun / scheduler / `:trigger` action / execution log。

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Workflow domain 的 **authoring 那一面**(锻造模式) — DAG 数据结构 + 13 节点类型 + ops + 校验 + LLM tools + HTTP CRUD。**不包含执行 plane**(scheduler / trigger / flowrun 在 Plan 05)。

**前置依赖**:Plan 01-03 已 merge(Function + Handler + eventlog scope + HTTP/2 都就位;workflow domain 引用 function / handler / eventlog scope:workflow:wf_xxx)。

**Architecture:** 锻造模式 vs 执行模式分离(D6)。本 plan 只管图怎么样(workflow domain),不管图怎么跑。Workflow 实体结构跟 Function / Handler 同套路:`workflows` + `workflow_versions` 2 表。Graph JSON 整图存 `workflow_versions.graph` 列。Op 集合:add_node / update_node / delete_node / add_edge / update_edge / delete_edge / set_variable / unset_variable / set_meta。13 节点类型:trigger / function / handler / mcp / skill / llm / http / condition / loop / parallel / approval / wait / variable。

**Tech Stack:** Go 1.25,GORM,Go `text/template`(表达式语言),pkg/eventlog(双写 conversation + workflow scope per D19)。

**关联**:[`04-workflow.md`](../04-workflow.md) 完整 spec / [`01-shared-tool-interface.md`](../01-shared-tool-interface.md) 工具接口 / [`07-notifications-and-eventlog.md`](../07-notifications-and-eventlog.md)。

---

## Phase 0:Branch Setup

### Task 1:Branch + 验证 prereq

- [ ] **Step 1: 验证 main 包含 Plan 01-03**

```bash
git checkout main && git pull origin main
ls backend/internal/domain/{function,handler,eventlog}
ls backend/internal/infra/eventlog/  # Bridge 应 multi-scope 化
```

- [ ] **Step 2: 创建分支**

```bash
git checkout -b feature/workflow-authoring
```

---

## Phase 1:Domain Layer(`internal/domain/workflow/`)

### Task 2:Workflow + 12 sentinels

**Files:** Create `backend/internal/domain/workflow/workflow.go`

- [ ] **Step 1: 写 entity + sentinels**

```go
// Package workflow defines the Workflow authoring domain — DAG + 13 node types.
//
// Package workflow 定义 Workflow authoring domain — DAG + 13 节点类型。
package workflow

import (
	"errors"
	"time"

	"gorm.io/gorm"
)

// Workflow is the user-named workflow entity. Active version points to the
// frozen graph that scheduler executes. Pending versions are LLM-driven edits
// awaiting user accept.
//
// Workflow 是用户命名的 workflow 实体。Active version 指向冻结图(scheduler
// 执行)。Pending 是 LLM 改动等用户 accept。
type Workflow struct {
	ID               string `gorm:"primaryKey"`
	UserID           string `gorm:"index;not null"`
	Name             string `gorm:"not null"`
	Description      string
	Tags             []string `gorm:"serializer:json"`
	Enabled          bool     `gorm:"default:true"`             // V1 必做(05-execution §6.5)
	Concurrency      string   `gorm:"default:'serial'"`         // serial | parallel(N)(V1 默认 serial,§6.3)
	NeedsAttention   bool     `gorm:"default:false"`            // D20 — 引用 capability 被删后自动标
	AttentionReason  string                                     // needs_attention=true 时简短说明
	ActiveVersionID  string   `gorm:"index"`
	CreatedAt        time.Time
	UpdatedAt        time.Time
	DeletedAt        gorm.DeletedAt `gorm:"index"`

	// Computed fields
	Pending      *Version   `gorm:"-"`
	LiveRuns     int        `gorm:"-"`
	LastFiredAt  *time.Time `gorm:"-"`
	NextFireAt   *time.Time `gorm:"-"` // cron trigger 时
}

var (
	ErrNotFound              = errors.New("workflow: not found")
	ErrDuplicateName         = errors.New("workflow: duplicate name")
	ErrVersionNotFound       = errors.New("workflow: version not found")
	ErrPendingNotFound       = errors.New("workflow: pending not found")
	ErrPendingConflict       = errors.New("workflow: pending conflict")
	ErrNoActiveVersion       = errors.New("workflow: no active version")
	ErrDAGCycle              = errors.New("workflow: DAG cycle detected")
	ErrInvalidReference      = errors.New("workflow: invalid reference")
	ErrNoTrigger             = errors.New("workflow: at least one trigger node required")
	ErrOpInvalid             = errors.New("workflow: op invalid")
	ErrDisabled              = errors.New("workflow: disabled")
	ErrCapabilityNotFound    = errors.New("workflow: capability not found")
	ErrMCPServerNotInstalled = errors.New("workflow: MCP server not installed")
	ErrMCPServerUnavailable  = errors.New("workflow: MCP server unavailable")
	ErrCapabilityRemoved     = errors.New("workflow: capability removed (D20)")
	ErrNeedsAttention        = errors.New("workflow: needs attention (D20)")
)
```

- [ ] **Step 2: 编译 + commit**

```bash
git add backend/internal/domain/workflow/workflow.go
git commit -m "feat(workflow): domain entity + 16 sentinels"
git push origin feature/workflow-authoring
```

---

### Task 3:Version + Graph + NodeSpec + EdgeSpec + VariableSpec

**Files:** Create `backend/internal/domain/workflow/version.go` + `node.go` + `edge.go`

- [ ] **Step 1: version.go**

```go
package workflow

import "time"

const (
	StatusPending  = "pending"
	StatusAccepted = "accepted"
	StatusRejected = "rejected"
)

// Version is a frozen graph snapshot.
//
// Version 是冻结的图快照。
type Version struct {
	ID           string `gorm:"primaryKey"`
	WorkflowID   string `gorm:"index;not null"`
	Status       string `gorm:"not null;check:status IN ('pending','accepted','rejected')"`
	Version      *int
	Graph        Graph  `gorm:"serializer:json"` // 整张 DAG
	ChangeReason string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// Graph is the DAG JSON shape (per spec 04-workflow §4).
//
// Graph 是 DAG JSON 形态。
type Graph struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Variables   []VariableSpec `json:"variables"`
	Nodes       []NodeSpec     `json:"nodes"`
	Edges       []EdgeSpec     `json:"edges"`
}

// VariableSpec — workflow-level state declaration.
type VariableSpec struct {
	Name    string `json:"name"`
	Type    string `json:"type"` // string | number | integer | boolean | object | array
	Default any    `json:"default,omitempty"`
}
```

- [ ] **Step 2: node.go**

```go
package workflow

// NodeSpec — universal node definition.
//
// NodeSpec 通用节点定义。
type NodeSpec struct {
	ID       string                 `json:"id"`
	Type     string                 `json:"type"`     // 13 V1 types
	Position Position               `json:"position"` // {x,y}
	Config   map[string]any         `json:"config"`   // type-specific
	Retry    *RetryPolicy           `json:"retry,omitempty"`
	OnError  string                 `json:"onError,omitempty"` // stop|continue|branch
	Timeout  int                    `json:"timeout,omitempty"` // ms
	Notes    string                 `json:"notes,omitempty"`
}

type Position struct {
	X int `json:"x"`
	Y int `json:"y"`
}

type RetryPolicy struct {
	MaxAttempts int    `json:"maxAttempts"`
	Backoff     string `json:"backoff"` // exponential | linear | fixed
	Delay       string `json:"delay"`   // duration string e.g. "1s"
}

// 13 V1 node types.
const (
	NodeTypeTrigger   = "trigger"
	NodeTypeFunction  = "function"
	NodeTypeHandler   = "handler"
	NodeTypeMCP       = "mcp"
	NodeTypeSkill     = "skill"
	NodeTypeLLM       = "llm"
	NodeTypeHTTP      = "http"
	NodeTypeCondition = "condition"
	NodeTypeLoop      = "loop"
	NodeTypeParallel  = "parallel"
	NodeTypeApproval  = "approval"
	NodeTypeWait      = "wait"
	NodeTypeVariable  = "variable"
)

// IsValidNodeType returns true if t is one of the V1 13.
func IsValidNodeType(t string) bool {
	switch t {
	case NodeTypeTrigger, NodeTypeFunction, NodeTypeHandler, NodeTypeMCP, NodeTypeSkill,
		NodeTypeLLM, NodeTypeHTTP, NodeTypeCondition, NodeTypeLoop, NodeTypeParallel,
		NodeTypeApproval, NodeTypeWait, NodeTypeVariable:
		return true
	}
	return false
}

// IsCapabilityNode returns true for nodes that have onError + retry semantics.
//
// IsCapabilityNode 返 true 给有 onError + retry 语义的节点(6 类)。
func IsCapabilityNode(t string) bool {
	switch t {
	case NodeTypeFunction, NodeTypeHandler, NodeTypeMCP, NodeTypeSkill, NodeTypeLLM, NodeTypeHTTP:
		return true
	}
	return false
}
```

- [ ] **Step 3: edge.go**

```go
package workflow

// EdgeSpec — connects from <node>.<output_port> to <node>.<input_port>.
//
// EdgeSpec 连接 <node>.<output_port> 到 <node>.<input_port>。
type EdgeSpec struct {
	ID   string `json:"id"`   // 系统生成
	From string `json:"from"` // "<nodeID>.<port>"
	To   string `json:"to"`
}

// 各节点 output / input ports — V1 fixed schema.
//
// 各节点 output / input port — V1 固定 schema。
var NodePorts = map[string]struct {
	Inputs  []string
	Outputs []string
}{
	"trigger":   {Inputs: nil, Outputs: []string{"next"}},
	"function":  {Inputs: []string{"input"}, Outputs: []string{"output", "error"}},
	"handler":   {Inputs: []string{"input"}, Outputs: []string{"output", "error"}},
	"mcp":       {Inputs: []string{"input"}, Outputs: []string{"output", "error"}},
	"skill":     {Inputs: []string{"input"}, Outputs: []string{"output", "error"}},
	"llm":       {Inputs: []string{"input"}, Outputs: []string{"output", "error"}},
	"http":      {Inputs: []string{"input"}, Outputs: []string{"output", "error"}},
	"condition": {Inputs: []string{"input"}, Outputs: []string{"true", "false"}},
	"loop":      {Inputs: []string{"input"}, Outputs: []string{"output"}},
	"parallel":  {Inputs: []string{"input"}, Outputs: []string{"output"}},
	"approval":  {Inputs: []string{"input"}, Outputs: []string{"approved", "rejected", "timeout"}},
	"wait":      {Inputs: []string{"input"}, Outputs: []string{"output"}},
	"variable":  {Inputs: []string{"input"}, Outputs: []string{"output"}},
}
```

- [ ] **Step 4: 编译 + commit**

```bash
git add backend/internal/domain/workflow/
git commit -m "feat(workflow): Version + Graph + NodeSpec(13 types) + EdgeSpec + Ports"
git push
```

---

### Task 4:Repository 接口

参考 Plan 01 Task 4。基本 CRUD + Version CRUD。

- [ ] Step 1-3

---

### Task 5:Domain layer 单测

- [ ] Step 1-3:sentinel uniqueness + IsValidNodeType + IsCapabilityNode coverage + Graph JSON round-trip

---

## Phase 2:Store Layer

### Task 6:GORM Repository 实现

参考 Plan 01 Task 6。partial UNIQUE on `(user_id, name) WHERE deleted_at IS NULL`。

- [ ] Step 1-3

### Task 7:Store 集成测试

参考 Plan 01 Task 7。加 Workflow 特殊字段(Enabled / Concurrency / NeedsAttention / AttentionReason)round-trip 测试。

- [ ] Step 1-3

---

## Phase 3:App Layer

### Task 8:Service struct

**Files:** Create `backend/internal/app/workflow/workflow.go`

```go
type Service struct {
	repo  workflowdomain.Repository
	notif *notificationspkg.Publisher

	// 跨 domain reference checking(D12 / D20):
	functionSvc CapabilityChecker
	handlerSvc  CapabilityChecker
	skillSvc    CapabilityChecker
	mcpSvc      MCPChecker

	scheduler SchedulerForwarder // 用于 trigger_workflow tool;Plan 05 实现真正 scheduler;V1 plan04 占位接口

	log *zap.Logger
}

// CapabilityChecker — 跨 domain 校验"name 在不在"用。
type CapabilityChecker interface {
	Exists(ctx context.Context, name string) (bool, error)
}

type MCPChecker interface {
	IsInstalled(ctx context.Context, serverName string) (bool, error)
}

// SchedulerForwarder — V1 plan04 占位;Plan 05 完整实现。
type SchedulerForwarder interface {
	StartRun(ctx context.Context, workflowID string, input map[string]any) (runID string, err error)
}
```

- [ ] Step 1-3:写 + 编译 + commit

---

### Task 9:apply.go — 9 个 ops

**Files:** Create `backend/internal/app/workflow/apply.go` + `apply_test.go`

参考 Plan 01 Task 9。Workflow ops 集合:

```go
func applyOne(state *GraphDraft, op Op) error {
	switch op.Type {
	case "set_meta": ...
	case "add_node":
		var p struct{ Node workflowdomain.NodeSpec }
		// 校验 node.id 唯一 / type 在白名单 / config schema 大致合法
	case "update_node":
		var p struct { ID string; Patch json.RawMessage }
		// JSON Merge Patch(RFC 7396)— 详见 Task 10
	case "delete_node":
		// 级联删 edge(from / to 该 node 的)
	case "add_edge":
		var p struct{ Edge workflowdomain.EdgeSpec }
		// 校验 from / to 节点存在 + port 存在
	case "update_edge":
	case "delete_edge":
	case "set_variable":
		var p struct{ Name, Type string; Default any }
	case "unset_variable":
		// 校验图内无 {{ vars.X }} 引用此变量,有则 reject
	}
}
```

每 op 应用时:per-op + 累积 + final 三层校验(Task 11)。

- [ ] Step 1-2:apply.go + apply_test.go(每 op 至少一个 happy + 一个 fail case;~10 测试)
- [ ] Step 3: commit

---

### Task 10:JSON Merge Patch 实现(D15 一致性,update_node / update_edge)

**Files:** Create `backend/internal/pkg/jsonpatch/merge.go`(若不存在)

per RFC 7396 实现 JSON Merge Patch。Go 现有库:`github.com/evanphx/json-patch/v5` 有 MergePatch 实现,加 dep 即可。

```go
package jsonpatch

import (
	jsonpatch "github.com/evanphx/json-patch/v5"
)

// Merge applies an RFC 7396 JSON Merge Patch to original document.
//
// Merge 把 RFC 7396 JSON Merge Patch 应用到原文档。
func Merge(original, patch []byte) ([]byte, error) {
	return jsonpatch.MergePatch(original, patch)
}
```

加进 `go.mod`:

```bash
cd backend && go get github.com/evanphx/json-patch/v5
```

- [ ] Step 1-3:写 + test + commit

---

### Task 11:validate.go — 校验规则(per-op + 累积 + final)

**Files:** Create `backend/internal/app/workflow/validate.go` + tests

per spec 04-workflow §7.3:

```go
// validateFinal — 全部 ops 应用完跑。9 条规则。
func validateFinal(g *workflowdomain.Graph, deps DependencyResolver) error {
	// 1. 顶层 DAG 无环
	if err := topoSort(g.Nodes, g.Edges); err != nil {
		return workflowdomain.ErrDAGCycle
	}
	// 2. 节点 id 唯一
	// 3. node type 白名单
	for _, n := range g.Nodes {
		if !workflowdomain.IsValidNodeType(n.Type) {
			return fmt.Errorf("invalid node type: %q", n.Type)
		}
	}
	// 4. 每条 edge 引用合法
	// 5. 至少 1 个 trigger
	// 6. trigger config 完整(各 kind 必填)
	// 7. capability 节点引用 — function / handler / skill / mcp 的 name 都要存在
	// 8. variable 引用合法
	// 9. container 节点 body 递归校验
	return nil
}

type DependencyResolver interface {
	FunctionExists(ctx context.Context, name string) (bool, error)
	HandlerExists(ctx context.Context, name string) (bool, error)
	SkillExists(ctx context.Context, name string) (bool, error)
	MCPServerInstalled(ctx context.Context, name string) (bool, error)
}
```

V1 关键校验(必做):
- DAG cycle
- 至少 1 trigger
- capability 节点引用存在
- mcp 节点 serverName 必须已装(`WORKFLOW_MCP_SERVER_NOT_INSTALLED`)

- [ ] Step 1-3:实现 + 单测(每条 final 校验规则一个失败 case)

---

### Task 12:expression.go — 模板表达式

**Files:** Create `backend/internal/app/workflow/expression.go` + tests

V1 简化:Go `text/template` + 自定义 funcMap(per spec §6.2)。

```go
package workflow

import (
	"bytes"
	"fmt"
	"text/template"
)

// EvalContext 封装 expression 求值时可见的所有变量(运行时由 scheduler 填,
// 锻造期校验时 V1 用空 ctx 仅做 syntax check)。
//
// EvalContext encloses everything visible to an expression at runtime.
type EvalContext struct {
	Vars       map[string]any        // workflow-level variables
	In         map[string]any        // current node's input
	Nodes      map[string]NodeOutput // outputs of past nodes
	Loop       *LoopContext          // loop body 内有 loop.item / loop.index
	Run        RunContext            // run.id / run.startedAt
	Env        map[string]string     // V1 受白名单限
}

type NodeOutput struct {
	Output map[string]any
}

type LoopContext struct {
	Item  any
	Index int
}

type RunContext struct {
	ID        string
	StartedAt time.Time
}

// Eval 编译 + 求值 expression(`{{ ... }}` 模板)。
//
// Eval compiles and evaluates an expression template.
func Eval(expr string, ec EvalContext) (string, error) {
	t, err := template.New("expr").Parse(expr)
	if err != nil {
		return "", fmt.Errorf("expression parse: %w", err)
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, map[string]any{
		"vars":  ec.Vars,
		"in":    ec.In,
		"nodes": ec.Nodes,
		"loop":  ec.Loop,
		"run":   ec.Run,
		"env":   ec.Env,
	}); err != nil {
		return "", fmt.Errorf("expression eval: %w", err)
	}
	return buf.String(), nil
}

// SyntaxCheck 仅 parse,不求值(锻造期 final 校验用)。
//
// SyntaxCheck only parses, no eval (for forge-time final validation).
func SyntaxCheck(expr string) error {
	_, err := template.New("syntax").Parse(expr)
	return err
}
```

- [ ] Step 1-3:实现 + 单测(Vars / In / Nodes / Loop / SyntaxCheck 失败 case;~10 tests)

---

### Task 13:Service CRUD + ApplyOps + pending/accept

**Files:** Modify `backend/internal/app/workflow/workflow.go`

参考 Plan 01 Task 11。Service 方法:Search / Get / Create / Edit / AcceptPending / RejectPending / Revert / Delete / Trigger(forwards to scheduler)。

每 op 应用 emit progress 时双写 `conversation:cv_xxx` + `workflow:wf_xxx` scope(D19)。

- [ ] Step 1-3

---

### Task 14:Cross-domain reference checker — wire function/handler/skill/mcp services

**Files:** Modify `backend/cmd/server/main.go` 装配段(预备 Plan 05 之前先把 workflow service 接 Function/Handler 等的 CapabilityChecker)

```go
// 现有 main.go 加:
workflowSvc := workflowapp.NewService(
	workflowstore.New(gdb),
	notificationsPub,
	functionSvc,  // CapabilityChecker
	handlerSvc,   // CapabilityChecker
	skillSvc,     // CapabilityChecker
	mcpSvc,       // MCPChecker
	nil,          // SchedulerForwarder — Plan 05 真接;V1 plan04 nil(trigger_workflow 返 503)
	log,
)
```

functionSvc / handlerSvc 需要新加 Exists method:

```go
// app/function/function.go 加:
func (s *Service) Exists(ctx context.Context, name string) (bool, error) {
	uid, _ := reqctxpkg.RequireUserID(ctx)
	_, err := s.repo.GetByName(ctx, uid, name)
	if err == functiondomain.ErrNotFound {
		return false, nil
	}
	return err == nil, err
}
```

- [ ] Step 1-3

---

### Task 15:Notifications listener — capability 删时标 needs_attention(D20)

**Files:** Create `backend/internal/app/workflow/cascade.go`

```go
// CapabilityRemovedListener — listens for function / handler / skill / mcp_server
// deletion notifications, scans workflow graphs referencing them, marks needs_attention.
//
// CapabilityRemovedListener — 听 function / handler / skill / mcp_server 删除通知,
// 扫引用此 capability 的 workflow,标 needs_attention。
func (s *Service) StartCapabilityListener(ctx context.Context, sub notificationsdomain.Subscriber) {
	go func() {
		ch := sub.Subscribe()
		for envelope := range ch {
			if !isCapabilityDeletion(envelope) {
				continue
			}
			capKind, capName := extractCapability(envelope)
			s.markReferencingWorkflows(ctx, capKind, capName)
		}
	}()
}

func (s *Service) markReferencingWorkflows(ctx context.Context, capKind, capName string) {
	// 列所有 workflow,扫 graph 引用
	wfs, _, _ := s.repo.List(ctx, "local-user", workflowdomain.ListFilter{Limit: 10000})
	for _, wf := range wfs {
		if wf.ActiveVersionID == "" { continue }
		v, _ := s.repo.GetVersionByID(ctx, wf.ActiveVersionID)
		if v == nil { continue }
		if !graphReferences(v.Graph, capKind, capName) { continue }

		s.repo.UpdateNeedsAttention(ctx, wf.ID, true,
			fmt.Sprintf("%s %q was deleted", capKind, capName))
		s.notif.Publish(notificationsdomain.Envelope{
			Type: "workflow",
			ID:   wf.ID,
			Data: map[string]any{"action": "needs_attention", "reason": ...},
		})
	}
}
```

- [ ] Step 1-3:实现 + 单测(模拟 function 删 → 引用 wf 标 needs_attention)

---

## Phase 4:LLM Tools(7 个)

### Tasks 16-22:7 LLM tools

参考 Plan 01 Task 14-20。每 task 一个文件,~80-150 行。

- [ ] Task 16: search_workflow
- [ ] Task 17: get_workflow
- [ ] Task 18: create_workflow(ops 流式 + 双写 scope)
- [ ] Task 19: edit_workflow(ops 流式 + 双写)
- [ ] Task 20: revert_workflow
- [ ] Task 21: delete_workflow
- [ ] Task 22: trigger_workflow(转发给 scheduler;V1 plan04 调时若 scheduler nil 返 503 `Scheduler not yet wired`,Plan 05 实装)

每 task 完成 commit + push。

---

## Phase 5:HTTP API

### Task 23:HTTP handlers ~14 endpoints

**Files:** Create `backend/internal/transport/httpapi/handlers/workflow.go` + 测试

per spec 04-workflow §10。包含 `/workflows/{id}/triggers` GET(看 trigger 状态;V1 plan04 返空数组,Plan 05 真接)。

- [ ] Step 1-5

---

## Phase 6:Wire-up

### Task 24:main.go + harness 装 WorkflowService + capability listener

参考 Plan 01 Task 22 + Task 14 + Task 15(listener startup)。

- [ ] Step 1: 装 WorkflowService
- [ ] Step 2: AutoMigrate 加 Workflow + Version
- [ ] Step 3: function / handler / skill / mcp service 加 Exists method
- [ ] Step 4: 启 capability listener goroutine
- [ ] Step 5: errmap 加 16 sentinels
- [ ] Step 6: LLM 工具 append to tools slice
- [ ] Step 7: router deps 加 WorkflowService 字段
- [ ] Step 8: 编译 + test-unit + commit

---

## Phase 7:Pipeline Test

### Task 25:Workflow pipeline tests(authoring side only;execution 在 Plan 05)

**Files:** Create `backend/test/workflow/workflow_pipeline_test.go`

V1 plan04 测试场景(无 scheduler):

1. **CreateWorkflowOpsStreaming**:LLM ops 流式 create_workflow → DB 落 → graph JSON 完整
2. **EditWorkflowGoesPending**:edit → pending 写入 → accept → active 翻
3. **DAGCycleRejected**:create with cycle → final 校验 reject + WORKFLOW_DAG_CYCLE 错误
4. **CapabilityNotFoundRejected**:create 引用不存在的 function 名 → reject
5. **MCPServerNotInstalledRejected**:create 引用未装 mcp server → WORKFLOW_MCP_SERVER_NOT_INSTALLED
6. **NeedsAttentionMarking**:create wf 引用 fn_x → delete_function fn_x → wf 自动标 needs_attention

**注:**`trigger_workflow` E2E 测试推到 Plan 05(scheduler 装好后)。

- [ ] Step 1-3

---

## Phase 8:Cross-platform + Doc Sync

### Task 26:三平台 + staticcheck + doc sync

参考 Plan 01 Task 25 + 26。

- [ ] Step 1-6:cross compile + staticcheck + service-design-documents/workflow.md + 4 contract docs sync + progress + backend-design

---

## Phase 9:PR + Merge

### Task 27:Open PR

```bash
gh pr create --title "feat(workflow): authoring domain (DAG + 13 nodes + ops + validation)" --body "$(cat <<'EOF'
## Summary
- Workflow authoring domain — entity, version, graph JSON storage
- 13 V1 node types + ports + retry/onError per capability node
- 9 ops (set_meta + node CRUD + edge CRUD + variable CRUD)
- Final validation: DAG cycle / capability refs / MCP installed / no trigger
- JSON Merge Patch (RFC 7396) for update_node / update_edge
- Text/template expression evaluator with Vars/In/Nodes/Loop/Run/Env
- Cross-domain capability listener — D20 cascade needs_attention
- 7 LLM tools, 14 HTTP endpoints
- D19 双写到 conversation + workflow scope
- trigger_workflow placeholder (returns 503 until Plan 05 scheduler)

## Test plan
- [x] make test-unit + make test-pipeline (authoring scenarios; trigger E2E in Plan 05)
- [x] 三平台 cross-compile / staticcheck 0 / S14 doc sync

## Related
- spec: 04-workflow.md
- plan: plans/04-workflow-authoring.md
EOF
)"
```

---

## Acceptance criteria

1. ✅ 27 task done
2. ✅ Workflow authoring 端 E2E 通(create / edit / accept / revert / delete / 校验全套)
3. ✅ trigger_workflow 占位返 503(Plan 05 接通)
4. ✅ capability deletion → needs_attention 工作
5. ✅ ops 流式双写 conversation + workflow scope
6. ✅ S14 doc sync
7. ✅ PR merge to main

完工后,Plan 05(Execution Plane)接力。

---

(本 plan 完)
