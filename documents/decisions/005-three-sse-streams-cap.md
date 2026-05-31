---
id: ADR-005
title: Three SSE streams, permanent cap
status: accepted
date: 2026-05-12
supersedes:
superseded-by:
---

# ADR-005: Three SSE streams, permanent cap

## Status

accepted — 2026-05-12

## Context

SSE connection count proliferates as features grow. Early design had many per-resource streams; each new feature added a new endpoint. This led to connection limit issues and unpredictable reconnect behavior.

## Decision

Exactly three SSE streams, never more (E1 standard):

1. **Event log** `GET /api/v1/eventlog` — 5 events × 7 block types; agent conversation stream
2. **Notifications** `GET /api/v1/notifications` — global entity change notifications; open vocabulary
3. **Forge stream** `GET /api/v1/forge` — 4 events × 3 kinds (function/handler/workflow); closed enum

All three are per-`user_id`. New features must fit into these three streams. Adding a fourth stream requires a new ADR.

## Rejected Alternatives

| Alternative | Reason Rejected |
|---|---|
| Per-resource SSE | Connection count unbounded; each conversation/flowrun would open its own stream |
| WebSocket | Heavier protocol; SSE sufficient for server-push-only patterns |
| Polling | Higher latency, more server load |

## Consequences

**Positive:**
- Predictable connection count (always exactly 3 per connected client)
- Forced discipline: new features must use existing streams
- Simpler client-side reconnect logic

**Negative / Trade-offs:**
- Notification fan-out requires client-side filtering
- Adding genuinely new stream types requires ADR + deliberate design
