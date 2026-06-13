import 'package:go_router/go_router.dart';

import 'shell/app_shell.dart';

/// The app router. A single shell route for now; the per-feature branches
/// (StatefulShellRoute) + workspace-selection guard land with the app-shape design. Kept
/// as a provider-free factory so the composition root owns its lifetime.
///
/// app 路由。当前单一 shell 路由;per-feature 分支(StatefulShellRoute)+ 选区门控随 app-shape
/// 落地。作无 provider 工厂,使装配根掌其生命周期。
GoRouter buildRouter() => GoRouter(
      initialLocation: '/',
      routes: [
        GoRoute(
          path: '/',
          builder: (context, state) => const AppShell(),
        ),
      ],
    );
