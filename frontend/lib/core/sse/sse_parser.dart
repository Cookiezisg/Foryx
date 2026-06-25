import 'dart:convert';

import 'frame.dart';

/// Incremental parser for the backend's SSE wire format. The server writes each event
/// as:
///
///   event: stream
///   id: SEQ              (only for durable frames, seq greater than 0)
///   data: JSON
///   (blank line ends the event)
///
/// plus `: keep-alive` comment lines every 15s. This parser is fed text lines and emits
/// one [StreamEnvelope] per completed event. It is pure (no IO) so the full reconnect
/// state machine can be unit-tested against recorded line fixtures — the discipline the
/// architecture review flagged as load-bearing.
///
/// 后端 SSE 线缆格式的增量解析器。逐行喂入,每个完整事件吐一个 [StreamEnvelope]。纯函数
/// (无 IO),使整套重连状态机可对录制行 fixture 单测——评审标记为承载性的纪律。
class SseEventParser {
  String _event = '';
  String? _id;
  final StringBuffer _data = StringBuffer();
  bool _sawData = false;

  /// The id of the last DURABLE event seen, for the resume cursor (Last-Event-ID).
  /// Ephemeral events (seq 0) carry no `id:` line and never advance this.
  ///
  /// 最近一个 DURABLE 事件的 id,供续传游标(Last-Event-ID)。ephemeral 事件(seq 0)无
  /// `id:` 行、绝不推进它。
  String? lastEventId;

  /// Feed one raw line (without trailing newline). Returns a parsed envelope when this
  /// line closed an event (a blank line), otherwise null. A decode failure returns null
  /// (a single malformed event must not kill the stream) — callers may observe it via
  /// [onDecodeError].
  ///
  /// 喂一行(无尾换行)。该行闭合了一个事件(空行)时返回解析后信封,否则 null。解码失败返回
  /// null(单个畸形事件不该杀流)——调用方可经 [onDecodeError] 观测。
  StreamEnvelope? addLine(String line, {void Function(Object error)? onDecodeError}) {
    if (line.isEmpty) {
      return _dispatch(onDecodeError);
    }
    if (line.startsWith(':')) {
      return null; // comment / keep-alive
    }
    final colon = line.indexOf(':');
    final String field;
    String value;
    if (colon == -1) {
      field = line;
      value = '';
    } else {
      field = line.substring(0, colon);
      value = line.substring(colon + 1);
      if (value.startsWith(' ')) value = value.substring(1);
    }
    switch (field) {
      case 'event':
        _event = value;
      case 'id':
        _id = value;
      case 'data':
        if (_sawData) _data.write('\n');
        _data.write(value);
        _sawData = true;
      default:
        break; // ignore unknown fields (retry, etc.)
    }
    return null;
  }

  StreamEnvelope? _dispatch(void Function(Object error)? onDecodeError) {
    if (!_sawData) {
      _reset();
      return null;
    }
    final raw = _data.toString();
    final id = _id;
    _reset();
    try {
      final json = jsonDecode(raw) as Map<String, dynamic>;
      final env = StreamEnvelope.fromJson(json);
      if (id != null && env.durable) lastEventId = id;
      return env;
    } catch (e) {
      onDecodeError?.call(e);
      return null;
    }
  }

  void _reset() {
    _event = '';
    _id = null;
    _data.clear();
    _sawData = false;
  }

  // _event is captured for completeness/diagnostics; all our events are `event: stream`.
  // ignore: unused_element
  String get currentEvent => _event;
}
