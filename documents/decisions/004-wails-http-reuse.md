---
id: ADR-004
title: Wails shell, reuse HTTP API (no native binding)
status: accepted
date: 2026-05-12
supersedes:
superseded-by:
---

# ADR-004: Wails shell, reuse HTTP API (no native binding)

## Status

accepted — 2026-05-12

## Context

Wails offers native Go↔JS bindings as its primary feature: Go functions callable directly from the frontend without HTTP. This is the "Wails way." However, Forgify had an existing full HTTP API.

## Decision

Use Wails only as a native window shell. The frontend communicates with the backend exclusively via the existing HTTP API (localhost). No Wails native bindings used.

## Rejected Alternatives

| Alternative | Reason Rejected |
|---|---|
| Wails native bindings | Would require maintaining two integration surfaces (HTTP for testend/dev + native for prod); SSE streaming is awkward via native bindings |
| Electron | Node.js backend; Go single-binary preferred |
| Tauri (Rust) | Rust backend would require rewrite |

## Consequences

**Positive:**
- Testend and `curl` work the same in desktop mode
- No Wails-specific abstraction layer to maintain
- `wails dev` just starts the existing HTTP server + Wails window

**Negative / Trade-offs:**
- Localhost HTTP has ~0.1ms overhead vs native IPC (irrelevant in practice)
- Wails native binding feature unused
