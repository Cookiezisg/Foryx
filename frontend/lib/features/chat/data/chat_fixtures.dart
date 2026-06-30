import '../../../core/contract/conversation.dart';
import '../../../core/contract/page.dart';
import 'chat_repository.dart';

/// In-memory, scriptable [ChatRepository] — the SINGLE seam the whole Chat feature is driven by in
/// gallery / widget / provider tests and the zero-backend demo (mirrors [FixtureEntityRepository]). It
/// reproduces the backend's list semantics faithfully so the demo and tests behave like the real
/// thing: the archive scope filter, the title search substring, the pinned-first + sort ordering, and
/// keyset pagination (cursor = next start index). Seeds are held in a mutable list so later slices can
/// add upsert / mutate for scripted live updates.
///
/// 内存、可脚本化的 ChatRepository——gallery / widget / provider 测试与零后端 demo 驱动整 Chat feature 的唯一
/// 缝(镜像 FixtureEntityRepository)。忠实复现后端列表语义,使 demo/测试行为如真:归档范围过滤、标题搜索子串、
/// 置顶优先 + sort 排序、keyset 分页(cursor = 下一起始下标)。种子放可变 list,供后续片加 upsert / mutate 脚本化实时。
class FixtureChatRepository implements ChatRepository {
  FixtureChatRepository({List<Conversation>? conversations})
      : _all = List.of(conversations ?? const []);

  final List<Conversation> _all;

  // cursor = the next start index, as a string (same scheme as FixtureEntityRepository._page).
  // cursor = 下一起始下标的字符串(同 FixtureEntityRepository._page 方案)。
  static Page<T> _page<T>(List<T> all, String? cursor, int? limit) {
    final start = int.tryParse(cursor ?? '') ?? 0;
    final n = limit ?? all.length;
    final end = (start + n).clamp(0, all.length);
    final slice = all.sublist(start.clamp(0, all.length), end);
    final more = end < all.length;
    return Page(items: slice, nextCursor: more ? '$end' : null, hasMore: more);
  }

  // pinned-first, then the sort's secondary key, then an id tiebreaker matching the backend
  // (activity/created → id DESC, name → id ASC) so paging is deterministic.
  // 置顶优先、再 sort 次键、再 id tiebreaker(与后端一致:activity/created→id 降序、name→id 升序),使分页确定。
  static Comparator<Conversation> _comparator(ConvSort sort) => (a, b) {
        if (a.pinned != b.pinned) return a.pinned ? -1 : 1;
        final primary = switch (sort) {
          ConvSort.activity => b.lastMessageAt.compareTo(a.lastMessageAt),
          ConvSort.created => b.createdAt.compareTo(a.createdAt),
          ConvSort.name => a.title.toLowerCase().compareTo(b.title.toLowerCase()),
        };
        if (primary != 0) return primary;
        return sort == ConvSort.name ? a.id.compareTo(b.id) : b.id.compareTo(a.id);
      };

  @override
  Future<Page<Conversation>> listConversations({
    String? cursor,
    int? limit,
    ConvSort sort = ConvSort.activity,
    ConvArchive archive = ConvArchive.active,
    String? search,
  }) async {
    final term = search?.trim().toLowerCase() ?? '';
    final rows = _all.where((c) {
      final scopeOk = switch (archive) {
        ConvArchive.active => !c.archived,
        ConvArchive.archivedOnly => c.archived,
        ConvArchive.all => true,
      };
      if (!scopeOk) return false;
      if (term.isNotEmpty && !c.title.toLowerCase().contains(term)) return false;
      return true;
    }).toList()
      ..sort(_comparator(sort));
    return _page(rows, cursor, limit);
  }
}
