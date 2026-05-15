# Dead-Logic Audit — low-level infra + pkg + cmd

**Scope** (per fork-G dead-8):

- `backend/internal/infra/db/` (3 .go: db.go / migrate.go / schema_extras.go)
- `backend/internal/infra/crypto/` (2 .go: aesgcm.go / fingerprint.go)
- `backend/internal/infra/llm/` (8 .go: llm.go / factory.go / adapter.go / openai.go / anthropic.go / sanitizer.go / mock.go / trace.go)
- `backend/internal/infra/memory/` — DOES NOT EXIST (already removed; see §Notes)
- `backend/internal/infra/logger/` (2 .go: zap.go / broadcast.go)
- `backend/internal/pkg/idgen/` (1 .go)
- `backend/internal/pkg/reqctx/` (4 .go: reqctx.go / locale.go / agentstate.go / agentrun.go)
- `backend/internal/pkg/pagination/` (1 .go)
- `backend/internal/pkg/pathguard/` (1 .go)
- `backend/internal/pkg/agentstate/` (2 .go: agentstate.go / skill.go)
- `backend/internal/pkg/llmclient/` (1 .go)
- `backend/internal/pkg/installprogress/` (1 .go)
- `backend/internal/pkg/llmparse/` (1 .go: extractjson.go)
- `backend/cmd/server/main.go`
- `backend/cmd/resources/main.go`

`_test.go` files explicitly excluded; only test references checked via grep when verifying dead-ness of exported symbols.

**Method**: every non-test file read end-to-end; cross-references via grep across the entire backend; tests counted as dead consumers (i.e. only-test-uses ≈ unwired in production).

**Tally**: 9 findings — 0 HIGH / 4 MED / 4 LOW + 1 EDGE.

---

## MED-1 — `pkg/agentstate.AgentState.SubagentTokenLog()` reader has zero non-test callers; entire token-log slice is write-only

- **Location**: `backend/internal/pkg/agentstate/agentstate.go:31-39, 53-65, 67-79, 81-91`. Caller: `backend/internal/app/subagent/spawn.go:277`.
- **Claims** (godoc `agentstate.go:31-37`): *"the conversation-detail UI / cost panel reads it to surface 'this turn spent N tokens including 3 subagents'"*.
- **Reality**: grep `SubagentTokenLog` against `backend/` returns exactly two non-doc hits: the method itself (`agentstate.go:85`) and `agentstate_test.go` (×4 reads). **Zero** non-test reader. The only writer is `subagentapp.spawn.go:277` calling `state.AddSubagentTokens(...)`. There is no HTTP endpoint, no notifications publisher, and no chat surface that reads the slice — the conversation-detail/cost-panel rationale never shipped. Each Subagent spawn keeps appending entries to a slice nobody reads, growing per-conversation memory by ~24 bytes × spawns until the AgentState is GC'd at conversation end.
- **Severity**: MED — ~30 lines of state + 4 exported symbols (`SubagentTokenEntry` type, `subTokensMu`, `subTokens`, `AddSubagentTokens`, `SubagentTokenLog`) plus the spawn.go call site, all justified by an unwired UI claim. The dead-2.md audit (subagent fork) already flagged the **call** at `spawn.go:277` as EDGE-1 because the dead reader lived "outside that audit's scope". This audit owns `pkg/agentstate`, so the dead-ness is now confirmed at the home layer: no caller anywhere.
- **Fix**: Two coordinated edits.
  1. Delete `subTokensMu`, `subTokens`, `SubagentTokenEntry`, `AddSubagentTokens`, `SubagentTokenLog` from `agentstate.go`. Strip the godoc reference at line 53-65.
  2. Drop the corresponding `state.AddSubagentTokens(...)` call at `subagent/spawn.go:277` (and any nearby comment that mentions token bookkeeping).
  3. Update agentstate package doc (`agentstate.go:1-6`) — currently says "must-Read-first SeenFiles, Bash cwd"; that is already accurate post-cleanup, so no edit needed there.
- **Risk**: Zero in production. Test breakage scoped to `agentstate_test.go::TestSubagentTokenLog_*` (4 tests) — out of read-scope for this audit but trivially deletable when fixing.

---

## MED-2 — `pkg/reqctx.WithSubagentRunID` writes a ctx key nothing reads (production OR tests)

- **Location**: `backend/internal/pkg/reqctx/agentrun.go:104-154` (godoc, key type, setter, getter). Sole writer: `backend/internal/app/subagent/spawn.go:143`.
- **Claims** (agentrun.go:106-111): *"SubagentRunID lets sub-runner code attribute downstream events back to the spawn."*
- **Reality**: grep `GetSubagentRunID\|subagentRunIDKey` across the entire `backend/` finds only the symbol declarations themselves — no caller in production and no caller in tests. `WithSubagentRunID` is invoked once in `spawn.go:143` to stamp the new sub-ctx, then nothing ever reads it back. The "downstream events back to the spawn" attribution path was never built — sub-run events flow through `eventlog.Emitter` keyed on conversationId + parentBlockID instead, which doesn't need the runID ctx key.
- **Severity**: MED — 4 unused exports (`WithSubagentRunID`, `GetSubagentRunID`, `subagentRunIDKey` type, ~25 lines of bilingual godoc) plus a write site that consumes a goroutine-allocated ctx slot per subagent spawn for nothing. The depth ctx-key right above (lines 112-134) is genuinely live (read by `app/skill/activate.go:98` + `app/tool/subagent/agent.go:172`); the runID key is a near-dead twin sibling.
- **Fix**: Delete `subagentRunIDKey`, `WithSubagentRunID`, `GetSubagentRunID` from `agentrun.go`. Update the section comment at lines 104-111 to mention only `SubagentDepth`. Drop the `WithSubagentRunID` invocation at `spawn.go:143`.
- **Risk**: Zero. Lexically unreferenced.

---

## MED-3 — `infra/llm.TraceRecorder.Clear()` method has zero callers (production OR tests)

- **Location**: `backend/internal/infra/llm/trace.go:138-147`.
- **Claims** (godoc): *"Clear drops all traces for one conversation. Returns count dropped."*
- **Reality**: grep `TraceRecorder\.Clear\|tracer\.Clear\|recorder\.Clear` across `backend/` returns zero hits. The only method on the recorder that's wired into the dev-only `/dev/llm-trace` endpoint is `TracesFor` + `Conversations` (`dev_mock_llm.go:240,246`). `Mock().Clear()` exists in dev_mock_llm.go but that's `MockClient.Clear()` (mock.go:113) — different type, unrelated. There's no `/dev/llm-trace?action=clear` handler, no UI-triggered reset; the recorder simply accumulates until process restart.
- **Severity**: MED — exported method + mutex-protected state mutation that no caller ever exercises. Smaller-than-MED-1/2 (single method, ~10 lines + godoc), but it lives in the most-actively-touched test-surface package and a future maintainer adding a "clear traces" feature will reasonably reach for `Clear` and never realize it was already test-dead.
- **Fix**: Delete `TraceRecorder.Clear` (trace.go:138-147 inclusive). If a "clear traces" UI surface is wanted later, re-add then with a real wire path.
- **Risk**: Zero. Lexically unreferenced.

---

## MED-4 — `pkg/llmclient.ErrBuildClient` sentinel never matched by any caller

- **Location**: `backend/internal/pkg/llmclient/llmclient.go:23-27, 95`.
- **Claims** (godoc:19-21): *"Step sentinels distinguish which resolve stage failed, for callers that surface different user-facing error codes per stage."*
- **Reality**: grep `ErrBuildClient` returns only the `var` declaration (line 26) and the wrap site (line 95). The two siblings `ErrPickModel` and `ErrResolveCreds` are correctly caught at `chat/runner.go:112,114` — that's where the per-stage error code mapping lives. `ErrBuildClient` is never `errors.Is`'d. Any failure inside `factory.Build` (only known case: provider="ollama"/"custom" without BaseURL → wraps `ErrBadRequest` in `factory.go::resolveBaseURL`) gets double-wrapped (`ErrBuildClient` → `ErrBadRequest`) and falls through to the default `LLM_PROVIDER_ERROR` code in chat/runner.go:110. The sentinel adds no resolution information that callers extract.
- **Severity**: MED — the broken-symmetry case in a 3-step error stage. Per-stage discrimination was the explicit design goal (godoc says so); the third leg is missing without comment. New maintainers writing "if errors.Is(err, ErrBuildClient)" will be quietly wrong because the wrap is in place but the catch isn't documented as missing.
- **Fix**: Two options.
  1. **Catch it**: extend `chat/runner.go:111-116` switch with `case errors.Is(err, llmclientpkg.ErrBuildClient): code = "LLM_BUILD_FAILED"` (or similar). Most coherent with the godoc claim. Other consumers (`forge`, `mcp`, `subagent`, `tool/forge`, `tool/web`) currently just bubble; would need a deliberate decision whether they should also map.
  2. **Drop it**: delete `ErrBuildClient` at line 26 + change the wrap at line 95 to plain `fmt.Errorf("llmclient.Build: %w", err)`. Honest "we don't differentiate this stage". Net code is smaller; the LLM_PROVIDER_ERROR fallback covers the case fine.
  Pick (2) unless someone has a real stage-specific UI plan; it removes the surface area that promises something the code doesn't deliver.
- **Risk**: Both options low. Option (1) just adds a switch arm. Option (2) is mechanical sentinel removal — no caller's `errors.Is` will go from "true" to "false" because no caller checks today.

---

## LOW-1 — `infra/llm.Adapter.AfterStreamEvent` extension hook has zero production overrides; fan-out wrapper machinery exists for tests only

- **Location**: `backend/internal/infra/llm/adapter.go:76-92` (interface + base no-op); `adapter.go:288-300` (`adapterWrappedClient.Stream` fan-out loop). Concrete adapters at lines 103-232 (12 total): every one inherits `baseAdapter.AfterStreamEvent` returning `[]StreamEvent{ev}` (1-element passthrough).
- **Claims** (godoc:17-19): *"AfterStreamEvent — incoming StreamEvent transformation (e.g. fan out / drop / rewrite). Currently no-ops; reserved for provider-specific event-level fixups when they appear."*
- **Reality**: The claim is honest about current state ("Currently no-ops; reserved for..."). What's worth flagging: the **fan-out loop** at lines 291-298 wraps every Stream call's events through a 1-element slice unconditionally. For every event from every provider — across every chat turn, every tool result, every reasoning delta — there's an extra `for _, transformed := range []StreamEvent{ev}` allocation. Tests in `adapter_test.go` exercise the fan-out with a `spyAdapter` that fans 1→2; production never. This is dead-logic in the strict "code runs but the work is no-op" sense: every LLM call pays an allocation tax for a future hook.
- **Severity**: LOW — perf cost is negligible (1 alloc per event in a path already doing JSON marshal). Conceptually: the interface contract "returning multiple events fans out" is intentional API surface for future use, and the test harness uses it. Not actionable today.
- **Fix**: Two options.
  1. **Keep as-is** (default): the hook is real extension surface, the perf overhead is trivial, the test exists. Just be aware it's dormant.
  2. **Simplify until needed**: change `Adapter.AfterStreamEvent(ev StreamEvent) StreamEvent` to single-event return; fan-out can be added back if/when a provider needs it. Saves ~10 lines of wrapper machinery + the slice alloc per event. The first concrete fan-out adapter would be the trigger to revert.
  Recommend (1) — the cost is real but trivial, and pre-emptive simplification often costs more on the eventual unwind.
- **Risk**: (1) zero. (2) test breakage in `adapter_test.go::TestAdapterWrappedClient_*` which depend on fan-out.

---

## LOW-2 — `infra/logger/broadcast.go:37-44` design-mirror reference points to a deleted package AND describes the wrong semantic

- **Location**: `backend/internal/infra/logger/broadcast.go:37-44`.
- **Claims**: *"Design mirrors infra/events/memory/bridge.go: snapshot subs under RLock, send outside the lock; slow subs drop entries."*
- **Reality**: `infra/events/memory/bridge.go` does not exist (the entire `infra/events/` tree was deleted in the `infra/eventlog` + `infra/notifications` split — confirmed via `find . -path '*/infra/events/*'` returning nothing). The two surviving Bridge implementations (`infra/eventlog/bridge.go`, `infra/notifications/bridge.go`) actually use **block-on-slow-subscriber** semantics, the **opposite** of broadcast.go's drop-on-buffer-full at line 179 (`default: // subscriber buffer full — drop silently`). So the comment is doubly wrong: (a) the path it references is gone, (b) the semantic it claims to mirror is the opposite of what currently survives. The actual broadcast.go implementation is fine — drop-on-full IS the right call for log lines (losing a few during a slow consumer is preferable to back-pressuring the entire app's logger).
- **Severity**: LOW — comment-only rot, not behavior. Misleads anyone reading "let me look at the reference design" and "if logs use drop, do eventlog/notifications also drop?".
- **Fix**: Two-line edit at lines 39-44. Replace with something like:
  > *"Drop-on-slow-subscriber semantic: log lines are append-only and many; losing a few in a slow consumer is preferable to back-pressuring the logger. Contrast `infra/eventlog` / `infra/notifications` which BLOCK on slow subscribers because their events carry state that mustn't be lost."*
- **Risk**: None — pure doc fix.

---

## LOW-3 — `pkg/llmparse.IsLikelyJSON` exported "for callers that want to validate without binding a specific schema" — no such caller exists

- **Location**: `backend/internal/pkg/llmparse/extractjson.go:51-58`. Internal usage at lines 31, 43.
- **Claims** (godoc): *"Exported for callers that want to validate without binding a specific schema."*
- **Reality**: grep `llmparse\.IsLikelyJSON\|llmparsepkg\.IsLikelyJSON` returns zero non-test, non-package hits. Only `extractjson.go` itself uses it. Every external caller of the package uses `ExtractJSON` (forge, mcp, catalog, tool/forge — all consume the (string, bool) shape directly). The exported godoc promises a use that never materialized.
- **Severity**: LOW — single exported function whose export motivation is fictional. Easy fix; impact is minimal because the function is so simple anyone wanting it would re-implement in 2 lines.
- **Fix**: Either (a) unexport to `isLikelyJSON` and trim the godoc claim; or (b) leave exported and remove the misleading "Exported for callers that..." sentence (this packages's domain is "LLM output parsing helpers", so an exported JSON-validity probe is plausibly useful in the abstract). Pick (a) — there's no actual external user, and an unexported helper is simpler to refactor later.
- **Risk**: Zero. No external caller breaks.

---

## LOW-4 — `pkg/installprogress.installprogress.go:168` retains `ctx` parameter "for future telemetry" — never wired

- **Location**: `backend/internal/pkg/installprogress/installprogress.go:147-169`. Specifically line 168: `_ = ctx // ctx no longer used for emit; retained in signature for future telemetry`.
- **Claims**: inline comment — *"retained in signature for future telemetry"*.
- **Reality**: The `close(ctx, err)` method takes ctx but uses `context.Background()` for both the final delta (line 153) and StopBlock (line 167); the §S9 detached-context discipline is the intentional reason (commented at lines 158-166). The trailing `_ = ctx` is purely cosmetic — a signature-preservation marker for "telemetry someday". No telemetry consumer has materialized, no log statement reads ctx for tracing, no follow-up ticket pins it. Compared with the surrounding production-grade comments, this lone "future telemetry" loose end stands out as design-debt sediment.
- **Severity**: LOW — a single discarded parameter + 1 line of comment. The signature itself (`close(ctx, err)`) is fine because the method has 1 caller (`installprogress.go:78` inside `Run`) which already has ctx in scope; dropping ctx from the signature is a 2-line edit if desired.
- **Fix**: Either (a) drop `ctx` from `(p *progressCallback) close` signature + the `_ = ctx` line, since detached context is used internally for emit and ctx-cancel observation is also intentionally NOT done; or (b) leave as-is and trim the comment to "ctx is intentionally not used — see decision lines 158-166". (b) is the smaller, safer fix.
- **Risk**: Both negligible. No caller breaks under (a) since the only caller has ctx in scope but doesn't depend on it being passed back.

---

## EDGE-1 — `pkg/reqctx.Locale.IsSupported()` exported but only internal + test use; defensive check fires for impossible inputs in production

- **Location**: `backend/internal/pkg/reqctx/locale.go:19-24, 40`.
- **Claims**: godoc — *"IsSupported reports whether the locale is one this backend handles."*
- **Reality**: `IsSupported` is exported but called from exactly one production site: `GetLocale` itself (line 40, defensive guard against unsupported values on ctx). Tests call it directly. The only `SetLocale` call sites are `middleware/locale.go:18` (always feeds `LocaleZhCN` or `LocaleEn` — the only outputs of `parseAcceptLanguage`) and `chat/chat.go:355` (forwards `GetLocale(ctx)` which itself returns only supported values). So in production, **the IsSupported check inside GetLocale never triggers a false branch** — the input is provably always supported. The defensive code path is correct as belt-and-suspenders, but the `IsSupported` export status is unjustified: no external caller exists, no plausible future caller has been hinted at.
- **Severity**: EDGE — defensive code that's mathematically dead in production but cheap and correctly defensive. The exported-vs-unexported question is cosmetic. Keep it as-is unless a "tighten public API" sweep is planned.
- **Fix**: Optional. (a) Unexport to `isSupported` — sub-1-minute edit, removes the misleading "external API" surface. (b) Leave exported, add the test that proves the constructor (`parseAcceptLanguage`) only emits supported values, treat IsSupported as the "self-test" affordance for tests. The current state already implicitly does (b); no edit required.
- **Risk**: (a) test-file references break (`locale_test.go:60-70` calls `c.in.IsSupported()`); easy mechanical update when fixing. Production unaffected.

---

## Notes (out-of-scope but observed)

### `infra/memory/` — already removed

The audit task listed `internal/infra/memory/` as scope. Directory does not exist (`ls: ...infra/memory: No such file or directory`). The legacy single-bridge memory implementation was split into `infra/eventlog/bridge.go` (per-conversation streaming protocol) + `infra/notifications/bridge.go` (global entity broadcast) during the V1.2 chat infra refactor (2026-04-27). Confirmed by the same broadcast.go reference rot flagged in LOW-2: the deleted `infra/events/memory/bridge.go` is the now-canonical-mirror source, replaced by the two new bridges.

### `pkg/eventlog/`, `pkg/notifications/` — out of audit scope

These exist (`pkg/eventlog/eventlog.go` and `pkg/notifications/notifications.go`) but are listed only in OTHER fork-G dead-logic audits if any. This audit (dead-8) does not own them per the explicit scope in the task; they were peeked at only to verify the broadcast.go reference rot.

### `infra/db/schema_extras.go` — clean

Only one extraGroup (forges partial UNIQUE index). The "tools → forges" rename historical context is preserved as a comment block (lines 33-38) about the now-removed FTS5 index on `messages.content`. Honest historical note, not dead-logic. Will become live again when FTS5 is re-added on `message_blocks.data`.

### `infra/crypto/` — clean

Both files clean. `v1Prefix` const + `ErrUnsupportedVersion` exist as forward-compatibility scaffolding for a future v2/KMS path (`aesgcm.go:16-21, 105-110`); used today in encrypt/decrypt fast-paths, not pre-built infrastructure for a path that doesn't exist. The `MachineFingerprint` switch is correctly platform-conditional with no fallback hardcode (per the explicit `ErrNoFingerprint` discipline that the comment loudly defends).

### `infra/llm/sanitizer.go` — clean

Defensive single-pass scanner; live, with concrete production-incident rationale documented at the top. No dead branches.

### `infra/llm/openai.go:108` — minor unwrap inconsistency (not dead-logic)

`yield(StreamEvent{Type: EventError, Err: fmt.Errorf("llm: provider returned error: %s", resp.Error.Message)})` doesn't wrap an `Err*` sentinel, in contrast with the surrounding `%w: ErrProviderError` style at lines 112, 256. Minor — `errors.Is` from this branch always returns false. Not dead-logic; flagged here so a future "make all stream errors discriminable" pass has the spot pre-located.

### `cmd/server/main.go` + `cmd/resources/main.go` — clean

Server main.go contains no DI registrations of deleted services (no `eventsdomain.Bridge`, no v0/v1 sandbox lingering). The `infra/events` family is entirely absent from imports. resources/main.go's "v1 uv + python-build-standalone fetcher → v2 mise embed" replacement is honestly documented in package doc; no leftover code paths or download paths for the v0 scheme.

---

## Summary

| ID | Severity | Location | One-line |
|---|---|---|---|
| MED-1 | MED | pkg/agentstate/agentstate.go:31-91 + subagent/spawn.go:277 | SubagentTokenLog reader has 0 production callers; ~30-line slice + mutex pure write-only |
| MED-2 | MED | pkg/reqctx/agentrun.go:104-154 + subagent/spawn.go:143 | WithSubagentRunID writes ctx key never read (no production OR test reader) |
| MED-3 | MED | infra/llm/trace.go:138-147 | TraceRecorder.Clear has 0 callers |
| MED-4 | MED | pkg/llmclient/llmclient.go:23-27, 95 | ErrBuildClient sentinel wrapped on output, never matched |
| LOW-1 | LOW | infra/llm/adapter.go:76-92, 288-300 | AfterStreamEvent fan-out wrapper has 0 production overrides |
| LOW-2 | LOW | infra/logger/broadcast.go:37-44 | "Design mirrors infra/events/memory/bridge.go" — path deleted AND wrong semantic claim |
| LOW-3 | LOW | pkg/llmparse/extractjson.go:51-58 | IsLikelyJSON "exported for callers" — no callers exist |
| LOW-4 | LOW | pkg/installprogress/installprogress.go:168 | `_ = ctx // future telemetry` never wired |
| EDGE-1 | EDGE | pkg/reqctx/locale.go:22, 40 | Locale.IsSupported defensive check unreachable in current call graph |

Net actionable: **MED-1 through MED-4** are clean deletions / one-arm switch additions. **LOW-1 through LOW-4** are doc/cosmetic touch-ups (LOW-2 is the most impactful — fixes a comment that misleads about the entire bridge family's design). **EDGE-1** is leave-alone unless a public-API tightening sweep happens.
