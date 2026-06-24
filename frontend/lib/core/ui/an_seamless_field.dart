import 'package:flutter/services.dart';
import 'package:flutter/widgets.dart';

import '../design/colors.dart';
import '../design/tokens.dart';
import 'an_input.dart';
import 'dry_intrinsic_width.dart';

/// The kit's in-place edit field leaf (shared by AnInlineEdit + AnEditableValue): a seamless [AnInput]
/// sized to its content ([DryIntrinsicWidth]), floored at [AnSize.inlineEditMin] (an empty field stays
/// clickable) and never wider than the width its parent allots — AnInlineEdit via a [Flexible],
/// AnEditableValue via AnLeadValue's value slot — so it scrolls at the cap.
/// A trailing pad keeps the caret off the last glyph at end-of-line (flutter#24612); no placeholder (a
/// hint pollutes the intrinsic width, flutter#93337). Enter → [onCommit], Esc → [onAbort], pointer
/// outside → [onTapOutside] (blur-commit; the consumer wraps its confirm buttons in a
/// [TextFieldTapRegion] for cancel-priority). The CALLER sets the controller's selection before opening
/// (rename → select-all; value-edit → caret at end), so this leaf stays selection-policy-agnostic.
///
/// 就地编辑字段叶子(AnInlineEdit + AnEditableValue 共用):seamless AnInput 按内容定宽、不窄于 inlineEditMin、不宽于父
/// 让出的空间(AnInlineEdit 经 Flexible、AnEditableValue 经 AnLeadValue 值槽,到顶横滚);尾留位防光标压末字(flutter#24612);不给 placeholder(污染固有宽 flutter#93337)。
/// Enter 存、Esc 弃、外点 onTapOutside(失焦提交;调用方 ✓✕ 套 TextFieldTapRegion 取消优先)。选区由调用方进编辑前设
/// (重命名全选 / 改值光标落末),本叶子不预设选区策略。
class AnSeamlessField extends StatelessWidget {
  const AnSeamlessField({
    required this.controller,
    required this.onCommit,
    required this.onAbort,
    this.focusNode,
    this.onTapOutside,
    this.autofocus = true,
    this.mono = false,
    this.tabular = false,
    this.framed = false,
    this.style,
    super.key,
  });

  final TextEditingController controller;
  final VoidCallback onCommit;
  final VoidCallback onAbort;
  final FocusNode? focusNode;
  final TapRegionCallback? onTapOutside;
  final bool autofocus;

  /// Monospace field (matches a mono display value so toggling doesn't change face/size). 等宽(匹配 mono 展示值)。
  final bool mono;

  /// Tabular figures (mono already implies it) — match a tabular display value. 等宽数字(匹配 tabular 展示值)。
  final bool tabular;

  /// Show the demo's edit FRAME while editing — a 1px [AnColors.lineStrong] inset border + [AnColors.surface]
  /// fill + [AnRadius.tag] corner. It takes NO vertical layout height (the box bleeds over the host row's
  /// fixed-height slack, demo `.v.editing` negative vertical margin) and grows ONLY to the right
  /// ([AnSize.editBoxPadX]) while the text stays anchored (left pad bleeds over slack). Requires the host to
  /// reserve ≥ text-height + [AnSize.editBoxPadY]·2 (true for control/row/islandHead). 编辑框:不占行高、只右生长、文字不跳。
  final bool framed;

  /// Text-style override to match the display text it replaces (e.g. an H2 title). 文字样式覆写(匹配被替换文字)。
  final TextStyle? style;

  @override
  Widget build(BuildContext context) {
    Widget field = DryIntrinsicWidth(
      child: Padding(
        padding: const EdgeInsetsDirectional.only(end: AnSize.caretEndPad), // caret room at line end 行尾光标留位
        child: ConstrainedBox(
          constraints: const BoxConstraints(minWidth: AnSize.inlineEditMin),
          child: AnInput(
            controller: controller,
            focusNode: focusNode,
            seamless: true,
            autofocus: autofocus,
            mono: mono,
            tabular: tabular,
            style: style,
            onSubmitted: (_) => onCommit(),
            onTapOutside: onTapOutside,
          ),
        ),
      ),
    );

    if (framed) {
      final c = context.colors;
      // The frame is drawn BEHIND the field via a Stack so its vertical padding overflows the row's slack
      // (zero layout height — demo's negative vertical margin) and its left padding bleeds over slack (text
      // stays anchored). Only the right pad is REAL layout (EdgeInsetsDirectional.end) so the box grows right.
      // 框经 Stack 画在字段后:纵向溢出余量(零行高)、左溢出(文字不跳),仅右内距真占位(框右生长)。
      field = Stack(
        clipBehavior: Clip.none,
        children: [
          Positioned.directional(
            textDirection: Directionality.of(context),
            start: -AnSize.editBoxPadX, // bleed left over slack (cancel left pad → text anchored) 左溢出
            end: 0, // right edge = the real right pad (Padding below) 右缘=真右内距
            top: -AnSize.editBoxPadY,
            bottom: -AnSize.editBoxPadY,
            child: DecoratedBox(
              decoration: BoxDecoration(
                color: c.surface,
                border: Border.all(color: c.lineStrong, width: AnSize.hairline),
                borderRadius: BorderRadius.circular(AnRadius.tag),
              ),
            ),
          ),
          Padding(
            padding: const EdgeInsetsDirectional.only(end: AnSize.editBoxPadX), // real right growth 右真占位
            child: field,
          ),
        ],
      );
    }

    return CallbackShortcuts(
      bindings: {const SingleActivator(LogicalKeyboardKey.escape): onAbort},
      child: field,
    );
  }
}
