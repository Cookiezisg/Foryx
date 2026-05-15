# D2 — catalog.md ↔ code gap report

Audited `documents/version-1.2/service-design-documents/catalog.md` against `internal/{domain,app}/catalog/` + `transport/httpapi/handlers/catalog.go`.

D1 already covered the `catalog` notification entity-state inclusion in events-design.md; this report focuses on design-doc-vs-code drift specific to `catalog.md`.

---

## In code but not in doc

| Item | Code location | Severity |
|---|---|---|
| `catalogdomain.ErrAllSourcesFailed` sentinel — Service.Refresh wraps when every source errors | `internal/domain/catalog/catalog.go:145` | MED |
| `notif notificationspkg.Publisher` field; emits `catalog` notification on every successful Refresh that updates the cache | `internal/app/catalog/catalog.go:87` + `polling.go:253` | HIGH |
| `Service.Stop()` (idempotent goroutine drain via stopOnce / pollDone) | `internal/app/catalog/polling.go:94` | LOW |
| `Service.SetGenerator(g Generator)` post-construction injector | `internal/app/catalog/catalog.go:162` | LOW |
| `Service.SetPollInterval(d)` test injection | `internal/app/catalog/catalog.go:172` | LOW |
| `Service.Get()` returning `*Catalog` for HTTP `GET /catalog` | `internal/app/catalog/catalog.go:213` | LOW |
| Generator interface as a separate top-level type (with nil → mechanical fallback) | `internal/app/catalog/catalog.go:72-74` | LOW |
| `LLMGenerator` constructor takes `picker / keys / factory` directly (not the doc's `llmclient.Resolver`-style); resolves bundle internally each call | `internal/app/catalog/generator.go:71-81` | LOW |
| User-ID injection in Refresh: `if _, ok := reqctxpkg.GetUserID(ctx); !ok { ctx = reqctxpkg.SetUserID(ctx, DefaultLocalUserID) }` — handles background goroutine case | `internal/app/catalog/polling.go:180-182` | LOW (mentioned in §11 testing log) |
| `disk.go::loadFromDisk + saveToDisk + corrupted-file → .bak` helpers | `internal/app/catalog/disk.go` | LOW |

## In doc but not in code (stale)

| Item | Doc location | Severity |
|---|---|---|
| §4 CatalogSource interface includes `EventTopics() []string` method; code's interface has only `Name() / Granularity() / ListItems()` | catalog.md:115-126 vs `domain/catalog/source.go:95-117` | HIGH |
| §6 Service struct shown without `notif` field | catalog.md:211-221 | MED |
| §6 Service struct shows `mu sync.Mutex // 仅保护 RegisterSource / 启停`; code uses `sync.RWMutex sourcesMu` for sources + `sync.Mutex versionMu` for version | catalog.md:221 vs `app/catalog/catalog.go:103,110` | LOW |
| §3 / §13 / §10 explicitly says **"不发 SSE"** (catalog 不发 SSE 事件); code emits `catalog` notification on every successful Refresh that updates cache | catalog.md:9,96,610-611 vs `app/catalog/polling.go:253-258` | HIGH |
| §10 errmap table only lists 2 sentinels (ErrCoverageIncomplete + ErrGenerationFailed) marked "(不到 handler)"; code has 3 (with ErrAllSourcesFailed mapped to 503 in errmap) | catalog.md:606-608 vs `errmap.go:148` | MED |
| §6 `tryRefresh` example shows skipping on busy CAS — matches code | matches | — |
| §6 `Refresh` example shows fingerprint short-circuit + lastFP always update — matches | matches | — |
| §7 generator code example uses 3-attempt retry with missing-id hint pattern — but the doc text below explicitly says "已删 retry loop / missing hint / coverage 校验, 回到单次设计" (屎山拯救计划 #7); both are present in §7 (the prose says deleted, the code block at line 421-484 still shows the deleted retry pattern as illustration). This is intentional historical reference + properly framed | catalog.md:420-485 | LOW (intentional, but bordering on confusion — verify the boundary text reads clearly) |
| §7 single-attempt `Generate` example uses `%w: ... %v` style (which loses inner sentinel chain — but only one %w is allowed; code uses same pattern) | matches | — |
| §11 测试覆盖 — generator_test.go file claim references "buildPrompt contains all items + retry-attempt embeds missing IDs / groupSourceIDs / findMissing" which were deleted per #7. File still exists per ls but those test cases must have been pruned | catalog.md:621 vs `app/catalog/generator_test.go` | LOW (if tests are stale, would need pipeline run; but tests pass per status header so the test names listed in §11 may simply be stale) |
| §13 演化方向 — none directly stale | catalog.md:661-668 | — |
| §6 `func (s *Service) Refresh(ctx)` — doc shows `error` return; matches code | matches | — |
| §6 SystemPromptProvider interface defined in `internal/domain/catalog/catalog.go` per code; matches `Service.GetForSystemPrompt() string` | matches | — |
| §8.3 main.go 装配 example: "1. 创建 service（无须 events bridge——polling 不订阅）"; matches but main.go now passes notif | matches conceptually | LOW |

## Mismatched

| Item | Code | Doc | Severity |
|---|---|---|---|
| Sentinel count | 3 (`ErrCoverageIncomplete` + `ErrGenerationFailed` + `ErrAllSourcesFailed`) | 2 in §4 / §10 | MED |
| §4 Sentinel block in domain code shows 3 sentinels; catalog.md §4 + §10 list only 2 | code at `domain/catalog/catalog.go:117-146` | catalog.md:165-170, 606-608 | MED |
| §7 Generator interface signature: doc says `Generate(ctx, items, granularityMap)` returning `(*Catalog, error)`; matches code | matches | — |
| §6 `pollLoop` description says "first tick fires immediately"; matches code at `polling.go:112` | matches | — |
| §3 Generator self-generates "路由观察" (Notes on choosing) inline; matches `generatorPromptTemplate` constraint #4 | matches | — |
| §9 HTTP API has 2 endpoints; code has 2 routes — matches | matches | — |
| §11 test files: `app/catalog/catalog_test.go` (18) / `generator_test.go` (8) — file paths exist; test counts not verified | (unverified) | — |
| §6 `cache atomic.Pointer[Catalog]` field in Service struct; matches code | matches | — |
| §11 mentions "Refresh PersistsToDisk" / "FingerprintShortCircuit" / "AllSourcesFailKeepsCache" tests — matches code paths in `polling.go::Refresh` | matches | — |
| §3 reads "rm .catalog.json reconstructs from sources (no data loss)" — matches `Start` with corrupt-file → .bak fallback | matches | — |
| `notif.Publish(ctx, "catalog", cat.Fingerprint, ...)` payload includes `fingerprint` + `version` + `generatedAt` — not described in any doc section | code at `polling.go:253-258` | catalog.md (no section) | MED |

## Sub-check
- Entities aligned: yes — Catalog / Item / Granularity all match between domain pkg and §4
- Service methods aligned: **partial** — doc §6 lists 6 methods; code has those + `Stop` / `SetGenerator` / `SetPollInterval`. Method names and signatures match where listed.
- Endpoints aligned: yes — 2 routes match between handler and §9
- Sentinels aligned: **no** — §4 / §10 list 2 but code has 3 (`ErrAllSourcesFailed` registered to 503 in errmap; D1 covered errmap side)
- Cross-domain deps aligned: yes — forge/skill/mcp implement CatalogSource port and register; chat consumes via SystemPromptProvider; main.go wires correctly per §8.3
- 端到端推演 valid: yes — §2 startup / chat hot path / polling tick flow all match code at high level
- Phase 5 / 屎山拯救计划 #7 大变更已反映: yes — the retry-loop deletion is well-documented in §7 with explicit "已删的旧代码" callout and rationale

---

## Summary

- HIGH: 2 (CatalogSource interface in §4 still has `EventTopics() []string` — never existed in code; "不发 SSE" claim repeated in §3/§10/§13 but code emits `catalog` notifications on every cache change)
- MED: 4 (3-vs-2 sentinel count off-by-one in §4 / §10; notif Publisher field undocumented in §6 Service struct; catalog notification payload undocumented; ErrAllSourcesFailed not mentioned in §10)
- LOW: 6 (Stop / SetGenerator / SetPollInterval methods; mu→sourcesMu RWMutex split; user-id injection mention vs full doc; main.go wiring no-bridge claim; potential stale generator_test.go file claim about deleted retry tests; intentionally-kept §7 historical retry code-block boundary clarity)
