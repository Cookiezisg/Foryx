import 'package:anselm/core/design/theme.dart';
import 'package:anselm/core/ui/ui.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';

// AnRow = core list row. Tap selects; a collapsible row toggles on the lead + selects elsewhere;
// passive is inert; collapsible carries `expanded` semantics. AnRow 核心行契约。
void main() {
  Widget host(Widget child) => MaterialApp(
        debugShowCheckedModeBanner: false,
        theme: AnTheme.light(),
        home: Scaffold(body: Center(child: SizedBox(width: 320, child: child))),
      );

  testWidgets('renders label + lead icon + meta', (tester) async {
    await tester.pumpWidget(host(AnRow(icon: AnIcons.function, label: 'normalize', meta: '2m', onSelect: () {})));
    expect(find.text('normalize'), findsOneWidget);
    expect(find.byIcon(AnIcons.function), findsOneWidget);
    expect(find.text('2m'), findsOneWidget);
  });

  testWidgets('tap selects (non-collapsible)', (tester) async {
    var sel = 0;
    await tester.pumpWidget(host(AnRow(label: 'row', onSelect: () => sel++)));
    await tester.tap(find.byType(AnRow));
    expect(sel, 1);
  });

  testWidgets('collapsible: lead toggles, label selects', (tester) async {
    var toggles = 0, selects = 0;
    await tester.pumpWidget(host(AnRow(
      collapsible: true,
      icon: AnIcons.workflow,
      label: 'tree node',
      onToggle: () => toggles++,
      onSelect: () => selects++,
    )));
    await tester.tap(find.byIcon(AnIcons.workflow)); // the lead
    expect(toggles, 1);
    expect(selects, 0);
    await tester.tap(find.text('tree node')); // the label
    expect(selects, 1);
    expect(toggles, 1);
  });

  testWidgets('collapsible exposes expanded disclosure semantics; non-collapsible does not', (tester) async {
    // G4: a collapsible row announces its disclosure state (the keyboard expand/collapse lives in the tree
    // consumer's roving focus, e.g. AnSidebarList). A non-collapsible row makes no disclosure promise.
    final handle = tester.ensureSemantics();
    await tester.pumpWidget(host(AnRow(collapsible: true, open: true, label: 'open node', onSelect: () {}, onToggle: () {})));
    expect(tester.getSemantics(find.byType(AnInteractive)).flagsCollection.isExpanded.toBoolOrNull(), isTrue);
    await tester.pumpWidget(host(AnRow(collapsible: true, open: false, label: 'closed node', onSelect: () {}, onToggle: () {})));
    expect(tester.getSemantics(find.byType(AnInteractive)).flagsCollection.isExpanded.toBoolOrNull(), isFalse);
    await tester.pumpWidget(host(AnRow(label: 'plain', onSelect: () {})));
    expect(tester.getSemantics(find.byType(AnInteractive)).flagsCollection.isExpanded.toBoolOrNull(), isNull);
    handle.dispose();
  });

  testWidgets('passive is inert (no tap, not a button)', (tester) async {
    final handle = tester.ensureSemantics();
    var sel = 0;
    await tester.pumpWidget(host(AnRow(label: 'passive', passive: true, onSelect: () => sel++)));
    await tester.tap(find.byType(AnRow), warnIfMissed: false);
    expect(sel, 0);
    expect(tester.getSemantics(find.byType(AnInteractive)).flagsCollection.isButton, isFalse);
    handle.dispose();
  });

  testWidgets('hint renders (taller row)', (tester) async {
    await tester.pumpWidget(host(AnRow(label: 'with hint', hint: 'a longer explanatory hint', onSelect: () {})));
    expect(find.text('a longer explanatory hint'), findsOneWidget);
    expect(tester.takeException(), isNull);
  });

  testWidgets('short row vertically centres its content (not pinned to the top)', (tester) async {
    // The minHeight floor makes the row taller than a single line; the content must centre, not sit high
    // (the Stack default topStart bug). 短行内容须居中、非顶对齐(Stack 默认 topStart bug)。
    await tester.pumpWidget(host(AnRow(icon: AnIcons.function, label: 'centered', onSelect: () {})));
    final rowMid = tester.getRect(find.byType(AnRow)).center.dy;
    final labelMid = tester.getRect(find.text('centered')).center.dy;
    expect((labelMid - rowMid).abs(), lessThan(1.5),
        reason: 'a single-line row taller than its text must centre the text, not pin it to the top');
  });

  testWidgets('dot lead + actions render without overflow', (tester) async {
    await tester.pumpWidget(host(AnRow(
      dot: AnStatus.run,
      label: 'running job',
      meta: '12s',
      actions: [AnButton.iconOnly(AnIcons.stop, semanticLabel: 'Stop', onPressed: () {})],
      onSelect: () {},
    )));
    expect(find.text('running job'), findsOneWidget);
    expect(tester.takeException(), isNull);
  });
}
