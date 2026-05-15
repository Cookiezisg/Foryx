# Dead-Logic Audit — `app/subagent/` + `domain/subagent/`

**Scope**: `backend/internal/app/subagent/` (5 files) + `backend/internal/domain/subagent/` (1 file). Test files excluded.

**Method**: every non-test file read end-to-end; cross-referenced against all consumers via grep (no test-file callers counted as live); fields & methods traced to terminal readers, not just first hop.

**Tally**: 9 findings — 4 HIGH / 3 MED / 2 LOW + 2 EDGE.

---

## HIGH-1 — `Service.Cancel` + `activeRuns` map have zero production callers

- **Location**: `backend/internal/app/subagent/queries.go:19-28`; `backend/internal/app/subagent/subagent.go:73-74,101,151-158`.
- **Claims**: queries.go header — *"Cancel preempts a running sub-run via its registered cancel func."* subagent.go file header — *"Cancel (in queries.go) preempts an in-flight spawn."*
- **Reality**: grep `subagentService.Cancel` / `SubagentService.Cancel` returns **zero hits in non-test code**. The only consumers of `subagentapp.Service` are (a) `cmd/server/main.go` + `test/harness/harness.go` for construction, (b) `app/tool/subagent/agent.go` and `app/skill/activate.go` for `Spawn`. The `SubagentService` interface in `app/skill/skill.go:55` exposes only `Spawn`. There is no `/api/v1/conversations/{id}/subagent-runs/{id}:cancel` endpoint (router.go:62-68 confirms all subagent HTTP routes were deleted). Parent-cancel cascades naturally because `subCtx` is derived from `parentCtx` via `context.WithTimeout` — no external preemption needed. **`Cancel` is reachable only from tests; the entire `activeRunsMu` + `activeRuns` map + register/deregister-on-defer dance in `spawn.go:151-158` exists to feed a method nobody calls.**
- **Severity**: HIGH — three things compounding: the public `Cancel` method, the mutex+map state on `Service`, and 8 lines of register/cleanup boilerplate per Spawn. All justified by a removed HTTP surface. New maintainers reading `spawn.go` reasonably assume there's an external cancellation path and design around it.
- **Fix**: Delete `queries.go`. Remove `activeRunsMu` + `activeRuns` field from `Service` struct and the corresponding `make(map[...])` in `New`. Remove the register-cancel + defer-delete blocks in `spawn.go:151-158` (keep `cancel` for the `defer cancel()` of the timeout context — that's still needed). Update `subagent.go` file-header file list to drop `queries.go`. Update package doc removing the "Cancel" sentence in `app/subagent/subagent.go:33-39`.
- **Risk**: Zero. No production code path observes `Cancel` and parent-cancellation continues to work via ctx-derivation. Only test files (`subagent_test.go`) need an update if they exercise `Cancel` — out-of-scope for this audit (we don't read `_test.go`), but worth checking when fixing.

---

## HIGH-2 — `Registry.List()` is test-only

- **Location**: `backend/internal/app/subagent/registry.go:113-121`.
- **Claims**: godoc — *"List returns all registered types in stable Name order so the LLM description and any HTTP listing are deterministic across calls."*
- **Reality**: grep `\.List()` against the entire backend (excluding `_test.go`) finds zero callers of `Registry.List()`. The only call site is `subagent_test.go:51`. The doc invokes "LLM description" and "HTTP listing" — but (a) the LLM-facing description is a hardcoded `const subagentDescription` in `app/tool/subagent/agent.go:59` that enumerates types as a static string ("Available: Explore, Plan, general-purpose"); (b) HTTP listing was deleted with the `/api/v1/subagent-types` endpoint. Both rationales for `List` are gone; only `Get(name)` is still consumed (`spawn.go:87`).
- **Severity**: HIGH — exported method actively misleads about a feature ("HTTP listing", "LLM description") that doesn't exist. New code reading the package will reasonably believe there's a List endpoint to wire.
- **Fix**: Delete `Registry.List()` and the `sort` import (only used by `List`). If a future LLM-facing dynamic enumeration returns, re-add it then.
- **Risk**: Zero in production. Test breakage scoped to `subagent_test.go:51` (out of audit scope to read but mentioned for fix awareness).

---

## HIGH-3 — `SubagentType.DefaultModel` field + `SpawnOpts.Model` field are pure ghosts

- **Location**: `backend/internal/domain/subagent/subagent.go:69` (`DefaultModel string`); `backend/internal/app/subagent/spawn.go:57` (`Model string` in `SpawnOpts`).
- **Claims**: spawn.go:57 inline comment — *"\"\" = type.DefaultModel ?? PickForChat"*. The stale promise is that empty `opts.Model` falls back to `type.DefaultModel` then `PickForChat`.
- **Reality**: Neither field is ever **read** in production code:
  - `DefaultModel`: grep returns only the struct declaration and the stale comment. `builtInTypes` (registry.go:46-67) doesn't even **set** it for any of the 3 types. `Spawn` never references `typ.DefaultModel`.
  - `SpawnOpts.Model`: declared but never consulted. `Spawn` always calls `llmclientpkg.Resolve(parentCtx, s.modelPicker, s.keyProvider, s.llmFactory)` (line 97) which uses `PickForChat`. No branch ever inspects `opts.Model`. Both consumers (`agent.go:186`, `activate.go:107`) pass `SpawnOpts{MaxTurns: ...}` and `SpawnOpts{}` respectively — neither sets Model.
- **Severity**: HIGH — the two-line promise in the comment (`type.DefaultModel ?? PickForChat`) is a pure lie. A maintainer adding model-override logic via this ghost field will be surprised when their setting evaporates.
- **Fix**: Three coordinated edits:
  1. Drop `DefaultModel` from `subagentdomain.SubagentType`.
  2. Drop `Model` from `subagentapp.SpawnOpts`.
  3. Trim the misleading comment on `MaxTurns` line in `spawn.go:56` (the "0 = use type.DefaultMaxTurns" half is honest; just remove the Model line). If model override is desired later, add it then with real wiring through `Resolve`.
- **Risk**: Zero — neither field is read or written by anything (including tests, per grep — but verify when fixing).

---

## HIGH-4 — `SubagentType.Description` field set, never read

- **Location**: `backend/internal/domain/subagent/subagent.go:66` (`Description string`); `backend/internal/app/subagent/registry.go:48,55,62` (set in builtInTypes).
- **Claims**: SubagentType godoc (subagent.go:55-63) — *"...the LLM sees as the legal `subagent_type` argument values."* The Description field would naturally feed an LLM-visible enum description or HTTP catalog.
- **Reality**: grep `\.Description` filtered to subagent-package consumers returns zero reads. The descriptions written in `builtInTypes` (e.g. "Fast read-only search agent for locating code/files...") never reach the LLM — `app/tool/subagent/agent.go:71-88` defines `subagentSchema` as a hardcoded JSON schema where `subagent_type` is a plain string field with description "Available: Explore, Plan, general-purpose." (literal const). The richer Description text is dead bytes. Removed HTTP listing route would have been the other consumer; that's gone.
- **Severity**: HIGH — non-trivial English content lives in registry.go as if it's powering something. It isn't. New maintainers updating descriptions will see no behavioral change and reasonably conclude the catalog is broken.
- **Fix**: Either (a) drop the field + the 3 description strings; or (b) **wire it** — propagate to the JSON schema's `subagent_type.enum` + per-type description block, since the in-code text is genuinely good and the LLM would benefit. Fix (b) gives the schema one source of truth; fix (a) is mechanical cleanup. Pick one based on whether LLM should see per-type descriptions.
- **Risk**: Both options low. (a) zero behavioral impact (already dead). (b) nets a richer LLM enum schema; risk only if old prompts depend on the literal "Available: Explore, Plan, general-purpose." substring.

---

## MED-1 — `subagentdomain.RoleUser` / `RoleAssistant` / `RoleTool` / `RoleSystem` mirror constants are unreferenced

- **Location**: `backend/internal/domain/subagent/subagent.go:46-51`.
- **Claims**: section header — *"Mirrored from chatdomain.Role* — keep here for stable references in type system prompts that mention sub-run roles. Source of truth is chatdomain.RoleUser / RoleAssistant."*
- **Reality**: grep `subagentdomain.Role` — zero hits. The `SystemPrompt` strings in `builtInTypes` are plain English ("You are Explore...") containing no Go-constant references. No `fmt.Sprintf` / template substitution into system prompts. The "mirrored for stable reference" rationale never materialized — these constants reference nothing and are referenced by nothing.
- **Severity**: MED — dead constants in a domain file. Light footprint but the misleading comment about "type system prompts that mention sub-run roles" implies a wiring that doesn't exist.
- **Fix**: Delete the entire 4-const block + the section comment. If chat domain prompt-templating starts substituting these, re-add then.
- **Risk**: Zero. They're lexically unreferenced.

---

## MED-2 — `composeSystemPrompt` locale branch silently disagrees with claim

- **Location**: `backend/internal/app/subagent/subagent.go:154-162`.
- **Claims**: godoc — *"composeSystemPrompt prepends the standard Forgify subagent preamble + appends a locale hint (zh-CN only) to the type's system prompt."*
- **Reality**: This is **live code** — `Spawn` (line 170) calls it with the parent's locale. The behavior matches the claim. **However**, since reqctxpkg supports more locales than zh-CN, the "(zh-CN only)" framing reads like an explicit policy ("we never localize anything else") rather than a TODO. Reading the rest of the codebase, only `chat/runner.go` and `chat/host.go` apply locale hints, and both have the same `if locale == LocaleZhCN` shape. So the "(zh-CN only)" is a legitimate current scope, not a forgotten TODO. **Not dead logic; included as an EDGE that could be misread.** Reclassifying down — this is fine.
- **Severity**: LOW — the comment is accurate but underspecifies *why* zh-CN is the only branch. Maintainers seeing it might think it's a stub awaiting more languages.
- **Fix**: Either accept as-is (LOW noise) or expand comment to "...locale hint (V1.2 supports zh-CN; English is implicit/default)". Optional.
- **Risk**: None — code is correct.

---

## MED-3 — `subagentHost.WriteFinalize` accepts `blocks` parameter solely to satisfy interface, then `_ = blocks` silently

- **Location**: `backend/internal/app/subagent/host.go:81,138`.
- **Claims**: end-of-function inline comment — *"blocks param is unused — sub blocks were emitted in real-time and written to message_blocks via the Emitter dual-write."*
- **Reality**: True. The signature comes from `loop.Host` interface. The `_ = blocks` makes the lint silence explicit. **Not dead per se — interface compliance — but the parameter design is a vestige of pre-emitter loop.Host where the host was responsible for persisting blocks.** Compared with `chat/host.go::WriteFinalize`, that one almost certainly has the same `_ = blocks` (out of scope to verify here). The interface itself drags this dead parameter through every Host implementation. This is a `loop.Host` design issue, not a subagent issue — flagged here only because the inline acknowledgement "blocks param is unused" surfaces it.
- **Severity**: MED — within the `loop.Host` interface design. Not actionable inside `app/subagent/`. Mentioned for cross-package consideration. If `loop.Host` is reviewed in a future audit, dropping `blocks` from the signature simplifies all hosts.
- **Fix**: Out of scope for subagent audit. Track in a `loop.Host` audit.
- **Risk**: Out of scope.

---

## LOW-1 — File-header file list in `app/subagent/subagent.go:25-31` lists a soon-dead file

- **Location**: `backend/internal/app/subagent/subagent.go:25-31`.
- **Claims**: file-list block — `queries.go   — Cancel`.
- **Reality**: When HIGH-1 lands and `queries.go` is deleted, this block needs to drop the line. Currently accurate but would become stale after the HIGH-1 cleanup.
- **Severity**: LOW — only relevant after HIGH-1 fix.
- **Fix**: Update file-header file list when deleting `queries.go`.
- **Risk**: None.

---

## LOW-2 — `defaultRunTimeout` const uses doc-promised behavior that's structurally redundant

- **Location**: `backend/internal/app/subagent/spawn.go:36`, applied at line 148.
- **Claims**: godoc — *"defaultRunTimeout caps a single Spawn — defends against stuck tool calls (e.g. an MCP server that never returns) holding a sub-runner forever and burning tokens."*
- **Reality**: True and live. Five-minute hard cap is a real safety net. **Not dead.** Mentioned only because in conjunction with HIGH-1 ("no external Cancel"), this 5-min ceiling becomes the *only* preemption. Worth being clear in the doc, since the rationale shifts: previously "defense alongside external cancel"; post-cleanup "the only preemption". Cosmetic doc tweak when HIGH-1 lands.
- **Severity**: LOW — purely a comment-clarity touch-up tied to HIGH-1 fix.
- **Fix**: When applying HIGH-1, expand the godoc line to make explicit "this is now the only preemption — there's no external cancel API".
- **Risk**: None.

---

## EDGE-1 — `state.AddSubagentTokens` writes to a log nothing reads in production

- **Location**: `backend/internal/app/subagent/spawn.go:276-278`.
- **Claims** (in `pkg/agentstate/agentstate.go:67-71` — out of audit scope but caller is in scope): *"the conversation-detail UI / cost panel reads it..."*
- **Reality**: grep `SubagentTokenLog` returns zero non-test readers. The UI/cost panel HTTP endpoint never materialized. The write site at spawn.go:276-278 hits live code (`AddSubagentTokens`) which appends to a slice nobody reads. **The dead recipient lives in `pkg/agentstate/`, outside this audit's scope. The write call itself is technically fine.** Marked EDGE because the **call** is in our scope but the **dead-ness** is downstream.
- **Severity**: EDGE — the write looks alive when read in isolation; it's only dead viewed end-to-end.
- **Fix**: Out of scope here. When auditing `pkg/agentstate`, decide whether to delete `subTokens` + `AddSubagentTokens` + `SubagentTokenLog` or wire a real HTTP endpoint. If deleted there, `spawn.go:276-278` should drop too.
- **Risk**: Address in agentstate audit.

---

## EDGE-2 — `if parentToolCallID != "" && parentMsgID != ""` guard in Spawn looks defensive but is unreachable in current usage

- **Location**: `backend/internal/app/subagent/spawn.go:116`.
- **Claims**: implicit — guards against parent-cancel-before-our-emit / fresh ctx.
- **Reality**: Both production callers (`agent.go::Execute` via `loop/tools.go::runOneTool` injection of `WithToolCallID`; `skill/activate.go::Activate` via the same chain) reach `Spawn` with `ToolCallID` and `MessageID` always set — they're written by the loop before tool dispatch. So `parentToolCallID == ""` or `parentMsgID == ""` is unreachable through the production call graph. **However**, `Spawn` is exported on `*Service` and could be called directly from a test harness or future direct-invocation surface. The guard is genuinely defensive and cheap; the EDGE is purely "documented as required when actually unreachable in current usage". Keeping the guard is the right call. Flagged only so a reader understands "this branch never fires in production today" — important for diagnosing why a frontend never sees `MessageStart` for a sub-run if the production path ever changes.
- **Severity**: EDGE — code is correct; reader understanding nuance.
- **Fix**: None. Optionally extend the inline comment to say "in current production paths both are always set; the guard is for direct test invocation / future direct entry points".
- **Risk**: None.

---

## Summary

| ID | Severity | Location | One-line |
|---|---|---|---|
| HIGH-1 | HIGH | queries.go + activeRuns | `Service.Cancel` + map state has no production caller |
| HIGH-2 | HIGH | registry.go:113 | `Registry.List` test-only |
| HIGH-3 | HIGH | domain/subagent.go:69 + spawn.go:57 | `DefaultModel` + `SpawnOpts.Model` ghost fields |
| HIGH-4 | HIGH | domain/subagent.go:66 + registry strings | `SubagentType.Description` set, never read |
| MED-1 | MED | domain/subagent.go:46-51 | 4 unreferenced `Role*` mirror constants |
| MED-2 | LOW (downgraded) | subagent.go:154 | Locale comment underspecifies scope (cosmetic) |
| MED-3 | MED (cross-pkg) | host.go:81,138 | `_ = blocks` is loop.Host design vestige |
| LOW-1 | LOW | subagent.go:25-31 | File list will go stale after HIGH-1 |
| LOW-2 | LOW | spawn.go:36 | `defaultRunTimeout` doc rationale needs update post-HIGH-1 |
| EDGE-1 | EDGE | spawn.go:276 | Token log write feeds unread `pkg/agentstate` slice |
| EDGE-2 | EDGE | spawn.go:116 | Defensive guard unreachable in current callers |

Net actionable inside subagent packages: **HIGH-1, HIGH-2, HIGH-3, HIGH-4, MED-1**, plus the LOW-1/LOW-2 doc touch-ups when the HIGHs land. MED-3 + EDGE-1 belong to other-package audits. EDGE-2 + downgraded MED-2 are leave-alone.
