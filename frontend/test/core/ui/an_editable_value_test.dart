import 'package:anselm/core/design/theme.dart';
import 'package:anselm/core/ui/ui.dart';
import 'package:anselm/i18n/strings.g.dart';
import 'package:flutter/gestures.dart';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';

// AnEditableValue = the two-anchor edit core (Field/Kv). Contract: pencil → field; Enter/✓/blur commit;
// Esc/✕ abort (cancel beats blur via TextFieldTapRegion); empty → em-dash; select editor via dropdown.
// AnEditableValue 双锚编辑核契约。
void main() {
  Future<String Function()> pump(
    WidgetTester tester, {
    String initial = 'v1',
    AnEditKind editor = AnEditKind.input,
    List<AnDropdownOption<String>> options = const [],
  }) async {
    var value = initial;
    await tester.pumpWidget(TranslationProvider(
      child: MaterialApp(
        debugShowCheckedModeBanner: false,
        theme: AnTheme.light(),
        home: Scaffold(
          body: Center(
            child: SizedBox(
              width: 360,
              child: StatefulBuilder(
                builder: (ctx, ss) => AnEditableValue(
                  leading: const Text('Key'),
                  fieldLabel: 'Key',
                  value: value,
                  editor: editor,
                  options: options,
                  onChanged: (v) => ss(() => value = v),
                ),
              ),
            ),
          ),
        ),
      ),
    ));
    return () => value;
  }

  testWidgets('pencil opens the field; Enter commits', (tester) async {
    final read = await pump(tester);
    await tester.tap(find.byIcon(AnIcons.edit));
    await tester.pump();
    expect(find.byType(TextField), findsOneWidget);
    await tester.enterText(find.byType(TextField), 'v2');
    await tester.testTextInput.receiveAction(TextInputAction.done);
    await tester.pumpAndSettle();
    expect(read(), 'v2');
    expect(find.byType(TextField), findsNothing); // field closed after commit
  });

  testWidgets('Esc aborts — value unchanged, field closes', (tester) async {
    final read = await pump(tester);
    await tester.tap(find.byIcon(AnIcons.edit));
    await tester.pump();
    await tester.enterText(find.byType(TextField), 'typed');
    await tester.sendKeyEvent(LogicalKeyboardKey.escape);
    await tester.pumpAndSettle();
    expect(read(), 'v1');
    expect(find.byType(TextField), findsNothing);
  });

  testWidgets('Cancel aborts — NOT a blur-commit (cancel-priority via TextFieldTapRegion)', (tester) async {
    final read = await pump(tester);
    await tester.tap(find.byIcon(AnIcons.edit));
    await tester.pump();
    await tester.enterText(find.byType(TextField), 'typed');
    await tester.pump();
    await tester.tap(find.text('Cancel'));
    await tester.pumpAndSettle();
    expect(read(), 'v1'); // aborted, not committed-on-blur
  });

  testWidgets('mouse-click ✓ does NOT focus the pencil (no focus ring)', (tester) async {
    // The pencil must not gain focus from a ✓✕ CLICK — neither via an explicit requestFocus (the button
    // path passes returnFocus:false) nor via Flutter restoring focus when the focused field is removed
    // (dropped pre-rebuild in _finish). Else revealPencil (reads hasFocus) pins it visible + paints a focus
    // ring. Uses a real MOUSE pointer so highlightMode is `traditional` (desktop). NB: the headless text
    // field never takes primary focus, so this guards the EXPLICIT-focus path (the prior regression: a
    // ✓ click calling returnFocus:true); the restoration path is verified on device via `make gallery`.
    // 鼠标点 ✓ 不该聚焦铅笔(否则可见+焦点框)。用鼠标指针(桌面 traditional);无头文本框不取主焦点,故守显式聚焦路径,恢复路径真机验。
    final read = await pump(tester);
    final mouse = await tester.createGesture(kind: PointerDeviceKind.mouse);
    await mouse.addPointer(location: Offset.zero);
    addTearDown(mouse.removePointer);

    Future<void> click(Finder f) async {
      final p = tester.getCenter(f);
      await mouse.moveTo(p);
      await tester.pump();
      await mouse.down(p);
      await mouse.up();
      await tester.pumpAndSettle();
    }

    await click(find.byIcon(AnIcons.edit)); // open
    await tester.enterText(find.byType(TextField), 'changed');
    await tester.pump();
    await click(find.text('Save')); // commit with a MOUSE click

    expect(read(), 'changed'); // committed
    expect(find.byType(TextField), findsNothing); // edit closed, pencil back
    expect(FocusManager.instance.primaryFocus?.debugLabel, isNot('AnEditableValue.pencil'),
        reason: 'a ✓ click must not pull focus onto the pencil (else it shows a focus ring)');
  });

  testWidgets('Save commits', (tester) async {
    final read = await pump(tester);
    await tester.tap(find.byIcon(AnIcons.edit));
    await tester.pump();
    await tester.enterText(find.byType(TextField), 'saved');
    await tester.pump();
    await tester.tap(find.text('Save'));
    await tester.pumpAndSettle();
    expect(read(), 'saved');
  });

  testWidgets('blur (tap outside) commits the typed value', (tester) async {
    final read = await pump(tester);
    await tester.tap(find.byIcon(AnIcons.edit));
    await tester.pump();
    await tester.enterText(find.byType(TextField), 'blurred');
    await tester.pump();
    await tester.tapAt(const Offset(5, 5)); // far outside the field's TextFieldTapRegion
    await tester.pumpAndSettle();
    expect(read(), 'blurred'); // onTapOutside → commit
    expect(find.byType(TextField), findsNothing);
  });

  testWidgets('empty value shows an em-dash placeholder', (tester) async {
    await pump(tester, initial: '');
    expect(find.text('—'), findsOneWidget);
  });

  testWidgets('select editor: an always-present dropdown — a pick commits', (tester) async {
    final read = await pump(tester, editor: AnEditKind.select, initial: 'low', options: const [
      AnDropdownOption(value: 'low', label: 'Low'),
      AnDropdownOption(value: 'high', label: 'High'),
    ]);
    expect(find.byType(AnDropdown<String>), findsOneWidget); // no pencil step — the dropdown IS the editor
    await tester.tap(find.byType(AnDropdown<String>));
    await tester.pumpAndSettle();
    await tester.tap(find.text('High').last);
    await tester.pumpAndSettle();
    expect(read(), 'high');
  });

  testWidgets('select editor: dismiss without pick leaves value unchanged (no dangling state)', (tester) async {
    final read = await pump(tester, editor: AnEditKind.select, initial: 'low', options: const [
      AnDropdownOption(value: 'low', label: 'Low'),
      AnDropdownOption(value: 'high', label: 'High'),
    ]);
    await tester.tap(find.byType(AnDropdown<String>));
    await tester.pumpAndSettle();
    await tester.sendKeyEvent(LogicalKeyboardKey.escape); // dismiss the menu without picking
    await tester.pumpAndSettle();
    expect(read(), 'low'); // unchanged — the dropdown just closes, nothing stuck
    expect(tester.takeException(), isNull);
  });
}
