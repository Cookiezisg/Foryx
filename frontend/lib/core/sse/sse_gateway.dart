import 'dart:async';

import 'package:flutter/foundation.dart';

import 'frame.dart';
import 'sse_connection.dart';

/// The three (and only three, E1) SSE streams the client keeps open for the whole
/// session.
///
/// 客户端整会话常驻的三条(且仅三条,E1)SSE 流。
enum StreamName {
  messages('/api/v1/messages/stream'),
  entities('/api/v1/entities/stream'),
  notifications('/api/v1/notifications/stream');

  const StreamName(this.path);
  final String path;
}

/// The single owner of all live SSE. Holds the three [SseConnection]s and, crucially,
/// a plain-Dart DEMUX layer BELOW Riverpod: it pre-buckets the workspace-wide frame
/// feed into per-scope and per-kind broadcast streams, so a subscriber attaches to a
/// stream that already carries only its frames. (Subscribing every consumer to the raw
/// feed and filtering with `.where` per frame would be O(frames × subscribers) — the
/// rebuild storm the architecture review flagged; demux is the fix.)
///
///  - [scopeStream] — frames for ONE scope (conversation:id / function:id …): the chat
///    thread, one entity panel, one flowrun. The high-frequency path (token deltas,
///    node ticks).
///  - [kindStream] — frames for a whole entity KIND on a stream: an entity LIST view
///    that must react to create/delete/version-move of entities not on the current page.
///  - [rawStream] — the unbucketed feed for the notifications centre (one stream, no
///    per-id filtering needed).
///
/// 全部 live SSE 的唯一持有者。持三条连接,并在 Riverpod **下面**垫纯 Dart DEMUX:把工作区级
/// 帧流预分桶成 per-scope / per-kind broadcast 流,使订阅方挂上只载自己帧的流(否则每消费者对
/// 原始流逐帧 `.where` 是 O(帧×订阅者) 的重建风暴)。
class SseGateway {
  SseGateway({
    required String baseUrl,
    required String? Function() workspaceId,
    required String? Function() authToken,
    @visibleForTesting SseConnection Function(StreamName name)? connectionFactory,
  }) {
    for (final name in StreamName.values) {
      final conn = connectionFactory?.call(name) ??
          SseConnection(
            streamPath: name.path,
            baseUrl: baseUrl,
            workspaceId: workspaceId,
            authToken: authToken,
          );
      _connections[name] = conn;
      conn.envelopes.listen((env) => _route(name, env));
    }
  }

  final _connections = <StreamName, SseConnection>{};
  final _byScope = <String, StreamController<StreamEnvelope>>{};
  final _byKind = <String, StreamController<StreamEnvelope>>{};

  void start() {
    for (final c in _connections.values) {
      c.start();
    }
  }

  /// Per-stream connection status (for a global "backend live / reconnecting" banner).
  ///
  /// 每流连接状态(供全局"后端 live / 重连"横幅)。
  SseConnection connection(StreamName name) => _connections[name]!;

  void _route(StreamName name, StreamEnvelope env) {
    _byScope[env.scope.key]?.add(env);
    _byKind['${name.name}:${env.scope.kind}']?.add(env);
  }

  /// Frames for exactly one scope (kind+id). Lazily created, auto-removed when the last
  /// listener leaves.
  ///
  /// 恰好一个 scope(kind+id)的帧。懒创建,末个监听者离开即自动移除。
  Stream<StreamEnvelope> scopeStream(StreamScope scope) =>
      _lazy(_byScope, scope.key);

  /// Frames for a whole entity kind on a stream (e.g. all `function` activity on
  /// entities) — for list views reconciling against entities not on the current page.
  ///
  /// 一条流上整个实体 kind 的帧(如 entities 上所有 `function` 活动)——供列表视图与不在当前页
  /// 的实体对账。
  Stream<StreamEnvelope> kindStream(StreamName name, String kind) =>
      _lazy(_byKind, '${name.name}:$kind');

  /// The unbucketed feed of a whole stream (notifications centre).
  ///
  /// 整条流的未分桶 feed(通知中心)。
  Stream<StreamEnvelope> rawStream(StreamName name) =>
      _connections[name]!.envelopes;

  /// A 410-resync signal for a stream — consumers refetch durable REST state, then the
  /// connection resumes at a fresh head automatically.
  ///
  /// 某条流的 410 重同步信号——消费方重取 REST 耐久态,连接随后自动从新 head 续。
  Stream<void> resync(StreamName name) => _connections[name]!.resync;

  Stream<StreamEnvelope> _lazy(
    Map<String, StreamController<StreamEnvelope>> registry,
    String key,
  ) {
    final existing = registry[key];
    if (existing != null) return existing.stream;
    late final StreamController<StreamEnvelope> ctrl;
    ctrl = StreamController<StreamEnvelope>.broadcast(
      onCancel: () => registry.remove(key),
    );
    registry[key] = ctrl;
    return ctrl.stream;
  }

  Future<void> dispose() async {
    for (final c in _connections.values) {
      await c.stop();
    }
    for (final c in _byScope.values) {
      await c.close();
    }
    for (final c in _byKind.values) {
      await c.close();
    }
    _byScope.clear();
    _byKind.clear();
  }
}
