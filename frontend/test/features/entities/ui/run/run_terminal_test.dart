import 'package:anselm/core/contract/entities/agent.dart';
import 'package:anselm/core/contract/entities/function.dart';
import 'package:anselm/core/contract/entities/values.dart';
import 'package:anselm/core/design/theme.dart';
import 'package:anselm/core/messages/block_tree_reducer.dart';
import 'package:anselm/core/sse/frame.dart';
import 'package:anselm/core/ui/an_button.dart';
import 'package:anselm/features/entities/data/entity_fixtures.dart';
import 'package:anselm/features/entities/data/entity_kind.dart';
import 'package:anselm/features/entities/data/entity_providers.dart';
import 'package:anselm/features/entities/state/selected_entity.dart';
import 'package:anselm/features/entities/ui/run/block_tree_view.dart';
import 'package:anselm/features/entities/ui/run/run_terminal.dart';
import 'package:anselm/i18n/strings.g.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';

// STEP 5.5 gate (widget) — the run terminal is bound to the SELECTED entity: idle shows the typed input
// form + idle state; pressing the verb runs and renders the streamed output + result; the agent trace
// renders reasoning collapsed-by-default + a danger badge.

final _t0 = DateTime.utc(2026, 6, 27);

class _Pre extends SelectedEntity {
  _Pre(this._r);
  final EntityRef _r;
  @override
  EntityRef? build() => _r;
}

FixtureEntityRepository _fix() => FixtureEntityRepository(
      runDelay: Duration.zero,
      functions: [
        FunctionEntity(
          id: 'fn_1',
          name: 'normalize',
          createdAt: _t0,
          updatedAt: _t0,
          activeVersionId: 'fn_1_v1',
          activeVersion: FunctionVersion(
            id: 'fn_1_v1',
            functionId: 'fn_1',
            version: 1,
            inputs: const [Field(name: 'text', type: 'string', description: 'raw input')],
            createdAt: _t0,
            updatedAt: _t0,
          ),
        ),
      ],
      agents: [
        AgentEntity(
          id: 'ag_1',
          name: 'researcher',
          createdAt: _t0,
          updatedAt: _t0,
          activeVersionId: 'ag_1_v1',
          activeVersion: AgentVersion(
            id: 'ag_1_v1',
            agentId: 'ag_1',
            version: 1,
            inputs: const [Field(name: 'topic', type: 'string')],
            createdAt: _t0,
            updatedAt: _t0,
          ),
        ),
      ],
    );

Widget _host(FixtureEntityRepository repo, {EntityRef? sel, Widget child = const RunTerminal()}) =>
    ProviderScope(
      overrides: [
        entityRepositoryProvider.overrideWithValue(repo),
        if (sel != null) selectedEntityProvider.overrideWith(() => _Pre(sel)),
      ],
      child: TranslationProvider(
        child: MaterialApp(
          debugShowCheckedModeBanner: false,
          theme: AnTheme.light(),
          home: Scaffold(body: SizedBox(width: 340, height: 800, child: child)),
        ),
      ),
    );

void main() {
  final r = t.entities.run;

  testWidgets('function idle → typed input field + idle state', (tester) async {
    await tester.pumpWidget(_host(_fix(), sel: const EntityRef(EntityKind.function, 'fn_1')));
    await tester.pump(const Duration(milliseconds: 50)); // detail load
    expect(find.text('text'), findsOneWidget); // the declared input's label
    expect(find.text(r.idleTitle), findsOneWidget);
    expect(tester.takeException(), isNull);
  });

  testWidgets('run function → ok, streamed output + result', (tester) async {
    await tester.pumpWidget(_host(_fix(), sel: const EntityRef(EntityKind.function, 'fn_1')));
    await tester.pump(const Duration(milliseconds: 50));
    await tester.tap(find.widgetWithText(AnButton, t.entities.detail.verb.run));
    await tester.pumpAndSettle();
    expect(find.text(r.resultHeading), findsOneWidget);
    expect(find.textContaining('done'), findsWidgets); // live stderr from the run node
    expect(find.text(t.status.done), findsOneWidget); // ok badge
  });

  testWidgets('agent invoke → ReAct trace with the tool name', (tester) async {
    await tester.pumpWidget(_host(_fix(), sel: const EntityRef(EntityKind.agent, 'ag_1')));
    await tester.pump(const Duration(milliseconds: 50));
    await tester.tap(find.widgetWithText(AnButton, t.entities.detail.verb.invoke));
    await tester.pumpAndSettle();
    expect(find.text(r.traceHeading), findsOneWidget);
    expect(find.text('web-search'), findsWidgets); // tool_call name
  });

  testWidgets('block tree: reasoning collapsed by default, danger badge on a dangerous tool_call', (tester) async {
    const scope = StreamScope(kind: 'agent', id: 'a');
    final reducer = BlockTreeReducer()
      ..apply(const StreamEnvelope(seq: 1, scope: scope, id: 'b1', frame: FrameOpen(node: StreamNode(type: 'reasoning'))))
      ..apply(StreamEnvelope(seq: 2, scope: scope, id: 'b1', frame: FrameClose(status: 'completed', result: const StreamNode(type: 'reasoning', content: {'content': 'secret thought'}))))
      ..apply(const StreamEnvelope(seq: 1, scope: scope, id: 'b2', frame: FrameOpen(node: StreamNode(type: 'tool_call', content: {'name': 'rm'}))))
      ..apply(StreamEnvelope(seq: 2, scope: scope, id: 'b2', frame: FrameClose(status: 'completed', result: const StreamNode(type: 'tool_call', content: {'name': 'rm', 'arguments': '{}', 'danger': 'dangerous'}))));

    await tester.pumpWidget(_host(_fix(), child: SingleChildScrollView(child: BlockTreeView(roots: reducer.roots))));
    await tester.pump();
    expect(find.text('rm'), findsOneWidget); // tool name
    expect(find.text(r.danger.dangerous), findsOneWidget); // danger badge (header, always visible)
    expect(find.text('secret thought'), findsNothing); // reasoning collapsed by default

    await tester.tap(find.text(r.reasoning)); // expand the reasoning disclosure
    await tester.pumpAndSettle();
    expect(find.text('secret thought'), findsOneWidget);
  });
}
