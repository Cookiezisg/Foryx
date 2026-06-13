import 'package:flutter_test/flutter_test.dart';
import 'package:forgify/core/sse/frame.dart';
import 'package:forgify/core/sse/sse_parser.dart';

/// Feed a raw SSE event block (lines joined by \n, terminated by a blank line) through
/// the parser and return the one envelope it produced (or null).
///
/// 把一段原始 SSE 事件块逐行喂入解析器,返回它产出的一个信封(或 null)。
StreamEnvelope? feed(SseEventParser p, String block) {
  StreamEnvelope? out;
  for (final line in block.split('\n')) {
    final env = p.addLine(line);
    if (env != null) out = env;
  }
  return out;
}

void main() {
  group('SseEventParser', () {
    test('parses a durable open frame and advances the resume cursor', () {
      final p = SseEventParser();
      final env = feed(p, [
        'event: stream',
        'id: 5',
        'data: {"seq":5,"scope":{"kind":"conversation","id":"cv_1"},"id":"blk_1","frame":{"kind":"open","node":{"type":"text"}}}',
        '',
      ].join('\n'));

      expect(env, isNotNull);
      expect(env!.seq, 5);
      expect(env.scope.kind, 'conversation');
      expect(env.scope.id, 'cv_1');
      expect(env.scope.key, 'conversation:cv_1');
      expect(env.id, 'blk_1');
      expect(env.durable, isTrue);
      expect(env.frame, isA<FrameOpen>());
      expect((env.frame as FrameOpen).node.type, 'text');
      expect(p.lastEventId, '5'); // durable → cursor advanced
    });

    test('an ephemeral delta (seq 0) does NOT advance the resume cursor', () {
      final p = SseEventParser()..lastEventId = '5';
      final env = feed(p, [
        'event: stream',
        'data: {"seq":0,"scope":{"kind":"conversation","id":"cv_1"},"id":"blk_1","frame":{"kind":"delta","chunk":"hel"}}',
        '',
      ].join('\n'));

      expect(env, isNotNull);
      expect(env!.durable, isFalse);
      expect(env.frame, isA<FrameDelta>());
      expect((env.frame as FrameDelta).chunk, 'hel');
      expect(p.lastEventId, '5'); // ephemeral → unchanged
    });

    test('parses a close frame carrying a result snapshot', () {
      final p = SseEventParser();
      final env = feed(p, [
        'event: stream',
        'id: 7',
        'data: {"seq":7,"scope":{"kind":"conversation","id":"cv_1"},"id":"blk_1","frame":{"kind":"close","status":"completed","result":{"type":"text","content":{"content":"hello"}}}}',
        '',
      ].join('\n'));

      final frame = env!.frame as FrameClose;
      expect(frame.status, 'completed');
      expect(frame.result, isNotNull);
      expect(frame.result!.type, 'text');
      expect(frame.result!.content!['content'], 'hello');
    });

    test('parses a durable notification signal', () {
      final p = SseEventParser();
      final env = feed(p, [
        'event: stream',
        'id: 12',
        'data: {"seq":12,"scope":{"kind":"notification","id":"noti_1"},"id":"sig_1","frame":{"kind":"signal","node":{"type":"function.created","content":{"id":"fn_1"}}}}',
        '',
      ].join('\n'));

      expect(env!.durable, isTrue);
      expect(env.frame, isA<FrameSignal>());
      final sig = env.frame as FrameSignal;
      expect(sig.node.type, 'function.created');
      expect(sig.node.content!['id'], 'fn_1');
    });

    test('ignores keep-alive comment lines', () {
      final p = SseEventParser();
      expect(p.addLine(': keep-alive'), isNull);
      expect(p.addLine(''), isNull); // blank with no data → no event
    });

    test('an unknown frame verb degrades instead of throwing', () {
      final p = SseEventParser();
      final env = feed(p, [
        'data: {"seq":1,"scope":{"kind":"agent","id":"ag_1"},"id":"x","frame":{"kind":"teleport"}}',
        '',
      ].join('\n'));

      expect(env, isNotNull);
      expect(env!.frame, isA<FrameSignal>());
      expect((env.frame as FrameSignal).node.type, 'unknown:teleport');
    });

    test('a malformed data payload is dropped, not thrown', () {
      final p = SseEventParser();
      Object? captured;
      final env = feed2(p, 'data: {not json', onError: (e) => captured = e);
      expect(env, isNull);
      expect(captured, isNotNull);
    });
  });
}

StreamEnvelope? feed2(SseEventParser p, String block,
    {void Function(Object)? onError}) {
  StreamEnvelope? out;
  for (final line in '$block\n'.split('\n')) {
    final env = p.addLine(line, onDecodeError: onError);
    if (env != null) out = env;
  }
  return out;
}
