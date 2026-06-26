import 'package:flutter/widgets.dart';

import '../../../../../core/contract/entities/workflow.dart';
import '../../../../../core/model/status_state.dart';
import '../../../../../core/ui/an_field.dart';
import '../../../../../core/ui/an_info_card.dart';
import '../../../../../core/ui/an_row.dart';
import '../../../../../core/ui/an_section.dart';
import '../../../../../core/ui/icons.dart';
import '../../../../../i18n/strings.g.dart';
import '../../../data/entity_format.dart';
import '../detail_sections.dart';

/// Workflow 概览:说明 + KV(含节点/边计数)→ 运行治理(生命周期/并发)→ 告警。**编排图可视化 + 进入图编辑器
/// 推迟到图编辑器阶段**(本步不渲图,只在 KV 里给节点/边数量)。
class WorkflowOverview extends StatelessWidget {
  const WorkflowOverview({required this.wf, super.key});

  final WorkflowEntity wf;

  @override
  Widget build(BuildContext context) {
    final d = context.t.entities.detail;
    final v = wf.activeVersion;
    if (v == null) return insetEmpty(d.state.noActiveVersion);
    final g = graphOf(v);

    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        AnSection(variant: AnSectionVariant.plain, children: [
          if (wf.description.isNotEmpty) AnField(label: d.kv.desc, value: wf.description, wrap: true),
          kvList([
            (d.kv.id, wf.id),
            (d.kv.currentVersion, 'v${v.version}'),
            if (g != null) (d.kv.nodes, '${g.nodes.length} · ${d.graph.edges} ${g.edges.length}'),
            (d.kv.lifecycle, wf.lifecycleState),
          ]),
        ]),
        AnSection(label: d.sec.governance, variant: AnSectionVariant.plain, grid: true, children: [
          AnInfoCard(
            title: d.card.lifecycle,
            icon: AnIcons.byKey('scheduler'),
            child: kvList([
              (d.kv.status, wf.lifecycleState),
              (d.kv.active, wf.active ? d.val.listening : d.val.stopped),
              (d.kv.lastAction, wf.lastActionBy),
            ]),
          ),
          AnInfoCard(
            title: d.card.concurrency,
            icon: AnIcons.byKey('workflow'),
            child: kvList([(d.kv.concurrency, wf.concurrency)]),
          ),
        ]),
        AnSection(label: d.sec.alerts, variant: AnSectionVariant.plain, children: [
          AnRow(
            icon: AnIcons.byKey(wf.needsAttention ? 'error' : 'check'),
            dot: wf.needsAttention ? AnStatus.err : AnStatus.done,
            label: wf.needsAttention ? (wf.attentionReason ?? d.val.needsAttention) : d.val.noAlerts,
            passive: true,
          ),
        ]),
        // 编排图可视化 + 进入图编辑器 → 图编辑器阶段(本步不渲)。
      ],
    );
  }
}
