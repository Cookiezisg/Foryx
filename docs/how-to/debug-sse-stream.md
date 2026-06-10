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

All three subscribe endpoints live on `StreamHandler`, are workspace-scoped, and stream the
complete unfiltered feed (the client filters by conversation/entity).

```bash
# Messages (agent conversation stream — text/reasoning/tool_call/tool_result)
curl -N -H "X-Forgify-Workspace-ID: <wsId>" http://localhost:8080/api/v1/messages/stream

# Notifications (durable inbox — entity change events)
curl -N -H "X-Forgify-Workspace-ID: <wsId>" http://localhost:8080/api/v1/notifications/stream

# Entities (forge pipeline progress)
curl -N -H "X-Forgify-Workspace-ID: <wsId>" http://localhost:8080/api/v1/entities/stream
```

## Check sequence gaps

SSE messages carry an `id:` field with sequence numbers. Gaps indicate dropped events. On reconnect, the client sends `Last-Event-ID`; if the server's buffer no longer holds that sequence, it returns `410 SEQ_TOO_OLD`.

## Check buffer overflow

If you see `410` responses, the resume cursor was evicted from the replay ring. The buffer holds the most recent durable events (`seq > 0`) per stream per **workspace** — currently 256 (`stream.New(bufSize)` in bootstrap). Investigate the emission rate or raise `bufSize`.

## Testend stream view

Start testend (`make testend`) then open `http://localhost:5173`. The SSE tab shows all three streams in real time with parsed event types and payloads.

## Event protocol reference

- Event log protocol: `docs/references/backend/events.md`
- SSE cap rule (3 streams only): `docs/decisions/005-three-sse-streams-cap.md`
