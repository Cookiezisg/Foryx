---
id: DOC-035
type: reference
status: active
owner: @weilin
created: 2026-06-11
reviewed: 2026-06-14
review-due: 2026-09-14
audience: [human, ai]
---

# 后端总览 —— 整个系统怎么组成、怎么流动

> 后端文档体系的**第 0 篇**：看完这篇知道全局怎么咬合，再进 `domains/` / `foundation/` 看细节。四索引（[api](api.md) / [database](database.md) / [error-codes](error-codes.md) / [events](events.md)）与代码逐字同步。

## 1. 一句话与一张图

**本地优先 Agentic Workflow 平台**：单进程、单用户、SQLite 落盘，Flutter 桌面 app（Go 后端作 sidecar）。核心心智 = **Quadrinity 四项全能**（Function / Handler / Agent / Workflow 四种能力实体）+ **Durable Execution**（节点结果记忆化 + 解释器幂等重走）。

```
┌─ transport ───────────────────────────────────────────────────────────┐
│  HTTP（统一 Envelope·28 资源 handler）+ 3 条 SSE 流（messages/entities/notifications）│
├─ app ────────────────────────────────────────────────────────────────┤
│  对话运行时        编排与执行              能力实体           支撑服务      │
│  chat ──┐         scheduler（解释器）      function           workspace    │
│  loop ←─┼─ 共享 ←─ workflow（图）          handler            apikey/model │
│  subagent┘        trigger（信号源+收件箱）  agent（挂载）       relation     │
│  conversation     flowrun（记忆化真相表）   control/approval   catalog/...  │
│  messages/todo/attachment/memory          skill/mcp/document  humanloop   │
├─ domain（纯 struct + 端口，零外部依赖）∪ infra/store（orm 三表范式）────────┤
├─ infra：llm（11 provider 一端口）· sandbox（直装运行时）· stream（Bus）· crypto│
├─ pkg 地基：orm · reqctx · errors · cel · schema · idgen · …（无上层依赖）  │
└─ bootstrap（composition root：唯一全知者，装配一切）────────────────────────┘
依赖单向：transport → app → (domain ∪ infra/store) → infra/db；pkg 全层可用
```

## 2. 模块地图（32 域 → 28 篇文档：domains/ 20 + foundation/ 8）

| 组 | 域（→ 文档） | 一句话 |
|---|---|---|
| **能力实体**（Quadrinity 前三 + 周边） | [function](domains/function.md) · [handler](domains/handler.md) · [agent](domains/agent.md) · [skill](domains/skill.md) · [mcp](domains/mcp.md) · [document](domains/document.md) | fn=每调用一进程的无状态代码；hd=常驻进程的有状态类（RPC）；ag=挂载能力的 LLM 员工；skill/document=指令与知识载体；mcp=外部工具网桥 |
| **编排与执行**（Quadrinity 第四 + 引擎） | [workflow](domains/workflow.md) · [trigger](domains/trigger.md) · [control](domains/control.md) · [approval](domains/approval.md) · [scheduler-flowrun](foundation/scheduler-flowrun.md) | wf=静态图（存/校验/pin）；trg=四源信号+durable 收件箱；ctl/apf=图的路由闸与人在环闸；引擎=幂等 advance 走记忆化 |
| **对话运行时** | [chat](domains/chat.md) · [messages](domains/messages.md) · [conversation](domains/conversation.md) · [subagent](domains/subagent.md) · [attachment](domains/attachment.md) · [memory](domains/memory.md) · [todo](domains/todo.md) | chat=枢纽但一无所有（全 DIP）；messages=中立块模型；subagent=递归子对话 |
| **支撑** | [relation](domains/relation.md) · [search](domains/search.md) · [微域合篇](domains/support-services.md) | 拓扑图 / 综搜垂搜积木 RAG 引擎 / 平台配置 + 横切服务 |
| **地基** | [orm](foundation/orm.md) · [reqctx](foundation/reqctx.md) · [loop](foundation/loop.md) · [stream-llm](foundation/stream-llm.md) · [sandbox](foundation/sandbox.md) · [platform-pkgs](foundation/platform-pkgs.md) · [bootstrap](foundation/bootstrap.md) | 自研 ORM / ctx 载体 / ReAct 引擎 / SSE 总线 + LLM 端口 / 隔离运行时 / 小件 / 装配根 |

## 3. 三条端到端数据流（系统的"整体感"在这）

### ① 一条 chat 消息的一生

```
POST /conversations/{id}/messages
 → chat.Send：落 user 回合 + 开 assistant 回合（mint id 作流锚点）+ message_start
 → 入该对话的串行队列（一次一个 assistant 回合）
 → processTask 重建 ctx（Detached+locale+ids+AgentState+双流桥+humanloop broker+cancel）
 → loop.Run：流式 LLM →（danger 门→）派发工具（执行组并行）→ 扩历史 → 循环
     工具 progress 实时嵌 tool_call 下；build 工具的 arg delta 镜像 entities 流
 → WriteFinalize（Detached：关页也不留 streaming 孤儿）落 blocks + message_stop
 → 回合后：首回合自动起标题 + 压缩检查（contextmgr 两步管线、水位幂等）
```

### ② 一次自动触发的一生（durable 主线）

```
cron tick / webhook / 文件变化 / sensor 探测
 → listener 报告 → onReport（Detached(wsID)）→ fanOut：
     1 条 Activation（触没触发都记——"为什么没触发"可查）
     + 每监听 workflow 1 条 Firing（pending，dedup key 防重复材化）
 → drainLoop 每 5s 逐 workspace：consumeFiring
     → overlap 决策（serial 推迟/skip 丢/allow_all 跑）
     → ClaimFiring 单事务：claim + 建 run 头 + seed trigger 节点（绝无半成品残留）
 → Advance（幂等心脏）：读 frn 行 + 冻结图 → walk 算 ready（统一 join 规则、
     从已落库决策重推活跃子图、回边推进轮次）→ runNode：
       action/agent → 4 端口派发（ctx 带 flowrunID → 执行实体审计列）
       control/approval → 内联求值（first-true-wins / 渲染后 park）
     → record-once 写行 → 循环到无人 ready → finalize
 → 审批：parked 行即收件箱；人工决策 vs 超时 first-wins；:replay 清 failed 行重走
 → 崩溃恢复 = boot 时对每个 running run 再调一遍 Advance
```

### ③ 一次构建（AI 造实体）的一生

```
LLM 调 create_function（ops 数组，jsonrepair 容错）
 → ApplyOps：逐 op 应用 + 每步校验 + 终校验（词法检查 + import 黑名单：无状态/有状态边界）
 → 写 Function + v1（active 指针）——立即生效、无审批态
 → ensureEnv：envfix 自愈循环（装不上 → LLM 改依赖重试 ≤3）→ env 状态镜像回版本行
 → relation 边（conversation→function create 边）+ 通知 + entities 流 build 镜像
   （编辑=新版本+移指针；revert=纯指针移动；版本 cap 50 放过 active）
```

## 4. 全局统一的横切机制（每个域都遵守）

- **workspace 隔离链**：HTTP 中间件注入 → ctx 一路下传 → orm 自动过滤/填充（D2）；异步用 `reqctx.Detached(wsID)` 重播种；**后台入口逐 workspace 播种**（`forEachWorkspace` 铁律 + 守护测试）。
- **错误系统**：一个类型（`pkg/errors`）、一种造法（`errorspkg.New(kind, code, msg)`）、285 码全 registry 登记；HTTP 读 Kind/Code、LLM 读 Message；机械守卫防回退（`standard_test.go`：sentinel 全用 errorspkg.New · 码全库唯一 · transport 走 FromDomainError）。
- **版本模型（方案 A，全实体统一）**：线性只增版本 + 自由 active 指针；无 pending/accept；revert=移指针；Trim cap 50 放过 active；**create（实体行+v1）与 edit（新版本+移指针）各为单事务**（store 复合方法 CreateWithVersion / SaveVersionAndActivate——不留无版本实体或孤儿版本+旧指针）。
- **执行审计（四执行单元统一）**：Log 表只增（D1）+ 溯源 5 列（conversation/message/toolCall 由 loop 注入 ctx；flowrun 2 列由调度器派发注入）+ Detached 记账（被取消也落账）。
- **ID 体系**：`<prefix>_<16hex>`（S15）；infra 侧自有前缀（fnenv_/hdenv_）。
- **三条 SSE 流（E1 封顶）**：messages（对话）/ entities（实体面板）/ notifications（通知中心）；durable 帧入 replay 环、ephemeral 即发即丢（E2）；嵌套靠 ParentID（E3）。
- **DIP 后注入**：跨实体协作全走端口（SetXxx 后注入破环）；bootstrap 是唯一全知者。

## 5. 阅读路径建议

1. 本篇 → 2. [scheduler-flowrun](foundation/scheduler-flowrun.md)（平台灵魂）→ 3. [chat](domains/chat.md) + [loop](foundation/loop.md)（交互主线）→ 4. 你要动的那个域的文档 → 5. 改代码前查四索引对账（改完同提交更新——CLAUDE.md 文档纪律）。
