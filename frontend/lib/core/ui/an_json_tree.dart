import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:flutter/rendering.dart' show TreeSliverIndentationType;

import '../../i18n/strings.g.dart';
import '../design/colors.dart';
import '../design/tokens.dart';
import '../design/typography.dart';
import 'an_interactive.dart';
import 'an_scroll_behavior.dart';
import 'icons.dart';

/// E2 — the one JSON / structured-data primitive (WRK-040 G5.2). JSON is parsed into a collapsible
/// TREE, never shown as a raw string. object/array = a foldable summary row (chevron + key + {n}/[n]);
/// leaf = a key + type-coloured value. Built on Flutter's BUILT-IN [TreeSliver] (zero new deps; the
/// official virtualized tree — `treeNodeBuilder` builds only the visible rows, so a ~650KB flowrun
/// result doesn't build the whole widget tree at once), with [TreeSliverIndentationType.none] so the
/// rows self-draw their indent in the kit's flat style. Type colours come from [SyntaxColors] (the same
/// palette the code editor / diff use). Read-only.
///
/// Dart-RUNTIME-TYPE dispatch (NOT the demo's `typeof`): Map→object / List→array / String→string /
/// num→number / bool→boolean / Null→null; a hand-built `dynamic` of an unexpected type falls back to
/// `toString` + neutral colour (never throws). A `seen` ancestor set yields `[Circular]` rather than a
/// stack overflow (jsonDecode output is acyclic, but a hand-built Map can cycle). The node tree is built
/// UPFRONT (the per-row WIDGETS virtualize, the node objects don't) and capped at [_maxNodes] with a
/// "… N more" leaf — building millions of nodes is the upfront ceiling (WRK-040 §9).
///
/// SCROLL HOST: a virtualized [TreeSliver] is a real viewport, so AnJsonTree REQUIRES a BOUNDED height
/// from its parent (a panel / inspector body / a [SizedBox] / an [Expanded]) — like AnPage. TreeSliver
/// does NOT support shrinkWrap (it throws inside a shrink-wrapping viewport), and that bounded viewport
/// is exactly what lets it build only the visible rows for a ~650KB result. Wrap in [AnScrollBehavior]
/// so the scrollbar is hidden (kit style).
///
/// E2——唯一 JSON/结构化展示原语。JSON 必解析成可折叠树、不裸露。object/array=可折叠 summary 行,leaf=key+类型着色值。
/// 搭 Flutter 内置 TreeSliver(零新依赖、官方虚拟化,只建可视行,解 650KB 悬崖)+ IndentationType.none 自绘缩进。
/// 类型按 Dart runtime type 分派(非 typeof);环检测出 [Circular];节点树 upfront 建+封顶。**滚动宿主:须父给有界高**
/// (同 AnPage;TreeSliver 不支持 shrinkWrap——有界 viewport 正是虚拟化前提)。
class AnJsonTree extends StatefulWidget {
  const AnJsonTree({
    this.data,
    this.jsonString,
    this.rootLabel = 'root',
    this.showRoot = true,
    this.openDepth,
    super.key,
  }) : assert(data == null || jsonString == null, 'pass data OR jsonString, not both');

  /// The decoded value (Map/List/String/num/bool/null). 已解码值。
  final Object? data;

  /// A JSON string to decode (shows an error row on parse failure). Mutually exclusive with [data]. JSON 串。
  final String? jsonString;

  /// The root row's label. 根行名。
  final String rootLabel;

  /// false → hide the root row and render its children at depth 0 (an outer Section owns the title). 隐根行。
  final bool showRoot;

  /// Initial expand depth (default: root → 2, no-root → 1). 默认展开深度。
  final int? openDepth;

  @override
  State<AnJsonTree> createState() => _AnJsonTreeState();
}

const int _maxNodes = 2000; // node cap — a huge JSON building millions of nodes freezes the build 节点上限
const int _maxVal = 500; // single-value cap — guard a giant text node (CSS ellipsis is visual only) 单值上限

class _AnJsonTreeState extends State<AnJsonTree> {
  final TreeSliverController _controller = TreeSliverController();
  late List<TreeSliverNode<_JNode>> _tree;
  String? _parseError;

  @override
  void initState() {
    super.initState();
    _rebuild();
  }

  @override
  void didUpdateWidget(AnJsonTree old) {
    super.didUpdateWidget(old);
    if (old.data != widget.data ||
        old.jsonString != widget.jsonString ||
        old.showRoot != widget.showRoot ||
        old.rootLabel != widget.rootLabel ||
        old.openDepth != widget.openDepth) {
      _rebuild();
    }
  }

  void _rebuild() {
    _parseError = null;
    Object? data;
    if (widget.jsonString != null) {
      try {
        data = jsonDecode(widget.jsonString!);
      } catch (e) {
        _parseError = e.toString();
        _tree = const [];
        return;
      }
    } else {
      data = widget.data;
    }
    _tree = _buildTree(data);
  }

  List<TreeSliverNode<_JNode>> _buildTree(Object? data) {
    final ctx = _BuildCtx();
    final openDepth = widget.openDepth ?? (widget.showRoot ? 2 : 1);
    final kind = _kindOf(data);
    if (!widget.showRoot && (kind == _JKind.object || kind == _JKind.array)) {
      // No root row → the data's entries ARE the top level, at depth 0 (flush, like the demo). 无根→顶层 depth 0。
      return _children(data, 0, openDepth, ctx);
    }
    return [_buildNode(widget.rootLabel, data, 0, openDepth, ctx)];
  }

  List<TreeSliverNode<_JNode>> _children(Object? value, int depth, int openDepth, _BuildCtx ctx) {
    final entries = value is List
        ? [for (var i = 0; i < value.length; i++) MapEntry('$i', value[i])]
        : (value is Map ? value.entries.map((e) => MapEntry('${e.key}', e.value)).toList() : const <MapEntry<String, Object?>>[]);
    final out = <TreeSliverNode<_JNode>>[];
    for (var i = 0; i < entries.length; i++) {
      if (ctx.count >= _maxNodes) {
        out.add(TreeSliverNode(_JNode(label: '…', kind: _JKind.nullValue, moreCount: entries.length - i)));
        break;
      }
      out.add(_buildNode(entries[i].key, entries[i].value, depth, openDepth, ctx));
    }
    return out;
  }

  TreeSliverNode<_JNode> _buildNode(String key, Object? value, int depth, int openDepth, _BuildCtx ctx) {
    ctx.count++;
    final kind = _kindOf(value);
    if (kind != _JKind.object && kind != _JKind.array) {
      return TreeSliverNode(_JNode(label: key, kind: kind, value: _valueText(value, kind)));
    }
    if (ctx.seen.contains(value)) {
      // Circular ref (hand-built Map) → don't recurse (a stack overflow otherwise). 环→不下钻。
      return TreeSliverNode(_JNode(label: key, kind: _JKind.nullValue, circular: true));
    }
    ctx.seen.add(value!);
    final children = _children(value, depth + 1, openDepth, ctx);
    ctx.seen.remove(value);
    final s = _summary(value, kind);
    // value: s lets an EMPTY collection render in the leaf path as a dim "{0}"/"[0]" (no fake chevron
    // button — children.isEmpty falls to leaf in _buildRow). value:s 让空集合走叶路径(无假分支钮)。
    return TreeSliverNode(
      _JNode(label: key, kind: kind, summary: s, value: s),
      children: children,
      expanded: depth < openDepth,
    );
  }

  @override
  Widget build(BuildContext context) {
    final c = context.colors;
    if (_parseError != null) {
      return Padding(
        padding: const EdgeInsets.symmetric(horizontal: AnSpace.s12, vertical: AnSpace.s8),
        child: Text.rich(
          TextSpan(children: [
            TextSpan(text: '${context.t.tree.invalidJson}  ', style: AnText.code.copyWith(color: c.danger)),
            TextSpan(text: _parseError, style: AnText.code.copyWith(color: c.inkFaint)),
          ]),
          style: AnText.code,
        ),
      );
    }
    final reduced = AnMotionPref.reduced(context);
    // Top-level item count for the container a11y label (collection root → its children; else the tree). 顶层项数。
    final topCount = widget.showRoot
        ? (_tree.isNotEmpty ? _tree.first.children.length : 0)
        : _tree.length;
    // A virtualized TreeSliver is a real viewport — it does NOT support shrinkWrap (RenderTreeSliver
    // throws inside a shrink-wrapping viewport), so AnJsonTree REQUIRES a bounded height from its parent
    // (a panel / inspector body / a SizedBox), like AnPage. That bounded viewport is what makes the
    // virtualization (build only visible rows) possible for a ~650KB result. 须有界高(虚拟化前提,不支持 shrinkWrap)。
    return Semantics(
      container: true,
      label: context.t.a11y.jsonTree(count: topCount),
      child: ScrollConfiguration(
        behavior: const AnScrollBehavior(),
        child: CustomScrollView(
          slivers: [
            TreeSliver<_JNode>(
              tree: _tree,
              controller: _controller,
              indentation: TreeSliverIndentationType.none,
              // reduced → AnimationStyle.noAnimation (the cleaner no-motion path; flutter#153889's
              // duration-zero freeze was fixed in 3.41.9 — both skip now — but noAnimation stays correct).
              // reduced 走 noAnimation(更干净;#153889 的 duration-zero 冻结 3.41.9 已修,仍用 noAnimation)。
              toggleAnimationStyle: reduced
                  ? AnimationStyle.noAnimation
                  : AnimationStyle(curve: AnMotion.easeOut, duration: AnMotion.mid),
              treeRowExtentBuilder: (node, dimensions) => AnSize.row,
              treeNodeBuilder: _buildRow,
            ),
          ],
        ),
      ),
    );
  }

  Widget _buildRow(BuildContext context, TreeSliverNode<Object?> node, AnimationStyle toggleAnimationStyle) {
    final n = node.content as _JNode;
    final c = context.colors;
    final syntax = context.syntax;
    final reduced = AnMotionPref.reduced(context);
    // A branch only if it's a collection WITH children — an EMPTY {}/[] renders as a (non-tappable) leaf
    // so it doesn't expose a fake expand button that does nothing. 仅非空集合是分支;空集合走叶、不假装可展开。
    final isBranch = (n.kind == _JKind.object || n.kind == _JKind.array) && node.children.isNotEmpty;
    final depth = node.depth ?? 0;
    final indent = depth * AnSize.iconLg; // demo --indent = 20 (== iconLg) 每级缩进

    if (isBranch) {
      // [chevron | key {n}] — tappable, hover-tinted; expanded announced for a screen reader.
      return AnInteractive(
        onTap: () => _controller.toggleNode(node),
        expanded: node.isExpanded,
        builder: (ctx, states) {
          final active = states.isActive;
          return AnimatedContainer(
            duration: reduced ? Duration.zero : AnMotion.fast,
            height: AnSize.row,
            padding: EdgeInsets.only(left: AnSpace.s8 + indent, right: AnSpace.s12),
            decoration: BoxDecoration(color: c.surfaceHover.whenActive(active), borderRadius: BorderRadius.circular(AnRadius.button)),
            child: Row(
              children: [
                AnimatedRotation(
                  turns: node.isExpanded ? 0.25 : 0, // ▸ → ▾
                  duration: reduced ? Duration.zero : AnMotion.mid,
                  child: Icon(AnIcons.chevronRight, size: AnSize.icon, color: c.inkFaint),
                ),
                const SizedBox(width: AnSpace.s6),
                Flexible(
                  child: Text.rich(
                    TextSpan(children: [
                      TextSpan(text: n.label, style: AnText.code.copyWith(color: active ? c.ink : c.inkMuted)),
                      TextSpan(text: '  ${n.summary}', style: AnText.metaTabular().copyWith(color: c.inkFaint, fontFamily: AnText.monoFamily)),
                    ]),
                    maxLines: 1,
                    overflow: TextOverflow.ellipsis,
                  ),
                ),
              ],
            ),
          );
        },
      );
    }

    // leaf: [key | value] flush at the indent (no chevron slot). Two Flexibles clip key & value
    // INDEPENDENTLY (demo's `[.k auto][.v 1fr]` double-track) — a long key can't hide the value. Scalars
    // read "key: value"; an empty collection / marker reads "key  text". 叶:双 Flexible 独立省略、长 key 不吞 value。
    final isCollection = n.kind == _JKind.object || n.kind == _JKind.array; // empty collection → leaf 空集合
    final String valueText;
    final Color valueColor;
    var italic = false;
    if (n.circular) {
      valueText = context.t.tree.circular;
      valueColor = c.danger; // a cycle IS an error 环=错误
    } else if (n.moreCount > 0) {
      valueText = context.t.tree.moreItems(count: n.moreCount);
      valueColor = c.inkFaint; // truncation is informational, not an error (demo uses the muted null tier) 截断非错误
    } else if (isCollection) {
      valueText = n.value ?? ''; // "{0}" / "[0]"
      valueColor = c.inkFaint;
    } else {
      valueText = n.value ?? '';
      valueColor = _valueColor(n.kind, syntax);
      italic = n.kind == _JKind.nullValue; // demo .null is italic null 走斜体
    }
    final useColon = !isCollection && !n.circular && n.moreCount == 0; // only scalars read "key: value" 仅标量带冒号
    return Padding(
      padding: EdgeInsets.only(left: AnSpace.s8 + indent, right: AnSpace.s12),
      child: Row(
        children: [
          Flexible(
            flex: 2,
            child: Text(n.label, maxLines: 1, overflow: TextOverflow.ellipsis, style: AnText.code.copyWith(color: n.circular ? c.danger : c.inkMuted)),
          ),
          Text(useColon ? ': ' : '  ', style: AnText.code.copyWith(color: c.inkFaint)),
          Flexible(
            flex: 5,
            child: Text(valueText, maxLines: 1, overflow: TextOverflow.ellipsis, style: AnText.code.copyWith(color: valueColor, fontStyle: italic ? FontStyle.italic : null)),
          ),
        ],
      ),
    );
  }
}

// One tree row's data. Diagnostic markers ([Circular] / "N more") carry semantic flags, formatted with
// i18n at row-build time (the node tree is built without a context). 树行数据;诊断标记带语义旗、建行时本地化。
class _JNode {
  const _JNode({required this.label, required this.kind, this.value, this.summary, this.circular = false, this.moreCount = 0});
  final String label;
  final _JKind kind;
  final String? value; // leaf value text 叶值
  final String? summary; // branch {n} / [n] 分支摘要
  final bool circular; // ancestor cycle → [Circular] 环
  final int moreCount; // node-cap truncation → "N more" 截断
}

enum _JKind { object, array, string, number, boolean, nullValue, unknown }

class _BuildCtx {
  int count = 0;
  final Set<Object> seen = {}; // identity ancestor path (Map/List == is identity) 祖先路径(identity)
}

// Dispatch by Dart RUNTIME type (NOT the demo's typeof). 按 Dart 运行时类型分派。
_JKind _kindOf(Object? v) {
  if (v == null) return _JKind.nullValue;
  if (v is Map) return _JKind.object;
  if (v is List) return _JKind.array;
  if (v is String) return _JKind.string;
  if (v is bool) return _JKind.boolean; // before num (bool is not num in Dart, but order is intentional) 先 bool
  if (v is num) return _JKind.number; // int + double both → number (one colour) int/double 同 number 色
  return _JKind.unknown; // hand-built dynamic of an unexpected type 兜底
}

String _summary(Object? v, _JKind kind) =>
    kind == _JKind.array ? '[${(v as List).length}]' : '{${(v as Map).length}}';

String _valueText(Object? v, _JKind kind) {
  switch (kind) {
    case _JKind.nullValue:
      return 'null';
    case _JKind.string:
      final s = v as String;
      if (s.isEmpty) return '""';
      return s.length > _maxVal ? '${s.substring(0, _maxVal)}…' : s;
    case _JKind.number:
    case _JKind.boolean:
      return '$v';
    default:
      return '$v'; // unknown → toString 兜底
  }
}

Color _valueColor(_JKind kind, SyntaxColors s) {
  switch (kind) {
    case _JKind.string:
      return s.string;
    case _JKind.number:
      return s.number;
    case _JKind.boolean:
      return s.keyword; // bool reads as a keyword (true/false) 布尔走关键字色
    case _JKind.nullValue:
      return s.comment; // null muted like a comment null 走注释色
    default:
      return s.comment;
  }
}
