import 'package:anselm/core/contract/conversation.dart';
import 'package:anselm/core/model/status_state.dart';
import 'package:anselm/features/chat/ui/conversation_rail_model.dart';
import 'package:flutter_test/flutter_test.dart';

// Local (not UTC) timestamps so toLocal() is identity → bucket/time tests are TZ-deterministic.
Conversation _cAt(String id, DateTime at, {bool pinned = false}) =>
    Conversation(id: id, title: id, pinned: pinned, createdAt: at, updatedAt: at, lastMessageAt: at);

final _now = DateTime(2026, 6, 26, 12);

final _timeStrings = ConvTimeStrings(
  justNow: 'now',
  yesterday: 'yest',
  minutesAgo: (n) => '${n}m',
  hoursAgo: (n) => '${n}h',
  daysAgo: (n) => '${n}d',
);

const _labels = ConvRailLabels(
  newLabel: 'New',
  filter: 'Filter',
  pinned: 'PINNED',
  today: 'TODAY',
  yesterday: 'YEST',
  lastWeek: 'WK',
  older: 'OLD',
  time: ConvTimeStrings(justNow: 'now', yesterday: 'yest', minutesAgo: _m, hoursAgo: _h, daysAgo: _d),
);
String _m(int n) => '${n}m';
String _h(int n) => '${n}h';
String _d(int n) => '${n}d';

// STEP 3 gate — the conversation-row lead-dot mapping. The row itself is a plain AnRow (verified
// visually in the gallery's Chat category); this pins the precedence that picks WHICH dot:
// generating > awaiting > unread > archived > none.

Conversation _c({bool generating = false, bool awaiting = false, bool unread = false, bool archived = false}) {
  final t = DateTime.utc(2026, 6, 26);
  return Conversation(
    id: 'cv_1',
    title: 't',
    createdAt: t,
    updatedAt: t,
    lastMessageAt: t,
    isGenerating: generating,
    awaitingInput: awaiting,
    hasUnread: unread,
    archived: archived,
  );
}

void main() {
  test('a plain active thread has no dot', () {
    expect(conversationDot(_c()), isNull);
  });

  test('generating → run (blue), the highest precedence', () {
    expect(conversationDot(_c(generating: true)), AnStatus.run);
    // wins even when every flag is set at once
    expect(conversationDot(_c(generating: true, awaiting: true, unread: true, archived: true)), AnStatus.run);
  });

  test('awaiting input → wait (amber), over unread + archived', () {
    expect(conversationDot(_c(awaiting: true)), AnStatus.wait);
    expect(conversationDot(_c(awaiting: true, unread: true, archived: true)), AnStatus.wait);
  });

  test('unread → done (green), over archived', () {
    expect(conversationDot(_c(unread: true)), AnStatus.done);
    expect(conversationDot(_c(unread: true, archived: true)), AnStatus.done);
  });

  test('archived → idle (gray marker), the lowest', () {
    expect(conversationDot(_c(archived: true)), AnStatus.idle);
  });

  group('conversationBucket', () {
    test('pinned → pinned regardless of time', () {
      expect(conversationBucket(_cAt('x', _now.subtract(const Duration(days: 99)), pinned: true), _now),
          ConvBucket.pinned);
    });
    test('same calendar day → today', () {
      expect(conversationBucket(_cAt('x', DateTime(2026, 6, 26, 2)), _now), ConvBucket.today);
    });
    test('previous calendar day → yesterday', () {
      expect(conversationBucket(_cAt('x', DateTime(2026, 6, 25, 23)), _now), ConvBucket.yesterday);
    });
    test('2–7 days → lastWeek', () {
      expect(conversationBucket(_cAt('x', DateTime(2026, 6, 23, 10)), _now), ConvBucket.lastWeek);
    });
    test('>7 days → older', () {
      expect(conversationBucket(_cAt('x', DateTime(2026, 6, 1, 10)), _now), ConvBucket.older);
    });
  });

  group('conversationTimeLabel', () {
    String label(DateTime at) => conversationTimeLabel(at, _now, _timeStrings);
    test('< 1 min → just now', () => expect(label(_now), 'now'));
    test('< 60 min → N min', () => expect(label(_now.subtract(const Duration(minutes: 5))), '5m'));
    test('same day, hours → N hr', () => expect(label(DateTime(2026, 6, 26, 9)), '3h'));
    test('previous day → yesterday', () => expect(label(DateTime(2026, 6, 25, 9)), 'yest'));
    test('2–7 days → N days', () => expect(label(DateTime(2026, 6, 23, 9)), '3d'));
    test('> 7 days → numeric y/m/d', () => expect(label(DateTime(2026, 5, 27, 9)), '2026/5/27'));
  });

  group('buildConversationRailModel', () {
    final rows = [
      _cAt('cv_pin', DateTime(2026, 6, 20, 9), pinned: true),
      _cAt('cv_today', DateTime(2026, 6, 26, 9)),
      _cAt('cv_yest', DateTime(2026, 6, 25, 9)),
      _cAt('cv_week', DateTime(2026, 6, 23, 9)),
      _cAt('cv_old', DateTime(2026, 6, 1, 9)),
    ];

    test('grouped → one group, one collapsible type per non-empty bucket (entities head style)', () {
      final m = buildConversationRailModel(rows, now: _now, groupByTime: true, labels: _labels);
      // ONE group holding N typed sections — mirrors the entities rail (count is the type's far-right meta).
      final types = m.groups.single.types;
      expect(types.map((t) => t.label), ['PINNED', 'TODAY', 'YEST', 'WK', 'OLD']);
      expect(types.map((t) => t.count), [1, 1, 1, 1, 1]);
      // the pinned thread is in PINNED, not its time bucket.
      expect(types.first.rows.single.id, 'cv_pin');
      expect(types[1].rows.single.id, 'cv_today');
      // rows carry a relative-time meta (>7 days → a numeric date).
      expect(types.last.rows.single.meta, '2026/6/1');
    });

    test('flat → one headless group (label null), all rows, server order preserved', () {
      final m = buildConversationRailModel(rows, now: _now, groupByTime: false, labels: _labels);
      expect(m.groups.length, 1);
      expect(m.groups.single.label, isNull);
      expect(m.groups.single.types.single.rows.map((r) => r.id),
          ['cv_pin', 'cv_today', 'cv_yest', 'cv_week', 'cv_old']);
    });
  });
}
