import 'package:anselm/core/design/theme.dart';
import 'package:anselm/core/design/tokens.dart';
import 'package:anselm/core/ui/an_island.dart';
import 'package:anselm/core/ui/an_shell.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';

/// Skeleton guards for the three-island shell: a draggable left island (240–400, default 320) +
/// a fixed right island (320) + the open ocean, and the window minimum guarantees the ocean's
/// content column never drops below its minimum even with the left island at its max.
/// 三岛 shell 骨架守卫:左岛可拖(240–400 默认 320)+ 右岛固定(320)+ 敞开海洋;窗口最小保证即便左岛
/// 拖到最大、海洋内容列仍不低于最小。
void main() {
  Widget harness() => MaterialApp(theme: AnTheme.light(), home: const AnShell());

  testWidgets('renders left(default 320, draggable) + right(fixed 320) islands + ocean',
      (tester) async {
    tester.view.physicalSize = const Size(1400, 900);
    tester.view.devicePixelRatio = 1.0;
    addTearDown(tester.view.reset);

    await tester.pumpWidget(harness());
    await tester.pump();

    expect(find.byType(AnIsland), findsNWidgets(2));
    expect(tester.getSize(find.byType(AnIsland).first).width, AnSize.sidebar); // left default 320
    expect(tester.getSize(find.byType(AnIsland).last).width, AnSize.rightIsland); // right fixed 320
    expect(find.text('Sidebar'), findsOneWidget);
    expect(find.text('Ocean'), findsOneWidget);
    expect(find.text('Inspector'), findsOneWidget);
  });

  testWidgets('left island drags within [min, max]; right stays fixed', (tester) async {
    tester.view.physicalSize = const Size(1400, 900);
    tester.view.devicePixelRatio = 1.0;
    addTearDown(tester.view.reset);
    await tester.pumpWidget(harness());
    await tester.pump();

    await tester.drag(find.byKey(const ValueKey('anShellLeftGrip')), const Offset(60, 0));
    await tester.pump();
    expect(tester.getSize(find.byType(AnIsland).first).width, AnSize.sidebar + 60); // 380
    expect(tester.getSize(find.byType(AnIsland).last).width, AnSize.rightIsland); // right unchanged

    await tester.drag(find.byKey(const ValueKey('anShellLeftGrip')), const Offset(999, 0));
    await tester.pump();
    expect(tester.getSize(find.byType(AnIsland).first).width, AnSize.sidebarMax); // clamped 400
  });

  testWidgets('inspectorOpen reveals/hides the right island; the ocean reclaims its width',
      (tester) async {
    tester.view.physicalSize = const Size(1400, 900);
    tester.view.devicePixelRatio = 1.0;
    addTearDown(tester.view.reset);

    Future<double> oceanWidth({required bool open}) async {
      await tester.pumpWidget(MaterialApp(
        theme: AnTheme.light(),
        home: AnShell(
          ocean: const SizedBox.expand(key: ValueKey('oceanProbe')),
          inspectorOpen: open,
        ),
      ));
      await tester.pumpAndSettle(); // let the reveal animation finish 让揭示动画走完
      return tester.getSize(find.byKey(const ValueKey('oceanProbe'))).width;
    }

    final openW = await oceanWidth(open: true);
    final closedW = await oceanWidth(open: false);
    // Hiding the right island hands the ocean exactly the island + its gap (it slides out, no reflow).
    // 收起右岛 → 海洋正好多得岛宽 + 间距(滑出、不重排)。
    expect(closedW, greaterThan(openW));
    expect(closedW - openW, closeTo(AnSize.rightIsland + AnSize.shellGap, 0.5));
  });

  testWidgets('collapsed right island is inert — its content leaves the semantics tree (no focus trap)',
      (tester) async {
    final handle = tester.ensureSemantics();
    tester.view.physicalSize = const Size(1400, 900);
    tester.view.devicePixelRatio = 1.0;
    addTearDown(tester.view.reset);

    Widget shell(bool open) => MaterialApp(
          theme: AnTheme.light(),
          home: AnShell(
            inspector: const Text('inspector body', semanticsLabel: 'inspectorProbe'),
            inspectorOpen: open,
          ),
        );

    await tester.pumpWidget(shell(true));
    await tester.pumpAndSettle();
    expect(find.bySemanticsLabel('inspectorProbe'), findsOneWidget); // open → announced

    // Mid-close: the island is STILL painted (sliding out, content held full-width behind the clip) — the
    // ExcludeFocus/ExcludeSemantics wrapper must keep it inert NOW (this is the transient the wrapper guards;
    // once fully closed the subtree is dropped entirely). 滑出中仍绘制,惰化包裹须此刻生效。
    await tester.pumpWidget(shell(false));
    await tester.pump(const Duration(milliseconds: 120)); // partway through the slide-out 滑出途中
    expect(find.bySemanticsLabel('inspectorProbe'), findsNothing, reason: 'sliding-out content excluded from semantics');

    await tester.pumpAndSettle();
    expect(find.bySemanticsLabel('inspectorProbe'), findsNothing, reason: 'fully closed → subtree removed (SizedBox.shrink)');
    handle.dispose();
  });

  testWidgets('the open right island shadow is intact + matches the left (not cut by a clip)', (tester) async {
    tester.view.physicalSize = const Size(1400, 900);
    tester.view.devicePixelRatio = 1.0;
    addTearDown(tester.view.reset);
    await tester.pumpWidget(harness()); // inspectorOpen defaults true
    await tester.pumpAndSettle();

    // Both islands are the SAME AnIsland primitive → identical shadowFloat (one source). 同一原语=阴影同源。
    expect(find.byType(AnIsland), findsNWidgets(2));
    // The open right island's reveal clip uses a NO-OP clipper, so the float shadow paints past the bounds
    // (unlike the old always-on ClipRect that cut it into a pointy dead corner). 敞开态用空裁切器,阴影不被裁。
    final clip = tester.widget<ClipRect>(
      find.ancestor(of: find.byType(AnIsland).last, matching: find.byType(ClipRect)),
    );
    expect(clip.clipper, isNotNull, reason: 'open island → no-op clipper → shadow not clipped (matches the left island)');
  });

  test('minimum window keeps the ocean ≥ its min column even with the left island at max', () {
    expect(
      AnSize.windowMinWidth,
      AnSize.shellPad +
          AnSize.sidebarMax +
          AnSize.shellGap +
          AnSize.oceanMin +
          AnSize.shellGap +
          AnSize.rightIsland +
          AnSize.shellPad,
    );
    // Worst case: left at MAX → ocean at the minimum window is exactly oceanMin (never below).
    final oceanWorstCase = AnSize.windowMinWidth -
        2 * AnSize.shellPad -
        AnSize.sidebarMax -
        2 * AnSize.shellGap -
        AnSize.rightIsland;
    expect(oceanWorstCase, greaterThanOrEqualTo(AnSize.oceanMin));
    expect(AnSize.windowMinHeight, closeTo(AnSize.windowMinWidth / AnSize.goldenRatio, 0.01));
    expect(AnSize.windowMinWidth, lessThan(1512)); // fits a scaled 14" MacBook with margin 留余量
  });
}
