import 'package:flutter/material.dart';

import '../design/colors.dart';
import '../design/tokens.dart';
import 'an_island.dart';
import 'an_window_controls.dart';

/// The three-island desktop shell skeleton: a left island ([sidebar]), the open ocean
/// ([ocean]) — the window's white surface, no card — and a right island ([inspector]). 8px
/// padding around + 8px gaps between. The LEFT island is drag-resizable (240–400, default 320);
/// the RIGHT island is FIXED (320). The ocean is the flex remainder; its content column is
/// elastic 480–720, and the window minimum guarantees the ocean never drops below 480 even with
/// the left island at its max. The left island carries the chrome bar (the macOS traffic lights
/// are centered by the OS in the taller title bar — see window_setup).
///
/// 三岛桌面 shell 骨架:左岛([sidebar],可拖 240–400 默认 320)· 敞开海洋([ocean],窗体白面无卡,
/// 内容列弹性 480–720)· 右岛([inspector],固定 320)。四周 8px + 岛间 8px。窗口最小尺寸保证即便左岛
/// 拖到最大、海洋仍 ≥ 480。左岛顶含 chrome 条(红绿灯由 OS 在加高标题栏居中,见 window_setup)。
class AnShell extends StatefulWidget {
  const AnShell({super.key, this.sidebar, this.ocean, this.inspector, this.inspectorOpen = true});

  final Widget? sidebar;
  final Widget? ocean;
  final Widget? inspector;

  /// Reveal / hide the right island (a feature opens it for a selected entity, closes it otherwise). It
  /// slides in/out (width + gap animate 0↔[AnSize.rightIsland]); the content is held full-width so it
  /// doesn't reflow during the slide, and the island is the SAME [AnIsland] the left island uses with its
  /// float shadow LEFT UNCLIPPED when open (so both islands' shadows match). Default true (shown).
  /// 右岛揭示/收起(滑入滑出、内容不重排);与左岛同一 AnIsland,敞开态阴影不裁、两岛阴影一致。
  final bool inspectorOpen;

  @override
  State<AnShell> createState() => _AnShellState();
}

class _AnShellState extends State<AnShell> {
  double _leftW = AnSize.sidebar; // default 320, drag-clamped to [sidebarMin, sidebarMax]

  @override
  Widget build(BuildContext context) {
    final c = context.colors;
    // The ocean reads as the window's surface; Material gives text a default style (no debug
    // underlines). 海洋即窗体白面;Material 给文本默认样式(无调试下划线)。
    return Material(
      color: c.surface,
      child: Padding(
        padding: const EdgeInsets.all(AnSize.shellPad),
        child: Row(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            SizedBox(
              width: _leftW,
              child: AnIsland(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.stretch,
                  children: [
                    const _ChromeBar(),
                    const SizedBox(height: AnSpace.s8),
                    Expanded(child: widget.sidebar ?? const _Placeholder('Sidebar')),
                  ],
                ),
              ),
            ),
            // Left island is draggable → the grip resizes it (and serves as the 8px gap).
            // 左岛可拖 → grip 调宽(兼作 8px 间距)。
            _Grip(
              key: const ValueKey('anShellLeftGrip'),
              onDrag: (dx) => setState(
                  () => _leftW = (_leftW + dx).clamp(AnSize.sidebarMin, AnSize.sidebarMax)),
            ),
            Expanded(child: widget.ocean ?? const _Placeholder('Ocean')),
            // Right island REVEAL: the gap + island width animate 0↔320. _RightReveal wraps the content in
            // the SAME [AnIsland] the left island uses (one shadow source) and leaves it UNCLIPPED when open
            // so its float shadow is intact + identical to the left's. 右岛揭示:与左岛同一 AnIsland,敞开态不裁、阴影同源。
            _RightReveal(
              open: widget.inspectorOpen,
              child: widget.inspector ?? const _Placeholder('Inspector'),
            ),
          ],
        ),
      ),
    );
  }
}

/// The left island's top control strip. Reserves the macOS traffic-light zone at the leading
/// edge (the OS draws the real lights there, centered in the taller title bar). Action buttons
/// (collapse / search) land here once the UI kit ships. 左岛顶栏:行首留红绿灯位(OS 在加高标题栏
/// 画真灯居中);收起/搜索钮待 UI 套件落地后接入。
class _ChromeBar extends StatelessWidget {
  const _ChromeBar();

  @override
  Widget build(BuildContext context) {
    return const SizedBox(
      height: AnSize.row,
      child: Row(children: [AnWindowControls(), Spacer()]),
    );
  }
}

/// Animated reveal / hide of the fixed-width right island. The leading gap + island width animate
/// 0↔[AnSize.rightIsland]; an [OverflowBox] holds the content at full width so it slides rather than
/// reflows. The island is the SAME [AnIsland] the left island uses, so its float shadow is one source —
/// and the slot is CLIPPED ONLY WHILE SLIDING: in the open steady state there is no clip, so the shadow
/// is intact + identical to the left island's (an always-on ClipRect was cutting it into a "pointy" dead
/// corner). reduced-motion → instant. Collapsed content is held full-width but made fully inert
/// (ExcludeFocus + ExcludeSemantics + IgnorePointer) so it isn't a focus trap behind the clip.
///
/// 右岛揭示:间距 + 宽动画 0↔320,OverflowBox 保持满宽(滑入不重排)。岛=与左岛同一 [AnIsland](阴影同源),
/// **仅滑动中裁切**——敞开态不裁,故阴影完整、与左岛一致(原 always-on ClipRect 把阴影裁成死角尖尖)。reduced→即时;
/// 收起态满宽但彻底惰化(ExcludeFocus+ExcludeSemantics+IgnorePointer),不成焦点陷阱。
class _RightReveal extends StatefulWidget {
  const _RightReveal({required this.open, required this.child});
  final bool open;
  final Widget child;

  @override
  State<_RightReveal> createState() => _RightRevealState();
}

class _RightRevealState extends State<_RightReveal> with SingleTickerProviderStateMixin {
  late final AnimationController _ctl;

  @override
  void initState() {
    super.initState();
    _ctl = AnimationController(vsync: this, duration: AnMotion.mid, value: widget.open ? 1 : 0);
  }

  @override
  void didUpdateWidget(_RightReveal old) {
    super.didUpdateWidget(old);
    if (old.open != widget.open) {
      if (AnMotionPref.reduced(context)) {
        _ctl.value = widget.open ? 1 : 0;
      } else {
        widget.open ? _ctl.forward() : _ctl.reverse();
      }
    }
  }

  @override
  void dispose() {
    _ctl.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    // The island is the SAME primitive the left side uses (one shadow source). Built once (stable across
    // animation frames). Collapsed → held full-width but fully inert (clip hides paint, not focus/SR).
    // 岛=左岛同一原语(阴影同源);收起态满宽但彻底惰化。
    final island = AnIsland(
      child: widget.open
          ? widget.child
          : ExcludeFocus(child: ExcludeSemantics(child: IgnorePointer(child: widget.child))),
    );
    return AnimatedBuilder(
      animation: _ctl,
      builder: (context, _) {
        final t = _ctl.value;
        if (t == 0) return const SizedBox.shrink(); // fully closed: take no space 全收:不占位
        final fullyOpen = t >= 1.0;
        final slot = SizedBox(
          width: AnSize.rightIsland * t,
          child: OverflowBox(
            minWidth: AnSize.rightIsland,
            maxWidth: AnSize.rightIsland,
            alignment: AlignmentDirectional.centerEnd, // island docks right; reveals leftward 右锚揭示
            child: SizedBox(width: AnSize.rightIsland, child: island),
          ),
        );
        return Row(
          mainAxisSize: MainAxisSize.min,
          children: [
            SizedBox(width: AnSize.shellGap * t), // gap animates with the reveal 间距随揭示
            // Clip ONLY while sliding; fully open → an effectively-unbounded clip so the shadow shows (the
            // ClipRect widget stays in the tree to keep the island's element identity stable). 仅滑动中裁,敞开态不裁。
            ClipRect(clipper: fullyOpen ? const _UnclippedRect() : null, child: slot),
          ],
        );
      },
    );
  }
}

/// A no-op [CustomClipper] — returns an effectively-unbounded rect so a [ClipRect] in the tree clips
/// nothing (lets the open island's float shadow paint past its bounds while keeping the widget stable).
/// 不裁切的 clipper(返回极大矩形):保留 ClipRect 在树中但不裁,放行敞开岛的阴影。
class _UnclippedRect extends CustomClipper<Rect> {
  const _UnclippedRect();
  @override
  Rect getClip(Size size) => const Rect.fromLTRB(-1e5, -1e5, 1e5, 1e5);
  @override
  bool shouldReclip(CustomClipper<Rect> oldClipper) => false;
}

/// Skeleton placeholder — a faint centered label so each empty region is identifiable. Replaced
/// by real feature content. 骨架占位:淡色居中标签,标识空区;真内容落地后替换。
class _Placeholder extends StatelessWidget {
  const _Placeholder(this.label);
  final String label;

  @override
  Widget build(BuildContext context) {
    final c = context.colors;
    return Center(
      child: Text(label, style: TextStyle(color: c.inkFaint, fontSize: AnSize.iconSm)),
    );
  }
}

/// The drag handle between the left island and the ocean — also serves as the 8px gap. Shows a
/// hairline on hover. 左岛与海洋间的拖拽柄,兼作 8px 间距;悬停现细线。
class _Grip extends StatefulWidget {
  const _Grip({super.key, required this.onDrag});
  final ValueChanged<double> onDrag;
  @override
  State<_Grip> createState() => _GripState();
}

class _GripState extends State<_Grip> {
  bool _hover = false;
  @override
  Widget build(BuildContext context) {
    final c = context.colors;
    final reduced = AnMotionPref.reduced(context);
    return MouseRegion(
      cursor: SystemMouseCursors.resizeColumn,
      onEnter: (_) => setState(() => _hover = true),
      onExit: (_) => setState(() => _hover = false),
      child: GestureDetector(
        behavior: HitTestBehavior.opaque,
        onHorizontalDragUpdate: (d) => widget.onDrag(d.delta.dx),
        child: SizedBox(
          width: AnSize.shellGap,
          child: Center(
            child: AnimatedContainer(
              duration: reduced ? Duration.zero : AnMotion.fast, // hover hairline = functional micro-feedback 功能性微反馈
              width: AnSize.gripLine,
              decoration: BoxDecoration(
                color: c.lineStrong.whenActive(_hover), // no-flash fade 无暗闪淡入
                borderRadius: BorderRadius.circular(AnRadius.pill),
              ),
            ),
          ),
        ),
      ),
    );
  }
}
