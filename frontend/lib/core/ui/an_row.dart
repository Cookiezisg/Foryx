import 'package:flutter/widgets.dart';

import '../design/colors.dart';
import '../design/tokens.dart';
import '../design/typography.dart';
import '../model/status_state.dart';
import 'an_interactive.dart';
import 'an_status_dot.dart';
import 'icons.dart';

/// C1 — the core list row: a three-column grid `[lead | label (1fr) | trail]`, alignment guaranteed by
/// structure (never hand-measured). The lead is a status [dot] OR an [icon] that swaps to a chevron on
/// row-hover (rotating 90° when [open]) for a [collapsible] tree node. The trail stacks [meta] text and
/// the hover-revealed [actions] at the SAME right anchor (opacity cross-fade, no reflow). [hint] makes a
/// taller, top-aligned row whose hint wraps. [selected] tints the row; [emphatic] + selected adds an
/// accent-soft fill + a left accent bar (run boards). [depth] indents per tree level; [mono] sets the
/// label monospace; [passive] is a non-interactive row.
///
/// Interaction: a [collapsible] non-[passive] row toggles on the LEAD (chevron) and selects on the rest;
/// other rows select on tap. The whole row is one [AnInteractive] (hover drives the lead + trail reveal;
/// passive → not focusable). A [collapsible] row announces its `expanded` disclosure state; the keyboard
/// expand/collapse (←/→, WAI-ARIA tree) is owned by the tree CONSUMER's roving focus (AnSidebarList), not
/// AnRow — so AnRow gains no competing keyboard owner.
///
/// C1 列表核心行:三列网格 [lead | label 1fr | trail],对齐靠结构。lead = 状态点 或 icon(collapsible 行 hover 换
/// chevron、open 转 90°)。trail = meta 与 hover 揭示的 actions 叠同一右锚(opacity 互换、不重排)。hint → 行变高顶对齐、
/// hint 换行。selected 提墨;emphatic+selected = accentSoft 底 + 左 accent 条(run 看板)。depth 每级缩进;mono 等宽 label;
/// passive 不可交互。交互:collapsible 非 passive 行点 lead 折叠、点其余选中;其它行点即选。整行一个 AnInteractive
/// (hover 驱动 lead/trail 揭示;passive 不可聚焦;collapsible 行透 expanded 折叠态语义,键盘展开 ←/→ 归树消费方 AnSidebarList、不在 AnRow)。
class AnRow extends StatelessWidget {
  const AnRow({
    this.icon,
    this.dot,
    required this.label,
    this.hint,
    this.meta,
    this.selected = false,
    this.emphatic = false,
    this.collapsible = false,
    this.open = false,
    this.passive = false,
    this.depth = 0,
    this.mono = false,
    this.actions = const [],
    this.onSelect,
    this.onToggle,
    super.key,
  });

  final IconData? icon;
  final AnStatus? dot;
  final String label;
  final String? hint;
  final String? meta;
  final bool selected;

  /// Selected goes accent (accent-soft fill + left accent bar) — for list selection (run boards). 选中走 accent 强调。
  final bool emphatic;
  final bool collapsible;
  final bool open;

  /// Non-interactive (no hover tint, no tap, not focusable). 不可交互。
  final bool passive;

  /// Tree indent level (each adds [AnSize.iconLg]). 树缩进层级。
  final int depth;
  final bool mono;
  final List<Widget> actions;
  final VoidCallback? onSelect;
  final VoidCallback? onToggle;

  bool get _hasHint => hint != null && hint!.isNotEmpty;
  bool get _hasMeta => meta != null && meta!.isNotEmpty;

  @override
  Widget build(BuildContext context) {
    // A collapsible row announces its disclosure state via `expanded` (screen readers say "collapsed/
    // expanded") — null on non-collapsible rows (no false disclosure promise). The KEYBOARD expand/collapse
    // (←/→, WAI-ARIA tree) is owned by the tree CONSUMER's roving-focus group (AnSidebarList), NOT baked
    // into AnRow — so AnRow doesn't grow a competing keyboard owner. collapsible 行透 expanded(屏读播报折叠态);
    // 键盘展开(←/→)归树消费方(AnSidebarList)的 roving 焦点组,不塞进 AnRow(避免双键盘属主)。
    return AnInteractive(
      onTap: passive ? null : onSelect,
      selected: selected,
      expanded: collapsible ? open : null,
      cursor: passive ? MouseCursor.defer : null,
      builder: (context, states) => _row(context, states.isActive && !passive),
    );
  }

  Widget _row(BuildContext context, bool active) {
    final c = context.colors;
    final reduced = AnMotionPref.reduced(context);

    final Color bg;
    if (emphatic && selected) {
      bg = c.accentSoft;
    } else if (selected) {
      bg = c.surfaceActive;
    } else {
      bg = c.surfaceHover.whenActive(active);
    }

    final content = Row(
      crossAxisAlignment: _hasHint ? CrossAxisAlignment.start : CrossAxisAlignment.center,
      children: [
        _lead(c, active, reduced),
        const SizedBox(width: AnSpace.s8),
        Expanded(child: _labelBlock(c, active)),
        const SizedBox(width: AnSpace.s8),
        _trail(c, active),
      ],
    );

    return ClipRRect(
      borderRadius: BorderRadius.circular(AnRadius.button),
      child: AnimatedContainer(
        duration: reduced ? Duration.zero : AnMotion.fast, // hover tint = functional micro-feedback 功能性微反馈
        constraints: BoxConstraints(minHeight: _hasHint ? AnSize.islandHead : AnSize.row),
        color: bg,
        // alignment:center — the minHeight floor makes this Stack taller than a short single-line content;
        // RenderStack's default (topStart) would pin that content to the top (the "text sits high" bug). The
        // positioned accent bar has top+bottom both set → tight full height, unaffected by alignment; a tall
        // hint row (content ≥ minHeight) centres to a zero offset → natural. 居中:补 minHeight 撑高时短内容默认顶对齐。
        child: Stack(
          alignment: Alignment.center,
          children: [
            Padding(
              padding: EdgeInsetsDirectional.only(
                start: AnSpace.s8 + depth * AnSize.iconLg, // pad-row + per-level indent 缩进
                end: AnSpace.s8,
                top: _hasHint ? AnSpace.s4 : 0,
                bottom: _hasHint ? AnSpace.s4 : 0,
              ),
              child: content,
            ),
            // emphatic+selected: a left SOLID accent bar (demo `inset --line-2 0 0 var(--accent)` —
            // solid accent, NOT the faint accentLine that AnCard's selected border uses). 左实心 accent 条(非淡线)。
            if (emphatic && selected)
              PositionedDirectional(
                start: 0,
                top: 0,
                bottom: 0,
                child: Container(width: AnSize.gripLine, color: c.accent),
              ),
          ],
        ),
      ),
    );
  }

  Widget _lead(AnColors c, bool active, bool reduced) {
    final Widget glyph;
    if (dot != null) {
      glyph = ExcludeSemantics(child: AnStatusDot(dot!)); // status conveyed by the dot's own a11y elsewhere; here decorative 装饰
    } else if (collapsible) {
      // icon ↔ chevron swap on hover; chevron rotates 90° when open. icon↔chevron 互换 + 旋转。
      glyph = _HoverSwap(
        alignment: Alignment.center,
        showSecond: active,
        first: icon != null ? Icon(icon, size: AnSize.icon, color: c.inkFaint) : const SizedBox.shrink(),
        second: AnimatedRotation(
          duration: reduced ? Duration.zero : AnMotion.mid,
          curve: AnMotion.spring, // demo --ease-spring (transform animates; opacity swap is instant) 旋转弹性、opacity 即时
          turns: open ? 0.25 : 0,
          child: Icon(AnIcons.chevronRight, size: AnSize.icon, color: c.inkFaint),
        ),
      );
    } else {
      glyph = icon != null
          ? ExcludeSemantics(child: Icon(icon, size: AnSize.icon, color: c.inkFaint))
          : const SizedBox.shrink();
    }

    final lead = SizedBox(width: AnSize.icon, height: AnSize.icon, child: Center(child: glyph));
    // collapsible non-passive: the lead toggles; other taps fall through to the row's select. lead 折叠、其余选中。
    if (collapsible && !passive && onToggle != null) {
      return GestureDetector(behavior: HitTestBehavior.opaque, onTap: onToggle, child: lead);
    }
    return lead;
  }

  Widget _labelBlock(AnColors c, bool active) {
    final strong = active || selected;
    final labelStyle = (mono ? AnText.mono : AnText.body).copyWith(color: strong ? c.ink : c.inkMuted);
    final labelText = Text(label, maxLines: 1, softWrap: false, overflow: TextOverflow.ellipsis, style: labelStyle);
    if (!_hasHint) return labelText;
    return Column(
      mainAxisSize: MainAxisSize.min,
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text(label, maxLines: 1, softWrap: false, overflow: TextOverflow.ellipsis,
            style: (mono ? AnText.mono : AnText.body).copyWith(color: c.ink)), // hint row: label is full-ink hint 行 label 强墨
        const SizedBox(height: AnSpace.s2),
        Text(hint!, softWrap: true, style: AnText.meta.copyWith(color: c.inkFaint)), // hint wraps 多行
      ],
    );
  }

  Widget _trail(AnColors c, bool active) {
    final metaWidget = _hasMeta
        ? Text(meta!, maxLines: 1, overflow: TextOverflow.ellipsis, textAlign: TextAlign.right,
            style: AnText.meta.copyWith(color: c.inkFaint, fontFeatures: const [FontFeature.tabularFigures()]))
        : null;
    if (actions.isEmpty) return metaWidget ?? const SizedBox.shrink();
    // meta ↔ actions at the same right anchor; actions revealed on hover (opacity cross-fade, no reflow).
    // meta↔actions 同右锚;hover 揭示 actions(opacity 交叉、不重排)。
    return _HoverSwap(
      alignment: Alignment.centerRight,
      showSecond: active,
      first: metaWidget ?? const SizedBox.shrink(),
      second: Row(mainAxisSize: MainAxisSize.min, children: actions),
    );
  }
}

/// Two layers at one anchor, swapped by [showSecond] with NO reflow (both always laid out, so the slot
/// width = max(first, second)). The opacity swap is INSTANT (demo: "opacity 互换即时,不入过渡" — only the
/// chevron's ROTATION animates, handled by the caller). The hidden layer is made fully inert:
/// [IgnorePointer] (opacity 0 doesn't stop hit-testing) + [ExcludeSemantics] (nor screen-reader) +
/// [ExcludeFocus] (nor keyboard traversal — else an invisible action button becomes a focus trap). Used
/// for AnRow's lead (icon↔chevron) + trail (meta↔actions).
///
/// 同位两层即时互换不重排(两层常驻、槽宽=max)。opacity 即时(demo 明示不入过渡;只 chevron 旋转动画、调用方管)。
/// 隐藏层彻底惰化:IgnorePointer(opacity0 不自挡命中)+ ExcludeSemantics(屏读)+ ExcludeFocus(键盘遍历,
/// 否则不可见动作钮成焦点陷阱)。AnRow 的 lead(icon↔chevron)+ trail(meta↔actions)复用。
class _HoverSwap extends StatelessWidget {
  const _HoverSwap({
    required this.alignment,
    required this.showSecond,
    required this.first,
    required this.second,
  });

  final AlignmentGeometry alignment;
  final bool showSecond;
  final Widget first;
  final Widget second;

  @override
  Widget build(BuildContext context) {
    return Stack(
      alignment: alignment,
      children: [
        _layer(first, visible: !showSecond),
        _layer(second, visible: showSecond),
      ],
    );
  }

  Widget _layer(Widget child, {required bool visible}) {
    if (visible) return child;
    // Hidden but still occupying its layout slot (no reflow on swap) — and fully inert. 隐藏但占位、彻底惰化。
    return ExcludeFocus(
      child: IgnorePointer(
        child: ExcludeSemantics(
          child: Opacity(opacity: 0, child: child),
        ),
      ),
    );
  }
}
