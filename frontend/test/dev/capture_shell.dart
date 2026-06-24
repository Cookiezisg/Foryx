// Dev screenshot harness — NOT part of the `flutter test` suite proper (it depends on the
// bundled font + writes a PNG). Run explicitly:  flutter test test/dev/capture_shell.dart
// Renders the three-island shell skeleton headlessly via Skia (no Xcode) → test/dev/out/shell.png
// so the layout/spacing/font can be inspected without launching the app. The macOS traffic
// lights are OS-drawn (absent here), so the left island's leading zone shows as reserved empty
// space — layout is still faithful.
// 开发截图夹具:无头渲染三岛骨架成 PNG,免起 app 看布局/间距/字体。红绿灯是 OS 画的(此处无),
// 故前导区显为留白——布局仍忠实。
import 'dart:io';
import 'dart:ui' as ui;

import 'package:anselm/core/design/theme.dart';
import 'package:anselm/core/design/tokens.dart';
import 'package:anselm/core/ui/an_shell.dart';
import 'package:flutter/material.dart';
import 'package:flutter/rendering.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';

Future<void> _load(String family, String path) async {
  final f = File(path);
  if (!f.existsSync()) return;
  final b = f.readAsBytesSync();
  final loader = FontLoader(family)
    ..addFont(Future.value(ByteData.view(b.buffer, b.offsetInBytes, b.length)));
  await loader.load();
}

void main() {
  setUpAll(() async {
    await _load('MiSans', 'assets/fonts/MiSansVF.ttf');
    await _load('SF Mono', '/System/Library/Fonts/SFNSMono.ttf');
  });

  testWidgets('shell', (tester) async {
    const key = ValueKey('cap');
    tester.view.devicePixelRatio = 1.0;
    tester.view.physicalSize = const Size(AnSize.windowInitialWidth, AnSize.windowInitialHeight);
    addTearDown(tester.view.reset);

    await tester.pumpWidget(MaterialApp(
      debugShowCheckedModeBanner: false,
      theme: AnTheme.light(),
      home: const RepaintBoundary(key: key, child: AnShell()),
    ));
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 50));

    // toImage()/toByteData() are REAL engine-thread async — their futures never resolve inside the
    // test's fake-async zone (flutter/flutter#49317, #50783), so the capture must run in runAsync().
    // 引擎线程真异步,fake-async zone 不解析其 Future,故截图须在 runAsync 内跑。
    late final Uint8List bytes;
    await tester.runAsync(() async {
      final boundary = tester.renderObject<RenderRepaintBoundary>(find.byKey(key));
      final image = await boundary.toImage(pixelRatio: 1.0);
      final png = await image.toByteData(format: ui.ImageByteFormat.png);
      bytes = png!.buffer.asUint8List();
      image.dispose();
    });
    final dir = Directory('test/dev/out')..createSync(recursive: true);
    File('${dir.path}/shell.png').writeAsBytesSync(bytes);
  });
}
