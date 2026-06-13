import 'package:dio/dio.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../core/net/api_client.dart';
import '../core/sse/sse_gateway.dart';
import 'backend_controller.dart';

/// Composition root, expressed as Riverpod providers (the only DI system — no get_it).
/// Mirrors the backend's single-omniscient-bootstrap: the one runtime-determined value,
/// the backend base URL, is injected as a provider OVERRIDE once the sidecar is healthy;
/// everything else is constructed lazily from it.
///
/// 装配根,以 Riverpod provider 表达(唯一 DI——无 get_it)。镜像后端"唯一全知 bootstrap":
/// 唯一运行期决定的值——后端 base URL——在 sidecar 健康后作 provider OVERRIDE 注入;其余据之懒构造。

/// The sidecar controller, created + started in main() and injected as an override.
///
/// sidecar 控制器,在 main() 创建+启动并作 override 注入。
final backendControllerProvider = Provider<BackendController>(
  (ref) => throw UnimplementedError('override in main with the started controller'),
);

/// The resolved backend base URL. Overridden in the nested "ready" ProviderScope once
/// the sidecar's health probe passes (its value is unknown until the port is bound).
///
/// 解析后的后端 base URL。在 sidecar 健康探针通过后、于嵌套的"ready" ProviderScope 中 override
/// (端口绑定前其值未知)。
final baseUrlProvider = Provider<String>(
  (ref) => throw UnimplementedError('override in the ready ProviderScope'),
);

/// The active workspace id (the only auth/isolation axis). Null until one is selected;
/// `RequireWorkspace` 401s any workspace-scoped call made while null. Persistence is a
/// follow-up; this holds the in-memory selection the ApiClient/SSE stamp on requests.
///
/// 活动 workspace id(唯一鉴权/隔离轴)。选定前为 null;为 null 时任何按 workspace 隔离的调用被
/// `RequireWorkspace` 401。持久化后续;此持内存选择,供 ApiClient/SSE 盖在请求上。
final workspaceIdProvider =
    NotifierProvider<WorkspaceIdNotifier, String?>(WorkspaceIdNotifier.new);

class WorkspaceIdNotifier extends Notifier<String?> {
  @override
  String? build() => null;

  void select(String? id) => state = id;
}

/// The REST Dio, bound to the resolved base URL.
///
/// REST 用 Dio,绑解析后的 base URL。
final dioProvider = Provider<Dio>((ref) {
  final base = ref.watch(baseUrlProvider);
  return Dio(BaseOptions(
    baseUrl: base,
    connectTimeout: const Duration(seconds: 10),
    receiveTimeout: const Duration(seconds: 30),
    // Decode every response (incl. error bodies) so the ErrorInterceptor sees the
    // envelope; never throw on status alone here — _send maps it.
    // 解码每个响应(含错误体)使 ErrorInterceptor 见信封;此处不因状态抛——_send 映射它。
    validateStatus: (s) => s != null && s < 500 || s == 503,
  ));
});

/// The single HTTP boundary. Reads the active workspace id lazily on every request.
///
/// 唯一 HTTP 边界。每请求懒读活动 workspace id。
final apiClientProvider = Provider<ApiClient>((ref) {
  return ApiClient(
    dio: ref.watch(dioProvider),
    workspaceId: () => ref.read(workspaceIdProvider),
  );
});

/// The single owner of all live SSE — three connections + the demux. Started on first
/// read and torn down with the scope.
///
/// 全部 live SSE 的唯一持有者——三连接 + demux。首次读时启动,随 scope 拆除。
final sseGatewayProvider = Provider<SseGateway>((ref) {
  final gateway = SseGateway(
    baseUrl: ref.watch(baseUrlProvider),
    workspaceId: () => ref.read(workspaceIdProvider),
  );
  gateway.start();
  ref.onDispose(gateway.dispose);
  return gateway;
});
