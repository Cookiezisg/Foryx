---
id: DOC-044
type: reference
status: active
owner: @weilin
created: 2026-06-22
reviewed: 2026-06-22
review-due: 2026-09-22
audience: [human, ai]
---

# 前端架构 —— Flutter 桌面端的物理结构（重建中）

> 前端已从 0 重建（见 git：`frontend-rebuild` 分支）。本篇是重建的**第 0 篇**:分层、文件住哪、纪律。
> 决策依据 [`ADR 0004`](../../decisions/0004-frontend-flutter-architecture.md)；工程规范见 [`CLAUDE.md`](../../../CLAUDE.md) 前端守则 + 设计原则。设计系统 / 契约 / SSE / shell 各篇随对应代码落地后填充。

## 1. 一句话

Go 后端作 **sidecar**,Flutter 桌面端是其纯客户端。**3-tier feature-first**:`core`(跨切共享)→ `features`(各域)→ `app`(装配根 + shell)。**无 use-case/domain 层**——Go 二进制即用例,DTO 都是后端投影。

## 2. 物理结构（`frontend/lib/`，当前 = 视觉地基 + 运行时骨干 [Phase 4.0 STEP 0–7 已落]）

```
main.dart                  # 入口:runZonedGuarded(binding 在内)→ scaled binding → installErrorHandlers → initWindow → 恢复缩放档 → runApp(ProviderScope(AnApp))
app/                       # 装配根
  app.dart                 # 根 widget(MaterialApp + 主题 + home=AppStartupGate(AppShell) + builder=AnOverlayHost[持 navigatorKey];绑 Cmd +/-/0)
  app_shell.dart           # 唯一壳组合 AppShell(哪个 feature 在哪个岛):make app 与 make demo 共用,只差数据源 + 启动(见 §6)
  app_startup_gate.dart    # 据 backend 单一 phase 门控:连接中 / 崩溃可重试 / 就绪显壳(整 app 单点门控)
  window_setup.dart        # 桌面窗口:window_manager(尺寸/最小/居中 + hidden-at-launch:原生 order 钩子隐藏、show() 一次性显示、无启动闪烁)+ macos_window_utils(无边框 + 加高标题栏红绿灯)
core/                      # 跨切共享层(不依赖上层)
  runtime.dart             # DI 装配:activeWorkspace + backendController/Startup(BackendState phase 桥) + dio/apiClient + sseGateway(就绪前 null)
  contract/                # 后端投影 DTO(freezed/json,1:1 镜像后端):api_error(N1 信封 + AnselmErr 码) · page(N4 keyset/聚合) · workspace(+ModelRef) · entities/(Quadrinity ~22 DTO,见 contract.md)
  net/                     # api_client:唯一 HTTP 边界,标准契约只编码一次 + workspace/bearer(ANSELM_AUTH_TOKEN)拦截器
  sse/                     # 3 流地基:frame(线缆 + seq 派生 durable) · sse_parser · sse_connection(重连 + 410 续传 + full-jitter + bearer) · sse_gateway(per-scope/per-kind demux,Riverpod 之下)
  process/                 # backend_controller:sidecar 监督(抢端口 / health 门控 / 有界崩溃重启 / SIGTERM→kill 优雅关停 / 铸 ANSELM_AUTH_TOKEN)
  perf/                    # coalescing_notifier(L2:值同步无损、监听者每帧≤1 通知 —— 流式 firehose 防整页重建)
  error/                   # error_boundary:installErrorHandlers(全局错误汇)+ 可恢复 ErrorWidget(构建抛错不灰屏)
  design/                  # tokens · colors · typography · theme —— 唯一值源,禁内联 px/hex/ms
  platform/                # OS 缝:host_platform(dart:io 收口) · window_zoom(应内 Cmd +/- 缩放)
  model/                   # 框架无关纯模型(无 Flutter import):status_state(状态折叠单源)
  ui/                      # An* 套件 G0–G6(49 原语:控件/行卡/导航壳/代码数据/浮层)+ 三岛壳;桶=ui.dart(见 design-system.md)
  overlay/                 # 命令式浮层派发(G6):AnOverlayController(NotifierProvider) + overlayProvider + AnOverlayHost
i18n/                      # slang:en/zh_CN 双语 + 生成 strings.g.dart（dart run slang,入库）
dev/                       # dev 工具:gallery_main（make gallery 组件画廊）· demo_main（make demo:真壳 AppShell + fixture override + 跳门控,零后端）
features/                  # ★中间层:每域 data+state+ui+model（随 feature 落地,Entities 起）
  entities/data/           # Entities feature 数据缝[Phase 4.1 STEP 1]:单一 EntityRepository(Live 接 ApiClient+SseGateway / Fixture 内存可脚本 / entityRepositoryProvider 单点 override) + EntityKind/EntityRow/EntitySignal
  entities/state/          # Entities 列表 state[Phase 4.1 STEP 2]:entityListProvider(AsyncNotifier.family:首页+loadMore+SSE 就地 patch) + railModelProvider(rail VM) + selectedEntityProvider + railSortProvider(最近更新/名称)
  entities/ui/             # Entities UI[Phase 4.1 STEP 3]:EntityRail over AnSidebarList(4 kind 段 + 状态点 + 四态)+ entity_rail_model(纯投影) + entity_ocean(详情海洋,STEP 3 占位/STEP 4 建)
  entities/data/entity_demo_fixture.dart  # demoEntityRepository():make demo 的零后端种子(STEP 4/5 续加版本/日志/flowrun)
```
**运行时骨干(Phase 4.0)**:sidecar 进程托管(`core/process`)+ 契约/net/SSE(`core/{contract,net,sse}`,PORT 自 main + 加固)+ Riverpod 装配(`core/runtime.dart`)+ 错误边界(`core/error`)+ 启动门控(`app/app_startup_gate.dart`)+ L0–L2 流式性能原语(`core/sse` demux + `core/perf` coalescer)。loopback 安全在后端(绑 127.0.0.1 + bearer + Host 校验,见 `references/backend/api.md`)。建造规范见 [`WRK-045`](../../working/platform-foundation/phase-4.0-runtime-backbone.md)。
**dev 工具**:截图夹具 `test/dev/capture_shell.dart`(无头渲染 PNG 看效果);产物 `test/dev/out/` **gitignore**。

## 3. 依赖规则（三层，单向）

`app → features → core`。**features 互不依赖**(跨片走 core provider / 导航 intent);`core` 不依赖上层。UI 只用 `core/ui` + `core/design` 组合,**禁内联配色/度量**。

**命令式浮层派发(G6,跨 feature 共享的命令式 UI 副作用)**:dialog/toast 经 `core/overlay` 的 **`overlayProvider`**(经典 **`NotifierProvider`**,非 legacy `ChangeNotifierProvider`)派发——feature 在 SSE/async 回调里**无 BuildContext** 也能 `ref.read(overlayProvider.notifier).showToast(...)` / `confirm(...)→Future<bool>`(后者经装配根 `AnOverlayHost`[挂在 `MaterialApp.builder`]在 `initState` 注册的 **root navigator key** push `RawDialogRoute`)。这是「跨 feature 走 core provider」在命令式副作用上的落地——**非**全局 `navigatorKey` 单例(app 建 key + widget 树注入 host + ref 接入 controller、可 override 测、合 [`ADR 0004`](../../decisions/0004-frontend-flutter-architecture.md))。toast 层渲在内容之上(z 序偏离已拍板,详见 design-system.md)。完整建造规范见 [`WRK-041`](../../archive/g6-overlays/README.md)。

## 4. 设计系统 + UI 套件（`core/design` + `core/ui`）

- 令牌(`core/design`,单一值源):`tokens.dart`(`AnSpace`/`AnRadius`/`AnSize`/`AnMotion`)· `colors.dart`(`AnColors` ThemeExtension,明暗双值 + lerp,镜像 demo `tokens.css`)· `typography.dart`(`AnText`)· `theme.dart`(`ThemeData`)。
- **中性 chrome + toB 蓝 accent + 功能色**:`accent`=蓝(demo `#0071e3`,主动作/选中/run 显蓝);状态语义 ok/warn/danger。
- **字体**:UI=**随包 MiSans VF**(wght 150–700,Latin+简中、全平台同字面),**渲染压细**(正文 Light w300);代码=**JetBrains Mono**(随包)。详见 [`design-system.md`](design-system.md)。
- **套件 + i18n**:An\* 组件 + 图标(Lucide)/品牌图/状态折叠/交互基座 + slang i18n —— 详见 [`design-system.md`](design-system.md)(随套件逐组填充)。

## 5. 三岛 shell 骨架（`core/ui/an_shell.dart`）

无边框**不透明白窗**:左岛(`AnIsland` 卡,**弹性 240–400 默认 320、可拖**)· 敞开海洋(窗体白面、无卡,内容列**弹性 480–720**)· 右岛(`AnIsland` 卡,**固定 320**);四周 8px + 岛间 8px(左岛 grip 兼间距、右岛纯间距)。**两岛恒在,不收起。**
- **尺寸(逻辑点,`window_manager` 管 → scale 正确、resize 不炸)**:**最小** = 保证即便左岛拖到 max、海洋仍有最小内容列 `内距 + 左岛max(400) + 间距 + 海洋min(480) + 间距 + 右岛(320) + 内距` = **1232×761**(黄金比例高)。**默认** ≈ 1280×791(居中、1512 屏上留余量)。海洋是弹性区,内容列在 480–720 间随窗伸缩(更宽则 720 居中)。
- **红绿灯**:macOS 由 `macos_window_utils`(成熟包)**加高标题栏**(`addToolbar` + unified 风格)→ OS 把灯纵向居中到更低位、**仍在可点击的标题栏层**(Apple 旗舰做法)。**绝不**把原生按钮挪进内容区(会被全尺寸内容视图吃掉点击)、**绝不手搓**(见设计原则 #8)。Windows/Linux 此位放产品标 + 名(`AnWindowControls`)。
- **缩放(两种,别混)**:① **系统显示档**(设置→显示器)——全用**逻辑点**即自动适配,无需特殊处理;② **应内 Cmd +/-/0**(`core/platform/window_zoom.dart`)——用 `scaled_app`(`ScaledWidgetsFlutterBinding` 重写视图配置)**整体重排式**缩放(非 Transform/textScaler),默认 100%、离散档持久化,变更时窗口最小值同步 ×zoom。**zoom-in 受屏幕容量管控**(`maxFactor` = 屏可容 / 设计min,逐轴取小):到顶即停、**绝不撑破布局**;持久化档恢复时也按当前屏可容上限收敛。**不手搓**(原则 #8)。

## 6. 工具链与门禁

- 工具链 = **mise**(go + flutter,仓库根 `mise.toml`)。
- **三个启动面(规范,永不增 per-feature 入口)**:
  - `make gallery` — 组件画廊(`lib/dev/gallery_main.dart`):看每个 An* 原语全态。**视觉**面,与 app/demo 正交。
  - `make app` — 真 app(`lib/main.dart`):**真壳 + 真后端 sidecar**(启动门控等后端就绪)。
  - `make demo` — 真 app 壳 + **假数据**(`lib/dev/demo_main.dart`):看真实形态、零后端。
  - **铁律:app 与 demo 共用唯一壳组合 [`app/app_shell.dart`](`AppShell`)**——哪个 feature 在哪个岛只写一次。二者**只差两点**:① 数据源(app 接 Live repository / demo 用 `ProviderScope` override 成 fixture,见 `features/*/data/*_demo_fixture.dart`)② 启动(app 走 `AppStartupGate` 等后端 / demo 跳门控直接进壳)。**新 feature 接进 `AppShell` 一次,app 与 demo 同时拥有**——**绝不为单个 feature 加 `make <feature>` 入口**(碎片化、必不 sync)。截图同理:`test/dev/capture_demo.dart` 截整 `AppShell`,不做 per-feature 截图。
- 门禁 `make fe-verify`(= `cd frontend && make verify`)= codegen(`dart run slang` + `dart run build_runner`)+ `flutter analyze` 净 + `flutter test` 绿。codegen 产物入库(deterministic,fresh checkout 直接 analyze)。

## 7. 文档纪律

`references/frontend/` 随骨架 / feature **同提交**重写填充,与代码逐字同步(CLAUDE.md #9)。
