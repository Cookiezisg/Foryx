import 'package:flutter/services.dart';
import 'package:flutter/widgets.dart';

import '../design/colors.dart';
import '../design/tokens.dart';
import '../design/typography.dart';
import 'an_interactive.dart';
import 'an_scroll_behavior.dart';

/// One tab of an [AnTabs]: a [key], a [label], an optional [count] (a secondary tabular number), and its
/// [pane]. AnTabs 一项。
class AnTabsItem {
  const AnTabsItem({required this.key, required this.label, this.count, required this.pane});
  final String key;
  final String label;
  final String? count;
  final Widget pane;
}

/// B5 — a text-underline view switcher (NOT a segmented control): a row of tab buttons (ink-faint → muted
/// on hover → ink when selected) with ONE spring-following underline, over an [IndexedStack] of panes that
/// stay alive (editing / scroll / stream state survives switching — "hide, don't destroy"). The strip
/// scrolls horizontally with the bar hidden when there are more tabs than fit. Controlled: [value] is the
/// selected key, [onSelect] fires on a user pick (NOT on a programmatic [value] change).
///
/// Hand-rolled on [AnInteractive] (NOT Material TabBar — its ripple / M3 indicator / 46px height clash, and
/// it has tab-role asserts + a desktop traverse-auto-switch bug). a11y: each tab is a `selected` button
/// (no `tab`/`tabBar` SemanticsRole — that role runs a structural child-assert that crashes when the
/// underline / scroll viewport / body are in its subtree; button+selected is safe). Keyboard: Tab traverses;
/// Enter/Space activates; ← → Home End rove focus within the strip WITHOUT auto-selecting (manual
/// activation, WAI-ARIA tabs). The underline lives INSIDE the scroll content (tracks the tab as it scrolls).
///
/// B5——文字下划线视图切换器(非分段器):tab 按钮(灰→hover muted→选中 ink)+ 一条弹簧下划线,下接 keep-alive 的
/// IndexedStack panes(编辑/滚动/流态跨切保留=「隐藏不销毁」)。tab 多则横滚隐条。受控:value=选中 key,onSelect 仅用户
/// 点选才派(非程序改 value;与 AnRow/AnSidebarList 同名)。手搓 on AnInteractive(非 Material TabBar:ink 波纹/M3 indicator/46px 高冲突 + tab-role
/// 断言 + 桌面遍历自动切 bug)。a11y:每 tab=selected button(**不挂 tab/tabBar role**,该 role 结构断言会因下划线/滚动
/// viewport/body 在子树而崩;button+selected 安全)。键盘:Tab 遍历、Enter/Space 激活、←→Home/End 在 strip 内移焦不自动选
/// (手动激活)。下划线在滚动内容内(随 tab 滚)。
class AnTabs extends StatefulWidget {
  const AnTabs(
      {required this.items,
      required this.value,
      required this.onSelect,
      this.enabled = true,
      this.flow = false,
      super.key});

  final List<AnTabsItem> items;
  final String value;

  /// Fires on a USER pick (not on a programmatic [value] change) — named [onSelect] to match the kit's
  /// other controlled-selection primitives (AnRow / AnSidebarList). 用户点选才派(非程序改 value);与 AnRow/AnSidebarList 同名。
  final ValueChanged<String> onSelect;
  final bool enabled;

  /// FLOW mode: the bar + ONLY the selected pane stack vertically and size to content, so the tabs read
  /// as part of a surrounding document scroll (the demo `an-tabs` model — bar + tabs + sections all scroll
  /// together inside one [AnPage]). Default (false) = FILL mode: an [Expanded] [IndexedStack] that fills a
  /// bounded box and keeps every pane alive (for a bounded panel). Flow drops keep-alive — fine when pane
  /// state lives in providers, not the widget. 流式:条 + 仅选中面顺排、随内容高,融入外层文档滚动(demo 模型);
  /// 默认填充式(Expanded+IndexedStack,bounded 面板保活)。
  final bool flow;

  @override
  State<AnTabs> createState() => _AnTabsState();
}

class _MoveTabFocus extends Intent {
  const _MoveTabFocus(this.delta);
  final int delta; // -1 prev / +1 next / ±large = home/end
}

class _AnTabsState extends State<AnTabs> with WidgetsBindingObserver {
  final GlobalKey _stripKey = GlobalKey();
  late List<GlobalKey> _tabKeys;
  late List<FocusNode> _focusNodes;
  final ScrollController _scroll = ScrollController();

  double? _sliderLeft;
  double? _sliderWidth;
  bool _measured = false; // first measure jumps (no animation); later picks animate 首测跳、后续动

  int get _index {
    final i = widget.items.indexWhere((it) => it.key == widget.value);
    return i < 0 ? 0 : i;
  }

  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addObserver(this);
    _allocNodes(widget.items.length);
    _scheduleMeasure();
  }

  // Re-measure the underline on any relayout the build()-time schedule can't see: window resize + text-scale
  // (the app's Cmd-+/- zoom). 窗口/字号变(无 rebuild)也重量下划线,避免停在旧几何。
  @override
  void didChangeMetrics() => _scheduleMeasure();
  @override
  void didChangeTextScaleFactor() => _scheduleMeasure();

  void _allocNodes(int n) {
    _tabKeys = List.generate(n, (_) => GlobalKey());
    _focusNodes = List.generate(n, (i) => FocusNode(debugLabel: 'AnTabs.tab$i'));
  }

  @override
  void didUpdateWidget(AnTabs old) {
    super.didUpdateWidget(old);
    if (old.items.length != widget.items.length) {
      for (final f in _focusNodes) {
        f.dispose();
      }
      _allocNodes(widget.items.length);
    }
    if (old.value != widget.value || old.items.length != widget.items.length) _scheduleMeasure();
  }

  @override
  void dispose() {
    WidgetsBinding.instance.removeObserver(this);
    for (final f in _focusNodes) {
      f.dispose();
    }
    _scroll.dispose();
    super.dispose();
  }

  // Measure the selected tab's offset + width WITHIN the strip content, then position the underline. 量选中 tab 定位下划线。
  void _scheduleMeasure() {
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (!mounted) return;
      final i = _index;
      if (i >= _tabKeys.length) return;
      final tabBox = _tabKeys[i].currentContext?.findRenderObject() as RenderBox?;
      final stripBox = _stripKey.currentContext?.findRenderObject() as RenderBox?;
      if (tabBox == null || stripBox == null || !tabBox.hasSize) return;
      final dx = stripBox.globalToLocal(tabBox.localToGlobal(Offset.zero)).dx;
      if (_sliderLeft != dx || _sliderWidth != tabBox.size.width) {
        setState(() {
          _sliderLeft = dx;
          _sliderWidth = tabBox.size.width;
          _measured = true;
        });
      } else if (!_measured) {
        setState(() => _measured = true);
      }
    });
  }

  void _pick(String key) {
    if (key != widget.value) widget.onSelect(key);
  }

  void _moveFocus(int delta) {
    if (!widget.enabled) return;
    final n = widget.items.length;
    if (n == 0) return;
    var cur = _focusNodes.indexWhere((f) => f.hasFocus);
    if (cur < 0) cur = _index;
    final target = delta.abs() >= n ? (delta < 0 ? 0 : n - 1) : (cur + delta).clamp(0, n - 1);
    _focusNodes[target].requestFocus();
    final ctx = _tabKeys[target].currentContext;
    if (ctx != null) Scrollable.ensureVisible(ctx, duration: AnMotion.fast, alignment: 0.5);
  }

  @override
  Widget build(BuildContext context) {
    final c = context.colors;
    final reduced = AnMotionPref.reduced(context);
    // Self-heal: re-measure post-frame on EVERY build (idempotent — the measure no-ops unless the selected
    // tab's geometry actually moved), so a content change (label/count width, reorder) or any relayout
    // corrects the underline without a value/length change. 每帧补量(幂等),内容/重排/重布局都自愈。
    _scheduleMeasure();

    final strip = ScrollConfiguration(
      behavior: const AnScrollBehavior(),
      child: SingleChildScrollView(
        controller: _scroll,
        scrollDirection: Axis.horizontal,
        // The Stack IS the scroll content: tabs + underline share its coordinate space, so the underline
        // scrolls WITH the tabs. Stack 即滚动内容:tabs 与下划线同坐标系、一起滚。
        child: Stack(
          key: _stripKey,
          children: [
            Row(
              mainAxisSize: MainAxisSize.min,
              spacing: AnSpace.s16,
              children: [for (var i = 0; i < widget.items.length; i++) _tab(c, i)],
            ),
            // underline (ExcludeSemantics — not a tab child). 下划线(排除语义,非 tab 子)。
            AnimatedPositioned(
              duration: (reduced || !_measured) ? Duration.zero : AnMotion.mid,
              curve: AnMotion.spring,
              left: _sliderLeft ?? 0,
              bottom: 0,
              width: _sliderWidth ?? 0,
              height: AnSize.gripLine,
              child: ExcludeSemantics(
                child: DecoratedBox(
                  decoration: BoxDecoration(color: c.ink, borderRadius: BorderRadius.circular(AnRadius.pill)),
                ),
              ),
            ),
          ],
        ),
      ),
    );

    final wiredStrip = FocusTraversalGroup(
      child: Shortcuts(
        shortcuts: const {
          SingleActivator(LogicalKeyboardKey.arrowRight): _MoveTabFocus(1),
          SingleActivator(LogicalKeyboardKey.arrowLeft): _MoveTabFocus(-1),
          SingleActivator(LogicalKeyboardKey.home): _MoveTabFocus(-1 << 20),
          SingleActivator(LogicalKeyboardKey.end): _MoveTabFocus(1 << 20),
        },
        child: Actions(
          actions: {
            _MoveTabFocus: CallbackAction<_MoveTabFocus>(onInvoke: (i) {
              _moveFocus(i.delta);
              return null;
            }),
          },
          child: strip,
        ),
      ),
    );

    // panes top pad (demo .panes padding-top sp-4). panes 顶留白。
    final panesPad = const EdgeInsets.only(top: AnSpace.s16);
    final Widget panes = widget.flow
        // FLOW: only the selected pane, sized to content → flows in the outer document scroll. 仅选中面、随内容高。
        ? Padding(padding: panesPad, child: widget.items[_index].pane)
        // FILL: keep-alive IndexedStack filling a bounded box. 保活 IndexedStack 填满 bounded。
        : Expanded(
            child: Padding(
              padding: panesPad,
              child: IndexedStack(
                index: _index,
                sizing: StackFit.expand,
                children: [for (final it in widget.items) it.pane],
              ),
            ),
          );

    return Opacity(
      opacity: widget.enabled ? 1 : AnOpacity.disabled,
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [wiredStrip, panes],
      ),
    );
  }

  Widget _tab(AnColors c, int i) {
    final it = widget.items[i];
    // selected tracks _index (the fallback-clamped selection), NOT raw widget.value — so highlight,
    // underline, and the visible pane stay in lockstep even when value is stale/unknown (then all resolve
    // to tab 0). 选中跟 _index(非裸 value),value 失效时高亮/下划线/pane 三者一致(都落 tab 0)。
    final selected = i == _index;
    // a11y: AnInteractive emits button + `selected` + the label Text as the accessible name ("Source,
    // selected button") — the safe path (no tab/tabBar SemanticsRole; see class doc). a11y 走 selected button。
    return AnInteractive(
      key: _tabKeys[i],
      enabled: widget.enabled,
      selected: selected,
      focusNode: _focusNodes[i],
      onTap: () => _pick(it.key),
      builder: (context, states) {
        final active = states.isActive;
        final fg = selected ? c.ink : (active ? c.inkMuted : c.inkFaint);
        return SizedBox(
          height: AnSize.tab,
          child: Padding(
            padding: const EdgeInsets.symmetric(horizontal: AnSpace.s4),
            child: Row(
              mainAxisSize: MainAxisSize.min,
              children: [
                Text(it.label,
                    maxLines: 1,
                    overflow: TextOverflow.clip,
                    style: AnText.body.weight(FontWeight.w500).copyWith(color: fg)),
                if (it.count != null && it.count!.isNotEmpty) ...[
                  const SizedBox(width: AnSpace.s6),
                  Text(it.count!,
                      style: AnText.metaTabular().copyWith(color: selected ? c.inkMuted : c.inkFaint)),
                ],
              ],
            ),
          ),
        );
      },
    );
  }
}
