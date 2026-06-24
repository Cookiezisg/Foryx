import 'package:flutter/widgets.dart';

import '../design/colors.dart';
import '../design/tokens.dart';
import '../design/typography.dart';
import '../model/sidebar_model.dart';
import 'an_button.dart';
import 'an_input.dart';
import 'an_interactive.dart';
import 'an_menu.dart';
import 'an_row.dart';
import 'an_scroll_behavior.dart';
import 'icons.dart';

/// C5 — the left-rail sidebar list: a New row + an in-domain filter (with a sliders menu) + a recursive
/// groups → types → rows tree. The New / filter heads share the same lead column as the entity rows so the
/// `+` / search glyph aligns with the row icons. Built on [AnRow] (entity rows + collapsible type heads),
/// [AnInput] (seamless filter), [AnMenu] (sliders), with the pure [SidebarModel] filter driving hide +
/// ancestor-reveal. Selection is controlled ([selectedId] + [onSelect]); fold state is internal (all open
/// by default). The keyboard expand/collapse for the tree rides here (the consumer of AnRow.expanded).
///
/// C5——左岛侧栏:New 行 + 域内过滤(带 sliders 菜单)+ 递归 groups→types→rows 树。New/过滤头与实体行共用行首列,故
/// +/🔍 与行图标对齐。搭 AnRow(实体行 + 可折叠类型头)+ AnInput(seamless 过滤)+ AnMenu(sliders),纯 SidebarModel
/// 过滤驱动隐藏 + 祖先回填。选中受控(selectedId+onSelect);折叠态内部(默认全开)。
///
/// Fold + filter state are TRANSIENT widget state (the widget owns them): branch fold keys on row id, but
/// group/type fold keys on POSITION (g$gi / g$gi/t$ti) — so the model should be positionally stable; a
/// reorder/insert can carry stale fold to a new slot, and a model swap keeps the active filter query. Keep
/// the model identity stable (memoize) across rebuilds. 折叠/过滤是瞬时态:树枝按 id、group/type 按位置键,故模型须位置稳定。
class AnSidebarList extends StatefulWidget {
  const AnSidebarList({
    required this.model,
    this.selectedId,
    this.onSelect,
    this.onNew,
    this.onFilterChanged,
    this.menuEntries = const [],
    this.showNew = true,
    this.rowActionsBuilder,
    super.key,
  });

  final SidebarModel model;
  final String? selectedId;
  final ValueChanged<String>? onSelect;
  final VoidCallback? onNew;
  final ValueChanged<String>? onFilterChanged;

  /// Sliders (Sort / Display) menu entries; empty → no sliders anchor. sliders 菜单项,空则不渲。
  final List<AnMenuEntry> menuEntries;
  final bool showNew;

  /// Optional trailing actions per row (e.g. add / more), keyed by row id. 行尾动作(add/more)。
  final List<Widget> Function(String rowId)? rowActionsBuilder;

  @override
  State<AnSidebarList> createState() => _AnSidebarListState();
}

class _AnSidebarListState extends State<AnSidebarList> {
  final TextEditingController _filter = TextEditingController();
  final Set<String> _collapsed = {}; // keys of collapsed group/type/branch (default: all open) 折叠集(默认全开)
  String _query = '';

  @override
  void dispose() {
    _filter.dispose();
    super.dispose();
  }

  void _toggle(String key) => setState(() => _collapsed.contains(key) ? _collapsed.remove(key) : _collapsed.add(key));

  bool _open(String key) => !_collapsed.contains(key);

  @override
  Widget build(BuildContext context) {
    final c = context.colors;
    final active = _query.trim().isNotEmpty;
    final visible = active ? sidebarVisibleIds(widget.model, _query) : const <String>{};

    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        if (widget.showNew) _newRow(c),
        _filterRow(c),
        Expanded(
          child: ScrollConfiguration(
            behavior: const AnScrollBehavior(),
            child: SingleChildScrollView(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.stretch,
                children: [
                  for (var gi = 0; gi < widget.model.groups.length; gi++) _group(c, gi, active, visible),
                ],
              ),
            ),
          ),
        ),
      ],
    );
  }

  // New: a full-width left-aligned button row (lead = +). New 行(整行左对齐按钮)。
  Widget _newRow(AnColors c) {
    return AnInteractive(
      onTap: widget.onNew,
      builder: (context, states) => Container(
        height: AnSize.row,
        padding: const EdgeInsets.symmetric(horizontal: AnSpace.s8),
        decoration: BoxDecoration(color: c.surfaceHover.whenActive(states.isActive), borderRadius: BorderRadius.circular(AnRadius.button)),
        child: Row(
          children: [
            Icon(AnIcons.plus, size: AnSize.icon, color: c.inkFaint),
            const SizedBox(width: AnSpace.s8),
            Expanded(
              child: Text(widget.model.newLabel,
                  maxLines: 1, overflow: TextOverflow.ellipsis, style: AnText.body.copyWith(color: c.inkMuted)),
            ),
          ],
        ),
      ),
    );
  }

  // Filter: lead = search, an inline seamless input, a trailing sliders menu. 过滤行。
  Widget _filterRow(AnColors c) {
    return Container(
      height: AnSize.row,
      padding: const EdgeInsets.symmetric(horizontal: AnSpace.s8),
      child: Row(
        children: [
          Icon(AnIcons.search, size: AnSize.icon, color: c.inkFaint),
          const SizedBox(width: AnSpace.s8),
          Expanded(
            child: AnInput(
              controller: _filter,
              seamless: true,
              placeholder: widget.model.filterPlaceholder,
              onChanged: (v) {
                setState(() => _query = v);
                widget.onFilterChanged?.call(v);
              },
            ),
          ),
          if (widget.menuEntries.isNotEmpty)
            AnMenu(
              entries: widget.menuEntries,
              anchorBuilder: (context, toggle, isOpen) =>
                  AnButton.iconOnly(AnIcons.sliders, size: AnButtonSize.sm, semanticLabel: 'Display options', onPressed: toggle),
            ),
        ],
      ),
    );
  }

  Widget _group(AnColors c, int gi, bool active, Set<String> visible) {
    final g = widget.model.groups[gi];
    final shown = !active || g.types.any((t) => t.rows.any((r) => _rowVisible(r, active, visible)));
    if (!shown) return const SizedBox.shrink();

    final types = Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [for (var ti = 0; ti < g.types.length; ti++) _type(c, gi, ti, active, visible)],
    );
    if (!g.collapsible) return types;

    // collapsible big group: a light chat-style head (gray w600 label + count + rotating chevron). 可折叠大组。
    // Key by POSITION (not label) — two groups may share a label, which would fuse their fold state. 按位置键(非标签,避免同名组折叠态串)。
    final key = 'g$gi';
    final open = active || _open(key); // a query forces groups open to reveal matches 过滤强制展开
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        AnInteractive(
          onTap: () => _toggle(key),
          expanded: open,
          builder: (context, states) => Container(
            height: AnSize.control,
            padding: const EdgeInsets.symmetric(horizontal: AnSpace.s8),
            decoration: BoxDecoration(color: c.surfaceHover.whenActive(states.isActive), borderRadius: BorderRadius.circular(AnRadius.button)),
            child: Row(
              children: [
                Flexible(child: Text(g.label!, maxLines: 1, overflow: TextOverflow.ellipsis, style: AnText.meta.weight(FontWeight.w600).copyWith(color: c.inkFaint))),
                const SizedBox(width: AnSpace.s6),
                Text('${g.totalRows}', style: AnText.meta.weight(FontWeight.w500).copyWith(color: c.inkFaint)),
                const Spacer(),
                AnimatedRotation(
                  duration: AnMotionPref.reduced(context) ? Duration.zero : AnMotion.mid,
                  curve: AnMotion.spring,
                  turns: open ? 0.25 : 0,
                  child: Icon(AnIcons.chevronRight, size: AnSize.iconSm, color: c.inkFaint),
                ),
              ],
            ),
          ),
        ),
        if (open) types,
      ],
    );
  }

  Widget _type(AnColors c, int gi, int ti, bool active, Set<String> visible) {
    final t = widget.model.groups[gi].types[ti];
    final shown = !active || t.rows.any((r) => _rowVisible(r, active, visible));
    if (!shown) return const SizedBox.shrink();

    if (t.headless) {
      return Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [for (final r in t.rows) _row(c, r, 0, active, visible)],
      );
    }
    final key = 'g$gi/t$ti';
    final open = active || _open(key);
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        // The type head is a disclosure BUTTON (not an entity): the whole head toggles — onSelect=toggle so
        // it is keyboard-operable (Enter/Space) + click, not just the mouse-only lead chevron. 类型头=展开按钮(整头切、键盘可达)。
        AnRow(
          icon: t.icon,
          label: t.label ?? '',
          meta: t.count != null ? '${t.count}' : null,
          collapsible: true,
          open: open,
          onSelect: () => _toggle(key),
          onToggle: () => _toggle(key),
        ),
        if (open)
          for (final r in t.rows) _row(c, r, 1, active, visible),
      ],
    );
  }

  // recursive entity row (+ branch children). 递归实体行(+ 树枝子)。
  Widget _row(AnColors c, SidebarRow r, int depth, bool active, Set<String> visible) {
    if (active && !visible.contains(r.id)) return const SizedBox.shrink();
    final branch = r.hasChildren;
    final open = active || _open('r:${r.id}'); // a query forces branches open to reveal matches 过滤强制展开
    final row = AnRow(
      depth: depth,
      icon: r.dot == null ? r.icon : null,
      dot: r.dot,
      label: r.label,
      hint: r.hint,
      meta: r.meta,
      selected: r.id == widget.selectedId,
      collapsible: branch,
      open: open,
      onSelect: () => widget.onSelect?.call(r.id),
      onToggle: branch ? () => _toggle('r:${r.id}') : null,
      actions: widget.rowActionsBuilder?.call(r.id) ?? const [],
    );
    if (!branch || !open) return row;
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        row,
        for (final child in r.children) _row(c, child, depth + 1, active, visible),
      ],
    );
  }

  bool _rowVisible(SidebarRow r, bool active, Set<String> visible) => !active || visible.contains(r.id);
}
