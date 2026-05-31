---
id: HOW-002
type: how-to
status: active
owner: @weilin
created: 2026-05-31
reviewed: 2026-05-31
review-due: 2026-11-30
audience: [human, ai]
---

# Release Checklist

## Pre-release Gates

- [ ] `make verify` green (vet×5 + build×5 + lintprompts + audit + lint-docs + mock)
- [ ] `make e2e` green (mock + sandbox + live)
- [ ] `make doc-matrix` — no STALE domains
- [ ] All `working/` docs either have `landed-into` filled or `review-due` in future
- [ ] `documents/references/changelog.md` up to date

## Build

```bash
make build
# Output: backend/cmd/desktop/build/bin/Forgify.app
```

## Tag and Push

```bash
git tag v1.2.X
git push origin v1.2.X
```

## Post-release

- [ ] Update phase table in `documents/concepts/architecture.md`
- [ ] Add release entry to `documents/references/changelog.md`
