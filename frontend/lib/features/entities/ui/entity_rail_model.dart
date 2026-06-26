import '../../../core/model/sidebar_model.dart';
import '../../../core/model/status_state.dart';
import '../../../core/ui/icons.dart';
import '../data/entity_kind.dart';
import '../data/entity_row.dart';
import '../state/rail_model.dart';
import '../state/rail_sort.dart';

/// Pure projection: the rail's [RailGroup]s → an [AnSidebarList] [SidebarModel]. Kept widget/context-free
/// so the mapping (4 kind sections, per-kind status dot, id→kind lookup) is unit-tested without pumping
/// UI. The i18n strings are injected ([RailLabels]) rather than read from context here.
///
/// 纯投影:rail 的 RailGroup → AnSidebarList 的 SidebarModel。无 widget/context,使映射可脱 UI 单测;
/// i18n 文案注入而非此处读 context。
class RailLabels {
  const RailLabels({required this.kindLabel, required this.newLabel, required this.filter});

  /// Display name for a kind section header (injected from i18n `ref.<kind>`). kind 段头名(i18n 注入)。
  final String Function(EntityKind) kindLabel;
  final String newLabel;
  final String filter;
}

/// The at-a-glance status dot for a row — only kinds that carry runtime state get one (handler runtime,
/// workflow lifecycle/attention); function/agent rows have no inherent live state, so no dot. Folds raw
/// status strings through the shared [AnStatus.fromRaw]. 行状态点:仅有运行态的 kind 显(handler/workflow)。
AnStatus? railDot(EntityRow r) => switch (r.kind) {
      EntityKind.handler => AnStatus.fromRaw(r.runtimeState),
      EntityKind.workflow =>
        r.needsAttention ? AnStatus.wait : AnStatus.fromRaw(r.lifecycleState),
      EntityKind.function || EntityKind.agent => null,
    };

/// Build the rail model: one flat group with four collapsible kind sections (icon + label + count),
/// entities as depth-1 rows ordered by [sort]. 构建 rail 模型:单平铺组 + 四 kind 折叠段(按 sort 排序)。
SidebarModel buildRailModel(List<RailGroup> groups, RailLabels labels, RailSort sort) => SidebarModel(
      newLabel: labels.newLabel,
      filterPlaceholder: labels.filter,
      groups: [
        SidebarGroup(
          types: [
            for (final g in groups)
              SidebarType(
                label: labels.kindLabel(g.kind),
                icon: AnIcons.byKey(g.kind.scopeKind),
                count: g.count,
                rows: [
                  for (final row in sortRows(g.state.value?.rows ?? const <EntityRow>[], sort))
                    SidebarRow(id: row.id, label: row.name, dot: railDot(row)),
                ],
              ),
          ],
        ),
      ],
    );

/// Which kind owns [id] among the loaded rows (AnSidebarList's onSelect gives only the row id, so the
/// rail resolves the kind to build an [EntityRef]). 据已载行解出 id 所属 kind。
EntityKind? kindForId(List<RailGroup> groups, String id) {
  for (final g in groups) {
    if ((g.state.value?.rows ?? const <EntityRow>[]).any((r) => r.id == id)) {
      return g.kind;
    }
  }
  return null;
}
