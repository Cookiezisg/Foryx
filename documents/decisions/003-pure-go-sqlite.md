---
id: ADR-003
title: Pure Go SQLite (modernc)
status: accepted
date: 2026-04-23
supersedes:
superseded-by:
---

# ADR-003: Pure Go SQLite (modernc)

## Status

accepted — 2026-04-23

## Context

SQLite is the right database for a local-first single-user app. The standard `mattn/go-sqlite3` requires CGO, which complicates cross-platform builds significantly (different toolchains for Windows/Linux/Mac).

## Decision

Use `modernc.org/sqlite` — a pure Go SQLite port with no CGO. DSN uses `_pragma=...` syntax instead of the standard `?_fk=on` form.

## Rejected Alternatives

| Alternative | Reason Rejected |
|---|---|
| mattn/go-sqlite3 | Requires CGO; cross-platform builds require separate toolchains |
| PostgreSQL | Overkill for local-first; requires external process |
| BoltDB/BadgerDB | No SQL; schema migrations would be manual |

## Consequences

**Positive:**
- `GOOS=windows go build ./...` works in one command, no cross-compile toolchain
- Pure Go, easier to embed in Wails

**Negative / Trade-offs:**
- DSN syntax differs (`_pragma=foreign_keys=on` instead of `?_fk=on`)
- Slightly slower than CGO SQLite in benchmarks (irrelevant for single-user)
