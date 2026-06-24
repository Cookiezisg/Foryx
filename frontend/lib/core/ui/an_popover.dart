import 'package:flutter/services.dart';
import 'package:flutter/widgets.dart';

import '../design/tokens.dart';

/// The anchored-overlay base — a floating layer pinned to a trigger, built on Flutter's mature
/// [OverlayPortal] + [CustomSingleChildLayout] / [SingleChildLayoutDelegate] (the exact mechanism behind
/// the framework's own DropdownButton / Tooltip / selection-toolbar positioning — NOT hand-rolled math,
/// see principle #8). The delegate measures the child first, THEN positions it with full knowledge of
/// child + screen size, so it can FLIP (open the other way) and CLAMP to stay on-screen — which a pure
/// [CompositedTransformFollower] affine link cannot. Opens/closes via [AnPopoverController]; dismisses on
/// outside tap or Escape. The dropdown + menu sit on this; the dialog/toast layer (G6) reuses it. The
/// overlay builder receives the anchor's size so a menu can match the trigger width.
///
/// 锚定浮层基座——搭在 Flutter 成熟的 OverlayPortal + CustomSingleChildLayout/SingleChildLayoutDelegate 上
/// (框架自身 Dropdown/Tooltip/选择工具条的定位机制,非手搓,见 #8)。delegate 先量子件、再据子件+屏幕尺寸定位,
/// 故能**翻转**(改朝向)+ **夹取**(不出屏)——纯 CompositedTransformFollower 仿射链做不到。经 controller 开关;点外/Esc 关。
class AnPopoverController extends ChangeNotifier {
  bool _open = false;
  bool get isOpen => _open;

  void open() => _set(true);
  void close() => _set(false);
  void toggle() => _set(!_open);

  void _set(bool v) {
    if (_open == v) return;
    _open = v;
    notifyListeners();
  }
}

class AnPopover extends StatefulWidget {
  const AnPopover({
    required this.controller,
    required this.anchor,
    required this.overlayBuilder,
    this.alignEnd = true,
    this.gap = const Offset(0, AnSpace.s4),
    super.key,
  });

  final AnPopoverController controller;

  /// The trigger. 触发器。
  final Widget anchor;

  /// Builds the floating content; receives the anchor size (for width matching). 浮层内容(收锚尺寸)。
  final Widget Function(BuildContext context, Size? anchorSize) overlayBuilder;

  /// Prefer aligning the menu's END edge to the anchor's end (the common ⋯-at-the-right case); the delegate
  /// flips to start-align when that would run off-screen. 优先尾对齐到锚尾(常见),越界则翻转到首对齐。
  final bool alignEnd;

  /// Offset from the anchor: [Offset.dy] = gap below the anchor, [Offset.dx] = optional horizontal nudge.
  /// 相对锚的偏移:dy=锚下间距,dx=可选水平微调。
  final Offset gap;

  @override
  State<AnPopover> createState() => _AnPopoverState();
}

class _AnPopoverState extends State<AnPopover> with SingleTickerProviderStateMixin {
  final GlobalKey _anchorKey = GlobalKey(); // reads the anchor's rect for the layout delegate 读锚矩形供 delegate
  final OverlayPortalController _portal = OverlayPortalController();

  // Who held focus when the overlay opened — handed back on close. The overlay's FocusScope seizes
  // focus, and a bare scope (unlike a Navigator route) won't auto-restore it, so a keyboard / screen-
  // reader user would be dropped to the document root on pick / Esc / outside-tap (WCAG 2.4.3).
  // 开前焦点持有者,关时归还:浮层 FocusScope 夺焦、裸 scope 不像路由自动恢复,否则键盘/屏读落到 root。
  FocusNode? _restoreFocus;

  // The nearest ancestor scrollable, listened to WHILE OPEN: scrolling it DISMISSES the popover (the
  // delegate snapshots the anchor rect at build, and a moving anchor can't be re-tracked without a frame of
  // lag — so, like the platform's own menus / dropdowns, we close rather than leave the menu stranded over
  // unrelated content). 开着时跟最近可滚祖先:滚动即关(同平台菜单/下拉,不让浮层悬停在错位内容上)。
  ScrollPosition? _scrollPos;

  // Open/close transition — fade + a small scale-from-top (the standard dropdown reveal). Created
  // EAGERLY in initState (NOT a lazy `late final =`): an unopened popover would otherwise first
  // touch _anim in dispose() → build a controller mid-teardown → crash.
  // 开关过渡:淡入 + 自顶部微缩放。必须在 initState 急切创建(非懒 late final),否则没开过的浮层会在 dispose 才首次访问→崩。
  late final AnimationController _anim;
  late final CurvedAnimation _scaleCurve; // held so it can be disposed (CurvedAnimation leaks its parent listener otherwise) 持有以便 dispose
  late final Animation<double> _scale;

  @override
  void initState() {
    super.initState();
    _anim = AnimationController(vsync: this, duration: AnMotion.fast);
    _scaleCurve = CurvedAnimation(parent: _anim, curve: AnMotion.easeOut);
    _scale = Tween<double>(begin: 0.96, end: 1).animate(_scaleCurve);
    widget.controller.addListener(_sync);
  }

  @override
  void didUpdateWidget(AnPopover old) {
    super.didUpdateWidget(old);
    if (old.controller != widget.controller) {
      old.controller.removeListener(_sync);
      widget.controller.addListener(_sync);
      _sync(); // new controller may have a different open-state with no pending notification 新 controller 状态可能不同、无待发通知
    }
  }

  void _sync() {
    if (widget.controller.isOpen) {
      if (!_portal.isShowing) {
        _restoreFocus = FocusManager.instance.primaryFocus; // remember the trigger before the scope seizes focus 记开前焦点
        _attachScrollDismiss(); // scrolling an ancestor list dismisses the menu 滚动祖先即关
        _portal.show();
      }
      _anim.forward();
    } else if (_portal.isShowing) {
      // Animate out, then remove the overlay (unless reopened mid-reverse) and hand focus back to the
      // trigger (if it's still mounted) so traversal / SR position survives the close. 反向播完撤浮层 + 归还焦点。
      _anim.reverse().whenComplete(() {
        if (!widget.controller.isOpen && _portal.isShowing) {
          _portal.hide();
          _detachScrollDismiss();
          final restore = _restoreFocus;
          _restoreFocus = null;
          if (restore != null && restore.context != null) restore.requestFocus();
        }
      });
    }
  }

  void _attachScrollDismiss() {
    _detachScrollDismiss();
    _scrollPos = Scrollable.maybeOf(context)?.position; // found from THIS context (under the list) 取自本 context
    _scrollPos?.addListener(widget.controller.close);
  }

  void _detachScrollDismiss() {
    _scrollPos?.removeListener(widget.controller.close);
    _scrollPos = null;
  }

  @override
  void dispose() {
    widget.controller.removeListener(_sync);
    _detachScrollDismiss();
    _scaleCurve.dispose(); // before the parent controller (CurvedAnimation owns a parent listener) 先于父控制器
    _anim.dispose();
    super.dispose();
  }

  // The anchor's current size (for the overlay builder's width-matching). 锚当前尺寸(供浮层宽匹配)。
  Size? _anchorSize() {
    final box = _anchorKey.currentContext?.findRenderObject() as RenderBox?;
    return (box != null && box.hasSize) ? box.size : null;
  }

  // The anchor's rect in the overlay's coordinate space, read at BUILD (allowed — the anchor was laid out a
  // prior frame; reading size during LAYOUT is forbidden). Re-read whenever the overlay child rebuilds. 锚矩形(build 时读)。
  Rect _anchorRectIn(BuildContext overlayContext) {
    final anchorBox = _anchorKey.currentContext?.findRenderObject() as RenderBox?;
    final overlayBox = Overlay.of(overlayContext).context.findRenderObject() as RenderBox?;
    if (anchorBox == null || overlayBox == null || !anchorBox.hasSize || !overlayBox.hasSize) return Rect.zero;
    return anchorBox.localToGlobal(Offset.zero, ancestor: overlayBox) & anchorBox.size;
  }

  @override
  Widget build(BuildContext context) {
    return OverlayPortal(
      controller: _portal,
      overlayChildBuilder: (overlayContext) {
        return Stack(
          children: [
            // Outside-tap barrier (transparent, full-screen). 点外关闭遮罩。
            Positioned.fill(
              child: GestureDetector(behavior: HitTestBehavior.opaque, onTap: widget.controller.close),
            ),
            // Position with a delegate that flips + clamps to stay on-screen (it measures the child first).
            // Anchor rect read at BUILD (size access during layout is forbidden). 经 delegate 翻转+夹取定位。
            Positioned.fill(
              child: CustomSingleChildLayout(
                delegate: _PopoverLayoutDelegate(
                  anchorRect: _anchorRectIn(overlayContext),
                  alignEnd: widget.alignEnd,
                  gap: widget.gap,
                  pad: AnSpace.s8,
                ),
                child: FadeTransition(
                  opacity: _anim,
                  child: ScaleTransition(
                    scale: _scale,
                    alignment: Alignment.topCenter, // grow downward from the top edge 自顶向下展开
                    child: CallbackShortcuts(
                      bindings: {const SingleActivator(LogicalKeyboardKey.escape): widget.controller.close},
                      // FocusScope (not a plain Focus): the overlay is a self-contained focus context — arrow
                      // keys traverse rows, Esc has a target, a descendant autofocus seeds it. 浮层自成焦点域。
                      child: FocusScope(autofocus: true, child: widget.overlayBuilder(overlayContext, _anchorSize())),
                    ),
                  ),
                ),
              ),
            ),
          ],
        );
      },
      child: KeyedSubtree(key: _anchorKey, child: widget.anchor),
    );
  }
}

/// Positions the popover child relative to the anchor, flipping + clamping to stay on-screen — the same
/// pattern as Flutter's DropdownButton / Tooltip layout delegates. Prefers [alignEnd] (child end edge at
/// the anchor end), flips to the other side when it would overflow, and clamps inside the overlay by [pad].
/// Vertically it sits below the anchor by [gap].dy, flipping above when there's no room below.
/// 据锚定位浮层子件,翻转+夹取不出屏(同 Flutter Dropdown/Tooltip delegate)。优先 alignEnd、越界翻转、按 pad 夹入屏内;
/// 竖向在锚下 gap.dy,下方放不下则翻到上方。
class _PopoverLayoutDelegate extends SingleChildLayoutDelegate {
  _PopoverLayoutDelegate({required this.anchorRect, required this.alignEnd, required this.gap, required this.pad});

  final Rect anchorRect; // snapshot read at build (re-read on rebuild, incl. scroll) 锚矩形快照(build 时读、滚动重读)
  final bool alignEnd;
  final Offset gap;
  final double pad;

  @override
  BoxConstraints getConstraintsForChild(BoxConstraints constraints) =>
      constraints.loosen(); // let the child take its own (clamped) size 让子件取自身尺寸

  @override
  Offset getPositionForChild(Size size, Size childSize) {
    // Horizontal: prefer the aligned edge; flip if it overflows that side; then clamp inside [pad, ...].
    double x = (alignEnd ? anchorRect.right - childSize.width : anchorRect.left) + gap.dx;
    if (x < pad) x = anchorRect.left; // end-align overflowed left → start-align 尾对齐越左→首对齐
    if (x + childSize.width > size.width - pad) x = anchorRect.right - childSize.width; // overflowed right → end-align 越右→尾对齐
    final maxX = size.width - childSize.width - pad;
    x = x.clamp(pad, maxX < pad ? pad : maxX);
    // Vertical: below by gap.dy; flip above if it overflows the bottom; then clamp.
    double y = anchorRect.bottom + gap.dy;
    if (y + childSize.height > size.height - pad) {
      final above = anchorRect.top - childSize.height - gap.dy;
      if (above >= pad) y = above; // no room below → flip above 下方放不下→翻上方
    }
    final maxY = size.height - childSize.height - pad;
    y = y.clamp(pad, maxY < pad ? pad : maxY);
    return Offset(x, y);
  }

  @override
  bool shouldRelayout(_PopoverLayoutDelegate old) =>
      old.anchorRect != anchorRect || old.alignEnd != alignEnd || old.gap != gap || old.pad != pad;
}
