import 'package:anselm/core/design/theme.dart';
import 'package:anselm/core/ui/ui.dart';
import 'package:anselm/i18n/strings.g.dart';
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
