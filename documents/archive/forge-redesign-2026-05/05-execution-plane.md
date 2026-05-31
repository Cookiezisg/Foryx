# Execution Plane — Scheduler + Trigger + FlowRun

> ⚠️ **本文档 Phase 4 范围,部分 SSE 协议章节已被 Plan 03 redesign 取代**
>
> 本文档的 scheduler / trigger / flowrun / 14 项生产级 V1 主体仍然有效(workflow Plan 04+ 实施时使用)。但其中"**eventlog 协议泛化**"(per-entity scope `?conversationId=` 向后兼容)章节描述的是原 Plan 03 D19 方案,**已被 2026-05-12 redesign 取代** — 现行 SSE 模型是三流 per-user(eventlog + notifications + forge),无 query 参,client 按 payload demux。详 [`discussions/2026-05-12-env-and-sse-rework.md`](./discussions/2026-05-12-env-and-sse-rework.md) §B + [`07-notifications-and-eventlog.md`](./07-notifications-and-eventlog.md)。

**关联**:
- [`00-overview.md`](./00-overview.md) — 顶层愿景
- [`04-workflow.md`](./04-workflow.md) — Authoring plane(workflow domain)
- [`01-shared-tool-interface.md`](./01-shared-tool-interface.md) — 工具接口形态

**本文档范围**:执行那一面 — 三个 sibling domain (scheduler / trigger / flowrun) 的职责、4 种触发器、scheduler 执行模型、FlowRun 持久化、~~eventlog 协议泛化~~(已被 Plan 03 redesign 取代)、**14 项生产级 V1 必做项**、HTTP API、错误码。

---

## 1. 三 domain 职责边界

| Domain | 管 | 不管 |
|---|---|---|
| **trigger** | 监听外部信号(cron / fsnotify / webhook / manual)→ 收信号转给 scheduler | 不知道 workflow 长啥样,不执行任何节点 |
| **scheduler** | 读 active WorkflowVersion → spawn FlowRun → 走 DAG → dispatch 每个节点 → 处理 retry / onError / approval / cancellation | 不持久化 FlowRun(写给 flowrun domain) |
| **flowrun** | FlowRun / FlowRunNode entity 存 + HTTP CRUD + 历史查询 | 不执行,只是记录簿 |

**单向依赖链**:trigger → scheduler → flowrun(只写)。scheduler 是中间编排层。

### 1.1 与 workflow domain 的接口

```go
// 给 scheduler 用 — 读 active version 起 run
type WorkflowReader interface {
    GetActiveVersion(ctx context.Context, workflowID string) (*Version, error)
    GetWorkflow(ctx context.Context, workflowID string) (*Workflow, error)
    ListEnabled(ctx context.Context) ([]*Workflow, error)
}

// 给 trigger 用 — 取所有 trigger 节点配置注册监听器
type TriggerSourceReader interface {
    ListActiveTriggers(ctx context.Context) ([]TriggerSpec, error)
}

// 反向 — workflow accept pending / 翻 active 时通知 trigger 重 register
type WorkflowChangeNotifier interface {
    SubscribeActiveVersionChange(ch chan<- WorkflowChangeEvent)
}
```

详见 [`04-workflow.md`](./04-workflow.md) §14。

---

## 2. Trigger 类型清单(V1 = 4 种)

### 2.1 总览

| 类型 | 实现 | trigger 节点 config | 触发链路 |
|---|---|---|---|
| **cron** | `robfig/cron` 全局 scheduler | `expression: "0 */1 * * *"` | tick → `scheduler.StartRun(wfId, {kind:"cron", at})` |
| **fsnotify** | `fsnotify v1.9` per-path watcher | `path, pattern?, events: [create,modify,delete]` | match → StartRun |
| **webhook** | 注册到 httpapi router 子路径 | `path, method, secret?` | `POST /api/v1/webhooks/{wfId}/{path}` → StartRun |
| **manual** | HTTP / LLM tool 显式调 | — | `POST /workflows/{id}:trigger` 或 `trigger_workflow` LLM tool → StartRun |

### 2.2 Cron Trigger 详细

**Library**:`github.com/robfig/cron/v3`(本项目首次引入)

**Config**:
```yaml
kind: cron
expression: "0 */1 * * *"     # 标准 cron 5-field
timezone: local                # V1 锁本地;V1.5 允许 override
```

**生产级要点**:
- **时区锁本地**(`time.Local`)— 桌面 app 跟用户笔记本时区一致
- **last_fired_at 持久化** — 在 trigger config 行(或 dedicated `trigger_state` 表)记录上次触发时间
- **missedPolicy 默认 `runOnce`** — 详见 §6.2

### 2.3 Fsnotify Trigger 详细

**Library**:`fsnotify v1.9`(已存在 indirect dep)

**Config**:
```yaml
kind: fsnotify
path: "/Users/me/Downloads"   # 监听路径
pattern: "*.csv"              # glob 模式,可选
events: [create, modify]      # subset of [create, modify, delete]
recursive: false              # V1 false;true 留 V1.5
```

**生产级要点**:
- **路径不存在 fail-soft** — listener 标 `state=error`,不阻塞 workflow 存在,通过 notification 告知用户
- 注册时校验路径存在 + 可读

### 2.4 Webhook Trigger 详细

**Library**:`net/http`(注册到 httpapi router 的 `/api/v1/webhooks/{wfId}/{path}` 子路径)

**Config**:
```yaml
kind: webhook
path: "github-pr"             # 唯一路径段
method: "POST"                # GET / POST / PUT / ...
secret: "{{ env.WEBHOOK_SECRET_GITHUB_PR }}"  # 可选,V1 必做
```

**生产级要点(V1 必做)**:
- **secret 防滥用** — 详见 §6.6
- **路径冲突拒** — 同 wfId 不同 trigger 节点不能用同一 path;不同 wfId 间也校验唯一
- **Body 大小限** — V1 默认 10MB,可改

### 2.5 Manual Trigger 详细

无 listener 注册;通过 HTTP / LLM tool 显式调用:
- HTTP:`POST /api/v1/workflows/{id}:trigger`(body 含 `input` JSON)
- LLM:`trigger_workflow({workflowId, input?, wait?})` 工具

主要给开发 / 调试 / "用户点立即跑" UX 路径用。

### 2.6 Trigger 注册时机

```
系统 boot 
  → trigger service 扫所有 enabled WorkflowVersion 的 trigger 节点
  → register listener (cron / fsnotify / webhook 都加)
  
Workflow accept pending(active version 翻)
  → trigger service unregister 老 trigger
  → register 新 trigger 节点
  
Workflow disabled(enabled=false)
  → trigger service unregister(详见 §6.5)
  
Workflow delete(soft)
  → trigger service unregister
```

---

## 3. Scheduler 执行模型

### 3.1 总流程伪代码

```go
StartRun(wfId string, triggerInput any) (runId string, err error):
  workflow := workflowReader.GetWorkflow(ctx, wfId)
  if !workflow.Enabled {
    return "", ErrWorkflowDisabled
  }
  
  // V1 必做:concurrency 检查
  if workflow.Concurrency == "serial" {
    if hasRunningRun(wfId) {
      return queueOrSkip()  // V1 默认 skip(下次再试);V1.5 加 queue
    }
  }
  
  version := workflowReader.GetActiveVersion(ctx, wfId)
  run := flowrun.Create(wfId, version.id, triggerInput, status=running)
  go executeRun(ctx, run, version.graph)
  return run.id, nil

executeRun(ctx, run, graph):
  defer recover()  // panic 兜底
  defer cleanup(ctx, run)  // handler instance 销毁 / final emit
  
  execCtx := buildExecutionContext(run, graph)
  ready := topoSort(graph).initial()  // 初始 ready = 入口 trigger 节点
  
  for {
    if isCancelled(ctx) || allDone(execCtx) {
      break
    }
    nodes := pickReady(execCtx, ready)
    runInParallel(ctx, nodes, dispatchNode)  // 多个并行 ready 一起跑
    advance(execCtx)
  }
  
  // cleanup:销毁此 run 拥有的所有 Handler instance
  handlerRegistry.DestroyAll(Owner{Kind: "flowrun", ID: run.id})
  flowrun.Finalize(run, status, output)
  publishNotification(run)  // 详见 §6.4

dispatchNode(ctx, node):
  emit("node_started", node.id)
  flowrun.UpdateNode(run.id, node.id, status="running")
  
  // V1 必做:节点级 timeout
  nodeCtx, cancel := context.WithTimeout(ctx, node.Timeout)
  defer cancel()
  
  // V1 必做:retry policy
  output, err := withRetry(nodeCtx, node.Retry, func() (any, error) {
    return nodeDispatcher[node.type](nodeCtx, node, execCtx)
  })
  
  // V1 必做:onError policy
  switch handleError(err, node.onError) {
    case stopRun:    failRun(run, err); return
    case continueOK: output = nil
    case branchToError: routeToErrorPort(node, err)
  }
  
  flowrun.UpdateNode(run.id, node.id, status="completed", output=output)
  emit("node_ended", node.id, output)
```

### 3.2 节点 dispatcher

每个节点类型一个 dispatcher 函数。**13 个,各自独立实现**(D5 — 不抽共享 helper):

| Node Type | Dispatcher 主要工作 |
|---|---|
| `trigger` | no-op(已经是入口,被外部 trigger fire 触发的) |
| `function` | `functionapp.Run(ctx, functionID, args)` |
| `handler` | `handlerapp.Call(ctx, handlerName, method, args)` — owner = `flowrun:<runID>` |
| `mcp` | `mcpapp.CallTool(ctx, serverName, toolName, args)` |
| `skill` | `skillapp.Activate + Execute(ctx, skillName, args)` |
| `llm` | `llminfra.Generate(ctx, model, prompt, tools?, knowledgeBaseId?)` |
| `http` | `net/http` GET/POST/...,带 SSRF 守卫 |
| `condition` | 求 expression 结果,标记 true/false output port |
| `loop` | 遍历 items,每次迭代起子图执行(子图同 dispatcher 链) |
| `parallel` | 并发起 branches,等所有完成,合并结果 |
| `approval` | persist execCtx,标 status=paused,return — 等 HTTP 唤醒(详见 §3.5) |
| `wait` | `time.Sleep(duration)` 或等到 `until` 时间 |
| `variable` | 从 / 到 execCtx.Variables 读 / 写 |

dispatcher 独立函数,每个 ~50-150 行 Go,switch case 集中在 `nodeDispatcher map`。**不抽公共框架** — 13 个 dispatcher 各自直白。

### 3.3 状态机

```
pending(可能跳过,V1 不必)
  ↓
running ← → paused(approval / wait 长延时)
  ↓
{ completed | failed | cancelled | timeout }
```

#### 终态语义(V1:5 状态,**无 run-level timeout**)
- `completed` — 正常 + 全部节点 OK
- `failed` — 某节点 onError=stop 触发整 run 停(若是节点 timeout 致 stop,`error_code=NODE_TIMEOUT`,run.status 仍是 failed)
- `cancelled` — 用户取消(HTTP DELETE / LLM cancel_flowrun)
- **不含 `timeout`**(V1 不做 run-level 总超时;V1.5 加时再扩 enum)

节点级 timeout 是 FlowRunNode 的状态(详见 [`08-executions.md`](./08-executions.md) §4.5 — 共享 schema status 含 timeout)。**run vs node 是两层概念,enum 不需对齐**。

### 3.4 Caller-context = FlowRun

参考 [`03-handler.md`](./03-handler.md) §3。Handler instance 在第一次 `handler` 节点执行时 spawn,owner = `flowrun:<runID>`,run 终态时统一 destroy。

execCtx 上挂 `Owner{Kind: "flowrun", ID: run.id}`,`handlerapp.Call` 从 ctx 拿到 owner 自动 acquire / spawn instance。

### 3.5 Approval 节点详细

最复杂的节点,因为它**让 scheduler goroutine 释放**:

```go
dispatchApproval(ctx, node, execCtx):
  // persist 整个 execCtx 到 DB
  flowrun.SetPausedState(run.id, &PausedState{
    NodeID:    node.id,
    Variables: execCtx.Variables,
    Outputs:   execCtx.Outputs,        // 已完成节点的输出
    Position:  execCtx.Position,        // 当前执行到哪
    PausedAt:  time.Now(),
  })
  flowrun.SetStatus(run.id, "paused")
  
  // 不 block —— 释放 goroutine
  return signalPaused
```

唤醒路径:

```
HTTP POST /api/v1/flowruns/{runId}/approvals/{nodeId}
body: { decision: "approved" | "rejected", reason?: string }
  ↓
Scheduler.Resume(runId, nodeId, decision):
  pausedState := flowrun.GetPausedState(runId)
  execCtx := rehydrateContext(pausedState)
  // 从 paused 节点继续走
  routeApprovalOutput(node, decision)  // 走 approved / rejected port
  go executeRun(ctx, run, graph)       // 继续执行
```

**进程重启时**(V1 必做,§6.1):scheduler boot 扫 `flowruns WHERE status='paused'` → 各自 rehydrate(但是 handler instance 已死,新一轮就重 spawn)。

### 3.6 Cancellation

```
HTTP DELETE /api/v1/flowruns/{runId}
或 LLM tool cancel_flowrun(runId)
  ↓
Scheduler.Cancel(runId):
  cancelFunc := activeCancels[runId]
  cancelFunc()  // ctx.Cancel 一路串
  ↓
当前正在跑的节点 ctx.Done() 触发 abort
handler instance cleanup
flowrun.SetStatus(runId, "cancelled")
```

---

## 4. Eventlog 协议泛化(V1 必做)

### 4.1 现状

`domain/eventlog` Bridge keys by `conversationId`,events 写入"某个 conv 的 stream"。

### 4.2 改动:scope 泛化

```go
// 新 type
type Scope struct {
    Kind string  // "conversation" | "flowrun"
    ID   string
}

// Bridge 改 key 类型
type Bridge interface {
    Publish(scope Scope, event Event) error
    Subscribe(scope Scope) <-chan Event
    // ...
}
```

HTTP 端点:
- 老 `GET /api/v1/eventlog?conversationId=...` 仍保(向后兼容)— server 自动转 `Scope{Kind:"conversation", ID:...}`
- 新 `GET /api/v1/eventlog?scope=<kind>:<id>` 接 flowrun 用

### 4.3 触发源 → events 写哪

| 触发源 | events 写哪 | UI 怎么看 |
|---|---|---|
| **chat 触发**(`trigger_workflow` LLM tool) | events 写 `flowrun:fr_xxx` scope;**chat 工具内部 subscribe 该 flowrun stream + re-emit 成 progress block delta** 挂 chat 的 tool_call 父下 | chat conv 用户看到 tool_call 卡片内部呼啦呼啦地长出节点执行 progress |
| **外部触发**(cron / webhook / fsnotify) | events 写 `flowrun:fr_xxx` scope | admin UI 经 `GET /api/v1/eventlog?scope=flowrun:fr_xxx` 订阅看 |

### 4.4 Entity-level scope 扩展(D19)

除 `conversation:` / `flowrun:`,V1 加 3 种 entity-level scope:

```
function:fn_xxx     该 function 锻造期 ops 流(Function 详情页订)
handler:hd_xxx      该 handler 锻造期 + instance 生命周期事件流
workflow:wf_xxx     该 workflow 锻造期 + edit ops 流
```

**双写策略**(LLM 在 chat 锻造时):
- `conversation:<convId>` 挂 tool_call 父下(chat 用户看)
- `function:<fnId>` / `handler:<hdId>` / `workflow:<wfId>` 直接挂(entity 视图看)

非 chat 触发(HTTP / UI / scheduler / watcher)— **单写 entity scope**。

详见 [`07-notifications-and-eventlog.md`](./07-notifications-and-eventlog.md) §2 / §4。

### 4.5 Multi-scope 单连接(D18 落地后随意)

HTTP/2 + TLS 落地(D18)后,前端订阅策略**自由**:
- 一条 SSE 多 scope:`?scope=conversation:cv_xxx&scope=workflow:wf_yyy`
- 多条 SSE 各订一个 scope

后端 Bridge `Subscribe([]Scope) <-chan ScopedEvent` 同时订多 scope,events 带 `scope` 字段返。HTTP handler 解多 `scope=` 参绑到一条 SSE。

### 4.6 实现成本

| 改动 | LOC |
|---|---|
| `Scope{Kind, ID}` struct(替代 `conversationId`)| ~30 |
| Bridge `Subscribe([]Scope)` + 内部 fan-out | ~80 |
| HTTP handler 解多 `scope=` 参 | ~50 |
| 各 service publish 到 entity scope(双写策略)| ~120 |
| 向后兼容 `?conversationId=` query 参 | ~10 |

总 ~290 行 backend,**作为 V1 准备件做**。

---

## 5. FlowRun 持久化 — 2 张表

### 5.1 `flowruns`

| 字段 | 类型 | 说明 |
|---|---|---|
| id | TEXT PK | `fr_<16hex>` |
| workflow_id | TEXT 索引 | FK → workflows.id |
| version_id | TEXT | 锁哪个版本(防 active 切换影响进行中的 run) |
| trigger_kind | TEXT | cron / fsnotify / webhook / manual |
| trigger_input | TEXT (JSON) | 触发时输入 |
| status | TEXT CHECK | `running / paused / completed / failed / cancelled` |
| started_at | DATETIME | — |
| ended_at | DATETIME NULL | 终态时填 |
| elapsed_ms | INT NULL | 同上 |
| output | TEXT (JSON) NULL | 终态产物 |
| error_code | TEXT NULL | failed 时填 |
| error_message | TEXT NULL | 同上 |
| paused_state | TEXT (JSON) NULL | approval / wait 节点保存的恢复点 |
| created_at | — | GORM |

**软删**(deleted_at)— 给保留策略 §6.7 用。

**索引**:`workflow_id, status, started_at`(组合)— 列表查询主路径

### 5.2 `flowrun_nodes` → 详见 [`08-executions.md`](./08-executions.md) §4.5

per D22,**`flowrun_nodes` 表迁到 5 张 per-entity execution log 表中**(共享 schema 模板,16 通用字段 + flowrun-specific:`node_id` / `node_type` / `attempts`)。本节不再重复定义。

要点(详 08 §4.5):
- ID 前缀 `frn_<16hex>`
- status 含 `timeout`(节点级 timeout 是真状态;FlowRun 整体没 timeout 状态 — 见 §3.3)
- 索引:`(flowrun_id, started_at)` 节点追溯主路径
- capability 节点(function/handler/mcp/skill)dispatch 时**同时写两条**:一条到 flowrun_nodes(workflow 视角)+ 一条到对应 entity 表(详 08 §4.5 cross-table linking)
- 非 capability 节点(condition/loop/parallel/approval/wait/variable)只写 flowrun_nodes

---

## 6. 14 项生产级 V1 必做项

下面这些**生产可用所必需的边界**,都在 V1 范围内。

### 6.1 进程重启 paused run rehydrate

**问题**:桌面 app 用户合盖 / 关机 = 常态;approval 节点等用户半小时回来时电脑可能已睡 → 重启就丢就极差体验。

**V1 答案**:
- `flowruns.paused_state` JSON 字段持久化整 ExecutionContext
- scheduler boot 时扫所有 `status='paused'` → rehydrate ExecutionContext + 重新等信号
- Handler instance 不跨重启(paused 时已 destroy);resume 时下游若需 handler 节点,新 spawn

### 6.2 Cron 漏触发(系统休眠)

**问题**:用户合盖丢 12 小时 cron tick = 常见;不补 = workflow 用户感受"经常没跑"。

**V1 答案**:
- 每 cron trigger 持久化 `last_fired_at`(DB 行)
- boot 时按当前时间 + cron expression 算应该 fire 的次数 N(自上次 last_fired_at)
- 默认 `missedPolicy: runOnce` — 漏 N 次只补 1 次(避免触发风暴)
- `skip` / `runAll` 留 V1.5 配置选项

### 6.3 同 workflow 多 run 并发

**问题**:cron 每小时一次,上次没跑完下次又起 → 数据竞争 / 重复处理。

**V1 答案**:
- workflow 实体加 `concurrency` 字段,默认 `serial`(等上次完才跑下次)
- scheduler StartRun 时检查 (workflow_id, status='running') 行数 ≥ 1 时按 policy:
  - `serial`:跳过本次(默认)
  - `parallel(N)`:允许最多 N 个并发
- V1 默认 serial,因大多数自动化 workflow 幂等性差

### 6.4 失败 run 通知

**问题**:后台 cron workflow 跑挂,用户在 chat 外不知道。

**V1 答案**:
- run 终态 `failed / cancelled` 时 publish `notifications.flowrun`(V1 无 run-level timeout state)
  - `flowrun.action=failed`
  - `flowrun.action=cancelled`
- 现有 notifications 系统直接用,前端 toast / 桌面 native notification

### 6.5 Workflow `enabled` 开关

**问题**:用户想"先停一下这个 workflow,不要删"(临时维护 / 调试)。

**V1 答案**:
- workflow 实体加 `enabled bool` 字段(default true)
- disabled 时:trigger service unregister listener;手动 trigger 返 `WORKFLOW_DISABLED` 422
- HTTP `PATCH /api/v1/workflows/{id}` 改 enabled

### 6.6 Webhook secret 防滥用

**问题**:webhook URL 一旦被泄露,任何人能 POST 触发 workflow。

**V1 答案**:
- webhook trigger config 加 `secret` 字段(可空,空=不校验)
- 非空时 POST 须带 `X-Webhook-Secret` header 匹配 OR URL `?token=<random>`
- 注册时 secret 走 `{{ env.X }}` 表达式拿(避免明文存 graph)

### 6.7 FlowRun 保留策略

**问题**:每小时一次 cron × 365 天 = 8760 行/年;表会大;debug 也只看最近的。

**V1 答案**:
- 默认每 workflow 保留最近 **N=200** 条 FlowRun(超过 hard delete 最旧)
- 每次 finalizeRun 后异步检查并 prune
- 配置 `Workflow.RetentionLimit` 可改(V1.5)

### 6.8 节点级 timeout 默认 + override

**问题**:LLM 节点跑慢 / 第三方 API 卡死;没超时保护 run 挂死永远。

**V1 答案**:
- 每 capability 节点 config 加 `timeout` 字段(unit ms)
- 类型默认:function 30s / handler 30s / mcp 30s / skill 60s / llm 60s / http 30s
- 超时按 `onError` 策略走

### 6.9 Approval 节点 timeout

**问题**:approval 不能永远等(用户可能永远不点 / 走丢)。

**V1 答案**:
- approval 节点 config 加 `timeout`(默认 7d 即 7 × 86400 × 1000 ms)
- + `onTimeout: stop / continue / branch`(默认 stop)
- timeout 触发后走 `onTimeout` 路径

### 6.10 Cron 时区明确

**问题**:"0 0 * * *" 是几点?UTC 还是本地?差几小时是 production-grade bug。

**V1 答案**:
- 锁本地时区(`time.Local`)
- robfig/cron 构造时传 `cron.WithLocation(time.Local)`
- 桌面 app 跟用户笔记本时区一致是 sane default

### 6.11 Fsnotify 路径不存在 fail-soft

**问题**:用户配的 path 错或被删 → listener 注册失败 → 静默不 fire = 隐性 bug。

**V1 答案**:
- 启动时校验路径存在 + 可读
- 不在 → trigger 标 `state=error` + workflow 标 `needs_attention`(不阻塞 workflow 本身存在)
- 通过 notifications 告知用户

### 6.12 Trigger 状态可见

**问题**:用户必须能看 "我的 cron 上次跑啥时候 / 下次几点跑"— 不然没法 debug。

**V1 答案**:
- HTTP `GET /api/v1/workflows/{id}/triggers` 返每个 trigger 的:
  - `lastFiredAt`(所有 kind)
  - `nextFireAt`(仅 cron)
  - `state`(active / idle / error)
  - `lastError`(error 时)

### 6.13 Trigger panic recover

**问题**:某 listener 崩溃不应该让其他 trigger 也死。

**V1 答案**:
- 每个 listener goroutine 包 `defer recover()`
- panic 时 log + 标 trigger state=error + 通知用户
- 一个 listener 死不影响其他;系统级 trigger restart 后重建 listener registry

### 6.14 Run cancellation cleanup

**问题**:用户取消 run 时,handler instance / sandbox subprocess 必须被 cleanup,否则 leak。

**V1 答案**:
- `Scheduler.Cancel(runId)` 内部 ctx.Cancel() 一路串
- defer cleanup 函数销毁 run 拥有的 handler instance
- 现有 sandbox v2 三层 leak 防御(A/B/C)处理 subprocess

---

## 7. HTTP API

```
POST   /api/v1/workflows/{id}:trigger              手动触发(scheduler 起 run,返 runId)
GET    /api/v1/flowruns                            列表(过滤 workflow_id / status / 时间)
GET    /api/v1/flowruns/{id}                       详情
GET    /api/v1/flowruns/{id}/nodes                 node 执行记录
DELETE /api/v1/flowruns/{id}                       取消(running/paused 时)

POST   /api/v1/flowruns/{id}/approvals/{nodeId}    approval 节点签收
                                                    body: {decision, reason?}

GET    /api/v1/eventlog?scope=flowrun:fr_xxx       流式订阅(eventlog 泛化后)

POST   /api/v1/webhooks/{wfId}/{path}              webhook 入口(动态注册)
                                                    返 202 + runId

GET    /api/v1/workflows/{id}/triggers             看 trigger 状态(§6.12)
```

LLM tool 等价物:
- `cancel_flowrun({runId})` — V1 加(power-user 路径)

---

## 8. 错误码

| Code | HTTP | Sentinel | 触发 |
|---|---|---|---|
| `FLOWRUN_NOT_FOUND` | 404 | `flowrun.ErrNotFound` | id 查不到 |
| `FLOWRUN_NOT_CANCELLABLE` | 422 | `flowrun.ErrNotCancellable` | 已终态,不能 cancel |
| `FLOWRUN_NOT_PAUSED` | 422 | `flowrun.ErrNotPaused` | approval 时 run 不在 paused |
| `APPROVAL_NODE_NOT_FOUND` | 404 | `flowrun.ErrApprovalNodeNotFound` | nodeId 不在 paused state |
| `APPROVAL_DECISION_INVALID` | 400 | `flowrun.ErrApprovalDecisionInvalid` | decision 不是 approved/rejected |
| `WORKFLOW_DISABLED` | 422 | `workflow.ErrDisabled` | enabled=false 时 trigger |
| `SCHEDULER_CONCURRENCY_LIMIT` | 429 | `scheduler.ErrConcurrencyLimit` | serial mode 已有 running |
| `WEBHOOK_SECRET_MISMATCH` | 401 | `trigger.ErrWebhookSecretMismatch` | secret 校验失败 |
| `TRIGGER_PATH_NOT_EXIST` | 422 | `trigger.ErrPathNotExist` | fsnotify path 不存在 |
| `TRIGGER_PATH_CONFLICT` | 409 | `trigger.ErrPathConflict` | webhook path 冲突 |

---

## 9. 测试覆盖(V1 目标)

| 测试套件 | 覆盖点 |
|---|---|
| `app/scheduler/dispatch_test.go` | 13 dispatcher 各自 mock + 串联 |
| `app/scheduler/state_machine_test.go` | running / paused / cancelled / failed 转换 |
| `app/scheduler/concurrency_test.go` | serial / parallel 并发控制 |
| `app/scheduler/rehydrate_test.go` | paused state persist + boot 时 rehydrate |
| `app/trigger/cron_test.go` | cron 注册 / 漏触发 / runOnce 补 |
| `app/trigger/fsnotify_test.go` | watcher 注册 / 路径不存在 fail-soft |
| `app/trigger/webhook_test.go` | secret 校验 / path 冲突 |
| `app/flowrun/store_test.go` | CRUD + retention prune |
| `test/scheduler/end_to_end_test.go` | E2E:cron → run → multiple nodes → notify |
| `test/scheduler/approval_test.go` | E2E:approval node 暂停 + HTTP 唤醒 |
| `test/scheduler/cancellation_test.go` | E2E:run 中途 cancel → cleanup |

---

## 10. 实现清单

### Trigger domain(~2000 LOC)

1. `domain/trigger/{trigger.go, types.go}` — TriggerSpec 通用类型 + 4 种 kind 各自 sentinel
2. `infra/trigger/cron/cron.go` — robfig/cron 包装
3. `infra/trigger/fsnotify/fsnotify.go` — fsnotify v1.9 包装
4. `infra/trigger/webhook/webhook.go` — net/http 注册到 router
5. `app/trigger/service.go` — 统一 Service 管 listener 生命周期
6. 装配到 main.go(boot 时 scan + register)

### Scheduler domain(~2500 LOC)

1. `domain/scheduler/scheduler.go` — interface 定义
2. `app/scheduler/scheduler.go` Service{StartRun, Cancel, Resume, Pause}
3. `app/scheduler/dispatch.go` — 13 nodeDispatcher 各自实现
4. `app/scheduler/state.go` — execCtx 构造 / persist / rehydrate
5. `app/scheduler/retry.go` — retry 策略实现
6. `app/scheduler/error_policy.go` — onError 实现

### FlowRun domain(~1200 LOC)

1. `domain/flowrun/flowrun.go` + sentinels
2. `infra/store/flowrun/flowrun.go` + 集成测试
3. `app/flowrun/service.go` Service{Create, Update, Get, List, Cancel, Approve}
4. `transport/httpapi/handlers/flowrun.go` + httptest

### Eventlog 泛化(~500 LOC)

1. `domain/eventlog/scope.go` — Scope struct
2. `infra/eventlog/bridge.go` — Bridge map key 改 Scope
3. `transport/httpapi/handlers/eventlog.go` — 加 `?scope=` 参,保留 `?conversationId=` 兼容

### 装配 / 文档同步

1. main.go / harness.go 装 scheduler / trigger / flowrun service
2. service-design-documents/scheduler.md / trigger.md / flowrun.md 各一份
3. service-contract-documents/api-design.md / database-design.md / error-codes.md / events-design.md 同步
4. progress-record.md + backend-design.md 收尾

---

## 11. 主要风险

| 风险 | 缓解 |
|---|---|
| Scheduler goroutine leak(panic 不 recover) | 全 goroutine 包 defer recover + log |
| FlowRun 状态不一致(running 但实际死了) | boot 时扫 status='running' + paused_state=null → 标 failed(orphan run cleanup) |
| 长跑 paused workflow 占库 | retention 策略默认 200 条;7d approval timeout 限制最长等待 |
| Webhook 路径冲突 | 注册时校验唯一,冲突拒;listener 启动顺序固定 |
| 多 run 并发改 same Handler instance | caller-owns 模型保证 instance per-run 隔离;不会跨 run 共享 |
| Cron 表达式恶意配置(每秒一次) | V1 不限,假定单用户自我节制;V1.5 加表达式频率守卫 |
| Approval 等了 7 天用户回来,workflow 已删 | resume 时校验 workflow / version 仍存在,已删则 mark failed + 通知 |

---

## 12. Transport Layer — HTTP/2 + TLS(D18,V1 必做)

### 12.1 为啥 HTTP/2

HTTP/1.1 浏览器 6-connection 限制(单 origin)是历史遗留,**项目 2026-05-09 testend 已经撞过一次**(6 个 SSE 占满,所有 fetch 静默卡死)。本 spec 设计加了 entity-level eventlog scope(D19),前端单页可能订:
```
conversation:cv_xxx + workflow:wf_xxx + function:fn_yyy + handler:hd_zzz + flowrun:fr_aaa + notifications = 6 SSE
```

加 dev mode `/dev/logs` / `/dev/errors` 直接撞死。**此问题必须当一等公民处理,不能拖到 V2**。

### 12.2 协议层

Go 标准库 `net/http` **原生支持 HTTP/2**,只要走 TLS(`ListenAndServeTLS`)自动协议协商上 h2。**0 第三方依赖**。

> **注**:HTTP/2 cleartext (h2c) 协议层 OK,但**浏览器只支持基于 TLS 的 HTTP/2**(Chrome / Firefox / Safari 都不接 h2c)。所以 **HTTP/2 必须走 TLS**。

### 12.3 本地 dev TLS — `mkcert`

`mkcert`(by FiloSottile)是工业标准本地自签工具,支持 mac / linux / windows。

**Bootstrap 流程**(对齐 sandbox v2 mise 模式):

```
首次启动 cmd/server:
  1. 检查 ~/.forgify/.tls/cert.pem 是否存在 + 有效
  2. 不存在 → 检查 mkcert binary 是否在 PATH
  3. mkcert 不在 → 引导用户(macOS: brew install mkcert / Linux: package mgr / Windows: scoop / 等)
  4. 跑 `mkcert -install`(装 root CA;sudo / admin once,系统级别信任)
  5. 跑 `mkcert -cert-file ~/.forgify/.tls/cert.pem -key-file ~/.forgify/.tls/key.pem localhost 127.0.0.1`
  6. main.go 用这对 cert 启 HTTPS server,自动 h2
```

### 12.4 启动 flag

```
cmd/server:
  --tls-cert <path>   TLS cert (默认 ~/.forgify/.tls/cert.pem)
  --tls-key  <path>   TLS key  (默认 ~/.forgify/.tls/key.pem)
  --http              强制 HTTP/1.1 cleartext 模式(向后兼容旧 testend)
  --port <int>        默认 0 = 随机
```

dev 默认 HTTPS;真撞问题用 `--http` 临时回退。

### 12.5 testend 兼容

testend 现有 `http://localhost:port` → 切到 `https://localhost:port`。第一次访问时(若 mkcert -install 已跑过)浏览器免警告。

### 12.6 Wails 桌面分发

V1 走 mkcert auto-setup(用户首装 Forgify 时一次性装 root CA,后续无感知)。Wails 期看反馈再考虑 cert 内嵌 + signing(免装 root CA)。

### 12.7 实施成本

| 改动 | LOC |
|---|---|
| `cmd/server/main.go` HTTPS 启动 + flag 解析 | ~50 |
| `cmd/resources` 扩展 mkcert auto-setup | ~150 |
| `~/.forgify/.tls/` 路径 + cert 校验 / 自动 renew | ~80 |
| testend URL 协议切换 + 文档 | ~30 |
| `--http` fallback flag | ~20 |
| (Wails 期 cert 嵌入)| 留实施期 |

总 ~330 行 backend + setup 工具。**比"前端共享 SSE 单连接 workaround"实施成本相当**,换来**彻底无 connection 限制 + 协议层面更现代**。

### 12.8 风险与回退

| 风险 | 缓解 |
|---|---|
| mkcert 装不上(用户没 sudo / 公司限制) | bootstrap fail-soft,退回 HTTP/1.1,提示用户;dev mode 仍可用 |
| Cert 过期(mkcert 默认 5 年) | bootstrap 时校验,过期自动 regen |
| 跨设备 cert 不可移植 | 每台设备首次启动重 mkcert(machine fingerprint 也是 per-device) |
| 用户禁用 root CA 信任 | 浏览器警告,用户手动信任或回退 HTTP |

---

(本文档完)
