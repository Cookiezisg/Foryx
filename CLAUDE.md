# Forgify — Claude 工作守则

> Claude Code 进入本项目自动加载本文件。**本文件是项目工程纪律的唯一事实源**。
> 项目愿景 / 架构 / Phase 路线 / Verification 见 [`documents/version-1.2/backend-design.md`](documents/version-1.2/backend-design.md)。

---

## 项目一句话

- **本地优先 Agentic Workflow Platform**，目标 **Wails 桌面 app**（不做 SaaS）
- **单人项目**；后端 Phase 0-4 完成、Phase 5（智能化）部分交付（document / mcp / skill / memory / compaction ✅；intent / chat 终极版未做）；**当前重心：前端**（V1.2 桌面 app）
- 架构：**4 层 clean arch**，依赖方向 `transport → app → (domain ∪ infra/store) → infra/db`

## 文档地图

| 用途 | 路径 |
|---|---|
| 项目愿景 / 架构 / Phase 路线 | `documents/version-1.2/backend-design.md` |
| 当前进展 / 决策日志 | `documents/version-1.2/progress-record.md` |
| 各 domain 详设计 | `documents/version-1.2/service-design-documents/<domain>.md` |
| 契约索引（API / DB / Error / Events） | `documents/version-1.2/service-contract-documents/` |
| 桌面端 / 调试控制台 | `documents/version-1.2/desktop-packaging-notes.md` / `testend-design.md` |

## 改代码前必做

1. 读对应 `service-design-documents/<domain>.md`
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
- **N7** SSE：`event:` + `id:` + `data:` 格式；Last-Event-ID 重连；超 buffer 返 410 `SEQ_TOO_OLD`。详见 `event-log-protocol.md`

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
- **S15** ID 格式：`<prefix>_<16hex>`（`crypto/rand` 8 字节，失败 panic）。前缀（trinity 后实测全集，按域分组）：`u_` user / `aki_` apikey / `mc_` model-config；`cv_` conv / `msg_` message / `blk_` block / `att_` attachment；`fn_` function / `fnv_` function-version / `fne_` function-exec / `fnenv_` fn-venv；`hd_` handler / `hdv_` handler-version / `hcl_` handler-call / `hdi_` handler-instance / `hdenv_` hd-venv；`wf_` workflow / `wfv_` workflow-version；`fr_` flowrun / `frn_` flowrun-node；`mcl_` mcp-call / `mch_` mcp-health / `ske_` skill-exec；`doc_` document / `mem_` memory / `rel_` relation / `td_` todo；`sr_` sandbox-runtime / `se_` sandbox-env / `bsh_` bash-shell。（skill / mcp 实体以 name 作主键、无实体前缀；`ske_`/`mcl_` 是其执行/调用子记录 ID）
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

**文档落后于代码 = bug。**

## 触发表

| 代码变动 | 必改文档 |
|---|---|
| 新 entity / 表 / struct 字段 / 约束 | `<domain>.md` + `database-design.md` + `progress-record.md` |
| 新 sentinel / errmap 行 | `<domain>.md` + `error-codes.md` + `progress-record.md` |
| 新 endpoint / path 变 | `<domain>.md` + `api-design.md` + `progress-record.md` |
| 新事件 / struct 改 | `<domain>.md` + `events-design.md` + `progress-record.md` |
| 方法签名 / 接口变 | `<domain>.md`（影响对外入口才动 contract 文档）|
| 新跨 domain 依赖 | `<domain>.md` + 受影响的 `<other>.md` |

## 子任务完成 checklist

- [ ] `<domain>.md` 实现清单勾 ✅
- [ ] 改了 API/schema/error → contract 文档对应行更新
- [ ] `progress-record.md` 加 dev log（含做了什么 + 测试数 + 新规范/决策）
- [ ] 新规范/原则变动 → 加到本文件

## domain 完工 checklist

- [ ] `<domain>.md` 整体逐字段匹配代码
- [ ] `service-contract-documents/*.md` 该 domain 行从 ⬜ 改 ✅
- [ ] `progress-record.md` 完工日志
- [ ] 新跨域模式 → 更新 `backend-design.md`

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

- **`staticcheck ./...` 提交前必跑**（比 `go vet` 严，能捞 SA1029/S1016/U1000）
- **`deadcode` 默认不扫测试**：跑时加 `-test=true`；曾误删 `ListProviders` / `ListScenarios`
- **staticcheck 用 `//lint:ignore <code> <reason>`**（不认 `//nolint`）
- **禁用 sed 批量改 import/函数名**：BSD sed `\b` 不识别，会清空文件；用 Edit 工具

## 测试命令（不要直接 `go test ./...`）

| 命令 | 用途 |
|---|---|
| `make test-backend` | **默认跑这个**：后端单测，in-memory SQLite，skip `TestIntegration_*`（= `cd backend && go test ./... -skip TestIntegration_`）|
| `make test` | 后端 + 前端单测（test-backend + test-frontend）|
| `make e2e` | 端到端 pipeline：source `.env`，`-tags=pipeline -p 1 ./test/...`（缺 key 优雅 skip）|
| `make verify` | 发布门禁：vet × 5 平台 + build × 5 + lintprompts |
| `make testend` | 起 testend 调试控制台（pre-frontend 时代遗留）|

> `make` 在仓库**根**跑（不在 `backend/`）。`staticcheck ./...` 在 `backend/` 内直接跑（非 make 目标）。

---

# 项目特殊性

- **单用户本地 + 同人写前后端** → 校验少、便利优先
- **已摆脱 Eino**：自有 LLM 客户端 `infra/llm`（OpenAI-compat + Anthropic 原生）
- **modernc.org/sqlite** 纯 Go；DSN 用 `_pragma=...` 语法；跨平台 build 一行命令
- **桌面端**：Wails 窗口外壳 + 复用 httpapi（不走 Wails native binding）
- **三条 SSE**：eventlog（5×7）+ notifications（全局 entity 变更）+ forge（4×3）；`domain/events` 已删
- **subagent 数据**：统一 `messages` 行（attrs.kind=subagent_run），无独立表
- **sandbox v2**：捆绑 mise binary（`go:embed`）；`make resources` 拉到 `mise/<goos>-<goarch>/`；v1 已废弃
- **测试基线**：`make test-backend`（单测，in-memory SQLite）全绿；⚠️ `make e2e`（pipeline tag）当前因 harness 签名漂移（`LocalCtxAs` 签名变 / `reqctxpkg.DefaultLocalUserID` 已删）编译失败、待修；LLM 集成测试缺 key 时优雅 skip

---

# 前端开发守则（前端阶段 — 现已生效）

> 前端 PRD 全文见 [`documents/version-1.2/frontend-prd.md`](documents/version-1.2/frontend-prd.md)。
> 本节是 PRD 的工程纪律摘要，两者不矛盾时以本节为准（本节更简洁）。

## 两份权威文档

| 问题类型 | 去哪里找答案 |
|---|---|
| 这个组件的 HTML 结构 / class 名 / CSS | `boilerplate/src/` 对应 `.jsx` 文件 + `styles.css` |
| 数据从哪来、接哪个 API、SSE 如何处理 | PRD §5–§7、§9–§14、§17 |
| 动效用什么参数 | PRD §3.2 |
| 这里有 bug，是否要修 | 见下方"遇到 boilerplate bug 的处理原则" |

**PRD 没有描述的视觉细节，以 boilerplate 为准。不要凭空发明。**

## 写每个组件前必做

1. **打开对应 boilerplate 文件**（PRD §18.5 有映射表），读清楚 HTML 结构和 class 名
2. **确认 `styles.css` 里对应的 CSS 规则**，不靠记忆或猜测
3. **按 PRD 替换数据层**：mock 数据 → TanStack Query hook；全局 window.Xxx → ES module import
4. **只改 PRD §16 登记的 bug**，其余照搬

## 遇到 boilerplate bug 的处理原则

**先判断：这是 bug 还是风格偏好？**

| 现象 | 判断 | 处理 |
|---|---|---|
| 元素遮挡导致内容不可见 | Bug | 修，最小干预 |
| 拖拽 / 点击目标区域无法触达 | Bug | 修，最小干预 |
| 宽度/高度导致 overflow 截断核心内容 | Bug | 修，最小干预 |
| 间距稍大或稍小 | 风格 | 保留 |
| 颜色稍浅或稍深 | 风格 | 保留 |
| 某个交互没实现（如 onMouseEnter 注释掉了）| 缺失功能 | 按 PRD §16 的正确实现补全 |

**修的方式：最完整修改。** 彻底搞清问题的机理，不只是表面现象。

**修完后：** 在 PRD §16 补一行记录（即使没有预先列出）。用一句话说明现象和修法，不需要确认。

## 绝对不改的 boilerplate 决策

以下设计是刻意的，不要在"优化"过程中改掉：

- `--t-fast/med/slow` 的 cubic-bezier 曲线值
- 信息密度：`--row-h: 32px`（cozy），nav-item 和表格行的紧凑程度
- 单一 accent 原则：全局只用一个 accent 色，不因"区分状态"而乱用
- tool_call block 默认折叠（`defaultOpenTools=false`）
- reasoning block 默认折叠
- msg-actions 默认隐藏，hover 显示
- conversation 列表的 status dot（streaming 脉动，approval warn 色）
- 对话流的 day-divider

## 前端代码规范

- **组件文件**：每文件一个主组件，文件名 = 组件名，`PascalCase.jsx`
- **hook 文件**：`useCamelCase.js`，只做一件事
- **CSS class**：沿用 boilerplate 的 kebab-case 命名，不引入 BEM 或 CSS Modules
- **不写注释**：同后端 S11——只写 why，不写 what；密度上限 1/3；无章节横幅
- **不做防御性校验**：同后端原则六——同人写前后端，API 结构已知，不加多余 null-check 和 fallback
- **import 顺序**：React → 第三方库 → 内部 api/store/sse → 内部 components → 同级文件
- **i18n（全量中英双语）**：面向用户文案走 react-i18next —— `useTranslation("<ns>")` + `t("key")`，字典 `frontend/src/i18n/locales/{zh,en}/<ns>.json`（结构化 key、按模块拆 ns）；通用动词用 `t("common:xxx")`；**不硬编码中文/英文 UI 串**（含 badge/label/tooltip/placeholder）；`toLocaleString` 等按 `i18n.language` 取 locale；文案夹 JSX 元素用 `<Trans>`；`settings.lang` 驱动切换。详见 `docs/superpowers/specs/2026-05-26-frontend-i18n-design.md`

## F1 前端文档同步

前端代码变动时，如果涉及以下情况，必须同步更新 PRD：

| 变动 | 必更新 PRD |
|---|---|
| 发现并修了 boilerplate bug | §16 补一行 |
| 实际 API endpoint 与 §17 不符 | §17 修正 |
| Phase 完成 | §15 对应项目打勾 |
| 设计决策变更（如改了某个动效参数）| §3 或对应章节更新 |
