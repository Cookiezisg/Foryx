import 'package:flutter/widgets.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../../core/design/tokens.dart';
import '../../../../core/ui/an_button.dart';
import '../../../../core/ui/an_row.dart';
import '../../../../core/ui/an_skeleton.dart';
import '../../../../core/ui/an_state.dart';
import '../../../../core/ui/an_version_diff.dart';
import '../../../../core/model/status_state.dart';
import '../../../../i18n/strings.g.dart';
import '../../data/entity_format.dart';
import '../../state/detail/version_list_provider.dart';
import '../../state/detail/version_list_state.dart';
import '../../state/selected_entity.dart';

/// The 版本 tab (kind-agnostic): a selectable version list (left) + the adjacent-version [AnVersionDiff]
/// (right). Selecting a version diffs it against the next-older loaded version (the earliest shows full
/// context). 版本 tab:左侧版本列表 + 右侧相邻版本 diff。
class VersionTab extends ConsumerWidget {
  const VersionTab(this.entityRef, {super.key});

  final EntityRef entityRef;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final d = context.t.entities.detail;
    final async = ref.watch(versionListProvider(entityRef));
    final notifier = ref.read(versionListProvider(entityRef).notifier);

    return async.when(
      loading: () => const AnSkeleton.lines(6),
      error: (_, _) => AnState(
        kind: AnStateKind.error,
        size: AnStateSize.inset,
        title: d.state.errorTitle,
        action: AnButton(label: d.state.loadMore, onPressed: () => ref.invalidate(versionListProvider(entityRef))),
      ),
      data: (st) {
        if (st.versions.isEmpty) {
          return AnState(kind: AnStateKind.empty, size: AnStateSize.inset, title: d.state.noVersions);
        }
        final sel = st.versions[st.selectedIndex];
        final older = st.selectedIndex + 1 < st.versions.length ? st.versions[st.selectedIndex + 1] : null;
        return Row(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Expanded(flex: 2, child: _list(context, st, notifier)),
            const SizedBox(width: AnSpace.s16),
            Expanded(
              flex: 3,
              child: AnVersionDiff(
                after: sel.src,
                before: older?.src,
                lang: sel.lang,
                range: older != null ? 'v${older.version} → v${sel.version}' : 'v${sel.version} · ${d.state.earliest}',
                note: sel.changeReason,
              ),
            ),
          ],
        );
      },
    );
  }

  // Column (not ListView): the surrounding AnPage owns the single document scroll (flow tabs). 文档单滚,用 Column。
  Widget _list(BuildContext context, VersionListState st, VersionListNotifier notifier) {
    final d = context.t.entities.detail;
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        for (var i = 0; i < st.versions.length; i++)
          AnRow(
            label: 'v${st.versions[i].version}',
            dot: st.versions[i].active ? AnStatus.done : null,
            hint: _hint(st.versions[i]),
            selected: i == st.selectedIndex,
            onSelect: () => notifier.select(i),
          ),
        if (st.loadingMore)
          const AnSkeleton.row()
        else if (st.hasMore)
          AnButton(label: d.state.loadMore, onPressed: notifier.loadMore),
      ],
    );
  }

  String _hint(VersionRow row) {
    final time = fmtTime(row.createdAt);
    final reason = row.changeReason;
    return reason != null && reason.isNotEmpty ? '$time · $reason' : time;
  }
}
