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
| 2026-05-01 | **[infra] SQLite 驱动从 mattn/go-sqlite3 (CGO) 迁移至 modernc.org/sqlite (纯 Go)**：动机是为未来桌面 app 分发铺路——CGO 阻碍跨平台编译、要求 C 工具链、增加 macOS 公证复杂度。改动：(1) `db.go` import `gorm.io/driver/sqlite` → `github.com/glebarez/sqlite`（GORM 适配层，底层用 modernc）；(2) `buildDSN` 重写——mattn 的 `_journal_mode=WAL` 等私有参数语法 → modernc 的 `_pragma=journal_mode(WAL)` 等标准 PRAGMA URI 语法，文件 DSN 加 `file:` 前缀；(3) `schema_extras.go` 注释更新（移除 CGO_CFLAGS 编译说明，改为"FTS5 内置"）；(4) go.mod 依赖切换：`gorm.io/driver/sqlite` + `mattn/go-sqlite3` 消失，新增 `glebarez/sqlite` + `modernc.org/sqlite` + `modernc.org/libc`。**核心收益**——四平台跨平台编译一行命令通过：`GOOS=darwin/linux/windows go build`，二进制 24-25MB。SQL 性能慢约 1.5-2 倍（本地单用户场景不可感知），二进制略大 5-10MB。无 entity / 接口 / 端点 / 错误码 / schema 变动 |

#### 桌面端分发方向定型（2026-04-30 ~ 2026-05-01）

| 日期 | 内容 |
|---|---|
| 2026-04-30 | **[doc] 桌面端分发方向讨论 + `desktop-packaging-notes.md` 落地**：明确产品定位为本地优先单人工具，**不做网页部署**；目标分发形态为 **Wails 原生桌面 app**（系统 webview，非 Electron）；集成方式选"Wails 当窗口外壳 + 复用 httpapi"，保留未来网页版退路（不走 Wails 原生 binding）；分发选 dmg/setup.exe/AppImage（v0.1 起即 L3 级），代码签名延后（macOS 公证 $99/年是 v1.0 时最高 ROI 投入）；Python 沙箱短期方案 A（README 写要求系统 Python），中期方案 C（捆绑 python-build-standalone +30-50MB）|
| 2026-05-01 | **[doc] 常驻后台模式 + Notifier 接口决策**：app 长期目标为 scheduler 不退出的常驻后台形态——关窗 ≠ 退出（Wails `HideWindowOnClose`）。必做四件事：系统托盘图标 / 单实例锁 / 开机自启选项（默认关）/ graceful shutdown。架构决策：`Notifier` 接口在 **Phase 4 写 scheduler 时**就定义在 `domain/notification/`，scheduler 依赖接口；`cmd/server` 注入 `LogNotifier`，未来 `cmd/desktop` 注入桌面通知实现。桌面端外壳代码（tray / notification / autostart / single-instance）放 `internal/infra/desktop/`，仅 `cmd/desktop` import，确保 `cmd/server` 编译产物不含 Wails |
| 2026-05-01 | **[doc] 为什么不走 Wails binding**（书面记录避免未来重复纠结）：HTTP 不比 binding 难（写法等价）；走 binding 等于扔掉 v1.2 重写好的 transport（middleware / response 包络 / errmap / curl 测试）；HTTP 有的优势 binding 没有：浏览器 Network 调试、SSE 天然契合 chat、进程隔离、可演进（未来 CLI / cloud sync 都能复用）。binding 唯一真有用的优势"类型同步"用 OpenAPI + openapi-typescript 也能做到。结论：transport 层永远只 httpapi 一份 |
| 2026-05-01 | **[doc] V1.2 文档全量校对与重写**：综合代码与开发记录，对 11 份 V1.2 文档系统性校对——发现并修复多处 drift。核心改动：(1) **testend-design.md** 整体重写——`integration/` → `testend/`、`index.html` → `tester.html`、collections 列出实际 12 个 YAML、Tab 顺序改为 Config/SSE/Logs/SQL/Tests/Tools 6 个、删除不存在的 `POST /dev/collections/{name}/run` 端点（实际由前端 JS 直接 fetch）、Makefile 段从 `integration/Makefile + make dev + CGO_CFLAGS` 改为项目根 `make testend`；(2) **backend-design.md** Strategy 段标记 Phase 6 原子切换已完成、Phase 路线图加状态列、Architecture tree 顶层 `backend-new/` → `backend/` 并把所有 domain/app/infra 的实际状态打勾、apikey 子目录文件结构按实际（`apikey.go / providers.go / tester.go`，mask + keyprovider 已合并）、Verification 段去 FTS5 性能基线 + 加跨平台编译验证、删 Phase 6 路线图条目；(3) **database-design.md** SQLite 驱动改为 modernc.org/sqlite + DSN PRAGMA 语法说明 + FTS5 当前未使用注明；(4) **service-design 5 份** 按 Strategy B 全部去 Eino——apikey.md / model.md 改 Service 示例代码（`eino` field → `llmFactory`）、apikey.md §9 重写"为什么 Tester 不复用通用 LLM 客户端"、tool.md 状态 `🔄 设计已定` → `✅ 已实现`、conversation.md 修两处 Phase 表述、chat.md 大修：§5.1 文件结构 `pipeline.go` → `runner.go`（5 文件 → 6 文件）、ReAct loop 函数名按实际重写（`runReactLoop` → `agentRun`，`persistMsg`+`finalPersist` → `writeDB(fatal)`，`consumeStream` → `streamLLM`，`executeToolCalls` → `runTools`，`buildLLMHistory` → `buildHistory` + 新增 `extendHistory`），§10 重复编号 8.3/8.4/8.5 修为 10.3/10.4/10.5、§9 §10 重复段（Phase 演化 + 完整调用链）合并精简为 §11 完整调用链 + §12 Phase 4-5 扩展点、关键决策表 Eino 行替换为"自实现 vs framework"理由、实现清单文件名按当前 6 文件结构更新；(5) **`.claude/settings.local.json`** 已有的 progress-log hook 在本次重写过程中持续提醒。错误码、事件清单与代码 100% 对齐无需改。|

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
