---
id: DOC-045
type: reference
status: active
owner: @weilin
created: 2026-06-22
reviewed: 2026-06-22
review-due: 2026-09-22
audience: [human, ai]
---

# 前端设计系统 + UI 套件（An*）

> demo 的视觉语言（`demo/core/tokens.css` + `core/primitives/*`）以 app 级严谨度移植到 Flutter:
> **设计令牌**（值源）+ **An\* 组件套件**（原语），让 features 纯组装、零 bespoke 样式。
> 本篇随套件 **逐组同提交** 填充（见 §4 路线）。架构/分层见 [`architecture.md`](architecture.md)；决策见 [`ADR 0004`](../../decisions/0004-frontend-flutter-architecture.md)。

## 1. 设计令牌（`core/design`，唯一值源，禁内联 px/hex/ms）

- `tokens.dart` —— 主题无关几何/时间:`AnSpace`(4 网格间距)· `AnRadius`(tag/button/chip/card/island/pill)· `AnSize`(行高 32、控件 28、图标 16/12/20、光标 1.5×高 16、就地编辑最小宽 32、状态图标 40、骨架行 12、步进当前点 18、标签×命中 18、三岛列宽、窗体外廓)· `AnMotion`(fast 120 / mid 240 / slow 340 / breath 1800ms + 打字机 typePerChar 55/deletePerChar 28/typeHold 1400/typeGap 400 + easeOut/spring 缓动)。
- **`AnMotionPref`**(`tokens.dart`,挨着 `AnMotion`)——无障碍动效门控单源:`reduced(c)`/`reducedOrAssistive(c)`(`MediaQuery` aspect 访问器,只在标志翻转 rebuild)。**每个动画 An\* 件 build() 里读它**;装饰循环(shimmer/光标/打字机/breath)门控 `reducedOrAssistive`(屏幕阅读器活跃时持续动效是噪声)。见 §2 动效纪律。
- `colors.dart` —— `AnColors` ThemeExtension(明暗双值 + lerp,糖 `context.colors`)。**中性 chrome + toB 蓝 accent + 功能色**:`accent`=蓝(demo `#0071e3`/暗 `#0a84ff`)——主动作/选中/聚焦/run 状态显蓝;`ok`/`warn`/`danger`(+ soft)语义色 + `skeletonBase`/`skeletonHighlight`(骨架哑底+扫光,单色)。值镜像 demo `tokens.css`。
- `typography.dart` —— `AnText`,模数字阶锚 13px 正文。**UI=随包 MiSans VF**(`assets/fonts/MiSansVF.ttf`,变量字体 wght 150–700,Latin+简中,全平台同字面)+ PingFang SC 兜底;**渲染压细**——正文/标签/次级 Light(w300)、强调 Regular(w400)、标题 Medium(w500),摆脱 MiSans 在 Regular 下的厚重(ExtraLight 200 部分字偏细)。**每个样式同时给 `fontWeight` + 显式 `fontVariations('wght')`**——否则 `TextField`/`EditableText` 不渲染对的 VF 字重,就地编辑框会比展示文字更粗更宽。**代码=JetBrains Mono**(随包,OFL)。
- `theme.dart` —— 装配 `ThemeData`,注册 `AnColors` 扩展,剥 Material 涟漪 + **hover/highlight/focus/splash 全置透明**(表面自管态,杜绝 Material 默认灰叠加)。

## 2. 命名 + 纪律

- 组件类 `An<Name>`(`core/ui/`);文件 `an_<name>.dart`。纯框架无关模型在 `core/model/`(无 Flutter import)。
- **颜色/度量只走 token**:widget 内禁裸 hex/rgb/`Color(0x…)`/px(只有 `core/design` 可声明色值)——`make verify` 加 grep 门禁机械兜底（套件落地时接）。
- **悬停/选中底色淡入铁律**:`AnimatedContainer` 静止底用**目标色 alpha=0**——统一走 `Color.whenActive(active)` 扩展(`core/design/colors.dart`),绝不用 `Color(0x00000000)`(transparent black,lerp 到浅色经暗中点=暗闪)。
- **共享判定/令牌(去重单源)**:`Set<WidgetState>.isActive`(hover/press/focus 任一,`an_interactive.dart`)各控件统一取「视觉激活」;`AnOpacity.disabled` 统一禁用变暗;占满宽统一名 `block`(AnButton/AnInput/AnDropdown/AnActionGroup)。
- **交互基座 = `FocusableActionDetector`**(`AnInteractive`,原则 #8):hover/focus 由平台高亮模式驱动(焦点环只在键盘聚焦显)、Enter/Space 走标准 `ActivateIntent`、禁用即不可聚焦、`Semantics(button)` 无障碍。下拉菜单据此 + `FocusScope`/`FocusTraversalGroup` + autofocus 选中行得键盘导航(方向键 + Enter)。
- **文案只走 i18n**:严禁硬编码中英文,走 slang `context.t.<key>`(见 §3)。
- **动效 + 生命周期纪律(动画件铁律,源自 G2 调研 [`WRK-037`](../../archive/g2-feedback-states/README.md)):** ① **每个纯动效都附"成品感"静态兜底**(WCAG 2.2.2/2.3.3)——`AnMotionPref` 门控,reduced 时**不启动**循环(`.repeat()` 默认不被 `disableAnimations` 缩短)并渲染**收尾完成的静止帧**(Skeleton 静态哑底/Typewriter 完整主句/Stepper 瞬切/StatusDot 实心点);② **急切初始化**——controller/ticker/timer 绝不放 `late final =` 字段初始化器(惰性首次读可在 teardown 触发→崩),在 `initState` 赋值;③ **永远 dispose + mounted 守卫**;自重排 Timer 持有于字段、dispose 取消;④ **离屏即静默**——循环视觉走 vsync 控制器(TickerMode 离屏自停),Timer 仅用于离散相位切换;⑤ **单 ticker 默认**(`SingleTickerProviderStateMixin`);⑥ **作用域 rebuild**——`AnimatedBuilder(animation,builder,child)` + 循环叶子裹 `RepaintBoundary`;⑦ **异步状态→`Semantics(liveRegion)`(永远 polite),warn/danger 急迫另发 `SemanticsService.sendAnnouncement(assertive)`**;装饰动效 `ExcludeSemantics`;⑧ **状态不靠色单独**(图标+文字+朗读词,WCAG 1.4.1)。matrix 门禁加 **reduced 轴**(每 specimen 在 `disableAnimations` 下须 `pumpAndSettle` 收敛——忘门控的循环卡死=FAIL)。
- 注释 S11 双语 Why-not-What。

## 3. G0 地基设施（已落）

套件开建前的共享基座——所有 An\* 组件都依赖它:

| 设施 | 位置 | 职责 |
|---|---|---|
| **AnIcons** | `core/ui/icons.dart` | 语义图标单源:领域键 → Lucide 字形(`lucide_icons_flutter`)。渲染**细字重族 `Lucide300`**(≈demo 1.7 笔画,默认 `Lucide` 偏粗;包内各字重共享码点,改一处 `_family` 即换粗细)。具名字段(`AnIcons.agent`…)+ 数据驱动 `byKey`/`toolIcon`/`node`;未知 → `fallback`。移植 `icons.js` + `entity-kinds.js`。 |
| **AnBrandIcon** | `core/ui/an_brand_icon.dart` | 品牌/项目图标三源:`.anselm`(随包 app 标 SVG,细边描 squircle 轮廓)· `.svg`(内联 logo 串,currentColor→ink)· `.glyph`(字母圆角底兜底);`size` sm/md/lg + `managed`(accent 底)/`elevated`(浮起)。`flutter_svg` 渲染 `assets/brand/anselm-icon.svg`。 |
| **AnStatus / AnTone** | `core/model/status_state.dart` | 状态折叠单源(纯 Dart):后端任意状态字串 → 5 通用态(idle/run/wait/err/done)+ 语义 `tone`(err→danger/wait→warn/done→ok/run→accent/idle→none)。徽章/点不再各写 if 链。移植 `state-model.js`。 |
| **AnInteractive** | `core/ui/an_interactive.dart` | 交互基座:hover/focus/pressed/disabled 统一态(`Set<WidgetState>` 喂 builder),指针 + 键盘(Enter/Space)激活;**禁用时不可聚焦、指针/按键都不激活**(对齐 demo disabled-passthrough 门)。取代手搓 MouseRegion。 |
| **i18n（slang）** | `lib/i18n/` | 类型安全 `context.t.<key>`,`en`(base)+ `zh_CN` 双语;`TranslationProvider` 裹 app 根、`LocaleSettings.useDeviceLocaleSync()` 选语言。生成 `strings.g.dart` 经 `dart run slang`(**入库**,不走 build_runner)。 |

## 4. UI 套件路线（G0–G2 已落，G3–G6 待建，逐组同提交填本表）

| 组 | 组件 | 状态 |
|---|---|---|
| G0 地基设施 | AnIcons · AnBrandIcon · AnStatus/AnTone · AnInteractive · i18n（+ **AnMotionPref** 动效门控） | ✅ 已落 |
| G1 基础控件 | StatusDot · Badge(+Tone 色) · GroupLabel · Button · Input · ActionGroup · EditAffordance · **InlineEdit** · Dropdown(+ **AnPopover** 浮层基座) | ✅ 已落 |
| G2 反馈态 | Callout · State · Stepper · Skeleton · Typewriter · Tags（+ **DryIntrinsicWidth** 共享原语） | ✅ 已落 |
| G3 行与卡 | Row · RowDetail · Card · InfoCard · Section · Field · Kv · ThinTable · RefPill（+ **AnTwoZone** 右锚两区共享原语 · AnEditableValue · AnAutoGrid） | ⏳（G3.1 AnTwoZone 已落） |
| G5 代码与数据 | CodeEditor · JsonTree · VersionDiff | ⏳ |
| G4 导航与壳 | Tabs · Toolbar · OceanHeader · RightIsland · SidebarList · Page · WireList | ⏳ |
| G6 浮层 | Menu · Dialog · Toast（复用 G1 的 **AnPopover** 基座） | ⏳ |

**G1 要点**:Button/Dropdown/EditAffordance 等可交互件都搭在 `AnInteractive` 上(态/激活/禁用一致);`AnInput` 用 `TextField`,**需 Material 祖先**(app 壳与 gallery 都提供)。`AnDropdown` 未做"桩"——已用 **AnPopover**(Flutter `OverlayPortal` + `CompositedTransformFollower`,点外/Esc 关)落地真菜单;此基座原计划在 G6,因 G3 的 Field/Kv 也需提早到 G1(G6 的 Menu/Dialog/Toast 复用它)。`AnTone→色`映射在 `core/ui/tone.dart`。`block`/`full`(占满宽)需有界父——无界父下优雅退化(不崩),见各件 LayoutBuilder。**AnDropdown 两区**(**`AnTwoZone`**,触发器与菜单行共用;G3.1 已从 private `_TwoZone` 升格为 `core/ui/an_two_zone.dart` 公共共享原语——G3 的 Section·InfoCard head·Row·Kv 尾槽都复用此骨架、不再各搓 Row+Spacer,原则 #8):label 占满左 + meta 上限右(≤45%)+ trailing(任意 Widget,如箭头/勾/动作)钉右,两者各自省略。**菜单宽 = 触发器宽夹 `[menuMin 200, menuMax 360]`**(紧凑 ghost 触发器也容富行、不溢出);开/关有 **fade + 自顶微缩放**动效(`AnPopover`,`AnMotion.fast`)。**`AnPopover` 的 `AnimationController` 在 initState 急切创建**(非懒 `late final =`,否则没开过的浮层在 dispose 才首次访问→崩)。

**`AnInlineEdit` 就地重命名**(组合 `AnEditAffordance` + seamless `AnInput`,定高行、切换不跳):idle 文字 + 铅笔跟字尾(超长省略 + 铅笔钉右);editing 换自适应框,**随打字增长、撑满行宽后按钮(取消/保存)钉右、框横滚**(光标可见)。Enter 存、Esc 弃、进编辑全选。增长封顶用**框架原生 `IntrinsicWidth`**(经共享 `DryIntrinsicWidth`(`core/ui/dry_intrinsic_width.dart`)垫片绕开 TextField 在嵌套/滚动上下文的 dry-layout 断言)按**输入框自身渲染树**定宽——所见即所得,**非**逐键 `TextPainter` 量宽(后者须逐字节重镜像 `AnText.body`,即 typography 记过的变量字体宽度漂移坑);本地化按钮宽经 Row 的 `Flexible` flex pass **自动让位**(无须手算 `−按钮宽`)。光标高 `caretHeight=16`(< 行高 18.2、贴合文字;kit 内**单行** `AnInput` 共用,多行用 Flutter 默认随行缩放);空框 `inlineEditMin=32` 兜底可点;尾部留 `caretEndPad=3`(光标宽+一丝)防行尾末字符被光标压住(flutter#24612);rename 不给 placeholder 避 hint 宽污染(flutter#93337);`startEditing` 进编辑即全选(Finder/F2 习惯)。`DryIntrinsicWidth` 同被 `AnTags` 内联添加框复用。

**G2 反馈态要点**(全 HAND-ROLL、**零新增包**;动画件遵 §2 动效纪律 + `AnMotionPref` 门控):**AnCallout** 通栏语气条(severity→AnTone:info→accent;两机制 a11y:polite liveRegion + warn/danger assertive announce;Stateful 仅为 announce)· **AnState** 空/载/错整块(error **单色**、红留 Callout;loading 仅短等待用 `CircularProgressIndicator.adaptive`、内容加载委托 AnSkeleton;一个 Semantics 读「标题. 提示」)· **AnStepper** 步进点(done/current/upcoming 三态各异、不靠色;1-based;**带 `onStepTap`**——已完成节点 AnInteractive + 「跳到第N步」按钮标签)· **AnSkeleton** 骨架(一个 ShaderMask `srcATop` 扫光,线性平移、`RepaintBoundary`;reduced=静态哑底;row/card/text/lines)· **AnTypewriter** 打字机(一个 controller 相位机 type→hold→delete→循环,**字素安全** `String.characters`;光标移动实/停顿呼吸;两层 a11y:动画文字 ExcludeSemantics + 外层暴露完整短语、liveRegion=false)· **AnTags** 可编辑标签集(Wrap 药丸 + 健康点 + 内联添加;重复拒+闪;空框 Backspace 删末;每×自有按钮语义)。

**G6 AnFloating 待补**(对抗复审记录的两项浮层短板,提前到 G6 generalize AnPopover 时处理):① 浮层不做视口避让——触发器贴右边缘 + 窄窗时菜单可能溢出屏幕右沿(需 flip/avoid-viewport);② 首帧 `LayerLink.leaderSize` 为 null → 宽触发器(块级)菜单宽度有一帧跳变(需预量锚宽)。

**推迟到各自 feature**(耦合 SSE/reducer/图模型,非套件):Chat 的 BlockTree/Composer/ApprovalGate/EntityWorkspace · 图的 GraphCanvas/KindLegend · 调度的 NodeGantt/RunBoard · 文档的 DocEditor/Outline。

## 5. 验收（套件每组 + 整体）

- **gallery**(`make gallery`,G1 立起):双栏目录复刻 demo `reference.html`,每组件**全态**做成 specimen;截图 harness `capture_gallery.dart` 逐类(`--dart-define=CAT=<i>`)出 `gallery_<i>.png` 对照 demo 保真(非 `_test.dart`、不进自动门禁;强制 reduced 出确定性静帧)。**动画件另真机 `flutter run -d macos`(Impeller)截图核对**(无头=Skia,渲染差异见会话 Impeller 调研)。
- **matrix widget-test**(进 `fe-verify`):复刻 demo `matrix.mjs` —— 能 build / 受限栅格内不溢出 / 富文本转义安全 / 渲染存在 / disabled 键盘穿透 / **reduced-motion 轴**(每 specimen 在 `disableAnimations` 下 `pumpAndSettle` 须收敛——忘门控的循环卡死=FAIL);**+ 压力床**五电池(空/超长/海量/极值/注入)specimen。
- **工程纪律**:tokens 不硬编码(grep 门禁)· i18n 无硬编码中英文 · 层依赖 · S11 注释 · 本篇 1:1 同步。

## 6. codegen

slang:`dart run slang`(读 `slang.yaml` + `lib/i18n/*.i18n.json` → `strings.g.dart`)。产物入库(deterministic、fresh checkout 直接 analyze)。freezed/json 暂未引入——套件是纯 widget、`AnStatus` 是原生 enum,无需;待契约 DTO 层落地再接。
