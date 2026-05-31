---
id: HOW-001
type: how-to
status: active
owner: @weilin
created: 2026-05-31
reviewed: 2026-05-31
review-due: 2026-11-30
audience: [human, ai]
---

# How to Add a New Domain

**Pre-condition:** You have a domain name (e.g., `widget`) and a clear idea of its entities and operations.

## Step 1: End-to-end Walkthrough

Before writing code, complete the end-to-end template (CLAUDE.md "端到端推演模板"):

```
触发源 → transport handler → app service → infra/store/domain → DB
```

List all cross-domain dependencies. Do not proceed until this is complete.

## Step 2: Domain Layer

Create `backend/internal/domain/widget/`:
- `widget.go` — Entity, Repository interface, Service interface, errors, sentinel values
- `providers.go` if the domain has a provider whitelist

Package name: `widget`. Imports: stdlib + domain types only (no app/infra).

## Step 3: Infra Layer

Create `backend/internal/infra/store/widget/`:
- `widget.go` — Repository implementation (GORM)

Package alias when imported: `widgetstore`.

## Step 4: App Layer

Create `backend/internal/app/widget/`:
- `widget.go` — Service implementation

Package alias when imported: `widgetapp`.

## Step 5: Transport Layer

Create `backend/internal/transport/httpapi/handlers/widget.go`:
- Register routes in `Register(mux, deps)` pattern
- Each handler: decode → call service → write envelope
- Register all sentinels in `errmap.go::errTable`

## Step 6: Wire in main.go

Add store → service → handler chain following existing patterns in `backend/cmd/desktop/main.go`.

## Step 7: Documentation

- Create `documents/references/backend/domains/widget.md` with frontmatter
- Update `documents/references/backend/api.md` with new endpoints
- Update `documents/references/backend/database.md` with new tables
- Update `documents/references/changelog.md` with dev log
- Write ADR in `documents/decisions/` if any significant design decision was made

## Step 8: Tests

- Unit tests: `backend/internal/app/widget/widget_test.go`
- Pipeline test: `backend/test/api/widget/widget_pipeline_test.go`
- Add `// covers: METHOD /path` annotations

## Verification

```bash
make unit
make lint-docs
cd backend && staticcheck ./...
```
