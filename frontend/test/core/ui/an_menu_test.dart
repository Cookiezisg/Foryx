import 'package:anselm/core/design/theme.dart';
import 'package:anselm/core/design/tokens.dart';
import 'package:anselm/core/ui/ui.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';

// AnMenu = floating menu on AnPopover: section labels + items (icon/check/meta, danger/disabled). Picking
// closes unless keepOpen. AnMenu 契约。
void main() {
  Widget host(Widget child) => MaterialApp(
        debugShowCheckedModeBanner: false,
        theme: AnTheme.light(),
        home: Scaffold(body: Center(child: child)),
      );

  Widget menu({required List<AnMenuEntry> entries}) => AnMenu(
        anchorBuilder: (context, toggle, isOpen) =>
            AnButton(label: 'Open', onPressed: toggle),
        entries: entries,
      );

  testWidgets('tapping the anchor opens the menu; items + section label render', (tester) async {
    await tester.pumpWidget(host(menu(entries: [
      const AnMenuSection('Section'),
      AnMenuItem(label: 'Edit', onTap: () {}),
      AnMenuItem(label: 'Delete', danger: true, onTap: () {}),
    ])));
    expect(find.text('Edit'), findsNothing); // closed
    await tester.tap(find.text('Open'));
    await tester.pumpAndSettle();
    expect(find.text('Section'), findsOneWidget);
    expect(find.text('Edit'), findsOneWidget);
    expect(find.text('Delete'), findsOneWidget);
  });

  testWidgets('picking an item fires onTap and closes the menu', (tester) async {
    var picked = 0;
    await tester.pumpWidget(host(menu(entries: [AnMenuItem(label: 'Edit', onTap: () => picked++)])));
    await tester.tap(find.text('Open'));
    await tester.pumpAndSettle();
    await tester.tap(find.text('Edit'));
    await tester.pumpAndSettle();
    expect(picked, 1);
    expect(find.text('Edit'), findsNothing); // closed after pick
  });

  testWidgets('keepOpen item stays open after a tap (multi-check toggle)', (tester) async {
    var toggles = 0;
    await tester.pumpWidget(host(menu(entries: [AnMenuItem(label: 'Show versions', checked: true, keepOpen: true, onTap: () => toggles++)])));
    await tester.tap(find.text('Open'));
    await tester.pumpAndSettle();
    await tester.tap(find.text('Show versions'));
    await tester.pumpAndSettle();
    expect(toggles, 1);
    expect(find.text('Show versions'), findsOneWidget); // still open
  });

  testWidgets('disabled item does not fire / does not close', (tester) async {
    var picked = 0;
    await tester.pumpWidget(host(menu(entries: [AnMenuItem(label: 'Archive', disabled: true, onTap: () => picked++)])));
    await tester.tap(find.text('Open'));
    await tester.pumpAndSettle();
    await tester.tap(find.text('Archive'), warnIfMissed: false);
    await tester.pumpAndSettle();
    expect(picked, 0);
    expect(find.text('Archive'), findsOneWidget); // still open (inert)
  });

  testWidgets('menu hugs content width — a short item is narrower than the max', (tester) async {
    await tester.pumpWidget(host(menu(entries: const [AnMenuItem(label: 'Sort')])));
    await tester.tap(find.text('Open'));
    await tester.pumpAndSettle();
    final shortW = tester.getSize(find.byType(IntrinsicWidth)).width;
    expect(shortW, lessThan(AnSize.menuMaxWidth), reason: 'hugs content, not always the 360 max');
    expect(shortW, greaterThanOrEqualTo(AnSize.menuMinWidth));
  });

  testWidgets('menu width clamps to the max for a very long label', (tester) async {
    await tester.pumpWidget(host(menu(entries: const [
      AnMenuItem(label: 'An extremely long menu item label well past the maximum menu width by a wide margin'),
    ])));
    await tester.tap(find.text('Open'));
    await tester.pumpAndSettle();
    expect(tester.getSize(find.byType(IntrinsicWidth)).width, AnSize.menuMaxWidth, reason: 'long label clamps to max');
  });

  testWidgets('an open menu dismisses when an ancestor scrolls (platform-standard, not stranded)', (tester) async {
    final scroll = ScrollController();
    addTearDown(scroll.dispose);
    await tester.pumpWidget(MaterialApp(
      debugShowCheckedModeBanner: false,
      theme: AnTheme.light(),
      home: Scaffold(
        body: SizedBox(
          height: 400,
          child: ListView(
            controller: scroll,
            children: [
              const SizedBox(height: 60),
              menu(entries: const [AnMenuItem(label: 'Edit'), AnMenuItem(label: 'Delete')]),
              const SizedBox(height: 1200),
            ],
          ),
        ),
      ),
    ));
    await tester.tap(find.text('Open'));
    await tester.pumpAndSettle();
    expect(find.text('Edit'), findsOneWidget); // open
    scroll.jumpTo(80); // ancestor scrolls → the menu must dismiss (not float over the wrong content) 祖先滚动→关
    await tester.pumpAndSettle();
    expect(find.text('Edit'), findsNothing, reason: 'scrolling the ancestor dismisses the open menu');
  });

  testWidgets('the menu uses the shared AnMenuSurface + AnMenuRow standard', (tester) async {
    await tester.pumpWidget(host(menu(entries: const [AnMenuItem(label: 'Edit'), AnMenuItem(label: 'Delete')])));
    await tester.tap(find.text('Open'));
    await tester.pumpAndSettle();
    expect(find.byType(AnMenuSurface), findsOneWidget);
    expect(find.byType(AnMenuRow), findsNWidgets(2));
  });

  testWidgets('a right-aligned menu near the left edge flips/clamps to stay on-screen', (tester) async {
    await tester.pumpWidget(MaterialApp(
      debugShowCheckedModeBanner: false,
      theme: AnTheme.light(),
      home: Scaffold(
        body: Align(
          alignment: Alignment.topLeft, // trigger hard against the left edge 触发器贴左缘
          child: menu(entries: const [AnMenuItem(label: 'Edit'), AnMenuItem(label: 'Delete')]),
        ),
      ),
    ));
    await tester.tap(find.text('Open'));
    await tester.pumpAndSettle();
    final rect = tester.getRect(find.byType(IntrinsicWidth));
    expect(rect.left, greaterThanOrEqualTo(0), reason: 'default alignEnd would extend LEFT off-screen → must flip/clamp');
  });

  testWidgets('opening takes focus; closing hands it back to the pre-open holder (WCAG 2.4.3)',
      (tester) async {
    final probe = FocusNode(debugLabel: 'probe');
    addTearDown(probe.dispose);
    late VoidCallback openMenu;
    await tester.pumpWidget(host(Column(
      mainAxisSize: MainAxisSize.min,
      children: [
        Focus(focusNode: probe, child: const SizedBox(width: 20, height: 20)),
        AnMenu(
          entries: const [AnMenuItem(label: 'One'), AnMenuItem(label: 'Two')],
          anchorBuilder: (context, toggle, isOpen) {
            openMenu = toggle;
            return const SizedBox(width: 20, height: 20);
          },
        ),
      ],
    )));
    probe.requestFocus();
    await tester.pump();
    expect(probe.hasFocus, isTrue);
    openMenu();
    await tester.pumpAndSettle();
    expect(probe.hasFocus, isFalse, reason: 'overlay FocusScope + first-item autofocus take focus on open');
    await tester.tap(find.text('One'));
    await tester.pumpAndSettle();
    expect(probe.hasFocus, isTrue, reason: 'focus handed back to the trigger context on close, not dropped to root');
  });
}
