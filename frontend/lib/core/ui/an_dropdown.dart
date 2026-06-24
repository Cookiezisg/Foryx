import 'package:flutter/widgets.dart';

import '../design/colors.dart';
import '../design/tokens.dart';
import '../design/typography.dart';
import 'an_interactive.dart';
import 'an_menu_surface.dart';
import 'an_popover.dart';
import 'an_two_zone.dart';
import 'icons.dart';

/// B3 — a controlled single-select dropdown (replaces the native select). The trigger echoes the
/// chosen label (+ optional mono meta + caret); tapping opens a rich-row menu (label / meta / icon
/// / check-current) on [AnPopover]. [variant] ghost is the borderless trigger; [menuAlignEnd]
/// right-aligns the menu (row-trailing controls). The menu scrolls past a cap so a huge option
/// list never overflows the screen.
///
/// B3——受控单选下拉(替原生 select)。触发器回显选中 label(+ 可选 mono meta + caret);点开富行菜单
/// (label/meta/icon/勾选当前)搭于 AnPopover。ghost=无边框触发器;menuAlignEnd=菜单右对齐。超量选项菜单滚动、不溢出。
class AnDropdownOption<T> {
  const AnDropdownOption({required this.value, required this.label, this.meta, this.icon});

  final T value;
  final String label;
  final String? meta;
  final IconData? icon;
}

enum AnDropdownVariant { normal, ghost }

class AnDropdown<T> extends StatefulWidget {
  const AnDropdown({
    required this.options,
    required this.value,
    required this.onChanged,
    this.placeholder = '—',
    this.variant = AnDropdownVariant.normal,
    this.block = false,
    this.enabled = true,
    this.menuAlignEnd = false,
    super.key,
  });

  final List<AnDropdownOption<T>> options;
  final T? value;
  final ValueChanged<T>? onChanged;
  final String placeholder;
  final AnDropdownVariant variant;
  final bool block;
  final bool enabled;
  final bool menuAlignEnd;

  @override
  State<AnDropdown<T>> createState() => _AnDropdownState<T>();
}

class _AnDropdownState<T> extends State<AnDropdown<T>> {
  final AnPopoverController _popover = AnPopoverController();

  @override
  void initState() {
    super.initState();
    _popover.addListener(_onPopover);
  }

  void _onPopover() => setState(() {});

  @override
  void dispose() {
    _popover.removeListener(_onPopover);
    _popover.dispose();
    super.dispose();
  }

  AnDropdownOption<T>? get _selected {
    for (final o in widget.options) {
      if (o.value == widget.value) return o;
    }
    return null;
  }

  // The row to seed keyboard focus on when the menu opens: the selected value, else the first.
  // 菜单打开时初始聚焦的行:选中值,否则第一项。
  T? get _autofocusValue =>
      _selected?.value ?? (widget.options.isNotEmpty ? widget.options.first.value : null);

  void _pick(T value) {
    _popover.close();
    widget.onChanged?.call(value);
  }

  @override
  Widget build(BuildContext context) {
    final enabled = widget.enabled && widget.onChanged != null;
    final ghost = widget.variant == AnDropdownVariant.ghost;

    final trigger = AnInteractive(
      enabled: enabled,
      onTap: _popover.toggle,
      builder: (context, states) => _trigger(context, states, ghost),
    );

    return Opacity(
      opacity: enabled ? 1 : AnOpacity.disabled,
      child: AnPopover(
        controller: _popover,
        alignEnd: widget.menuAlignEnd,
        overlayBuilder: (context, anchorSize) => _menu(context, anchorSize),
        anchor: widget.block ? SizedBox(width: double.infinity, child: trigger) : trigger,
      ),
    );
  }

  Widget _trigger(BuildContext context, Set<WidgetState> states, bool ghost) {
    final c = context.colors;
    final open = _popover.isOpen;
    final active = open || states.isActive;
    final sel = _selected;

    final label = Text(
      sel?.label ?? widget.placeholder,
      maxLines: 1,
      overflow: TextOverflow.ellipsis, // label hugs LEFT, ellipsis when long 标签靠左、超长省略
      style: (ghost ? AnText.meta : AnText.body).copyWith(
        color: sel == null ? c.inkFaint : (ghost ? (active ? c.ink : c.inkMuted) : c.ink),
      ),
    );

    final caret = AnimatedRotation(
      duration: AnMotion.fast,
      turns: open ? 0.5 : 0,
      child: Icon(AnIcons.chevronDown, size: AnSize.iconSm, color: c.inkFaint),
    );

    final metaStyle = AnText.metaTabular().copyWith(color: c.inkFaint);

    // Ghost = compact, content-hugging (settings-style) — label + caret, intrinsic. Ghost 紧凑贴合内容。
    if (ghost) {
      return AnimatedContainer(
        duration: AnMotion.fast,
        height: AnSize.controlSm,
        padding: const EdgeInsets.symmetric(horizontal: AnSize.btnPadXSm),
        decoration: BoxDecoration(
          color: c.surfaceHover.whenActive(active),
          borderRadius: BorderRadius.circular(AnRadius.button),
        ),
        child: Row(
          mainAxisSize: MainAxisSize.min,
          children: [
            Flexible(child: label),
            const SizedBox(width: AnSpace.s6),
            caret,
          ],
        ),
      );
    }

    // Boxed = TWO ZONES: label fills LEFT, meta caps RIGHT, caret pinned right (see AnTwoZone).
    // 盒式=两区:label 占满左、meta 上限右、箭头钉右。
    return AnimatedContainer(
      duration: AnMotion.fast,
      height: AnSize.control,
      constraints: const BoxConstraints(minWidth: AnSize.inputMin),
      padding: const EdgeInsets.symmetric(horizontal: AnSize.btnPadXSm),
      decoration: BoxDecoration(
        color: c.surface,
        border: Border.all(color: active ? c.lineStrong : c.line, width: AnSize.hairline),
        borderRadius: BorderRadius.circular(AnRadius.button),
      ),
      child: AnTwoZone(label: label, meta: sel?.meta, metaStyle: metaStyle, trailing: caret),
    );
  }

  Widget _menu(BuildContext context, Size? anchorSize) {
    // Menu width = the trigger width CLAMPED to [menuMin, menuMax] — tracks the trigger (aligned,
    // dropped directly below) but never narrower than menuMin (so rich rows fit even off a compact ghost
    // trigger) nor wider than menuMax. (The MENU hugs content via IntrinsicWidth; the DROPDOWN deliberately
    // matches its trigger instead.) 下拉宽=触发框宽夹 [min,max](菜单贴内容、下拉跟触发器,各自策略)。
    final triggerW = anchorSize?.width ?? AnSize.inputMin;
    final menuW = triggerW.clamp(AnSize.menuMinWidth, AnSize.menuMaxWidth);
    return ConstrainedBox(
      constraints: BoxConstraints(minWidth: menuW, maxWidth: menuW, maxHeight: AnSize.menuMaxHeight),
      // shared menu chrome (surface + s4-all-sides inset + FocusTraversalGroup) — same standard AnMenu uses
      // so the selected/hover pill floats inset, not edge-to-edge. 共用面板壳(与 AnMenu 同标准、药丸内缩)。
      child: AnMenuSurface(
        children: [
          for (final o in widget.options)
            _MenuRow(
              option: o,
              selected: o.value == widget.value,
              autofocus: _autofocusValue == o.value, // seed focus on the selected (else first) row 聚焦选中行
              onTap: () => _pick(o.value),
            ),
        ],
      ),
    );
  }
}

class _MenuRow<T> extends StatelessWidget {
  const _MenuRow({required this.option, required this.selected, required this.onTap, this.autofocus = false});

  final AnDropdownOption<T> option;
  final bool selected;
  final bool autofocus;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    // Shared row standard (rounded inset pill, hover/active fill, reduced-gate, autofocus) — same surface
    // AnMenu items use; only the content below (TWO ZONES + trailing check) is dropdown-specific. 共用行标准。
    return AnMenuRow(
      onTap: onTap,
      autofocus: autofocus,
      builder: (context, active) {
        final c = context.colors;
        // Menu row = same TWO ZONES as the trigger: optional leading icon, then label LEFT + meta RIGHT
        // (via AnTwoZone), with the selected-check as the trailing slot (reserved when unchecked so rows
        // align). 菜单行=与触发器同两区:可选前导图标 + label 左 + meta 右,选中勾为尾槽(未选留位对齐)。
        return Row(
          children: [
            if (option.icon != null) ...[
              Icon(option.icon, size: AnSize.icon, color: c.inkMuted),
              const SizedBox(width: AnSpace.s8),
            ],
            Expanded(
              child: AnTwoZone(
                label: Text(option.label,
                    maxLines: 1, overflow: TextOverflow.ellipsis, style: AnText.body.copyWith(color: c.ink)),
                meta: option.meta,
                metaStyle: AnText.meta.copyWith(color: c.inkFaint),
                trailing: SizedBox(
                  width: AnSize.iconSm,
                  child: selected ? Icon(AnIcons.check, size: AnSize.iconSm, color: c.ink) : null,
                ),
              ),
            ),
          ],
        );
      },
    );
  }
}
