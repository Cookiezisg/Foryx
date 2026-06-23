import 'package:anselm/core/design/theme.dart';
import 'package:anselm/core/ui/ui.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';

// AnInfoCard = borderless info unit: head (icon+title+meta, only when present) + body + actions.
// Title is a header node. AnInfoCard 无边信息单元契约。
void main() {
  Widget host(Widget child) => MaterialApp(
        debugShowCheckedModeBanner: false,
        theme: AnTheme.light(),
        home: Scaffold(body: Center(child: SizedBox(width: 320, child: child))),
      );

  testWidgets('renders title + body', (tester) async {
    await tester.pumpWidget(host(const AnInfoCard(title: 'Schedule', child: Text('body'))));
    expect(find.text('Schedule'), findsOneWidget);
    expect(find.text('body'), findsOneWidget);
  });

  testWidgets('title is a header semantics node', (tester) async {
    final handle = tester.ensureSemantics();
    await tester.pumpWidget(host(const AnInfoCard(title: 'Schedule', child: Text('b'))));
    expect(tester.getSemantics(find.bySemanticsLabel('Schedule')).flagsCollection.isHeader, isTrue);
    handle.dispose();
  });

  testWidgets('meta + actions render', (tester) async {
    await tester.pumpWidget(host(AnInfoCard(
      title: 'Schedule',
      meta: 'UTC',
      actions: [AnButton(label: 'Edit', size: AnButtonSize.sm, onPressed: () {})],
      child: const Text('b'),
    )));
    expect(find.text('UTC'), findsOneWidget);
    expect(find.text('Edit'), findsOneWidget);
  });

  testWidgets('no title/icon/meta → headless, body only, no exception', (tester) async {
    await tester.pumpWidget(host(const AnInfoCard(child: Text('only body'))));
    expect(find.text('only body'), findsOneWidget);
    expect(tester.takeException(), isNull);
  });
}
