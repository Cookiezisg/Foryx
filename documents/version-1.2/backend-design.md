# Backend 全新重写 — 契约优先 + 分层架构 + Agentic Workflow Platform

**创建于**：2026-04-22
**分支**：`backend-iteration`
**当前进度 / 开发日志**：[`progress-record.md`](./progress-record.md)

**本文档定位**：**项目愿景 + 架构 + Phase 路线图**。**所有代码规范、工程纪律、设计原则、S/T 系列、工具纪律全部在项目根 [`CLAUDE.md`](../../CLAUDE.md)**——这里只放"项目长什么样、怎么走"，不重复规则。

---

## Context — 为什么重构

经过对 Forgify 后端 + DB + SSE + 前端调用的全面审计，现有代码存在系统性架构债：

- **HTTP API**（45 端点）一致性 3.2/10：响应结构各异、0/45 端点有分页、REST 动词乱用、字段命名混用
- **DB schema**（10 表）健康度 5.8/10：软删除 3 种风格并存、关键 UNIQUE/FK 约束缺失、被引用的 `workflow` 表不存在
- **SSE 事件**（21 定义）一致性 3/10：14/21 是死事件、载荷多种形态、字段名混乱
- **架构**：handler 直接写 SQL、`ToolService` 是 29 方法 696 行的 god object、`routes_chat.go` 一个文件装 7 个责任

目标：**地基先打好**，再往上长。

---

## Strategy — 契约优先 + Green-field 重写 + 原子切换（已完成）

1. **第一阶段**（`backend-iteration` 分支，2026-04-22 ~ 2026-04-25）：在 `backend-new/` 全新写代码 + 配完整测试 → 与旧 `backend/` 并存 → 验证通过后**原子切换**（删 `backend/`，改名 `backend-new/` → `backend/`）。**已完成**——目录现在就是单份的 `backend/`，旧实现归档于 `legacy/`。
2. **第二阶段**（进行中）：后端 Phase 0-4 已交付、Phase 5 部分落地，**当前重心转入前端**——按 [`frontend-prd.md`](./frontend-prd.md) + boilerplate 开发（见 CLAUDE.md 末节"前端开发守则"）。

**后端 Phase 0-4 已定型；Phase 5 智能化部分交付（document / mcp / skill / memory / compaction ✅，intent / chat 终极版未做）。前端现按 PRD 节奏开发。**

---

## 产品愿景（Phase 2 起）

Forgify 不只是"对话 + 造工具"—目标是 **Agentic Workflow Platform**：用户一句话能编排出工作流，工作流由多种节点构成，可挂文档库做 LLM-ranked attach（无 RAG，详 [`final-sweep.md`](./final-sweep.md) §14 2026-05-16 设计改向），最终由调度器部署运行。

### 核心能力清单

1. **意图识别 / Intent Routing**：聊天时识别用户想干啥（创建工作流？改工具？更新知识库？纯问答？）
2. **工作流引擎**：节点 + 边的 DAG，能跑、有运行历史
3. **多种节点类型**：用户工具 / MCP 工具 / LLM 节点 / Skill / 知识库检索 / 触发器 / 审批
4. **文档库**：上传文档 → LLM-ranked 选择 → 全文 / 章节 attach，挂在 LLM 或工作流节点上（**无 RAG / 向量检索**——本地单用户文档量小 + 大 context + prompt cache 让"塞全文"反超 RAG；详 [`final-sweep.md`](./final-sweep.md) §14 2026-05-16 设计改向）
5. **MCP 集成**：接 Anthropic 的 MCP 服务器，第三方能力即插即用
6. **调度部署**：cron / 文件触发 / Webhook 触发
7. **Skill 系统**：预制 + 元数据完善的能力模板（V1 浅版即可）

### 业界对标

| 产品 | 对标的能力 |
|---|---|
| **Dify** | 工作流 + 知识库 + Agent |
| **Coze**（字节）| Bot + 工作流 + 插件 / Skill |
| **n8n + AI 节点** | 通用工作流 + AI |
| **Langflow / Flowise** | 可视化 LLM pipeline |

定位：**桌面版 + 中文场景优化** — 在锻造工具 + 离线运行上做差异化。

### LLM 客户端策略（2026-04-27 更新）

Eino 框架已完全移除（`infra/eino/` 目录删除，go.mod 中 Eino 依赖全部清除）。
改用完全自有的 `infra/llm` 包（4 文件，OpenAI-compat + Anthropic 原生，iter.Seq 流式）。

| 能力 | 方案 |
|---|---|
| LLM 流式客户端 | 自有 `infra/llm`（openai.go + anthropic.go + factory.go）|
| ReAct 循环 | `app/loop`（V1.2 D3 抽出的通用引擎，Host 接口 + Run 函数）；chat / subagent / Skill fork / workflow LLM 节点都是调用方，不再各自一份 |
| Tool 接口 | `app/tool/tool.go` 9 方法接口 + summary/destructive 标准字段注入机制（详见 CLAUDE.md §S18）|
| Workflow Engine | Phase 4 自实现（不依赖 Eino compose）|
| Cron 调度 | `robfig/cron`（Phase 4）|
| MCP 集成 | `modelcontextprotocol/go-sdk` v1.6（**官方** SDK，V1.2 D5+D6 已交付——提前到 Phase 4 准备件，不等到 Phase 5）|
| Python 沙箱 | subprocess `infra/sandbox`（已有）|

---

## Phase 路线图

**当前状态 / 任务细化** → [`progress-record.md`](./progress-record.md)

| Phase | 主题 | 工时 | 完成后产品形态 | 状态 |
|---|---|---|---|---|
| 0-1 | 地基 | 10h | 基础设施全就位 | ✅ 2026-04-23 |
| 2 | 基础对话 | 11h | ChatGPT 客户端 | ✅ 2026-04-25 |
| 3 | 工具锻造 | 12h | Forgify V1.0 体验 | ✅ 2026-04-26 |
| — | Phase 3 后基础设施优化轮 | — | chat 重构 + 调研 + 驱动迁移 + 桌面端方向 | ✅ 2026-04-27 ~ 2026-05-06 |
| — | **Phase 4 准备件**（D2-D9）| — | sandbox v2 (mise) + subagent + mcp + skill + catalog + 跨 cutting 集成测试 | ✅ 2026-05-06 |
| — | **Windows 代码层适配**（D10-D15）| — | Bash/cmd.exe + 5-platform vet + mise.exe embed + Wails 包文档；真 Windows runtime 验证待物理机 | ✅ 2026-05-06 |
| 4 | 工作流 | 20h | 桌面版 Coze | ✅ 2026-05-13（见下方 Phase 4 正文 :115；执行引擎在 `app/scheduler`，~2587 行）|
| 5 | 智能 + 知识库 + MCP | 15h | 完整 Agent 平台 | 🚧 部分交付（document / mcp / skill / memory / compaction ✅；intent / chat 终极版未做）|
| **合计** | | **~70h** | 完整愿景 | |

> Phase 6 原子切换（`backend-new/` → `backend/`）已在 Phase 2 收尾时内嵌完成（2026-04-25），不再单列。

### Phase 2 — 基础对话能力（已完成）

4 个 domain：`apikey`（凭证）+ `model`（场景 → provider/model 策略）+ `conversation`（对话 CRUD）+ `chat`（流式对话；Phase 2 时 `tools=nil`，Phase 3 起注入 system tools）。

**关键调用链**：
```
handler.SendMessage
  → chat.Send
      → model.PickForChat                       → (provider, modelID)
      → apikey.ResolveCredentials(provider)     → (key, baseURL)
      → llmFactory.Build(Config{...})           → llminfra.Client
      → buildHistory(ctx, convID, userMsgID)    → []LLMMessage
      → agentRun → client.Stream(Request)       → iter.Seq[StreamEvent] → SSE
```

### Phase 3 — 工具锻造能力 + 执行 plane + 多 agent 锻造(forge_redesign Plan 01-06 全交付)
`function` 主 domain(版本 / pending / sandbox 执行 / 执行日志 D22,12 端点)+ `handler` 二条腿(stateful Python class + caller-owns lifetime + Config + handler_calls D22,16 端点)+ `workflow` 三条腿(DAG 锻造 + 13 节点类型 + 9 op + Kahn cycle + CapabilityChecker,11 端点 — Plan 04)+ `app/tool/function/` 9 LLM 工具 + `app/tool/handler/` 10 LLM 工具 + `app/tool/workflow/` 6 LLM 工具 + chat ReAct 多步循环。Python 沙箱通过统一 PluginSandbox v2(mise embed)+ SandboxAdapter。

> Phase 3 历史:(1) 2026-05-02 第一轮 `tool` → `forge` 大重命名;(2) 2026-05-11 forge_redesign Plan 01 把 forge 重设为 trinity 域 Function 部分(13 commits);(3) 2026-05-12 Plan 02 handler trinity 第二条腿(11 commits);(4) 2026-05-12 Plan 03 eventlog + forge 三流统一(6 commits + 2 doc commits)— env 模型重整 + SSE 改三流 per-user + 删 :resync/env_synced/env_failed/ErrPendingConflict;(5) 2026-05-12 Plan 04 workflow authoring trinity 第三条腿(9 commits W1-W9);(6) 2026-05-13 **Plan 05 execution plane**(17 commits E1-E17)— scheduler + trigger + flowrun + 13 dispatcher + retry/timeout/onError + pause/resume + RehydrateOnBoot + 4 张新表 D22(mcp_calls / skill_executions / flowruns / flowrun_nodes)+ 6 D22 LLM 工具 + 14 项生产 hardening;(7) 2026-05-13 **Plan 06 trinity 收尾**(5 commits F1-F5)— D21 filterTools strip workflow ops + 主 chat agent multi-agent forging system prompt + trinity catalog 源验证 + approval lifecycle E2E + forge_redesign README。详见 [`adhoc-topic-documents/forge_redesign/README.md`](./adhoc-topic-documents/forge_redesign/README.md) 完工导航 + [`discussions/2026-05-12-env-and-sse-rework.md`](./adhoc-topic-documents/forge_redesign/discussions/2026-05-12-env-and-sse-rework.md) 26 项 D-redo 决策。

**Phase 3 后基础设施优化轮(2026-04-27 起,完工 2026-05-12)**:chat 基础设施重构(移除 Eino + Block 模型)/ chat pipeline.go → runner.go 二次重构 / Brewfile + Makefile setup / Claude Code 内部机制调研(9 份报告)/ SQLite 驱动迁移(mattn → modernc,纯 Go)/ 桌面端 Wails 分发方向定型 / 大规模代码 review 战役 / forge_redesign trinity 重做 + Plan 03 SSE 三流统一。详见 [`progress-record.md`](./progress-record.md) §2。

### Phase 4 — 工作流能力 ✅(已交付,2026-05-13)
`workflow`(DAG + 状态机) + `flowrun`(执行实例) + **13 类节点**(trigger / function / handler / mcp / skill / llm / http / condition / loop / parallel / approval / wait / variable)+ `scheduler` + `trigger`(cron / fsnotify / webhook / manual)+ `chat` 已支持"对话创建工作流"(主 agent multi-agent forging system prompt 教学)。执行引擎自实现(不依赖 Eino compose,Eino 已全面移除)。

**实际落地**:forge_redesign Plan 04(authoring) + Plan 05(execution plane)+ Plan 06(收尾)共 31 commits;详 [`adhoc-topic-documents/forge_redesign/README.md`](./adhoc-topic-documents/forge_redesign/README.md)。**焦点实体延伸**:workflow 节点编辑时推 `workflow` entity 通知 + flowrun 状态变更推 `flowrun` entity 通知(slim payload D-redo-6,UI 经 GET 拉详情)。

### V1.2 final-sweep — 跨对话能力 ✅(已交付,2026-05-16)
两块"对话长记忆"基础设施一并落地:

- **§1 compaction**(`app/contextmgr.Manager`):对话超阈值时自动压缩。3 路径——< Soft(0.70) 跳过 / Soft 降级老 tool_result 到 `warm`/`cold` / Hard(0.85) 调便宜 LLM 生成 anchored-merge 摘要并 archive。schema:`conversations` 加 `summary` + `summary_covers_up_to_seq` 两列;`message_blocks` 加 `context_role` 一列 + 新 block type `compaction`(eventlog 协议 6→7);`pkg/modelmeta` + `pkg/tokencount` 给估算 + 校准。投影:`loop/history.BlocksToAssistantLLM` 按 role 渲染(archived 丢 / warm 200B preview / cold 元数据占位 / hot 全文);`chat.buildHistory` 前置 `<conversation_summary>` wrapper。
- **§2 memory**(`app/memory.Service`):跨对话长期事实库,4 类(user / feedback / project / reference)× 2 source(user / ai);pinned memory 全文进每个 system prompt,非 pinned 只入索引段。3 system tools(`read_memory` / `write_memory` / `forget_memory`)给 LLM 自管;7 HTTP endpoints + testend `/config/memory` 面板给用户管。LLM 不见 `pinned` 字段——pinning 是用户控件(系统提示词预算只用户能控)。

详见 [`service-design-documents/compaction.md`](./service-design-documents/compaction.md) + [`service-design-documents/memory.md`](./service-design-documents/memory.md);7 pipeline tests(4 memory + 3 compaction)全绿。

### Phase 5 — 智能化（🚧 部分交付）
`document` **✅ 已交付**（LLM-ranked attach，**无向量库 / 无 sqlite-vec / 无 chunking pipeline**——2026-05-16 设计改向，详 [`final-sweep.md`](./final-sweep.md) §14；CRUD + Notion-style tree + @mention + catalog/relation 接入）+ `intent`（⬜ 自实现 ReAct Agent，尚未开工）+ `chat` 终极版（⬜ 意图识别 → 工作流推荐 → 自动建草稿）。**注**：mcp + skill 已提前在 V1.2 D5-D7 交付（Phase 4 准备件，官方 `modelcontextprotocol/go-sdk` v1.6）；memory + compaction 在 final-sweep（2026-05-16）交付。

**焦点实体延伸**：knowledge / mcp / skill 同理，消息打标后右侧面板跟随切换。

### 跨 domain 协作图

```
                    ┌──────────────────┐
                    │ chat (智能编排)   │ ← Phase 5 终极
                    └────────┬─────────┘
              ┌──────────────┼──────────────┐
              ↓              ↓              ↓
        ┌──────────┐  ┌──────────┐  ┌──────────┐
        │ workflow │  │   tool   │  │ document │  ← 中层"能力载体"
        └────┬─────┘  └────┬─────┘  └────┬─────┘
             ↓             ↓             ↓
        flowrun       function     子节点 tree
        scheduler     attachment   (Notion-style)
        trigger
                                    ┌──────────┐
                                    │   mcp    │
                                    └──────────┘
                                    ┌──────────┐
                                    │  skill   │
                                    └──────────┘

       ┌─────────────────────────────────────────────────────┐
       │ 全程依赖：Phase 0-1 地基 + apikey / model / conversation│
       │ + crypto / events / db / logger / reqctx              │
       └─────────────────────────────────────────────────────┘
```

---

## 工程规范 → 见 CLAUDE.md

**所有代码规范、工程纪律、设计原则、契约宪法（N/D/E）、代码规范（S 系列）、测试规范（T 系列）、注释规范、包结构、包命名、文档同步纪律、开发期工具纪律——全部搬到项目根 [`CLAUDE.md`](../../CLAUDE.md)**。

理由：
- 单一事实源——规则改一处，避免 backend-design.md / CLAUDE.md 双份漂移
- Claude Code 自动加载 `CLAUDE.md` 进 context，确保代码改动时规则始终在线
- 本文件回归"项目说明书"定位（愿景、架构、Phase 路线、Verification），不再背规范

---

## Target Architecture

> 以 apikey 为参照样板。其他 domain 按同样套路开。

```
backend/
├── cmd/server/main.go              ← 入口，DI 组装
├── go.mod / go.sum
└── internal/
    ├── domain/                     ← 纯业务（仅 import 标准库 + GORM tag）
    │   ├── apikey/                 ← ✅ apikey.go + providers_test.go（providers.go 在 app 层）
    │   ├── model/                  ← ✅
    │   ├── conversation/           ← ✅
    │   ├── chat/                   ← ✅ Message + Block + Attachment（Block 模型，2026-04-27 重构）
    │   ├── function/               ← ✅ Function + Version + Execution (D22) + Repository + ExecutionRepository + 14 sentinel（forge_redesign Plan 01 替代 forge domain）
    │   ├── handler/                ← ✅ Handler + Version + Call (D22) + MethodSpec + InitArgSpec + Repository + CallRepository + 19 sentinel（forge_redesign Plan 02 trinity 第二条腿）
    │   ├── workflow/               ← ✅ Workflow + Version + Graph + NodeSpec (13 types) + EdgeSpec + VariableSpec + 9 Op + Repository + 11 sentinel（forge_redesign Plan 04 trinity 第三条腿；故意无 ErrPendingConflict — iterate-same-pending D-redo-11）
    │   ├── flowrun/                ← ✅ FlowRun + Node + PausedState + 6 sentinel + 11 方法 Repository (Plan 05 执行 plane 记录簿)
    │   ├── trigger/                ← ✅ 4 Kind + 3 State + Spec + State + 4 sentinel (Plan 05 listener 类型)
    │   ├── crypto/                 ← ✅ 接口
    │   ├── events/                 ← ✅ 接口 + types.go（强类型事件）
    │   ├── errors/                 ← ✅ 跨 domain 通用 sentinel
    │   ├── subagent/               ← ✅ SubagentType + SubagentRun + SubagentMessage + Repository + 4 sentinel（无 SubRunner 接口——chat/subagent 通过 app/loop 解耦，详见 service-design-documents/subagent.md §6）
    │   ├── mcp/                    ← ✅ ServerConfig + ServerStatus + ToolDef + HealthResult + HealthSnapshot + HealthHistoryRepository + 5 status const + RegistryEntry + 10 sentinels
    │   ├── skill/                  ← ✅ Skill + Frontmatter（Anthropic SKILL.md spec 全字段保留 cross-vendor）+ 5 sentinel + MaxBodyBytes/MaxDescriptionChars 常量
    │   ├── catalog/                ← ✅ CatalogSource port + Catalog + Item + Granularity (PerItem/PerServer/PerCollection) + SystemPromptProvider + 2 sentinel（内部消化不进 errmap）
    │   ├── sandbox/                ← 📐 Phase 4 准备件 Runtime + Env + Owner + RuntimeInstaller / EnvManager port + 8 sentinel（统一 PluginSandbox）
    │   ├── relation/               ← ✅ Relation + 8 EdgeKind const + Service/Repository interfaces + EntityMeta + GraphNode + Snapshot + SyncEdge + Filter + 5 sentinels（V1.2 §16 跨实体边图）
    │   (flowrun/trigger/scheduler ✅ 上方;Plan 05 完成)
    │   ├── document/               ← ✅ Phase 5（LLM-ranked attach，无向量库；CRUD + tree + @mention + catalog source 已交付；详 final-sweep §14）
    │   └── intent/                 ← ⬜ Phase 5
    │
    ├── app/                        ← service 层（协调 domain + infra）
    │   ├── apikey/                 ← ✅ apikey.go（Service + KeyProvider + MaskKey 全合并）+ providers.go + tester.go
    │   ├── model/                  ← ✅ model.go（Service + ModelPicker 合并）
    │   ├── conversation/           ← ✅ conversation.go
    │   ├── loop/                   ← ✅ 通用 ReAct 引擎：loop.go（Host 接口 + Run）+ stream.go（LLM 流式装配）+ tools.go（partition by execution_group + dispatch）+ history.go（extendHistory）。chat / subagent / Skill fork / Phase 4 workflow LLM 节点都是调用方
    │   ├── chat/                   ← ✅ 重构为 loop 调用方：chat.go / runner.go（agentRun → 构造 chatHost → loop.Run + autoTitle）/ host.go / history.go / util.go（stream/tools 已迁出到 loop 包）
    │   ├── function/               ← ✅ function.go (Service + Sandbox port) + apply.go (6-op engine + ParseOps) + validate.go + crud.go (CRUD + pending/accept/reject/revert) + run.go (RunFunction + SyncEnvForVersion + recordExecution) + executions.go (SearchExecutions + GetExecutionDetail + hints) + sandbox_adapter.go + sandbox_types.go + catalog_source.go
    │   ├── handler/                ← ✅ handler.go (Service + Sandbox port + ClientFactory) + apply.go (10-op method-level engine + JSON Merge Patch) + validate.go + crud.go (CRUD + pending/accept/reject/revert + UpdateMeta) + config.go (AES-GCM Load/Update/Clear + ConfigState + MaskedConfig) + rpc.go (AssembleClass + DriverScript) + registry.go (Owner / Instance / instanceRegistry caller-owns lifetime) + call.go (Service.Call per-call vs registry + recordCall D22) + calls.go (SearchCalls + GetCallDetail + hints) + sandbox_adapter.go + sandbox_types.go + catalog_source.go + GetByName (workflow CapabilityChecker 跨域)
    │   ├── workflow/               ← ✅ workflow.go (Service + WorkflowReader 接口) + apply.go (9-op engine + RFC 7396 JSON Merge Patch + cloneGraph deep) + validate.go (Kahn cycle + CapabilityChecker + container body 递归 ≤3) + crud.go (CRUD + iterate-same-pending + 自动 accept v1 + slim notif) + expression.go (Go text/template ~140 LOC) + checker_production.go (ProductionChecker 装 function/handler/skill/mcp)
    │   ├── scheduler/              ← ✅ scheduler.go (Service + StartRun 7-gate + Cancel) + state.go (ExecutionContext + topo Kahn + driveLoop) + dispatcher.go (Dispatcher port + Router + DispatcherFunc) + 13 个 dispatch_*.go (capability:trigger/function/handler/mcp/skill/llm + control/io:http/condition/loop/parallel/approval/wait/variable) + retry.go (per-node retry + per-node Timeout + fatal sentinel 短路) + pause.go (approval pauseRun + ResumeApproval + continueRun) + rehydrate.go (RehydrateOnBoot 跨进程重启)
    │   ├── trigger/                ← ✅ trigger.go (Service 整合 4 listener + SetScheduler post-construction + FireManual 手动入口)
    │   ├── tool/                   ← ✅ Tool framework：tool.go（9 方法接口 + 标准字段注入 + ToLLMDef）；嵌套子包按 tool 家族（§S12 例外）
    │   │   ├── function/           ← ✅ 9 LLM tools: search/get/create/edit/revert/delete/run + search_function_executions/get_function_execution (D22)
    │   │   ├── handler/            ← ✅ 10 LLM tools: search/get/create/edit/revert/delete + call_handler + update_handler_config + search_handler_calls/get_handler_call (D22)
    │   │   ├── workflow/           ← ✅ 6 + 2 + 1 = 9 LLM tools: search/get/create/edit/revert/delete (Plan 04) + search_workflow_executions/get_workflow_execution (Plan 05) + trigger_workflow (dryRun;接 scheduler;2026-05-26)
    │   │   ├── filesystem/         ← ✅ Read/Write/Edit/Glob/Grep
    │   │   ├── shell/              ← ✅ Bash/BashOutput/KillShell
    │   │   ├── web/                ← ✅ WebFetch/WebSearch
    │   │   ├── todo/               ← ✅ TodoCreate/List/Get/Update（Phase 5；2026-05-05 改名 Task → Todo）
    │   │   ├── ask/                ← ✅ AskUserQuestion（Phase 5）
    │   │   ├── subagent/           ← ✅ Subagent tool（spawn 子 LLM loop 入口；改名避开 todo domain 撞车）
    │   │   ├── mcp/                ← ✅ search_mcp + call_mcp（不 flat 注册 N×M tools；search 走 LLM ranking 模式 A）
    │   │   └── skill/              ← ✅ search_skills + activate_skill（activate 写 agentstate.ActiveSkill 让 framework dispatch 短路 CheckPermissions）
    │   ├── subagent/               ← ✅ Service{Spawn/Cancel/Get/ListTypes/ListByConversation/ListMessages} + subagentHost（loop.Host 实现，5min total-timeout + panic recover + agentstate token log）+ 内置 3 类型注册表（Explore / Plan / general-purpose）
    │   ├── mcp/                    ← ✅ Service{Start/Stop/Add/Remove/Reconnect/Search/CallTool/HealthCheck/ListHealthHistory/InstallFromRegistry/Import} + 6 内置 marketplace Registry + 单 RWMutex 模型 + §5.6 健康追踪（连续失败≥3 → degraded → 自愈）+ §5.7 HealthSnapshot 历史写入（healthRepo 装配后 HealthCheck auto-record）
    │   ├── skill/                  ← ✅ Service{Scan/Get/List/Search/Activate/Body/Create/Replace/Delete/Import} + atomic 写 + fsnotify watcher（debounce 500ms + symlink loop guard + Linux fd-limit fail-soft + 5min poll backstop）+ \$1/\$ARGUMENTS/\${CLAUDE_*} 占位替换 + fork 模式派发 SubagentService（depth ≥ 1 抑制嵌套 fork）
    │   ├── catalog/                ← ✅ Service{Start/Stop/Refresh/RegisterSource/GetForSystemPrompt/Get} + LLMGenerator{Generate 3-attempt retry + coverage 校验 + mechanical fallback}+ pollLoop 1s + atomic.Bool 单 flight + fingerprint dedup（hash sort source+name+description）+ ~/.forgify/.catalog.json 原子读写 + chat.runner SystemPromptProvider 注入 + 3 source（function/skill/mcp via AsCatalogSource）
    │   ├── relation/               ← ✅ Service{SyncOutgoing/SyncIncoming/PurgeEntity/List/Neighborhood/GetRelgraph} + diffSync core（Insert ON CONFLICT DO NOTHING + DeleteByIDs）+ 7 reader ports（function/handler/workflow/document/conversation/mcp/skill）+ BFS Neighborhood + GetRelgraph（full graph join）
    │   ├── askai/                  ← ✅ Spawner{Spawn} + BuildFunctionContext/BuildHandlerContext/BuildWorkflowContext/BuildDocumentContext/BuildTriageContext（5 context builders）；被 :iterate / :triage action handlers 调用；返 conversationId
    │   ├── sandbox/                ← 📐 Phase 4 准备件 Service + EnsureRuntime/EnsureEnv/Spawn/SpawnLongLived/SpawnShell/Destroy/GC（统一 PluginSandbox）
    │   └── <Phase 4-5>/
    │
    ├── infra/                      ← 技术实现
    │   ├── db/                     ← ✅ db.go（modernc.org/sqlite）+ migrate.go + schema_extras.go
    │   ├── store/                  ← ✅ apikey / model / conversation / chat / function (含 execution_log) / handler (含 call_log) / workflow (Plan 04) / flowrun (Plan 05;FlowRun + Node) / mcpcalls (Plan 05 D22) / skillexec (Plan 05 D22) / todo / sandbox / relation (ON CONFLICT DO NOTHING batch insert + cursor List + PurgeEntity) / mcphealth (Insert + ListSince)
    │   ├── mcp/                    ← ✅ ~/.forgify/mcp.json Load/Save/Merge（Claude Desktop schema 兼容，0600 权限，atomic 写）+ stdio Client wrapper（基于 modelcontextprotocol/go-sdk v1.6；stderr→zap+256KB ring；CommandTransport 处理 SIGTERM→5s→SIGKILL）
    │   ├── sandbox/                ← ✅ 统一 PluginRuntime（mise embed + per-plugin 隔离 env,5 类 owner: function/handler/mcp/skill/conversation）
    │   ├── handler/                ← ✅ infra/handler/client.go(stdio JSON-line RPC client wrapping subprocess pipes;5 methods: Init/Call/StreamCall/Shutdown/Crashed;V1 per-instance 串行经 sync.Mutex)
    │   │   ├── sandbox.go          ← Service 实现 RuntimeInstaller/EnvManager 注册 + spawn 派发
    │   │   ├── bootstrap/embed.go  ← go:embed mise binaries（per-platform，~10MB）
    │   │   └── installer/          ← 各语言子包
    │   │       ├── mise/           ← 通配 installer（python/node/rust/java/go/ruby/php/...）
    │   │       ├── playwright/     ← Browsers
    │   │       ├── dotnet/         ← .NET 微软官方脚本
    │   │       └── static/         ← 静态二进制 plugin（如 GitHub MCP）
    │   ├── llm/                    ← ✅ 自有 LLM 流式客户端（替代 Eino，2026-04-27）
    │   │   ├── llm.go              ← StreamEvent / LLMMessage / Client 接口 / Generate helper
    │   │   ├── openai.go           ← OpenAI-compat SSE（DeepSeek/Qwen/Moonshot/Ollama 等）
    │   │   ├── anthropic.go        ← Anthropic 原生 /v1/messages 客户端
    │   │   └── factory.go          ← Factory.Build(Config) provider dispatch
    │   ├── chat/                   ← ✅ extractor.go（附件内容提取，7 种格式 + Vision 路径）
    │   ├── sandbox/                ← ✅ python.go（Python subprocess + 30s 超时）
    │   ├── events/memory/          ← ✅ in-memory pub-sub Bridge
    │   ├── crypto/                 ← ✅ aesgcm.go + fingerprint.go
    │   └── logger/                 ← ✅ zap.go + broadcast.go（dev-only LogBroadcaster）
    │
    ├── pkg/                        ← 跨层共享纯工具（无业务、无 infra 依赖）
    │   ├── reqctx/                 ← ✅ reqctx.go（user 身份）+ locale.go + agentrun.go（convID/msgID/toolCallID）
    │   ├── pagination/             ← ✅ cursor.go（Parse + EncodeCursor + DecodeCursor + Cursor 共享类型）
    │   ├── idgen/                  ← ✅ idgen.go（New(prefix string) string；§S15 标准 ID 形状唯一实现）+ prefix.go（KindByPrefix map，wikilink 解析用）
    │   ├── wikilink/               ← ✅ wikilink.go（Parse `[[<prefix>_<16hex>]]` → EntityRef{Kind, ID}；5 已知前缀 fn/hd/wf/doc/cv）
    │   ├── llmparse/               ← ✅ extractjson.go（ExtractJSON + IsLikelyJSON；LLM 响应 markdown fence + 外层括号兜底）
    │   └── llmclient/              ← ✅ llmclient.go（Resolve picker→keys→factory 三段舞；ErrPickModel/ErrResolveCreds/ErrBuildClient sentinel）
    │
    └── transport/
        └── httpapi/                ← 包名避开 net/http 冲突
            ├── router/             ← ✅ router.go + deps.go（DI struct，nil-tolerant）
            ├── response/           ← ✅ envelope.go + errmap.go + sse.go（StreamSSE[T] 泛型 helper）
            ├── middleware/         ← ✅ recover / logger / cors / locale / auth(InjectUserID) / notfound
            └── handlers/           ← ✅ health / apikey / model / conversation / chat / function / dev / util.go（idAndAction）
```

`legacy/` 存放 V1.0/V1.1 的旧实现（Electron + Eino）作为参考。`testend/` 是开发期调试控制台（详见 [`testend-design.md`](./testend-design.md)）。

**依赖方向**：`transport → app → (domain ∪ infra/store)`、`infra/store → domain`（实现接口）、`infra/db → 标准库`、`domain` 不依赖任何人。

**`infra/db/` vs `infra/store/<domain>/` 的拆分**：
- `infra/db/` —通用 DB 底层（连接、迁移、schema_extras），与任何具体表无关
- `infra/store/<domain>/` —表相关的 CRUD（业务 aware），实现 `domain/<domain>.Repository`
- 同一个 domain 在 store 层的包名也叫 `<domain>`（如 `apikey`），调用方 import 时按 `<name><role>` 起别名（详见 CLAUDE.md §S13）

**类型策略**：domain 类型直接带 GORM tag（一份到底）；store 层不再做 entity↔row 转换。

**transport/httpapi 内部分层原则**：**稳定的（通用能力）和频繁变的（业务 handler）分开放**。
- `response/` `middleware/` 属于 HTTP 层框架级通用能力，写一次用很久
- `handlers/` 属于业务级代码，每加一个 feature 就新增/修改

> **`pagination/` 不在 httpapi 下**——cursor 编解码是与 HTTP 无关的纯工具，会被 `infra/store/*` 和 `transport/httpapi/handlers/*` 同时消费。把它放在 transport 下会迫使 store 层反向 import transport（破坏依赖方向 `transport → app → (domain ∪ infra/store)`），所以放在 `internal/pkg/pagination/`。

---

## 文档分册结构

本文件 + CLAUDE.md 是**稳定规范层**。其余按角色分三组：

| 文档 | 用途 | 推进节奏 |
|---|---|---|
| [`../../CLAUDE.md`](../../CLAUDE.md) | **代码规范、工程纪律、设计原则、契约宪法**——单一事实源 | 规则演化时改 |
| [`service-contract-documents/api-design.md`](./service-contract-documents/api-design.md) | **全部 REST API 一眼索引** | 每 domain 开工时加一段 |
| [`service-contract-documents/database-design.md`](./service-contract-documents/database-design.md) | **全部表一眼索引** | 同上 |
| [`service-contract-documents/error-codes.md`](./service-contract-documents/error-codes.md) | **全部错误码一眼索引** | 同上 |
| [`service-contract-documents/events-design.md`](./service-contract-documents/events-design.md) | **全部 SSE 事件一眼索引** | 涉及流式时加 |
| [`service-design-documents/<domain>.md`](./service-design-documents/) | **每个 domain 详设计** | 每 domain 开工前写 |
| [`progress-record.md`](./progress-record.md) | 开发日志 + 当前快照 + 任务清单 | 实时更新 |
| [`desktop-packaging-notes.md`](./desktop-packaging-notes.md) | 桌面端分发方向（Wails / 打包 / 常驻后台）| 大决策时改 |

**工作流**：
1. **开工前** → 填 `service-design-documents/<domain>.md` 详设计（含端到端推演 + 实现清单）
2. **实现中** → 同步更新 `service-contract-documents/*.md` 里该 domain 的索引段
3. **完成后** → 在 `progress-record.md` 加一行 dev log + 勾任务清单

---

## v1 平台支持声明

**全功能支持**：
- macOS arm64（Apple Silicon, M1/M2/M3/M4）
- macOS amd64（Intel）—— 系统 ≥ 10.15 (Catalina)
- Linux amd64（glibc 系：Ubuntu / Debian / Fedora / CentOS / RHEL）
- Linux arm64（同上 + Raspberry Pi 4+, AWS Graviton）

**Windows amd64（10/11）—— 限制版**：Python / Node 类 plugin 全可用（覆盖 99% 需求）；Ruby / PHP / Erlang / Elixir / Lua / Crystal / Zig 等长尾语言 plugin 在 Windows 隐藏不可装（mise 这些 plugin 用 bash 实现）。Bash tool 内部用 PowerShell 替代 sh，命令兼容性大部分一致。详 [`service-design-documents/sandbox.md`](./service-design-documents/sandbox.md) §17。

**不支持**：
- Linux musl（Alpine 等）—— mise 是 glibc binary，bootstrap fail-soft 进 degraded mode
- 32-bit 架构（i386 / armv7）
- FreeBSD / OpenBSD / 其他 Unix
- macOS amd64 < 10.15 / 旧版 Windows

每平台 binary 通过 `GOOS=<os> GOARCH=<arch> go build` 单独构建（mise binary 用 build tag 仅 embed 当前平台版本，每 binary ~35 MB）。

---

## Verification

### 测试分级（按外部依赖成本，2026-05-27 overhaul 后）

| 命令 | 依赖 | 用途 | 耗时 |
|---|---|---|---|
| `make unit` | 无 | Go 单测（in-memory SQLite） | ~30s |
| `make web` | 无 | vitest 前端单测 | ~10s |
| `make mock` | 无 | Pipeline fake LLM 测试（16 包） | ~60s |
| `make sandbox` | `FORGIFY_DEV_RESOURCES` | mock + 真 sandbox lifecycle | +60s |
| `make live` | `DEEPSEEK_API_KEY`，**烧 token** | only 真 LLM 测试 | ~3min |
| `make e2e` | 上述全部 | full pipeline release gate | ~5min |
| `make cover` | 无 | HTML coverage 报告（`coverage/pipeline.html`） | ~60s |

### Pipeline 测试结构（详 `backend/test/README.md`）

7 个 axis：
- `smoke/`：启动冒烟
- `api/`：单 domain × HTTP 端点（happy + 主错）
- `cross/`：跨 domain 锈川（catalog / scheduler / mention / subagent / isolation / compaction / askai / eventlog / relation / tool framework）
- `sse/`：3 流协议 round-trip（eventlog / notifications / forge）
- `lifecycle/`：长链路 / 真 sandbox（function env / handler config / workflow DAG）
- `errcodes/`：每 sentinel 一行 sweep
- `live/`：真 LLM 测试（`Live_` 前缀 + `RequireDeepSeekKey` gate）

### 覆盖矩阵（自动反查）

- `make matrix` 扫 handlers + errmap + SSE truth + seams.yaml + 测试 `// covers:` annotation，生成 `backend/test/README.md` 覆盖矩阵段
- `make audit` 严格检查（warn-only；annotation 完整后切 `--strict` 自动 fail `verify`）
- 工具源码：`backend/cmd/coverage-matrix/`

### 性能基准
- 流式对话 token latency < 旧版 110%
- 工具列表加载 < 500ms
- 工具搜索通过 LLM 排序，响应取决于上游 LLM（Phase 5 重新加 FTS5 时再加本地搜索基准）

### Schema 完整性
- `PRAGMA foreign_key_check` 零返回
- `PRAGMA integrity_check` 返回 `ok`

### 跨平台编译（modernc.org/sqlite 迁移后）
- `GOOS=darwin GOARCH=arm64 go build ./cmd/server`
- `GOOS=linux GOARCH=amd64 go build ./cmd/server`
- `GOOS=windows GOARCH=amd64 go build ./cmd/server`

三平台单条命令出二进制，约 24-25MB，无 CGO / 无 C 工具链需求。

---

## 非目标（本轮不做）

- ❌ 真实账号鉴权（密码 / session / token）—— 产品定位为本地单用户桌面 app（详见 [`desktop-packaging-notes.md`](./desktop-packaging-notes.md)），不计划做 SaaS 多租户。`X-Forgify-User-ID` header 是身份标识，无密码；middleware（`IdentifyUser` + `RequireUser`）只校验 id 存在，不验明身份。前端 onboarding 创建第一个 user 后把 id 存 localStorage，每次请求带回。无 magic id：unknown id → 401 / UNAUTH_NO_USER（前端 self-heal）；非 `/api/v1/users` / `/api/v1/health` 路由都要求带有效 id。后台任务遍历真实 users（0 user → no-op）
- ❌ Docker 沙箱 —— 保持 Python subprocess（`infra/sandbox/python.go`，30s 超时）。本地单用户场景下 Docker 是过度工程
- ❌ 前端类型生成工具链 —— 下轮前端 iteration 再接
- ~~❌ 前端代码改动~~ —— **已解除**：后端 Phase 0-4 定型后前端已进入开发阶段（按 [`frontend-prd.md`](./frontend-prd.md) + boilerplate；见 CLAUDE.md 末节"前端开发守则"）
