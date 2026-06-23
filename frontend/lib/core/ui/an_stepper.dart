import 'package:flutter/widgets.dart';

import '../../i18n/strings.g.dart';
import '../design/colors.dart';
import '../design/tokens.dart';
import '../design/typography.dart';
import 'an_interactive.dart';
import 'icons.dart';

/// C3 — a step-progress indicator: a row of nodes showing done / current / upcoming for a [count]-step
/// flow at the 1-based [current] step. HAND-ROLL (a Row of AnimatedContainers; the step-progress
/// packages ship their own theme system + ZERO Semantics, the only hard part). Stateless — a stepper
/// advances DISCRETELY, so there is NO repeating motion (a breath pulse would violate 动效克制); the
/// only motion is an implicit AnimatedContainer on the current-step change, frozen under reduced.
///
/// Three DISTINCT treatments per node (never colour alone): done = accent (+ a check glyph in
/// [numbered]), current = accent emphasis (an elongated dot / filled circle), upcoming = a faint
/// line/outline. One [Semantics] reads "<label>. Step N of M" as a polite live region; the decorative
/// nodes are excluded. When [onStepTap] is set, COMPLETED nodes become AnInteractive (a soft focus/
/// hover ring + Enter/Space + a "go to step N" button label); current/upcoming stay non-focusable.
///
/// C3——步骤进度:一排节点显 done/current/upcoming(1-based current)。HAND-ROLL。Stateless,离散推进、
/// 无循环动效(breath 违背克制),仅当前步变化有隐式 AnimatedContainer、降级冻结。三态各异(不靠色单独):
/// done=accent(numbered 带 check)、current=accent 强调、upcoming=淡线。一个 Semantics 播报「<label>. 第N/共M」,
/// 装饰节点排除;onStepTap 时已完成节点变 AnInteractive(柔焦点/悬停环 + Enter/Space + 「跳到第N步」标签)。
enum AnStepperVariant { dots, numbered }

enum _Status { done, current, upcoming }

class AnStepper extends StatelessWidget {
  const AnStepper({
    required this.count,
    required this.current,
    this.variant = AnStepperVariant.dots,
    this.labels,
    this.onStepTap,
    this.semanticLabel,
    super.key,
  });

  /// Total steps M. 总步数。
  final int count;

  /// The 1-based current step (count+1 = all done). 当前步(1-based;count+1=全完成)。
  final int current;

  final AnStepperVariant variant;

  /// Optional short label under each node (uses labels[i-1] when present). 每步短标签(可选)。
  final List<String>? labels;

  /// Tap a COMPLETED step to jump back (1-based). null = pure indicator. 点已完成步跳回。
  final void Function(int step)? onStepTap;

  /// Process name for the a11y value. 流程名(无障碍)。
  final String? semanticLabel;

  _Status _statusOf(int i) => i < current
      ? _Status.done
      : i == current
          ? _Status.current
          : _Status.upcoming;

  @override
  Widget build(BuildContext context) {
    if (count <= 0) return const SizedBox.shrink();
    final dur = AnMotionPref.reduced(context) ? Duration.zero : AnMotion.mid;

    return Semantics(
      container: true,
      liveRegion: true,
      label: semanticLabel,
      value: context.t.feedback.stepOf(n: current.clamp(1, count), m: count),
      child: Row(
        mainAxisSize: MainAxisSize.min,
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          for (var i = 1; i <= count; i++) ...[
            if (i > 1) const SizedBox(width: AnSpace.s6),
            _node(context, i, _statusOf(i), dur),
          ],
        ],
      ),
    );
  }

  Widget _node(BuildContext context, int i, _Status status, Duration dur) {
    final c = context.colors;
    final tappable = onStepTap != null && status == _Status.done;

    final Widget node = tappable
        ? MergeSemantics(
            child: Semantics(
              label: context.t.feedback.goToStep(n: i),
              child: AnInteractive(
                onTap: () => onStepTap!(i),
                builder: (ctx, states) => _dot(ctx, i, status, dur, active: states.isActive),
              ),
            ),
          )
        : ExcludeSemantics(child: _dot(context, i, status, dur, active: false));

    final label = labels != null && i <= labels!.length ? labels![i - 1] : null;
    if (label == null) return node;
    return Column(
      mainAxisSize: MainAxisSize.min,
      children: [
        node,
        const SizedBox(height: AnSpace.s6),
        ExcludeSemantics(
          child: Text(label, style: AnText.meta.copyWith(color: status == _Status.upcoming ? c.inkFaint : c.ink)),
        ),
      ],
    );
  }

  Widget _dot(BuildContext context, int i, _Status status, Duration dur, {required bool active}) {
    final c = context.colors;
    // A soft accent ring marks a tappable node that's keyboard-focused / hovered (visible on either
    // bg colour, unlike a same-colour border). 柔色环标记可点节点的聚焦/悬停。
    final ring = active ? [BoxShadow(color: c.accentSoft, spreadRadius: AnSpace.s4)] : const <BoxShadow>[];

    if (variant == AnStepperVariant.numbered) {
      final done = status == _Status.done;
      final upcoming = status == _Status.upcoming;
      return AnimatedContainer(
        duration: dur,
        curve: AnMotion.easeOut,
        width: AnSize.badge,
        height: AnSize.badge,
        alignment: Alignment.center,
        decoration: BoxDecoration(
          color: upcoming ? c.surface : c.accent,
          shape: BoxShape.circle,
          border: Border.all(color: upcoming ? c.line : c.accent, width: AnSize.hairline),
          boxShadow: ring,
        ),
        child: done
            ? Icon(AnIcons.check, size: AnSize.iconSm, color: c.onAccent)
            : Text('$i',
                style: AnText.meta.copyWith(
                    color: upcoming ? c.inkFaint : c.onAccent, fontFeatures: const [FontFeature.tabularFigures()])),
      );
    }

    // dots: done = accent dot, current = elongated accent pill, upcoming = faint line dot.
    final isCurrent = status == _Status.current;
    return Container(
      // a constant-height hit/centre box so dots align with the taller `current` pill and stay tappable
      // 定高居中盒:小圆点与拉长的 current 对齐、且有可点区
      height: AnSize.badge,
      alignment: Alignment.center,
      child: AnimatedContainer(
        duration: dur,
        curve: AnMotion.easeOut,
        width: isCurrent ? AnSize.stepCurrent : AnSize.dot,
        height: AnSize.dot,
        decoration: BoxDecoration(
          color: status == _Status.upcoming ? c.line : c.accent,
          borderRadius: BorderRadius.circular(AnRadius.pill),
          boxShadow: ring,
        ),
      ),
    );
  }
}
