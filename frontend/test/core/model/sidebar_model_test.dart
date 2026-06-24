import 'package:anselm/core/model/sidebar_model.dart';
import 'package:flutter_test/flutter_test.dart';

// SidebarModel filter is the pure, framework-free core of AnSidebarList. Tested directly (no widget pump)
// across the 5-battery edges + ancestor-reveal. SidebarModel 过滤纯核,直接测(无 widget),含 5 电池 + 祖先回填。
void main() {
  const tree = SidebarModel(groups: [
    SidebarGroup(label: 'Pinned', types: [
      SidebarType(label: 'Functions', rows: [
        SidebarRow(id: 'fn1', label: 'normalize-input', meta: 'fn'),
        SidebarRow(id: 'fn2', label: 'validate', meta: 'fn'),
      ]),
    ]),
    SidebarGroup(types: [
      SidebarType(rows: [ // headless
        SidebarRow(id: 'doc1', label: 'README', children: [
          SidebarRow(id: 'doc2', label: 'getting-started', children: [
            SidebarRow(id: 'doc3', label: 'deep-leaf'),
          ]),
        ]),
      ]),
    ]),
  ]);

  test('empty / blank query → empty set (caller treats as no-filter)', () {
    expect(sidebarVisibleIds(tree, ''), isEmpty);
    expect(sidebarVisibleIds(tree, '   '), isEmpty);
  });

  test('matches by label and by meta, case-insensitive', () {
    expect(sidebarVisibleIds(tree, 'NORMALIZE'), contains('fn1'));
    expect(sidebarVisibleIds(tree, 'validate'), contains('fn2'));
    expect(sidebarVisibleIds(tree, 'normalize'), isNot(contains('fn2')));
  });

  test('ancestor reveal: a deep match includes its whole branch chain', () {
    final v = sidebarVisibleIds(tree, 'deep-leaf');
    expect(v, containsAll(['doc3', 'doc2', 'doc1']));
    expect(v, isNot(contains('fn1')));
  });

  test('no match → empty visible set', () => expect(sidebarVisibleIds(tree, 'zzzznotfound'), isEmpty));

  // ── 5-battery ──
  test('empty model → empty (no crash)', () => expect(sidebarVisibleIds(const SidebarModel(), 'x'), isEmpty));

  test('CJK query matches a CJK label', () {
    const m = SidebarModel(groups: [SidebarGroup(types: [SidebarType(rows: [SidebarRow(id: 'c', label: '工作流编排器')])])]);
    expect(sidebarVisibleIds(m, '编排'), contains('c'));
  });

  test('regex / markup metacharacters are inert literal substrings (injection-safe)', () {
    const m = SidebarModel(groups: [SidebarGroup(types: [SidebarType(rows: [SidebarRow(id: 'x', label: 'a.*b <script>')])])]);
    expect(sidebarVisibleIds(m, '.*'), contains('x')); // literal, not a regex wildcard
    expect(sidebarVisibleIds(m, '<script>'), contains('x')); // literal, not markup
    expect(sidebarVisibleIds(m, 'xyz'), isEmpty);
  });

  test('massive flat list filters in one pass', () {
    final rows = [for (var i = 0; i < 2000; i++) SidebarRow(id: 'r$i', label: 'item $i')];
    final m = SidebarModel(groups: [SidebarGroup(types: [SidebarType(rows: rows)])]);
    expect(sidebarVisibleIds(m, 'item 1999'), contains('r1999'));
    expect(sidebarVisibleIds(m, 'item 1999').length, 1);
  });

  test('group.totalRows counts recursively (branch children included)', () {
    expect(tree.groups[1].totalRows, 3); // doc1 + doc2 + doc3
    expect(tree.groups[0].totalRows, 2);
  });
}
