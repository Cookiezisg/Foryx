# Anselm — Claude 工作守则

> Claude Code 进入本项目自动加载本文件。**本文件是项目工程纪律的唯一事实源**。
> 项目愿景 / 架构 / 实体地图 / 引擎见 [`docs/concepts/architecture.md`](docs/concepts/architecture.md)；文档规范见 [`docs/GOVERNANCE.md`](docs/GOVERNANCE.md)。
> 旧版（覆盖回 `backend/` 之前的快照）在 `version-0.2` git 分支——参考旧版 checkout 它即可，不在当前文档维护任何历史。
>
> **交流语言**：本项目的所有对话回复一律用**中文**（代码、标识符、commit message 等技术产物的语言约定不受此限）。

---

## 项目一句话

- **本地优先 Agentic Workflow Platform**，目标 **Flutter 桌面 app**（macOS/Linux/Windows，Go 后端作 sidecar）、**单进程单用户**、SQLite 落盘（**不做 SaaS**）。
- **核心心智**：**Quadrinity（四项全能）** 实体（Function/Handler/Agent/Workflow）+ **Durable Execution**（节点结果记忆化 + 解释器幂等重走）。
- **架构**：4 层 Clean Architecture，依赖单向 `transport → app → (domain ∪ infra/store) → infra/db`。地基自研：`pkg/orm`（去 GORM）+ `glebarez/go-sqlite`（纯 Go、无 CGO）。
- **当前状态**：后端 `backend/` 全实体 + durable 引擎，编译/装配/启动/服务全通（单一后端）；前端 `frontend/` Flutter 桌面端**地基已落**（sidecar/契约/SSE gateway/装配根/i18n，analyze+test 绿，见 ADR 0004）；**下一步**：按 app 形态铺 features、对接 `backend` 契约。

## 文档地图

> 入口 = [`docs/INDEX.md`](docs/INDEX.md)（AI 会话先读它再循链接）。后端全域 reference 已成体系——overview 鸟瞰 + `api/database/error-codes/events` 四索引 + `domains/` 分域 + `foundation/` 地基，与代码逐字同步；前端 reference 随 features 落地填充。

| 用途 | 路径 |
|---|---|
| 文档入口（索引 + 结构） | `docs/INDEX.md` |
| 愿景 / 架构 / 实体 / 引擎 / 路线 | `docs/concepts/architecture.md` |
| 文档规范（类型 / 同步 / 执行） | `docs/GOVERNANCE.md` |
| 后端鸟瞰（第 0 篇） | `docs/references/backend/overview.md` |
| 契约四索引（端点 / 表 / 错误码 / 事件） | `docs/references/backend/{api,database,error-codes,events}.md` |
| 分域 / 地基详解 | `docs/references/backend/domains/` · `foundation/` |
| 架构决策（ADR） | `docs/decisions/` |

---

# 设计原则（9 条，#9 最高优先级）

1. **Quadrinity 实体化**：任何能力必须归属于 Function / Handler / Agent / Workflow 之一。
2. **Durable 为魂**：工作流执行基于**节点结果记忆化**（`flowrun_nodes` 行表 + record-once）+ **解释器幂等重走**实现崩溃恢复与确定性重放——**非**事件日志（Temporal 式 journal 已否决）。
3. **依赖自下而上**：`domain` 层**严禁 import 任何外部包**（含 ORM / cel-go）；`app` 层协调 domain 与 infra；跨实体协作走 DIP 端口、不硬依赖具体实现。
4. **后端契约是事实源**：`reference` 文档 = 代码的精确投影；前端按 [`ADR 0004`](docs/decisions/0004-frontend-flutter-architecture.md)（Flutter 3-tier feature-first）对接已定型的后端契约（地基已落）。
5. **端到端推演先行**：开工前必走完整数据流 + 列出跨域依赖（relation 边）。
6. **反校验剧场**：只保留有物理价值的校验（JSON、必填、CHECK/UNIQUE）；不加多余 null-check。
7. **零历史包袱 + 状态即重述**：项目未上线，禁止维护兼容性、禁止历史演化描述，只留当前物理事实（历史从 git 取）。**状态文档**（本文件 / `architecture.md` / `GOVERNANCE.md`）改任何状态/事实 = **整体重述当前状态、非追加**——绝不在旧内容旁堆新句、不留旧状态痕迹（见末「文档纪律」节 + GOVERNANCE §1.7）。
8. **复用优先、不造轮子**：动工前先盘点 `pkg/*` 与 `infra/*` 既有能力——能复用就复用；业务层将手搓的样板本应由地基提供时（如 orm 补 UNIQUE 冲突翻译），**强化地基**而非模块内重抄。错误抽象与重复样板比多写一行更糟。
9. **📌 文档与代码物理同步（最高优先级）**：每个代码改动必须在**同一提交**伴随对应文档的 1:1 更新——**文档落后于代码 = 严重 Bug，与编译失败同级**。完整执行规则见本文件末「**文档纪律（强制）**」节 + [`docs/GOVERNANCE.md`](docs/GOVERNANCE.md)。

---

# Standards — 契约宪法

## HTTP API（N 系列）

- **N1 统一 Envelope**：成功 `{"data": ...}`；失败 `{"error": {"code", "message", "details"}}`。
- **N2 状态码**：202 Accepted（异步流）/ 204 No Content / 410 Gone（SSE 淘汰）。
- **N3 命名规约**：API 线缆 camelCase；数据库物理列 snake_case。
- **N4 强制分页**：所有 List 接口必须支持 `?cursor=...&limit=...`。
- **N5 动作后缀**：非 CRUD 逻辑用 `:action`。
    - **`:run`**(fn) / **`:call`**(hd) / **`:invoke`**(ag) / **`:trigger`**(wf) 为标准执行动词。
    - **`:iterate`**（AI 编辑实体）/ **`:triage`**（AI 诊断执行）统一返回 `conversationId` 开启对话。

## 数据库（D 系列）

- **D1 软删除**：业务表用 `deleted_at DATETIME`；**Log 表**（`flowrun_nodes` / trigger 的 firing·activation / messages 块 等内容/执行日志）**无 `deleted_at`、严禁逻辑删除**——唯一物理删例外：`:replay` 经 `DeleteFailedNodes` 清 `flowrun_nodes` 的 failed 行（failed 是非结果、清掉让幂等重走重跑，record-once 真相不损；与 `database.md` 对齐）。
- **D2 物理隔离**：所有表（除全局配置外）必须持 **`workspace_id`** 物理列；`pkg/orm` 据 ctx 自动双向隔离。
- **D3 唯一性铁律**：`idx_frn_once`（flowrun 记忆化 `UNIQUE(flowrun_id,node_id,iteration)`）与 `idx_trf_dedup`（trigger firing 去重）必须保证幂等。

## SSE 协议（E 系列）

- **E1 三条流限制**：全系统仅 `messages` / `entities` / `notifications` 三条 SSE，**永不再加**。前端启动即常驻全连；三流 **workspace 级、后端不过滤**（发完整 delta、前端自滤）；订阅统一在 `StreamHandler`（`GET /api/v1/{messages,entities,notifications}/stream`）。
- **E2 Ephemeral 帧**：delta / tick（如 flowrun 节点推进）标 `seq=0`，**不入 buffer、不产生背压**；open/close/signal 为 durable（close 带快照供 replay）。
- **E3 嵌套递归**：messages 流支持 `parentBlockId` 嵌套，前端据此渲染 subagent 树。

---

# 代码规范（S 系列）

- **S5 物理文件对齐**：handler 文件名对应 API 资源域；domain 文件名对应 Repository 接口。
- **S9 确定性上下文**：每个跨层调用强制传 `ctx`；异步 Finalize 必须用 **Detached Context**（保留 workspace 种子、脱离请求取消）。
- **S11 注释双语化**：`// English \n\n // 中文`。**只写 Why、不写 What**。
- **S13 导入别名**：所有 `internal/` 包导入带 `<name><role>` 别名（如 `apikeydomain`、`chatapp`、`workflowstore`）。
- **S15 ID 宪法**：`<prefix>_<16hex>`。前缀全集必须在 `references/backend/database.md` 登记（infra 侧 ID 用自己的前缀，不从消费实体 ID 派生）。
- **S18 Tool 规范**：Tool 实现 **5 方法接口**（`Name`/`Description`/`Parameters`/`ValidateInput`/`Execute`）；`summary` / `danger`（三级 safe/cautious/dangerous，LLM 逐次自报）/ `execution_group` 三字段由 Framework 强制注入 schema 并从 args 剥离。**无中央权限门控**：危险靠 LLM 自报 + 逐次内存阻塞确认（active skill 的 `allowed-tools` 预授权可免确认）。
- **S20 错误构造（全量统一）**：所有**命名 sentinel 错误**一律 `errorspkg.New(kind, code, msg)`（`pkg/errors`——错误类型是纯机制、放地基、全层可用，无反向依赖）；带 Kind（→HTTP status）+ 稳定 `<ENTITY>_<REASON>` wire code。**无"是否冒泡 HTTP"之分**——同一错误两种出口：HTTP 读 Kind/Code 走 Envelope，LLM tool 读 Message。**禁止**用标准库 `errors.New` 造命名 sentinel；`fmt.Errorf("…: %w", err)` 包裹照常（保留 `errorspkg.Error` 链供 `errors.Is/As`）。泛型原语（如 `orm.ErrNotFound`）带兜底码、由 domain 翻译成具体码。`errors.Is`/`errors.As` 用标准库。见 [`decisions/0002`](docs/decisions/0002-unified-error-type.md)。
- **S22 工作区卫生 + 事实同步**：仓库只留源码 + 必要配置——**散落二进制 / 构建产物 / OS·编辑器生成物一律不入库**（`go build` 出 `bin/`、日常用 `go run`；`.DS_Store`·`mise.local.toml`·`backend/<cmd>` 散件 gitignore，stale 产物随手删）。改 `cmd/` 子命令 / 工具 / 目录结构 → **同提交把 `.gitignore`·`Makefile`·`mise.toml` 同步到当前物理事实**（删尽对已不存在之物的忽略 / 引用 / 目标——同 #7「状态即重述」、把 gitignore·Makefile 也当状态）。删前先辨：产物（可删）vs 源码 / 版本钉文件（如 `mise.toml`，不动）。

---

# 测试与门禁（T 系列）

- **T5 验收双层**：单元/集成测试随包；**全功能黑盒验收在 `testend/`**（独立 module、零 backend import、拉真二进制打纯 HTTP/SSE）——`make testend`（llmmock 零 token，分钟级）+ `make evals`（真模型金标，EVALS=1 门控烧钱）。两者不进 `make verify`。见 [`references/testend/overview.md`](docs/references/testend/overview.md)。
- **T6 Fake LLM**：默认测试用 `fake_llm`，0 Token 消耗。
- **`make verify`（pre-push 门禁，host 平台）**：`gofmt` 净 + `go vet` + `go build` + 单测 + 文档门禁全绿。并发/取消测试带 `-race`。
- **`make docs`（文档门禁）**：`cmd/docs` 跑 GOVERNANCE §11 全套（frontmatter / 类型 / 生命周期 / INDEX≤50 / 孤儿链接）。
- **跨平台 release**：任意平台 `cd backend && GOOS=x GOARCH=y go build ./cmd/server` 直接出二进制——**无内嵌、无预拉**（运行时由自研 `directInstaller` 在目标机首用按需下，见 [`decisions/0001`](docs/decisions/0001-sandbox-runtime-direct-install.md)）。
- **`make fe-verify`（前端门禁，mise flutter）**：codegen（freezed/json/slang）+ `flutter analyze` 净 + `flutter test` 绿。与 `make verify`（后端）分列、各自 pre-push。

---

# 前端开发守则（Flutter 桌面端，按本节 + [`decisions/0004`](docs/decisions/0004-frontend-flutter-architecture.md)）

- **技术栈**：Flutter 桌面端（Dart）。状态 **Riverpod**（经典 provider 写法，非 codegen——此 Dart SDK + freezed 3 太新，riverpod_generator/lint 生态未跟上，见 ADR 0004 取舍）；**freezed + json_serializable + slang** 经 build_runner codegen；**dio**（HTTP）/ **go_router**（导航）/ **window_manager**（窗口）。工具链经 **mise**（`go` + `flutter`，真·可写官方 SDK；devbox/nix 已弃——只读 store 构建不了 macOS app，见 [`decisions/0005`](docs/decisions/0005-toolchain-mise.md)）。
- **进程模型**：Go 后端作 **sidecar**，客户端经 localhost HTTP+SSE 对接——Dart 抢临时端口 → `ANSELM_ADDR` 拉起 → `/api/v1/health` 门控（零后端改）。dev 用 `ANSELM_BACKEND_URL` 挂已跑后端（`make server`）。
- **分层（3-tier feature-first，对齐 Clean 不照搬）**：`shared/core`（contract/net、SSE gateway、design、i18n、router、process）→ `features/<域>`（各自管 data+state+ui）→ `app`（装配根 + shell）。**无 use-case 层**（客户端零业务规则，Go 二进制即用例）。features **互不依赖**（跨 feature 走 shared provider / nav intent）。唯一框架无关纯模型层：`BlockTreeReducer` / `GraphModel`（承载性正确、须脱 widget/socket 单测）。
- **状态 + 实时**：Riverpod 托管 server-state（`AsyncNotifier` 分页 `loadMore`）+ 三条 `keepAlive` SSE 流。SSE 经 `SseGateway` 的 plain-Dart **`Map<Scope,Stream>` demux 自滤**（**不**在 Riverpod 里逐帧 `.where`）。铁律 **DB 行是真相、流只为实时**：`seq>0` 才 durable / 推进续传游标；ephemeral（delta/tick）只改瞬时视图态、不进耐久缓存。
- **DIP 注入**：`shared` 不依赖上层；**workspace**（=唯一鉴权轴，header `X-Anselm-Workspace-ID`）+ **baseUrl** 由 `app` 经 `ProviderScope` override 注入；401（`UNAUTH_NO_WORKSPACE`→清选区重选）/ 410（`SEQ_TOO_OLD`→重取 REST 再续）在此拦截。
- **契约层 = 后端投影**：freezed DTO 逐字镜像 `references/`；**仅 seal 真封闭集**（4 frame 动词 / 6 block 型 / 5 图节点 kind / 4 trigger 源），协议级 SSE `node.type` 与 ~261 错误码**保持开放 + `unknown` 兜底**。改后端字段 → **同提交**改 Dart DTO（文档纪律延伸到前端契约）。
- **视觉灵魂**：明亮、通透、轻盈。`Tokens.rowHeight = 32` 紧凑；`tool_call` 与 `reasoning` 默认折叠。颜色/度量走 design token，禁内联硬编码。
- **i18n**：严禁在 Dart 硬编码中英文；文案走 slang `context.t.<key>`、登记在 `lib/i18n/<locale>.i18n.json`。
- **门禁**：`make fe-verify`（codegen + `flutter analyze` 净 + `flutter test` 绿）。codegen 产物入库（源等价、deterministic，fresh checkout 直接 analyze）。层依赖暂用目录约定 + review 守（custom_lint 待生态跟上 SDK 再接）。桌面真跑 `flutter run -d <平台>` 需完整 Xcode/CocoaPods 等机器层面工具，不入门禁。

---

# 文档纪律（强制 —— 完整规范见 [`docs/GOVERNANCE.md`](docs/GOVERNANCE.md)）

> 本节是文档规范的**常驻执行层**：CLAUDE.md 每次会话自动加载，故下列规则你**每次都已读到、无「不知道」借口**。详尽规则（6 类型 / frontmatter / 生命周期 / 命名 / 质量门禁）在 `GOVERNANCE.md`——它是 binding。**本节与 GOVERNANCE §0/§7/§12 必须一致**（改一处即同步另一处）。

## 三条铁律（违反 = 严重 Bug，与编译失败同级）

1. **同步**：改代码 → **同一提交**改对应文档。文档落后于代码 = 这次改动**未完成**。
2. **触发即停**：发现文档与代码不符 → 立刻停下修文档（记 `[doc-fix]` dev log），再续原任务。
3. **存疑即查**：不确定 → 查 `GOVERNANCE.md`；它没覆盖 → 按设计原则推导 + 回头补一条进 GOVERNANCE。

## 同步触发表（改左列代码 → 同一提交改右列文档）

| 代码改动 | 必须同步 |
|---|---|
| 新增/改 API 端点 | `references/backend/api.md` + 对应 `domains/<域>.md` |
| 新增/改 DB 表/列 | `references/backend/database.md` + 对应 `domains/<域>.md` |
| 新增/改 error code | `references/backend/error-codes.md` + 对应 `domains/<域>.md` |
| 新增/改 SSE 事件 | `references/backend/events.md` + 对应 `domains/<域>.md` |
| 架构决策（选型/取舍） | `decisions/` 新建一篇 ADR |
| 架构 / 实体 / 引擎 / 路线状态变更 | **整体重述** `concepts/architecture.md` 相关节（非追加） |
| 工程规则 / 设计原则 / N·D·E·S·T 变更 | **整体重述** 本文件相关节（非追加） |
| 前端契约层（DTO / envelope / 错误码）变更 | `references/frontend/contract.md` + 对应 `domains/<域>.md` |
| 前端架构 / 分层 / SSE gateway 规则变更 | `references/frontend/{architecture,sse-gateway}.md` + 本文件前端节 + [`ADR 0004`](docs/decisions/0004-frontend-flutter-architecture.md) |

非穷举。**两种 mode 不混**：`reference` 文档 = 精确同步（逐字吻合代码）；`architecture.md` / 本文件 = **整体重述**（相关节重写到当前状态、删尽旧状态，绝不追加堆叠）——见 GOVERNANCE §1.7。

## 收尾清单（声明任何代码改动「完成」前逐条勾，任一未过 = 未完成）

1. ☐ 碰了上表的东西？→ 对应文档**同提交**更新了？
2. ☐ 改的 `reference` 文档与代码**逐字**对得上（端点/字段/码/事件 一一吻合）？
3. ☐ 改的是状态文档（architecture / 本文件 / GOVERNANCE）？→ 是**整体重述到当前状态**（没在旧内容旁追加、没留旧状态痕迹）？
4. ☐ 新文档 frontmatter 合法（`type`/`status`/`id`）、放对目录（GOVERNANCE §5）？
5. ☐ 删/移文档后无孤儿链接（`INDEX.md` 及他处指向它的都修了）？
6. ☐ 没编辑 `decisions/` 里的 ADR（不可变，只能新建 supersede）？
7. ☐ working 文档落地了（结论提取进 concepts/references + 填 `landed-into` + 移 `archive/`）？

> 工作区卫生（散落二进制 / 产物 / OS 垃圾 + `.gitignore`·`Makefile`·`mise.toml` 同步到当前事实）见 **S22**——每次提交前一并自检（非文档纪律范畴，不入本清单）。
