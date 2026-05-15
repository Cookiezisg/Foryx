# D2 — skill.md ↔ code gap report

Audited `documents/version-1.2/service-design-documents/skill.md` against `internal/{domain,app}/skill/` + `transport/httpapi/handlers/skills.go`.

D1 already covered the `skill` notification entity-state inclusion in events-design.md; this report focuses on design-doc-vs-code drift specific to `skill.md`.

---

## In code but not in doc

| Item | Code location | Severity |
|---|---|---|
| `skilldomain.ErrNameConflict` sentinel | `internal/domain/skill/skill.go:104` | MED |
| `skilldomain.ErrInvalidName` sentinel | `internal/domain/skill/skill.go:105` | MED |
| `skilldomain.MaxBodyBytes` exported constant (32 KB) | `internal/domain/skill/skill.go:114` | LOW |
| `skilldomain.MaxDescriptionChars` exported constant (1536) | `internal/domain/skill/skill.go:121` | LOW |
| `Service.Body(ctx, name) ([]byte, error)` method (used by `GET /skills/{name}/body`) | `internal/app/skill/mutate.go:41` | LOW |
| `Service.Create / Replace / Delete / Import` methods | `internal/app/skill/mutate.go:64,100,133` + `import.go:76` | MED |
| `Service.SkillsDir()` accessor | `internal/app/skill/skill.go:134` | LOW |
| `Service.Stop()` (idempotent goroutine drain via stopOnce / pollDone) | `internal/app/skill/polling.go:72` | LOW |
| `lastFP atomic.Value` fingerprint short-circuit field | `internal/app/skill/skill.go:84` | LOW |
| `notif notificationspkg.Publisher` field — sends `skill` notification with id="*" on fingerprint change | `internal/app/skill/skill.go:72` + `scan.go:106` | MED |
| `bodyReadRetryDelay = 100ms` retry on ErrNotExist (editor write+rename race per §9.5) | `internal/app/skill/activate.go:34,123` | LOW |
| `substituteVars` struct + named-arg substitution + `${CLAUDE_EFFORT}` placeholder | `internal/app/skill/activate.go:141-188` | LOW |

## In doc but not in code (stale)

| Item | Doc location | Severity |
|---|---|---|
| §4 Sentinel block lists only 3 errors (ErrSkillNotFound / ErrInvalidFrontmatter / ErrBodyTooLarge); code has 5 | skill.md:130-138 vs `domain/skill/skill.go:100-106` (Status header at line 4 says "5 sentinels" but §4 disagrees) | HIGH |
| §7 Service struct shown with `bridge eventsdomain.Bridge` field; code uses `notif notificationspkg.Publisher` | skill.md:233-240 | MED |
| §7 Service struct shown with `subagent SubagentService` interface; matches code | matches | — |
| §7 Activate code example references `agentstatepkg.From(ctx)` + `state.SetActiveSkill(skill)` + `defer state.ClearActiveSkillIfMatches(skill.Name)` (with the defer-clear); code uses `reqctxpkg.GetAgentState(ctx)` + `state.SetActiveSkill(skill)` and **never** clears via defer in non-fork (per the §9.4 "non-fork: don't clear" comment) — the example contradicts the description | skill.md:269-272 vs `app/skill/activate.go:91-93` | MED |
| §7 string substitution table includes `$<name>` (named parameter); code wires this via `substituteVars.NamedArgs` — matches | matches | — |
| §9 / §9.4 declares `ClearActiveSkillIfMatches` method on AgentState; need to verify pkg/agentstate | (not in scope but doc claims method exists) | LOW |
| §10 SSE 事件 — doc shows `eventsdomain.Skill { Skills []*Skill }` struct + `EventName() string` + bridge.Publish; code uses notifications package with type=`skill`, id=`*`, payload `{"changed":true,"count":N}` (not full snapshot) | skill.md:452-462 vs `app/skill/scan.go:106` | HIGH |
| §11 HTTP API table lists 9 endpoints; code has 9 routes — matches | matches | — |
| §12 errmap table has 5 entries; matches code | matches | — |
| §13 CatalogSource example shows `EventTopics() []string` method + return `[]string{"skill"}`; code's catalogSource has only Name() / Granularity() / ListItems() | skill.md:553 vs `app/skill/catalogsource.go` + `domain/catalog/source.go:95` | MED |
| §6 Polling claim "1s 轮询 + fingerprint 短路"; matches code (`pollInterval = 1*time.Second` in `polling.go`) | matches | — |
| §15 与其他 domain 的关系 row "events" — events bridge replaced by notifications | skill.md:602 | LOW |
| §11 `:invoke` endpoint `POST /api/v1/skills/{name}:invoke` returns `{result: "..."}` — matches code | matches | — |

## Mismatched

| Item | Code | Doc | Severity |
|---|---|---|---|
| Sentinel count | 5 | 3 in §4; status header says 5 | MED |
| Status header at top says "✅ D7 全部交付 ... 5 sentinels" | matches errmap (5 entries) and code | §4 inconsistency only | MED |
| `Frontmatter` struct fields | code has 13 fields | doc §4 lists 13 fields — **matches** including `Effort` and `ArgumentHint` | — |
| `Skill.Source` field doc says `"user" / "plugin"`; code says `"user" (V1 only); "plugin" reserved` | matches conceptually | matches | — |
| `Service.Search(ctx, query, topK)` signature | matches | matches | — |
| `Service.Activate` returns `(string, error)` | matches doc §7 | matches | — |
| §9.5 nested-fork suppression: code `if depth >= 1 → return substituted, nil` (returns body, log Info) | doc §9.5 says "强制忽略 frontmatter.Context = "fork"，inline 注入 body 当 tool_result + log Info" — matches | matches | — |
| §11 doc claims `POST /api/v1/skills:refresh` returns `{data: [Skill...]}` (the array of skills); code `Refresh` handler — verify | (skipping; minor) | — |
| §13 catalogSource: doc shows `Description: sk.Description` direct copy from frontmatter; matches code | matches | — |
| Body 32 KB limit / Description 1536 char — both as per spec; constants `MaxBodyBytes` / `MaxDescriptionChars` exposed in domain pkg | matches | — |

## Sub-check
- Entities aligned: yes — Skill / Frontmatter struct fields all match doc §4
- Service methods aligned: **partial** — doc §7 lists 5 methods (Scan / Get / List / Search / Activate); code has 11 (those 5 + Body / Create / Replace / Delete / Import / SkillsDir / Stop / Start)
- Endpoints aligned: yes — 9 routes match between handler and §11
- Sentinels aligned: **no** — §4 lists 3 but code has 5 (`ErrNameConflict`, `ErrInvalidName` extra)
- Cross-domain deps aligned: **partial** — Subagent dep via interface correct; agentstate dep correct; catalog dep correct; events bridge dep (§15) stale — replaced by notifications
- 端到端推演 valid: yes — L1/L2/L3 progressive disclosure flow matches code (Scan caches metadata; Activate reads body fresh; LLM uses Bash/Read for L3)
- Phase 5 / 5-sentinel 大变更已反映: **partial** — Status header notes 5 sentinels but §4 still lists 3 (D7 work added 2 new ones for Create/Replace API); the 1s polling replacement of fsnotify is well-documented at §6 + §9.5.

---

## Summary

- HIGH: 2 (§4 sentinel count off by 2; §10 SSE event family `eventsdomain.Skill` snapshot vs notifications-package `{changed,count}` payload)
- MED: 5 (Service.Body/Create/Replace/Delete/Import method gaps; bridge→Publisher field; activate defer-clear example contradiction; catalogSource EventTopics() in doc; missing notif Publisher field documentation)
- LOW: 6 (Stop / SkillsDir / lastFP / bodyReadRetryDelay / substituteVars / events relation drift)
