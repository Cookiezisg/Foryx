import 'package:flutter/widgets.dart';

import '../design/tokens.dart';

/// The kit's right-anchored two-zone row content (the demo's `.lab{flex:1}` + `.meta{flex:none;
/// max-width}`, in Flutter): primary [label] fills the LEFT and ellipsis-truncates last; secondary
/// [meta] sits RIGHT, capped at ≤45% of the row so a long id can't crowd out the label, ellipsis when
/// over; [trailing] (caret / check / actions) is pinned to the right edge because the label is
/// [Expanded] (greedy). Both texts truncate independently — no overflow.
///
/// Promoted from AnDropdown's private `_TwoZone` (it recurred across the dropdown trigger + its menu
/// rows, and G3 reuses the same skeleton for Section heads, InfoCard heads, and the Row/Kv trailing
/// slot) — principle #8: the shared skeleton lives in one place, consumers don't re-roll Row+Spacer.
/// [trailing] takes any Widget, so a consumer (e.g. AnRow) can drop a hover-swap layer into it.
///
/// 套件右锚两区行内容(demo 的 lab flex:1 + meta flex:none·max-width 的 Flutter 版):label 占满左、最后才省略;
/// meta 居右、上限 45%(长 id 挤不掉 label)、超长省略;trailing(箭头/勾/动作)因 label Expanded 而钉在右沿。
/// 两者各自截断、不溢出。由 AnDropdown 的 private `_TwoZone` 升格(触发器/菜单行/G3 的 Section·InfoCard head·
/// Row·Kv 尾槽都复用此骨架,原则 #8:骨架归一处、消费方不再各搓 Row+Spacer);trailing 收任意 Widget。
const double _kMetaMaxFraction = 0.45; // meta zone ≤ 45% of the row (label keeps ≥ 55%) meta 区上限
const double _kMetaFallbackWidth = 160; // meta cap when the row width is unbounded 无界时 meta 上限

class AnTwoZone extends StatelessWidget {
  const AnTwoZone({required this.label, this.meta, this.metaStyle, required this.trailing, super.key});

  final Widget label;
  final String? meta;
  final TextStyle? metaStyle;
  final Widget trailing;

  @override
  Widget build(BuildContext context) {
    return LayoutBuilder(
      builder: (context, constraints) {
        final metaCap = constraints.maxWidth.isFinite ? constraints.maxWidth * _kMetaMaxFraction : _kMetaFallbackWidth;
        return Row(
          children: [
            Expanded(child: label),
            if (meta != null) ...[
              const SizedBox(width: AnSpace.s8),
              ConstrainedBox(
                constraints: BoxConstraints(maxWidth: metaCap),
                child: Text(meta!, maxLines: 1, overflow: TextOverflow.ellipsis, textAlign: TextAlign.right, style: metaStyle),
              ),
            ],
            const SizedBox(width: AnSpace.s8),
            trailing,
          ],
        );
      },
    );
  }
}
