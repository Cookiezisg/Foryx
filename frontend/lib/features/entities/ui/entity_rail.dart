import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../core/design/tokens.dart';
import '../../../core/ui/an_button.dart';
import '../../../core/ui/an_menu.dart';
import '../../../core/ui/an_sidebar_list.dart';
import '../../../core/ui/an_skeleton.dart';
import '../../../core/ui/an_state.dart';
import '../../../i18n/strings.g.dart';
import '../data/entity_kind.dart';
import '../state/entity_list_provider.dart';
import '../state/rail_model.dart';
import '../state/rail_sort.dart';
import '../state/selected_entity.dart';
import 'entity_rail_model.dart';

/// The left-island entity navigator (Phase 4.1 STEP 3). Watches [railModelProvider] (the 4 kinds' live
/// list states) + [selectedEntityProvider], resolves ONE of four screens — loading skeleton / error /
/// empty / the [AnSidebarList] of kind sections — and wires selection back to [selectedEntityProvider].
/// All data flows through the repository seam, so the gallery/tests drive every state with a fixture.
///
/// 左岛实体导航(4.1 STEP 3)。watch railModel + selected,解出四态之一(骨架/错/空/列表),并把选择写回
/// selectedEntityProvider。全数据过 repository 缝,故 gallery/测试用 fixture 驱动每态。
class EntityRail extends ConsumerWidget {
  const EntityRail({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final groups = ref.watch(railModelProvider);
    final selected = ref.watch(selectedEntityProvider);
    final sort = ref.watch(railSortProvider);
    final t = context.t;

    final anyData = groups.any((g) => g.state.hasValue);
    final anyLoading = groups.any((g) => g.state.isLoading);
    final allError = groups.every((g) => g.state.hasError);

    // Loading: nothing resolved yet. A shaped skeleton reads faster than a spinner for content. 首载骨架。
    if (!anyData && anyLoading) return const _RailSkeleton();

    // Error: every kind failed and there is nothing to show — offer a retry that refetches all. 全错可重试。
    if (!anyData && allError) {
      return AnState(
        kind: AnStateKind.error,
        title: t.entities.errorTitle,
        hint: t.entities.errorHint,
        action: AnButton(
          label: t.entities.retry,
          onPressed: () => _retryAll(ref),
        ),
      );
    }

    // Empty: loaded, but zero entities across all kinds. 加载完但空。
    final total = groups.fold<int>(0, (sum, g) => sum + g.count);
    if (total == 0) {
      return AnState(
        kind: AnStateKind.empty,
        title: t.entities.emptyTitle,
        hint: t.entities.emptyHint,
      );
    }

    final model = buildRailModel(
      groups,
      RailLabels(
        kindLabel: (k) => _kindLabel(t, k),
        newLabel: t.entities.kNew,
        filter: t.entities.filter,
      ),
      sort,
    );

    return AnSidebarList(
      model: model,
      selectedId: selected?.id,
      showNew: false, // entity creation is a later phase; the rail is read+select only in 4.1
      menuEntries: _sortMenu(ref, t, sort),
      onSelect: (id) {
        final kind = kindForId(groups, id);
        if (kind != null) {
          ref.read(selectedEntityProvider.notifier).select(EntityRef(kind, id));
        }
      },
    );
  }

  /// The filter-row sliders menu (Sort) — checkable radio over [RailSort]. 排序 sliders 菜单(单选)。
  List<AnMenuEntry> _sortMenu(WidgetRef ref, Translations t, RailSort current) {
    void pick(RailSort s) => ref.read(railSortProvider.notifier).set(s);
    return [
      AnMenuSection(t.entities.sortLabel),
      AnMenuItem(
        label: t.entities.sortRecent,
        checked: current == RailSort.recent,
        onTap: () => pick(RailSort.recent),
      ),
      AnMenuItem(
        label: t.entities.sortName,
        checked: current == RailSort.name,
        onTap: () => pick(RailSort.name),
      ),
    ];
  }

  void _retryAll(WidgetRef ref) {
    for (final kind in EntityKind.values) {
      ref.invalidate(entityListProvider(kind));
    }
  }

  String _kindLabel(Translations t, EntityKind k) => switch (k) {
        EntityKind.function => t.ref.function,
        EntityKind.handler => t.ref.handler,
        EntityKind.agent => t.ref.agent,
        EntityKind.workflow => t.ref.workflow,
      };
}

/// The first-load placeholder — a few bone rows under the chrome zone. 首载占位:数行骨架。
class _RailSkeleton extends StatelessWidget {
  const _RailSkeleton();

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.symmetric(vertical: AnSpace.s8),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: const [
          AnSkeleton.row(),
          SizedBox(height: AnSpace.s8),
          AnSkeleton.row(),
          SizedBox(height: AnSpace.s8),
          AnSkeleton.row(),
          SizedBox(height: AnSpace.s8),
          AnSkeleton.row(),
          SizedBox(height: AnSpace.s8),
          AnSkeleton.row(),
        ],
      ),
    );
  }
}
