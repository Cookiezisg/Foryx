---
id: WRK-037
type: working
status: active
owner: @weilin
created: 2026-06-23
reviewed: 2026-06-23
review-due: 2026-09-21
audience: [human, ai]
landed-into:
---

# G2 反馈态套件 —— 联网调研已确认的建造规范（开工前对齐用）

> **来源**:开工前的完整联网 best-practice 扇出(6 组件 agent + 2 跨切面 agent + 综合,见会话 workflow `research-g2-feedback-states`)。
> **用法**:这是 G2(Skeleton/State/Callout/Stepper/Tags/Typewriter)的**建造事实源**——逐件按本篇做。**§1 通用标准是 kit-wide 绑定规则**(落地时提取进 [`design-system.md`](../../references/frontend/design-system.md) §2);§3 逐组件方案随各件提交落地。
> **决策前置**:§6 开放项需先和人对齐,再开 §2 pre-work。
> 视觉/分层/令牌见 [`design-system.md`](../../references/frontend/design-system.md);架构见 [`ADR 0004`](../../decisions/0004-frontend-flutter-architecture.md)。

---

## 1. 通用标准（kit-wide 绑定 —— 所有动画 An* 件都遵循）

1. **动效偏好单源 `AnMotionPref`**:在 `core/design` 加 `AnMotionPref`(挨着 `AnMotion`)。两个取值器:`reduced(c) => MediaQuery.disableAnimationsOf(c)`(aspect 作用域、只在标志翻转时 rebuild)、`reducedOrAssistive(c) => disableAnimationsOf(c) || accessibleNavigationOf(c)`。每个动画件 `build()` 里调它——**禁用** `MediaQuery.of(context).disableAnimations`(过度 rebuild)、**禁用**逐平台检测(#8)。**功能性一次性揭示**门控 `reduced`;**装饰性循环**(shimmer / 光标闪 / typewriter / breath 脉冲)门控 `reducedOrAssistive`(屏幕阅读器活跃时持续动效是噪声)。
2. **循环不会被自动降级**:`AnimationController.repeat()` 默认 `AnimationBehavior.preserve` → `disableAnimations` 下**不会**自动缩短。每个循环效果**必须**显式查 helper:reduced 时**不启动**(或 `.stop()`+`value=0`)控制器并渲染**具体静态兜底**。只有默认 `.normal` 的一次性 `forward()` 才自动缩短——循环绝不能依赖它。
3. **每个纯动效都附"成品感"静态兜底**(WCAG 2.2.2 + 2.3.3):不是"无动画"而是**收尾完成的静止帧**。Skeleton→静态填充(mid 不透明度 token,非透明);breath→实心点(run tone,不振荡);Typewriter→主短语**完整最终串**(无光标或稳定光标);Stepper→瞬时切步。静止时必须显得是有意为之、绝不像坏掉/空/删一半。
4. **急切初始化(最高优先级生命周期规则,修一个真崩溃)**:**绝不**在 `late final` **字段初始化器**里建 AnimationController / Ticker / Timer / FocusNode / ScrollController——那是惰性初始化、首次**读**时(可能在 teardown 中、vsync `this` 已失活)触发 → 崩。声明 `late final AnimationController _c;` + 在 `initState()` **赋值**。⚠️ **`an_status_dot.dart:23` 正是此 bug**(`late final _c = AnimationController(...)`),是 G2 commit 的参考迁移;`an_popover.dart` 是正例(声明 + initState 赋值),所有 G2 stateful 件照此。
5. **永远 dispose + mounted 守卫**:State 建的每个 controller/ticker/timer 必须在 `dispose()` 拆(`.dispose()`/`.cancel()`),`super.dispose()` 最后。任何从 async/Timer 回调进的 `setState` 前置 `if (!mounted) return;`(`an_interactive.dart` 的 `_set` 范式)。自重排的 Timer 链:把当前未决 Timer 存进可空字段、每步覆写,dispose 时取消未决的。
6. **离屏即静默**:所有循环视觉动效走 **vsync 的 AnimationController**(绝不用裸 `Timer.periodic`+setState 做视觉循环)。State-vsync 的控制器会被 Flutter 给离屏路由/非活动 tab/`Offstage` 插的 `TickerMode(enabled:false)` 自动静音;Timer **不感知 TickerMode**、离屏仍烧帧。Timer **仅**用于离散相位切换(Typewriter type→pause→delete→next)。
7. **默认单 ticker**:常见单控制器用 `SingleTickerProviderStateMixin`(对齐 AnStatusDot)。仅当 State 真同时拥 2+ 控制器才用 `TickerProviderStateMixin`——**Typewriter 必须把光标闪 + 打字/删除折进一个控制器**保持 Single。绝不"以防万一"用多 ticker。
8. **作用域 rebuild**:动画绘制走 `AnimatedBuilder(animation:_c, builder:, child:)`——每帧只重跑闭包。真·逐帧重绘的**叶子**(shimmer 渐变 / breath 环 / 闪烁光标)裹 `RepaintBoundary` 隔离图层。**静态件**(State/Callout/Stepper/Tags)**别**加 RepaintBoundary(只隔绝绘制、白付一个 GPU 层)。非循环过渡用隐式 `AnimatedContainer`/`AnimatedDefaultTextStyle`,不自拥控制器。
9. **异步状态→liveRegion;装饰动效→ExcludeSemantics**:状态节点(State loading→empty/error、Skeleton→内容替换、事件触发的 Callout、Tags 增删)裹 `Semantics(liveRegion:true, label: context.t.<key>)`(polite、不抢焦,仿 SnackBar)。纯装饰动画层(shimmer/光标/breath 环)**无意义**、必须 `ExcludeSemantics`。意义由 live-region/已解析内容承载,绝不由动效承载。绝不给 shimmer 贴 'loading animation' 标签。
10. **状态不靠颜色单独表达**(WCAG 1.4.1 A):每个 severity/state 区分都带 **图标 + 文字 +(a11y label 里的)状态词**——绝不只靠色。适用 Callout severity、Stepper done/current/upcoming(还要差形状/大小/字形,如 done 用 Lucide check)、Tags 每标签 tone。matrix 的 escaping/render-present 是天然执行面。
11. **两套不同的 a11y 机制(别混)**:`Semantics(liveRegion:true)` **永远 polite**、无 assertive 参数(= role=status,适合 info/ok)。warn/danger(role=alert/assertive)**必须额外**在 initState `SemanticsService.announce(msg, dir, assertiveness: Assertiveness.assertive)` 一次,并在 didUpdateWidget 文案/severity 原地变时**重发**。混为一谈是 G2 最大 a11y bug 风险(尤其 Callout)。
12. **令牌 + i18n 在动画下也绑定**:所有时长/曲线**只**读 `AnMotion`(fast/mid/slow/breath + easeOut/spring)——绝不内联 `Duration(milliseconds:…)`/裸 `Cubic`;新节奏先成 `AnMotion` token。色/尺/透明走 AnColors/AnSize/AnSpace/AnOpacity;新值(如 skeleton 静态填充透明度)先成命名 token。reduced 路径靠**不启动循环 + 渲染 token 驱动的静态兜底**实现,**不是**在调用处撒 `Duration.zero`。所有文案走 slang `context.t.*`。
13. **G2 零新增包**:每件都在 G0/G1 原语(AnInteractive/AnBadge/AnButton/AnInput/AnStatusDot/AnIcons/AnTone/whenActive)+ AnimationController/AnimatedBuilder/ShaderMask + tokens 上干净组合。候选包(shimmer/skeletonizer/animated_text_kit/step_progress/textfield_tags/MaterialBanner/empty_widget)各因 #8 健康门、自带非 token 主题系统、绕过 slang、或缺 reduced-motion + Semantics 契约而被否。**每件 S11 doc-comment 记下逐件否决理由**。
14. **matrix 门禁加 reduced-motion 轴**:每个动画件在 motion-on **和** reduced 两态都断言——(a) 受限栅格内 build 不溢出;(b) reduced 下**无控制器残留 tick**(`pumpAndSettle` 能终止;循环件卡死 pumpAndSettle = 门禁 FAIL);(c) escaping + render-present 过。加上既有 build/no-overflow/escaping/render-present 轴 + 真机 macOS 截图。

---

## 2. Pre-work（阻塞项,先于任何 G2 组件,同一 commit 类）

1. `core/design` 加 **`AnMotionPref`**(`reduced` / `reducedOrAssistive`)。
2. **修 `an_status_dot.dart:23`**:惰性 `late final _c = AnimationController(...)` → 声明 + initState 赋值(急切初始化参考迁移),并补上缺的 `reducedOrAssistive` 门控(run 呼吸在 reduced 下渲染静态实心点)。
3. matrix 门禁夹具加 **reduced-motion 轴**(`MediaQuery(...copyWith(disableAnimations:true))` 或 platformDispatcher accessibility override)。

> 这三项是所有动画 G2 件的 kit-wide 前置。

---

## 3. 逐组件方案（已确认）

> 全部 **HAND-ROLL、零新增包**(理由见 §1.13 + 各件)。下列为建造契约。

### 3.1 AnCallout（最简,先做)——通栏语气提示条
- **Stateless**;`Container(color: tone.softBg, radius: chip/card)` → `Row(crossAxisAlignment.start)`:`Icon(tone 字形)` | `Expanded(Text.rich body w300 + strong 内联)` | 0–2 个 `AnButton(sm)` | 可选 `AnButton.iconOnly(close)`。
- **severity 映射(承重)**:`AnTone` 无 `info` 成员 → **info→AnTone.accent**(#0071e3),ok→ok,warn→warn,danger→danger。**无新 tone、无新色 token**(softBg/fg 全覆盖)。
- **变体**:severity(info/ok/warn/danger,各配 Lucide 字形)· dismissible(danger/阻塞错误默认**不可关**)· actions 0/1/2(上限 2,放不下则 actions 换行、绝不截断正文)· content 纯串或富 InlineSpan· 单行 vs 多行(`CrossAxisAlignment` + intrinsic 高,**绝不**capto `AnSize.row`)。
- **a11y**:§1.11 两机制——`Semantics(liveRegion:true)`(polite,info/ok)+ warn/danger 额外 `SemanticsService.announce(assertive)`(initState + didUpdateWidget 重发);label 含 severity 词;icon ExcludeSemantics;close 给真 label/tooltip + 焦点。
- **reduced**:默认静态即原生呈现;若开启进出动效则 `AnMotionPref.reduced` 门控瞬时显隐。
- **新 token**:**无**(reuse accent / softBg/fg / chip·card / s8·s12 / fast·easeOut / icon)。

### 3.2 AnState（第二)——空/载/错 整块占位
- **Stateless 默认**;`Center + ConstrainedBox(maxWidth)` → `Column(min)`:大 `Icon(inkFaint, excludeFromSemantics)` → `Text(title, strong/h3, center)` → 可选 `Text(hint, meta, inkMuted)` → 可选 `AnButton`。`enum AnStateKind{empty,loading,error}`。
- **关键配对**:**内容加载不要转圈** → 委托 `AnSkeleton`;`AnState.loading` 仅留给短不定等待(保存/鉴权),用 `CupertinoActivityIndicator.adaptive`(自带 ticker,保持 Stateless)。
- **变体**:empty(可带主 CTA,无 liveRegion)· error(**单色 inkFaint 图标**(决策①)+ 'Try again' ghost-outline + refresh,severity 词只进 a11y label、不着红,liveRegion)· loading(短等待,liveRegion)· size:inset(嵌卡/表)vs page(整块)· actionless(可省按钮及其上间距)。
- **决策①**:error 用单色(红留给 AnCallout)→ AnState **不引** AnToneColors。
- **a11y**:**一个** `Semantics(container, label:'<title>. <hint>')`;装饰图标 excludeFromSemantics;loading/error 用 liveRegion(empty 不用,避免误报);焦点序 title→hint→action(error 出现时可 focus 'Try again')。
- **reduced**:loading 指示器门控 `reduced` → 静态字形/'Loading…';empty/error 本就静态。
- **新 token**:`AnSize.stateIcon`(≈40)· `AnSize.stateMaxWidth`(**先试 reuse** menuMaxWidth 360 / block 280,不合再加)。

### 3.3 AnStepper（第三)——步骤进度点
- **Stateless**(`onStepTap` 走 AnInteractive、无需自拥控制器);`Row` of 段,每段 `AnimatedContainer`(`BoxShape.circle` 点 / `radius pill` 药丸)按状态填 token 色;current 强调用 `AnimatedContainer(mid+easeOut)` + whenActive no-flash。**无 breath**(stepper 离散推进,持续动效违背动效克制)。**1-based** current。
- **变体**:dots(默认)/ pills(短标签)/ numbered· 连接线(token `line`)可选· done/current/upcoming **三种不同治理**(upcoming ≠ disabled 样式)· 可选每步短标签(默认关)· **决策③:G2 就带 `onStepTap`**——已完成节点各自组合 AnInteractive(焦点环 + Enter/Space + `Semantics(button)`),current/upcoming **不可聚焦**;不传 onStepTap = 纯静态指示器。
- **a11y**:**一个** `Semantics(label:<流程名>, value:'Step N of M' [+current label], liveRegion)`;装饰点 ExcludeSemantics(别读 N 个空圈);done/upcoming 进 value 串和/或 check 字形,非仅靠色。**可点时**每个已完成节点 `Semantics(button, label: 跳到第N步)` + 焦点(current/upcoming 仍无焦点)。
- **reduced**:`reduced` → `AnimatedContainer` 时长 `Duration.zero` 瞬切;三态靠色+形+字形,不丢信息。
- **新 token**:`AnSize.stepDot`(**likely reuse** dot=7);`stepDotCurrent`/`stepConnector`(reuse hairline)/`stepPill`(reuse badge=22)**仅在对应变体真做且 reuse 不合时**才加。

### 3.4 AnSkeleton（第四,首个自拥控制器件)——骨架/扫光
- **Stateful + SingleTicker**;4 个手写固定形(row/card/text/lines)填 muted token 色;**一个** `AnimatedBuilder` 在根裹**一个** `ShaderMask(blendMode: srcATop)`,其 `LinearGradient(base,highlight,base)` 由 `_AnSweepTransform`(GradientTransform → `Matrix4.translationValues(bounds.width*t,0,0)`)平移。控制器 `AnMotion.breath` + `.repeat()`,value 上 easeOut 软滑。根裹 `RepaintBoundary`。骨头是**不透明**填充(srcATop 对透明不画)、`AnRadius.tag`(文本行)/`card`(卡块)。生命周期仿 AnStatusDot `_sync`,但**急切初始化**(§1.4)。
- **变体**:row(前导点/图标骨 + 1–2 文本行,AnSize.row 高)· card(卡块 + 标题行 + 2 meta 行)· text(单文本行,高=body 行盒)· lines(N 行,`count`,**末行短 ~60%**)· static(reduced 兜底:同骨、静态 mid 填充、无控制器)。
- **a11y**:整体 `Semantics(liveRegion:true, label:<loading>)`(骨架→内容替换被播报);每根装饰骨 ExcludeSemantics;非交互、无焦点。
- **reduced**:门控 `reducedOrAssistive`(shimmer 是装饰循环)→ 同骨、静态 mid 填充、**不启动**控制器;`didChangeDependencies` 处理标志中途翻转。
- **新 token**:`AnColors.skeletonBase`(muted 骨色,light+dark+copyWith+lerp,介于 surfaceHover/surfaceActive,**非**裸灰/accent)· `AnColors.skeletonHighlight`(单色扫光高光,略亮)· `AnSize.skeletonLine`(文本行高≈body 行盒,reuse caretHeight 级度量若合)。reuse breath / tag·card 半径。

### 3.5 AnTypewriter（第五,最难控制器件)——打字机循环
- **Stateful + SingleTicker(急切初始化)**;**一个** controller 跑归一化 0..1 时间线:TYPING(`.characters.take(n)`)→ HOLD → DELETING → GAP → 进下个 phraseIndex;光标闪是同一 value 的第二信号(保持 Single)。`AnimatedBuilder` 只重建 Text+光标(裹 RepaintBoundary)。`Text(maxLines:1, overflow:clip)`。**预留** `caretHeight + caretEndPad` 防行重排/裁字。**无 Timer**(TickerMode 离屏自停控制器;Timer 不会)。**字素安全**:`String.characters.take/length`,绝不 substring/codeUnit(否则切碎 emoji/CJK)。
- **变体**:`phrases: List<String>`(循环)· `loop:bool`(false → 打完最后一句停、稳定光标)· `showCaret` + 光标样式(细条默认/块)· 光标 tone(inkMuted/accent)· 速度预设映射 token· textStyle/alignment。
- **a11y**:两层——`ExcludeSemantics` 裹动画 Text(AT 不读半串/逐字)+ 外层 `Semantics(label:<当前完整短语>)`。装饰循环 label **liveRegion=false**(循环 liveRegion 刷爆 SR);仅当短语是必听状态才 true。可只暴露 phrases.first 避 label 抖动。
- **reduced**:门控 `reducedOrAssistive` → 渲染**完整静态主短语**(`phrases.first`,确定性)为纯 Text、不启动控制器、无闪光标;`didChangeDependencies` 切静态不漏跑控制器。
- **新 token**:`AnMotion.typePerChar`(≈60ms,derive from fast)· `deletePerChar`(~半)· `typeHold`(reuse slow/breath 若合)· `typeGap`(小,derive from fast)。光标闪 = **reuse `AnMotion.breath`**(决策②,不新增 caretBlink token)。reuse caret/caretHeight/caretEndPad + ink/accent。

### 3.6 AnTags（最后,最复合)——可编辑标签集
- **Stateful**(持内联输入 FocusNode/controller + 键盘焦点索引);`Wrap(spacing,runSpacing)` of **AnInteractive 药丸**(复用 AnBadge 几何 + 可选 AnStatusDot 健康点)+ 尾随 `AnInput(seamless)` 内联添加(约束到 `inlineEditMin`)。每药丸:健康点 + label + **自有 Semantics 的** AnInteractive 移除-x(AnIcons.close,iconSm)。`FocusTraversalGroup` → Tab 走药丸再到输入。**Backspace 删末标签仅在** `controller.text.isEmpty` + 折叠选区 offset 0 时(Shortcuts/onKeyEvent 作用域化,**别**全局绑)。Stateful 持有并 **dispose** FocusNode/controller;async 增删 mounted 守卫。
- **变体**:mode single(**状态层**强制单值,非仅视觉)/ multi(默认)· editable vs readOnly(readOnly 去 x + 输入 → 纯展示药丸,AnTags 即标签**展示**正典)· 带/不带健康点· 添加方式(常驻内联输入 vs '+' 揭示)· 每标签 tone· 空态(editable→输入 placeholder;readOnly→委托 AnState faint hint)。
- **决策④:重复添加 = 拒 + 短暂聚焦已存在药丸**(不加重复、不静默);逗号/空格是否作分隔走 config/i18n、不硬编码。
- **a11y**:每药丸是 Semantics 节点;移除-x **自有可聚焦** `Semantics(button, label: removeTag(name))`;可选药丸 `Semantics(selected:)`;输入框带 slang placeholder;FocusTraversalGroup 阅读序;健康点装饰 → ExcludeSemantics(非焦点停)。
- **reduced**:门控 `reducedOrAssistive`(AnStatusDot 已只在 run 动)→ 点静态满色、增删跳过 AnimatedSwitcher 瞬时、hover/focus fade 瞬时;静态不丢功能。
- **新 token**:`tagRemoveHit`(x 最小命中区,避 InputChip 'tap 附近就删' bug)· `tagGap`/`tagRunGap`(**或 reuse** AnSpace.s6/s4)。reuse iconSm + AnToneColors + AnStatusDot 色 + fast/breath。

---

## 4. 新 token 汇总（去重;先 reuse、不合再 mint）

| token | 用途 | 备注 |
|---|---|---|
| `AnColors.skeletonBase` | 骨架骨色 | 新增到 ThemeExtension(light+dark+copyWith+lerp);介于 surfaceHover/surfaceActive |
| `AnColors.skeletonHighlight` | 扫光高光 | 单色、略亮、非 accent |
| `AnSize.skeletonLine` | 文本行骨高 | ≈body 行盒;先试 reuse caretHeight 级 |
| `AnSize.stateIcon` (≈40) | State 大占位字形 | 大于 iconLg(20) |
| `AnSize.stateMaxWidth` | State 内容列上限 | **先试 reuse** menuMaxWidth(360)/block(280) |
| `AnSize.tagRemoveHit` | Tags x 命中区 | |
| `AnMotion.typePerChar` | 打字逐字素节奏 | derive from fast,~60ms |
| `AnMotion.deletePerChar` | 删除节奏 | ~半 typePerChar |
| `AnMotion.typeHold` | 满短语停顿 | **reuse** slow/breath 若合 |
| `AnMotion.typeGap` | 换句空隙 | derive from fast |
| ~~`AnMotion.caretBlink`~~ | 光标闪周期 | **决策②:reuse breath,不新增** |
| (条件)`stepDotCurrent`/`stepConnector`/`stepPill` | Stepper | 仅对应变体真做且 reuse 不合时 |
| (条件)`tagGap`/`tagRunGap` | Tags Wrap 间距 | 优先 reuse AnSpace.s6/s4 |

**刻意不新增**:info 色(map accent)· Callout 半径/内距/淡入(reuse chip·card / s8·s12 / fast·easeOut)· Stepper 色/动效(reuse 现有 + mid·easeOut)· Tags tone/健康色(AnToneColors/AnStatusDot)。

---

## 5. 建造顺序（简→繁、依赖优先;逐件提交单一作者)

`0. Pre-work(§2)` → `1. AnCallout` → `2. AnState` → `3. AnStepper` → `4. AnSkeleton` → `5. AnTypewriter` → `6. AnTags`

理由:静态件(Callout/State/Stepper)先于自拥控制器件(Skeleton/Typewriter)先于最依赖兄弟的复合件(Tags);AnState 在 AnSkeleton 前(State 只引用 loading→skeleton 交接契约,Skeleton 才实现它);急切初始化修复 + AnMotionPref 最先落(所有动画件绑定它)。Callout 先立 severity→tone 映射 + 两机制 a11y 范式供 State 复用。

---

## 6. 决策（已定 —— 2026-06-23）

| # | 决策 | 影响 |
|---|---|---|
| ① | **AnState.error = 单色**(inkFaint 图标,severity 词只进 a11y label,红留给 AnCallout) | AnState 不引 AnToneColors |
| ② | **AnTypewriter 光标闪 = reuse `AnMotion.breath`**(1800ms,克制) | 不新增 caretBlink token |
| ③ | **AnStepper G2 就带 `onStepTap`**(已完成节点可点跳回) | 已完成节点组合 AnInteractive(焦点 + Enter/Space + `Semantics(button)`);current/upcoming 不可聚焦 |
| ④ | **AnTags 重复添加 = 拒 + 短暂聚焦已存在药丸** | 不加重复、不静默;分隔符走 config/i18n |
| ⑤ | (默认)**Skeleton 静态填充直接用 `skeletonBase` 色值** | 不新增 AnOpacity token |
| ⑥ | (默认)**AnTypewriter reduced 主短语 = `phrases.first`** | 确定性;如某调用方需指定主索引再加参数 |
| ⑦ | (默认)**新 AnSize 先 reuse、不投机 mint**:`stateMaxWidth` 先试 reuse menuMaxWidth/block;`stepDotCurrent`/`stepConnector`/`stepPill`、`tagGap`/`tagRunGap` 等变体真做再加 | 避免没用的 token |

---

## 7. 验收（同 G1 已验证流程 + reduced 轴）

逐件:搭 G0/G1 基座 → gallery 全态 specimen(含**两态**:motion-on + reduced)→ `make verify`(matrix 加 reduced 轴,§1.14)→ **真机 macOS 截图复核** → 复杂件对抗复审 → 同提交同步 `design-system.md`(§4 表 + G2 注 + §2 通用标准提取)→ commit。落地后本篇填 `landed-into:` + 移 `archive/`(GOVERNANCE §9)。
