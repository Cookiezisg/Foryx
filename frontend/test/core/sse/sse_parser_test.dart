import 'package:anselm/core/sse/frame.dart';
import 'package:anselm/core/sse/sse_parser.dart';
import 'package:flutter_test/flutter_test.dart';

// STEP 3 gate (the load-bearing pure unit) — the SSE wire parser. Recorded line fixtures
// matching the backend wire (`event: stream` / `id: <seq>` / `data: <json>` / blank), incl.
// the space-after-colon, comments, multi-data, and the seq-derived durability cursor.

/// Feed lines, return the last non-null envelope dispatched.
StreamEnvelope? _feed(SseEventParser p, List<String> lines, {void Function(Object)? onErr}) {
  StreamEnvelope? out;
  for (final l in lines) {
    final env = p.addLine(l, onDecodeError: onErr);
    if (env != null) out = env;
  }
  return out;
}

void main() {
  test('durable event: parses + advances Last-Event-ID cursor', () {
    final p = SseEventParser();
    final env = _feed(p, [
      'event: stream',
      'id: 5',
      'data: {"seq":5,"scope":{"kind":"conversation","id":"c1"},"id":"b1","frame":{"kind":"close","status":"completed"}}',
      '',
    ]);
    expect(env, isNotNull);
    expect(env!.seq, 5);
    expect(env.durable, isTrue);
    expect(env.scope.key, 'conversation:c1');
    expect(env.frame, isA<FrameClose>());
    expect(p.lastEventId, '5'); // durable → cursor advanced
  });

  test('ephemeral event (seq 0, no id line): does NOT advance cursor', () {
    final p = SseEventParser();
    final env = _feed(p, [
      'event: stream',
      'data: {"seq":0,"scope":{"kind":"conversation","id":"c1"},"id":"b1","frame":{"kind":"delta","chunk":"hi"}}',
      '',
    ]);
    expect(env!.seq, 0);
    expect(env.durable, isFalse);
    expect((env.frame as FrameDelta).chunk, 'hi');
    expect(p.lastEventId, isNull); // ephemeral never advances the resume cursor
  });

  test('space after colon is stripped (exactly one)', () {
    final p = SseEventParser();
    final env = _feed(p, [
      'data: {"seq":1,"scope":{"kind":"notification"},"id":"","frame":{"kind":"signal","node":{"type":"toast"}}}',
      '',
    ]);
    expect(env, isNotNull); // would fail to JSON-decode if the leading space leaked in
    expect(env!.scope.kind, 'notification');
  });

  test('comment / keep-alive lines are ignored', () {
    final p = SseEventParser();
    expect(p.addLine(': keep-alive'), isNull);
    final env = _feed(p, [
      ': ping',
      'data: {"seq":2,"scope":{"kind":"conversation","id":"c1"},"id":"b1","frame":{"kind":"signal","node":{"type":"x"}}}',
      '',
    ]);
    expect(env, isNotNull);
    expect(env!.seq, 2);
  });

  test('multi-data lines are joined with newline before decode', () {
    final p = SseEventParser();
    // valid JSON split across two data: lines
    final env = _feed(p, [
      'data: {"seq":3,"scope":{"kind":"conversation","id":"c1"},"id":"b1",',
      'data: "frame":{"kind":"delta","chunk":"two-line"}}',
      '',
    ]);
    expect(env, isNotNull);
    expect((env!.frame as FrameDelta).chunk, 'two-line');
  });

  test('malformed JSON → null + onDecodeError (stream survives)', () {
    final p = SseEventParser();
    Object? captured;
    final env = _feed(p, [
      'data: {not valid json',
      '',
    ], onErr: (e) => captured = e);
    expect(env, isNull);
    expect(captured, isNotNull);
    // parser is reset and usable again
    final next = _feed(p, [
      'data: {"seq":7,"scope":{"kind":"conversation","id":"c1"},"id":"b","frame":{"kind":"signal","node":{"type":"x"}}}',
      '',
    ]);
    expect(next!.seq, 7);
  });

  test('blank line with no buffered data does not dispatch', () {
    final p = SseEventParser();
    expect(p.addLine(''), isNull);
    expect(p.addLine(''), isNull);
  });

  group('StreamFrame.fromJson — the closed 4-verb union + forward-compat', () {
    test('open / delta / close / signal', () {
      expect(StreamFrame.fromJson({'kind': 'open', 'node': {'type': 'text'}}), isA<FrameOpen>());
      expect(StreamFrame.fromJson({'kind': 'delta', 'chunk': 'c'}), isA<FrameDelta>());
      expect(StreamFrame.fromJson({'kind': 'close', 'status': 'completed'}), isA<FrameClose>());
      expect(StreamFrame.fromJson({'kind': 'signal', 'node': {'type': 'tick'}}), isA<FrameSignal>());
    });

    test('unknown verb degrades to a no-op signal (does not crash)', () {
      final f = StreamFrame.fromJson({'kind': 'teleport'});
      expect(f, isA<FrameSignal>());
      expect((f as FrameSignal).node.type, 'unknown:teleport');
    });

    test('FrameOpen.parentId nests (E3 subagent tree)', () {
      final f = StreamFrame.fromJson({'kind': 'open', 'parentId': 'tc1', 'node': {'type': 'message'}}) as FrameOpen;
      expect(f.parentId, 'tc1');
    });
  });
}
