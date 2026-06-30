import 'package:anselm/core/contract/conversation.dart';
import 'package:anselm/features/chat/data/chat_fixtures.dart';
import 'package:anselm/features/chat/data/chat_providers.dart';
import 'package:anselm/features/chat/data/chat_repository.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';

// STEP 1 gate — the chat data seam. Pins: the Conversation DTO mirrors the wire (incl. the three
// status flags + ignoring unknown keys), and the fixture reproduces the backend list semantics
// (archive scope, title search, pinned-first + sort ordering, keyset paging) at the single provider
// the whole feature swaps backends at.

DateTime _at(int hour) => DateTime.utc(2026, 6, 26, hour);

Conversation _c(
  String id,
  String title, {
  bool pinned = false,
  bool archived = false,
  int hour = 12,
}) =>
    Conversation(
      id: id,
      title: title,
      pinned: pinned,
      archived: archived,
      createdAt: _at(hour),
      updatedAt: _at(hour),
      lastMessageAt: _at(hour),
    );

FixtureChatRepository _repo() => FixtureChatRepository(conversations: [
      _c('cv_a', 'Apple', hour: 9),
      _c('cv_b', 'banana', pinned: true, hour: 8),
      _c('cv_c', 'Cherry', hour: 11),
      _c('cv_d', 'delta', archived: true, hour: 10),
    ]);

void main() {
  group('Conversation DTO', () {
    test('fromJson mirrors the wire (incl. the three status flags)', () {
      final c = Conversation.fromJson({
        'id': 'cv_1',
        'title': 'hello',
        'autoTitled': true,
        'archived': false,
        'pinned': true,
        'createdAt': '2026-06-26T08:00:00Z',
        'updatedAt': '2026-06-26T09:00:00Z',
        'lastMessageAt': '2026-06-26T09:30:00Z',
        'isGenerating': true,
        'awaitingInput': false,
        'hasUnread': true,
      });
      expect(c.id, 'cv_1');
      expect(c.title, 'hello');
      expect(c.pinned, true);
      expect(c.isGenerating, true);
      expect(c.awaitingInput, false);
      expect(c.hasUnread, true);
      expect(c.lastMessageAt, DateTime.utc(2026, 6, 26, 9, 30));
    });

    test('unknown wire keys (e.g. the removed lastMessagePreview) are ignored; optionals default', () {
      final c = Conversation.fromJson({
        'id': 'cv_2',
        'title': '',
        'lastMessagePreview': 'IGNORED',
        'somethingNew': 42,
        'createdAt': '2026-06-26T08:00:00Z',
        'updatedAt': '2026-06-26T08:00:00Z',
        'lastMessageAt': '2026-06-26T08:00:00Z',
      });
      expect(c.id, 'cv_2');
      expect(c.title, '');
      expect(c.hasUnread, false);
      expect(c.pinned, false);
    });
  });

  group('FixtureChatRepository.listConversations', () {
    test('active scope excludes archived (the default)', () async {
      final p = await _repo().listConversations();
      expect(p.items.map((c) => c.id), isNot(contains('cv_d')));
    });

    test('archive=all includes archived; archivedOnly returns only archived', () async {
      final all = await _repo().listConversations(archive: ConvArchive.all);
      expect(all.items.map((c) => c.id), contains('cv_d'));
      final only = await _repo().listConversations(archive: ConvArchive.archivedOnly);
      expect(only.items.map((c) => c.id), ['cv_d']);
    });

    test('sort=name is A–Z case-insensitive, pinned-first', () async {
      final p = await _repo().listConversations(sort: ConvSort.name);
      // cv_b (banana, pinned) leads; then NOCASE A–Z over active rows: Apple, Cherry.
      expect(p.items.map((c) => c.id), ['cv_b', 'cv_a', 'cv_c']);
    });

    test('sort=activity is recency-desc, pinned-first', () async {
      final p = await _repo().listConversations(sort: ConvSort.activity);
      // pinned cv_b first; then lastMessageAt desc: cv_c(11) > cv_a(9).
      expect(p.items.map((c) => c.id), ['cv_b', 'cv_c', 'cv_a']);
    });

    test('search filters by title substring, case-insensitive', () async {
      final p = await _repo().listConversations(search: 'APP');
      expect(p.items.map((c) => c.id), ['cv_a']);
    });

    test('keyset pagination walks the list via cursor (no dup/miss)', () async {
      final p1 = await _repo().listConversations(sort: ConvSort.name, limit: 2);
      expect(p1.items.map((c) => c.id), ['cv_b', 'cv_a']);
      expect(p1.hasMore, true);
      final p2 = await _repo()
          .listConversations(sort: ConvSort.name, limit: 2, cursor: p1.nextCursor);
      expect(p2.items.map((c) => c.id), ['cv_c']);
      expect(p2.hasMore, false);
    });
  });

  test('chatRepositoryProvider swaps to the fixture at one seam', () {
    final container = ProviderContainer(
      overrides: [chatRepositoryProvider.overrideWithValue(_repo())],
    );
    addTearDown(container.dispose);
    expect(container.read(chatRepositoryProvider), isA<FixtureChatRepository>());
  });
}
