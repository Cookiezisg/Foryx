import 'package:flutter/widgets.dart';

import '../design/colors.dart';
import '../design/tokens.dart';
import 'an_interactive.dart';

/// The kit's single standard for a floating "pick one from a list" popover panel — shared by [AnMenu]
/// (command / option menu) and [AnDropdown] (single-select). One source so every such popover reads
/// identically: white [AnColors.surface] panel, hairline border, [AnRadius.chip] corner, pop shadow, and
/// **`s4` padding on ALL sides** so each row's hover/selected pill floats INSET from the panel edge (never
/// edge-to-edge). The caller wraps it in its own [ConstrainedBox] (the menu content-hugs via [IntrinsicWidth];
/// the dropdown matches its trigger width) — width policy is per-consumer, the chrome is shared.
///
/// 浮层「列表单选」面板的统一标准——AnMenu(命令/选项)与 AnDropdown(单选)共用。单源:白面 + 发丝边 + chip 圆角 + 浮影,
/// **四周 s4 内距**故每行 hover/选中药丸内缩悬浮(不贴边)。宽策略由调用方定(菜单 IntrinsicWidth 贴内容、下拉跟触发器宽),壳共享。
class AnMenuSurface extends StatelessWidget {
  const AnMenuSurface({required this.children, super.key});

  final List<Widget> children;

  @override
  Widget build(BuildContext context) {
    final c = context.colors;
    return DecoratedBox(
      decoration: BoxDecoration(
        color: c.surface,
        borderRadius: BorderRadius.circular(AnRadius.chip),
        border: Border.all(color: c.line, width: AnSize.hairline),
        boxShadow: c.shadowPop,
      ),
      child: ClipRRect(
        borderRadius: BorderRadius.circular(AnRadius.chip),
        child: SingleChildScrollView(
          padding: const EdgeInsets.all(AnSpace.s4), // s4 ALL sides → rows inset, pill floats off the edge 四周 s4、药丸不贴边
          child: FocusTraversalGroup(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.stretch,
              mainAxisSize: MainAxisSize.min,
              children: children,
            ),
          ),
        ),
      ),
    );
  }
}

/// The kit's single standard for ONE selectable popover row — shared by [AnMenu] items + [AnDropdown]
/// options. Owns the row chrome so the hover / press / focus and selected feel are byte-identical: a
/// [AnSize.row]-tall [AnInteractive] with the **rounded inset pill** ([AnRadius.button]) that fades in on
/// hover/active ([AnColors.surfaceHover], or [AnColors.dangerSoft] when [danger]) via the alpha-0
/// `whenActive` idiom, reduced-motion gated, and dimmed + inert when not [enabled]. The CONTENT (lead /
/// label / meta / check arrangement) is the caller's [builder] (it differs: a menu puts the check in the
/// lead, a dropdown in the trailing) — only the row surface is standardised. [active] is handed to the
/// builder so it can brighten its own foreground.
///
/// 浮层可选行的统一标准——AnMenu 项 + AnDropdown 选项共用。统一行壳(hover/press/focus/选中手感一致):row 高的
/// AnInteractive + **圆角内缩药丸**,hover/active 经 alpha-0 whenActive 淡入(danger 用 dangerSoft),reduced 门控,
/// 禁用变暗惰化。内容(前导/标签/meta/勾的排布)由调用方 builder(菜单勾在前导、下拉勾在尾,各异),仅行壳标准化。
class AnMenuRow extends StatelessWidget {
  const AnMenuRow({
    required this.onTap,
    required this.builder,
    this.enabled = true,
    this.danger = false,
    this.autofocus = false,
    super.key,
  });

  final VoidCallback? onTap;

  /// Builds the row content; [active] = hover / press / focus, so the builder can brighten its foreground.
  /// 建行内容;active=hover/press/focus,供 builder 提亮前景。
  final Widget Function(BuildContext context, bool active) builder;

  final bool enabled;

  /// Danger flavour — the hover/active fill uses [AnColors.dangerSoft] (the caller reds its text). 危险风味。
  final bool danger;
  final bool autofocus;

  @override
  Widget build(BuildContext context) {
    return AnInteractive(
      enabled: enabled,
      autofocus: autofocus,
      onTap: onTap,
      builder: (context, states) {
        final c = context.colors;
        final active = states.isActive;
        final reduced = AnMotionPref.reduced(context);
        // resting fill = same hue at alpha 0 (whenActive) → no dark-midpoint flash on the fade 静止底走 alpha-0 单源
        final bg = danger ? c.dangerSoft.whenActive(active) : c.surfaceHover.whenActive(active);
        return Opacity(
          opacity: enabled ? 1 : AnOpacity.disabled,
          child: AnimatedContainer(
            duration: reduced ? Duration.zero : AnMotion.fast, // hover tint = functional micro-feedback 功能性微反馈
            height: AnSize.row,
            padding: const EdgeInsets.symmetric(horizontal: AnSpace.s8),
            decoration: BoxDecoration(color: bg, borderRadius: BorderRadius.circular(AnRadius.button)),
            child: builder(context, active),
          ),
        );
      },
    );
  }
}
