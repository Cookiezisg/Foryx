---
id: ADR-002
title: Clean Architecture 4 layers
status: accepted
date: 2026-04-22
supersedes:
superseded-by:
---

# ADR-002: Clean Architecture 4 layers

## Status

accepted — 2026-04-22

## Context

The pre-rewrite backend had handlers writing SQL directly, a 696-line god `ToolService`, and mixed responsibilities everywhere. A new architecture was needed before Phase 2+.

## Decision

Strict 4-layer Clean Architecture:

```
transport → app → (domain ∪ infra/store) → infra/db
```

Dependency direction is strictly bottom-up. Lower layers never import upper layers. Domain layer defines interfaces (ports); infra implements them.

## Rejected Alternatives

| Alternative | Reason Rejected |
|---|---|
| Flat package structure | Previous state — caused the god object problem |
| 3-layer (no domain) | Domain layer needed to own business invariants and port interfaces independently of infra |
| Hexagonal (many adapters) | Overkill for local single-user app; 4 layers sufficient |

## Consequences

**Positive:**
- Clear testability: domain and app layers testable without HTTP or DB
- Enforced via `staticcheck` + package naming conventions (S12/S13)
- New domain can be added by following the pattern

**Negative / Trade-offs:**
- More boilerplate (interface per domain)
- Package naming aliases required everywhere (`apikeyapp`, `apikeystore`, etc.)
