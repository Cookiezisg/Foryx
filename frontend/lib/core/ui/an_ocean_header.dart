import 'package:flutter/widgets.dart';

import '../design/colors.dart';
import '../design/tokens.dart';
import '../design/typography.dart';
import 'an_action_group.dart';
import 'an_inline_edit.dart';

/// D5 — the ocean page header: breadcrumb (where) + big H2 title (what) + meta (notes) + right actions
/// (what-you-can-do). It sits ON the ocean (no card), layered above the white content islands below it.
/// When [onTitleChange] is given the title edits IN PLACE via the parameterized [AnInlineEdit] (the H2
/// keeps its size/box, no font jump — G4.6); otherwise it is a read-only H2. v1 single-lines the title
/// (ellipsis) — multi-line wrap is deferred. The scroll-collapse linkage (big title scrolls out → shell
/// compact title) is owned downstream by the AnPage scroll host (this header has no scrollable), so v1
/// just renders the static header from [title].
///
/// D5——海洋页头:面包屑(在哪)+ 大 H2 标题(是什么)+ meta(附注)+ 右动作(能做什么)。坐于海面(无卡),与下方白岛分层。
/// 给 onTitleChange 则标题经参数化 AnInlineEdit **就地改名**(H2 保号/盒、不跳,G4.6),否则只读 H2。v1 标题单行省略
/// (换行推迟)。滚动收起联动(大标题滑出→壳紧凑标题)归下游 AnPage 滚动宿主(本头无可滚),故 v1 只渲静态头。
class AnOceanHeader extends StatelessWidget {
  const AnOceanHeader({
    required this.title,
    this.crumbs = const [],
    this.onTitleChange,
    this.actions = const [],
    this.meta = const [],
    super.key,
  });

  final String title;

  /// Breadcrumb parts ("where"), joined with `/`. 面包屑层级。
  final List<String> crumbs;

  /// Non-null → the title edits in place (H2 inline rename). 非空则标题就地改名。
  final ValueChanged<String>? onTitleChange;

  /// Right-side actions (an [AnActionGroup]). 右侧动作。
  final List<Widget> actions;

  /// Meta row widgets (badges / status dots), wrapping. meta 行(徽章/状态点,换行)。
  final List<Widget> meta;

  @override
  Widget build(BuildContext context) {
    final c = context.colors;
    final titleStyle = AnText.h2.weight(FontWeight.w600);
    final onChange = onTitleChange;

    return Padding(
      padding: const EdgeInsets.only(bottom: AnSpace.s24),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        mainAxisSize: MainAxisSize.min,
        children: [
          if (crumbs.isNotEmpty || actions.isNotEmpty)
            ConstrainedBox(
              constraints: const BoxConstraints(minHeight: AnSize.control),
              child: Row(
                children: [
                  Expanded(child: crumbs.isEmpty ? const SizedBox.shrink() : _crumb(c)),
                  if (actions.isNotEmpty) ...[const SizedBox(width: AnSpace.s12), AnActionGroup(actions)],
                ],
              ),
            ),
          Padding(
            padding: const EdgeInsets.symmetric(vertical: AnSpace.s8),
            child: onChange != null
                ? AnInlineEdit(
                    value: title,
                    style: titleStyle,
                    // H2 line box + slack for the edit frame's vertical bleed (editBoxPadY each side). H2 行盒 + 编辑框纵向余量。
                    minHeight: titleStyle.fontSize! * (titleStyle.height ?? 1.0) + AnSize.editBoxPadY * 2,
                    onCommit: onChange,
                  )
                // The page's PRIMARY heading — header semantics so screen readers can jump to it (rotor /
                // H key); mirrors AnInfoCard's title. The editable branch's field can't carry it. 主标题=header 节点。
                : Semantics(
                    header: true,
                    child: Text(title, maxLines: 1, overflow: TextOverflow.ellipsis, style: titleStyle.copyWith(color: c.ink)),
                  ),
          ),
          if (meta.isNotEmpty)
            Wrap(
              spacing: AnSpace.s16,
              runSpacing: AnSpace.s8,
              crossAxisAlignment: WrapCrossAlignment.center,
              children: meta,
            ),
        ],
      ),
    );
  }

  // crumb = parts joined by a faint `/` separator, ellipsizing as one line. 面包屑:parts + 灰 / 分隔,整行省略。
  Widget _crumb(AnColors c) {
    final spans = <InlineSpan>[];
    for (var i = 0; i < crumbs.length; i++) {
      if (i > 0) spans.add(TextSpan(text: '  /  ', style: AnText.meta.copyWith(color: c.lineStrong)));
      spans.add(TextSpan(text: crumbs[i]));
    }
    return Text.rich(
      TextSpan(children: spans),
      maxLines: 1,
      overflow: TextOverflow.ellipsis,
      style: AnText.meta.copyWith(color: c.inkFaint),
    );
  }
}
