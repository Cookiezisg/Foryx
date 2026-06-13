import 'dart:async';
import 'dart:convert';

import 'package:dio/dio.dart';
import 'package:flutter/foundation.dart';

import 'frame.dart';
import 'sse_parser.dart';

/// Lifecycle of one SSE connection, surfaced for a global status banner.
///
/// 一条 SSE 连接的生命周期,供全局状态横幅。
enum SseStatus { idle, connecting, live, reconnecting, stopped }

/// One long-lived SSE connection to a single stream endpoint (messages / entities /
/// notifications). Owns the reconnect STATE MACHINE — the part the architecture review
/// flagged as where bugs hide, not the line parser:
///
///  - resumes via `Last-Event-ID` (the highest DURABLE seq seen); ephemeral frames
///    (seq 0) never advance the cursor, so a dropped tick is never re-requested;
///  - on HTTP 410 (replay seq evicted) emits a one-shot [resync] then reconnects from a
///    FRESH head (no cursor) — consumers refetch durable truth from REST;
///  - reconnects on any drop with capped exponential backoff;
///  - never throws to callers: failures become status transitions + a backoff retry.
///
/// The emitted [envelopes] stream is broadcast and workspace-wide UNFILTERED (the
/// backend does not filter); the gateway demuxes it per scope.
///
/// 到单条流端点的长生命周期 SSE 连接。拥有重连**状态机**(评审标记 bug 藏身处、非行解析器):
/// 经 `Last-Event-ID`(见过的最高 DURABLE seq)续传;410 发一次性 [resync] 后从新 head 重连;
/// 任何断开按上限指数退避重连;绝不向调用方抛——失败化为状态转移 + 退避重试。
class SseConnection {
  SseConnection({
    required this.streamPath,
    required String baseUrl,
    required String? Function() workspaceId,
  })  : _workspaceId = workspaceId,
        _dio = Dio(BaseOptions(
          baseUrl: baseUrl,
          // No receive timeout: the stream is long-lived and the server pings every 15s.
          // 无接收超时:流长生命周期,服务端每 15s ping。
          receiveTimeout: null,
          connectTimeout: const Duration(seconds: 10),
        ));

  /// e.g. `/api/v1/messages/stream`.
  final String streamPath;
  final String? Function() _workspaceId;
  final Dio _dio;

  final _envelopes = StreamController<StreamEnvelope>.broadcast();
  final _resync = StreamController<void>.broadcast();
  final status = ValueNotifier<SseStatus>(SseStatus.idle);

  CancelToken? _cancel;
  bool _stopped = false;
  String? _lastEventId;
  int _backoffMs = 0;

  /// Workspace-wide, unfiltered frames (broadcast). Demuxed by the gateway.
  ///
  /// 工作区级、不过滤的帧(broadcast)。由 gateway demux。
  Stream<StreamEnvelope> get envelopes => _envelopes.stream;

  /// Fires once each time a 410 forces a full resync (consumers refetch REST).
  ///
  /// 每次 410 强制全量重同步时触发一次(消费方重取 REST)。
  Stream<void> get resync => _resync.stream;

  /// Begin connecting + the reconnect loop. Idempotent-safe to call once.
  ///
  /// 开始连接 + 重连循环。
  void start() {
    if (_stopped) return;
    unawaited(_runLoop());
  }

  Future<void> stop() async {
    _stopped = true;
    _cancel?.cancel('stopped');
    status.value = SseStatus.stopped;
    await _envelopes.close();
    await _resync.close();
    status.dispose();
  }

  Future<void> _runLoop() async {
    while (!_stopped) {
      try {
        status.value = _lastEventId == null && _backoffMs == 0
            ? SseStatus.connecting
            : SseStatus.reconnecting;
        await _connectOnce();
        // Clean EOF (server closed): loop and reconnect.
        // 干净 EOF(服务端关闭):循环重连。
      } on _ResyncSignal {
        // 410: drop the cursor, tell consumers to refetch, reconnect at fresh head.
        // 410:丢游标,通知消费方重取,从新 head 重连。
        _lastEventId = null;
        if (!_resync.isClosed) _resync.add(null);
        _backoffMs = 0;
        continue;
      } catch (e) {
        if (_stopped) break;
        debugPrint('SSE $streamPath error: $e');
      }
      if (_stopped) break;
      await Future<void>.delayed(Duration(milliseconds: _nextBackoff()));
    }
  }

  Future<void> _connectOnce() async {
    _cancel = CancelToken();
    final ws = _workspaceId();
    final response = await _dio.get<ResponseBody>(
      streamPath,
      queryParameters: {if (ws != null && ws.isNotEmpty) 'workspaceID': ws},
      options: Options(
        responseType: ResponseType.stream,
        headers: {
          'Accept': 'text/event-stream',
          if (_lastEventId != null) 'Last-Event-ID': _lastEventId,
        },
        // 410 (seq evicted) must reach us as a value, not a thrown DioException.
        // 410(seq 被淘汰)须作为值到达、非抛出的 DioException。
        validateStatus: (s) => s != null && (s == 200 || s == 410),
      ),
      cancelToken: _cancel,
    );

    if (response.statusCode == 410) {
      throw const _ResyncSignal();
    }

    status.value = SseStatus.live;
    _backoffMs = 0;
    final parser = SseEventParser()..lastEventId = _lastEventId;
    final lines = utf8.decoder
        .bind(response.data!.stream)
        .transform(const LineSplitter());
    await for (final line in lines) {
      if (_stopped) break;
      final env = parser.addLine(line);
      if (env != null && !_envelopes.isClosed) {
        _envelopes.add(env);
        if (env.durable) _lastEventId = parser.lastEventId;
      }
    }
  }

  /// Capped exponential backoff with a fixed floor; jitter is unnecessary for a single
  /// local connection (no thundering herd).
  ///
  /// 带固定下限的上限指数退避;单条本地连接无需抖动(无惊群)。
  int _nextBackoff() {
    _backoffMs = _backoffMs == 0 ? 500 : (_backoffMs * 2).clamp(500, 10000);
    return _backoffMs;
  }
}

/// Internal control-flow marker for a 410 resync (not an error surfaced to callers).
///
/// 410 重同步的内部控制流标记(非上呈调用方的错误)。
class _ResyncSignal implements Exception {
  const _ResyncSignal();
}
