---
id: HOW-003
type: how-to
status: active
owner: @weilin
created: 2026-05-31
reviewed: 2026-05-31
review-due: 2026-11-30
audience: [human, ai]
---

# How to Debug SSE Streams

## Monitor a stream live

```bash
# Event log (agent conversation stream)
curl -N -H "X-User-ID: local-user" http://localhost:8080/api/v1/eventlog

# Notifications (entity change events)
curl -N -H "X-User-ID: local-user" http://localhost:8080/api/v1/notifications

# Forge stream (function/handler/workflow generation)
curl -N -H "X-User-ID: local-user" http://localhost:8080/api/v1/forge
```

## Check sequence gaps

SSE messages carry an `id:` field with sequence numbers. Gaps indicate dropped events. On reconnect, the client sends `Last-Event-ID`; if the server's buffer no longer holds that sequence, it returns `410 SEQ_TOO_OLD`.

## Check buffer overflow

If you see `410` responses, the event buffer was exceeded. Default buffer is 512 events per stream per user. Investigate the emission rate or increase `SSEBufferSize` in config.

## Testend stream view

Start testend (`make testend`) then open `http://localhost:5173`. The SSE tab shows all three streams in real time with parsed event types and payloads.

## Event protocol reference

- Event log protocol: `documents/references/backend/events.md`
- SSE cap rule (3 streams only): `documents/decisions/005-three-sse-streams-cap.md`
