import 'package:flutter/material.dart';

import '../../core/design/colors.dart';
import '../../core/design/theme.dart';
import '../../core/design/tokens.dart';
import '../../core/design/typography.dart';
import '../../core/ui/ui.dart';
import 'catalog.dart';
import 'specimen.dart';

/// The component gallery (`make gallery`) — a dual-pane catalog mirroring the demo's reference.html:
/// a category nav rail + a scrollable specimen grid, with a light/dark toggle. Every An* widget
/// appears here in all its states the same commit it lands; this is the human fidelity loop against
/// the demo (the machine gate is the widget-test matrix).
///
/// 组件画廊(make gallery)——双栏目录,镜像 demo reference.html:类目导航栏 + 可滚 specimen 栅格 + 明暗切换。
/// 每个 An* 组件全态在此、与落地同提交;这是对照 demo 的人工保真环(机器门禁=widget-test 矩阵)。
// Vertical room for the frameless macOS title-bar (traffic lights live here). 无边框标题栏纵向留位。
const double _titleBarReserve = 28;

class GalleryApp extends StatefulWidget {
  const GalleryApp({this.initialCategory = 0, super.key});

  /// Which category to open on (dev-only; the capture harness snapshots each). 初始类目(截图夹具逐类用)。
  final int initialCategory;

  @override
  State<GalleryApp> createState() => _GalleryAppState();
}

class _GalleryAppState extends State<GalleryApp> {
  Brightness _brightness = Brightness.light;
  late int _categoryIndex = widget.initialCategory;
  // Shared with AnOverlayHost so the G6 overlay specimens can push confirm dialogs (mirrors app.dart).
  // 与 AnOverlayHost 共用,供 G6 浮层 specimen push 确认框(同 app.dart)。
  final GlobalKey<NavigatorState> _navigatorKey = GlobalKey<NavigatorState>();

  @override
  Widget build(BuildContext context) {
    return MaterialApp(
      debugShowCheckedModeBanner: false,
      theme: _brightness == Brightness.light ? AnTheme.light() : AnTheme.dark(),
      navigatorKey: _navigatorKey,
      builder: (context, child) => AnOverlayHost(navigatorKey: _navigatorKey, child: child!),
      home: Builder(
        builder: (context) {
          final c = context.colors;
          final category = galleryCatalog[_categoryIndex];
          // Material ancestor — AnInput's TextField (and other Material leaves) require it; the app
          // shell provides one too. Material 祖先:AnInput 的 TextField 等需要,app 壳也提供。
          // The sidebar SURFACE runs to the very top (lights float over it); only the brand/title
          // CONTENT is inset below the title-bar zone. 侧栏面铺到顶(灯浮其上),仅品牌/标题内容下移避开灯。
          return Material(
            color: c.canvas,
            child: Row(
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: [
                _nav(context),
                Expanded(
                  child: Column(
                    children: [
                      const SizedBox(height: _titleBarReserve),
                      _topBar(context, category),
                      Expanded(child: _content(context, category)),
                    ],
                  ),
                ),
              ],
            ),
          );
        },
      ),
    );
  }

  Widget _nav(BuildContext context) {
    final c = context.colors;
    return Container(
      width: AnSize.sidebarMin,
      decoration: BoxDecoration(
        color: c.surface,
        border: Border(right: BorderSide(color: c.line, width: AnSize.hairline)),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          // Reserve the title-bar zone so the brand clears the OS traffic lights (the white surface
          // above still reaches the window top). 留出标题栏区让品牌避开红绿灯(上方白面仍铺到窗顶)。
          const SizedBox(height: _titleBarReserve),
          Padding(
            padding: const EdgeInsets.fromLTRB(AnSpace.s12, AnSpace.s8, AnSpace.s12, AnSpace.s8),
            child: Row(
              children: [
                const AnBrandIcon.anselm(size: AnBrandSize.sm),
                const SizedBox(width: AnSpace.s8),
                Text('组件画廊', style: AnText.strong.copyWith(color: c.ink)),
              ],
            ),
          ),
          Expanded(
            child: ListView.builder(
              padding: const EdgeInsets.symmetric(horizontal: AnSpace.s8),
              itemCount: galleryCatalog.length,
              itemBuilder: (context, i) => _navRow(context, i),
            ),
          ),
        ],
      ),
    );
  }

  Widget _navRow(BuildContext context, int i) {
    final cat = galleryCatalog[i];
    final selected = i == _categoryIndex;
    return AnInteractive(
      onTap: () => setState(() => _categoryIndex = i),
      selected: selected,
      builder: (context, states) {
        final c = context.colors;
        final hovered = states.contains(WidgetState.hovered);
        return AnimatedContainer(
          duration: AnMotion.fast,
          height: AnSize.row,
          padding: const EdgeInsets.symmetric(horizontal: AnSpace.s8),
          decoration: BoxDecoration(
            color: selected ? c.surfaceActive : (hovered ? c.surfaceHover : c.surfaceHover.withValues(alpha: 0)),
            borderRadius: BorderRadius.circular(AnRadius.button),
          ),
          child: Row(
            children: [
              Icon(cat.icon, size: AnSize.icon, color: selected ? c.ink : c.inkMuted),
              const SizedBox(width: AnSpace.s8),
              Expanded(
                child: Text(cat.label,
                    maxLines: 1,
                    overflow: TextOverflow.ellipsis,
                    style: AnText.body.copyWith(color: selected ? c.ink : c.inkMuted)),
              ),
            ],
          ),
        );
      },
    );
  }

  Widget _topBar(BuildContext context, GalleryCategory category) {
    final c = context.colors;
    return Container(
      height: AnSize.islandHead,
      padding: const EdgeInsets.symmetric(horizontal: AnSpace.s24),
      alignment: Alignment.center,
      child: Row(
        children: [
          Text(category.label, style: AnText.strong.copyWith(color: c.ink)),
          const Spacer(),
          AnButton(
            label: _brightness == Brightness.light ? 'Dark' : 'Light',
            size: AnButtonSize.sm,
            onPressed: () => setState(() =>
                _brightness = _brightness == Brightness.light ? Brightness.dark : Brightness.light),
          ),
        ],
      ),
    );
  }

  Widget _content(BuildContext context, GalleryCategory category) {
    return LayoutBuilder(
      builder: (context, constraints) {
        final width = constraints.maxWidth - AnSpace.s24 * 2;
        return SingleChildScrollView(
          padding: const EdgeInsets.fromLTRB(AnSpace.s24, AnSpace.s8, AnSpace.s24, AnSpace.s48),
          // ExcludeFocus: the catalog is a passive display — a specimen that opens in its EDIT state
          // (AnInlineEdit/AnEditableValue with startEditing) mounts a seamless field whose `autofocus`
          // would otherwise (a) steal app focus and (b) make EditableText.showOnScreen scroll this page
          // down to the field on launch (it opened ~73% down). descendantsAreFocusable:false skips the
          // autofocus so the page opens at the top; pointer taps on interactive specimens still work
          // (overlays/dialogs they push live outside this subtree). 目录是被动展示:编辑态 specimen 的
          // autofocus 会抢焦点 + 把页面滚到字段处(开机停在 73% 处);ExcludeFocus 让 autofocus 不触发→开机即顶部。
          child: ExcludeFocus(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                for (final item in category.items) _itemBlock(context, item, width),
              ],
            ),
          ),
        );
      },
    );
  }

  Widget _itemBlock(BuildContext context, GalleryItem item, double width) {
    final c = context.colors;
    const cellW = AnSize.block; // 280 grid track
    return Padding(
      padding: const EdgeInsets.only(top: AnSpace.s24),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            crossAxisAlignment: CrossAxisAlignment.baseline,
            textBaseline: TextBaseline.alphabetic,
            children: [
              Text(item.name, style: AnText.h3.copyWith(color: c.ink)),
              const SizedBox(width: AnSpace.s12),
              // Flexible + ellipsis so a long blurb never overflows the header row (dev-tool resilience). 长 blurb 省略不溢出。
              Flexible(
                child: Text(item.blurb, maxLines: 1, overflow: TextOverflow.ellipsis, style: AnText.meta.copyWith(color: c.inkMuted)),
              ),
            ],
          ),
          const SizedBox(height: AnSpace.s12),
          Wrap(
            spacing: AnSpace.s12,
            runSpacing: AnSpace.s12,
            children: [
              for (final s in item.specimens)
                SizedBox(width: s.span ? width : cellW, child: _cell(context, s)),
            ],
          ),
        ],
      ),
    );
  }

  Widget _cell(BuildContext context, GallerySpecimen s) {
    final c = context.colors;
    return Container(
      padding: const EdgeInsets.all(AnSpace.s16),
      decoration: BoxDecoration(
        color: c.surface,
        border: Border.all(color: c.line, width: AnSize.hairline),
        borderRadius: BorderRadius.circular(AnRadius.chip),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        mainAxisSize: MainAxisSize.min,
        children: [
          // A maxWidth-constrained specimen renders narrow so long content actually truncates; a
          // height-bounded specimen gives scroll-hosting components a viewport. 受限宽逼截断;有界高给滚动宿主视口。
          Align(
            alignment: Alignment.centerLeft,
            child: (s.maxWidth != null || s.height != null)
                ? SizedBox(width: s.maxWidth, height: s.height, child: Builder(builder: s.builder))
                : Builder(builder: s.builder),
          ),
          const SizedBox(height: AnSpace.s12),
          Row(
            children: [
              if (s.stress) ...[
                Container(
                  padding: const EdgeInsets.symmetric(horizontal: AnSpace.s4),
                  margin: const EdgeInsets.only(right: AnSpace.s6),
                  decoration: BoxDecoration(color: c.warnSoft, borderRadius: BorderRadius.circular(AnRadius.tag)),
                  child: Text('压力', style: AnText.meta.copyWith(color: c.warn)),
                ),
              ],
              Flexible(
                child: Text(s.label,
                    maxLines: 1,
                    overflow: TextOverflow.ellipsis,
                    style: AnText.meta.copyWith(color: c.inkFaint)),
              ),
            ],
          ),
        ],
      ),
    );
  }
}
