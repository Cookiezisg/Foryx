# V1.2 Backend 进展记录

**关联**：
- [`backend-design.md`](./backend-design.md) — 总体设计 + 规范（相对稳定，很少动）
- [`service-contract-documents/`](./service-contract-documents/) — 每个 domain 的服务契约索引（一眼清单）
- [`service-design-documents/`](./service-design-documents/) — 每个 domain 的详细设计
- [`desktop-packaging-notes.md`](./desktop-packaging-notes.md) — 桌面端分发方向（Wails / 打包 / 常驻后台）
- [`claude-code-research-documents/`](./claude-code-research-documents/) — Claude Code 内部机制调研（9 份主题报告）

**本文档定位**：所有"正在发生"的状态都在这里。开发日志 / 完成快照 / 待办清单 / 原则演化。规范/架构/愿景这些"相对不变"的放 `backend-design.md`。

---

## 1. 当前快照（截止 2026-05-26）

| Phase | 主题 | 状态 | 里程碑 |
|---|---|---|---|
| **Phase 0** | 骨架(go mod + main + /health) | ✅ | 2026-04-22 |
| **Phase 1** | 基础 infra 7 件套(GORM / logger / crypto / events / middleware / response / pagination) | ✅ | 2026-04-23 |
| **Phase 2** | 基础对话能力(apikey / model / conversation / chat) | ✅ | 2026-04-25 |
| **Phase 3** | 工具锻造(forge → trinity:function / handler / workflow) | ✅ | 2026-04-26 |
| **Phase 3 后优化轮 + forge_redesign** | chat 重构 / 驱动迁移 / 打包方向 + trinity 重做(forge→function/handler + Plan 03 SSE 三流) | ✅ | 2026-05-12 |
| **Phase 4** | 工作流(workflow + flowrun + scheduler + trigger + 13 节点 dispatcher) | ✅ | 2026-05-13 |
| **Phase 5** | 智能化(document / intent / chat 终极版) | 🚧 部分交付 | document/mcp/skill/memory/compaction ✅；intent/chat 终极版未做 |
| **前端 revamp** | **TS + FSD 6 层完整重构**（阶段 0-5 全交付；401 bug 根治；文档体系建立） | ✅ | 2026-05-27 |
| **当前重心** | **前端功能实现**（V1.2 桌面 app；FSD 架构已定型；接入后端 API） | 🚧 进行中 | FSD 架构已定型，开始各 page / feature 功能交付 |

**当前测试规模**:`make test-backend`(单测,in-memory SQLite)全绿(174 包,见 settings-redesign 条)。⚠️ **`make e2e`(pipeline tag)当前编译失败**——harness 签名漂移(`LocalCtxAs` 签名变 / `reqctxpkg.DefaultLocalUserID` 已删,见 `completeness-audit-report.md` 🟡-B),修复后重新校准计数。LLM 集成测试因 DeepSeek API key 环境失效优雅 skip。
**当前驱动**:modernc.org/sqlite(纯 Go,无 CGO),跨平台编译一行命令。
**当前依赖体系**:完全摆脱 Eino(chat 重构后)。新增 `robfig/cron/v3 v3.0.1`(Plan 05 首次引入);`fsnotify/fsnotify` 提 direct v1.10.1。
**forge_redesign ✅ 全交付**(2026-05-13):Plan 01-06 全部 merge — function trinity + handler trinity + eventlog/forge 三流 + workflow authoring + execution plane(scheduler/trigger/flowrun + 14 hardening + 4 张新表 D22)+ subagent forger D21 + 主 agent multi-agent forging 教学 + trinity catalog 源 + approval lifecycle E2E。trinity architecture 完工。下一阶段:V1.2 桌面端 Wails 迁移 + Phase 5 智能化。

---

## 2. 开发日志

按时间顺序（旧 → 新）。每个时间块按 phase 或专题分组。

### Phase 0-1：地基（2026-04-22 ~ 2026-04-23）

| 日期 | 内容 |
|---|---|
| 2026-04-22 | 全面契约审计（45 API 端点 + 10 DB 表 + 21 SSE 事件），一致性评分均低 |
| 2026-04-22 | 确定 12 条契约标准（N1-N5 API + D1-D5 DB + E1-E2 SSE） |
| 2026-04-22 | 确定 4 层架构：domain / app / infra / transport，GORM，单份结构带 tag |
| 2026-04-22 | Phase 0 完成：`backend-new/` 骨架，`/api/v1/health` 返回 envelope，优雅退出 |
| 2026-04-22 | 立 **S11 双语注释规范**（英文 + 中文），backend-new 全套代码/注释必须遵守 |
| 2026-04-22 | 日志框架定为 **zap**（dev 彩色 / prod JSON），`infra/logger/zap.go` 封装 |
| 2026-04-22 | transport 层结构升级：`http/` → `httpapi/`（避免包名冲突），拆出 `response/` / `middleware/` / `handlers/` 3 子包 |
| 2026-04-22 | **Phase 1 Step 2** 完成：`response/envelope.go`（Success / Created / NoContent / Paged / Error）+ `response/errmap.go`（FromDomainError）。N1 标准落地为强制 API |
| 2026-04-23 | **Phase 1 Step 3** 完成：`pagination/cursor.go`（Parse / EncodeCursor / DecodeCursor），cursor 分页 + 10 单测 |
| 2026-04-23 | **Phase 1 Step 4a** 完成：`middleware/recover.go`，panic → 500 INTERNAL_ERROR + 6 单测（含敏感信息不泄漏守卫）|
| 2026-04-23 | **Phase 1 Step 4b** 完成：`middleware/logger.go`（method/path/status/bytes/elapsed）+ 6 单测 |
| 2026-04-23 | **Phase 1 Step 4c** 完成：`middleware/notfound.go`，envelope 格式 404 fallback + 4 单测 |
| 2026-04-23 | 模块名纠正：`github.com/sunweilin/forgify-new` → `github.com/sunweilin/forgify/backend`（Go multi-module repo 标准命名）|
| 2026-04-23 | **Phase 1 Step 4d** 完成：`middleware/cors.go`，白名单 CORS（拒绝 `*`）+ 7 单测 |
| 2026-04-23 | **Phase 1 Step 4e** 完成：`router/` 子包 + `handlers/health.go` Register pattern 模版，4 个集成测试验证端到端中间件链 |
| 2026-04-23 | Phase 1 地基 4/7，37 测试零失败；envelope、CORS、访问日志全链路通 |
| 2026-04-23 | **Phase 1 Step 5** 完成：crypto 接口化（`domain/crypto/Encryptor`）+ AES-GCM 实现。修 4 个老代码安全问题（fallback 密钥共享灾难 / decrypt 返 nil nil / 无版本标识 / shell 脆弱）。密文 `v1:` 前缀给 KMS 留兼容位。14 新测试 |
| 2026-04-23 | **Phase 1 Step 6** 完成：`infra/db/`（db.go / migrate.go / schema_extras.go）。WAL / FK / PrepareStmt / UTC。AutoMigrate + schema_extras 模式，4 个 schema 业务问题推迟到 Phase 3 |
| 2026-04-23 | **Phase 1 Step 7** 完成：`domain/events/` 接口 + `infra/events/memory/` 内存实现。强类型事件（禁 `map[string]any`）、扇出 pub-sub、buffer 满非阻塞丢弃、ctx 自动 cancel、sync.Once 幂等 |
| 2026-04-23 | **路线图升级**：定位从"V1.0 重写"→ Agentic Workflow Platform 完整愿景。引入 6 新 domain（workflow / flowrun / scheduler / knowledge / mcp / skill / intent），对标 Dify+Coze 桌面版 |
| 2026-04-23 | 文档目录重组：`Documents/` → `documents/`；按版本分 `version-1.0` / `1.1` / `1.2`；文件名 kebab-case |
| 2026-04-23 | 加 auth middleware `InjectUserID`（硬编码 `DefaultLocalUserID = "local-user"`），Phase 2 多租户字段就绪 |
| 2026-04-23 | 加 locale middleware `InjectLocale` + 跨层共享包 `internal/pkg/reqctx/`（只 stdlib、无状态、单一职责） |
| 2026-04-23 | **全量注释瘦身**：15 个生产文件共砍 ~420 行冗余注释。S11 规范扩展为"双语 + 节制" |
| 2026-04-23 | **Phase 2 路线图修正**：新增 `model` domain（"场景 → provider/model"策略层）。立第 5 条设计原则 **"端到端推演先行"** |

### Phase 2：基础对话能力（2026-04-24 ~ 2026-04-26）

| 日期 | 内容 |
|---|---|
| 2026-04-24 | **apikey domain 层**完成。试过扁平 / 按角色子包 / Go 社区味子包多种结构，最终定**平铺**：`apikey.go`（entity + 常量 + errors + Credentials + ListFilter + Repository + KeyProvider）+ `providers.go`（11 provider 白名单）。立 **S12 包结构**（domain 平铺按概念拆，禁子目录）|
| 2026-04-24 | **apikey Repository + 18 集成测试**（CRUD / 跨用户隔离 / 分页 / GetByProvider 排序）。立 **S13 包命名**（三层同名 + `<name><role>` 别名：apikeydomain / apikeyapp / apikeystore）|
| 2026-04-24 | **apikey ConnectivityTester + HTTPTester + 21 httptest 用例**。4 种 HTTP 模式分派（openai-compatible `/models` / anthropic `/v1/messages` 1-token / google `/v1beta/models?key=` / ollama `/api/tags`）。立 **"spec 优先于邻居文件"** 审计纪律 |
| 2026-04-24 | **apikey Service + KeyProvider + 18 单测**。Service 拥有加密边界（repo 见密文、tester 见明文）。Test 编排：`repo.Get → decrypt → tester.Test → repo.UpdateTestResult → log` |
| 2026-04-24 | **apikey 5 个 HTTP 端点 + 15 个 E2E 契约测试**。`:action` URL 规范通过 `POST /{idAction}` 通配符 + `strings.Cut(":")` 拆分实现。`:test` 失败 → 422 `API_KEY_TEST_FAILED` |
| 2026-04-24 | **apikey 装配**。`router/deps.go` 加 `APIKeyService` 字段；`main.go` 串起 `MachineFingerprint → DeriveKey → AES-GCM → Store → HTTPTester → Service`。curl 实机冒烟 4/5 通过 |
| 2026-04-24 | **立设计原则 #6 "反校验剧场"**：Forgify 是本地 Electron + 单用户 + 同人写前后端。跳过"前端下拉已筛 + 下游自然报错"式的 backend 校验 |
| 2026-04-24 | **model domain 设计定档**：Q1 `/model-configs/{scenario}` 复数 path + path param；Q2 不校验 provider 白名单；Q3 不校验 hasKey。4 sentinel |
| 2026-04-24 | **文档结构重组**：`backend-rewrite.md` → `backend-design.md`；分册迁入 `service-contract-documents/`；详设计迁入 `service-design-documents/` |
| 2026-04-24 | **文档大审计 + 重写**：apikey.md 与实际代码对齐（14 处失真）。立 **设计原则 #7 + S14 "文档同步纪律"（最高优先级）**：每次代码改动联动三处文档，发现不符立刻修 |
| 2026-04-25 | **[arch-fix] providers.go 归属修正**：从 `domain/apikey/` 迁到 `app/apikey/`。理由：所有消费者都在 app 层，符合 Go "接口在消费方" 原则 |
| 2026-04-25 | **[arch] S12 文件命名规范扩展**：主文件用包名的规则从 domain 层扩展到 app / infra/store 全部三层。`service.go` → `apikey.go` / `model.go` |
| 2026-04-25 | **[arch] app/apikey 文件整合**：`keyprovider.go` + `mask.go` 合并入 `apikey.go`；测试同步合并 |
| 2026-04-25 | **model domain 完成**：7 步套路全跑完。domain（ModelConfig + 4 sentinel）→ store（9 集成测试）→ app（Service + PickForChat，12 单测）→ handler（GET + PUT，7 E2E 测试）→ errmap 4 条 → curl 冒烟 6 场景全通 |
| 2026-04-25 | **conversation domain 完成**：7 步套路全跑完。domain → store（11 集成测试）→ app（Create/List/Rename/Delete，11 单测）→ handler（POST/GET/PATCH/DELETE，6 E2E 测试）|
| 2026-04-25 | **chat domain 完成（Phase 2 版）**：Eino ReAct Agent 驱动，per-conversation 队列化（buffered channel 5）；SSE 15s keep-alive；ContentExtractor 7 种格式 + Vision；auto-titling；FTS5 全文索引（`CGO_CFLAGS="-DSQLITE_ENABLE_FTS5"`）；8 sentinel + errmap 全覆盖 |
| 2026-04-25 | **目录重组**：`backend-new/` → `backend/`；旧 Electron 代码移入 `legacy/`；`.gitignore` 按标准 Go 重写。Phase 6 原子切换内嵌完成，从路线图移除 |
| 2026-04-25 | **[doc-fix] 文档补全**：model.md / conversation.md 完整详设计；api-design.md / database-design.md / error-codes.md 同步 |
| 2026-04-26 | **[feat] apikey.ModelsFound 持久化**：`APIKey` entity 新增 `ModelsFound []string`（GORM `serializer:json`）。前端配模型时直接用作下拉选项 |
| 2026-04-26 | **[fix] SSE buffer 扩容**：`infra/events/memory/bridge.go` `defaultBufferSize` 64 → 2048，解决 DeepSeek 等快速 LLM 大量 token 事件被丢弃导致回复不完整的问题 |

### Phase 3：工具锻造（2026-04-26）

| 日期 | 内容 |
|---|---|
| 2026-04-26 | **Phase 3 开工：tool domain layer**。`domain/tool/tool.go`：5 个 entity + ExecutionResult（定义在 domain 避免循环依赖）+ 9 sentinel + Repository（30 方法）。ToolVersion 合并 pending 职责 |
| 2026-04-26 | **`infra/sandbox/python.go`**：PythonSandbox 实现，Python subprocess + 30s 超时；driver 模板追加 __main__ 桥接；Python 异常返回 ok=false 不上升为 Go error。8 测试全绿 |
| 2026-04-26 | **`domain/events/types.go` 追加 6 个 tool SSE 事件**：`tool.code_streaming` / `tool.created` / `tool.pending_created` / `tool.test_case_generated` / `tool.test_cases_done` / `tool.test_cases_not_supported` |
| 2026-04-26 | **`infra/db/schema_extras.go` 重构**：单列表 → 按 table 分组的 extraGroup 结构。追加 tools 部分唯一索引 `UNIQUE(user_id, name) WHERE deleted_at IS NULL` |
| 2026-04-26 | **[arch] 工具搜索方案切换**：chromem-go 向量搜索 → LLM 排序（SearchTool 把全部工具发给 LLM 选最相关 N 个）。删除 `infra/vectordb/`，移除 chromem-go 依赖 |
| 2026-04-26 | **`infra/store/tool/tool.go`**：完整 Repository 实现，30 个方法，覆盖 Tool CRUD / Version+Pending / TestCase / RunHistory / TestHistory。11 集成测试全绿 |
| 2026-04-26 | **`app/tool/ast.go`**：Python subprocess AST 解析，提取函数名/参数（含 required/description/default）/返回值。Google-style docstring 解析，无 docstring 不报错 |
| 2026-04-26 | **`app/tool/tool.go`**：Service 完整实现，含 CRUD / 版本管理 / pending 生命周期 / sandbox 执行 / 测试用例 / LLM 生成测试用例（emit callback 解耦 HTTP）/ 导入导出 |
| 2026-04-26 | **`app/agent/forge.go`**：5 个 System Tool（SearchTool/GetTool/CreateTool/EditTool/RunTool）+ ForgeTools 工厂。SearchTool 用 LLM 排序；Create/EditTool 流式推 ToolCodeStreaming SSE；RunTool att_id 解析 |
| 2026-04-26 | **Phase 3 装配 + 冒烟**：handlers/tool.go（22 端点）+ errmap 9 条 + main.go（Migrate 5 表、创建 sandbox/toolService、ForgeTools 注入 chatService.SetTools）。curl 验证 create / list / :run / versions / run-history / delete 全通 |
| 2026-04-26 | **[feat] testend 工具面板**：新增 Tools tab（System + User Tools 子面板）。`GET /dev/tools` 列出注册 tool；`POST /dev/invoke` 直接调用任意 system tool（绕过 LLM agent，用于冒烟） |
| 2026-04-26 | **[feat] testend SSE 双视图 + chat tool 步骤卡片**：SSE 标签页加 Stream/Raw 切换；chat 面板 assistant 消息内嵌 tool step collapsible 卡片（⚙ running → ✓ ok/✗ error） |
| 2026-04-26 | **[feat] chat tool call 可见性**：`app/chat/chat.go` 拆分为 4 文件（chat / pipeline / interceptor / util）。新增 `toolInterceptor` 包装所有 tool，发布 `chat.tool_call` / `chat.tool_result` SSE（含 `summary` 人类可读）。`Summarizable` 接口 + `CoreInfo` 方法 |

### Phase 3 后基础设施优化轮（2026-04-27 起）

Phase 3 完成后未直接开工 Phase 4，而是进入一轮深度优化与调研——chat 架构重构、生产 bug 收尾、开发体验改进、Claude Code 内部机制调研、SQLite 驱动迁移、桌面端分发方向定型。

#### Chat 基础设施重构（2026-04-27）

| 日期 | 内容 |
|---|---|
| 2026-04-27 | **[refactor] 重构决策**：审计 chat 管线发现 3 处系统性设计债——DB schema 拍扁多列 / Eino 黑盒渗透 app 层 / collectStream 收完再推。新增 `archaved/refactor-chat-infra.md` 设计文档 |
| 2026-04-27 | **[arch]** 自实现 ReAct Loop 替换 `react.NewAgent + Callback`：Eino v0.8.11 `OnEnd` 对流式不触发，改直接调 `ToolCallingChatModel.WithTools().Stream()` |
| 2026-04-27 | **[refactor Step 1]** 新建 `internal/infra/llm/`（4 文件 OpenAI-compat + Anthropic 原生）替代 Eino，`iter.Seq[StreamEvent]` 替代 channel |
| 2026-04-27 | **[refactor Steps 2-11]** chat 基础设施全量重构完成：Tool 接口 4 方法、Message 拆 Block 模型（5 类型）、message_blocks 新表、自实现 ReAct 替 Eino agent、`app/chat/` 拆 5 文件、Eino import 全清 |
| 2026-04-27 | **[refactor 测试补全]**：infra/llm 21 / app/agent 35 / app/chat 18 / store/chat Block 模型适配 + 3 新增。22 包全绿 |
| 2026-04-27 | **[fix]** 修 3 处 ReAct 严重 bug：多步循环 DB 覆盖（统一 allBlocks 累积一次保存）/ maxSteps 退出 stopReason 错 / 用户消息附件 block 缺元数据 |
| 2026-04-27 | **[refactor]** 代码清理：删 `app/agent/summarizable.go`；统一 `blocksToAssistantLLM`；修 S13 alias 违规 |
| 2026-04-27 | **[fix] T15-T19 补丁 5 条**：forge.go ctx helpers / GenerateTestCases 改 json.RawMessage / extractJSON 剥 markdown fence / extractTextContent 取最后 text block / chatRepo 共享单例 |
| 2026-04-27 | **[feat] Thinking 可见性**：新增 `chat.reasoning_token` SSE + `Message.ReasoningContent` 字段（DeepSeek-R1 history 重建必需）；testend 加 `🤔 Thinking…` 折叠块 |
| 2026-04-27 | **[fix]** 集成测试拍出 4 个生产 bug：created_at=0001 错排（OnConflict.DoUpdates 修）/ 取消流后助手消息丢失（detached ctx）/ web_search 返 null（切 lite.duckduckgo）/ 快速连发历史顺序错 |
| 2026-04-27 | **[test]** 集成测试 13 组（A-M）全通（真实 DeepSeek API），覆盖 CRUD / API Key / 分页 / 工具 / ReAct / Attachment / Auto-title / SSE messageId 等 |
| 2026-04-27 | **[doc-sync] events-design.md / database-design.md / chat.md** 全量同步：messages 表精简、message_blocks 新表、chat.tool_call_start / chat.reasoning_token 新增 |

#### Chat pipeline 二次重构（2026-04-27 后）

| 日期 | 内容 |
|---|---|
| 2026-04-27+ | **[refactor]** 移除 pipeline.go，引入 runner.go（commit b6a9199）：chat 执行管道二次拆分，为后续 context compaction 预留接口 |

#### 开发体验工程化

| 日期 | 内容 |
|---|---|
| 2026-04-27+ | **[devx]** Brewfile + Makefile setup target + 11 testend YAML collections（commit 6113d16）：`make setup` 一键检查 Xcode CLT / 装 Homebrew / 装依赖 |

#### Claude Code 内部机制调研

| 日期 | 内容 |
|---|---|
| 2026-04-28 | **[research]** Claude Code 内部机制调研：产出 `claude-code-research-documents/` 9 份主题报告 + `agent-core-upgrade.md` + `summary.md`，为 Phase 4-5 chat 终极版设计提供参考 |

#### SQLite 驱动迁移（2026-05-01）

| 日期 | 内容 |
|---|---|
| 2026-05-01 | **[infra]** SQLite 驱动 mattn → modernc.org/sqlite（纯 Go），三平台一行交叉编译，删 CGO_CFLAGS。性能慢 1.5-2x（本地无感） |

#### 桌面端分发方向定型（2026-04-30 ~ 2026-05-01）

| 日期 | 内容 |
|---|---|
| 2026-04-30 | **[doc]** 桌面端分发方向定型 + `desktop-packaging-notes.md` 落地：目标 Wails 原生桌面 app（窗口外壳 + 复用 httpapi，不走 binding）；分发 dmg/setup.exe/AppImage（v0.1 起 L3）；Python 沙箱短期 A、中期 C |
| 2026-05-01 | **[doc]** 常驻后台模式 + Notifier 接口决策：scheduler 不退出（关窗 ≠ 退出）。Phase 4 写 scheduler 时 `domain/notification/Notifier` 接口先行；桌面壳代码限 `internal/infra/desktop/` |
| 2026-05-01 | **[doc]** 决定不走 Wails binding：HTTP 等价但能复用 v1.2 transport（middleware/errmap/curl）；SSE 天然契合；binding 只换"类型同步"一项收益，OpenAPI 也能做到 |
| 2026-05-01 | **[refactor]** `schema_extras` guard 改 `db.Migrator().HasTable()` 替代 raw `sqlite_master` 查询；真正 GORM 写不出的 SQL（partial UNIQUE 等）仍走 raw exec |
| 2026-05-01 | **[refactor]** message_blocks 复合索引 `(MessageID, Seq)` 迁到 GORM tag（`index:idx_mb_msg_seq,priority:N`），删 schema_extras 对应 group |
| 2026-05-01 | **[cleanup]** 死代码清扫：删 3 个未发布的 `ToolTestCase*` event 类型（SSE 实际走 callback 不经 Bridge）；events-design.md 同步 |
| 2026-05-01 | **[arch]** pagination 迁到 `pkg/pagination` + S13 全代码补别名：4 store 反向 import 各自抄一份合并删 ~64 行；S13 加 `httpapi` 后缀全代码补 `*httpapi` 别名 |
| 2026-05-01 | **[fix] staticcheck 全套 5 修**：恢复误删 ListProviders/ListScenarios（deadcode 默认不扫测试）；SA1029 改 `//lint:ignore`；S1016 改类型转换。staticcheck 0 |
| 2026-05-01 | **[fix]** 5 处 `_ = err` 静默吞错改正：tool.newID 加 panic 与其他 newID 一致；tool.Import/Export 加 log.Warn；2 处 w.Write 加注释保留 |
| 2026-05-01 | **[review]** TODO 扫描：全代码仅 3 处 TODO 全是合法前瞻性标记（A1 中流执行 / context compaction 钩子点），无历史包袱 |
| 2026-05-01 | **[refactor]** `userID(ctx)` helper 统一到 `pkg/reqctx`：合 11 处重复；新增 `ErrMissingUserID` sentinel + `RequireUserID` helper。事故：sed 清空 apikey store，立教训"项目内禁用 sed 改 import / 函数名" |
| 2026-05-01 | **[review]** errmap 完整性反查：32 个 domain sentinel 全部已映射 ✅；补登记 `reqctxpkg.ErrMissingUserID` + `cryptoinfra.ErrUnsupportedVersion`（均 500） |
| 2026-05-01 | **[arch]** S5 / S6 降级为参考线：行数当硬规则会噪音（main.go DI / SSE 状态机 / Service 956 行都是结构必要）；改措辞"可读性优先于行数"。同步 backend-design.md |
| 2026-05-01 | **[review]** S13 别名全代码验证：176 处 internal import 0 处无别名 ✅，32 个别名全部规范后缀，100% 合规 |
| 2026-05-01 | **[refactor]** 跨 store 共享 Cursor 类型：4 store 的 `pageCursor` JSON tag 漂移，`pkg/pagination` 加共享 `Cursor` 类型，4 store 删本地副本统一为 `c` |
| 2026-05-01 | **[doc]** V1.2 文档全量校对：11 份反查代码 drift——testend-design 整体重写 / backend-design tree 更新 / chat.md pipeline→runner / 5 份 service-design 去 Eino 残留 |
| 2026-05-01 | **[arch]** backend-design.md 规范补完：新增 N6 / D6 / D7 / S15 / S16 / S17，扩 S9 detached context；新增 **T 系列测试规范**（T1-T4）+ **开发期工具纪律**（staticcheck / deadcode -test / 禁 sed 改 import） |
| 2026-05-01 | **[doc]** 创建项目根 `CLAUDE.md` + `backend-design.md` 拆分：把全部代码规范从后者搬到前者（自动加载进 context）；后者退化为"项目说明书"。前者 378 行 / 后者 304 行 |

#### Tool 系统大重构（2026-05-02 起，Phase 0-8 计划，Phase 0-7 完成，Phase 8 进行中）

对照 Claude Code 调研后认定当前 tool 实现"基础设施过于薄"。原 7 阶段计划中途扩成 8 阶段（Phase 5/6 改造为 DB schema 统一 + SSE 3-event entity-state 模型，原 Phase 5 重建 system tools 撤销）。

**关键决策**：(1) 推流仍 `bridge.Publish` 直调不引 emit 抽象；(2) agent 包改 tool / 原 app/tool 改 app/forge；(3) §S12 例外允许 tool/ 嵌套子包；(4) 每 user-facing domain 一个 SSE entity-state 事件；(5) Phase 5 数据库统一（forge_executions 合表 / Forge.Pending / Message errorCode 等）。

| 日期 | 内容 |
|---|---|
| 2026-05-03 | **[devx]** 测试包 `internal/e2e/` → `backend/test/`（build tag `e2e` → `pipeline`）；Makefile 加 `test-pipeline` / `test-console`。`forgeapp.Service.PublishSnapshot` 是 forge SSE 唯一发布点（与 chat.runner 同模式） |
| 2026-05-03 | **[test]** Step 3 chat E2E 5 场景全绿（~11s）：SimpleText / MissingModelConfig / ToolCall / CancelMidStream / ReasoningModel |
| 2026-05-02 | **[test]** Step 2 E2E harness：`internal/e2e/` 3 文件（harness/seed/sse），real DI + entity-state SSE 解析。smoke 680ms |
| 2026-05-02 | **[fix]** Step 1 防御代码大摸排：全 backend `_ = err` 扫描，修 6 处真问题 + 2 加 Warn + 1 conversation 时戳 flake。`.env` 注入 + Makefile 3 targets |
| 2026-05-02 | **[doc]** Phase 7 文档同步 #2：8 份跟齐 Phase 5/6 改造（events / database / forge / chat / api） |
| 2026-05-02 | **[refactor]** Phase 6 SSE 12→3 entity-state 事件（chat.message / forge / conversation），载荷 = entity GET 完整快照；`runner.go` 三 helper 是 chat.message 唯一发布源。22 包绿 |
| 2026-05-02 | **[refactor]** Phase 5 DB schema 重构 4 领域：forge_executions 统一表 / Forge.Pending 计算字段 / Message 加 errorCode/Message/updatedAt / attachments 软删。22 包绿 |
| 2026-05-02 | **[refactor]** Phase 0 清理过时 tool：删 `app/agent/{system,web}.go` 共 8 件（read_file/write_file/list_dir/run_shell/run_python/datetime/web_search/fetch_url），Phase 5 重建 |
| 2026-05-02 | **[refactor]** Phase 0 GenerateTestCases 去流改普通 HTTP（底层 `llm.Generate` 本就非流式）。`GenerateEvent` 删；新增 `GenerateResult{NotSupported, Reason, TestCases}` |
| 2026-05-03 | **[refactor]** 重复实现 8 项整改：新建 `pkg/idgen` / `pkg/llmparse` / `pkg/llmclient`；`forgeapp.PublishSnapshot` 收敛 6 处；`response.StreamSSE[T]` 泛型 helper |
| 2026-05-02 | **[fix]** testend 前端跟齐 Phase 0-3：tab-sse `forge.*` 事件 / tab-tools generateTestCases 改普通 fetch / tab-sql f_/fv_ 前缀 / destructive 红色徽章 |
| 2026-05-02 | **[doc]** Phase 4 文档同步 #1：Phase 0-3 跟齐 6 文件（CLAUDE.md §S15/§S18 / events / database / api / backend-design / forge / chat） |
| 2026-05-02 | **[refactor]** Phase 3 Tool 接口扩 10 方法 + forge tool 重写：3 静态元字段 + 3 钩子；destructive per-call AI 自报落 DB；5 forge tool 移 `tool/forge/`；runTools 改 IsConcurrencySafe 分批。22 包绿 |
| 2026-05-02 | **[arch]** Phase 2 `agent/` → `tool/` 包重组：CLAUDE.md §S12/§S13 加例外条款（tool 是 framework meta-namespace 允许嵌套子包） |
| 2026-05-02 | **[refactor]** Phase 1 大重命名 tool → forge："用户造的 Tool" 全语义改 Forge：6 entity / 5 表 / ID 前缀 t_→f_ / 22 端点 / 5 LLM-facing 名 / 3 Bridge 事件 / testend 161 处。**保留** Tool 接口 / ChatToolCall / tc_ 前缀 |

#### 沙箱方向迭代设计（2026-05-02）

| 日期 | 内容 |
|---|---|
| 2026-05-03 | **[devx]** devbox + Makefile 二轮整理：`$HOME/go/bin` 入 PATH；加 `_require-devbox` / `_refuse-inside-devbox` 两守卫；help 加 Setup/Daily 分组 |
| 2026-05-03 | **[devx]** Makefile 收成 5 核心命令 + help 默认 target：`environment` / `test-console` / `test-unit` / `test-pipeline` / `stop`。删 `make dev` / `logs` / `LOG_FILE` |
| 2026-05-03 | **[devx]** 依赖基线 + devbox 落地：Go 1.25.5；modernc.org/sqlite v1.23→v1.50（27 minor）；devbox 锁 go/python/uv/make；bootstrap 装 staticcheck/deadcode + 沙箱资源。~190 单测全绿 |
| 2026-05-03 | **[fix]** draft forge 首拒后该消失但留下空壳：`Service.RejectPending` 末尾若 `ActiveVersionID==""` 触发 `s.Delete(forgeID)`；已 active 的 forge 行为不变。新增 2 测试 + forge.md §8.5 |
| 2026-05-03 | **[fix/devx]** 沙箱迭代 1 出场 bug：`parse()` 不再裸吞 cause；加 `ensure-resources` target；新增 `smoke_bootstrap_test.go`（§T3 门控） |
| 2026-05-03 | **[doc]** 沙箱迭代 1 Phase G 收尾：8 份契约文档全量同步（forge.md ~360 行新内容为最大份） |
| 2026-05-03 | **[refactor]** 沙箱迭代 1 实施完工（Phase A-F）：sandbox 包 10 文件重写；Forge/ForgeVersion 加 env 字段；Sandbox 接口 6 方法；Service lifecycle 改造（CreateDraft / env 守卫 / LRU）。~80+13+19+11 测全绿 |
| 2026-05-03 | **[doc]** 沙箱迭代 1 MVP "punt 给 AI 自救" 哲学：砍 5 个自动恢复机制，保留 2 个真兜底（mac codesign / EnvError 收集） |
| 2026-05-03 | **[doc]** 沙箱迭代 1 反查 5 处认知偏差（wheel clone/hardlink / Python embed.FS / uv stage 名 / macOS `com.apple.provenance` codesign 重签）。详见 `desktop-packaging-notes.md §六` |
| 2026-05-02 | **[doc]** 沙箱迭代 1 设计文档 v2 重写：EnvID 算法（sha256+排序）/ 磁盘布局 / N=3 LRU / EnvStatus 5 态。**vs v1**：砍异步 sync worker / 不引新 SSE 事件类 / create_forge 进 pending / 删 SandboxTimeout |

#### 测试流水线迭代设计（2026-05-03）

| 日期 | 内容 |
|---|---|
| 2026-05-03 | **[fix]** sandbox `uv sync` 加 `--no-workspace` 试图阻止 `.venv` 外溢（事后查 flag 不存在，05-04 回退） |
| 2026-05-03 | **[infra/test]** 流水线迭代 1 Phase G 收尾：Makefile 加 `test-pipeline` / `-quick` / `-live` 三 target；CLAUDE.md 加 T6（fake LLM 约定）。**迭代 1 全 7 段完工** |
| 2026-05-03 | **[infra/test]** Phase E+F：`chat_forge` (3) / `errcodes` (16+3) / `isolation` (3) pipeline tests。**68 测全绿，2.6s** |
| 2026-05-03 | **[infra/test]** Phase D：`forge_http` (12) + `forge_lifecycle` (4) pipeline tests，`RequireForgeResources` gate |
| 2026-05-03 | **[infra/test]** Phase C：5 个 chat 场景 pipeline tests（basic/react/autotitle/queue/attachment）；harness `SetMaxOpenConns(1)` 修 in-mem SQLite 多连接 bug。**30 测全绿，2.3s** |
| 2026-05-03 | **[infra/test]** Phase B：`apikey` (5) / `model` (4) / `conversation` (4) pipeline tests；fake_llm 加 `/v1/models`。**19 测全绿，1.1s** |
| 2026-05-03 | **[infra/test]** Phase A：fake LLM 基础设施（`fake_llm.go` httptest + 5 scripts + 5 helpers）；harness 修 sandbox drift；4 chat 测试切 fake LLM 离线可跑。**5 测，1.0s** |
| 2026-05-03 | **[doc]** 流水线迭代 1 设计文档：`adhoc-topic-documents/test-pipeline-iteration-documents/01-foundation-and-coverage.md`，~13h / 7 phase / fake LLM + 真 sandbox 双层 / ~80 测覆盖目标。完整方案见该文件 |

#### Claude Code tool 抄录研究（2026-05-03）

| 日期 | 内容 |
|---|---|
| 2026-05-03 | **[research]** CC tool 抄录启动：v2.1.88→v2.1.126 delta + 41 工具 inventory（8 P0 / 7 P1 / 13 P2 / 10 Skip）。新建 `02-tools-deep/00-inventory.md` |
| 2026-05-03 | **[research]** deep-dive `01-file-ops.md`：Read/Write/Edit Piebald 原文 + Go 实现 6 节。MultiEdit 已下线（issue #11125 not planned），inventory P1 7→6 |
| 2026-05-03 | **[research]** deep-dive `02-search.md`：Grep（ripgrep wrapper 12 字段）+ Glob（doublestar + mtime-desc + 1000 cap）。LS 已下线，文档留 A/B 两方案待决 |
| 2026-05-03 | **[research]** deep-dive `03-shell.md`：Bash 描述全 41 子文件抓取 + Go 实现（cwd 状态机 + dangerous detect + background + 30K 截断）。**v1 不做 OS-level sandbox**，用 PathGuard + Ask pattern 替代 |
| 2026-05-03 | **[research]** deep-dive `04-web.md`：WebFetch（HTML→md + 小模型摘要 + 15min cache，独立 context）+ WebSearch（CC 美国限制改接 Tavily）。Forgify 走 Jina Reader 优先 + html-to-markdown fallback |
| 2026-05-03 | **[research]** deep-dive `05-ux-tasks.md`：AskUserQuestion + TaskCreate 族 4-in-1 + TodoWrite legacy + EnterPlanMode 简评。**02-tools-deep 系列收官**，5 篇覆盖 15 P0/P1 |
| 2026-05-03 | **[research]** **02-tools-deep 13 决策复审 + V1 清单**：8 P0（Read/Write/Edit/Glob/Grep/Bash/WebFetch/WebSearch）+ 5 P1（Task 族 4 + AskUserQuestion）+ 框架重构（execution_group / AgentState / PathGuard）= **13 工具 + 0.6d 框架，~7 天**。详 13 决策见 `02-tools-deep/` 各篇 |
| 2026-05-03 | **[refactor]** 框架重构 F1-F10（V1 工具前置）：新增 `pkg/agentstate` + `pkg/pathguard` + `pkg/reqctx/agentstate.go`（共 15 测）；Tool 接口 10→9 方法；`StandardFields` 加 `ExecutionGroup`；分批改按 LLM 自报 group 调度 |
| 2026-05-03 | **[fix/devx]** `llm_integration_test.go::testKey()` 从 `"shabi"` placeholder 改 `requireKey(t)+t.Skip`（per §T3）；CLAUDE.md 加"测试命令选择"小节（禁直跑 `go test ./...`） |
| 2026-05-03 | **[feat]** O1 Read tool：`app/tool/filesystem/{filesystem,read}.go`，9 方法 + 19 单测。chat 层 wire AgentState（convQueue 字段 + ctx 注入）让 must-Read-first 守卫工作 |
| 2026-05-03 | **[feat]** O2 Write tool：`write.go`，9 方法 + 13 单测。原子写 `CreateTemp+Rename`，覆写保留原 mode |
| 2026-05-04 | **[feat]** O3 Edit tool：`edit.go`，9 方法 + 19 单测，含 `#51986 markdown bold 5 处全替`。信任 stdlib `strings.Replace`，显式报 N occurrence 比 CC "All replaced" 透明 |
| 2026-05-04 | **[fix]** sandbox `sync.go` 删 `--no-workspace`——uv 0.11.8 无此 flag（昨日加错），真正建 `.venv` 的源头是 devbox python venvShellHook（已修） |
| 2026-05-04 | **[feat/test]** O4 file-ops 装配 + pipeline test：`main.go` + `harness.go` 装 PathGuard + FilesystemTools；新建 `test/filesystem/` 3 场景（ReadEditClosedLoop / WriteWithoutReadDenied / PathGuardDeniesSensitivePath）。29.7s 通过 |
| 2026-05-04 | **[feat]** S1 Grep tool：`search/{grep,grep_rg,grep_stdlib}.go`，9 方法 + 28 单测。双后端 rg shell out（10-100× 加速）+ stdlib 兜底，surface 一致。装到 main + harness |
| 2026-05-04 | **[feat]** S2 Glob tool：`search/glob.go`，9 方法 + 19 单测。`doublestar.Glob` over `os.DirFS`，mtime 降序，limit 100/1000。决策 D3：pattern `*` 即 LS 替代，无单独 LS tool |
| 2026-05-04 | **[test]** S3 search pipeline test：`test/search/` 3 场景（GrepFindsMatches / GlobListsDirectoryWithMetadata / GrepPathGuardDeniesSensitivePath）。40s 通过；errmap 无变更——tool 错误返友好字符串不到 handler |
| 2026-05-04 | **[feat]** W1 model `web_summary` scenario：domain 加 `ScenarioWebSummary` 常量 + `IsValidScenario`/`ListScenarios` 扩展；`ModelPicker` 接口加 `PickForWebSummary`，`*Service` 实现。WebFetch 工具未配置时 fallback 到 `PickForChat`。model.md 同步 4 处（清单/接口/字段/方法签名）|
| 2026-05-04 | **[feat]** W2 WebFetch tool（24 单测）：Jina → 直 GET fallback；30s/1MB cap；SSRF 守卫（含 DNS rebinding 防御）。`pkg/llmclient` 加 `ResolveForWebSummary`。 |
| 2026-05-04 | **[feat]** W3 WebSearch tool（21 单测）：3 层 fallback SearXNG 池 → Bing HTML → Bing CN HTML，每后端 10s 超时；`x/net/html` visitor 解析；`FORGIFY_SEARXNG_INSTANCES` env 可覆盖池。 |
| 2026-05-04 | **[test]** W4 web 装配 + pipeline test：`main.go` + `harness.go` 装 WebTools；`test/web/` 2 场景（WebFetchBlocksLoopback / WebSearchRejectsEmptyQuery）故意 short-circuit 不打外网，验 LLM ↔ tool 接线。11s 通过；errmap 无变更——tool 错误返友好字符串不到 handler |
| 2026-05-04 | **[feat]** B1 shell 三件套（Bash/BashOutput/KillShell，47 单测）：cwd 状态机（`cd` 整命令短路，链式不追踪）+ 前后台双模式（前台 120s/600s，后台 spawn 返 `bsh_<16hex>`）+ 256KB 环形缓冲。故意不带 banned-command 表（本地单用户）。 |
| 2026-05-04 | **[test]** B2 shell 装配 + pipeline（3 场景，19s 通过）：main 装 ShellTools + 优雅关停杀子。tool 错误返友好字符串不到 handler，errmap 无变更。 |
| 2026-05-04 | **[feat]** U1 task mini-domain + 4 tools（4 层 + 60+ 单测）：Task entity 含 ConversationID 作用域、status 白名单、`tk_<16hex>` ID；跨 conv 报 ErrNotFound 防泄漏；变更发 `task` SSE。`pkg/reqctx` 加 `RequireConversationID`。 |
| 2026-05-04 | **[feat]** U2 AskUserQuestion 后端：`app/ask`（in-memory 会合 Service，Wait 阻塞 + Resolve 原子防双答竞态）+ AskUserQuestion 工具（5 min 超时）+ `POST /conversations/{id}/answers`。决策 D11：不新建事件家族。errmap 加 7 行。 |
| 2026-05-04 | **[test]** U3 ux-tasks 装配 + pipeline test：harness `eventsBridge` 笔误修正为 `bridge`；`test/uxtask/` 3 场景（TaskCreateAndList / AskUserQuestionAnswerDelivered / AnswerEndpointUnknownCallID_404，旁路 goroutine POST 答案验真实接线）。20s 通过；pipeline 全 12 suite 全绿 |
| 2026-05-04 | **[doc]** Z1 V1 batch 文档全量同步：新建 task.md；api/database/error-codes/events-design 4 契约文档 + chat.md（9 方法 + execution_group + AgentState 注入 + 20 工具）全更。 |
| 2026-05-03 | **[devx]** 项目根 + Makefile + devbox 瘦身：删 `.githooks/` / `.air.toml` / `tmp/` / `scripts/`；Makefile 砍 4 项；devbox 删 `python@3.12`（venvShellHook `.venv` 坑）+ `uv@0.11`（装饰） |

#### Phase 4 准备件 — 4 domain 设计批（2026-05-05，待实施）

为 Phase 4-5 的 workflow / 智能化 提前打地基。整批仅设计文档落档，代码未动；预估 ~10 天实施周期（mid-month deadline）。

| 日期 | 内容 |
|---|---|
| 2026-05-05 | **[doc]** 4 份 service-design 文档落档 ~2700 行：`subagent.md` / `mcp.md` / `skill.md` / `catalog.md`。完整方案见各文件 |
| 2026-05-05 | **[doc]** 4 份 contract 文档全量同步（api-design 加 ~25 端点 / database-design 加 subagent_runs + subagent_messages 两表 / error-codes 加 ~20 sentinel / events-design 加 subagent + mcp + skill + forge.persisted 4 事件 + chat.message 加 subagentRunId 字段）；backend-design.md Architecture 树加 4 新 domain |
| 2026-05-05 | **[arch]** 4 domain 关键设计决策：① subagent 双流 SSE（chat.message 带 subagentRunId 复用主对话 schema）；② MCP search/call 模式不 flat 注册（避 70k token 启动开销）；③ Skill 三层 progressive disclosure；④ CatalogSource port 反转（新 source 0 行改 catalog）；⑤ MCP 内置 8 server Registry + marketplace。 |
| 2026-05-05 | **[arch]** 自检纠偏：删 `enabled` 字段（catalog 已解 token 爆炸）+ 删 skill/mcp 项目级目录 + 删 catalog routing-hints 用户文件 + catalog debouncer→1s BurstCoalescer + 仅订阅 forge.persisted + fingerprint dedup（name/desc 不变跳过 LLM）。 |
| 2026-05-05 | **[arch]** P0 生产级缺口补全：MCP per-call 30s timeout + cancel 级联 + stderr ring buffer + mcp.json fail-soft；Subagent ctx 级联 + 5min 超时 + panic recover + RunID isolation；Skill fork 防护 + symlink 循环防护；Catalog 全 fail 保旧 cache + output 2k cap。 |
| 2026-05-05 | **[arch]** Catalog 触发机制简化：事件订阅 + BurstCoalescer + dirty-loop → 1s polling + atomic.Bool 单 flight + fingerprint dedup（~150 行→~30 行）。CatalogSource 砍 `EventTopics()`；删 forge.persisted 事件。 |
| 2026-05-05 | **[arch]** Subagent SSE 简化：双流 → 单流 chat.message（载荷嵌完整 subagentRun 快照）。删独立 `subagent` 事件类型，事件总数 7→6。前端按 subagentRunId 分流。 |
| 2026-05-05 | **[arch]** 终轮自审修 5 stale + 2 过度设计：catalog.md/subagent.md 残留清理；Skill ActiveSkill 栈 → `atomic.Pointer` last-write-wins；Subagent LastTool* 5 字段从表移到 in-memory `gorm:"-"`。 |
| 2026-05-05 | **[arch]** Catalog 失败策略落定"用户活跃度驱动重试"：LLM 失败 → mechanical fallback + lastFP 照更；用户改东西 fp 变才重试（无后台 backoff）。Generator 内部重试 2 次（共 3 attempt），key 轮训改真跑 LLM 调用。 |
| 2026-05-05 | **[devx]** 排程从 D1-D10 砍到 **D1-D8**，binary 打包 + 上手文档 + demo 预演由用户自己解决 |
| 2026-05-05 | **[arch]** Subagent 整体从 catalog 移除：原设计有 SubagentCatalogSource 列举 3 个内置类型，但 Subagent system tool 自身 description 已覆盖 subagent 类型说明，catalog 再列一遍冗余。CatalogSource 实现方从 4 个收为 **3 个：forge / skill / mcp** |
| 2026-05-05 | **[arch]** Subagent spawn 工具改名 **`Task` → `Subagent`**——避开与已有 `task` mini-domain（TaskCreate/List/Get/Update 管 TODO）的 LLM-facing 命名撞车；Go struct `TaskTool` → `SubagentTool`，包 `app/tool/agent` → `app/tool/subagent` |
| 2026-05-05 | **[arch]** ⭐ Sandbox v2 大重构：forge-only（uv + python bundle）→ 统一 PluginSandbox。Bootstrap 极简（仅 mise ~10MB go:embed），所有语言 runtime lazy install；per-plugin env 隔离（forge/mcp/skill/conversation 4 owner）；SQLite 双表 manifest。设计见 sandbox.md（~940 行）。 |
| 2026-05-05 | **[arch]** Bash 自动路由 + 对话 scratch env：LLM 跑 `pip install`/`python x.py` 时 sandbox 检测意图自动路由到该对话 scratch env。denylist 整套机制不再需要（靠基础设施收口）。conversation env 30d auto-GC。 |
| 2026-05-05 | **[arch]** 摆脱 OOTB 预装：原 mcp.md 的 cmd/resources 扩展预装 5 个 server + Chromium 设计**全部废弃**——改 lazy install via sandbox v2。Forgify 总安装 ~25 MB binary + ~10 MB mise bootstrap = ~35 MB，比原 ~250 MB 砍 85%。用户首装某 server 才触发 runtime + 包的下载（带进度条）|
| 2026-05-05 | **[devx]** 配套改造：Makefile clear 加清 sandbox；devbox bootstrap 去 cmd/resources；cmd/resources 重写为 mise fetcher；main.go sandbox 装配段重写；bash.go 加 detectRuntime 自动路由。 |
| 2026-05-05 | **[doc]** 沙箱 v2 文档同步：sandbox.md 新建（943 行）+ database-design 加两表 + error-codes 加 8 sentinel + api-design 加 sandbox endpoints + backend-design 加 domain/app/infra 三层 + forge.md / mcp.md 改为指向 sandbox.md（forge sandbox 接口保留作 adapter；mcp 整段 OOTB 预装设计删除）|
| 2026-05-05 | **[arch]** 包管理器共享机制 + GC 简化：Node 由 npm 改 **pnpm**（content-addressable global store，多 conv 共装同包磁盘 ≈ 1×）；uv 已自带 hardlink wheel cache。**v1 全 owner 默认手动 GC**（共享让磁盘开销极小，auto-GC 价值低）；用户主动点 `:gc` 端点或 plugin 卸载时触发 |
| 2026-05-05 | **[arch]** Sandbox bootstrap 失败 → Degraded Mode：app 仍启动，纯文本 chat + 不需 runtime 的工具可用，needs-runtime 操作 fail-fast 返友好错 + UI banner/retry。新增 bootstrap-status + :retry-bootstrap 端点。 |
| 2026-05-05 | **[arch]** ⭐ Windows v1 加入：5 平台 binary；Bash 用 PowerShell 分支；进程 cancel 用 Job Object；flock 用 LockFileEx；fsnotify ReadDirectoryChangesW；mise per-platform embed。Ruby/PHP/长尾经 UnsupportedPlatforms 在 Windows 隐藏。 |
| 2026-05-05 | **[devx]** 排程从 D8 → **D15**：Windows 适配 + 测试加 D10-D15（~6 天）；总周期内仍能赶投资人月中回来 demo |
| 2026-05-05 | **[arch]** mcp.md §5.5 内置 marketplace 重选 5 个（Playwright/MarkItDown/Context7/DuckDuckGo/SQLite + everything hidden）；砍与内置工具/原生计划重复的。RegistryEntry 加 Bundled/Hidden/PostInstallSteps/OnlineOnly/Notes 5 字段。（注：OOTB 预装后被 lazy install 取代。） |

#### Phase 4 准备件 — D1 sandbox v2 实施（2026-05-05~）

| 日期 | 内容 |
|---|---|
| 2026-05-05 | **[feat]** D1-2 sandbox domain 包落地：`internal/domain/sandbox/`（sandbox.go + installer.go，~410 行）—— Runtime/Env 两实体 + Owner/RuntimeSpec/EnvSpec/SpawnOpts/ExecutionResult/LongLivedHandle/ProgressFunc 值对象 + 8 sentinels + Repository + RuntimeInstaller/EnvManager 双端口 |
| 2026-05-05 | **[feat]** D1-3 sandbox store：Repository GORM 实现（Runtime/Env CRUD + 多查询方法），系统级表不按 userID 过滤。19 集成测试全绿。 |
| 2026-05-05 | **[feat]** D1-4/5/6 sandbox 装配：AutoMigrate 两表 + errmap 8 sentinel + `bootstrap_mise.go` 骨架（per-platform go:embed，darwin ad-hoc codesign 绕 Gatekeeper）。 |
| 2026-05-05 | **[feat]** D2-1 mise binary fetcher：`cmd/resources` 重写为 mise per-platform 下载器（SHA256 + 原子写 + 幂等），输出到源码树给 go:embed。Makefile target `fetch-mise`→`resources`。v1 dev resources 不再消费。 |
| 2026-05-05 | **[feat]** D2-2 mise embed.FS + ExtractMiseBinary：5 个 per-platform build-tag embed 文件 + unsupported fallback；写 mise 到 dataDir + chmod + darwin codesign + SHA256 幂等。3 单测 + 6 平台 cross 全过。 |
| 2026-05-05 | **[feat]** D2-3a mise generic Installer + Python EnvManager：通配 RuntimeInstaller（共享 MISE_DATA_DIR + `mise where` 解析部分版本约束）+ Python `uv venv`。RuntimeInstaller 签名改返 relPath（mise 用全局 data dir）。6 单测，真 install 由 D9 pipeline 覆盖。 |
| 2026-05-05 | **[doc]** D2-3 范围澄清：全部 11 EnvManager + 4 Installer 进 D2 不延后（投资人承诺）；Java 选方案 A（每 env 独立 Maven repo）。D2-3 拆 a-f 6 子任务。 |
| 2026-05-05 | **[feat]** D2-3b Node + Playwright：pnpm content-addressable store 多 env 共享磁盘 + Playwright 全 env 共享 browsers 缓存（避免重下 ~300MB Chromium）。11 单测，真网络 install 推 D9。 |
| 2026-05-05 | **[feat]** D2-3c Generic + Static binary：Generic 兜底（mise 长尾 600+ 语言 no-op deps）+ Static（HTTP GET → chmod → darwin codesign，version 支持 `sha256:...@URL` 校验）。13 单测。 |
| 2026-05-05 | **[feat]** D2-3d Rust + Go：`cargo install --root` + `go install`（GOPATH/GOBIN per-env），针对 binary CLI 工具。每 env 独立 cache。7 单测。 |
| 2026-05-05 | **[feat]** D2-3e Java + Ruby + PHP：per-env Maven local repo（方案 A）/ Bundler BUNDLE_PATH / Composer working-dir。9 单测。 |
| 2026-05-05 | **[feat]** D2-3f .NET（D2-3 收尾）：dotnet-install 脚本（unix sh / Windows ps1，不走 mise）+ per-env nuget packages。7 单测。**D2-3 整组完成**——4 Installer + 11 EnvManager 全进 v1，投资人承诺达成。 |
| 2026-05-05 | **[arch]** D2-4 part 1 — ToolRegistry 抽象解耦 EnvManager 与支持工具安装：5 EnvManager 从持具体 binary path 改持 ToolRegistry 懒解析（`EnsureTool`）。好处：接口纯净 + boot 快 + 测试可注 fake。 |
| 2026-05-05 | **[feat]** D2-4 part 2 — `app/sandbox` Service：Bootstrap（extract mise + atomic ready）+ EnsureRuntime/EnsureEnv（per-key 锁 + deps drift rebuild）+ Destroy + 查询。disk.go 辅助（求 size + removeAll 防灾难路径）。 |
| 2026-05-05 | **[feat]** D2-4 part 3a — 进程树 leak 防御（Job Object 从 D10 提前）：Linux PR_SET_PDEATHSIG / darwin Setpgid / Windows Job Object KILL_ON_JOB_CLOSE（v1 最强 leak 防御）。3 平台 cross 全过。 |
| 2026-05-05 | **[feat]** D2-4 part 3b — `infra/sandbox/spawn.go`：SpawnOnce（一次性，非零退出返 Ok=false 不上抛）+ SpawnLongLived（返 handle）；都套进程组 + ctx-cancel 杀。8 单测。app 层不直接碰 exec。 |
| 2026-05-05 | **[feat]** D2-4 part 3c — Service.Spawn/SpawnLongLived + 层 A leak 防御（active handle 注册表 + Shutdown 并发 Kill）。10 单测。macOS crash leak 概率从"每次"降到"仅 SIGKILL Forgify 时"。 |
| 2026-05-05 | **[feat]** D2-4 part 3d — 层 B leak 防御 + D2-4 收尾：Env 加 `running_pid` 列 + boot 扫 manifest 杀 stale PID。3 单测。Bootstrap/Service/Spawn + 三层 leak 防御（A/B/C）全到位。 |
| 2026-05-05 | **[refactor]** D2-5a forge sandbox 迁新 service：`SandboxAdapter` 委托 sandboxapp.Service（Owner ID `<forgeID>:<envID>` 保 N=3 buffer）；SyncRequest 等类型挪进 forge 包（forge.go 不再 import sandboxinfra）。main.go 装配重写。~170 单测全绿。 |
| 2026-05-05 | **[refactor]** D2-5b 清 v1 sandbox 残留：删 8 v1 文件 + 6 测试；macCodesign 提到 codesign.go 加非 darwin 早返。infra/sandbox 仅剩 v2。**D2-5 完成**——forge 完全切 v2，0 残留。 |
| 2026-05-05 | **[feat]** D2-6 sandbox HTTP：13 端点（runtimes/envs/disk-usage/bootstrap-status + destroy/gc/retry-bootstrap + conv-scoped envs）按 sandbox.md §11；Service 加 GetEnv/DeleteRuntime（有引用 409）/GC。12 httptest 全绿。 |
| 2026-05-05 | **[feat]** D2-7 Bash 自动路由 + 对话 scratch env（D2 收尾）：detectRuntime 按命令分类到 runtime kind + 派生 per-kind PATH 前置 + 懒建 scratch env。Sandbox 不可用优雅降级 plain shell。21 单测。**D2 整组完成**——PluginSandbox v2 完整落地。 |
| 2026-05-05 | **[refactor]** infra/sandbox 按 §S12 重组：22 个 by-kind 文件 → 12 个 by-concept（mise/playwright/dotnet/static + 8 语言单文件）。0 行为改动，全套测试 + 3 平台 cross 全绿。 |

#### Tool 自检 batch 1 — 3 个真 bug（2026-05-05）

| 日期 | 内容 |
|---|---|
| 2026-05-05 | **[fix]** Tool 自检 batch 1 修 3 bug：grep_stdlib 多行匹配误标 context；web/fetch 加 CheckRedirect 防 302 绕 SSRF；bash runForeground 加 ctx.Canceled 分支防误报。+7 回归测试。 |
| 2026-05-05 | **[doc-fix]** Tool 自检 batch 2 — chat.md 全量同步 tool drift：§4.4 工具表扩 20 个 + IsConcurrencySafe→execution_group + 旧事件类型→chat.message 单事件 + §15 实现清单重写为 7 家族子包。 |
| 2026-05-05 | **[doc-fix]** Tool 自检 batch 3 — CLAUDE.md 同步 tool 演化：§S15 删 frh_/fth_ + 加 fe_/b_/tk_/bsh_ 四前缀；§S18 加静态元数据 3 字段对照表（全 20 工具）。 |
| 2026-05-05 | **[fix/security]** Tool 自检 batch 4 — PathGuard 跨平台覆盖：DefaultDenyList 加 Linux/Windows 关键路径 + 浏览器 Login Data + kube/docker config。包注释明示 Bash 故意不走 PathGuard（护栏非安全边界）。+4 回归测试。 |
| 2026-05-05 | **[doc]** Tool 自检 batch 5A — 新建 5 份 tool 家族 design doc（filesystem/search/web/shell/ask）对照 task.md 模式；task.md §10 ask 详细设计压成指向 ask.md 指针。 |
| 2026-05-05 | **[cleanup]** Tool 自检 batch 6 — 5 条小 nit：补接口断言（全 20 工具一致）+ 删死代码 + 测试改名 + 注释 + error-codes 注脚（fs/search/web/shell 不进 errmap）。build/test/staticcheck 全绿。 |
| 2026-05-05 | **[doc/refactor]** §S11 注释规范 v1.1 + `internal/pkg/` 注释清理：加硬上限 + 内容禁令（设计意图/决策叙事/Phase 状态/横幅）。10 文件注释比例 21-79%→21-48%，行数 1046→788（-25%）。 |
| 2026-05-05 | **[fix]** Tool 自检 batch 7 — Bash auto-route 改 AST walk（`mvdan.cc/sh/v3`）：覆盖 `bash -c`/`env VAR=`/subshell/`$()`（之前静默逃逸）。drop stripCDPrefix。+15 测试。详 sandbox.md §9.5。 |
| 2026-05-05 | **[arch]** D3-0 docs only — subagent 设计大调：删 SubRunner port；新设 `app/loop/` 通用 ReAct 引擎（Host 接口 + Run），chat/subagent/Skill fork/workflow LLM 节点都是调用方（纯单向依赖，无循环 import）。 |
| 2026-05-05 | **[feat]** D3-1+D3-2 抽 `app/loop/` 通用 ReAct 引擎；chat 重构为调用方（runner.go 452→213 行）。loop.Host 5 方法；BlocksToAssistantLLM 导出供复用。chatapp 公开 API 0 变更，~170 单测全绿。 |
| 2026-05-05 | **[refactor]** 命名审计 #1 — Task → Todo 全栈改名（避与 Subagent 工具语义混）：包/entity/LLM 工具/DB 表 tasks→todos/ID `tk_`→`td_`/SSE/errmap 全改。build/test/staticcheck/3 平台 cross 全绿。同步 8 文档。 |
| 2026-05-05 | **[fix]** test/harness：D2-5 沙箱重构后 pipeline harness 编不过（被"全套绿"指单测掩盖）。改用 sandboxapp.Service v2 + registerSandboxStack（镜像 main.go）；RequireForgeResources 改查 IsReady。 |
| 2026-05-05 | **[fix]** Sandbox 支持工具版本钉死：uv@0.11.4 / pnpm@9.15.4 / maven@3.9.9（替隐式 latest）。bundler+composer 移出（不在 mise registry）——Ruby/PHP EnvManager 遗留 bug，单独跟踪不阻塞 demo。 |
| 2026-05-05 | **[fix]** Sandbox §S3 系统性扫错：抽 `RunWithStderrCapture`（9 处 install 调用 ~150 行样板→3 行）；mise 落 mise.toml 关 attestation（GitHub rate-limit 是 pipeline 反复 403 真因）+ Locate 识别 aqua flat-binary；test-pipeline 加 `-p 1` 串行。12 套 4 分钟全绿。 |
| 2026-05-05 | **[feat]** D3-3 subagent domain + store + reqctx：SubagentRun（16 持久 + 5 瞬时 gorm:"-"）+ SubagentMessage（复用 chatdomain.Block）+ AppendMessage 事务内 SELECT MAX(seq)+1 防撞号。14 集成测试（含并发 12 路）。72 包全绿。 |
| 2026-05-06 | **[feat]** D4-1 events.ChatMessage 扩 3 字段（SubagentRunID/ParentConversationID/*SubagentRun）+ MarshalJSON 双路径（零 subagent 走快路径 wire 向后兼容）。agentstate 加 SubagentTokenLog（32 路并发求和验证）。 |
| 2026-05-06 | **[feat]** D4-2 app/subagent service + registry + subagentHost：registry 内置 3 类型（Explore 只读 / Plan 含 web / general-purpose 继承父去 Subagent 自身）；Service.Spawn 一站式（解类型 → 过滤 tools → CreateRun → ctx 注 RunID+Depth+1 → 5min 超时 → loop.Run → detached UpdateRun）；subagentHost 满足 loop.Host 5 方法（Publish 推 chat.message 嵌快照）。 |
| 2026-05-06 | **[feat]** D4-3 SubagentTool 9 方法 + 装配：双保险防递归（filterTools 剥 + Execute 查 ctx depth）；终态友好注脚；hard sentinel 走 Go err。装配顺序保 filterTools 见全部其他 tool。18 单测。 |
| 2026-05-06 | **[feat]** D4-4 HTTP 4 端点：subagent-runs 列表/详情/messages + subagent-types。8 httptest。V1 不出 :cancel（父 ctx 自动级联）。 |
| 2026-05-06 | **[test]** D4-5 pipeline 3 场景：Spawn_EndToEnd / SSE_CarriesSnapshot / MaxTurns_Triggered。13 套 pipeline 全绿 ~4 分钟。V1 不写嵌套递归（结构性防御单测已覆盖）。 |
| 2026-05-06 | **[doc]** D4-6 文档收尾：subagent.md §13 + 4 契约表 📐→✅ + backend-design 树。**D4 整组完成**——LLM 能调 Subagent spawn 子 loop，独立 context，进展经 chat.message 嵌快照回流。 |
| 2026-05-06 | **[feat]** D5-1 domain/mcp + 10 sentinels：ServerConfig/ServerStatus（含 degraded 触发计数）/ToolDef/HealthResult + 5 status const + IsCallable。errmap 全 10 行接。8 单测。 |
| 2026-05-06 | **[feat]** D5-2 RegistryEntry + V1 marketplace 6 内置 + GOOS filter：playwright/markitdown/context7/duckduckgo/sqlite/everything(hidden)；Visible() drop Hidden + UnsupportedPlatforms。10 单测。 |
| 2026-05-06 | **[feat]** D5-3 ~/.forgify/mcp.json I/O：Load/Save/Merge（Claude Desktop 兼容 schema）；Save atomic+0600+排序+末换行；损坏 wrap ErrConfigCorrupt（Start 不 panic）。17 单测。 |
| 2026-05-06 | **[doc]** D5-4 文档收尾（部分）：mcp.md 📐→🔄 + error-codes mcp ×10 ✅。**D5 完成**——MCP 基础（types + marketplace + 配置 I/O），35 单测全绿。 |
| 2026-05-06 | **[feat]** D6-1 stdio Client wrapper：包 `go-sdk` v1.6 CommandTransport（自处理 SIGTERM→5s→SIGKILL）；Client 5 方法 + stderr→zap + 256KB ringBuffer + Content→domain 转换。 |
| 2026-05-06 | **[feat]** D6-2 Service runtime：Start/Stop/Add/Remove/Reconnect/Search/CallTool/HealthCheck/Install/Import，单 RWMutex 守三 map。recordCallResult §5.6 degraded 自愈；resolveCallTimeout §5.7 precedence；Search 用 forge.search 模式 A。 |
| 2026-05-06 | **[feat]** D6-3 search_mcp + call_mcp 系统工具：9 方法 + DI；sentinel → 对 LLM 可读字符串。关键：不 flat 注册 N×M tools（避 ~70k 启动开销）。 |
| 2026-05-06 | **[feat]** D6-4 HTTP 10 端点：servers CRUD + import + reconnect + health-check + registry list/get/install；`:action` 走 `{nameAction}` wildcard + splitAction。20 httptest。 |
| 2026-05-06 | **[feat]** D6-5 pipeline + fake stdio MCP server：~70 行 go-sdk fakeserver（echo/fail/crash）；4 离线场景（happy/failed/degraded 自愈）+ 1 Live_（双门控）。3.6s。 |
| 2026-05-06 | **[doc]** D6-6 文档收尾：mcp.md 🔄→✅ + events/api/backend-design 翻 ✅。**D6 整组完成**——MCP 端到端，88 单测全绿。 |
| 2026-05-06 | **[feat]** D7-1 domain/skill + 5 sentinels：Skill/Frontmatter（Anthropic SKILL.md spec 全字段，V1 消费 7）+ MaxBodyBytes 32KiB / MaxDescriptionChars 1536。errmap 全 5 行。6 单测。 |
| 2026-05-06 | **[feat]** D7-2 agentstate ActiveSkill + matchAllowedTool：`atomic.Pointer[Skill]` + 预授权检查；3 form（bare/Bash 任意/Bash wildcard），malformed fail-closed。10 单测（含并发 LastWriteWins）。 |
| 2026-05-06 | **[feat]** D7-3 app/skill Service：Scan/Get/List/Search/Activate。Scan per-skill 错误隔离；Activate body 不缓存（重读 + 100ms 重试覆盖编辑 race）+ \$N/\$ARGUMENTS 替换 + fork depth≥1 抑制嵌套。SubagentService 接口注入避循环。20 单测。 |
| 2026-05-06 | **[feat]** D7-4 fsnotify watcher：递归 watch + 500ms debounce + symlink 循环防护 + ENOSPC fail-soft + fsnotify 失败回 5min poll（健康时仍并行 backstop）。SSE 由 Scan 发（单一事实源）。6 单测。 |
| 2026-05-06 | **[feat]** D7-5 search_skills + activate_skill 工具 + DI：9 方法；activate_skill 写 ActiveSkill 旁路（与 state-mutating 同 execution_group 串行）；friendly-string 错误。12 单测。 |
| 2026-05-06 | **[feat]** D7-6 framework permission integration：`loop.tools.executeTool` 在 CheckPermissions 前加 IsToolPreApprovedBySkill 短路（集中 dispatch 层，新 tool 自动继承）。3 单测。 |
| 2026-05-06 | **[feat]** D7-7 HTTP 9 端点：Body/Create/Replace/Delete/Import + nameRegexp + atomic 写 + per-row ImportResult；9 端点（含 literal :import/:refresh）。V1 只接 SKILL.md 直传。20 httptest。 |
| 2026-05-06 | **[test]** D7-8 pipeline 3 场景：Activate inline（\$1 替换）/ Search-then-Activate / PreApproval Bash after Activate（D7-6 端到端）。<2.5s。94 单测 + 3 pipeline 全绿。 |
| 2026-05-06 | **[doc]** D7-9 文档收尾：skill.md 📐→✅ + events/api/error-codes/backend-design 翻 ✅。**D7 整组完成**——Skill 端到端（domain + agentstate + Service + watcher + 2 工具 + 9 HTTP + 3 pipeline）。 |
| 2026-05-06 | **[feat]** D8-1 domain/catalog + 2 sentinels：Catalog（7 字段）+ CatalogSource port（Name/Granularity/ListItems）+ Granularity 枚举（PerItem 默认）+ SystemPromptProvider port + 2 内部 sentinel。5 单测。 |
| 2026-05-06 | **[feat]** D8-2 app/catalog Service + pollLoop + cold-start cache：cache atomic.Pointer 零锁 + busy atomic.Bool 单 flight；Start 加载 disk cache；pollLoop 1s tick + fingerprint 短路（~99% tick）→ Generator nil/失败 → mechanicalFallback。18 单测。 |
| 2026-05-06 | **[feat]** D8-3 LLMGenerator + 3-attempt retry：每 attempt buildPrompt → llmGenerate → 10KB 上限 → extractJSON → coverage 校验；传输失败不内部重试冒泡。3 attempt 用尽 → mechanical。8 单测。 |
| 2026-05-06 | **[feat]** D8-4 3 CatalogSource + DI：forge/skill/mcp 各 AsCatalogSource()；Granularity forge/skill=PerItem、mcp=PerServer（跳非 ready server + 合成 per-server desc）。main.go RegisterSource + Start。 |
| 2026-05-06 | **[feat]** D8-5 chat runner SystemPromptProvider 集成：`buildSystemPrompt` 在 base 与 locale hint 间插 catalog summary（nil/空跳）。5 chat 单测。 |
| 2026-05-06 | **[feat]** D8-6 HTTP 2 端点：GET /catalog（缓存）+ POST /catalog:refresh（强制刷返结果）。4 httptest。 |
| 2026-05-06 | **[feat]** D8-7 pipeline 3 场景 + 关键 bug 修：AllSourcesCovered / DescriptionChange triggers regen / NoLLMKey→mechanical。**实施发现 bug**：catalog 后台 goroutine 无 HTTP ctx → 'missing user id'，修：Refresh 入口注 DefaultLocalUserID。 |
| 2026-05-06 | **[doc]** D8-8 文档收尾：catalog.md 📐→✅ + api/backend-design 翻 ✅。**D8 整组完成**——Capability Catalog 端到端，100 单测全绿。 |
| 2026-05-06 | **[feat]** D9 跨切集成 + 2 harness 修：`backend/test/integration/d9_test.go` (~250 行) 3 离线场景：(1) CatalogReachesLLM——seed forge + skill → drive chat turn → 断言 fake.LastSystemPrompt() 真含 catalog 块（关上 D8 单测留的 "GetForSystemPrompt 返对但有没有进 wire" gap）；(2) DynamicSkillUpdate——atomic rename SKILL.md 进 watched root（避 mkdir-then-write 竞态丢内部 Create 事件）→ 等 fsnotify debounce → trigger Catalog.Refresh → 断言新 skill 进 Summary + fp 变；(3) BootSmoke——投资人 demo 前 confidence check：每个 service handle 非 nil + 24 个 tool 家族都注册 + Catalog.Refresh 出非空 Summary + MCP.ListServers 返空 slice。**2 个 harness 修复**：FakeLLMServer 加 LastSystemPrompt() 捕 role:system 内容（让 pipeline 测试能验真到 LLM 看到啥）；harness skillWatcher 启动 + skills 根 mkdir 预建（之前 main.go 启 watcher 但 harness 没启，pipeline 无 fsnotify 路径） |
| 2026-05-06 | **[doc]** D9-4 文档收尾：backend-design 路线图加 "Phase 4 准备件 (D2-D9) ✅"。**D9 完成**——sandbox v2 + subagent + mcp + skill + catalog 端到端集成验证，100 单测 + 3 pipeline 全绿。 |
| 2026-05-06 | **[fix]** Pipeline 全套发现 2 个 D8 回归并修：catalog polling 抢 FakeLLM 脚本队列（harness 走 mechanical）+ catalog goroutine 与 TempDir cleanup 竞态（加 Service.Stop drain）。仅改 harness。17 套全绿 8 分钟。 |
| 2026-05-06 | **[feat]** D10+D11 Windows 代码层硬化：bash.go 把 Windows 升一等公民（cmd.exe /c）；8 个 bash_test skip 消息改"pending real Windows test env"。MCP 6 entries 全审 Windows 兼容（无 UnsupportedPlatforms 需填）。 |
| 2026-05-06 | **[verify]** D12 mise Windows binary embed 就绪：`embed_mise_windows_amd64.go`（windows&&amd64 build tag + go:embed mise.exe 92MB）+ cmd/resources 知 windows 用 .zip。GOOS=windows build 干净。 |
| 2026-05-06 | **[doc]** D13 Windows 平台支持文档：desktop-packaging-notes 加第十二节（10 模块 Windows 状态表 + 家目录 layout + wails build 打包命令 + 5 项待真机验清单）。 |
| 2026-05-06 | **[feat]** D14 `make check-cross` Makefile target:5 平台（darwin/linux/windows × amd64/arm64）vet + build 一键跑。CGO_ENABLED=0 给非 host 平台（cgo 会让 Mac SDK linker 错对 Linux/Windows 头文件——modernc.org/sqlite 纯 Go 选型让此行得通）。本地通过 ✓ |
| 2026-05-06 | **[doc]** D15 Phase 4 准备件 + Windows 代码层全交付收尾:backend-design 路线图加 "Windows 代码层适配 (D10-D15) ✅ 2026-05-06" 行(注明真 Windows runtime 验证待物理机)；progress-record 6 条 dev log。**整组完成**——5 平台代码层 vet/build 干净,Wails Windows 打包文档齐,真机验证清单留好。下一步 Phase 4 工作流真启动 |
| 2026-05-07 | **[fix]** TE-15 forge run 路径 3 项打包修：拆 `ErrNoActiveVersion` sentinel（原 ErrEnvNotReady 文案误导 LLM）；抽 `ensureRunnable` 统一 Run/RunTestCase 双检查；首次创建 post-sync ready 时 auto-accept（edit 流仍手动 review）。100 单测全绿。 |
| 2026-05-07 | **[perf]** TE-16 长流式不打死浏览器：后端 publish 60fps 节流；前端 SSE rAF batch + 同 msg 覆盖；reasoning/tool step 默认折叠（x-if 不建 DOM）。~50MB/对话→~15MB，render 6940→~1800 次。仍是 entity-snapshot 模型，delta 化留后。100 单测全绿。 |
| 2026-05-07 | **[refactor]** 屎山拯救 #1：skill watcher fsnotify → 1s 轮询（复用 catalog/polling 模子 + sha256 fingerprint 短路 SSE）。净 ~-200 行 + 删 4 类边界条件（symlink loop / fd 上限 / debounce / 兜底）。全绿跨平台。 |
| 2026-05-07 | **[fix]** TE-22 reasoning-only assistant 消息致 DeepSeek 400 锁死对话：`BlocksToAssistantLLM` 末尾兜底——仅 reasoning 无 content/tool_calls 时拷 reasoning→content。3 测试。 |
| 2026-05-07 | **[refactor]** TE-23 引入 `infra/llm/Adapter` 集中 provider wire 适配（11 provider + lookupAdapter，DefaultBaseURL 替散落 switch）。TE-22 fallback 归位到 openai.go（不污染 Anthropic）。修 content 空也 emit + OpenRouter mid-stream error 检测 + Gemini base URL。~10 测试。100 单测全绿。 |
| 2026-05-07 | **[fix]** TE-24 LLM provider 兼容 audit 收尾 + 立 §S20「禁止留问题无理由」。真 bug 全修：Ollama tool_calls.index 兜底 + Ollama streaming+tools 静默吞 tool_calls（加 DisableStream 走非流式）。2 项「0% 触发」从风险表删。4 测试。 |
| 2026-05-07 | **[fix]** TE-25 二轮 audit「永久死锁对话」类：新建 `sanitizer.go::SanitizeMessages` 强制 tool_calls↔tool message 配对不变量（缺则合成 stub，游离丢）——防 ESC/崩溃/脏数据致 400 锁死。DeepSeek turn-type reasoning round-trip。Anthropic 5MB 图片守门按 §S20 说明留下次。9 测试。 |
| 2026-05-07 | **[refactor]** 屎山拯救 #3：subagent host 三方法去复制粘贴——抽 `ensureStreamingRow` + `logPersistErr` 两 helper，建/refine 逻辑只剩一处。Host 接口不动。全绿跨平台。 |
| 2026-05-07 | **[refactor]** 屎山拯救 #4：WebSearch SearXNG 池 + Bing 国际抓 → BYOK + MCP + Bing CN 三层。BYOK 4 provider（brave/serper/tavily/bocha）入 apikey 域 + CategorySearch；MCPSearchRouter 端口反转避 web↔mcp 互引。净 ~-200 行 + 保护国内用户（duckduckgo/bing 国际被墙）。11 测试。 |
| 2026-05-07 | **[feat]** 屎山拯救 #4 收尾：testend 接 GET /providers（ProviderMeta 注册表，category=llm/search 过滤）；前端删硬编码 12 项改 fetch。加 provider 只改后端。4 handler 测试。 |
| 2026-05-07 | **[fix]** 屎山拯救 #4 二次收尾：删 Bing CN HTML 抓取这层「假兜底」——dogfood 实测 cn.bing.com 301 到主页、全 JS 渲染 0 命中。WebSearch 收成 BYOK→MCP 两层 + LLM-actionable 失败提示。新增 list_mcp_marketplace + install_mcp_server 工具引导装 duckduckgo。100 单测全绿。 |
| 2026-05-08 | **[feat]** Sandbox 加 Docker runtime（marketplace V2 准备件）：DockerInstaller 探活 daemon + DockerEnvManager + BuildDockerRunArgs。Forgify 不替用户装 Docker（探活失败返 2 sentinel + 引导链接）。默认安全：仅 envPath bind 挂载、image 永不自动删。17 单测。 |
| 2026-05-08 | **[refactor]** Marketplace V2：删 6 hardcoded → 接官方 Registry。加 RegistrySource 端口 + `official_registry.go`（HTTP fetch registry.modelcontextprotocol.io cursor 分页 + packages[] 适配）。rename search_mcp→search_mcp_tools、call_mcp→call_mcp_tool；新增 search_mcp_marketplace + install_mcp_server（两阶段 needs_confirmation 契约）+ uninstall。5 新 sentinel。6 测试。 |
| 2026-05-08 | **[doc]** 起草 `event-log-protocol.md`——SSE 龙骨级重构设计。entity-snapshot 6 事件 + 节流 + subagent 借壳被定为屎山根因；新协议参考 Anthropic + Vercel AI SDK 统一 5 事件 × 6 block + parentId 递归嵌套。flag-day 切换。完整方案见该文件。 |
| 2026-05-08 | **[feat]** 事件日志协议 Phase 1 基础设施：新建 domain/infra/pkg/eventlog 4 包 + reqctx parentBlockId。Bridge per-conv 单调 seq + replay buffer 4096 + 慢订阅者阻塞 publisher（delta 不能丢）+ Last-Event-ID 重连 + 410。BlockV2 7 字段 + message_blocks_v2 表 + /api/v1/eventlog SSE（与 legacy 共存）。38 测。producer 未切。 |
| 2026-05-08 | **[feat]** 事件日志协议 Phase 2A producer dual-write：chat 主管线与 legacy 同时推新 eventlog bridge 5 类事件。emitUserMessage burst + streamLLM 跟踪 text/reason/tool block + runOneTool 把 tool_result 挂 tool_call 父（共享 LLM tc_id）。未做：subagent/LLM-inside-tool emit、DB dual-write。全绿跨平台。 |
| 2026-05-08 | **[feat]** 事件日志协议 Phase 2B subagent 递归 emit + DB dual-write：Spawn 铸 message-block 占位 + subCtx 清 ParentBlockID 让 sub 顶层挂 subMsgID；Emitter 接 BlockV2Repository 落表（StartBlock/AppendDelta SQL content||?/FinalizeStop）。6 测。未做：两表 backfill+drop。 |
| 2026-05-08 | **[doc + feat]** 事件日志协议 Phase 3 — 规范同步 + 装包进度 + 历史回放：CLAUDE.md §E1/§E2/§D3 重写 + 新 §N7（wire format/重连/410）+ §S21（事件流 invariants）；events-design 整篇重写。install progress 推流（MCP 装包黑屏痛点）。ReplayEventsAfter + history refetch 端点。8 测。 |
| 2026-05-08 | **[doc + test]** 事件日志协议 Phase 5 文档收尾 + 协议契约测试：database/api/chat/subagent.md 同步。TestProtocolContract_ChatRoundtrip 模拟 12 envelopes + 校验 5 项 §S21 invariants。Phase 4 前端重写不在本轮（V1.2 后端期不动前端）。全绿跨平台。 |
| 2026-05-08 | **[refactor + feat]** 事件日志协议 final cleanup（~3000 行净增）：删所有 dual-write，总线只剩 eventlog + notifications 两条 SSE。新建 notifications 协议。Block 模型重整（BlockV2→Block，table 回 message_blocks，写入唯一路径=emit）。subagent 删两表整包 + 4 端点，sub-run = 统一 messages 行（attrs.kind=subagent_run）。loop.Host 3 方法→1。删 domain/events 整包 + EventsBridge 全栈。conv/todo 切 notifications。testend 部分适配。全绿跨平台。 |
| 2026-05-08 | **[refactor]** 事件日志 cleanup 收尾：subagent.go 401 行拆 3 文件（subagent/spawn/queries）；chat.js 完整重写（644→580 行）——删 entity-snapshot rAF batch 一坨，新 _connectEventLog 订 5 事件实时 mutate + 6 block renderer + _blockIndex 快查。全绿跨平台。 |
| 2026-05-08 | **[fix + feat]** 事件日志/通知协议接入轮：skill/mcp Search 内部 LLM rerank 加 progress block；conversation/mcp/skill/catalog 状态变化推 notifications。pipeline harness 完整重写适配新协议（订两端点 + 合成 envelope 让老测试零改）。doc 全同步。全绿 + pipeline 编译跑。 |
| 2026-05-08 | **[refactor]** 屎山拯救 #7：catalog generator 删 3-attempt retry → 单次。现代 LLM「读列表写总结」首试 ~99% 成功，1s 轮询本身是天然重试，mechanical 兜底。净 -159 行 + 省 token + 响应快。3 测试。 |
| 2026-05-09 | **[audit + fix]** Phase A1.1 calibration shot — `app/apikey` §S3/§S9/§S15/§S16/§S17 全审（fork 必读 `_spec-extracts.md` + 边读边写 trace 避 watchdog）。46 sites / 1 HIGH（MarkInvalid 漏 detached ctx）/ 1 MED / 8 LOW。修复 + 状态字段更新。 |
| 2026-05-09 | **[fix + refactor]** §S3/§S9 + dev 数据隔离三联：(a) apikey test 终态写用 detached ctx（hard refresh ctx cancel 致结果丢失、陈旧 ok 假象）；(b) errmap 映射 context.Canceled→499、DeadlineExceeded→504；(c) 加 `--forgify-home` flag，dev 默认根到 `<data-dir>/.forgify`。全绿。 |
| 2026-05-09 | **[fix]** testend 进入后所有按钮静默卡死：HTTP/1.1 单 origin 6 connection 上限被 6 个 SSE 占满。删废弃 tab-sse.js + 加 notifBus/logBus 单连接共享 EventSource fan-out。6→3 SSE。 |
| 2026-05-09 | **[fix]** Ctrl+C 后多溢 ERROR `shutdown context deadline exceeded`：SSE handler 在 r.Context().Done() select，但 Shutdown 不主动 cancel request ctx 要等 5s。加 `srv.BaseContext` + 先 cancelBase 解 SSE 再 Shutdown，秒退。 |
| 2026-05-09 | **[refactor]** Marketplace V3 search → list 化：21 条 curated 关键词 AND-match 召回过低致 LLM 误判「无可用」。RegistrySource.Search→List（无 query 全列表 tier+name 排序）；tool search_mcp_marketplace→list_mcp_marketplace（删 rerank）；REST 去 query。删 ErrQueryRequired。3 测试。 |
| 2026-05-09 | **[fix]** install 进度可见性 + MCP stderr 降级：tester.html 补渲 item.progress；installprogress 发合成 [starting]/[done]/[error] delta（mise MISE_QUIET 下零 stderr 也不零 delta）；drainStderr 默认 INFO，仅自报 WARNING/ERROR 才升 WARN。全绿。 |
| 2026-05-08 | **[refactor]** Marketplace V3 — curated 21 + 砍 sandbox 7 EnvManager + schema 简化（~净删数千行）：`curated_registry.go` hardcoded 21（全 npm/pypi 装机即用），删 official/fake source。RegistryEntry 砍 8 字段 + 加 Category/Tier/Notes。install 签名去 alias。sandbox 删 10 文件（rust/java/ruby/php/dotnet/playwright/docker/...），EnvManager 11→2（python/node）——curated 全 npm/pypi 不需其他 runtime。docs 全同步。7 测试。 |
| 2026-05-08 | **[fix]** chat user-message render race + emit/notification 路径接通 + pipeline 修复 (`fa9b8c4`)。乐观 user 行打真 messageId 后 SSE message_start 找到原位更新；嵌入点 emit/notif 接通；pipeline 测同步 Block shape。chat hot path 真 bug。 |
| 2026-05-08 | **[refactor]** marketplace V3 雏形 (`3c50b8c`) — search-only 改 + 修官方 Registry v0.1 schema 真实形状。删 LLM rerank 走纯 substring；为 V3 curated 21 落地铺路（与 line 408 同步）。 |
| 2026-05-08 | **[feat]** AskUserQuestion testend 交互 UI (`b22417d`) — 工具发问时前端弹自由输入框替代静默卡顿；描述强调用户自由输入而非选项。chat 使用 §S18 ask 工具体验补全。 |
| 2026-05-08 | **[test]** mcp curated marketplace pipeline (`a75dde5`) — 21 smoke + 5 T0 live tool calls 真装机 + 真 ToolCall 验证。`make test-pipeline` 大覆盖批次。 |
| 2026-05-08 | **[test]** mcp AllSmoke 真验装机路径 (`8a9b853`) — stub 凭证 + 严守测试作者 bug；§T6 Live_ 前缀 + ENV gate 实践。 |
| 2026-05-08 | **[chore]** mcp curated entry: gmail → google-workspace (`49792c5`) — 替 taylorwilsdon 全套 + 真维护 fork（V3 curated 21 演化）。 |
| 2026-05-09 | **[refactor + feat]** testend M1-M14 全 tab 适配事件日志协议 + 错误反馈 + 死代码清（13 commit）：M3.5 chat 渲染 subagent 嵌套 + progress 独立 block / M4 notifs tab 重做统一 SSE + toast / M2 SSE tab 重写 raw 双通道 viewer / M1 删 subagent 死 tab / 其余 polish。 |
| 2026-05-09 | **[fix]** sandbox conv env owner.ID 用 `_` 替 `:` 解 PATH 冲突 (`3cdf18a`) — 真 PATH 拼接 bug，与 ff8fd77 owner.ID 修复同源。 |
| 2026-05-09 | **[fix]** sandbox+bash 进度共享 helper + 错误 surface + 新加 type=`sandbox_env` notification (`888739c`) — 装包黑屏期 LLM/UI 可见；3 处修复同 commit。 |
| 2026-05-09 | **[fix]** sandbox reset-all conv envs 路由注册漏 `:reset-all` 后缀 (`9789b19`) — 单端点 path 漏注册修。 |
| 2026-05-10 | **[fix]** chat §S9 detached ctx + §S3 错误吞 + §S16 wrap (`f272503`) — chat hot path 真 bug 批修；app-chat audit 13 FIXED + 2 LOW review (`054e242`)。 |
| 2026-05-10 | **[fix]** mcp+loop §S3 + §S16 (`26f9c55` + `505d6e3`) — 错误吞 + wrap-format polish；app-mcp + app-loop audit batch 1 closeout (`4f147b9`)；chat host.go #8 收尾 (`e5b65fa`)。 |
| 2026-05-10 | **[fix]** sandbox §S9 ready/failed detach + §S17 sentinel gaps (`e36f890`) + 6 §S16 wrap LOW + empty-Cmd → sentinel (`0d4a48e`) — 装包终态写入加 detached ctx；audit batch closeout。 |
| 2026-05-10 | **[fix]** llm + errmap 引入 HTTP-status sentinel 家族 (`94ab56a`) — provider error 按 4xx/5xx 分类 + §S17 errmap 单一事实源登记。新 sentinel 簇。 |
| 2026-05-10 | **[fix]** forge owner.ID 分隔符 `:` → `_` 对齐 B1 + 新 sentinel `ErrInvalidOwnerID` (`ff8fd77`) — sandbox owner.ID 解析冲突 CRITICAL bug；app-forge audit drop (`da425c9`)。 |
| 2026-05-10 | **[fix]** sandbox 4 MED %w:%v sentinel-chain truncation (`d6b626f`) — install 路径 §S16 wrap 修；errors.Is 现可贯穿。 |
| 2026-05-10 | **[fix]** infra-llm 17 LOW sentinel + prefix sweep (`363b084`) + infra-sandbox EDGE §S16 prefix + scanner.Err (`d2b8af8`) — 大批 §S16 风格统一。 |
| 2026-05-10 | **[fix]** tool/search + tool/shell 1 HIGH + 8 LOW (`f9d0265`) — audit 找到的真 bug + style 修；tool/web 2 MED sentinel-based MarkInvalid + non-silent BYOK fail (`7dba737`)。 |
| 2026-05-10 | **[fix]** tool/forge 2 §S3/§S17 LOW + skill scan.go 2 MED %w:%v sentinel-chain truncation (`64d9535` + `7f3ef2c`) — 真 sentinel-chain 修复 + audit closeout。 |
| 2026-05-10 | **[fix]** subagent §S9 emit-side detach + mapEventLogStatus drift (`a70d73a`) — emit 路径加 detached ctx + status 映射对齐 eventlog domain。真 §S9 修。 |
| 2026-05-10 | **[fix]** catalog 新 sentinel `ErrAllSourcesFailed` + errmap 行 (`2d47cb0`) — §S17 三处联动；refresh 全失败时清晰错误码。 |
| 2026-05-10 | **[doc]** forge_redesign trinity architecture spec — Function/Handler/Workflow 三位一体 (`f98c152`) + plan 01 Function domain (`41d9212`) + plans 02-06 完整路线 (`2a0a1a0`)。Phase 4/5 重定向到 forge 重设计。完整方案见 `documents/version-1.2/adhoc-topic-documents/forge_redesign/`。 |
| 2026-05-10 | **[fix]** handlers-B1-B4 audit 全套 closeout (`905d141` / `8d7f797` / `a183b16` / `3f89c03`) — 4 batch 共 4 MED + 24 LOW；handler 层 §S16 wrap + §S3 错误处理 + 死代码清。 |
| 2026-05-10 | **[fix]** pkg/eventlog 1 MED + 3 LOW (`87b9fe7`) + pkg/llmclient 4 LOW %w:%v (`a13c21d`) + installprogress §S9 ctx-asymmetry (`9cb09b2`) — pkg 层 audit 滚动。 |
| 2026-05-10 | **[doc]** audit 滚动批量 drop — app-{tool-{filesystem,mcp,ask,subagent,todo,skill},todo,model,conversation} clean (8 traces 0 violations) + cmd 2 LOW (`872a265`) + transport/{middleware,response,router} 4 LOW + pkg/{idgen,llmparse,pagination} 4 LOW + errmap subagent 注释 stale 修 (`54ab931`)。LOW 集合不逐条记，整批 closeout。 |
| 2026-05-10 | **[doc-fix]** D contract-doc audit closeout — 7 HIGH + 11 MED gap close (`5186a95`)。§S14 文档同步纪律 (#7) 执行：service-design / service-contract / progress-record 三处对齐代码现状。 |
| 2026-05-11 | **[fix]** Phase G dead-A 真 bug — loop.Result.Status 终态推算（subagent error-classification 不再误报 completed）+ emitFatalError 加 convID stamp（stub message_stop emit 不再静默 skip）+ ErrBuildClient 加 chat/runner switch case + notifications.Publisher.Publish variadic→必填 5 参数。 |
| 2026-05-11 | **[refactor]** Phase G dead-A 死代码清 — 删 pkg/eventlog 4 死 export（StartMessage/StartBlockUnder/MustFrom/WithParent）+ pkg/notifications 整套 ctx-wiring（With/From/MustFrom/publisherKey）+ Bridge.log/bufferedEnvelope.at 死字段 + tools.go elapsedMs 死计算 + assembleBlocks/host.go 死 Status/CreatedAt 赋值。 |
| 2026-05-11 | **[doc-fix]** Phase G dead-A stale godoc — chat.go file-listing / runner msgID 注释 / stream.go file-header + dual-write godoc / loop.go Skill fork / broadcast.go events/memory 引用 / saveBlockRow seq==0 / eventlog New "legacy callers" / installprogress `_ = ctx`。Build + scope 内 unit tests 全绿。 |
| 2026-05-11 | **[refactor]** Phase G fix-C 死代码清 — mcp dead-4: snapshotLocked / ServerStatus.PID / ErrHandshakeFailed / ErrMarketplaceUnavailable / RegistryEntry.Version 5 死全删 + recordCallResult ctx arg 删（不发 SSE）+ stale "search_mcp_marketplace" 文案改 list_mcp_marketplace（5 处）+ var _ MergeResult keep-alive 删；apikey dead-7: ErrTestFailed / ErrInvalid 永不返 sentinel + APIFormatOpenAICompatible 装饰 const 删 + MaskKey/IsValidProvider 大写 → 小写（包内）；ask: ErrAlreadyAnswered ghost 删；todo: ErrConversationMismatch 永不返 删；errors: ErrInternal ghost 删；catalog: PerCollection future-reserved enum 删（同步 generator prompt）；shell: bash detector 6 语言 trim 剩 python+node（用户拍板：rust/go/ruby/php/java/dotnet 落系统 shell）+ envBinDirsForKind 同步 + Bash description 重写。同步 errmap.go 删 7 行；error-codes.md / apikey.md / ask.md / mcp.md / todo.md / sandbox.md / shell.md 7 文档对齐。 |
| 2026-05-11 | **[refactor]** Phase G fix-B 死代码清 — subagent dead-2: queries.go (Cancel + activeRuns map) + DefaultModel/Description/Roles* domain const 全删 + SpawnOpts.Model ghost 删；sandbox dead-3: Extras 全链路（domain/app/store/2 EnvManager InstallExtras）+ IsDefault flag + FindDefaultRuntime + idx_sr_kind_def + RunningStartedAt + ListAvailable interface method 全删 + macCodesign WalkDir 简化为单文件 codesign + ExtractMiseBinary sig 改 sandboxRoot 直传 + SpawnLongLived envID 走 prepareSpawn 不重查 + mcp install Owner.Kind 用 OwnerKindMCP const + Owner.ID 文档 `:` → `_`；agentstate dead-8 M1/M2: SubagentTokenLog write-only API（slice + mutex + 3 export）+ reqctx WithSubagentRunID/GetSubagentRunID 死 ctx key 全删。同步 subagent.md / sandbox.md / database-design.md 三处文档；测试同步 mkRuntime 改 3 参 + spawn_test fakeEnvManager 删 InstallExtras + mise_test sandboxRoot 改名。 |
| 2026-05-11 | **[refactor]** Phase C tool-result audit closeout — 14 tool 文件 + 5 测试 LLM-facing 文本清理：framework `sanitizeToolErr` 剥 §S16 wrap 链（5 单测）+ Bash `formatAutoRouteError` 重写并删 `description` 重复字段 + 13 tool description 瘦身（AP1/AP3 教学口吻、AP4 路径/UI/sentinel 名泄漏、AP5 "Successfully" 冗余、AP6 KillShell `shell_id`→`bash_id`）。同步 shell/filesystem/ask/mcp 4 处设计文档。build + 170 单测全绿。 |
| 2026-05-11 | **[doc-fix]** D-redo contract docs 同步 41 gaps（read-it-all 重审 vs D1 grep）—— 4 真 bug grep 漏：`:duplicate` ghost route 删 doc + manifest / notifications wire `event: notification` 硬码 vs doc `<type>` 占位 / SEQ_TOO_OLD wire code 补 error-codes 索引 / 4 ghost sentinel 删（chatdomain.ErrProviderUnavailable+ErrVisionNotSupported + mcpdomain.ErrRuntimeMissing+ErrUnsupportedRuntime）+ web BYOK 3 sentinel errmap 删（控制流 sentinel）+ §11.2 producer 表方法名 / 触发场景全修（fsnotify→polling 等 6 处错）+ messages.parent_block_id/attrs 字段补 + sandbox_envs 假 FK 措辞改 + forge_executions retention TODO→已实现 + sandbox nil-Publisher 改构造器兜底（其他 5 service pattern 对齐）+ alias 全局 sed `domain` 后缀对齐 §S13 + buffer 数字明示 + History endpoint response shape 加 + CLAUDE.md §D4 改 aspirational 措辞。0 HIGH ship-blocker；build + tests 全绿。 |
| 2026-05-12 | **[feat]** forge_redesign Plan 01 完整交付(13 commits 直推 main)—— function trinity domain 替代 forge:(1) domain+store 24 单测;(2) app 层 apply.go 6-op 引擎 + crud/pending/accept/revert + sandbox_adapter + execution log(D22) 8 单测;(3) 9 LLM tools(search/get/create/edit/revert/delete/run + search_function_executions/get_function_execution);(4) 11 HTTP endpoints + errmap 14 sentinel + router/main/harness wire-up;(5) Phase 7 destructive 删除 forge ~2500 LOC + 4 个包目录(app/forge / app/tool/forge / domain/forge / infra/store/forge)+ handlers/forge.go + test/forge/ + OwnerKindForge;(6) D22 function_executions 表 + 通用 16 字段 + 函数专属 3 字段 + per-entity LLM 诊断工具;(7) 4 pipeline tests(CRUD lifecycle / 分页 / LLM search empty / Run+exec log sandbox-gated)。Phase 9 同步 backend-design + 4 contract docs + 新 function.md + 删 forge.md。设计 spec [`adhoc-topic-documents/forge_redesign/`](./adhoc-topic-documents/forge_redesign/)。 |
| 2026-05-12 | **[feat]** forge_redesign Plan 02 完整交付(11 commits 直推 main)—— handler trinity 第二条腿(stateful Python class + caller-owns instance lifetime):(1) 18 sentinel + 3 表(handlers/handler_versions/handler_calls);(2) infra/handler/client.go stdio JSON-line RPC client(5 methods + 8 wire-format 单测,fresh-write per D5 — MCP 用 JSON-RPC 2.0 协议不同);(3) app 层:apply.go 10-op method-level + RFC 7396 JSON Merge Patch / config.go AES-GCM Load+Update+Clear+MaskedConfig+ConfigState / rpc.go AssembleClass+DriverScript / registry.go Owner+Instance+caller-owns lifetime(chat=per-call,workflow/test/session=persistent;无 idle GC,scope-end 钩子调 DestroyOwner)/ call.go Service.Call+recordCall(D22)+stderr 256KB ring / 33 单测;(4) 10 LLM tools(search/get/create/edit/revert/delete + call_handler + update_handler_config + search_handler_calls/get_handler_call);(5) 16 HTTP endpoints + errmap 19 sentinel + router/main/harness wire-up + graceful shutdown 钩子;(6) D22 handler_calls 表(通用 16 + handler 专属 6 字段:handler_id/version_id/method/instance_id/owner_kind/owner_id);(7) 4 pipeline tests(HTTP CRUD / Config round-trip / LLM search empty / Call+CallLog sandbox-gated)。**用户细化决策**(forge_redesign 04-discussion 2026-05-12):lifetime 完全 caller-driven 无 idle GC;function 同步移除 per-call timeout knob(只走 ctx cancel);Handler→Handler 不强制禁止(结构上自然防);stdio JSON-line client 独立写不抽公共包(协议跟 MCP 不一样)。Phase 9 同步 4 contract docs + 新 handler.md + backend-design Architecture 树。设计 spec [`03-handler.md`](./adhoc-topic-documents/forge_redesign/03-handler.md) + [`plans/02-handler-domain.md`](./adhoc-topic-documents/forge_redesign/plans/02-handler-domain.md)。 |
| 2026-05-12 | **[doc]** Plan 03 redesign 落档(b0af578/89b0d4c/25db693)— 讨论 §A-I + Doc A env 模型 9 files + Doc B SSE 三流 6 files;CLAUDE.md §E1 双→三协议。完整方案 [`discussions/2026-05-12-env-and-sse-rework.md`](./adhoc-topic-documents/forge_redesign/discussions/2026-05-12-env-and-sse-rework.md)。 |
| 2026-05-12 | **[feat]** Plan 03 C1+C1.1 env 模型 backend(9f0cb4b/2971161,32 files)— EnvID 每版本独立生成(`fnenv_`/`hdenv_`,与 versionID 解耦);Create/Edit 同步装 env;Accept 纯指针、Reject 销 env 删行、Edit iterate-same-pending;Service 前置 sandbox ping;删 ErrPendingConflict + Resync 路径 + `:resync` HTTP 端点。 |
| 2026-05-12 | **[feat]** Plan 03 C2 LLM env-fix loop(5cc57f9)— 新 `pkg/envfix` + 11 单测;4 LLM tool(create/edit × function/handler)装失败调主 chat scenario LLM 修 deps 重试最多 3 次,attempt 进 chat progress block。 |
| 2026-05-12 | **[feat]** Plan 03 C3 SSE 三流 user_id 化 + 通知瘦身(5067c16,13 files)— eventlog + notifications Bridge 改 per-user key(从 ctx 抽);eventlog HTTP 去 `?conversationId=`(client demux);删 env_synced/env_failed 通知;notification payload 瘦身(只 `{action, versionId?}`,UI 经 GET 拉)。 |
| 2026-05-12 | **[feat]** Plan 03 C4 forge stream(031954c,17 files)— 新 SSE 第三流 `/api/v1/forge`(4 events × 3 kinds function/handler/workflow);domain+infra+pkg+transport 全套 + 11 单测;4 LLM tool 双写 forge bus + 保留 chat eventlog progress block。复用 `eventlogdomain.Scope` 嵌套(D-redo-23)。 |
| 2026-05-12 | **[feat]** Plan 03 C5 testend 三 bus listener(2911777,4 files)— app.js 加 chatBus + forgeBus 共享 EventSource;chat.js 改订 chatBus + 客户端 demux `conversationId`;新 `tab-forge.js` 显示 4 类 forge 事件 + kind/type 过滤。Plan 03 6-commit 切分完工。 |
| 2026-05-12 | **[feat]** forge_redesign Plan 04 完整交付(9 commits W1-W9 直推 main)—— workflow trinity 第三条腿(DAG authoring,锻造 vs 执行分离 D6):(1) W1 domain 11 sentinel + Workflow/Version/Graph + 13 NodeType + 9 Op + 6 单测;(2) W2 store 13 Repository 方法 + partial UNIQUE + 12 集成测试;(3) W3 apply.go(450L 9-op engine + RFC 7396 JSON Merge Patch + 12 单测)+ validate.go(380L Kahn cycle + CapabilityChecker + container body 递归 ≤3 + 12 单测);(4) W4 expression.go(Go text/template ~140L + 11 单测);(5) W5 Service(CRUD + iterate-same-pending + 自动 accept v1 + slim notifications D-redo-6 + WorkflowReader 接口预留 Plan 05);(6) W6 6 LLM tools(search/get/create/edit/revert/delete + forge 双写;**无 envfix loop** — workflow 无 env);(7) W7 11 HTTP endpoints + errmap 11 sentinel + 13 httptest;(8) W8 ProductionChecker 装 function/handler/skill/mcp + main/harness 装配 + D9 BootSmoke 加 handler+workflow tool 家族 + 3 pipeline test(CRUDLifecycle / VersionsAndPending 真 CapabilityChecker / LLM SearchEmpty);(9) W9 docs sync 4 contract docs + 新 workflow.md + Plan 04 完工 banner。drive-by 修 filesystem pipeline test 错误断言。**故意不含**:ErrPendingConflict(iterate-same-pending D-redo-11)。Plan 05 territory:trigger / flowrun / scheduler / `:trigger` action / execution log 端点。设计 spec [`adhoc-topic-documents/forge_redesign/04-workflow.md`](./adhoc-topic-documents/forge_redesign/04-workflow.md) + [`plans/04-workflow-authoring.md`](./adhoc-topic-documents/forge_redesign/plans/04-workflow-authoring.md)。 |
| 2026-05-13 | **[feat]** forge_redesign Plan 06 完整交付(5 commits F1-F5 直推 main)—— trinity 收尾 + multi-agent forging 教学:(F1) `app/subagent/Service.filterTools` 加 closed deny-list(Subagent + create/edit/delete/revert/trigger_workflow,D21);保留 search/get_workflow read-only + call_handler/run_function 子 agent 自测必需;3 单测;(F2) 主 chat LLM 的 system prompt 永远拼 multi-agent forging 教学段(`chat/multi_agent_prompt.go`,40 行 markdown:6 step 流程教 explore decomposer / parallel forger spawn / configState 闸 / workflow 装配主 agent 独享 D21 / 简单需求别 spawn);放 `chat/runner.buildSystemPrompt` 不放 catalog generator(generator 是 meta-LLM 生成 catalog summary 的 prompt,跟主 agent system prompt 独立);3 子用例测试(nil/empty/populated catalog 都拼);(F3) test/catalog/trinity_catalog_test.go 2 pipeline test 验 function + handler 都在 catalog.Coverage + Summary 含 name + configState 暴露;(F4) test/scheduler/approval_e2e_test.go 3 HTTP E2E(approve happy path 跑 ack 节点 / decision=maybe 返 400 / wrongNodeID 返 404);RehydrateOnBoot 由 E10 单测覆盖,本测专注用户 UX HTTP 面;(F5) forge_redesign/README.md(新,trinity 完工导航 + Plan 01-06 + 关键数字 + 完工能力清单)+ plans/06 完工 banner + 本 dev log + backend-design Phase 4 行同步。**Scope 调整**:原 plan Task 5 邮件 workflow 全栈 E2E (~25 turn fake LLM script) 跳过 — 长期维护成本太高,效益不如直接 dogfood;multi-agent mechanics 已由 subagent 单测 + filterTools test + system prompt test 覆盖。trinity architecture **全交付**:Plan 01-06 全在 main,V1.2 后端期收尾。设计 spec [`adhoc-topic-documents/forge_redesign/`](./adhoc-topic-documents/forge_redesign/) + [`README.md`](./adhoc-topic-documents/forge_redesign/README.md)。 |
| 2026-05-14 | **[fix]** workflow EdgeSpec port routing 从根上重构 — 历史 V1 用 stringly-typed `From: "<node>.<port>"` 字符串编码 port,LLM/手写都易踩坑(approval edge 没标 port → 静默 park 不跑 → run 假成功 nodesCompleted=2/3)。改成显式 `FromPort` + `ToPort` 字段(n8n/NiFi/Step Functions 主流模式),domain/app/scheduler 三层重构 + 加 `BranchOutputPorts` 表 + IsBranchingNode 接口 + validate 阶段强制(分叉节点必带 fromPort + 单输出节点必空 + 拒 legacy dotted) + scheduler.advance 改读 e.FromPort + 6 个 unit test(approval/condition/legacy/single-out 拒绝 + branching routing 正负向)。tool description 大改 create_workflow / edit_workflow 加 op cheatsheet + branching 规则示例。E2E 实测:trigger→approval→function 走 fromPort=approved → completed,nodesCompleted=3/3,flowrun_nodes 2 行(t1+f1,approval 设计上不写)。同步 workflow.md §3+§6 / flowrun.md §2.2 / error-codes.md / burn-in log。**违反 §S14 文档同步纪律 + 早期 stringly-typed 妥协的根本修复**。 |
| 2026-05-15 | **[test]** burn-in B10 sandbox lifecycle + B13 pagination 补测 — 收尾 24 维度。B10 9 子测试全过(list/destroy/gc/retry-bootstrap/conv-reset/runtime-in-use 409 整合性保护);新 finding #15-17 全低(env destroy 后 function lazy-rebuild 缺失 / ownerKind 非白名单悄吞 / runtime 表 3.12 vs >=3.12 duplicate row)。B13 4 子测试过(cursor base64 keyset {c,i} walk-to-end 干净 / limit 错入 400 / cursor garbage 400);finding #18 limit 无 cap 上限。剩 #7/#11/#15-18 全低,dogfood 时再扫。同步 api-burn-in §B10/B13 节。|
| 2026-05-17 | **[feat]** §21 多语言 i18n（中/英）—— 后端 `User.Language string`(zh-CN / en + DB CHECK 约束) + Create/UpdateInput + PATCH endpoint + `ErrLanguageInvalid` errmap。前端 `vue-i18n@10` + `src/i18n.ts`(zh-CN 默认 + localStorage `forgify:locale` + 浏览器嗅探)+ `locales/{zh-CN,en}.json` 骨架(common/topbar/nav/convs/chat/users 6 节~80 keys)+ `client.ts` Accept-Language header 注入 + `usersAPI.update` + `users.setLanguage` 双写 vue-i18n(active user 改语言即时生效)。**门面层翻译完**:TopBar / TabNav / ConvSidebar / UserPicker / UserSwitcher / Profile.vue;Profile 页加 language select(zh-CN / en)。**深层 panel 60+ .vue 保留原文**(FlowRunDetail / WorkflowDetail / FunctionDetail / 等)dogfood 撞到再补。test-unit + vet + vue-tsc 全绿。|
| 2026-05-17 | **[feat]** §20 多用户 / Profile 切换 V1.2 minimal —— **§20.1** `domain/user` + `app/user` + `infra/store/user` + `handlers/users.go`（5 endpoints）+ 5 sentinel + errmap + 8 单测；EnsureDefault 启动给空表创 `ID="local-user"` 默认 user 让现有 row 自然 surface（DB 已 user_id scope）。**§20.2** `InjectUserIDWith(resolver)` 读 `X-Forgify-User-ID` header + `?userID=` query 兜底（SSE EventSource 浏览器 API 不能自定义 header）；fallback 链 unknown → 首个 user → DefaultLocalUserID。3 新单测。**§20.3** `pkg/userpath`：`UserHome(homeRoot, uid)` + `MigrateLegacy`；main.go mcp.json / skills / .catalog.json / settings.json 切 `defaultUserHome`。4 单测。**§20.4** `api/users.ts` + `stores/users.ts`（localStorage 持久化 + switchTo reload 清 state）+ `UserPicker.vue` startup picker（≥2 profile + 无 active 显示）+ `UserSwitcher.vue` header avatar dropdown + `/config/profile` 管理页 + client.ts header 注入 + sse.ts URL `?userID=` append。**V1.2 限制**（用户接受）：后台 polling 走默认 user / 无密码 / per-user service tree 重建留 V1.5。**§20 4/4 ✅**。test-unit + vue-tsc + vet 全绿。|
| 2026-05-17 | **[feat]** §5.7 Run-level timeout + Dry-run + Live UI —— **§5.7** `Workflow.TimeoutSec int` + `StartRun` 用 `context.WithTimeout`；`runReadyLoop` 检测 `ctx.Err == DeadlineExceeded` 区分 → failed + `RUN_TIMEOUT`。**Dry-run** `FlowRun.DryRun bool` + `StartRunWithOptions{DryRun}` + `ExecutionContext.DryRun` propagate（含子图）+ `dispatchWithPolicies` 拦 function/handler/mcp/skill/llm/agent/http/approval/wait 返 `{out:"[DRY RUN]",_dryRun:true}`；approval 自动 approved；`POST /workflows/{id}:trigger?dryRun=true` 接入。**Live UI** FlowRunDetail.vue 2s polling(running/paused) + notifications store watch(type=flowrun match id 即时 refresh) + 头部 👁 DRY RUN / ⏱ RUN_TIMEOUT 徽章 + 🟢 live-pulse。Workflow trigger UI 加 👁 Dry-run 按钮。4 新单测全绿。test-unit + vet + vue-tsc 全清。|
| 2026-05-17 | **[feat]** §5.1 Workflow Loop body subgraph 完整版 —— scheduler 真 for-each：`pause.go::driveLoop` 抽 `runReadyLoop`(与 finalizeRun 解耦让子图复用); 新 `subdag.go`：`ExecuteSubDAG(req)` per-iteration + `SubDAGFromBody(map)→Graph` decoder + `SubstituteLoopTemplates` 深度模板替换(`{{ .loop.item }}` / `{{ .loop.index }}` 在 string/nested map/list 叶节点全生效)+ `concurrentRun` 并发 helper。`LoopDispatcher` 重写：sequential 默认 / `parallel: true` + `concurrency: N` 并行(default cap 5)/ `onError: stop` 默认 vs `continue` opt-in(返 `{successes, failures: [{index, error}]}`)。`flowrun_nodes` 加 `parent_loop_node TEXT` + `iteration_index INT` 列；`recordNode` propagate。`ExecutionContext` 加 `Loop / ParentLoopNodeID / IterationIndex`。body 含 approval 节点拒(V1 不暂停中途)。沿用 validate.go 早有 container-body 3 层嵌套校验。5 单测(foreach seq / fail-fast / continue / approval rejected / SubstituteLoopTemplates 嵌套)。create_workflow tool description 加 loop body 完整例。§5 状态 0/7→1/10(§5.2 标 defer)。test-unit + vet + Windows cross 全绿。|
| 2026-05-17 | **[feat]** §18 Prompt Governance 全批 — 吸收 §14.6 / §14.7 / §17.1 —— **§18.1** `GET /api/v1/dev/prompts` 41 条 prompt 总览（33 tool desc + 2 chat-system 静态段 + 3 internal-llm + 3 subagent），每条 `{name, category, content, length, tokensEst, source}`。testend `/dev/prompts` 视图。**§18.2** `chat.SystemPromptSections` 返 cache-friendly 顺序命名段；`AssemblePromptSections` 加 `<section name="...">` XML markers；`/conversations/{id}/system-prompt-preview` 端点 + testend chat header 📋 prompt modal。`buildSystemPrompt` 重构（sections → assemble 单一事实源）。**§18.3** `cmd/lintprompts`（regex 抽 const，4 规则：length / 第一人称 / weasel / emoji）；`make lint-prompts` target；基线 1 violation。**§18.4** `prompt-principles.md` 6 原则 + anti-pattern 表。**Collapse**：§14.6 + §14.7 + §17.1 归 §18 ☑（本质 prompt 工程非 feature）。**§18 4/4 ✅；§14 5/7→5/5 完工**。test-unit + vue-tsc + Windows cross 全绿。|
| 2026-05-17 | **[feat]** §12.3 + §12.4 中等批 — Per-conv model + Webhook HMAC —— **§12.3** `modeldomain.ModelRef` 共享 struct + `Conversation.ModelOverride *ModelRef`（GORM serializer:json）+ `UpdateInput.ModelOverride **ModelRef` 三态 + handler `HasModelOverride` flag 区分 absent/null + `Service.SetKeyProvider` 启 F1 422 校验（复用 ErrProviderHasNoKey 已 errmap）+ `llmclient.ResolveWithOverride` override-first + `chat/runner` 切到新 API + main / harness 注入 KeyProvider + testend types + setModelOverride API/store + `ModelOverrideEditor.vue` 弹窗 + chat header `⚙ <model>` accent-bg。autoTitle 仍走全局 picker。5 unit test。**§12.4** webhook spec 接 `signatureAlgo: "hmac-sha256-hex"` + `signatureHeader?`（缺省 `X-Hub-Signature-256`）。registration 加 2 字段；Register 验 algo 白名单 + algo 缺 secret → `ErrInvalidConfig`。handler auth 块移到 body 读后按 algo 分支：HMAC 走 `verifyHMACSHA256Hex`（`hmac.Equal` constant-time + 自动剥 `sha256=` 前缀），plain 兜底走原 `X-Webhook-Secret` / `?token=`。5 unit test。**§12 状态 2/4→4/4 完工**。同步 conversation.md + trigger.md + api-design + database-design。test-unit + vue-tsc 全绿。|
| 2026-05-17 | **[fix+feat]** §13.4 + §15.6 + §17.12 test 小修批 —— **§13.4** `cmd/server/main.go` SIGTERM 路径加 `catalogService.Stop()` + `skillService.Stop()` + `mcpService.Stop(shutdownCtx)`，polling goroutine 不再泄漏（之前依赖 OS 回收）。**§17.12** `TestList_ArchivedFilter` 3 subtest（nil 排除归档 / *false 只 active / *true 只 archived）。**§15.6** Conversation pinning：domain `Pinned bool` + UpdateInput + PATCH `pinned` + slim notif `action: pinned/unpinned` + DB ORDER BY `pinned DESC, created_at DESC, id DESC`（cursor 仍 `(created_at,id)`，单用户场景 pinned ≪ limit 无跨页漂移）+ testend setPinned / 右键置顶/取消置顶 / 📌 标识 / bg highlight / 客户端同 sort。docs：conversation.md（struct + DB schema + §7.2/7.3 API + 字段说明表）+ api-design + database-design。§13 状态 2/5→3/5；§15 状态 0/16→1/16。test-unit + vue-tsc 全绿。|
| 2026-05-17 | **[feat]** §17.11 + §17.12 真活 + §17 tracker 大扫除 —— **§17.11** chatdomain Status drift 契约：`AllStatuses` 单一事实源 + 两 host switch 拆纯函数返 `(out, known bool)` + 两契约测试遍历 AllStatuses 保 default 分支永不命中。**§17.12** Conversation `archived bool`：domain + GORM index + `ListFilter.Archived *bool`（缺省排除）+ PATCH `archived` field + handler `?archived=` query + slim notif `action: archived/unarchived` + testend ConvSidebar 📁 toggle + 右键归档项 + types/api/store 同步。**§17 tracker bitrot 扫**：§17.6（catalog snapshotSources 不持锁）+ §17.13（search.go BYOK→DDG hint 早含）核实 ☑；§17.1/§17.2（tool description + NodeType "end"）2026-05-11/14 已主体覆盖标 🔄；§17.4/§17.5/§17.7-10 ⏳ defer V1.5/v1.2.x（单用户场景不必要的 multi-user 防御）。§17 状态 1/13→7/13。test-unit + vue-tsc + cross-platform 全绿。|
| 2026-05-17 | **[fix]** §11.5 env corruption 防御 + §11 tracker 大整理 —— `infra/sandbox/spawn.go::checkBinaryExists` 预检：绝对路径 cmd 走 `os.Stat`(跟 symlink),缺失返 `ErrEnvNotFound` 复用 §11.1 lazy rebuild 路径(零额外代码路径)。**用户场景**:mise 自动升级 Python 3.12.5→3.12.6 删旧 install 目录,venv 内 `.venv/bin/python` 成 dangling symlink → 用户跑 function 不再炸 cryptic ENOENT,而是自动重建 venv 重试。3 新单测(dangling symlink E2E + 已有 nonexistent path 改归 ErrEnvNotFound + PATH-resolved 跳过预检)。**Tracker 大整理**:§11.1 (P3 batch 已做) + §11.3 (NormalizeVersion P3 batch 已做)补 ☑;§11.4 Ruby/PHP **删除**——前提不成立(`sandbox.md` §1 写明 2026-05-08 Marketplace V3 collapse 已删 Ruby/PHP/Rust/Go/Java EnvManager,无消费方);§11.6 Docker 明确"V1.5 真 defer";§11.7 disk warn 用户判定不做。§11 真活清单从 1/7 翻 5/6 主体完工。同步 final-sweep §11 全部条目重写 + tracker。|
| 2026-05-17 | **[feat]** §14.5 全交付(4 子件)—— **§14.5a** `LLMNodeConfig.AttachedDocuments` schema + `Repository.ListSubtreeIDs` 暴露 + `documentapp.ResolveAttached` live-resolve subtree + `documentapp.RenderAttachedAsXML` 共用渲染器 + `dispatch_llm.go` 前置 `<documents>` 段 + 新 `DefaultLLMCaller` adapter (llmclient.Resolve 替原 TODO nil,workflow LLM 节点首次真正可跑)。**§14.5b** `NodeTypeAgent` 14th 节点类型 + `dispatch_agent.go` 用 `app/loop.Run` + lazy toolsFn closure (避免装配序问题) + agentHost(LoadHistory/Tools/WriteFinalize) + AgentNodeConfig (prompt/attachedDocuments/enabledTools/maxTurns 默认 10 硬上限 50) + enabledTools 白名单过滤工具集。**§14.5c** `Conversation.AttachedDocuments` 字段(GORM serializer:json) + `convapp.UpdateInput` struct refactor (兼容 Rename 旧 API) + `chat.DocumentResolver` 端口 + `chat.Service.SetDocumentResolver` + `chat/runner.buildSystemPrompt` 调 ResolveAttached 前置 `<documents>` 段(memory pinned 同 cache-friendly 静态层)。**§14.5d** testend `AttachedDocsEditor.vue` 对话头部 📎 按钮 + 多选挂载 + 含子树 toggle + 文件大小估算 + `convAPI.setAttachedDocuments` + `convStore.setAttachedDocuments`。**Workflow validate** `CapabilityChecker.HasDocument` + `ProductionChecker.Document` + llm/agent 节点 attached_documents 引用预校验(缺失 → WORKFLOW_CAPABILITY_NOT_FOUND)。**6 新 pipeline test**:agent 创 doc E2E / llm attach 到 prompt / Conv 单挂 / Conv 子树挂 / Conv 空挂 / workflow validate 拒绝缺失 doc。make test-unit 全套绿;Windows 跨平台 OK;vue-tsc 干净;新文件 staticcheck 干净。同步 final-sweep §14.5 ☐→☑ + §19 tracker 4/7→5/7。|
| 2026-05-17 | **[feat]** §14.4 document → catalog 第 4 source —— `app/document/catalog_source.go` 50 行(`AsCatalogSource()` + `documentCatalogSource{Name/Granularity/ListItems}` + `topLevelSegment` helper)。Item 形态:Source="document" / Name=Path / Description(描述 → tags → "(no description)" 三层 fallback)/ **Category=path 顶段**让 Generator 按 path 分组(`/Projects/Q1` → "Projects" group)。main.go + harness 注册 3 → 4 source。**Notification invalidate hook 撤回**——catalog 已 1s polling + fingerprint dedup,document Create/Edit/Move/Delete 自动被下一 tick 捡到,spec 那条是误读已删。**4 单测 + 1 pipeline (document_catalog_test.go E2E)全绿;catalog 全 suite 47s 通;make test-unit + staticcheck + Windows cross 都干净**。同步 final-sweep §14.4 ☐→☑ + §19 tracker 3/7→4/7。|
| 2026-05-17 | **[fix]** test/harness HTTP client timeout 30s → 120s —— `TestFunction_HTTP_ListPaginated` 偶发 30s 失败,诊断:function Create 内 syncEnvSync 首次同步装 Python via mise(15-25s 典型,负载下 40s),原 30s 客户端超时偶尔输给死线。**非生产 bug**——sync 是 D-redo-14 设计意图(env 状态走 LLM tool_result + GET poll 不走 notification)。修测试 harness 4x safety margin。|
| 2026-05-17 | **[feat]** §14.3 7 个 document system tool —— `app/tool/document/` 8 文件（factory + search/list/read/create/edit/move/delete）。3 ReadOnly + 4 WorkspaceWrite，permissionsgate `toolLevels` 7 项登记，chat agent 注入（main.go + harness.go 同步）。Tool 内部把 domain sentinel 翻成 friendly text（ErrNotFound / ErrNameConflict / ErrInvalidParent / ErrContentTooLarge）给 LLM 自纠错，不直接抛错。Search 暂 SQL LIKE，LLM-ranked rerank 留下次。`delete_document` destructive 由 LLM 自报 per-call(§3 permissions 自动走 ask 路径)。**22 tool 单测 + 2 pipeline test(agent create_document 真落库 + agent read→edit 持久化)全绿**;staticcheck 干净;make test-unit 通。同步 final-sweep §14.3 ☐→☑ + §19 tracker 2/7→3/7。|
| 2026-05-16 | **[design]** §14.5 设计调整 —— (1) 拆 LLM 节点为两种：`llm`(单次+挂知识库，原节点扩 AttachedDocuments)与 `agent`(NEW 14th NodeType，agentic loop + 全套 system tools 注入)。心智清晰：`llm` ≈ 1 次 LLM call/$0.001-0.01，`agent` ≈ 1-N 次/$0.01-0.10+；写 doc 必走 `agent`（dispatch_llm.go 当前 single-shot 无 tool，确认过 backend-design L209 设计意图未实现）。(2) AttachedDocuments schema 改 `{documentId, includeSubtree?}` 替 flat ID list——挂"整 Notebook"live-resolve 跟随 doc 树变化自动包含新文档。(3) Conversation 复用同 struct，跟 workflow 节点共享 `documentapp.ResolveAttached` resolver；Repository 加 public `ListSubtreeIDs` (expose 已有私有 collectDescendantIDs)。§14.5 拆 4 子件：5a(llm 节点 attach,0.5 天)/5b(agent 节点,1 天)/5c(Conv attach,0.5 天)/5d(testend Notion UI,1.2 天) 共 ~3 天。同步 final-sweep §14.5 + document.md §9 重写 + §10 schema 改 + §14 实施顺序表。纯设计 pivot 未写代码。|
| 2026-05-16 | **[feat]** §14.2 testend 烟雾层 —— `api/resources.ts::documentAPI`(list / tree / get / create / update / remove / move 7 方法)+ `Document` domain 类型 + `'doc'` IDPrefix + `views/config/Documents.vue` 扁平表(path 缩进显示 + create/edit/move/delete dialog + raw 查看)+ `/config/documents` 路由 + Config nav 入口。`client.ts` 加 `deleteJSON` helper(document DELETE 返 body 不是 204)。vue-tsc 干净。Notion 树 + Monaco 编辑器 + 拖拽留 §14.5。|
| 2026-05-16 | **[feat]** §14.2 document HTTP API + errmap —— 7 端点（GET list / tree / id, POST create / :move, PATCH, DELETE）+ 6 errmap sentinel（DOCUMENT_NOT_FOUND/INVALID_PARENT/NAME_CONFLICT/CONTENT_TOO_LARGE/INVALID_NAME/PARENT_NOT_FOUND）+ router 注册 + 13 httptest 端到端绿（含 rename cascade / move cycle 拒绝 / delete recursive / tree 不含 content）。同步 api-design.md + error-codes.md + final-sweep §14.2。|
| 2026-05-16 | **[feat]** §14.1 document domain 4 层 + DB 全交付 —— `domain/document`（Document entity + 6 sentinel + Repository）+ `infra/store/document`（GORM 实现 + 树操作 IsAncestor/SoftDeleteSubtree/MaxSiblingPosition/UpdateBatch + BFS 后裔收集）+ `app/document`（Service.Create/Get/List/Update/Move/Delete/Search + recomputePath 子树级联）+ schema_extras `idx_documents_parent_name_active` partial UNIQUE 含 `COALESCE(parent_id, '')`（守 SQLite NULL!=NULL 漏检）+ AutoMigrate 接入 main.go + harness.go + Deps.DocumentService 字段。**19 store + 13 app + 4 domain 测试全绿**（含 cycle 检测 + 子树 path 级联 + 软删递归 + COALESCE 守卫）；全量 `make test-unit` 跑通，无 staticcheck issue，Windows 跨平台编译 OK。同步 `service-contract-documents/database-design.md` + final-sweep §14.1 ☐→☑ + §19 tracker。|
| 2026-05-16 | **[design]** §14 document 数据模型再精化为 Notion-style 树状嵌套（单表自引用 + ParentID + Position + Path 冗余字段），弃"flat doc + section 子表"。心智模型对齐——用户要的是"大文档套小文档"（项目树 / 笔记树）不是"PDF 切章节"。系统工具 2 个 → 7 个（search/list/read/create/edit/move/delete），让 AI 能用 create/move/delete 真正帮用户组织文档不止读。Workflow 接入由"专设第 14 种 document 节点"改为"现有 llm 节点 config 加 AttachedDocumentIds 字段"，Conversation 同样加 AttachedDocumentIDs（跟 memory pinned 同一 cache-friendly 层）。Catalog 按 path 分组，>50 docs 自动 progressive disclosure。新建 `service-design-documents/document.md` 详设计 doc。同步 final-sweep §14 5 子项重写 + §19 tracker + §19.2 dev log + backend-design 协作图。纯设计 pivot 未写代码。|
| 2026-05-16 | **[design]** §14 Phase 5 knowledge 弃 RAG/sqlite-vec，改 LLM-ranked document attach 模式（抄 forge/skill/mcp catalog 套路）。理由：本地单用户文档量小 + 2026-04-26 已有先例（tool search 弃向量改 LLM 排序）+ 大 context + Anthropic prompt cache 让塞全文反超 RAG。§14.1 sqlite-vec spike 取消（modernc 不再需要 C 扩展），§14.1-14.5 重设计为 `document` domain + 2 system tools + catalog 第 4 source + workflow `document` 节点，工程量减半。同步 final-sweep §14（全部 7 子项重写）+ backend-design.md（能力清单 #4 + Phase 5 描述 + 跨 domain 协作图 + domain tree）。纯设计 pivot 未写代码。|
| 2026-05-16 | **[doc]** §S11 注释规范重写 — 旧版"双语 + 节制"7 节散文版本压成 5 条硬规则(强制 3 行格式 / 只写为什么 / 一概念一注 / 密度 ≤ 1/3 / 凝练双语对照禁翻译式)。配套递归清扫 backend/ 全量 .go(~493 文件)的注释,删 design intent 段 / 决策叙事 / 翻译式重复 / 字段 godoc。CLAUDE.md §S11 + 本条 dev log;backend-design.md §157 单一指针不变。 |
| 2026-05-16 | **[feat]** **V1.2 §4 token/cost + §13 retry/timeout + §3 permissions+hooks final-sweep** —— ship gate 第三块 + 可靠性 + 可观测一波落地。**§4.1+4.2** Conversation `tokensUsed` 聚合(`SumTokensByConversation` 单 SQL SUM) + `Message` 加 `provider`/`modelId` 列(chat runner 写) + 新 `pkg/llmcost` 单价 registry(DeepSeek/Anthropic/OpenAI 16 model) + `GET /api/v1/usage?conversationId=\|period=day/week/month/all` 按 model 拆 + cost 估算。**§13.1+13.5** `infra/llm.Generate` 套 `withRetry`(3 attempts / 500ms→1.5s 指数退避,Rate/Provider/Deadline 重试;Auth/BadRequest/Canceled 不重试) + chat queue worker `context.WithTimeout(10min)` 硬上限。**§3 permissions+hooks** 9 模块完整:domain `permissions`(3 级 DangerLevel + 3 Action + 4 DefaultMode + Settings/Hook IO) → `infra/settings`(atomic.Value 快照 + fsnotify watch + 5s poll 兜底 + 坏 JSON 保留旧) → `app/tool/permissionsgate`(56 tool 静态登记 + glob 翻译 regex `*`→`.*` + deny→ask→allow→default 评估 + session ask-once SHA256 缓存) → `app/hooks`(shell exec stdin/stdout JSON + Pre/Post/Stop 3 时机 + exit 0/2/其他 三态语义 + timeout) → `pkg/pathguard.AllowWrite`(读写 deny 分离 + 默认写专属 .git/.env/node_modules/.venv) → `loop.ToolInterceptor` ctx 注入(subagent 自然继承) → `chat.toolInterceptor`(gate+Pre/Post+Ask V1.2 MVP 自动批准+缓存) → 5 HTTP endpoints(GET/PUT /settings atomic tmp+rename / :reload / /permissions/tools 按 level 拆 / /permissions/test 预测) + 2 errmap sentinel(INVALID_SETTINGS 400 / BLOCKED_BY_RULE 422) → testend `/config/permissions` 3 tab → 3 pipeline test(DenyRule_BlocksTool / PreToolUseHook_DeniesViaExitCode2 / PostToolUseHook_AppendsHint)。**附带修旧债** chat_test.go `MissingAPIKey` 测试(model.Upsert 现要求 api-key 存在,改 seed→delete 触发原 API_KEY_PROVIDER_NOT_FOUND 路径)。**设计 doc** `service-design-documents/permissions.md` 已存 18 节(Claude Code 抄 + 单用户本地优化)。**测试** 30+ 新单测全绿 / 5 pipeline 包(chat/compaction/memory/uxtodo/permissions)全绿。|
| 2026-05-16 | **[feat]** **V1.2 §2 memory + §1 compaction final-sweep** —— 两大跨对话能力一气落地。**memory**(§2):新 domain `memorydomain` 4 type × 2 source 枚举 + Repository 8 方法 + 部分 UNIQUE on name(GORM partial via schema_extras);`memoryapp.Service` CRUD + Pin/Unpin + ForSystemPrompt(pinned 全文 + 200-行 index);3 system tools `read_memory` / `write_memory` / `forget_memory`(LLM 不见 pinned 字段);7 HTTP endpoints + 3 errmap sentinel(MEMORY_NOT_FOUND / NAME_CONFLICT / INVALID_NAME);chat runner system prompt 注 catalog 后 multi-agent prompt 前;testend `/config/memory` Memory.vue 4 type badge + pin/unpin/edit/delete + create dialog;4 pipeline test(UserPinnedReachesLLM / UnpinnedOnlyIndex / AIWritePersistsAndNotifies / PinTogglesContent)。**compaction**(§1):`conversations` 加 `summary` + `summary_covers_up_to_seq` 两列 + `message_blocks` 加 `context_role` 一列(`hot`/`warm`/`cold`/`archived` CHECK)+ 新 block type `compaction`(eventlog 协议 6→7 种);`pkg/modelmeta`(8 model context window registry + UsableInput 含 2K safety buffer)+ `pkg/tokencount`(CJK 1tok / ascii bytes/4 + Calibrate + MergeCalibration 指数平滑 α=0.3);`app/contextmgr.Manager` MaybeCompact 3 路径(< Soft 跳 / Soft 降级 tool_result→warm/cold / Hard 调便宜 LLM 生成 anchored-merge 摘要并 archive);默认阈值 Soft=0.70 / Hard=0.85 / RecentTurns=3 / RecentTRKeep=5;`loop/history.BlocksToAssistantLLM` 按 context_role 投影(archived 丢 / warm 200B preview / cold tool 名占位 / hot 全文);`chat.buildHistory` 前置 `<conversation_summary>` wrapper;runner 在 turn 末同步跑 MaybeCompact(autoTitle 之前,保 fake LLM 测试确定);testend `/current/compaction` Compaction.vue 显 running summary + role 分布 + compaction events 表;3 pipeline test(NoOpBelowThreshold / DemoteOnly / FullCompactSummaryReachesNextTurn)。**附带修旧债**:test/harness Attrs `[]byte`→`map[string]any` migration 漏改 / chat_test.go Attrs 同 / scheduler approval_e2e_test legacy `gate.approved` dotted edge → `fromPort` / handler_test Python `init_args`/`args['name']` → exploded named param / scheduler `TestFlowRun_HTTP_CancelPropagates` 用 `h.Delete` ≥300 Fatalf 挡 422 分支 → 换 `DoRequest` 返 status。harness 加 `Memory` + `ContextManager` 字段 + AutoMigrate Memory + FunctionExecution + HandlerCall + MemoryService 接入 router Deps。**测试**:170+ 单测全绿 / 22 pipeline 包全绿(catalog 88s + handler 51s + function 46s 全过)。**同步文档**:`service-design-documents/memory.md` + `compaction.md` 已存 design docs;本 dev log + contract docs(api / database / error-codes / events)由本提交一并更新。|
| 2026-05-15 | **[fix]** P3 批次 — 5 issue 一气修(#7/#11/#15/#16/#17;#18 已存在 MaxLimit=200 clamp 是误报)。**#15** function/handler env 被外部销毁后 `Run`/`Call` 自动按 stored spec 重建并重试一次(避免死循环再失败即上抛);live E2E 实测:destroy → :run 触发 lazy rebuild 4.4s 完成。**#17** `RuntimeInstaller` 接口加 `NormalizeVersion(version) string`,MiseInstaller 复用 B-01 的 `normalizeVersionForMise`,`EnsureRuntime` 在 lookup/insert 前归一化,`>=3.12`/`3.12` 共用一行;1 新单测覆盖 11 边界。**#16** `validOwnerKinds` 5 值白名单 + 400 INVALID_OWNER_KIND,告别"空 list 当无数据"误读。**#7** `create_handler` / `create_workflow` description 加 MINIMAL COMPLETE EXAMPLE 段(完整 ops 数组示例),省 LLM 多次试错 tokens。**#11** workflow `validate.go::isPseudoTerminalType` 识别 7 个常见伪 terminal 类型(end/output/finish/terminate/stop/return/exit)并返教学型错误"workflows have no terminal node; the DAG ends implicitly";1 新 sub-test 5 用例。同步 sandbox.md/function.md/handler.md/workflow.md/error-codes.md/api-burn-in。backend 全绿,生产 LLM 之后撞这 6 类痛点直接 ✅。|
| 2026-05-16 | **[fix]** **burn-in v2 后续整改批 1** — 用户挨个讨论 11 项 logged finding 后,7 项决定修(其余 4 项 deferred:#1 文档准确性回写 / #8 24k token 行业都这样 / #13 DeepSeek 无视觉 / #9+#41 等 testend 编辑器第二波)。**#3** api_keys 加 (user_id, display_name) partial UNIQUE WHERE deleted_at IS NULL AND display_name != ''；新 sentinel `ErrDisplayNameConflict` → 409 API_KEY_NAME_CONFLICT;空 displayName 允许重复(server-default)。**#5** model-config PUT 加早校验:provider 没 api-key 直接 422 PROVIDER_HAS_NO_KEY,告别绿保存红运行时;model.Service 加 `keys apikeydomain.KeyProvider` 字段 + KeyProvider 接口加 `HasKeyForProvider` 方法。**#7** api-key PATCH 加 Key 字段支持原地旋转密钥(重新加密 + 重新 mask + 重置 test_status=pending + 清 lastTestedAt/modelsFound),testend type 同步加回 key?。**#4** model-config 加 GET /api/v1/model-configs/{scenario}(原只有 list + PUT)。**#23** chat runner 加连续全失败熔断 K=3(loop.go::maxConsecutiveAllFailTurns),防 LLM 在 tool error 卡壳时连建多个废 entity;新 errorCode TOOL_ERROR_STORM。**#26** trinity 加 HTTP `:edit` action(function/handler/workflow 各 ~20 行,镜像 LLM tool 语义,使用 ParseOps);testend 编辑器 UI 留第二波 1-2 周。**#27** paused flowrun 现可强 cancel:scheduler.Cancel 找不到 goroutine 但状态 paused → 直接 DB transition status=cancelled + 发通知;repo.Get 失败也归类 ErrNotCancellable 保旧 API 契约。**#28** approval reason 不再被吞:ResumeApproval 加 reason 参数,写 flowrun_nodes 一行 {decision, reason}留 audit(reason 允许空,用户可选填)。**#32** skill JSON 接受扁平 shape:`{name,description,body}` 自动回填到 frontmatter(原只接受 `{name,frontmatter:{name,description},body}` 嵌套形)。**#1** 撤回原 "CLAUDE.md 漂浮 4 scenarios" 错误说法 - 实际 CLAUDE.md 只 generic 提 scenario, 真正状态是 chat+web_summary 两个实装,推迟 forge/catalog scenario 拆分到未来。146 unit 包 0 失败,go vet 干净。同步 error-codes.md(+3 行)+ burn-in-v2 doc 状态从 logged 改 fixed。|
| 2026-05-16 | **[fix]** **burn-in v2 (live-test-v2)** — 12h 真后端 + 真 DeepSeek 端到端反复鞭打,7 个真 bug 一气修。**#11** `loop/stream.go` cancel 时 `closeStatus` 在 ctx.Err 提升前算完,open block 落 `completed` 与 message `cancelled` 状态分裂(§S21 invariant violation);**#12** `chatstore.GetAttachment` not-found 裸 `fmt.Errorf` 无 sentinel → errmap 翻 500 `INTERNAL_ERROR` 不是 404,加 `ErrAttachmentNotFound` + `%w` 包装;**#14** `POST /messages` 空 content 无附件竟接受并烧 ~25k token LLM,加 `ErrEmptyContent` 拒;**#16** `function/sandbox_adapter.writeAtomic` 写固定 `<path>.tmp` → 并发 run rename race,改 `os.CreateTemp` 唯一 tmp(20/20 并行测验证);**#20** `create_handler` 工具描述说 `set_init` 用 `body` / `set_init_args_schema` 用 `init_args`,但 `apply.go` 读 `init_body` / `args` —— LLM 永远造出空 `initBody` + 空 `initArgsSchema` 的废 handler,改对齐;**#24** workflow function dispatcher 读 `config["input"]`,handler 读 `config["args"]`,LLM 永远写 `args` → function 节点拿 nil 投 Python `**None` TypeError,改 function dispatcher 也读 `args`(legacy `input` 保留 alias);**#25** workflow `apply.go::applyAddNode` 不走 validate teach error,LLM 用 `type:"end"` 拿到通用 "unknown node type" 而非 v1#11 加的教学话,镜像 fix 进 apply 阶段。findings 41 项录入 `live-test-v2/burn-in-2026-05-v2.md`,11 项 logged-only(scenario 文档漂 / 24k 工具基线 / WebFetch 不可见成本 / DeepSeek 不收 image_url / 暂停 flowrun 无 cancel 路径 / 直 HTTP 不发 forge SSE / etc.)。146 unit 包 0 失败,go vet 干净。同步 `error-codes.md`(+ATTACHMENT_NOT_FOUND/+EMPTY_CONTENT)+ `chat.md` 错误码表。 |
| 2026-05-15 | **[fix]** burn-in P2 批次 — 6 issue 一口气修(#5/#14/#8/#13/#4/#6)。**#5** mcp/skill boot publish 缺 user ctx → WARN noise:`main.go` 包 detached ctx + `DefaultLocalUserID`(§S9 模式)。**#14** notif data 字段未瘦身:`conversation`/`sandbox_env` 改 slim shape(只 ID + 关键字段),UI 拿通知 → 主动 GET。**#8** handler `__init__` 注入名 `init_args` 与 method `args` 字典歧义:深修——`AssembleClass` 改 exploded named params(`def __init__(self, p1: str, p2: int = None)` 而非 `**init_args`),method 同样 explode + `pythonType`/`pythonDefault` 类型映射 + identifier 校验。LLM 现在 InitBody 用 bare 名字。**#13** catalog coverage 键名 LLM 决定不固定:改后端 `computeCoverage(items)` 按 `item.Source` 名字机械 group,LLM 只输出 summary;coverage 100% 完整永不漏。**#4** Block.Attrs / Message.Attrs 是 JSON-encoded string:domain 改 `map[string]any` + GORM `serializer:json`,store/SSE/handler/eventlog/subagent/loop 级联适配 + 测试同步改用 map literal。**#6** AskUserQuestion 7d timeout(替 5min)+ Skipped 字段(sentinel `(user skipped)` 让 agent 自决 default)+ options 可选;testend Composer 重写 3-state 状态机(IDLE / AGENT_RUNNING / AWAITING_ANSWER),共用一个输入框智能 route 到 `/messages` 或 `/answers`,Skip 按钮 + 黄光呼吸 + Jump-to-question 导航。剩 #7/#11(LLM tool description / NodeType 设计)留下次独立轮。同步 chat.md / catalog.md / handler.md / ask.md / api-burn-in。backend 全绿,testend 0 ts error。|
| 2026-05-19 | **[feat]** **V1.2 §17 askai + capability check + MCP 健康历史落地** —— 前端洞察 / 编辑器 / mcp 详情屏需要的最后一批后端端点。**C1** `POST /workflows/{id}:capability-check`（ValidateGraph 包成报告，pre-flight 校验前端编辑器试跑前调）。**C2** 新 `app/askai/` 包：Spawner 工厂 = 创建带 system prompt 的内部对话 + 发首条消息 + 返 conversationId（前端订阅 eventlog 看 AI 推理）。**C3** 4 个 `:iterate` 端点（function/handler/workflow/document），每个用 entity 当前状态做 system prompt，复用 askai Spawner。**C4** `POST /flowruns/{id}:triage` —— flowrun 全状态 + workflow graph 做 context，AI 失败排查；可选 user prompt（空时默认"分析这次失败"）。**C5** 新 `mcp_health_history` 表（mch_ prefix）+ `mcpHealthRepo` + HealthCheck 每次调用自动追加快照 + `GET /mcp-servers/{name}/health-history?sinceMinutes=N` 端点。**C6** `POST /mcp-servers/{name}/tools/{tool}:invoke` 直接调 MCP 工具（绕 chat/LLM）。conversation 加 `CreateWithSystemPrompt` 方法支持预填 SystemPrompt 字段。`harness.go` + `main.go` DI 装配 askaiSpawner + mcpHealthRepo + 4 handler.SetSpawner / flowrunHandler.SetAskAI。共新增 1 新表 + 1 新 app 包 + 5 新端点 + 1 capability check 端点。167 unit tests 0 失败。设计决策：askai 起的对话用户可见（不焚毁，可追问），名字 AI auto-title 自然生成；不自动重跑 flowrun；统一 chat scenario model；prompt 可选。同步 contract docs（api-design + database-design）+ CLAUDE.md S15 加 mch_ prefix。 |
| 2026-05-19 | **[feat]** 零散补 2 项：(1) notifications REST 快照 — `GET /api/v1/notifications` 无 SSE header 时走 JSON 分页列表（Bridge.List + Accept-header 分叉，max 200 条）。(2) pending questions 红点 — ask.Service 注入 notif.Publisher，Wait 推 `type=ask` 通知，Resolve/cleanup 推 `action=resolved`；前端订阅 notifications 流控制 sidebar 红点。另确认 conv pin/archive PATCH、skill invoke return shape、workflow activeVersionId 已满足需求，无后端改动。build + 全单测绿。 |
| 2026-05-19 | **[doc-fix]** §S14 联动补齐：workflow/flowrun/mcp/conversation.md 补端点与方法；backend-design 架构树加 relation/askai/mcphealth/wikilink；CLAUDE.md §S13 别名表 + N5 `:iterate`/`:triage` 标准 action。 |
| 2026-05-19 | **[feat]** relation domain（跨实体关系图，§16 R1-R4）落地：relationdomain/store/app + `pkg/wikilink` 解析 `[[id]]`（5 实体）+ trinity version 表加 `ForgedInConversationID`。18 个 source-domain hook（CRT/Delete 级联，`SetRelationSyncer` 反注入避循环）+ 3 端点 + 4 sentinel。domain/store/app/wikilink 单测全绿，17 测试包无回归。设计见 [`relation.md`](./service-design-documents/relation.md)。欠 R5 pipeline test。 |
| 2026-05-13 | **[feat]** forge_redesign Plan 05 交付（17 commits）——workflow execution plane（trinity 最后一块，trigger→scheduler→flowrun 单向链）：flowrun domain+store；trigger（cron/fsnotify/webhook/manual 4 listener，robfig/cron v3 首引入）；scheduler（7-gate StartRun + topo Kahn + 13 节点 dispatcher + retry/timeout/approval pause + 跨进程 rehydrate §6.1）；mcp_calls/skill_executions 两表（D22）；6 LLM tools + 8 端点 + 14 sentinel。134 单测 + 38 pipeline 全绿。trinity 完工。设计见 [`05-execution-plane.md`](./adhoc-topic-documents/forge_redesign/05-execution-plane.md)。 |
| 2026-05-24 | **[refactor]** user-identity cleanup（7 commits）——砍 `local-user` magic + 4 级 fallback。新 sentinel `ErrUnauthorizedNoUser`（401）；前端 401 自愈 + 单用户自动选；后台任务真按 user 遍历（缺 user 直接 drop 不静默）；middleware 拆 `IdentifyUser`/`RequireUser`（breaking，router `requireUserExempt` 放行 users/health）；删 `EnsureDefault`/`DefaultLocalUserID`。backend 单测 + vet + frontend vitest 607 全绿。spec/plan 见 docs/superpowers。 |
| 2026-05-24 | **[feat]** 新 `GET /api/v1/scenarios` 端点（无 service 依赖,直读 `modeldomain.ListScenarios()`）+ `/providers` 与 `/scenarios` 一并加入 `requireUserExempt`（含 2 个 router 守护测试）。修了前端 ConfigPane ModelsTab 硬编码 5 scenario 含 3 个后端不支持 + onboarding ProviderStep 触发 401 拿不到 provider 两处 bug。model.md §10.3 / api-design.md / 本条同步。 |

### 前端开发：Welcome + Sidebar Gemini 风格重做（2026-05-25）

| 日期 | 内容 |
|---|---|
| 2026-05-25 | **[feat]** Welcome + Sidebar Gemini 风格重做（20 task TDD）：greetings 池 + useGreeting/useContextStrip/useDisplayName 三 hook + WelcomeInput/SidebarSection；重写 Sidebar/Dashboard；ui.js persist 三展开态；NotificationsDrawer 加待办 tab。新增 28 测试全绿（38 个 pre-existing fail 不动）。 |

### 后端：Catalog 懒生成 + mechanical 重构（2026-05-25）

| 日期 | 内容 |
|---|---|
| 2026-05-25 | **[refactor]** Catalog 从"轮询 + LLM Generator + 磁盘 cache"收敛为"开聊按需现查 + mechanical 拼装"。动因：Generator 每次变更/冷启都烧 API 余额。删 polling/disk/generator/history；HTTP 4→1（`GET /catalog`）；document 移出 catalog 改走 @-mention。单测 + catalog pipeline 全绿。决策：喂结构化清单优于二手摘要，零成本/瞬时/确定。 |

### 前端 + 后端：@-mention 引用（2026-05-25）

| 日期 | 内容 |
|---|---|
| 2026-05-25 | **[feat]** @ 引用：发送时把实体内容**快照**进消息（非每轮重注入）。新 `domain/mention` 端口 + document/trinity 的 `AsMentionResolver`；chat `Send` 存 `Attrs["mentions"]` + 渲染 `<mention>` 块；修 `DisallowUnknownFields` 拒收断链。范围 document + trinity，skill/mcp 不做（自有 activation）。单测全绿，e2e 留后。设计见 mention.md。 |
| 2026-05-25 | **[feat]** 首次启动/身份引导重做。根因：脏 `activeUserId`（指向已删 user）越过旧闸门 → SSE/查询 401 刷屏。修：`computeBootState`（ready 要求 activeUserId∈users）+ 7 查询加 `enabled` 纵深防御 + 引导改 6 步向导（早建 user、显式选模型、自动识别语言/明暗）+ accent 真生效（此前全锁同一蓝）。vitest 653 全绿 + Playwright 6 步 0 console error/0 个 401。spec 见 docs/superpowers。 |

### 后端：settings-redesign — APIKey is_default（2026-05-25）

| 日期 | 内容 |
|---|---|
| 2026-05-25 | **[feat]** `api_keys` 加 `is_default` 列 + `ClearDefaultForCategory`/`DefaultProvider`（per-category 单选）；`WebSearch` 把标记 provider 提到首位。2 单测，make test-backend 全绿（174 包）。apikey.md/api/db 同步。 |
| 2026-05-26 | **[feat]** 设置重做（前端，9 task）：齿轮 → 居中 modal `SettingsModal`（单开手风琴），删 `ConfigPane` 5-tab + `SettingsPopover`。API Keys 改 key 为中心（对话默认 = `model-config.chat.provider` 而非 api-key.isDefault）；ProviderGrid/KeyVerifyField/ModelSelect 引导页 + 设置共用（DRY）。外观/系统 section。vitest 全绿 + Playwright 走查 0 error。spec/原型见 docs/superpowers。 |

### 文档对齐：完成度审计 + 反误导大修（2026-05-26）

| 日期 | 内容 |
|---|---|
| 2026-05-26 | **[audit]** 全项目契约一致性审计（5 并行 agent，报告 `completeness-audit-report.md`）。2 🔴：① askai prompt 指向不存在的 `edit_forge`；② `set_dependencies` cheatsheet 键 ≠ apply.go 解析键 → 静默丢依赖。+ ~14 🟡。反直觉发现：workflow 执行引擎其实早建好（scheduler ~2587 行、有 e2e），文档却说未实现 Phase 4。 |
| 2026-05-26 | **[doc-fix]** 反误导大修（仅文档）：CLAUDE.md（阶段 0-4 完成 + P5 部分 + 转前端、§S15 前缀表重写、eventlog block 6→7、测试基线如实标 e2e 编译失败）+ backend-design + 本文件快照。契约文档漂移另清。 |

### 审计修复:代码落地(2026-05-26)

| 日期 | 内容 |
|---|---|
| 2026-05-26 | **[fix]** op 键名全仓归一：`set_dependencies` `deps`→`dependencies`；workflow op 显式 `nodeId`/`edgeId`；handler `init_body`→`initBody` 等（camelCase 合 N3，DB 列仍 snake）。3 apply_test 加断言。根因审计 🔴-2：cheatsheet 键 ≠ apply.go 解析键 → 静默丢字段。 |
| 2026-05-26 | **[fix]** 清 prompt 幽灵工具名(🔴-1 + 🟡-C):askai iterate/triage `edit_forge`→`edit_function`/`edit_handler`/`edit_workflow`(参数 `functionId`/…→`id`);Explore `AllowedTools` `search_forges`→真名 + 去 `LS`;`call_mcp`→`call_mcp_tool`、`search_mcp`→`search_mcp_tools`;`list_function` 删;multi_agent configState 门限定 handler。 |
| 2026-05-26 | **[feat]** 新增 `trigger_workflow` 聊天工具(🟡-A):薄包 `scheduler.StartRunWithOptions`(`dryRun` 支持);`WorkflowTriggerTool` 在 scheduler 构造后注册 → 只进编排者工具集、不进 subagent(D21)。makes multi_agent_prompt step 6 成真。4 单测。 |
| 2026-05-26 | **[feat]** lintprompts 防复发守卫:prompt 反引号工具引用 + `AllowedTools` 条目跟真实注册表(扫 `Name()` 返回)对账,引用不存在工具名 → `make verify` fail;roots 补 `internal/app/askai`。4 单测。根治 edit_forge/trigger_workflow/search_forges 这类复发。 |
| 2026-05-26 | **[doc]** workflow.md/backend-design 加 trigger_workflow（8→9 工具）；function.md `deps`→`dependencies` 同步。遗留：`compactSystemPrompt` 有 pre-existing lintprompts length 890>800 违例（未动）。 |

### 能力披露 token 重构 — Task 8 按需工具(2026-05-26)

| 日期 | 内容 |
|---|---|
| 2026-05-26 | **[feat]** 能力披露 Task 8——工具按需加载（核心 ReAct 循环）。`loop.Host.Tools()`→`Tools(ctx)`；`loop.Run` 改循环内每步重算 tools + byName（与 offer 集严格一致，防调度到未展示工具）；`chatHost.Tools` 返 Resident + 已激活 Lazy 组。TDD 先写 4 个失败测试，make test-backend 全绿（174 包）。chat.md 同步。注：T6/T7 此前直推 main 无 dev log。 |

### §S14 文档同步 — 能力披露重构全量补记(2026-05-26)

| 日期 | 内容 |
|---|---|
| 2026-05-26 | **[doc]** 能力披露重构（T1-T15）全量 §S14 补记。核心：`injectStandardFields` 改 slim shells（省 ~13k token）+ `tool_conventions`/`capabilities` system 段 + `activate_tools` RESIDENT meta-tool + `Toolset{Resident 28, Lazy 6 组}` per-step 重算 + Anthropic `cache_control`。实测 context ~5.1k token（vs 重构前 28k）。文档：capability-disclosure/catalog/chat/api-design/CLAUDE.md §S18 同步。make verify PASS。 |

### chat system prompt 重写 + prompt 面一致性(2026-05-27)

| 日期 | 内容 |
|---|---|
| 2026-05-27 | **[feat]** chat system prompt 重写（对齐 best practice、治对话效果）+ prompt 面一致性修复。段重构：`base`→`identity`、新增 `how_to_work`（7 操作原则）、`tool_conventions`→`tools`、删 `multi_agent_forging` 段、`locale_hint`→`environment`。🔴 修 live bug：旧 prompt 教 `Subagent(type=)` 但真参数是 `subagent_type`。另删双 H2、memory banner 换 markdown 等审计修复。make verify PASS（34 prompts 0 violation），token ~5016 持平。设计见 chat-prompt-redesign.md。 |

### 前端：DESIGN.md 视觉 + 文案落地（2026-05-25，已被 FSD revamp 覆盖）

| 日期 | 内容 |
|---|---|
| 2026-05-25 | **[style]** DESIGN.md 视觉 + 文案在 main 上落地（`def93c9`）：tokens.css 换值（单一 accent `#378ADD`、字重只 400/500、pill 圆角、加 breathe/rise keyframe）+ components.css 批量去 uppercase/600-700；onboarding/chat/cmdk/settings/forge/dashboard 文案口语化。**[fix]** 修 conversation-not-found 死循环（`444ec95`，切户未清 `activeConv`）。纯美术+文案、未动 JSX 结构；此版已被后续 boilerplate 退役 + FSD revamp 覆盖，留作历史。原 246 行详报告 `PROGRESS.md` 已并入本条。 |

### 前端 TS + FSD 完整 Revamp（2026-05-26 ~ 2026-05-27）

| 日期 | 内容 |
|---|---|
| 2026-05-26 | **[决策]** TS + FSD 6 层 revamp 方向批准（spec `2026-05-26-frontend-architecture-revamp-design.md`）。推翻 PRD §1 "不引入 TypeScript"；完整 FSD 6 层零裁剪；DIP 注入模式解 shared→上层反向依赖；身份建模为 `entities/session`（D6 最规范）。设计原则：与后端 Go clean arch 对等低耦合高内聚，为长生命周期留满空间。 |
| 2026-05-26 | **[feat] 阶段 0** shared 基础设施迁移：`shared/api/{httpClient,authProvider,queryKeys,sse,errorMap}.ts`、`shared/bridge/wails.ts`、`shared/ui/{toastStore,Button,Badge,Icon,Kbd,Spinner,Select,index}.ts`、`shared/lib/{motion,i18n}`。authProvider DIP 注册点（`setUserIdProvider`/`setOnAuthFailure`）实现。vitest 全绿。 |
| 2026-05-26 | **[feat] 阶段 1** entities 层迁移（12 entity + session + settings）：`entities/session/model/sessionStore.ts`（唯一真相 + phase 状态机 + `resolve()` fresh-only）；`entities/settings/model/settingsStore.ts`（偏好从 ui.js 拆出）；`entities/conversation/model/chatStore.ts`（rAF 合并 + tree 算法原样保留）；function / handler / workflow / flowrun / document / skill / mcp / memory / apikey / relation / user 各 `api/` + `model/types.ts` + `index.ts`。entity 类型集中定型（协议变更唯一改动点）。 |
| 2026-05-26 | **[feat] 阶段 2** features 层迁移：`features/{send-message,forge-iterate,forge-review,workflow-edit,onboarding,settings,ask-user,entity-link}`。用例 hook 在 `model/`（= 后端 app/service）；组件 `onClick` 零业务决策铁律落地。 |
| 2026-05-26 | **[feat] 阶段 3** widgets / pages 层迁移：Sidebar / CommandPalette / NotificationsDrawer / RelGraph / VersionRail → `widgets/`；6 个 pane → `pages/{chat,forge,execute,library,dashboard,observe}`。paneStore / overlayStore / sidebarStore 归 `app/model/`。 |
| 2026-05-26 | **[feat] 阶段 4（根治 401 bug）** app 层收口：`app/model/useSessionBootstrap.ts` 注入 DIP + 启动 `session.resolve()`；删 5 处散落自愈（App.jsx 两个 effect + httpClient 401 清除 + sse 401 自愈 + boot.js valid 判定）；app boot gate（session.status=ready 才挂载 AppShell → enabled gate 统一、不散在每个 entity hook）。 |
| 2026-05-26 | **[feat] 阶段 5** 严格化 + 文档体系：`steiger` + `eslint-plugin-boundaries`（6 层注册 + 单向规则）+ `tsc --noEmit`；`make lint-frontend` 与 `staticcheck` 同等地位。文档体系建立：`frontend-design.md`（架构愿景）+ `frontend-contract-documents/{fsd-layers,entity-types,cross-cutting}.md`（3 契约索引）+ `frontend-design-documents/`（38 slice 详设计）。 |
| 2026-05-27 | **[doc] Task 5.8** CLAUDE.md FSD 宪法写入（§FSD 6 层定义 + 依赖规则 + DIP + 横切归属表 + §F1 触发表）；文档地图补前端 5 条；§S14 明确覆盖前端；PRD §1/§2/§5/§17 同步架构变更；progress-record 补完整前端进展段。 |
| 2026-05-27 | **[fix]** revamp 审计修复：`PendingAsk` 提至 `shared/api/types.ts`（清 `@app` 反向依赖）；删 5 空 ui/ 目录；22 测试文件扩展名规范化（vitest 756 不减）；文档计数修正。tsc strict 0 / vite build ✅ / steiger 无问题 / eslint 0。 |
| 2026-05-27 | **[fix]** revamp 深度审计：测试 harness 重写为 .ts 删 16 个 `@ts-nocheck`（修 10+ 测试字段错）；`useWorkflowEdit.resetDirty()` 防脏态渗版本；`resolveSession` 并发去重 + onboarding 清 stale userId；entity-link 契约核对（代码已正确，补注释）。tsc 0 / vitest 760 全绿 / vite build ✅。 |
| 2026-05-27 | **[test] 前端覆盖率门槛达标** — 补 28 个 `.test.ts/.test.tsx` 文件（纯新增，零改源码），覆盖 features/entities/widgets/app 各层低覆盖 hook；最终 `npm run test:coverage` 全绿：Stmts 82.75% / Branches 75.64% / Functions 77.18% / Lines 84.98%，四项全超门槛（80/75/75/80）。110 个测试文件 / 935 测试。关键突破点：`useEntityDirectory` 后备分支（单独 fallbacks 文件）、`useForge` handler/workflow 失效路径、`SSEProvider.deriveOverall` 三分支、`diffToOps` position?.x 可选链、`markDirty` 空 ops 短路。 |
| 2026-05-27 | **[完工]** Task 5.9 revamp 总验收——前端 TS + FSD revamp 阶段 0-5 全量交付。spec §16 九项验收全 ✅：tsc strict 0 / steiger 无问题 / 每 slice 有 index.ts / page onClick 零业务 / 身份单一真相 + DIP boot gate / 全局 errorMap / vitest 756 / vite build / 文档体系全同步。 |

### 仓库清理：boilerplate 退役 + 产物治理（2026-05-27）

| 日期 | 内容 |
|---|---|
| 2026-05-27 | **[chore]** 停止追踪误入库的编译产物：`backend/{server,lintprompts,fakeserver}`（散落 `go build` 二进制，Makefile 全用 `go run`、从不消费）`git rm --cached` + 加 `.gitignore`；`server` 达 96MB 触发 GitHub 大文件警告。**`make clean` 扩容**：除 dev 数据外，新增清散落二进制 + `backend/sandbox` + `frontend/{dist,coverage}` + `testend/dist`（可再生、恢复快）；`node_modules` 与内嵌 `mise/` 仍归 `reset`（重装慢）。 |
| 2026-05-27 | **[chore]** **clean/reset 边界归位**——上一条把散落二进制 + `backend/sandbox` + `dist/coverage` 塞进 `make clean` 是越界：`clean` 回归只清 dev 数据（轻量、日常安全），所有构建产物 + 全部 `node_modules`（root/frontend/testend）+ superpowers 散件统一归 `reset`。同时把 superpowers 工作流产出 `docs/`（22 追踪文件，plans/specs/prototypes，非项目文档——真文档在 `documents/`）`git rm --cached` + 加 `.gitignore`（连同已 ignore 的 `.superpowers/`），并入 `reset` 清理范畴。`make -n clean/reset` 解析通过。 |
| 2026-05-27 | **[chore]** **boilerplate 原型退役**——`boilerplate/`（29 追踪文件）整删。前端 FSD revamp 完工后视觉事实源已转为已实现的 `frontend/src`（组件 + `src/styles/`），原型继续留着会误导。同步文档：CLAUDE.md（文档地图 / 改代码前必做 / "遇到 UI bug 原则" 改名去 boilerplate / "已定型视觉决策" 改名 / CSS class 命名 / §F1 触发表）；`frontend-prd.md` 顶部加退役横幅 + §0 视觉事实源改指 `frontend/src`（正文 ~40 处历史 boilerplate 提及保留为历史实现上下文，由横幅统一声明，不逐行擦）。 |

---

## 3. Phase 4-5 路线（粗粒度）

**状态更新（2026-05-26）**：Phase 4 ✅ 已交付（2026-05-13）；Phase 5 🚧 部分交付（document / mcp / skill / memory / compaction ✅，intent / chat 终极版未做）。下方为当初的粗粒度规划，保留作历史参考。

### Phase 4：工作流能力（✅ 已交付 2026-05-13，执行引擎 `app/scheduler` ~2587 行）

workflow（DAG + 状态机）+ flowrun（执行实例）+ 节点系统（LLM / Tool / Trigger / Approval / Variable 5 类）+ scheduler（cron / fsnotify / HTTP webhook）+ chat 再升级支持"对话创建工作流"。执行引擎自实现（Eino 已全面移除，不再依赖 eino/compose）。

**桌面端预留**（来自优化轮决策）：
- `Notifier` 接口在此阶段定义（domain/notification/），scheduler 依赖
- `Preferences` service 在此阶段定义（含 startOnLogin / missedTaskPolicy 等配置项）
- scheduler 状态全部走 store 持久化；时间源用 monotonic 算间隔、wall clock 调度具体时间；错过任务策略明确决策（skip/runOnce/runAll）

### Phase 5：智能化（🚧 部分交付）

document（**LLM-ranked attach 模式，无向量库 / 无 sqlite-vec / 无 chunking**——2026-05-16 设计改向，详 final-sweep §14）+ intent（ReAct Agent）+ chat 终极版（意图识别 → 工作流推荐 → 自动建草稿）。mcpserver / skill 已提前在 V1.2 D5-D7 准备件交付。

**风险点已消除**：原计划 sqlite-vec 兼容性 spike 在 2026-05-16 设计改向后取消——document 不引向量库，跨平台编译一行命令保住。如未来真撞上"全公司 wiki 几千篇"或"代码 chunk 索引"这类**真正大规模跨文档模糊查询**场景，再加 embedding 列 + 向量库当二进制工具平滑扩展。

---

## 4. 规范/原则演化

按时间倒序，查最近变化用。

| 日期 | 变化 |
|---|---|
| 2026-05-01 | **桌面端架构边界定型**：`internal/infra/desktop/` 仅 `cmd/desktop` import，`cmd/server` 编译产物保持纯净（不含 Wails / 托盘 / 通知代码）。transport 层永远只 httpapi 一份，不走 Wails binding |
| 2026-04-26 | **S14 hook 落地**：在 `.claude/settings.local.json` 配 PostToolUse hook，编辑 `backend/internal/` 下文件时自动注入文档同步提醒 |
| 2026-04-25 | **S3 扩展"严禁藏错误"**：`_ = err` 静默跳过严禁——隐藏的错误会在意想不到的地方爆发（教训：FTS5 虚拟表建失败后触发器仍建成，INSERT 时才炸）|
| 2026-04-25 | **S12 扩展**：主文件用包名规则推广至 app / infra/store 全层；明确"仅 Service 实现接口 / 小工具函数"合并入主文件，不单独建文件 |
| 2026-04-25 | **providers.go 归属原则**：辅助注册表放在消费它的层（非 domain）；domain 层只放 entity + sentinel + 接口 |
| 2026-04-24 | 立 **设计原则 #7 + S14 "文档同步纪律"（最高优先级）**：每次改代码联动三处文档；发现不符立刻修 |
| 2026-04-24 | 立 **设计原则 #6 "反校验剧场"**（单开发者 + 本地 Electron 不搞前端已覆盖的校验）|
| 2026-04-24 | 立 **"spec 优先于邻居文件"** 审计纪律（避免复制 pre-existing 违规）|
| 2026-04-24 | 文档结构三层分工：`backend-design.md`（规范） / `service-contract-documents/`（索引） / `service-design-documents/`（详设计） / `progress-record.md`（进展） |
| 2026-04-24 | 立 **S13 包命名**（三层同名 + `<name><role>` 调用方别名）|
| 2026-04-24 | 立 **S12 包结构**（domain 平铺按概念拆，禁子目录）|
| 2026-04-23 | 立 **设计原则 #5 "端到端推演先行"**（每 domain 开工前走完整数据流）|
| 2026-04-23 | 路线图升级：V1.0 重写 → Agentic Workflow Platform 完整愿景 |
| 2026-04-23 | S11 扩展为 **"双语 + 节制"** 完整规则；全量瘦身 ~420 行冗余注释 |
| 2026-04-22 | 立 **S11 双语注释规范** |
| 2026-04-22 | 定 **12 条契约标准**（N1-N5 / D1-D5 / E1-E2）|
