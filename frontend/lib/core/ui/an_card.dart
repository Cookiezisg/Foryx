import 'package:flutter/widgets.dart';

import '../design/colors.dart';
import '../design/tokens.dart';
import 'an_interactive.dart';

/// Card emphasis variant. 卡片强调变体。
enum AnCardVariant {
  /// Neutral hairline border. 中性海岸线描边。
  normal,

  /// Accent border (an editing / focused config card). accent 描边(编辑/聚焦卡)。
  accent,
}

/// Card inner padding. 卡片内距。
enum AnCardPad { normal, tight }

/// The bordered card container (the "edged" dual of borderless AnInfoCard). A hairline border (drawn
/// inside the box so the rounded corners stay clean, no outset corner tips) + [AnRadius.chip] + surface
/// fill + padding; the caller composes the [child] (icon / title / actions / a Row, whatever). [variant]
/// accent → accent border; [pad] tight → compact inset; [selectable] (+ [selected]) → a tappable card
/// (hover deepens the border, selected goes accent, emits via [onSelect]). Collapses the kit's repeated
/// settings/MCP/onboarding card skins into one place (principle #8).
///
/// Not [row]-flagged like the demo (CSS flex-direction doesn't map to a single-child container) — pass
/// a Row as the child when you want a horizontal card.
///
/// 有边卡片容器(无边 AnInfoCard 的对偶):hairline 描边(画在盒内、圆角干净无灰尖)+ r-chip + surface 底 + 内距;
/// 内容走 child(调用方自拼 icon/标题/动作/Row)。variant=accent→accent 描边;pad=tight→紧凑;selectable[+selected]→
/// 可选卡(hover 深边、选中 accent、派 onSelect)。收口 settings/MCP/onboarding 重复卡皮肤(原则 #8)。不设 demo 的
/// row 旗标(CSS flex-direction 不映射单 child 容器)——要横向卡就传 Row child。
class AnCard extends StatelessWidget {
  const AnCard({
    required this.child,
    this.variant = AnCardVariant.normal,
    this.selectable = false,
    this.selected = false,
    this.pad = AnCardPad.normal,
    this.onSelect,
    super.key,
  });

  final Widget child;
  final AnCardVariant variant;
  final bool selectable;
  final bool selected;
  final AnCardPad pad;
  final VoidCallback? onSelect;

  @override
  Widget build(BuildContext context) {
    if (selectable) {
      // The whole card is one button (a navigation/selection card); inner content rides along. 整卡即 button。
      return AnInteractive(
        onTap: onSelect,
        selected: selected,
        builder: (context, states) => _card(context, active: states.isActive),
      );
    }
    // Static card: a container whose children stay individually reachable (NOT merged), like AnInfoCard. 静态卡:子件各自可达。
    return Semantics(container: true, explicitChildNodes: true, child: _card(context, active: false));
  }

  Widget _card(BuildContext context, {required bool active}) {
    final c = context.colors;
    final reduced = AnMotionPref.reduced(context);
    final padding = pad == AnCardPad.tight
        ? const EdgeInsets.symmetric(horizontal: AnSpace.s8, vertical: AnSpace.s4)
        : const EdgeInsets.symmetric(horizontal: AnSpace.s16, vertical: AnSpace.s12);

    // Priority (matches demo source order): selected (2px accent) > selectable-hover (lineStrong) >
    // accent variant (accentLine) > neutral (line). So an accent selectable card's hover deepens to
    // lineStrong (demo `:host([selectable]:hover)` wins over `[variant=accent]`). 选中>hover>accent>中性。
    final Border border;
    if (selected) {
      border = Border.all(color: c.accentLine, width: AnSize.gripLine); // gripLine == demo --line-2 (2px) 选中 2px
    } else if (active) {
      border = Border.all(color: c.lineStrong, width: AnSize.hairline); // selectable hover (active ⇒ selectable)
    } else if (variant == AnCardVariant.accent) {
      border = Border.all(color: c.accentLine, width: AnSize.hairline);
    } else {
      border = Border.all(color: c.line, width: AnSize.hairline);
    }

    return AnimatedContainer(
      duration: reduced ? Duration.zero : AnMotion.fast, // selectable hover/selected = functional micro-feedback 功能性微反馈
      padding: padding,
      decoration: BoxDecoration(
        color: c.surface,
        border: border,
        borderRadius: BorderRadius.circular(AnRadius.chip),
      ),
      child: child,
    );
  }
}
