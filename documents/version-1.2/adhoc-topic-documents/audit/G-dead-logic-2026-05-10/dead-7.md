# Dead-Logic Audit — small-CRUD apps + the rest of `domain/` + select stores

**Scope** (per fork instructions):
- `backend/internal/app/apikey/` (3 files: apikey.go / providers.go / tester.go)
- `backend/internal/app/ask/` (1 file: ask.go)
- `backend/internal/app/model/` (1 file: model.go)
- `backend/internal/domain/` — every subpackage **except** chat / forge / mcp / sandbox / subagent (already audited): apikey / ask\* / attachment\* / catalog / conversation / crypto / errors / eventlog / model / notifications / skill / todo
- `backend/internal/infra/store/{apikey,chat,conversation,model}/`

(\* `domain/ask` and `domain/attachment` do not exist — `app/ask` keeps its sentinels in-package, attachment lives in `domain/chat`.)

Test files (`_test.go`) are **not** read per fork rules.

**Method**: every non-test file read end-to-end; cross-referenced against all consumers via grep (no test-file callers counted as live); error sentinels traced from declaration through `errmap.go` to actual return sites.

**Tally**: 9 findings — 3 HIGH / 3 MED / 2 LOW + 1 EDGE.

---

## HIGH-1 — `apikeydomain.ErrTestFailed` is a registered-but-never-returned sentinel

- **Location**: `backend/internal/domain/apikey/apikey.go:90`; registered at `backend/internal/transport/httpapi/response/errmap.go:54`.
- **Claims**: declaration — `ErrTestFailed = errors.New("apikey: connectivity test failed")`. errmap entry — maps to `422 / API_KEY_TEST_FAILED`, claiming the wire code "API_KEY_TEST_FAILED" is real.
- **Reality**: grep `apikeydomain.ErrTestFailed` (excluding `_test.go` and the declaration / errmap entry) returns **zero** hits. Inspection of `app/apikey/apikey.go::Service.Test` (lines 180-230) shows that on a failed probe (`result.OK=false`), the function returns the `*TestResult` value with `OK=false` and `nil` error — a "successful test that found a bad credential" — never `ErrTestFailed`. The only error paths in `Test` wrap `ErrInvalidProvider` / encryption / DB errors, never `ErrTestFailed`. The `422 API_KEY_TEST_FAILED` code in error-codes.md is unreachable.
- **Severity**: HIGH — the wire-code "API_KEY_TEST_FAILED" appears in error-codes.md as if it were a real production status. Any client (testend / future Wails UI) that branches on it is dead-branching. New maintainers reading errmap reasonably believe Service.Test returns this error and design retry / UX around it.
- **Fix**: Two options. (a) Delete the sentinel + the errmap entry + the doc row. (b) Rewire `Service.Test` to return `fmt.Errorf("...: %w", apikeydomain.ErrTestFailed)` when `result.OK=false` and let the handler render 422 — but this would change the existing Test endpoint contract from "200 with `{ok:false}`" to "422", a wire break. (a) is the mechanical option matching current behavior; (b) is a contract-redesign decision out of scope here. Pick (a) unless the team explicitly wants the contract change.
- **Risk**: (a) zero — sentinel is unreachable.

---

## HIGH-2 — `apikeydomain.ErrInvalid` is a registered-but-never-returned sentinel

- **Location**: `backend/internal/domain/apikey/apikey.go:91`; registered at `backend/internal/transport/httpapi/response/errmap.go:55`.
- **Claims**: declaration — `ErrInvalid = errors.New("apikey: key rejected by provider")`. errmap entry — maps to `401 / API_KEY_INVALID`. Stated semantic: returned when a provider rejects a key (401/403 in upstream call).
- **Reality**: grep `apikeydomain.ErrInvalid` (excluding declaration / errmap / `_test.go`) returns **zero** hits. The reachable "401/403 from provider" code path is `apikey.Service.MarkInvalid` (apikey.go:270-288), which writes `test_status=error` to DB and **returns nil** on success — it has no path that returns `ErrInvalid` to the caller. The actual 401/403 propagation uses `llminfra.ErrAuthFailed` (errmap.go:206) for LLM and `webtool.ErrAuthFailed` (errmap.go:221) for web search. `ErrInvalid` is a leftover from a pre-refactor design where MarkInvalid was supposed to surface the rejection error to its caller.
- **Severity**: HIGH — same defect class as HIGH-1: the `401 / API_KEY_INVALID` wire code is dead. Pairing with HIGH-1, two of the eight apikey wire codes registered in errmap are unreachable. Future-maintainer hazard: someone adding handler logic that special-cases `errors.Is(err, apikeydomain.ErrInvalid)` will write code that never fires.
- **Fix**: Delete `ErrInvalid`, its errmap entry, and the error-codes.md row. The actual behavior — 401 from upstream LLM provider → `llminfra.ErrAuthFailed` → 401 to client — is already covered by the `llminfra` sentinels.
- **Risk**: Zero — sentinel is unreachable.

---

## HIGH-3 — `tododomain.ErrConversationMismatch` is explicitly never returned

- **Location**: `backend/internal/domain/todo/todo.go:74`. Decision is documented (not landing-as-bug) at `backend/internal/app/todo/todo.go:117-123, 165-168, 224-225`.
- **Claims**: declaration godoc — *"ErrConversationMismatch: caller tried to mutate a todo from a different conversation than ctx — defensive reject to prevent scope leak."*
- **Reality**: grep `ErrConversationMismatch` returns the declaration plus **a single comment** at app/todo/todo.go:119: *"A todo belonging to another conversation is reported as ErrNotFound rather than ErrConversationMismatch — we don't want to leak existence across conversations."* All three cross-conversation guard sites (Get line 134, Update line 168, Delete line 225) return `tododomain.ErrNotFound` instead. The sentinel is declared explicitly to NOT be returned. Correct existence-non-leak design — but the sentinel itself is purely vestigial.
- **Severity**: HIGH — exported sentinel that documentation explicitly states is the right thing to throw, but the implementation explicitly doesn't. A maintainer adding a permission-aware admin endpoint who sees this sentinel would reasonably wire it; everyone else reading the comment chain in todo.go finds it confusing ("declared a thing whose role is to never exist").
- **Fix**: Delete `ErrConversationMismatch` from the sentinel block (todo.go:74). Rewrite the inline comment in app/todo.go:117-123 to drop the "instead of X" framing and just say "cross-conversation lookup → ErrNotFound (no existence leak)". If true admin override paths land later, re-introduce the sentinel.
- **Risk**: Zero — sentinel has no live consumers.

---

## MED-1 — `errorsdomain.ErrInternal` is a registered-but-never-returned cross-domain sentinel

- **Location**: `backend/internal/domain/errors/sentinel.go:16`; registered at `backend/internal/transport/httpapi/response/errmap.go:45`.
- **Claims**: godoc — *"ErrInternal: unexpected failure — bug or infra outage."* errmap entry — maps to `500 / INTERNAL_ERROR`, alongside `ErrInvalidRequest` (400 / `INVALID_REQUEST`) which IS used (4 sites in pagination.go and apikey handler).
- **Reality**: grep `errorsdomain.ErrInternal` outside of declaration / errmap / `_test.go` returns **zero** hits. No code wraps anything in `ErrInternal`. Compare with the explicit-justification block at errmap.go:188-196 for `reqctxpkg.ErrMissingUserID` etc. (registered to suppress "unmapped domain error" warnings) — `ErrInternal` has no such justification. It's a leftover from the original sentinel pair design (ErrInvalidRequest/ErrInternal as cross-cutting bookends) where only ErrInvalidRequest got real consumers.
- **Severity**: MED — exported cross-domain sentinel with the most generic name in the codebase, declared and forgotten. Any developer adding a "fallback for unrecognized error" probably reaches for this — and they'll write something that's correct in shape (500 / INTERNAL_ERROR fallback) but disconnected from the real fallback flow (which is errmap.go's `unmapped domain error` warn-and-500). The 500 fallback is already implemented at errmap.go's `lookup` miss path (no need for the sentinel).
- **Fix**: Delete `ErrInternal` from sentinel.go and the errmap entry. The package can keep just `ErrInvalidRequest` (single-cross-cutting-sentinel) or be renamed to a simpler module reflecting its actual single-sentinel size.
- **Risk**: Zero — sentinel is unreferenced.

---

## MED-2 — `askapp.ErrAlreadyAnswered` is exported "for errmap dictionary completeness" alone

- **Location**: `backend/internal/app/ask/ask.go:43-50` declaration; registered at `backend/internal/transport/httpapi/response/errmap.go:185`.
- **Claims**: declaration godoc — *"ErrAlreadyAnswered: Resolve was called twice for the same tool_call ID. The first answer is the answer of record."* errmap entry — maps to `409 / ASK_ALREADY_ANSWERED`.
- **Reality**: ask.go itself contains a confessional block at lines 125-128: *"We never return ErrAlreadyAnswered any more — it is now subsumed by ErrNoPendingQuestion since the second caller cannot see the entry."* and 133-134: *"不再返 ErrAlreadyAnswered（已被 ErrNoPendingQuestion 覆盖）；sentinel 仍导出供 errmap / 测试文档化概念。"* `Resolve` (lines 135-151) atomically deletes the entry under the lock so a second call is structurally impossible to differentiate from "no pending"; both produce `ErrNoPendingQuestion`. The sentinel is kept "for errmap dictionary completeness" (literal author note) — but errmap.go is supposed to be a sentinel→HTTP code map, not a dictionary of concepts. Wire code `409 ASK_ALREADY_ANSWERED` is unreachable.
- **Severity**: MED — same defect family as HIGH-1/2 (dead wire code in error-codes.md), demoted because the author has at least documented the contradiction. The error-codes.md row still misleads frontend/test code that branches on `code === "ASK_ALREADY_ANSWERED"`. If frontends ever began to differentiate "already answered" vs "no pending", they'd need a real producer; right now the dictionary entry is a lie.
- **Fix**: Three coordinated edits. (1) Delete `ErrAlreadyAnswered` from ask.go:50. (2) Delete the errmap entry at line 185. (3) Drop the row from error-codes.md. Trim the contradiction commentary at ask.go:125-128, 133-134. The single-sentinel ask domain (`ErrNoPendingQuestion` + `ErrTimeout`) is the actual contract.
- **Risk**: Zero — sentinel unreachable; comment block already acknowledges this.

---

## MED-3 — `apikeyapp.ListProviders` godoc claim is a lie

- **Location**: `backend/internal/app/apikey/providers.go:132-144`.
- **Claims**: godoc lines 134-138 — *"ListProviders returns all supported provider names (unordered). Production code does not call this — it exists for the contract test that asserts the registry stays in sync with documentation."*
- **Reality**: `backend/internal/transport/httpapi/handlers/providers.go:63` literally calls `apikeyapp.ListProviders()` to enumerate the registry for `GET /api/v1/providers` — a real production endpoint that the testend control panel and future Wails UI hit to render the "Add API Key" dropdown. The godoc claim was true at one point (T4 in `CLAUDE.md` documents the exact concern about deadcode tools mistakenly removing test-only exports) but has not been updated since the providers HTTP route landed. **This is misleading docs, not dead code** — but per the audit's "claims to do / reality" framing, the disagreement matters: a maintainer reading "production code does not call this" might delete it under a deadcode pass and break the providers endpoint.
- **Severity**: MED — the misleading claim creates the same hazard as the T4 commandment was meant to prevent (the comment exists *because* of T4, then drifted).
- **Fix**: Replace godoc with: *"ListProviders returns all supported provider names (unordered). Consumed by the GET /api/v1/providers handler and by the contract test that asserts the registry stays in sync with documentation."*
- **Risk**: Zero documentation-only edit. (`tododomain.ListStatuses:60-62` and `modeldomain.ListScenarios:60-65` carry similarly-worded godoc and are genuinely test-only — verified — so theirs is correct.)

---

## LOW-1 — `apikeydomain.APIFormatOpenAICompatible` constant has no production consumer

- **Location**: `backend/internal/domain/apikey/apikey.go:60`.
- **Claims**: declaration godoc lines 56-58 — *"APIFormat values for APIKey.APIFormat (custom provider only)."*
- **Reality**: grep `APIFormatOpenAICompatible` returns the declaration + a single test usage (`tester_test.go:244`). The dispatcher in `app/apikey/tester.go:103-108` checks `apiFormat == apikeydomain.APIFormatAnthropicCompatible` and falls through to the OpenAI-compatible branch on **any other value** — including empty string, the literal `"openai-compatible"`, and arbitrary frontend input. The exported constant has no role in dispatch. Handler input is pass-through (handlers/apikey.go:65,89): no whitelist check. Frontend dropdown likely uses the literal string, not the Go constant.
- **Severity**: LOW — small footprint (one exported const), but the semantic is unclear: "is this whitelisted?", "is `""` equivalent to `"openai-compatible"`?", "what does an unknown apiFormat do?" The dispatcher's silent fall-through lets typos like `"opanai-compatible"` succeed, which is consistent with §6 (no validation theater) — but then the existence of the constant suggests there's a check that doesn't exist.
- **Fix**: Either (a) delete the constant (and update the test to use the literal string `"openai-compatible"`), or (b) wire it in handlers/apikey.go's `Create` to validate `req.APIFormat` against a small allow-list (only `""`, `APIFormatOpenAICompatible`, `APIFormatAnthropicCompatible` legal). (a) matches §6 more closely; (b) makes the constant earn its keep. Pick (a).
- **Risk**: (a) zero (constant is decorative).

---

## LOW-2 — `apikeyapp.MaskKey` and `apikeyapp.IsValidProvider` are exported but only used in-package

- **Location**: `backend/internal/app/apikey/apikey.go:298-308` (MaskKey); `backend/internal/app/apikey/providers.go:127-130` (IsValidProvider).
- **Claims**: package doc apikey.go:2 — *"…and the MaskKey helper used only by this service."*
- **Reality**: grep `apikeyapp.MaskKey` and `apikeyapp.IsValidProvider` (excluding `_test.go`) return zero cross-package hits. The package doc itself says "used only by this service" yet the function is exported (`M`askKey). `IsValidProvider` is exported but only consumed at apikey.go:119 (same package). They could both be lowercase (`maskKey` / `isValidProvider`) without changing any external API.
- **Severity**: LOW — over-export of internal helpers. Doesn't change behavior; minor risk that future cross-domain code imports `apikeyapp.MaskKey` and creates a coupling that wasn't intended.
- **Fix**: Lowercase to `maskKey` / `isValidProvider` (in same file). Update test references if any. Or accept the over-export and remove the "used only by this service" claim from the package doc to match reality.
- **Risk**: Zero — symbol is unused outside the package.

---

## EDGE-1 — `catalogdomain.PerCollection` granularity is documented as future-reserved but already in the LLM prompt

- **Location**: `backend/internal/domain/catalog/source.go:39-44, 57-58`; mentioned at `backend/internal/app/catalog/generator.go:166`.
- **Claims**: source.go:14-16 — *"…PerCollection is reserved for future knowledge sources where each collection is a distinct namespace."*
- **Reality**: No `CatalogSource` implementation in production returns `PerCollection` from `Granularity()` (forge / skill / mcp use `PerItem` or `PerServer`). However, the LLM-driven Generator prompt at generator.go:166 already mentions *"source granularity=PerCollection: one mention per collection"* — telling the LLM about a granularity it'll never see in practice. This is benign noise (the LLM ignores irrelevant prompt sections) but it's the kind of inconsistency that drifts: when a future knowledge-base source IS added, the prompt and the enum are already in lockstep, but the `String()` switch case path is dead today.
- **Severity**: EDGE — the codebase is internally consistent under the "future-reserved" intent. Calling it dead would be wrong; the design choice is honest. Flagging because the mention in the LLM prompt could mislead someone into thinking a source already returns it.
- **Fix**: Optional. Either (a) leave as-is — the future-knowledge-base intent is documented; or (b) drop the PerCollection bullet from the Generator prompt template until a source actually emits the granularity. (b) is more conservative.
- **Risk**: None either way.

---

# Findings tally

- **3 HIGH** (HIGH-1/2/3) — registered-but-never-returned sentinels in apikey + todo domains; each creates a fictitious wire code in error-codes.md.
- **3 MED** (MED-1/2/3) — `errorsdomain.ErrInternal` ghost; `askapp.ErrAlreadyAnswered` self-acknowledged ghost; `apikeyapp.ListProviders` godoc lies about its callers.
- **2 LOW** (LOW-1/2) — `APIFormatOpenAICompatible` decorative constant; `MaskKey` / `IsValidProvider` over-exported.
- **1 EDGE** (EDGE-1) — `PerCollection` granularity future-reserved consistent but unused in practice.

# Notes on what I checked but found clean

For audit-completion bookkeeping, the following candidates were investigated and **not** flagged:

- `apikeydomain.{TestStatusPending,TestStatusOK,TestStatusError}` — all three used (apikey.go:104,213,217,280).
- `apikeydomain.SearchProviderPriority` — consumed by `app/tool/web/search.go:230` (BYOK iteration).
- `apikeydomain.ErrNotFound` / `ErrNotFoundForProvider` / `ErrInvalidProvider` / `ErrBaseURLRequired` / `ErrAPIFormatRequired` / `ErrKeyRequired` — all returned in production paths.
- `apikeyapp.TestMethod{*}` (6 constants) — all referenced in providers.go map and tester.go switch.
- `apikeyapp.GetProviderMeta` / `ProviderMeta` / `ProviderCategory` / `Category{LLM,Search}` — all consumed.
- `apikeyapp.{TestResult, ConnectivityTester}` — TestResult is the API response shape (handler:195,207); ConnectivityTester is Service's port (apikey.go:37, NewService).
- `apikeyapp.HTTPTester.Test` panic default branch (tester.go:111-122) — explicitly justified ("config-time complete-set invariant"), correct §S3 behavior.
- `askapp.{ErrNoPendingQuestion, ErrTimeout}` — both returned in production (Resolve, Wait).
- `askapp.Service.pendingCount` — lowercase = package-private, used by tests in same package; OK.
- `modeldomain.{ScenarioChat, ScenarioWebSummary}` — both used (model.go:112,128); fallback chain (web → chat) wired in `pkg/llmclient`.
- `modeldomain.{ErrNotConfigured,ErrInvalidScenario,ErrProviderRequired,ErrModelIDRequired}` — all returned.
- `modeldomain.IsValidScenario` — consumed (model.go:69).
- `modeldomain.ListScenarios` — godoc honestly claims test-only and it is test-only; not flagged.
- `tododomain.{StatusPending,InProgress,Completed,Deleted}` + `IsValidStatus` + `ListStatuses` — Status constants are read in publish path; ListStatuses honest about test-only; remaining sentinels (NotFound / SubjectRequired / InvalidStatus) all returned.
- `convdomain.{Conversation,ListFilter,ErrNotFound,Repository}` — all consumed.
- `convdomain.Conversation.{AutoTitled,SystemPrompt}` — both read (chat/runner.go:144,192,246).
- `cryptodomain.Encryptor` — consumed by `app/apikey.Service`.
- `errorsdomain.ErrInvalidRequest` — consumed by pagination + apikey handler. (Only `ErrInternal` is dead, see MED-1.)
- `eventlogdomain.*` — `ValidateEvent` invoked by `infra/eventlog/bridge.go:117`; all 5 event types and 6 block types referenced; `ErrSeqTooOld` consumed by handlers; `ErrInvalidEvent` wraps producer-side bugs (logged at `pkg/eventlog`).
- `notificationsdomain.*` — `ValidateEvent` invoked by `infra/notifications/bridge.go:86`; `ErrSeqTooOld` consumed by handlers/notifications.go:77; `ErrInvalidEvent` returned by Bridge.Publish on validation, swallowed-but-logged by `pkg/notifications.publisher` (publish failures are observability-class, not domain errors).
- `skilldomain.{MaxBodyBytes,MaxDescriptionChars}` — both consumed in scan / activate / mutate / import paths.
- `skilldomain.Skill.{Source,LoadedAt,DirPath,BodyPath,Description,Frontmatter}` — JSON-marshalled to UI via `GET /api/v1/skills`; all alive.
- `skilldomain.Frontmatter.{WhenToUse,Paths,ArgumentHint,Model,UserInvocable}` — package doc explicitly documents these as "V1 parses but ignores"; cross-vendor frontmatter preservation is documented intent, not dead code.
- `infra/store/apikey/Store.{Get,List,GetByProvider,Save,Delete,UpdateTestResult}` — all consumed by `app/apikey.Service`.
- `infra/store/conversation/Store.{Save,Get,List,Delete}` — all consumed by `app/conversation.Service`.
- `infra/store/model/Store.{GetByScenario,List,Upsert}` — all consumed by `app/model.Service`.
- `infra/store/chat/Store.*` — chat domain is out of audit scope; only confirmed `chatstore.GetAttachment:397` constructs a raw error string instead of wrapping a sentinel, but that's a chat-domain finding for the chat fork, not this one.
