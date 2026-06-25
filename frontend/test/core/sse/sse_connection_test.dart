import 'dart:async';
import 'dart:convert';
import 'dart:math';
import 'dart:typed_data';

import 'package:anselm/core/sse/frame.dart';
import 'package:anselm/core/sse/sse_connection.dart';
import 'package:dio/dio.dart';
import 'package:flutter_test/flutter_test.dart';

// STEP 3 gate (the reconnect STATE MACHINE — "where bugs hide" per the review). Driven by a
// fake HttpClientAdapter that returns queued SSE stream responses and records each request,
// so we can assert: durable cursor advance + Last-Event-ID on reconnect, the bearer header,
// 410 → resync + cursor drop, and the jittered backoff bounds. Zero-jitter Random keeps the
// reconnect loop instant (no real waits).

class _ZeroRandom implements Random {
  @override
  int nextInt(int max) => 0; // full-jitter sleep collapses to 0 → instant reconnect in test
  @override
  bool nextBool() => false;
  @override
  double nextDouble() => 0;
}

class _MaxRandom implements Random {
  @override
  int nextInt(int max) => max - 1; // top of [0,max) → reveals the backoff base deterministically
  @override
  bool nextBool() => true;
  @override
  double nextDouble() => 0.999;
}

class _SseAdapter implements HttpClientAdapter {
  _SseAdapter(this.responses);
  final List<ResponseBody Function()> responses; // consumed in order; last repeats
  final requests = <RequestOptions>[];
  int _i = 0;

  @override
  Future<ResponseBody> fetch(RequestOptions o, Stream<Uint8List>? rs, Future<void>? cf) async {
    requests.add(o);
    final f = responses[_i < responses.length ? _i : responses.length - 1];
    _i++;
    return f();
  }

  @override
  void close({bool force = false}) {}
}

Uint8List _b(String s) => Uint8List.fromList(utf8.encode(s));

const _hdr = {
  Headers.contentTypeHeader: ['text/event-stream'],
};

/// One durable SSE event (seq>0, carries id:) then clean EOF (stream closes → reconnect).
ResponseBody _durableThenEof(int seq) => ResponseBody(
      Stream.value(_b(
        'event: stream\n'
        'id: $seq\n'
        'data: {"seq":$seq,"scope":{"kind":"conversation","id":"c1"},"id":"b1","frame":{"kind":"signal","node":{"type":"x"}}}\n'
        '\n',
      )),
      200,
      headers: _hdr,
    );

/// A connection that never emits and never closes (parks the loop in `live`).
ResponseBody _hang() => ResponseBody(StreamController<Uint8List>().stream, 200, headers: _hdr);

/// 410 Gone — the resume seq was evicted (validateStatus lets it through as a value).
ResponseBody _gone() => ResponseBody(Stream<Uint8List>.empty(), 410);

SseConnection _conn(_SseAdapter adapter, {String? token = 'tok'}) {
  final dio = Dio(BaseOptions(baseUrl: 'http://127.0.0.1:9/'))..httpClientAdapter = adapter;
  return SseConnection(
    streamPath: '/api/v1/messages/stream',
    baseUrl: 'http://127.0.0.1:9/',
    workspaceId: () => 'ws_1',
    authToken: () => token,
    dio: dio,
    random: _ZeroRandom(),
  );
}

Future<void> _until(bool Function() cond, {Duration timeout = const Duration(seconds: 3)}) async {
  final deadline = DateTime.now().add(timeout);
  while (!cond()) {
    if (DateTime.now().isAfter(deadline)) {
      throw TimeoutException('condition not met in time');
    }
    await Future<void>.delayed(const Duration(milliseconds: 5));
  }
}

void main() {
  test('durable frame emits, advances cursor, reconnect carries Last-Event-ID + bearer', () async {
    final adapter = _SseAdapter([() => _durableThenEof(1), _hang]);
    final conn = _conn(adapter);
    final got = <StreamEnvelope>[];
    conn.envelopes.listen(got.add);

    conn.start();
    await _until(() => got.isNotEmpty);
    expect(got.first.seq, 1);
    expect(got.first.durable, isTrue);

    // first request carried the bearer; the reconnect (req #2) carried the resume cursor.
    await _until(() => adapter.requests.length >= 2);
    expect(adapter.requests[0].headers['Authorization'], 'Bearer tok');
    expect(adapter.requests[0].queryParameters['workspaceID'], 'ws_1');
    expect(adapter.requests[1].headers['Last-Event-ID'], '1');

    await conn.stop();
  });

  test('410 → resync fires, cursor drops, next connect has NO Last-Event-ID', () async {
    final adapter = _SseAdapter([() => _durableThenEof(1), _gone, _hang]);
    final conn = _conn(adapter);
    final got = <StreamEnvelope>[];
    var resynced = false;
    conn.envelopes.listen(got.add);
    conn.resync.listen((_) => resynced = true);

    conn.start();
    await _until(() => got.isNotEmpty); // cursor advanced to 1
    await _until(() => resynced); // the 410 on reconnect fired a resync
    await _until(() => adapter.requests.length >= 3);
    // after a 410 the connection resumes from a FRESH head — no resume cursor
    expect(adapter.requests[2].headers.containsKey('Last-Event-ID'), isFalse);

    await conn.stop();
  });

  test('no token → Authorization header omitted', () async {
    final adapter = _SseAdapter([_hang]);
    final conn = _conn(adapter, token: null);
    conn.start();
    await _until(() => adapter.requests.isNotEmpty);
    expect(adapter.requests[0].headers.containsKey('Authorization'), isFalse);
    await conn.stop();
  });

  test('backoff: full-jitter base doubles 500→10s cap (max-jitter reveals the base)', () {
    final adapter = _SseAdapter([_hang]);
    final dio = Dio()..httpClientAdapter = adapter;
    final conn = SseConnection(
      streamPath: '/x',
      baseUrl: 'http://x/',
      workspaceId: () => null,
      authToken: () => null,
      dio: dio,
      random: _MaxRandom(), // nextInt(base+1) → base (top of range)
    );
    // full jitter is [0, base] inclusive (nextInt(base+1)); _MaxRandom returns the top = base:
    expect(conn.debugNextBackoff(), 500); // base 500
    expect(conn.debugNextBackoff(), 1000);
    expect(conn.debugNextBackoff(), 2000);
    expect(conn.debugNextBackoff(), 4000);
    expect(conn.debugNextBackoff(), 8000);
    expect(conn.debugNextBackoff(), 10000); // capped
    expect(conn.debugNextBackoff(), 10000); // stays capped
  });
}
