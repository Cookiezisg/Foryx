import 'package:anselm/core/design/theme.dart';
import 'package:anselm/core/ui/ui.dart';
import 'package:anselm/i18n/strings.g.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  Widget host(Widget child, {double width = 300, bool reduced = false}) => TranslationProvider(
        child: MaterialApp(
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
        ),
      );

  testWidgets('shimmers (ShaderMask) normally; static (no ShaderMask) under reduced-motion + settles', (tester) async {
    await tester.pumpWidget(host(const AnSkeleton.text()));
    await tester.pump();
    expect(find.byType(ShaderMask), findsOneWidget); // animated sweep present

    await tester.pumpWidget(host(const AnSkeleton.text(), reduced: true));
    await tester.pumpAndSettle(const Duration(milliseconds: 16), EnginePhase.sendSemanticsUpdate, const Duration(seconds: 5));
    expect(tester.takeException(), isNull); // no ticker left running under reduced
    expect(find.byType(ShaderMask), findsNothing); // froze to flat bones
  });

  testWidgets('a11y: a "loading" polite live region; bones are decorative', (tester) async {
    final handle = tester.ensureSemantics();
    await tester.pumpWidget(host(const AnSkeleton.card()));
    await tester.pump();
    final s = tester.getSemantics(find.byType(AnSkeleton));
    expect(s.flagsCollection.isLiveRegion, isTrue);
    expect(s.label, 'Loading');
    handle.dispose();
  });

  testWidgets('all variants build with no overflow in a narrow box', (tester) async {
    for (final v in [
      const AnSkeleton.text(),
      const AnSkeleton.lines(4),
      const AnSkeleton.row(),
      const AnSkeleton.card(),
    ]) {
      await tester.pumpWidget(host(v, width: 180, reduced: true)); // reduced so pumpAndSettle terminates
      await tester.pumpAndSettle(const Duration(milliseconds: 16), EnginePhase.sendSemanticsUpdate, const Duration(seconds: 5));
      expect(tester.takeException(), isNull, reason: 'variant overflowed or hung');
    }
  });
}
