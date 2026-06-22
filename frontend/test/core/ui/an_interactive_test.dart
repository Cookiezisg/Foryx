import 'package:anselm/core/ui/an_interactive.dart';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';

// AnInteractive is the activation substrate for every control — the key contract is that a DISABLED
// surface activates by neither pointer nor keyboard (the demo matrix's disabled-passthrough gate).
// AnInteractive 是所有控件的激活基座——关键契约:禁用时指针与键盘都不激活(对齐 demo disabled 门)。
void main() {
  testWidgets('enabled surface activates by tap', (tester) async {
    var taps = 0;
    await tester.pumpWidget(MaterialApp(
      home: Center(
        child: AnInteractive(
          onTap: () => taps++,
          builder: (_, _) => const SizedBox(width: 48, height: 48),
        ),
      ),
    ));
    await tester.tap(find.byType(AnInteractive));
    expect(taps, 1);
  });

  testWidgets('pressed state is surfaced while the pointer is down', (tester) async {
    late Set<WidgetState> states;
    await tester.pumpWidget(MaterialApp(
      home: Center(
        child: AnInteractive(
          onTap: () {},
          builder: (_, s) {
            states = s;
            return const SizedBox(width: 48, height: 48);
          },
        ),
      ),
    ));
    final gesture = await tester.startGesture(tester.getCenter(find.byType(AnInteractive)));
    await tester.pump();
    expect(states.contains(WidgetState.pressed), isTrue);
    await gesture.up();
    await tester.pump();
    expect(states.contains(WidgetState.pressed), isFalse);
  });

  testWidgets('enabled surface activates by keyboard (Enter / Space)', (tester) async {
    var taps = 0;
    final focus = FocusNode();
    addTearDown(focus.dispose);
    await tester.pumpWidget(MaterialApp(
      home: Center(
        child: AnInteractive(
          focusNode: focus,
          onTap: () => taps++,
          builder: (_, _) => const SizedBox(width: 48, height: 48),
        ),
      ),
    ));
    focus.requestFocus();
    await tester.pump();
    await tester.sendKeyEvent(LogicalKeyboardKey.enter);
    await tester.pump();
    expect(taps, 1, reason: 'Enter activates a focused surface');
    await tester.sendKeyEvent(LogicalKeyboardKey.space);
    await tester.pump();
    expect(taps, 2, reason: 'Space activates a focused surface');
  });

  testWidgets('disabled surface is NOT focusable and does not activate by keyboard', (tester) async {
    var taps = 0;
    final focus = FocusNode();
    addTearDown(focus.dispose);
    await tester.pumpWidget(MaterialApp(
      home: Center(
        child: AnInteractive(
          enabled: false,
          focusNode: focus,
          onTap: () => taps++,
          builder: (_, _) => const SizedBox(width: 48, height: 48),
        ),
      ),
    ));
    focus.requestFocus();
    await tester.pump();
    expect(focus.hasFocus, isFalse, reason: 'disabled → non-focusable (FAD enabled:false)');
    await tester.sendKeyEvent(LogicalKeyboardKey.enter);
    await tester.pump();
    expect(taps, 0);
  });

  testWidgets('disabled surface is inert (no tap, carries disabled state)', (tester) async {
    var taps = 0;
    late Set<WidgetState> states;
    await tester.pumpWidget(MaterialApp(
      home: Center(
        child: AnInteractive(
          enabled: false,
          onTap: () => taps++,
          builder: (_, s) {
            states = s;
            return const SizedBox(width: 48, height: 48);
          },
        ),
      ),
    ));
    await tester.tap(find.byType(AnInteractive), warnIfMissed: false);
    expect(taps, 0);
    expect(states.contains(WidgetState.disabled), isTrue);
  });
}
