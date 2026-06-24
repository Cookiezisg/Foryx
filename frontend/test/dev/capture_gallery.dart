// Dev screenshot harness — NOT part of the gate. Run explicitly:
//   flutter test test/dev/capture_gallery.dart
// Renders the component gallery headlessly via Skia (no Xcode) → test/dev/out/gallery.png so the
// kit's look (spacing, type, monochrome, states) can be reviewed against the demo without launching.
// Loads the bundled UI/mono fonts + the Lucide icon font so glyphs render (brand SVG may be blank —
// flutter_svg decodes async). 开发截图夹具:无头渲染画廊成 PNG 供对照 demo 审阅(非门禁)。
import 'dart:io';
import 'dart:ui' as ui;

import 'package:anselm/dev/gallery/catalog.dart';
import 'package:anselm/dev/gallery/gallery_app.dart';
import 'package:anselm/i18n/strings.g.dart';
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
    await _load('MiSans', 'assets/fonts/MiSansVF.ttf'); // bundled UI face (VF; capture may show default weight)
    await _load('JetBrains Mono', 'assets/fonts/JetBrainsMono.ttf');
    // Thin Lucide weight (matches AnIcons._family). 细笔画 Lucide,与 AnIcons._family 对齐。
    final cache = '${Platform.environment['HOME']}/.pub-cache/hosted/pub.dev';
    await _load('packages/lucide_icons_flutter/Lucide300',
        '$cache/lucide_icons_flutter-3.1.14+2/assets/build_font/LucideVariable-w300.ttf');
  });

  // ONE category per run (`--dart-define=CAT=<i>`, default 0) → gallery_<i>.png (+ gallery.png for 0).
  // A single test per run: multiple toImage tests in one isolate hang teardown, so we parameterize
  // instead of looping. 一次截一类(--dart-define=CAT=i);单测/次(同隔离多 toImage 会卡 teardown)。
  final cat = int.tryParse(const String.fromEnvironment('CAT', defaultValue: '0')) ?? 0;
  // Surface height (`--dart-define=H=<px>`, default 4200) — bump for a tall category so nothing clips.
  // 画布高(--dart-define=H=px,默认 4200)——类目高时调大,避免底部裁切。
  final h = double.tryParse(const String.fromEnvironment('H', defaultValue: '4200')) ?? 4200;
  testWidgets('gallery cat $cat — ${galleryCatalog[cat].label}', (tester) async {
    const key = ValueKey('cap');
    tester.view.devicePixelRatio = 1.0;
    tester.view.physicalSize = Size(1280, h);
    addTearDown(tester.view.reset);
    // Reduced-motion → deterministic still + no leftover ticker. 降级:确定性静帧、无残留 ticker。
    tester.platformDispatcher.accessibilityFeaturesTestValue =
        const FakeAccessibilityFeatures(disableAnimations: true);
    addTearDown(tester.platformDispatcher.clearAccessibilityFeaturesTestValue);

    await tester.pumpWidget(RepaintBoundary(
      key: key,
      child: TranslationProvider(child: GalleryApp(initialCategory: cat)),
    ));
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 60));

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
    File('${dir.path}/gallery_$cat.png').writeAsBytesSync(bytes);
    if (cat == 0) File('${dir.path}/gallery.png').writeAsBytesSync(bytes);
  });
}
