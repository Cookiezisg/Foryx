import 'package:flutter/semantics.dart';
import 'package:flutter/widgets.dart';

import '../../i18n/strings.g.dart';
import '../design/colors.dart';
import '../design/tokens.dart';
import '../design/typography.dart';
import 'an_button.dart';
import 'an_dropdown.dart';
import 'an_edit_affordance.dart';
import 'an_lead_value.dart';
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
/// Focus returns to the pencil only on a KEYBOARD finish (Enter/Esc), not on a pointer ✓✕ / blur — see
/// [_finish]. Entering edit announces politely. The display value mirrors the field's style so toggling never jumps.
///
/// [AnEditKind.select]: the value zone is an always-present ghost [AnDropdown] (it IS the editor — a
/// pick commits, outside-tap / Esc dismiss it harmlessly), so there's no dangling edit state to get stuck
/// in. [rowHeight] is parameterized (Field [AnSize.islandHead] / Kv [AnSize.row]) so one core serves both.
///
/// 双锚就地值编辑核(AnField + AnKv 共用,= demo field.js)。input:平时只读,hover 时 key 右冒铅笔 → 点铅笔值换
/// seamless 框、value 右出 取消/保存(两锚,异于 AnEditAffordance 同处三连)。Enter/✓/失焦提交、Esc/✕ 取消;abort 经
/// 一次性 _finished 守卫优先,✓✕ 套 TextFieldTapRegion 不触发失焦提交(取消优先);仅键盘完成(Enter/Esc)回落焦点到铅笔
/// (指针 ✓✕/失焦不回落,见 _finish);进编辑礼貌宣告;展示值镜像编辑框样式不跳。select:值区=常驻 ghost AnDropdown(它即编辑器,
/// pick 提交、外点/Esc 无害收起),无悬空编辑态。rowHeight 参数化(Field AnSize.islandHead / Kv AnSize.row)。
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
    this.startEditing = false,
    super.key,
  }) : assert(!startEditing || editor == AnEditKind.input,
            'startEditing applies to AnEditKind.input only — select has no edit state to open. 仅 input 适用');

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

  /// Open directly in edit mode ([AnEditKind.input] only) — for galleries / matrix coverage of the
  /// editing state, or a freshly-added row. 直接进编辑态(仅 input,供 gallery/matrix + 新增行)。
  final bool startEditing;

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
    if (widget.startEditing && widget.editor == AnEditKind.input) {
      _editing = true;
      _ctl.selection = TextSelection.collapsed(offset: _ctl.text.length); // caret at end 光标落末
    }
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
    // Reset from the current value — discards any text left over from a PRIOR ABORTED edit (didUpdateWidget
    // only syncs on an external value change, and an abort leaves widget.value unchanged). 重置丢弃上次取消遗留文本。
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

  // One-shot per session ([_finished]); abort (✕ / Esc) wins if it lands first. Commit trims (no dirty
  // whitespace). The returnFocus decision is explained inline below. 一次性;abort 先到胜;提交去首尾空白。
  void _finish(bool commit, {required bool returnFocus}) {
    if (_finished) return;
    _finished = true;
    final next = _ctl.text.trim();
    if (!returnFocus) {
      // Pointer finish (✓✕ click / blur): drop focus from the about-to-be-removed editing zone (the field
      // or the ✓✕ button) BEFORE the rebuild — otherwise, when that focused node is removed, Flutter
      // RESTORES focus to the nearest survivor (the pencil), re-revealing + focus-ringing it. Doing it
      // pre-rebuild (synchronously) avoids the restoration entirely; a click elsewhere then takes focus via
      // its own gesture. 指针完成:重建前(同步)卸掉编辑区焦点,杜绝被自动恢复到铅笔(否则铅笔再现+画焦点框)。
      FocusManager.instance.primaryFocus?.unfocus();
    }
    setState(() => _editing = false);
    if (commit && next != widget.value) widget.onChanged(next);
    // returnFocus is decided by the SOURCE of the finish, NOT the input modality: the KEYBOARD paths
    // (Enter/Esc in the field) pass true so keyboard nav continues on the pencil; the POINTER paths (a
    // ✓✕ click, blur) pass false — a click must NOT focus the pencil, else `revealPencil` (reads hasFocus)
    // pins it visible AND it paints its focus ring instead of returning to its hidden resting state.
    // NB: FocusManager.highlightMode can't tell mouse from keyboard on desktop — a MOUSE pointer is also
    // `traditional` (only finger-touch is `touch`) — so the call site, not highlightMode, decides.
    // returnFocus 按完成「来源」(非输入模态)判定:键盘路径(字段 Enter/Esc)传 true 续导航;指针路径(点 ✓✕、失焦)
    // 传 false——点击不该聚焦铅笔(否则 revealPencil 读 hasFocus 卡可见 + 画焦点框而非隐回默认)。注:桌面上鼠标指针
    // 的 highlightMode 也是 traditional(只有触摸是 touch),分不开鼠标/键盘,故按调用点而非 highlightMode 判定。
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
          child: widget.editor == AnEditKind.select ? _selectRow() : _inputRow(c),
        ),
      ),
    );
  }

  // select: the value zone is an always-present ghost dropdown — no pencil, no editing state, so no
  // dangling state on dismiss (a pick commits; outside-tap / Esc just close it). select:常驻 ghost 下拉。
  Widget _selectRow() {
    return AnLeadValue(
      leading: widget.leading,
      trailing: AnDropdown<String>(
        options: widget.options,
        value: widget.value,
        variant: AnDropdownVariant.ghost,
        menuAlignEnd: true,
        onChanged: widget.onChanged,
      ),
    );
  }

  Widget _inputRow(AnColors c) {
    final revealPencil = _hovered || _pencilFocus.hasFocus;
    return AnLeadValue(
      leading: widget.leading,
      // Editing single-lines the field (Align-right); only the resting display honours wrap. 编辑单行、展示才换行。
      wrap: !_editing && widget.wrap,
      // Pencil: kept in the tree while idle (keyboard-reachable), revealed by opacity on hover/focus; gone
      // while editing (the confirm takes the value end). 铅笔常驻(键盘可达)、opacity 揭示;编辑时撤、由 ✓✕ 接管。
      afterLeading: _editing
          ? null
          : Opacity(
              opacity: revealPencil ? 1 : 0,
              child: AnButton.iconOnly(
                AnIcons.edit,
                size: AnButtonSize.sm,
                semanticLabel: context.t.action.edit,
                focusNode: _pencilFocus,
                onPressed: _begin,
              ),
            ),
      trailing: _inputValueZone(c),
      // ✓✕ in a TextFieldTapRegion so tapping them isn't a blur-commit (cancel-priority); returnFocus:false
      // because a click is a pointer finish (see _finish). ✓✕ 套 TapRegion 不触发失焦提交;点击不回落焦点(见 _finish)。
      afterValue: _editing
          ? TextFieldTapRegion(
              child: AnEditAffordance(
                editing: true,
                onCommit: () => _finish(true, returnFocus: false),
                onAbort: () => _finish(false, returnFocus: false),
              ),
            )
          : null,
    );
  }

  Widget _inputValueZone(AnColors c) {
    final color = widget.valueColor ?? c.inkMuted;
    if (_editing) {
      return AnSeamlessField(
        controller: _ctl,
        mono: widget.mono,
        tabular: true, // value column: digits always tabular (idle ↔ editing same width) 值列数字恒等宽
        framed: true, // demo edit frame (no row-height growth, right-only horizontal) 编辑框(不加行高、右生长)
        onCommit: () => _finish(true, returnFocus: true),
        onAbort: () => _finish(false, returnFocus: true),
        onTapOutside: (_) => _finish(true, returnFocus: false), // blur-commit; focus stays where clicked 失焦提交、焦点不回落
      );
    }
    // Display mirrors the seamless field's style (shared value-column style) so idle ↔ editing never
    // changes size/face; empty shows an em-dash. 展示走值列样式单源(切换不跳),空显 —。
    final base = AnText.value(mono: widget.mono);
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
