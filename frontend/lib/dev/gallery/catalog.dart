import 'package:flutter/widgets.dart';

import '../../core/ui/ui.dart';
import 'specimen.dart';

// Gallery catalog — dev-only tool, so plain strings here are exempt from the i18n rule (like test
// code; never shipped). Grows one category per build group (G0–G6).
// 画廊目录——dev-only 工具,此处明文串豁免 i18n 规则(同测试代码,永不发布)。每组追加一类目。
final List<GalleryCategory> galleryCatalog = [
  _g1Controls,
  _g2Feedback,
];

// ── G1 — Foundational controls ──
final GalleryCategory _g1Controls = GalleryCategory('基础控件 Controls', AnIcons.sliders, [
  GalleryItem('AnStatusDot', '语义状态点;run 呼吸', [
    for (final s in AnStatus.values) GallerySpecimen(s.name, (_) => AnStatusDot(s)),
  ]),
  GalleryItem('AnBadge', '状态/标签药丸 + 可选点', [
    GallerySpecimen('neutral', (_) => const AnBadge('neutral')),
    GallerySpecimen('ok', (_) => const AnBadge('passed', tone: AnTone.ok)),
    GallerySpecimen('warn', (_) => const AnBadge('pending', tone: AnTone.warn)),
    GallerySpecimen('danger', (_) => const AnBadge('failed', tone: AnTone.danger)),
    GallerySpecimen('accent', (_) => const AnBadge('active', tone: AnTone.accent)),
    GallerySpecimen('dot=done', (_) => const AnBadge('completed', tone: AnTone.ok, dot: AnStatus.done)),
    GallerySpecimen('dot=run', (_) => const AnBadge('running', tone: AnTone.accent, dot: AnStatus.run)),
    GallerySpecimen('超长截断', (_) => const AnBadge('a-very-long-tag-that-must-truncate-not-blow-out', tone: AnTone.ok), stress: true, maxWidth: 150),
    GallerySpecimen('注入转义', (_) => const AnBadge('<b>not</b> & <i>html</i>', tone: AnTone.warn), stress: true),
  ]),
  GalleryItem('AnGroupLabel', '极薄分组小标题', [
    GallerySpecimen('default', (_) => const AnGroupLabel('Entities'), span: true),
    GallerySpecimen('超长截断', (_) => const AnGroupLabel('a very long section caption that should ellipsis instead of wrapping'), stress: true, maxWidth: 150),
  ]),
  GalleryItem('AnButton', '统一动作钮:变体/尺寸/图标/态', [
    GallerySpecimen('ghost', (_) => AnButton(label: 'Ghost', onPressed: () {})),
    GallerySpecimen('primary', (_) => AnButton(label: 'Run', icon: AnIcons.run, variant: AnButtonVariant.primary, onPressed: () {})),
    GallerySpecimen('danger', (_) => AnButton(label: 'Delete', variant: AnButtonVariant.danger, onPressed: () {})),
    GallerySpecimen('danger outline', (_) => AnButton(label: 'Delete', icon: AnIcons.trash, variant: AnButtonVariant.danger, outline: true, onPressed: () {})),
    GallerySpecimen('icon', (_) => AnButton.iconOnly(AnIcons.more, semanticLabel: 'More', onPressed: () {})),
    GallerySpecimen('size=sm', (_) => AnButton(label: 'Small', size: AnButtonSize.sm, onPressed: () {})),
    GallerySpecimen('disabled', (_) => const AnButton(label: 'Disabled', onPressed: null)),
    GallerySpecimen('block', (_) => AnButton(label: 'Block', icon: AnIcons.enter, block: true, onPressed: () {}), span: true),
    GallerySpecimen('超长截断', (_) => AnButton(label: 'a-really-long-button-label-that-must-ellipsis-within-its-box', block: true, onPressed: () {}), stress: true, maxWidth: 170),
  ]),
  GalleryItem('AnInput', '值叶子:单行/多行/等宽', [
    GallerySpecimen('default', (_) => const AnInput(placeholder: 'Type…')),
    GallerySpecimen('mono', (_) => const AnInput(initialValue: 'fn_3a9f', mono: true)),
    GallerySpecimen('readonly', (_) => const AnInput(initialValue: 'read only', readOnly: true)),
    GallerySpecimen('disabled', (_) => const AnInput(initialValue: 'disabled', enabled: false)),
    GallerySpecimen('multiline full', (_) => const AnInput(placeholder: 'Multiple lines…', multiline: true, block: true), span: true),
    GallerySpecimen('超长值', (_) => const AnInput(initialValue: 'this-is-an-extremely-long-single-line-value-that-should-scroll-horizontally-and-never-overflow-the-bordered-box', block: true), stress: true, maxWidth: 180),
  ]),
  GalleryItem('AnActionGroup', '动作组:对齐/间距/换行', [
    GallerySpecimen('default', (_) => AnActionGroup([AnButton(label: 'Cancel', onPressed: () {}), AnButton(label: 'Save', variant: AnButtonVariant.primary, onPressed: () {})]), span: true),
    GallerySpecimen('end compact', (_) => AnActionGroup([AnButton(label: 'A', size: AnButtonSize.sm, onPressed: () {}), AnButton(label: 'B', size: AnButtonSize.sm, onPressed: () {})], end: true, compact: true), span: true),
    GallerySpecimen('stack', (_) => AnActionGroup([AnButton(label: 'First', block: true, onPressed: () {}), AnButton(label: 'Second', block: true, onPressed: () {})], stack: true), span: true),
  ]),
  GalleryItem('AnEditAffordance', '就地编辑触发器原语:铅笔 → 取消/保存', [
    GallerySpecimen('idle (铅笔)', (_) => AnEditAffordance(editing: false, onEdit: () {})),
    GallerySpecimen('editing (取消/保存)', (_) => AnEditAffordance(editing: true, onCommit: () {}, onAbort: () {})),
  ]),
  GalleryItem('AnInlineEdit', '就地重命名:文字 → 自适应框(增长→封顶→截断,按钮跟随→钉右)', [
    GallerySpecimen('idle (点铅笔进编辑)', (_) => AnInlineEdit(value: 'Untitled workflow', onCommit: (_) {})),
    GallerySpecimen('editing 态', (_) => AnInlineEdit(value: 'Editing title', startEditing: true, onCommit: (_) {})),
    GallerySpecimen('超长·idle (省略+铅笔钉右)', (_) => AnInlineEdit(value: 'A very long entity title that must ellipsis when idle', onCommit: (_) {}), stress: true, maxWidth: 220),
    GallerySpecimen('超长·editing (框封顶→按钮钉右→横滚)', (_) => AnInlineEdit(value: 'A very long title being edited that grows, caps at the row, then scrolls', startEditing: true, onCommit: (_) {}), stress: true, maxWidth: 240),
  ]),
  GalleryItem('AnDropdown', '受控单选下拉 + 富行菜单', [
    GallerySpecimen('label + meta', (_) => const _DropdownDemo(initial: 'fn')),
    GallerySpecimen('single value(无 meta)', (_) => const _DropdownDemo(initial: 'med', simple: true)),
    GallerySpecimen('placeholder', (_) => const _DropdownDemo(initial: null, simple: true)),
    GallerySpecimen('ghost', (_) => const _DropdownDemo(initial: 'ag', ghost: true)),
    GallerySpecimen('disabled', (_) => const AnDropdown<String>(options: [], value: null, onChanged: null, placeholder: 'disabled', enabled: false)),
    GallerySpecimen('block', (_) => const _DropdownDemo(initial: 'wf', block: true), span: true),
    GallerySpecimen('两区都超长', (_) => AnDropdown<String>(
          options: const [AnDropdownOption(value: 'x', label: 'An extremely long entity name that must ellipsis on the left', meta: 'a_very_long_identifier_that_also_truncates')],
          value: 'x',
          onChanged: (_) {},
        ), stress: true, maxWidth: 200),
    GallerySpecimen('海量选项', (_) => const _DropdownDemo(initial: '0', massive: true), stress: true),
  ]),
]);

// ── G2 — Feedback states ──
final GalleryCategory _g2Feedback = GalleryCategory('反馈态 Feedback', AnIcons.info, [
  GalleryItem('AnCallout', '通栏语气提示条:图标 + 文案 + 动作 + 关闭', [
    GallerySpecimen('info', (_) => const AnCallout('Heads up — this workflow has unsaved changes.'), span: true),
    GallerySpecimen('ok', (_) => const AnCallout('Saved. Your changes are live.', severity: AnCalloutSeverity.ok), span: true),
    GallerySpecimen('warn', (_) => const AnCallout('The sandbox runtime is not installed yet.', severity: AnCalloutSeverity.warn), span: true),
    GallerySpecimen('danger', (_) => const AnCallout('Deploy failed — the trigger could not reach the handler.', severity: AnCalloutSeverity.danger), span: true),
    GallerySpecimen('title + body', (_) => const AnCallout('Re-run skipped nodes, or replay the whole flow from the failed step.', title: 'Run finished with 2 failures', severity: AnCalloutSeverity.warn), span: true),
    GallerySpecimen('actions + dismiss', (_) => AnCallout('An update is available.', actions: [AnButton(label: 'Update', size: AnButtonSize.sm, variant: AnButtonVariant.primary, onPressed: () {}), AnButton(label: 'Later', size: AnButtonSize.sm, onPressed: () {})], onDismiss: () {}), span: true),
    GallerySpecimen('超长换行', (_) => const AnCallout('A deliberately very long callout message that must wrap onto multiple lines while the leading icon stays pinned to the top of the first line and the bar grows in height instead of overflowing or truncating the text.', severity: AnCalloutSeverity.danger), stress: true, maxWidth: 260),
    GallerySpecimen('注入转义', (_) => const AnCallout('<b>not</b> & <i>html</i>', severity: AnCalloutSeverity.warn), stress: true, span: true),
  ]),
  GalleryItem('AnState', '空/载/错 整块占位:图标 + 标题 + 提示 + 动作', [
    GallerySpecimen('empty', (_) => AnState(kind: AnStateKind.empty, title: 'No functions yet', hint: 'Create your first Function to get started.', action: AnButton(label: 'New Function', icon: AnIcons.function, variant: AnButtonVariant.primary, onPressed: () {})), span: true),
    GallerySpecimen('loading', (_) => const AnState(kind: AnStateKind.loading, title: 'Loading…'), span: true),
    GallerySpecimen('error', (_) => AnState(kind: AnStateKind.error, title: "Couldn't load entities", hint: 'Check the backend is running, then try again.', action: AnButton(label: 'Try again', onPressed: () {})), span: true),
    GallerySpecimen('inset (嵌入)', (_) => const AnState(kind: AnStateKind.empty, size: AnStateSize.inset, title: 'Nothing here', hint: 'This panel has no content.'), span: true),
    GallerySpecimen('超长换行', (_) => const AnState(kind: AnStateKind.error, title: 'A long error title that should wrap and stay centered without overflowing', hint: 'An equally long explanatory hint sentence that must wrap onto several centered lines within the capped content column and never overflow.'), stress: true, maxWidth: 260),
  ]),
  GalleryItem('AnStepper', '步骤进度:done/current/upcoming(1-based,可点跳回)', [
    GallerySpecimen('dots (2/4)', (_) => const AnStepper(count: 4, current: 2)),
    GallerySpecimen('numbered (2/4)', (_) => const AnStepper(count: 4, current: 2, variant: AnStepperVariant.numbered)),
    GallerySpecimen('numbered + labels', (_) => const AnStepper(count: 3, current: 2, variant: AnStepperVariant.numbered, labels: ['Setup', 'Configure', 'Review']), span: true),
    GallerySpecimen('可点 (onStepTap)', (_) => AnStepper(count: 4, current: 3, variant: AnStepperVariant.numbered, onStepTap: (_) {})),
    GallerySpecimen('all done (4/3)', (_) => const AnStepper(count: 3, current: 4, variant: AnStepperVariant.numbered)),
    GallerySpecimen('海量步 (4/10)', (_) => const AnStepper(count: 10, current: 4), stress: true, maxWidth: 200),
  ]),
]);

// ── small stateful demo wrappers (specimens need live state) 小型有态演示包 ──

// final (not const): AnIcons.* are runtime IconData (thin-weight family). 非 const:图标是运行期 IconData。
final List<AnDropdownOption<String>> _entityOptions = [
  AnDropdownOption(value: 'fn', label: 'Function', meta: 'fn_3a9f', icon: AnIcons.function),
  AnDropdownOption(value: 'hd', label: 'Handler', meta: 'hd_71c2', icon: AnIcons.handler),
  AnDropdownOption(value: 'ag', label: 'Agent', meta: 'ag_0e88', icon: AnIcons.agent),
  AnDropdownOption(value: 'wf', label: 'Workflow', meta: 'wf_4d10', icon: AnIcons.workflow),
];

// Single-value options (label only, no meta) — the common case for a plain select. 单值(仅标签、无 meta)。
final List<AnDropdownOption<String>> _simpleOptions = const [
  AnDropdownOption(value: 'low', label: 'Low'),
  AnDropdownOption(value: 'med', label: 'Medium'),
  AnDropdownOption(value: 'high', label: 'High'),
];

class _DropdownDemo extends StatefulWidget {
  const _DropdownDemo({
    this.initial,
    this.ghost = false,
    this.block = false,
    this.massive = false,
    this.simple = false,
  });

  final String? initial;
  final bool ghost;
  final bool block;
  final bool massive;

  /// Single-value options (no meta). 单值选项(无 meta)。
  final bool simple;

  @override
  State<_DropdownDemo> createState() => _DropdownDemoState();
}

class _DropdownDemoState extends State<_DropdownDemo> {
  late String? _value = widget.initial;

  @override
  Widget build(BuildContext context) {
    final options = widget.massive
        ? [for (var i = 0; i < 80; i++) AnDropdownOption(value: '$i', label: 'Option number $i', meta: 'opt_$i')]
        : (widget.simple ? _simpleOptions : _entityOptions);
    return AnDropdown<String>(
      options: options,
      value: _value,
      variant: widget.ghost ? AnDropdownVariant.ghost : AnDropdownVariant.normal,
      menuAlignEnd: widget.ghost,
      block: widget.block,
      onChanged: (v) => setState(() => _value = v),
    );
  }
}
