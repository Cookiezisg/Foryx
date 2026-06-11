# Forgify Documentation Index

> AI session entry point. Read this first, then follow links.

## What Are You Looking For?

| Question | Go here |
|---|---|
| System architecture, phase roadmap, vision | `concepts/architecture.md` |
| Engineering rules + work discipline (S/T/N/D/E series) | `../CLAUDE.md` |
| Doc governance + directory spec | `GOVERNANCE.md` |

## Structure — skeleton restored, content pending (V0.2 → V-next)

The tree below is the canonical organization (per `GOVERNANCE.md`). The reference / decision /
how-to / working / archive / superpowers folders are **empty placeholders** (`.gitkeep`, each
stating what it holds) — content is regenerated against the new structure as the rewrite covers
back and the frontend is rebuilt.

```
docs/
├── INDEX.md          ← this file (AI entry point)
├── GOVERNANCE.md     ← doc types, frontmatter, directory spec
├── concepts/         ← architecture.md (only surviving content doc)
├── references/       ← must-stay-in-sync-with-code specs (empty)
│   ├── backend/      ← api / database / events / error-codes / changelog + domains/
│   └── frontend/     ← fsd-layers / entity-types / cross-cutting + slices/
├── decisions/        ← ADRs (empty)
├── how-to/           ← operational playbooks (empty)
├── working/          ← in-progress research, 90-day max (empty)
├── archive/          ← read-only graveyard (empty)
└── superpowers/      ← Superpowers skill artifacts: plans/ + specs/ (empty)
```

**Previous version's complete docs** are on the **`version-0.2`** git branch —
`git checkout version-0.2 -- docs/...` recovers any of them.

## Authority Hierarchy

`CLAUDE.md` > `references/` > `concepts/` > `working/` > `archive/`
