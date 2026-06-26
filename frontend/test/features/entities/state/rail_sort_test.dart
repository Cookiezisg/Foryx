import 'package:anselm/features/entities/data/entity_kind.dart';
import 'package:anselm/features/entities/data/entity_row.dart';
import 'package:anselm/features/entities/state/rail_sort.dart';
import 'package:flutter_test/flutter_test.dart';

// The rail's in-section ordering. Pins: recent = newest updatedAt first (name tiebreak), name = A→Z
// (id tiebreak), both stable + non-mutating.

EntityRow _row(String id, String name, DateTime updated) =>
    EntityRow(kind: EntityKind.function, id: id, name: name, updatedAt: updated);

void main() {
  final t1 = DateTime.utc(2026, 6, 1);
  final t2 = DateTime.utc(2026, 6, 2);
  final t3 = DateTime.utc(2026, 6, 3);

  test('recent → newest updatedAt first', () {
    final rows = [_row('a', 'alpha', t1), _row('b', 'beta', t3), _row('c', 'gamma', t2)];
    final out = sortRows(rows, RailSort.recent);
    expect(out.map((r) => r.id), ['b', 'c', 'a']);
  });

  test('name → case-insensitive A→Z', () {
    final rows = [_row('a', 'Zebra', t1), _row('b', 'apple', t2), _row('c', 'Mango', t3)];
    final out = sortRows(rows, RailSort.name);
    expect(out.map((r) => r.name), ['apple', 'Mango', 'Zebra']);
  });

  test('recent ties break by name (deterministic, no jitter)', () {
    final rows = [_row('a', 'beta', t1), _row('b', 'alpha', t1)];
    expect(sortRows(rows, RailSort.recent).map((r) => r.id), ['b', 'a']);
  });

  test('does not mutate the input list', () {
    final rows = [_row('a', 'b', t1), _row('b', 'a', t2)];
    final before = [...rows];
    sortRows(rows, RailSort.name);
    expect(rows, before);
  });
}
