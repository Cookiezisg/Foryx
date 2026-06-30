import '../../../core/contract/conversation.dart';
import '../../../core/contract/page.dart';
import '../../../core/net/api_client.dart';

/// How the conversation list is ordered. Mirrors the backend's three sort values exactly (a sealed
/// closed set — the rail's sort menu offers only these). [wire] is the `?sort=` query value.
///
/// 对话列表排序。逐字镜像后端三个 sort 值(封闭集——rail 排序菜单只此三项)。[wire] 是 `?sort=` 值。
enum ConvSort {
  activity, // pinned-first, then most-recently-active (default)
  created, // pinned-first, then creation order
  name; // pinned-first, then title A–Z (case-insensitive)

  // this.name (the Enum.name string getter) — a bare `name` would resolve to the ConvSort.name VALUE.
  // this.name（Enum.name 字符串 getter）——裸 `name` 会解析成 ConvSort.name 这个枚举值。
  String get wire => this.name;
}

/// Which archive states the list returns. Mirrors the backend `ArchiveScope`: active-only (default),
/// archived-only, or all (active + archived together — the rail's "show archived" mode, where archived
/// rows carry archived=true for the gray dot). [wire] is the `?archived=` value (null = omit = active).
///
/// 列表返回哪些归档态。镜像后端 ArchiveScope:仅活跃(默认)/仅归档/全部(活跃+归档同列——rail「显示已归档」,归档行
/// 带 archived=true 供灰点)。[wire] 是 `?archived=` 值(null = 省略 = 活跃)。
enum ConvArchive {
  active, // active only (default)
  all, // active + archived together
  archivedOnly; // archived only

  String? get wire => switch (this) {
        ConvArchive.active => null,
        ConvArchive.all => 'all',
        ConvArchive.archivedOnly => 'true',
      };
}

/// THE seam for the Chat feature's data access — every read/realtime/action the feature makes passes
/// through here, so the whole feature can be driven by one [FixtureChatRepository] override (no
/// per-provider HTTP/SSE mocking), exactly as the Entities feature does. [LiveChatRepository] wires the
/// Phase-4.0 pipeline (ApiClient); realtime + the per-thread message/action surface are added to this
/// interface as their build slices land (kept lean here — step 1 is the conversation LIST).
///
/// Chat feature 数据访问的唯一缝——feature 的每个读/实时/动作都过此,故整 feature 可被单个
/// FixtureChatRepository override 驱动(无 per-provider HTTP/SSE mock),与 Entities 同款。Live 接 Phase 4.0
/// 管道(ApiClient);实时与逐线程消息/动作面随各建造片落地再加(此处保持精简——step 1 = 对话列表)。
abstract interface class ChatRepository {
  /// One keyset page of the conversation list. [sort] / [archive] map to `?sort=` / `?archived=`;
  /// [search] is a case-insensitive title substring. Switching sort MUST drop the cursor (a cursor is
  /// meaningless under a different sort), so callers start a fresh page on sort change.
  ///
  /// 对话列表的一页 keyset。sort/archive 映射 `?sort=`/`?archived=`;search 是标题大小写不敏感子串。切换 sort
  /// 必须丢弃游标(跨 sort 游标无意义),故调用方切换排序时重新翻页。
  Future<Page<Conversation>> listConversations({
    String? cursor,
    int? limit,
    ConvSort sort,
    ConvArchive archive,
    String? search,
  });
}

/// The production repository over the Phase-4.0 pipeline. Holds no state; the method is a thin
/// envelope-decode over [ApiClient.getPage]. (Realtime gets the nullable SseGateway added in the
/// live-wiring slice — omitted now since step 1 has no realtime method.)
///
/// 生产 repository(接 Phase 4.0 管道)。无状态;方法是 ApiClient.getPage 上的薄信封解码。(实时在 live-wiring
/// 片加可空 SseGateway——此处省,step 1 无实时方法。)
class LiveChatRepository implements ChatRepository {
  LiveChatRepository({required ApiClient api}) : _api = api;

  final ApiClient _api;

  @override
  Future<Page<Conversation>> listConversations({
    String? cursor,
    int? limit,
    ConvSort sort = ConvSort.activity,
    ConvArchive archive = ConvArchive.active,
    String? search,
  }) {
    final q = <String, dynamic>{
      'cursor': ?cursor,
      'limit': ?limit,
      'sort': sort.wire,
      'archived': ?archive.wire,
      'search': ?search,
    };
    return _api.getPage('/api/v1/conversations', Conversation.fromJson, query: q);
  }
}
