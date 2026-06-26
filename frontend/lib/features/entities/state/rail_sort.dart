import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../data/entity_row.dart';

/// How the rail orders rows within each kind section. Default [recent] (most-recently-touched first) —
/// the working-set order; [name] is A→Z for hunting by name. A transient view preference (not server
/// state), so a plain Notifier. 行在各 kind 段内的排序:默认最近更新,name 为字母序。瞬时视图偏好。
enum RailSort { recent, name }

class RailSortNotifier extends Notifier<RailSort> {
  @override
  RailSort build() => RailSort.recent;

  void set(RailSort sort) => state = sort;
}

final railSortProvider = NotifierProvider<RailSortNotifier, RailSort>(RailSortNotifier.new);

/// Order a kind's rows by [sort], stably (name tiebreak keeps it deterministic frame to frame, so the
/// list never jitters on equal keys). Returns a new list; never mutates the input. 稳定排序(name 兜底)。
List<EntityRow> sortRows(List<EntityRow> rows, RailSort sort) {
  final out = [...rows];
  switch (sort) {
    case RailSort.recent:
      out.sort((a, b) {
        final byTime = b.updatedAt.compareTo(a.updatedAt); // newest first
        return byTime != 0 ? byTime : a.name.toLowerCase().compareTo(b.name.toLowerCase());
      });
    case RailSort.name:
      out.sort((a, b) {
        final byName = a.name.toLowerCase().compareTo(b.name.toLowerCase());
        return byName != 0 ? byName : a.id.compareTo(b.id);
      });
  }
  return out;
}
