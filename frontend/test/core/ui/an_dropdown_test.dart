import 'package:anselm/core/design/theme.dart';
import 'package:anselm/core/ui/ui.dart';
import 'package:anselm/i18n/strings.g.dart';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';

// The dropdown opens an overlay menu — the matrix only builds the trigger, so the menu's open/
// select/dismiss + the massive-list (海量) overflow are covered here.
// 下拉开浮层菜单——矩阵只 build 触发器,故菜单的开/选/关 + 海量溢出在此覆盖。
void main() {
  Widget host(Widget child) => TranslationProvider(
        child: MaterialApp(
          debugShowCheckedModeBanner: false,
          theme: AnTheme.light(),
          home: Scaffold(body: Center(child: SizedBox(width: 280, child: child))),
        ),
      );

  const opts = [
    AnDropdownOption(value: 'a', label: 'Apple'),
    AnDropdownOption(value: 'b', label: 'Banana'),
  ];

  testWidgets('opens, selects a value, closes', (tester) async {
    String? picked;
    String? value;
    await tester.pumpWidget(host(StatefulBuilder(
      builder: (context, setState) => AnDropdown<String>(
        options: opts,
        value: value,
        onChanged: (v) {
          picked = v;
          setState(() => value = v);
        },
      ),
    )));

    expect(find.text('Banana'), findsNothing); // menu closed
    await tester.tap(find.byType(AnDropdown<String>));
    await tester.pumpAndSettle();
    expect(find.text('Apple'), findsOneWidget); // menu open
    expect(find.text('Banana'), findsOneWidget);

    await tester.tap(find.text('Banana'));
    await tester.pumpAndSettle();
    expect(picked, 'b');
    expect(find.text('Banana'), findsOneWidget); // echoed in trigger
    expect(find.text('Apple'), findsNothing); // menu dismissed
  });

  testWidgets('the menu uses the shared AnMenuSurface + AnMenuRow standard (rows match AnMenu)', (tester) async {
    await tester.pumpWidget(host(AnDropdown<String>(options: opts, value: 'a', onChanged: (_) {})));
    await tester.tap(find.byType(AnDropdown<String>));
    await tester.pumpAndSettle();
    // same chrome + row primitives as AnMenu → the selected/hover pill is a rounded inset, not edge-to-edge. 共用标准。
    expect(find.byType(AnMenuSurface), findsOneWidget);
    expect(find.byType(AnMenuRow), findsNWidgets(opts.length));
  });

  testWidgets('menu is keyboard-navigable — arrow focuses a row, Enter selects', (tester) async {
    // After the FAD rewrite the rows are focusable + Enter-activatable; MaterialApp maps arrow keys
    // to directional focus, so the menu gets keyboard nav without per-row wiring.
    // FAD 改造后行可聚焦 + Enter 激活;MaterialApp 把方向键映射到方向聚焦 → 菜单免接线即可键盘导航。
    String? picked;
    await tester.pumpWidget(host(AnDropdown<String>(options: opts, value: null, onChanged: (v) => picked = v)));
    await tester.tap(find.byType(AnDropdown<String>));
    await tester.pumpAndSettle();
    await tester.sendKeyEvent(LogicalKeyboardKey.arrowDown);
    await tester.pumpAndSettle();
    await tester.sendKeyEvent(LogicalKeyboardKey.enter);
    await tester.pumpAndSettle();
    expect(picked, isNotNull, reason: 'arrow-down then Enter should select a row');
  });

  testWidgets('disabled does not open', (tester) async {
    await tester.pumpWidget(host(const AnDropdown<String>(
      options: opts,
      value: null,
      onChanged: null,
      enabled: false,
    )));
    await tester.tap(find.byType(AnDropdown<String>), warnIfMissed: false);
    await tester.pump(const Duration(milliseconds: 50));
    expect(find.text('Apple'), findsNothing);
  });

  testWidgets('block dropdown in a wide container opens menu without non-normalized constraints', (tester) async {
    // Regression: a full-width trigger makes the menu's minWidth large; the maxWidth cap must rise
    // with it or BoxConstraints goes minWidth>maxWidth (the real-run white/red error).
    // 回归:块级触发器→菜单 minWidth 大,maxWidth 上限须随之抬,否则 min>max 非法(真跑报错)。
    await tester.pumpWidget(TranslationProvider(
      child: MaterialApp(
        debugShowCheckedModeBanner: false,
        theme: AnTheme.light(),
        home: Scaffold(
          body: SizedBox(
            width: 900,
            child: AnDropdown<String>(options: opts, value: 'a', block: true, onChanged: (_) {}),
          ),
        ),
      ),
    ));
    await tester.tap(find.byType(AnDropdown<String>));
    await tester.pumpAndSettle();
    expect(tester.takeException(), isNull);
    expect(find.text('Banana'), findsOneWidget);
  });

  testWidgets('ghost (compact trigger) menu fits rich rows — no overflow', (tester) async {
    // Regression: forcing menu width == a compact ghost trigger overflowed the rich rows. The menu
    // now clamps to [menuMin, menuMax], so a narrow trigger still yields a wide-enough menu.
    // 回归:菜单=紧凑 ghost 触发器宽 → 富行溢出。现夹到 [min,max],窄触发器也给够宽菜单。
    final rich = const [
      AnDropdownOption(value: 'fn', label: 'Function', meta: 'fn_3a9f'),
      AnDropdownOption(value: 'ag', label: 'Agent', meta: 'ag_0e88'),
    ];
    await tester.pumpWidget(host(AnDropdown<String>(
      options: rich,
      value: 'fn',
      variant: AnDropdownVariant.ghost,
      onChanged: (_) {},
    )));
    await tester.tap(find.byType(AnDropdown<String>));
    await tester.pumpAndSettle();
    expect(tester.takeException(), isNull);
    expect(find.text('Agent'), findsOneWidget);
  });

  testWidgets('massive option list opens and scrolls without overflow', (tester) async {
    // Mirror the gallery's massive specimen: rich rows WITH meta, so the gate exercises the same
    // AnTwoZone two-zone overflow path the gallery shows. 与画廊 massive 一致:带 meta 的富行。
    final many = [for (var i = 0; i < 80; i++) AnDropdownOption(value: '$i', label: 'Option number $i', meta: 'opt_$i')];
    await tester.pumpWidget(host(AnDropdown<String>(options: many, value: '0', onChanged: (_) {})));
    await tester.tap(find.byType(AnDropdown<String>));
    await tester.pumpAndSettle();
    expect(tester.takeException(), isNull);
    expect(find.text('Option number 0'), findsWidgets);
    // the menu is scrollable — drag up and confirm no overflow after scrolling
    await tester.drag(find.text('Option number 0').last, const Offset(0, -400));
    await tester.pumpAndSettle();
    expect(tester.takeException(), isNull);
  });
}
