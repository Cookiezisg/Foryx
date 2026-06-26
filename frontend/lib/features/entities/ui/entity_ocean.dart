import 'package:flutter/widgets.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../core/design/tokens.dart';
import '../../../core/ui/an_button.dart';
import '../../../core/ui/an_deferred_loading.dart';
import '../../../core/ui/an_page.dart';
import '../../../core/ui/an_skeleton.dart';
import '../../../core/ui/an_state.dart';
import '../../../core/ui/an_tabs.dart';
import '../../../i18n/strings.g.dart';
import '../data/entity_kind.dart';
import '../state/detail/entity_detail.dart';
import '../state/detail/entity_detail_provider.dart';
import '../state/run/right_panel.dart';
import '../state/run/run_terminal_controller.dart';
import '../state/selected_entity.dart';
import 'detail/log_tab.dart';
import 'detail/ocean_header.dart';
import 'detail/overview/agent_overview.dart';
import 'detail/overview/function_overview.dart';
import 'detail/overview/handler_overview.dart';
import 'detail/overview/workflow_overview.dart';
import 'detail/version_tab.dart';

/// The detail "ocean" (the open window surface). Reads [selectedEntityProvider]: null → empty state;
/// else watches [entityDetailProvider] → loading skeleton / error+retry / the header + 概览/版本/日志 tabs.
/// The selected tab is local widget state and resets to overview when the selection changes. STEP 4:
/// verb CTA + rename + build-mirror are disabled stubs (STEP 5). 详情海洋:选中→详情头 + 三 tab。
class EntityOcean extends ConsumerStatefulWidget {
  const EntityOcean({super.key});

  @override
  ConsumerState<EntityOcean> createState() => _EntityOceanState();
}

class _EntityOceanState extends ConsumerState<EntityOcean> {
  String _tab = 'overview';

  @override
  Widget build(BuildContext context) {
    final d = context.t.entities.detail;
    final selected = ref.watch(selectedEntityProvider);

    // Reset to the overview tab whenever the selected entity changes. 选区变化时回到概览 tab。
    ref.listen(selectedEntityProvider, (prev, next) {
      if (prev != next && _tab != 'overview') setState(() => _tab = 'overview');
    });

    if (selected == null) {
      return Center(
        child: AnState(
          kind: AnStateKind.empty,
          title: context.t.entities.selectTitle,
          hint: context.t.entities.selectHint,
        ),
      );
    }

    final async = ref.watch(entityDetailProvider(selected));
    return async.when(
      // Loading lives in the SAME AnPage (centered 720 column) as the loaded content, so there is no
      // width jump when data arrives; deferred so a fast load never flashes a skeleton. 同 720 列 + 延迟防闪。
      loading: () => const AnPage(
        child: AnDeferredLoading(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [AnSkeleton.card(), SizedBox(height: AnSpace.s16), AnSkeleton.lines(6)],
          ),
        ),
      ),
      error: (_, _) => Center(
        child: AnState(
          kind: AnStateKind.error,
          title: d.state.errorTitle,
          hint: d.state.errorHint,
          action: AnButton(
            label: d.state.loadMore,
            onPressed: () => ref.invalidate(entityDetailProvider(selected)),
          ),
        ),
      ),
      // ONE document: header + tabs + content all live in a single AnPage (centered 720 reading column,
      // one scroll) and scroll together — AnTabs in FLOW mode so the selected pane flows inline (the demo
      // an-page/an-tabs model). 整个海洋一份文档:头+tab+内容同在一个 AnPage(居中 720 单滚)一起滚。
      data: (detail) => AnPage(
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            EntityOceanHeader(
              detail: detail,
              // The right island is already bound to the selection; the verb CTA ensures it's revealed
              // then fires the run (header CTA = trigger run, demo-aligned). 头部动词钮:展开右岛 + 直接执行。
              onVerb: () {
                ref.read(rightPanelCollapsedProvider.notifier).set(false);
                ref.read(runTerminalProvider(detail.ref).notifier).run();
              },
            ),
            AnTabs(
              flow: true,
              value: _tab,
              onSelect: (k) => setState(() => _tab = k),
              items: [
                AnTabsItem(key: 'overview', label: d.tab.overview, pane: _overview(detail)),
                AnTabsItem(key: 'versions', label: d.tab.versions, pane: VersionTab(detail.ref)),
                AnTabsItem(key: 'logs', label: d.tab.logs, pane: LogTab(detail.ref)),
              ],
            ),
          ],
        ),
      ),
    );
  }

  Widget _overview(EntityDetail d) => switch (d.ref.kind) {
        EntityKind.function => FunctionOverview(fn: d.function!),
        EntityKind.handler => HandlerOverview(hd: d.handler!),
        EntityKind.agent => AgentOverview(agent: d.agent!, mountHealth: d.mountHealth),
        EntityKind.workflow => WorkflowOverview(wf: d.workflow!),
      };
}
