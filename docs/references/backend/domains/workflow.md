---
id: DOC-128
type: reference
status: active
owner: @weilin
created: 2026-04-22
reviewed: 2026-05-31
review-due: 2026-06-30
audience: [human, ai]
---
# Workflow

> Trinity-domain DAG authoring surface, the Plan 04 deliverable of the forge_redesign topic. The third leg of the trinity (Function = stateless code / Handler = stateful class / **Workflow = orchestration**).

**Code 位置**:`backend/internal/{domain,app,infra/store,transport/httpapi/handlers}/workflow/` + `backend/internal/app/tool/workflow/`

**联动文档**:
- 完整设计 spec:[`archive/forge-redesign-2026-05/04-workflow.md`](../archive/forge-redesign-2026-05/04-workflow.md)
- 跨域决策(D1-D22 + D-redo-1..23):[`archive/forge-redesign-2026-05/00-overview.md`](../archive/forge-redesign-2026-05/00-overview.md)
- 实施计划:[`archive/forge-redesign-2026-05/plans/04-workflow-authoring.md`](../archive/forge-redesign-2026-05/plans/04-workflow-authoring.md)
- 后续:[`archive/forge-redesign-2026-05/plans/05-execution-plane.md`](../archive/forge-redesign-2026-05/plans/05-execution-plane.md)(scheduler / trigger / flowrun)

---

## 1. 定位

Workflow 是 trinity 第三条腿 — **用户命名的有向无环图(DAG)**,描述"trigger 触发后这些节点按这个顺序跑"。一个 Workflow = `{nodes, edges, variables, meta}` 的封装,以 frozen graph JSON 存在 version 行。

**锻造 vs 执行分离**(D6):本 plan 只管图怎么样(workflow domain),不管图怎么跑。执行 plane(scheduler / trigger / flowrun)在 Plan 05。

跟 Function / Handler 区别:
- Function = 一段无状态 Python def(每次调用 fresh subprocess)
- Handler = 有状态 Python class(`__init__ + N methods + shutdown`,instance 持续存活)
- Workflow = **编排器**,不持有自己代码;节点引用其他 capability(Function/Handler/MCP/Skill/LLM/HTTP)+ 内置控制流(condition/loop/parallel/approval/wait/variable)

形态:
- HTTP **direct ops definition**(POST `/workflows` 收 `{ops, changeReason}`,curl / UI / 未来 Workflow Builder 用)
- LLM ops stream(`create_workflow` / `edit_workflow` 工具,流式锻造)
- **校验前置**:Create / Edit 都在 ApplyOps 后跑 strict `ValidateGraph`(cycle / 引用 / trigger 存在性)
- **iterate-same-pending**(D-redo-11):Edit 重写同 ID pending 行,不创建新行
- 跟 function / handler 区别:**无 env 装配**(workflow 不持代码,无 sandbox 调用),Create / Edit 路径不走 fix loop

---

## 2. 锻造模型

### 2.1 创建

`POST /api/v1/workflows` body `{ops, changeReason}` → `workflowapp.ParseOps` 解析 → `Service.Create`:
1. `ApplyOps(ctx, base=nil, ops, progressBlockID)` 把 ops 应用到空图
2. 校验 `graph.Name` 非空 + 字符集白名单(`^[a-zA-Z][a-zA-Z0-9_\-]{0,63}$`)
3. `ValidateGraph(ctx, graph, checker)` — 必有 ≥1 trigger 节点 + 无 cycle + 节点引用都解析(D-redo-23 Scope nesting + container body recursive ≤3)+ variable refs
4. Dup-check `GetWorkflowByName`
5. 持久化 Workflow + auto-accepted v1 + ActiveVersionID=v1.ID
6. publish `workflow` notification `{action:"created", versionId, versionNumber:1}`

**Workflow 没 env 装配**,因此 Create 不前置 sandbox ping(对比 function/handler 的 D-redo-20)— validation 错误直接返,无 fix loop。

### 2.2 编辑 — iterate same pending(D-redo-11)

`Service.Edit(EditInput{ID, Ops, ChangeReason, ProgressBlockID})`:
- 拒绝 `ops=[]`(workflow 无 env 要"force-rebuild",ops=[] 在 workflow 域无意义,返 `ErrOpInvalid`)
- **无 pending** → 在 active 之上 ApplyOps → 持久化为新 pending 行
- **有 pending** → 在 pending 之上 ApplyOps → **重写同 ID pending 行**
- 全 strict 校验(Edit 期失败比 Reject 后再失败更好 UX)
- publish `pending_created` notification `{versionId}`

`ErrPendingConflict` 已移除 — Edit 不返冲突。

### 2.3 接受 / 拒绝 / 回滚

- `AcceptPending` — pending → 带号 accepted(`nextVersionNumber = max(accepted.version)+1`),`SetActiveVersion`,清 NeedsAttention,trim 到 `AcceptedVersionCap=50`;publish `version_accepted`
- `RejectPending`(D-redo-12)— **HardDeleteVersion pending 行**(不留 rejected status);publish `pending_rejected`
- `Revert(id, targetVersion)` — `GetVersionByNumber` → `SetActiveVersion`;publish `reverted`
- `Delete(id)` — soft delete(`deleted_at` 写时间)+ publish `deleted`

---

## 3. Graph 形状

```
type Graph struct {
    Name        string         `json:"name"`
    Description string         `json:"description,omitempty"`
    Tags        []string       `json:"tags,omitempty"`
    Variables   []VariableSpec `json:"variables,omitempty"`
    Nodes       []NodeSpec     `json:"nodes"`
    Edges       []EdgeSpec     `json:"edges"`
}

type NodeSpec struct {
    NodeID        string                 `json:"nodeId"`
    Type          string                 `json:"type"`
    Config        map[string]any         `json:"config,omitempty"`
    Position      *Position              `json:"position,omitempty"`
    ModelOverride *modeldomain.ModelRef  `json:"modelOverride,omitempty"`  // 2026-05-28 model selection redesign;仅 agent/llm 节点生效;{apiKeyId, modelId}
}

type EdgeSpec struct {
    ID       string `json:"id"`                  // 系统生成 edge_N
    From     string `json:"from"`                // 纯 node ID
    FromPort string `json:"fromPort,omitempty"`  // 分叉节点选出口;单输出节点必空
    To       string `json:"to"`                  // 纯 node ID
    ToPort   string `json:"toPort,omitempty"`    // 预留 V1.5,V1 单输入,留空
}
```

**`NodeSpec.ModelOverride` 字段**（2026-05-28 model selection redesign）：仅对 `agent` / `llm` 节点生效；其他 11 种节点类型若设了 modelOverride，validate 阶段 warn log 不报错（容错优于硬抓）。F1 校验在 `set_node_model_override` op + `validate.go` 双层走：apiKeyId 与 modelId 都必填 → `ErrInvalidNodeModelOverride`（400）；apiKeyId 不存在 / 跨用户 → `apikey.ErrNotFound`（404）。dispatcher（`dispatch_agent.go` / `dispatch_llm.go`）走 `llmclient.ResolveAgentWithOverride(ctx, node.ModelOverride, picker, keys, factory)` — override-first，否则 fallback `PickForAgent`。

整图作为 JSON 整存 `workflow_versions.graph` TEXT 列;Service 层 `attachGraph` 在 GET 时解为 `*Graph` 填到 `Version.GraphParsed`(`gorm:"-"`)。

### 3.1 Edge port 路由(2026-05 重构)

**From/To 是纯 node ID,port 信息走独立字段**(不再用 `"<node>.<port>"` 字符串编码,跟 n8n / NiFi / Step Functions 等成熟工具对齐)。

分叉节点(emit multi-port at runtime)及其合法 FromPort 值:

| Node type | FromPort 必填 | 合法值 |
|---|---|---|
| `approval` | ✓ | `"yes"` / `"no"`（17 §7 canon；`approved`/`rejected` 是 `approvals.status` / API decision 值，**非端口**）|
| `loop` | ✓ | `"iterate"` / `"done"` |
| `condition` | ✓ | 节点 config.cases 里声明的 case 名 |

**单输出节点**(trigger / function / handler / mcp / skill / llm / http / wait / variable / parallel):FromPort **必须为空**。

ValidateGraph 在 Service.Create / Edit 时强制以上规则,违反返 `ErrOpInvalid`。Scheduler `topo.advance` 按 dispatcher 选的 `NextPort` 跟 edge.FromPort 精确匹配选下游,不匹配的边 park(in-degree 减但不 enqueue)。

> **历史**:V1 早期 EdgeSpec 用 `From: "<nodeId>.<port>"` 字符串编码 port,因 stringly-typed 隐含字段导致 LLM/手写 workflow 易踩坑(approval 后流程静默没跑通,run 假成功)。2026-05 重构为显式字段。Legacy 字符串格式在 validate 阶段被显式拒绝(`"...uses legacy dotted node ID..."`)。

---

## 4. 14 节点类型（§14.5b 加 agent）

| Type | 用途 | Config 关键字段 |
|---|---|---|
| `trigger` | 触发源(manual/schedule/webhook/event)| `triggerType` + per-type config |
| `function` | 调 Function | `functionId`, `input`(变量表达式)|
| `handler` | 调 Handler method | `handlerName`, `method`, `args` |
| `mcp` | 调 MCP tool | `serverName`, `tool`, `args` |
| `skill` | 触发 Skill 子 agent | `skillName`, `prompt` |
| `llm` | 直接 LLM 调用 | `scenario`, `prompt`, `outputVariable` |
| `http` | HTTP 请求 | `method`, `url`, `headers`, `body` |
| `condition` | if/else 分支 | `condition`(expression)+ body subgraph |
| `loop` | for-each + body 子图（§5.1 ✅ 2026-05-17）| `items` + `body: {nodes, edges}` + `parallel?: bool` + `concurrency?: int` (default cap 5) + `onError?: "stop"\|"continue"` |
| `parallel` | 并发 | body subgraph(多个起点)|
| `approval` | 等用户 approve | `prompt`, `timeout` |
| `wait` | sleep / 等事件 | `duration` 或 `event` |
| `variable` | 命名变量 | `name`, `value`(表达式)|

枚举封闭(`Const NodeType...`);校验时未知 type 返 `ErrOpInvalid`。

**故意没有 terminal/end 节点**:DAG 跑完所有节点(无 outgoing edge 的节点都是 leaf)自动结束。LLM 从 n8n/Zapier/StepFunctions 带习惯尝试加 `end`/`output`/`finish`/`terminate`/`stop`/`return`/`exit` 类型时,`validate.go::isPseudoTerminalType` 拦截并返**教学型错误**:`"workflows have no terminal node; the DAG ends implicitly when no edges remain"`(#11,2026-05)。`create_workflow` tool description 同步加 MINIMAL COMPLETE EXAMPLE + "DO NOT add 'end' node" 提示。

---

## 5. 10 个 Ops

```
set_meta / add_node / update_node / delete_node /
add_edge / update_edge / delete_edge /
set_variable / unset_variable /
set_node_model_override
```

`update_node` / `update_edge` 用 **JSON Merge Patch(RFC 7396)** — patch 值覆盖、nil 删除键。其他都是整字段覆盖或简单 add/delete。

`set_node_model_override` 设置 / 清除 `NodeSpec.ModelOverride`(§12.3 per-node 模型选择,与 conv override 同形状):payload 是 `{"nodeId":"...", "modelOverride":{"apiKeyId":"...", "modelId":"..."}}`,字段缺失或为 null = clear。F1 校验:`apiKeyId` 与 `modelId` 都必填,否则 `ErrInvalidNodeModelOverride`(400 `INVALID_NODE_MODEL_OVERRIDE`);若 Service 装了 `keyProvider`(通过 `SetKeyProvider(apikeyService)`),则按 id 校验存在 + 跨用户 → `apikey.ErrNotFound`(404 `API_KEY_NOT_FOUND`)。注:F1 sentinel 不被外层 `ErrOpInvalid` 吞掉,errmap 命中正确 HTTP 码。

LLM 发 `[{op:"set_meta",name:"...",...}, {op:"add_node",node:{...}}, ...]`,`workflowapp.ParseOps` 解码为 `[]Op{Type, Raw}`(Raw 是不透明 body,各 op handler 自取字段)。

错误映射:per-op apply 错误 → `ErrOpInvalid`(400 WORKFLOW_OP_INVALID);final 校验错误 → 对应 sentinel(`ErrDAGCycle` / `ErrNoTrigger` / `ErrCapabilityNotFound` / 等)。例外:`set_node_model_override` 的 F1 sentinel(`ErrInvalidNodeModelOverride` / `apikey.ErrNotFound`)bubble up 不再被 `ErrOpInvalid` 包装。

---

## 6. ValidateGraph(`app/workflow/validate.go`)

最终校验链:

1. **至少 1 个 trigger** — 否则 `ErrNoTrigger`
2. **DAG cycle 检测** — Kahn 拓扑排序(in-degree 算法);存 cycle → `ErrDAGCycle`
3. **节点 ID 唯一** — duplicate id → `ErrOpInvalid: "add_node: duplicate id"`(在 apply 阶段就抓)
4. **Edge 引用合法** — from/to 必须指向已知节点 ID;**禁用 legacy `"<node>.<port>"` 字符串编码**(报 `ErrOpInvalid`)
5. **Edge port 一致性**(2026-05 加):
   - 分叉节点(`approval` / `loop` / `condition`)出边必带 `FromPort`,且必须在 `BranchOutputPorts[type]` 里(condition 动态读 `config.cases`)
   - 单输出节点出边 `FromPort` 必须为空
   - 任一违反 → `ErrOpInvalid` + 清楚错误信息
6. **CapabilityChecker** — function/handler/mcp/skill 节点的引用必须在对应 service 找得到(`HasFunction(id)` / `HasHandler(name)` / `HasSkill(name)` / `HasMCPServer(name)`)
7. **Variable refs** — `{{ vars.NAME }}` 引用扫描;未定义变量 → `ErrInvalidReference`
8. **Container body subgraph 递归校验** — condition/loop/parallel 的 body 子图递归套同样的校验链,**depth ≤ 3** 防 runaway 嵌套

`CapabilityChecker` 是 interface;生产 `ProductionChecker` 装 function/handler/skill/mcp 四个 service(允许任一 nil 让对应 check 全过,测试用);单测默认 `NopChecker()`(全过)。

---

## 7. 表达式语言(`app/workflow/expression.go`)

- `Compile(tmpl string)` → `*Expression`
- `Expression.Execute(EvalContext) (string, error)`
- 使用 Go `text/template`,~140 行 thin wrapper
- `EvalContext` 包含 `Vars` / `Trigger` / `Nodes`(节点输出按 ID 索引)/ `Loop` / `Run` / `Env`(白名单 USER/HOME/LANG/TZ/HOSTNAME)
- funcMap 空(V1);未来加 `len` / `upper` / `lower` 等

只在节点 config 字符串模板里使用(`{{ vars.NAME }}` / `{{ nodes.fn1.output.field }}`)。**不是图灵完备**;不需要 sandboxing。

---

## 8. 持久化 — 2 张本域表

### 8.1 `workflows`(主键 `wf_<16hex>`)

软删 / 用户作用域 / partial UNIQUE `(user_id, name) WHERE deleted_at IS NULL`(schema_extras `idx_workflows_user_name_active`)。

字段:
- `name` / `description` / `tags`(JSON 数组)
- `enabled` / `concurrency`(V1 只支持 `"serial"`;`"parallel(N)"` V1.5)
- `needs_attention` / `attention_reason`(D20 capability 删除时由 Plan 05 listener 写)
- `active_version_id`
- 标准时间戳 + `deleted_at`

GORM tag 注释:`Enabled` / `Concurrency` / `NeedsAttention` **故意不写 `default:` tag** — GORM 在 Save 时会用 column 默认值填零值字段,把显式 `Enabled=false` / `NeedsAttention=false` 静默覆盖。Service 层显式赋值。

**计算字段**(`gorm:"-"`):`Pending *Version` / `LiveRuns int` / `LastFiredAt *time.Time` / `NextFireAt *time.Time`(后三个是 Plan 05 territory,响应形状预留)。`attachComputed` 在 GET 时填 Pending。

### 8.2 `workflow_versions`(主键 `wfv_<16hex>`)

`status` 约束 `IN ('pending','accepted','rejected')`,pending/rejected 时 `version` 为 NULL。字段:`workflow_id` 索引 / `version` INT / `status` / `graph` TEXT(整图 JSON)/ `change_reason` / 时间戳。

AcceptedVersionCap=50/workflow,超限 HardDeleteOldestAccepted(同 function/handler 模式)。RejectPending 不留 rejected 行 — 直接 HardDeleteVersion(D-redo-12)。

---

## 9. HTTP API(13 端点)

| Method | Path | 用途 |
|---|---|---|
| POST   | `/api/v1/workflows`                              | 创建(ops 流;校验通过 auto-accept v1)|
| GET    | `/api/v1/workflows`                              | 列表(`?enabled=true` 过滤)|
| GET    | `/api/v1/workflows/{id}`                         | 详情(含 pending + 计算字段)|
| PATCH  | `/api/v1/workflows/{id}`                         | 改 meta(name/description/tags/enabled/concurrency/needs_attention/attention_reason)|
| DELETE | `/api/v1/workflows/{id}`                         | 软删 |
| POST   | `/api/v1/workflows/{id}:revert`                  | 回滚到 accepted 版本号 |
| POST   | `/api/v1/workflows/{id}:iterate`                 | 起 AI 编辑对话（askai.Spawner）；返 `{conversationId}` |
| POST   | `/api/v1/workflows/{id}:capability-check`        | 预检 active version（ValidateGraph）；返 `{ok, issues}` |
| GET    | `/api/v1/workflows/{id}/versions`                | 版本分页(`?status=`)|
| GET    | `/api/v1/workflows/{id}/versions/{v}`            | 单版本(int → ByNumber, wfv_* → ById)|
| GET    | `/api/v1/workflows/{id}/pending`                 | 当前 pending |
| POST   | `/api/v1/workflows/{id}/pending:accept`          | accept(纯指针)|
| POST   | `/api/v1/workflows/{id}/pending:reject`          | reject(hard-delete pending 行,D-redo-12)|

**Plan 05 领域**(本 plan 不实现):`:trigger` action / flowrun list / flowrun detail / cancel — 这些是执行 plane 的 HTTP 面。

### :iterate — AI 编辑对话

`POST /api/v1/workflows/{id}:iterate` body `{"userRequest": "..."}` → `askai.Spawner.Spawn(BuildWorkflowContext + userRequest)` → 返 `{data: {conversationId}}` (201)。

spawner 为 nil 时返 503 `FEATURE_UNAVAILABLE`。响应后前端跳转到该 conversationId 的对话界面，AI 持有 workflow 完整状态 + LLM 编辑工具。

### :capability-check — 能力预检

`POST /api/v1/workflows/{id}:capability-check`（无 body）→ `workflowapp.Service.CapabilityCheck(ctx, id)` → 返 `{data: {ok: bool, issues: [...]}}` (200)。

永远 200（即使 `ok=false`）；错误分两种：
- `ErrNotFound` / `ErrNoActiveVersion` → 正常 4xx
- graph 引用残缺（function/handler/mcp/skill 找不到）→ 200 + `ok=false, issues=[...]`

```go
type CapabilityReport struct {
    OK     bool              `json:"ok"`
    Issues []CapabilityIssue `json:"issues"`
}
type CapabilityIssue struct {
    NodeID string `json:"nodeId"`
    Kind   string `json:"kind"`    // "function" | "handler" | "mcp" | "skill"
    Ref    string `json:"ref"`     // id or name that wasn't found
    Reason string `json:"reason"`
}
```

Create 用 `json.RawMessage` 接 ops 而非 `[]workflowapp.Op` — 顶层 envelope 走 `DisallowUnknownFields`,而每 op 内部 raw body 由 ParseOps 不强制全字段已知(per-op handler 自取字段)。

---

## 10. LLM 工具(6 锻造 + 3 执行 plane = 9)

| 工具 | 用途 |
|---|---|
| `search_workflow` | LLM 排序 query → 相关 workflow id+score(case-insensitive substring + name/description/tags)|
| `get_workflow` | 完整详情含 pending + active version `GraphParsed` |
| `create_workflow` | 流式 ops 创建;校验失败抛错(workflow 无 env-fix loop);auto-accept v1 |
| `edit_workflow` | 流式 ops 编辑;pending 已存在则 iterate same pending(D-redo-11);拒绝 `ops=[]`(无 env 要重建)|
| `revert_workflow` | 翻 active 版本指针 + forge 双写 |
| `delete_workflow` | 软删 + forge 双写 |

执行 plane 工具(**已交付**):`trigger_workflow`(启动一次运行,`dryRun` 支持 — `WorkflowTriggerTool` 接 scheduler.StartRunWithOptions)/ `search_workflow_executions` / `get_workflow_execution`(查 flowrun_nodes — `WorkflowExecutionTools` 接 flowrun repo)。这 3 个在 scheduler 构造后注册,**不进 subagent 工具集**(D21 — 只编排者触发)。

---

## 11. Notifications + Forge 双写

每个 Service 突变都 publish `workflow` entity notification(slim payload per D-redo-6):
- `created` `{versionId, versionNumber}`
- `updated` `nil`
- `deleted` `nil`
- `pending_created` `{versionId}`
- `pending_rejected` `{versionId}`
- `version_accepted` `{versionId, versionNumber}`
- `reverted` `{versionId, versionNumber}`

LLM 工具额外双写 forge SSE(D-redo-4 trinity-forging 进度流):`forge_started` + `forge_completed`。`scope = {Kind: KindWorkflow, ID: wfID}`。

---

## 12. CapabilityChecker(`app/workflow/checker_production.go`)

生产期 `ProductionChecker` 装四个 service 引用:

```go
type ProductionChecker struct {
    Function *functionapp.Service
    Handler  *handlerapp.Service
    Skill    *skillapp.Service
    MCP      *mcpapp.Service
}
```

任一 nil → 对应 `HasX` 返 `true, nil`(测试 fallback)。每方法用 sentinel `errors.Is(err, ...NotFound)` 判存在性,其他错误透传。

`main.go` / `harness.go` 装配顺序:checker struct 先建(填 function + handler),Skill/MCP 服务在下方构造完成后再回填(closure persists)。

---

## 13. 错误码(11 sentinels)

详见 [`../references/backend/error-codes.md`](../references/backend/error-codes.md)。

| Sentinel | Status | Wire code |
|---|---|---|
| `ErrNotFound` | 404 | `WORKFLOW_NOT_FOUND` |
| `ErrDuplicateName` | 409 | `WORKFLOW_NAME_DUPLICATE` |
| `ErrVersionNotFound` | 404 | `WORKFLOW_VERSION_NOT_FOUND` |
| `ErrPendingNotFound` | 404 | `WORKFLOW_PENDING_NOT_FOUND` |
| `ErrNoActiveVersion` | 422 | `WORKFLOW_NO_ACTIVE_VERSION` |
| `ErrDAGCycle` | 422 | `WORKFLOW_DAG_CYCLE` |
| `ErrInvalidReference` | 422 | `WORKFLOW_INVALID_REFERENCE` |
| `ErrNoTrigger` | 422 | `WORKFLOW_NO_TRIGGER` |
| `ErrOpInvalid` | 400 | `WORKFLOW_OP_INVALID` |
| `ErrCapabilityNotFound` | 422 | `WORKFLOW_CAPABILITY_NOT_FOUND` |
| `ErrMCPServerNotInstalled` | 422 | `WORKFLOW_MCP_SERVER_NOT_INSTALLED` |

**故意不含** `ErrPendingConflict` — Edit 走 iterate-same-pending(D-redo-11),pending 不冲突。

---

## 14. 测试覆盖

- 6 domain 单测(sentinel + Workflow/Version + Op/NodeType 常量 + Graph JSON 形状)
- 12 store 集成测试(in-memory SQLite — Workflow CRUD + cross-user isolation + soft-delete UNIQUE reuse + Version pending/accept/cap-trim flow)
- 12 apply.go 单测(每 op 至少一例;cloneGraph 深拷;mergePatch null=remove)
- 12 validate.go 单测(no-trigger / cycle / dangling edge / capability fail / variable undefined / container depth ≤ 3)
- 11 expression.go 单测(简单 / vars / nodes / loop / env whitelist)
- 13 httptest(`transport/httpapi/handlers/workflow_test.go`):Create/List/Get/UpdateMeta/Delete/Versions/Pending/Revert/unknown-action/dup-name/no-trigger/bad-json
- 3 pipeline 测试(`test/workflow/workflow_test.go`):HTTP CRUDLifecycle / VersionsAndPending(含真实 CapabilityChecker 验失败路径)/ LLM SearchEmpty

---

## 15. Plan 05 接口预留

`WorkflowReader` interface(`app/workflow/workflow.go`):

```go
type WorkflowReader interface {
    GetActiveVersion(ctx context.Context, workflowID string) (*Version, error)
    GetWorkflow(ctx context.Context, workflowID string) (*Workflow, error)
    ListEnabled(ctx context.Context) ([]*Workflow, error)
}
var _ WorkflowReader = (*Service)(nil)
```

Plan 05 的 trigger / flowrun 域消费 `WorkflowReader`,不依赖完整 Service。这样执行 plane 可独立单测。

`Workflow.NeedsAttention` + `AttentionReason` 字段已 schema 化,Plan 05 listener(function/handler 删除事件)写值,UI 看 enabled=false 同步停 firing。

---

## 16. 历史

- 2026-05-12 forge_redesign Plan 04 完成:domain + store + ops engine + validate(Kahn cycle / capability check / container recursive ≤3)+ expression(Go text/template ~140 LOC)+ Service(CRUD + iterate-same-pending + slim notifications + forge double-write)+ 6 LLM tools + 11 HTTP endpoints + ProductionChecker + main/harness 装配 + 3 pipeline test。9 commits 直推 main。
- Plan 05 接口预留(`WorkflowReader`)、`NeedsAttention` schema 已就位;触发器 / scheduler / flowrun 在 Plan 05 引入。

---

## Relations Integration（2026-05-19）

workflow 既是关系图节点也是**最多出向边**的实体（出 5 种 `workflow_uses_*` 边到 function/handler/mcp/skill/document）。

| 方法 | 触发的 relation 操作 |
|---|---|
| `Service.Create` | `SyncIncoming(workflow, id, [forged], ...)` 写 v1 forged 边 + `SyncOutgoing(workflow, id, [5种 uses], computeWorkflowOutgoingEdges(v1.GraphParsed))` 从 graph nodes 抽出所有引用 |
| `Service.AcceptPending` | 翻 ActiveVersionID 后同 Create 的两段；outgoing 从新 active version 重算 |
| `Service.Revert` | 同 AcceptPending（active 翻向老版本，outgoing 从老 graph 重算） |
| `Service.Delete` | `PurgeEntity("workflow", id)` 级联清边 |

**`computeWorkflowOutgoingEdges` 抽 5 种引用**（见 `app/workflow/relations.go`）：
- function 节点 → `workflow_uses_function`，attrs 含 `nodeIds`（list）+ optional `pinnedVersionId`
- handler 节点 → `workflow_uses_handler`，attrs 同上
- mcp 节点 → `workflow_uses_mcp`，attrs 含 `serverName`
- skill 节点 → `workflow_uses_skill`，attrs 含 `skillName`
- llm / agent 节点 `attached_documents` 列表 → `workflow_uses_document`，attrs 含 `includeSubtree`

workflow_versions 表加 `ForgedInConversationID *string` 列。详 [`./relation.md`](./relation.md) §7、§4.2 attrs 表。
