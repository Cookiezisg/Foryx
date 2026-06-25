import 'package:anselm/dev/gallery/gallery_app.dart';
import 'package:anselm/i18n/strings.g.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';

/// REGRESSION — the gallery must OPEN AT THE TOP. An edit-state specimen (AnInlineEdit /
/// AnEditableValue with `startEditing`) mounts a seamless field whose `autofocus` would otherwise (a)
/// steal app focus on launch and (b) make EditableText.showOnScreen scroll the page down to the field
/// (it used to open ~73% down — user-reported). `gallery_app` wraps the catalog scroll body in
/// [ExcludeFocus] to neutralise that. This guards the fix stays in place for every category.
/// 画廊必须从顶部打开:编辑态 specimen 的 autofocus 会把页面滚下去;ExcludeFocus 守住——此测试为其回归网。
void main() {
  for (var cat = 0; cat < 6; cat++) {
    testWidgets('gallery category $cat opens at the top (no autofocus scroll)', (tester) async {
      tester.view.physicalSize = const Size(1440, 900);
      tester.view.devicePixelRatio = 1.0;
      addTearDown(tester.view.resetDevicePixelRatio);
      addTearDown(tester.view.reset);

      await tester.pumpWidget(
        TranslationProvider(
          child: ProviderScope(
            child: GalleryApp(key: ValueKey('cat$cat'), initialCategory: cat),
          ),
        ),
      );
      // NOT pumpAndSettle — some specimens run legit repeating animations (status dots, spinners) that
      // never settle. A few fixed frames are enough: the autofocus showOnScreen scroll (the regression)
      // fires in a post-frame callback on the first build. 不用 pumpAndSettle(合法循环动画);固定几帧即足。
      await tester.pump();
      await tester.pump(const Duration(milliseconds: 300));
      await tester.pump(const Duration(milliseconds: 300));

      // The vertical PAGE scrollable = the down-axis one with the largest scroll extent.
      ScrollableState? page;
      for (final el in find.byType(Scrollable).evaluate()) {
        final s = (el as StatefulElement).state as ScrollableState;
        if (s.axisDirection == AxisDirection.down &&
            (page == null || s.position.maxScrollExtent > page.position.maxScrollExtent)) {
          page = s;
        }
      }
      expect(page, isNotNull, reason: 'category $cat should have a vertical page scrollable');
      expect(
        page!.position.pixels,
        0.0,
        reason: 'category $cat must open at the top — a specimen autofocus must not scroll the page',
      );
    });
  }
}
