# Forge Redesign — Trinity Architecture

> ✅ **完工 2026-05-13**(Plan 01-06 全交付)
> V1.2 后端期完整重做。从 forge 单 domain → Trinity(Function / Handler / Workflow)+ 执行 plane(Scheduler / Trigger / FlowRun)+ 多 agent 锻造 + 14 项生产硬化。

---

## 📚 文档导航

### Spec 系列(设计)
| 文档 | 内容 |
|---|---|
| [00-overview.md](./00-overview.md) | 顶层愿景 + 21 D 决策 + D-redo 改动(2026-05-12 second pass) |
| [01-shared-tool-interface.md](./01-shared-tool-interface.md) | 21 LLM tool 矩阵 + 9 方法 Tool 接口 + StandardFields(summary/destructive/execution_group)|
| [02-function.md](./02-function.md) | Function domain(无状态 Python def)详细 |
| [03-handler.md](./03-handler.md) | Handler domain(stateful Python class + caller-owns instance lifetime)详细 |
| [04-workflow.md](./04-workflow.md) | Workflow domain(DAG + 13 节点 + 9 ops)详细 |
| [05-execution-plane.md](./05-execution-plane.md) | Scheduler + Trigger + FlowRun + 14 hardening 详细 |
| [06-subagent-forging.md](./06-subagent-forging.md) | 多 agent 并行锻造模型(pure prompt-driven + D21 filterTools strip)|
| [07-notifications-and-eventlog.md](./07-notifications-and-eventlog.md) | 通知 + eventlog scope 三流(eventlog / notifications / forge)|
| [08-executions.md](./08-executions.md) | D22 共享 16 字段模板 + 5 表 schema + cross-table linking |

### Plan 系列(实施)
| Plan | 范围 | 实施日期 | 状态 |
|---|---|---|---|
| [plans/01-function-domain.md](./plans/01-function-domain.md) | function trinity 第一条腿(替代 forge)| 2026-05-11 | ✅ |
| [plans/02-handler-domain.md](./plans/02-handler-domain.md) | handler trinity 第二条腿(stateful class)| 2026-05-12 | ✅ |
| [plans/03-eventlog-and-transport.md](./plans/03-eventlog-and-transport.md) | eventlog + forge 三流 + env 模型重整 | 2026-05-12 | ✅ |
| [plans/04-workflow-authoring.md](./plans/04-workflow-authoring.md) | workflow trinity 第三条腿(DAG authoring)| 2026-05-12 | ✅ |
| [plans/05-execution-plane.md](./plans/05-execution-plane.md) | scheduler + trigger + flowrun + 14 hardening | 2026-05-13 | ✅ |
| [plans/06-subagent-catalog-e2e.md](./plans/06-subagent-catalog-e2e.md) | subagent forger + catalog ext + E2E + 收尾 | 2026-05-13 | ✅ |

### 讨论记录(decision diff)
- [discussions/2026-05-12-env-and-sse-rework.md](./discussions/2026-05-12-env-and-sse-rework.md) — 26 项 D-redo 决策(env 模型 + SSE 三流 + iterate-same-pending + slim notif)

---

## 🏗️ 完工时数字

| 维度 | 数值 |
|---|---|
| Commits | 70+(Plan 01:13 / Plan 02:11 / Plan 03:6 / Plan 04:9 / Plan 05:17 / Plan 06:5)|
| Code 净增 | ~22,000 LOC backend |
| Domain 数 | 5 个新(function/handler/workflow/flowrun/trigger)+ 1 删(forge)|
| Service 数 | 6 个新(functionapp/handlerapp/workflowapp/schedulerapp/triggerapp + ProductionChecker)|
| HTTP endpoints | 50+ 新(function 13 / handler 17 / workflow 11 / flowrun 5 + 共享 :trigger + /triggers)|
| LLM tools | 33 trinity(function 9 / handler 10 / workflow 8 / D22 6 总)+ 系统 tool 不变 |
| DB 表 | 4 新(flowruns / flowrun_nodes / mcp_calls / skill_executions)+ trinity 三 domain 6 表 |
| 测试 | ~310 unit + 40 pipeline(Plan 01-06 累计加 ~118 测试)|
| D 决策 | 23 个 D + 26 个 D-redo 全落地 |
| 项目耗时 | 2 周(2026-04-28 spec 起草 → 2026-05-13 全 merge)|

---

## 🎯 完工后能力

### chat 主对话端用户能
- ✅ 造 Function(无状态 Python def + AST 校验 + 同步装 venv + LLM env-fix loop)
- ✅ 造 Handler(stateful Python class + AES-GCM init args + caller-owns instance + driver.py JSON-RPC)
- ✅ 造 Workflow(DAG + 13 节点类型 + 9 ops + Kahn cycle 检测 + CapabilityChecker)
- ✅ 多 agent 并行锻造(主 chat LLM spawn forger sub-agent;sub-agent 无 workflow ops D21)
- ✅ 触发 workflow(cron / fsnotify / webhook / manual)
- ✅ 看 run 状态(running / paused / completed / failed / cancelled)+ 取消运行中 run
- ✅ Approval 节点跨进程重启 rehydrate(§6.1)
- ✅ 14 项生产 hardening(详 [05-execution-plane.md §6](./05-execution-plane.md#6-14-项生产级-v1-必做项))

### LLM 工具诊断面
- ✅ D22 5 张 execution log 表(function/handler/mcp/skill/flowrun_nodes)
- ✅ 10 个 D22 per-entity 工具(search + get × 5 域)
- ✅ Cross-table linking 经 `flowrun_node_id` 字段(capability 节点 dispatch 时跨表写两条)

### 协议层
- ✅ SSE 三流 per-user(eventlog + notifications + forge)client demux per payload
- ✅ HTTP/2 + TLS(connection limit 解,详 Plan 03 D18)
- ✅ entity-level eventlog scope(function:fn_xxx / handler:hd_xxx / workflow:wf_xxx / flowrun:fr_xxx / conversation:cv_xxx)
- ✅ slim notification payload(D-redo-6 — UI 经 GET refetch 详情)
- ✅ HTTP envelope `{data}` / `{error: {code, message}}` 统一

---

## 🚀 下一阶段:V1.2 桌面端 Wails 迁移

V1.2 后端期收尾,实际可交付 V1。下一阶段:
- Wails 桌面 app 外壳(复用 httpapi,不走 Wails native binding;详 [`desktop-packaging-notes.md`](../../desktop-packaging-notes.md))
- Phase 5(智能化)— knowledge / document / intent / chat 终极版

---

## 🔖 历史脉络

- **2026-04-23** — V1.2 routing:从"V1.0 重写"升级到 Agentic Workflow Platform 完整愿景
- **2026-04-28** — forge_redesign spec 起草(00-08 spec 文档 + 6 plans)
- **2026-05-12** — Plan 03 D-redo 二次评审(26 项决策修订 — env 模型 / SSE 三流 / iterate-same-pending)
- **2026-05-13** — Plan 05 + Plan 06 完工 ✅(forge_redesign 全交付)
