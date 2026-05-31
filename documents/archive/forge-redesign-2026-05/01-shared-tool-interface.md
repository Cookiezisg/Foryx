# Shared LLM Tool Interface — Trinity 三类产物统一形态

**关联**:
- [`00-overview.md`](./00-overview.md) — 顶层愿景与决策摘要
- [`02-function.md`](./02-function.md) — Function 详设计
- [`03-handler.md`](./03-handler.md) — Handler 详设计
- [`04-workflow.md`](./04-workflow.md) — Workflow 详设计

**定位**:三类产物(Function / Handler / Workflow)共享的 LLM 工具接口形态、ops 模式、流式协议、catalog 取舍。是**跨域统一的合同**,但**实现各自一份不复用代码**(每 domain 自己 apply_ops,详见决策 D5)。

---

## 1. 21 个 LLM 工具矩阵

| Action | Function | Handler | Workflow | 流式? |
|---|---|---|---|---|
| 搜 | `search_function` | `search_handler` | `search_workflow` | ✗ |
| 看 | `get_function` | `get_handler` | `get_workflow` | ✗ |
| **建** | `create_function` | `create_handler` | `create_workflow` | **✓ ops-driven** |
| **改** | `edit_function` | `edit_handler` | `edit_workflow` | **✓ ops-driven** |
| 回滚 | `revert_function` | `revert_handler` | `revert_workflow` | ✗ |
| 删 | `delete_function` | `delete_handler` | `delete_workflow` | ✗ |
| **执行** | `run_function` | `call_handler` | `trigger_workflow` | **✓ progress** |
| **搜执行** | `search_function_executions` | `search_handler_executions` | `search_workflow_executions` | ✗ |
| **看执行** | `get_function_execution` | `get_handler_execution` | `get_workflow_execution` | ✗ |

执行类工具因类目不同语义化命名(run / call / trigger),其余 8 个动词三类完全统一。

**9 actions × 3 kinds = 27 trinity 矩阵工具**。LLM 学一类工具,会用其他两类。心智负担一致。

> **额外工具(矩阵之外)**:
> - `update_handler_config` — Handler-specific(D16,详 [`03-handler.md`](./03-handler.md) §6.5)
> - 平行 mcp / skill execution 工具(per-entity,跟 trinity 同模式):
>   - `search_mcp_executions` / `get_mcp_execution`(详 [`08-executions.md`](./08-executions.md) §7)
>   - `search_skill_executions` / `get_skill_execution`(同上)
>
> 总 LLM 工具数 = 27 矩阵 + 1 Handler config + 4 mcp/skill executions = **32 个**。
>
> **设计哲学**:5 张 per-entity execution log 表(D22)→ **5 套 per-entity 工具(search + get 各一)**,跟表分开一致。LLM 拿到 execution id 时已知 kind(从 search 返来),get_<kind>_execution 无需 dispatcher。

---

## 2. 工具 args / return 统一形态

### 2.1 创建 (`create_*`)

```typescript
create_<kind>({
  name: string,
  description: string,
  ops: Op[],          // 流式构建:从空白基础逐 op 长出来
  changeReason: string
}) → {
  id: string,         // fn_<16hex> / hd_<16hex> / wf_<16hex>
  version: number,    // 通常是 1(first-create auto-accept,详见 §6)
  status: "accepted" | "pending",
  opsApplied: number  // 实际成功 apply 的 op 数
}
```

### 2.2 修改 (`edit_*`)

```typescript
edit_<kind>({
  id: string,
  ops: Op[],          // 从 active version 副本逐 op 改
  changeReason: string
}) → {
  pendingId: string,  // fnv_<16hex> / hdv_<16hex> / wfv_<16hex>
  opsApplied: number
}
```

`create` 和 `edit` 区别仅在 base:
- create:base = 空白产物
- edit:base = 当前 active version 副本

底层是 `apply_ops(base, ops) → result`,**每域一份独立实现**(D5)。

### 2.3 读类

```typescript
search_<kind>({ query?, limit?, cursor? }) → {
  items: Array<<kind>Summary>,
  nextCursor?: string,
  hasMore: boolean
}

get_<kind>({ id }) → {
  <kind>: Entity,
  activeVersion?: Version,
  pending?: Version
}
```

`<kind>Summary` 各域不同(Function 含 parameters / Handler 含 methods / Workflow 含 trigger 类型),Entity 同。

### 2.4 版本管理

```typescript
revert_<kind>({ id, targetVersion }) → {
  pendingId: string  // revert 走 pending → user accept 流程,不直接修改 active
}

delete_<kind>({ id }) → {
  deleted: boolean   // 软删(deleted_at 写时间戳,引用方继续可用直至硬删)
}
```

### 2.5 执行类(三类各自特化签名)

```typescript
// Function — stateless 一次跑
run_function({
  functionId: string,
  args: object,
  version?: number  // 默认 active
}) → { ok: bool, output: any, error?: string, elapsedMs: int }

// Handler — 隐式 acquire current caller-context 的 instance
call_handler({
  handlerName: string,
  method: string,
  args: object
}) → { ok: bool, output: any, error?: string, elapsedMs: int }
// 注:LLM 不传 instance_id,系统按当前 caller-context 自动选

// Workflow — 触发执行,可选等待结果
trigger_workflow({
  workflowId: string,
  input?: object,
  wait?: boolean = true  // true = 订阅 run 全程并等结果
}) → {
  runId: string,
  status: "running" | "completed" | "failed" | "cancelled",
  output?: any  // wait=true 时附终态产物
}
```

### 2.6 Execution Log 工具(D22)— 5 套 per-entity 工具

跟 D22 5 张 per-entity 表对应,**每域各自 search + get 一对工具**,共 10 个。Trinity 3 个(function/handler/workflow)在 §1 矩阵第 8/9 行;平行 mcp/skill 同模式不在矩阵。

#### 统一签名模板(各域各自实例化)

```typescript
// 搜:filter + 分页 + aggregates 摘要
search_<kind>_executions({
  // 通用 filter
  status?: "ok" | "failed" | "cancelled" | "timeout",
  conversationId?: string,    // chat 触发追溯
  flowrunId?: string,         // workflow 触发追溯
  since?: string,             // ISO8601
  until?: string,
  limit?: number = 50,
  cursor?: string,
  // kind-specific filter(下面各域列)
  ...
}) → {
  count,
  executions[],               // 每条含 input_preview / output_preview 截 200B
  nextCursor?,
  aggregates: { ok_count, failed_count, avg_elapsed_ms, p95_elapsed_ms, ... }
}

// 看:单 id 详情
get_<kind>_execution({ id }) → {
  ...all fields...,
  input,                      // 截 4KB
  output,                     // 截 4KB,sensitive 字段 mask 为 "***"
  hints: { output_empty, significantly_slower, duplicates_previous_input? }
}
```

#### Kind-specific filter(只列差异;签名其余跟模板一致)

| 工具 | kind-specific filter |
|---|---|
| `search_function_executions` | `functionId?`, `versionId?` |
| `search_handler_executions` | `handlerId?`, `method?`, `ownerKind?`, `instanceId?` |
| `search_workflow_executions` | `workflowId?`, `nodeType?`(flowrun_nodes 域) |
| `search_mcp_executions` | `serverName?`, `toolName?` |
| `search_skill_executions` | `skillName?`, `forkDepth?` |

每域 search/get 工具实现在对应 `app/tool/<kind>/` 目录(per spec D5 — 不在跨域共享代码里)。

详细 schema 与实施细节见 [`08-executions.md`](./08-executions.md) §7。

---

## 3. Op 类型分类

每域有自己的 op 集合(详见各域文档)。下方按类列出,具体 schema 在各域文档详细化。

### 3.1 通用 op(三域共享语义,实现各自一份)

```typescript
{ op: "set_meta", name?, description?, tags? }
```

### 3.2 Function-specific ops

```typescript
{ op: "set_code", code: string }
{ op: "set_parameters", parameters: ParameterSpec[] }
{ op: "set_return_schema", returnSchema: object }
{ op: "set_dependencies", deps: string[] }     // PEP 508
{ op: "set_python_version", version: string }  // PEP 440
```

详见 [`02-function.md`](./02-function.md) §3。

### 3.3 Handler-specific ops(method-level,跟 workflow 节点级 ops 心智一致)

Handler class 由系统按 ops 拼装(LLM 不写整 class)。每 op 局部应用,改 1 个 method 不动其他。

```typescript
{ op: "set_imports", imports: string }           // class 顶部 import 语句
{ op: "set_init", init_body: string }            // __init__ body
{ op: "set_shutdown", shutdown_body: string }    // shutdown body(可选)
{ op: "set_init_args_schema", args: InitArgSpec[] }   // 启动时一次性参数 schema
{ op: "add_method", method: MethodSpec }              // 含 args / return_schema / body / streaming
{ op: "update_method", name: string, patch: object }  // patch 走 JSON Merge Patch(RFC 7396)
{ op: "delete_method", name: string }
{ op: "set_dependencies", deps: string[] }
{ op: "set_python_version", version: string }
```

`MethodSpec` 含 body 字段(LLM 提供 method body,系统拼到 class 里)。`InitArgSpec` 含 `sensitive` flag 标 secret。详见 [`03-handler.md`](./03-handler.md) §4 / §6.5。

### 3.4 Workflow-specific ops

```typescript
{ op: "add_node", node: NodeSpec }
{ op: "update_node", id: string, patch: object }
{ op: "delete_node", id: string }
{ op: "add_edge", edge: EdgeSpec }
{ op: "update_edge", edgeId: string, patch: object }
{ op: "delete_edge", edgeId: string }
{ op: "set_variable", name: string, type: string, default?: any }
{ op: "unset_variable", name: string }
```

详见 [`04-workflow.md`](./04-workflow.md) §3。

---

## 4. 流式协议(三域统一)

### 4.1 包装机制

所有 LLM tool 调用产生一个 `tool_call` block(LLM 自带 `tc_id`),tool 内部用 ctx 上的 Emitter emit 子 block,子 block 全部 `parentId = tool_call.id`。tool 结束 emit `tool_result` block。

事件协议**复用现有 `domain/eventlog`**(5 events × 6 block types × parentId 嵌套),**不引入新协议**。

### 4.2 创建 / 修改 — ops-driven 流式

`create_*` 和 `edit_*` 工具的流式机制:**每应用一个 op 都 emit 一个 `block_delta` 给 progress block**。前端按 op 类型做 incremental 渲染。

#### 事件流示例(`create_workflow`)

```
block_start  tc_xxx     tool_call    attrs.tool="create_workflow"
  block_start  blk_p   progress   (parent=tc_xxx) attrs.stage="applying_ops"
  block_delta  blk_p   '{"op":"set_meta","name":"email-headhunter",...}\n'
  block_delta  blk_p   '{"op":"add_node","node":{"id":"trig","type":"trigger",...}}\n'
  block_delta  blk_p   '{"op":"add_node","node":{"id":"fetch",...}}\n'
  block_delta  blk_p   '{"op":"add_edge","from":"trig.next","to":"fetch.input"}\n'
  ...
  block_stop   blk_p   completed
block_start  blk_r     tool_result  (parent=tc_xxx)
block_delta  blk_r     '{"id":"wf_xxx","version":1,"status":"accepted",...}'
block_stop   blk_r     completed
```

#### 前端 op-driven 渲染

| 域 | 视觉效果 |
|---|---|
| Function | 代码框打字机式逐字长出(`set_code` 内部 LLM 流式写入),deps 行陆续添加,parameters 表逐行长出 |
| Handler | class 主体打字机式长出,methods 表一个个长出方法签名 + body |
| Workflow | 节点逐个淡入,边逐条连出,变量陆续出现 — 正是"呼啦呼啦"的源头 |

三类视觉差异在前端 op 解析逻辑里,**后端流式协议完全一致**。

### 4.3 执行类 — progress block 流式

`run_function` / `call_handler` / `trigger_workflow` 内部:

| 工具 | progress 内容 |
|---|---|
| `run_function` | sandbox spawn 进度 + Python 子进程 stdout / stderr 行 |
| `call_handler` | instance acquire 进度(若需 spawn);method 内部 `yield` 进度;stdout |
| `trigger_workflow`(wait=true) | 每节点 start / end / 输出 emit 成 progress;run 完结时 final result;详见 [`05-execution-plane.md`](./05-execution-plane.md) §4 |

---

## 5. Catalog 取舍

| 产物 | 进 catalog? | 理由 |
|---|---|---|
| **Function** | ✅ | LLM 主对话该知道有哪些已锻造 Function;catalog summary 教 LLM"你有这几类计算能力" |
| **Handler** | ✅ | LLM 该知道有哪些 Handler Definition + 暴露什么 method,主对话能 `call_handler` |
| **Workflow** | ❌ | trigger-driven,LLM 主对话不该现场调用 workflow;需要时通过 `search_workflow` 工具按需查 |

实现:
- `app/function/AsCatalogSource()` — Per-item granularity,LLM-gen description
- `app/handler/AsCatalogSource()` — Per-item granularity(注:这里"item"是 HandlerDefinition,不是 Instance)
- workflow 不实现 CatalogSource

参考 `app/forge/` 现有 `AsCatalogSource()` 模式(已在 catalog domain D8 落地)。

---

## 6. Pending / Accept 流(三类共用)

LLM 调 `create_*` 或 `edit_*` → 系统写入 version 行 status=pending → 用户在前端 review (diff 视图 + 视觉对比)→ 用户 accept / reject。

```
LLM apply ops → workflow_versions / function_versions / handler_versions 多一条 status=pending
                graph / code / class spec = base 副本 + ops 应用结果
                change_reason = LLM 给的或自动生成
                ↓
         用户在前端 review
                ↓
       ┌────────┴────────┐
     accept            reject
       ↓                  ↓
   status=accepted   status=rejected
   version int+1     activeVersionId 不变
   activeVersionId 翻
```

### 特殊优化(对齐 forge TE-15)
**首次 `create_*` + 校验全过 → auto-accept**。后续 edit 仍走手动 review(改图 / 改代码风险大,diff 必看)。

---

## 7. 校验时机

| Op 应用阶段 | 校验内容 |
|---|---|
| Per-op 应用前 | op schema 自身合法(必填字段、类型) |
| 每个 op 应用后 | 累积 base 仍合法(部分阶段允许临时不合法,如先 `add_node` 再 `add_edge` 时单看 add_node 后图无 edge 是合法的) |
| 全部 ops 应用完 | 整体合法性(DAG 无环、AST 可解析、外部引用存在等)— 不通过则整批 reject |

校验失败时:
- 已应用的 ops **不回滚到 active**(我们写到 pending,所以 active 不动)
- pending 写入但 status 为 `rejected`(让用户看见 LLM 试图改了啥)+ 错误信息附在 `change_reason`

---

## 8. 错误信息形态(LLM-facing)

各工具失败返回的 LLM-facing 错误字符串遵循统一模式:

```
<kind>_<errType>: <human-readable description>
suggestion: <actionable next step for LLM>
```

例:

```
function_NotFound: function "fn_xxx" doesn't exist or has been deleted
suggestion: call search_function to find the right id

handler_InstanceSpawnFailed: failed to spawn handler instance "pg" — env not ready (status=failed)
suggestion: check Definition's deps + python_version, may need edit_handler

workflow_OpInvalid: ops[2] add_edge references non-existent node "filter"
suggestion: ensure nodes are added before edges referencing them

workflow_DAGCycle: ops result in a cycle: trig -> fetch -> filter -> trig
suggestion: review edges, remove or restructure the cycle
```

LLM 通过 suggestion 字段自动恢复 / 重试。

---

## 9. 不在共享层的 — 重要边界

下列**故意不共享**,因为强统一会让各域臃肿:

- **op 实现** — 每域 `app/<kind>/apply.go` 一份,~80 行各自;**不抽 helper**(详见决策 D5)
- **校验规则** — Function 校验(parameters schema / 代码 AST)、Handler 校验(class 形态 / method schema)、Workflow 校验(DAG 无环 / port 匹配)各自一套
- **持久化 schema** — 三域各自 entity / version 表,字段不互相对齐(每域有自己的 specific 字段)
- **执行模型** — 各自 dispatcher(function 调 sandbox / handler 调 instance RPC / workflow 走 scheduler)

共享的只有 **接口形态**(args / return / 流式协议),不下沉到实现层。

---

## 10. LLM 工具 description 模板(建议)

为统一心智,三类产物的工具 description 用同一模板风格:

```
search_<kind>: 在用户的 <kind> 库里找匹配 query 的条目;query 留空时返完整列表(LLM ranked top K)
get_<kind>: 看一个 <kind> 的完整定义,返 active version 内容 + 待批 pending(若有)
create_<kind>: 锻造一个新 <kind>;ops 数组流式构建,从空白逐 op 长出;首次创建 auto-accept
edit_<kind>: 改一个 <kind>;ops 数组流式应用到 active 副本,生成 pending 等用户 review
revert_<kind>: 回滚到指定历史 version,生成 pending
delete_<kind>: 软删(deleted_at 写时间戳),引用方继续可用直至硬删
run_function / call_handler / trigger_workflow: 执行(各自语义,详见各文档)
```

工具实现时 description 要包含:1) 一句话用途;2) 关键参数 / 返回值要点;3) 何时用(对比另两类的 hint);4) 错误处理 hint。

---

## 11. ID 前缀规约(对齐 §S15)

| 实体 | 前缀 |
|---|---|
| Function entity | `fn_<16hex>` |
| FunctionVersion | `fnv_<16hex>` |
| **FunctionExecution**(D22)| **`fne_<16hex>`** |
| Handler Definition | `hd_<16hex>` |
| HandlerVersion | `hdv_<16hex>` |
| HandlerInstance(运行时,不持久化)| `hdi_<16hex>` |
| **HandlerCall**(D22)| **`hcl_<16hex>`** |
| **MCPCall**(D22)| **`mcl_<16hex>`** |
| **SkillExecution**(D22)| **`ske_<16hex>`** |
| Workflow | `wf_<16hex>` |
| WorkflowVersion | `wfv_<16hex>` |
| FlowRun | `fr_<16hex>` |
| FlowRunNode | `frn_<16hex>` |
| Trigger 配置(若持久化)| `trg_<16hex>` |
| Webhook secret(若有)| `whs_<16hex>` |

详见各域文档的持久化章节;execution log 5 表前缀详见 [`08-executions.md`](./08-executions.md) §11。

---

## 12. ops 实现的统一模式建议

虽然实现不共享,但每域 `apply_ops` 的 **代码骨架**建议一致(便于 review / 测试):

```go
// app/<kind>/apply.go
func (s *Service) apply(ctx context.Context, base *Spec, ops []Op) (*Spec, []OpResult, error) {
    state := base.Clone()
    results := make([]OpResult, 0, len(ops))
    for i, op := range ops {
        before := state.Snapshot() // 部分校验需要 before / after
        if err := state.Apply(op); err != nil {
            return nil, results, fmt.Errorf("apply.<kind>: ops[%d] %s failed: %w", i, op.Type, err)
        }
        if err := state.ValidateIncremental(); err != nil {
            return nil, results, fmt.Errorf("apply.<kind>: ops[%d] %s invalid: %w", i, op.Type, err)
        }
        results = append(results, OpResult{Index: i, Type: op.Type, OK: true})
        // emit progress(每 op 一个 delta)
        if em := eventlogpkg.From(ctx); em != nil {
            em.DeltaBlock(ctx, progressBlockID, op.JSONString()+"\n")
        }
    }
    if err := state.ValidateFinal(); err != nil {
        return nil, results, fmt.Errorf("apply.<kind>: final validation: %w", err)
    }
    return state.Build(), results, nil
}
```

每域的 `state.Apply(op)` 做 switch case 处理本域的 op set。三域独立实现这个 `apply.go`,~80 行各自。

---

## 13. 测试统一约定(建议)

虽实现不共享,**测试形态统一**便于跨域比对:

| 测试套件(每域) | 用途 |
|---|---|
| `app/<kind>/apply_test.go` | 单 op + 多 op 组合 + 校验失败的单测 |
| `test/<kind>/<kind>_pipeline_test.go` | E2E:LLM tool 调用 → 流式 → DB 写入 → 状态翻 |
| `test/<kind>/streaming_test.go` | 流式协议 invariant(seq 单调 / parent_block_id 正确 / 终态正确) |

3 套测试文件 × 3 域 = 9 份,各自独立,无共享 helper。harness `test/harness/` 已支持每个 domain 自动 wire,新 kind 加入只需注册新 service。

---

## 14. 落地顺序约束

实现这些 LLM 工具时按下面顺序避免循环依赖:

1. 各域 `apply.go`(纯函数,无外部依赖)
2. 各域 Service(组合 store + sandbox + apply.go + emit)
3. 各域 LLM tool(包装 Service + 标准字段 + ToLLMDef)
4. main.go / harness 装配(三域并列注册,无相互依赖)
5. catalog 加 source(只对 function / handler 加;workflow 不加)
6. subagent 加 forger types(可看到三域工具)
7. pipeline test

每步独立,可分 PR 提交。

---

## 15. 用户 Selection Metadata(圈选改一下协议)

用户在 UI 编辑器里选中某节点 / 某代码段 / 某 method,跟 LLM 说"改这里"时,**前端把选中信息作为结构化元数据附在 chat message 上**,后端 chat service 织入 LLM prompt,LLM 看到后用 `edit_workflow` / `edit_function` / `edit_handler` 发 targeted ops。

### 15.1 ChatMessage 扩展

```typescript
type ChatMessage = {
  text: string,
  attachments?: AttachmentRef[],
  selectionContext?: SelectionContext      // 新增可选字段
}

type SelectionContext = {
  target: {
    kind: "workflow" | "function" | "handler",
    id: string                              // wf_xxx / fn_xxx / hd_xxx
  },
  // workflow 专用
  nodeIds?: string[],                       // 选中的节点
  edgeIds?: string[],                       // 选中的边
  fieldPath?: string,                       // 钻到字段(如 "config.params.threshold")
  // function / handler 代码圈选
  textRange?: { start: number, end: number },  // 字符 offset
}
```

### 15.2 持久化与重放

`selectionContext` **存在 Message attrs JSON**(per `messages.attrs` 字段),让 conversation 重放时 LLM 能看到当时上下文。但前端渲染时**不显示这个 metadata**(隐藏给 LLM 用)。

### 15.3 LLM 看到的 prompt 格式

后端 `chat.runner.buildSystemPrompt` 检测 selectionContext 存在时,**动态从 workflow/function/handler service 取当前内容**,生成一段 system note:

```
The user has selected:
- Target: workflow wf_abc / "email-headhunter"
- Nodes: filter_cond (type=condition, current config.expression="{{ in.score > 0.8 }}")
- Field: config.expression

(Selection captured at 2026-05-10T12:34:56Z; the workflow may have been modified since.)

User instruction:
"让它判断更宽松点"
```

LLM 自然理解后发 targeted `edit_workflow({id:"wf_abc", ops:[{op:"update_node", id:"filter_cond", ...}]})`。

### 15.4 边界

- **selectionContext 是 hint 不是强制** — LLM 看到后仍可违背(用户说"算了改全图吧"时 LLM 不应傻按 selection 做)
- **stale 防御** — prompt 模板注明 selection 捕获时间,LLM 拿到时如发现已被改可主动 `get_*` 重新读
- **多选支持** — nodeIds 是数组,multi-select 一次性发
- **跨 target 选不允许** — selection 必须 anchor 在一个 target;跨 workflow / function 的"圈"V1 不支持
- **textRange** 仅给 function / handler **代码圈选**用(workflow 不用,workflow 用 nodeIds / edgeIds 锚)

### 15.5 LLM 工具 description 要点

`edit_workflow` / `edit_function` / `edit_handler` 工具 description 明确:
> If the user message comes with `selectionContext`, prefer ops that target the selected entity (nodeIds / fieldPath / textRange) — don't rewrite unrelated parts unless the user explicitly asks. The selection captures user intent narrowly.

### 15.6 实现成本

- domain/chat Message struct 加 selectionContext 字段(JSON 存 attrs):~50 行
- chat.runner buildSystemPrompt 检测 + 动态拼 selection note:~150 行
- HTTP `POST /messages` body 增 selectionContext 可选字段:~20 行
- 前端 selection 状态管理 + send 时附 metadata(后续 Wails 期):~200 行

总 ~400 行后端,前端待 Wails 实施时配套。

---

(本文档完)
