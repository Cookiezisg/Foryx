import 'package:flutter/widgets.dart';

import '../design/colors.dart';
import '../design/tokens.dart';
import '../design/typography.dart';
import 'an_action_group.dart';
import 'an_two_zone.dart';

/// D2 — a borderless info unit (the "edgeless" dual of AnCard): organised by title / whitespace /
/// hierarchy, not rule lines. The head (icon + [title] + [meta]) renders only when one is present —
/// title fills + ellipsis-truncates, meta caps right and yields first (via [AnTwoZone]); [child] is the
/// body (a SINGLE slot — a multi-block body composes its own Column gap, like AnCard's dropped `row`
/// flag); [actions] sit below (collapsing when empty). Title is a `header` semantics node.
///
/// D2——无边信息单元(有边 AnCard 的对偶):靠标题/留白/层级组织、不靠横线。head(icon + title + meta)仅在有内容时渲——
/// title 占满省略、meta 居右先让位(经 AnTwoZone);child 为 body;actions 在下(无则塌)。title 为 header 语义节点。
class AnInfoCard extends StatelessWidget {
  const AnInfoCard({
    this.title,
    this.icon,
    this.meta,
    required this.child,
    this.actions = const [],
    super.key,
  });

  final String? title;
  final IconData? icon;
  final String? meta;
  final Widget child;
  final List<Widget> actions;

  bool get _hasTitle => title != null && title!.isNotEmpty;
  bool get _hasMeta => meta != null && meta!.isNotEmpty;
  bool get _hasHead => _hasTitle || icon != null || _hasMeta;

  @override
  Widget build(BuildContext context) {
    final c = context.colors;
    return Semantics(
      container: true,
      explicitChildNodes: true, // head / body / actions each individually reachable (NOT merged) 各自可达不 merge
      child: Padding(
        padding: const EdgeInsets.symmetric(horizontal: AnSpace.s8, vertical: AnSpace.s4),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          mainAxisSize: MainAxisSize.min,
          children: [
            if (_hasHead) ...[
              _head(c),
              const SizedBox(height: AnSpace.s4),
            ],
            child,
            if (actions.isNotEmpty) ...[
              const SizedBox(height: AnSpace.s12),
              AnActionGroup(actions),
            ],
          ],
        ),
      ),
    );
  }

  Widget _head(AnColors c) {
    return ConstrainedBox(
      constraints: const BoxConstraints(minHeight: AnSize.control), // demo .head min-height --ctl(28) — keeps card heads on one rhythm 卡头垂直节奏
      child: Row(
        children: [
          if (icon != null) ...[
            ExcludeSemantics(child: Icon(icon, size: AnSize.iconSm, color: c.inkFaint)), // decorative 装饰
            const SizedBox(width: AnSpace.s8),
          ],
        // AnTwoZone: title fills + ellipsis, meta caps right + yields first. title is a header node.
        // (icon is prepended outside — AnTwoZone has no leading slot.) title 占满、meta 让位;icon 在外。
        Expanded(
          child: AnTwoZone(
            label: _hasTitle
                ? Semantics(
                    header: true,
                    child: Text(
                      title!,
                      maxLines: 1,
                      overflow: TextOverflow.ellipsis,
                      style: AnText.meta.weight(FontWeight.w600).copyWith(color: c.inkFaint),
                    ),
                  )
                : const SizedBox.shrink(),
            meta: _hasMeta ? meta : null,
            metaStyle: AnText.meta.copyWith(color: c.inkFaint),
            trailing: const SizedBox.shrink(),
          ),
        ),
        ],
      ),
    );
  }
}
