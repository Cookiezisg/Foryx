---
id: WRK-001-19
type: working
status: draft
owner: @weilin
created: 2026-06-08
reviewed: 2026-06-08
review-due: 2026-09-08
audience: [human, ai]
landed-into:
---
# 19 — Workflow 模块落地设计（静态编排图实体 · backend-new）

> **本文是 backend-new「workflow 静态图实体」模块的落地蓝图**。它把 `18`（图模型总纲）+ `17`（durable 执行契约）+ 旧代码考古结论，收成一份可直接开工的实现设计。
>
> **范围铁律**：本文**只设计三模块里的第一个 —— workflow 静态实体**（`wf_`/`wfv_`，版本化编排图）。flowrun（journal）与 scheduler（durable 解释器）只在 §9 划边界 + 定接口，**各自的深设计另起一文**（20/21）。

---

## 0. 三模块切分 + 本文边界

workflow 这块物理上是**三个独立模块**，依赖自下而上：

| 模块 | 实体 | 是什么 | 本文 | 像谁 |
|---|---|---|---|---|
| **workflow** | `wf_`/`wfv_` | 静态编排图：版本化、ops 编辑、graph=JSON blob、catalog/relation/pin | ✅ **本文** | function 模板套「图」 |
| flowrun | `fr_`/`fre_`/`apv_` | 运行时实例 + journal（append-only）+ approvals 投影 | §9 预览 | 纯 store |
| scheduler | —（无实体） | durable 解释器（walk + replay + park/resume + firing 消费 + dispatch） | §9 预览 | 旧 7400 行引擎 |

**本文产出**：workflow 模块能独立编译、测试、commit、上契约——**不依赖 flowrun/scheduler 存在**（它只存图 + 校验 + 提供只读契约给上层消费）。

---

## 1. 数据模型（domain structs）

镜像 function 的「header + 单调版本 + active 指针」范式（`db` tag orm、无 pending、computed ActiveVersion）。

```go
// wf_<16hex> — 编排图 header；图在 active Version 上，不在本表。
type Workflow struct {
    ID              string     `db:"id,pk"`
    WorkspaceID     string     `db:"workspace_id,ws"`
    Name            string     `db:"name"`
    Description     string     `db:"description"`
    Tags            []string   `db:"tags,json"`
    Active          bool       `db:"active"`            // 是否参与调度（trigger 监听 / firing 派发）
    LifecycleState  string     `db:"lifecycle_state"`   // active | draining | inactive（17 §1）
    Concurrency     string     `db:"concurrency"`       // serial | Skip | BufferOne | BufferAll | AllowAll
    NeedsAttention  bool       `db:"needs_attention"`
    AttentionReason string     `db:"attention_reason"`
    LastActionBy    string     `db:"last_action_by"`    // user | system（区分自动 deactivate，ADR-022）
    ActiveVersionID string     `db:"active_version_id"`
    CreatedAt       time.Time  `db:"created_at,created"`
    UpdatedAt       time.Time  `db:"updated_at,updated"`
    DeletedAt       *time.Time `db:"deleted_at,deleted"`

    ActiveVersion *Version `db:"-"` // computed，Service.Get 附上
}

// wfv_<16hex> — 一份不可变的图快照。
type Version struct {
    ID                     string    `db:"id,pk"`
    WorkspaceID            string    `db:"workspace_id,ws"`
    WorkflowID             string    `db:"workflow_id"`
    Version                int       `db:"version"`              // 单调 max+1，绝不重排
    Graph                  string    `db:"graph"`                // JSON blob（下方 Graph 序列化）
    ChangeReason           string    `db:"change_reason"`
    ForgedInConversationID *string   `db:"forged_in_conversation_id"`
    CreatedAt              time.Time `db:"created_at,created"`
    UpdatedAt              time.Time `db:"updated_at,updated"`

    GraphParsed *Graph `db:"-"` // computed，Service 解析 Graph 后附上
}
```

### 图本体（存进 `Version.Graph` JSON）

```go
type Graph struct {
    Nodes []Node `json:"nodes"`
    Edges []Edge `json:"edges"`
}

// 节点 = 结构角色 + 引用一个实体 + 接线（18 §2）。
type Node struct {
    ID     string            `json:"id"`               // 图内局部 id（如 "n1"）
    Kind   string            `json:"kind"`             // trigger | action | agent | control | approval
    Ref    string            `json:"ref"`              // 引用的实体：trg_ / fn_|hd_.method|mcp: / ag_ / ctl_ / apf_
    Args   map[string]string `json:"args,omitempty"`   // 仅 action：argName → 裸 CEL（接线）
    Prompt string            `json:"prompt,omitempty"` // 仅 agent：{{ CEL }} 任务模板（接线，复用 pkg/cel Template）
    Retry  *RetryConfig      `json:"retry,omitempty"`  // 仅 action：平台 activity retry（非业务循环）
    Pos    *Position         `json:"pos,omitempty"`    // UI 布局
    Notes  string            `json:"notes,omitempty"`
}

// 边 = payload 数据管道（18 §4）。无类型（不分控制边/数据边）。
type Edge struct {
    ID       string `json:"id"`
    From     string `json:"from"`               // 源节点 id
    FromPort string `json:"fromPort,omitempty"` // control：分支 port 名；approval：yes|no；其余空
    To       string `json:"to"`                 // 目标节点 id
}

type RetryConfig struct {
    MaxAttempts int    `json:"maxAttempts"`        // 总尝试次数（1=不重试）
    Backoff     string `json:"backoff,omitempty"`  // "" | exponential
    DelayMs     int    `json:"delayMs,omitempty"`
}
type Position struct{ X, Y int }
```

**相对旧 NodeSpec 砍掉的（18 模型的简化）**：
- `Config map[string]any`（自由 JSON）→ 拆成 typed `Ref`/`Args`/`Prompt`（逻辑搬进实体后，节点只剩接线）。
- `Variables`（工作流级变量 `{{ vars.x }}`）→ **删**。作用域 = 上游节点输出（§3），无独立 var store。
- `ModelOverride`（agent/llm 节点的模型覆盖）→ **删**。模型配置归 `ag_` 实体自己。
- `OnError`（stop/continue/branch）→ **删**。失败 → flowrun failed → 修好 replay（durable 模型），不在图里接错误边（首版；未来若要可作为 control 的一种输入再议）。
- `at?`/`after?` 节点级 timer gate → **删**（18 §5，时间归 trigger / approval timeout / retry backoff）。

---

## 2. 节点 config schema（per kind · canon 字段名）

每个节点 = `{ kind, ref, 接线 }`。接线分两种（18 §2 铁律 2）：**接线 CEL 留节点，逻辑 CEL 进实体**。

| kind | ref 形态 | 节点侧接线 | 出口（由边的 fromPort 表达） | 逻辑住哪 |
|---|---|---|---|---|
| **trigger** | `trg_<hex>` | 无（payload 由触发方按 trg_ 的 payloadSchema 注入） | 单出口（空 port） | trg_ 实体（图外） |
| **action** | `fn_<hex>` / `hd_<hex>.method` / `mcp:server/tool` | `Args`：每值裸 CEL，读上游求类型化入参 | 单出口（空 port） | callable 实体 |
| **agent** | `ag_<hex>` | `Prompt`：`{{ CEL }}` 任务模板 | 单出口（空 port） | ag_ 实体 |
| **control** | `ctl_<hex>` | 无 | N 出口，`fromPort` = 分支 port 名（含回边） | **ctl_ 实体**（when/emit） |
| **approval** | `apf_<hex>` | 无 | 2 出口，`fromPort` ∈ `yes`/`no` | **apf_ 实体**（模板/timeout） |

> control/approval 节点**图上零接线**——它们的全部逻辑在实体里，图只负责「哪个 port 连哪个下游」。这正是 18「逻辑在实体、拓扑在图」的物理体现。

---

## 3. 边 = payload + 作用域模型【关键决策 ①】

18 §4 定了「边 = payload 数据管道，下游读上游已记账的 `node_completed.result`」。但**「下游怎么读上游」有两种实现模型**，旧代码与 18 意图不一致，必须现在拍板（它决定 CEL 怎么写、校验查什么）：

### 模型 A：payload-passing（旧代码实现）
- 边携带 payload，多入边在 join 处**隐式 merge**（`mergeMaps`）。节点 CEL 读 `payload.x`。
- 作用域 = `{ payload(已合并), ctx{runId, trigger} }`。
- ✅ 简单、旧代码已验证；❌ 多入边字段撞名时**覆盖歧义**（18 §4 明确反对）。

### 模型 B：node-addressable（17 §5 + 18 §4 意图）★ 推荐
- 节点 CEL 按**上游节点 id**寻址：`reviewer.score`、`{user: A.user, order: B.order}`。多入边**显式写、不隐式 merge**。
- 作用域 = `{ <上游节点id>: 其 result, ctx{runId, trigger} }`（从 journal 按数据流重建）。
- ✅ 对齐 durable journal（每节点 result 按 node_id 记账，作用域天然按 id 重建）、无覆盖歧义、守确定性；❌ 解释器作用域构建更复杂、循环里「读哪一轮」需定义（读当前 iteration 可见的上游）。

**我的推荐 = B**：它才是 17/18 真正想要的，且与「result 按 node_id 记账」的 journal 物理一致；旧代码的 A 是历史近似。**代价**是 scheduler 解释器要按入边来源建 id 寻址作用域（不难，但比 merge 多写一截）。

> **对 workflow 模块的影响（本文范围内）**：workflow **只存 CEL 字符串 + 校验可编译**；作用域语义是 scheduler 的运行时职责。但**选 B 则 capability_check 可顺带 lint「CEL 引用的上游 node id 确实是本节点的入边来源」**（选 A 则无此校验）。本文按 B 设计校验（§6），若改 A 则去掉该 lint。

---

## 4. 版本模型【关键决策 ②】

旧 workflow 有 `pending/accept/reject` 暂存态。但 backend-new 的 function/agent **已统一砍掉 pending**（零历史包袱）：每次 edit 写新版本、**立即生效**，revert 只移 active 指针。

**推荐：workflow 跟 function 一样砍 pending。**
- `Create` → 写 v1 + active 指向 v1。
- `Edit`（ops）→ 写 vN+1 + active 指向 vN+1（**立即生效**）。
- `Revert(targetVersion)` → active 指针移到历史某版本。
- **在途 flowrun 不漂移**：flowrun 启动时 `version_id` 钉死图拓扑 + pin 闭包钉死引用实体版本（17 §1 / ADR-020）。所以「edit 即生效」对在跑的 run 无副作用——它们看的是自己钉的旧版本。
- 版本上限 `AcceptedVersionCap = 50`，超出 trim 最旧（保护 active）。

> **唯一顾虑**：workflow 是**会被调度**的实体（不像 function 被动调用）。「edit 即 active」意味着改一半的图可能开始 firing。**缓冲方式**：编辑期 workflow 处于 `inactive`（不调度），改完显式 `:activate`。即「安全靠 lifecycle_state，不靠 pending 暂存」。若你认为编辑活跃 workflow 风险大、想保留 pending，这是要拍的点。

---

## 5. ops 编辑模型（AI 锻造）

跟旧 workflow 一致：编辑不是整图覆盖，而是**一串 ops**（AI 友好、可逐 op 推流到 forge SSE）。

| op | 作用 |
|---|---|
| `set_meta` | 改 name/description/tags |
| `add_node` | 插节点（kind + ref + 接线） |
| `update_node` | RFC 7396 merge patch 节点接线 |
| `delete_node` | 删节点（级联删关联边） |
| `add_edge` | 插边（from/fromPort/to） |
| `update_edge` | merge patch 边 |
| `delete_edge` | 删边 |

- 删 `set_variable`/`unset_variable`/`set_node_model_override`（§1 砍掉的概念）。
- `Create`/`Edit` 把 ops apply 到（空 / 上一版）图 → 校验（§6）→ 写新版本。apply + 校验逻辑在 app 层。

---

## 6. 校验（workflow 管前两层 · 17 §8）

17 §8 定了三层校验，**workflow 模块管 ①②，③ 运行时归 scheduler**：

**① JSON schema 形状**（每 op / 每 node.kind 的字段名、必填、枚举）：
- kind ∈ 5 枚举；ref 前缀匹配 kind（trigger→trg_、action→fn_|hd_|mcp:、agent→ag_、control→ctl_、approval→apf_）。
- action 的 Args 值非空 CEL 串；agent 的 Prompt 可空。

**② capability_check**（引用存在 + kind 匹配 + 结构良构）：
- 每个 node.ref **解析得到 + kind 对**（fn_ 真是 function、ctl_ 真是 control…）；hd_.method 的 method 在 active version 还在。
- **图良构**：节点 id 唯一、边 id 唯一、边两端节点存在、无自环、**至少一个 trigger 节点**、可达性（无孤儿节点）。
- **环 = 仅可归约回边**（control 回边、单入口循环；`BackEdges()` 与解释器共用，保证锻造与执行对环的判断一致）。
- **control 出口对账**：control 节点的出边 `fromPort` 必须命中 ctl_ 实体的某个 branch.port；approval 的 `fromPort` ∈ {yes,no}。
- **CEL 可编译**：每个 Args/Prompt/（经 ctl_ 的 when/emit）能 `cel.Compile`；选模型 B 则顺带 lint 引用的上游 node id 是入边来源（§3）。

> `capability_check` 是独立工具（旧 revamp 13 §1-E 要求「真查 ref 存在 + lint」）。被引用实体改 active 时**反向重查**（relation needs_attention）。

---

## 7. store schema（两表）

`internal/infra/store/workflow/`，orm + 手写 DDL（partial unique 进 schema_extras）。

**workflows**：`id` PK、`workspace_id`、`name`、`description`、`tags`(json)、`active`(int bool)、`lifecycle_state`(CHECK in active/draining/inactive)、`concurrency`(CHECK)、`needs_attention`(int)、`attention_reason`、`last_action_by`、`active_version_id`、D1/D2 时间戳。
- **partial UNIQUE**：`(workspace_id, name) WHERE deleted_at IS NULL` → orm `ErrConflict` 翻 `WORKFLOW_NAME_DUPLICATE`。

**workflow_versions**：`id` PK、`workspace_id`、`workflow_id`(index)、`version`(int)、`graph`(text json)、`change_reason`、`forged_in_conversation_id`(nullable)、D2 时间戳（**无软删**：版本不可变，trim 走硬删最旧）。
- **UNIQUE**：`(workflow_id, version)`。

> 17 §1 的 flowrun 相关列（`active`/`lifecycle_state`/`last_action_by`）本模块就建好；**transition（activate/deactivate/drain）由 scheduler/trigger 驱动**（本模块只提供 `UpdateMeta`/`SetLifecycle`/`SetNeedsAttention` 写入口）。

---

## 8. CRUD + tool + handler 表面

镜像 function 模块形态。

**Service**（`internal/app/workflow/`）：`Create` / `Edit`(ops) / `Revert` / `UpdateMeta` / `SetLifecycle` / `SetNeedsAttention` / `Get` / `List` / `Search` / `Delete` / `GetActiveVersion` / `GetVersion` / `ListVersions` / `CapabilityCheck` + catalog/relation 适配器 + **`BuildPinClosure`**（§9）。

**Tool**（`internal/app/tool/workflow/`，Lazy）：`create_workflow` / `edit_workflow` / `revert_workflow` / `delete_workflow` / `get_workflow` / `search_workflow` / `capability_check_workflow`。
- **不含** `trigger_workflow` / 执行查询类工具——那些消费 scheduler/flowrun，**留 scheduler 轮**。

**Handler**（REST）：`POST /workflows`、`GET /workflows`、`GET /workflows/{id}`、`PATCH /workflows/{id}`、`DELETE`、`GET /{id}/versions`、`GET /{id}/versions/{v}`、`POST /{id}:edit`、`POST /{id}:revert`、`POST /{id}:activate`、`POST /{id}:deactivate`、`POST /{id}:capability-check`。
- **不含** `:trigger` / `/triggers` / 执行历史——留 scheduler 轮。

**relation**：workflow 是第 **12** 类 EntityKind（前缀 `wf_`）；`workflow → {trg_, fn_/hd_/mcp_, ag_, ctl_, apf_}` 记引用边（引用计数 + 改 active 反查 needs_attention）。relation 实体 **11 → 12**。

---

## 9. 与 flowrun/scheduler 的边界（接口契约 · 预览）

workflow 模块对上层暴露**三个只读/纯函数契约**，scheduler 轮照此消费——本模块负责实现它们，不依赖 scheduler 存在：

```go
// scheduler 从 workflow 消费的只读契约（旧代码同名，DIP）。
type WorkflowReader interface {
    GetActiveVersion(ctx, workflowID) (*Version, error) // 取冻结图
    GetWorkflow(ctx, workflowID) (*Workflow, error)
    ListActive(ctx) ([]*Workflow, error)                // trigger 注册监听用
}
```

**pin 闭包构建（归属本模块）**：`BuildPinClosure(ctx, graph) (map[string]string, error)` —— 走图收集所有 node.ref，递归解析每个实体的 callable 依赖（agent 挂的 fn/hd），快照 `{entity_id: active_version_id}`（含 ctl_/apf_，18 §3 扩展；闭包深度 ≤2，ADR-020）。**放本模块因为它最懂「图 + 引用解析」**；scheduler 在 `StartRun` 单事务内调它。

**Dispatcher 端口（归属 scheduler，本文不实现）**：action（fn_/hd_.method/mcp:）+ agent（ag_）的执行器接口，M7 装配期接到各 app。这是 §briefing 说的「唯一硬地基缺口」，scheduler 轮定义。

**control/approval Resolve（已就绪）**：scheduler 解释器跑到 control/approval 节点时调 `control.Resolve(ref, 钉版本)→[]Branch` / `approval.Resolve(ref, 钉版本)→*Version`（R0045/R0046 已建）。

---

## 10. 关键决策清单（请拍板）

| # | 决策 | 选项 | 我的推荐 |
|---|---|---|---|
| ① | **CEL 作用域模型**（§3） | A payload-merge（旧、简单）/ B node-addressable（17/18 意图） | **B** |
| ② | **版本模型**（§4） | 砍 pending（同 function，安全靠 lifecycle）/ 留 pending 暂存 | **砍 pending** |
| ③ | **agent 节点接线**（§1/§2） | ref ag_ + `{{ CEL }}` prompt 模板 | 确认（复用 pkg/cel Template） |
| ④ | **OnError**（§1） | 删（失败→failed→replay）/ 保留图内错误边 | **删**（首版） |
| ⑤ | **pin 闭包归属**（§9） | workflow.BuildPinClosure helper / scheduler 自己走图 | **workflow 提供** |

①② 是真正影响形态的，③④⑤ 我倾向直接定。

---

## 11. R0047 任务分解（workflow 静态实体模块）

1. `domain/workflow`：Workflow/Version/Graph/Node/Edge/RetryConfig + ops 类型 + `BackEdges`/校验纯函数 + errorsdomain（WORKFLOW_*）。
2. `infra/store/workflow`：orm 两表 + 手写 DDL（partial unique + version unique）+ trim。
3. `app/workflow`：Service（CRUD + ops apply + 校验 + CapabilityCheck + BuildPinClosure + WorkflowReader 实现）+ catalog/relation 适配器。
4. `app/tool/workflow`：7 Lazy 工具（无 trigger/执行类）。
5. `transport/handler/workflow`：REST（无 :trigger/执行历史）。
6. `relation/entitykind`：第 12 类 `workflow`（wf_）。
7. 测试：domain（校验/BackEdges 表驱动）+ store（真 SQLite）+ app（ops apply + capability_check + pin 闭包）+ tool（wiring）+ relation。
8. 契约文档：`domains/workflow.md` 重写 + database/api/error-codes + relation 11→12 + contract-changes。
9. lab round + verify + commit + push。

> **装配留 M7**：`WorkflowTools`→Toolset、handler.Register、catalog source、relation namer、schema→migrate、SetRelationSyncer。

---

> **一句话**：workflow 模块 = function 范式套「图」。难度不在它，在后面 flowrun+scheduler；但把图模型（节点引用实体、边=payload、作用域模型）在这里定死，后两个模块才有干净的地基。本文先收口 workflow，把 ①② 两个跨模块决策一并拍了。
