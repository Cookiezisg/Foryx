---
id: WRK-038
type: working
status: active
owner: @weilin
created: 2026-06-23
reviewed: 2026-06-23
review-due: 2026-09-21
audience: [human, ai]
landed-into:
---

# G3 行与卡套件 —— 联网调研已确认的建造规范（开工前对齐用）

> **来源**：开工前的完整联网 best-practice 扇出（workflow `g3-rows-cards-research`：2 地基盘点 + 6 跨切面联网 + 9 逐件方案 + 5 镜对抗复审 + 综合；外加 2 个补研 agent 把首轮 schema 失败的 **T1 多列对齐 / T2 尾槽互换** 两硬题联网补全，见本篇附录）。
> **用法**：这是 G3（Row/RowDetail/Card/InfoCard/Section/Field/Kv/ThinTable/RefPill + 共享原语 AnEditableValue/AnTwoZone/AnAutoGrid）的**建造事实源**——逐件按本篇做。§0 系统结论 + §1 共享原语是 kit-wide 绑定；§2 逐件方案卡随各件提交落地；**§7 reduced 政策落地时提取进 [`design-system.md`](../../references/frontend/design-system.md) §2**。
> **决策前置**：下面「决策前置」5 条需先与人对齐，再开 §9 G3.1 pre-work。
> **两硬题权威落地**：AnThinTable（§2 G3-e）与 AnRow 尾槽（§2 G3-f）的最终 Flutter 落地定论见**附录 A（T1/T2 联网补研）**——首轮该两题 schema 重试超限丢失，已单独补研、有出处佐证，附录优先于卡内的初稿措辞。
> 视觉/分层/令牌见 [`design-system.md`](../../references/frontend/design-system.md)；架构见 [`ADR 0004`](../../decisions/0004-frontend-flutter-architecture.md)；上一组见 [`g2 archive`](../../archive/g2-feedback-states/README.md)。

---

## 决策前置（开工前对齐，5 条）

> **5 条已于 2026-06-23 全部拍板（均取推荐项）**，下列 ✅ 即定稿，据此开工 G3.1。

1. **blur 失焦提交**：`AnEditableValue` 是否采纳 demo 的「失焦即提交」（`onTapOutside`）？采纳 → 给 `AnInput` 加 `onTapOutside` 透传（地基增强）+ `AnInlineEdit` 一并获得 + ✓✕ 用 `TextFieldTapRegion` 物理隔离 + `_finished` abort 优先守卫。**✅ 已定：采纳（地基增强）**——demo 核心 UX 是失焦提交，密集列表里失焦保持编辑态反而易丢改；一次强化地基让两套编辑核统一（#8）。
2. **header 语义真机**：`AnSection`/`AnThinTable`/`AnInfoCard` 标题用 `Semantics(header:true)`（kit 内零先例）——是否落地前强制真 macOS VoiceOver 验证？**✅ 已定：先真机验证再用**（verify-by-real-run 铁律；不通过则退回 plain labeled Text）。
3. **AnAutoGrid 实现路径**：用 Flutter 原生 `GridView + SliverGridDelegateWithMaxCrossAxisExtent` 还是手搓 `LayoutBuilder + Wrap + SizedBox` 等分？原生 delegate 强制 `childAspectRatio`＝全表统一行高，违 demo `align-items:start`。**✅ 已定：先实测原生 delegate**（#8「自研网格必须先证伪标准 delegate」），证伪后再退手搓。
4. **caption 字重 w500/w600**：`AnSection` caption 复用 `AnGroupLabel` 的 w500（自称 inspector sections 单源）还是 demo 的 w600？**✅ 已定：w500 复用单源**（12px 大写 meta 上字重差不可辨，另立 w600 违既有单源声明且制造两档维护面；demo 仅视觉参考非命名事实源 #4）。
5. **AnKv/AnField 同文件**：`AnField + AnKv + AnEditableValue + 共享叶子`是否同一 `an_field.dart`（贴 demo `field.js` 同域）？**✅ 已定：同文件**（共享编辑核 library-private 不外泄，贴 demo 结构最直观）。

---

## G3「行与卡」WRK 工作规范

> 阶段:**仅调研出方案,不写组件代码**。本规范吸收四镜复审的 HIGH/MED 修正,给出可直接落地的逐件方案卡 + 跨件共享原语 + token/i18n 汇总 + 构建子步顺序 + 三层验收。
> 验证基线:已逐字核对 `demo/core/tokens.css`、`demo/core/primitives/{row,row-detail,field,section,thin-table,ref-pill,card,info-card}.js`、既有 `frontend/lib/core/ui/*`、`frontend/lib/core/design/{tokens,colors,typography}.dart`、`gallery/catalog.dart`、`test/dev/gallery_matrix_test.dart`。**所有 demo token 值已查证锚定,不留「实测待定」暗门**(原则 #8「遇不确定先查」)。

---

### 0. 概述 — G3 件清单与共享地基

G3 共 **9 件展示/布局件** + **3 个必须先行抽出的共享原语**。展示件:`AnSection · AnKv · AnField · AnRefPill · AnThinTable · AnRow · AnRowDetail · AnCard · AnInfoCard`,外加布局件 `AnAutoGrid`(被 AnSection grid 依赖)。

四镜复审的系统性结论(全部吸收):
1. **契约事实源 = backend,不是 demo**(原则 #4)。RefPill kind→图标走既有 `AnIcons.byKey`(已含 11 类全集 + chrome + `?? fallback`),**不另抄 9 行映射表**。
2. **VF 字重双轴铁律**:本 kit typography 每个 style 显式带 `fontVariations:[FontVariation('wght',N)]`,裸 `copyWith(fontWeight:)` **不改 wght 轴 = 静默失效**。改字重必须双轴同改,或直接用命名 style。
3. **color token 唯一访问 = `context.colors.X`**(ThemeExtension 实例字段);`AnColors.inkFaint` 静态不存在、`AnColors.light.X` 锁死亮色破坏 lerp。全文用 `context.colors.*`。
4. **token 不开语义别名口子**:`AnSpace` 是刻意的纯值阶梯(tokens.dart 头注「Value-named」),语义密度量归 `AnSize`。既有值能覆盖就不造名——`padRow→AnSpace.s8`、`gridMinCol→AnSize.block` 直接复用,**不新增 AnSpace.padRow / AnSize.gridMinCol**。
5. **共享编辑核 `AnEditableValue` 是净新增件、非「薄提取」**:既有 `_EditZone`/`AnInlineEdit` 只有 Enter/Esc,**没有** onTapOutside / `_finished` 竞态守卫 / 退出焦点回落 / announce。这些是净新增的 a11y+生命周期代码。
6. **功能性一次性微动效豁免 reduced 门控**(focus/hover/press 反馈),只有 **loop + 功能性 reveal** 需门控——此豁免应写进 `design-system.md §2`,否则每件 reduced 主张都对着未定义标尺。
7. **Semantics 不过度包裹**:含独立可达叶子(button/textField)的子树**绝不** `MergeSemantics` 或套显式 role-Semantics,改用 `explicitChildNodes` 容器。

---

### 1. 跨件共享原语(必须先于消费件落地,单一作者)

#### 原语 A — `AnEditableValue`(双锚就地编辑核,library-private,净新增)
- **归属**:`an_field.dart`(与 AnField/AnKv 同文件,贴 demo `field.js` 同域一文件)。
- **为何不复用整件**:`AnEditAffordance`=单锚一体三连;`AnInlineEdit`=单锚 grow-then-pin + select-all 重命名语义。Field/Kv 是**双锚**(铅笔贴 key 右、✓✕ 贴 value 右)+ **改值光标落末尾**(非全选)——几何拓扑与编辑语义都不同。**只复用叶子机制**(seamless 框 + `caretEndPad` + `inlineEditMin` + Esc 绑定),不复用容器。
- **关键修正(吸收 a11y/reuse HIGH)**:这是 **StatefulWidget 净新增件**,必须自己实现:
  - `onTapOutside` 失焦提交 + `TextFieldTapRegion` 包 ✓✕(点 ✓✕ 不触发 onTapOutside = 取消优先的物理隔离)。**这需要给 `AnInput` 加 `onTapOutside` 透传参 = 地基增强**(见原语 D),非「复用」。
  - `_finished` 一次性守卫,语义为 **abort 路径(✕/Esc)优先抢占**:`abort()` 与 `commit()` 共用 `_finished` bool,✕ 的 onTap 同步置位先于任何 blur 回调。
  - **退出编辑焦点显式回落到铅笔**(新建 pencil `FocusNode` + `requestFocus`,对齐 WAI-ARIA grid:Esc 关编辑返回单元),否则 200+ 行列表里焦点掉 body = WCAG 2.4.3 回归。
  - 进编辑 `SemanticsService.announce(t.a11y.editingField(field), polite)` + **隐式 Semantics 兜底**(announce 部分平台限 web)。
  - **行高 + 竖向内距参数化外提**(吸收 contract-fidelity HIGH):`rowHeight` + `padV` 由调用处传——Field 传 `AnSize.islandHead(44)`+`AnSpace.s4`,Kv 传 `AnSize.row(32)`+`AnSpace.s4`。**绝不把行高写死进核**,否则 Field/Kv 保真二选一必崩。
  - G2 生命周期铁律:`_ctl` / pencil `_focus` 在 `initState` 急切建、`dispose` 释放、`setState` 前 `mounted` 守卫,绝不放 late-final 初始化器。
- **编辑核内部叶子**:`DryIntrinsicWidth` + `ConstrainedBox(minWidth: AnSize.inlineEditMin)` + `EdgeInsetsDirectional.only(end: AnSize.caretEndPad)` + `AnInput(seamless, autofocus)`。**不给 placeholder**(污染固有宽 flutter#93337),空态占位用 idle 的 `—` 文本。
- **进编辑不全选**:光标落值末尾(`collapse(false)`,改值语义),**不照抄 `AnInlineEdit._selectAll`**(那是重命名语义)。
- **blur-commit 拍板(见 decisions)**:demo `finish(true)` 在失焦即提交。若采纳 blur-commit,须把它列为 `AnInput` 地基增强 + 让 `AnInlineEdit` 一并获得(避免两套失焦语义);若不采纳,与既有 AnInlineEdit 对齐(只 Enter/✓ 提交)。
- **`[doc-fix]` G3.6 落地修正(本节即重述,与落地代码对齐)**:① **AnEditableValue 公开**(非 library-private)——独立文件 `an_editable_value.dart` 导出,为可单测 + 可复用(是正经 kit 编辑核,非内部细节);② **select 编辑器 = 常驻 ghost `AnDropdown`**(非 demo 的 pencil→下拉)——下拉自管 open/close/pick/dismiss,**根除「外点/Esc 关浮层却卡在编辑态」的悬空 bug**(HIGH 修);input 编辑器才走 pencil→field→✓✕;③ **blur-commit 采纳**:`AnInput.onTapOutside`(地基增强,原语 D)已落,AnEditableValue 用之、✓✕ 套 `TextFieldTapRegion` 取消优先;**`AnInlineEdit` 不取 blur-commit**(`onTapOutside:null`)——重命名点别处不该静默改名,与「值编辑」语义区分(非「两套失焦语义」,是 input-rename 有意不失焦);④ **共享叶子 `AnSeamlessField`** 已提取(AnInlineEdit 重构复用、byte-equal);⑤ **原语 D tabular** 已落:`AnInput` mono 路径 + AnEditableValue mono 展示值加 `FontFeature.tabularFigures`;⑥ commit 去首尾空白(同 demo);焦点 Enter/Esc 回落铅笔、失焦不回落;`SemanticsService.sendAnnouncement`(非 deprecated announce)。

#### 原语 B — `AnTwoZone` 升格(右锚两区布局,从 AnDropdown 提为顶层共享)
- **现状**:`_TwoZone` 已在 `an_dropdown.dart` 落地(private),是「左填充 Expanded + 右 meta 上限 ≤45% + trailing 钉右」骨架,被 dropdown trigger + menu row 共用。
- **复审裁决(reuse HIGH)**:此骨架在 **AnDropdown / AnKv 行 / AnSection head / AnRow trail** 四处复现。**升格为顶层共享原语**(提到 `lib/core/ui/an_two_zone.dart`,导出),消费件直接走它,不各搓 Row+Spacer。
- **合理泛化点**:当前 `meta` 是单文本槽;泛化为「trailing 区可放任意 Widget(idle Text ↔ editing 框 / actions)」——这是泛化、非另起炉灶。
- **重构边界**:升格 = 改 `an_dropdown.dart` 引共享件 + 同提交 AnDropdown 回归测试 + `design-system.md` 1:1 同步。须确认 AnDropdown 行为 byte-equal。

#### 原语 C — `AnAutoGrid`(auto-fit 响应式块网格,被 AnSection grid 依赖)
- **手搓 vs 标准 widget 的 #8 依据(吸收 reuse HIGH 的强制证伪)**:
  - **首选验证 `GridView` + `SliverGridDelegateWithMaxCrossAxisExtent(maxCrossAxisExtent: AnSize.block)`** —— 这是 CSS `auto-fit minmax` 的**框架原生对应**,极可能免手搓 LayoutBuilder 算列。**但它强制 `childAspectRatio` 全表统一宽高比 = 全表统一行高**,违 demo `align-items:start` 各行按内容定高;且在 Column 里需 `shrinkWrap`+`NeverScrollable`,本质为滚动设计。**先实测此冲突再决定是否退而手搓**。
  - `flutter_layout_grid` 的 `minmax/auto-fit`「planned but NOT implemented」(GitHub issue #25),引整包不划算。
  - 裸 `Wrap` 各行按内容定高 ✓ 但不能等分拉伸(flutter#135301/#135284 未合)。
  - **结论**:若原生 delegate 的统一行高不可接受,落地 `LayoutBuilder(算列数) + Wrap(标准) + SizedBox(等分列宽)` 薄封装——本质是「编排三个标准 widget + 一行确定性公式」,非造算法。
- **列数公式(含 gap 补偿,易漏)**:`n = ((W+gap)/(minColWidth+gap)).floor().clamp(1, ...)`;等分列宽 `colW = (W-(n-1)*gap)/n`(用 double 除法,**不取整**,避免末列累积误差)。
- **守卫**:`if (!W.isFinite)` 退化单列(误放横向无界容器 → colW=NaN 崩);`clamp` 下界保底 1(避免除零)。
- **token**:`minColWidth` 默认 `AnSize.block(=280)`(=demo `--w-block`),`gap/runGap` 默认 `AnSpace.s16`(=demo `--sp-4` grid gap)。**无新增 token**。
- **a11y/reduced**:纯静态布局容器,**不加 Semantics 包裹**(否则把卡片塞进一个语义容器破坏逐卡遍历),阅读序=children 序;**无动效 → reduced N/A**(列数随 resize 是布局重算非动画)。
- **独立验收**:必须先于/同期 AnSection 落地 + 独立进 matrix。

#### 原语 D — `AnInput` 地基增强(若采纳 blur-commit)
- **显式标为地基增强,非「复用」**(吸收 reuse HIGH):`AnInput` 当前不暴露 `onTapOutside`、不包 `TextFieldTapRegion`。新增 `onTapOutside` 透传参 → `AnInput`/`AnInlineEdit`/`AnEditableValue` 同获能力 + `design-system.md` 1:1 同步 + 既有消费者回归。
- **附带统一 tabular**:idle Text 与 editing 框数字字形必须同源。**value 数字列无条件 `tabularFigures`**(见 AnKv/ThinTable),mono 开关只切字体族——故 `AnInput` mono 路径也应加 `fontFeatures:[FontFeature.tabularFigures()]`(地基侧统一)。

---

### 2. 逐件方案卡

#### G3-a · AnSection — 小节容器
- **API**:`AnSection({String? label, List<Widget> actions = const [], AnSectionVariant variant = caption, bool grid = false, required List<Widget> children, double? gridMinColWidth, String? semanticLabel})`。`enum AnSectionVariant { caption, plain }`。
  - **actions 改收 `List<Widget>`(吸收 consistency MED)**:对齐既有 `AnCallout.actions`,内部用 `AnActionGroup(end: true)` 排布(复用、零重造)。两个容器件 actions 入参同构。
- **复用**:head 行走 **`AnTwoZone`**(label 左 Expanded + actions 右锚);body `grid=false` → Column 织入间距;`grid=true` → 委托 **`AnAutoGrid`**(阻塞依赖,标注于风险)。
- **caption label(吸收 consistency HIGH + token HIGH)**:**默认 w500,复用 `AnGroupLabel` 字重单源**(它自称「inspector sections 单源」,12px 大写 meta 上 w500/w600 几乎不可辨,不值分叉)。把 `AnGroupLabel` 的 padding 参数化(`padding` 可选,默认保留 rail 邻近内距,Section 传 `EdgeInsets.zero`)让两处共享同一字色/字重源——**杜绝 `inkFaint+meta+uppercase` 出现第二份字面拷贝**。
- **plain label(吸收 consistency MED)**:用 `AnText.strong`(16/w400)作**有意的最小标题档**(低于 h3),在 `design-system.md` 明确记录「section plain = strong 16,非 ramp 不一致」。行高接受 strong 默认 1.4(demo lh-tight=1.25 单行差 <1px,不为单点引行高 token)。
- **token(全查证)**:段底距 caption=`AnSpace.s24`(--sp-6) / plain=`AnSpace.s32`(--sp-8);head→body caption=`AnSpace.s8`(--sp-2) / plain=`AnSpace.s12`(--sp-3);body 块间 `AnSpace.s12`(--sp-3);grid gap `AnSpace.s16`(--sp-4);caption head 横向光学内缩 `AnSpace.s2`(=--grid/2,**实现注释写明是 grid/2 派生、非独立间距,随 AnAutoGrid gap 模型变更须核对**);色 `context.colors.inkFaint`(caption)/`context.colors.ink`(plain)。**无新增 token**。
- **a11y**:外层 `Semantics(container, explicitChildNodes)`(不 merge,子块各自可达);label → `Semantics(header: true)`。**`header:true` 是 kit 内首次使用的新模式(吸收 a11y MED)**:落地前须按 verify-by-real-run 在真 macOS VoiceOver 上验证读作标题,否则退回 plain labeled Text。head Row 结构上 label(Expanded)物理先于 actions = 阅读序首位天然满足,作为结构不变量声明,matrix 加 Semantics-order 断言。
- **reduced**:N/A(纯静态)。**但 grid=true 委托 AnAutoGrid → reduced-N/A 保证以 AnAutoGrid 无动效为前提**(吸收 a11y LOW),matrix 加 grid specimen 让 reduced 轴透传覆盖 AnAutoGrid。
- **`[doc-fix]` G3.4 落地修正(本规范本节即重述,与落地代码对齐)**:① **actions 用 `AnActionGroup`(不带 `end`)**——`AnTwoZone` 已右锚 trailing,`AnActionGroup(end:true)` 会包 `SizedBox(width:infinity)`、在 Row 内撑无限宽崩溃(上文 §101 的 `end:true` 作废);② **`grid` / `gridMinColWidth` 参数 G3.4 不暴露**,随 `AnAutoGrid` 在 **G3.5** 一并上(避免半成品 API);③ Semantics-order 不变量已由 `an_section_test` 的 reading-order 断言锁(非 matrix);④ `header:true` 真机 VoiceOver 验证仍 **待补**(decision②,环境无法跑音频,作人工后续)。

#### G3-b · AnKv — 紧凑定义列表
- **API**:`AnKv({required List<AnKvRow> rows, ValueChanged<List<AnKvRow>>? onChanged, bool mono = false, bool wrap = false})`。
  - **回调改 whole-value(吸收 consistency HIGH)**:`ValueChanged<List<AnKvRow>>?` 对齐 `AnTags`(同为 list 编辑器、应同构),弃 demo 的 `(key,value,index)` 三位置参孤儿。
  - `class AnKvRow { final String label; final String? value; final bool editable; final AnKvFieldKind editor; final List<AnDropdownOption<String>> options; }` —— **字段 `key`→`label`(吸收 consistency LOW,避 Widget.key 撞名,对齐 AnTag/AnDropdownOption 命名族)**。`enum AnKvFieldKind { input, select }`(**`Editor` 后缀→`Kind` 族**,吸收 consistency MED)。
- **行布局**:走 **`AnTwoZone`**(key 左 + 铅笔槽 + 撑开 + value 区 + ✓✕ 槽);编辑核走 **`AnEditableValue`**(传 `rowHeight: AnSize.row(32)`,`padV: AnSpace.s4`=--sp-1)。
- **token(全查证)**:行高 `AnSize.row(32)`(min-height 语义、padding 内含非外加);竖向内距 `AnSpace.s4`(--sp-1)、水平内距 `AnSpace.s8`(=--pad-row,**已查证 demo `--pad-row:8px`,不是决策**);行内 gap `AnSpace.s8`(--sp-2);✓✕ 间距 `AnSpace.s6`(--gap-tight);行 hover 底 `context.colors.surfaceHover`(=island-3,`whenActive` no-flash 淡入)、圆角 `AnRadius.button`;编辑框白底 `context.colors.surface`(=island)、inset 描边 `context.colors.lineStrong`、圆角 `AnRadius.tag`;key 色 `context.colors.inkMuted`(ink-2)+`AnText.body`;value 色 `context.colors.inkFaint`(ink-3)+`AnText.meta`。**无新增 token**。
- **tabular(吸收 contract MED)**:**value 数字无条件 `FontFeature.tabularFigures()`**(demo `.v{font-variant-numeric:tabular-nums}` 常驻),mono 开关**只切** `AnText.mono` 字体族(demo `[mono]{font-family}` 分离)。写法 `style.copyWith(fontFeatures: const [FontFeature.tabularFigures()])`。
- **wrap**:`true`=value softWrap 左对齐 + `overflow-wrap anywhere`(无空格长串可断);`false`=单行 ellipsis 右对齐。
- **a11y(吸收 a11y HIGH:分支显式化)**:
  - **只读行**(`onChanged==null || !editable`,无交互叶子):`MergeSemantics > Semantics(label: 行模板, value)` 读「label: value」。
  - **可编辑行**(铅笔在树):**绝不 MergeSemantics**(会吞铅笔 button),改 `Semantics(container, explicitChildNodes)` 让 key+value 文本连贯 + 铅笔/✓✕ 各自可达。
  - 编辑态 value 区:**不**套 `Semantics(textField:true)`(`AnInput`/`EditableText` 已自带,双包=重复节点);只传 label(经 AnInput 的 label affordance 或 `Semantics(label)` 不带 textField),确保每编辑格恰一个 textField 节点。
  - 铅笔=`AnButton.iconOnly(semanticLabel: t.action.edit)`;✓=`t.action.save`、✕=`t.action.cancel`,保存 accent **另带「保存」文字**(不靠颜色单独表意 WCAG 1.4.1)。
  - `FocusTraversalGroup` 保证 Tab 序 key→铅笔→value→✓✕。
- **reduced(吸收 a11y MED:门控单源显式化)**:AnKv 自身 hover 行底 + 铅笔 Opacity 揭示是**功能性反馈 → 读 `AnMotionPref.reduced`**(非 reducedOrAssistive,屏读用户未请求 reduced 时不该被剥夺 hover 反馈),真时 `Duration.zero` 即时切。**继承动效须如实列举**(吸收 a11y HIGH):`AnInput` focus-border `AnimatedContainer` + select 编辑器 `AnDropdown` caret `AnimatedRotation` 均为**未门控但一次性 settle**的功能性微动效——按豁免策略可接受(matrix-safe),方案显式声明、不假称「全覆盖」。无 `.repeat` loop → matrix reduced 轴天然不超时。
- **重构同步**:`AnEditableValue` 提取需同改 `an_inline_edit.dart`/`an_edit_affordance.dart`(去私有 `_EditZone`/`_SaveButton` 引共享叶子)+ 回归测试 + `design-system.md` 同提交。提取前先订正 `an_edit_affordance.dart` 注释(自称「墨/单色」实为 accent 皮肤,`[doc-fix]`)。

#### G3-c · AnField — 键值大行(承载控件)
- **API**:`AnField({required String label, String? hint, String? value, bool editable = false, AnKvFieldKind editor = input, List<AnDropdownOption<String>> options = const [], bool wrap = false, Widget? child, ValueChanged<String>? onChanged})`。
- **passive/slot 态(吸收 contract MED,Field 的本质能力,易漏)**:复刻 demo `hasValueAttr` 三岔——
  - `value != null` + `editable` → 走 `AnEditableValue` 文本/select 编辑路径(铅笔出);
  - `value != null` + 非 editable → 只读 value Text(右对齐,无铅笔);
  - `value == null` → 渲调用方传入的 **`child`(任意控件:下拉/开关,右对齐)**,**无铅笔、无编辑核**。这是 Field 区别于 Kv 的本质(Field 承载控件,Kv 只承文本)。
- **行高(吸收 contract HIGH)**:`AnEditableValue` 传 `rowHeight: AnSize.islandHead(44)`(=demo `--field-row`/`--island-head` 阅读行)+ `padV: AnSpace.s4`(=--grid)。**与 Kv(32)不同 → 共享核必须参数化外提**。
- **复用**:行外壳走 `AnTwoZone`(label+hint 列 左 / 铅笔槽 / 撑开 / value-or-child 右 / ✓✕);hint = `AnText.meta` + `context.colors.inkFaint` + softWrap anywhere。
- **`[doc-fix]` G3.8 落地修正(本节即重述,与落地代码对齐)**:① **行外壳手搓 `Row`(key-hug + value 撑右几何),不走 AnTwoZone**——AnTwoZone 的 label 强制 Expanded 几何不适此行,同 AnKv(上文「行外壳走 AnTwoZone」作废,与 §1 原语 B 的「AnKv 不用」一致);可编辑态走 **`AnEditableValue`**(`rowHeight: AnSize.islandHead`=44、`valueColor: inkMuted`)、只读/child 态自渲;② hint = `AnText.meta`+`inkFaint`+`softWrap`(**词边界**;break-anywhere 留待 AnRow §166 同款处理);③ **值列 tabular** 对 AnField 是有意增强(demo Field `.v` 实无 tabular,只 Kv 有)——理由是与 AnEditableValue 编辑态对齐 + 数值列对齐;④ **只读/child 态无 hover 提墨**(只读无可点性、hover 无功能意义,有意偏离 demo `.field` 全态 hover);⑤ **editing 态 specimen 委托 AnEditableValue**(AnField/AnKv 可编辑路径=AnEditableValue 零包装,消解 §266 字面要求)。
- **token**:行 hover 底 surfaceHover、圆角 `AnRadius.button`、水平内距 `AnSpace.s8`(--pad-row)、行内 gap `AnSpace.s8`(--sp-2)、label/hint 列内 gap `AnSpace.s2`(--grid/2)。**无新增 token**。
- **a11y / reduced**:同 AnKv(只读/可编辑分支、门控 `reduced`)。slot 态(child)的语义由 child 自带。

#### G3-d · AnRefPill — 实体提及药丸
- **API**:`AnRefPill({required String kind, String? id, required String label, ValueChanged<({String kind, String id})>? onTap})`。`id` 空 = 纯标注(不可点、不可聚焦、键盘穿透);非空 = 可点。
- **契约修正(吸收 contract-fidelity HIGH,核心漏洞)**:kind→图标**两级解析,直接复用 `AnIcons.byKey`**(已含 backend 11 类全集:function..skill + doc + conversation + chrome,且内置 `?? fallback`),**不另抄 9 行表**。`AnIcons.byKey(kind.toLowerCase())` 一步到位:doc/conversation/search 纯提及命中、未知键退可见「?」。**事实源 = backend `relation.entitykind.go` 11 类,demo 仅视觉参考**。图标只依赖显式 `kind` 入参,**不从 id 反推前缀**(吸收 contract MED:demo skill/冒号变体 ≠ backend sk/mcp:,若后续需 id→kind 以 backend `prefixKind` 为唯一源 + contract.md 登记)。
- **复用/布局**:与 AnTags chip 同款皮肤——`AnInteractive`(id 非空才 onTap、才 button)包 pill:白底 `context.colors.surface`、海岸线描边 `context.colors.line`、`AnRadius.pill`、`AnText.meta` w500、`context.colors.inkMuted` 文字;hover(有 id)→ surfaceHover 底 + ink 文字。图标 `AnIcons` size `AnSize.iconSm(12)`、色 inkFaint,**`ExcludeSemantics`**(装饰,照搬 AnBadge 状态点先例)。`max-width: min(AnSize.block, 100%)` + ellipsis。
- **token**:内 gap `AnSpace.s4`(--grid)、padding `AnSpace.s2`(=--grid/2 竖)×`AnSpace.s6`(--gap-tight 横)。**无新增 token**。
- **a11y**:id 非空 = `Semantics(button, label: '{类型}: {名称}', onTap)`(类型前缀走 i18n,不硬编码);id 空 = 仅 label 文本、不可聚焦(键盘穿透,避免污染 Tab 序);图标 `ExcludeSemantics`;类型/状态不靠颜色(label 含文本前缀,WCAG 1.4.1)。
- **reduced**:仅 hover 底/文字 `AnMotion.fast` 过渡(功能性)→ `AnMotionPref.reduced` 门控、`Duration.zero` 兜底。

#### G3-e · AnThinTable — 对齐多列(非表格)
- **API**:`AnThinTable({required List<AnTableColumn> columns, required List<Map<String,String>> rows, bool selectable = false, ValueChanged<Map<String,String>>? onRowTap})`。`class AnTableColumn { final String key; final String? label; final AnTableAlign align; }` `enum AnTableAlign { left, right, center }`。
- **Flutter 机制选型(吸收 contract-fidelity HIGH:废稿红线,Flutter 无 CSS subgrid)**:demo 靠 `subgrid` + `minmax(0,1fr)`(首列吃富余)/`minmax(0,auto)`(非首列可缩截断)。**Flutter 无 subgrid 等价**,必须显式另选并实测:
  - **首选 `Table` widget + `columnWidths`**:首列 `FlexColumnWidth(1)`、非首列 `IntrinsicColumnWidth` —— **但 IntrinsicColumnWidth 无 max,超长值会撑破**(正是 demo `minmax(0,auto)` 规避的真坑)。须每格 `ConstrainedBox(maxWidth) + Text(ellipsis, maxLines:1)` clamp,或
  - **退而自研 `LayoutBuilder` 两遍测量**:第一遍量各列内容 max、第二遍按 1fr/auto 等价分配 + 非首列 clamp 上限。
  - **无论哪条,「超长值不撑破列」必须给 clamp 机制 + 压力床超长格断言 no-overflow**——不能只写「复刻 subgrid」。
- **视觉灵魂逐条复刻(吸收 contract MED:靠字色分层的承重点)**:
  - 表头行 `min-height: AnSize.controlSm(24)` + **底对齐** + th = `context.colors.inkFaint` + `AnText.meta` + w600(双轴!`fontVariations:[FontVariation('wght',600)]`)+ 底 padding `AnSpace.s4`;
  - 数据格首列 `context.colors.ink`、**非首列 `context.colors.inkMuted` + `tabularFigures` 常驻**(mono 无关);
  - `selectable` hover → 整行 surfaceHover + 全格 td 转 ink;选中 → surfaceActive(=island-4);
  - 列 align 走 `column.align`(justify-self end/center)。
- **token**:列间 `AnSpace.s16`(--sp-4)、行高 `AnSize.row`、水平内距 `AnSpace.s8`(--pad-row)、行圆角 `AnRadius.button`。**无新增 token**。
- **a11y**:每行 `MergeSemantics` 压成一句(label=列名+值配对,如「名称: 采购单A, 状态: 运行中」)+ 装饰 `ExcludeSemantics` + 表头 `header:true`(同 Section,真机验证);selectable 行整行 `AnInteractive`(button+selected+Enter/Space),**不再外裹 Semantics**(双 button 节点);列名并入 label(避免逐格 swipe 丢上下文)。
- **reduced**:仅 selectable hover/选中 `AnMotion.fast` 功能性过渡 → `reduced` 门控。

#### G3-f · AnRow — 核心行
- **API**:`AnRow({IconData? icon, AnStatus? dot, required String label, String? hint, String? meta, bool selected = false, bool emphatic = false, bool collapsible = false, bool open = false, bool passive = false, int depth = 0, bool mono = false, List<Widget> actions = const [], VoidCallback? onSelect, VoidCallback? onToggle})`。
- **布局(模板铁律)**:三列 `[lead --lead(16) | label 1fr | trail 锚位]`。lead 槽 dot/icon↔chevron 叠放居中(`Stack`/`Grid` 同心);trail 槽 = **`AnTwoZone` 思路右锚**:meta 文本与 actions 叠放同一隐形锚位、右缘锚定 → hover 互换不重排。
- **collapsible chevron hover 互换 + 旋转(吸收联网定论)**:`AnimatedSwitcher`(icon↔chevron 不同 ValueKey 触发交叉淡入)+ `AnimatedRotation(turns: open?0.25:0)`(右箭头转 90° 指下)。hover 维度来自 `AnInteractive` 的 WidgetState set。
- **多行 hint**:整行顶对齐(`align-items: start`),lead 与 label 首行对齐;label 恒单行 ellipsis(导航契约),hint 多行 softWrap anywhere、`context.colors.inkFaint`+`AnText.meta`。hint 行高 `AnSize.islandHead`(=demo --field-row min-height)。
- **emphatic 选中**:`context.colors.accentSoft` 底 + 左 inset accent 条(`AnSize.gripLine` 宽,**需 `accentLine` 色,见新增 token**)。
- **token**:lead 槽宽 `AnSize.icon(16)`(=demo --lead,**复用 icon,不新增**);trail 槽宽=demo `--trail(20)`=`AnSize.controlSm − AnSpace.s4`(语义派生,实现注释写明);indent/level=demo `--indent(20)`;水平内距 `AnSpace.s8`(--pad-row);列 gap `AnSpace.s8`(--gap);hover 底 surfaceHover、选中 surfaceActive。**新增见 §3**。
- **a11y**:行包 `AnInteractive`(passive 时 onTap=null → 不 button、不可聚焦、键盘穿透);**collapsible 行须透传 `expanded: open`**(吸收 RowDetail a11y:AnInteractive Semantics 容器需新增 expanded 透传位,屏读读 expanded/collapsed);chevron/dot 装饰 `ExcludeSemantics`(状态由 expanded/label 承载)。
- **reduced**:collapsible 的 `AnimatedSwitcher`+`AnimatedRotation` 是功能性一次性 → `AnMotionPref.reduced` 门控,真时 `Duration.zero` 即时换图/即时到角度。hover 底 `reduced` 门控。无 loop → matrix 安全。

#### G3-g · AnRowDetail — 可展开详情行
- **API**:`AnRowDetail({required Widget row, required Widget detail, bool open = false, ValueChanged<bool>? onOpenChanged})`(或内部自管 `_open` 瞬时态 + 受控 `open`)。
- **高度揭示(吸收联网定论:全用标准隐式 widget,零手搓 controller)**:`AnimatedSize(duration: AnMotion.mid, curve: AnMotion.easeOut, alignment: Alignment.topCenter, clipBehavior: Clip.hardEdge, child: open ? detail : const SizedBox.shrink())`。
  - **`alignment: topCenter` 必须显式**(默认 center 会双向涨开穿模上下行,最易漏);
  - **child 不能传 null**(收起用 `const SizedBox.shrink()` 让它补间到 0 高);
  - **现代 API 内置 ticker,不传 vsync**(老 issue #89250 已过时,别手搓)。
- **缩进对齐**:detail 左缩 = `lead + gap + pad-row`(对齐 row label 起点)= `AnSize.icon + AnSpace.s8 + AnSpace.s8`;底分隔线 `context.colors.line` `AnSize.hairline`;竖内距 `AnSpace.s4`(--grid)上、`AnSpace.s12`(--sp-3)下。
- **状态归属**:`_open` 纯 UI 瞬时态(setState,不进 Riverpod);detail 异步内容用 AnState/AnSkeleton 占位保持稳定高度(避免展开中二次补间抖动)。
- **clipBehavior 与圆角**:`hardEdge` 是矩形裁剪不跟 `AnRadius`——圆角裁剪交给外层 `ClipRRect`/`AnIsland`,AnimatedSize 只管高度。
- **a11y**:行主体 `Semantics(button, expanded: open, onTap)` 随 open 翻转(屏读读 expanded/collapsed,`SemanticsProperties.expanded`);**AnInteractive Semantics 容器须新增 expanded 透传位**(当前只透 button/enabled/selected);chevron 装饰 ExcludeSemantics(态由 expanded 承载,不重复朗读)。
- **reduced(唯一必落铁律点)**:`AnMotionPref.reduced(context)` 为真 → `AnimatedSize` duration 置 `Duration.zero`(即时显隐、状态一致);门控单源 `reduced`(功能性揭示)。隐式 widget 无自持 controller → 铁律的 initState/dispose/mounted 守卫天然不适用。无 loop → matrix reduced 轴 pumpAndSettle 不超时。

#### G3-h · AnCard — 通用卡片(有边)
- **API**:`AnCard({required Widget child, AnCardVariant variant = normal, bool row = false, bool selectable = false, bool selected = false, AnCardPad pad = normal, VoidCallback? onSelect})`。`enum AnCardVariant { normal, accent }` `enum AnCardPad { normal, tight }`。
- **皮肤**:inset hairline 描边(`context.colors.line`,避圆角灰尖)+ `AnRadius.chip` + `context.colors.surface` 底 + padding `AnSpace.s12`×`AnSpace.s16`(tight=`AnSpace.s4`×`AnSpace.s8`);accent → `accentLine` 描边(新增 token);`row`=横向 flex;selectable hover→`lineStrong` 描边、选中→`accentLine` `gripLine` 宽。内 gap `AnSpace.s8`(--sp-2)。
- **复用**:selectable 走 `AnInteractive`(onSelect、button、selected);否则纯 `DecoratedBox`+`Padding`。**注意:与 `AnIsland`(shadowFloat)区别——Card 是 inset 描边无阴影**,不复用 AnIsland。
- **a11y**:`Semantics(container, explicitChildNodes)`(**不 merge,卡内控件各自可达**,吸收 a11y:漏 explicitChildNodes 会吞卡内 button);整体可点导航卡才升级 `AnInteractive(button)`。
- **reduced**:selectable hover/选中 `AnMotion.fast` 功能性 → `reduced` 门控。

#### G3-i · AnInfoCard — 无边信息单元
- **API**:`AnInfoCard({String? title, IconData? icon, String? meta, required Widget child, List<Widget> actions = const []})`。head(icon+title+meta)仅在有 title/icon/meta 时渲。
- **布局**:head 走 `AnTwoZone`(title flex:1 左截断 + meta shrink 100 先让位)——title `context.colors.inkFaint`+`AnText.meta`+w600(双轴!)、icon `AnSize.iconSm` inkFaint、meta inkFaint+gap-tight;body Column gap `AnSpace.s8`(--sp-2);actions 走 `AnActionGroup`、上距 `AnSpace.s12`(--sp-3),无动作时塌掉。
- **token**:卡内距 `AnSpace.s4`(--sp-1)×`AnSpace.s8`(--sp-2)、`AnRadius.button`、head min-height `AnSize.control(28)`。**无新增 token**。
- **a11y**:`Semantics(container, explicitChildNodes)`(不 merge);title `header:true`(真机验证);icon ExcludeSemantics。
- **reduced**:N/A(纯静态;body 内若嵌 AnStatusDot/AnSkeleton 各自带 reducedOrAssistive)。

---

### 3. 跨切面共享原语小结(先抽)

| 原语 | 类型 | 归属 | 被谁用 |
|---|---|---|---|
| `AnEditableValue` | 净新增 StatefulWidget(双锚编辑核,行高参数化) | `an_field.dart`(private) | AnKv · AnField |
| `AnTwoZone` 升格 | 既有 private → 顶层共享(泛化 trailing 槽) | `an_two_zone.dart`(导出) | AnDropdown(回归) · AnSection head · AnRow trail · AnInfoCard head（**AnKv 不用**:key 贴左+value 撑右,非 label 贪婪 Expanded 几何，[doc-fix] G3.7） |
| `AnAutoGrid` | 净新增布局件(优先验证原生 delegate,否则薄封装) | `an_auto_grid.dart`(导出) | AnSection grid |
| `AnInput.onTapOutside` | 地基增强(若采纳 blur-commit) + mono tabular 统一 | `an_input.dart` | AnEditableValue · AnInlineEdit |
| `AnInteractive.expanded` | 地基增强(Semantics 容器新增 expanded 透传位) | `an_interactive.dart` | AnRow(collapsible) · AnRowDetail |
| `AnGroupLabel.padding` | 地基增强(padding 参数化,共享 caption 样式) | `an_group_label.dart` | AnSection caption |
| `design-system.md §2` | 文档:功能性一次性微动效豁免 reduced 门控政策 | 文档 | 全 kit |

---

### 4. 新增 token 汇总(去重,命名归属一致)

**结论:G3 几乎零新增 token**——四镜复审一致裁决 `AnSpace.padRow`/`AnSize.gridMinCol` 是造同义词,直接复用既有值。唯一真缺口是 AnRow emphatic + AnCard accent 描边色。

- `accentLine = rgba(0,113,227,0.30) 亮 / rgba(10,132,255,0.40) 暗 (AnColors)` —— demo `--accent-line`,AnRow emphatic 左 inset 条 + AnCard accent/selected 描边;既有 `accentSoft`(0.10/0.16)太浅做不了描边线,是真缺口(需在 AnColors ThemeExtension 加字段 + light/dark + lerp + copyWith)。

**明确不新增**(吸收 token/consistency 多条):
- `AnSpace.padRow` → 用 `AnSpace.s8`(--pad-row 已查证=8);
- `AnSize.gridMinCol` → 用 `AnSize.block`(=280,注释已含网格语义);
- 行高/竖内距/lead/trail/indent → 复用 `AnSize.row/islandHead/icon/controlSm` + `AnSpace.s4/s8`(语义留调用处注释);
- 无新增圆角/时长/不透明度 token。

---

### 5. i18n key 汇总(en/zh 草案,参数风格对齐既有 $name)

- `action.edit` / `action.cancel` / `action.save` —— **已存在**,复用。
- `feedback.emptyValue` —— **新增**(value 空占位)。**裁决(吸收 consistency/token MED):`—` 是 locale 无关纯标点 glyph,与既有 `AnDropdown.placeholder='—'` 裸字面口径统一 → 实为常量 em-dash,不入 i18n**。若团队反向要 i18n 则同提交把 AnDropdown 也改走 key;**建议:不新增此 key,用常量 `—`**。
- `a11y.editingField` —— **新增**(进编辑 polite 宣告,带 `$field` 参数)。en:「Editing $field」/ zh:「正在编辑 $field」。需新建 `a11y` 命名空间。
- RefPill 类型前缀 `a11y.refKind.{function,handler,agent,...}` 或复用既有 `status.*` 风格 —— **新增**(屏读「{类型}: {名称}」前缀,11 类)。en/zh 按 backend kind 命名。

> gallery specimen / 压力床 label(如「超长截断」「注入」)属 **dev-only,按 `catalog.dart` 既有约定硬编码、豁免 i18n**(同 test 代码),与产品文案划清界线。

---

### 6. a11y 总则(G3 全件)

1. 桌面承重 = 久稳 `SemanticsFlag`(button/selected/enabled/header/expanded)+ label/value/onTap;`SemanticsRole.table/row/cell/link` 仅 web 前向兼容标注(桌面屏读器不消费)。
2. **含独立可达叶子的子树绝不 MergeSemantics / 套显式 role-Semantics**,用 `explicitChildNodes` 容器。matrix 加断言:每可编辑 specimen 计 button/textField 节点数(防吞噬回归)。
3. 状态/类型不靠颜色(WCAG 1.4.1):label 含文本前缀/状态词。
4. 装饰 glyph 一律 `ExcludeSemantics`;异步/状态变化 `Semantics(liveRegion, polite)`,warn/danger `SemanticsService.sendAnnouncement(assertive)`。
5. 键盘:可选行/RefPill/Kv 编辑触发走 `AnInteractive` 的 Enter/Space;disabled(onTap=null)键盘穿透。
6. `header:true` 是 kit 新模式 → 落地前真 macOS VoiceOver 验证(verify-by-real-run),不假设 demo web role:header 映射干净。
7. 就地编辑焦点回落到铅笔(WAI-ARIA grid);每编辑格恰一个 textField 节点。

---

### 7. reduced-motion 政策(吸收 a11y 跨切面,先写进 design-system.md §2)

- **功能性一次性微动效(focus-border / hover-tint / press / caret-rotation / 编辑框 swap)豁免 reduced 门控**——它们 settle、过 matrix reduced 轴,不是 loop。
- **必须门控**:loop(shimmer/breath/typewriter)走 `reducedOrAssistive`;功能性 reveal(AnRowDetail 高度揭示、AnRow chevron 互换/旋转)走 `reduced`。
- **每件读哪个门控显式声明**:件自身功能性过渡读 `reduced`,**绝不**用 `reducedOrAssistive` 门控功能性反馈(会剥夺屏读用户的 hover/focus 线索);嵌套 loop 件保留自己的 `reducedOrAssistive`。

---

### 8. 压力床计划(每件五电池)

- **空**:rows/children/options/columns 全空 → 不溢出不抛(render-exists 断言)。
- **超长**:label/value/key 200+ 无空格串 → ellipsis/wrap/横滚,窄壳(maxWidth 120~280)断言 no-overflow。ThinTable 超长格断言不撑破列。
- **海量**:rows 200+ / grid 60+ 卡 → 一次性 layout(普通流非滚动区;海量滚动由调用方外包 ListView/sliver),断言不溢出/可接受。
- **极值**:maxWidth=120(挤压钉锚)/2000(两端撑开);grid maxWidth=200 塌 1 列、600 多列;单子块 grid 不拉满全行;mono 纯数字验 tabular 对齐;select options=[] 占位。
- **注入**:`<b>x</b> & "y" {json}` → Flutter Text 默认转义安全;i18n 中↔英 + 明暗主题 + reduced 三轴 matrix 注入。
- **可编辑件必登 editing specimen**(吸收 a11y LOW):AnKv/AnField 须注册「editing 态」specimen(startEditing 等价),让 matrix 真演练 textField+✓✕ 语义树、✓✕ 可达性、idle↔editing swap 的 reduced 轴——不只 idle 展示态。所有 AnInput-bearing specimen 须包 Material/AnIsland(matrix _host 已 MaterialApp>Scaffold)。

---

### 9. 构建子步顺序 G3.1..G3.9(单一作者,逐件提交,gallery-first,依赖在前)

依赖拓扑:共享原语/地基增强先行 → 静态展示件 → 编辑件 → 网格件。每件:catalog specimen + matrix(normal+reduced 双测)+ gallery 截图目视对 demo + `design-system.md` 路线表 ✅ 同提交。

- G3.1 共享原语先行(地基,无 UI 消费):升格 AnTwoZone 到顶层 an_two_zone.dart(泛化 trailing 槽)+ 同提交 AnDropdown 改引共享件回归 byte-equal + design-system.md 同步。依赖:无。产出:AnTwoZone 顶层共享件 + AnDropdown 回归绿。
- G3.2 地基增强批:AnInteractive 加 expanded 透传位;AnGroupLabel padding 参数化;AnColors 加 accentLine 字段(light/dark/lerp/copyWith);design-system.md §2 写 reduced 豁免政策。依赖:无。产出:三处地基增强 + token + 政策文档,既有件回归绿。
- G3.3 AnRefPill(纯静态、零编辑、复用 AnIcons.byKey 11 类 + AnInteractive):catalog specimen(各 kind/无 id/超长/注入)+ matrix + 截图对 demo。依赖:无(AnIcons 已存在)。产出:AnRefPill + 验收。
- G3.4 AnSection(静态布局,caption/plain,actions 走 AnActionGroup,head 走 AnTwoZone;grid 路径暂留 TODO 待 AnAutoGrid):catalog + matrix(含 header Semantics-order 断言)+ 截图。依赖:G3.1(AnTwoZone)、G3.2(AnGroupLabel padding)。产出:AnSection 非 grid 路径 + 验收;header 真机验证。
- G3.5 AnAutoGrid(先证伪原生 delegate;落地后回填 AnSection grid=true + 加 grid specimen 让 reduced 轴透传):catalog(空/单卡/海量/超窄塌1列/超宽多列)+ matrix + 截图。依赖:G3.4(AnSection 接其 grid)。产出:AnAutoGrid + AnSection grid 路径 + 验收。
- G3.6 AnEditableValue(净新增双锚编辑核,行高参数化,onTapOutside/_finished/focus 回落/announce 全实现;同提取共享叶子改 an_inline_edit/an_edit_affordance + 回归):无独立 gallery(被 Kv/Field 消费),但须先于它们落地 + 单测覆盖竞态/焦点/announce。依赖:G3.2(AnInput onTapOutside 若采纳 blur)。产出:AnEditableValue 编辑核 + AnInlineEdit/AnEditAffordance 回归绿 + design-system.md 同步。
- G3.7 AnKv(行高32+padV s4,走 AnEditableValue + AnTwoZone;只读/可编辑 Semantics 分支显式;tabular 常驻;whole-value 回调):catalog(含 editing specimen + mono + wrap + 海量 + 注入)+ matrix(button/textField 节点计数断言)+ 截图。依赖:G3.1、G3.6。产出:AnKv + 验收。
- G3.8 AnField(行高44+padV s4 复用同核;passive/child slot 三岔态;hint 多行):catalog(有值可编辑/有值只读/无值放控件/editing/超长)+ matrix + 截图。依赖:G3.1、G3.6。产出:AnField + 验收;确认 Field/Kv 行高参数化各自保真。
- G3.9 行卡件收尾 AnRow + AnRowDetail + AnCard + AnInfoCard(静态/折叠展开):AnRow(三列网格 + chevron AnimatedSwitcher/AnimatedRotation + expanded 透传)→ AnRowDetail(AnimatedSize topCenter/hardEdge + reduced Duration.zero)→ AnCard(inset 描边 + selectable)→ AnInfoCard(head AnTwoZone + actions AnActionGroup)。各 catalog + matrix(reduced 轴 + expanded Semantics 断言)+ 截图 + AnRowDetail/AnRow 动效真机 flutter run -d macos 验证。依赖:G3.1、G3.2(expanded/accentLine)。产出:四件 + 全 G3 验收 + design-system.md 路线表 G3 ✅。

---

### 三层验收接入

1. **gallery specimen**:每件登记进 `catalog.dart` 新 `_g3RowsCards` 类目(GalleryItem + specimen,含五电池 stress 标);`make gallery` 目视对 `demo/reference.html` G3 段;`flutter test test/dev/capture_gallery.dart --dart-define=CAT=<idx>` 出截图(动效 disableAnimations 静态帧)。**动态动效(AnRowDetail 展开/AnRow chevron)须 `flutter run -d macos` 真跑验证**(verify-by-real-run,G1 白屏教训)。
2. **matrix 5 维 + reduced 轴**:`gallery_matrix_test.dart` 自动覆盖每 specimen build / no-overflow / 转义安全 / render-exists / disabled 键盘穿透 + **reduced 轴 pumpAndSettle**(忘门控 loop = 超时 FAIL);**新增 a11y 断言轴**:关键节点 Semantics(label/button/selected/header/expanded)、disabled 行不可聚焦、RefPill 无 id 无 button flag、每可编辑 specimen button/textField 节点计数。
3. **五电池**:见 §8,`stress: true` + `maxWidth` 触发截断/塌列/海量。
4. **工程门禁**:`make fe-verify`(codegen + analyze 净 + test 绿);无内联 hex/px/ms(grep 守);i18n 无硬编码;S11 注释;**文档 1:1 同步**(`design-system.md` 路线表 + 共享原语重构 + accentLine token + reduced 政策 §2 + header 真机结论)。

---

## 附录 A — 两硬题联网补研定论（T1 / T2，2026-06-23 补，有出处）

> 首轮 workflow 该两题（也是 G3 最难的两个 Flutter 问题）schema 重试超限丢失。已单独无 schema 补研、联网佐证。**本附录是 AnThinTable（§2 G3-e）与 AnRow 尾槽（§2 G3-f）的权威落地源，优先于卡内初稿措辞。**

### T1 — AnThinTable：无 chrome 多列跨行对齐（Flutter 无 CSS subgrid）

**定论：用 Flutter 内置 `Table` widget**（`package:flutter/widgets.dart`，**非** Material `DataTable`）。`RenderTable` 本就是「所有行共享同一组列轨、一次性测全表 cell intrinsic 宽再统一分配 + 给每格下发 tight width 约束」的渲染器——**这正是 CSS subgrid 在 web 上做的事**，跨行列对齐内置即得、不手量。零额外 pub 依赖。

**列宽映射（= demo 的 minmax 语义）**：
- 首列 `minmax(0,1fr)` → `FlexColumnWidth(1)`（吃所有非 flex 列定宽后的剩余空间）。
- 其余 `minmax(0,auto)` → `MinColumnWidth(IntrinsicColumnWidth(), FixedColumnWidth(maxColPx))`（intrinsic 贴内容，但**封顶 maxColPx**）。
- **关键坑**：裸 `IntrinsicColumnWidth` **无上限**（取该列各 cell 的 maxIntrinsicWidth 最大值），配普通 Text **会被超长值撑破**（正是 demo「裸 auto 会撑破」要规避的）。但当列宽总和 > 可用宽时，`RenderTable` 的收缩算法（先压 flex 列到 min intrinsic，再在 intrinsic 列间均匀收缩，**永不溢出**）会 clamp——故必须给可截断列套 `MinColumnWidth(..., FixedColumnWidth(max))` 给理性上限，让 ellipsis 到上限即生效（issue #43334：ellipsis Text 的 maxIntrinsicWidth 仍报完整宽）。

**单元格 ellipsis**：`RenderTable` 给每格下发 **tight width** 约束 → cell 内 `Text(maxLines:1, overflow:ellipsis, softWrap:false)` 正常截断，**无需** `Flexible/Expanded`。

**整行跨列命中（无 Material）**：`TableRow` 不是 widget、不能直接包手势（issue #42609 仍 open）；**不用** `TableRowInkWell`（Material ink，本 kit 已关 Material 波纹、且需 Material 祖先）。绕法：① 整行底画在 **`TableRow.decoration`**（`RenderTable` 的 decoration 跨全列满铺＝整条高亮，等价 subgrid 整行）；② 每格内层叠一个透明命中层（`MouseRegion` 报 hover + `GestureDetector(behavior:opaque)` 报 tap），共同更新**该行同一份 hover/selected 状态 + 回调携 `rows[i]`**。底色 `c.surfaceHover.whenActive(hovered)`（无暗闪）/ selected `c.accentSoft`，圆角 `AnRadius.button`。复用 `AnInteractive` 的**态集惯例与底色 idiom**（行非单 widget，故复用其状态约定而非整行塞进单个 AnInteractive）。

**表头**：第一个 `TableRow`，每格 `AnText.meta` + `context.colors.inkFaint` + w600（**双轴！`fontVariations:[FontVariation('wght',600)]`**）+ 底对齐（`TableCell(verticalAlignment: bottom)`）+ 底 padding `AnSpace.s4`。**不画任何线**（不给 `Table.border`、无行 divider、无斑马）。非首列数字 `FontFeature.tabularFigures()` + `Align`+`textAlign` 走 `column.align`。

**性能 / 大数据退守**：`Table` **一次性 layout 全部行**，且 `IntrinsicColumnWidth` 最贵（测每格）。ThinTable 作「一块内容」（几十行级、随父滚动）→ `Table` 够。**行数上百 / 需自身滚动 / 无限 → 退官方 `two_dimensional_scrollables` 的 `TableView.builder`**（懒构建只渲可视区，但放弃 intrinsic 自适配、列宽自给 `TableSpan`）。规范化阈值实现期实测定。

**否决**：`flutter_layout_grid`（minmax = open issue #25 未实现）；`GridView/SliverGridDelegate`（等宽等高，无列贴内容跨行对齐）；`CustomMultiChildLayout`/自定义 `RenderObject`/两遍 LayoutBuilder 测量（= 重抄 `RenderTable` 已写好的算法，且易踩 intrinsic×ellipsis 边界 bug）。

**a11y**：`Table` 自带表格语义（行列结构）；表头 cell 套 `Semantics(header:true)`（Flutter 不自动标表头）；每行命中层 `Semantics(button, selected, label:行摘要, onTap)`，键盘 Enter/Space 经 `AnInteractive` 的 `ActivateIntent` 激活；列名并入行 label 避免逐格 swipe 丢上下文。

**实现期必实测**：① 逐 cell 透明命中层在 column gap 区是否无缝（真跑 hover 截图）；② `maxColPx` 取值贴 demo 观感（中英混排/超长 ID 实测）；③ 窄容器（如 320 左岛）下 flex 列压 0 后 intrinsic 才缩的中间态是否难看（可能给首列也配 MinColumnWidth 兜底最小宽）；④ 行底圆角 + column gap 视觉；⑤ Impeller/macOS 上多少行开始掉帧（定退 TableView 阈值）。

**出处**：[Table](https://api.flutter.dev/flutter/widgets/Table-class.html) · [columnWidths](https://api.flutter.dev/flutter/widgets/Table/columnWidths.html) · [IntrinsicColumnWidth（无上限）](https://api.flutter.dev/flutter/rendering/IntrinsicColumnWidth-class.html) · [MinColumnWidth](https://api.flutter.dev/flutter/rendering/MinColumnWidth-class.html) · [RenderTable 机制（一次性全量/收缩序/tight cell/row decoration 满宽）](https://flutter.megathink.com/user-interface/tables) · intrinsic×ellipsis [#43334](https://github.com/flutter/flutter/issues/43334) · Min/Max flex bug 已修 [#131467](https://github.com/flutter/flutter/issues/131467) · TableRow 无 tap [#42609](https://github.com/flutter/flutter/issues/42609) · flutter_layout_grid minmax [#25](https://github.com/shyndman/flutter_layout_grid) · [two_dimensional_scrollables / TableView](https://pub.dev/packages/two_dimensional_scrollables)

### T2 — AnRow 尾槽：meta↔actions 同位 opacity 互换不重排 + 整行 hover 揭示

**定论：一个 `AnInteractive` 包整行**（它已基于 `FocusableActionDetector`，把 `Set<WidgetState>` 经 `builder(context, states)` 下发——正是「祖先 hover 态下发后代」的标准载体）；**尾槽（与 lead 槽）用 `Stack(alignment: Alignment.centerRight)` 叠两层 + 各裹 `AnimatedOpacity` 交叉淡入淡出**。

**逐点**：
1. **同位右锚**：两层都 **non-positioned**（不包 `Positioned`），`Stack.alignment = centerRight`。Stack 尺寸 = max(meta 宽, actions 宽) → 右缘自动对齐、**槽宽恒等于二者最大值、hover 进出不变**。（**忌** `Positioned(right:0)`——定位子不撑 Stack，会塌 0 宽。）
2. **交叉淡**：两层 `AnimatedOpacity`（meta `opacity: revealed?0:1`、actions `revealed?1:0`）。**忌 `AnimatedCrossFade`**（其 `defaultLayoutBuilder` 把子左上锚 + 整体裹 `AnimatedSize` → meta/actions 宽不同会让槽宽**做动画推挤** label = 重排）；**忌 `AnimatedSwitcher`**（尺寸取当前 child、过渡期双显可命中）。opacity 不影响布局 = 零重排。
3. **祖先 hover 下发后代（核心）**：**只在行根一个 `AnInteractive`**，`builder` 拿 `states` 直接当入参传给 lead 与 trail 组件——后代**不各自探 hover**（嵌套 MouseRegion = 多 hover 源、整行语义丢）。揭示判定：trail 建议用 `states.isActive`（含 focus，纯键盘可见 actions）；lead 的 icon↔chevron 用纯 `hovered` 即可。**不引** `WidgetStatesController`（lead/trail 是行直接孩子，穿参最简；controller 仅「后代离祖先很远」才值得）。
4. **零重排三保**：行高固定（`Tokens.rowHeight`/hint 行另算）；label 用 `Expanded`、trail Stack 宽恒定 → label 可用宽不随 hover 变；lead 两层等尺寸叠放。
5. **`_TwoZone` 升格为公共 `AnTwoZone`**（见 §1 原语 B）：泛化出 lead/label(Expanded)/trail 三槽 + hover 揭示；`an_dropdown.dart` 同提交改指、回归 byte-equal。

**命中 + a11y（关键）**：`AnimatedOpacity` 的 **opacity 0 不自动挡命中、也不挡屏读**（[Opacity docs](https://api.flutter.dev/flutter/widgets/Opacity-class.html)、[#12283](https://github.com/flutter/flutter/issues/12283)）→ 被淡出层必须裹 **`IgnorePointer` + `ExcludeSemantics`**（Flutter 3.8 后 `IgnorePointer` 仅挡语义动作、保留节点，故**必须显式 `ExcludeSemantics`**，见 [迁移](https://docs.flutter.dev/release/breaking-changes/ignoringsemantics-migration)），否则出现「看不见却点得到 / 被重复朗读」的 ghost。揭示后撤掉 IgnorePointer，焦点可正常进 actions。

**reduced**：功能性 hover 微动效**豁免门控**，但 reduced 时应即时——`duration: AnMotionPref.reduced(context) ? Duration.zero : AnMotion.fast`。

**遗留/拍板点**：① 揭示判定默认 `isActive`（trail）/`hovered`（lead）；② 行级 `expanded` 语义由谁给——`AnInteractive` 目前只透 button/enabled/selected，**collapsible/RowDetail 需给 `AnInteractive` 扩 `expanded` 透传位**（见 §1 原语「AnInteractive.expanded」）；③ trail 整体给一个 max 宽约束让两层共用（避免 actions 较宽时撑破 meta 的 ≤45% 约束）；④ 真机 Impeller 跑 hover 进出无抖、无 ghost 命中（verify-by-real-run）。

**出处**：[Stack（尺寸=最大非定位子）](https://api.flutter.dev/flutter/widgets/Stack-class.html) · [AnimatedCrossFade（裹 AnimatedSize）](https://api.flutter.dev/flutter/widgets/AnimatedCrossFade-class.html) · [AnimatedOpacity](https://api.flutter.dev/flutter/widgets/AnimatedOpacity-class.html) · [Opacity（0 不停命中）](https://api.flutter.dev/flutter/widgets/Opacity-class.html) · Opacity 命中/a11y [#12283](https://github.com/flutter/flutter/issues/12283) · [ignoringSemantics 迁移](https://docs.flutter.dev/release/breaking-changes/ignoringsemantics-migration) · [Hover 权威指南（单点检测+下发）](https://wilsonwilson.dev/articles/flutter-hover-effect-triggers-the-definitive-guide)
