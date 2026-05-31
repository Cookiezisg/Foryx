# Workflow-Revamp Implementation Log

Per-milestone work log: end-to-end推演, chosen/rejected approaches + why, design-gap root resolutions, pitfalls. Strategy in [`docs/superpowers/specs/2026-05-31-durable-engine-implementation-strategy.md`](../../superpowers/specs/2026-05-31-durable-engine-implementation-strategy.md). ADRs in `docs/decisions/`.

Impl owner: Claude (full take-over per `/goal` 2026-05-31). Design authority over 00–17. Root-fix over patch.

---

## 2026-05-31 — Kickoff: review → take-over → play locked

**Context.** Took over after a 5th adversarial review (49-agent fan-out + per-finding verification) put "照着建" readiness at **4/10**: 1 blocker + ~15 majors, concentrated in the journal/replay-key geology and an incomplete DRY consolidation, not in missing code. Goal directive: resolve gaps **from the root** (amend 17/00 + ADR), never patch the call site.

**End-to-end推演 of the engine** (the spine I'm building toward): `trigger fires → trigger_firings inbox (persist-before-act) → single dispatcher claims (single-tx) + StartRun → interpreter walks pinned graph from trigger node → each agent/tool node = activity (node_started → execute → node_completed journaled) → case reads journaled payload, picks branch (branch_taken) → fork-join awaits active in-edges → approval parks (signal_awaited + approvals row), resumes on signal_received → terminal → flowrun completed`. Crash anywhere → boot replays from journal (copy journaled results, stop at first un-journaled), parked approvals resume at their wait point. `:replay` a failed run → generation++, re-run only highest-gen failures.

**Engine shape — chosen: refactor-in-place onto the durable spine** (ADR-016). Reshape `app/scheduler` topo-walk → structural durable interpreter; collapse 14 dispatchers → 5-node activities; CEL replaces text/template; amend `workflow` graph model; new journal/approval/trigger stores. **Rejected:** (a) greenfield parallel engine + big-bang cutover — pre-launch, no data to preserve (CANON-MIGRATION), big-bang violates "each phase delivers value"; (b) bolt journaling under the old topo-walk — keeps the message-queue-era abstractions the revamp killed.

**Why M0 (contract) is not code.** The journal table, its record-once keys, and `iteration_key` are the geology every later milestone TDDs against. You cannot write a record-once dedup test on a contract whose `17`§1 declares a blanket `UNIQUE(...,type)` that the same doc's §2 says must not apply to `node_failed`. So M0 makes `17`§1/§7 the complete, typed, internally-consistent source + ADRs, then M1+ build on solid ground. Behavioral gaps (join-skip, polling-dedup, handler-state, agent-host, continue-as-new) are deferred to their milestones and resolved JIT — not over-designed ahead of the code that teaches them.

**Root resolutions locked for M0** (detail in spec §2; ADRs 016–022):
- **R1/ADR-017** `iteration_key` = enclosing loop header's back-edge traversal ordinal at activation, computed by the deterministic walk (pure function of walk position, can't drift), 1-D (nested loops rejected). Closes the "geology undefined" major.
- **R2/ADR-018** unify the record-once mess: one computed `dedup_key TEXT NOT NULL` + one partial unique index `WHERE type NOT IN ('node_started','node_failed')`. Dodges SQLite's NULL-distinct-in-unique trap a naive `turn/tool_call_id` index would hit. Closes the §1-vs-§2 contradiction + agent_step 3-way key inconsistency.
- **R3/ADR-019** one state principle: a step's current state = its highest-generation record-once event. Resolves replay copy-hit-vs-write-key + the failures-query predicate together.
- **R4/ADR-020** `pinned_callables` = transitive forge-callable closure at StartRun (depth ≤ 2). Fixes the blocker + `02:32`'s "无 pin" A-5 contradiction.
- **R5/ADR-021** mandate single-tx claim; delete the deadlock-prone two-step fallback.
- **R6/ADR-022** trigger retry is schedule-level: `trigger_schedules.retry_policy` + `consecutive_failures`; deactivate reads the counter.
- **R7** `workflows.concurrency` already exists (old field) — doc-completeness, not a gap (resolved by reading code).
- **R8** rewrite `17`§1 complete+typed; **delete** stale schema copies in `00`/`11` (finish the DRY consolidation the contract claimed).
- **R9** field-name DRY: `signal_awaited` (not `awaiting_signal`) for the event; add `allowReason`; polling interval on `function_versions` (drop `intervalSeconds`); timer-gate documented on all non-trigger nodes.

**Surfaces confirmed solid** (attacked in review, held — building on them as-is): wall-clock determinism (CEL no-now + journaled deadlines), single-writer seq monotonicity, cron dedup idempotency, boot (c)-before-(d) ordering, approval timeout↔decision first-wins (both `signal_received`), A-5 parked-resume version consistency, field-name canon for `agentRef`/`callable`/`yes`-`no`.

**Pitfalls noted.** Explore A's "keep 14 dispatchers verbatim" is wrong for the revamp (5-node collapse + CEL + active-branch join change them) — corrected. SQLite treats NULLs as distinct in unique indexes → drove the `dedup_key`-column design (R2).

Next: write ADRs 016–022, rewrite `17`§1/§7, delete stale copies, fix 02/05/16, then M1 (journal foundation, TDD).

### 2026-05-31 — M0 DONE

**Done.** ADRs 016–022 written + registered (`docs/decisions/README.md` new "Durable Engine Implementation" section). `17` rewritten as the complete typed single-source-of-truth: §1 all tables with columns/types/constraints/indexes (flowruns, flowrun_events[+`dedup_key`], approvals[+`cancelled`], trigger_schedules[+`retry_policy`/`consecutive_failures`], trigger_firings[`status` w/ `shed`, no `outcome` column], polling_states, workflows/function_versions added columns) + the record-once partial-index spec + §2 (dedup_key compare-and-insert) + §4 (highest-generation principle) + §6 (single-tx claim, polling dedup=`(cursor_in,segment_index)`, trigger auto-deactivate) + §7 (timer-gate on non-trigger nodes, `allowReason`, polling spec `{functionRef}`) + §9 (`signal_awaited` event). Stale schema copies in `00`/`11` **deleted** → pointers to 17. Fixed `02:32` (A-5 pin), `02` timer-gate, `05` `signal_awaited` event name (×4), `16` (WP9 owned trade-off, A-2→ADR-018 landing, C-1 tick has-seq).

**Verification (M0 = docs, no code; consistency grep):** blanket `UNIQUE(...,type,generation)` gone from all docs; `intervalSeconds` only in "removed" notes; `05` remaining `awaiting_signal` are all the `flowruns.status` value (events = `signal_awaited`); no residual column-list schema copies in `00`/`11`; `17` has no stale `outcome` column. The §1-vs-§2 record-once contradiction (the recurring 4-round footgun) and the agent_step 3-way key inconsistency are closed by ADR-018's one `dedup_key` + one partial index.

**Outcome:** the lone blocker (pin-closure, ADR-020) + the geology majors (iteration_key R1, record-once R2, replay-reset R3) + DRY incompleteness are resolved at the contract level. `17` is now buildable-by-following. Readiness on the M1–M2 foundation moves from 4/10 toward implementable; remaining behavioral resolutions (join-skip M3, polling-dedup M5 detail, handler-state M7, agent-host M7, continue-as-new M6) deferred to their milestones with JIT ADRs (stances locked in spec §2).

**Decisions taken solo (per async authorization), notable:** (a) collapse `trigger_firings.status`+`outcome` into one enum (drift-7 root fix — two overlapping enums was the smell); recorded inline in `17`§1, no separate ADR (column-shape cleanup). (b) `dedup_key` as a computed NOT NULL column rather than a composite unique over nullable `turn`/`tool_call_id` — driven by SQLite's NULL-distinct-in-unique rule (a naive index would silently break record-once for normal events).

Next: M1 — journal + schema foundation, TDD (record-once dedup via the partial index; seq monotonic; first-wins). First code milestone → use writing-plans for the M1 task breakdown, then failing-test-first.

### 2026-05-31 — M1 DONE (journal + schema foundation)

Plan: `docs/superpowers/plans/2026-05-31-m1-journal-schema-foundation.md`.

**End-to-end推演 (M1 scope):** the journal is the geology — before any interpreter (M2) can replay, the `flowrun_events` table + its record-once keying must exist and be provably correct. M1 builds exactly that, **strangler-fig (non-breaking)**: the old topo-walk scheduler keeps running on `paused_state`/`flowrun_nodes` until M2 deletes it.

**Shipped:**
- `FlowRun` amended **additively** (T1): +`pinned_callables`/`generation`/`trigger_node_id`, +`awaiting_signal` status; kept `paused_state`+`paused` for the old scheduler.
- Durable entities (T2–T3): `FlowRunEvent` (+`ComputeDedupKey`, ADR-018), `Approval` (+`cancelled`), `TriggerSchedule` (+`retry_policy`/`consecutive_failures`, ADR-022), `TriggerFiring` (`status` subsumes `outcome`), `PollingState`.
- Migrations + index (T4): `FlowRunEvent`/`Approval` registered (main.go + harness.go); record-once **partial unique** `idx_fre_record_once ON (flowrun_id, dedup_key) WHERE type NOT IN ('node_started','node_failed')` in `schema_extras.go`. `applySchemaExtras` is `HasTable`-guarded → subset migrations safe.
- Journal store (T5–T7, TDD): `flowruneventstore.AppendEvent` (per-flowrun seq in write-tx + compare-and-insert on dedup_key collision → first-wins) + `LoadJournal`. **3 correctness命脉 tests green:** record-once dedup, seq strict-monotonic + attempt-class append-many, signal_received first-wins (**proves the approval timeout↔decision double-fire the review suspected cannot happen** — both are `signal_received`, one dedup bucket).

**Verification:** `go build ./...` green; `make unit` green; 3 TDD tests green; `go vet` + `staticcheck` clean on M1 packages; `make mock` (pipeline baseline) — see commit.

**Decisions taken solo:**
- `AppendEvent` fills `ID` if empty (one place) rather than the plan's caller-side `NewEventID()` — matches the journal-writer ergonomics (the interpreter shouldn't gen IDs).
- Migrated only the flowrun-domain durable tables now; **deferred the 3 trigger-table migrations to M5** (entities defined; registered with their store+dispatcher when needed — avoids premature `triggerdomain` import in main.go for unused tables).

**Pitfall / pre-existing finding:** `make unit` surfaced `TestNodeTimeoutDuration_DefaultByType` (app/scheduler) red. Confirmed **pre-existing** (red at `3f5d16d`, before any M1 code) — it asserts per-type default timeouts the revamp removed (00 mechanism-vs-policy; `nodeTimeoutDuration` returns 0). Skipped with a documented reason (test + old scheduler deleted in M2). Not an M1 regression. SQLite NULL-distinct trap (ADR-018 motivation) validated in practice: the `dedup_key` string column avoids it.

Next: M2 — interpreter core (linear + crash-replay), TDD: the replay-determinism property test (same journal replayed twice = identical events; replay copies, never re-runs). Delete old scheduler state/pause/rehydrate/subdag + `PausedState`.

### 2026-05-31 — M2 IN PROGRESS (interpreter core proven; deletions + pipeline remain)

Plan: `docs/superpowers/plans/2026-05-31-m2-interpreter-core-replay.md`.

**Done (T1–T2, pushed):** `app/scheduler/interpreter.go` — the durable interpreter. One goroutine walks the pinned graph from the trigger node; each agent/tool node = an activity (`node_started` → `Router.Dispatch` → `node_completed`/`node_failed`); `Run`/`Resume` share one `walk` loop that consults the journal's `node_completed` results before each step (copy-hit, ADR-019) → replay copies, never re-dispatches. **The承重 invariant is proven by a passing property test** (`TestInterpreter_ReplayIsDeterministicAndCopiesNotReruns`): same journal replayed with a fresh interpreter ⇒ identical event sequence + the counting router's dispatch count does **not** increase. Reuses the M1 journal store + the existing `Dispatcher`/`Router` contract unchanged. Build + scheduler pkg tests + staticcheck (my files) green; coexists with the old scheduler (non-breaking).

**Scope held to linear (M2):** `iteration_key=0`, `generation=0`, single out-edge (`successor`), trigger = pass-through. `completedResults` keys on nodeID only for now — generalizes to highest-generation (ADR-019) when replay-reset (M6) + loops (M3) land; flagged in-code.

**Remaining (T3–T4, next):** rewrite `scheduler.go` `executeRun` → `Interpreter.Run` (thread real `flowrun.input` + pinned graph; single terminal status write); **delete** `state.go`/`pause.go`/`rehydrate.go`/`subdag.go` + `FlowRun.PausedState` + the paused repo methods + the `RehydrateOnBoot` paused-scan (grep all refs first); delete the 2 pre-existing staticcheck warnings' files along the way (`pause.go:198` SA4006, `loop_body_test.go` ST1012 — both in old-scheduler code T3 removes) + the M1-skipped stale `TestNodeTimeoutDuration_DefaultByType`; linear pipeline test (`test/durable/`) + full gate.

**Note:** the `Dispatcher` interface (`DispatchInput.ExecCtx *ExecutionContext`) is preserved; the interpreter passes `ExecCtx=nil` for now (fake router ignores it). Real-callable wiring + whether to keep a slim `ExecutionContext` shim vs delete it is resolved during T3 (the grep). The 14→5 node-type dispatcher collapse is M3, not M2.

### 2026-05-31 — M2-T3 scope refined (grep finding: ExecutionContext is dispatcher-entangled)

**Finding (T3 Step 1 blast-radius grep):** `ExecutionContext` is referenced by `dispatcher.go` + **all 13 `dispatch_*.go`** + **7 scheduler test files**. Deleting it in M2 would cascade a special-case rewrite through the entire dispatcher layer — which is precisely the **14→5 node-type collapse that the spec/plan scoped to M3**. Per the goal's triage ("一个改动要同时改 N 个文件 / 特例级联 → 回到设计层重新划界"), forcing it into M2 is the wrong cut.

**Decision (scope T3 surgically; design unchanged, just sequencing):**
- **KEEP in M2:** `ExecutionContext` (the `Dispatcher` contract) + `retry.go` + the 13 dispatchers. The interpreter will construct a **minimal `ExecutionContext`** per flowrun for real dispatchers (replacing `ExecCtx=nil`) so callable nodes work — no dispatcher edits.
- **DELETE in M2:** only the **topo-walk + pause-snapshot** machinery — `pause.go` (`driveLoop`/`runReadyLoop`/`pauseRun`/`continueRun`), the topo bits of `state.go` (`buildTopo`/`topoState`/`advance`; **keep `ExecutionContext`**, which also lives in `state.go` → split, not wholesale-delete), `rehydrate.go`, `subdag.go`; `FlowRun.PausedState` + `SetPausedState`/`ClearPausedState`/`ListPaused` (flowrun domain+repo+store) + the `RehydrateOnBoot` paused-scan (main.go); + the now-dead scheduler tests (`pause_test`/`state_test`/`timeout_dryrun`/the M1-skipped `TestNodeTimeoutDuration_DefaultByType`).
- **REWRITE:** `scheduler.go` `executeRun` → `Interpreter.Run` (thread `flowrun.input` + pinned graph; single terminal status write; drop `ExecuteFn`).
- **M3 inherits:** full `ExecutionContext` removal happens when the 14→5 collapse rewrites the dispatchers into the 5-node activity model.

This keeps M2's cut clean (engine swap, not dispatcher rewrite) and avoids dragging M3's collapse forward. The cutover surgery (this scoped T3 + T4 pipeline) is the next unit — multi-file, build-break-prone, best executed with fresh budget against the live files.

### 2026-05-31 — M2-T3 cutover, EVIDENCE-BASED execution plan (corrects the entanglement worry)

Read `state.go` + `dispatcher.go` + every `dispatch_*.go`. **Evidence corrects my earlier "ExecutionContext-entangled → defer to M3" worry:**
- Old engine's cross-node data flow is barely wired: `buildNodeInput` returns an **empty map**; `FunctionDispatcher` reads only static `Node.Config["functionId"]`/`["args"]`, **never `ExecCtx`**.
- Only **4** dispatchers touch `ExecCtx`: `condition`/`loop_parallel` (read `.Outputs`/`.Variables` — the deep flow, = M3 case/collapse), `llm` (`.Variables`, linear unused), `handler` (**`.Run.ID` only**). **7** dispatchers don't touch it (function/agent/approval/http/mcp/skill/wait).
- ∴ the **M2 linear path (trigger+function+handler) does NOT need the deep `ExecCtx`** — function ignores it, handler needs only `Run.ID`. So the cutover is M2-doable with **no journal→ExecCtx bridge patch**; `ExecutionContext` stays (the 4-dispatcher contract; removed in M3's collapse).

**Precise cutover steps (next unit, zero-rediscovery):**
1. `interpreter.step` passes `ExecCtx: &ExecutionContext{Run: &flowrundomain.FlowRun{ID: flowrunID}, Variables: map[string]any{}, Outputs: map[string]map[string]any{}}` (handler gets Run.ID; nil-safe empties for any reader). T1–T2 tests unaffected (countingRouter ignores ExecCtx).
2. `Service`: add `journal flowrundomain.JournalRepository` field + `SetJournal(j)` setter (non-breaking); wire in `main.go` + `harness.go` (construct `flowruneventstore.New(gdb)` + `SetJournal`).
3. Rewrite `executeRun`: empty graph → `finalizeRun(completed)`; else `New(s.journal, s.router).Run(ctx, run.ID, *graph)` → `finalizeRun(completed/failed)`. Drop `ExecuteFn` pluggability.
4. **Delete the old loop as one cut** (executeRun no longer references them → all become dead): `pause.go`, `rehydrate.go`, `subdag.go`; from `state.go` delete `topoState`/`buildTopo`/`initialReady`/`advance`/`dispatchBatch`/`recordNode`/`buildNodeInput`/`dispatchResult`/`nodeOnError`/`maxInt` (KEEP `ExecutionContext` + `newExecutionContext` + `finalizeRun`); from `retry.go` delete `dispatchWithPolicies` + `nodeTimeoutDuration` (the M1-skipped test + its fn die here). Resolve every U1000 unused.
5. `FlowRun.PausedState` field+struct + `ErrNotPaused`/`ErrApprovalNodeNotFound`/`ErrApprovalDecisionInvalid` sentinels deleted; `SetPausedState`/`ClearPausedState`/`ListPaused` off the flowrun repo port+impl; `Cancel`'s paused branch (scheduler.go L220–230) removed (running-only cancel; awaiting_signal resume = M4); `RehydrateOnBoot` paused-scan off main.go.
6. Delete/rewrite the now-dead scheduler tests: `pause_test`/`state_test`/`timeout_dryrun`/`loop_body`(uses ExecCtx loop) — keep `scheduler_test`/`dispatchers_capability` adjusted; fix `flowrun` domain+store tests referencing PausedState.
7. T4: linear pipeline test in `test/durable/`; full gate (unit+mock+staticcheck) green.

This is the old→new engine swap as one coherent build-green commit; `ExecutionContext` + the 4 deep-flow dispatchers (condition/loop/llm) carry to M3's 14→5 collapse where the deep data flow is rewired to journal scope-vars (§5) + CEL.
