import 'package:flutter/widgets.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../core/design/colors.dart';
import '../../core/design/tokens.dart';
import '../../core/design/typography.dart';
import '../../core/ui/ui.dart';
import 'specimen.dart';

// Gallery catalog — dev-only tool, so plain strings here are exempt from the i18n rule (like test
// code; never shipped). Grows one category per build group (G0–G6).
// 画廊目录——dev-only 工具,此处明文串豁免 i18n 规则(同测试代码,永不发布)。每组追加一类目。
final List<GalleryCategory> galleryCatalog = [
  _g1Controls,
  _g2Feedback,
  _g3RowsCards,
  _g4NavShell,
  _g5CodeData,
  _g6Overlays,
  _chatRail,
];

// ── Chat — the conversation rail's row, in every state ──
// The row is an AnRow composition (no new primitive): a lead status dot + single-line title + a
// trailing relative-time meta that swaps to a ⋯ menu on hover. The dot encodes the live signal —
// generating (blue, breathing) / awaiting-input (amber) / unread (green) / archived (gray marker) /
// none — via conversationDot() in the feature; here each state is shown explicitly for visual review,
// at rail width so truncation reads true. (Plain dev strings, i18n-exempt like the rest of the catalog.)
// Chat——会话 rail 的行,每态一格。行是 AnRow 组合(非新原语):前导状态点 + 单行标题 + 尾部相对时间(hover 换 ⋯ 菜单)。
// 点编码活态(生成蓝呼吸/等你琥珀/未读绿/归档灰/无),feature 里走 conversationDot();此处显式列每态供视觉审,按 rail 宽显以真截断。
const double _railW = 260;
final GalleryCategory _chatRail = GalleryCategory('对话 Chat', AnIcons.chat, [
  GalleryItem('会话行 Conversation row', '状态点 · 标题 · 时间/⋯;rail 列表的一行', [
    GallerySpecimen('普通 (无点)', (_) => const AnRow(label: '周报初稿整理', meta: '1 小时前'), span: true, maxWidth: _railW),
    GallerySpecimen('生成中 (蓝呼吸)', (_) => const AnRow(dot: AnStatus.run, label: '竞品日报流程', meta: '刚刚'), span: true, maxWidth: _railW),
    GallerySpecimen('等你输入 (琥珀)', (_) => const AnRow(dot: AnStatus.wait, label: '诊断 · flowrun frn_8a1c 失败', meta: '25 分前'), span: true, maxWidth: _railW),
    GallerySpecimen('答完未读 (绿)', (_) => const AnRow(dot: AnStatus.done, label: 'AI 编辑 · sync_inventory 加重试', meta: '10 分前'), span: true, maxWidth: _railW),
    GallerySpecimen('已归档 (灰标记)', (_) => const AnRow(dot: AnStatus.idle, label: '旧版迁移笔记', meta: '上月'), span: true, maxWidth: _railW),
    GallerySpecimen('选中', (_) => const AnRow(selected: true, dot: AnStatus.done, label: '周会纪要整理', meta: '昨天'), span: true, maxWidth: _railW),
    GallerySpecimen('带 ⋯ 菜单 (hover 显)', (_) => AnRow(label: 'API key 轮换排查', meta: '3 小时前', actions: [AnButton.iconOnly(AnIcons.more, semanticLabel: 'More', onPressed: () {})]), span: true, maxWidth: _railW),
    GallerySpecimen('超长截断', (_) => const AnRow(label: '一个非常非常长的对话标题，应当省略号截断而不撑破侧栏宽度无限延伸下去', meta: '3 天前'), stress: true, span: true, maxWidth: _railW),
    GallerySpecimen('注入转义', (_) => const AnRow(label: '<b>not</b> & <i>html</i> 注入标题', meta: '上周'), stress: true, span: true, maxWidth: _railW),
  ]),
]);

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
  GalleryItem('AnSkeleton', '加载骨架:扫光(降级=静态);row/card/text/lines', [
    GallerySpecimen('text', (_) => const AnSkeleton.text(), span: true),
    GallerySpecimen('lines (3)', (_) => const AnSkeleton.lines(3), span: true),
    GallerySpecimen('row', (_) => const AnSkeleton.row(), span: true),
    GallerySpecimen('card', (_) => const AnSkeleton.card(), span: true),
  ]),
  GalleryItem('AnTypewriter', '打字机:type→hold→delete→循环(降级=静态主句)', [
    GallerySpecimen('cycling', (_) => const AnTypewriter(['Build agents.', 'Wire workflows.', 'Ship faster.'])),
    GallerySpecimen('welcome (accent, 不循环)', (_) => const AnTypewriter(['Welcome to Anselm'], loop: false, accentCaret: true)),
    GallerySpecimen('emoji 字素', (_) => const AnTypewriter(['Ready 👋🏽 to go'], loop: false)),
    GallerySpecimen('无光标', (_) => const AnTypewriter(['No caret here'], loop: false, showCaret: false)),
  ]),
  GalleryItem('AnTags', '可编辑标签集:药丸 + 健康点 + 内联添加(重复拒+闪)', [
    GallerySpecimen('multi (可编辑)', (_) => const _TagsDemo(initial: ['agent', 'workflow']), span: true),
    GallerySpecimen('single (单值)', (_) => const _TagsDemo(initial: ['medium'], single: true)),
    GallerySpecimen('空 (placeholder)', (_) => const _TagsDemo()),
    GallerySpecimen('readOnly + tone/health', (_) => const AnTags(readOnly: true, tags: [AnTag('passed', tone: AnTone.ok, health: AnStatus.done), AnTag('pending', tone: AnTone.warn, health: AnStatus.wait), AnTag('failed', tone: AnTone.danger, health: AnStatus.err)]), span: true),
    GallerySpecimen('超长换行', (_) => const _TagsDemo(initial: ['a-very-long-tag', 'another-long-one', 'third', 'fourth-tag', 'fifth']), stress: true, maxWidth: 220),
  ]),
]);

// ── G3 — Rows & cards ──
final GalleryCategory _g3RowsCards = GalleryCategory('行与卡 Rows & Cards', AnIcons.entities, [
  GalleryItem('AnRefPill', '实体提及药丸:类型图标 + 文案;id 非空可点(派 {kind,id})、空=纯标注', [
    GallerySpecimen('agent (可点)', (_) => AnRefPill(kind: 'agent', id: 'ag_1', label: 'deploy-bot', onTap: (_) {})),
    GallerySpecimen('function', (_) => AnRefPill(kind: 'function', id: 'fn_1', label: 'normalize-input', onTap: (_) {})),
    GallerySpecimen('workflow', (_) => AnRefPill(kind: 'workflow', id: 'wf_1', label: 'nightly-deploy', onTap: (_) {})),
    GallerySpecimen('document (纯提及)', (_) => const AnRefPill(kind: 'document', label: 'spec.md')),
    GallerySpecimen('approval (可点)', (_) => AnRefPill(kind: 'approval', id: 'apf_1', label: 'deploy-gate', onTap: (_) {})),
    GallerySpecimen('纯标注 (无 id 不可点)', (_) => const AnRefPill(kind: 'handler', label: 'on-webhook')),
    GallerySpecimen('未知 kind (兜底 ?)', (_) => const AnRefPill(kind: 'quasar', label: 'unknown-kind'), stress: true),
    GallerySpecimen('超长截断', (_) => const AnRefPill(kind: 'workflow', label: 'an-extremely-long-entity-reference-name-that-must-ellipsis-within-its-cap'), stress: true, maxWidth: 180),
    GallerySpecimen('注入转义', (_) => const AnRefPill(kind: 'agent', label: '<b>not</b> & <i>html</i>'), stress: true),
  ]),
  GalleryItem('AnSection', '小节:caption/plain 标题 + 无边内容 + 右侧 actions', [
    GallerySpecimen('caption + body', (_) => const AnSection(label: 'Inputs', children: [AnInput(placeholder: 'name'), AnInput(placeholder: 'value')]), span: true),
    GallerySpecimen('caption + actions', (_) => AnSection(label: 'Environment', actions: [AnButton(label: 'Add', size: AnButtonSize.sm, icon: AnIcons.enter, onPressed: () {})], children: const [AnBadge('NODE_ENV=prod'), AnBadge('REGION=us')]), span: true),
    GallerySpecimen('plain (文档标题)', (_) => const AnSection(label: 'Overview', variant: AnSectionVariant.plain, children: [Text('A document-tier section leans on whitespace, not rule lines.')]), span: true),
    GallerySpecimen('actions-only head', (_) => AnSection(actions: [AnButton(label: 'New', size: AnButtonSize.sm, onPressed: () {})], children: const [Text('Head with no label still renders its actions.')]), span: true),
    GallerySpecimen('超长 label 截断', (_) => const AnSection(label: 'a very long section caption that must ellipsis instead of wrapping the head', children: [Text('body')]), stress: true, maxWidth: 220),
    GallerySpecimen('注入转义', (_) => const AnSection(label: '<b>not</b> & <i>html</i>', children: [Text('escaped label')]), stress: true, span: true),
    GallerySpecimen('grid (并排块)', (_) => const AnSection(label: 'Blocks', grid: true, children: [_GridCell('inputs'), _GridCell('outputs'), _GridCell('environment')]), span: true),
  ]),
  GalleryItem('AnAutoGrid', '响应式块网格:auto-fit 等宽列(1fr 填满)、窄塌 1 列、每行按内容高', [
    GallerySpecimen('auto-fit (6 块·变高)', (_) => const AnAutoGrid(children: [_GridCell('input', height: 72), _GridCell('output', height: 48), _GridCell('env', height: 96), _GridCell('schedule', height: 56), _GridCell('triggers', height: 64), _GridCell('mounts', height: 40)]), span: true),
    GallerySpecimen('少块拉伸 (2 块·1fr)', (_) => const AnAutoGrid(children: [_GridCell('left'), _GridCell('right')]), span: true),
    GallerySpecimen('单卡 (1 块拉满)', (_) => const AnAutoGrid(children: [_GridCell('only')]), span: true),
    GallerySpecimen('海量 (12 块·多行流)', (_) => AnAutoGrid(children: [for (var i = 0; i < 12; i++) _GridCell('blk $i', height: (40 + (i % 4) * 16).toDouble())]), stress: true, span: true),
    GallerySpecimen('窄塌 1 列', (_) => const AnAutoGrid(children: [_GridCell('a'), _GridCell('b')]), stress: true, maxWidth: 200, span: true),
    // (空/0 块 走单测——它渲 SizedBox.shrink,matrix 的 render-exists 断言天然不容空 specimen。)
  ]),
  GalleryItem('AnKv', '紧凑定义列表:key 左·value 右;可编辑行就地编辑(铅笔→框/下拉)', [
    GallerySpecimen('可编辑 + 只读混排', (_) => const _KvDemo(rows: [
          AnKvRow('Name', 'normalize-input', editable: true),
          AnKvRow('Created', '2026-06-24'),
          AnKvRow('Effort', 'medium', editable: true, editor: AnEditKind.select, options: _effortOptions),
        ]), span: true),
    GallerySpecimen('只读展示', (_) => const AnKv(rows: [
          AnKvRow('Kind', 'function'),
          AnKvRow('Owner', null),
          AnKvRow('Version', 'v3'),
        ]), span: true),
    GallerySpecimen('mono (id/hash)', (_) => const AnKv(mono: true, rows: [
          AnKvRow('Run', 'run_3a9f0e88'),
          AnKvRow('Hash', 'a1b2c3d4e5f6'),
        ]), span: true),
    GallerySpecimen('wrap (长值换行)', (_) => const _KvDemo(wrap: true, rows: [
          AnKvRow('Description', 'A deliberately long value that should wrap onto several lines instead of truncating.', editable: true),
        ]), span: true),
    GallerySpecimen('超长截断', (_) => const AnKv(rows: [
          AnKvRow('an-extremely-long-key-name-that-must-ellipsis', 'and-an-equally-long-value-that-also-truncates-on-the-right'),
        ]), stress: true, maxWidth: 240, span: true),
  ]),
  GalleryItem('AnEditableValue', '双锚就地编辑核(被 Kv/Field 消费;此处直展编辑态)', [
    GallerySpecimen('editing (input)', (_) => const _EditableDemo(value: '0.85', startEditing: true), span: true),
    GallerySpecimen('select (常驻下拉)', (_) => const _EditableDemo(value: 'medium', editor: AnEditKind.select, options: _effortOptions), span: true),
  ]),
  GalleryItem('AnField', '键值大行:label(+hint)左 + 值右;可编辑 / 只读 / 控件槽 三态', [
    GallerySpecimen('可编辑值', (_) => const _FieldDemo(label: 'Name', value: 'normalize-input'), span: true),
    GallerySpecimen('label + hint', (_) => const _FieldDemo(label: 'Timeout', hint: 'seconds before the run is aborted', value: '30'), span: true),
    GallerySpecimen('select 可编辑', (_) => const _FieldDemo(label: 'Effort', value: 'medium', editor: AnEditKind.select, options: _effortOptions), span: true),
    GallerySpecimen('只读', (_) => const AnField(label: 'Kind', value: 'function'), span: true),
    GallerySpecimen('控件槽 (value=null)', (_) => const AnField(label: 'Visibility', child: _DropdownDemo(initial: 'med', simple: true)), span: true),
    GallerySpecimen('超长 label/value', (_) => const _FieldDemo(label: 'an-extremely-long-field-label-that-must-ellipsis', value: 'and-a-very-long-value-that-truncates-on-the-right'), stress: true, maxWidth: 280, span: true),
  ]),
  GalleryItem('AnCard', '有边卡片容器:normal / accent / 可选 / 紧凑', [
    GallerySpecimen('normal', (_) => const AnCard(child: Text('A bordered card collects settings / MCP config / onboarding choices.')), span: true),
    GallerySpecimen('accent 变体', (_) => const AnCard(variant: AnCardVariant.accent, child: Text('Editing — accent border draws focus.')), span: true),
    GallerySpecimen('可选 (点选)', (_) => const _CardSelectDemo(), span: true),
    GallerySpecimen('selected (静态·2px accent 边)', (_) => AnCard(selectable: true, selected: true, onSelect: () {}, child: const Text('Selected card')), span: true),
    GallerySpecimen('row child (横向卡)', (_) => AnCard(child: Row(children: [Icon(AnIcons.mcp, size: AnSize.icon), const SizedBox(width: AnSpace.s8), const Expanded(child: Text('Horizontal card — compose a Row as the child')), AnButton(label: 'Edit', size: AnButtonSize.sm, onPressed: () {})])), span: true),
    GallerySpecimen('pad=tight', (_) => const AnCard(pad: AnCardPad.tight, child: Text('Tight inset')), span: true),
  ]),
  GalleryItem('AnInfoCard', '无边信息单元:head(icon+title+meta)+ body + actions', [
    GallerySpecimen('head + body', (_) => AnInfoCard(
          title: 'Schedule',
          icon: AnIcons.scheduler,
          meta: 'UTC',
          child: const AnKv(rows: [AnKvRow('Cron', '0 0 * * *'), AnKvRow('Next run', 'in 3h')]),
        ), span: true),
    GallerySpecimen('title + actions', (_) => AnInfoCard(
          title: 'Environment',
          actions: [AnButton(label: 'Edit', size: AnButtonSize.sm, onPressed: () {})],
          child: const AnKv(rows: [AnKvRow('NODE_ENV', 'production'), AnKvRow('REGION', 'us-east')]),
        ), span: true),
    GallerySpecimen('无 head (body only)', (_) => const AnInfoCard(child: Text('A headless info unit — just body content, organised by whitespace.')), span: true),
    GallerySpecimen('超长 title + meta', (_) => const AnInfoCard(title: 'an-extremely-long-info-card-title-that-must-ellipsis', meta: 'and-a-long-meta', child: Text('body')), stress: true, maxWidth: 260, span: true),
  ]),
  GalleryItem('AnRow', '核心行:lead(dot/icon↔chevron)+ label + trail(meta↔hover actions)', [
    GallerySpecimen('icon + meta', (_) => AnRow(icon: AnIcons.function, label: 'normalize-input', meta: '2m ago', onSelect: () {}), span: true),
    GallerySpecimen('dot + hover actions', (_) => AnRow(dot: AnStatus.run, label: 'nightly-deploy', meta: '12s', actions: [AnButton.iconOnly(AnIcons.stop, semanticLabel: 'Stop', onPressed: () {})], onSelect: () {}), span: true),
    GallerySpecimen('selected', (_) => AnRow(icon: AnIcons.agent, label: 'deploy-bot', meta: 'active', selected: true, onSelect: () {}), span: true),
    GallerySpecimen('emphatic selected (run 看板)', (_) => AnRow(dot: AnStatus.done, label: 'run #4821', meta: 'passed', selected: true, emphatic: true, onSelect: () {}), span: true),
    GallerySpecimen('collapsible (tree)', (_) => const _CollapsibleRowDemo(), span: true),
    GallerySpecimen('hint (多行)', (_) => AnRow(icon: AnIcons.handler, label: 'on-webhook', hint: 'Fires when an external HTTP request hits the mounted path.', onSelect: () {}), span: true),
    GallerySpecimen('depth + mono', (_) => AnRow(dot: AnStatus.idle, label: 'fn_3a9f0e88', mono: true, depth: 2, onSelect: () {}), span: true),
    GallerySpecimen('passive', (_) => AnRow(icon: AnIcons.doc, label: 'read-only annotation', passive: true), span: true),
    GallerySpecimen('超长 label', (_) => AnRow(icon: AnIcons.workflow, label: 'an-extremely-long-row-label-that-must-ellipsis-not-overflow-the-row', meta: 'now', onSelect: () {}), stress: true, maxWidth: 240, span: true),
  ]),
  GalleryItem('AnRowDetail', '可展开详情行:点行展开下方详情(AnExpandReveal 高度揭示)', [
    GallerySpecimen('expandable (点开/收起)', (_) => const _RowDetailDemo(), span: true),
  ]),
  GalleryItem('AnThinTable', '对齐多列(非表格、无 chrome):首列吃富余 + 其余贴内容、表头灰 meta', [
    GallerySpecimen('对齐多列 (只读)', (_) => const AnThinTable(columns: _tableCols, rows: _tableRows), span: true),
    GallerySpecimen('可点选行', (_) => AnThinTable(columns: _tableCols, rows: _tableRows, selectable: true, onRowTap: (_) {}), span: true),
    GallerySpecimen('海量行 (流)', (_) => AnThinTable(columns: _tableCols, rows: [for (var i = 0; i < 14; i++) {'name': 'job-$i', 'kind': 'function', 'runs': '$i'}]), stress: true, span: true),
    GallerySpecimen('超长格截断', (_) => const AnThinTable(columns: _tableCols, rows: [
          {'name': 'an-extremely-long-entity-name-that-must-ellipsis', 'kind': 'workflow', 'runs': '999999'},
        ]), stress: true, maxWidth: 240, span: true),
  ]),
]);

// ── G4 — Navigation & shell ──
final GalleryCategory _g4NavShell = GalleryCategory('导航与壳 Nav & Shell', AnIcons.grip, [
  GalleryItem('AnToolbar', '三区工具条:左附件 | 标题+meta | 右动作(非卡;bordered=顶栏)', [
    GallerySpecimen('default', (_) => AnToolbar(
          title: 'normalize-input',
          meta: 'function',
          trailing: [
            AnButton.iconOnly(AnIcons.run, semanticLabel: 'Run', onPressed: () {}),
            AnButton.iconOnly(AnIcons.more, semanticLabel: 'More', onPressed: () {}),
          ],
        ), span: true),
    GallerySpecimen('bordered (top bar)', (_) => AnToolbar(
          bordered: true,
          leading: [AnButton.iconOnly(AnIcons.search, semanticLabel: 'Search', onPressed: () {})],
          title: 'Settings',
          trailing: [AnButton(label: 'Save', size: AnButtonSize.sm, variant: AnButtonVariant.primary, onPressed: () {})],
        ), span: true),
    GallerySpecimen('compact + meta', (_) => const AnToolbar(compact: true, title: 'Versions', meta: '12 revisions'), span: true),
    GallerySpecimen('center slot (custom main)', (_) => AnToolbar(
          leading: [AnButton.iconOnly(AnIcons.sliders, semanticLabel: 'Options', onPressed: () {})],
          center: AnButton(label: 'custom main', size: AnButtonSize.sm, onPressed: () {}),
          trailing: [AnButton.iconOnly(AnIcons.more, semanticLabel: 'More', onPressed: () {})],
        ), span: true),
    GallerySpecimen('超长标题截断(动作不挤出)', (_) => AnToolbar(
          title: 'an-extremely-long-toolbar-title-that-must-ellipsis-and-never-push-the-actions-off',
          meta: 'fn',
          trailing: [AnButton.iconOnly(AnIcons.more, semanticLabel: 'More', onPressed: () {})],
        ), stress: true, maxWidth: 280, span: true),
  ]),
  GalleryItem('AnInspector', '右岛内容壳:head(icon+title)+ 滚动块流 body(不画岛皮,AnIsland 供)· headless 占满自管', [
    GallerySpecimen('head + body (in island)', (_) => SizedBox(
          width: 320,
          child: AnIsland(
            padding: EdgeInsets.zero,
            child: AnInspector(
              title: 'normalize-input',
              icon: AnIcons.function,
              children: const [
                AnInfoCard(title: 'Overview', child: AnKv(rows: [AnKvRow('Kind', 'function'), AnKvRow('Created', '2026-06-24')])),
                AnInfoCard(title: 'Source', child: AnKv(rows: [AnKvRow('Lines', '42'), AnKvRow('Lang', 'CEL')])),
              ],
            ),
          ),
        ), height: 360, span: true),
    GallerySpecimen('headless (slot fills + self-manages)', (_) => const SizedBox(
          width: 320,
          child: AnIsland(
            padding: EdgeInsets.zero,
            child: AnInspector(
              headless: true,
              child: AnState(kind: AnStateKind.empty, size: AnStateSize.inset, title: 'Headless', hint: 'slot self-draws its head + scroll'),
            ),
          ),
        ), height: 220, span: true),
    GallerySpecimen('超长标题 + 多卡滚动', (_) => SizedBox(
          width: 320,
          child: AnIsland(
            padding: EdgeInsets.zero,
            child: AnInspector(
              title: 'an-extremely-long-inspector-title-that-must-ellipsis-in-the-head',
              icon: AnIcons.workflow,
              children: [
                for (final t in const ['Overview', 'Source', 'Versions', 'History'])
                  AnInfoCard(title: t, child: const AnKv(rows: [AnKvRow('k', 'v')])),
              ],
            ),
          ),
        ), height: 200, stress: true, span: true),
  ]),
  GalleryItem('AnPage', '海洋记录页:唯一滚动区 + 居中 720 内容列 + overlay 滚动条(头净空 pad)', [
    GallerySpecimen('page (scroll + centered col)', (_) => AnPage(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            mainAxisSize: MainAxisSize.min,
            spacing: AnSpace.s16,
            children: [
              for (final t in const ['Overview', 'Source', 'Versions', 'History', 'Runs', 'Settings'])
                AnInfoCard(title: t, child: const AnKv(rows: [AnKvRow('key', 'value'), AnKvRow('another', 'thing')])),
            ],
          ),
        ), height: 360, span: true),
  ]),
  GalleryItem('AnMenu', '浮层菜单(on AnPopover):分组小标题 + icon/check/meta + danger/disabled(多选 keepOpen)', [
    GallerySpecimen('actions menu (⋯) — tap to open', (_) => const _MenuActionsDemo()),
    GallerySpecimen('sliders (multi-check, keepOpen)', (_) => const _MenuSlidersDemo()),
    GallerySpecimen('match anchor width (workspace 切换器) — tap to open', (_) => const _MenuMatchWidthDemo(), span: true),
  ]),
  GalleryItem('AnTabs', '文字下划线切换器:灰→选中黑 + 弹簧下划线;IndexedStack panes 隐藏不销毁;多 tab 横滚', [
    GallerySpecimen('tabs (underline + panes)', (_) => const _TabsDemo(), height: 220, span: true),
    GallerySpecimen('many tabs (horizontal scroll)', (_) => const _TabsDemo(many: true), height: 200, span: true),
  ]),
  GalleryItem('AnSidebarList', '左岛侧栏:New + 域内过滤(sliders 菜单)+ groups→types→rows 递归树(文档树可折叠)', [
    GallerySpecimen('sidebar (filter + tree + select)', (_) => const _SidebarDemo(), height: 420, span: true),
  ]),
  GalleryItem('AnOceanHeader', '海洋页头:面包屑 + H2 标题(可就地改名)+ 右动作 + meta', [
    GallerySpecimen('editable (crumb + H2 + actions + meta)', (_) => const _OceanHeaderDemo(), span: true),
    GallerySpecimen('read-only', (_) => AnOceanHeader(
          crumbs: const ['Docs', 'API'],
          title: 'reference.md',
          actions: [AnButton.iconOnly(AnIcons.more, semanticLabel: 'More', onPressed: () {})],
          meta: const [AnBadge('document', tone: AnTone.accent)],
        ), span: true),
  ]),
  GalleryItem('AnOceanSwitcher', '左岛海洋切换器:选中展开标签 + 匹配几何滑动(旧的收起、新的展开,单药丸滑动+变宽,整行回流);降级=瞬切', [
    GallerySpecimen('interactive (点不同海洋,看滑动)', (_) => const _OceanSwitcherDemo(), span: true),
    GallerySpecimen('rest · Chat', (_) => AnOceanSwitcherFrame(items: _oceanItems, fromIndex: 0, toIndex: 0, t: 1), span: true),
    GallerySpecimen('rest · Documents', (_) => AnOceanSwitcherFrame(items: _oceanItems, fromIndex: 3, toIndex: 3, t: 1), span: true),
    GallerySpecimen('frozen · 滑动中 (t=0.5)', (_) => AnOceanSwitcherFrame(items: _oceanItems, fromIndex: 0, toIndex: 1, t: 0.5), span: true),
    GallerySpecimen('rest · 无选中 (设置/通知激活)', (_) => AnOceanSwitcherFrame(items: _oceanItems, fromIndex: -1, toIndex: -1, t: 1), span: true),
    GallerySpecimen('frozen · 取消选中淡出 (t=0.5)', (_) => AnOceanSwitcherFrame(items: _oceanItems, fromIndex: 1, toIndex: -1, t: 0.5), span: true),
    GallerySpecimen('超长标签 (clip 不溢出)', (_) => AnOceanSwitcherFrame(items: _longOceanItems, fromIndex: 1, toIndex: 1, t: 1), stress: true, maxWidth: 240, span: true),
  ]),
  GalleryItem('AnWorkspaceButton', '左岛底栏 workspace 触发钮:头像 + 名字(省略)+ chevron;点开快捷操作菜单(isOpen 高亮 + chevron 翻转)', [
    GallerySpecimen('default', (_) => SizedBox(width: 200, child: AnWorkspaceButton(name: 'Personal', onTap: () {})), span: true),
    GallerySpecimen('open (菜单展开态)', (_) => SizedBox(width: 200, child: AnWorkspaceButton(name: 'Personal', isOpen: true, onTap: () {})), span: true),
    GallerySpecimen('超长名字 (省略)', (_) => SizedBox(width: 180, child: AnWorkspaceButton(name: 'A very long workspace name that must ellipsize', onTap: () {})), stress: true, span: true),
  ]),
  GalleryItem('AnSidebarFooter', '左岛底栏:workspace 菜单 | 设置格 | 通知格(两格独立高亮 + 未读红点)', [
    GallerySpecimen('idle', (_) => _footerDemo(), span: true),
    GallerySpecimen('settings active (齿轮高亮)', (_) => _footerDemo(settingsActive: true), span: true),
    GallerySpecimen('notifications active + 红点', (_) => _footerDemo(notificationsActive: true, unread: 3), span: true),
  ]),
]);

// AnSidebarFooter demo helper — a footer with a sample workspace button. 底栏演示:带样例 workspace 钮。
AnSidebarFooter _footerDemo({bool settingsActive = false, bool notificationsActive = false, int unread = 0}) =>
    AnSidebarFooter(
      workspace: AnWorkspaceButton(name: 'Personal', onTap: () {}),
      settingsLabel: 'Settings',
      notificationsLabel: 'Notifications',
      onSettings: () {},
      onNotifications: () {},
      settingsActive: settingsActive,
      notificationsActive: notificationsActive,
      unreadCount: unread,
    );

// ── G5 — Code & data ──
const _pyCode = 'def normalize(input):\n'
    '    # strip + lowercase the name\n'
    '    name = input.get("name", "").strip()\n'
    '    return {"name": name.lower(), "len": len(name)}';
const _celCode = 'has(input.user)\n'
    '  ? input.user.name\n'
    '  : "anonymous"  // {{ default }} fallback';
const _jsonCode = '{\n'
    '  "id": "fn_3a9f",\n'
    '  "active": true,\n'
    '  "retries": 3,\n'
    '  "tags": ["prod", "io"]\n'
    '}';
const _flowrunNode = {
  'nodeId': 'normalize',
  'status': 'completed',
  'iteration': 0,
  'result': {'name': 'john', 'len': 4, 'valid': true, 'score': 0.92},
  'tags': ['prod', 'io', 'fast'],
  'startedAt': '2026-06-24T10:00:00Z',
  'error': null,
};
const _deepJson = {
  'a': {
    'b': {
      'c': {
        'd': {'e': 'deep', 'n': 42}
      }
    }
  }
};
const _diffBefore = 'def f(x):\n'
    '    # add one\n'
    '    return x + 1';
const _diffAfter = 'def f(x, y):\n'
    '    # add two numbers\n'
    '    return x + y\n'
    '    # done';

final GalleryCategory _g5CodeData = GalleryCategory('代码与数据 Code & Data', AnIcons.function, [
  GalleryItem('AnCodeEditor', '唯一代码块/轻编辑:高亮 + 行号 + 顶栏;只读/可编辑/内联/换行', [
    GallerySpecimen('python 只读', (_) => const AnCodeEditor(code: _pyCode, lang: 'py'), span: true),
    GallerySpecimen('CEL (插值上色)', (_) => const AnCodeEditor(code: _celCode, lang: 'cel'), span: true),
    GallerySpecimen('json 只读', (_) => const AnCodeEditor(code: _jsonCode, lang: 'json'), span: true),
    GallerySpecimen('可编辑 (铅笔→编辑)', (_) => const _CodeEditDemo(), span: true),
    GallerySpecimen('compact', (_) => const AnCodeEditor(code: _celCode, lang: 'cel', compact: true), span: true),
    GallerySpecimen('inline (无框)', (_) => const AnCodeEditor(code: 'input.x > 0 && has(node.y)', lang: 'cel', inline: true), span: true),
    // wrap: long lines reflow; v1 gutter is EQUAL-HEIGHT per logical line (not per visual line), so a
    // wrapped line's number doesn't track each visual row — documented v1 degrade (WRK-040). 标注:wrap 行号 v1 等高。
    GallerySpecimen('wrap (行号 v1 等高·非逐视觉行)', (_) => const AnCodeEditor(code: 'short = 1\nresult = compute_a_very_long_expression(alpha, beta, gamma) + offset_value_for_the_pipeline_stage_that_wraps\ndone = True', lang: 'py', wrap: true), span: true),
    GallerySpecimen('超长行 (横滚)', (_) => const AnCodeEditor(code: 'x = "a really long single line that exceeds the editor width and must scroll horizontally without wrapping or overflow"', lang: 'py'), stress: true, span: true),
    GallerySpecimen('空', (_) => const AnCodeEditor(code: '', lang: 'py'), stress: true, span: true),
    GallerySpecimen('海量行 (内容高、父滚动)', (_) => AnCodeEditor(code: [for (var i = 0; i < 60; i++) 'line_$i = step($i)'].join('\n'), lang: 'py'), stress: true, span: true),
    GallerySpecimen('注入转义', (_) => const AnCodeEditor(code: '<b>not</b> & <i>html</i> — \${raw}', lang: 'md'), stress: true, span: true),
  ]),
  GalleryItem('AnJsonTree', '唯一 JSON/结构化展示:可折叠树 + 类型着色(TreeSliver 虚拟化);只读', [
    GallerySpecimen('flowrun 节点结果', (_) => const AnJsonTree(data: _flowrunNode), span: true, height: 280),
    GallerySpecimen('数组根', (_) => const AnJsonTree(data: ['alpha', 'beta', 'gamma'], rootLabel: 'tags'), span: true, height: 160),
    GallerySpecimen('深嵌套 (open-depth)', (_) => const AnJsonTree(data: _deepJson, openDepth: 5), span: true, height: 220),
    GallerySpecimen('无根行 (showRoot=false)', (_) => const AnJsonTree(data: _flowrunNode, showRoot: false), span: true, height: 240),
    GallerySpecimen('标量 / null / 空', (_) => const AnJsonTree(data: {'s': '', 'n': null, 'empty': <String, Object?>{}, 'arr': <Object?>[]}), span: true, height: 180),
    GallerySpecimen('无效 JSON', (_) => const AnJsonTree(jsonString: '{ bad json,, }'), span: true, height: 80),
    GallerySpecimen('环检测 [Circular]', (_) {
      final m = <String, Object?>{'name': 'node'};
      m['self'] = m;
      return AnJsonTree(data: m, openDepth: 3);
    }, stress: true, span: true, height: 140),
    GallerySpecimen('海量键 (截断)', (_) => AnJsonTree(data: {for (var i = 0; i < 80; i++) 'key_$i': 'value_$i'}), stress: true, span: true, height: 260),
    GallerySpecimen('注入转义', (_) => const AnJsonTree(data: {'html': '<b>not</b> & <i>x</i>', 'tmpl': '\${raw} {{cel}}'}), stress: true, span: true, height: 140),
  ]),
  GalleryItem('AnVersionDiff', '版本 diff:单框 unified(增绿删红)+ 行内高亮(唯一 tokenizer);只读', [
    GallerySpecimen('范围 + 说明 + 计数', (_) => const AnVersionDiff(before: _diffBefore, after: _diffAfter, lang: 'py', range: 'v3 → v4', note: 'rename + add param'), span: true),
    GallerySpecimen('最早版本 (整段 ctx)', (_) => const AnVersionDiff(before: null, after: _diffAfter, lang: 'py', range: 'v1'), span: true),
    GallerySpecimen('CEL diff', (_) => const AnVersionDiff(before: 'has(input.x)\n  ? input.x\n  : 0', after: 'has(input.user)\n  ? input.user.name\n  : "anon"', lang: 'cel', range: 'v2 → v3'), span: true),
    GallerySpecimen('bare (无框内联)', (_) => const AnVersionDiff(before: 'a = 1', after: 'a = 2', lang: 'py', bare: true), span: true),
    GallerySpecimen('全删全增 (无公共行)', (_) => const AnVersionDiff(before: 'old line one\nold line two', after: 'new line one\nnew line two', lang: 'py', range: 'v4 → v5'), stress: true, span: true),
    GallerySpecimen('超长行 (横滚)', (_) => const AnVersionDiff(before: 'x = short', after: 'x = "a really long replacement line that exceeds the diff width and must scroll horizontally"', lang: 'py'), stress: true, span: true),
    GallerySpecimen('注入转义', (_) => const AnVersionDiff(before: '<b>old</b>', after: '<b>new</b> & \${raw}', lang: 'md', range: 'v1 → v2'), stress: true, span: true),
  ]),
]);

// ── G6 — Overlays (dialog + toast) ──
final GalleryCategory _g6Overlays = GalleryCategory('浮层 Overlays', AnIcons.more, [
  GalleryItem('AnToast', '屏角瞬时提示:tone 色条 + action + 自动消隐;命令式 showToast()', [
    // Static, sticky specimens (duration: zero → no auto-dismiss) — the chip in every tone. 静态常驻 specimen。
    GallerySpecimen('neutral', (_) => AnToast(text: '已保存', duration: Duration.zero, onDismissed: () {}), span: true),
    GallerySpecimen('ok', (_) => AnToast(text: '已保存 · flowrun fne_5e1a 运行完成', tone: AnToastTone.ok, duration: Duration.zero, onDismissed: () {}), span: true),
    GallerySpecimen('warn', (_) => AnToast(text: '已暂停调度', tone: AnToastTone.warn, duration: Duration.zero, onDismissed: () {}), span: true),
    GallerySpecimen('danger', (_) => AnToast(text: '运行失败:连接超时', tone: AnToastTone.danger, duration: Duration.zero, onDismissed: () {}), span: true),
    GallerySpecimen('含 action (撤销)', (_) => AnToast(text: '已删除「订单处理」', tone: AnToastTone.danger, action: AnToastAction(label: '撤销', onPressed: () {}), duration: Duration.zero, onDismissed: () {}), span: true),
    GallerySpecimen('超长文本 (2 行省略)', (_) => AnToast(text: 'a really long toast message that wraps to two lines and then ellipsizes when it exceeds the available width of the toast chip surface', duration: Duration.zero, onDismissed: () {}), stress: true, span: true),
    GallerySpecimen('注入转义', (_) => AnToast(text: '<b>not</b> & <i>html</i> — \${raw}', tone: AnToastTone.warn, duration: Duration.zero, onDismissed: () {}), stress: true, span: true),
    // Live trigger — fires into the real bottom-right corner overlay (auto-dismiss + soft cap). 命令式触发(真弹右下角)。
    GallerySpecimen('命令式触发 (弹到右下角)', (_) => const _ToastTriggerDemo(), span: true),
  ]),
  GalleryItem('AnDialog (confirm)', '模态确认框:遮罩 + 居中卡 + 焦点陷阱 + Escape;命令式 confirm()', [
    GallerySpecimen('危险确认 (删除)', (_) => const _DialogTriggerDemo(tone: AnDialogTone.danger), span: true),
    GallerySpecimen('主操作确认 (无说明)', (_) => const _DialogTriggerDemo(tone: AnDialogTone.primary, withMessage: false), span: true),
  ]),
]);

// AnToast command-trigger demo — fires toasts into the real corner overlay (auto-dismiss + soft cap 5).
// toast 命令式触发演示——弹进真右下角浮层(自动消隐 + 软上限 5)。
class _ToastTriggerDemo extends StatelessWidget {
  const _ToastTriggerDemo();
  @override
  Widget build(BuildContext context) {
    return Consumer(
      builder: (context, ref, _) {
        final ctrl = ref.read(overlayProvider.notifier);
        return AnActionGroup([
          AnButton(label: '弹一条', icon: AnIcons.info, onPressed: () => ctrl.showToast('已保存 · flowrun fne_5e1a 运行完成', tone: AnToastTone.ok)),
          AnButton(label: '含撤销', onPressed: () => ctrl.showToast('已删除「订单处理」', tone: AnToastTone.danger, action: AnToastAction(label: '撤销', onPressed: () => ctrl.showToast('已恢复', tone: AnToastTone.ok)))),
          AnButton(label: '连发 8 条 (验上限 5)', onPressed: () {
            for (var i = 1; i <= 8; i++) {
              ctrl.showToast('通知 #$i · 批量操作进行中');
            }
          }),
        ]);
      },
    );
  }
}

// AnDialog command-trigger demo — confirm() resolves true/false; the result is echoed as a toast.
// dialog 命令式触发演示——confirm() 返 true/false,结果回弹 toast。
class _DialogTriggerDemo extends StatelessWidget {
  const _DialogTriggerDemo({required this.tone, this.withMessage = true});
  final AnDialogTone tone;
  final bool withMessage;
  @override
  Widget build(BuildContext context) {
    final danger = tone == AnDialogTone.danger;
    return Consumer(
      builder: (context, ref, _) {
        return AnButton(
          label: danger ? '删除实体…' : '应用更改…',
          icon: danger ? AnIcons.trash : AnIcons.check,
          variant: danger ? AnButtonVariant.danger : AnButtonVariant.primary,
          onPressed: () async {
            final ctrl = ref.read(overlayProvider.notifier);
            final ok = await ctrl.confirm(
              title: danger ? '确认删除' : '应用更改',
              message: withMessage ? (danger ? '此操作不可撤销,确定删除该实体吗?' : '将把改动写入当前版本。') : null,
              confirmLabel: danger ? '删除' : '应用',
              cancelLabel: '取消',
              barrierLabel: '关闭对话框',
              confirmTone: tone,
            );
            ctrl.showToast(ok ? '已执行' : '已取消', tone: ok ? AnToastTone.ok : AnToastTone.neutral);
          },
        );
      },
    );
  }
}

// AnCodeEditor editable demo — holds the committed value across save. 可编辑代码:持保存后的值。
class _CodeEditDemo extends StatefulWidget {
  const _CodeEditDemo();
  @override
  State<_CodeEditDemo> createState() => _CodeEditDemoState();
}

class _CodeEditDemoState extends State<_CodeEditDemo> {
  String _code = _celCode;
  @override
  Widget build(BuildContext context) =>
      AnCodeEditor(code: _code, lang: 'cel', editable: true, onChanged: (v) => setState(() => _code = v));
}

// AnMenu match-anchor-width demo — a full-width dropdown (the workspace switcher pattern). 等宽下拉演示。
class _MenuMatchWidthDemo extends StatelessWidget {
  const _MenuMatchWidthDemo();
  @override
  Widget build(BuildContext context) => SizedBox(
        width: 240,
        child: AnMenu(
          matchAnchorWidth: true,
          alignEnd: false,
          anchorBuilder: (context, toggle, isOpen) =>
              AnWorkspaceButton(name: 'Personal', onTap: toggle, isOpen: isOpen),
          entries: [
            AnMenuItem(label: 'Personal', checked: true, onTap: () {}),
            AnMenuItem(label: 'New workspace', icon: AnIcons.plus, onTap: () {}),
            AnMenuItem(label: 'Workspace settings', icon: AnIcons.gear, onTap: () {}),
          ],
        ),
      );
}

// AnMenu demos (stateful: hold the picked / checked state). AnMenu 演示(持选中态)。
class _MenuActionsDemo extends StatelessWidget {
  const _MenuActionsDemo();
  @override
  Widget build(BuildContext context) => AnMenu(
        anchorBuilder: (context, toggle, isOpen) =>
            AnButton.iconOnly(AnIcons.more, semanticLabel: 'More', onPressed: toggle),
        entries: [
          AnMenuItem(label: 'Edit', icon: AnIcons.edit, onTap: () {}),
          AnMenuItem(label: 'Duplicate', icon: AnIcons.iterate, onTap: () {}),
          const AnMenuSection('Danger'),
          AnMenuItem(label: 'Delete', icon: AnIcons.trash, danger: true, onTap: () {}),
          AnMenuItem(label: 'Archive', icon: AnIcons.history, disabled: true),
        ],
      );
}

class _MenuSlidersDemo extends StatefulWidget {
  const _MenuSlidersDemo();
  @override
  State<_MenuSlidersDemo> createState() => _MenuSlidersDemoState();
}

class _MenuSlidersDemoState extends State<_MenuSlidersDemo> {
  final Set<String> _on = {'recent', 'versions'};
  void _toggle(String k) => setState(() => _on.contains(k) ? _on.remove(k) : _on.add(k));
  @override
  Widget build(BuildContext context) => AnMenu(
        anchorBuilder: (context, toggle, isOpen) =>
            AnButton(label: 'Sort', icon: AnIcons.sliders, size: AnButtonSize.sm, onPressed: toggle),
        entries: [
          const AnMenuSection('Sort'),
          AnMenuItem(label: 'Recently updated', checked: _on.contains('recent'), keepOpen: true, onTap: () => _toggle('recent')),
          AnMenuItem(label: 'Name', checked: _on.contains('name'), keepOpen: true, onTap: () => _toggle('name')),
          const AnMenuSection('Display'),
          AnMenuItem(label: 'Show versions', checked: _on.contains('versions'), keepOpen: true, onTap: () => _toggle('versions')),
          AnMenuItem(label: 'Show status dots', checked: _on.contains('dots'), keepOpen: true, onTap: () => _toggle('dots')),
        ],
      );
}

// AnOceanSwitcher demo data + interactive wrapper. 海洋切换器演示数据 + 交互包。
// final (not const): AnIcons.* are runtime IconData. 非 const:图标是运行期 IconData。
final List<AnOceanItem> _oceanItems = [
  AnOceanItem(id: 'chat', icon: AnIcons.chat, label: 'Chat'),
  AnOceanItem(id: 'entities', icon: AnIcons.entities, label: 'Entities'),
  AnOceanItem(id: 'scheduler', icon: AnIcons.scheduler, label: 'Scheduler'),
  AnOceanItem(id: 'documents', icon: AnIcons.doc, label: 'Documents'),
];

// Stress: a deliberately long label to verify the selected slot clips (no overflow). 超长标签压力。
final List<AnOceanItem> _longOceanItems = [
  AnOceanItem(id: 'chat', icon: AnIcons.chat, label: 'Chat'),
  AnOceanItem(id: 'long', icon: AnIcons.workflow, label: 'A very long ocean name that must clip'),
  AnOceanItem(id: 'docs', icon: AnIcons.doc, label: 'Docs'),
];

// AnOceanSwitcher is controlled — the demo owns the selected index so taps animate the droplet flow.
// AnOceanSwitcher 受控:演示持选中索引,点击即播水滴流转。
class _OceanSwitcherDemo extends StatefulWidget {
  const _OceanSwitcherDemo();
  @override
  State<_OceanSwitcherDemo> createState() => _OceanSwitcherDemoState();
}

class _OceanSwitcherDemoState extends State<_OceanSwitcherDemo> {
  int _sel = 0;
  @override
  Widget build(BuildContext context) => Align(
        alignment: Alignment.centerLeft,
        child: AnOceanSwitcher(
          items: _oceanItems,
          selectedIndex: _sel,
          onSelect: (i) => setState(() => _sel = i),
        ),
      );
}

// AnThinTable sample data. AnThinTable 演示数据。
const List<AnTableColumn> _tableCols = [
  AnTableColumn('name', label: 'Name'),
  AnTableColumn('kind', label: 'Kind'),
  AnTableColumn('runs', label: 'Runs', align: AnTableAlign.right),
];
const List<Map<String, String>> _tableRows = [
  {'name': 'normalize-input', 'kind': 'function', 'runs': '128'},
  {'name': 'nightly-deploy', 'kind': 'workflow', 'runs': '4821'},
  {'name': 'on-webhook', 'kind': 'handler', 'runs': '37'},
];

// dev-only grid cell: a bordered block of a given height, to show AnAutoGrid's auto-fit columns +
// per-row content height. 仅演示用的网格块(有边、定高),展示 auto-fit 列 + 每行按内容高。
class _GridCell extends StatelessWidget {
  const _GridCell(this.text, {this.height = 56});

  final String text;
  final double height;

  @override
  Widget build(BuildContext context) {
    final c = context.colors;
    return Container(
      height: height,
      alignment: Alignment.center,
      decoration: BoxDecoration(
        color: c.surface,
        borderRadius: BorderRadius.circular(AnRadius.chip),
        border: Border.all(color: c.line, width: AnSize.hairline),
      ),
      child: Text(text, style: AnText.meta.copyWith(color: c.inkMuted)),
    );
  }
}

// Shared enum options for the Kv/EditableValue select demos. Kv/EditableValue 下拉演示选项。
const List<AnDropdownOption<String>> _effortOptions = [
  AnDropdownOption(value: 'low', label: 'Low'),
  AnDropdownOption(value: 'medium', label: 'Medium'),
  AnDropdownOption(value: 'high', label: 'High'),
];

// ── small stateful demo wrappers (specimens need live state) 小型有态演示包 ──

// AnKv is controlled — the demo owns the rows so in-place edits actually mutate + rebuild. AnKv 受控。
class _KvDemo extends StatefulWidget {
  const _KvDemo({required this.rows, this.wrap = false});

  final List<AnKvRow> rows;
  final bool wrap;

  @override
  State<_KvDemo> createState() => _KvDemoState();
}

class _KvDemoState extends State<_KvDemo> {
  late List<AnKvRow> _rows = widget.rows;

  @override
  Widget build(BuildContext context) =>
      AnKv(rows: _rows, wrap: widget.wrap, onChanged: (r) => setState(() => _rows = r));
}

// AnRow collapsible demo — owns the open state so the chevron toggles. AnRow 折叠演示。
class _CollapsibleRowDemo extends StatefulWidget {
  const _CollapsibleRowDemo();

  @override
  State<_CollapsibleRowDemo> createState() => _CollapsibleRowDemoState();
}

class _CollapsibleRowDemoState extends State<_CollapsibleRowDemo> {
  bool _open = false;

  @override
  Widget build(BuildContext context) => AnRow(
        collapsible: true,
        open: _open,
        icon: AnIcons.workflow,
        label: 'nightly-deploy (click the lead to toggle)',
        meta: '5 nodes',
        onToggle: () => setState(() => _open = !_open),
        onSelect: () {},
      );
}

// AnRowDetail demo — owns open; the row tap toggles the detail panel. AnRowDetail 演示。
class _RowDetailDemo extends StatefulWidget {
  const _RowDetailDemo();

  @override
  State<_RowDetailDemo> createState() => _RowDetailDemoState();
}

class _RowDetailDemoState extends State<_RowDetailDemo> {
  bool _open = false;

  @override
  Widget build(BuildContext context) => AnRowDetail(
        open: _open,
        row: AnRow(
          collapsible: true,
          open: _open,
          icon: AnIcons.scheduler,
          label: 'Schedule (tap to expand)',
          meta: _open ? 'open' : 'closed',
          onToggle: () => setState(() => _open = !_open),
          onSelect: () => setState(() => _open = !_open),
        ),
        detail: const AnKv(rows: [
          AnKvRow('Cron', '0 0 * * *'),
          AnKvRow('Timezone', 'UTC'),
          AnKvRow('Next run', 'in 3h 12m'),
        ]),
      );
}

// AnCard selectable demo — owns the selected toggle. AnCard 可选演示,自持选中态。
class _CardSelectDemo extends StatefulWidget {
  const _CardSelectDemo();

  @override
  State<_CardSelectDemo> createState() => _CardSelectDemoState();
}

class _CardSelectDemoState extends State<_CardSelectDemo> {
  bool _sel = false;

  @override
  Widget build(BuildContext context) => AnCard(
        selectable: true,
        selected: _sel,
        onSelect: () => setState(() => _sel = !_sel),
        child: Text(_sel ? 'Selected — tap to deselect' : 'Selectable — tap to select'),
      );
}

// AnField is controlled — the demo owns the value so an in-place edit sticks. AnField 受控。
class _FieldDemo extends StatefulWidget {
  const _FieldDemo({
    required this.label,
    this.hint,
    required this.value,
    this.editor = AnEditKind.input,
    this.options = const [],
  });

  final String label;
  final String? hint;
  final String value;
  final AnEditKind editor;
  final List<AnDropdownOption<String>> options;

  @override
  State<_FieldDemo> createState() => _FieldDemoState();
}

class _FieldDemoState extends State<_FieldDemo> {
  late String _v = widget.value;

  @override
  Widget build(BuildContext context) => AnField(
        label: widget.label,
        hint: widget.hint,
        value: _v,
        editable: true,
        editor: widget.editor,
        options: widget.options,
        onChanged: (v) => setState(() => _v = v),
      );
}

// AnEditableValue is controlled — the demo owns the value so a commit sticks. AnEditableValue 受控。
class _EditableDemo extends StatefulWidget {
  const _EditableDemo({
    required this.value,
    this.editor = AnEditKind.input,
    this.options = const [],
    this.startEditing = false,
  });

  final String value;
  final AnEditKind editor;
  final List<AnDropdownOption<String>> options;
  final bool startEditing;

  @override
  State<_EditableDemo> createState() => _EditableDemoState();
}

class _EditableDemoState extends State<_EditableDemo> {
  late String _v = widget.value;

  @override
  Widget build(BuildContext context) => AnEditableValue(
        leading: Text('Threshold', style: AnText.body.copyWith(color: context.colors.inkMuted)),
        fieldLabel: 'Threshold',
        value: _v,
        valueColor: context.colors.inkFaint,
        editor: widget.editor,
        options: widget.options,
        startEditing: widget.startEditing,
        onChanged: (v) => setState(() => _v = v),
      );
}


// AnTags is controlled — the demo owns the live list so add/remove actually mutate. AnTags 受控,演示持列表。
class _TagsDemo extends StatefulWidget {
  const _TagsDemo({this.initial = const [], this.single = false});

  final List<String> initial;
  final bool single;

  @override
  State<_TagsDemo> createState() => _TagsDemoState();
}

class _TagsDemoState extends State<_TagsDemo> {
  late List<AnTag> _tags = [for (final s in widget.initial) AnTag(s)];

  @override
  Widget build(BuildContext context) => AnTags(
        tags: _tags,
        single: widget.single,
        placeholder: 'Add…',
        onChanged: (t) => setState(() => _tags = t),
      );
}

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

// AnTabs demo (stateful: holds the selected key). AnTabs 演示(持选中 key)。
class _TabsDemo extends StatefulWidget {
  const _TabsDemo({this.many = false});
  final bool many;
  @override
  State<_TabsDemo> createState() => _TabsDemoState();
}

class _TabsDemoState extends State<_TabsDemo> {
  late String _v;
  late final List<AnTabsItem> _items = widget.many
      ? [
          for (final k in const ['overview', 'source', 'versions', 'history', 'terminal', 'runs', 'settings'])
            AnTabsItem(
              key: k,
              label: '${k[0].toUpperCase()}${k.substring(1)}',
              pane: AnState(kind: AnStateKind.empty, size: AnStateSize.inset, title: '$k pane'),
            ),
        ]
      : const [
          AnTabsItem(key: 'overview', label: 'Overview', pane: AnState(kind: AnStateKind.empty, size: AnStateSize.inset, title: 'Overview pane')),
          AnTabsItem(key: 'source', label: 'Source', count: '42', pane: AnState(kind: AnStateKind.empty, size: AnStateSize.inset, title: 'Source pane')),
          AnTabsItem(key: 'versions', label: 'Versions', count: '3', pane: AnState(kind: AnStateKind.empty, size: AnStateSize.inset, title: 'Versions pane')),
        ];
  @override
  void initState() {
    super.initState();
    _v = _items.first.key;
  }

  @override
  Widget build(BuildContext context) => AnTabs(items: _items, value: _v, onSelect: (k) => setState(() => _v = k));
}

// AnSidebarList demo (stateful: holds selection + slider checks). AnSidebarList 演示(持选中 + 滑块勾选)。
class _SidebarDemo extends StatefulWidget {
  const _SidebarDemo();
  @override
  State<_SidebarDemo> createState() => _SidebarDemoState();
}

class _SidebarDemoState extends State<_SidebarDemo> {
  String _sel = 'fn1';
  final Set<String> _opts = {'updated', 'versions', 'status'};
  void _opt(String k) => setState(() => _opts.contains(k) ? _opts.remove(k) : _opts.add(k));

  @override
  Widget build(BuildContext context) {
    final model = SidebarModel(
      newLabel: 'New entity',
      filterPlaceholder: 'Filter…',
      groups: [
        SidebarGroup(label: 'Pinned', types: [
          SidebarType(label: 'Functions', icon: AnIcons.function, count: 2, rows: const [
            SidebarRow(id: 'fn1', label: 'normalize-input', dot: AnStatus.done),
            SidebarRow(id: 'fn2', label: 'validate-schema', dot: AnStatus.idle),
          ]),
          SidebarType(label: 'Workflows', icon: AnIcons.workflow, count: 1, rows: const [
            SidebarRow(id: 'wf1', label: 'nightly-deploy', dot: AnStatus.run, meta: '4821'),
          ]),
        ]),
        SidebarGroup(types: [
          SidebarType(rows: [
            SidebarRow(id: 'd1', label: 'docs', icon: AnIcons.doc, children: [
              SidebarRow(id: 'd2', label: 'guide.md', icon: AnIcons.doc),
              SidebarRow(id: 'd3', label: 'api', icon: AnIcons.doc, children: const [
                SidebarRow(id: 'd4', label: 'reference.md'),
              ]),
            ]),
          ]),
        ]),
      ],
    );
    return AnSidebarList(
      model: model,
      selectedId: _sel,
      onSelect: (id) => setState(() => _sel = id),
      onNew: () {},
      menuEntries: [
        const AnMenuSection('Sort'),
        AnMenuItem(label: 'Recently updated', checked: _opts.contains('updated'), keepOpen: true, onTap: () => _opt('updated')),
        AnMenuItem(label: 'Name', checked: _opts.contains('name'), keepOpen: true, onTap: () => _opt('name')),
        const AnMenuSection('Display'),
        AnMenuItem(label: 'Show versions', checked: _opts.contains('versions'), keepOpen: true, onTap: () => _opt('versions')),
        AnMenuItem(label: 'Show status dots', checked: _opts.contains('status'), keepOpen: true, onTap: () => _opt('status')),
      ],
    );
  }
}

// AnOceanHeader demo (stateful: holds the editable title). AnOceanHeader 演示(持可改标题)。
class _OceanHeaderDemo extends StatefulWidget {
  const _OceanHeaderDemo();
  @override
  State<_OceanHeaderDemo> createState() => _OceanHeaderDemoState();
}

class _OceanHeaderDemoState extends State<_OceanHeaderDemo> {
  String _title = 'normalize-input';
  @override
  Widget build(BuildContext context) => AnOceanHeader(
        crumbs: const ['Workspace', 'Functions'],
        title: _title,
        onTitleChange: (v) => setState(() => _title = v),
        actions: [
          AnButton.iconOnly(AnIcons.run, semanticLabel: 'Run', onPressed: () {}),
          AnButton.iconOnly(AnIcons.more, semanticLabel: 'More', onPressed: () {}),
        ],
        meta: const [
          AnBadge('function', tone: AnTone.accent),
          AnBadge('passed', tone: AnTone.ok, dot: AnStatus.done),
        ],
      );
}
