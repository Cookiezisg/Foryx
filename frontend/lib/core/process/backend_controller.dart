import 'dart:async';
import 'dart:convert';
import 'dart:io';
import 'dart:math';

import 'package:dio/dio.dart';
import 'package:flutter/foundation.dart';

/// Where the local Go backend is in its lifecycle. The whole app gates on this: a single
/// banner / splash reads it, NOT per-feature error handling.
///
/// 本地 Go 后端处于生命周期何处。整个 app 据此门控:单一横幅/启动屏读它,非逐 feature 处理。
enum BackendPhase { starting, ready, crashed }

/// The supervised backend's snapshot: the base URL + the per-launch bearer token the rest of
/// the app needs to talk to it (both null until ready; token also null on dev-attach).
///
/// 受管后端快照:base URL + 余下全 app 对接所需的每次启动 bearer token(就绪前均 null;dev-attach
/// 时 token 亦 null)。
@immutable
class BackendState {
  const BackendState(this.phase, {this.baseUrl, this.authToken, this.error});
  final BackendPhase phase;
  final String? baseUrl;
  final String? authToken;
  final String? error;

  bool get isReady => phase == BackendPhase.ready && baseUrl != null;
}

/// Injectable child-process launcher (default [Process.start]) so the supervisor is testable
/// against a fake process without a real binary. 可注入的子进程启动器(默认 Process.start),便于假进程测试。
typedef ProcessLauncher = Future<Process> Function(
  String executable,
  List<String> arguments, {
  Map<String, String>? environment,
});

/// Owns the Go backend as a child process (the sidecar model, ADR 0004 §1) and SUPERVISES it:
/// free-port pre-grab → spawn with env (incl. the per-launch loopback token) → health-gate →
/// bounded crash-restart → graceful shutdown. The backend is a standalone localhost HTTP+SSE
/// server reading `ANSELM_ADDR` / `ANSELM_DATA_DIR` / `ANSELM_AUTH_TOKEN` and serving
/// `GET /api/v1/health`.
///
/// Resolution order:
///  1. `ANSELM_BACKEND_URL` — dev escape hatch: attach to an already-running backend
///     (`make server`), spawn nothing; no per-launch token (the dev backend has none).
///  2. spawn the bundled binary: pre-grab a free loopback port, mint a random
///     `ANSELM_AUTH_TOKEN`, pass both via env, then poll `/api/v1/health` (WITH the token —
///     the backend requires it under loopback hardening) until 200.
///
/// 以 sidecar 模型托管并**监督** Go 后端:预抢端口→带 env(含每次启动 token)spawn→健康门控→
/// 有界崩溃重启→优雅关停。解析序:① `ANSELM_BACKEND_URL` dev 逃生口(挂已跑后端、不 spawn、无 token);
/// ② spawn 内置二进制(抢端口 + 铸随机 `ANSELM_AUTH_TOKEN`,带 token 轮询 /health 至 200)。
class BackendController {
  BackendController({
    this.binaryPath,
    this.dataDir,
    Dio? probe,
    ProcessLauncher? launcher,
    String? Function()? externalUrl,
    String Function()? tokenGen,
    Duration probeInterval = const Duration(milliseconds: 200),
    int maxHealthAttempts = 100,
    int maxRestarts = 5,
    Duration restartWindow = const Duration(seconds: 60),
    Duration restartBackoffBase = const Duration(milliseconds: 300),
    Duration shutdownGrace = const Duration(seconds: 8),
    Random? random,
  })  : _probe = probe ?? Dio(),
        _launch = launcher ??
            ((exe, args, {environment}) =>
                Process.start(exe, args, environment: environment)),
        _externalUrl =
            externalUrl ?? (() => Platform.environment['ANSELM_BACKEND_URL']),
        _tokenGen = tokenGen ?? (() => _mintToken(random ?? Random.secure())),
        _probeInterval = probeInterval,
        _maxHealthAttempts = maxHealthAttempts,
        _maxRestarts = maxRestarts,
        _restartWindow = restartWindow,
        _restartBackoffBase = restartBackoffBase,
        _shutdownGrace = shutdownGrace;

  /// Absolute path to the bundled `server` binary; null = resolve next to the app
  /// executable. Ignored when `ANSELM_BACKEND_URL` is set. 内置二进制绝对路径(null=app 旁解析)。
  final String? binaryPath;
  final String? dataDir;
  final Dio _probe;
  final ProcessLauncher _launch;
  final String? Function() _externalUrl;
  final String Function() _tokenGen;
  final Duration _probeInterval;
  final int _maxHealthAttempts;
  final int _maxRestarts;
  final Duration _restartWindow;
  final Duration _restartBackoffBase;
  final Duration _shutdownGrace;

  final state =
      ValueNotifier<BackendState>(const BackendState(BackendPhase.starting));

  Process? _child;
  bool _stopped = false;
  String? _authToken; // null on dev-attach
  String? _baseUrl;
  final _restarts = <DateTime>[]; // sliding window of restart timestamps

  /// Launch + health-gate. On any failure the state goes [BackendPhase.crashed] (never throws
  /// to the caller — the UI gates on [state]). 启动 + 健康门控;失败转 crashed、绝不抛(UI 据 state 门控)。
  Future<void> start() async {
    _stopped = false;
    try {
      final external = _externalUrl();
      if (external != null && external.isNotEmpty) {
        _authToken = null; // dev backend has no per-launch token
        _baseUrl = external;
        await _awaitHealth(external, token: null);
        _setReady();
        return;
      }
      await _spawnAndGate();
    } catch (e) {
      state.value = BackendState(BackendPhase.crashed, error: e.toString());
    }
  }

  Future<void> _spawnAndGate() async {
    _authToken = _tokenGen();
    _baseUrl = await _spawn(_authToken!);
    await _awaitHealth(_baseUrl!, token: _authToken);
    _setReady();
    _watchExit(_child!);
  }

  void _setReady() => state.value =
      BackendState(BackendPhase.ready, baseUrl: _baseUrl, authToken: _authToken);

  Future<String> _spawn(String token) async {
    final exe = binaryPath ?? _defaultBinaryPath();
    if (!File(exe).existsSync()) {
      throw StateError(
          'backend binary not found at $exe (set ANSELM_BACKEND_URL for dev)');
    }
    // Bind :0 to claim a free loopback port, then release it for the child. The TOCTOU window
    // is tiny + local; a bind clash on launch is the (rare) retry signal.
    // 绑 :0 抢空闲 loopback 端口再释放给子进程;TOCTOU 窗口极小且本地。
    final socket = await ServerSocket.bind(InternetAddress.loopbackIPv4, 0);
    final port = socket.port;
    await socket.close();

    final env = {
      'ANSELM_ADDR': '127.0.0.1:$port',
      'ANSELM_AUTH_TOKEN': token,
    };
    if (dataDir != null) env['ANSELM_DATA_DIR'] = dataDir!;
    final child = await _launch(exe, const [], environment: env);
    _child = child;
    // Drain stderr into the debug console (an undrained pipe can deadlock the child).
    // 排空 stderr 到调试台(未排空的管道会让子进程死锁)。
    child.stderr
        .transform(const SystemEncoding().decoder)
        .listen((l) => debugPrint('[backend] $l'));
    return 'http://127.0.0.1:$port';
  }

  /// Watch for an UNEXPECTED child exit (not via [stop]) and restart with a bounded,
  /// circuit-broken backoff. 监视非 stop 的子进程退出,有界 + 熔断退避重启。
  void _watchExit(Process child) {
    unawaited(child.exitCode.then((code) {
      if (_stopped || !identical(_child, child)) return;
      debugPrint('[backend] exited unexpectedly (code $code) — restarting');
      unawaited(_restart(code));
    }));
  }

  Future<void> _restart(int code) async {
    final now = DateTime.now();
    _restarts.removeWhere((t) => now.difference(t) > _restartWindow);
    if (_restarts.length >= _maxRestarts) {
      state.value = BackendState(
        BackendPhase.crashed,
        error:
            'backend crashed $_maxRestarts× within ${_restartWindow.inSeconds}s (last code $code) — giving up',
      );
      return;
    }
    _restarts.add(now);
    state.value = const BackendState(BackendPhase.starting);
    await Future<void>.delayed(_restartBackoffBase * _restarts.length);
    if (_stopped) return;
    try {
      await _spawnAndGate();
    } catch (e) {
      state.value = BackendState(BackendPhase.crashed, error: e.toString());
    }
  }

  String _defaultBinaryPath() {
    final dir = File(Platform.resolvedExecutable).parent.path;
    final name = Platform.isWindows ? 'anselm-server.exe' : 'anselm-server';
    return '$dir/$name';
  }

  /// Poll `GET /api/v1/health` (sending the bearer token, which the backend now requires) until
  /// 200, bounded. 轮询带 token 的 /health 至 200(有界)。
  Future<void> _awaitHealth(String baseUrl, {String? token}) async {
    for (var i = 0; i < _maxHealthAttempts; i++) {
      if (_stopped) return;
      try {
        final r = await _probe.get<dynamic>(
          '$baseUrl/api/v1/health',
          options: Options(
            sendTimeout: const Duration(seconds: 2),
            receiveTimeout: const Duration(seconds: 2),
            headers: {if (token != null) 'Authorization': 'Bearer $token'},
            validateStatus: (s) => s == 200,
          ),
        );
        if (r.statusCode == 200) return;
      } catch (_) {
        // not up yet
      }
      await Future<void>.delayed(_probeInterval);
    }
    throw StateError('backend did not become healthy at $baseUrl');
  }

  /// Graceful shutdown tied to app termination: SIGTERM (the backend does an ordered shutdown),
  /// wait out a grace window, then SIGKILL if it overstays. 优雅关停:SIGTERM→宽限→逾时 SIGKILL。
  Future<void> stop() async {
    _stopped = true;
    final child = _child;
    _child = null;
    if (child == null) return;
    child.kill(ProcessSignal.sigterm);
    try {
      await child.exitCode.timeout(_shutdownGrace);
    } on TimeoutException {
      child.kill(ProcessSignal.sigkill);
    }
  }

  void dispose() {
    state.dispose();
  }

  /// 32 random bytes, base64url — the per-launch loopback bearer token. 每次启动的 loopback token。
  static String _mintToken(Random rng) {
    final bytes = List<int>.generate(32, (_) => rng.nextInt(256));
    return base64Url.encode(bytes);
  }
}
