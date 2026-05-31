# Forge Redesign — Trinity Architecture

**创建于**:2026-05-10
**状态**:Plan 01 (function) + Plan 02 (handler) 已 merge;Plan 03 (eventlog/transport) 大幅修订(见下)
**关联**:
- 上层路线 — `documents/version-1.2/backend-design.md` Phase 4 / forge 重做
- 现状 forge — 完全推倒重做(已删除 forge domain/store/app/tool/HTTP/table 全套)
- **📌 2026-05-12 redesign**:[`discussions/2026-05-12-env-and-sse-rework.md`](./discussions/2026-05-12-env-and-sse-rework.md) — env 模型(EnvID=versionID + tool 内同步装 + env-fix loop)+ SSE 三流统一(user_id 订阅 + Wails native event 取代 TLS)。**此后所有 env / SSE 决策以该文档为事实源**;本文 §3 D 系列若有冲突,以 D-redo-* 为准

---

## 1. 一句话愿景

提供 **三种粒度的可锻造产物**,让用户用自然语言定义自己的专属能力。Forgify 不预设用户做什么,LLM 根据需求从三种粒度选最合适的来锻造、组装、交付。

| 粒度 | 名字 | 本质 | 例子 |
|---|---|---|---|
| 最轻 | **Function** | 一次性、无状态的 Python 函数 | "把这段 markdown 转成 PDF" |
| 中等 | **Handler** | 常驻的 Python 服务对象,挂多个 method | "PostgreSQL 连接器,暴露 query/insert/migrate" |
| 最重 | **Workflow** | 触发器 + DAG + 控制流的编排组合体 | "每小时扫邮箱,猎头线索写库 + WhatsApp 通知" |

LLM 根据用户措辞判断粒度,用户可以调整。三类形态的 LLM 工具接口高度统一,LLM 学一类会用其他两类。

---

## 2. 与现有系统的关系

### 2.1 完全替代 forge

现有 `forgedomain.Forge` / `forge_executions` 表 / `app/forge/*` / `app/tool/forge/*` / 5 个 forge LLM tool / 22 个 forge HTTP 端点 **全部废弃**。Function 是新的等价物(也是 Python sandbox 函数),但 entity / table / 工具 / API 全套重新设计,**不背 forge 历史包袱**。

### 2.2 覆盖 Phase 4 plan

原 backend-design.md Phase 4 路线图列的 `workflow / flowrun / scheduler / trigger` 4 domain 在本次重做范围内详细化。节点类型从原 5 种(LLM/Tool/Trigger/Approval/Variable)扩到 13 种,加 14 项生产级 V1 必做项。

### 2.3 与现有系统集成

| 现有组件 | 集成方式 |
|---|---|
| `app/loop` ReAct 引擎 | **复用,不动**(三类 LLM 工具调用都走 loop) |
| `app/sandbox` v2 | **复用**(Python runtime + per-instance env) |
| `app/subagent` | **扩展**(加 4 类 forger subagent type) |
| `app/catalog` | **扩展**(加 function / handler 两 source;workflow 不进) |
| `domain/eventlog` 协议 | **泛化**(scope 从 conversationId 泛化到 conversation \| flowrun) |
| `domain/notifications` | **扩展**(加 flowrun_failed 等 type) |
| `app/mcp` / `app/skill` | **不变**(仍是独立 domain,跟 trinity 平行) |

---

## 3. 核心架构决策(摘要)

详见各域文档,这里只列结论。

### D1 — 三类产物分层
- **Function** = Python 沙箱函数(stateless,一次性)— 详见 [`02-function.md`](./02-function.md)
- **Handler** = Python 类 + 多 method(stateful,Definition + Instance 二层)— 详见 [`03-handler.md`](./03-handler.md)
- **Workflow** = DAG + 触发器(authoring 与 execution 分 plane)— 详见 [`04-workflow.md`](./04-workflow.md)

### D2 — Handler 独立 domain,不折叠到 MCP
理由:lifecycle 哲学不同(MCP 是 daemon,Handler 是 instance)、质量 / 文档 / 协议稳定性可控性差距大。**协议可参考,代码不复用**。详见 [`03-handler.md`](./03-handler.md) §1。

### D3 — Handler Instance lifetime 走 caller-owns 模型
Definition + Instance 二层。Instance 的 owner = 发起它的 caller-context:
- chat conv → conv-bound + 闲置 GC
- FlowRun → run-bound + run 结束 cleanup
- test execution → ephemeral

Definition 上**不加 lifetime 字段**(纯净),系统按 caller-context 自动决定。详见 [`03-handler.md`](./03-handler.md) §3。

### D4 — 三类产物 LLM 工具接口形态高度统一
**7 actions × 3 kinds = 21 tools**。create / edit 都走 ops-driven 流式模式。args / return / 流式协议三类一致,LLM 心智负担骤降。详见 [`01-shared-tool-interface.md`](./01-shared-tool-interface.md)。

### D5 — 代码不在三类 domain 间复用
**统一 SHAPE 不等于统一 IMPL**。每 domain 自己写 `apply.go`(~80 行),switch op type;**不抽共享 helper**。改 function 的 op 实现不会牵动 workflow。

### D6 — Workflow 锻造 vs 执行 — 两个 plane
- **authoring plane** 关心图怎么样(workflow domain)
- **execution plane** 关心图怎么跑(scheduler / trigger / flowrun)

workflow domain 边界干净,只 2 张表;执行 plane 各自独立 domain。详见 [`04-workflow.md`](./04-workflow.md) §1 / [`05-execution-plane.md`](./05-execution-plane.md)。

### D7 — Function 内不能调 Handler
Function 是纯叶子节点(stateless),组合靠 workflow 节点编排,**不允许 Function 内 import Handler client**。简单 + 防递归 + 强制可视编排。详见 [`02-function.md`](./02-function.md) §6。

### D8 — 多 agent 并行锻造收成纯配置
不需要新 architecture。复用现有 subagent 基础设施,加 4 类 forger subagent type(function-forger / handler-forger / workflow-forger / decomposer),配主 agent 协作 prompt。详见 [`06-subagent-forging.md`](./06-subagent-forging.md)。

### D9 — Catalog 取舍
- **Function** ✅ 进 catalog
- **Handler** ✅ 进 catalog
- **Workflow** ❌ 不进(trigger-driven,LLM 主对话不该现场调用)

### D10 — Workflow 节点 13 种 + onError / retry config
13 节点 = trigger / function / handler / mcp / skill / llm / http / condition / loop / parallel / approval / wait / variable。每个 capability 节点(function/handler/mcp/skill/llm/http)带 `onError`(stop/continue/branch)+ `retry`(maxAttempts/backoff)config。详见 [`04-workflow.md`](./04-workflow.md) §2。

### D11 — MCP 不可用走 fail-fast → retry → onError(Topic 3-B)
workflow `mcp` 节点撞 server 不健康(disconnected / degraded / failed)→ 立刻返错 → 走节点 retry config → 仍失败走 onError。**workflow 不参与 MCP lifecycle 管理**(MCP 自有 self-heal §5.6 处理 transient 故障)。详见 [`04-workflow.md`](./04-workflow.md) §11 / [`05-execution-plane.md`](./05-execution-plane.md)。

### D12 — Workflow 不管 MCP server lifecycle(Topic 3-C)
两个 NO:
- **NO auto-restart** — server 被用户主动停 = intentional,workflow 不重启,fail-fast
- **NO auto-install** — workflow create / edit 时校验所有 `mcp` 节点 serverName 在已安装列表(否则 reject `WORKFLOW_MCP_SERVER_NOT_INSTALLED`);LLM 自行 `install_mcp_server` 前置依赖

附:server 卸载 → broadcast `mcp_server_uninstalled` 通知 → 引用它的 workflow 标 `needs_attention`(类比 forge `EnvStatus=evicted`)。

### D13 — Selection-as-message-metadata(Topic 4-A)
用户在 UI 圈选某节点 / 某代码段 → 前端把选中信息**作为结构化 metadata 附在 chat message 上** → 后端织入 LLM prompt → LLM 用 `edit_*` 发 targeted ops。**无新 LLM 工具**,只扩 chat.message + system prompt 模板。详见 [`01-shared-tool-interface.md`](./01-shared-tool-interface.md) §15。

### D14 — Function parameters schema LLM 自报(Topic 5-A)
LLM 同时 emit `set_code` + `set_parameters` ops,后端校验 Python 函数签名 vs 声明的 parameters 必须**一致**(参数名 / required / default / type),不一致整批 reject。**不依赖 forge 现有 AST 提取**(D5 不复用 forge)。详见 [`02-function.md`](./02-function.md) §3。

### D15 — Handler 走 method-level ops(Topic 5-B.1,跟 workflow 一致)
Handler class 由系统按 ops **拼装**,LLM 不写整 class。Op 集合:`set_imports / set_init / set_shutdown / set_init_args_schema / add_method(带 body) / update_method / delete_method`。改 1 个 method 不动其他,跟 workflow 节点级 ops 心智一致。详见 [`03-handler.md`](./03-handler.md) §4 / §5。

### D16 — Handler Config 模型(Topic 5-B.2)
init_args(DSN / API key 等)由用户在 Handler 详情页填一次,**`handlers.config_encrypted` AES-GCM 加密存**,spawn 时透明注入 Python `__init__`。LLM 缺 config 时收 `HANDLER_CONFIG_INCOMPLETE` 422 → 用 `AskUserQuestion` 收用户输入 → `update_handler_config` 写回 → retry。**敏感字段** `sensitive: true` 走密码框 + 永不返明文 + 日志过滤。复用 `infra/crypto.AESGCMEncryptor` + machine fingerprint(同 apikey domain)。详见 [`03-handler.md`](./03-handler.md) §6.5。

### D17 — V1 不做 test_cases(Topic 5-C)
Function / Handler **不带 test_cases 子表 / LLM 工具家族**。LLM 锻造完跑 1-2 次 happy-path 自验,workflow 编排里也能做 condition + assert。V1.5 看用户反馈再加。

### D18 — Transport Layer 保持 HTTP/1.1(TLS / HTTP/2 永久搁置)
Backend 保持纯 HTTP(无 TLS / 无 HTTP/2);浏览器 HTTP/1.1 6-conn 限制由 **D-redo-1 + D-redo-2 三流统一**解决(只占 3 个连接,远低于 6)。mkcert / TLS cert / 证书轮换全部不需要。打包阶段如确实需要绕开 HTTP,走 **Wails native events**(V1.5 / 打包期实施)。详 [`07-notifications-and-eventlog.md`](./07-notifications-and-eventlog.md) §2 + [`discussions/2026-05-12-env-and-sse-rework.md`](./discussions/2026-05-12-env-and-sse-rework.md) §A。

### D19 — SSE 三流统一按 user_id 订阅
后端 3 条 SSE 流(eventlog / notifications / forge)**全部按 user_id key**,无 query 参数。eventlog payload 带 `conversationId` 给 client demux,forge payload 带 `scope:{kind,id}` struct(嵌套,复用 `domain/eventlog.Scope`)。**永远不再加新 SSE 流**(D-redo-5)— 所有未来"entity 详情面板想看实时事件"需求走 forge 流 + client filter 或 Wails native event。详 [`07-notifications-and-eventlog.md`](./07-notifications-and-eventlog.md)。

### D20 — Capability 删除级联标 workflow `needs_attention`(D12 扩展)
删除 function / handler / skill / mcp_server 时,引用此 capability 的 workflow 自动标 `needs_attention` + 发通知。trigger 仍 register 但触发时 fail-fast(`WORKFLOW_CAPABILITY_REMOVED`),UI 列表项显示警告。用户决定:`edit_workflow` 替换 / `delete_workflow` / 配相应替代品。详 [`07-notifications-and-eventlog.md`](./07-notifications-and-eventlog.md) §5。

### D21 — Sub-agent 不能控 workflow domain
Sub-agent(`general-purpose` 默认)继承父 tool registry 时,**filterTools 额外 strip workflow mutation + execution ops**(`create_workflow` / `edit_workflow` / `delete_workflow` / `revert_workflow` / `trigger_workflow`)。保留 `search_workflow` / `get_workflow` 只读 + `call_handler` / `run_function` 自测能力。理由:workflow 是用户高可见 orchestration entity,装配 + 触发是主 agent + 用户对话的责任,不让 sub-agent 偷改 / 偷跑产生外部副作用。**跟 D7 同精神**(显式 domain 分工 > 默认全权限)。详见 [`06-subagent-forging.md`](./06-subagent-forging.md) §5。

**附:不引入新 SubagentType** — 之前推 4 forger types(function-forger / handler-forger / workflow-forger / decomposer)被简化掉了。改用现有 `general-purpose`(general 任务)+ `Explore`(只读 plan 角色)+ 主 agent prompt instructions 控制行为。0 行 SubagentType 注册改动,~40 行 filterTools + prompt 教学。

### D22 — 执行记录:5 张 per-entity 表(共享 schema 模板)+ 10 套 per-entity LLM 工具
每类 capability(function / handler / mcp / skill / workflow 节点)每次执行落一行 execution log。**5 张表 schema 模板完全统一**(16 通用字段 — id / user_id / status / triggered_by / input / output / elapsed / 时间戳 / chat 上下文 / workflow 上下文),kind-specific 字段各表自加(handler_calls 加 method;mcp_calls 加 server_name 等)。**不走 unified attrs JSON**(reviewer 心智重)— per-entity 表 self-documenting + 跨实体查通过 conversation_id / flowrun_id 索引。

**LLM 工具同样 per-entity**:**5 张表 × 2 工具(search + get) = 10 个**,各自在 `app/tool/<kind>/` 实现,跟该域其他工具同包(per D5 不抽共享)。trinity 3 个(function/handler/workflow)进 01 §1 matrix 第 8/9 row;平行 mcp/skill 同模式不在 matrix。

Production observability + LLM 自诊断必备(看不只 status=failed,**也看 status=ok 但 output 不对**)。详见 [`08-executions.md`](./08-executions.md)。

**bug fix 顺手**:FlowRun.status enum 删 `timeout` 值(V1 没 run-level 总超时;节点 timeout 致 run 终止时,run.status=failed + error_code=NODE_TIMEOUT)。FlowRunNode.status 跟通用 execution status 一致 — 含 `timeout`(节点超时是真状态)。

---

## 4. V1 范围

### 4.1 V1 必做

#### 后端 domain
- **Function** domain(完全替代 forge)+ DB + LLM 工具 + HTTP API
- **Handler** domain(全新)+ caller-owns instance 模型 + DB + LLM 工具 + HTTP API
- **Workflow** domain(authoring)+ 13 种节点类型 + ops 编辑 + 校验 + DB + LLM 工具 + HTTP API
- **Scheduler** domain — 执行 + 14 项生产级 V1 必做项(详见 [`05-execution-plane.md`](./05-execution-plane.md) §6)
- **Trigger** domain — cron / fsnotify / webhook / manual 4 种触发器
- **FlowRun** domain — 执行记录持久化 + HTTP CRUD

#### 系统级扩展
- **eventlog 协议泛化** — scope = `conversation | flowrun` 二选一
- **catalog 加 source** — function + handler
- **subagent 加 forger types** — 4 类 forger 注册 + 主 agent prompt
- **notifications 加 type** — flowrun_failed / flowrun_succeeded 等

### 4.2 V1.5 推迟
- Workflow nested workflow(subworkflow)— 用户复制粘贴够用
- Workflow 级 timeout / Run 总超时
- Approval 节点跨进程重启时 handler instance state preservation(V1 已 rehydrate paused state,但 instance 销毁)
- 节点扩展:switch / transform / aggregator / map / filter / reduce
- Event-bus trigger(workflow 间互相唤醒)
- Checkpoint / resume(任意节点中途持久化)
- Dry-run / replay
- Budget cap(token / cost 上限)
- Workflow 进 catalog(看 V1 用户反馈)

### 4.3 V2 推迟
- Function 多形态(SQL / HTTP / shell)— V1 限 Python,Handler 吃多形态需求
- Handler 跨 conv 共享 instance pool(预热 5GB embedding model 等场景)
- Distributed execution(横向扩展)
- 边上 inline transform / filter
- Streaming between nodes(节点边产边喂下游)

---

## 5. 工程量预估

### 5.1 新 / 重做的 domain

| Domain | 文件数估 | LOC 估 | 备注 |
|---|---|---|---|
| function | ~20 | ~3000 | 替代现有 forge |
| handler | ~20 | ~3500 | 比 function 复杂(Instance lifecycle + RPC) |
| workflow | ~25 | ~5000 | 13 节点 + ops + 校验 + 持久化 |
| scheduler | ~10 | ~2500 | 执行 + 14 production 项 |
| trigger | ~12 | ~2000 | 4 种触发器各自实现 |
| flowrun | ~8 | ~1200 | 持久化 + HTTP |

### 5.2 改动 / 扩展
- `app/loop` — 复用,不动(可能 minor 适配 ops 流式 emit)
- `app/sandbox` — 复用,Python EnvManager 已有
- `app/loop` — `filterTools` 加 strip workflow ops(D21,~10 行);**不加新 SubagentType**(D8 简化,纯 prompt-driven)
- `app/catalog` — 加 function / handler source
- `domain/eventlog` + `infra/eventlog` — scope 泛化
- `domain/notifications` — 加 flowrun_failed 等 type
- `transport/httpapi` — 加 ~40 个新端点

### 5.3 删除
- `app/forge/*` 整个 — function 替代
- `app/tool/forge/*` — function 替代
- `domain/forge/*` — function 替代
- `infra/store/forge/*` — function 替代
- `forges` / `forge_versions` / `forge_test_cases` / `forge_executions` 4 表 + 历史数据
- 22 个 forge HTTP 端点

### 5.4 总量

~17000 LOC 净增 + ~5000 LOC 删除 = **~22000 LOC 改动**。**预估 6-8 周(单人)**。

---

## 6. 既定开发顺序(拟)

按依赖自下而上,每步完成后跑 pipeline test 验证:

1. **Function domain** — 替代 forge;LLM 工具家族最熟,先把基础替换掉
2. **Handler domain** — Instance lifecycle 是新概念,需要 caller-owns infra
3. **Eventlog scope 泛化** — 为后续 workflow 流式准备
4. **Workflow domain**(authoring 那一面)— DAG + 13 节点 + ops 编辑
5. **FlowRun domain** — 执行记录
6. **Scheduler + Trigger** — 执行编排 + 4 种触发器 + 14 项生产级配置
7. **Subagent forger types** — 4 类 forger 注册 + 主 agent prompt 教学
8. **Catalog source 加** — function + handler 两 CatalogSource
9. **End-to-end pipeline tests + 14 项生产级 V1 检查表跑通**

每步交付时同步更新 `service-contract-documents/*.md`(api / database / error / events)+ `progress-record.md`(per S14)。

---

## 7. 文档导航

| 文件 | 内容 |
|---|---|
| [`00-overview.md`](./00-overview.md) | **本文件** — 顶层愿景 + 决策摘要 + V1 scope + 工程量 |
| [`01-shared-tool-interface.md`](./01-shared-tool-interface.md) | 21 LLM tools matrix + ops shapes + 流式协议 + catalog 取舍 + Selection Metadata |
| [`02-function.md`](./02-function.md) | Function domain 详设计(Python 函数 + DB + LLM 锻造) |
| [`03-handler.md`](./03-handler.md) | Handler domain 详设计(Definition/Instance + caller-owns lifetime + Handler Config) |
| [`04-workflow.md`](./04-workflow.md) | Workflow domain 详设计(DAG + 13 节点 + edge + 表达式) |
| [`05-execution-plane.md`](./05-execution-plane.md) | Scheduler + Trigger + FlowRun + 14 项生产级 V1 必做(原 Transport Layer 章节已被 D-redo-1 取代,内容搁置)|
| [`06-subagent-forging.md`](./06-subagent-forging.md) | 多 agent 并行锻造(4 类 forger + 协作模式 + Config 引导) |
| [`07-notifications-and-eventlog.md`](./07-notifications-and-eventlog.md) | 通知 type 总表 + Eventlog scope 多视图订阅策略 |
| [`08-executions.md`](./08-executions.md) | 5 张 per-entity execution log 表(共享 schema 模板)+ LLM 诊断工具(D22)|

---

## 8. 不在范围

**本轮不做**:
- ❌ Knowledge base / RAG 节点(原 Phase 5)— Workflow `llm` 节点 config 上 `knowledgeBaseId` 字段**预留空位**,实际 RAG 实现推 Phase 5
- ❌ 前端实现 — V1.2 后端期不动前端(per CLAUDE.md §设计原则 #4),所有可视化协议设计完后,前端按 spec 在 Wails 迁移阶段一起做
- ❌ MCP / Skill 重做 — 仍按现有设计走,不在本轮触碰
- ❌ Workflow 多租户 — 单用户本地场景,user_id 仍硬编码 `local-user`
- ❌ Workflow 部署到云 / 分布式执行 — V2 之后看需求

---

## 9. 风险与开放问题

### 已识别并缓解
- **进程重启 paused run 丢失**(V1 用 paused_state JSON rehydrate)
- **cron 漏触发**(V1 用 missedPolicy=runOnce + last_fired_at 持久化)
- **同 wf 多 run 竞争**(V1 默认 `concurrency: serial`)
- **Function 内引用 Handler 的递归风险**(D7:Function 不允许调 Handler)
- **Handler instance leak**(V1 caller-context 强约束 + 进程退出统一 cleanup)

### 仍需 V1.5 / V2 解决
- Run 总超时(V1 节点级超时是临时 mitigation)
- 长跑 paused workflow 跨重启的 Handler instance state(V1 paused 时 destroy instance)
- 用户写的 Python Handler / Function 性能 / 安全风险(沙箱隔离 + dependency 白名单 V2 加)

### 真正未决
- Cron 时区定义(V1 锁本地 `time.Local`,跨时区用户场景如何 — 实际本地桌面 app 不应有此问题)
- Workflow 版本上限策略(V1 forge 是 50 条/forge 硬删最旧,workflow 沿用)

---

(本文档完)
