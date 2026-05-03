# V1.2 Backend 进展记录

**关联**：
- [`backend-design.md`](./backend-design.md) — 总体设计 + 规范（相对稳定，很少动）
- [`service-contract-documents/`](./service-contract-documents/) — 每个 domain 的服务契约索引（一眼清单）
- [`service-design-documents/`](./service-design-documents/) — 每个 domain 的详细设计
- [`desktop-packaging-notes.md`](./desktop-packaging-notes.md) — 桌面端分发方向（Wails / 打包 / 常驻后台）
- [`claude-code-research-documents/`](./claude-code-research-documents/) — Claude Code 内部机制调研（9 份主题报告）

**本文档定位**：所有"正在发生"的状态都在这里。开发日志 / 完成快照 / 待办清单 / 原则演化。规范/架构/愿景这些"相对不变"的放 `backend-design.md`。

---

## 1. 当前快照（截止 2026-05-01）

| Phase | 主题 | 状态 | 里程碑 |
|---|---|---|---|
| **Phase 0** | 骨架（go mod + main + /health） | ✅ | 2026-04-22 |
| **Phase 1** | 基础 infra 7 件套（GORM / logger / crypto / events / middleware / response / pagination） | ✅ | 2026-04-23 |
| **Phase 2** | 基础对话能力（apikey / model / conversation / chat） | ✅ | 2026-04-25 |
| **Phase 3** | 工具锻造（forge / attachment / tool / chat 加 tool-calling） | ✅ | 2026-04-26 |
| **Phase 3 后优化轮** | chat 基础设施重构 / pipeline → runner / 调研 / 驱动迁移 / 打包方向 | 🔄 进行中 | 2026-04-27 起 |
| **Phase 4** | 工作流（workflow / flowrun / 节点 / scheduler / trigger） | ⬜ 未开工 | — |
| **Phase 5** | 智能化（knowledge / intent / mcp / skill / chat 终极版） | ⬜ 未开工 | — |

**当前测试规模**：~170 单元/集成测试全绿（除 5 个 LLM 集成测试因 DeepSeek API key 环境失效，与代码无关）。
**当前驱动**：modernc.org/sqlite（纯 Go，无 CGO），跨平台编译一行命令。
**当前依赖体系**：完全摆脱 Eino（chat 重构后）。

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
| 2026-04-27 | **[refactor] 重构决策**：深度分析当前 chat 管线，发现三处系统性设计债：(1) DB schema 把 LLM 内容结构拍扁成多列；(2) Eino 黑盒渗透 app 层，SSE 解析和请求构建完全不可见；(3) collectStream 整流收完再推，且 mid-stream 取消状态写错。新增 `archaved/refactor-chat-infra.md` 详细设计文档 |
| 2026-04-27 | **[arch] 自实现 ReAct Loop**（替换 react.NewAgent + Callback）：发现 Eino v0.8.11 `ModelCallbackHandler.OnEnd` 对流式 ChatModel 不触发，导致 DB content 空、status=error、tool_call_id 用 fallback。彻底弃用 `react.NewAgent`，改为直接调 `model.ToolCallingChatModel.WithTools().Stream()` |
| 2026-04-27 | **[refactor Step 1] `internal/infra/llm/` 新建**：完全自主 LLM 流式客户端，取代 Eino。4 个文件：`llm.go`（核心类型 + Client 接口）+ `openai.go`（OpenAI/DeepSeek/Qwen/Moonshot/Ollama）+ `anthropic.go`（/v1/messages 原生格式）+ `factory.go`。关键决策：`iter.Seq[StreamEvent]` 替代 channel；`EventToolStart` 在 tool name 首次出现时立刻 emit；`classifyHTTPError` 区分 401/429/400/404/5xx |
| 2026-04-27 | **[refactor Steps 2-11] chat 基础设施全量重构完成**：(2) `app/agent/tool.go` 4 方法 Tool 接口；(3) `domain/chat/chat.go` Message 精简为纯元数据 + Block 实体 + 5 种类型；(4) `infra/db` 新增 message_blocks 表，移除 FTS5；(5) `infra/store/chat` Save 事务写 blocks，ListByConversation 批量取 blocks 避 N+1；(6) 事件层加 ChatToolCallStart，ChatDone 用 inputTokens/outputTokens；(7) system/web/forge 全部新接口，Eino import 全消除，DDG 切 lite 端点；(8-9) `app/chat/` 拆 5 文件（chat/pipeline/stream/tools/history），事件驱动 iter.Seq，并行 tool call，mid-stream 取消正确写 cancelled；(10) main.go 切 llmFactory；(11) 删除 `infra/eino/`，go.sum 清空 Eino |
| 2026-04-27 | **[refactor 测试补全]**：infra/llm 21 单测（OpenAI/Anthropic SSE 解析 + request builder）；app/agent 35 单测（4 方法接口合规 + system tools）；app/chat 18 单测（assembleAssistantBlocks + history rebuild）；store/chat 适配 Block 模型 + 3 新增 Block 测试。22 包全绿 |
| 2026-04-27 | **[fix] 三处严重逻辑 bug 修复**：(1) ReAct 多步循环 DB 覆盖——`runReactLoop` 统一管理持久化，所有步骤 blocks 累积进 `allBlocks`，一次 save 完整记录；(2) maxSteps 退出 DB 状态不一致——退出后显式 persistMsg 写正确 stopReason；(3) 用户消息附件 block 缺元数据——`buildUserBlocks` 从 DB 查附件完整信息再构建 |
| 2026-04-27 | **[refactor] 代码清理**：删除死代码 `app/agent/summarizable.go`；统一 `blocksToAssistantLLM` 消除重复；全局修正 S13 alias 违规（agentpkg → agentapp，eventsdomain，chatinfra） |
| 2026-04-27 | **[fix] T15-T19 补丁**：T15 forge.go 加 msgID/toolCallID context helpers；T16 GenerateTestCases Input/ExpectedOutput 改 json.RawMessage；T17 extractJSONFromLLM 剥 markdown fence；T18 extractTextContent 改返回最后一个 text block（多步 ReAct auto-title 质量提升）；T19 chatRepo 共享单实例 |
| 2026-04-27 | **[feat] Thinking 可见性**：新增 `chat.reasoning_token` SSE；`Message.ReasoningContent` 字段（DeepSeek-R1 thinking 模式 history 重建必需，否则 400）；testend 聊天面板 `🤔 Thinking…` 折叠块 |
| 2026-04-27 | **[fix] 集成测试发现 4 个生产级 bug 并修复**：(1) `created_at=0001-01-01` 错排——改用 `OnConflict.DoUpdates` 仅更新 status/tokens 不覆盖 created_at；(2) 取消流后助手消息丢失——finalPersist 用 `context.Background()` 不受取消影响；(3) `web_search` 返回 null——切到 `lite.duckduckgo.com/lite/` POST 表单；(4) 快速连发消息历史顺序错——`buildLLMHistory` 加 currentUserMsgID 参数，跳过后单独追加到末尾 |
| 2026-04-27 | **[test] 集成测试 13 组全通（真实 DeepSeek API）**：A-M 涵盖 Conversation CRUD / API Key / 分页 / 系统工具 / 并行 tool call / 多步 ReAct / Attachment 内联 / 错误处理 / Auto-title / 历史重建 / SSE messageId 一致性 / Forge 工具创建 |
| 2026-04-27 | **[doc-sync] events-design.md / database-design.md / chat.md** 全量同步：messages 表精简、message_blocks 新表、chat.tool_call_start / chat.reasoning_token 新增 |

#### Chat pipeline 二次重构（2026-04-27 后）

| 日期 | 内容 |
|---|---|
| 2026-04-27+ | **[refactor] 移除 pipeline.go，引入 runner.go**（commit b6a9199）：chat 执行管道二次拆分，优化 task 处理流程，为后续 **context compaction**（长对话上下文压缩）能力预留接口。pipeline 概念合并入 runner，事件流和 tool 调度逻辑更内聚 |

#### 开发体验工程化

| 日期 | 内容 |
|---|---|
| 2026-04-27+ | **[devx] Brewfile + Makefile setup target + 11 testend YAML collections**（commit 6113d16）：`Brewfile` 锁定 Mac 开发环境（Go + Python3 等）；`make setup` 一键检查 Xcode CLT / 装 Homebrew / 装依赖 / `go mod download`；testend 新增 11 个 YAML collection 文件用于测试不同场景 |

#### Claude Code 内部机制调研

| 日期 | 内容 |
|---|---|
| 2026-04-28 | **[research] Claude Code 内部机制调研**（commits 3851981 / f7ca688 / 35b5b90）：产出 `claude-code-research-documents/` 9 份主题报告——01-agent-loop / 02-tools / 03-context / 04-memory / 05-subagent / 06-hooks / 07-ux-tools / 08-permissions / 09-mcp，外加 `agent-core-upgrade.md` 和 `summary.md`。目的：吸收业界最成熟 agent loop / context compaction / memory 机制经验，为 Phase 4-5 的 chat 终极版（intent + workflow 推荐）和 long-running agent 设计提供参考 |

#### SQLite 驱动迁移（2026-05-01）

| 日期 | 内容 |
|---|---|
| 2026-05-01 | **[infra] SQLite 驱动 mattn → modernc.org/sqlite（纯 Go）**：为桌面分发铺路。glebarez/sqlite 接 GORM，DSN 改 modernc 的 `_pragma=...` 语法，删 CGO_CFLAGS。三平台一行交叉编译。性能慢 1.5-2x（本地无感）。|

#### 桌面端分发方向定型（2026-04-30 ~ 2026-05-01）

| 日期 | 内容 |
|---|---|
| 2026-04-30 | **[doc] 桌面端分发方向讨论 + `desktop-packaging-notes.md` 落地**：明确产品定位为本地优先单人工具，**不做网页部署**；目标分发形态为 **Wails 原生桌面 app**（系统 webview，非 Electron）；集成方式选"Wails 当窗口外壳 + 复用 httpapi"，保留未来网页版退路（不走 Wails 原生 binding）；分发选 dmg/setup.exe/AppImage（v0.1 起即 L3 级），代码签名延后（macOS 公证 $99/年是 v1.0 时最高 ROI 投入）；Python 沙箱短期方案 A（README 写要求系统 Python），中期方案 C（捆绑 python-build-standalone +30-50MB）|
| 2026-05-01 | **[doc] 常驻后台模式 + Notifier 接口决策**：scheduler 不退出（关窗 ≠ 退出）。必做：托盘 / 单实例锁 / 开机自启 / graceful shutdown。Phase 4 写 scheduler 时 `domain/notification/Notifier` 接口先行；桌面壳代码限 `internal/infra/desktop/`，`cmd/server` 不含 Wails。|
| 2026-05-01 | **[doc] 决定不走 Wails binding**（书面记录免未来纠结）：HTTP 写法等价但能复用 v1.2 已写的 transport（middleware/errmap/curl 测试），SSE 天然契合，可演进 CLI/cloud sync。binding 只换"类型同步"一项收益，OpenAPI 也能做到。transport 永远只 httpapi 一份。|
| 2026-05-01 | **[refactor] schema_extras guard 改 GORM Migrator**：`applySchemaExtras` 检查表存在用 `db.Migrator().HasTable()` 替代 raw `sqlite_master` 查询。`schemaExtraGroups` 里真正 GORM 写不出的 SQL（partial UNIQUE 等）仍保留 raw `tx.Exec`。|
| 2026-05-01 | **[refactor] message_blocks 复合索引迁到 GORM tag**：`Block.MessageID + Seq` 加 `index:idx_mb_msg_seq,priority:N`，删 schema_extras 对应 group。复合索引最左前缀已覆盖原单列 index。schema_extras 现仅剩 tools partial UNIQUE 一条。|
| 2026-05-01 | **[cleanup] 死代码清扫**：删 3 个未发布的 `ToolTestCase*` event 类型——SSE 实际走 `toolapp.GenerateEvent` 直接 callback 不经 Bridge。同步 events-design.md 移除 phantom 事件 + 加注 generate-test-cases 端点的特殊形态。|
| 2026-05-01 | **[arch] pagination 迁到 pkg + S13 全代码补别名**：cursor 编解码本是 HTTP 无关纯工具，放 transport 下导致 4 store 反向 import 不便，各自抄了一份私有 encode/decode。搬到 `pkg/pagination`，4 store 改通用版（删 ~64 行）。顺带扩 S13 加 `httpapi` 后缀，全代码补 `*httpapi` 别名。同步 backend-design.md。事故：sed `\b` 清空 main.go，git 恢复改 Edit。|
| 2026-05-01 | **[fix] staticcheck 全套 5 修**：恢复误删的 `ListProviders`/`ListScenarios`（deadcode 默认不扫测试导致误判，加 -test 防再踩）；SA1029 改 `//lint:ignore`（staticcheck 不认 `//nolint`）；S1016 `oaiFuncDef{...}` 改类型转换 `oaiFuncDef(d)`。staticcheck 0 errors。|
| 2026-05-01 | **[fix] _ = err 静默吞错 5 处**：`tool.newID` 加 panic 与其他 4 个 newID 一致；`tool.Import`/`Export` 加 `log.Warn` 不再静默失败；2 处 `w.Write` 加注释保留（HTTP 迟发写错按惯例忽略）。|
| 2026-05-01 | **[review] TODO 扫描**：全代码仅 3 处 TODO，全是合法前瞻性标记（A1 中流执行 / context compaction 钩子点）。无历史包袱。|
| 2026-05-01 | **[refactor] `userID(ctx)` helper 统一到 pkg/reqctx**：5 store + 6 app inline 共 11 处重复（store 私有 helper 还有 `userID` vs `uid` 命名漂移）。新增 `reqctxpkg.ErrMissingUserID` sentinel + `RequireUserID(ctx)` helper，全替换。**事故**：sed 二次清空 apikey store，git 恢复改 Edit。教训：**项目内禁用 sed 改 import / 函数名**。|
| 2026-05-01 | **[review] errmap 完整性反查**：32 个 domain sentinel 全部已映射，零 gap 零死映射 ✅。补登记 `reqctxpkg.ErrMissingUserID` + `cryptoinfra.ErrUnsupportedVersion` 到 errmap（均 500），抑制 "unmapped domain error" 误报。同步 error-codes.md。|
| 2026-05-01 | **[arch] S5 / S6 降级为参考线**：行数当烟雾报警有用，当硬规则会噪音（main.go DI 170 行、parseAnthropicSSE 状态机 83 行、tool.go Service 956 行都是结构上必要的长）。改措辞强调"可读性、人的理解优先于行数"，超长伴随职责模糊才该拆。同步 backend-design.md。|
| 2026-05-01 | **[review] S13 别名全代码验证**：176 处 internal import，0 处无别名 ✅，32 个别名全部规范后缀。S13 100% 合规。|
| 2026-05-01 | **[refactor] 跨 store 共享 Cursor 类型**：4 store 的 `pageCursor` 形状相同但 tool 用 `json:"t"` 与其他 3 个 `json:"c"` 漂移。`pkg/pagination` 加共享 `Cursor` 类型，4 store 删本地副本统一为 `c`。|
| 2026-05-01 | **[doc] V1.2 文档全量校对**：11 份文档反查代码 drift——testend-design 整体重写、backend-design Architecture tree 更新、chat.md `pipeline.go` → `runner.go` 同步、5 份 service-design 去 Eino 残留。契约文档已对齐无需改。|
| 2026-05-01 | **[arch] backend-design.md 规范补完**：把已实践但未成文的约定全部写进 Standards——新增 N6（PUT 幂等返 200）/ D6（schema_extras 幂等 + table guard 模式）/ D7（普通索引走 GORM tag、partial 索引才进 schema_extras）/ S15（ID 统一 `<prefix>_<16hex>` + `rand.Read` panic）/ S16（错误包装 `<pkg>.<Method>: %w` + sentinel 在最里层）/ S17（errmap 单一事实源，含 pkg/infra 跨层 sentinel）；扩 S9 加 detached context 终态写模式；新增 **T 系列测试规范**（T1 命名 / T2 in-memory SQLite / T3 外部依赖环境变量门控 / T4 删导出符号必搜测试）；新增 **开发期工具纪律** 章节（staticcheck 必跑、deadcode -test、lint:ignore 不是 nolint、禁 sed 改 import / 函数名、跨平台编译当烟雾测试）；更新设计原则 #4 措辞（前端是 Wails 桌面 app，不是"暂不跟进"）。|
| 2026-05-01 | **[doc] 创建项目根 `CLAUDE.md` + `backend-design.md` 拆分**：把全部代码规范（设计原则 7 条 / N1-N6 / D1-D7 / E1-E2 / S3-S17 / T1-T4 / §S11-S14 详节 / 开发期工具纪律）从 `backend-design.md` 整体搬到项目根 `CLAUDE.md`——Claude Code 自动加载该文件进 context，规则在每次 session 都在线。`backend-design.md` 退化为"项目说明书"（愿景 / Phase 路线 / Architecture tree / Verification / 非目标），从 649 行缩到 304 行；`CLAUDE.md` 378 行。**关键好处**：单一事实源——规则改一处，避免双份漂移；规则始终在我的 context，不依赖"记得读 backend-design.md"。同步 `backend-design.md` 顶部加指针说明，文档分册结构表加 CLAUDE.md 一行。|

#### Tool 系统大重构（2026-05-02 起，Phase 0-8 计划，Phase 0-7 完成，Phase 8 进行中）

对照 Claude Code 内部机制调研（`adhoc-topic-documents/claude-code-research-documents/`）后认定当前 13 个 tool 的实现"基础设施过于薄"。原计划 7 阶段（0 清理 → 1 大重命名 tool→forge → 2 包嵌套子包 → 3 新接口+forge tool 重写 → 4 文档同步 #1 → 5 重建 system tools → 6 文档同步 #2），中途用户提出"洁癖式重构"目标——让 SSE 事件 + DB schema 都"异常优美"，于是把 Phase 5 重定义为 DB schema 统一、新增 Phase 6 SSE 3-event entity-state 模型、Phase 7 doc sync #2、Phase 8 testend 联调；原 Phase 5 重建 system tools 暂时撤销（用户指示）。

**关键决策**：
- 推流仍用 `bridge.Publish` 直调（不引入 emit 抽象）—— 心智统一优先，"所有推流形态一致"对单人项目认知负担最小
- agent 包改名 `tool`、原 app/tool 改名 `app/forge`（"用户造的 tool"全语义改 Forge：types / tables / IDs / paths / API / LLM-facing 名）
- agent → tool 内允许嵌套子包（forge / filesystem / shell / web）—— §S12 的特殊例外，Phase 4 文档落地
- **Phase 5/6 新决策**：每个 user-facing domain（chat / forge / conversation）一个 SSE 事件类型，载荷 = 该 domain entity 的完整 GET 形状快照；前端按 ID 替换本地拷贝渲染。tool 内嵌套 LLM 流（create_forge / edit_forge 代码生成）由所属 domain 事件承载——属 forge UI 看的就走 `forge` 事件，与 chat 解耦。
- **Phase 5 数据库统一**：forge_run_history + forge_test_history 合并为 forge_executions（kind 区分），加 chat 触发上下文；Forge 加 Pending 计算字段；Message 加 errorCode/errorMessage/updatedAt；ToolResultData 加 errorMsg/elapsedMs；Attachment 加软删 + 表名 chat_attachments → attachments。

| 日期 | 内容 |
|---|---|
| 2026-05-03 | **[devx] 测试包改名/换位 + 与 Service-side bridge 重构汇合**。
- **`backend/internal/e2e/` → `backend/test/`**（扁平），包名 `e2e` → `test`，build tag `e2e` → `pipeline`，文件 `chat_e2e_test.go` → `chat_pipeline_test.go`。理由：内部 e2e 子目录与 `internal/{app,domain,infra}` 这种"生产分层"放一起语义不对——测试基础设施应该有自己的位置；扁平 `backend/test/` 一目了然。"e2e" 名字也偏松（其实没真浏览器没跨服务），改 `pipeline` 抓住"驱真实 stack 全管道"的本质。
- **Makefile 同步**：`make test-e2e` → `make test-pipeline`（路径走 `./test/...`、tag 走 `-tags=pipeline`）；同时把 `make testend` 改名 `make test-console`（`testend` 这词本身是 test+end，过去含义不清；`test-console` 准确说明它启 dev server + 开浏览器进控制台）。`testend/` **目录名保留**——HTML 资源路径 + collections 文件路径都基于此，改名连锁太大。
- **顺手汇合 Service-side bridge 重构**：与 user/linter 分支的"把 forge tools 的 bridge 字段集中到 Service"refactor 合流——`forgeapp.NewService` 现在多收一个 `eventsdomain.Bridge` 参数；`forgeapp.Service.PublishSnapshot(ctx, convID, *Forge)` 是 forge entity-state SSE 的统一发布点（bridge 为 nil 或 convID 为空时 no-op，让 HTTP CRUD 调用方传 nil 也安全）。`CreateForge` / `EditForge` 工具不再持有 `bridge` 字段，全用 `t.svc.PublishSnapshot()`。`forgetool.ForgeTools` 工厂去掉 bridge 参数。这与 chat 域的 `runner.publishMessageSnapshot` 是同一模式——"每个 domain 的 SSE 由该 domain 的 Service 集中发"，不再让 tool 直接 `bridge.Publish`。
- **校验**：unit 22 包绿、pipeline 6 测试绿（11s）、staticcheck 0（默认 + pipeline tag 双跑）。Makefile 现 5 个测试相关 target（test / test-integration / test-all / test-pipeline / test-console）。|
| 2026-05-03 | **[test] Step 3 完成：chat 真实端到端 5 个场景全绿**。新建 `backend/test/chat_pipeline_test.go`，5 个场景全部一次跑通，**总耗时 ~11s**：
- **`TestChat_SimpleText_StreamingSnapshots`**（0.56s）— 发"Reply with one short word"，验证：(a) 多帧 chat.message 快照中 text 内容**单调生长**（entity-state 模型严格超集要求）；(b) 最终 status=completed + stopReason=end_turn；(c) DB 里 `messages` 行 id 与 SSE 最终快照一致、status=completed、error_code=空。
- **`TestChat_MissingModelConfig_ErrorCodePersisted`**（0.03s，零 LLM 调用）— 故意不 SeedDeepSeek 直接发消息；验证：(a) status=error + ErrorCode=`MODEL_NOT_CONFIGURED` + ErrorMessage 非空；(b) **DB 里 stub 行真的写下来了**（之前 pre-LLM 错误只飞 SSE 不落库，Phase 6 加的 emitFatalError 现在写 stub message）。这是 Step 1 摸排里防御性吞错改完后第一个有意义的 e2e 验证。
- **`TestChat_ToolCall_SearchForges`**（4.01s）— 先 NewForge 一个 parse_csv，让 LLM 调 search_forges；验证：(a) 最终 blocks 含 search_forges 的 tool_call 块；(b) tool_call_id 配对的 tool_result 块出现且 ok=true；(c) **forge_executions 表为空**——search_forges 是只读，不应写执行行（如果走错了路径会被这个反向断言抓到）。
- **`TestChat_CancelMidStream_StatusCancelled`**（0.95s）— 让 LLM 写"200-word essay about rivers"；等第一帧 streaming + text 非空（确认已收到 token）；DELETE /stream；验证：(a) 终态 status=cancelled + stopReason=cancelled；(b) **DB 也是 cancelled**——这是对 §S9 detached-context 模式的真实验证（流被取消后终态写入仍能完成）。
- **`TestChat_ReasoningModel_BlocksSeparate`**（5.01s）— 切换到 deepseek-reasoner，问 19*23 推理步骤；验证：(a) 最终 blocks 同时含 `reasoning` 和 `text` 两类（DeepSeek-R1 reasoning_content 流到 reasoning block，answer 流到 text block）；(b) text 包含 "437"。reasoner 偶尔不可用时 skip 而非 fail。
- **新增 helper `extractTextFromBlocks`** 抽出 block 切片里所有 text 的拼接——多个测试复用。
- **校验**：6 个 e2e 测试全 PASS（含原 smoke）+ `make test` 22 包绿 + staticcheck 0（默认 + e2e tag 双跑）。
- **本步暴露的 API 行为细节**（之前仅靠单测+读代码判断的，现在用真 wire 确认）：(1) chat.message 快照单调生长可作为契约；(2) ToolCallID 在 tool_call/tool_result block 间稳定配对；(3) cancel 路径的 stub 终态写入靠 detached context 真的工作；(4) deepseek-reasoner 的 reasoning_content 真的走 reasoning block 而非 text。|
| 2026-05-02 | **[test] Step 2 完成：E2E 测试 harness 落地**。新建 `backend/internal/e2e/`（`//go:build e2e` 门控，默认 `go test ./...` 不进），3 文件：
- **`harness.go`** — `Harness.New(t)` 装配与 `cmd/server/main.go` 一致的 DI 图（apikey / model / conversation / forge / chat services + sandboxinfra(`python3`) + memoryinfra.Bridge + llminfra.Factory），底层用 in-memory SQLite + httptest.NewServer。暴露 DB / Bridge / 5 个 service 直接拿、URL() 拿 base URL、PostJSON / GetJSON / PatchJSON / Delete 4 个 HTTP helper（非 2xx 直接 fail，传入 `out` 自动 decode）、HTTPClient() 短连接 client、RequireDeepSeekKey(t) 缺 key 时 skip 而非中途失败。crypto 用确定性测试指纹 `forgify-e2e-test-fingerprint` 让同一 harness 实例能解开自己加密的 key。Cleanup 全部注册到 t.Cleanup。
- **`seed.go`** — `LocalCtx()` 返回打了 DefaultLocalUserID 的 ctx（绕 HTTP 直调 service 用）；`SeedDeepSeek(t, key)` 一行塞 api_key + chat scenario model config（key 留空时从 DEEPSEEK_API_KEY 取）；`NewConversation(t, title)` / `NewForge(t, name, code)` 直调 service 拿 entity。`ProviderDeepSeek = "deepseek"` 常量在此 e2e 包内（domain 把 provider 当自由字符串，e2e 测试需要稳定值）。
- **`sse.go`** — `SubscribeSSE(t, conversationID)` 开 GET /api/v1/events 长连接，`SSESub` goroutine 解 text/event-stream 格式（处理 `id:/event:/data:` + 空行边界 + `:` keep-alive）+ 按 entity-state 模型整理：`messages map[id]*Message` / `forges map[id]*Forge` / `conv *Conversation` 各 keyed by id 替换。访问器：AllMessages / LastMessage / MessageByID / AllForges / LastForge / Conversation / RawEvents。等待器：`WaitForMessage(predicate, timeout)` / `WaitForMessageStatus(id, status, timeout)` / `WaitForAssistantTerminal(timeout)`（事先不知道服务端分配的 msgID 时用，匹配任意 assistant 消息进 completed/error/cancelled 终态）/ `WaitForForge(predicate, timeout)`。`FormatRawEvents()` 给断言失败时输出"实际 wire 上发了什么"。SSE 长连接用 no-timeout `http.Client`（30s 超时会切断）+ ctx.Cancel 作 kill switch；t.Cleanup 注册自动 Close。
- **Makefile 加 `make test-e2e`** target，自动 source .env + `-tags=e2e`，仅跑 `./internal/e2e/...`。
- **Smoke 测试 `harness_smoke_test.go`** — 启动 + SeedDeepSeek + NewConversation + SubscribeSSE + POST 一条短消息 + WaitForAssistantTerminal(60s) + 断言 status=completed / 至少 1 个 text block / token 计数已填 / 至少 2 帧 chat.message 快照（流式中间状态）。**首次跑通 680ms**：4 帧快照、1 个 text block、in=1393 out=3 tokens。Step 3-5 有可靠地基。
- **校验**：`make test` 22 包绿、`make test-all` 22 包绿、`make test-e2e` smoke pass、staticcheck 默认 + `-tags=e2e` 双跑 0 warnings。harness 不影响生产代码（只读 service 接口、不改 entity / endpoint / 契约）。|
| 2026-05-02 | **[fix] Step 1 完成：防御性代码大摸排**。用户原话："过去你为了过测试把代码写得防御性拉满，错误就直接吞掉"——本轮系统扫描全 backend 的 `_ = err` / 静默 fallback / 错误返 nil 等可疑模式，逐处分类（合法 vs 该 surface），把吞错的位置改回真报错。**修了 6 处真问题 + 2 处加 log warn + 1 处真 flake**：
- **`forgeapp.attachPending` 改返 error**——之前 DB 真错误时 `f.Pending = nil`，与 "no pending" 无法区分，让 GET /forges/{id} 在 SQLite 抖动时撒谎。`Get` / `GetDetail` / `List` 三个调用点全跟改。
- **`forgeapp.GetDetail` 4 处 `, _ :=` 改返错**——ListTestCases / ListExecutions 错误以前静默吞，TestSummary 显示 "0 cases" 看起来合法，实则 DB 故障。
- **`forgeapp.recordExecution` 签名 `*Execution` → `(*Execution, error)`**——SaveExecution 失败以前只 log warn 但仍返已构造 entity，调用者拿到一个**没落库的"假"行**（RunTestCase handler 当真返 200 给 HTTP）。retention 的 CountExecutions / DeleteOldestExecution 失败也由 `_ =` 吞掉（表能涨过 cap），改为 log warn 不让本次失败但留 tracability。RunForge / RunTestCase 都跟改 propagate error。
- **`forgeapp.RunTestCase` InputData 解析改 fail loudly**——以前 `_ = json.Unmarshal` 让坏 JSON 静默变空 map，sandbox 跑空 input 拿假 result 进对照，测试通过原因完全错位。改 surface：损坏的测试用例直接报错。
- **`apikeyapp.Service.Test` 失败时 UpdateTestResult 加 log warn**——以前 `_ = s.repo.UpdateTestResult(ErrorState, ...)` 写库再失败，DB test_status 维持原值，UI 还显示 "ok" 实际已坏却无线索可追。
- **`toolapp.injectStandardFields` 必填字段坏 schema 改 panic**——以前 `_ = json.Unmarshal(raw, &required)` 静默继续，工具作者写坏 required 数组会被悄悄吞掉、LLM 跳过必填项。改 panic 与同函数 191/196/200 行的 panic-on-bad-schema 策略一致。
- **`infra/llm/anthropic.go` 历史 tool-call args 改 log warn + `{}` fallback**——以前 `_ = json.Unmarshal(...)` 让坏 JSON 静默变 nil，Anthropic API 收到 nil tool input 行为不可控。改：empty 直接走 `{}`；坏 JSON log warn + fallback `{}` 满足 schema 要求且留 trace。
- **`toolapp.StripStandardFields` 留两处刻意 silent + 解释注释**——LLM 产出 args 类型错（如 summary=int）走下游 ValidateInput 自然拒绝（会以 retry signal 回 LLM），那才是真正暴露面。注释明确说"刻意不返错不打日志"避免后人误改。
- **flake fix: `app/conversation/TestRename_Success`**——`time.Now()` 在快机上微秒级，Create + Rename 命中同一 tick 时 `updated.UpdatedAt.After(c.UpdatedAt)` 严格大于会假阳性失败（实测 1/5 概率）。改用 `!Before`（"时间戳没回退"才是真正的语义）；20/20 验证消失。
- **判定为合法保留的 9 处**：HTTP `w.Write` / `json.NewEncoder.Encode` 在 status 已发后（无可挽回）；`db.Close` on error path（教科书可忽略）；`middleware/recover.go` 的 recover；`logger/broadcast.go:164` 的 best-effort never-block-logger（设计如此）；`extractTextContent` 的 `if json.Unmarshal == nil`（兜底取 last text）；LLM-origin `map[string]any` 的 `json.Marshal, _ =`（来源已是 JSON 不可能失败）。
**校验**：22/23 包测试绿（5 LLM 集成测试现在通过 .env 加载 DEEPSEEK_API_KEY）+ staticcheck 0 + conversation flake 20/20 不复现。**.env 注入机制**：根目录 `.env`（gitignored）+ `.env.example` 模板（committed）+ Makefile 三 targets（`make test` 跳过集成 / `make test-integration` 单测集成 / `make test-all` 全测自动 source .env）。|
| 2026-05-02 | **[doc] Phase 7 完成：文档同步 #2（Phase 5-6 全量同步）**。8 份文档跟齐 Phase 5 + 6 改造：
- **events-design.md** Phase 2 + 3 共 12 条事件清单整段重写为 entity-state 模型 3 事件表（`chat.message` / `forge` / `conversation`），加配套实现细节（embedded *Entity 指针 + 自定义 MarshalJSON 让 wire shape = REST GET）+ 已删 12 个旧事件清单
- **api-design.md** forge 端点表删 `run-history` / `test-history` 两行，加统一 `executions` 一行；备注 GET /forges/{id} 响应含 `pending` 字段
- **database-design.md** 改 `chat_attachments` → `attachments` + 加软删；messages 加 `error_code/error_message/updated_at`；message_blocks 的 tool_result JSON shape 加 `errorMsg/elapsedMs`；`tools` 表名改回 `forges` + 加 `Pending` 计算字段说明；forge_versions 字段 `message` → `change_reason`；删 forge_run_history + forge_test_history，新增 forge_executions（含 2 个复合索引说明）；跨表关系图全量更新
- **error-codes.md** chat 域错误码表后新增 "Message.errorCode 字段值" 子表，列 6 个：MODEL_NOT_CONFIGURED / API_KEY_PROVIDER_NOT_FOUND / LLM_PROVIDER_ERROR / LLM_STREAM_ERROR / HISTORY_EXTEND_FAILED / INTERNAL_ERROR——这些是 SSE chat.message 携带、不走 HTTP 路径的错误代码
- **service-design/forge.md** §3.1 Tool struct 改名 Forge + 加 Pending 计算字段；§3.2 ForgeVersion 字段 ToolID/Message → ForgeID/ChangeReason；§3.4 + 3.5 ForgeRunHistory + ForgeTestHistory 整段删除，新写 §3.4 ForgeExecution（含触发上下文 4 字段 + 2 复合索引说明）；§4 常量加 ExecutionKind* / TriggeredBy* + 删 MaxRun/TestHistory 常量；§5 sentinel 错误前缀 tool→forge；§6 Repository 接口 9 个 history 方法合并为 4 + 加 ExecutionFilter 定义；§11 HTTP 端点表 22→21（合并 history 端点）；§13 SSE 事件章节整段重写为 entity-state Forge 事件 + 7 个触发点表 + 失败语义说明
- **service-design/chat.md** §6.1 messages 加 ErrorCode/ErrorMessage/UpdatedAt + 说明走 SSE chat.message；§6.2 tool_result JSON shape 加 errorMsg/elapsedMs；§6.4 chat_attachments → attachments + 加 UpdatedAt + DeletedAt 软删 + 解释；§8 SSE 事件章节整段重写为 chat.message entity-state 模型（传输示意图 + ChatMessage struct + 3 个 helper publishMessageSnapshot/writeAndPublish/emitFatalError + 7 个触发场景表 + 旧 7 个事件 → 新字段对照表）
- **service-design/conversation.md** §9 事件章节重写为 entity-state Conversation 事件
- **progress-record.md** 本条 |
| 2026-05-02 | **[refactor] Phase 6 完成：SSE 12 事件 → 3 entity-state 事件**。
**核心决策**：每个 user-facing domain 一个事件，载荷 = 该 domain entity 的 GET 形状完整快照——前端按 entity ID 替换本地拷贝即可渲染，无需追 token / 合 delta / 分 12 种事件形状。chat 界面 UI 不关心 tool 内部流；forge 面板 UI 关心 create_forge / edit_forge 的代码生成流。所以 forge tool 内部代码 LLM 流走 `forge` 事件，不走 `chat.message`。
**3 个事件**：
- `chat.message` — 载荷 = 完整 Message（含 blocks / status / stopReason / errorCode / errorMessage / 三 token 计数 / updatedAt）。触发：message slot 创建 / 每个 LLM token / tool_call 出现 / args 完整 / 每个 tool result 完成（mutex 守护并行）/ 终态 / pre-LLM 失败 stub。
- `forge` — 载荷 = 完整 Forge（含 pending 子对象）。触发：create_forge 进入预 stub + 逐 token 流 + 末尾定型；edit_forge 进入预 draft pending + 逐 token 流 + 末尾定型；仅元数据路径单帧最终快照。
- `conversation` — 载荷 = 完整 Conversation。触发：auto-titling 完成。
**Event struct**：每个嵌入 `*<entity>.Entity` 指针 + 自定义 MarshalJSON 委托给嵌入指针——wire shape 直接是 entity 字段（无 wrapper key）。
**chat 层是唯一发布事实源**：`runner.go` 三 helper（`publishMessageSnapshot` 仅 SSE / `writeAndPublish` 写库 + SSE / `emitFatalError` stub error message）。`stream.go` + `tools.go` 通过 closure 调它们，绝不自己 `bridge.Publish`。
**forge tool 预分配 ID 模式**：`forgeapp.NewForgeID()` / `forgeapp.NewVersionID()` 公开 helper；CreateInput / PendingSnapshot 加 ID 字段；create_forge 先建内存 stub Forge（带预分配 ID），逐 token 更新 stub.Code 并发快照，末尾走 svc.Create(ID=...) 才真正落库。失败干净丢弃 draft 不污染 DB；订阅方观察到的最后一帧是错误前部分快照，前端可按"创建/编辑失败"语义清理。edit_forge 同理但目标是 Pending 子对象。
**chat 错误模型升级**：pre-LLM 失败（MODEL_NOT_CONFIGURED / API_KEY_PROVIDER_NOT_FOUND / LLM_PROVIDER_ERROR）也 emit stub assistant message + chat.message 快照——所有 chat 错误现都装进 chat.message 里，UI 一处处理。新增 errorCode：LLM_STREAM_ERROR / HISTORY_EXTEND_FAILED / INTERNAL_ERROR。
**runTools 并发安全**：parallel batch 内多 goroutine 写 blocks 切片不同索引；mutex 守护 publishProgress 读 + 构造快照；不阻塞执行。
**parseToolArgs 保留旧 tuple 签名**（无谓 API 变化已回滚——本意是顺手清理，但破坏既有测试）。streamLLM 加 errMsg 出参承载上游错误文本；agentRun 据此回填 Message.ErrorCode/ErrorMessage。bridge_test.go sampleToken 改用 ChatMessage 类型（绕过删除的 ChatToken）。
12 个旧事件全部删除：chat.{token,reasoning_token,tool_call_start,tool_call,tool_result,done,error}、conversation.title_updated、forge.{code_streaming,created,pending_created,metadata_updated}。`go build` ✅ / `staticcheck` 0 ✅ / 22/23 包测试绿（基线 5 个 LLM 集成测试除外）。|
| 2026-05-02 | **[refactor] Phase 5 完成：DB schema 异常优美重构**。用户提出"洁癖式重构"——希望 backend 数据库结构能让"什么都看起来异常优美"。本阶段处理 4 个领域：forge 历史合并、forge entity pending 字段、message 错误信息字段、attachment 软删 + 表改名。
**1. ForgeRunHistory + ForgeTestHistory → ForgeExecution（统一表）**：
- 新表 `forge_executions`（id `fe_<16hex>`），用 `kind` 字段区分 `"run"` / `"test"`；test 专属字段（test_case_id / batch_id / pass *bool）在 run 行可空。
- 新增 chat 触发上下文 4 字段：`triggered_by` ("chat"/"http") + `conversation_id` + `message_id` + `tool_call_id`——LLM 在 chat 中调 run_forge，行可从 chat 消息追溯。
- 复合索引 2 个：`idx_fe_forge_created (forge_id, created_at)` 单 forge 历史按时间倒序检索；`idx_fe_msg (conversation_id, message_id)` chat 消息触发的所有 forge 调用追溯。
- Repository 9 方法（SaveRun/Test、ListRun/Test、ListTestByBatch、CountRun/Test、DeleteOldestRun/Test）合并为 4 方法（SaveExecution、ListExecutions、CountExecutions、DeleteOldestExecution）。`ExecutionFilter` struct 支持 forge / kind / batch_id / test_case_id / chat 上下文 / cursor / limit 任意组合。
- `ListExecutions` 用 cursor 分页（与项目其他列表一致用 paginationpkg.Cursor 元组）；BatchID 设置时反转排序为 ASC（单批次按运行顺序展示）。
- HTTP 端点 `GET /api/v1/forges/{id}/run-history` + `/test-history` 删除，统一为 `GET /forges/{id}/executions?kind=&batchId=&cursor=&limit=` 分页 envelope。
- 常量 `MaxRunHistoryPerForge=100` + `MaxTestHistoryPerForge=200` 合并为 `MaxExecutionsPerForge=300`。
**2. Forge.Pending 计算字段**：
- 加 `Pending *ForgeVersion (gorm:"-")` 字段，由 service 层 `attachPending` 在 GET / List 后填充。SSE `forge` 事件载荷由此机制承载 pending 状态——entity-state 模型的关键支撑。
- `ForgeVersion.Message` 字段改名 `ChangeReason`（更准确表达"变更意图"语义；JSON tag 跟改）。
**3. Message 加错误信息字段**：
- 加 `ErrorCode` + `ErrorMessage`（仅 status="error" 时填）+ `UpdatedAt`（GORM 自动）。runner.go 在 streamLLM EventError / extendHistory 失败 / writeDB fatal 失败时填这两字段，让 SSE chat.message 快照能携带结构化失败原因。
- ToolResultData 加 `ErrorMsg`（仅 ok=false 时）+ `ElapsedMs`（wall time）。runOneTool 用 time.Now() 起止计时；executeTool 签名加 errMsg 出参；ValidateInput / CheckPermissions / Execute 三类失败各自填 errMsg。
**4. Attachment 软删 + 改名**：
- 表名 `chat_attachments` → `attachments`（去掉冗余 domain 前缀）。
- 加 `UpdatedAt` + `DeletedAt` 软删——用户删附件后旧对话仍持 attachment_ref block，软删保留行让解引用不变 dangling。
**Service 行为更新**：
- `Service.RunForge` / `RunTestCase` / `RunAllTests` 通过 `recordExecution` helper 写 ForgeExecution——从 ctx 读 conversation/message/toolCallID 自动填触发上下文，无 ctx 时 triggered_by="http"。
- `Service.attachPending` 在 Get / List / GetDetail 后填 forge.Pending；ErrPendingNotFound 静默忽略，其他错误 log warn。
- `Service.GetDetail` 用 ListExecutions(Kind="test", Limit=1) 找最近 batch；再用 BatchID 过滤拉全 batch（替代旧 ListTestHistory(200) 暴力扫描模式）。
**测试**：infra/store/forge/forge_test.go 整体重写覆盖 ForgeExecution + ExecutionFilter + cursor 分页（5 个新测试覆盖 retention / kind+batch 过滤 / chat ctx 过滤 / 3 页 cursor walk）。`go build` ✅ / `staticcheck` 0 ✅ / 22/23 包测试绿（基线 5 个 LLM 集成测试除外）。|
| 2026-05-02 | **[refactor] Phase 0 完成：清理过时 tool**。删除 `app/agent/system.go`（read_file / write_file / list_dir / run_shell / run_python / datetime 6 件）+ `app/agent/web.go`（web_search / fetch_url 2 件）+ `system_test.go`，共 3 文件 ~600 行。`main.go` DI 拆掉 `SystemTools()` / `WebTools()` 注册。chat 暂时只剩 5 个 forge tool（search/get/create/edit/run）—— Phase 5 重建新一代 system tools（Edit + 持久 cwd Bash + Glob/Grep + 先读才能写约束等）。|
| 2026-05-02 | **[refactor] Phase 0 完成：GenerateTestCases 去流改普通 HTTP**。`POST /api/v1/tools/{id}:generate-test-cases` 端点从 SSE 改 200 JSON envelope。理由：底层 `llm.Generate(ctx, prompt)` 是非流式调用，等完整响应后再 for loop emit "test_case" → "done"，所谓"流"完全是化妆，徒增 SSE plumbing 复杂度。改动：`toolapp.GenerateEvent` struct 删除，新增 `GenerateResult{NotSupported, Reason, TestCases}`；service 签名 `GenerateTestCases(ctx, toolID, count) (*GenerateResult, error)`；handler 删 SSE plumbing 改 `responsehttpapi.Success(200, result)`。同步 `events-design.md`（删"不走 Bridge 的特例"段）/ `api-design.md` / `tool.md` 6 处（决策表 / GenerateResult struct / 函数签名 / API 表 / 已废 3 个 SSE 事件类型 / Chain 6 调用链）/ `domain/events/types.go` 删过时注释。`go build` ✅ / `staticcheck` 0 ✅ / 测试除基线 5 个 LLM 集成测试外全绿。|
| 2026-05-03 | **[refactor] 重复实现摸排 + 8 项整改**。摸排 16 个候选位置后认定 9 处真重复实现，本轮修复其中 8 项（第 9 项 `pkg/pyrun` 被并行的 sandbox iteration A 大改造吞掉，取消）：(1) 新建 **`internal/pkg/idgen`**（`New(prefix) string`）—— 5 个 domain 各自的 newID（apikey/model/conversation/forge/chat）的核心 5 行（crypto/rand → panic on err → hex encode → prefix）合一；chat 的 `randHex(8)` 一并清掉；(2) **`chat/stream.go::parseToolArgs` 改调 `toolapp.StripStandardFields`**——原副本类型不严（`m["summary"].(string)` 类型不对就丢值），canonical 用 `_ = json.Unmarshal(raw, &v)` 容错；保留 chat-side "JSON 损坏塞 args[\"raw\"]" 兜底；(3) 新建 **`internal/pkg/llmparse`**（`ExtractJSON` + `IsLikelyJSON`）—— `app/forge/forge.go::extractJSONFromLLM`（无验证、bracket 容易误切）和 `app/tool/forge/forge.go::extractJSON`（带 isLikelyJSON 验证）合并为单一较严版本；(4) 新建 **`handlers/util.go::idAndAction`** helper —— apikey/forge handler 共 3 处 `strings.Cut(idAction, ":")` 收敛；(5) **`apikey/tester.go::parseGoogleModels` + `parseOllamaModels` 合并为 `parseModelsByName`**——两个 200-字符函数逐字相同，注释明示意图但放弃"future drift"假设（真漂移再拆，YAGNI）；(6) **`forgeapp.Service.PublishSnapshot(ctx,convID,*Forge)` helper**——6 处 `t.bridge.Publish(ctx, convID, eventsdomain.Forge{...})` 收敛；`forgeapp.NewService` 增 `bridge` 参数；`forgetool.ForgeTools` 工厂签名 **去掉** bridge 参数（tools 通过 `t.svc.PublishSnapshot` 间接发布，与 chat.runner.publishMessageSnapshot 同模式）；CreateForge / EditForge struct 字段 `bridge` 移除；(7) **`response.StreamSSE[T]` 泛型 helper**（`sse.go`）—— chat.EventsSSE + dev.StreamLogs 两处 SSE 样板（4 个 header / 15s ticker / ctx.Done 退出）收敛，dev.StreamLogs 通过 `onPrelude` 钩子做 ring 回放；(8) 新建 **`internal/pkg/llmclient`**（`Resolve(ctx, picker, keys, factory) (*Bundle, error)` + 3 sentinel: `ErrPickModel` / `ErrResolveCreds` / `ErrBuildClient`）—— picker.PickForChat → keys.ResolveCredentials → factory.Build 三段舞 5 处收敛（chat.runner.processTask 用 sentinel 区分 MODEL_NOT_CONFIGURED / API_KEY_PROVIDER_NOT_FOUND / LLM_PROVIDER_ERROR；chat.runner.autoTitle、tool/forge/streamCode、search_forges、main.go forgeLLMClientAdapter、test/harness 都改用 Resolve）；删除 `tool/forge/forge.go` 内的 `builtClient` / `buildClient` / `newRequest` 三个本地 helper。**取消的项**：原计划 `pkg/pyrun` 抽 sandbox.Run + ast.go::parseForgeCode 共享的 python subprocess 启动样板；用户在并行做 sandbox iteration A（`python.go` 删除，新建 `sandbox.go` / `paths.go` / `pyproject.go`，新 API 接 `Config{DataDir,UVPath,PythonPath,DefaultPython,Logger}` + 捆绑 uv + per-version venv），整个抽象被新 sandbox 包揽——pyrun helper 在新模型下没有独立存在的必要。**测试**：`go test -count=1 ./internal/pkg/... ./internal/app/... ./internal/transport/... ./internal/infra/...` 全绿（基线 5 个 LLM 集成测试除外）；`staticcheck ./internal/...` 零警告。**当前 build 状态**：`cmd/server/main.go` + `test/harness.go` 因 sandbox iteration A in-flight（`*sandbox.Sandbox` 还未实现 `forgeapp.Sandbox` 接口的 Run 方法 / `sandboxinfra.New` 签名从 `string` 改为 `Config`）暂时无法整体编译——本轮 8 项改动与 sandbox 互不耦合，待 sandbox iteration A 收尾后整体绿。|
| 2026-05-02 | **[fix] testend 前端跟齐 Phase 0-3 重命名 + 新事件 + destructive UI**。Phase 1 perl 只批量改了 API 路径和 LLM-facing 工具名，前端 JS / HTML / CSS 还有大量漏网。本轮全清：(1) `tab-sse.js` forge view 完全失灵——订阅列表 `tool.code_streaming/tool.created/tool.pending_created` → `forge.*`，3 个 handler 同步改名，事件 payload 字段 `data.toolId/toolName` → `data.forgeId/forgeName`，新增 `forge.metadata_updated` handler（仅元数据 pending 路径）；(2) `tab-tools.js::generateTestCases` 整段重写——Phase 0 把端点从 SSE 改成 200 JSON envelope，前端还在解 `tool.test_case_generated/tool.test_cases_not_supported`（俩事件已删），改成 `fetch().then(r=>r.json())` 拿 `result.testCases`；(3) `tab-sql.js` 5 个 quick query + clickCell 全引旧表名 `tools/tool_versions/...` 和列名 `tool_id/tool_version` + ID 前缀 `t_`，全改 `forges/forge_versions/forge_id/forge_version/f_`，clickCell 加 `f_/fv_/frh_/fth_` 4 个新前缀分支；(4) `collections/11-full-e2e-workflow.yaml:121` 把"use the datetime tool" prompt 换成"search forge library"——`datetime` 是 Phase 0 删的；(5) collections 04/05 YAML 变量名 `toolId` → `forgeId`（cosmetic 一致性，46 处）；(6) destructive UI 落地——Phase 3 给 `chat.tool_call` SSE 加了 destructive 字段但前端没读：`chat.js`/`tab-sse.js` 两处 handler 接 `d.destructive`，`tester.html` 两处 tool step 模板加 `<span class="destructive-badge">⚠ destructive</span>`，`style.css` 加红色徽章样式；(7) `style.css` SSE event class `tool-code_streaming/tool-created/...` → `forge-*`。**保留**：`toolName/toolCallId` 字段名（指 LLM tool-call 概念，不是 user-forge），CSS 类 `tool-step-*`（UI 元素名）。后端测试不受影响（22/23 包绿）。|
| 2026-05-02 | **[doc] Phase 4 完成：文档同步 #1**。Phase 0-3 大改造的文档跟齐，覆盖 6 个文件：
- **CLAUDE.md** §S15 ID 前缀清单从 `t_/tv_/...` 改 `f_/fv_/...`（Phase 1 漏改）；新增 §S18 Tool 接口规约（10 方法 / 标准字段注入 summary+destructive / runTools 分批语义 / 子包结构 / 钩子链 / 完整 Search 例子）；§S 系列索引加 S18 一行
- **events-design.md** ChatToolCall 加 `destructive` 字段说明；新增 `forge.metadata_updated` 事件行；ForgeCreated/PendingCreated 字段名 `toolId/toolName` → `forgeId/forgeName` 跟齐
- **database-design.md** message_blocks tool_call 类型 JSON shape 加 `destructive` 字段说明
- **api-design.md** "tool" 小节标题改 "forge"
- **backend-design.md** Architecture tree 全量更新：domain/forge、app/forge、app/tool（含 forge/filesystem/shell/web 嵌套子包）、infra/store/forge、pkg/reqctx 三文件、handler 改 forge；"Tool 接口"行注明 10 方法 + §S18；Phase 3 介绍段加 Phase 3 后优化轮总结块
- **service-design/forge.md** 标题 tool→forge v3；System Tool 位置 / SearchForge 命名 / RunForge resolveAttachments 等多处更新；§1 决策表加 destructive per-call / AST dry-run / 50KB 截断 三条；§10 章节大改：5 文件子包结构 + ForgeTools 工厂签名 + 标准注入/钩子链总览 + Tool 元数据表 + 5 个 tool 流程更新（含 ParseCode dry-run / forge.metadata_updated 路径 / 输出截断）
- **service-design/chat.md** §4 Tool 接口章节大改：10 方法签名 / injectStandardFields/StripStandardFields 注入剥离 / context helpers 已搬到 pkg/reqctx；§4.4 system tools 表去掉 8 个旧 tool（Phase 0 已删），保留 5 个 forge tool 跟 tool/forge/ 子包对齐；§5.4 runTools 章节重写："一律全并行" 改 "按 IsConcurrencySafe 分批" 算法 + 钩子链 ValidateInput/CheckPermissions/Execute |
| 2026-05-02 | **[refactor] Phase 3 完成：新 Tool 接口 + forge tool 重写 + 顺手修一票小问题**。

**核心**：Tool 接口从 4 方法扩到 10 方法（全必填，无 BaseTool）：3 静态元字段（`IsReadOnly` / `NeedsReadFirst` / `RequiresWorkspace`）+ 3 args-dependent 钩子（`IsConcurrencySafe` / `ValidateInput` / `CheckPermissions`）+ 4 原有方法。配套 `PermissionMode` / `PermissionResult` 类型。

**关键决策：destructive 走 per-call AI 自报 + 存 DB**。同 `summary` 注入模式：`injectStandardFields` 在每个 tool 的 Parameters schema 注入 `summary`（必填）和 `destructive`（可选默认 false）；`StripStandardFields` 剥两个字段；`chatdomain.ToolCallData` 加 `Destructive bool` 一等字段；SSE `ChatToolCall` 事件加 `Destructive` 让前端实时显示警示徽章。LLM 知道这次调用是否危险，UI 据此提示用户——比静态 IsDestructive() 更精准（同一 tool 不同 args 可不同）。

**5 个 forge tool 移到 `tool/forge/` 子包**（一文件一 tool）：`forge.go`（工厂 + 共享 helpers）+ `search.go` / `get.go` / `create.go` / `edit.go` / `run.go`。命名 Style B 保留显式 `SearchForge` / `GetForge` / `CreateForge` / `EditForge` / `RunForge` 后缀。每个实现完整 10 方法（IsReadOnly: Search/Get=true，Create/Edit/Run=false；其余字段全用同名默认零值）。删除老 `tool/forge.go`。

**reqctx 包重组**：`userid.go` 改名 `reqctx.go`（§S12 主文件用包名）；新增 `agentrun.go` 装 3 个 agent-run 标识符 helpers（`WithConversationID/MessageID/ToolCallID`）+ 配套 `agentrun_test.go`；package doc 扩写涵盖三类（user identity / locale / agent-run IDs）。这套 ctx helpers 此前在 `agent/forge.go` 里、Phase 2 后挪到 `tool/forge.go`、Phase 3 一度临时放在 `tool/context.go`，最终落点 `pkg/reqctx`——理由：所有"塞 ctx 的标识符"集中一个包，新加 ID 有明确归属。

**runTools 改"按 IsConcurrencySafe 分批"**（`chat/tools.go`）：相邻 safe 调用合并并行 batch，non-safe 调用各自独立串行。每次调用前跑 `ValidateInput` + `CheckPermissions(mode=Default)`；返 Deny 直接转失败 tool_result，Ask 当前阶段当 Allow 处理（Phase 4+ scheduler 才接真审批）。`partitionByConcurrencySafety` 算法：相邻 safe 合并；unsafe 边界强制起新 batch。例：`[safe,safe,unsafe,safe,unsafe]` → `[B1: 2 safe // 并行] [B2: 1 unsafe // 串行] [B3: 1 safe // 单跑] [B4: 1 unsafe // 串行]`。

**顺手修小问题**：(1) Description / 错误 wrap / prompt 里残留 "tool" / "t_xxx" 字面量全清——LLM 看到的字段名跟 Phase 1 重命名对齐。(2) SearchForge 输出字段 `similarity` → `score`（避免误导 LLM 以为是向量 cosine）。(3) `extractJSON` 加 markdown fence 优先（` ```json ... ``` `）+ `isLikelyJSON` 验证候选，bracket fallback；防 LLM 散文中夹方括号导致取错段。(4) EditForge 仅元数据变更（无 Instruction）发新事件 `forge.metadata_updated`，UI 区分"代码重生" vs "静默元数据"。(5) CreateForge/EditForge 在 `streamCode` 后 `forgeapp.ParseCode` dry-run AST，失败立刻返 LLM 重试信号，不进 `svc.Create` 存储 I/O——给 LLM 干净的错误路径。(6) RunForge 输出 50KB 截断（`maxOutputBytes`）防失控 forge 撑爆 LLM context，超限替换为 notice 字符串而非裁剪。

**新增事件类型**：`ForgeMetadataUpdated`（`forge.metadata_updated`）；`ChatToolCall.Destructive` 字段。

**API 变化**：`forgeapp.Service.ParseCode(code) error` 公开方法，供 forge tool 调 dry-run。

**测试**：`tool/tool_test.go` 全量重写覆盖新接口（injectStandardFields / StripStandardFields / 含 destructive 的各 case）；新 `agentrun_test.go` 12 个 case 覆盖 3 个 ID helpers + key isolation；`stream_test.go::parseToolArgs` 测试加 destructive 维度。`go build` ✅ / `staticcheck` 0 ✅ / 22/23 包测试绿（基线 5 个 LLM 集成测试除外）。|
| 2026-05-02 | **[arch] Phase 2 完成：agent/ → tool/ 包重组 + S12/S13 例外条款**。`internal/app/agent/` → `internal/app/tool/`（包名 `agent` → `tool`）。理由：现包名「agent」其实装的是 Tool 接口 + 框架 + 系统 tool 实现，不是真正的 agent 概念；统一为 tool framework 命名。包名变化连锁：调用方别名 `agentapp` → `toolapp`（与 Phase 1 已腾出的 `toolapp` 名字 collison-free——`app/tool` 现在重新存在，`app/forge` 才是 user-forge service）。CLAUDE.md 加两条例外：(1) §S12 「app/tool 是 tool 框架 meta-namespace，按 tool 家族允许嵌套子包」（forge / filesystem / shell / web）；(2) §S13 加规则「app 层嵌套子包别名 = `<子名><父名>`」，例 `forgetool`（tool/forge/）vs `forgeapp`（app/forge/）一眼分辨。本阶段只做包改名 + 规范落地，子包占位与 forge 5 件搬家留 Phase 3。`go build` ✅ / `staticcheck` 0 ✅ / 测试除基线 5 个 LLM 集成测试外全绿。|
| 2026-05-02 | **[refactor] Phase 1 完成：大重命名 tool → forge（彻底改）**。规则："Tool 意思是用户造的就改 Forge；Tool 接口本身（抽象 callable）保留"。改动覆盖：(1) Domain 6 个 entity——`Tool`/`ToolVersion`/`ToolTestCase`/`ToolRunHistory`/`ToolTestHistory`/`ExecutionResult` → `Forge`/`Forge*`；(2) DB 5 张表——`tools`/`tool_versions`/`tool_test_cases`/`tool_run_history`/`tool_test_history` → `forges`/`forge_*`，`schema_extras` partial UNIQUE 跟改；(3) ID 前缀——`t_`/`tv_`/`trh_`/`tth_` → `f_`/`fv_`/`frh_`/`fth_`（`tc_` 留：是 test_case 不带 tool 含义）；(4) 包路径——`domain/tool`/`app/tool`/`infra/store/tool` → `domain/forge`/`app/forge`/`infra/store/forge`；(5) 别名——`tooldomain`/`toolapp`/`toolstore` → `forgedomain`/`forgeapp`/`forgestore`；(6) HTTP 22 端点——`/api/v1/tools/...` → `/api/v1/forges/...`；(7) Handler——`ToolHandler`/`NewToolHandler` → `ForgeHandler`/`NewForgeHandler`，文件 `tool.go` → `forge.go`；(8) Router Deps 字段——`ToolService` → `ForgeService`（`Tools []agentapp.Tool` 字段保留：抽象接口列表）；(9) LLM-facing 名 5 个——`search_tools`/`get_tool`/`create_tool`/`edit_tool`/`run_tool` → `search_forges`/`get_forge`/`create_forge`/`edit_forge`/`run_forge`；(10) Forge system tool struct 5 个——`SearchTool`/`GetTool`/`CreateTool`/`EditTool`/`RunTool` → `SearchForge`/`GetForge`/`CreateForge`/`EditForge`/`RunForge`；(11) Bridge 3 个事件类型——`ToolCodeStreaming`/`ToolCreated`/`ToolPendingCreated` → `ForgeCodeStreaming`/`ForgeCreated`/`ForgePendingCreated`，事件名 `tool.code_streaming`/`tool.created`/`tool.pending_created` → `forge.*`，事件 field `ToolID`/`ToolName` → `ForgeID`/`ForgeName`；(12) testend 9 文件 161 处替换。**保留**：`agent/tool.go::Tool` 接口、`Tools []agentapp.Tool` 字段、`ChatToolCall*` 事件（LLM tool-call 概念）、`ToolCallID` 字段（LLM-assigned id）、`tc_` ID 前缀。文档同步：service-design/tool.md 重命名为 forge.md，4 份契约文档批量更新，progress-record 加本条。`go build` ✅ / `staticcheck` 0 ✅ / 22/23 包测试绿（基线 5 个 LLM 集成测试除外）。|

#### 沙箱方向迭代设计（2026-05-02）

| 日期 | 内容 |
|---|---|
| 2026-05-03 | **[devx] devbox + Makefile 二轮整理：环境守卫 + PATH 修复 + 删冗余**。一轮 5 核心命令落地后用户指出 `test-console: ensure-resources` 这种 piggyback 已经冗余（devbox bootstrap 已负责装资源）+ 建议加"在不在 devbox shell"的环境检测。先盘 devbox 实际行为：跑 `devbox shellenv` + `devbox run -- 'env | grep DEVBOX'` 查证——`DEVBOX_SHELL_ENABLED=1` 是 canonical 检测变量，`DEVBOX_PROJECT_ROOT` / `DEVBOX_WD` 同时也设；`FORGIFY_DEV_RESOURCES` 由 devbox.json `env` 块导出已经在 PATH/env 里；但 `$GOPATH/bin` **没** 进 PATH——这是为啥之前 Makefile 要 `$$(go env GOPATH)/bin/air` 显式取路径。**改动**：(1) **devbox.json**：`env` 块加 `"PATH": "$HOME/go/bin:$PATH"` 让 air/staticcheck/deadcode 等 `go install` 出来的二进制 bare 命令可用。(2) **Makefile 加两个守卫 phony target**：`_require-devbox`（任何日常 target 前置；`DEVBOX_SHELL_ENABLED` 没设就退出 + 指引 `make environment` 或 `devbox shell`）+ `_refuse-inside-devbox`（仅 environment 用；在 devbox 里跑会退出，因为 setup 必须在外层 shell 才能 invoke devbox 自身）。(3) **删 `EXPORT_RESOURCES` 宏**（devbox.json env 已设 FORGIFY_DEV_RESOURCES，Makefile 不再重复 export）。(4) **删 `test-console: ensure-resources` 这种 piggyback**——devbox bootstrap 已负责，运行时不再重复 check；`ensure-resources` 仅留作 devbox bootstrap 脚本调用入口。(5) **air / staticcheck / deadcode 改 bare 命令**——配合 PATH 修复，doctor / test-console 不再 `$$(go env GOPATH)/bin/...` 啰嗦。(6) **stop / clear 不要 devbox shell**——纯 lsof+kill+rm，任何 shell 里都能跑。**验证 4 场景**：(a) make help 任何 shell 都跑 ✓；(b) make test-unit 在 normal shell → "✗ this target needs 'devbox shell'" + 退出码非 0；(c) make test-unit 在 devbox shell → 全 unit suite 绿；(d) make environment 在 devbox shell → "✗ must run from your normal shell" + 退出。**PATH 验证**：进 devbox shell 后 `which air` → `~/go/bin/air`，`which staticcheck` 同理，bare 命令可用。**可读性**：Makefile 注释加 §"Setup vs Daily"分组；help 输出明确"Setup (your normal shell, NOT inside devbox)" vs "Daily (inside 'devbox shell')"两段——用户一眼能看到什么命令该在哪跑。 |
| 2026-05-03 | **[devx] Makefile 收成 5 核心命令 + help 默认 target**。用户提议把 Makefile 收成最小可用集，每个命令一眼能看出干啥。**核心 5 个**：(1) `make environment` 新增——首次装配 devbox launcher（curl 到 `~/.local/bin/devbox` 无 sudo）+ `devbox install`（首次拉 Nix 要 sudo 一次）+ `devbox run bootstrap`（air/staticcheck/deadcode + 沙箱资源），run from 普通 shell；幂等。(2) `make test-console` 收编原 `make dev` —— air 前台 live reload + 后台 watcher 等 health 通过开浏览器，Ctrl+C 停；替代两步切换的尴尬。(3) `make test-unit` 不变。(4) `make test-pipeline` 不变。(5) `make stop` 加更友好输出（"nothing running" / "stopping PID(s)"），用 `lsof -ti :PORT | xargs kill` 兜底所有绑端口的进程。**辅助 2 个**：`make doctor`（commit 前体检）+ `make clear`（重置 dev 数据）保留作 optional。**`.DEFAULT_GOAL := help`**——裸跑 `make` 输出命令列表 + "Daily flow: devbox shell → make test-console" 一行教程。**删除**：`make dev`（被 test-console 吸收）/ `make logs`（air 前台日志直接终端走，不再写 LOG_FILE）/ `LOG_FILE` 变量。**README.md 重写为 5 命令版**：quick-start 三行命令 + 五个核心命令对照表 + 不用 devbox 的回退路径。**验证**：`make help` 输出清爽 / `make stop` 实际清掉两个孤儿 PID（54462 + 54564，沙箱迭代调试时遗留的 dev 端口占用）/ `make test-unit` 全绿。 |
| 2026-05-03 | **[devx] 依赖基线统一 + devbox 落地**。三轮收口：(1) **R1 Go toolchain + 直接依赖小升**：`go.mod` 收紧 1.25.1 → 1.25.5（匹配本机 1.25.5）；`zap` 1.27.1 → 1.28.0；`golang.org/x/net` 0.50 → 0.53；`golang.org/x/tools` 0.41 → 0.44；连带 `x/crypto`/`x/mod`/`x/sync`/`x/sys`/`x/text`/`x/telemetry` 自动跟随；`go mod tidy` + `go build ./...` + 全 unit suite（除 baseline 5 个 LLM integration）全绿。(2) **R2 modernc.org/sqlite 大跳**：`modernc.org/sqlite` v1.23.1 → v1.50.0（27 minor 跨度）+ `libc` v1.22.5 → v1.72.1 + `mathutil` v1.5 → v1.7.1 + `memory` v1.5 → v1.11；`google.golang.org/protobuf` 1.36.8 → 1.36.11；`go mod tidy` 自动新增 `modernc.org/cc/v4` + `ccgo/v4` + `opt` + `github.com/ncruces/go-strftime` 间接依赖；store 套件（apikey / chat / conversation / forge / model 五个包）+ 全 unit suite 全绿——SQLite 行为零回归。(3) **R3 devbox 落地**：写 `devbox.json` 锁 `go@1.25` / `python@3.12` / `uv@0.11` / `gnumake@latest`；`env` 块预设 `FORGIFY_DEV_RESOURCES=$HOME/.forgify-dev-resources`（与 Makefile 默认对齐）；`init_hook` 进 shell 时打印工具版本；`scripts.bootstrap` 一键装 `air`/`staticcheck`/`deadcode` + 调 `make ensure-resources` 下沙箱资源。`README.md` 重写为 quick-start（devbox 路径 + 不用 devbox 路径）+ 常用命令表 + 文档地图。`devbox` 0.17.2 launcher 装到 `~/.local/bin/devbox`（非 sudo 路径）；JSON 语法 / packages 字段 / scripts 结构均验证。**用户侧仍需手动**：`devbox install` 实际拉 Nix packages 时 Nix-installer 需 sudo——这步用户首次跑时输密码即可，之后纯 cache 命中。**未做**：未升级 `gorm.io/gorm` / `excelize` / `glebarez/sqlite` / `dslipak/pdf`——前两者无新 release，后两者 wrapper 不动反而稳；间接依赖里 air 的 hugo 链若干 minor 滞后但不影响开发。**测试基线**：~190 个单测全绿（含 19 个 forge service + 13 个 sandbox integration + 11 个 store/forge + AST + smoke bootstrap），baseline 5 个 LLM integration 仍因 DEEPSEEK_API_KEY 401 跳过——与项目长期基线一致。 |
| 2026-05-03 | **[fix] draft forge 首拒后该消失但留下空壳**。用户反馈：`create_forge` 创建出 forge 后立刻 reject，库里残留一个无代码的空壳。根因：`Service.RejectPending` 只把 pending 标 rejected，没意识到 draft forge（`ActiveVersionID == ""`）失去其唯一候选代码后整个 forge 该一并删除。**修法**：`RejectPending` 末尾重读 forge，若 `ActiveVersionID == ""` 触发 `s.Delete(ctx, forgeID)`（含 `sandbox.Destroy` 清 venv 目录）；已 active 的 forge（edit_forge 产生的 pending 被拒）保留 prior 代码，行为不变。**测试**：`TestRejectPending_DraftFirstPendingDeletesForge`（draft 拒 → forge ErrNotFound + sandbox destroy 一次）+ `TestRejectPending_ActiveForgeKeepsAlive`（已 active 拒 pending → forge Get 仍成功 + 无 sandbox destroy）。**文档**：`forge.md` §8.5 RejectPending 注释加 draft-cleanup 分支说明。 |
| 2026-05-03 | **[fix/devx] 沙箱迭代 1 出场 bug：parse() 吞错 + 资源缺失静默 + Makefile 守卫缺失**。用户实际跑 `create_forge` 时复现："code AST parse failed"——但 LLM 生成的代码本身没问题（独立 Python 解析正常）。根因：用户没跑过 `make download-resources`，sandbox 启动时 fail-soft 警告但 `Sandbox.PythonPath()` 仍返回**不存在**的捆绑路径 `<dataDir>/bin/python/bin/python3`；`parseForgeCode` exec 失败 → `Service.parse()` 把原 `fork/exec ...: no such file or directory` 错误**裸吞**只返 sentinel `forgedomain.ErrASTParseError`，违反 §S16；LLM 看到 "AST parse failed" 误以为是代码语法问题，循环重写完美代码却永远跑不通。**3 处修**：(1) **`backend/internal/app/forge/forge.go::parse()`**——把 `return parsedFields{}, forgedomain.ErrASTParseError` 改为 `return parsedFields{}, fmt.Errorf("%w: %v", forgedomain.ErrASTParseError, err)`；errors.Is 仍工作，原 cause 链路保留；加双语注释解释为什么不能吞；(2) **`Makefile`** 加 `ensure-resources` target——按平台键检查 `$RESOURCES_DIR/uv-<platform>` + `python-<platform>.tar.gz` 是否存在，缺则触发 `download-resources`；幂等。`dev` / `test-console` / `test-pipeline` 三个启动 backend 的 target 都加 `ensure-resources` 依赖 + `EXPORT_RESOURCES` 注入 `FORGIFY_DEV_RESOURCES` 给子进程；新加 `RESOURCES_DIR` 变量（默认 `~/.forgify-dev-resources`）和 `PLATFORM` 变量（自动从 uname 推算 `darwin-arm64` / `linux-amd64` 等）；(3) **`backend/internal/infra/sandbox/smoke_bootstrap_test.go`** 新增——按 §T3 用 `FORGIFY_DEV_RESOURCES` 门控的 smoke 测试，跑完整 Bootstrap 流程：解 `~/.forgify-dev-resources/{uv,python}-<platform>.tar.gz` → mac codesign → `s.PythonPath()` 解析到存在的文件 → `python --version` 返 `Python 3.12.13`。**实际跑通**：用户机器 `~/.forgify-dev-resources/` 已下好 `uv-darwin-arm64`（45MB）+ `python-darwin-arm64.tar.gz`（17MB），smoke test 端到端 3.5s 绿（uv 0.11.8 + cpython-3.12.13）。**CLAUDE.md 项目特殊性** 那行更新：明示 `make dev` / `test-console` / `test-pipeline` 自动守卫，裸跑 `go run ./cmd/server` 时记得手动 `export FORGIFY_DEV_RESOURCES=...`，否则 AST parser 误报 "AST parse failed"。**测试**：`go build ./...` ✅ / forge + sandbox + store/forge 套件全绿（含 19 个 service 单测 + 13 沙箱集成 + 11 store + AST 解析）/ 整 unit suite (`go test -count=1 ./... -skip TestIntegration_`) 全绿。**未做**：未给 errmap 加专门错误码区分"sandbox unavailable"vs"真 syntax error"——cause 现在透传到日志已足够定位，进一步细分留观察实际再决定。 |
| 2026-05-03 | **[doc] 沙箱迭代 1 Phase G 完成：8 份契约文档全量同步**。按 sandbox-iteration §10 清单收口，文档与代码 100% 对齐：(1) **`progress-record.md`**（本节 Phase A–F 长 entry 已在；本条收尾）；(2) **`CLAUDE.md`** 项目特殊性：`infra/sandbox` 行加沙箱迭代后果（捆绑 uv + python-build-standalone + 同步 sync + per-EnvID venv），设计原则 #4 不动；(3) **`error-codes.md`** 加 4 行 FORGE_* 映射（`FORGE_ENV_NOT_READY` 422 / `FORGE_ENV_FAILED` 422 / `FORGE_SANDBOX_UNAVAILABLE` 500 / `FORGE_DEPENDENCY_RESOLUTION` 422）置于 TOOL_* 块之后，明示新增前缀策略；(4) **`database-design.md`** forges 表加 `active_version_id` 列说明 + 5 个计算字段（gorm:"-"），forge_versions 表加 8 列说明（dependencies / python_version / env_id 带 index / env_status / env_error / env_synced_at / env_sync_stage / env_sync_detail）；(5) **`events-design.md`** 把"3 个事件"表里 forge 行的载荷字段名扩到含全部新 env 字段（envStatus / envError / envSyncedAt / envSyncStage / envSyncDetail），触发点列表加 EnvStatus 状态转换 + uv stderr 行解析两类；(6) **`api-design.md`** Phase 3 forge 块尾追加沙箱迭代段（POST /forges body 加 dependencies + pythonVersion 可选；PATCH 不接 deps；AcceptPending 422 守卫；revert 自动检测 evicted）；(7) **`desktop-packaging-notes.md`** §五 重写——A 表头降级，C 标记"沙箱迭代 1 已选定（2026-05-03）"，加资源目录约定（dev `$FORGIFY_DEV_RESOURCES` / prod `cmd/desktop` embed.FS）和 Bootstrap 流程位置；(8) **`forge.md`**（最大一份，~360 行新内容）：§1 决策表加 8 行沙箱迭代决策；§3.1 Forge 加 ActiveVersionID + 5 计算字段（含 gorm:"-" 注释）；§3.2 ForgeVersion 加 8 字段定义 + 5 态状态机图 + N=3 EnvID buffer 上限；§4 常量加 EnvStatus 5 值 + MaxEnvIDsPerForge=3 + DefaultPythonVersion + 删 SandboxTimeout；§5 sentinel 加 4 个；§6 Repository 接口加 6 方法（GetVersionByID / UpdateVersionEnvID / UpdateVersionEnvStatus / UpdateVersionEnvProgress / ListEnvIDsForForge / EvictEnvForVersions / GetActiveVersion）；§8.1 Sandbox 接口从 1 方法 Run 扩到 6（新增 SyncRequest/RunRequest/SyncError struct）；§8.1.1 attachActiveEnv 新节；§8.1.2 SyncEnvForVersion + trimEnvBuffer 新节；§8.3 CRUD 全部加 sync 流程注释（CreateDraft 新方法）；§8.4 RevertToVersion 加 evicted 检测；§8.5 AcceptPending 加 EnvStatus 三态守卫 + CreatePending 新方法签名；§8.6 RunForge 路径改为通过 ActiveVersionID → EnvID；§8.9 ASTParser 改 struct 形态（持 pythonPath）；§10 system tool 表更新 SSE 列；create_forge / edit_forge 完整重写（schema 加 deps + python_version + tool_result 含 env_status/env_error）；§11 HTTP 沙箱段（POST body / AcceptPending 守卫 / revert evicted）；§12 错误码加 4 行 FORGE_*；§13 SSE 触发点加状态转换 + uv stderr 进度两条；§14 chains 1/2/3/4 全部沙箱版重写 + 新增"链 7 用户 revert 到 evicted 版本"；§15 表名 tools→forges 修正 + executions 合并；§16 完全重写为新 sandbox 包结构（10 个文件结构 + 6 方法表 + EnvID 算法 + 进程隔离 + 集成测试门控）；§17 拆"Phase 3 初版" + "沙箱迭代 1" 两个清单标 ✅。**手段**：每条 Edit 锁定具体段落，hook 触发的 progress-log 提醒攒到本条统一记录而非逐 Edit 写。**验证**：每段落完成后视觉读 diff；未跑额外自动校验工具——文档 drift 一致性靠下一次代码改动触发的 §S14 强制再 check 兜底。**剩余**：~5% 边角（如 §10 system tools 表的"推 SSE"列描述还可能再细化、§14 链 6 GenerateTestCases 与沙箱无直接关系所以未改）；下次 forge 改动时若发现遗漏即时补。 |
| 2026-05-03 | **[infra/refactor/feat] 沙箱迭代 1 实施完成（Phase A–F 收官）**：把 `01-uv-bundled-python-per-forge-venv.md` 设计落码——backend 整体编译 ✅、~80 sandbox 单测 + 13 集成测试 + 19 service 单测 + 11 store 适配测试全绿、staticcheck 0 errors。**Phase A — sandbox 包重写**：删 `python.go`/`python_test.go`，建 6 文件（`sandbox.go` Config+New+ensureReady+withUVEnv / `paths.go` ComputeEnvID + bundledPythonPath + forgeMutexMap / `pyproject.go` 渲染 + strconv.Quote 防注入 / `progress.go` parseUVLine 三大 stage + scanProgress 双路 errBuf / `preflight.go` Bootstrap 含解压 uv + python-build-standalone + mac codesign + verify / `sync.go` SyncRequest/SyncError + `uv sync` + OnProgress 回调 / `run.go` RunRequest + driver 注入 + ctx-cancel 进程组杀 / `destroy.go` per-forge 锁 / `proc_unix.go` 进程组 SIGKILL / `proc_windows.go` taskkill /T /F）；A5 集成测试用系统 uv + uv 装的 python 3.12 真跑：13 测试全绿（含 fork 子进程也被 ctx-cancel 杀的 process tree 测试）；意外发现 uv 0.11 在 venv 缺时回退 UV_PYTHON 跑 stdlib 代码——更柔和，记录到测试。**Phase B — domain + store 扩展**：`Forge` 加 `ActiveVersionID` GORM 列 + 5 个计算字段（attachActiveEnv 填）；`ForgeVersion` 加 `Dependencies / PythonVersion / EnvID + 5 env 字段`（每版本独立的环境状态）；常量加 `EnvStatus*` 5 值 + `MaxEnvIDsPerForge=3` + `DefaultPythonVersion=">=3.12"`；4 个 sentinel（`ErrEnvNotReady / ErrEnvFailed / ErrSandboxUnavailable / ErrDependencyResolution`）；Repository 接口加 6 方法（`GetVersionByID / UpdateVersionEnvStatus / UpdateVersionEnvProgress / UpdateVersionEnvID / UpdateForgeActiveVersion / ListEnvIDsForForge`）；store 实现含"`UpdateVersionEnvID` 拒改 accepted 行兜底"+"ListEnvIDsForForge 用 `Pluck` 避开 modernc.org/sqlite 的聚合列 string 类型坑"；11 store 测试覆盖跨用户隔离/状态转换/syncedAt 行为/EnvID LRU 顺序。**Phase C — Service 缝合**：`Sandbox` interface 从单方法 Run 扩成 6 方法（PythonPath / Sync / Run / WriteCodeFile / Destroy / DestroyEnv）；加 `attachActiveEnv` helper（镜像 attachPending 模式）+ `publishForgeAfterChange`（集中发布点）+ `SyncEnvForVersion` 同步包装（解 deps JSON → markSyncing → publish → sandbox.Sync(OnProgress) → SyncError unwrap → markReady/Failed）；改 6 个 lifecycle：`Create` 加 fillEnvFields + 触发 sync + ActiveVersionID = v1.ID；新加 `CreateDraft`（无 version 行）；`Update` 改 Code 时新 version **继承** active 的 deps/EnvID/EnvStatus 不重 sync；`CreatePending` snap 提供 deps 优先继承 active fallback；`AcceptPending` 守卫 EnvStatus="ready" + 切 ActiveVersionID + `trimEnvBuffer` 驱逐 LRU EnvID；`RevertToVersion` 复用目标版本 EnvID + evicted 触发 sync 重建；`Delete` 调 sandbox.Destroy；`Get/GetDetail/List` 加 attachActiveEnv 让 GET 含 env 状态；`ast.go::parseForgeCode` 改接 pythonPath 参数（用捆绑 Python 替代系统 python3）；19 service 单测覆盖 lifecycle + sync 路径 + EnvID 共享 + N=3 trim + draft Run 拒绝。**Phase D — Tool + HTTP**：`create.go` 流程改成 **CreateDraft + CreatePending**（不再直接 accepted v1，进 pending 等 user accept）；`edit.go` 已存 pending 时先 `RejectPending` 再创新（venv 跨版本共享所以不浪费）；两个 schema 加 `dependencies / python_version` 可选字段；prompt + tool description 强调 LLM 申报 non-stdlib 包；`tool_result` 含 `env_status` / `env_error` 让 LLM 据此决定下一步；errmap 加 4 行（`FORGE_ENV_NOT_READY` 422 / `FORGE_ENV_FAILED` 422 / `FORGE_SANDBOX_UNAVAILABLE` 503 / `FORGE_DEPENDENCY_RESOLUTION` 422）；`:revert` 端点已就绪（C2 RevertToVersion 自动接 evicted sync）；AcceptPending HTTP 守卫由 service 层 + errmap 自动翻译。**Phase E — 装配**：`scripts/download-sandbox-resources.sh` 平台自动检测 + GitHub latest tag 解析 + uv tarball 解压取 binary + python-build-standalone install_only.tar.gz 按 sandbox 约定文件名落地（uv-`<platform>` / python-`<platform>`.tar.gz）；Makefile 加 `download-resources` target；`cmd/server/main.go` `sandboxinfra.New(Config{DataDir,DefaultPython,Logger})` + Bootstrap from `$FORGIFY_DEV_RESOURCES` (fail-soft logged warn) + `forgeapp.NewService(repo,sandbox,llm,bridge,log)` 接好；smoke run 验证 backend 干净启动 + 缺资源时 warn 提示。**Phase F — testend UI**：`tester.html` action bar 加 `forge-env-badge`（5 状态颜色分支：ready/syncing/pending/failed/evicted）+ `envSyncDetail` 进度条 + `envError` 错误显示区；`style.css` 加配套样式；**不加** `:resync` 按钮（按 §11.1 punt-to-AI 哲学，让 LLM/user 看 EnvError 调 edit_forge 修）；JS 不动（Alpine 自动绑 userSelected.envStatus）。**关键决策定型（vs 文档讨论）**：(1) Sandbox.Run 接收 `(forgeID, versionID, envID)` 元组——sandbox 不知 forge 概念；(2) RunForge / RunTestCase 走 `forge.ActiveVersionID → GetVersionByID → sandbox.Run`，草稿 forge（ActiveVersionID="""）返 ErrEnvNotReady；(3) Create 内部 sync 失败仅 log 不传播——forge 留下 EnvStatus="failed" + EnvError 让 LLM 看；(4) trimEnvBuffer **不主动**改老 ForgeVersion EnvStatus="evicted"——下次 Run 命中 uv 自然报错触发 :resync（最纯粹的 punt-to-AI）；(5) edit_forge 已有 pending 时先 reject 再创新（统一草稿期 + 激活期入口）；(6) Update 不接 deps（仅 Code）——deps 改走 edit_forge → CreatePending → AcceptPending 路径；(7) errmap 现状 `TOOL_*` wire codes 保留作 Phase 1 客户端兼容；新 sentinel 用 `FORGE_*` 前缀。**仍 broken**：2 个 LLM 集成测试（DEEPSEEK_API_KEY 失效，与本迭代无关，progress-record 长期基线）。**剩 Phase G**：契约文档 8 份按 sandbox-iteration §10 清单同步——本条目即是其一；其他 7 份接下来。 |
| 2026-05-03 | **[doc] 沙箱迭代 1 应用 MVP "punt 给 AI 自救" 哲学 + stage 名字修正**：审视设计后决定砍掉一批"自动恢复"机制，把它们 punt 给 LLM 看错自救——agent 系统跟传统 backend 的核心红利就是 AI 看到错误能调 `edit_forge` / `:resync` 自愈，不需要 backend 替它写恢复逻辑。**砍掉的"自动修复"**：(1) 启动期 reconcile `EnvStatus="syncing"` 残留；(2) venv 完整性严密校验（`stat .venv` 简单判断即可）；(3) Run 时 evicted 自检 + eager 状态同步；(4) 孤儿 venv 目录定期 GC；(5) 进程退出前清半成品。**只保留两个真兜底**：(a) mac codesign（不修就内核 SIGKILL 无日志，LLM 也救不了）；(b) 错误信息收集到 EnvError（不收集 LLM 看不到错就没法救自己）。这跟设计原则 #6 "反校验剧场" 一脉相承——不预先防 LLM/用户能自然修复的事。**文档改动**：(1) §4.4 Sync 内部加 `errBuf` + `SyncError{Cause, Stderr}` 包装类型——成功时 stderr 丢弃，失败时整段 stderr 透传 EnvError 让 LLM 看真实错误（如"numpy>=2.0 conflicts with python<3.12"）；(2) §4.5 Progress 解析 stage 名字 `downloading` → `preparing`（uv 真实输出三大行 `Resolved/Prepared/Installed`，"Downloaded" 是 Prepared 阶段内部 sub-progress 不当总结），代码示例完整化 scanProgress 函数；(3) §4.6 Run 内部 stderr 透传到 ExecutionResult.ErrorMsg → tool_result，明确不做 sandbox 层 evicted 自愈；(4) §5.4 时间线示例 stage 名字跟改；(5) §11 重写为两小节——§11.1 新增"MVP 哲学：punt 给 AI 自救"段（含砍掉机制对照表 + 保留兜底说明），§11.2 原"不做的具体清单"前置 punt 哲学条目。结果：开工范围更窄，文档更简短，错误处理路径更统一（成功 / 失败两条线分明）。 |
| 2026-05-03 | **[doc] 沙箱迭代 1 文档对照实际行为修正 + macOS 公证 entitlement 落档**：基于多轮 web search 反查 uv / python-build-standalone / macOS 安全机制实际行为，发现并修正 5 处认知偏差，且把 macOS 公证关键细节写进 `desktop-packaging-notes.md` §六。**5 处技术修正（沙箱迭代文档）**：(1) wheel 共享机制——之前说"硬链接"，实际 uv 默认 link-mode 在 mac/linux 是 `clone` (copy-on-write，APFS / btrfs reflink)、windows 才是 `hardlink`，效果一样但术语错；(2) Python 来源——之前说 Bootstrap 跑 `uv python install` 联网下，实际我们走 embed.FS 解压后通过 `UV_PYTHON` 环境变量或 `--python <path>` 让 uv 用我们捆绑的，完全离线（uv `--python` flag 接受路径不仅是版本号）；(3) uv stderr stage 名字——之前说 `resolving/downloading/installing`，实际是 `Resolved/Prepared/Installed` 三大总结行（"Downloaded" 子事件发生在 Prepared 内部），progress.go 解析改识别这三个；(4) `pyproject.toml [tool.uv] managed = true` 默认即 true 可省；(5) macOS quarantine 真相——之前说锅扣给 `com.apple.quarantine` xattr 跑 `xattr -dr` 即可，实际元凶是 **`com.apple.provenance`** xattr（issue uv#16726 / #16003），单纯 `xattr -d` 不够，必须 `codesign --force --sign -` ad-hoc 重签才能清 Gatekeeper 缓存——不然 macOS 内核层 SIGKILL 无日志极难调试。**沙箱迭代文档 §4.3 Bootstrap 改写**：删掉错误的 `uv python install` 步；mac 步改成两阶段——v0.x 早期跑 `xattr -dr com.apple.provenance` + walk codesign 所有可执行；v1.0+ 公证后此步可省（公证 ticket 覆盖嵌入二进制）。**windows 杀进程树**：从"未决"降级为"用 Job Object + JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE"（Go 1.26+ 不需 unsafe pointer），平台分文件 `paths_unix.go` / `paths_windows.go`；taskkill /T /F 作 fallback。**desktop-packaging-notes.md §六新增整段"macOS 公证的内嵌二进制覆盖"**：(a) 不公证时主 .app 用户可右键→打开绕过但 python 子进程仍被内核 SIGKILL，要应急 codesign；(b) 公证时关键 entitlement `com.apple.security.cs.disable-library-validation` 让 Python 能 dlopen 运行时下载的 wheel `.so`——没这条公证再多新装包仍挂；(c) 公证不解决 TCC 隐私权限 / App Management 等用户态权限对话框（普通 mac app 都有，不算 Forgify 特有问题）；(d) Bootstrap 跟两阶段对应表。两份文档交叉指引（沙箱迭代 §4.3 ↔ packaging-notes §六）。 |
| 2026-05-02 | **[doc] 沙箱迭代 1 设计文档完整重写（v2）**：`adhoc-topic-documents/sandbox-iteration-documents/01-uv-bundled-python-per-forge-venv.md` 整篇重写。基线吃透 Phase 5+6（forge_executions 合并 / 3 entity-state 事件 / Forge.Pending 计算字段）后多轮迭代讨论收敛出最终设计。**核心**：捆绑 python-build-standalone + 捆绑 uv 二进制（资源进 cmd/desktop embed.FS，dev 走 `$FORGIFY_DEV_RESOURCES`），每 EnvID 一个独立 venv（**venv identity = deps，不是 version**——同 specifier+pythonVersion 多版本零代价共享）。**用户动线开篇**：search→create（流式 name/desc/code/deps→装环境→失败）→edit 改 deps→重装→user accept→run 热路径。所有设计为这条线服务。**EnvID 算法**：`"env_" + sha256(normalize_and_sort(deps).join + pythonVersion)[:6]`；标准化层做 trim+包名小写+排序，PEP 440 等价 specifier 不强求合并。**文件布局**：`forges/<id>/envs/<EnvID>/{pyproject.toml,uv.lock,.venv}` + `forges/<id>/versions/<versionID>/main.py`；代码跟 version 一对一，venv 跟 EnvID 一对一。**N=3 EnvID 缓冲**：超出删最旧；老 ForgeVersion 不删，EnvStatus 转 `"evicted"`，revert 到那时触发即时重建。**数据模型重新分布**：`Forge` 加 `ActiveVersionID` GORM 列 + 4 个计算字段（EnvStatus/EnvError/EnvSyncStage/EnvSyncDetail，attachActiveEnv 从 ActiveVersion 拷过来）；`ForgeVersion` 加 `Dependencies / PythonVersion / EnvID + 5 env 字段`（env 状态挂在每版本上，pending 自带自己的 env 状态）。**EnvStatus 5 值状态机**：pending→syncing→ready/failed；ready→evicted→syncing。**Sandbox 接口 5 方法**：Bootstrap / Sync / Run / WriteCodeFile / Destroy / DestroyEnv；不知 forge 概念，只接 (forgeID,versionID,envID) 元组。**关键变更 vs 之前讨论稿**：(1) 砍掉异步 sync worker——create/edit 同步等 sync 完成，跟 LLM tool call 一气呵成；(2) 不引入新 SSE 事件类——Phase 6 entity-state 模型规约，所有进度通过现有 `forge` entity-state 事件推送，触发点表加 #6 状态转换/#7 每行 stderr 解析；(3) sandbox 不直接 bridge.Publish——通过 OnProgress callback 让 forgeapp 写库 + publishForgeSnapshot（跟 chat.runner 是 chat.message 唯一发布事实源同模式）；(4) **create_forge 改进 pending 流程**——不再直接 accepted v1，跟 edit_forge 走统一审核入口（用户语义"环境装好才算工具创建成功"）；(5) Run 不设 timeout，只靠 ctx-cancel；删 `ErrSandboxTimeout` sentinel。**4 个新 sentinel**：ErrEnvNotReady / ErrEnvFailed / ErrSandboxUnavailable / ErrDependencyResolution。**ast.go 也走捆绑 Python**——抽 ASTParser 接收路径。**Phase 划分** ~4 天独立交付：A sandbox 内部 / B domain+service / C tool+HTTP / D 装配 / E 文档同步 / F testend UI。**明确不做**：异步 worker / run timeout / 安全隔离 / Forge 静态元属性（destructive 走 per-call）/ 多 Python 版本 / venv 自动过期 / 新 SSE 事件类——所有不在用户动线上的复杂度全部不进。 |

---

## 3. Phase 4-5 路线（粗粒度）

各 Phase 开工前在此段展开细节。当前状态均为 ⬜。

### Phase 4：工作流能力（~20h，最大一块）

workflow（DAG + 状态机）+ flowrun（执行实例）+ 节点系统（LLM / Tool / Trigger / Approval / Variable 5 类）+ scheduler（cron / fsnotify / HTTP webhook）+ chat 再升级支持"对话创建工作流"。执行引擎自实现（Eino 已全面移除，不再依赖 eino/compose）。

**桌面端预留**（来自优化轮决策）：
- `Notifier` 接口在此阶段定义（domain/notification/），scheduler 依赖
- `Preferences` service 在此阶段定义（含 startOnLogin / missedTaskPolicy 等配置项）
- scheduler 状态全部走 store 持久化；时间源用 monotonic 算间隔、wall clock 调度具体时间；错过任务策略明确决策（skip/runOnce/runAll）

### Phase 5：智能化（~15h）

knowledge + document（本地 sqlite-vec）+ intent（ReAct Agent）+ mcpserver（`mark3labs/mcp-go`）+ skill（V1 浅版：打标签的工具）+ chat 终极版（意图识别 → 工作流推荐 → 自动建草稿）。

**风险点**：sqlite-vec 是 C 扩展，需验证 modernc.org/sqlite 加载兼容性。Phase 5 开工前先做兼容性 spike，跑不通则评估替代方案（备选：换回 mattn 接受 CGO / 用别的本地向量存储）。

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
