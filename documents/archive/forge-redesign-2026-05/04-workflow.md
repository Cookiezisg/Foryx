# Workflow — DAG + 触发器(authoring 那一面)

**关联**:
- [`00-overview.md`](./00-overview.md) — 顶层愿景
- [`01-shared-tool-interface.md`](./01-shared-tool-interface.md) — 工具接口形态(本域共用)
- [`05-execution-plane.md`](./05-execution-plane.md) — 执行那一面(scheduler / trigger / flowrun)

**本文档范围**:Workflow domain 自身 — DAG 数据结构 / 节点类型 / edge / 表达式语言 / ops / 校验 / 持久化 / LLM 工具 / HTTP API。**不包含**执行细节(scheduler / trigger / flowrun)— 那是话题 1.5 / [`05-execution-plane.md`](./05-execution-plane.md)。

---

## 1. 锻造模式 vs 执行模式 — 决策 D6

| 模式 | 什么时候 | 谁参与 | 图(graph)是 |
|---|---|---|---|
| **锻造模式** | 用户跟 LLM 聊创建 / 修改 workflow | 用户 + LLM(可能多 agent 并行) | 流式可变 — "呼啦呼啦地变" |
| **执行模式** | workflow 部署后,trigger 触发跑 | 触发器 + workflow runtime | 冻结 — 按版本快照确定执行 |

把"锻造可视化呼啦变"跟"执行时严肃确定" **分开** —— 前者是 UX 体验,后者是工程严肃性,两者复杂度不交叉。锻造时让 LLM / 多 agent / 用户随便造,造好"保存为版本"才进入执行域。

### Plane 分离:Workflow domain vs Execution plane

```
┌─ Authoring Plane(workflow domain — 本文档)─────┐
│  Workflow / WorkflowVersion                     │
│  锻造模式:LLM + 用户 + 多 agent 流式编辑         │
│  关心:图定义 / 版本 / 校验                      │
└─────────────┬────────────────────────────────────┘
              │ active version 引用
              ↓
┌─ Execution Plane(scheduler + trigger + flowrun)─┐
│  详见 05-execution-plane.md                      │
│  关心:执行 / 重试 / 状态 / 可观测性               │
└──────────────────────────────────────────────────┘
```

唯一的 plane 间通讯:**scheduler 读 active WorkflowVersion → 起 FlowRun**。

---

## 2. 节点类型清单(V1 = 13 种)

### 2.1 Entry(1 种)

| Type | 用途 | 主要 config |
|---|---|---|
| `trigger` | 入口 — workflow 怎么起跑 | `kind: cron \| fsnotify \| webhook \| manual` + 各 kind 自身 config |

**注**:trigger 节点的具体配置由 [`05-execution-plane.md`](./05-execution-plane.md) §2 定义,workflow domain 只校验 schema 合法性。

### 2.2 Capability invocation(6 种)

每节点接 `onError` + `retry` config(详见 §2.6)。

| Type | 用途 | 主要 config |
|---|---|---|
| `function` | 调一个 Function | `functionId`, 参数映射 |
| `handler` | 调 Handler instance 的某 method | `handlerName`, `method`, 参数映射 |
| `mcp` | 调外部 MCP server 的某 tool | `serverName`, `toolName`, 参数映射 |
| `skill` | 执行一个 Skill | `skillName`, 参数 |
| `llm` | LLM 调用(可挂 tools + 知识库) | `model`, `prompt`(模板), `tools[]?`, `knowledgeBaseId?`(Phase 5) |
| `http` | 通用 HTTP 调用 | `method`, `url`, `headers?`, `body?`(模板), `timeout?` |

### 2.3 Control flow(4 种)

| Type | 用途 | 主要 config | Output ports |
|---|---|---|---|
| `condition` | if/else 二分支 | `expression`(布尔表达式) | `true` / `false` |
| `loop` | for-each 循环 | `items: <expr>`, `body: SubGraph` | `output`(收集结果数组) |
| `parallel` | 扇出多分支并发 | `branches: SubGraph[]` | `output`(合并结果数组) |
| `approval` | 等人工 OK | `message`, `timeout`(默认 7d), `onTimeout` | `approved` / `rejected` / `timeout` |

`loop` / `parallel` 是**容器节点** — body / branches 是 **inline 子图**(`{nodes: [...], edges: [...]}`)。

### 2.4 State(2 种)

| Type | 用途 | 主要 config |
|---|---|---|
| `variable` | 读 / 写 workflow 级变量 | `op: get \| set`, `key`, `value`(set 时) |
| `wait` | 暂停 N 秒 / 等到某时间点 | `duration` 或 `until` |

### 2.5 节点通用字段

每个节点都有:

| 字段 | 说明 |
|---|---|
| `id` | graph 内唯一 |
| `type` | 13 种之一 |
| `position` | `{x: int, y: int}`,可选(LLM 默认 0,0,前端 auto-layout) |
| `config` | 节点类型特定的 JSON object |
| `notes?` | 用户自由备注,可选 |

### 2.6 Capability 节点的 onError + retry

每 capability 节点(function / handler / mcp / skill / llm / http)还可选挂:

```yaml
onError: stop      # 默认:整 run 停 marked failed
        | continue # 忽略,带 null 进下游
        | branch   # 走特殊 "error" output port,接错误处理子图

retry:
  maxAttempts: 3
  backoff: exponential | linear | fixed   # default exponential
  delay: 1s                                # 初始 delay,exponential 时翻倍
```

无该字段时:`onError=stop`,无 retry。

### 2.7 节点级 timeout(V1 必做)

每 capability 节点 config 加 `timeout` 字段 + 类型默认:
- `function`: 30s
- `handler`: 30s
- `mcp`: 30s(可被 ServerConfig override,详见 mcp.md §5.7)
- `skill`: 60s(LLM-driven,慢)
- `llm`: 60s
- `http`: 30s

超时按 onError 路径走(默认 stop)。详见 [`05-execution-plane.md`](./05-execution-plane.md) §6。

### 2.8 不在 V1 的节点(V1.5 / V2 候补)

| 候补 | 推迟原因 |
|---|---|
| `switch`(多路) | cascaded `condition` 表达足够 |
| `transform`(JSON 模板) | edge 表达式上做轻量映射够,复杂的用 function |
| `aggregator` / `map` / `filter` / `reduce` | 都 function 能做 |
| `subworkflow`(workflow 当节点) | V1 用复制粘贴(LLM 能做);V2 真做时另设计 |
| `event-bus`(workflow 间事件) | 引入耦合,V2 |

---

## 3. Op 集合(workflow-specific)

LLM 通过 ops 流式构建 / 修改 workflow。完整集合:

| Op | 字段 | 校验 |
|---|---|---|
| `set_meta` | `name?, description?, tags?` | name 非空(create) / partial UNIQUE |
| `add_node` | `node: NodeSpec` | id 唯一,type 在白名单,config schema 合法 |
| `update_node` | `id, patch: object` | node 存在,patch 后 config 仍合法。**`patch` 走 JSON Merge Patch(RFC 7396)** — 浅合并,字段 `null` 表示删除该字段;LLM 友好 |
| `delete_node` | `id` | node 存在;级联删该节点的所有 edge |
| `add_edge` | `edge: EdgeSpec` | from / to 节点存在,port 存在,无重复 edge |
| `update_edge` | `edgeId, patch` | edge 存在;`patch` 同走 JSON Merge Patch(RFC 7396) |
| `delete_edge` | `edgeId` | edge 存在 |
| `set_variable` | `name, type, default?` | name 非空,type 在 [`string`, `number`, `integer`, `boolean`, `object`, `array`] |
| `unset_variable` | `name` | variable 存在;校验图内无引用(否则 reject) |

### 3.1 NodeSpec

```json
{
  "id": "fetch_emails",
  "type": "handler",
  "position": {"x": 300, "y": 100},
  "config": {
    "handlerName": "gmail",
    "method": "list_unread",
    "params": {"since": "{{ vars.lastCheckTime }}"}
  },
  "retry": {"maxAttempts": 3, "backoff": "exponential"},
  "onError": "stop",
  "timeout": 30000,
  "notes": "Fetch unread emails since last check"
}
```

### 3.2 EdgeSpec

```json
{
  "id": "edge_1",
  "from": "trig.next",          // <nodeId>.<output_port>
  "to": "fetch_emails.input"    // <nodeId>.<input_port>
}
```

Edge id 由系统生成(`edge_<random>`),LLM add_edge 时不传 id(or 传时被 ignore)。

### 3.3 容器节点的 inline 子图

`loop` / `parallel` 节点的 config 内嵌子图:

```json
{
  "id": "loop1",
  "type": "loop",
  "config": {
    "items": "{{ in.emails }}",
    "body": {
      "nodes": [/* 子节点 */],
      "edges": [/* 子 edge */]
    }
  }
}
```

子图的 `add_node` / `add_edge` ops:LLM 通过 `update_node({id:"loop1", patch:{config:{body:{nodes:[...], edges:[...]}}}})` 做整体替换,**或者**通过子图特定 op (`add_node_in_body`, etc.) — V1 走前者(整体替换 patch,简单)。

---

## 4. Graph JSON shape(WorkflowVersion 持久化)

整个 WorkflowVersion 落到 `workflow_versions` 表的 `graph` 列(单 JSON):

```json
{
  "name": "email-headhunter-watcher",
  "description": "每小时扫邮箱,猎头线索写库 + WhatsApp 通知",
  "variables": [
    {"name": "lastCheckTime", "type": "string", "default": "1970-01-01T00:00:00Z"}
  ],
  "nodes": [
    {
      "id": "trig",
      "type": "trigger",
      "position": {"x": 100, "y": 100},
      "config": {"kind": "cron", "expression": "0 */1 * * *"}
    },
    {
      "id": "fetch_emails",
      "type": "handler",
      "position": {"x": 300, "y": 100},
      "config": {
        "handlerName": "gmail",
        "method": "list_unread",
        "params": {"since": "{{ vars.lastCheckTime }}"}
      },
      "retry": {"maxAttempts": 3, "backoff": "exponential"},
      "onError": "stop",
      "timeout": 30000
    },
    {
      "id": "loop1",
      "type": "loop",
      "position": {"x": 500, "y": 100},
      "config": {
        "items": "{{ in.emails }}",
        "body": {
          "nodes": [
            {"id": "is_recruiter", "type": "llm", ...},
            {"id": "filter_cond", "type": "condition", "config": {"expression": "{{ in.isRecruiter }}"}},
            {"id": "save_pg", "type": "handler", ...}
          ],
          "edges": [...]
        }
      }
    },
    {
      "id": "notify",
      "type": "http",
      "position": {"x": 700, "y": 100},
      "config": {
        "method": "POST",
        "url": "https://api.whatsapp.com/...",
        "headers": {"Content-Type": "application/json"},
        "body": "{{ in.processedSummary }}"
      }
    }
  ],
  "edges": [
    {"id": "e1", "from": "trig.next", "to": "fetch_emails.input"},
    {"id": "e2", "from": "fetch_emails.output", "to": "loop1.input"},
    {"id": "e3", "from": "loop1.output", "to": "notify.input"}
  ]
}
```

---

## 5. 边 = 数据管道

### 5.1 V1 行为

```
upstream node 跑完
   ↓ 产出 output(JSON)
   ↓ 顺 edge 流向下游
downstream node 拿到输入
   ↓ 在 config 里通过 {{ in.field }} 读
   ↓ 跑自己的逻辑
```

### 5.2 边的能力边界(V1)

✅ **能做**:
- 携带任意 JSON(整个 upstream output 直送)
- 多个 output port 各自有 edge(condition 的 true / false / capability 的 output / error)
- 同一节点的 output port 接多条 edge(扇出 — 多个下游都收到同一份数据)

❌ **V1 不做**:
- 边上 inline `transform` / `filter` 表达式 — **数据 shape 在节点 config 表达式里做**(消费方决定怎么读)
- 多 edge 汇入同一 input port — 用 `parallel` merge 或专门 transform 节点
- 边上条件 — 用 `condition` 节点表达(显式 + 可视化)

**原则**:边只是连线,**语义全在节点上**。视觉编辑器渲染时边不需要展开看里面藏了啥;LLM 锻造时也只需 add_edge,不写 transform 逻辑。

### 5.3 端口

每节点定义有哪些 input port + output port。大多数节点只 `input` + `output` 一对。特殊:

| Node Type | Input Ports | Output Ports |
|---|---|---|
| trigger | — | `next` |
| function | `input` | `output` + `error`(若 onError=branch) |
| handler | `input` | `output` + `error`(若 onError=branch) |
| mcp / skill / llm / http | `input` | `output` + `error`(若 onError=branch) |
| condition | `input` | `true` / `false` |
| loop | `input` | `output`(收集) |
| parallel | `input` | `output`(合并) |
| approval | `input` | `approved` / `rejected` / `timeout` |
| variable | `input` | `output` |
| wait | `input` | `output` |

每个 input port **最多接 1 条 edge**;output port 可以接 N 条(扇出)。

---

## 6. 表达式语言

### 6.1 语法

字符串中出现 `{{ ... }}` 即表达式,系统按 Go `text/template` 模板求值。

### 6.2 V1 支持的引用

| 引用 | 含义 |
|---|---|
| `vars.x` | workflow 级变量(顶层 `variables[]` 声明) |
| `in.field` | 当前节点的输入(input port 数据) |
| `in.<port>.field` | 指定 input port 的数据 |
| `nodes.<id>.output.field` | 显式引用上游某节点的 output |
| `nodes.<id>.output.<port>.field` | 显式引用某节点某 output port |
| `loop.item` / `loop.index` | 在 loop body 内,当前迭代项 / 索引 |
| `run.id` / `run.startedAt` | 当前 FlowRun 元信息 |
| `env.X` | 系统环境变量(V1 受白名单约束,防泄漏) |

### 6.3 实现

Go `text/template` + 自定义 funcMap,~80 行实现:
- 不引 Jinja / Lua / 其他 DSL
- V1 不支持复杂表达式(三元 / 数学运算)— 复杂的写一个 function 节点处理
- 表达式编译期(graph 校验时)做 syntax 检查

### 6.4 V1.5 候补
- 三元 (`{{ in.x > 10 ? 'high' : 'low' }}`)
- 数学运算
- JSONPath / JMESPath 集成

---

## 7. 校验规则(V1 必做)

写入 / accept pending 时跑校验。

### 7.1 单 op 校验(per-op apply)
- op schema 自身合法(必填字段、类型)
- op 类型在白名单
- 字段值合法(node id 字符集、表达式 syntax 等)

### 7.2 累积校验(每 op 应用后)
- 引用合法:add_edge 时 from / to 节点存在
- 部分阶段允许临时不合法(先 add_node 再 add_edge — 单看 add_node 后图无 edge 是合法的)

### 7.3 Final 校验(全部 ops 应用完)

| 规则 | 检查 |
|---|---|
| **DAG 无环** | 顶层 toposort 必须 succeed(loop / parallel 的 body 子图同样校验) |
| **节点 id 唯一** | 全图(含子图)节点 id 不重复 |
| **node type 在白名单** | 13 种 V1 type |
| **每条 edge 引用合法** | from / to 节点存在,port 存在 |
| **至少 1 个 trigger 节点** | 否则 workflow 永远不会跑 |
| **trigger config 完整** | 各 kind 自己的必填字段 |
| **capability 节点引用** | `function` / `handler` 节点的 name 在已 forged 列表里;`skill` 节点的 name 在 ~/.forgify/skills/;`mcp` 节点的 serverName **必须在已安装 mcp.json server 列表里**(D9-3-C);未装时 reject(错误码 `WORKFLOW_MCP_SERVER_NOT_INSTALLED`) |
| **变量引用合法** | `{{ vars.x }}` 引的变量在顶层 `variables[]` 里 |
| **container 节点 body 递归校验** | loop / parallel 的子图同样跑这套规则 |

### 7.4 V1.5 不做的校验
- 类型流静态检查(节点 output 类型 vs 下游 input 类型匹配)
- unreachable node 检测(图里有节点但永远跑不到)
- 表达式静态求值(检查 `{{ vars.x }}` 是不是真用过了)

### 7.5 校验失败时

写入 `workflow_versions` 一行 status=`rejected`,`change_reason` 含错误信息(让用户看见 LLM 试图改了啥),activeVersionId 不动。

---

## 8. 持久化 — 2 张表

### 8.1 `workflows`

| 字段 | 类型 | 说明 |
|---|---|---|
| id | TEXT PK | `wf_<16hex>` |
| user_id | TEXT 索引 | local-user |
| name | TEXT | partial UNIQUE `(user_id, name) WHERE deleted_at IS NULL` |
| description | TEXT | — |
| tags | TEXT (JSON) | — |
| **enabled** | BOOLEAN default true | V1 必做(详见 [`05-execution-plane.md`](./05-execution-plane.md) §6) |
| **concurrency** | TEXT default `serial` | `serial` / `parallel(N)`,V1 默认 serial(详见 [`05-execution-plane.md`](./05-execution-plane.md) §6) |
| **needs_attention** | BOOLEAN default false | D20 — 引用 capability 被删后系统自动标 true,trigger 时 fail-fast(`WORKFLOW_NEEDS_ATTENTION`);用户 edit 修后 reset false |
| **attention_reason** | TEXT NULL | needs_attention=true 时简短说明(如 `"handler 'pg-prod' was deleted"`)给 UI 显示 |
| active_version_id | TEXT | 指向当前活 WorkflowVersion;草稿期空 |
| 时间戳 + 软删 | — | GORM 标配 |

**计算字段**(`gorm:"-"`):
- `Pending *WorkflowVersion`(由 service 在 GET / List 后填)
- `LiveRuns int`(可选,从 flowrun 拿 status=running 数量,UI 显示)
- `LastFiredAt *time.Time` / `NextFireAt *time.Time`(cron trigger 时,从 trigger service 拿)

### 8.2 `workflow_versions`

| 字段 | 类型 | 说明 |
|---|---|---|
| id | TEXT PK | `wfv_<16hex>` |
| workflow_id | TEXT 索引 | FK → workflows.id |
| status | TEXT CHECK | `pending` / `accepted` / `rejected` |
| version | INT NULL | accepted 递增;pending/rejected 为 NULL |
| **graph** | TEXT | 整张 DAG JSON(§4 shape) |
| change_reason | TEXT | "edit_workflow: applied N ops: ..." / "manual edit" / "reverted to v{N}" / "initial" |
| 时间戳 | — | GORM 标配 |

accepted 版本上限 50 条/workflow,超限硬删最旧。

**就 2 张表**。Triggers / FlowRuns 都不在 workflow domain — Trigger 属 trigger domain(scheduler 自己 register 监听器),FlowRun 属 flowrun domain。Workflow domain 边界干净。

---

## 9. LLM 工具集(7 个)

```typescript
search_workflow({ query?, limit?, cursor? }) 
  → { items: WorkflowSummary[], nextCursor?, hasMore }

get_workflow({ id }) 
  → { workflow: Workflow, activeVersion?: WorkflowVersion, pending?: WorkflowVersion }

create_workflow({ name, description, ops, changeReason }) 
  → { id, version, status, opsApplied }
  // 流式:每 op emit progress delta

edit_workflow({ id, ops, changeReason }) 
  → { pendingId, opsApplied }
  // 流式:同 create

revert_workflow({ id, targetVersion }) 
  → { pendingId }

delete_workflow({ id }) 
  → { deleted }

trigger_workflow({ workflowId, input?, wait?: boolean = true }) 
  → { runId, status, output? }
  // 流式:wait=true 时订阅 run 全程进度

// D22 — execution log 工具(per-entity,flowrun_nodes 表)
search_workflow_executions({ workflowId?, flowrunId?, nodeType?, status?, conversationId?, since?, until?, limit?, cursor? })
  → { count, executions[], nextCursor?, aggregates }
get_workflow_execution({ id })
  → { ...全字段..., input 截 4KB, output 截 4KB, hints }
```

详见 [`01-shared-tool-interface.md`](./01-shared-tool-interface.md)。

---

## 10. HTTP API(~14 端点)

```
POST   /api/v1/workflows                          创建(直接传 graph,LLM 工具走 ops)
GET    /api/v1/workflows                          列表(分页 + 过滤 enabled)
GET    /api/v1/workflows/{id}                     详情(含 pending + LiveRuns + LastFiredAt)
PATCH  /api/v1/workflows/{id}                     改 name / description / tags / enabled / concurrency
DELETE /api/v1/workflows/{id}                     软删

GET    /api/v1/workflows/{id}/versions            版本列表
GET    /api/v1/workflows/{id}/versions/{v}        单版本详情
POST   /api/v1/workflows/{id}:revert              回滚

GET    /api/v1/workflows/{id}/pending             看 pending
POST   /api/v1/workflows/{id}/pending:accept      接受
POST   /api/v1/workflows/{id}/pending:reject      拒绝

POST   /api/v1/workflows/{id}:trigger             手动触发(转发 scheduler)
GET    /api/v1/workflows/{id}/triggers            看 trigger 状态(详见 05-execution-plane.md §6)
PATCH  /api/v1/workflows/{id}/triggers/{idx}      改单个 trigger 配置(罕用,通常通过 edit_workflow ops)
```

`:trigger` 端点逻辑住 **scheduler domain**(workflow 本身不执行),HTTP handler 从 workflow handler 转发到 scheduler service。

---

## 11. 错误码

| Code | HTTP | Sentinel | 触发 |
|---|---|---|---|
| `WORKFLOW_NOT_FOUND` | 404 | `workflow.ErrNotFound` | id 查不到 |
| `WORKFLOW_NAME_DUPLICATE` | 409 | `workflow.ErrDuplicateName` | 重名 |
| `WORKFLOW_VERSION_NOT_FOUND` | 404 | `workflow.ErrVersionNotFound` | revert 不到 |
| `WORKFLOW_PENDING_NOT_FOUND` | 404 | `workflow.ErrPendingNotFound` | accept/reject 无 pending |
| `WORKFLOW_PENDING_CONFLICT` | 409 | `workflow.ErrPendingConflict` | edit 时已有 pending |
| `WORKFLOW_NO_ACTIVE_VERSION` | 422 | `workflow.ErrNoActiveVersion` | 草稿 + pending 未 accept 即 trigger |
| `WORKFLOW_DAG_CYCLE` | 422 | `workflow.ErrDAGCycle` | 校验时检测到环 |
| `WORKFLOW_INVALID_REF` | 422 | `workflow.ErrInvalidReference` | edge / 表达式引用不存在的节点 / 变量 |
| `WORKFLOW_NO_TRIGGER` | 422 | `workflow.ErrNoTrigger` | 校验时无 trigger 节点 |
| `WORKFLOW_OP_INVALID` | 400 | `workflow.ErrOpInvalid` | ops 应用失败 |
| `WORKFLOW_DISABLED` | 422 | `workflow.ErrDisabled` | enabled=false 时 trigger |
| `WORKFLOW_CAPABILITY_NOT_FOUND` | 422 | `workflow.ErrCapabilityNotFound` | 节点引用 function / handler / skill 不存在 |
| `WORKFLOW_MCP_SERVER_NOT_INSTALLED` | 422 | `workflow.ErrMCPServerNotInstalled` | `mcp` 节点 serverName 不在已安装 mcp.json 里(D9-3-C);LLM 应先 `install_mcp_server` |
| `WORKFLOW_MCP_SERVER_UNAVAILABLE` | 502 | `workflow.ErrMCPServerUnavailable` | runtime 调 mcp 节点时 server status≠ready;走 retry/onError(D9-3-B) |
| `WORKFLOW_CAPABILITY_REMOVED` | 422 | `workflow.ErrCapabilityRemoved` | runtime 触发时引用的 function / handler / skill / mcp_server 已被删(D20 级联);workflow 标 `needs_attention`,用户需 edit 替换 |
| `WORKFLOW_NEEDS_ATTENTION` | 422 | `workflow.ErrNeedsAttention` | trigger 时 workflow 处于 needs_attention 态(引用过期 / 未修复);用户需先 edit 修复才能跑 |

---

## 12. Catalog 不进 — 决策 D9

详见 [`00-overview.md`](./00-overview.md) §决策 D9 / [`01-shared-tool-interface.md`](./01-shared-tool-interface.md) §5。

Workflow 是 trigger-driven,LLM 主对话不该现场调用 workflow,因此 **不实现 CatalogSource**。LLM 需要时通过 `search_workflow` 工具按需查。

---

## 13. 测试覆盖(V1 目标)

| 测试套件 | 覆盖点 |
|---|---|
| `app/workflow/apply_test.go` | 各 op 应用 + 校验失败路径(DAG 环 / 引用错 / no trigger / 等) |
| `app/workflow/expression_test.go` | 表达式语言 syntax / 各引用形式 |
| `app/workflow/service_test.go` | CRUD + pending 流 |
| `test/workflow/workflow_pipeline_test.go` | E2E:create → trigger(基础)→ edit → revert |
| `test/workflow/streaming_test.go` | 流式 ops 应用 + 协议 invariants |
| `test/workflow/validation_test.go` | 校验规则覆盖 7.3 全部 |

---

## 14. 与执行 plane 的接口

Workflow domain 暴露给 scheduler / trigger / flowrun 的接口:

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
    // TriggerSpec 含 workflow_id / trigger 节点 id / kind / kind-specific config
}

// 反向 — workflow accept pending / 翻 active version 时通知 trigger 重 register
type WorkflowChangeNotifier interface {
    SubscribeActiveVersionChange(ch chan<- WorkflowChangeEvent)
}
```

详见 [`05-execution-plane.md`](./05-execution-plane.md) §1。

---

## 15. 实现清单(7 步,~5000 LOC)

1. **domain layer** — `domain/workflow/{workflow.go, version.go, node.go, edge.go, expression.go}` + 12 sentinel + Repository / 接口
2. **store layer** — `infra/store/workflow/workflow.go` + 集成测试
3. **app layer**:
   - `app/workflow/workflow.go` Service{search/get/create/edit/revert/delete}
   - `app/workflow/apply.go` apply_ops
   - `app/workflow/validate.go` 校验规则(单 op + 累积 + final)
   - `app/workflow/expression.go` 表达式 compiler / runtime
   - `app/workflow/notifier.go` ActiveVersion 翻新通知机制
4. **LLM tool** — `app/tool/workflow/{workflow.go, search.go, get.go, create.go, edit.go, revert.go, delete.go, trigger.go}` 7 工具
5. **HTTP API** — `transport/httpapi/handlers/workflow.go` + 14 端点
6. **装配 + WorkflowReader 接口暴露** — main.go / harness.go 把 Service 给 scheduler / trigger 用
7. **文档同步**:`service-design-documents/workflow.md` + 4 contract 文档 + progress + backend-design

---

## 16. 主要风险

| 风险 | 缓解 |
|---|---|
| 用户 / LLM 写出复杂图,前端渲染卡 | V1 限定单 workflow ≤ 100 节点(校验时 reject);V1.5 加分页渲染 |
| 表达式注入(用户写 `{{ env.PASSWORD }}` 拿密)| env 引用走白名单 + 默认空 |
| 图改完未 accept,trigger 走老版本 | scheduler 永远只读 active version,pending 不影响执行 |
| LLM ops 序列长导致 token 爆 | LLM 应自我节流(超 50 ops 拆成多次 edit);系统不强制限 |
| Container 节点 body 嵌套过深 | V1 限制递归深度 ≤ 3 层,reject 更深的 |

---

(本文档完)
