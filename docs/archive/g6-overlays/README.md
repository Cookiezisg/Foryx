---
id: WRK-041
type: working
status: archived
owner: @weilin
created: 2026-06-25
reviewed: 2026-06-25
review-due: 2026-09-23
audience: [human, ai]
landed-into: references/frontend/design-system.md
---

> **已归档(2026-06-25)**:G6 全落地(AnDialog `RawDialogRoute` + AnToast 自管 Overlay 栈 + AnOverlayController/Host 命令式派发 `NotifierProvider` + `shadowWin`/`toastMaxWidth` token 地基 + app/gallery `builder` 装配 + ProviderScope),结论(用户拍板 2 + 自决 3 + SDK 实证纠正 + 验收)已 landed 进 [`design-system.md`](../../references/frontend/design-system.md) §4「G6 浮层要点」+ [`architecture.md`](../../references/frontend/architecture.md) §3「命令式浮层派发」。**UI kit 至此全部落成**(G0–G6)。**推迟**(§5):`openDialog(builder)` 富内容口 · `context.showToast()` 糖壳 · `AnModalCard` 抽象 · go_router root-Navigator 归属 · feature 级 notification rail。**落地后对抗复审(31-agent)折入**:dialog 自补 `Semantics(namesRoute,label)`(RawDialogRoute 不白送路由命名)· toast/dialog 约束 clamp ≥0(防极小视口负约束断言)· toast `_anim` 时长每帧 re-sync(运行期 reduced 切换作用离场)· host `detachNavigator` identity 守卫 · confirm 被抢占解 false 入文档 · 补单实例/reduced/软上限驱逐测试。**真机(Impeller)二次复审(用户)修**:① 浮层文字补 `Material(type: transparency)` 祖先——消 toast/dialog 的「missing Material」黄色下划线 debug 标记(画廊静态内容裹在 Material 里故干净,而浮层在 builder Stack / RawDialogRoute 下无 Scaffold);② **去 `_AnToastLayer` 的 `ClipRect`+`ConstrainedBox(maxHeight)`**——它把每条 toast 的 `shadowPop` 在 Column 紧边界四向**截断**(用户报「阴影被长方形挡住」),软上限 5 已兜高度、host Stack 裁到屏即唯一想要的边界(故 toast 层不再有 ConstrainedBox,负约束 clamp 只剩 dialog 卡)。**实施简化(本篇 §2.2 规划 vs 落地)**:toast 宽由规划的弹性 `minWidth 280 / maxWidth 360` 简化为**均匀 360**(`Expanded` 钉关钮右;真弹性须自研 RenderObject,瞬时件不值,见 design-system)。本篇留作建造存档(调研 + 决策 + SDK 实证)。

# WRK-041 — G6 浮层(Dialog + Toast)建造规范

> UI kit **最后一组**。开工前经 13-agent 工作流(7 维并行 survey → 综合 → 4 镜对抗复审 → 折叠定稿)+ 主循环对 Flutter 3.41.9 SDK 源**逐条实证**(纠正了调研的一处核心论据,见 §6),用户拍板 2 决策 + 主循环自决 3 决策。

## §0 一句话

造 **2 个浮层原语 + 1 组装配**:
- **AnDialog** —— 全屏阻断 modal(占屏居中卡 + 变暗遮罩 + 焦点陷阱/归还/Escape/点遮罩关)。v1 = `confirm()` 确认框(取消 + 危险动作),富内容口推迟。
- **AnToast** —— 屏角无锚定时栈(右下、向上堆叠、tone 色条、自动消隐、可选 action)。
- **AnOverlayHost / AnOverlayController** —— `NotifierProvider` service + app/gallery 根挂的单一 host:命令式 `showToast()`(无 context 直驱)/ `confirm()`(经 host 注册的 root-navigator push)。

**机制全骑框架成熟件(#8),零新依赖**:Dialog 自建 `RawDialogRoute`(焦点陷阱/归还/Escape/遮罩/scopesRoute a11y 全框架白送),Toast 在 `MaterialApp.builder` 静态挂自管 `Overlay`(两条 entry:业务树 + toast 层,绕开 `Overlay.of(rootOverlay)` 的 `LookupBoundary` 雷区)。`AnFloating(demo F1)≡ AnPopover` 已落,**本组不重做**,仅同提交修其头注释失效承诺。

## §1 锁定决策

**用户拍板(2026-06-25)**:
1. **toast × dialog z 序 = 接受偏离(A)**:`MaterialApp.builder` 的 toast 层渲染在 Navigator 之上,故 toast 浮在 dialog 之上(与 demo `--z-dialog 1400 > --z-toast 1200` 倒置)。接受 —— toast 非阻断、modal 下弹 toast 极罕见、无实际伤害;复刻 demo 需把 toast 拖回 `Overlay.of(rootOverlay)` 雷区,机制风险高。**显式留痕于 AnOverlayHost 头注释**。
2. **toast 海量软上限 = 5 条**:超出时最旧**立即**移除(无退场动画,符「立即」语义)。demo 底层 list 不限会堆爆,本上限是健壮性增强。

**主循环自决**:
3. **命令式触发架构 = `NotifierProvider` service + app 单 host**:`AnOverlayController extends Notifier<AnOverlayState>`(Riverpod 3.x 一等原语;**禁用 legacy `ChangeNotifierProvider`** —— 仅 `riverpod/legacy.dart` 导出、违 ADR 0004 取向、与 widget-local 的 `AnPopoverController extends ChangeNotifier` 撞用途)。toast 无 context 直驱;dialog 经 host 在 `initState` 注册的 root-navigator(`GlobalKey<NavigatorState>`,由 app 创建并经 widget 树注入 + ref 注册,**非**全局静态 key)push。否决 rootNavigatorKey 全局(隐藏全局/不可 override 测/违 Riverpod DI)与 BuildContext extension(async 无 context 调不了)。
4. **toast action 钮 = 内联 AnInteractive 小钮**:demo accent 文字 + accentSoft hover;现 `AnButton` 变体仅 `{ghost,primary,danger,icon}` 无 accent,全 demo 仅此一处用 accent 文字钮,不为单点膨胀 AnButton。两个 close 钮直接复用 `AnButton.iconOnly`(dialog=md / toast=sm)。
5. **架构留痕落点 = `references/frontend/architecture.md`**(reference 类可演进),**不改**不可变的 ADR 0004。token 缺口 + 视觉解剖归 design-system.md。

## §2 逐件落地规格

### §2.1 AnDialog → 自建 `RawDialogRoute<bool>`

机制(已核 3.41.9 SDK 源,见 §6):自建 `RawDialogRoute` 经注册的 root navigator `push`,**非** `showDialog`(强制 Material 转场 + SafeArea + black54 遮罩 + 需 caller context)、**非** `showGeneralDialog`。直接构造可拿 route 句柄做单实例治理 + 用自定义 spring 转场 + scrim 遮罩。

构造(全部参数实证存在):
- `barrierColor: c.scrim`、`barrierDismissible: true`、`barrierLabel: <调用方传入,非空>`(barrierDismissible 时框架 assert 必非空)。
- `traversalEdgeBehavior: TraversalEdgeBehavior.closedLoop`(**直接构造不默认它**;showDialog 才默认 —— §6 纠正)。
- `transitionDuration: reduced ? Duration.zero : AnMotion.mid`(**唯一**时长参数;`reverseTransitionDuration` getter 默认回落它,无需设两个)。
- `transitionBuilder`:reduced 直返 child;否则 `FadeTransition(opacity: animation.drive(CurveTween(spring)))` 套 `AnimatedBuilder` 应用 `Transform.translate(0,(1-t)*s8)` + `Transform.scale(.98+.02*t)`。**用 `animation.drive(CurveTween)` 而非 `CurvedAnimation`**(后者须 dispose、inline 创建泄漏)。遮罩淡入由框架 `AnimatedModalBarrier` 随 transitionDuration 自动,无需手淡。
- `pageBuilder`:`_AnConfirmCard`(title/message/cancel/confirm)。框架 `buildPage` 已硬包 `Semantics(scopesRoute:true, explicitChildNodes:true)`。

卡 `_AnConfirmCard`:`Center` → `ConstrainedBox(maxWidth: AnSize.content=720, maxHeight: MediaQuery.sizeOf(context).height − AnSpace.s48)` → `DecoratedBox(color: surface, borderRadius: AnRadius.island=20, border: Border.all(line, hairline), boxShadow: shadowWin)` → `ClipRRect(island)` → `Column(head / body / foot)`,`mainAxisSize.min`。
- head:`SizedBox(height: AnSize.islandHead=44)` + `Row(padding LTRB(s16,0,s8,0))`:title(`Flexible` 单行省略,`AnText.strong` 色 ink)+ close `AnButton.iconOnly(AnIcons.close, size: md, semanticLabel: t.feedback.dismiss)`。下 hairline。列间距 `AnSpace.s8`。
- body:`Flexible` + `SingleChildScrollView(padding s16)` + message `Text(style: AnText.body 色 inkMuted, height 1.6)`(`message==null` → 无 body)。
- foot:`Padding(LTRB(s16,s12,s16,s12))` 上 hairline + `Row(MainAxisAlignment.end, 间距 s8)`:cancel `AnButton(ghost, label: cancelLabel)` → `Navigator.pop(context,false)`;confirm `AnButton(variant: confirmTone==danger?danger:primary, label: confirmLabel)` → `Navigator.pop(context,true)`。

**保真行为**:遮罩 scrim 居中 padding s24(由 Center + maxWidth 体现);卡 island20 + surface + hairline line + shadowWin + 进场 translateY(s8)scale(.98)→spring;单实例(controller 持 route 句柄,新 confirm 前若旧 dialog 活跃先 `removeRoute`);**不照搬** demo 的 transitionend 退场兜底(Navigator pop 驱动退场确定,无 web 失火坑)。

**a11y 断言**(★=框架白送):★scopesRoute(`find.bySemanticsLabel(title)`)、★closedLoop 闭环陷阱(连发 Tab N+1 焦点回首项、背景探针 hasFocus 恒 false)、★Escape 关(`_DismissModalAction.maybePop()`,barrierDismissible 同闸)、★点遮罩关/点卡内不关、★焦点归还(所有关闭路径走同一 Navigator.pop、route 统一兜)、★背景 tap 不触发。手接:无(onClick-返-false 阻关属推迟的富内容口,confirm v1 不需)。XSS:`Text` 天然免疫。

### §2.2 AnToast → app 根自管 `Overlay` 常驻栈

机制:`MaterialApp.builder` 返回 `AnOverlayHost(child: child)`,host build `Overlay(initialEntries: [内容 entry(child), toast 层 entry])`。toast 非锚定/非阻断/不夺焦,**绝不进 Navigator route**。否决 `ScaffoldMessenger`(底部居中一次一条 + Material 皮,与右下真栈形态冲突)、裸 `Stack`(toast 变更触发整子树 rebuild)。

toast 层 `_AnToastLayer`(`ConsumerWidget`,`ref.watch(overlayProvider).toasts`):`Positioned(right: s24, bottom: s24)`(**只占右下小区**,栈外天然穿透,避 `IgnorePointer` 误吞)→ `ConstrainedBox(maxHeight: MediaQuery.sizeOf().height − s48)` → `ClipRect` → `Column(verticalDirection: VerticalDirection.up, mainAxisSize.min, 间距 s8)`,children 为 `_AnToastItem(key: ValueKey(id), data, onDismissed: () => ref.read(overlayProvider.notifier).dismissToast(id))`。

单条 `_AnToastItem`(`StatefulWidget` + `SingleTickerProviderStateMixin`):
- `initState` **急切**建 `_anim`(`AnimationController(duration: reduced?Duration.zero:AnMotion.mid)`)+ `forward()`;`duration>Duration.zero` 才建 `_timer = Timer(duration, _dismiss)`(缺省 4s,`Duration.zero`=常驻)。controller **绝不**放 late-final 懒初始化器(AnPopover dispose 期崩教训)。
- `dispose`:`_timer?.cancel()` + `_anim.dispose()`。
- `_dismiss`:`_anim.reverse().whenComplete(() { if (mounted) widget.onDismissed(); })`(自驱退场 → 再通知 controller 移除;**不需** transitionend 兜底 setTimeout,`whenComplete` 确定)。
- build:`Semantics(liveRegion: true, container: true, label: text)` → `FadeTransition + SlideTransition`(s8 位移,spring 经 drive)→ `DecoratedBox(surface + Border.all(line,hairline) + AnRadius.chip + shadowPop)`(= AnMenuSurface 同款 surface idiom)→ `DefaultTextStyle(基色 inkMuted = demo --ink-2)` → `Padding(LTRB(s8,s8,s12,s8))` → `Row(间距 s8, minWidth: AnSize.block=280, maxWidth: AnSize.toastMaxWidth=360)`:
  - tone 色条 `ExcludeSemantics`(`width AnSpace.s4` = demo --grid 一格,`align stretch`,`AnRadius.pill`,色:none→inkFaint / ok→ok / warn→warn / danger→danger;**none 特判 inkFaint(=demo --ink-3),非 AnToneColors 的 inkMuted**)。
  - text `Flexible(Text(style: AnText.body 色 ink, maxLines 2, ellipsis))`。
  - 可选 action:`AnInteractive` 小钮(controlSm 高 / `AnText.meta.weight(w500)` 色 accent / hover accentSoft 圆角 button)→ onPressed + `_dismiss`。
  - close `AnButton.iconOnly(AnIcons.close, size: sm, semanticLabel: t.feedback.dismiss)` → `_dismiss`。

**保真行为**:右下固定栈、column-reverse 向上堆叠、pointer 分层(栈容器只占小区 + 单条 auto)、自动消隐 timer、栈空不撤容器(常驻 entry,比 demo 省)、tone 色条默认中性。

**a11y 断言**:`getSemantics(toast).flagsCollection.isLiveRegion == true`(永远 polite,**不用** `SemanticsService.announce` —— 桌面坏 + 被 VoiceOver 抢读 + deprecated)、show 后文本被语义拾取、**不夺焦**(背景探针 show 后 hasFocus 恒 true)、action/close `isButton` 可达 tap、不可仅靠自动消隐(永远带 close;`Duration.zero` → pump 10s 仍在)、timer 与动画解耦(reduced + 默认时长 → pump 4s+ 已摘)。

### §2.3 AnOverlayController / AnOverlayState / AnOverlayHost(`core/overlay/an_overlay.dart`)

- `AnToastTone { neutral, ok, warn, danger }`、`AnToastAction({label, onPressed})`、`AnToastData({id, text, tone, action, duration})`(`duration` 缺省 4s,`Duration.zero`=常驻)。
- `AnDialogTone { primary, danger }`。
- `AnOverlayState({toasts = const []})` 不可变 + copyWith。
- `AnOverlayController extends Notifier<AnOverlayState>`:
  - `build() => const AnOverlayState()`。
  - `showToast(text, {tone, action, duration})` → 生成 id(`'toast_${_seq++}'` 递增,确定性)、append、超 `maxToasts=5` 移最旧、`state = copyWith`。返回 id。
  - `dismissToast(id)`。
  - `attachNavigator(GlobalKey<NavigatorState>?)`(host initState 注册 / dispose 清)。
  - `confirm({title, message, confirmLabel, cancelLabel, barrierLabel, confirmTone=danger}) → Future<bool>`:`_navKey?.currentState` 空→返 false;旧 dialog 活跃先 `removeRoute`;建 `RawDialogRoute<bool>` 持句柄、push、`whenComplete` 清句柄;`(result ?? false)`。`reduced`/`scrim` 从 `nav.context` 读。
- `overlayProvider = NotifierProvider<AnOverlayController, AnOverlayState>(AnOverlayController.new)`,**放 `core/overlay/`**(纯前端 UI 机制、无外部依赖、默认实现即终态、override 仅测试)。
- `AnOverlayHost extends ConsumerStatefulWidget({child})`:app/gallery 经 `MaterialApp.builder` 挂;`initState` 注册 navKey(app 创建并经构造注入)、`dispose` 清;build `Overlay(initialEntries: [内容, toast 层])`。**头注释显式记 z 序偏离(toast 永在 dialog 之上,决策 1)**。

调用样例(feature 校准 demo 极简用法):
```dart
ref.read(overlayProvider.notifier).showToast(context.t.feedback.copied);
final ok = await ref.read(overlayProvider.notifier).confirm(
  title: context.t.feedback.confirmDelete,
  message: '…',
  confirmLabel: context.t.action.delete, cancelLabel: context.t.action.cancel,
  barrierLabel: context.t.feedback.dialogBarrier,
);
```

## §3 token + i18n 缺口(均同提交补 + doc-sync)

| 缺口 | 落点 | 取值(对照 demo tokens.css) |
|---|---|---|
| `AnColors.shadowWin`(dialog 卡阴影,**单层**) | `colors.dart` light/dark/copyWith/lerp **四处** | light `BoxShadow(rgba(0,0,0,.20), blur 50, offset (0,16))`;dark alpha `.50`。**勿据此类推其它三档**(island/float/pop 暗色 offset/blur 全异)。 |
| `AnSize.toastMaxWidth = 360` | `tokens.dart` | demo `--island-w 360`。语义独立(`menuMaxWidth`/`stateMaxWidth` 数值同 360 但语义各异,retune 会牵连)。 |
| toast 色条宽 | **复用 `AnSpace.s4`** | demo `--grid 4`。**不新增 `toastBar`**(单消费者 + 数值重复 = 反 #8 错误抽象)。注释「色条 = --grid 一格」。 |

i18n 新增键(en + zh_CN,同提交):`feedback.confirmDelete`(确认删除标题)、`feedback.dialogBarrier`(遮罩 aria-label,如「关闭对话框」/「Dismiss dialog」)。新增 `action.delete`(删除)。复用现有:`feedback.dismiss`(两 close aria-label)、`action.cancel`(取消)。无须新增色/曲线/时长 token(spring/mid/4s 走 `AnMotion.*` + `Timer`;lh-prose 1.6 用 `copyWith(height:1.6)`)。

## §4 验收三层

**demo 基线**(catalog.js「浮层」逐字核):`an-toast` 1 specimen(bell 触发,fire-and-forget)、`an-dialog` 1 specimen(trash 触发,confirm-delete)、`an-floating` 1 callout(基座不展示)。零富内容 dialog、零需 context 的 toast。

1. **gallery specimen**(gallery-first,铺得比 demo 全):
   - **前置硬伤修(必)**:`gallery_main.dart` 明文无 ProviderScope + GalleryApp 是 plain StatefulWidget → `ref.read(overlayProvider)` 当场抛「No ProviderScope」。**给 gallery 补 `ProviderScope`** + GalleryApp 的 `MaterialApp` 加 `builder:` 挂 `AnOverlayHost`(host 须可复用)。
   - Dialog:confirm-delete 触发钮(点开真现于 Overlay)、含 primary-confirm 与 danger-confirm 两 specimen、无 message 一条。
   - Toast:tone × {neutral/ok/warn/danger} 全枚举 + 含 action 一条 + `Duration.zero` 常驻一条 + 连发触发(验软上限 5 + 堆叠)。
2. **widget-test matrix + 五电池**:能 build / 不溢出 / 转义安全(`<script>` 作 title/text 原样) / 渲染存在(pump 后浮层现于 Overlay)/ 内部钮 disabled 契约。五电池:空(无 message foot / 空 text)、超长(title 单行省略 + body 滚动夹 vh−s48 / toast 280–360 省略)、海量(连发 N → 软上限 5 + dialog 单实例防叠)、极值(无空格超长 token 不溢出)、注入。触发可测:`ProviderScope(overrides: [overlayProvider.overrideWith(RecordingController)])`。
   - 测试约定:`tester.ensureSemantics()`/`handle.dispose()`;matcher 用 `flagsCollection.<flag>`(`isLiveRegion`/`isButton`/`scopesRoute`,`.toBoolOrNull()`,**禁** `hasFlag`/`containsSemantics`);焦点用背景探针 `Focus(focusNode: probe)` + `addTearDown(probe.dispose)`;装饰元素 `ExcludeSemantics`。
   - SDK 兜底测试:barrierDismissible 缺 barrierLabel 触 assert;reduced 进退瞬时。
3. **工程纪律**:grep 禁裸 hex/px(3 缺口走地基)、slang 无硬编码(service 不持文案、close/barrier aria-label 走 `t.*`、新键登记)、S11 双语 only-why、层依赖(controller 在 core/overlay 不依上层、host 经 app 挂)、doc-sync(§7)。截图验收含缩放档(Cmd +/- 下 dialog maxHeight 夹取、toast 栈不溢出)。

## §5 推迟项

- `openDialog(builder)` 富内容口:v1 砍(全 demo 零富内容、与「service 不持 widget/文案调用方解析」边界冲突)。
- `context.showToast()` 语法糖薄壳:v1 砍(硬依赖 context、async 调不了)。
- `AnModalCard` 抽象(**不叫 AnIslandCard** —— 避与现存 `an_island.dart`〔chip 圆角 + shadowFloat〕撞名;dialog 卡是 island20 + shadowWin + inset 物理不同):单消费者,**内联在 AnDialog**(YAGNI),待第 2 个 island 档卡再 G0-style 抽。
- go_router root-Navigator 归属:仅预埋注释(go_router 未接);落地后须确认 dialog push 的是 root Navigator,否则 ShellRoute 子 Navigator 会让 dialog 被 nav rail 等 shell chrome 裁剪。
- feature 级浮层(notification rail 属 feature 非 kit)。

## §6 SDK 实证纠正(主循环对 3.41.9 源逐条核,纠正调研)

- **closedLoop 是 `showDialog`/`DialogRoute` 的默认值**(`material/dialog.dart:1522` `traversalEdgeBehavior ?? TraversalEdgeBehavior.closedLoop`)—— 调研「showGeneralDialog 不给闭环陷阱、故必须 RawDialogRoute」的论据在本版**不成立**。改用 RawDialogRoute 的真理由是:① 自定义 scrim 遮罩 + spring 转场(showDialog 强制 Material 转场/SafeArea/black54);② 拿 route 句柄做单实例治理;③ 无 caller context 触发。**直接构造 RawDialogRoute 不默认 closedLoop**(它是 `super.` 参数、走 ModalRoute 全局默认 parentScope),故须**显式传** `closedLoop`。
- `RawDialogRoute` 真实构造(`widgets/routes.dart:2581`):唯一 `transitionDuration`(无 reverse 参数,getter 默认回落);`barrierColor` 默认 `0x80000000`;`buildPage` 硬包 `Semantics(scopesRoute:true, explicitChildNodes:true)`。
- Escape 关闭框架白送(`widgets/routes.dart:980` `_DismissModalAction.isEnabled = route.barrierDismissible` → `invoke = Navigator.maybePop()`;文档 1720「pressing the escape key … will cause the current route to be popped with null」)。

调研其余结论(toast 用自管 Overlay、命令式用 NotifierProvider、liveRegion 而非 announce、shadowWin/toastMaxWidth 缺口、z 序偏离、gallery 缺 ProviderScope、ChangeNotifierProvider 是 legacy、AnFloating≡AnPopover)经核**成立**,照采。

## §7 doc-sync 清单(同提交)

- `references/frontend/design-system.md`:§4 routing 表 G6 行 →✅ + token 缺口(shadowWin/toastMaxWidth)+ dialog/toast 视觉解剖 + 链接 working→archive。
- `references/frontend/architecture.md`:命令式浮层触发架构留痕(NotifierProvider service + host 注册 push + z 序偏离),**不改 ADR 0004**。
- `an_popover.dart` 头注释:删「the dialog/toast layer (G6) reuses it」→「dialog 走 RawDialogRoute、toast 走静态 Overlay entry,均非锚定故不经本基座」(S11 only-why)。
- `app.dart` 头注释:home: → builder: 包 Overlay 的整体重述。
- 新增 gallery 的 ProviderScope + host(代码,属同提交装配变更)。
- 本篇落地后:提取结论进 design-system.md + architecture.md、填 `landed-into`、移 `docs/archive/g6-overlays/`。
