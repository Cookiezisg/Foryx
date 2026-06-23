import 'package:anselm/core/ui/ui.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';

// AnTwoZone is the shared right-anchored two-zone skeleton (promoted from AnDropdown's _TwoZone).
// The dropdown's own regression suite proves byte-equal behavior in context; these cover the
// primitive directly: label fills left, meta caps right, trailing pins to the right edge.
// AnTwoZone = 升格自 AnDropdown _TwoZone 的右锚两区骨架。AnDropdown 测试证 byte-equal,此处直测原语。
void main() {
  Widget host(Widget child, {double width = 280}) => MaterialApp(
        debugShowCheckedModeBanner: false,
        home: Scaffold(body: Center(child: SizedBox(width: width, child: child))),
      );

  testWidgets('label renders and trailing is pinned to the right edge', (tester) async {
    const trailingKey = Key('trail');
    await tester.pumpWidget(host(const AnTwoZone(
      label: Text('Name'),
      trailing: SizedBox(key: trailingKey, width: 16, height: 16),
    )));
    expect(find.text('Name'), findsOneWidget);
    final zoneRight = tester.getTopRight(find.byType(AnTwoZone)).dx;
    final trailRight = tester.getTopRight(find.byKey(trailingKey)).dx;
    // label is Expanded (greedy) → trailing sits flush against the row's right edge.
    expect((zoneRight - trailRight).abs(), lessThan(1.0), reason: 'trailing pinned right');
  });

  testWidgets('no meta → only label + trailing, no overflow', (tester) async {
    await tester.pumpWidget(host(const AnTwoZone(label: Text('K'), trailing: Icon(Icons.check, size: 16))));
    expect(tester.takeException(), isNull);
  });

  testWidgets('long label ellipsis-truncates without overflow', (tester) async {
    await tester.pumpWidget(host(
      const AnTwoZone(
        label: Text('A very very very long primary label that must ellipsis',
            maxLines: 1, overflow: TextOverflow.ellipsis),
        trailing: SizedBox(width: 16, height: 16),
      ),
      width: 160,
    ));
    expect(tester.takeException(), isNull);
    expect(find.byType(AnTwoZone), findsOneWidget);
  });

  testWidgets('long meta is capped (≤45%) so it never crowds out the label — no overflow', (tester) async {
    const longMeta = 'an_extremely_long_meta_identifier_that_would_overflow_if_uncapped_0123456789';
    await tester.pumpWidget(host(
      const AnTwoZone(
        label: Text('Primary', maxLines: 1, overflow: TextOverflow.ellipsis),
        meta: longMeta,
        trailing: SizedBox(width: 16, height: 16),
      ),
      width: 200,
    ));
    expect(tester.takeException(), isNull);
    expect(find.text('Primary'), findsOneWidget); // label survives, not crowded out
    // Assert the CAP MECHANISM directly: the meta zone's ConstrainedBox.maxWidth == 45% of the row
    // (200*0.45=90). Asserting RENDERED width can't catch a loosened cap — the Expanded label + finite
    // Row mask it; pinning the constraint means raising _kMetaMaxFraction turns this red.
    // 直接锁 cap 机制:meta 区 ConstrainedBox.maxWidth == 行宽 45%(改大 _kMetaMaxFraction 必红;验渲染宽抓不到)。
    final metaBox = tester
        .widgetList<ConstrainedBox>(find.ancestor(of: find.text(longMeta), matching: find.byType(ConstrainedBox)))
        .first;
    expect(metaBox.constraints.maxWidth, closeTo(200 * 0.45, 0.01));
  });
}
