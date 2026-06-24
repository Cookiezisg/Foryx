import 'package:anselm/core/design/theme.dart';
import 'package:anselm/core/ui/ui.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';

// AnSidebarList = New + in-domain filter + groups→types→rows tree (on AnRow/AnInput/AnMenu). Selection is
// controlled; filter hides non-matches + reveals ancestors. AnSidebarList 契约。
void main() {
  SidebarModel model() => SidebarModel(
        newLabel: 'New',
        filterPlaceholder: 'Filter…',
        groups: [
          SidebarGroup(label: 'Pinned', types: [
            SidebarType(label: 'Functions', icon: AnIcons.function, count: 2, rows: const [
              SidebarRow(id: 'fn1', label: 'normalize-input'),
              SidebarRow(id: 'fn2', label: 'validate-schema'),
            ]),
          ]),
        ],
      );

  Widget host(Widget child) => MaterialApp(
        debugShowCheckedModeBanner: false,
        theme: AnTheme.light(),
        home: Scaffold(body: Center(child: SizedBox(width: 280, height: 400, child: child))),
      );

  testWidgets('renders New + filter + group/type heads + rows', (tester) async {
    await tester.pumpWidget(host(AnSidebarList(model: model(), onNew: () {}, onSelect: (_) {})));
    expect(find.text('New'), findsOneWidget);
    expect(find.text('Pinned'), findsOneWidget); // group head
    expect(find.text('Functions'), findsOneWidget); // type head
    expect(find.text('normalize-input'), findsOneWidget);
    expect(find.text('validate-schema'), findsOneWidget);
    expect(tester.takeException(), isNull);
  });

  testWidgets('tapping an entity row selects it', (tester) async {
    String? sel;
    await tester.pumpWidget(host(AnSidebarList(model: model(), onSelect: (id) => sel = id)));
    await tester.tap(find.text('normalize-input'));
    await tester.pumpAndSettle();
    expect(sel, 'fn1');
  });

  testWidgets('filter hides non-matching rows, keeps matches (+ reveals their type)', (tester) async {
    await tester.pumpWidget(host(AnSidebarList(model: model(), onSelect: (_) {})));
    await tester.enterText(find.byType(TextField), 'normalize');
    await tester.pumpAndSettle();
    expect(find.text('normalize-input'), findsOneWidget);
    expect(find.text('validate-schema'), findsNothing); // filtered out
    expect(find.text('Functions'), findsOneWidget); // type stays (has a match)
  });

  testWidgets('the type head is a disclosure button — tapping it collapses its rows', (tester) async {
    await tester.pumpWidget(host(AnSidebarList(model: model(), onSelect: (_) {})));
    expect(find.text('normalize-input'), findsOneWidget);
    await tester.tap(find.text('Functions')); // type head toggles (whole head; keyboard-operable, not mouse-only lead)
    await tester.pumpAndSettle();
    expect(find.text('normalize-input'), findsNothing); // type collapsed
  });

  testWidgets('collapsing a group hides its rows', (tester) async {
    await tester.pumpWidget(host(AnSidebarList(model: model(), onSelect: (_) {})));
    expect(find.text('normalize-input'), findsOneWidget);
    await tester.tap(find.text('Pinned')); // the group head toggles
    await tester.pumpAndSettle();
    expect(find.text('normalize-input'), findsNothing); // collapsed
  });
}
