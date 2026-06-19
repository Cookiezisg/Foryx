---
id: DOC-014
type: reference
status: active
owner: @weilin
created: 2026-06-11
reviewed: 2026-06-14
review-due: 2026-09-14
audience: [human, ai]
---

# workflow —— 编排图实体（Quadrinity 的编排者）

## 1. 定位

"function 范式套一张图"：Workflow 持一条只增的不可变**图** Version 线 + 自由 active 指针（与 function 同款方案 A，无 pending/accept）。本实体**存/校验/pin** 图，**不执行**——执行是 [scheduler](../foundation/scheduler-flowrun.md) 的事（它 import 同一批纯 helper 走 pin 的版本）。

## 2. 心智模型

**图 = 静态"DAG + 回边"的有类型节点 + 接线边**。节点五类，各按 ref 前缀引用一个实体族：`trigger`(trg_) / `action`(fn_/hd_…method/mcp:server/tool) / `agent`(ag_) / `control`(ctl_) / `approval`(apf_)。节点 `ID` 是图内局部名——**也是下游 Input CEL 引用其 result 的名字**。`Input` 把被引用实体的每个声明字段映射到一条读上游节点结果的裸 CEL。**节点结果整形**（`dispatch.toResultMap`）：callable 返 JSON 对象 → 直通（下游读 `node.<字段>`）；fn/hd 返标量 → 落 `node.text`（**声明输出对 fn/hd 是 advisory**——要 `node.<字段>` 须返 dict，F40）；**唯 agent 在 invoke 处回解析**自己的终答（恰 1 声明 → 裹进该名、2+ 声明 + 非对象 → 报 `AGENT_OUTPUT_NOT_STRUCTURED` 大声失败）。边的 `FromPort` 只在 control（分支名）/approval（yes|no）源上有值——解释器把分支结局路由到 FromPort 匹配的边。

**头部三轴**（比任何图版本长寿，住 Workflow 行）：
- `LifecycleState`：active（监听中）/ draining（跑完在途、不起新——deactivate 时有 run 在飞）/ inactive。
- `Concurrency`（五种全实现，标准重叠菜单，默认 serial）：`serial`（排队、当前跑完再跑）/ `skip`（丢新 firing、记 `FiringSkipped`）/ `buffer_one`（排队但只留最新待处理——更早的待处理 firing 经 `SupersedePendingOlderThan` 标 `FiringSuperseded`）/ `replace`（优雅取消在途 run——`cancelRunningForReplace` 标 cancelled + 打断 advance、与 KillWorkflow 同 race-safe 序——改跑新 firing）/ `allow_all`（并发）。`overlapDecision`（run.go）在 `running>0` 时按策略分流。
- `NeedsAttention/AttentionReason/LastActionBy`：user vs system 状态变更归因。

## 3. 校验三层（依次设闸）

1. **`ValidateGraph`（domain 纯函数）**：形状（kind/ref 前缀匹配/action 接线非空）、良构（id 唯一、无悬挂/自环边、≥1 trigger、**全节点从 trigger 可达**）、**环纪律**（每条回边必须出自 control/approval——循环只能由分支决策闭合）、端口结构调和。失败 = `WORKFLOW_INVALID_GRAPH` + details.reason。
2. **CEL 编译 + 可见性 lint**（app `compileGraphCEL`，create/edit 时逐 Input 编译——domain 不准 import cel-go）：每节点先用**全图根** env 编译（区分语法错/引用不存在的名字），再用**恰为其祖先**（`Ancestors`）的 env 编译——后者强制「节点只能读保证已先于它完成的祖先 result」，越界标识符直接编译失败 →「references a non-ancestor node」，当场拒、非运行时。
3. **`CapabilityCheck`（app，对 resolver）**：ref 解析得到吗、kind 对吗、control 的 FromPort 在解析后分支集吗、hd 的 `.method` 存在吗、**mcp 的 `/tool` 在已连 server 的工具集吗**（F51——镜像 `.method` 校验、补上不对称；server 未连无名可校则跳过）。**绝不为缺失 ref 返 transport 错误**——把全部问题收进 `report.Problems`（编辑器一次看齐），仅在 resolver 自身故障时冒泡。`WORKFLOW_REF_NOT_FOUND` 是 resolver 的 miss 信号、被此处 `errors.Is` 捕获转成 problem 串，非由本检查抛出。无 resolver 时退化为仅结构报告（`Resolved=false`）。

## 4. 生命周期 / 行为

- **编辑 = 图 ops**（`set_meta/add_node/update_node/delete_node/add_edge/update_edge/delete_edge`，JSON 判别式；update 走 merge patch、id 不可变；delete_node 级联删边）。**活监听重绑**：active workflow 的 Edit/Revert 若改了入口 trigger ref，按旧/新图 diff 重指绑定（detach 删除者 + attach 新增者，`rebindIfListening`）——否则旧 trigger 永远触发本 workflow、新 trigger 无人听；staged 的一次性武装在 binder 内部、编辑保留旧武装。与 function 的差异：**不修 JSON**（ops 来自结构化编辑器/工具，畸形是该上呈的真错误）。`set_meta` 折成头部 patch（`ExtractMeta`），不动图。
- **执行生命周期五动作**（`execution.go`，协调 trigger Binder + scheduler Runner 两端口）：`:trigger`（立即跑一次，不碰监听）· `:stage`（待命恰一次真实触发后自动撤防；已 active 报 `WORKFLOW_ALREADY_ACTIVE`）· `:activate`（逐入口 trigger Attach + 翻 active；无 trigger 节点报 `WORKFLOW_NO_TRIGGER_ENTRY`——纯手动图只能 :trigger）· `:deactivate`（Detach + 翻 inactive/draining，在途 run 不杀）· `:kill`（Detach + 取消全部在途 run + inactive）。
- **pin**（`BuildPinClosure`）：跑前把图引用的每个实体解析成 active 版本快照；agent 额外递归一层进其挂载（深度封顶 2——agent 不能挂 agent）。解析不到的 ref 不算 pin 失败（那是 CapabilityCheck 的事）。
- boot 时 `ReattachActive` 重挂所有 active workflow 的监听（监听注册表在内存）——**在 per-workspace 播种 ctx 下跑**（见引擎文档 §5）。

## 5. 关键设计决策

图与执行彻底分离（本实体连 scheduler 都不 import——Runner/Binder 是注入端口）；回边判定 `BackEdges` 是导出纯函数、校验与执行共用（"回边"单一定义）；lifecycle/concurrency 放头不放版本（治理执行、比图长寿）；draining 由 scheduler 的 run 结算收口翻 inactive（`MarkInactiveIfDrained` 条件幂等）。

## 6. 契约（引用）

端点（CRUD + 9 个 `POST :action`——构建/查询 `:edit`/`:revert`/`:capability-check`/`:iterate` + 执行生命周期 `:trigger`/`:stage`/`:activate`/`:deactivate`/`:kill` + versions）→ [api.md](../api.md) · 表（`workflows`/`workflow_versions`，CHECK lifecycle+concurrency）→ [database.md](../database.md) · 码 `WORKFLOW_*` 16 个 → [error-codes.md](../error-codes.md) · ID：`wf_`/`wfv_`。LLM 工具 16 个：7 构建/查询 + 5 执行生命周期（trigger/stage/activate/deactivate/kill）+ 4 运行可观测/恢复/决策（`get_flowrun`——run 头 + 全节点记录；`search_flowruns`——闭合 `trigger_workflow` 返回 flowrunId 后的检查环；`replay_flowrun`——从断点重跑失败 run，清 failed 节点、留记忆化、按 run 原 pin 版本重走；`decide_approval`——批/拒 park 在审批节点的 run，包 `:decide` 同一 `DecideApproval`、首决胜，补全 agent 席不可达的人在环决策半边）。

## 7. 跨域集成

被 scheduler 读（WorkflowReader：冻结版本 + pin）；驱动 trigger Binder（Attach/AttachOnce/Detach）+ scheduler Runner（StartRun/Kill/CountRunning）；catalog/mention/relation 三适配器与 function 同构；`:iterate` 走 aispawn。
