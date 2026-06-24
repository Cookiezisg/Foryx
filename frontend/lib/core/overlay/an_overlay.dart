import 'package:flutter/widgets.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../design/colors.dart';
import '../design/tokens.dart';
import '../ui/an_dialog.dart';
import '../ui/an_toast.dart';

/// G6 imperative-overlay dispatch (WRK-041 §2.3) — the ONE service a feature reaches for to fire a
/// toast or a confirm dialog WITHOUT a BuildContext (e.g. from an SSE handler or an async callback).
/// A classic Riverpod [NotifierProvider] (NOT the legacy `ChangeNotifierProvider`, which only lives in
/// `riverpod/legacy.dart` and clashes with the widget-local `AnPopoverController extends ChangeNotifier`):
///   - [showToast] is fully context-free — it just appends to the toast stack the [AnOverlayHost]
///     renders. A soft cap (5) drops the oldest immediately (WRK-041 decision 2).
///   - [confirm] needs a Navigator to push the dialog route; the host registers the root navigator key
///     ([attachNavigator]) so the controller can push without holding a BuildContext. This is the
///     ADR-0004-clean alternative to a global rootNavigatorKey (app-created, tree-injected, ref-wired,
///     override-able in tests) — NOT a hidden global singleton.
///
/// Lives in `core/overlay` (a pure front-end UI mechanism, no upward deps; the default impl is the
/// final one, overridden only in tests). The host mounts at the assembly root via `MaterialApp.builder`.
///
/// G6 命令式浮层派发——feature 无 BuildContext(SSE/async 回调里)也能弹 toast / confirm 的唯一 service。
/// 经典 NotifierProvider(非 legacy ChangeNotifierProvider)。showToast 全程无 context;confirm 经 host 注册的
/// root navigator key push(ADR 0004 干净替代全局 key:app 建 + 树注入 + ref 接 + 可 override 测)。放 core/overlay。
///
/// [AnDialogTone] (the confirm button's tone) is defined in `an_dialog.dart`. 确认钮 tone 在 an_dialog.dart。

/// One toast's data (the controller's state; the presentational [AnToast] widget renders it). id is a
/// monotonic counter (deterministic — good for tests). toast 数据(controller 态;AnToast 渲染)。id 单调计数(确定、测试友好)。
@immutable
class AnToastData {
  const AnToastData({
    required this.id,
    required this.text,
    this.tone = AnToastTone.neutral,
    this.action,
    this.duration = anToastDefaultDuration,
  });

  final String id;
  final String text;
  final AnToastTone tone;
  final AnToastAction? action;
  final Duration duration;
}

/// Immutable overlay state — just the live toast stack (oldest first; the host stacks bottom-up). 不可变浮层态。
@immutable
class AnOverlayState {
  const AnOverlayState({this.toasts = const []});
  final List<AnToastData> toasts;
  AnOverlayState copyWith({List<AnToastData>? toasts}) =>
      AnOverlayState(toasts: toasts ?? this.toasts);
}

class AnOverlayController extends Notifier<AnOverlayState> {
  /// Soft cap on simultaneous toasts (WRK-041 decision 2) — over this, the oldest is dropped at once
  /// (demo's unbounded list would pile up under a burst). 海量软上限:超出立即移最旧(demo 不限会堆爆)。
  static const int maxToasts = 5;

  int _seq = 0;
  GlobalKey<NavigatorState>? _navKey;
  Route<dynamic>? _activeDialog;

  @override
  AnOverlayState build() => const AnOverlayState();

  // ── host wiring ──

  /// The host registers the root navigator key so [confirm] can push without a BuildContext. host 注册 root navigator。
  void attachNavigator(GlobalKey<NavigatorState> key) => _navKey = key;

  /// Detach ONLY if [key] is still the registered one — a later host may have re-registered before this
  /// (disposing) host runs, and an unconditional clear would null out the live host's key (same
  /// identity-guard idiom as [confirm]'s `_activeDialog`). 仅当 key 仍是当前注册者才解绑(防后注册 host 被误清)。
  void detachNavigator(GlobalKey<NavigatorState> key) {
    if (identical(_navKey, key)) _navKey = null;
  }

  // ── toasts (context-free) ──

  /// Fire a toast. Returns its id (so a caller can [dismissToast] early). fire-and-forget;返回 id。
  String showToast(
    String text, {
    AnToastTone tone = AnToastTone.neutral,
    AnToastAction? action,
    Duration duration = anToastDefaultDuration,
  }) {
    final data = AnToastData(
      id: 'toast_${_seq++}',
      text: text,
      tone: tone,
      action: action,
      duration: duration,
    );
    final next = [...state.toasts, data];
    // Over the cap → drop oldest immediately (no exit animation — "立即" per decision 2). 超上限立即移最旧。
    if (next.length > maxToasts) next.removeRange(0, next.length - maxToasts);
    state = state.copyWith(toasts: next);
    return data.id;
  }

  /// Remove a toast by id (called by the widget AFTER its exit animation). 按 id 移除(widget 离场动画后调)。
  void dismissToast(String id) {
    if (state.toasts.any((t) => t.id == id)) {
      state = state.copyWith(
        toasts: state.toasts.where((t) => t.id != id).toList(growable: false),
      );
    }
  }

  // ── confirm dialog (via the registered navigator) ──

  /// Show a confirm dialog; resolves true (confirm) / false (cancel / barrier / Escape / no host). All
  /// copy is passed in by the caller (which has a context for `context.t.*`) — the service holds no
  /// i18n. Single-instance: a stale dialog is popped before the new one (demo doesn't stack modals) —
  /// the preempted caller then resolves **false**, indistinguishable from a user cancel (fire-and-forget;
  /// false is the safe direction for a confirm-delete). 单实例:先 pop 旧的;被抢占的旧调用方解为 false(与用户取消不可区分,确认删除的安全侧)。
  /// 确认框 → `Future<bool>`(确认 true;取消/遮罩/Escape/无 host/被抢占 → false)。文案调用方传(service 不持 i18n)。
  Future<bool> confirm({
    required String title,
    String? message,
    required String confirmLabel,
    required String cancelLabel,
    required String barrierLabel,
    AnDialogTone confirmTone = AnDialogTone.danger,
  }) async {
    // host not mounted (or detached) → safe default 无 host → 安全默认
    final nav = _navKey?.currentState;
    if (nav == null) return false;
    // single-instance: drop the stale dialog first (demo doesn't stack modals) 单实例:先撤旧
    if (_activeDialog != null && _activeDialog!.isActive) {
      nav.removeRoute(_activeDialog!);
    }
    final route = anConfirmRoute(
      scrim: nav.context.colors.scrim,
      reduced: AnMotionPref.reduced(nav.context),
      title: title,
      message: message,
      confirmLabel: confirmLabel,
      cancelLabel: cancelLabel,
      barrierLabel: barrierLabel,
      confirmTone: confirmTone,
    );
    _activeDialog = route;
    final result = await nav.push<bool>(route);
    if (identical(_activeDialog, route)) _activeDialog = null;
    return result ??
        false; // null pop (barrier / Escape) → false 遮罩/Escape 的 null → false
  }
}

/// The single source for imperative overlays. keepAlive (default) — app-lifetime. override only in tests.
/// 命令式浮层唯一源。默认 keepAlive(app 生命周期),仅测试 override。
final overlayProvider = NotifierProvider<AnOverlayController, AnOverlayState>(
  AnOverlayController.new,
);

/// The assembly-root overlay host — mounted via `MaterialApp.builder` (so it sits inside the app's
/// Theme / MediaQuery / Directionality and above the [TranslationProvider]). It (a) registers the root
/// navigator key with the controller so [AnOverlayController.confirm] can push dialogs, and (b) lays
/// the toast stack in the bottom-right corner, ABOVE the app content.
///
/// Z-ORDER (WRK-041 decision 1, user-accepted): this host paints the toast layer ABOVE [child] (the
/// MaterialApp's Navigator), so toasts float OVER a modal dialog (which is a route inside that
/// Navigator) — the reverse of the demo's `--z-dialog > --z-toast`. Accepted: a toast is non-blocking
/// and a toast firing while a modal is open is vanishingly rare; faithfully reproducing the demo order
/// would mean dragging the toast layer into the `Overlay.of(rootOverlay)` LookupBoundary minefield.
///
/// 装配根浮层宿主(MaterialApp.builder 挂)。(a) 注册 root navigator key 供 confirm push;(b) 右下角堆 toast 栈、在内容之上。
/// z 序(决策 1,用户已认):toast 层画在 child(Navigator)之上 → toast 浮在 modal dialog 上(与 demo 相反)。已接受:
/// toast 非阻断、modal 下弹 toast 极罕见;复刻 demo 序须把 toast 拖进 Overlay.of(rootOverlay) 的 LookupBoundary 雷区。
class AnOverlayHost extends ConsumerStatefulWidget {
  const AnOverlayHost({
    required this.navigatorKey,
    required this.child,
    super.key,
  });

  /// The app's root navigator key (also passed to `MaterialApp.navigatorKey`) — registered with the
  /// controller so confirm dialogs push onto it. app 的 root navigator key(亦给 MaterialApp.navigatorKey)。
  final GlobalKey<NavigatorState> navigatorKey;
  final Widget child;

  @override
  ConsumerState<AnOverlayHost> createState() => _AnOverlayHostState();
}

class _AnOverlayHostState extends ConsumerState<AnOverlayHost> {
  late final AnOverlayController _controller = ref.read(
    overlayProvider.notifier,
  );

  @override
  void initState() {
    super.initState();
    _controller.attachNavigator(widget.navigatorKey);
  }

  @override
  void dispose() {
    // Identity-guarded so a re-registered live host isn't unhooked by this disposing one. 守恒解绑。
    _controller.detachNavigator(widget.navigatorKey);
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    // A Stack (not a nested Overlay): the host does NOT watch the toast list, so [child] (a constant
    // widget instance) never rebuilds when toasts change — only [_AnToastLayer] (which watches) does.
    // child below, toast layer above (the accepted z-order). Stack(非嵌套 Overlay):host 不 watch → child 不因 toast 重建。
    return Stack(
      textDirection: TextDirection.ltr,
      children: [
        widget.child,
        Positioned(
          right: AnSpace.s24,
          bottom: AnSpace.s24,
          child: const _AnToastLayer(),
        ),
      ],
    );
  }
}

/// The toast stack — a column that grows UPWARD from the bottom-right (newest at the bottom, demo
/// column-reverse). Only THIS widget watches the toast list, so a toast change repaints just the stack,
/// never the app content. NOT clipped (so each chip's shadow spreads freely — the soft cap bounds the
/// height); the small corner footprint keeps the rest of the screen click-through (no IgnorePointer).
/// toast 栈:右下向上堆(最新在底)。仅此件 watch;不裁(阴影自由扩散,软上限兜高);角落小占位故余屏可穿透。
class _AnToastLayer extends ConsumerWidget {
  const _AnToastLayer();

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final toasts = ref.watch(overlayProvider.select((s) => s.toasts));
    if (toasts.isEmpty) return const SizedBox.shrink();
    // NO ClipRect / maxHeight ConstrainedBox: the soft cap (5) already bounds the stack height, and any
    // clip here CUTS the toasts' shadows — each chip's shadowPop must spread freely past the column on
    // all four sides (the real-machine review caught the shadow being sliced at the column box). The
    // only bound we want is the host Stack's screen clip. 不裁/不限高:软上限已兜高度;裁会切掉 toast 四向阴影(真机复审揪出)。
    return Column(
      mainAxisSize: MainAxisSize.min,
      verticalDirection:
          VerticalDirection.up, // newest at the bottom, older pushed up 最新在底
      crossAxisAlignment: CrossAxisAlignment.end,
      children: [
        for (final t in toasts)
          Padding(
            padding: const EdgeInsets.only(
              top: AnSpace.s8,
            ), // gap between stacked toasts 栈间距
            child: AnToast(
              key: ValueKey(t.id),
              text: t.text,
              tone: t.tone,
              action: t.action,
              duration: t.duration,
              onDismissed: () =>
                  ref.read(overlayProvider.notifier).dismissToast(t.id),
            ),
          ),
      ],
    );
  }
}
