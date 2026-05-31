# Forgify — Claude 工作守则

> Claude Code 进入本项目自动加载本文件。**本文件是项目工程纪律的唯一事实源**。
> 项目愿景 / 架构 / Phase 路线 / Verification 见 [`docs/concepts/architecture.md`](docs/concepts/architecture.md)。

---

## 项目一句话

- **本地优先 Agentic Workflow Platform**，目标 **Wails 桌面 app**（不做 SaaS）
- **单人项目**；后端 Phase 0-4 完成、Phase 5（智能化）部分交付（document / mcp / skill / memory / compaction ✅；intent / chat 终极版未做）；**当前重心：前端**（V1.2 桌面 app）
- 架构：**4 层 clean arch**，依赖方向 `transport → app → (domain ∪ infra/store) → infra/db`

## 文档地图

| 用途 | 路径 |
|---|---|
| 项目愿景 / 架构 / Phase 路线 | `docs/concepts/architecture.md` |
| 当前进展 / 决策日志 | `docs/references/changelog.md` |
| 各 domain 详设计 | `docs/references/backend/domains/<domain>.md` |
| 契约索引（API / DB / Error / Events） | `docs/references/backend/` |
| 桌面端 / 调试控制台 | `docs/archive/desktop-packaging-notes-2026-05/` |
| testend 子项目工程纪律 | `testend/CLAUDE.md` |
| testend V3 设计 | `docs/working/testend/` |
| 前端 PRD（产品需求 / UI 细节） | `docs/concepts/frontend-prd.md` |
| 前端 FSD 层契约索引 | `docs/references/frontend/fsd-layers.md` |
| 前端 entity TS 类型 ↔ 端点映射 | `docs/references/frontend/entity-types.md` |
| 前端横切机制（DIP / SSE / errorMap） | `docs/references/frontend/cross-cutting.md` |
| 前端各 slice 详设计 | `docs/references/frontend/slices/<slice>.md` |
| 架构决策记录（ADR） | `docs/decisions/README.md` |
| 文档导航（AI session 入口） | `docs/INDEX.md` |

## 改代码前必做

1. 读对应 `docs/references/backend/domains/<domain>.md`
2. 改完跑 `make test-backend`（单测）+ `cd backend && go build ./... && staticcheck ./...`（编译 + 静态）
3. 同步联动文档（§S14）

---

# 设计原则（7 条，#7 最高优先级）

1. **每个 Phase 独立交付价值** — 不会出现"做了 80% 但啥都用不了"
2. **依赖严格自下而上** — 每个 Phase 只依赖前面已完成的
3. **复杂度阶梯式增长** — CRUD → 编排 → 智能
4. **前后端分阶段、不并行开发** — 后端 Phase 0-4 已交付定型，现转入前端阶段（见末节"前端开发守则"）；后端如需改动走 curl / testend / `make test-backend` 验证
5. **端到端推演先行** — 开工前必走完整数据流 + 列出跨域依赖，**不走不开工**
6. **反校验剧场** — 本地单用户同人写前后端；只保留有价值的校验（JSON 畸形、必填非空、NotFound、DB CHECK/UNIQUE）；加校验前问："前端能防住？下游自然炸？"两者都是则不加
7. **📌 文档与代码同步（最高优先级）** — 每个代码改动必须伴随文档更新（见 §S14）。**文档落后于代码 = bug**

## 端到端推演模板

```
触发源（HTTP/定时/事件）
  → transport 层：哪个 handler
    → app 层：哪个 service 方法（调谁 / 从 ctx/repo 读什么）
      → infra 层：最终落到哪里（DB / 外部 API / 沙箱）
  → 响应路径：成功 / 失败分别怎么返
```

---

# Standards — 契约宪法 + S/T 系列

## HTTP API（N 系列）

- **N1** 成功 `{"data": ...}`；失败 `{"error": {"code","message","details"}}`
- **N2** 200 读/更新 / 201 创建 / 204 删 / 400 参数错 / 404 不存在 / 409 冲突 / 410 淘汰 / 422 业务拒绝 / 500 内部错
- **N3** API 请求/响应 camelCase；DB 列 snake_case，repo 层转换
- **N4** 列表强制分页：`?cursor=xxx&limit=50` → `{data, nextCursor, hasMore}`
- **N5** 资源用名词；状态改动走 `PATCH`；动词用 `:action` 后缀。**`:iterate`**（AI 编辑对话）和 **`:triage`**（AI 调试对话）是标准 action 名，分别挂在 function/handler/workflow/document 和 flowrun 上，统一返 `{conversationId}`（201）
- **N6** upsert 类 PUT 无论新建/更新一律返 200
- **N7** SSE：`event:` + `id:` + `data:` 格式；Last-Event-ID 重连；超 buffer 返 410 `SEQ_TOO_OLD`。详见 `docs/references/backend/events.md`

## 数据库（D 系列）

- **D1** 软删除：所有表用 `deleted_at DATETIME`（NULL = 未删除）
- **D2** 每表必有 `created_at` / `updated_at`，GORM 自动维护
- **D3** 稳定白名单用 DB CHECK；会扩张的白名单在 app 层校验
- **D4** `foreign_keys=ON`；V1.2 暂不声明 GORM FK tag，app 层管 lifecycle
- **D5** 业务唯一性用 UNIQUE 约束
- **D6** AutoMigrate 表达不了的 SQL 走 `schema_extras.go`（幂等 + `IF NOT EXISTS`）
- **D7** 普通/复合/UNIQUE 索引走 GORM tag；带 `WHERE` 的 partial 索引才进 schema_extras

## SSE（E 系列）

**E1 三协议**（全部 per-user_id 订阅；**SSE 上限三条，永不再加**）：
- **事件日志** `GET /api/v1/eventlog`：5 events × 7 block types（text/reasoning/tool_call/tool_result/progress/message/compaction）；payload 带 `conversationId` + `seq`
- **通知** `GET /api/v1/notifications`：envelope `{type, id, data, conversationId?}`；data 只送轻字段（禁完整 entity）；开放词表
- **锻造流** `GET /api/v1/forge`：4 events × 3 kinds（function/handler/workflow）；payload 嵌 `scope:{kind,id}`；封闭枚举

**E2 协议演进**：事件日志/锻造流新 type 先改对应协议文档再加 code（封闭枚举）；通知新 type 加字符串即可

## 代码规范（S 系列）

- **S3** 不吞错误；`_ = err` 必须带注释；严禁静默跳过
- **S5** 文件 ~250-500 行、函数 ~60 行为烟雾报警；可读性优先；超长伴随职责模糊才拆
- **S6** handler 只做"解 JSON → 调 service → 写 envelope"；业务逻辑出现在 handler 才是真违规
- **S8** SQL 只在 `infra/store/` 和 `infra/db/`
- **S9** 每个跨层调用传 `ctx`；终态写入用 `reqctxpkg.SetUserID(context.Background(), uid)` detached context
- **S10** 结构化日志用 zap；同步原语不打 log；异步/fire-and-forget 必须打
- **S11** 注释规范 — 见 §S11
- **S12** 包结构 — 见 §S12
- **S13** 包命名 — 见 §S13
- **S14** 📌 文档同步纪律 — 见 §S14（**最高优先级**）
- **S15** ID 格式：`<prefix>_<16hex>`（`crypto/rand` 8 字节，失败 panic）。前缀（trinity 后实测全集，按域分组）：`u_` user / `aki_` apikey / `mc_` model-config / `mco_` model-cap-override；`cv_` conv / `msg_` message / `blk_` block / `att_` attachment；`fn_` function / `fnv_` function-version / `fne_` function-exec / `fnenv_` fn-venv；`hd_` handler / `hdv_` handler-version / `hcl_` handler-call / `hdi_` handler-instance / `hdenv_` hd-venv；`wf_` workflow / `wfv_` workflow-version；`fr_` flowrun / `frn_` flowrun-node；`mcl_` mcp-call / `mch_` mcp-health / `ske_` skill-exec；`doc_` document / `mem_` memory / `rel_` relation / `td_` todo；`sr_` sandbox-runtime / `se_` sandbox-env / `bsh_` bash-shell。（skill / mcp 实体以 name 作主键、无实体前缀；`ske_`/`mcl_` 是其执行/调用子记录 ID）
- **S16** 错误包装：`fmt.Errorf("<pkg>.<Method>: %w", err)`；`errors.Is` 必须能从最外层 unwrap 到 sentinel
- **S17** 每个到达 handler 的 sentinel 必须登记到 `errmap.go::errTable`
- **S18** Tool 接口规约 — 见 §S18
- **S19** Dev log 节制：1-2 句 ~30-100 汉字；超 200 字拆成多条；设计内容禁入 dev log
- **S20** 📌 禁止无理由"留下次"：声明延迟必须满足 (a) 结构性硬约束 + (b) 当场清晰说明前置条件和爆发场景
- **S21** 事件流 invariants：`parentId` 不悬空；`block/message.status` 单向流转；`seq` 严格单调递增；deltas append-only

## 测试（T 系列）

- **T1** 命名：`Test<Function>_<Scenario>`，Scenario 描述条件不描述动作
- **T2** DB 测试用 `dbinfra.Open(dbinfra.Config{DataDir: ""})` in-memory SQLite
- **T3** 外部依赖用 env gate + `t.Skip`；禁 hardcoded fallback key
- **T4** 删导出符号先 grep `_test.go`；测试专用导出加注释说明
- **T5** 大功能模块完工必加 `backend/test/<domain>_pipeline_test.go`，用 `harness.New(t)` 端到端；幂等硬要求（新内存 SQLite 天然隔离；不得依赖前次残留）
- **T6** pipeline 默认 fake LLM；真 LLM 测试加 `Live_` 前缀 + `RequireDeepSeekKey(t)` gate

---

# §S11 注释规范

**只写为什么，不写是什么。**

格式：英文一行 + 空行 + 中文一行，每种语言不超过 ~80 字符；双语凝练同一意思（非互译）；密度上限 1/3。

写：类/接口职责、public 方法用途、方法内部的坑/约束。
不写：private 方法、getter/setter、字段声明、代码语义重复、被注释掉的旧代码、章节横幅 `// ── X ──`。

---

# §S12 包结构

- **按概念/feature 拆文件，禁按 kind 拆**（不要 types.go + errors.go + constants.go）
- **主文件用包名**（`apikey.go`，不叫 `service.go` / `store.go`），三层（domain/app/store）统一
- **平铺到包根，禁子目录**；拆子包需同时满足：(a) 有独立词汇体系 + (b) 10+ 文件
- **例外**：`app/tool/` 允许按家族嵌套（forge / filesystem / shell / web）
- **例外**：`backend/test/` 按测试 axis 嵌套子目录（api / cross / sse / lifecycle / errcodes / live / smoke）；每子目录独立词汇体系 + ≥ 文件阈值，满足拆子包标准
- 跨 domain 纯函数放 `internal/pkg/<name>/`
- `providers.go` 等辅助注册表放**消费它的层**（domain 自用放 domain；仅 app 消费放 app）

---

# §S13 包命名（三层同名 + 调用方别名）

**所有从 `internal/` 导入的包必须使用别名** `<name><role>`，无别名视为违规。

| 目录 | 后缀 | 示例 |
|---|---|---|
| `internal/app/<name>/` | `app` | `apikeyapp`, `chatapp`, `convapp`, `relationapp`, `askaiapp` |
| `internal/domain/<name>/` | `domain` | `apikeydomain`, `chatdomain`, `errorsdomain`, `relationdomain` |
| `internal/infra/<name>/`（非 store）| `infra` | `llminfra`, `dbinfra`, `loggerinfra`, `sandboxinfra`, `chatinfra` |
| `internal/infra/store/<name>/` | `store` | `apikeystore`, `chatstore`, `convstore`, `relationstore`, `mcphealthstore` |
| `internal/pkg/<name>/` | `pkg` | `reqctxpkg`, `paginationpkg`, `wikilinkpkg` |
| `internal/transport/httpapi/<name>/` | `httpapi` | `responsehttpapi`, `middlewarehttpapi`, `routerhttpapi` |
| `internal/app/tool/<sub>/` | `<sub>tool` | `forgetool`, `fstool`, `shelltool`, `webtool` |

包内三层都用 domain 单名（`package apikey`）。接口定义在 domain 层；跨 domain 消费只通过 port 接口，禁止暴露 entity。

---

# §S14 📌 文档同步纪律（最高优先级）

**文档落后于代码 = bug。后端遵守本节；前端遵守 §F1（同等级别）。**

## 触发表

| 代码变动 | 必改文档 |
|---|---|
| 新 entity / 表 / struct 字段 / 约束 | `<domain>.md` + `database.md` + `changelog.md` |
| 新 sentinel / errmap 行 | `<domain>.md` + `error-codes.md` + `changelog.md` + `errcodes/sweep_pipeline_test.go` 加 sweep case + `// covers: errcode:CODE` annotation + `make matrix` |
| 新 endpoint / path 变 | `<domain>.md` + `api.md` + `changelog.md` + 对应 `api/<domain>/<domain>_pipeline_test.go` 加测试 + `// covers: METHOD /path` annotation + `make matrix` |
| 新事件 / struct 改 | `<domain>.md` + `events.md` + `changelog.md` + `sse_truth.go` 加枚举（如属 SSE 协议）+ `// covers: sse:<stream>:<event>` annotation + `make matrix` |
| 新跨 domain 锈川 | `<domain>.md` + `seams.yaml` 加 id + `cross/<file>_pipeline_test.go` 加测试 + `// covers: cross:<id>` annotation + `make matrix` |
| 方法签名 / 接口变 | `<domain>.md`（影响对外入口才动 contract 文档）|
| 新跨 domain 依赖 | `<domain>.md` + 受影响的 `<other>.md` |

## 子任务完成 checklist

- [ ] `<domain>.md` 实现清单勾 ✅
- [ ] 改了 API/schema/error → contract 文档对应行更新
- [ ] `changelog.md` 加 dev log（含做了什么 + 测试数 + 新规范/决策）
- [ ] 新规范/原则变动 → 加到本文件

## domain 完工 checklist

- [ ] `<domain>.md` 整体逐字段匹配代码
- [ ] `docs/references/backend/*.md` 该 domain 行从 ⬜ 改 ✅
- [ ] `changelog.md` 完工日志
- [ ] 新跨域模式 → 更新 `architecture.md`

发现文档与代码不符 → **立刻停下修文档**，记 `[doc-fix]` dev log。

---

# §S18 Tool 接口规约

9 个方法全必填，无 BaseTool 嵌入：

```go
type Tool interface {
    Name() string; Description() string; Parameters() json.RawMessage  // Identity
    IsReadOnly() bool; NeedsReadFirst() bool; RequiresWorkspace() bool  // 静态元数据（文档性）
    ValidateInput(args json.RawMessage) error
    CheckPermissions(args json.RawMessage, mode PermissionMode) PermissionResult
    Execute(ctx context.Context, argsJSON string) (string, error)       // argsJSON 已剥除标准字段
}
```

**3 个标准注入字段**（`injectStandardFields` 注入 slim shells；framework 自动剥除；Parameters() 不得包含这三个名称）：
- `summary` string 必填 — LLM 一句话描述本次调用；slim shell: `{"type":"string","description":"One sentence: what you're doing and why."}`
- `destructive` bool — LLM 自报是否不可逆；UI 显示警示；slim shell 指向 `tool_conventions` 段
- `execution_group` int — 同 group 并行；不同 group 按升序串行；缺失 = 独自串行（排在显式 group 之后）；slim shell 指向 `tool_conventions` 段

**三字段 slim shells 保留在每把工具 schema**；长 guidance 移至 system prompt 的 `tool_conventions` 段（讲一次）—— 省 ~13k token。

**钩子链**：`ValidateInput` → `CheckPermissions` → `Execute`（任一失败转 tool_result 错误）

**推流**：Tool 内优先用 `eventlogpkg.From(ctx)` Emitter（自动继承 parentBlockId；fallback 到 no-op）。

**Resident / Lazy Toolset 模型**（能力披露重构后）：
- `toolapp.Toolset{Resident []Tool, Lazy map[string][]Tool}` — main.go 装配，`chatService.SetToolset(ts)` 注入。
- `Resident`（~28 把）：高频通用工具；每轮始终在 `req.Tools`。
- `Lazy`（6 组：function/handler/workflow/mcp/document/skill）：按需；`activate_tools(category)` 激活后写 `AgentState.ActivatedGroups`。
- `loop.Run` 每步调 `host.Tools(ctx)` 重算（Resident + 已激活 Lazy 组），步内 `byName` 与 offer 集严格一致。
- `activate_tools` 是 RESIDENT meta-tool，始终可见；`app/tool/toolset/activate.go`。

子包结构（§S12 例外）：`app/tool/{forge,filesystem,shell,web,toolset}/`，别名 `forgetool` / `fstool` / `shelltool` / `webtool`。

---

# 开发期工具纪律

- **禁开 git worktree**：所有 session 一律在 main 直接干（含 Claude Code 的自动 worktree 隔离）。理由：曾出现 worktree 改动跑不到主仓库的 vite dev server、定位半天才发现是 file path 不同。需要隔离就用 `git stash` / 精细 `git add`；写代码前先 `git status` 看清状态。
- **`staticcheck ./...` 提交前必跑**（比 `go vet` 严，能捞 SA1029/S1016/U1000）
- **`deadcode` 默认不扫测试**：跑时加 `-test=true`；曾误删 `ListProviders` / `ListScenarios`
- **staticcheck 用 `//lint:ignore <code> <reason>`**（不认 `//nolint`）
- **禁用 sed 批量改 import/函数名**：BSD sed `\b` 不识别，会清空文件；用 Edit 工具
- **`make audit` 提交前必跑**（已在 `make verify` 内）：矩阵新鲜 + warn-only 报漂移（未来切 strict）
- **`make mock` 日常 driver**（`make verify` 已含）：~60s pipeline 测试，零外部依赖、零 token

## 测试命令（不要直接 `go test ./...`）

| 命令 | 用途 |
|---|---|
| `make unit` | **默认 Go 单测**：in-memory SQLite，skip `TestIntegration_*`（= `cd backend && go test ./... -skip TestIntegration_`）|
| `make web` | 前端 vitest 单测 |
| `make test` | unit + web 聚合 |
| `make mock` | Pipeline fake LLM 测试（~60s，离线，零 token）|
| `make sandbox` | mock + 真 sandbox lifecycle（要 `FORGIFY_DEV_RESOURCES`）|
| `make live` | only 真 LLM 测试（要 `DEEPSEEK_API_KEY`，烧 token）|
| `make e2e` | full pipeline：mock + sandbox + live（release gate）|
| `make matrix` | 生成 README 覆盖矩阵段 |
| `make audit` | 矩阵严格检查（verify 内部调用）|
| `make verify` | 发布门禁：vet × 5 平台 + build × 5 + lintprompts |
| `make testend` | 起 testend 调试控制台（pre-frontend 时代遗留）|

> `make` 在仓库**根**跑（不在 `backend/`）。`staticcheck ./...` 在 `backend/` 内直接跑（非 make 目标）。

---

# 项目特殊性

- **单用户本地 + 同人写前后端** → 校验少、便利优先
- **已摆脱 Eino**：自有 LLM 客户端 `infra/llm`（OpenAI-compat + Anthropic 原生）
- **`infra/llm` 架构（2026-05-30 R1-R5 重构，最终状态）**：`Provider` 接口（`Name/DefaultBaseURL/BuildRequest/ParseStream`）+ 共享传输铁律（`transport.go`，120s `*http.Client` + `doRequest` + `classifyHTTPError`）+ `providerRegistry`。每个 provider 完全自包含：openai / deepseek / qwen / zhipu / moonshot / doubao / openrouter / ollama / custom 各自拥有完整 `BuildRequest`（含 thinking 编码 + auth 头）和 `ParseStream`，逻辑写到各家官方 API 标准；anthropic 和 gemini（原生 generateContent）讲各自原生方言。**共享 `openAICompatProvider` 已删除**（R5）：不再有跨 9 家共用一份 body/SSE 逻辑的设计。`factory.Build` 按 provider / APIFormat 路由到 registry（`lookupProvider`）。
- **`pkg/modelcaps`（2026-05-30）**：per-(provider, model) ability catalog（thinking shape + context window + max output），按 family 规则 + per-model 精确行覆盖，替代已删的 `pkg/modelmeta`。`CapabilityService.ResolveCapabilities` 合并：user override（`model_cap_overrides`）> 静态规则。详 `docs/working/llm-providers/04-capability-catalog.md`。
- **Gemini 走原生 `generateContent`（R4，2026-05-30）**：`gemini.go` `geminiProvider` 自有标准 —— base `…/v1beta` + `/models/{model}:streamGenerateContent?alt=sse`（model 在 URL 路径）+ `x-goog-api-key`；contents/parts、systemInstruction、tools.functionDeclarations、generationConfig.thinkingConfig；能读回 reasoning 文本（`thought:true` parts）+ round-trip `thoughtSignature`（Gemini-3 多轮工具循环必需）。functionResponse 按**函数名(+id)** 配对（从前序 tool_call 反查名字）。OpenAI-compat shim 已删。**Live capability overlay deferred**（从 provider API 自动拉取能力）：静态规则 + user override 已满足当前需求，接口位置预留。
- **modernc.org/sqlite** 纯 Go；DSN 用 `_pragma=...` 语法；跨平台 build 一行命令
- **桌面端**：Wails 窗口外壳 + 复用 httpapi（不走 Wails native binding）
- **三条 SSE**：eventlog（5×7）+ notifications（全局 entity 变更）+ forge（4×3）；`domain/events` 已删
- **subagent 数据**：统一 `messages` 行（attrs.kind=subagent_run），无独立表
- **sandbox v2**：捆绑 mise binary（`go:embed`）；`make resources` 拉到 `mise/<goos>-<goarch>/`；v1 已废弃
- **测试基线**：`make verify` 绿（vet×5 + build×5 + lintprompts + matrix audit + pipeline mock 全 16 包绿）；`make e2e` = mock + sandbox + live 全套（release 前跑，烧 token）；详 `backend/test/README.md`
- **测试 axis**：api / cross / sse / lifecycle / errcodes / live / smoke 七维度；axis 定义 + 内容 inventory 见 `backend/test/README.md`
- **覆盖矩阵**：`make matrix` 自动生成 README 矩阵段（消费 `// covers:` annotation）；`make audit` 严格检查（Phase 5 起 warn-only，覆盖率达高位后切 strict）；工具 `backend/cmd/coverage-matrix/`

---

# 前端开发守则（前端阶段 — 现已生效）

> 前端 PRD 全文见 [`docs/concepts/frontend-prd.md`](docs/concepts/frontend-prd.md)。
> FSD 层契约索引见 [`docs/references/frontend/fsd-layers.md`](docs/references/frontend/fsd-layers.md)。
> 本节是工程纪律唯一事实源，与 PRD 不矛盾时以本节为准（本节更简洁）。

## 权威文档地图

| 问题类型 | 去哪里找答案 |
|---|---|
| 层定义 / slice 清单 / 依赖规则 | `docs/references/frontend/fsd-layers.md` |
| 12 entity TS 类型 ↔ 后端端点 | `docs/references/frontend/entity-types.md` |
| DIP / errorMap / SSE / queryKeys | `docs/references/frontend/cross-cutting.md` |
| 各 slice 详细设计 | `docs/references/frontend/slices/<slice>.md` |
| 数据从哪来、接哪个 API、SSE 如何处理 | PRD §5–§7、§9–§14、§17 |
| 动效用什么参数 | PRD §3.2 |
| 视觉细节（class 名 / CSS） | 已实现的 `frontend/src`（组件 + `src/styles/`）—— 实现即视觉事实源 |

---

## §FSD FSD 6 层宪法（完整 Feature-Sliced Design）

### 层定义

依赖**严格单向自上而下**（上层 import 下层；下层永不知上层；同层 slice 默认不互引）。

| FSD 层 | 职责 | 后端对位 | 可 import |
|---|---|---|---|
| **`app`** | 应用组装：入口、providers、全局 store、SSE 单例、identity boot、主题 | `transport` 组装 + `main.go` wire | 全部下层 |
| **`pages`** | 完整屏幕（一个 pane = 一个 page）；零业务，只读 hook → 渲染 → 调 mutation | HTTP handler（路由式入口） | widgets / features / entities / shared |
| **`widgets`** | 自包含组合 UI 块（组合多个 feature / entity） | 无直接对位（组合层） | features / entities / shared |
| **`features`** | 用户用例 / 交互（带业务价值）；用例 hook 在 `model/` | `app/service`（用例层） | entities / shared |
| **`entities`** | 单个业务实体（数据访问 + 模型 + 展示卡）；fetch 只调 `shared/api` | `domain`（实体层） | shared（+ `@x` 跨 slice） |
| **`shared`** | 零业务：传输底座、UI kit、工具函数 | `infra` + `pkg` | 仅自身 |

**后端对位精确性**：
- `pages` 对应后端 handler（只解参数 → 调用 → 渲染），零业务
- `features/model/` hook = 后端 `app/service`（用例编排）
- `entities/api/` hook = 后端 `infra/store`（薄数据访问）
- `shared/api/httpClient` = 后端 transport 的底层（唯一发 fetch 的地方）
- `entities/<x>/index.ts` barrel = 后端 domain port（只暴露公开契约）

### 依赖规则

```
app → pages → widgets → features → entities → shared
```

- 反向依赖（下层引上层）= 违规，`steiger` + `eslint-plugin-boundaries` 报错
- 越级 import（如 features 直接引 app）= 违规
- 深引 slice 内部文件（绕过 `index.ts`）= 违规；只准 `import { Foo } from "@/entities/conversation"`

### Slice Public API（index.ts barrel）

每个 slice **必须有 `index.ts`**，外部只准 import 该文件，不准深引内部路径。= 后端 port"不暴露 entity 内部"。

```
entities/conversation/
  ├── api/           (useConversations / useSendMessage)
  ├── model/         (chatStore / types.ts)
  ├── ui/            (ConversationCard)
  └── index.ts       ← 唯一对外出口
```

### DIP 注入模式（解 shared → 上层反向依赖）

`shared` 不可依赖上层（FSD 铁律）。横切关注点用控制反转——结构与后端 `domain` 定 port / `main.go` wire 同构：

| 横切关注点 | 注册点（shared 侧） | 注入方（app 侧） | 说明 |
|---|---|---|---|
| **userId 注入 header** | `shared/api/authProvider.ts::setUserIdProvider(fn)` | `app/model/useSessionBootstrap.ts` | httpClient 调注入 fn，不知 session 存在 |
| **401 → 信号** | `shared/api/authProvider.ts::setOnAuthFailure(fn)` | `app/model/useSessionBootstrap.ts` | 注入的 fn → `session.resolve()`，无循环 |

**身份**：建模为 `entities/session`（业务概念，非 shared）；`currentUserId / status` 唯一真相，唯一 writer。

**toast**：`shared/ui/toastStore.ts`（无业务原语）；feature 抛 `ApiError(code)` → app 全局 `onError` → `errorMap`（shared）→ toast。feature hook 决定文案，组件不碰 toast。

**偏好**：`entities/settings/model/settingsStore.ts`（单例配置实体；下层组件顺向 import；app 驱动 i18n/theme 应用）。

**导航**：feature 返回意图对象，page 执行导航（`paneStore` 在 `app/model`，pages 从 props 拿，不直接 import app store）。

### 横切归属表

| 关注点 | 规范归属 | 读者（顺向） |
|---|---|---|
| 身份（userId / status） | `entities/session/model` | features / widgets / pages / app |
| userId 注入 shared | DIP：shared 注册点 + app 注入 | httpClient / sse（不知上层） |
| toast 队列 | `shared/ui/toastStore.ts` | widgets/toaster + app onError |
| 用户偏好（theme/accent/density/lang/reasoningDefault） | `entities/settings/model` | 下层组件（顺向）；app 驱动应用 |
| UI 编排（openPanes / activeConv / overlays / sidebar） | `app/model/`（paneStore 等） | 只 AppShell 读；pages 收 props |
| SSE 实时消息树 | `entities/conversation/model/chatStore.ts` | 按 block id 细粒度订阅 |
| SSE 连接单例 | `app/sse/SSEProvider.tsx` | 永不开第四条（对位 E1） |
| invalidate 映射 | `shared/api/queryKeys.ts`（单一失效集） | SSE / mutation 都查它 |
| enabled gate | app/page 级 boot gate（session.status=ready 才挂载 AppShell） | entity hook 纯数据，不含 gate |

---

## 改代码前必做（前端版）

1. **读对应 `docs/references/frontend/slices/<slice>.md`**（PRD §18.5 有映射）
2. **确认 `fsd-layers.md`、`entity-types.md`、`cross-cutting.md`** 中对应条目
3. **视觉细节参照已实现的 `frontend/src`**（组件 + `src/styles/`），不靠记忆（boilerplate 原型已退役 2026-05-27）
4. **改完跑 Verification 三段**（见下方）
5. **同步文档**（§F1）

---

## Verification（前端门禁）

```bash
make lint-frontend          # eslint + tsc --noEmit + steiger（FSD 结构）
make test-frontend           # vitest
wails dev                    # 冒烟：窗口起得来 + 能连后端
```

`make lint-frontend` 与后端 `staticcheck` 同等地位；违规 push 不过去。

---

## 遇到 UI bug 的处理原则

**先判断：这是 bug 还是风格偏好？**

| 现象 | 判断 | 处理 |
|---|---|---|
| 元素遮挡导致内容不可见 | Bug | 修，最小干预 |
| 拖拽 / 点击目标区域无法触达 | Bug | 修，最小干预 |
| 宽度/高度导致 overflow 截断核心内容 | Bug | 修，最小干预 |
| 间距稍大或稍小 | 风格 | 保留 |
| 颜色稍浅或稍深 | 风格 | 保留 |

**修的方式：最完整修改。** 彻底搞清问题的机理，不只是表面现象。

**修完后：** 在 `changelog.md` 记一行 `[bug-fix]` dev log。用一句话说明现象和修法，不需要确认。

## 已定型的视觉决策（勿在"优化"中改掉）

以下设计是刻意的、已落地在 `frontend/src`，不要在"优化"过程中改掉：

- `--t-fast/med/slow` 的 cubic-bezier 曲线值
- 信息密度：`--row-h: 32px`（cozy），nav-item 和表格行的紧凑程度
- 单一 accent 原则：全局只用一个 accent 色，不因"区分状态"而乱用
- tool_call block 默认折叠（`defaultOpenTools=false`）
- reasoning block 默认折叠
- msg-actions 默认隐藏，hover 显示
- conversation 列表的 status dot（streaming 脉动，approval warn 色）
- 对话流的 day-divider

---

## 前端代码规范

- **组件文件**：每文件一个主组件，文件名 = 组件名，`PascalCase.tsx`
- **hook 文件**：`useCamelCase.ts`，只做一件事
- **CSS class**：沿用现有 `frontend/src/styles` 的 kebab-case 命名，不引入 BEM 或 CSS Modules
- **组件内铁律（= 后端 S6）**：`onClick` / `onSubmit` 里不准有业务决策；组件只调一个 feature hook 拿意图级 API
- **不写注释**：同后端 S11——只写 why，不写 what；密度上限 1/3；无章节横幅
- **不做防御性校验**：同后端原则六——同人写前后端，API 结构已知，不加多余 null-check 和 fallback
- **import 顺序**：React → 第三方库 → `@/shared` → `@/entities` → `@/features` → `@/widgets` → `@/pages` → 同级文件
- **TS strict**：`tsconfig.json` `strict: true`；新文件 `.tsx/.ts`；entity 类型集中在 `entities/<x>/model/types.ts`
- **i18n（全量中英双语）**：面向用户文案走 react-i18next —— `useTranslation("<ns>")` + `t("key")`，字典 `frontend/src/i18n/locales/{zh,en}/<ns>.json`（结构化 key、按模块拆 ns）；通用动词用 `t("common:xxx")`；**不硬编码中文/英文 UI 串**（含 badge/label/tooltip/placeholder）；`toLocaleString` 等按 `i18n.language` 取 locale；文案夹 JSX 元素用 `<Trans>`；`settings.lang` 驱动切换。详见 `docs/superpowers/specs/2026-05-26-frontend-i18n-design.md`

---

**testend 子项目**：扁平 view-driven，**不进 FSD**；共享 frontend entity types via vite path alias（type-only deep import）；详见 [`testend/CLAUDE.md`](testend/CLAUDE.md)。

---

## §F1 📌 前端文档同步纪律（最高优先级）

**文档落后于代码 = bug。** 本纪律覆盖前端，与后端 §S14 同等级别。

### 触发表

| 前端代码变动 | 必改文档 |
|---|---|
| 新 slice / entity / feature / widget / page | `docs/references/frontend/slices/<slice>.md` + `fsd-layers.md`（slice 清单行）+ `changelog.md` |
| 新 entity TS 类型 / 字段变 | `docs/references/frontend/entity-types.md` + `changelog.md` |
| DIP / errorMap / SSE / queryKeys 接口变 | `docs/references/frontend/cross-cutting.md` + `changelog.md` |
| FSD 层边界变 / 依赖规则变 | `docs/references/frontend/fsd-layers.md` + `CLAUDE.md §FSD` + `changelog.md` |
| 新 API endpoint / path 变 | `frontend-prd.md §17` + `entity-types.md` + `changelog.md` |
| 发现并修了 UI bug | `changelog.md` dev log（`[bug-fix]`）|
| Phase 完成 | `frontend-prd.md §15` 对应项打勾 + `changelog.md` |
| 设计决策变更 | `frontend-prd.md §3` 或对应章节 + `changelog.md` |
| 所有改动（兜底） | `changelog.md` dev log（格式同后端：1-2 句 ~30-100 汉字） |

### 子任务完成 checklist（前端版）

- [ ] `docs/references/frontend/slices/<slice>.md` 实现清单勾 ✅
- [ ] 新 entity 类型 → `entity-types.md` 对应行更新
- [ ] 横切变动 → `cross-cutting.md` 对应行更新
- [ ] `changelog.md` 加 dev log
- [ ] 新规范/决策变动 → 加到本文件 + 对应 contract 文档

发现文档与代码不符 → **立刻停下修文档**，记 `[doc-fix]` dev log。
