import 'package:anselm/core/design/theme.dart';
import 'package:anselm/core/ui/ui.dart';
import 'package:anselm/i18n/strings.g.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';

// AnJsonTree = the JSON / structured-data tree (TreeSliver). No-overflow + virtualization are matrix +
// real-run concerns; here: rendering, expand/collapse, type dispatch, circular, invalid JSON, showRoot,
// edges. It's a SCROLL HOST → host gives it a bounded height. AnJsonTree 渲染/折叠/类型/环/错误/边界。
void main() {
  Widget host(Widget child, {double width = 420, double height = 400}) => TranslationProvider(
        child: MaterialApp(
          debugShowCheckedModeBanner: false,
          theme: AnTheme.light(),
          home: Scaffold(body: Center(child: SizedBox(width: width, height: height, child: child))),
        ),
      );

  testWidgets('renders object keys + values (type-coloured leaves)', (tester) async {
    await tester.pumpWidget(host(const AnJsonTree(data: {'name': 'john', 'n': 3, 'ok': true}, showRoot: false)));
    await tester.pumpAndSettle();
    expect(find.textContaining('name'), findsOneWidget);
    expect(find.textContaining('john'), findsOneWidget);
    expect(find.textContaining('3'), findsOneWidget);
    expect(find.textContaining('true'), findsOneWidget);
  });

  testWidgets('branch expands on tap to reveal children (collapsed by open-depth)', (tester) async {
    // openDepth 0 → the depth-0 branch starts collapsed (expanded = depth < openDepth). openDepth 0 → 顶层折叠。
    await tester.pumpWidget(host(const AnJsonTree(data: {'outer': {'inner': 'v'}}, showRoot: false, openDepth: 0)));
    await tester.pumpAndSettle();
    expect(find.textContaining('outer'), findsOneWidget);
    expect(find.textContaining('inner'), findsNothing); // collapsed
    await tester.tap(find.textContaining('outer'));
    await tester.pumpAndSettle();
    expect(find.textContaining('inner'), findsOneWidget); // expanded
  });

  testWidgets('circular reference → [Circular], no stack overflow', (tester) async {
    final m = <String, Object?>{'name': 'node'};
    m['self'] = m;
    await tester.pumpWidget(host(AnJsonTree(data: m, openDepth: 3)));
    await tester.pumpAndSettle();
    expect(tester.takeException(), isNull);
    expect(find.textContaining('[Circular]'), findsOneWidget);
  });

  testWidgets('invalid JSON → an error row, not a crash', (tester) async {
    await tester.pumpWidget(host(const AnJsonTree(jsonString: '{ bad,, }')));
    await tester.pumpAndSettle();
    expect(tester.takeException(), isNull);
    expect(find.textContaining('Invalid JSON'), findsOneWidget);
  });

  testWidgets('showRoot false omits the root row', (tester) async {
    await tester.pumpWidget(host(const AnJsonTree(data: {'a': 1, 'b': 2}, showRoot: false)));
    await tester.pumpAndSettle();
    expect(find.textContaining('root'), findsNothing);
    expect(find.textContaining('a'), findsWidgets);
  });

  testWidgets('showRoot true shows the labelled root branch', (tester) async {
    await tester.pumpWidget(host(const AnJsonTree(data: {'a': 1}, rootLabel: 'payload')));
    await tester.pumpAndSettle();
    expect(find.textContaining('payload'), findsOneWidget);
  });

  testWidgets('scalar / null / empty collections render without error', (tester) async {
    await tester.pumpWidget(host(const AnJsonTree(data: {
      's': '',
      'nil': null,
      'emptyObj': <String, Object?>{},
      'emptyArr': <Object?>[],
    }, showRoot: false)));
    await tester.pumpAndSettle();
    expect(tester.takeException(), isNull);
    expect(find.textContaining('null'), findsOneWidget);
  });

  testWidgets('array root indexes its items', (tester) async {
    await tester.pumpWidget(host(const AnJsonTree(data: ['x', 'y'], rootLabel: 'list', openDepth: 2)));
    await tester.pumpAndSettle();
    expect(find.textContaining('x'), findsOneWidget);
    expect(find.textContaining('y'), findsOneWidget);
  });

  testWidgets('special characters render as plain text (no injection surface)', (tester) async {
    await tester.pumpWidget(host(const AnJsonTree(data: {'html': '<b>x</b> & y'}, showRoot: false)));
    await tester.pumpAndSettle();
    expect(tester.takeException(), isNull);
    expect(find.textContaining('<b>x</b> & y'), findsOneWidget);
  });

  testWidgets('a11y: container is a labelled tree; a branch exposes a toggling expanded state', (tester) async {
    final handle = tester.ensureSemantics();
    await tester.pumpWidget(host(const AnJsonTree(data: {'cfg': {'a': 1}, 'x': 2}, showRoot: false, openDepth: 0)));
    await tester.pumpAndSettle();
    // container label (top-level item count) 容器 label
    expect(find.bySemanticsLabel(RegExp('JSON tree, 2 items')), findsOneWidget);
    // 'cfg' is a branch button, collapsed (expanded false) 分支=按钮、折叠
    final branchFinder = find.ancestor(of: find.textContaining('cfg'), matching: find.byType(AnInteractive)).first;
    expect(tester.getSemantics(branchFinder), isSemantics(isButton: true, isExpanded: false));
    // toggle → expanded flips 展开翻转
    await tester.tap(find.textContaining('cfg'));
    await tester.pumpAndSettle();
    expect(tester.getSemantics(branchFinder), isSemantics(isExpanded: true));
    handle.dispose();
  });

  testWidgets('a large tree scrolls within its bounded viewport (virtualized, no overflow)', (tester) async {
    final big = {for (var i = 0; i < 50; i++) 'key_$i': 'value_$i'};
    await tester.pumpWidget(host(AnJsonTree(data: big, showRoot: false), height: 200));
    await tester.pumpAndSettle();
    expect(tester.takeException(), isNull);
    expect(find.textContaining('key_0'), findsOneWidget);
    // scroll down → later keys come into view, no overflow. 下滚 → 后面的键进视口、不溢出。
    await tester.drag(find.byType(AnJsonTree), const Offset(0, -300));
    await tester.pumpAndSettle();
    expect(tester.takeException(), isNull);
  });
}
