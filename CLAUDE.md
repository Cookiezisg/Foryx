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
- **当前状态**：后端 `backend/` 全实体 + durable 引擎，编译/装配/启动/服务全通；**+ loopback 加固**（默认绑 `127.0.0.1` + `RequireBearerToken`[`ANSELM_AUTH_TOKEN`,空=关] + `RequireLoopbackHost`[防 DNS rebinding]）。前端 `frontend-rebuild`（**当前活跃线**）= 新设计系统 + **UI kit G0–G6（49 原语 + gallery）** + 三岛 shell + 窗口/缩放/i18n/浮层 + **Phase 4.0 运行时骨干**（`core/{contract,net,sse,process,perf,error}` + `runtime.dart` Riverpod 装配 + `app_startup_gate` 启动门控 + L0–L2 流式性能合并原语；从 `main` PORT + 加固）。建造规范 [`WRK-045`](docs/working/platform-foundation/phase-4.0-runtime-backbone.md)；平台地基总账 [`WRK-042`](docs/working/platform-foundation/README.md) + 发行 Playbook [`WRK-043`](docs/working/platform-foundation/release-distribution-playbook.md)。**Phase 4.1（Entities feature）进行中**：STEP 0 契约层（`core/contract/entities/` ~22 Quadrinity DTO[function/handler/agent/workflow + 共享值 + 跨域]+ golden 往返，见 [`contract.md`](docs/references/frontend/contract.md)）+ STEP 1 数据缝（`features/entities/data/` 单一 `EntityRepository` 缝 = Live[接 ApiClient+SseGateway,补 `getPageWithAggregate`] / Fixture[内存+可脚本信号] / `entityRepositoryProvider` override；EntityKind/EntityRow/EntitySignal）+ STEP 2 列表 state（`features/entities/state/` `entityListProvider` family[AsyncNotifier:首页+loadMore+SSE 就地 patch,durable 才改列表/ephemeral 不动]+ `railModelProvider` rail VM）+ STEP 3 rail UI（`features/entities/ui/` `EntityRail` over `AnSidebarList`:4 kind 折叠段 + 计数 + 状态点[handler runtime/workflow lifecycle·attention,经 `AnStatus.fromRaw`]+ loading/error/empty/loaded 四态 + 过滤(自带)+ 排序 sliders 菜单(`railSortProvider` 最近更新/名称)+ `selectedEntityProvider`)+ STEP 4 详情 sea（`features/entities/{state/detail,ui/detail}/`:**整海洋 = 单一 `AnPage` 文档**——头 + tab + 内容居中 720 阅读列**一起滚**(`AnTabs` flow 模式,对齐 demo an-page/an-tabs);`EntityOcean`=`AnOceanHeader`[各 kind 状态徽 + 动词 CTA 接右岛 run 终端(STEP 5)]+ `AnTabs`(概览/版本/日志):4 kind 概览段[复用 AnKv/AnCodeEditor/AnInfoCard,handler 敏感默认遮蔽;**编排图可视化 + 图编辑器入口推迟到图编辑器阶段**]+ 版本 tab(AnVersionDiff 相邻版本)+ 日志 tab(PageWithAggregate loadMore + 行展开,workflow flowrun 懒取节点)+ 各 tab loading/error/empty;`entityDetail/versionList/logList` provider[durable 生命周期→重取、ephemeral no-op、关自动重试]）+ STEP 5 执行 + 右岛 run-terminal（`core/contract/messages/block_content`[`BlockKind` 6 sealed + Text/ToolCall/ToolResult/Message Content,投影 backend messages/loop/chat]+ `core/messages/block_tree_reducer`[**唯一框架无关纯模型层**,折 open/delta/close→嵌套 `BlockNode` 树,E3 parentId 嵌套 + 设防]+ `features/entities/{state/run,ui/run}`:`RunTerminalController`[**family by EntityRef**——每可执行实体独立运行态 + SSE 订阅 + coalescer,**run 起 `keepAlive` → 切走后台续流、切回看进度**;接 4 execute 动词 + 先订 entities 面板 scope 再发执行、按 kind 分流:fn/hd run-node delta→文本日志 / agent ReAct 块帧→`BlockTreeReducer` / wf tick→live 节点 + 触发后 `getFlowrun` 取 durable 节点;高频 body 在 L2 `CoalescingNotifier<RunStream>`(每帧≤1 重画)、生命周期态独立;输入草稿在 controller(头钮可触发)、run 时按 Field 类型强转;cancel=seq-guard 丢陈旧结果]+ `RunTerminal`[headless `AnInspector`,**强链 `selectedEntity`**(右岛随选区重绑):状态机头(idle/running/ok/failed/cancelled + 运行 meta)+ 类型化逐字段 `RunInputForm`(Field→string/number/boolean/object/array 控件 + handler method 下拉 + wf 可选 JSON payload,object/array jsonDecode 内联校验)+ 流式 body(fn/hd 文本+结果+日志 / agent `BlockTreeView` 嵌套折叠[reasoning/tool_call 默认收起、danger 徽] / wf flowrun 节点+状态徽)+ stick-to-bottom + 整 body `SelectionArea`];动词 CTA→`run`(直接执行,demo 对齐),`AppShell` 转 `ConsumerWidget`(`inspectorOpen`=有选中 ∧ 未收起、`rightPanelCollapsedProvider` sticky 收起、右岛挂 `RunTerminal`);`FixtureEntityRepository` execute 改脚本流(`runDelay`)使 `make demo` 真流式 + 合成 durable flowrun）已落（`make fe-verify` 822 测绿,run 终端四 kind[function/agent/workflow/handler]真渲染截图核对）。**壳整合规范**：app 与 demo 共用唯一 `app/app_shell.dart`（`AppShell`=rail 左岛 + ocean 海洋),`make app`(真后端)/`make demo`(`lib/dev/demo_main.dart`,fixture 零后端)只差数据源 + 启动门控;截图 `test/dev/capture_demo.dart`。**冷启动**：`core/workspace/workspace_bootstrap`(后端就绪后 列/建 workspace + 设 `activeWorkspace`,否则全 workspace 域 API 401)+ `app/workspace_gate`(在 startup gate 之下、壳之上);`make app` 端到端真跑截图 `test/dev/shot_app_real.sh`(起后端 + 种 workspace/实体 + flutter build macos + 直跑二进制带 `ANSELM_BACKEND_URL` → 截真窗口)。**真跑抓到并修了一处关键缺陷**:macOS sandbox 缺 `com.apple.security.network.client` → app 无法出站连后端、永久卡"连接中"(Release 连 network 全无);Debug/Release entitlements 已补 network.client+server。**make app 现真后端端到端可用**:冷启动定 workspace → rail 显真实体(真截图核对)。`make app` = `test/dev/run_app.sh`(自动起后端[`make server` 后台常驻、`make stop` 关]+ dev-attach `ANSELM_BACKEND_URL` + 热重载);**生产的 spawn-打包-sidecar 路径**(.app 自带签名 Go 二进制、无需 make server)= 发行阶段(WRK-043,network entitlements 已是其前置)。**加载态**:首载骨架经 `AnDeferredLoading`(`AnMotion.loaderDelay` 160ms 防闪)+ 详情骨架与内容同 720 列(不跳宽)。续 STEP 6（select + go_router 路由 deep-link + 五电池矩阵收尾），建造规范 [`WRK-046`](docs/working/entities/README.md)。（修了 F172 两处既有 bug:① SSE 三流全 500——`muxErrorWriter` 没转发 `http.Flusher`,`021566d5`;② 全 API 的 handler 404/405 被 clobber 成 ROUTE_NOT_FOUND——`envelopeMuxErrors` 包了每个响应,改为仅 mux 未匹配时套信封,`800b6b53`。testend 原红 3 个 R4 scenario 全转绿。）

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
4. **后端契约是事实源**：`reference` 文档 = 代码的精确投影；前端按 [`ADR 0004`](docs/decisions/0004-frontend-flutter-architecture.md)（Flutter 3-tier feature-first）对接已定型的后端契约（运行时管道状态见「当前状态」节）。
5. **端到端推演先行**：开工前必走完整数据流 + 列出跨域依赖（relation 边）。
6. **反校验剧场**：只保留有物理价值的校验（JSON、必填、CHECK/UNIQUE）；不加多余 null-check。
7. **零历史包袱 + 状态即重述**：项目未上线，禁止维护兼容性、禁止历史演化描述，只留当前物理事实（历史从 git 取）。**状态文档**（本文件 / `architecture.md` / `GOVERNANCE.md`）改任何状态/事实 = **整体重述当前状态、非追加**——绝不在旧内容旁堆新句、不留旧状态痕迹（见末「文档纪律」节 + GOVERNANCE §1.7）。
8. **复用优先、不造轮子 + 最佳实践优先（遇问题先查、不手搓）**：动工前先盘点 `pkg/*` 与 `infra/*` 既有能力——能复用就复用。**遇到任何不确定的问题（工程 OR 视觉），第一反应是联网查成熟方案 / 官方文档 / 标准库 / 既有最佳实践，绝不一上来自己手搓**——本项目在红绿灯重定位、窗口 chrome 等问题上反复手搓、反复跌跟头，教训惨痛：手搓的"看似能跑"往往埋着边界 bug，成熟方案已替你踩过坑。有成熟包/标准 API 就用它（如 macOS 窗口用 `macos_window_utils`），而非抄它的实现。业务层手搓的样板本应由地基提供时 **强化地基**、非模块内重抄。错误抽象与重复样板比多写一行更糟。
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

- **技术栈**：Flutter 桌面端（Dart）。状态 **Riverpod**（经典 provider 写法，非 codegen——此 Dart SDK + freezed 3 太新，riverpod_generator/lint 生态未跟上，见 ADR 0004 取舍）；**freezed + json_serializable + slang** 经 build_runner codegen；**dio**（HTTP）/ **go_router**（导航）/ **window_manager**（窗口尺寸·最小·居中·resize,逻辑点 scale 正确）+ **macos_window_utils**（仅 macOS 窗口外观:无边框 + 加高标题栏让红绿灯居中可点）/ **scaled_app**（应内 Cmd +/- 整体缩放）——窗口三件套都用成熟包、**不手搓**,见原则 #8。工具链经 **mise**（`go` + `flutter`，真·可写官方 SDK；devbox/nix 已弃——只读 store 构建不了 macOS app，见 [`decisions/0005`](docs/decisions/0005-toolchain-mise.md)）。
- **进程模型**：Go 后端作 **sidecar**，客户端经 localhost HTTP+SSE 对接——Dart 抢临时端口 → `ANSELM_ADDR` 拉起 → `/api/v1/health` 门控（零后端改）。dev 用 `ANSELM_BACKEND_URL` 挂已跑后端（`make server`）。
- **分层（3-tier feature-first，对齐 Clean 不照搬）**：`shared/core`（contract/net、SSE gateway、design、i18n、router、process）→ `features/<域>`（各自管 data+state+ui）→ `app`（装配根 + shell）。**无 use-case 层**（客户端零业务规则，Go 二进制即用例）。features **互不依赖**（跨 feature 走 shared provider / nav intent）。唯一框架无关纯模型层：`BlockTreeReducer` / `GraphModel`（承载性正确、须脱 widget/socket 单测）。
- **状态 + 实时**：Riverpod 托管 server-state（`AsyncNotifier` 分页 `loadMore`）+ 三条 `keepAlive` SSE 流。SSE 经 `SseGateway` 的 plain-Dart **`Map<Scope,Stream>` demux 自滤**（**不**在 Riverpod 里逐帧 `.where`）。铁律 **DB 行是真相、流只为实时**：`seq>0` 才 durable / 推进续传游标；ephemeral（delta/tick）只改瞬时视图态、不进耐久缓存。
- **DIP 注入**：`shared` 不依赖上层；**workspace**（=唯一鉴权轴，header `X-Anselm-Workspace-ID`）+ **baseUrl** 由 `app` 经 `ProviderScope` override 注入；401（`UNAUTH_NO_WORKSPACE`→清选区重选）/ 410（`SEQ_TOO_OLD`→重取 REST 再续）在此拦截。
- **契约层 = 后端投影**：freezed DTO 逐字镜像 `references/`；**仅 seal 真封闭集**（4 frame 动词 / 6 block 型 / 5 图节点 kind / 4 trigger 源），协议级 SSE `node.type` 与 ~261 错误码**保持开放 + `unknown` 兜底**。改后端字段 → **同提交**改 Dart DTO（文档纪律延伸到前端契约）。
- **视觉灵魂**：明亮、通透、轻盈。`Tokens.rowHeight = 32` 紧凑；`tool_call` 与 `reasoning` 默认折叠。颜色/度量走 design token，禁内联硬编码。
- **i18n**：严禁在 Dart 硬编码中英文；文案走 slang `context.t.<key>`、登记在 `lib/i18n/<locale>.i18n.json`。
- **门禁**：`make fe-verify`（codegen + `flutter analyze` 净 + `flutter test` 绿）。codegen 产物入库（源等价、deterministic，fresh checkout 直接 analyze）。层依赖暂用目录约定 + review 守（custom_lint 待生态跟上 SDK 再接）。桌面真跑 `flutter run -d <平台>` 需完整 Xcode/CocoaPods 等机器层面工具，不入门禁。
- **启动面（规范，三个、永不增 per-feature 入口）**：`make gallery`（组件视觉目录）· `make app`（真壳 + 真后端 sidecar）· `make demo`（真壳 + 假数据、零后端）。**app 与 demo 共用唯一壳 `app/app_shell.dart`（`AppShell`，哪个 feature 在哪个岛只写一次）**，只差两点 ①数据源（app 接 Live repository / demo `ProviderScope` override 成 `features/*/data/*_demo_fixture`）②启动（app 走 `AppStartupGate` 等后端 / demo 跳门控）。**新 feature 接进 `AppShell` 一次、app+demo 同时拥有；绝不为单 feature 加 `make <feature>` 入口或 per-feature 截图**（碎片化必不 sync；截图统一 `test/dev/capture_demo.dart` 截整 `AppShell`）。详见 [`architecture.md`](docs/references/frontend/architecture.md) §6。
- **🔁 迭代流程铁律（Phase 4 起,每个 feature/任务强制)**：① **对接后端前先吃透后端**——凡涉及后端契约的任务,**开工前先多 agent 扇出详读相关后端代码 + `references/backend/`**,产出精确"集成契约"(端点/帧/DTO/错误码/SSE 语义)再动手,**绝不照猜后端**。② **必要时改后端**——前端需要而后端缺的(如 loopback 鉴权、新端点、契约缺口),**允许给后端加端点/中间件**,但须同提交守后端纪律(N/D/E/S/T 系列 + `make verify` + 文档 1:1 同步 #9)。③ **每步执行前大规模扇出调研(两段,缺一不可)**——**(a) 读码吃透相关后端**(见①,产出精确集成契约),**紧接 (b) 联网详调该解决方案的 best practice**(怎么把这套建好:成熟包 / 业界模式 / 已知坑,原则 #8——例:Dart SSE 断线续传、dio 拦截器、Riverpod 分页、子进程托管的标准做法);两段均过对抗验证;再 → working 规范 → **用户拍板** → 单一作者建 → 对抗复审 → 真机截图验 → landed-into docs。④ **超强覆盖测试**——feature 落地配 widget-test 矩阵(空/超长/海量/极值/注入五电池)入 `make fe-verify`;涉后端改动配 `testend` 黑盒(llmmock 零 token);两端门禁各自全绿才算完。

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
