# 08 — 编排 UI

脑爆结论笔记(2026-05-27)。
2026-05-31 改向 durable execution(详 [00-overview](./00-overview.md))。

> **本 doc 不细抠视觉**——直接沿用 Forgify 现有 `frontend/src/features/workflow-edit/ui/WorkflowEditor.tsx` + `pages/forge/ui/WorkflowDetail.tsx` 的形态。
> 只列**新设计对画布的功能性影响**(node palette / inspector / 触发入口 / 运行时滴答 / chat 协作)。
> UI 细节后续优化。
>
> **但 V1 前端 bar = 功能可用 + 可测**(不是"随便糊一下"):本 doc 列的每个功能点(5 节点 palette / inspector 字段 / lifecycle 开关 / ▶ 触发 / 运行时滴答 / inline diagnostic)都要**真能用、能端到端点通**,并留**最小可测路径**(关键交互可被 vitest / 手动冒烟覆盖)。视觉抛光与 subpage 右栏精细化留**未来前端重构**;但"功能可用 + 可测"是 V1 硬要求。

---

## 现状直接复用的部分

frontend FSD 已有:

| 既有 | 复用 |
|---|---|
| `features/workflow-edit/ui/WorkflowEditor.tsx` | 画布 + palette + pan/zoom/drag + 4-handle 连边 + 自动布局 + 2s autosave + inspector | ✅ 直接用 |
| `pages/forge/ui/WorkflowDetail.tsx` | 详情页框 + VersionRail + AskAiTrigger + RunDrawer + CapabilityCheckPanel | ✅ 直接用 |
| `entities/workflow/api/workflow.ts` + `model/types.ts` | API hooks + TS 类型 | ✅ 适配新 schema |
| `features/forge-iterate` / `forge-review` | AI 帮造 / accept-pending 流程 | ✅ 直接用 |
| `widgets/version-rail` | 版本历史 + diff 视图 | ✅ 直接用 |

**核心交互形态全部已经在了**:用户在画布上看(只读 + 跑时滴答),AI 在 chat 里改(`edit_workflow` 工具),画布实时刷新,VersionRail 看版本 / 一键 accept。

---

## 跟新设计的对接清单

### 1. Node palette 改 14 → 5

现 `NODE_KINDS` 数组(14 个)改成 5 个 + 表达更精确:

```typescript
const NODE_KINDS = [
  { kind: "trigger",  label: "Trigger",  icon: "Zap",   desc: "workflow 入口(cron / webhook / fsnotify / polling / manual)" },
  { kind: "agent",    label: "Agent",    icon: "Bot",   desc: "LLM 节点(prompt + skill + knowledge + tool)" },
  { kind: "tool",     label: "Tool",     icon: "Code",  desc: "调 forge 出来的 callable(function / handler / mcp)" },
  { kind: "case",     label: "Case",     icon: "GitBranch", desc: "switch 路由 + 回边形成 loop" },
  { kind: "approval", label: "Approval", icon: "Pause", desc: "等用户 yes/no" },
];
```

退役 / 合并 11 个旧 kind 的 palette 项(llm / function / handler / mcp / skill / condition / loop / variable / parallel / wait / http —— function/handler/mcp/skill 调用并入 `tool`、condition → `case`、llm → `agent`、loop/variable/parallel/wait/http → case 回边 / 作用域变量 / fork-join / durable timer)。

### 2. Inspector 字段跟新 node config schema

每个 kind 的 inspector 字段:

| 节点 | inspector 字段 |
|---|---|
| **trigger** | `kind`(cron/webhook/fsnotify/polling/manual)+ kind-specific config(cron expression / webhook path 等)+ `payloadSchema` |
| **agent** | `prompt` 段 + `skill` 单挂下拉 + `knowledge` 多挂列表 + `tool` 多挂列表 + `outputSchema` + `model`((apikey, modelId) 二元组)|
| **tool** | `callable` ref + `args`(JSON / key-value)+ `retry`(可选)+ `timeout`(可选)〔无 `onInfraCrash` knob:durable 模型下 infra 崩 = 该 activity 没记账 → 重放重跑,与 retry 是同一条路,不需独立旋钮〕|
| **case** | `branches`(有序列表;每条 = `when`(布尔 CEL 守卫,带 lint)+ `to` + 可选 `emit` CEL 对象;**首个 `when` 为真者胜出**,末条写 `when: "true"` 兜底)—— **无顶层 `expression` 字段**(per-branch 守卫取代"值 == 分支名",对齐 04 finding A)|
| **approval** | `prompt`(markdown,可插值)+ `timeout`(可选)+ `timeoutBehavior` + `allowReason` |

所有"平台默认值"字段一律改 placeholder("AI 编排时拍" / "不填 = ..."),**没有 hardcoded 默认值**。

### 3. 顶部加 Workflow lifecycle 开关

WorkflowDetail 头部加 **Active toggle**:

```
┌─ workflow X [v3 active] ────────── [○ Inactive] [▶ 试触发] [AI iterate] [Run] ─┐
│                                                                                  │
│  [画布]                                                  [VersionRail]            │
│                                                                                  │
└──────────────────────────────────────────────────────────────────────────────────┘
```

Toggle 调 `:activate` / `:deactivate` HTTP action(详 [06-workflow-lifecycle.md](./06-workflow-lifecycle.md))。

### 4. Trigger 节点上的 ▶ 触发按钮

画布上每个 trigger 节点角上加 `▶` 按钮(只在 trigger 节点)。点开弹 modal,按节点的 `payloadSchema` 渲染表单。

```
[trigger cron] ▶
                ↓ 点开
            ┌─ 触发 ────────────────────┐
            │ payload(按 schema 填):    │
            │   firedAt: [_____now_____]│
            │                            │
            │       [ 取消 ]  [ 触发 ]   │
            └────────────────────────────┘
```

提交 → `POST /workflows/{id}:trigger { triggerNodeId, payload }`(详 [01-triggers.md](./01-triggers.md) 触发统一抽象段)。一次提交起一个 flowrun(`scheduler.StartRun`);画布随即进入运行时滴答(下一节)。

### 5. 运行时滴答可视化(新功能,核心)

flowrun 跑起来时,画布实时把**该次执行的进度**映射成节点的视觉状态。**节点是唯一的滴答载体——边不画"流动"、不画"消息在传"**(durable execution 模型里边只是"谁接谁"的箭头,数据是当作值传递并记进事件日志的,没有"消息在边上飞"这回事,见 [00-overview](./00-overview.md) 术语映射)。

| 视觉 | 含义 |
|---|---|
| 节点 spinning border | 正在执行这个 activity(node_started 已记账、node_completed 未到) |
| 节点绿色 ✓ | 该 activity 已完成、结果已记进事件日志 |
| 节点红色 | 失败 / retry 用尽进死信 |
| 节点黄色 + ⏸ | approval durable 挂起,等用户信号 |
| 节点角标"循环第 N 轮" | case 回边触发的结构化循环,当前在第 N 轮(从事件日志该节点的记账条目推导轮次;`iteration_key` 是内部重放键,不抬到 UI 契约) |

**数据源(关键改动)**:**不新加 SSE**——SSE 上限三条(eventlog / notifications / forge),永不再加(对位后端 E1 + 前端 `cross-cutting.md`)。运行时状态从两处来,都是已有通道:

- **`notifications` SSE 的 `flowrun` 进度 tick**【CANON-X4】 → 进度 tick = **best-effort、可丢、绝不背压执行引擎**;它走 notifications 的一个**可丢 best-effort 子类**,与会阻塞的实体变更事件**隔离对待**,且**不进 notifications 的 replay buffer**。后端限流/合并:**每节点状态变化或每 N ms 一条**。前端收到 tick → react-query invalidate `flowruns` / `flowrun(id)` / `flowrunNodes(id)`(详 `cross-cutting.md` invalidate 映射)→ 重拉节点状态。节点颜色直接读 `FlowRunNode.status`(`pending / running / ok / failed / cancelled / timeout / skipped`)。
- **`eventlog` SSE** → 正在跑的 agent/tool 节点的 token / reasoning / tool_call / progress 实时流(节点的 `conversationId` 关联),让 spinning 节点能显示活体进度,不用等整步完成。

**真相在 journal,tick 只是实时提示**【CANON-X4】:节点状态的唯一真相是 flowrun 事件日志(`flowrun_events`,durable journal),tick 丢了无所谓——这跟 durable 一致(journal 是 durable 真相)。**UI 重连或丢了 tick** → 从 `get_flowrun_trace` **拉一次全量补**(REST 投影,见下一节 `GET /flowruns/{id}/trace`),不靠 tick 流自身可靠。

实现上新加一个 `useFlowrunTicker`(消费上面两条已有流 + 维护"nodeId → 视觉状态"映射 + 重连/丢失时从 trace 全量补),**而不是订一条新 SSE**。心智跟现有 `useForgeProgress`(锻造进度走 forge 流)同类——都是"订已有流 + 映射到 UI 状态机",只是数据流不同。

> **实现期待表达清楚(低优,X7)**:画布上**失败步**(红色 + 失败原因)/ **循环第 N 轮**(从 journal 推导轮次)/ **replay**(durable 重放时节点状态如何回放)三种场景的视觉表达需在实现期想清楚,本 doc 不细抠。

### 6. 节点详情 inline diagnostic

用户/AI 点节点 → 右侧 FloatingInspector 显示:

- 节点 config(只读 + "在 chat 里改这个节点" 按钮)
- 节点的运行时状态(本次执行:状态 / 已重试次数 / 耗时;循环节点则按轮次列)
- **该 flowrun 在这个节点上的事件日志(trace)**:这个节点每次被执行的 input + 结果 + 分支选择(case)+ 第几轮(循环),按序列出

**数据源:`GET /flowruns/{id}/trace?nodeId=X`**——读的是**该 flowrun 的事件日志(append-only journal,唯一真相)**过滤到该节点的那些条目,而不是任何"消息队列"。trace 即"这次执行在此节点记了哪些账":每条 = 一次 node_started/node_completed(activity 的 input/result)或 branch_taken(case 选了哪条)或 signal_awaited/received(approval)。循环节点会有多条,用事件日志的 `iteration_key` 区分轮次。

---

## chat-画布双 pane 协作

**chat-first 编辑**(现状已有,沿用):

```
用户在 chat:"帮我加个判断,字数 < 100 就不推 Slack"
   ↓
AI 调 edit_workflow + apply 2 ops(在 agent 和 tool 之间插 case)
   ↓
画布自动刷新(react-query invalidate)
   ↓
新 case 节点显示成黄色"待 accept"
   ↓
用户在 chat / VersionRail 点 accept → 落 active
```

**画布点节点 → chat 聚焦**:

```
用户在画布上点 agent 节点 → 节点上的"在 chat 里改"按钮
   ↓
chat 自动起话题:"想改这个 agent 节点的什么?prompt / outputSchema / model / 挂载的 tool?"
   ↓
用户聊几句 → AI 改 → 画布刷新
```

**实现**:画布节点上的按钮发送一个 intent 给 chat(用现有 `Shell.openConv` + 预填 prompt 模式)。

---

## 现状 → 新设计 改动量

| 改 | 代码量 |
|---|---|
| `features/workflow-edit/ui/WorkflowEditor.tsx` 的 `NODE_KINDS` 14→5 | 删 9 条加 3 条改 2 条 |
| 各 kind inspector 字段(替换现有) | per kind 一个组件,约 50-100 行 |
| WorkflowDetail 顶部 Active toggle | ~20 行 |
| Trigger 节点 ▶ 触发按钮 + payloadSchema 表单 | ~50 行 |
| 滴答动画(`useFlowrunTicker` + 节点状态映射) | ~80 行(消费已有 notifications + eventlog 两流 + state machine,**不开新 SSE**) |
| FloatingInspector 加运行时状态 + 节点 trace | ~50 行 |
| **总** | **~300-400 行,2-3 天** |

后端不需要新增 SSE 流——运行时状态复用已有 `notifications`(flowrun 事件)+ `eventlog`(节点 token 流)+ flowrun REST。只需补一个 Trace API(`GET /flowruns/{id}/trace`,读事件日志,~100 行),整套 UI 落地估**3-4 天**。

---

## 决策总览

```
1. 画布主体(palette + canvas + connect + inspector + autosave)  → 沿用现有 WorkflowEditor 形态
2. Node palette                                                 → 14 → 5(改 NODE_KINDS 数组)
3. Inspector 字段                                                → 跟新 node config schema(无 hardcoded 默认值)
4. Workflow Active 开关                                          → 顶部 toggle,调 :activate / :deactivate
5. Trigger 节点 ▶ 触发                                           → 每个 trigger 节点角上一个按钮 + payloadSchema 表单
6. 运行时画布滴答                                                → useFlowrunTicker(消费已有 notifications + eventlog;进度 tick best-effort/可丢/不背压引擎,真相在 journal,重连从 trace 全量补;不开第 4 条 SSE)【CANON-X4】
7. chat-画布协作                                                 → 双向(画布点节点 → chat 起话题;chat 改 → 画布刷新)
8. AI 帮造 / iterate / accept-pending                            → 沿用现有 forge-iterate + forge-review + VersionRail
```

UI 视觉细节(色板 / 间距 / 文案 / 动效)留下次优化,**功能层已覆盖**。
