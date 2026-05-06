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
2. **第二阶段**（待启动）：前端按本文档列出的"前端变更清单"统一跟进。

**前端在 V1.2 后端阶段保持不动。** Phase 4-5 工作流/智能化能力按计划继续在后端落地，前端按"V1.2 完成后整体迁移"节奏走。

---

## 产品愿景（Phase 2 起）

Forgify 不只是"对话 + 造工具"—目标是 **Agentic Workflow Platform**：用户一句话能编排出工作流，工作流由多种节点构成，可挂知识库做 RAG，最终由调度器部署运行。

### 核心能力清单

1. **意图识别 / Intent Routing**：聊天时识别用户想干啥（创建工作流？改工具？更新知识库？纯问答？）
2. **工作流引擎**：节点 + 边的 DAG，能跑、有运行历史
3. **多种节点类型**：用户工具 / MCP 工具 / LLM 节点 / Skill / 知识库检索 / 触发器 / 审批
4. **知识库 / RAG**：上传文档 → 切分 → 向量化 → 检索，挂在 LLM 或工作流节点上
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
| MCP 集成 | `mark3labs/mcp-go`（Phase 5）|
| Python 沙箱 | subprocess `infra/sandbox`（已有）|

---

## Phase 路线图

**当前状态 / 任务细化** → [`progress-record.md`](./progress-record.md)

| Phase | 主题 | 工时 | 完成后产品形态 | 状态 |
|---|---|---|---|---|
| 0-1 | 地基 | 10h | 基础设施全就位 | ✅ 2026-04-23 |
| 2 | 基础对话 | 11h | ChatGPT 客户端 | ✅ 2026-04-25 |
| 3 | 工具锻造 | 12h | Forgify V1.0 体验 | ✅ 2026-04-26 |
| — | Phase 3 后基础设施优化轮 | — | chat 重构 + 调研 + 驱动迁移 + 桌面端方向 | 🔄 2026-04-27 起 |
| 4 | 工作流 | 20h | 桌面版 Coze | ⬜ |
| 5 | 智能 + 知识库 + MCP | 15h | 完整 Agent 平台 | ⬜ |
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

### Phase 3 — 工具锻造能力（已完成）
`forge` 主 domain（版本 / pending / 测试用例 / 沙箱执行 / 导入导出，22 端点）+ `app/tool/forge/`（5 个 forge 系统工具：search/get/create/edit/run.go 各一文件）+ chat 升级支持 ReAct 多步循环。Python 沙箱 `infra/sandbox/python.go`（subprocess + 30s 超时 + AST 函数提取）。

> Phase 3 后优化轮（2026-05-02）大改造：(1) `tool` 大重命名 → `forge`（types/tables/IDs/paths/22 endpoints/LLM-facing 名）；(2) `agent` 包重命名 `tool`，新建嵌套子包结构（forge/filesystem/shell/web）；(3) Tool 接口扩到 10 方法（IsReadOnly / NeedsReadFirst / RequiresWorkspace / IsConcurrencySafe / ValidateInput / CheckPermissions 等）；(4) 删除 8 个旧通用 system tool（read_file / write_file / list_dir / run_shell / run_python / datetime / web_search / fetch_url），新一代 system tools（Read/Write/Edit/Bash/Glob/Grep/LS）将在 Phase 5 重建。详见 progress-record.md。

**Phase 3 后基础设施优化轮（2026-04-27 起，进行中）**：chat 基础设施重构（移除 Eino + Block 模型）/ chat pipeline.go → runner.go 二次重构 / Brewfile + Makefile setup / Claude Code 内部机制调研（9 份报告）/ SQLite 驱动迁移（mattn → modernc，纯 Go） / 桌面端 Wails 分发方向定型 / 大规模代码 review 战役（staticcheck / 死代码 / 跨域重复 / errmap 完整性等）。详见 [`progress-record.md`](./progress-record.md) §2。

### Phase 4 — 工作流能力（最大的一块）
`workflow`（DAG + 状态机）+ `flowrun`（执行实例）+ 5 类节点（LLM / Tool / Trigger / Approval / Variable）+ `scheduler` + `trigger`（cron / fsnotify / HTTP webhook）+ `chat` 再升级支持"对话创建工作流"。执行引擎自实现（不依赖 Eino compose，Eino 已全面移除）。

**焦点实体延伸**：workflow 节点编辑时推 `workflow.node_updated` 事件；右侧面板切换到对应 workflow 展示。

### Phase 5 — 智能化
`knowledge` + `document`（本地 sqlite-vec）+ `intent`（自实现 ReAct Agent，基于 `infra/llm`）+ `mcpserver`（`mark3labs/mcp-go`）+ `skill`（V1 浅版：打标签的工具）+ `chat` 终极版（意图识别 → 工作流推荐 → 自动建草稿）。

**焦点实体延伸**：knowledge / mcp / skill 同理，消息打标后右侧面板跟随切换。

### 跨 domain 协作图

```
                    ┌──────────────────┐
                    │ chat (智能编排)   │ ← Phase 5 终极
                    └────────┬─────────┘
              ┌──────────────┼──────────────┐
              ↓              ↓              ↓
        ┌──────────┐  ┌──────────┐  ┌──────────┐
        │ workflow │  │   tool   │  │knowledge │  ← 中层"能力载体"
        └────┬─────┘  └────┬─────┘  └────┬─────┘
             ↓             ↓             ↓
        flowrun       forge         document
        scheduler     attachment    (向量库)
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
    │   ├── forge/                  ← ✅ Forge + ForgeVersion + ForgeTestCase + ForgeRunHistory + ForgeTestHistory（Phase 1 大重命名 tool→forge）
    │   ├── crypto/                 ← ✅ 接口
    │   ├── events/                 ← ✅ 接口 + types.go（强类型事件）
    │   ├── errors/                 ← ✅ 跨 domain 通用 sentinel
    │   ├── subagent/               ← ✅ SubagentType + SubagentRun + SubagentMessage + Repository + 4 sentinel（无 SubRunner 接口——chat/subagent 通过 app/loop 解耦，详见 service-design-documents/subagent.md §6）
    │   ├── mcp/                    ← ✅ ServerConfig + ServerStatus + ToolDef + HealthResult + 5 status const + RegistryEntry + 10 sentinels（D5-1+D5-2 完成 2026-05-06；runtime/Service 在 D6）
    │   ├── skill/                  ← 📐 Phase 4 准备件 Skill + Frontmatter + 5 sentinel
    │   ├── catalog/                ← 📐 Phase 4 准备件 CatalogSource port + Catalog + Item + Granularity
    │   ├── sandbox/                ← 📐 Phase 4 准备件 Runtime + Env + Owner + RuntimeInstaller / EnvManager port + 8 sentinel（统一 PluginSandbox）
    │   ├── workflow/               ← ⬜ Phase 4
    │   ├── flowrun/                ← ⬜ Phase 4
    │   ├── scheduler/              ← ⬜ Phase 4
    │   ├── trigger/                ← ⬜ Phase 4
    │   ├── knowledge/ document/    ← ⬜ Phase 5
    │   └── intent/                 ← ⬜ Phase 5
    │
    ├── app/                        ← service 层（协调 domain + infra）
    │   ├── apikey/                 ← ✅ apikey.go（Service + KeyProvider + MaskKey 全合并）+ providers.go + tester.go
    │   ├── model/                  ← ✅ model.go（Service + ModelPicker 合并）
    │   ├── conversation/           ← ✅ conversation.go
    │   ├── loop/                   ← ✅ 通用 ReAct 引擎：loop.go（Host 接口 + Run）+ stream.go（LLM 流式装配）+ tools.go（partition by execution_group + dispatch）+ history.go（extendHistory）。chat / subagent / Skill fork / Phase 4 workflow LLM 节点都是调用方
    │   ├── chat/                   ← ✅ 重构为 loop 调用方：chat.go / runner.go（agentRun → 构造 chatHost → loop.Run + autoTitle）/ host.go / history.go / util.go（stream/tools 已迁出到 loop 包）
    │   ├── forge/                  ← ✅ forge.go（30 方法 Service + ParseCode）+ ast.go（Python AST 解析）
    │   ├── tool/                   ← ✅ Tool framework：tool.go（9 方法接口 + 标准字段注入 + ToLLMDef）；嵌套子包按 tool 家族（§S12 例外）
    │   │   ├── forge/              ← ✅ user-forged-tool 系统工具
    │   │   ├── filesystem/         ← ✅ Read/Write/Edit/Glob/Grep
    │   │   ├── shell/              ← ✅ Bash/BashOutput/KillShell
    │   │   ├── web/                ← ✅ WebFetch/WebSearch
    │   │   ├── todo/               ← ✅ TodoCreate/List/Get/Update（Phase 5；2026-05-05 改名 Task → Todo）
    │   │   ├── ask/                ← ✅ AskUserQuestion（Phase 5）
    │   │   ├── subagent/           ← ✅ Subagent tool（spawn 子 LLM loop 入口；改名避开 todo domain 撞车）
    │   │   ├── mcp/                ← 📐 Phase 4 准备件 search_mcp + call_mcp
    │   │   └── skill/              ← 📐 Phase 4 准备件 search_skills + activate_skill
    │   ├── subagent/               ← ✅ Service{Spawn/Cancel/Get/ListTypes/ListByConversation/ListMessages} + subagentHost（loop.Host 实现，5min total-timeout + panic recover + agentstate token log）+ 内置 3 类型注册表（Explore / Plan / general-purpose）
    │   ├── mcp/                    ← 🔄 V1 marketplace Registry（6 内置 + Get/List/Visible+GOOS filter）已落地 2026-05-06；Service + Connect/Disconnect/Search/CallTool/Install/Health 在 D6
    │   ├── skill/                  ← 📐 Phase 4 准备件 Service + frontmatter + fsnotify watcher
    │   ├── catalog/                ← 📐 Phase 4 准备件 Service + Generator + 1s polling + atomic 单 flight + fingerprint dedup
    │   ├── sandbox/                ← 📐 Phase 4 准备件 Service + EnsureRuntime/EnsureEnv/Spawn/SpawnLongLived/SpawnShell/Destroy/GC（统一 PluginSandbox）
    │   └── <Phase 4-5>/
    │
    ├── infra/                      ← 技术实现
    │   ├── db/                     ← ✅ db.go（modernc.org/sqlite）+ migrate.go + schema_extras.go
    │   ├── store/                  ← ✅ apikey / model / conversation / chat / forge / todo / sandbox / subagent
    │   ├── mcp/                    ← 🔄 ~/.forgify/mcp.json Load/Save/Merge（Claude Desktop schema 兼容，0600 权限，atomic 写）已落地 2026-05-06；stdio Client wrapper（基于 modelcontextprotocol/go-sdk v1.x）在 D6
    │   ├── sandbox/                ← 🔄 大重构：原 forge-only 升级为统一 PluginRuntime
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
    │   ├── idgen/                  ← ✅ idgen.go（New(prefix string) string；§S15 标准 ID 形状唯一实现）
    │   ├── llmparse/               ← ✅ extractjson.go（ExtractJSON + IsLikelyJSON；LLM 响应 markdown fence + 外层括号兜底）
    │   └── llmclient/              ← ✅ llmclient.go（Resolve picker→keys→factory 三段舞；ErrPickModel/ErrResolveCreds/ErrBuildClient sentinel）
    │
    └── transport/
        └── httpapi/                ← 包名避开 net/http 冲突
            ├── router/             ← ✅ router.go + deps.go（DI struct，nil-tolerant）
            ├── response/           ← ✅ envelope.go + errmap.go + sse.go（StreamSSE[T] 泛型 helper）
            ├── middleware/         ← ✅ recover / logger / cors / locale / auth(InjectUserID) / notfound
            └── handlers/           ← ✅ health / apikey / model / conversation / chat / forge / dev / util.go（idAndAction）
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

### 单元测试
- `cd backend && go test -count=1 -race ./...` 零失败（CGO 已不需要——modernc.org/sqlite 纯 Go）
- domain/ 层覆盖率 > 80%（纯逻辑好测）
- app/ 层核心 service 必测

### 契约测试
每个端点一个 curl 脚本，验证：
- 状态码正确
- envelope 格式正确
- 错误码符合约定
- 分页参数生效

### 端到端场景（Phase 3 起，集成测试 13 组覆盖）
A. Conversation CRUD / B. API Key & Model Config / C. 分页 cursor / D-E. 系统工具组 / F. 并行 tool call / G. 多步 ReAct（write→read→python）/ H. Attachment 内联 / I. 错误处理 / J. Auto-title / K. 含 tool_call blocks 的多轮历史重建 / L. SSE messageId 一致性 / M. Forge 工具创建。详见 [`progress-record.md`](./progress-record.md) 的 chat 重构段。

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

- ❌ 多租户真实 user_id 来源 —— 先硬编码 `local-user`（`reqctx.DefaultLocalUserID`）；产品定位为本地单用户桌面 app（详见 [`desktop-packaging-notes.md`](./desktop-packaging-notes.md)），不计划做 SaaS 多租户
- ❌ Docker 沙箱 —— 保持 Python subprocess（`infra/sandbox/python.go`，30s 超时）。本地单用户场景下 Docker 是过度工程
- ❌ 前端类型生成工具链 —— 下轮前端 iteration 再接
- ❌ 前端代码改动 —— V1.2 后端阶段不动前端，统一在后端完成后整体迁移
