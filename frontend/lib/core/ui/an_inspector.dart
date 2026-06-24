import 'package:flutter/widgets.dart';

import '../design/colors.dart';
import '../design/tokens.dart';
import '../design/typography.dart';
import 'an_scroll_behavior.dart';

/// D4 — the right-island CONTENT shell: an optional head (icon + title) above a scrolling block-flow body.
/// It does NOT draw the island skin nor manage width — the shell wraps it in an [AnIsland] of a fixed
/// width, so this only renders head + body (named [AnInspector], not "AnRightIsland", to keep that clear).
/// The body is a vertical block flow ([children] stacked with [AnSpace.s12]) that scrolls with the bar
/// hidden ([AnScrollBehavior]). [headless] omits the head and lets [child] fill + self-manage its own
/// scroll (e.g. a streaming entity workspace). v1 carries NO landmark `complementary` role — that lands at
/// app-assembly when the full landmark map is designed (a lone empty-label complementary asserts).
///
/// D4——右岛内容壳:可选头(icon+title)+ 滚动块流 body。**不画岛皮、不管宽**(壳用固定宽 AnIsland 包它),故只渲 head+body
/// (命名 AnInspector 而非 AnRightIsland,免误指它画岛)。body 是竖向块流(children 以 s12 堆叠)、滚动隐条(AnScrollBehavior)。
/// headless 去头、让 child 占满并自管滚动(如流式工作台)。v1 不挂 complementary landmark(留 app 装配,空 label 会断言)。
class AnInspector extends StatelessWidget {
  const AnInspector({
    this.title,
    this.icon,
    this.children = const [],
    this.headless = false,
    this.child,
    super.key,
  });

  /// Head title (ink, w600, ellipsis). 头标题。
  final String? title;

  /// Head icon (decorative, inkFaint). 头图标(装饰)。
  final IconData? icon;

  /// Body blocks, stacked top-to-bottom with [AnSpace.s12] between, scrolling. body 块流。
  final List<Widget> children;

  /// Omit the head; [child] fills and self-manages scroll. 去头,child 占满自管滚动。
  final bool headless;

  /// The fill content for [headless]. headless 的占满内容。
  final Widget? child;

  @override
  Widget build(BuildContext context) {
    final c = context.colors;

    // headless: no head, no scroll wrapper — the child fills the (bounded) island + owns its scroll. 占满自管。
    if (headless) return child ?? const SizedBox.shrink();

    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        // head: icon + title, islandHead tall, top pad only (sits in the island's top band). 头。
        SizedBox(
          height: AnSize.islandHead,
          child: Padding(
            padding: const EdgeInsetsDirectional.only(start: AnSpace.s16, end: AnSpace.s16, top: AnSpace.s8),
            child: Row(
              children: [
                if (icon != null) ...[
                  ExcludeSemantics(child: Icon(icon, size: AnSize.icon, color: c.inkFaint)), // decorative 装饰
                  const SizedBox(width: AnSpace.s8),
                ],
                Expanded(
                  child: Text(
                    title ?? '',
                    maxLines: 1,
                    overflow: TextOverflow.ellipsis,
                    style: AnText.body.weight(FontWeight.w600).copyWith(color: c.ink),
                  ),
                ),
              ],
            ),
          ),
        ),
        // body: vertical block flow, scrolls with the bar hidden. 块流滚动、隐条。
        Expanded(
          child: ScrollConfiguration(
            behavior: const AnScrollBehavior(),
            child: SingleChildScrollView(
              padding: const EdgeInsets.symmetric(horizontal: AnSpace.s16, vertical: AnSpace.s8),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.stretch,
                mainAxisSize: MainAxisSize.min,
                spacing: AnSpace.s12,
                children: children,
              ),
            ),
          ),
        ),
      ],
    );
  }
}
