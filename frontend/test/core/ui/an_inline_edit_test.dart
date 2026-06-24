import 'package:anselm/core/design/theme.dart';
import 'package:anselm/core/design/tokens.dart';
import 'package:anselm/core/design/typography.dart';
import 'package:anselm/core/ui/ui.dart';
import 'package:anselm/i18n/strings.g.dart';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';

// AnInlineEdit composes a content-sized seamless field + the edit affordance into a fixed-height
// rename row. The grow-then-cap LAYOUT is verified by the matrix (no-overflow) + a real run; here we
// cover the STATE machine (begin / commit / abort, Enter, Esc, disabled) + the empty/narrow edges.
// AnInlineEdit 把自适应 seamless 框 + 编辑触发器组成定高重命名行。增长封顶布局由矩阵(不溢出)+ 真跑验;此处覆状态机。
void main() {
  Widget host(Widget child, {double width = 280}) => TranslationProvider(
        child: MaterialApp(
          debugShowCheckedModeBanner: false,
          theme: AnTheme.light(),
          home: Scaffold(body: Center(child: SizedBox(width: width, child: child))),
        ),
      );

  testWidgets('style + minHeight parameterize the row (H2 title rename, no font jump on toggle)', (tester) async {
    final style = AnText.h2.weight(FontWeight.w600);
    await tester.pumpWidget(host(AnInlineEdit(value: 'Big Title', style: style, minHeight: AnSize.islandHead, onCommit: (_) {})));
    // idle Text uses the H2 style (not body). idle 走 H2 而非 body。
    final idle = tester.widget<Text>(find.text('Big Title'));
    expect(idle.style?.fontSize, AnText.h2.fontSize);
    // the row is the taller given height. 行高 = 给定的高。
    expect(tester.getSize(find.byType(AnInlineEdit)).height, AnSize.islandHead);
    // entering edit keeps the same font size (the field uses the same style). 编辑态字号不变。
    await tester.tap(find.byType(AnButton));
    await tester.pumpAndSettle();
    expect(find.byType(AnInput), findsOneWidget);
    expect(tester.getSize(find.byType(AnInlineEdit)).height, AnSize.islandHead);
  });

  testWidgets('editing draws the edit frame (bordered) without growing the row height', (tester) async {
    await tester.pumpWidget(host(AnInlineEdit(value: 'Hello', minHeight: AnSize.control, onCommit: (_) {})));
    final idleH = tester.getSize(find.byType(AnInlineEdit)).height;

    await tester.tap(find.byType(AnButton)); // pencil → edit
    await tester.pumpAndSettle();
    expect(find.byType(AnSeamlessField), findsOneWidget);
    // the frame is a bordered DecoratedBox inside the seamless field (1px lineStrong inset, tag radius). 框=有边 DecoratedBox。
    final bordered = tester.widgetList<DecoratedBox>(
      find.descendant(of: find.byType(AnSeamlessField), matching: find.byType(DecoratedBox)),
    ).where((d) => (d.decoration as BoxDecoration).border != null);
    expect(bordered, isNotEmpty, reason: 'edit mode shows the bordered frame');
    // no vertical jump: the row keeps its height despite the frame's vertical bleed. 行高不变(框纵向溢出余量)。
    expect(tester.getSize(find.byType(AnInlineEdit)).height, idleH);
  });

  testWidgets('H2 edit frame fits the zero-slack production row height (no clip past the row)', (tester) async {
    // AnOceanHeader's production formula: minHeight = h2 line box + editBoxPadY*2 — EXACTLY the frame height
    // (zero slack), so this pins frame-height <= row-height (Clip.none overflow wouldn't otherwise error).
    final style = AnText.h2.weight(FontWeight.w600);
    final minH = style.fontSize! * (style.height ?? 1.0) + AnSize.editBoxPadY * 2;
    await tester.pumpWidget(host(AnInlineEdit(value: 'Title', style: style, minHeight: minH, startEditing: true, onCommit: (_) {})));
    await tester.pumpAndSettle();
    final rowH = tester.getSize(find.byType(AnInlineEdit)).height;
    final frame = find.descendant(
      of: find.byType(AnSeamlessField),
      matching: find.byWidgetPredicate((w) => w is DecoratedBox && (w.decoration as BoxDecoration).border != null),
    );
    expect(frame, findsOneWidget);
    expect(tester.getSize(frame).height, lessThanOrEqualTo(rowH + 0.5),
        reason: 'the edit frame must not exceed the zero-slack H2 row height (no clip)');
  });

  testWidgets('idle shows the value; tapping the pencil enters edit', (tester) async {
    await tester.pumpWidget(host(AnInlineEdit(value: 'Hello', onCommit: (_) {})));
    expect(find.text('Hello'), findsOneWidget);
    expect(find.byType(AnInput), findsNothing); // not editing yet

    await tester.tap(find.byType(AnButton)); // the only AnButton in idle is the pencil
    await tester.pumpAndSettle();
    expect(find.byType(AnInput), findsOneWidget); // now editing
  });

  testWidgets('startEditing opens with the value selected (first keystroke replaces)', (tester) async {
    await tester.pumpWidget(host(AnInlineEdit(value: 'Hello', startEditing: true, onCommit: (_) {})));
    await tester.pump();
    final sel = tester.widget<TextField>(find.byType(TextField)).controller!.selection;
    expect(sel.start, 0);
    expect(sel.end, 'Hello'.length); // whole value selected — Finder/F2 rename convention 全选
  });

  testWidgets('Enter commits the typed value and returns to idle', (tester) async {
    String? committed;
    await tester.pumpWidget(host(AnInlineEdit(value: 'Hello', startEditing: true, onCommit: (v) => committed = v)));
    expect(find.byType(AnInput), findsOneWidget);

    await tester.enterText(find.byType(TextField), 'World');
    await tester.testTextInput.receiveAction(TextInputAction.done); // onSubmitted → commit
    await tester.pumpAndSettle();

    expect(committed, 'World');
    expect(find.byType(AnInput), findsNothing); // back to idle
    expect(find.text('World'), findsOneWidget); // resting text updated
  });

  testWidgets('Esc aborts — reverts to the original, no commit', (tester) async {
    String? committed;
    await tester.pumpWidget(host(AnInlineEdit(value: 'Hello', startEditing: true, onCommit: (v) => committed = v)));
    await tester.enterText(find.byType(TextField), 'World');
    await tester.sendKeyEvent(LogicalKeyboardKey.escape);
    await tester.pumpAndSettle();

    expect(committed, isNull); // never committed
    expect(find.byType(AnInput), findsNothing); // back to idle
    expect(find.text('Hello'), findsOneWidget); // reverted to original
  });

  testWidgets('disabled does not enter edit', (tester) async {
    await tester.pumpWidget(host(AnInlineEdit(value: 'X', enabled: false, onCommit: (_) {})));
    await tester.tap(find.byType(AnButton), warnIfMissed: false);
    await tester.pump(const Duration(milliseconds: 50));
    expect(find.byType(AnInput), findsNothing);
  });

  testWidgets('long content caps at the space the affordance leaves, then scrolls — no overflow', (tester) async {
    // 220px > the row's natural minimum (Cancel+Save+gap ≈ 179, which can't shrink). Long content
    // must therefore CAP at the leftover (the Flexible's freeSpace) and scroll, not overflow.
    // 220 > 行自然最小(取消+保存+间距≈179、不可压);长内容须封顶到剩余空间(Flexible 余量)横滚、不溢出。
    await tester.pumpWidget(host(
      AnInlineEdit(
        value: 'A very long title being edited that grows past the cap and must scroll, not overflow',
        startEditing: true,
        onCommit: (_) {},
      ),
      width: 220,
    ));
    await tester.pumpAndSettle();
    expect(tester.takeException(), isNull);
    // The field is capped well below the content's natural width → it scrolls. 框被封顶、远小于内容自然宽 → 横滚。
    final fieldW = tester.getSize(find.byType(AnInput)).width;
    expect(fieldW, lessThan(80), reason: 'field must cap at the leftover space, not take its content width');
  });

  testWidgets('empty value while editing keeps a clickable min-width field (no collapse)', (tester) async {
    await tester.pumpWidget(host(AnInlineEdit(value: '', startEditing: true, onCommit: (_) {})));
    await tester.pumpAndSettle();
    expect(tester.takeException(), isNull);
    expect(find.byType(AnInput), findsOneWidget);
    // The seamless field sits inside a ConstrainedBox(minWidth: inlineEditMin) so an empty field stays
    // wide enough to click. 空框被 inlineEditMin 兜底、仍可点。
    expect(tester.getSize(find.byType(AnInput)).width, greaterThanOrEqualTo(AnSize.inlineEditMin));
  });
}
