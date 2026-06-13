import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../core/design/theme.dart';
import '../core/design/tokens.dart';
import '../i18n/strings.g.dart';
import 'backend_controller.dart';
import 'providers.dart';
import 'router.dart';

/// Root widget. Gates the whole app on the sidecar's lifecycle (ADR 0004 §1): a single
/// splash / crash / ready switch, NOT per-feature error handling. Once the backend is
/// healthy, a nested ProviderScope injects the resolved base URL as the one
/// runtime-determined override; everything below it (Dio, SSE gateway, repos) is built
/// from it.
///
/// 根 widget。整 app 门控于 sidecar 生命周期(ADR 0004 §1):单一 splash/crash/ready 切换,
/// 非逐 feature 处理。后端健康后,嵌套 ProviderScope 注入解析后的 base URL 作唯一运行期 override;
/// 其下一切(Dio、SSE gateway、repo)据之构建。
class ForgifyApp extends ConsumerWidget {
  const ForgifyApp({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final controller = ref.watch(backendControllerProvider);
    return ValueListenableBuilder<BackendState>(
      valueListenable: controller.state,
      builder: (context, st, _) => switch (st.phase) {
        BackendPhase.starting => _StatusApp(message: context.t.backend.starting),
        BackendPhase.crashed => _StatusApp(
            message: context.t.backend.crashedTitle,
            detail: st.error,
            onRetry: controller.start,
          ),
        BackendPhase.ready => ProviderScope(
            overrides: [baseUrlProvider.overrideWithValue(st.baseUrl!)],
            child: const _ReadyApp(),
          ),
      },
    );
  }
}

/// The live app once the backend is healthy: MaterialApp.router over the desktop shell.
///
/// 后端健康后的 live app:MaterialApp.router 套桌面 shell。
class _ReadyApp extends StatefulWidget {
  const _ReadyApp();

  @override
  State<_ReadyApp> createState() => _ReadyAppState();
}

class _ReadyAppState extends State<_ReadyApp> {
  late final GoRouter _router = buildRouter();

  @override
  Widget build(BuildContext context) {
    return MaterialApp.router(
      debugShowCheckedModeBanner: false,
      title: 'Forgify',
      theme: ForgifyTheme.light(),
      locale: TranslationProvider.of(context).flutterLocale,
      supportedLocales: AppLocaleUtils.supportedLocales,
      routerConfig: _router,
    );
  }
}

/// The splash / crash screen — a standalone MaterialApp so it themes + localizes before
/// the backend is up. Shows a retry on crash.
///
/// splash / crash 屏——独立 MaterialApp,后端起来前即可主题化+本地化。crash 时给重试。
class _StatusApp extends StatelessWidget {
  const _StatusApp({required this.message, this.detail, this.onRetry});

  final String message;
  final String? detail;
  final VoidCallback? onRetry;

  @override
  Widget build(BuildContext context) {
    final crashed = onRetry != null;
    return MaterialApp(
      debugShowCheckedModeBanner: false,
      theme: ForgifyTheme.light(),
      home: Scaffold(
        body: Center(
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              if (!crashed)
                const SizedBox(
                  width: 22,
                  height: 22,
                  child: CircularProgressIndicator(strokeWidth: 2),
                )
              else
                const Icon(Icons.error_outline,
                    color: Tokens.danger, size: 28),
              const SizedBox(height: Tokens.gapLg),
              Text(message,
                  style: Theme.of(context).textTheme.titleMedium),
              if (detail != null) ...[
                const SizedBox(height: Tokens.gap),
                ConstrainedBox(
                  constraints: const BoxConstraints(maxWidth: 480),
                  child: Text(
                    detail!,
                    textAlign: TextAlign.center,
                    style: const TextStyle(
                        color: Tokens.textMuted, fontSize: 12),
                  ),
                ),
              ],
              if (onRetry != null) ...[
                const SizedBox(height: Tokens.gapLg),
                FilledButton(onPressed: onRetry, child: Text(context.t.backend.retry)),
              ],
            ],
          ),
        ),
      ),
    );
  }
}
