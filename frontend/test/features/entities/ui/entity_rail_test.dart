import 'dart:async';

import 'package:anselm/core/contract/entities/function.dart';
import 'package:anselm/core/contract/entities/handler.dart';
import 'package:anselm/core/contract/page.dart';
import 'package:anselm/core/design/theme.dart';
import 'package:anselm/core/ui/an_sidebar_list.dart';
import 'package:anselm/core/ui/an_skeleton.dart';
import 'package:anselm/core/ui/icons.dart';
import 'package:anselm/features/entities/data/entity_fixtures.dart';
import 'package:anselm/features/entities/data/entity_kind.dart';
import 'package:anselm/features/entities/data/entity_providers.dart';
import 'package:anselm/features/entities/data/entity_repository.dart';
import 'package:anselm/features/entities/data/entity_row.dart';
import 'package:anselm/features/entities/state/selected_entity.dart';
import 'package:anselm/features/entities/ui/entity_rail.dart';
import 'package:anselm/i18n/strings.g.dart';
import 'package:flutter/material.dart' hide Page;
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';

// STEP 3 gate (widget) — the rail resolves the four states off the repository seam: loading skeleton /
// error / empty / the AnSidebarList of kind sections, and selection writes back to selectedEntityProvider.
// (The real-machine look is verified separately by the PNG capture harness.)

final _t = DateTime.utc(2026, 6, 26);
FunctionEntity _fn(String id, String name) =>
    FunctionEntity(id: id, name: name, createdAt: _t, updatedAt: _t);
HandlerEntity _hd(String id, String name, String runtime) =>
    HandlerEntity(id: id, name: name, createdAt: _t, updatedAt: _t, runtimeState: runtime);

Widget _host(EntityRepository repo) => ProviderScope(
      overrides: [entityRepositoryProvider.overrideWithValue(repo)],
      child: TranslationProvider(
        child: MaterialApp(
          debugShowCheckedModeBanner: false,
          theme: AnTheme.light(),
          home: const Scaffold(body: SizedBox(width: 300, height: 600, child: EntityRail())),
        ),
      ),
    );

/// Repo whose list never resolves — pins the loading state (a Future, not a Timer, so no pending-timer
/// failure and no need to settle the shimmer ticker).
class _PendingRepo extends FixtureEntityRepository {
  @override
  Future<Page<EntityRow>> listEntities(EntityKind kind, {String? cursor, int? limit}) =>
      Completer<Page<EntityRow>>().future;
}

/// Repo whose list always throws — pins the error state.
class _ErrRepo extends FixtureEntityRepository {
  @override
  Future<Page<EntityRow>> listEntities(EntityKind kind, {String? cursor, int? limit}) async =>
      throw Exception('boom');
}

void main() {
  testWidgets('loaded → AnSidebarList with kind sections + rows', (tester) async {
    await tester.pumpWidget(_host(FixtureEntityRepository(
      functions: [_fn('fn_1', 'normalize-input'), _fn('fn_2', 'validate-schema')],
      handlers: [_hd('hd_1', 'slack', 'running')],
    )));
    await tester.pump(const Duration(milliseconds: 50)); // let the 4 list futures resolve

    expect(find.byType(AnSidebarList), findsOneWidget);
    expect(find.text(t.ref.function), findsOneWidget); // kind section head
    expect(find.text(t.ref.handler), findsOneWidget);
    expect(find.text('normalize-input'), findsOneWidget);
    expect(find.text('slack'), findsOneWidget);
    expect(tester.takeException(), isNull);
  });

  testWidgets('tapping a row writes selection to selectedEntityProvider', (tester) async {
    await tester.pumpWidget(_host(FixtureEntityRepository(functions: [_fn('fn_1', 'normalize-input')])));
    await tester.pump(const Duration(milliseconds: 50));

    final container = ProviderScope.containerOf(tester.element(find.byType(EntityRail)));
    expect(container.read(selectedEntityProvider), isNull);

    await tester.tap(find.text('normalize-input'));
    await tester.pump();

    expect(container.read(selectedEntityProvider), const EntityRef(EntityKind.function, 'fn_1'));
  });

  testWidgets('empty → AnState empty screen', (tester) async {
    await tester.pumpWidget(_host(FixtureEntityRepository()));
    await tester.pump(const Duration(milliseconds: 50));

    expect(find.byType(AnSidebarList), findsNothing);
    expect(find.text(t.entities.emptyTitle), findsOneWidget);
  });

  testWidgets('error → AnState error with retry', (tester) async {
    await tester.pumpWidget(_host(_ErrRepo()));
    await tester.pump(const Duration(milliseconds: 50));

    expect(find.text(t.entities.errorTitle), findsOneWidget);
    expect(find.text(t.entities.retry), findsOneWidget);
  });

  testWidgets('loading → skeleton (no spinner settle)', (tester) async {
    await tester.pumpWidget(_host(_PendingRepo()));
    await tester.pump(); // one frame — futures still pending

    expect(find.byType(AnSkeleton), findsWidgets);
    expect(find.byType(AnSidebarList), findsNothing);
  });

  testWidgets('sort sliders menu opens with the sort options', (tester) async {
    await tester.pumpWidget(_host(FixtureEntityRepository(functions: [_fn('fn_1', 'a')])));
    await tester.pump(const Duration(milliseconds: 50));

    // The sliders anchor renders (menuEntries wired); opening it reveals the Sort options.
    await tester.tap(find.byIcon(AnIcons.sliders));
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 50));

    expect(find.text(t.entities.sortLabel), findsOneWidget);
    expect(find.text(t.entities.sortRecent), findsOneWidget);
    expect(find.text(t.entities.sortName), findsOneWidget);
  });
}
