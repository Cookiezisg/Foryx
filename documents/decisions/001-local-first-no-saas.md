---
id: ADR-001
title: Local-first, no SaaS
status: accepted
date: 2026-04-22
supersedes:
superseded-by:
---

# ADR-001: Local-first, no SaaS

## Status

accepted — 2026-04-22

## Context

Forgify is an agentic workflow platform. The build-or-SaaS fork was the first major decision: build a hosted multi-tenant service, or a local desktop app. SaaS would mean auth, billing, multi-tenancy, data residency concerns, and infrastructure ops from day one.

## Decision

Local-first desktop app (Wails). Single user, single machine. No SaaS, no multi-tenancy.

## Rejected Alternatives

| Alternative | Reason Rejected |
|---|---|
| SaaS / cloud-hosted | Auth, billing, infra, compliance overhead for a one-person project before product-market fit. Agents touching local filesystem makes remote execution dangerous and slow. |
| Electron with Node backend | Go ecosystem (type safety, performance, single binary) preferred; Wails provides equivalent native shell with Go backend. |

## Consequences

**Positive:**
- Zero auth complexity (single hardcoded `local-user`; no tokens, sessions, or RBAC)
- Agents can safely touch local filesystem, run local processes
- No infra costs during development
- Single binary distribution

**Negative / Trade-offs:**
- No collaboration features
- Smaller addressable market
- Future SaaS migration would require non-trivial auth layer

**Neutral:**
- All SSE subscriptions are per-user (only one user exists, simplifies design)
