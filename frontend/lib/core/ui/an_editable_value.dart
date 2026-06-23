import 'package:flutter/semantics.dart';
import 'package:flutter/widgets.dart';

import '../../i18n/strings.g.dart';
import '../design/colors.dart';
import '../design/tokens.dart';
import '../design/typography.dart';
import 'an_button.dart';
import 'an_dropdown.dart';
import 'an_edit_affordance.dart';
import 'an_seamless_field.dart';
import 'icons.dart';

/// How an editable value is edited in place. 就地编辑方式。
enum AnEditKind {
  /// Free text via a pencil → seamless field. 自由文本(铅笔 → seamless 框)。
  input,

  /// A closed set via an always-present inline dropdown. 枚举(常驻内联下拉)。
  select,
}

/// The kit's in-place value editor — the shared edit core of AnField + AnKv (the demo's `field.js`
/// editText / editSelect). A row of [leading] (key / label) + value.
///
/// [AnEditKind.input]: display-only until you click the pencil that reveals on hover at the KEY's
/// right; the value then swaps to a seamless field with cancel / save at the VALUE's right — TWO
/// anchors, unlike AnEditAffordance's co-located triad (hence net-new, not a reuse of that container).
/// Commit on Enter / ✓ / blur; cancel on Esc / ✕. Abort wins via a one-shot [_finished] guard, and the
/// confirm buttons sit in a [TextFieldTapRegion] so tapping them is NOT a blur-commit (cancel-priority).
/// Keyboard focus returns to the pencil on Enter/Esc (NOT on blur — that came from a click elsewhere).
/// Entering edit announces politely. The display value mirrors the field's style so toggling never jumps.
///
/// [AnEditKind.select]: the value zone is an always-present ghost [AnDropdown] (it IS the editor — a
/// pick commits, outside-tap / Esc dismiss it harmlessly), so there's no dangling edit state to get
/// stuck in. [rowHeight] is parameterized (Field 44 / Kv 32) so one core serves both without baking a height.
///
/// 双锚就地值编辑核(AnField + AnKv 共用,= demo field.js)。input:平时只读,hover 时 key 右冒铅笔 → 点铅笔值换
/// seamless 框、value 右出 取消/保存(两锚,异于 AnEditAffordance 同处三连)。Enter/✓/失焦提交、Esc/✕ 取消;abort 经
/// 一次性 _finished 守卫优先,✓✕ 套 TextFieldTapRegion 不触发失焦提交(取消优先);Enter/Esc 后焦点回落铅笔(失焦不回落);
/// 进编辑礼貌宣告;展示值镜像编辑框样式不跳。select:值区=常驻 ghost AnDropdown(它即编辑器,pick 提交、外点/Esc 无害
/// 收起),无悬空编辑态。rowHeight 参数化(Field 44 / Kv 32)。
class AnEditableValue extends StatefulWidget {
  const AnEditableValue({
    required this.leading,
    required this.fieldLabel,
    required this.value,
    required this.onChanged,
    this.rowHeight = AnSize.row,
    this.valueColor,
    this.editor = AnEditKind.input,
    this.options = const [],
    this.mono = false,
    this.wrap = false,
    super.key,
  });

  /// The visual left zone (a key [Text], or a label + hint column). 视觉左区(key 文本 / label+hint 列)。
  final Widget leading;

  /// Identifies the field for the edit-entry announcement + a11y. 用于编辑宣告 + a11y 的字段名。
  final String fieldLabel;
  final String value;
  final ValueChanged<String> onChanged;

  /// Row floor height (Field [AnSize.islandHead] / Kv [AnSize.row]). 行高下限。
  final double rowHeight;

  /// Display value colour (Field [AnColors.inkMuted] / Kv [AnColors.inkFaint]); defaults to inkMuted. 值色。
  final Color? valueColor;

  final AnEditKind editor;

  /// Options for [AnEditKind.select]. 枚举选项。
  final List<AnDropdownOption<String>> options;
  final bool mono;

  /// Long value wraps (left-aligned, multi-line) instead of single-line ellipsis. 长值换行。
  final bool wrap;

  @override
  State<AnEditableValue> createState() => _AnEditableValueState();
}

class _AnEditableValueState extends State<AnEditableValue> {
  late final TextEditingController _ctl;
  late final FocusNode _pencilFocus;
  bool _editing = false;
  bool _finished = false; // one-shot per edit session 每次编辑一次性
  bool _hovered = false;

  @override
  void initState() {
    super.initState();
    // Eager init (never a late-final field initializer — lazy first-read can fire in teardown). 急切初始化。
    _ctl = TextEditingController(text: widget.value);
    _pencilFocus = FocusNode(debugLabel: 'AnEditableValue.pencil');
    _pencilFocus.addListener(_onPencilFocus);
  }

  void _onPencilFocus() {
    if (mounted) setState(() {}); // reveal the pencil when it takes keyboard focus 键盘聚焦时显铅笔
  }

  @override
  void didUpdateWidget(AnEditableValue old) {
    super.didUpdateWidget(old);
    // External value change refreshes the resting text — never clobbers an in-progress edit. 外部改值刷新静态文字、不打断编辑。
    if (widget.value != old.value && !_editing) _ctl.text = widget.value;
  }

  @override
  void dispose() {
    _pencilFocus.removeListener(_onPencilFocus);
    _ctl.dispose();
    _pencilFocus.dispose();
    super.dispose();
  }

  void _begin() {
    _ctl.text = widget.value;
    // Caret at END — editing a value, NOT renaming, so no select-all. 光标落末(改值非重命名,不全选)。
    _ctl.selection = TextSelection.collapsed(offset: _ctl.text.length);
    setState(() {
      _editing = true;
      _finished = false;
    });
    if (SemanticsBinding.instance.semanticsEnabled) {
      // sendAnnouncement (not the deprecated announce) — polite by default. 进编辑礼貌宣告。
      SemanticsService.sendAnnouncement(
        View.of(context),
        context.t.a11y.editingField(field: widget.fieldLabel),
        Directionality.of(context),
      );
    }
  }

  // One-shot per session; abort (✕ / Esc) wins if it lands first. blur passes returnFocus:false (the
  // user clicked elsewhere — don't steal focus back). Commit trims (matches demo, no dirty whitespace).
  // 一次性;abort 先到则胜;失焦不回落焦点;提交去首尾空白(同 demo)。
  void _finish(bool commit, {required bool returnFocus}) {
    if (_finished) return;
    _finished = true;
    final next = _ctl.text.trim();
    setState(() => _editing = false);
    if (commit && next != widget.value) widget.onChanged(next);
    if (returnFocus) {
      WidgetsBinding.instance.addPostFrameCallback((_) {
        if (mounted) _pencilFocus.requestFocus();
      });
    }
  }

  @override
  Widget build(BuildContext context) {
    final c = context.colors;
    final reduced = AnMotionPref.reduced(context);

    return Semantics(
      container: true,
      explicitChildNodes: true, // key / value / pencil / ✓✕ each individually reachable (NOT merged) 各自可达、不 merge
      child: MouseRegion(
        onEnter: (_) => setState(() => _hovered = true),
        onExit: (_) => setState(() => _hovered = false),
        child: AnimatedContainer(
          duration: reduced ? Duration.zero : AnMotion.fast, // hover / editing tint = functional micro-feedback 功能性微反馈
          constraints: BoxConstraints(minHeight: widget.rowHeight),
          padding: const EdgeInsets.symmetric(horizontal: AnSpace.s8, vertical: AnSpace.s4),
          decoration: BoxDecoration(
            color: c.surfaceHover.whenActive(_hovered || _editing),
            borderRadius: BorderRadius.circular(AnRadius.button),
          ),
          child: widget.editor == AnEditKind.select ? _selectRow(c) : _inputRow(c),
        ),
      ),
    );
  }

  // select: the value zone is an always-present ghost dropdown — no pencil, no editing state, so no
  // dangling state on dismiss (a pick commits; outside-tap / Esc just close it). select:常驻 ghost 下拉。
  Widget _selectRow(AnColors c) {
    return Row(
      children: [
        Flexible(child: widget.leading),
        const Expanded(child: SizedBox.shrink()),
        Flexible(
          child: Align(
            alignment: Alignment.centerRight,
            child: AnDropdown<String>(
              options: widget.options,
              value: widget.value,
              variant: AnDropdownVariant.ghost,
              menuAlignEnd: true,
              onChanged: widget.onChanged,
            ),
          ),
        ),
      ],
    );
  }

  Widget _inputRow(AnColors c) {
    final revealPencil = _hovered || _pencilFocus.hasFocus;
    return Row(
      children: [
        Flexible(child: widget.leading),
        // Pencil: kept in the tree while idle (keyboard-reachable), revealed by opacity on hover/focus;
        // gone while editing (the confirm sits at the value end). 铅笔常驻(键盘可达)、opacity 揭示;编辑时撤。
        if (!_editing) ...[
          const SizedBox(width: AnSpace.s6),
          Opacity(
            opacity: revealPencil ? 1 : 0,
            child: AnButton.iconOnly(
              AnIcons.edit,
              size: AnButtonSize.sm,
              semanticLabel: context.t.action.edit,
              focusNode: _pencilFocus,
              onPressed: _begin,
            ),
          ),
        ],
        const Expanded(child: SizedBox.shrink()), // grow: pins the value to the right edge 撑开、值钉右
        Flexible(child: _inputValueZone(c)),
        if (_editing) ...[
          const SizedBox(width: AnSpace.s6),
          // TextFieldTapRegion: tapping cancel / save isn't "outside" the field → no blur-commit
          // (cancel-priority). 点 ✓✕ 不算字段外 → 不触发失焦提交(取消优先)。
          TextFieldTapRegion(
            child: AnEditAffordance(
              editing: true,
              onCommit: () => _finish(true, returnFocus: true),
              onAbort: () => _finish(false, returnFocus: true),
            ),
          ),
        ],
      ],
    );
  }

  Widget _inputValueZone(AnColors c) {
    final color = widget.valueColor ?? c.inkMuted;
    if (_editing) {
      return AnSeamlessField(
        controller: _ctl,
        mono: widget.mono,
        onCommit: () => _finish(true, returnFocus: true),
        onAbort: () => _finish(false, returnFocus: true),
        onTapOutside: (_) => _finish(true, returnFocus: false), // blur-commit; focus stays where clicked 失焦提交、焦点不回落
      );
    }
    // Display: mirror AnInput's seamless style so the idle ↔ editing toggle never changes size/face;
    // mono gets tabular figures (numeric alignment, 原语 D). Empty shows an em-dash. 展示镜像编辑框样式;mono 走 tabular;空显 —。
    final base = widget.mono
        ? AnText.mono.copyWith(fontSize: AnText.meta.fontSize, fontFeatures: const [FontFeature.tabularFigures()])
        : AnText.body;
    final display = widget.value.isEmpty ? '—' : widget.value;
    return Text(
      display,
      textAlign: widget.wrap ? TextAlign.left : TextAlign.right,
      maxLines: widget.wrap ? null : 1,
      softWrap: widget.wrap,
      overflow: widget.wrap ? TextOverflow.clip : TextOverflow.ellipsis,
      style: base.copyWith(color: color),
    );
  }
}
