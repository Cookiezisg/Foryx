/// One keyset page of a List endpoint. Mirrors the N4/MD2 contract exactly: the
/// response is `{data: [...], nextCursor?, hasMore}` — pagination coordinates sit at
/// the TOP level (siblings of `data`), never inside it. `nextCursor` is omitted on the
/// last page. Pass the next request `?cursor=<nextCursor>&limit=…` to page forward.
///
/// List 端点的一页 keyset。精确镜像 N4/MD2 契约:响应为 `{data:[...], nextCursor?, hasMore}`
/// ——分页坐标在顶层(`data` 的兄弟),绝不在其内。`nextCursor` 末页省略。下次请求带
/// `?cursor=<nextCursor>&limit=…` 翻页。
class Page<T> {
  const Page({required this.items, this.nextCursor, required this.hasMore});

  final List<T> items;
  final String? nextCursor;
  final bool hasMore;

  bool get isLastPage => !hasMore || nextCursor == null;

  /// Parse the full response body. [item] decodes one element of `data`.
  ///
  /// 解析整个响应体。[item] 解一个 `data` 元素。
  factory Page.fromBody(
    Map<String, dynamic> body,
    T Function(Map<String, dynamic>) item,
  ) {
    final data = (body['data'] as List<dynamic>? ?? const [])
        .map((e) => item(e as Map<String, dynamic>))
        .toList(growable: false);
    return Page(
      items: data,
      nextCursor: body['nextCursor'] as String?,
      hasMore: body['hasMore'] as bool? ?? false,
    );
  }
}

/// A List page whose `data` is an object carrying both a list AND an aggregate sidecar
/// — the execution / call / mcp-call / search log endpoints (MD2): `{data: {LIST,
/// aggregates|total}, nextCursor?, hasMore}`. Pagination stays top-level; the aggregate
/// rides inside `data` next to the list.
///
/// `data` 是同时携带列表与聚合旁挂的 List 页——执行/调用/mcp-call/搜索日志端点(MD2):
/// `{data:{LIST, aggregates|total}, nextCursor?, hasMore}`。分页仍顶层;聚合在 `data`
/// 内与列表并列。
class PageWithAggregate<T, A> {
  const PageWithAggregate({
    required this.items,
    required this.aggregate,
    this.nextCursor,
    required this.hasMore,
  });

  final List<T> items;
  final A aggregate;
  final String? nextCursor;
  final bool hasMore;

  bool get isLastPage => !hasMore || nextCursor == null;

  /// [listKey] is the array field inside `data` (e.g. "executions" / "calls" / "hits");
  /// [item] decodes one element; [aggregate] decodes the whole `data` object into A.
  ///
  /// [listKey] 是 `data` 内的数组字段(如 "executions"/"calls"/"hits");[item] 解一个元素;
  /// [aggregate] 把整个 `data` 对象解成 A。
  factory PageWithAggregate.fromBody(
    Map<String, dynamic> body,
    String listKey,
    T Function(Map<String, dynamic>) item,
    A Function(Map<String, dynamic>) aggregate,
  ) {
    final data = (body['data'] as Map<String, dynamic>? ?? const {});
    final list = (data[listKey] as List<dynamic>? ?? const [])
        .map((e) => item(e as Map<String, dynamic>))
        .toList(growable: false);
    return PageWithAggregate(
      items: list,
      aggregate: aggregate(data),
      nextCursor: body['nextCursor'] as String?,
      hasMore: body['hasMore'] as bool? ?? false,
    );
  }
}
