import 'package:flutter/services.dart';
import 'package:flutter/widgets.dart';

/// The interaction substrate every actionable surface composes — buttons, rows, chips, tabs all
/// build on this one place so hover / focus / pressed / disabled behave identically everywhere.
/// Built on the framework's [FocusableActionDetector] (principle #8 — standard API over hand-rolled
/// MouseRegion/Focus/key handling): FAD drives hover + focus via the platform highlight mode (so the
/// focus ring shows on KEYBOARD focus, not on a mouse click) and nulls them when disabled; Enter/
/// Space activate through the standard [ActivateIntent]; we keep only the pressed tracking (a
/// GestureDetector) and the [builder]'s live [WidgetState] set. Disabled = non-focusable, inert.
///
/// 可交互基座——按钮/行/chip/tab 都搭在这一处,hover/focus/pressed/disabled 全局一致。搭在框架的
/// FocusableActionDetector 上(原则 #8:用标准 API 而非手搓):FAD 按平台高亮模式驱动 hover/focus(焦点环只在
/// 键盘聚焦时显、点击不显)并在禁用时清零;Enter/Space 走标准 ActivateIntent;我们只留 pressed 跟踪 + 态集。
class AnInteractive extends StatefulWidget {
  const AnInteractive({
    required this.builder,
    this.onTap,
    this.enabled = true,
    this.selected = false,
    this.focusNode,
    this.autofocus = false,
    this.cursor,
    super.key,
  });

  /// Paints the surface for the current interaction state. 据交互态绘制表面。
  final Widget Function(BuildContext context, Set<WidgetState> states) builder;

  /// Activation callback. When null the surface is inert (no click cursor, not focusable).
  /// 激活回调;为 null 则惰性(无点击光标、不可聚焦)。
  final VoidCallback? onTap;
  final bool enabled;

  /// Caller-driven selected state (surfaced as [WidgetState.selected]). 调用方驱动的选中态。
  final bool selected;
  final FocusNode? focusNode;
  final bool autofocus;

  /// Cursor override; defaults to a click cursor when activatable. 光标覆盖;可激活时默认 click。
  final MouseCursor? cursor;

  @override
  State<AnInteractive> createState() => _AnInteractiveState();
}

class _AnInteractiveState extends State<AnInteractive> {
  bool _hovered = false;
  bool _focused = false;
  bool _pressed = false;

  bool get _canActivate => widget.enabled && widget.onTap != null;

  Set<WidgetState> get _states => {
        if (!widget.enabled) WidgetState.disabled,
        if (widget.selected) WidgetState.selected,
        if (widget.enabled && _hovered) WidgetState.hovered,
        if (widget.enabled && _focused) WidgetState.focused,
        if (widget.enabled && _pressed) WidgetState.pressed,
      };

  void _set(VoidCallback f) {
    if (mounted) setState(f);
  }

  @override
  void didUpdateWidget(AnInteractive old) {
    super.didUpdateWidget(old);
    // FAD stops tracking when disabled but won't fire a hover/focus-off if the pointer leaves while
    // disabled — so a control disabled mid-hover would re-enable stuck "hovered". Clear on disable.
    // FAD 禁用时停止跟踪,但禁用期间指针移开不会回调 → 重新启用会卡在 hover。禁用时清零。
    if (old.enabled && !widget.enabled && (_hovered || _focused || _pressed)) {
      _hovered = false;
      _focused = false;
      _pressed = false;
    }
  }

  void _activate() => widget.onTap?.call();

  @override
  Widget build(BuildContext context) {
    final canActivate = _canActivate;

    // Pressed is the one state FAD doesn't track — keep a GestureDetector for it + the tap. pressed 自管。
    Widget result = GestureDetector(
      behavior: HitTestBehavior.opaque,
      onTap: canActivate ? widget.onTap : null,
      onTapDown: canActivate ? (_) => _set(() => _pressed = true) : null,
      onTapUp: canActivate ? (_) => _set(() => _pressed = false) : null,
      onTapCancel: canActivate ? () => _set(() => _pressed = false) : null,
      child: widget.builder(context, _states),
    );

    return FocusableActionDetector(
      enabled: canActivate,
      focusNode: widget.focusNode,
      autofocus: widget.autofocus,
      mouseCursor: canActivate
          ? (widget.cursor ?? SystemMouseCursors.click)
          : (widget.cursor ?? MouseCursor.defer),
      shortcuts: const <ShortcutActivator, Intent>{
        SingleActivator(LogicalKeyboardKey.enter): ActivateIntent(),
        SingleActivator(LogicalKeyboardKey.space): ActivateIntent(),
      },
      actions: <Type, Action<Intent>>{
        ActivateIntent: CallbackAction<ActivateIntent>(onInvoke: (_) {
          _activate();
          return null;
        }),
      },
      onShowHoverHighlight: (h) => _set(() => _hovered = h),
      onShowFocusHighlight: (f) => _set(() => _focused = f),
      child: Semantics(
        button: widget.onTap != null,
        enabled: widget.enabled,
        selected: widget.selected,
        child: result,
      ),
    );
  }
}
