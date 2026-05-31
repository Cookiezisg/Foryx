# Package audit summary: internal/app/tool/todo

## Spec understanding (Phase A — §S3 / §S9 / §S15 / §S16 / §S17)

- **§S3 错误不吞**: error suppression that hides user-visible failure / data loss / config drift is forbidden. The §S18 friendly tool_result pattern (errors.Is sentinel → human text via shared `classifyTodoErr`) is the canonical compliant form here — errors are surfaced as readable LLM-facing text, not swallowed. No bare `_ = err` exists in this package.
- **§S9 detached ctx 终态写**: terminal-state writes that MUST persist regardless of caller cancel use detached ctx. **N/A at the tool layer for todo**: the 3 mutating tools (Create / Update / Delete) all run mid-LLM-turn; if caller cancels the tool call, aborting the mutation is the **correct** semantic (no user-visible state to preserve). Unlike chat's "final assistant message after stream cancel" which IS terminal-state, todo mutations are in-flight steps. Service.Create / Update / Delete legitimately use the pass-through ctx. (Verified app/todo/todo.go does not use detached ctx, consistent with this analysis.)
- **§S15 ID 生成**: `<prefix>_<16hex>` via idgenpkg with `td_` prefix. Tool layer doesn't generate business IDs — `newID()` lives in `app/todo/todo.go::Service.Create` (`idgenpkg.New("td")` per §S15). Tools correctly defer.
- **§S16 错误 wrap 格式**: `fmt.Errorf("<pkg>.<Method>: %w", err)`. Package consistently uses `<ToolName>.<Method>:` literal prefix in its tools (`TodoCreate.ValidateInput:`, `TodoUpdate.Execute:`, `TodoList.Execute: marshal:`). Shared helper `marshalIndent` in create.go uses helper-style `marshal:` (mild deviation, WAIVE per established `<helper>:` precedent across app-tool-* packages — commit 64d9535). Two `errors.New("todo_id is required")` sites (get.go, update.go) are bare instead of using a domain sentinel — same EDGE LOW pattern as app-tool-mcp install_server.go #2 / uninstall_server.go #1, same WAIVE recommendation.
- **§S17 errmap 单一事实源**: every sentinel that can reach a handler must be in errmap.go. Package defines no local sentinels; consumes 3 `tododomain` sentinels (ErrNotFound, ErrSubjectRequired, ErrInvalidStatus) — all 3 verified registered errmap.go:96-98. The 4th `tododomain.ErrConversationMismatch` is intentionally **never** surfaced from Service (translated to ErrNotFound to prevent existence leak across conversations — documented at app/todo/todo.go:117-120) and therefore correctly absent from errmap.

## Files audited

| File | LOC | Sites | OK | POST-FIX | VIOLATION | EDGE |
|---|---|---|---|---|---|---|
| todo.go | 44 | 2 | 2 | 0 | 0 | 0 |
| create.go | 148 | 6 | 5 | 0 | 0 | 1 |
| get.go | 80 | 4 | 3 | 0 | 0 | 1 |
| list.go | 72 | 4 | 4 | 0 | 0 | 0 |
| update.go | 152 | 6 | 4 | 0 | 0 | 2 |
| **TOTAL** | **496** | **22** | **18** | **0** | **0** | **4** |

(Note: 496 LOC vs parent's stated 493 — small drift, doesn't affect audit.)

## Severity breakdown (only VIOLATION + EDGE)

| Severity | Count | Sites | Status |
|---|---|---|---|
| HIGH | 0 | — | — |
| MED | 0 | — | — |
| LOW (helper-style §S16 prefix `marshal:` instead of `todotool.marshalIndent:`) | 1 | create.go:#6 | FOUND |
| LOW (bare `errors.New("todo_id is required")` instead of domain sentinel — inconsistent with TodoCreate's `tododomain.ErrSubjectRequired` pattern) | 2 | get.go:#1; update.go:#1 | FOUND |
| LOW (hand-built JSON literal `fmt.Sprintf({"deleted":...})` instead of `marshalIndent` — output-format inconsistency, not functional bug) | 1 | update.go:#6 | FOUND |

(Counts: 1 + 2 + 1 = 4 EDGE; matches table above.)

## Cross-cutting

### Sentinel chain integrity (§S17)

- **Defined locally**: none (zero local sentinels — package only consumes `tododomain` sentinels).
- **Consumed from `tododomain`**: ErrNotFound (errmap.go:96), ErrSubjectRequired (errmap.go:97), ErrInvalidStatus (errmap.go:98) — **all 3 registered**, sentinel chain via `errors.Is` intact.
- **Intentionally unmapped**: `tododomain.ErrConversationMismatch` (4th sentinel) — Service translates to ErrNotFound on cross-conversation access to prevent existence leak (app/todo/todo.go:117-120 doc string + lines 133-134 / 165-168 implementation). Correctly absent from errmap because it never reaches a handler.

**No missing registrations**.

### Tool result anti-pattern audit (parent's specific concern)

Parent flagged "teaching-style result / impl-detail leak / self-promoting error" as anti-patterns (per app-tool-mcp summary). Findings for app-tool-todo:

| Pattern | Sites | Verdict |
|---|---|---|
| Teaching-style (LLM-to-user copy in tool result) | classifyTodoErr branches: "Todo not found in this conversation." / "Allowed: pending, in_progress, completed, deleted." | OK by §S18 — concise sentinel-classified text, no instructional prose |
| Impl-detail leak (file paths / package internals) | none — no leaks |
| Self-promoting (suggesting another product feature) | none — no suggestions |
| Suggested-question template | none |
| Tool description prose (§S18 description field) | TodoCreate / TodoList / TodoGet / TodoUpdate descriptions all read like clean usage docs | OK — consistent with mcp/forge tool description style |

**Verdict on the anti-pattern question**: package is **textbook-clean** for §S18 friendly-tool_result. No leaks, no self-promotion, no teaching prose. Cleaner than app-tool-mcp on this dimension (mcp had 4 LOW EDGE on path leaks; todo has zero).

### Detached ctx coverage (§S9) — N/A at this layer

All terminal-state-eligible writes are below this tool layer (in `app/todo/todo.go::Service`). However, the writes are NOT terminal in the §S9 sense — they're in-flight LLM step results. Caller cancel = abort mutation is the **correct** semantic for tool-driven todo CRUD. No detached ctx needed; verified app/todo/todo.go uses pass-through ctx and this is correct.

If TodoUpdate were ever extended to do post-cancel persistence (e.g. "mark as cancelled before tool aborts"), §S9 would apply. Currently it doesn't.

### Style consistency cross-check vs sibling packages

This package is **stylistically consistent** with app-tool-shell + app-tool-search + app-tool-forge + app-tool-mcp:

- Per-tool `<ToolName>.<Method>:` literal prefix in tool methods (slightly stricter than mcp's `<tool_name>:` helper-style — but compliant)
- Shared helper `marshal:` prefix in `create.go::marshalIndent` (helper-style, WAIVED by precedent)
- §S18 friendly tool_result pattern via `classifyTodoErr`
- Local validation via bare `errors.New` for "X required" (consistent with mcp install_server.go / uninstall_server.go pattern)

The 31 §S16 prefix LOW were WAIVED in app-tool-forge (commit 64d9535) per established precedent; same WAIVE applies here for the `marshal:` and `errors.New` deviations.

### Package structure (§S12 / §S13)

- **§S12 main file**: `todo.go` matches package name + holds package doc + factory + compile-time assertions. Compliant.
- **§S13 alias**: `todotool` (per nested sub-package alias rule for `app/tool/<sub>/`). Compliant.
- **§S18 9-method contract**: all 4 tools implement all 9 methods explicitly (no BaseTool inheritance). Compile-time assertions in todo.go:38-43 enforce. Compliant.
- **Standard fields**: `summary` / `destructive` / `execution_group` injection deferred to framework (`StripStandardFields` in `app/tool`). None of the 4 tool schemas defines these — correct per §S18 §2.

## Spot-check (random clean sites — verify mechanism not rubber-stamping)

5 sites picked from `OK` set across 4 files:

1. **create.go:#1** (ValidateInput): verified — `fmt.Errorf("TodoCreate.ValidateInput: %w", err)` prefix + `%w`; sentinel branch returns `tododomain.ErrSubjectRequired` (registered errmap.go:97). errors.Is chain intact through framework → handler if leaked.
2. **create.go:#5** (classifyTodoErr): verified — switch cases `errors.Is(err, tododomain.Err{NotFound,SubjectRequired,InvalidStatus})` cover all 3 registered sentinels; default returns `fmt.Sprintf("Todo %s failed: %v", op, err)` — `%v` is correct because output is friendly text content, not propagated error chain (consistent with app-tool-mcp call.go:#5 spot-check verdict).
3. **list.go:#1** (ValidateInput → nil): verified — schema at line 26-29 has empty `properties: {}`; nothing to validate. Compliant. (Same pattern as mcp list_marketplace.go:#1.)
4. **list.go:#4** (Marshal err wrap): verified — `fmt.Errorf("TodoList.Execute: marshal: %w", err)` full literal pkg.method prefix + sub-tag + `%w`. Stricter than the shared `marshalIndent` helper.
5. **update.go:#4** (Delete branch): verified — sentinel-classified friendly text on err; success path `fmt.Sprintf({"deleted":true,"id":%q})` is hand-built JSON but `%q` quote-escapes correctly per Go spec — well-formed JSON guaranteed even if `raw.TodoID` contains backticks/quotes. Logically correct, just style-inconsistent (audit logged as LOW EDGE site #6).

All 5 spot-checks confirmed correct classification — mechanism functioning, not rubber-stamping.

## Recommended fix priorities

1. **Bare `errors.New("todo_id is required")`** (LOW × 2 — get.go:#1, update.go:#1): EITHER (a) extract `tododomain.ErrTodoIDRequired` sentinel + register errmap.go (parallel to ErrSubjectRequired), OR (b) WAIVE per established mcp precedent. Audit-recommend (b) — bare error is acceptable in pre-Execute validation since framework intercepts before tool_result wire-up.
2. **`marshal:` helper-style prefix** (LOW × 1 — create.go:#6): WAIVE per established `<helper>:` prefix precedent (commit 64d9535). Optional uplift to `todotool.marshalIndent:`.
3. **Hand-built JSON literal in TodoUpdate delete branch** (LOW × 1 — update.go:#6): WAIVE-eligible. Optional: replace with `marshalIndent(struct{Deleted bool; ID string}{true, raw.TodoID})` for output-format consistency with sibling tools.

**Net assessment**: package is **§S3/S9/S15/S16/S17 clean** at the tool layer. 4 EDGE LOW are all stylistic / precedent-WAIVED; **0 HIGH or MED**. The §S18 friendly tool_result pattern is implemented textbook-correctly via the shared `classifyTodoErr` helper (3-sentinel switch + `%v` default for friendly text). Cleanest of the audited app-tool-* packages on the anti-pattern dimension (zero impl-detail leaks, zero self-promotion, zero teaching prose). Service-layer handles all `td_` ID generation + conversation-scoping (existence-leak-safe via ErrConversationMismatch → ErrNotFound translation), tool layer correctly stays thin.

## Out-of-scope notes (parent should verify)

1. **Service layer (`app/todo/todo.go`)**: not audited in this fork. Verified via spot-grep that it uses `td_` prefix via `newID()`, owns terminal writes via pass-through ctx (correct semantic for tool-driven mutation), and handles cross-conversation existence leak via ErrNotFound translation. Full app-todo audit should confirm publisher (`s.publish`) error handling and `newID()` panic-on-rand-fail compliance with §S15.
2. **`tododomain.ErrConversationMismatch` usage**: this 4th sentinel is intentionally never surfaced (translated to ErrNotFound). If a future code path forgets the translation and propagates it directly, errmap registration will be needed. Currently safe.
3. **TodoCreate / TodoUpdate `Metadata` field**: TodoCreate's struct in tool layer doesn't include `Metadata` (only Service.CreateInput has it); TodoUpdate's `updateRaw` also lacks it. If schema is ever extended to expose Metadata to LLM, schema + struct must be added in sync.
4. **`todo_test.go`**: per audit constraint, not read.
