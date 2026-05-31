# Phase F audit — testing discipline (§T1-T6)

Scope: 112 `_test.go` files (~1071 `TestX` funcs) across `backend/internal/*` (unit) + `backend/test/` (pipeline).
Reviewed against §T1-T6 in `CLAUDE.md`.

## T1 测试命名 — `Test<Function>_<Scenario>` format

### Single-segment tests (no `_`)

Scanning all 1071 funcs found 32 with no underscore. After classification:

- **TestMain** (test/mcp/mcp_test.go:53) — Go std test entry point. **OK**.
- 31 others are all **single-symbol-under-test functions** (e.g. `TestStripPath`, `TestEnvKeyEqual`, `TestExtractMediaType`, `TestConstants`, `TestWildcardMatch`, `TestSaveAndGetForge`, `TestCreateAndGetRuntime`, `TestUpdateForgeActiveVersion`, `TestUpdateVersionEnvProgress`, `TestMaskKey`, `TestTruncate`, `TestTotalSizeBytes`, `TestExecutionRetention`, `TestVersionLifecycle`, `TestDeleteOldestAcceptedVersion`, `TestTestCaseCRUD`, `TestListAllForges`, `TestSetAndListEnvsWithRunningPID`, `TestClearEnvRunningPID`, `TestListEnvsLastUsedBefore`, `TestFindEnvByOwner`, `TestCreateAndGetEnv`, `TestDeleteEnv`, `TestDeleteRuntime`, `TestExtractBase64Data`, `TestClassifyHTTPError`, `TestIsCallable`, `TestIsValidProvider`, `TestIsValidScenario`, `TestFormatProgressLine`).
  - All use table-driven multi-case bodies; the function tested is self-documenting.
  - Per T1 the format is "`Test<Function>_<Scenario>`" — single-segment tests violate the letter of the rule but communicate clearly (the function name IS the description when there's one scenario or it's a table-driven sweep).
  - **Action**: Not a violation per the "命名清晰则不强求" guidance; left **as-is**. Borderline LOW.

### Action-verb scenarios (the explicit T1 anti-pattern: scenario describes action not condition)

| Test name | File:line | Issue |
|---|---|---|
| `TestSave_Insert` | internal/infra/store/apikey/apikey_test.go:62 | scenario `Insert` is an action (what the test does), not a condition (when X, expect Y). Better: `TestSave_NewKeyPersisted` or `TestSave_NewRowCreated`. |
| `TestSave_Update` | internal/infra/store/conversation/conversation_test.go:62 | same: `Update` is action. Better: `TestSave_ExistingTitleReplaced`. |
| `TestUpsert_Insert` | internal/infra/store/model/model_test.go:55 | same: `Insert` is action. Better: `TestUpsert_NewRowCreated`. |

These three are the only clear T1 violations. The 1068 others use scenario forms describing conditions / outcomes (e.g. `_NotFound`, `_HappyPath`, `_RoundTrip`, `_RejectsX`, `_ReturnsY`, `_EmptyY`, `_DuplicateZ`). Severity **LOW** (intent is clear from siblings: `TestSave_Update` paired with implicit insert in `TestSave_Insert` — the name *almost* describes the condition by negation).

No `ShouldX` / `ShouldWork` / `ShouldFail` style violations found anywhere.

## T2 in-memory SQLite — disk-DB tests

### `dbinfra.Open` survey

All `_test.go` files calling `dbinfra.Open` use either:
- `dbinfra.Config{DataDir: ""}` → in-memory (explicit), or
- `dbinfra.Config{LogLevel: gormlogger.Silent}` (DataDir zero value → in-memory by code path in `dbinfra.buildDSN`: empty `dataDir` ⇒ `:memory:?...`).

22 sites verified. All in-memory.

### Disk-DB legitimate uses (NOT violations)

| Test | File:line | Reason |
|---|---|---|
| `TestOpen_FileDB` | internal/infra/db/db_test.go:42 | tests dbinfra disk-mode behavior; `t.TempDir()` is correct. |
| `TestOpen_WALEnabled` | internal/infra/db/db_test.go:75 | tests WAL pragma which is undefined for in-memory; disk required. |
| `TestOpen_InvalidDataDir` | internal/infra/db/db_test.go:97 | tests error path; OS-level path required. |

These are self-tests of the dbinfra layer — exempt by purpose.

### Other `t.TempDir()` uses

Various `internal/infra/sandbox/*_test.go` use `t.TempDir()` for sandbox **filesystem** (mise binary + node envs), not for SQLite. Not T2-relevant.

### `FORGIFY_TEST_SANDBOX_DIR` (test/mcp/curated_pipeline_test.go:64)

Optional env-set shared sandbox dir for **mise binary cache** (NOT the DB). When set, `t.WithSandboxDataDir` reuses the warmed cache across `go test` invocations. DB remains per-test in-memory (set by `harness.New`'s hard-coded `dbinfra.Open(Config{DataDir: ""})` at line 186). The shared dir's mcp.json persistence is handled with **defensive pre-clean** + idempotent `RemoveServer` (line 451-456) — not a T2 violation, and idempotency is preserved (T5).

### Violations

**None.** No test bypasses `dbinfra.Open` with raw `gorm.Open` or `sqlite.Open`. No test uses a disk-backed DB outside the dbinfra self-tests.

## T3 外部依赖门控

### Env-gated tests (compliant)

| Test family | Helper | Behavior on missing env |
|---|---|---|
| `TestIntegration_*` (5 tests) in `internal/infra/llm/llm_integration_test.go` | `requireKey(t)` — `os.Getenv("DEEPSEEK_API_KEY")` + `t.Skip` | `t.Skip` graceful |
| `TestChat_Live_ReasoningModel_BlocksSeparate` (test/chat/chat_test.go:269) | `th.RequireDeepSeekKey(t)` | `t.Skip` graceful |
| `TestMCP_Live_RegistryInstallEverything` (test/mcp/mcp_test.go:236) | `os.Getenv("FORGIFY_LIVE_MCP_INSTALL") != "1"` + sandbox `IsReady()` | `t.Skip` graceful |
| `TestCuratedMarketplace_*` (5 Live + 1 AllSmoke; test/mcp/curated_pipeline_test.go) | `curatedSmokeEnabled(t)` checks `FORGIFY_CURATED_SMOKE != "1"` + `sandbox.IsReady()` | `t.Skip` graceful |
| `TestErrCodes_ForgeWithSandbox` (test/cross/errcodes_test.go:200) | uses `RequireForgeResources(t, h)` | `t.Skip` graceful when sandbox not ready |
| `TestForgeLifecycle_*` (4 tests; test/forge/lifecycle_test.go) | `RequireForgeResources(t, h)` | `t.Skip` graceful |

### Borderline (network attempt but offline-safe by design)

| Test | File:line | Note |
|---|---|---|
| `TestHTTPTester_ProviderDefaultBaseURL_UsedWhenUserEmpty` | internal/app/apikey/tester_test.go:383 | Calls `tester.Test(ctx, "openai", "k", "", "")` which attempts DNS resolution of `api.openai.com`. Mitigated by `&http.Client{Timeout: 1 * time.Millisecond}` — DNS resolution itself exceeds 1ms so the call always errors fast. Comment explicitly claims "offline-safe". **Not a hard violation** but DNS resolution still attempted (rare lookup failure on `localhost`-only hosts). Theoretical flake on truly air-gapped hosts where DNS resolver hangs >1ms or is broken. Borderline. |

### Violations

**None.** All tests that genuinely call upstream LLM / network use env-gating + `t.Skip`. The one borderline case (1ms timeout test) is documented as offline-safe by the test author.

## T4 production-not-used godoc verification

Symbols D-phase fixed (`ListProviders`, `ListScenarios`) re-verified:

| Symbol | Godoc claim | Reality | Action |
|---|---|---|---|
| `apikeyapp.ListProviders` | "Consumed by the GET /api/v1/providers handler and by the contract test that asserts the registry stays in sync with documentation." (`internal/app/apikey/providers.go`) | Production caller exists: `internal/transport/httpapi/handlers/providers.go:63` | **godoc accurate** |
| `modeldomain.ListScenarios` | "Backs the contract test... production code does not call it." (`internal/domain/model/model.go:60-65`) | `grep -rn "ListScenarios" --include="*.go" \| grep -v _test.go` returns only the definition itself. No production caller. | **godoc accurate** |

Survey for other production-not-used claims:

- `internal/domain/sandbox/sandbox.go:ErrCmdRequired` — comment says "preserves the existing test contract (TestServiceSpawn_EmptyCmd_Errors)" — this is a referential note, not a "test-only" claim. The sentinel IS used in production (`internal/app/sandbox/spawn.go`) via `errors.Is`. **godoc accurate**.
- `internal/app/sandbox/spawn.go:Spawn` — comment says "preserve the explicit test contract" — referential, not test-only. **godoc accurate**.

### Violations

**None.** No symbol claims test-only when it's actually used in production, and vice versa.

## T5 Pipeline 覆盖

### Existing pipeline tests (`backend/test/*_test.go`)

| File | Module covered | Tests | 幂等 OK? |
|---|---|---|---|
| test/smoke/smoke_test.go | Harness boot + chat smoke | 1 | ✅ fresh harness; fake LLM |
| test/uxtodo/uxtodo_test.go | Todo CRUD + AskUserQuestion + answer endpoint | 3 | ✅ fresh harness; fake LLM |
| test/skill/skill_test.go | Skill scan + activate + pre-approval | 3 | ✅ fresh harness; fake LLM |
| test/chat/chat_test.go | Chat send/stream/cancel/error/tool/parallel/auto-title/queue/attachment | 15 | ✅ fresh harness per test |
| test/chat/forge_test.go | Chat ↔ forge tool wiring (get/run/create via LLM) | 3 | ✅ fresh harness; fake LLM |
| test/shell/shell_test.go | Bash + cd state machine + KillShell | 3 | ✅ fresh harness; fake LLM |
| test/web/web_test.go | WebFetch SSRF + WebSearch validate-input | 2 | ✅ fresh harness; fake LLM |
| test/integration/d9_test.go | D9 cross-cutting (catalog→LLM, dynamic skill, boot smoke) | 3 | ✅ fresh harness |
| test/catalog/catalog_test.go | Catalog refresh + fingerprint + mechanical fallback | 3 | ✅ fresh harness |
| test/cross/isolation_test.go | Per-user data isolation (conversation, apikey) | 3 | ✅ fresh harness |
| test/cross/errcodes_test.go | Errmap sentinel coverage across HTTP surface | 2 | ✅ fresh harness; sandbox-gated 2nd test skips when sandbox unavailable |
| test/filesystem/filesystem_test.go | Read/Write/Edit closed-loop + must-Read-first + PathGuard | 3 | ✅ fresh harness; fake LLM |
| test/mcp/mcp_test.go | MCP install + tools/list + degraded + auto-heal + live install | 4 | ✅ fresh harness; sandbox-gated |
| test/mcp/curated_pipeline_test.go | Curated marketplace T0 install + 5 live tool calls | 6 | ✅ defensive pre-clean for shared sandbox cache; per-test t.Cleanup uninstalls |
| test/search/search_test.go | Grep + Glob + PathGuard | 3 | ✅ fresh harness; fake LLM |
| test/apikey/apikey_test.go | APIKey CRUD + test-against-fake + pagination | 5 | ✅ fresh harness |
| test/forge/lifecycle_test.go | Forge sandbox bootstrap + run + testcase + SSE env status | 4 | ✅ fresh harness; sandbox-gated |
| test/forge/http_test.go | Forge HTTP CRUD + duplicate-name + version + export-import | 12 | ✅ fresh harness |
| test/model/model_test.go | Model upsert (idempotent) + list + invalid-scenario | 4 | ✅ fresh harness |
| test/subagent/subagent_test.go | Subagent spawn + event log metadata + max-turns | 3 | ✅ fresh harness; fake LLM |
| test/conversation/conversation_test.go | Conversation CRUD + soft-delete + pagination | 4 | ✅ fresh harness |

**Total: 90 pipeline tests across 21 files.**

### Idempotency check (T5 hard requirement)

`harness.New(t)` creates:
- Fresh in-memory SQLite per call (line 186 of test/harness/harness.go) — single connection per test (`SetMaxOpenConns(1)` line 201).
- All cleanup wired via `t.Cleanup` (DB close, fake LLM teardown, sandbox shutdown).
- Per-test `t.TempDir()` for sandbox `dataDir` (unless test opts into shared cache for performance).
- For shared sandbox cache (`FORGIFY_TEST_SANDBOX_DIR`), tests do **defensive pre-clean** + idempotent `RemoveServer` (test/mcp/curated_pipeline_test.go:451-456) — survives prior-run crashes that skipped `t.Cleanup`.

Verified: all 90 tests are re-runnable. No cross-test mutation; no shared `*gorm.DB`; no global state carryover.

### Missing pipeline tests

| Module | Reason missing | Severity |
|---|---|---|
| eventlog SSE replay + Last-Event-ID + 410/SEQ_TOO_OLD | Covered at unit/handler level (`internal/infra/eventlog/bridge_test.go:107-184` + `internal/transport/httpapi/handlers/eventlog_test.go:90-121`). No dedicated `test/eventlog/*_test.go` exercising HTTP-level replay through full DI graph. Acceptable per T5 (handler-level tests fully exercise the protocol; pipeline would duplicate). | LOW |
| notifications broadcast | No dedicated `test/notifications/*_test.go`. However, `test/harness/sse.go:84-123` `SubscribeSSE` subscribes to BOTH eventlog and notifications channels, so every chat / todo / catalog test that uses harness `SubscribeSSE` implicitly exercises notification broadcast (todo notifications visible at test/uxtodo/uxtodo_test.go assertions; conversation auto-title at test/chat/chat_test.go:618-689). Indirect coverage adequate. | LOW |
| sandbox bootstrap as standalone family | No `test/sandbox/*_test.go`. Sandbox bootstrap + env + spawn are exercised end-to-end by **test/forge/lifecycle_test.go** (4 tests, real sandbox, real `mise` extraction, real env install). Also by **test/mcp/curated_pipeline_test.go** Live tests. Sandbox is **never** a top-level test target — only consumed via forge/mcp. Acceptable: sandbox has no LLM-facing surface of its own to test in isolation. | None — by design |
| WebSearch BYOK + MCP fallback | Pipeline `test/web/web_test.go` covers ValidateInput + SSRF (2 tests). BYOK routing (Brave / Serper / Tavily / Bocha priority) + MCP fallback are covered exhaustively at unit level by `internal/app/tool/web/search_test.go:Execute_*` (12 tests). No dedicated end-to-end LLM↔BYOK pipeline test, but unit coverage thoroughly tests the search-backend selection logic via httptest mocks. Acceptable. | LOW |

### Violations

**None hard.** Coverage is comprehensive for every big-feature module. Three modules have observed-coverage at unit level rather than pipeline level — defensible given the unit tests fully exercise the HTTP-visible wire behavior. Severity LOW.

## T6 fake LLM 默认 + Live_ 前缀

### Pipeline tests that use FakeLLMServer

55 pipeline tests use `th.NewFakeLLMServer(t) + th.WithFakeLLMBaseURL(fake.URL())`. All deterministic and offline.

### Pipeline tests that use `h := th.New(t)` (no fake LLM)

Verified each does NOT reach LLM:
- All CRUD tests (apikey, conversation, forge, model, todo) — pure HTTP/DB.
- `TestChat_MissingModelConfig_ErrorCodePersisted` (test/chat/chat_test.go:122) — fails pre-LLM.
- `TestChat_MissingAPIKey_ErrorCodePersisted` (test/chat/chat_test.go:318) — fails pre-LLM.
- `TestErrCodes_Sweep` (test/cross/errcodes_test.go:27) — error sentinel coverage, no LLM path.
- `TestIsolation_*` (test/cross/isolation_test.go) — per-user data scoping, no LLM.
- `TestD9_DynamicSkillUpdate`, `TestD9_BootSmoke` — pure boot/state checks.
- `TestCatalog_*` — mechanical fallback path (no LLM key wired); catalog never sees an LLM.
- `TestForge_*` (test/forge/http_test.go) — Forge HTTP CRUD only.
- `TestForgeLifecycle_*` (test/forge/lifecycle_test.go) — sandbox-bound forge runs; no LLM call (forge runs Python code in sandbox).
- `TestMCP_*` non-Live — uses FakeMCPServer / FakeMCPRegistry, no LLM.

### Live_ prefix + RequireKey gate audit

| Test | `Live_` prefix | RequireKey gate | FakeLLM default for siblings |
|---|---|---|---|
| `TestChat_Live_ReasoningModel_BlocksSeparate` (test/chat/chat_test.go:269) | ✅ | ✅ `th.RequireDeepSeekKey(t)` | ✅ siblings use fake |
| `TestMCP_Live_RegistryInstallEverything` (test/mcp/mcp_test.go:236) | ✅ | ✅ `FORGIFY_LIVE_MCP_INSTALL=1` env + `Sandbox.IsReady()` | n/a (not LLM-related) |
| `TestCuratedMarketplace_T0_Live_DuckDuckGo` (curated_pipeline_test.go:307) | ✅ | ✅ `curatedSmokeEnabled(t)` | n/a |
| `TestCuratedMarketplace_T0_Live_Context7` (curated_pipeline_test.go:325) | ✅ | ✅ `curatedSmokeEnabled(t)` | n/a |
| `TestCuratedMarketplace_T0_Live_Memory` (curated_pipeline_test.go:357) | ✅ | ✅ `curatedSmokeEnabled(t)` | n/a |
| `TestCuratedMarketplace_T0_Live_Playwright` (curated_pipeline_test.go:380) | ✅ | ✅ `curatedSmokeEnabled(t)` | n/a |
| `TestCuratedMarketplace_T0_Live_ChromeDevTools` (curated_pipeline_test.go:403) | ✅ | ✅ `curatedSmokeEnabled(t)` | n/a |
| `TestIntegration_*` (5 in internal/infra/llm/llm_integration_test.go) | uses `TestIntegration_` prefix (not `Live_`) | ✅ `requireKey(t)` | n/a (unit test, NOT in pipeline tree) |

Note: `internal/infra/llm/llm_integration_test.go` uses `TestIntegration_*` rather than `TestX_Live_*` because these tests pre-date the T6 convention and live under `internal/` (unit-suite location) rather than `test/`. The convention specifically applies to `backend/test/` pipeline tests. Comment in the file (lines 18-39) documents the convention via `requireKey(t)`. **No T6 violation** — convention satisfied in spirit (env-gated, skip-graceful).

### Violations

**None.** Every test crossing the wire to a real provider is `Live_`-prefixed (or `Integration_` prefixed at unit level) and gates with a `Require*Key`-style helper.

### Naming convention nit (NOT a T6 violation)

`TestSpawnLongLived_*` in `internal/infra/sandbox/spawn_test.go` and `internal/app/sandbox/spawn_test.go`, and `TestSubscribe_LiveDelivery` / `TestEventLog_StreamDeliversLiveEvents` use "Live" in the name without meaning "uses real upstream provider." Names refer to "long-lived subprocess" and "live SSE delivery." T6's `Live_` convention is specifically a **prefix segment** (e.g. `TestX_Live_Y`); these tests use "Live" mid-name. No grep ambiguity since they don't match `_Live_` exactly. Documented for clarity.

## Summary

| Rule | Violations | Severity |
|---|---|---|
| T1 测试命名 | 3 (`TestSave_Insert`, `TestSave_Update`, `TestUpsert_Insert` — scenario describes action not condition) | LOW |
| T2 in-memory SQLite | 0 | — |
| T3 外部依赖门控 | 0 hard (1 borderline: `TestHTTPTester_ProviderDefaultBaseURL_UsedWhenUserEmpty` does live DNS lookup with 1ms timeout) | LOW |
| T4 production-not-used godoc | 0 | — |
| T5 Pipeline 覆盖 + 幂等 | 0 hard (3 LOW gaps: no dedicated eventlog-pipeline, notifications-pipeline, WebSearch-BYOK-pipeline — all have unit/indirect coverage) | LOW |
| T6 fake LLM + `Live_` 前缀 | 0 | — |

**Totals: 0 HIGH / 0 MED / 7 LOW.**

### Highlights

- **Zero HIGH or MED violations.** Backend testing discipline is clean.
- §T1 violations are limited to 3 storage-layer Save/Upsert tests where `_Insert` / `_Update` could be renamed to a condition-shaped form (`_NewRowCreated`, `_ExistingRowReplaced`). Cosmetic only.
- §T3 borderline (1ms timeout DNS-attempting test) is documented by the test author as offline-safe; not a hard violation.
- §T5 coverage gaps are all defensible by handler-level / unit-level coverage of the underlying contract; no big feature is unexercised end-to-end.
- All `Live_` tests gate with `RequireDeepSeekKey` / `curatedSmokeEnabled` / `FORGIFY_LIVE_MCP_INSTALL` env + `t.Skip` cleanly.
- Harness idempotency is strong: fresh in-memory SQLite per `harness.New(t)`, single connection, all cleanup via `t.Cleanup`, defensive pre-clean for shared-cache opt-in cases.
