import 'package:flutter/widgets.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../core/ui/an_inspector.dart';
import '../core/ui/an_shell.dart';
import '../features/entities/state/run/right_panel.dart';
import '../features/entities/state/selected_entity.dart';
import '../features/entities/ui/entity_ocean.dart';
import '../features/entities/ui/entity_rail.dart';
import '../features/entities/ui/run/run_terminal.dart';

/// THE single shell composition — which feature sits in which island. Mounted by BOTH entries so the
/// real app and the demo never diverge: `lib/main.dart` (→ `make app`) wraps it in the startup gate and
/// feeds it the LIVE repositories; `lib/dev/demo_main.dart` (→ `make demo`) skips the gate and overrides
/// the repository seam with fixtures. App vs demo differ ONLY in data source + startup.
///
/// The right island is the run terminal, STRONG-LINKED to the selection: it reveals whenever an entity is
/// selected (all four kinds are executable) and isn't manually collapsed, and re-binds to whichever entity
/// is selected (the terminal itself reads `selectedEntityProvider`). A run keeps streaming in the
/// background when you switch entities (the controller family + keepAlive).
///
/// 唯一的壳组合。右岛=run 终端,强链选区:有选中且未手动收起即揭示,并随选区重绑(终端自读 selectedEntityProvider);
/// 切换实体时运行在后台续流(controller family + keepAlive)。
class AppShell extends ConsumerWidget {
  const AppShell({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final hasSelection = ref.watch(selectedEntityProvider) != null;
    final collapsed = ref.watch(rightPanelCollapsedProvider);
    return AnShell(
      sidebar: const EntityRail(),
      ocean: const EntityOcean(),
      inspector: const AnInspector(headless: true, child: RunTerminal()),
      inspectorOpen: hasSelection && !collapsed,
    );
  }
}
