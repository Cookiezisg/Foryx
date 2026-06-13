import 'package:dio/dio.dart';

import '../contract/api_error.dart';
import '../contract/page.dart';

/// The HTTP boundary to the local Go backend. Encodes the standardized contract
/// (ADR 0003) exactly ONCE so no feature hand-rolls envelope/error/pagination handling:
///
///  - success  → `{"data": <bare entity>}`            → [getEntity] / [postEntity]
///  - list     → `{data:[…], nextCursor?, hasMore}`   → [getPage]
///  - async    → `202 {"data":{"id": …}}`             → [postForId]
///  - 204      → no body                              → [delete] / [postNoContent]
///  - error    → `{"error":{code,message,details}}`   → thrown as [ApiException]
///
/// Workspace isolation rides the `X-Forgify-Workspace-ID` header on every request
/// (backend middleware.HeaderWorkspaceID); the client never sends/reads workspace_id in
/// bodies. The id source is injected as a callback so this layer stays Riverpod-free.
///
/// 到本地 Go 后端的 HTTP 边界。把标准化契约(ADR 0003)**只编码一次**,使无 feature 手搓
/// envelope/error/分页。workspace 隔离经每请求的 `X-Forgify-Workspace-ID` header;客户端
/// 体内绝不带 workspace_id。id 来源以回调注入,使本层不沾 Riverpod。
class ApiClient {
  ApiClient({required Dio dio, required String? Function() workspaceId})
      : _dio = dio,
        _workspaceId = workspaceId {
    _dio.interceptors.add(InterceptorsWrapper(onRequest: _onRequest));
  }

  final Dio _dio;
  final String? Function() _workspaceId;

  void _onRequest(RequestOptions options, RequestInterceptorHandler handler) {
    final ws = _workspaceId();
    if (ws != null && ws.isNotEmpty) {
      options.headers['X-Forgify-Workspace-ID'] = ws;
    }
    handler.next(options);
  }

  /// GET a single entity: unwrap `{data:<obj>}` → [parse].
  ///
  /// GET 单实体:拆 `{data:<obj>}` → [parse]。
  Future<T> getEntity<T>(
    String path,
    T Function(Map<String, dynamic>) parse, {
    Map<String, dynamic>? query,
  }) =>
      _send(() async {
        final r = await _dio.get<Map<String, dynamic>>(path,
            queryParameters: query);
        return parse(_data(r.data));
      });

  /// GET a keyset page: `{data:[…], nextCursor?, hasMore}` → [Page].
  ///
  /// GET 一页 keyset → [Page]。
  Future<Page<T>> getPage<T>(
    String path,
    T Function(Map<String, dynamic>) item, {
    Map<String, dynamic>? query,
  }) =>
      _send(() async {
        final r = await _dio.get<Map<String, dynamic>>(path,
            queryParameters: query);
        return Page.fromBody(r.data ?? const {}, item);
      });

  /// GET a raw envelope body (for composite reads like `{flowrun, nodes}` whose `data`
  /// is a multi-key object the caller destructures itself).
  ///
  /// GET 原始信封体(供 `{flowrun, nodes}` 这类 `data` 为多 key 对象、调用方自解的复合读)。
  Future<Map<String, dynamic>> getData(String path,
          {Map<String, dynamic>? query}) =>
      _send(() async {
        final r = await _dio.get<Map<String, dynamic>>(path,
            queryParameters: query);
        return _data(r.data);
      });

  /// POST returning a created/edited entity: `{data:<obj>}` → [parse]. Covers Create
  /// (201) and state-change actions (`:activate` … return the post-action snapshot).
  ///
  /// POST 返回创建/编辑后实体 → [parse]。覆盖 Create(201)与状态变更动作的后置快照。
  Future<T> postEntity<T>(
    String path,
    T Function(Map<String, dynamic>) parse, {
    Object? body,
  }) =>
      _send(() async {
        final r = await _dio.post<Map<String, dynamic>>(path, data: body);
        return parse(_data(r.data));
      });

  /// PATCH/PUT returning the updated entity snapshot.
  ///
  /// PATCH/PUT 返回更新后实体快照。
  Future<T> patchEntity<T>(
    String path,
    T Function(Map<String, dynamic>) parse, {
    Object? body,
    bool put = false,
  }) =>
      _send(() async {
        final r = put
            ? await _dio.put<Map<String, dynamic>>(path, data: body)
            : await _dio.patch<Map<String, dynamic>>(path, data: body);
        return parse(_data(r.data));
      });

  /// POST an async action that returns a single new resource id: `202 {data:{id}}` →
  /// the id string (MD3). E.g. send-message, `:trigger`, `:fire`, `:iterate`.
  ///
  /// POST 返单产物 id 的异步动作 → id 字符串(MD3)。如发消息、`:trigger`、`:fire`、`:iterate`。
  Future<String> postForId(String path, {Object? body}) => _send(() async {
        final r = await _dio.post<Map<String, dynamic>>(path, data: body);
        return _data(r.data)['id'] as String;
      });

  /// POST a synchronous executor (`:run`/`:call`/`:invoke`) that returns a BARE result
  /// (not wrapped in `{data}`/`{result}`). Returns the decoded body as-is.
  ///
  /// POST 同步执行器(`:run`/`:call`/`:invoke`),返**裸结果**(不裹 `{data}`/`{result}`)。
  Future<dynamic> postBare(String path, {Object? body}) => _send(() async {
        final r = await _dio.post<dynamic>(path, data: body);
        return r.data;
      });

  /// POST a fire-and-forget action with no product (204) — e.g. `:reindex`, resolve.
  ///
  /// POST 无产物的 fire-and-forget(204)——如 `:reindex`、resolve。
  Future<void> postNoContent(String path, {Object? body}) =>
      _send(() async => _dio.post<void>(path, data: body));

  /// DELETE (204).
  Future<void> delete(String path) =>
      _send(() async => _dio.delete<void>(path));

  /// Unwrap the `data` object from an envelope, or throw if absent/wrong shape.
  ///
  /// 从信封拆出 `data` 对象,缺失/形状不对则抛。
  Map<String, dynamic> _data(Map<String, dynamic>? body) {
    final data = body?['data'];
    if (data is Map<String, dynamic>) return data;
    throw ApiException(
      code: ForgifyErr.unknown,
      message: 'response had no data object',
      httpStatus: 200,
    );
  }

  /// Run a Dio call, translating every DioException into a typed [ApiException]: a
  /// response carrying `{error:{…}}` → [ApiException.fromEnvelope]; a transport failure
  /// (no response) → [ApiException.transport]. The single place HTTP plumbing becomes a
  /// domain error.
  ///
  /// 跑一次 Dio 调用,把每个 DioException 译成 typed [ApiException]:带 `{error:{…}}` 的响应
  /// → fromEnvelope;无响应的传输失败 → transport。HTTP 管道化为 domain 错误的唯一处。
  Future<T> _send<T>(Future<T> Function() call) async {
    try {
      return await call();
    } on DioException catch (e) {
      final resp = e.response;
      if (resp != null) {
        final body = resp.data;
        final error = (body is Map<String, dynamic>)
            ? body['error'] as Map<String, dynamic>?
            : null;
        throw ApiException.fromEnvelope(error, resp.statusCode ?? 0);
      }
      throw ApiException.transport(e.message ?? 'transport failure');
    }
  }
}
