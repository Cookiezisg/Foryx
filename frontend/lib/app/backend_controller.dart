import 'dart:async';
import 'dart:io';

import 'package:dio/dio.dart';
import 'package:flutter/foundation.dart';

/// Where the local Go backend is in its lifecycle. The whole app gates on this: a
/// single banner / splash reads it, NOT per-feature error handling.
///
/// 本地 Go 后端处于生命周期何处。整个 app 据此门控:单一横幅/启动屏读它,非逐 feature 处理。
enum BackendPhase { starting, ready, crashed }

@immutable
class BackendState {
  const BackendState(this.phase, {this.baseUrl, this.error});
  final BackendPhase phase;
  final String? baseUrl;
  final String? error;

  bool get isReady => phase == BackendPhase.ready && baseUrl != null;
}

/// Owns the Go backend as a child process (the sidecar model, ADR 0004 §1). The backend
/// is a standalone localhost HTTP+SSE server; the client launches it, health-gates it,
/// and shuts it down with the app — zero backend changes (it already reads `FORGIFY_ADDR`
/// / `FORGIFY_DATA_DIR` and serves `GET /api/v1/health`, verified in the backend).
///
/// Resolution order:
///  1. `FORGIFY_BACKEND_URL` env — the dev escape hatch: attach to an already-running
///     backend (e.g. `make server`), spawn nothing. This is the primary dev path until
///     the binary is bundled.
///  2. spawn the bundled binary: bind a free ephemeral port in Dart, pass it via
///     `FORGIFY_ADDR=127.0.0.1:<port>`, launch, then poll `/api/v1/health` until 200.
///
/// 以 sidecar 模型托管 Go 后端(ADR 0004 §1)。后端是独立 localhost HTTP+SSE server;客户端拉起、
/// 健康门控、随 app 关停——零后端改动(它已读 `FORGIFY_ADDR`/`FORGIFY_DATA_DIR`、提供
/// `GET /api/v1/health`,已核实)。解析序:① `FORGIFY_BACKEND_URL` dev 逃生口(挂到已跑后端、不 spawn);
/// ② spawn 内置二进制(Dart 抢临时端口→`FORGIFY_ADDR` 注入→轮询 /health 至 200)。
class BackendController {
  BackendController({
    this.binaryPath,
    this.dataDir,
    Dio? probe,
  }) : _probe = probe ?? Dio();

  /// Absolute path to the bundled `server` binary; null = resolve next to the app
  /// executable (packaging concern). Ignored when `FORGIFY_BACKEND_URL` is set.
  ///
  /// 内置 `server` 二进制的绝对路径;null = 在 app 可执行文件旁解析(打包关注点)。
  final String? binaryPath;
  final String? dataDir;
  final Dio _probe;

  final state = ValueNotifier<BackendState>(const BackendState(BackendPhase.starting));

  Process? _child;

  Future<void> start() async {
    try {
      final external = Platform.environment['FORGIFY_BACKEND_URL'];
      if (external != null && external.isNotEmpty) {
        await _awaitHealth(external);
        state.value = BackendState(BackendPhase.ready, baseUrl: external);
        return;
      }
      final baseUrl = await _spawn();
      await _awaitHealth(baseUrl);
      state.value = BackendState(BackendPhase.ready, baseUrl: baseUrl);
    } catch (e) {
      state.value = BackendState(BackendPhase.crashed, error: e.toString());
    }
  }

  Future<String> _spawn() async {
    final exe = binaryPath ?? _defaultBinaryPath();
    if (!File(exe).existsSync()) {
      throw StateError(
          'backend binary not found at $exe (set FORGIFY_BACKEND_URL for dev)');
    }
    // Bind :0 to claim a free port, then release it for the child to take. The TOCTOU
    // window is tiny and local; bind failure on launch is the retry signal.
    // 绑 :0 抢空闲端口,再释放给子进程。TOCTOU 窗口极小且本地;启动 bind 失败即重试信号。
    final socket = await ServerSocket.bind(InternetAddress.loopbackIPv4, 0);
    final port = socket.port;
    await socket.close();

    final env = {'FORGIFY_ADDR': '127.0.0.1:$port'};
    final dir = dataDir;
    if (dir != null) env['FORGIFY_DATA_DIR'] = dir;
    _child = await Process.start(exe, const [], environment: env);
    // Surface the child's stderr to the debug console; supervision (bounded restart) is
    // a packaging-time concern layered on top.
    // 把子进程 stderr 引到调试台;监督(有界重启)是打包期叠加的关注点。
    _child!.stderr
        .transform(const SystemEncoding().decoder)
        .listen((l) => debugPrint('[backend] $l'));
    return 'http://127.0.0.1:$port';
  }

  String _defaultBinaryPath() {
    final dir = File(Platform.resolvedExecutable).parent.path;
    final name = Platform.isWindows ? 'forgify-server.exe' : 'forgify-server';
    return '$dir/$name';
  }

  /// Poll `GET /api/v1/health` (the workspace-exempt liveness probe) until it answers
  /// 200, with a bounded number of attempts.
  ///
  /// 轮询 `GET /api/v1/health`(豁免 workspace 的 liveness),有界次数,至答 200。
  Future<void> _awaitHealth(String baseUrl) async {
    const maxAttempts = 100; // ~20s at 200ms
    for (var i = 0; i < maxAttempts; i++) {
      try {
        final r = await _probe.get<dynamic>(
          '$baseUrl/api/v1/health',
          options: Options(
            sendTimeout: const Duration(seconds: 2),
            receiveTimeout: const Duration(seconds: 2),
            validateStatus: (s) => s == 200,
          ),
        );
        if (r.statusCode == 200) return;
      } catch (_) {
        // not up yet
      }
      await Future<void>.delayed(const Duration(milliseconds: 200));
    }
    throw StateError('backend did not become healthy at $baseUrl');
  }

  Future<void> stop() async {
    // Backend does an ordered graceful shutdown on SIGTERM (+ boot reconciliation), so a
    // hard signal is survivable.
    // 后端收 SIGTERM 走有序优雅关停(+ boot 对账),硬信号可承。
    _child?.kill(ProcessSignal.sigterm);
    _child = null;
  }
}
