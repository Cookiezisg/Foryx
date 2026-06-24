import 'package:flutter/widgets.dart';
import 'package:lucide_icons_flutter/lucide_icons.dart';

/// Semantic icon registry — the ONE place a domain meaning binds to a concrete glyph. Mirrors the
/// demo's `core/icons.js` (ALIAS) + `config/entity-kinds.js`: features/widgets reference a semantic
/// name, never a raw Lucide identifier, so re-skinning an icon is a one-line edit here. The glyph
/// set is Lucide; we render the THIN weight family ([_family]) — the package ships static per-weight
/// faces (Lucide100–600) that SHARE codepoints, so re-pointing the family to a lighter stroke (≈ the
/// demo's stroke-width 1.7, vs the heavier default ~2) is a one-token change. [byKey]/[toolIcon]/
/// [node] resolve data-driven strings and fall back to [fallback] so an unknown key degrades to a
/// visible "?" instead of crashing.
///
/// 语义图标单源——领域含义 → 字形的唯一绑定处(镜像 icons.js + entity-kinds.js)。字形集=Lucide,渲染 THIN
/// 字重族(_family):包内各字重是共享码点的独立字体,改一处 _family 即换更细的笔画(≈demo 1.7,默认偏粗 ~2)。
abstract final class AnIcons {
  // Lighter Lucide weight face — codepoints are shared with the default 'Lucide', so we keep the
  // same glyph, thinner stroke. Lucide300 ≈ demo stroke 1.7. 更细字重族,码点共享、笔画更细。
  static const String _family = 'Lucide300';
  static const String _pkg = 'lucide_icons_flutter';
  static IconData _thin(IconData base) => IconData(base.codePoint, fontFamily: _family, fontPackage: _pkg);

  // ── chrome ──
  static final IconData chevronRight = _thin(LucideIcons.chevronRight);
  static final IconData chevronDown = _thin(LucideIcons.chevronDown);
  static final IconData more = _thin(LucideIcons.ellipsis);
  static final IconData grip = _thin(LucideIcons.gripVertical);
  static final IconData close = _thin(LucideIcons.x);
  static final IconData sliders = _thin(LucideIcons.slidersHorizontal);
  static final IconData wrap = _thin(LucideIcons.wrapText);
  static final IconData expand = _thin(LucideIcons.maximize2);
  static final IconData plus = _thin(LucideIcons.plus); // New / add (sidebar New row, row-add) 新建/添加
  static final IconData search = _thin(LucideIcons.search);
  static final IconData check = _thin(LucideIcons.check);

  // ── entities / graph nodes / mounts ──
  static final IconData function = _thin(LucideIcons.squareFunction);
  static final IconData handler = _thin(LucideIcons.box);
  static final IconData agent = _thin(LucideIcons.bot);
  static final IconData workflow = _thin(LucideIcons.workflow);
  static final IconData trigger = _thin(LucideIcons.zap);
  static final IconData control = _thin(LucideIcons.gitBranch);
  static final IconData action = _thin(LucideIcons.play);
  static final IconData approval = _thin(LucideIcons.shieldCheck);
  static final IconData mcp = _thin(LucideIcons.plug);
  static final IconData skill = _thin(LucideIcons.bookOpen);
  static final IconData doc = _thin(LucideIcons.fileText);
  static final IconData entities = _thin(LucideIcons.layoutGrid);
  static final IconData chat = _thin(LucideIcons.messageSquare);
  static final IconData scheduler = _thin(LucideIcons.clock);
  static final IconData gear = _thin(LucideIcons.settings);

  // ── block / conversation semantics ──
  static final IconData reasoning = _thin(LucideIcons.brain);
  static final IconData tool = _thin(LucideIcons.wrench);
  static final IconData subagent = _thin(LucideIcons.gitFork);
  static final IconData turnEnd = _thin(LucideIcons.flag);
  static final IconData terminal = _thin(LucideIcons.squareTerminal);

  // ── execution verbs / actions ──
  static final IconData run = _thin(LucideIcons.play);
  static final IconData enter = _thin(LucideIcons.cornerDownLeft);
  static final IconData stop = _thin(LucideIcons.square);
  static final IconData spin = _thin(LucideIcons.loaderCircle);
  static final IconData forge = _thin(LucideIcons.hammer);
  static final IconData edit = _thin(LucideIcons.squarePen);
  static final IconData trash = _thin(LucideIcons.trash2);
  static final IconData web = _thin(LucideIcons.globe);
  static final IconData iterate = _thin(LucideIcons.sparkles); // AI edit (≠ forge: rebuild env) AI 编辑
  static final IconData history = _thin(LucideIcons.history);
  static final IconData diff = _thin(LucideIcons.gitCompare);

  // ── state placeholders ──
  static final IconData empty = _thin(LucideIcons.inbox);
  static final IconData error = _thin(LucideIcons.triangleAlert);

  // ── feedback severities (callout / state) ──
  static final IconData info = _thin(LucideIcons.info);
  static final IconData success = _thin(LucideIcons.circleCheck);
  static final IconData warning = _thin(LucideIcons.triangleAlert);
  static final IconData danger = _thin(LucideIcons.octagonAlert);

  /// Unknown-key sink — a visible "?" so a missing binding is obvious, never a crash.
  /// 未知键兜底——可见的"?",缺绑定一眼可见、绝不崩。
  static final IconData fallback = _thin(LucideIcons.circleQuestionMark);

  /// Semantic key → glyph, for data-driven resolution (a backend node kind, a derived tool icon).
  /// Prefer the named fields above at call sites; this map is for strings only.
  /// 语义键 → 字形,供数据驱动解析。调用处优先用上面的具名字段。
  static final Map<String, IconData> _byKey = {
    'chevr': chevronRight, 'chevd': chevronDown, 'more': more, 'grip': grip,
    'close': close, 'sliders': sliders, 'wrap': wrap, 'expand': expand, 'search': search, 'check': check,
    'function': function, 'handler': handler, 'agent': agent, 'workflow': workflow,
    'trigger': trigger, 'control': control, 'action': action, 'approval': approval,
    'mcp': mcp, 'skill': skill, 'doc': doc, 'document': doc, 'entities': entities, // 'document' = backend EntityKind wire 后端实体 kind 线缆值
    'chat': chat, 'conversation': chat, 'scheduler': scheduler, 'gear': gear,
    'reasoning': reasoning, 'tool': tool, 'subagent': subagent, 'turnend': turnEnd, 'terminal': terminal,
    'run': run, 'enter': enter, 'stop': stop, 'spin': spin, 'forge': forge,
    'edit': edit, 'trash': trash, 'web': web, 'iterate': iterate, 'history': history, 'diff': diff,
    'empty': empty, 'error': error,
  };

  /// Resolve a semantic key string to a glyph (unknown → [fallback]). 语义键字串 → 字形。
  static IconData byKey(String key) => _byKey[key] ?? fallback;

  /// Exact tool-name → icon overrides (the rest are inferred by [toolIcon]). 工具名精确映射。
  static final Map<String, IconData> _toolExact = {
    'run_function': action, 'call_handler': handler, 'invoke_agent': agent, 'trigger_workflow': workflow,
    'run_shell': tool, 'read_file': doc, 'write_file': edit, 'edit_file': edit,
    'web_search': web, 'web_fetch': web, 'search_blocks': search,
  };

  /// Tool name → icon. Exact match first, then keyword inference (mirrors demo `toolIcon`); the
  /// block-tree shows a per-tool glyph so a `read_file` call reads differently from a `web_fetch`.
  /// 工具名 → 图标:先精确、后关键字推断(镜像 demo toolIcon)。
  static IconData toolIcon(String name) {
    final n = name.toLowerCase();
    final exact = _toolExact[n];
    if (exact != null) return exact;
    if (RegExp(r'shell|bash|exec').hasMatch(n)) return tool;
    if (n.contains('search')) return search;
    if (RegExp(r'file|read|write|doc').hasMatch(n)) return doc;
    if (RegExp(r'web|fetch|http|url').hasMatch(n)) return web;
    if (n.contains('function')) return function;
    if (n.contains('handler')) return handler;
    if (n.contains('agent')) return agent;
    if (RegExp(r'workflow|trigger').hasMatch(n)) return workflow;
    if (RegExp(r'create|edit|build|forge').hasMatch(n)) return forge;
    if (n.startsWith('mcp')) return mcp;
    return tool;
  }

  /// Graph node kind → icon (the 5 closed kinds; unknown → [fallback]). 图节点 kind → 图标。
  static final Map<String, IconData> _nodeKind = {
    'trigger': trigger, 'action': action, 'agent': agent, 'control': control, 'approval': approval,
  };

  static IconData node(String kind) => _nodeKind[kind] ?? fallback;
}
