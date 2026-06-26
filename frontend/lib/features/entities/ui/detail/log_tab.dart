import 'package:flutter/widgets.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../../core/design/tokens.dart';
import '../../../../core/model/status_state.dart';
import '../../../../core/ui/an_badge.dart';
import '../../../../core/ui/an_button.dart';
import '../../../../core/ui/an_row.dart';
import '../../../../core/ui/an_row_detail.dart';
import '../../../../core/ui/an_skeleton.dart';
import '../../../../core/ui/an_state.dart';
import '../../../../core/ui/icons.dart';
import '../../../../i18n/strings.g.dart';
import '../../state/detail/log_list_provider.dart';
import '../../state/detail/log_list_state.dart';
import '../../state/selected_entity.dart';
import 'detail_sections.dart';

/// The 日志 tab (kind-dispatched): the ok/failed aggregate header (function/handler/agent only) + the
/// run history as expandable rows; a workflow flowrun expands to its node list (lazily fetched). Hold
/// the list on load-more error; never flip the whole list to a spinner. 日志 tab。
class LogTab extends ConsumerWidget {
  const LogTab(this.entityRef, {super.key});

  final EntityRef entityRef;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final d = context.t.entities.detail;
    final async = ref.watch(logListProvider(entityRef));
    final notifier = ref.read(logListProvider(entityRef).notifier);
    final kindIcon = AnIcons.byKey(entityRef.kind.scopeKind);

    return async.when(
      loading: () => const AnSkeleton.lines(8),
      error: (_, _) => AnState(
        kind: AnStateKind.error,
        size: AnStateSize.inset,
        title: d.state.errorTitle,
        action: AnButton(label: d.state.loadMore, onPressed: () => ref.invalidate(logListProvider(entityRef))),
      ),
      data: (st) {
        if (st.rows.isEmpty) {
          return AnState(
              kind: AnStateKind.empty, size: AnStateSize.inset, title: d.state.noLogs, hint: d.state.noLogsHint);
        }
        // Column (not ListView): the surrounding AnPage owns the single document scroll (flow tabs). 文档单滚,用 Column。
        return Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            if (st.hasAggregate) _aggHeader(context, st),
            for (final row in st.rows)
              AnRowDetail(
                open: st.openIds.contains(row.id),
                row: AnRow(
                  icon: kindIcon,
                  dot: row.dot,
                  label: row.label,
                  meta: row.meta,
                  hint: row.hint,
                  collapsible: true,
                  open: st.openIds.contains(row.id),
                  onToggle: () => notifier.toggle(row.id),
                  onSelect: () => notifier.toggle(row.id),
                ),
                detail: _detail(st, row),
              ),
            if (st.loadingMore)
              const AnSkeleton.row()
            else if (st.hasMore)
              AnButton(label: d.state.loadMore, onPressed: notifier.loadMore),
          ],
        );
      },
    );
  }

  Widget _aggHeader(BuildContext context, LogListState st) {
    final t = context.t;
    return Padding(
      padding: const EdgeInsets.only(bottom: AnSpace.s8),
      child: Row(children: [
        AnBadge('${st.aggregates.okCount} ${t.status.done}', tone: AnTone.ok),
        const SizedBox(width: AnSpace.s8),
        AnBadge('${st.aggregates.failedCount} ${t.status.err}',
            tone: st.aggregates.failedCount > 0 ? AnTone.danger : AnTone.none),
      ]),
    );
  }

  Widget _detail(LogListState st, LogRow row) {
    final comp = st.flowruns[row.id];
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        kvList([for (final r in row.detailRows) (r.$1, r.$2)], wrap: true),
        if (comp != null)
          for (final n in comp.nodes)
            AnRow(
              dot: AnStatus.fromRaw(n.status),
              label: '${n.nodeId} · ${n.kind}',
              meta: n.ref,
              passive: true,
            ),
      ],
    );
  }
}
