import 'package:anselm/core/design/theme.dart';
import 'package:anselm/core/ui/an_expand_reveal.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';

// AnExpandReveal = the kit's shared collapse/expand reveal (ClipRect + Align heightFactor, nestable).
// Open shows the child + animates; closed removes it from the tree (collapsed rows aren't focusable).
void main() {
  Widget host(Widget child) => MaterialApp(
        debugShowCheckedModeBanner: false,
        theme: AnTheme.light(),
        home: Scaffold(body: Center(child: child)),
      );

  testWidgets('open shows the child; closed removes it from the tree', (tester) async {
    await tester.pumpWidget(host(const AnExpandReveal(open: true, child: Text('PANEL'))));
    await tester.pumpAndSettle();
    expect(find.text('PANEL'), findsOneWidget);

    await tester.pumpWidget(host(const AnExpandReveal(open: false, child: Text('PANEL'))));
    await tester.pumpAndSettle();
    expect(find.text('PANEL'), findsNothing, reason: 'fully collapsed → dropped from the tree (not just clipped)');
  });

  testWidgets('toggling open animates the height (intermediate frame between 0 and full)', (tester) async {
    var open = false;
    await tester.pumpWidget(host(StatefulBuilder(
      builder: (ctx, ss) => Column(mainAxisSize: MainAxisSize.min, children: [
        TextButton(onPressed: () => ss(() => open = true), child: const Text('go')),
        AnExpandReveal(open: open, child: const SizedBox(height: 100, child: Text('PANEL'))),
      ]),
    )));
    await tester.tap(find.text('go'));
    await tester.pump(); // start the tween
    await tester.pump(const Duration(milliseconds: 120)); // mid-flight
    final mid = tester.getSize(find.text('PANEL')).height; // Align clips via heightFactor < 1
    // the panel's own text height is stable; the REVEALED height is what we assert via the ClipRect/Align —
    // measure the Align's box instead
    final alignH = tester.getSize(find.byType(Align).last).height;
    expect(alignH, greaterThan(0));
    expect(alignH, lessThan(100), reason: 'mid-tween: revealed height between 0 and full (animating, not instant)');
    expect(mid, greaterThan(0));
    await tester.pumpAndSettle();
    expect(tester.getSize(find.byType(Align).last).height, 100, reason: 'settles to full height');
  });

  testWidgets('reduced motion → instant (no intermediate frame)', (tester) async {
    var open = false;
    await tester.pumpWidget(MaterialApp(
      theme: AnTheme.light(),
      home: MediaQuery(
        data: const MediaQueryData(disableAnimations: true),
        child: Scaffold(body: StatefulBuilder(
          builder: (ctx, ss) => Column(mainAxisSize: MainAxisSize.min, children: [
            TextButton(onPressed: () => ss(() => open = true), child: const Text('go')),
            AnExpandReveal(open: open, child: const SizedBox(height: 80, child: Text('P'))),
          ]),
        )),
      ),
    ));
    await tester.tap(find.text('go'));
    await tester.pump(); // single frame
    expect(tester.getSize(find.byType(Align).last).height, 80, reason: 'reduced motion snaps to full in one frame');
  });
}
