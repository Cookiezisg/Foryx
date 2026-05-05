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
| 2026-05-04 | **[feat]** W2 WebFetch tool：`web/{web,fetch}.go`，9 方法 + 24 单测。两段抓：Jina r.jina.ai → 直 GET fallback；30s timeout / 1MB cap；SSRF 守卫（loopback/private/link-local/unspecified/multicast 全拒，含 DNS rebinding 防御：解析全 IP 任一禁区即拒）。`pkg/llmclient` 加 `ResolveForWebSummary`（透明 fallback 到 chat 场景）|
| 2026-05-04 | **[feat]** W3 WebSearch tool：`web/{search,search_bing}.go`，9 方法 + 21 单测。3 层 fallback：SearXNG 公共池（随机洗牌 + JSON 解析）→ Bing HTML 抓 → Bing CN HTML 抓；每后端 10s 单超时。Bing 解析用 `x/net/html` visitor（`<li class="b_algo">` 提 title/url/snippet，含 `b_caption` 缺失时 fallback 到首个 `<p>`）。`FORGIFY_SEARXNG_INSTANCES` env 可覆盖实例池 |
| 2026-05-04 | **[test]** W4 web 装配 + pipeline test：`main.go` + `harness.go` 装 WebTools；`test/web/` 2 场景（WebFetchBlocksLoopback / WebSearchRejectsEmptyQuery）故意 short-circuit 不打外网，验 LLM ↔ tool 接线。11s 通过；errmap 无变更——tool 错误返友好字符串不到 handler |
| 2026-05-04 | **[feat]** B1 shell tools 三件套：`shell/{shell,manager,bash,output,kill}.go`，3 工具 × 9 方法 + 47 单测。Bash 含 cwd 状态机（AgentState 加 `Cwd()/SetCwd()`，`cd <path>` 整命令短路更新；链式 `cd && ...` 不追踪——与 subshell 语义一致）+ 前后台双模式（前台 sh -c 带 timeout，120s 默认 / 600s 上限；后台 spawn + ProcessManager 注册返 `bsh_<16hex>`）。BashOutput 增量游标 + 可选 regex filter；KillShell SIGKILL 幂等。256 KB 环形输出缓冲，溢出丢头 + 游标 rewind。Forgify 故意不带 banned-command 表（本地单用户） |
| 2026-05-04 | **[test]** B2 shell 装配 + pipeline test：`main.go` 装 ShellTools + `defer Manager.Stop()` 优雅关停杀子，`harness.go` 走 `t.Cleanup`；`test/shell/` 3 场景（BashEchoForeground / CdStateMachinePersistsAcrossCalls / BashOutputAndKillShellHandleUnknownID）。19s 通过；errmap 无变更——tool 错误返友好字符串 |
| 2026-05-04 | **[feat]** U1 task mini-domain + 4 tools：新建 `domain/task` + `infra/store/task` + `app/task` + `app/tool/task`（factory + 4 工具）。Task entity 含 ConversationID 作用域、status 白名单（pending/in_progress/completed/deleted）、`tk_<16hex>` ID。Service 跨 conv 报 ErrNotFound 防泄漏。每次变更发 `task` SSE 事件（entity-state，与 forge/conversation 同模式）。`pkg/reqctx` 加 `ErrMissingConversationID` + `RequireConversationID`。装到 main + harness + DB migrate。共 60+ 单测 + store 集成测试全绿 |
| 2026-05-04 | **[feat]** U2 AskUserQuestion 后端：新建 `app/ask`（in-memory 会合 Service：Wait 阻塞 + Resolve 原子摘条目防双答竞态）+ `app/tool/ask`（AskUserQuestion 工具，5 分钟超时，问题坐 chat.message SSE）+ HTTP `POST /api/v1/conversations/{id}/answers`（handler + Deps.AskService + router 装配）。errmap 加 4 行（task ×3 + ask ×3 + reqctx.ErrMissingConversationID）。决策 D11 落地——不新建事件家族 |
| 2026-05-04 | **[test]** U3 ux-tasks 装配 + pipeline test：harness `eventsBridge` 笔误修正为 `bridge`；`test/uxtask/` 3 场景（TaskCreateAndList / AskUserQuestionAnswerDelivered / AnswerEndpointUnknownCallID_404，旁路 goroutine POST 答案验真实接线）。20s 通过；pipeline 全 12 suite 全绿 |
| 2026-05-04 | **[doc]** Z1 V1 batch 文档全量同步：新建 `service-design-documents/task.md`（task mini-domain 完整设计 + §10 附 ask 服务）；4 契约文档全更——api-design 加 Phase 5 工具家族表 + answers endpoint，database-design 加 `tasks` 表行 + 关系图，error-codes 加 task ×3 + ask ×3 + reqctx.ErrMissingConversationID，events-design 加 `task` 事件（entity-state 4 个）；chat.md §2.3 / §4.1 / §4.2 / §4.3 / §4.4 同步 9 方法 + execution_group + AgentState 注入 + 20 工具 |
| 2026-05-03 | **[devx]** 项目根 + Makefile + devbox 瘦身：删 `.githooks/` / `.air.toml` / `tmp/` / `scripts/`；Makefile 砍 4 项；devbox 删 `python@3.12`（venvShellHook `.venv` 坑）+ `uv@0.11`（装饰） |

#### Phase 4 准备件 — 4 domain 设计批（2026-05-05，待实施）

为 Phase 4-5 的 workflow / 智能化 提前打地基。整批仅设计文档落档，代码未动；预估 ~10 天实施周期（mid-month deadline）。

| 日期 | 内容 |
|---|---|
| 2026-05-05 | **[doc]** 4 份 service-design 文档落档 ~2700 行：`subagent.md` / `mcp.md` / `skill.md` / `catalog.md`。完整方案见各文件 |
| 2026-05-05 | **[doc]** 4 份 contract 文档全量同步（api-design 加 ~25 端点 / database-design 加 subagent_runs + subagent_messages 两表 / error-codes 加 ~20 sentinel / events-design 加 subagent + mcp + skill + forge.persisted 4 事件 + chat.message 加 subagentRunId 字段）；backend-design.md Architecture 树加 4 新 domain |
| 2026-05-05 | **[arch]** 关键设计决策：① **subagent 双流 SSE**——subagent 事件传 run lifecycle，chat.message 带 subagentRunId 复用主对话 schema 流式传内容；② **MCP search/call 模式**不 flat 注册，避 70k token 启动开销；③ **Skill progressive disclosure 三层**（L1 metadata always-on / L2 body Read-on-trigger / L3 resources LLM 自取）；④ **Capability Catalog 接口反转**——CatalogSource port 让新 source 接入 0 行修改 catalog；⑤ MCP 内置 8 server Registry（everything/memory/sequential-thinking/fetch/time/filesystem/git/sqlite）+ marketplace 安装流程 |
| 2026-05-05 | **[arch]** 一轮自检 + 多轮纠偏：(a) 删 `enabled` 字段（catalog 解决 token 爆炸已无须装而不起）；(b) 删 skill/mcp 的项目级目录概念（自包含原则、单用户场景用户级足够）；(c) 删 catalog 的 routing-hints 用户文件（应由 generator 自动推断而非用户维护）；(d) 修订 catalog debouncer→1s BurstCoalescer + singleflight（5min 是过度防御）；(e) catalog 仅订阅 `forge.persisted` 不订阅 streaming `forge`，加 fingerprint dedup（name/description 不变跳过 LLM 调用）|
| 2026-05-05 | **[arch]** P0 生产级缺口补全：MCP per-call 30s timeout + per-server override / Cancel 通过 `notifications/cancelled` 级联 / in-flight RPC 在 disconnect 时清理 / stderr 256KB ring buffer / mcp.json 损坏 fail-soft；Subagent cancel ctx 级联 / 总超时 5min / panic recover / 并发 isolation by RunID / conv 删除不级联 subagent_runs；Skill fork-in-fork 强制忽略 fork directive / symlink 循环防护 / 并发 activate 栈结构 / allowed-tools scan 时校验；Catalog 全 source fail 保留旧 cache / generator output 2k token cap / cache 损坏移 .bak |
| 2026-05-05 | **[arch]** Catalog 触发机制大简化：从"事件订阅 + BurstCoalescer + singleflight + dirty-loop + forge.persisted 专用事件"改成 **1s polling + atomic.Bool 单 flight + fingerprint dedup**——~150 行复杂度删 ~30 行替代；MCP 加载中间态、forge streaming 噪音、并发竞态、启动顺序烦恼全自然消化。CatalogSource 接口砍掉 `EventTopics()` 方法；events-design 删 `forge.persisted` 行 |
| 2026-05-05 | **[arch]** Subagent SSE 大简化：原"双流（自己事件 + chat.message+subagentRunId）"改成 **单流 chat.message** —— subagent 上下文的 chat.message 载荷里嵌套完整 `subagentRun` 快照（含 lifecycle / token 累计 / lastTool 等），一个事件同时承载消息 + run 状态。**不再有独立 `subagent` SSE 事件类型**。事件总数从 7 减到 6。前端单一渲染逻辑（chat.message），按 subagentRunId 字段分流到主对话区 / 流式小窗 |
| 2026-05-05 | **[arch]** 终轮自审 5 处 stale + 2 处过度设计修复：catalog.md `:refresh` 描述去掉 BurstCoalescer 残留；catalog.md source 总览表删 EventTopics 列；subagent.md 关联文档 / 端到端推演 / 测试 desc 3 处独立 `Subagent` 事件残留改为 chat.message 嵌入 subagentRun；**Skill ActiveSkill 栈结构改 `atomic.Pointer[Skill]` last-write-wins**（单用户场景过度防御）；**Subagent LastTool* 5 字段从 `subagent_runs` 表移到 in-memory `gorm:"-"`**（瞬时字段重启丢失无所谓，DB schema 干净）|
| 2026-05-05 | **[arch]** Catalog 失败策略最终落定："**用户活跃度驱动重试**"——LLM 失败 → mechanical fallback 顶上 + lastFP 照常更新。用户不动东西就不再耗 LLM；改了东西 fp 自然变 → LLM 重新机会跑。无须后台 backoff，无须无限重试。Generator 内部重试改 1 次→ **2 次（共 3 次 attempt）**；key 轮训改成"**真跑 LLM 调用**"（不再只是 build client，单 key Generate fail 立即试下一个 key）|
| 2026-05-05 | **[devx]** 排程从 D1-D10 砍到 **D1-D8**，binary 打包 + 上手文档 + demo 预演由用户自己解决 |
| 2026-05-05 | **[arch]** Subagent 整体从 catalog 移除：原设计有 SubagentCatalogSource 列举 3 个内置类型，但 Subagent system tool 自身 description 已覆盖 subagent 类型说明，catalog 再列一遍冗余。CatalogSource 实现方从 4 个收为 **3 个：forge / skill / mcp** |
| 2026-05-05 | **[arch]** Subagent spawn 工具改名 **`Task` → `Subagent`**——避开与已有 `task` mini-domain（TaskCreate/List/Get/Update 管 TODO）的 LLM-facing 命名撞车；Go struct `TaskTool` → `SubagentTool`，包 `app/tool/agent` → `app/tool/subagent` |
| 2026-05-05 | **[arch]** ⭐ Sandbox v2 大重构：原 `infra/sandbox` forge-only（uv + python-build-standalone 直 bundle）改为统一 PluginSandbox 服务。Bootstrap 极简（仅 mise ~10MB，go:embed），所有语言 runtime（Python/Node/Rust/Java/Go/Ruby/PHP/...）lazy install；mise 通配 installer + Playwright/dotnet/static 专用 installer；per-plugin env 隔离（forge / mcp / skill / **conversation** 4 类 owner）；SQLite 双表 manifest（sandbox_runtimes + sandbox_envs）取代 JSON 文件。完整方案见新建的 `service-design-documents/sandbox.md`（~940 行）|
| 2026-05-05 | **[arch]** Bash tool 自动路由 + 对话 scratch env：LLM 通过 Bash 跑 `pip install pandas` / `python script.py` 等命令时，sandbox 检测命令意图自动路由到该对话的 scratch env（`Owner{Kind:"conversation", ID:"<conv>:<runtime>"}`）。**denylist 整套机制不再需要**——靠基础设施收口。conversation env 30d auto-GC（forge/mcp 默认手动）|
| 2026-05-05 | **[arch]** 摆脱 OOTB 预装：原 mcp.md 的 cmd/resources 扩展预装 5 个 server + Chromium 设计**全部废弃**——改 lazy install via sandbox v2。Forgify 总安装 ~25 MB binary + ~10 MB mise bootstrap = ~35 MB，比原 ~250 MB 砍 85%。用户首装某 server 才触发 runtime + 包的下载（带进度条）|
| 2026-05-05 | **[devx]** 配套改造：`Makefile clear` target 加清 `~/.forgify/sandbox/`；`devbox.json` bootstrap 删 `cmd/resources` 调用、删 `FORGIFY_DEV_RESOURCES` env；`cmd/resources/main.go` 重写为 mise binary fetcher（per-platform，给 go:embed 用）；`cmd/server/main.go` sandbox 装配段重写——sandboxapp.New + 注册 10+ installer + forge service 切到新接口；`app/tool/shell/bash.go` 加 detectRuntime + 自动路由逻辑 |
| 2026-05-05 | **[doc]** 沙箱 v2 文档同步：sandbox.md 新建（943 行）+ database-design 加两表 + error-codes 加 8 sentinel + api-design 加 sandbox endpoints + backend-design 加 domain/app/infra 三层 + forge.md / mcp.md 改为指向 sandbox.md（forge sandbox 接口保留作 adapter；mcp 整段 OOTB 预装设计删除）|
| 2026-05-05 | **[arch]** 包管理器共享机制 + GC 简化：Node 由 npm 改 **pnpm**（content-addressable global store，多 conv 共装同包磁盘 ≈ 1×）；uv 已自带 hardlink wheel cache。**v1 全 owner 默认手动 GC**（共享让磁盘开销极小，auto-GC 价值低）；用户主动点 `:gc` 端点或 plugin 卸载时触发 |
| 2026-05-05 | **[arch]** Sandbox bootstrap 失败 + Degraded Mode：bootstrap 失败（mise 解出 / chmod / exec 失败）→ app 仍启动；纯文本 chat + 不需 runtime 的工具（Read/Write/Edit/Bash 跑 ls/cat/git）仍可用；needs-runtime 操作（forge/mcp install/Bash 跑 pip etc.）fail-fast 返友好错；UI banner + retry 按钮；新增 GET /sandbox/bootstrap-status + POST /sandbox:retry-bootstrap |
| 2026-05-05 | **[arch]** ⭐ Windows v1 加入：5 平台 binary（macOS arm64/amd64 + Linux amd64/arm64 + Windows amd64）；Python/Node 类 MCP 全 Windows 可用；Ruby/PHP/长尾 plugin 通过 RegistryEntry.UnsupportedPlatforms 在 Windows marketplace 隐藏；Bash tool 加 Windows 分支用 PowerShell 替代 sh；进程 cancel 用 Job Object；flock 用 sys/windows.LockFileEx；fsnotify ReadDirectoryChangesW；mise binary per-platform embed（build tag 控制）。覆盖 99% 用户需求 |
| 2026-05-05 | **[devx]** 排程从 D8 → **D15**：Windows 适配 + 测试加 D10-D15（~6 天）；总周期内仍能赶投资人月中回来 demo |
| 2026-05-05 | **[arch]** mcp.md §5.5 内置 marketplace 大改——基于 web research 重选 5 个（4 用户可见 + 1 hidden test）：**Playwright / MarkItDown / Context7 / DuckDuckGo / SQLite + everything**。砍掉与内置工具重复的（fetch / filesystem / git / time）；砍掉跟原生计划撞车的（memory）；不要 OAuth 系列（github/notion/slack v2+）。RegistryEntry 加 `Bundled` / `Hidden` / `PostInstallSteps` / `OnlineOnly` / `Notes` 5 字段。**OOTB 预装机制**：cmd/resources/ 扩展抓 Node binary + npm/uvx 预装 5 个 MCP 包 + Playwright Chromium，~250 MB 总成本；首次启动一次性 setup，之后 marketplace 全绿可用 |

#### Phase 4 准备件 — D1 sandbox v2 实施（2026-05-05~）

| 日期 | 内容 |
|---|---|
| 2026-05-05 | **[feat]** D1-2 sandbox domain 包落地：`internal/domain/sandbox/`（sandbox.go + installer.go，~410 行）—— Runtime/Env 两实体 + Owner/RuntimeSpec/EnvSpec/SpawnOpts/ExecutionResult/LongLivedHandle/ProgressFunc 值对象 + 8 sentinels + Repository + RuntimeInstaller/EnvManager 双端口 |
| 2026-05-05 | **[feat]** D1-3 sandbox store 落地：`internal/infra/store/sandbox/`（sandbox.go + sandbox_test.go）—— Repository GORM 实现（Runtime/Env CRUD + FindDefaultRuntime/FindRuntime/FindEnvByOwner/ListEnvsByRuntime/ListEnvsByOwnerKind/TotalSizeBytes/ListEnvsLastUsedBefore），系统级表不按 userID 过滤，19 集成测试全绿 |
| 2026-05-05 | **[feat]** D1-4/5/6 sandbox 装配三件：(a) AutoMigrate 注册 Runtime + Env 两表；(b) errmap 加 8 sandbox sentinels（4xx/5xx 按 error-codes.md）；(c) `bootstrap_mise.go` 骨架定 v2 mise embed 方案（per-platform `//go:embed`，darwin 沿用 macCodesign ad-hoc 签绕 Gatekeeper，待 Apple Developer ID 后切 notarization；D2 填二进制 + extractMise 函数体）|
| 2026-05-05 | **[feat]** D2-1 mise binary fetcher：`cmd/resources` 重写——旧的 uv + python-build-standalone fetcher 整体替换为 jdx/mise per-platform 下载器（默认当前平台，`--all-platforms` 拉全 5 平台），SHA256 校验 + 原子写 + 幂等。输出到源码树 `backend/internal/infra/sandbox/mise/<goos>-<goarch>/mise[.exe]` 给 D2-2 的 `go:embed` 用。Makefile target 由 `fetch-mise` 改为 `resources`，devbox bootstrap 自动跑。**v1 dev resources 目录** (`~/.forgify-dev-resources` + `FORGIFY_DEV_RESOURCES` env) 不再被消费；forge sandbox v1 在 D2-5 之前 fail-soft 返 ErrSandboxUnavailable |
| 2026-05-05 | **[feat]** D2-2 mise embed.FS + ExtractMiseBinary：5 个 per-platform `embed_mise_<goos>_<goarch>.go` build-tag 文件 + 1 个 `embed_mise_unsupported.go` fallback（freebsd/linux-386 等返空 miseBinary）；D1-6 placeholder 替换为真实 `ExtractMiseBinary(ctx, dataDir, log)`——写到 `<dataDir>/sandbox/bin/mise` + chmod 0755 + darwin ad-hoc codesign（沿用 preflight.go::macCodesign）+ SHA256 hash 文件幂等。3 单测全绿（embed sanity / happy path / "binary 删除但 hash 残留" 恢复路径）。6 平台 cross-compile 全过（含 freebsd unsupported fallback） |
| 2026-05-05 | **[feat]** D2-3a mise generic Installer + Python EnvManager：`installer_mise.go` 通配 RuntimeInstaller（python/node/rust/go/java/ruby/php/...）共享 `<sandboxRoot>/mise-data/` MISE_DATA_DIR + `mise install -y` + 用 `mise where` 解析实际 install path（处理 `3.12` → `3.12.5` 部分版本约束）；`envmanager_python.go` 用 `uv venv --python` + `uv pip install --python` 建 venv + 装 deps（uv hardlink wheel 缓存让多 env 共享磁盘）。**RuntimeInstaller 接口签名调整**：`Install(ctx, version, dest, stream) error` → `Install(ctx, version, sandboxRoot, stream) (relPath string, err error)`，因 mise 用全局 MISE_DATA_DIR 不接受任意 dest——sandbox.md §7-8 同步改。6 pure-function 单测；真 install/venv 由 D9 pipeline 测试覆盖（避免单测下载几十 MB python） |
| 2026-05-05 | **[doc]** D2-3 范围澄清：sandbox.md §7 加详细的 v1 EnvManager 矩阵 + 实施顺序——**全部 11 个 EnvManager + 4 个 Installer 进 D2，不延后到 v2**（Python ✅ + Node + Rust + Java + Go + Ruby + PHP + Playwright + dotnet + Static + Generic fallback；Mise + Playwright + dotnet + Static 4 个 Installer）。投资人承诺。Java 选**方案 A**（每 env 独立 Maven local repo，跟 venv 哲学一致；磁盘大但 v1 demo 不显著）。D2-3 拆为 a/b/c/d/e/f 6 子任务，按消费方+复杂度分组逐 commit |
| 2026-05-05 | **[feat]** D2-3b Node + Playwright 全栈：`envmanager_node.go`（pnpm + 写最小 private package.json + content-addressable global store 让多 env 共享 npm 包磁盘）+ `installer_playwright.go`（`playwright install <browser>` + 全 env 共享 `<sandboxRoot>/playwright-browsers/` 缓存避免重复下 ~300 MB Chromium）+ `envmanager_playwright.go`（委托 Node CreateEnv/InstallDeps；InstallExtras 跑浏览器下载）。3 个文件 ~440 行 + 11 单测全绿（Kind / package.json 写出 + 幂等 / EnvBin Windows 后缀 / 浏览器频道列表 / 共享缓存路径 / Node 委托）。真 npm + Playwright 网络 install 推到 D9 pipeline 套 |
| 2026-05-05 | **[feat]** D2-3c Generic + Static binary：`envmanager_generic.go`（兜底 EnvManager；mkdir 后 InstallDeps/InstallExtras no-op；给 mise 长尾 600+ 语言 Erlang/Elixir/Lua/Zig/Deno/...）+ `installer_static.go`（HTTP GET URL → 写 `<sandboxRoot>/static-binaries/<kind>/` → chmod 0755 → darwin macCodesign；version 支持 `sha256:<64hex>@<URL>` 格式做下载校验；给 GitHub MCP 等纯静态二进制 plugin 用）+ `envmanager_static.go`（CreateEnv mkdir，InstallDeps/Extras no-op；EnvBin 反查共享 binary 路径）。13 单测全绿（含 parseStaticVersion 6 子用例）|
| 2026-05-05 | **[feat]** D2-3d Rust + Go：`envmanager_rust.go`（CARGO_HOME=<env>/.cargo + `cargo install --root=<env>` 编到 `<env>/bin/`；目标是 binary CLI 工具不是库 deps）+ `envmanager_go.go`（GOPATH=<env>/gopath + GOBIN=<env>/bin 跑 `go install <pkg@ver>`；同样针对 binary 工具）。两者都"每 env 独立 cache"——v2 视磁盘压力可考虑共享 GOMODCACHE（Go content-addressable 干净；Cargo 不行）。7 pure-function 单测全绿 |
| 2026-05-05 | **[feat]** D2-3e Java + Ruby + PHP：`envmanager_java.go`（方案 A：MAVEN_OPTS=-Dmaven.repo.local=<env>/m2 per-env Maven local repo + `mvn dependency:get -Dartifact=<GAV>`；EnvBin 故意只返 binName 不前缀 envPath，让调用方经 JDK PATH 解析）+ `envmanager_ruby.go`（BUNDLE_PATH=<env>/bundle + 写最小 Gemfile + `bundle add <gem>`；EnvBin 返 bundle/bin/）+ `envmanager_php.go`（composer --working-dir=<env> + COMPOSER_HOME=<env>/.composer + 写最小 composer.json + `composer require <pkg> --no-interaction`；EnvBin 返 vendor/bin/）。9 pure-function 单测全绿 |
| 2026-05-05 | **[feat]** D2-3f .NET（D2-3 收尾）：`installer_dotnet.go`（unix 走 dotnet-install.sh，Windows 走 PowerShell + dotnet-install.ps1，不走 mise——sandbox.md §4 明确"v1 单独走专用 installer"；脚本运行时拉，不预 embed；--install-dir=<sandboxRoot>/dotnet-installs/<version>/）+ `envmanager_dotnet.go`（写最小 env.csproj + nuget.config 钉 `./packages` 让 NuGet 包本地化 + DOTNET_CLI_HOME=<env>/.dotnet + `dotnet add package <pkg>`）。7 单测全绿。**D2-3 整组完成**——4 Installer + 11 EnvManager 全部进 v1（Mise/Playwright/Static/Dotnet 四 installer + Python/Node/Rust/Java/Go/Ruby/PHP/Playwright/Dotnet/Static/Generic 十一 EnvManager），投资人承诺达成 |
| 2026-05-05 | **[arch]** D2-4 part 1 — `domain/sandbox/tooling.go` 新加 **ToolRegistry 抽象**解耦 EnvManager 与"支持工具怎么装"。改 5 个 EnvManager（Python/Node/Java/Ruby/PHP）构造从持具体 binary path（uvBin / pnpmBin / mvnBin / bundleBin / composerBin）改为持 sandboxdomain.ToolRegistry，操作时 `tools.EnsureTool(ctx, "uv", "")` 懒解析。app/sandbox/Service 实现 ToolRegistry（EnsureTool 内部链 EnsureRuntime + Installer.Locate），main.go 把 service 自己注入作 registry。**好处**：(a) EnvManager 接口纯净不依赖具体实现细节；(b) 支持工具懒装让 boot 极快（首次用某 plugin 时才装该 lang 的支持工具）；(c) unit test 用 fakeToolRegistry 注入，无需真起 mise。sandbox.md §7 加 ToolRegistry 接口说明 + §7-8 main.go 装配示例改为分两步注册（RegisterInstaller / RegisterEnvManager），ToolRegistry 注入 |
| 2026-05-05 | **[feat]** D2-4 part 2 — `app/sandbox/sandbox.go` Service 落地：Bootstrap (extract mise + atomic.Bool ready) + IsReady + RetryBootstrap + BootstrapError（degraded mode 暴露给 HTTP）+ EnsureRuntime（per-kind 锁 + 双重检查）+ EnsureEnv（per-(ownerKind,ownerID) 锁 + deps drift 触发 destroy + rebuild）+ Destroy + EnsureTool（实现 ToolRegistry）+ ListRuntimes/ListEnvs/TotalDiskUsage 查询。`disk.go` 辅助：computeDirSize（best-effort 求和；失败返 0 不挡 install）+ removeAll（防灾难性路径如 "/" 守卫）。无外部 spawn 还（D2-4 part 3 来）|
| 2026-05-05 | **[feat]** D2-4 part 3a — 进程树管理 leak 防御加强（**D10 Job Object 提前到 D2**）：拆 `proc_unix.go` → `proc_linux.go`（Setpgid + **PR_SET_PDEATHSIG=SIGTERM** 让 child 在 Forgify 死时收 SIGTERM）+ `proc_darwin.go`（Setpgid，无原生等价靠层 A+B 兜底）；重写 `proc_windows.go` 用 **Job Object + JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE**（master Job 创建时 assign Forgify 自身，所有 child 自动继承；Forgify 死时 OS 强 kill 所有成员，**v1 平台集中最强 leak 防御**）。golang.org/x/sys/windows 转 direct dep。3 平台 cross-compile 全过。日常 ctx-cancel 仍用 SIGKILL/taskkill 不变 |
| 2026-05-05 | **[feat]** D2-4 part 3b — `infra/sandbox/spawn.go` 进程 spawn helper：`SpawnOnce(ctx, opts) → ExecutionResult`（一次性，跑完拿 stdout/stderr/exitCode；非零退出返 Ok=false 不上抛 Go error 让调用方传 LLM）+ `SpawnLongLived(ctx, opts) → LongLivedHandle`（长生命周期，stdio 管道接好返 handle 给调用方驱动 stdin/stdout/wait/kill）+ `SpawnOptions`（Cmd / Args / Cwd / Env / Stdin 全由调用方解析）。两者都套 setupProcessGroup + cmd.Cancel = killProcessGroup（接 ctx-cancel）。8 端到端单测全绿（echo / cat / sleep / 不存在 binary / ctx-cancel 真杀 sleep 30 → 100ms 内返）。infra 把 exec.Cmd 编排藏在两个简洁函数后，app 层不直接碰 exec |
| 2026-05-05 | **[feat]** D2-4 part 3c — `app/sandbox/spawn.go` Service.Spawn / SpawnLongLived + **层 A leak 防御**（active handle 注册表 + Service.Shutdown）：Service.Spawn 解析 Owner → env → EnvManager.EnvBin（裸 cmd 名翻译）/ EnvManager.EnvDir（cwd）/ os.Environ() + opts.Env overlay，调 sandboxinfra.SpawnOnce；SpawnLongLived 类似但返 trackedHandle 自动注册到 activeHandles，Wait/Kill 自动反注册。Service.Shutdown 并发 Kill 所有 active handle 等所有 done 或 ctx 过期。10 端到端单测全绿（happy / 非 ready / 空 cmd / owner mismatch / 绝对 cmd 跳 EnvBin / env overlay / handle 注册 + Wait 反注册 / Kill 反注册 / Shutdown 杀 3 个 / 空 Shutdown）。**层 A 完成**——投资人 demo macOS crash 时 leak 概率从"每次"降到"仅 SIGKILL Forgify 时"|
| 2026-05-05 | **[feat]** D2-4 part 3d — **层 B leak 防御 + D2-4 收尾**：Env struct 加 `RunningPID` + `RunningStartedAt` 列（GORM 显式 `column:running_pid` 防 NamingStrategy 把 PID 拆成 `p_id`）；Repository 加 SetEnvRunningPID / ClearEnvRunningPID / ListEnvsWithRunningPID 三方法 + 2 store 集成测试。`app/sandbox/restore.go` Service.RestoreOrCleanupOnBoot 启动扫 manifest，对 running_pid > 0 的 env 调 killIfAlive（unix 用 signal 0 探测 + p.Kill；windows 直接 p.Kill）+ ClearEnvRunningPID。Service.Bootstrap 成功后自动调 RestoreOrCleanupOnBoot。SpawnLongLived 启动后 SetEnvRunningPID(pid)，trackedHandle.unregister 同时清 manifest。3 端到端测试全绿（killStaleProcessAndClearsPID 真 spawn sleep 验证 p.Kill 落地 / NoOpWhenNoStalePIDs / HandlesAlreadyDeadPID）。**D2-4 完整收尾**——Bootstrap + Service + Spawn + 三层 leak 防御（A/B/C）全到位 |
| 2026-05-05 | **[refactor]** D2-5a forge sandbox 迁到新 service：`app/forge/sandbox_adapter.go` SandboxAdapter 满足 forge.Sandbox 接口，委托 sandboxapp.Service（Owner ID 用 `<forgeID>:<envID>` 拼合保 v1 N=3 EnvID buffer 行为）；adapter 拥有 main.py + driver 模板写盘（v2 sandbox 不管 forge 文件布局）；非零退出翻译为 Ok=false ExecutionResult 不上抛 Go 错（跟 v1 行为一致）。`SyncRequest` / `RunRequest` / `SyncError` / `ComputeEnvID` 从 sandboxinfra 包**挪到 forge 包内** —— forge.go 不再 import sandboxinfra（D2-5b 可删 v1 sandbox 文件）。main.go 装配段重写：sandboxstore + sandboxapp.New + Bootstrap + `registerSandboxStack` helper（注册 7 mise 主流 + 5 mise 支持工具 + dotnet + 11 EnvManager）；forge 用 `forgeapp.NewSandboxAdapter(sandboxSvc, dataDir)`。fakeSandbox in forge_test 也切到本包 types。所有 ~170 单测全绿 + 3 平台 cross-compile 通过 |
| 2026-05-05 | **[refactor]** D2-5b 清 v1 sandbox 残留：删 8 个 v1 文件（`sandbox.go` v1 Sandbox struct + `paths.go` + `preflight.go` + `sync.go` + `run.go` + `destroy.go` + `pyproject.go` + `progress.go`）+ 6 个 v1 测试。`macCodesign` 提到 `codesign.go` 加 `runtime.GOOS != "darwin"` 早返让所有平台编译干净（保留被 ExtractMiseBinary + StaticBinaryInstaller 用）。infra/sandbox 现仅 v2 内容（installer/envmanager/spawn/embed/proc/codesign）。**D2-5 整组完成** ——forge 完全切到 PluginSandbox v2，v1 代码 0 残留，test 全绿，3 平台 cross-compile 通过 |
| 2026-05-05 | **[feat]** D2-6 sandbox HTTP endpoints：`handlers/sandbox.go` 13 个端点（GET runtimes/envs/envs/{id}/disk-usage/bootstrap-status + POST envs/{id}:destroy/runtimes/{id}:destroy/:gc/:retry-bootstrap/runtimes:install + GET conversations/{id}/sandbox-envs + POST conversations/{id}/sandbox-envs/{kind}:reset + sandbox-envs:reset-all）按 sandbox.md §11 表实施；`:action` 风格用 PathValue + splitAction 派发。Service 加 GetEnv / DeleteRuntime（先检查无 env 引用否则返 ErrEnvInUse=409）/ GC（用 ListEnvsLastUsedBefore 删旧 env，olderThanDays 默认 30）。router/deps 加 SandboxService 字段，main.go 装配 sandboxSvc。`MarkReadyForTest` / `ActiveHandleCountForTest` 从 export_test.go 挪到生产 sandbox.go（带 `ForTest` 后缀 godoc 警告生产禁用）让 cross-package handler 测试可调。dataSlice helper 加 apikey_test.go 共享。12 端到端 httptest 全绿（ListRuntimes empty/seeded / ListEnvs requires ownerKind / GetEnv 404+200 / DiskUsage / BootstrapStatus / RuntimeDestroy 409 conflict / GC default / RetryBootstrap returns status / ListConvEnvs 前缀过滤）|

#### Tool 自检 batch 1 — 3 个真 bug（2026-05-05）

| 日期 | 内容 |
|---|---|
| 2026-05-05 | **[fix]** Tool 自检 batch 1 修 3 bug：(1) `grep_stdlib.go` line-mode 多行连续匹配时把 match 行错标为 context（`-`）——镜像 multiline 版本预算 matchLines map 跳过；(2) `web/fetch.go` `fetchClient` 加 `CheckRedirect`——不加守卫则公网 URL 302 到 loopback 能绕过 SSRF；(3) `shell/bash.go::runForeground` 加 `context.Canceled` 分支——父 ctx 取消时不再误报 "exec failed: signal: killed"。+7 回归测试，全套绿 |
| 2026-05-05 | **[doc-fix]** Tool 自检 batch 2 — chat.md 全量同步上周末 tool 改动留下的 drift：§4.4 工具表扩到 20 个 + 加家族级设计要点；§5.1/5.2/5.3/5.4/5.5 把 IsConcurrencySafe 分批改为 execution_group；writeDB 全 → writeAndPublish；旧事件类型（ChatToolCallStart/ChatToken/ChatReasoningToken/ChatDone/ChatError）改为 chat.message 单事件；§10.1 `agentapp.Tool` → `toolapp.Tool`；§10.2/§10.3/§11.1/§11.2/§13 `chat.error` SSE 改为 status=error 的 chat.message stub；§15 实现清单 `app/agent/*` 全删，重写为 `app/tool/{forge,filesystem,search,web,shell,task,ask}/` 七家族子包 + main.go 装配链 |
| 2026-05-05 | **[doc-fix]** Tool 自检 batch 3 — CLAUDE.md 同步 tool 体系演化：§S15 ID 前缀清单删 `frh_`/`fth_`（Phase 5 已合并到 forge_executions/`fe_` 表）+ 加 `fe_`/`b_`/`tk_`/`bsh_` 四个新前缀 + 注明 `pkg/idgen.New(prefix)` 是统一实现；§S18 加 §8 静态元数据 3 字段对照表（覆盖全 20 个工具 + Bash 故意不走 PathGuard 的 trade-off 注脚 + must-Read-first/RequiresWorkspace 是文档性元数据非框架强制的明示） |
| 2026-05-05 | **[fix/security]** Tool 自检 batch 4 — PathGuard 跨平台覆盖 + 包注释明示局限。`DefaultDenyList` 原偏 macOS（Linux/Windows 用户大量关键路径无保护），现加：Linux runtime/secrets（`/proc/`/`/run/secrets/`/`/var/run/secrets/`/`/sys/class/`）、Windows 系统 + 凭据库（`C:/Windows/`、`~/AppData/.../Microsoft/{Credentials,Crypto,Protect,Vault}/`）、跨平台浏览器 Login Data（Chrome/Edge macOS+Linux+Windows 三处）、kubectl + docker config。**包注释加局限段**——明示 Bash 故意不走 PathGuard（`bash cat ~/.ssh/id_rsa` 能成功），PathGuard 是 file-tool 防 LLM 手滑的护栏，不是 LLM 跑 shell 的安全边界。+4 回归测试（Linux runtime / Windows credential store / browser logins / kube+docker config），全套绿 |
| 2026-05-05 | **[doc]** Tool 自检 batch 5A — 新建 5 份 tool 家族 service-design doc 对照 task.md 模式：`filesystem.md`（Read/Write/Edit + PathGuard + must-Read-first + AgentState.SeenFiles）/ `search.md`（Grep rg+stdlib 双后端 + Glob 替 LS 决策 D3）/ `web.md`（WebFetch Jina+直 GET + WebSearch SearXNG/Bing/Bing CN 三层 + SSRF 守卫含逐跳重定向校验）/ `shell.md`（Bash 前后台双模式 + cd 状态机 + ProcessManager 256KB 环形缓冲 + Bash 不走 PathGuard 的 trade-off）/ `ask.md`（in-memory rendezvous + 决策 D11 不新建事件家族 + 原子摘条目防双答竞态）；task.md §10 由 ~80 行 ask 详细设计压缩为指向 ask.md 的指针。完整方案见各文件 |
| 2026-05-05 | **[cleanup]** Tool 自检 batch 6 — 收尾 5 条小 nit：(1) `tool/filesystem/read.go` + `tool/task/task.go` 加 `var _ toolapp.Tool = (*X)(nil)` 编译期接口断言（5 个工具补齐，全 20 工具一致）；(2) `app/task/task.go` 删 `_ = errors.Is; _ = idgenpkg.New` 死代码 + 同步删 `errors` 死 import；(3) 重命名 `TestService_Resolve_DoubleAnswerIsErrAlreadyAnswered` → `_IsErrNoPendingQuestion` 反映 atomic-pop 后的契约；(4) `shell/bash.go::runBackground` 加显式注释解释 `context.Background()` 是切断 ctx 的设计意图；(5) `error-codes.md` Phase 5 段加 NB 注脚明示 fs/search/web/shell 不进 errmap（错误以友好字符串走 tool_result）。`go build` + `make test-unit`（零 FAIL）+ staticcheck（0 警告）三件套全绿 |

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
