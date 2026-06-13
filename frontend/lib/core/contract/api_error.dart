/// The client-side projection of the backend's N1 error envelope
/// (`{"error":{code,message,details}}`) plus the HTTP status it arrived with. Every
/// failed request surfaces as one of these (the Dio ErrorInterceptor builds it), so
/// features branch on a single typed error — never on raw Dio/HTTP plumbing.
///
/// 后端 N1 错误信封的客户端投影 + 到达时的 HTTP status。每个失败请求都化为它(Dio
/// ErrorInterceptor 构造),故 feature 只 branch 一种 typed 错误、不碰裸 Dio/HTTP。
class ApiException implements Exception {
  const ApiException({
    required this.code,
    required this.message,
    required this.httpStatus,
    this.details,
  });

  /// Stable `<ENTITY>_<REASON>` wire code (globally unique, ~256 registered). The UI
  /// branches on the handful in [ForgifyErr]; the rest are shown via [message].
  ///
  /// 稳定 `<ENTITY>_<REASON>` wire code(全局唯一,~256 登记)。UI 只 branch [ForgifyErr]
  /// 里的少数,其余经 [message] 呈现。
  final String code;

  /// Human/LLM-facing message from the backend (already localized-neutral English).
  ///
  /// 后端给的人/LLM 面向消息(已是中性英文)。
  final String message;

  /// HTTP status the error arrived with (derived from the backend Kind). 0 = transport
  /// failure before any HTTP response (connection refused, timeout, backend down).
  ///
  /// 错误到达时的 HTTP status(由后端 Kind 派生)。0 = 任何 HTTP 响应前的传输失败
  /// (连接拒绝、超时、后端未起)。
  final int httpStatus;

  /// Optional structured detail payload (`details` in the envelope).
  ///
  /// 可选结构化细节(信封里的 `details`)。
  final Object? details;

  bool get isTransport => httpStatus == 0;
  bool get isNotFound => httpStatus == 404;
  bool get isConflict => httpStatus == 409;
  bool get isUnauthorized => httpStatus == 401;
  bool get isGone => httpStatus == 410;

  /// Build from a decoded N1 error body `{code,message,details}` + the status.
  ///
  /// 从解码后的 N1 错误体 `{code,message,details}` + status 构造。
  factory ApiException.fromEnvelope(Map<String, dynamic>? error, int httpStatus) {
    final e = error ?? const {};
    return ApiException(
      code: e['code'] as String? ?? ForgifyErr.unknown,
      message: e['message'] as String? ?? 'request failed',
      httpStatus: httpStatus,
      details: e['details'],
    );
  }

  /// A transport-level failure with no HTTP envelope (backend unreachable).
  ///
  /// 无 HTTP 信封的传输级失败(后端不可达)。
  factory ApiException.transport(String message) =>
      ApiException(code: ForgifyErr.transport, message: message, httpStatus: 0);

  @override
  String toString() => 'ApiException($code, http=$httpStatus): $message';
}

/// The small set of wire codes the UI actually branches on (vs. just displaying the
/// message). The full ~256-code catalog is generated from `error-codes.md` in a
/// follow-up build step; this is the hand-curated subset that drives UX flows.
///
/// UI 真正 branch 的少数 wire code(其余仅显示消息)。全 ~256 码目录后续由 `error-codes.md`
/// 生成;此为驱动 UX 流程的手挑子集。
abstract final class ForgifyErr {
  /// Client-synthesized: transport failure before any response.
  static const transport = 'CLIENT_TRANSPORT';

  /// Client-synthesized: a code field was absent.
  static const unknown = 'CLIENT_UNKNOWN';

  /// 401 — isolated route reached without a valid workspace; clear ws + reselect.
  static const unauthNoWorkspace = 'UNAUTH_NO_WORKSPACE';

  /// 409 — this conversation already has an assistant turn running; disable send.
  static const streamInProgress = 'STREAM_IN_PROGRESS';

  /// 409 — a reindex is already running; surface a toast, keep the button disabled.
  static const searchReindexRunning = 'SEARCH_REINDEX_RUNNING';

  /// 422 — approval decision lost the first-wins race (already decided / timed out).
  static const approvalAlreadyDecided = 'APPROVAL_ALREADY_DECIDED';
}
