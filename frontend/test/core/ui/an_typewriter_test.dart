import 'package:anselm/core/design/theme.dart';
import 'package:anselm/core/ui/ui.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  Widget host(Widget child, {double width = 320, bool reduced = false}) => MaterialApp(
        debugShowCheckedModeBanner: false,
        theme: AnTheme.light(),
        home: Scaffold(
          body: Center(
            child: SizedBox(
              width: width,
              child: Builder(builder: (ctx) {
                return reduced ? MediaQuery(data: MediaQuery.of(ctx).copyWith(disableAnimations: true), child: child) : child;
              }),
            ),
          ),
        ),
      );

  String visible(WidgetTester t) {
    final texts = t.widgetList<Text>(find.descendant(of: find.byType(AnTypewriter), matching: find.byType(Text)));
    return texts.isEmpty ? '' : (texts.first.data ?? '');
  }

  testWidgets('types progressively; a11y exposes the FULL phrase, not the half-typed string', (tester) async {
    final handle = tester.ensureSemantics();
    await tester.pumpWidget(host(const AnTypewriter(['Hello world'], loop: false)));
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 165)); // ~3 graphemes @ 55ms
    final partial = visible(tester);
    expect(partial.length, lessThan('Hello world'.length));
    expect('Hello world'.startsWith(partial), isTrue);
    // the a11y label is the COMPLETE phrase even mid-type
    expect(tester.getSemantics(find.byType(AnTypewriter)).label, 'Hello world');
    await tester.pump(const Duration(milliseconds: 11 * 55)); // finish typing
    expect(visible(tester), 'Hello world');
    handle.dispose();
    await tester.pump(const Duration(milliseconds: 1500)); // let the controller complete + stop
  });

  testWidgets('reduced-motion shows the full primary phrase, steady, and settles', (tester) async {
    await tester.pumpWidget(host(const AnTypewriter(['Welcome', 'Second'], loop: true), reduced: true));
    await tester.pumpAndSettle(const Duration(milliseconds: 16), EnginePhase.sendSemanticsUpdate, const Duration(seconds: 5));
    expect(tester.takeException(), isNull); // no controller left running
    expect(visible(tester), 'Welcome'); // phrases.first, full, static
  });

  testWidgets('grapheme-safe: an emoji phrase types without splitting a cluster', (tester) async {
    const phrase = 'Hi 👋🏽 ok';
    await tester.pumpWidget(host(const AnTypewriter([phrase], loop: false)));
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 220)); // ~4 graphemes
    final partial = visible(tester);
    // partial is always a whole-grapheme prefix (re-slicing the phrase by partial's cluster count matches)
    expect(phrase.characters.take(partial.characters.length).join(), partial);
    expect(tester.takeException(), isNull);
    await tester.pump(const Duration(milliseconds: 2500)); // finish + hold + stop
  });

  testWidgets('empty phrases render nothing and do not crash', (tester) async {
    await tester.pumpWidget(host(const AnTypewriter([])));
    await tester.pumpAndSettle(const Duration(milliseconds: 16), EnginePhase.sendSemanticsUpdate, const Duration(seconds: 5));
    expect(tester.takeException(), isNull);
  });
}
