import 'package:anselm/core/design/theme.dart';
import 'package:anselm/core/ui/an_state.dart';
import 'package:anselm/features/entities/data/entity_demo_fixture.dart';
import 'package:anselm/features/entities/data/entity_kind.dart';
import 'package:anselm/features/entities/data/entity_providers.dart';
import 'package:anselm/features/entities/state/selected_entity.dart';
import 'package:anselm/features/entities/ui/entity_ocean.dart';
import 'package:anselm/i18n/strings.g.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';

// STEP 4 gate (widget) — the detail ocean over the rich demo fixture: empty when nothing is selected;
// per-kind overview + 概览/版本/日志 tabs when selected; switching tabs shows version/log content.

class _Pre extends SelectedEntity {
  _Pre(this._r);
  final EntityRef _r;
  @override
  EntityRef? build() => _r;
}

Widget _host({EntityRef? sel}) => ProviderScope(
      overrides: [
        entityRepositoryProvider.overrideWithValue(demoEntityRepository()),
        if (sel != null) selectedEntityProvider.overrideWith(() => _Pre(sel)),
      ],
      child: TranslationProvider(
        child: MaterialApp(
          debugShowCheckedModeBanner: false,
          theme: AnTheme.light(),
          home: const Scaffold(body: SizedBox(width: 900, height: 800, child: EntityOcean())),
        ),
      ),
    );

void main() {
  final d = t.entities.detail;

  testWidgets('no selection → empty state', (tester) async {
    await tester.pumpWidget(_host());
    await tester.pump(const Duration(milliseconds: 50));
    expect(find.byType(AnState), findsOneWidget);
    expect(find.text(t.entities.selectTitle), findsOneWidget);
  });

  testWidgets('function → header + tabs + overview content', (tester) async {
    await tester.pumpWidget(_host(sel: const EntityRef(EntityKind.function, 'fn_normalize')));
    await tester.pump(const Duration(milliseconds: 50));
    expect(find.text('normalize-input'), findsWidgets); // ocean header title (+ the name KV row)
    expect(find.text(d.tab.overview), findsOneWidget);
    expect(find.text(d.tab.versions), findsOneWidget);
    expect(find.text(d.tab.logs), findsOneWidget);
    expect(find.text('Coerce + trim raw fields'), findsOneWidget); // description (overview)
    expect(tester.takeException(), isNull);
  });

  testWidgets('agent → unhealthy mount badge in the header', (tester) async {
    await tester.pumpWidget(_host(sel: const EntityRef(EntityKind.agent, 'ag_researcher')));
    await tester.pump(const Duration(milliseconds: 50));
    expect(find.text('researcher'), findsOneWidget);
    expect(find.text(d.mounts.unhealthy(count: 1)), findsWidgets); // 1 项异常
  });

  testWidgets('workflow → governance + alerts (graph deferred to the editor phase)', (tester) async {
    await tester.pumpWidget(_host(sel: const EntityRef(EntityKind.workflow, 'wf_digest')));
    await tester.pump(const Duration(milliseconds: 50));
    expect(find.text('daily-digest'), findsWidgets);
    expect(find.text(d.sec.governance), findsOneWidget); // 运行治理
    expect(find.text(d.card.concurrency), findsWidgets); // 并发策略 (card title + kv label)
    expect(find.text(d.sec.graph), findsNothing); // graph viz deferred
  });

  testWidgets('handler → init args shown but sensitive default MASKED', (tester) async {
    await tester.pumpWidget(_host(sel: const EntityRef(EntityKind.handler, 'hd_slack')));
    await tester.pump(const Duration(milliseconds: 50));
    expect(find.text('token'), findsWidgets); // the sensitive arg name is shown
    expect(find.textContaining('xoxb'), findsNothing); // its secret default is NEVER rendered
  });

  testWidgets('switch to 日志 tab → execution rows appear', (tester) async {
    await tester.pumpWidget(_host(sel: const EntityRef(EntityKind.function, 'fn_normalize')));
    await tester.pump(const Duration(milliseconds: 50));
    await tester.tap(find.text(d.tab.logs));
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 50)); // log page loads
    expect(find.text('user · ok'), findsWidgets); // a function execution row label
  });
}
