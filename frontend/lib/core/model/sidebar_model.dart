import 'package:flutter/widgets.dart' show IconData;

import 'status_state.dart';

/// One entity row in the sidebar tree — recursive ([children] make it a foldable branch, e.g. the
/// documents tree). Carries the same fields an [AnRow] renders. [id] MUST be unique across the whole
/// model — selection, branch-fold, and filter-visibility all key on it; a duplicate id fuses those states
/// across the rows that share it. 侧栏实体行(可递归树枝);id 必须全模型唯一(选中/折叠/过滤都据此,重复会串)。
class SidebarRow {
  const SidebarRow({
    required this.id,
    required this.label,
    this.meta,
    this.hint,
    this.icon,
    this.dot,
    this.children = const [],
  });

  final String id;
  final String label;
  final String? meta;
  final String? hint;
  final IconData? icon;
  final AnStatus? dot;
  final List<SidebarRow> children;

  bool get hasChildren => children.isNotEmpty;
}

/// A kind/type block: a collapsible head ([label]/[icon]/[count]) over its [rows], OR headless (no
/// label AND no icon) — rows flush at depth 0 (scheduler / a documents-folder root). 类型块(或 headless)。
class SidebarType {
  const SidebarType({this.label, this.icon, this.count, this.rows = const []});

  final String? label;
  final IconData? icon;
  final int? count;
  final List<SidebarRow> rows;

  bool get headless => label == null && icon == null;
}

/// A big group: a collapsible chat-style header ([label]) over its [types], OR — when [label] is null —
/// its types flatten directly (single-group compatibility). 大组(可折叠或平铺)。
class SidebarGroup {
  const SidebarGroup({this.label, this.types = const []});

  final String? label;
  final List<SidebarType> types;

  bool get collapsible => label != null && label!.isNotEmpty;

  /// Total rows under this group (the head count), recursing into branches. 组内行总数(含树枝)。
  int get totalRows => types.fold(0, (s, t) => s + _countRows(t.rows));
}

int _countRows(List<SidebarRow> rows) => rows.fold(0, (s, r) => s + 1 + _countRows(r.children));

/// The sidebar's data model: groups → types → rows. Framework-agnostic + pure (the widget renders it; the
/// filter below is unit-tested). 侧栏数据模型(框架无关、纯;过滤单测)。
class SidebarModel {
  const SidebarModel({this.groups = const [], this.newLabel = 'New', this.filterPlaceholder = ''});

  final List<SidebarGroup> groups;
  final String newLabel;
  final String filterPlaceholder;
}

/// Whether a single row matches a (pre-lowercased) query on its label OR meta. 单行命中(label/meta)。
bool sidebarRowMatches(SidebarRow row, String lowerQuery) =>
    row.label.toLowerCase().contains(lowerQuery) || (row.meta?.toLowerCase().contains(lowerQuery) ?? false);

/// PURE in-domain filter: the set of row ids VISIBLE under [query] — a row is visible if it matches OR any
/// descendant matches (so an ancestor of a deep match is included → the consumer force-opens it to reveal
/// the match). Returns an EMPTY set for an empty/blank query (the caller treats empty as "no filter — show
/// all", and must not consult the set). Substring match is plain text, so any query characters (CJK, regex
/// metacharacters, angle brackets) are inert — there is no markup to inject.
///
/// 纯域内过滤:query 下可见的行 id 集——行命中或其子孙命中即可见(深层命中的祖先也入集 → 消费方据此强制展开揭示)。
/// 空 query 返回空集(调用方视空为「不过滤、全显」,勿查集)。纯文本子串匹配,CJK/正则元字符/尖括号一律惰性,无标记可注入。
Set<String> sidebarVisibleIds(SidebarModel model, String query) {
  final q = query.trim().toLowerCase();
  final out = <String>{};
  if (q.isEmpty) return out;

  bool walk(SidebarRow r) {
    var visible = sidebarRowMatches(r, q);
    for (final c in r.children) {
      if (walk(c)) visible = true; // a visible descendant makes this branch visible (ancestor reveal) 子孙可见则祖先可见
    }
    if (visible) out.add(r.id);
    return visible;
  }

  for (final g in model.groups) {
    for (final t in g.types) {
      for (final r in t.rows) {
        walk(r);
      }
    }
  }
  return out;
}
