import 'package:flutter/widgets.dart';

import '../design/tokens.dart';

/// The kit's single collapse / expand reveal: a controller-driven [ClipRect] + [Align] height-factor tween
/// (the ExpansionTile idiom) that grows DOWNWARD only and is gated to instant under reduced motion.
/// [AnRowDetail]'s detail panel and [AnSidebarList]'s group / type / branch children both route through it
/// so the disclosure motion is byte-identical kit-wide (not re-rolled per site — #8).
///
/// NOT [AnimatedSize]: AnimatedSize re-dirties itself during its own performLayout when the child resizes,
/// which ASSERTS when reveals are NESTED (a group containing a type containing a branch — the sidebar's
/// shape). ClipRect + Align(heightFactor) driven by an explicit controller has no such re-dirty, so it nests
/// safely. [child] shows when [open], else it collapses to zero height AND is removed from the tree once fully
/// closed (so collapsed rows aren't focusable / screen-reader-announced). Pass [duration] = `Duration.zero`
/// to force-skip the tween (e.g. a filter query is driving the open state — per-keystroke tweens are janky).
///
/// 套件统一折叠/展开揭示:控制器驱动的 ClipRect + Align(heightFactor)(ExpansionTile 习语),仅向下,reduced 即时。
/// **非 AnimatedSize**(后者在嵌套时会 performLayout 内自脏断言——sidebar 是嵌套树);ClipRect+Align 可安全嵌套。
/// open 显 child,否则补间到 0 高、全收后从树移除(收起的行不可聚焦/不被屏读)。duration=Duration.zero 强制即时。
class AnExpandReveal extends StatefulWidget {
  const AnExpandReveal({required this.open, required this.child, this.duration, super.key});

  final bool open;
  final Widget child;

  /// Override the reveal duration. `Duration.zero` → instant (e.g. filter-forced open). Default = [AnMotion.mid]
  /// (→ instant under reduced motion). 覆写时长,zero=即时(如过滤强制展开),默认 mid(reduced 即时)。
  final Duration? duration;

  @override
  State<AnExpandReveal> createState() => _AnExpandRevealState();
}

class _AnExpandRevealState extends State<AnExpandReveal> with SingleTickerProviderStateMixin {
  late final AnimationController _ctl;
  late final CurvedAnimation _factor; // CurvedAnimation (not Animation) so it can be disposed 须具体类型以便 dispose

  @override
  void initState() {
    super.initState();
    _ctl = AnimationController(vsync: this, duration: AnMotion.mid, value: widget.open ? 1 : 0);
    _factor = CurvedAnimation(parent: _ctl, curve: AnMotion.easeOut);
  }

  @override
  void didUpdateWidget(AnExpandReveal old) {
    super.didUpdateWidget(old);
    if (old.open != widget.open) {
      // instant when reduced motion OR the caller forces it (filter-driven open) 即时:reduced 或调用方强制
      if (widget.duration == Duration.zero || AnMotionPref.reduced(context)) {
        _ctl.value = widget.open ? 1 : 0;
      } else {
        _ctl.duration = widget.duration ?? AnMotion.mid;
        widget.open ? _ctl.forward() : _ctl.reverse();
      }
    }
  }

  @override
  void dispose() {
    _factor.dispose(); // before the parent controller (CurvedAnimation owns a parent listener) 先于父控制器
    _ctl.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return AnimatedBuilder(
      animation: _factor,
      // child built once (not per frame); only inserted into the tree while opening / open / closing. 子件建一次。
      child: widget.child,
      builder: (context, child) {
        // fully closed → take no space AND drop the subtree (collapsed rows must not be focusable). 全收:不占位、移出树。
        if (_ctl.value == 0 && !widget.open) return const SizedBox.shrink();
        return ClipRect(
          child: Align(
            alignment: Alignment.topCenter, // grow downward only 仅向下
            heightFactor: _factor.value.clamp(0.0, 1.0),
            child: child,
          ),
        );
      },
    );
  }
}
