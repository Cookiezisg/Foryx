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

1. **Messages** `GET /api/v1/messages/stream` — agent conversation stream (text/reasoning/tool_call/tool_result block lifecycle)
2. **Notifications** `GET /api/v1/notifications/stream` — global entity change notifications; open vocabulary
3. **Entities** `GET /api/v1/entities/stream` — forge pipeline progress (fn/hd/wf/ag/document/skill); closed enum

All three are per-`workspace`. New features must fit into these three streams. Adding a fourth stream requires a new ADR.

> **Rename (2026-06-03, names only — the 3-stream cap stands)**: `eventlog → messages`, `forge → entities`, `notifications` unchanged; `user_id → workspace` (R0018). Subscribe endpoints unified under `StreamHandler` at `/api/v1/{messages,entities,notifications}/stream` (workspace-scoped, unfiltered, `Last-Event-ID` resume, 410 on evicted cursor). Original bullets above updated to the as-built names/endpoints.

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
