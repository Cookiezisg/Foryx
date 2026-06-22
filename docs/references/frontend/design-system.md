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

- `tokens.dart` —— 主题无关几何/时间:`AnSpace`(4 网格间距)· `AnRadius`(tag/button/chip/card/island/pill)· `AnSize`(行高 32、控件 28、图标 16/12/20、三岛列宽、窗体外廓)· `AnMotion`(fast 120 / mid 240 / slow 340 / breath 1800ms + easeOut/spring 缓动)。
- `colors.dart` —— `AnColors` ThemeExtension(明暗双值 + lerp,糖 `context.colors`)。**中性 chrome + toB 蓝 accent + 功能色**:`accent`=蓝(demo `#0071e3`/暗 `#0a84ff`)——主动作/选中/聚焦/run 状态显蓝;`ok`/`warn`/`danger`(+ soft)语义色。值镜像 demo `tokens.css`。
- `typography.dart` —— `AnText`,模数字阶锚 13px 正文。**UI=随包 MiSans VF**(`assets/fonts/MiSansVF.ttf`,变量字体 wght 150–700,Latin+简中,全平台同字面)+ PingFang SC 兜底;**渲染压细**——正文/标签/次级 Light(w300)、强调 Regular(w400)、标题 Medium(w500),摆脱 MiSans 在 Regular 下的厚重(ExtraLight 200 部分字偏细)。**代码=JetBrains Mono**(随包,OFL)。
- `theme.dart` —— 装配 `ThemeData`,注册 `AnColors` 扩展,剥 Material 涟漪 + **hover/highlight/focus/splash 全置透明**(表面自管态,杜绝 Material 默认灰叠加)。

## 2. 命名 + 纪律

- 组件类 `An<Name>`(`core/ui/`);文件 `an_<name>.dart`。纯框架无关模型在 `core/model/`(无 Flutter import)。
- **颜色/度量只走 token**:widget 内禁裸 hex/rgb/`Color(0x…)`/px(只有 `core/design` 可声明色值)——`make verify` 加 grep 门禁机械兜底（套件落地时接）。
- **悬停/选中底色淡入铁律**:`AnimatedContainer` 静止底用**目标色的 alpha=0**(`c.surfaceHover.withValues(alpha:0)`),**绝不用 `Color(0x00000000)`**(transparent black)——后者 lerp 到不透明浅色会经暗灰中点、产生"暗闪"(Flutter `Color.lerp` 官方坑)。
- **文案只走 i18n**:严禁硬编码中英文,走 slang `context.t.<key>`(见 §3)。
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

## 4. UI 套件路线（G0–G1 已落，G2–G6 待建，逐组同提交填本表）

| 组 | 组件 | 状态 |
|---|---|---|
| G0 地基设施 | AnIcons · AnBrandIcon · AnStatus/AnTone · AnInteractive · i18n | ✅ 已落 |
| G1 基础控件 | StatusDot · Badge(+Tone 色) · GroupLabel · Button · Input · ActionGroup · EditAffordance · Dropdown(+ **AnPopover** 浮层基座) | ✅ 已落 |
| G2 反馈态 | Skeleton · State · Callout · Stepper · Tags · Typewriter | ⏳ |
| G3 行与卡 | Row · RowDetail · Card · InfoCard · Section · Field · Kv · ThinTable · RefPill | ⏳ |
| G5 代码与数据 | CodeEditor · JsonTree · VersionDiff | ⏳ |
| G4 导航与壳 | Tabs · Toolbar · OceanHeader · RightIsland · SidebarList · Page · WireList | ⏳ |
| G6 浮层 | Menu · Dialog · Toast（复用 G1 的 **AnPopover** 基座） | ⏳ |

**G1 要点**:Button/Dropdown/EditAffordance 等可交互件都搭在 `AnInteractive` 上(态/激活/禁用一致);`AnInput` 用 `TextField`,**需 Material 祖先**(app 壳与 gallery 都提供)。`AnDropdown` 未做"桩"——已用 **AnPopover**(Flutter `OverlayPortal` + `CompositedTransformFollower`,点外/Esc 关)落地真菜单;此基座原计划在 G6,因 G3 的 Field/Kv 也需提早到 G1(G6 的 Menu/Dialog/Toast 复用它)。`AnTone→色`映射在 `core/ui/tone.dart`。`block`/`full`(占满宽)需有界父——无界父下优雅退化(不崩),见各件 LayoutBuilder。

**G6 AnFloating 待补**(对抗复审记录的两项浮层短板,提前到 G6 generalize AnPopover 时处理):① 浮层不做视口避让——触发器贴右边缘 + 窄窗时菜单可能溢出屏幕右沿(需 flip/avoid-viewport);② 首帧 `LayerLink.leaderSize` 为 null → 宽触发器(块级)菜单宽度有一帧跳变(需预量锚宽)。

**推迟到各自 feature**(耦合 SSE/reducer/图模型,非套件):Chat 的 BlockTree/Composer/ApprovalGate/EntityWorkspace · 图的 GraphCanvas/KindLegend · 调度的 NodeGantt/RunBoard · 文档的 DocEditor/Outline。

## 5. 验收（套件每组 + 整体）

- **gallery**(`make gallery`,G1 立起):双栏目录复刻 demo `reference.html`,每组件**全态**做成 specimen;截图 harness 逐类对照 demo 保真。
- **matrix widget-test**(进 `fe-verify`):复刻 demo `matrix.mjs` 5 维 —— 能 build / 受限栅格内不溢出 / 富文本转义安全 / 渲染存在 / disabled 键盘穿透;**+ 压力床**五电池(空/超长/海量/极值/注入)specimen。
- **工程纪律**:tokens 不硬编码(grep 门禁)· i18n 无硬编码中英文 · 层依赖 · S11 注释 · 本篇 1:1 同步。

## 6. codegen

slang:`dart run slang`(读 `slang.yaml` + `lib/i18n/*.i18n.json` → `strings.g.dart`)。产物入库(deterministic、fresh checkout 直接 analyze)。freezed/json 暂未引入——套件是纯 widget、`AnStatus` 是原生 enum,无需;待契约 DTO 层落地再接。
