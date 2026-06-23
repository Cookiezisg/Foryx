import 'package:anselm/core/design/theme.dart';
import 'package:anselm/core/ui/ui.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';

// AnCard = bordered container; selectable → a tappable button with a selected state. AnCard 契约。
void main() {
  Widget host(Widget child) => MaterialApp(
        debugShowCheckedModeBanner: false,
        theme: AnTheme.light(),
        home: Scaffold(body: Center(child: SizedBox(width: 280, child: child))),
      );

  testWidgets('renders its child', (tester) async {
    await tester.pumpWidget(host(const AnCard(child: Text('content'))));
    expect(find.text('content'), findsOneWidget);
  });

  testWidgets('selectable card taps → onSelect and is a button', (tester) async {
    final handle = tester.ensureSemantics();
    var taps = 0;
    await tester.pumpWidget(host(AnCard(selectable: true, onSelect: () => taps++, child: const Text('pick me'))));
    await tester.tap(find.byType(AnCard));
    expect(taps, 1);
    expect(tester.getSemantics(find.byType(AnInteractive)).flagsCollection.isButton, isTrue);
    handle.dispose();
  });

  testWidgets('selected reflects in semantics', (tester) async {
    final handle = tester.ensureSemantics();
    await tester.pumpWidget(host(AnCard(selectable: true, selected: true, onSelect: () {}, child: const Text('x'))));
    expect(tester.getSemantics(find.byType(AnInteractive)).flagsCollection.isSelected.toBoolOrNull(), isTrue);
    handle.dispose();
  });

  testWidgets('non-selectable card is inert (no AnInteractive button)', (tester) async {
    await tester.pumpWidget(host(const AnCard(child: Text('static'))));
    expect(find.byType(AnInteractive), findsNothing);
    expect(tester.takeException(), isNull);
  });
}
