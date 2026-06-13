import 'package:flutter/material.dart';

import '../../core/design/tokens.dart';
import '../../i18n/strings.g.dart';

/// The persistent desktop shell: a compact nav rail + content pane (a right-side
/// entity inspector is added with the app-shape work). It stays mounted across
/// navigation so the three session-long SSE streams and shell chrome never tear down.
/// The destinations below are placeholders — the real screens land with the app-shape
/// design; the foundation only proves the shell + theme + i18n compile and render.
///
/// 常驻桌面 shell:紧凑 nav rail + 内容区(右侧实体 inspector 随 app-shape 加)。导航期间常驻,
/// 使三条会话级 SSE 流与 shell chrome 不卸载。下列目的地是占位——真屏随 app-shape 设计落地;
/// 地基仅证明 shell + theme + i18n 可编译可渲染。
class AppShell extends StatefulWidget {
  const AppShell({super.key});

  @override
  State<AppShell> createState() => _AppShellState();
}

class _AppShellState extends State<AppShell> {
  int _index = 0;

  @override
  Widget build(BuildContext context) {
    final t = Translations.of(context);
    final destinations = <(IconData, String)>[
      (Icons.chat_bubble_outline, t.nav.chat),
      (Icons.functions, t.nav.functions),
      (Icons.dns_outlined, t.nav.handlers),
      (Icons.smart_toy_outlined, t.nav.agents),
      (Icons.account_tree_outlined, t.nav.workflows),
      (Icons.search, t.nav.search),
      (Icons.settings_outlined, t.nav.settings),
    ];

    return Scaffold(
      body: Row(
        children: [
          NavigationRail(
            selectedIndex: _index,
            onDestinationSelected: (i) => setState(() => _index = i),
            labelType: NavigationRailLabelType.all,
            minWidth: Tokens.navRailWidth,
            backgroundColor: Tokens.surfaceMuted,
            destinations: [
              for (final (icon, label) in destinations)
                NavigationRailDestination(
                  icon: Icon(icon, size: 20),
                  label: Text(label, style: const TextStyle(fontSize: 11)),
                ),
            ],
          ),
          const VerticalDivider(width: 1),
          Expanded(
            child: Center(
              child: Column(
                mainAxisSize: MainAxisSize.min,
                children: [
                  Text(t.app.name,
                      style: Theme.of(context).textTheme.headlineSmall),
                  const SizedBox(height: Tokens.gap),
                  Text(
                    '${destinations[_index].$2} — app shape TBD',
                    style: const TextStyle(color: Tokens.textMuted),
                  ),
                ],
              ),
            ),
          ),
        ],
      ),
    );
  }
}
