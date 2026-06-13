import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:window_manager/window_manager.dart';

import 'app/app.dart';
import 'app/backend_controller.dart';
import 'app/providers.dart';
import 'i18n/strings.g.dart';

/// Entry point. Initializes the desktop window, picks the UI locale, starts the Go
/// backend sidecar (non-blocking — the app shows a splash until it is healthy), and
/// mounts the composition root (ProviderScope) with the controller injected.
///
/// 入口。初始化桌面窗口、选 UI locale、启动 Go 后端 sidecar(非阻塞——健康前 app 显启动屏),
/// 挂载装配根(ProviderScope)并注入 controller。
Future<void> main() async {
  WidgetsFlutterBinding.ensureInitialized();
  await windowManager.ensureInitialized();

  const windowOptions = WindowOptions(
    size: Size(1280, 820),
    minimumSize: Size(960, 600),
    center: true,
    title: 'Forgify',
    titleBarStyle: TitleBarStyle.normal,
  );
  unawaited(windowManager.waitUntilReadyToShow(windowOptions, () async {
    await windowManager.show();
    await windowManager.focus();
  }));

  LocaleSettings.useDeviceLocale();

  final backend = BackendController();
  unawaited(backend.start());

  runApp(
    ProviderScope(
      overrides: [backendControllerProvider.overrideWithValue(backend)],
      child: TranslationProvider(child: const ForgifyApp()),
    ),
  );
}
