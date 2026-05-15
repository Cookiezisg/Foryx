# Phase B audit-1 — TODO/FIXME sweep

## Methodology

1. Searched backend/ for: TODO / FIXME / 留下次 / 改天 / 之后修 / 后面再 / 以后再 / 待修 / 待实现 / 稍后 / XXX / HACK / not yet / 暂未 / 尚未 / 还未 / 目前不
2. Searched documents/version-1.2/ for: same set + 待实现
3. Filtered out:
   - `todo` domain noise (the v1.2 mini-domain "todo" is an actual entity, not a deferred marker — `TodoCreate` / `TODO_NOT_FOUND` / `Todo 追踪` etc. are domain artifacts)
   - General "future" intent narratives in package docs (e.g. KMS envelope-encryption mention in `crypto/encryptor.go`, "rewrite later for real auth" in middleware/auth.go) — these describe forward-looking architecture, not pending bugs
   - Comments using "later" in narrative sentences ("@latest drift bites later", "if it later wants the image")
4. Read 50 lines of context per finding; ran `git log` where dates were unclear

Result: very clean codebase. Only **one** backend Go TODO-style marker remains (already §S20-annotated). Bulk of findings live in docs.

---

## Per-finding

### [1] Anthropic 5MB image guard deferred to future context-optimizer layer
- **Location**: `/Users/SP14921/Documents/Personal/PersonalCodeBase/Forgify/backend/internal/infra/llm/anthropic.go:271-285`
- **Marker text**:
  ```
  // NOTE — Anthropic enforces a 5 MB per-image limit (decoded bytes); over
  // the limit returns 400 AND poisons the conversation history... Per §S20
  // deferred WITH justification:
  // (a) structural constraint — fix belongs to a layer that doesn't yet
  // exist; (b) explanation — wire-layer guard would double-process or
  // conflict with the upcoming optimizer.
  ```
- **Context**: Comment block immediately above `buildAnthropicUserMsg`. Author flags that >5MB images poison the entire conversation (subsequent 400s) but argues the fix belongs to a context-optimizer layer (image resize / summary replacement at history level) which doesn't yet exist; wire-layer guard would conflict when optimizer arrives.
- **Why deferred (claimed)**: Wire-layer guard would double-process or conflict with future optimizer's resize/swap behavior.
- **§S20 evaluation**:
  - (a) Structural hard constraint? **YES** — fix sits at history-level transformation layer (image resize / summary replacement) that doesn't exist; adding wire-layer guard now requires reverting when optimizer ships.
  - (b) On-the-spot explanation? **YES** — comment explicitly cites §S20, names the missing layer, and explains user-scenario blast radius (whole conversation poisons after one oversized image).
- **Classification**: **VALID-DEFER**
- **Severity**: LOW (already correctly justified per §S20)
- **Recommended action**: keep as-is. This is the textbook example §S20 envisions.

---

### [2] event-log-protocol.md §B3 V2 placeholder TODO sweep listed as Phase 3 acceptance
- **Location**: `/Users/SP14921/Documents/Personal/PersonalCodeBase/Forgify/documents/version-1.2/event-log-protocol.md:1033` and `:1214`
- **Marker text**:
  ```
  - [ ] V2 placeholder TODO 三连清（§B3）
  ```
  and decision-row "V2 placeholder 三连 | 清（§B3）"
- **Context**: Phase 3 acceptance criteria checkbox in event-log-protocol implementation plan. progress-record dev log 2026-05-08 says "§B3 V2 placeholder 经审计实为 design note 不是 TODO，无需清理" — meaning the audit already concluded this item is a non-issue, but the unchecked box was never updated in this plan document.
- **Why deferred (claimed)**: progress-record states it was audited and ruled "not actually a TODO — design note misclassification".
- **§S20 evaluation**:
  - Not applicable — this is a planning checkbox, not deferred code. But the doc is now inconsistent: it shows an unchecked Phase 3 item that progress-record says is done/N-A.
- **Classification**: **STALE** (doc plan checkbox out of sync with audit result)
- **Severity**: LOW (cosmetic doc drift; misleads only if someone reads the plan without the dev log)
- **Recommended action**: tick the box and add note "audited 2026-05-08 — design note, no actual TODO", or strike the row. Not code-blocking.

---

### [3] chat.md claims `app/loop/loop.go` has a TODO hook point for context compaction — file no longer has it
- **Location**: `/Users/SP14921/Documents/Personal/PersonalCodeBase/Forgify/documents/version-1.2/service-design-documents/chat.md:994`
- **Marker text**:
  ```
  - 长对话 context compaction（`app/loop/loop.go` 已预留 TODO 钩子点）：超长时压缩历史，保留关键消息
  ```
- **Context**: Phase 5 future-work entry asserting loop.go has reserved a TODO hook point for context compaction. I read all of `backend/internal/app/loop/loop.go` and `stream.go`: **no TODO marker, no compaction hook, no placeholder function** exists for compaction. The current loop body cleanly runs the ReAct cycle without any commented-out compaction stub.
- **Why deferred (claimed)**: Phase 5 (intelligence) feature — long-conversation history compression. Doc says "已预留 TODO 钩子点".
- **§S20 evaluation**:
  - Not §S20 territory — this is a doc-vs-code consistency issue, not deferred code. Doc is **lying about code state**.
- **Classification**: **DOC-ONLY** (lying — sub-class: stale doc claim)
- **Severity**: LOW (a future-feature paragraph; misleads reader about loop.go shape but doesn't block anything)
- **Recommended action**: drop the "已预留 TODO 钩子点" clause or rewrite to "Phase 5 will add a hook point in app/loop/loop.go".

---

### [4] 01-agent-loop.md cites `stream.go:64` / `tools.go:29` in chat package — those files moved to app/loop/
- **Location**: `/Users/SP14921/Documents/Personal/PersonalCodeBase/Forgify/documents/version-1.2/adhoc-topic-documents/claude-code-research-documents/01-agent-loop.md:242,248`
- **Marker text**:
  ```
  > 现有实现：`backend/internal/app/chat/runner.go` 中 `agentRun` 单层 for+`maxSteps=20`，
  > `stream.go:64` 已留 TODO(A1) 标记 mid-stream 触发；`tools.go:29` `runTools` 一律
  > goroutine 全并行；无 stop hook、无 retry、无 context shaper。
  ```
  and table row 3 says "TODO(A1) 已留位置 | ... 在 `stream.go:64` 把"args 完整即推到执行池"做掉".
- **Context**: Research doc comparing Claude Code's agent loop to Forgify's. References `chat/stream.go:64` and `chat/tools.go:29`. **Both files no longer exist** in `app/chat/` — they were moved to `app/loop/` (current `app/chat/` contains only chat.go / runner.go / host.go / history.go / util.go). I grepped both `app/loop/stream.go` and `app/loop/tools.go`: **no `TODO(A1)` marker exists** in either.
- **Why deferred (claimed)**: Research doc lists A1 (mid-stream tool execution) as P2 improvement target.
- **§S20 evaluation**:
  - Not §S20 territory — research/comparison doc, not code commitment. But it doubly-lies: about file location AND about a TODO(A1) marker that doesn't exist in any version of the code I can find.
- **Classification**: **DOC-ONLY** (lying — also: research doc references stale file paths)
- **Severity**: LOW (research doc, not authoritative; but misleads anyone using it to scope work)
- **Recommended action**: this is a research note not a contract — either (a) add stale-as-of-DATE banner at top, or (b) update the two line refs to current file paths in app/loop/. Don't auto-fix without user sign-off; the research has informational value beyond the line numbers.

---

### [5] agent-core-upgrade.md says `// TODO: context compaction` was placed in code — code doesn't have it
- **Location**: `/Users/SP14921/Documents/Personal/PersonalCodeBase/Forgify/documents/version-1.2/adhoc-topic-documents/claude-code-research-documents/agent-core-upgrade.md:525`
- **Marker text**:
  ```
  - 本轮**不做** A1（mid-stream 工具执行），`// TODO: context compaction` 注释占位
  ```
- **Context**: Upgrade plan retrospective. Claims the 2026-04 chat refactor left `// TODO: context compaction` placeholder. I grepped `app/loop/` and `app/chat/` — **no such comment exists**. Either the placeholder was removed in a subsequent cleanup (per §S20-like discipline) or it was never landed.
- **Why deferred (claimed)**: A1 mid-stream tool execution + context compaction reserved for later phase.
- **§S20 evaluation**:
  - Not §S20 territory — retrospective doc describing past work. But it lies about residual code state.
- **Classification**: **DOC-ONLY** (lying about code state)
- **Severity**: LOW
- **Recommended action**: drop the "注释占位" clause from line 525 (sentence still works: "本轮不做 A1（mid-stream 工具执行）"). Or update if the placeholder was intentionally removed.

---

### [6] database-design.md states forge_executions 300-row eviction is a TODO
- **Location**: `/Users/SP14921/Documents/Personal/PersonalCodeBase/Forgify/documents/version-1.2/service-contract-documents/database-design.md:117`
- **Marker text**:
  ```
  主键 `fe_<16hex>`；无软删；表行无主动 eviction（**TODO**：300 条/forge 上限策略待实现，
  原文档曾设此目标但 store 层无 TrimExecutions / 触发器；当前由用户手动 GC / 调试需要时清理）。
  ```
- **Context**: Database design contract document. Says 300-rows-per-forge cap was a documented target but the store layer never implemented `TrimExecutions` / a trigger; current behavior is user-manual cleanup. Located in the `forge_executions` table description.
- **Why deferred (claimed)**: Implementation gap. No structural-constraint justification given.
- **§S20 evaluation**:
  - (a) Structural hard constraint? **NO** — `TrimExecutions` is a self-contained store-layer method or a `CREATE TRIGGER` in `schema_extras.go`. No cross-domain dependency, no missing layer.
  - (b) On-the-spot explanation of why it can't be done now? **NO** — the doc says "当前由用户手动 GC / 调试需要时清理", which acknowledges the gap but doesn't justify deferring per §S20.
- **Classification**: **LANDMINE** (per §S20, this is禁止理由 #2 / #3 territory — "no one has hit it yet" / "we know where to fix it")
- **Severity**: MED — for single-user local dev usage, an unbounded table is mostly noise (DB stays small), but Phase 5 dogfooding of test-batch runs could rack up thousands of rows per forge over weeks. SQLite will keep working but disk grows + slow `idx_fe_forge_created` scans on filterless `kind=run` lists.
- **Recommended action**: either (a) implement the 300-row trim (one-shot store method `TrimExecutions(forgeID, limit)` called from Service.Create after insert, or a `CREATE TRIGGER` in schema_extras.go), or (b) explicitly drop the cap from the design doc and acknowledge "unbounded by design — manual cleanup". Per §S20 the default is (a).

---

### [7] forge.md table entry `accept_pending / reject_pending` marked TODO Phase 6+
- **Location**: `/Users/SP14921/Documents/Personal/PersonalCodeBase/Forgify/documents/version-1.2/service-design-documents/forge.md:1118`
- **Marker text**:
  ```
  | accept_pending / reject_pending | （**TODO** Phase 6+）：HTTP handler 在状态变更后调 bridge.Publish |
  ```
- **Context**: In the "触发点" (trigger points) table for forge SSE entity-state events. Says HTTP accept/reject pending → bridge.Publish wiring is deferred to Phase 6+. Sits next to row 1119 "HTTP CRUD（POST/PATCH/DELETE）| **MVP 暂不广播**——单用户单窗口；多窗口同步留待后续".
- **Why deferred (claimed)**: Phase 6+ scope; MVP single-user-single-window means HTTP CRUD doesn't need broadcast yet.
- **§S20 evaluation**:
  - (a) Structural hard constraint? **PARTIAL** — accept/reject HTTP handlers exist; the broadcast wiring itself isn't blocked by any missing layer. But the surrounding row 1119 establishes the design principle "MVP 暂不广播 because 单用户单窗口"; reading the two rows together, the deferral is anchored to a stated MVP scope choice.
  - (b) On-the-spot explanation? **PARTIAL** — neighbor row explains the rationale; this row inherits it but doesn't cite §S20 directly.
- **Classification**: **VALID-DEFER** (borderline; saved by the row-1119 context)
- **Severity**: LOW
- **Recommended action**: optional — add "(per row above: MVP 单窗口不广播)" cross-reference to make the deferral self-evident without context-reading.

---

### [8] forge_redesign/plans/05-execution-plane.md cites "SchedulerForwarder interface 待实现"
- **Location**: `/Users/SP14921/Documents/Personal/PersonalCodeBase/Forgify/documents/version-1.2/adhoc-topic-documents/forge_redesign/plans/05-execution-plane.md:11`
- **Marker text**:
  ```
  **关联**:[`05-execution-plane.md`](../05-execution-plane.md) 完整 spec / [`04-workflow.md`](../04-workflow.md)
  (scheduler 接 workflow active version)/ Plan 04 的 `SchedulerForwarder` interface 待实现。
  ```
- **Context**: This is a *future* plan document for the forge_redesign initiative (Phase 4+ workflow/execution-plane redesign). The phrase "待实现" refers to a Plan 04 deliverable that Plan 05 depends on — both plans are forward-looking implementation plans, not active TODOs sitting in the live codebase.
- **Why deferred (claimed)**: Plan 04 dependency for Plan 05; documents the implementation order.
- **§S20 evaluation**:
  - Not §S20 territory — this is a plan document explicitly describing future work sequencing; "待实现" is informational, not a deferral against existing code.
- **Classification**: **DOC-ONLY** (forward-looking plan doc, legitimate use of "待实现")
- **Severity**: LOW
- **Recommended action**: none.

---

### [9] 05-subagent.md ⚠️ note about Claude Code worktree bug "是 bug, 待修"
- **Location**: `/Users/SP14921/Documents/Personal/PersonalCodeBase/Forgify/documents/version-1.2/adhoc-topic-documents/claude-code-research-documents/05-subagent.md:145`
- **Marker text**:
  ```
  ⚠️ 已知 issue（#47548）：`isolation: "worktree"` 在某些 git 配置下会切父 worktree 的
  branch 而不是建新 worktree——是 bug，待修。
  ```
- **Context**: Research note about Anthropic's Claude Code CLI (issue #47548). This is upstream-vendor info, NOT a Forgify deferral.
- **Why deferred (claimed)**: N/A — observation about Claude Code upstream.
- **§S20 evaluation**:
  - Not applicable — describing upstream vendor bug, not Forgify code.
- **Classification**: **DOC-ONLY** (legitimate research observation)
- **Severity**: LOW
- **Recommended action**: none.

---

### [10] progress-record 2026-05-01 entry: "TODO 扫描：全代码仅 3 处 TODO"
- **Location**: `/Users/SP14921/Documents/Personal/PersonalCodeBase/Forgify/documents/version-1.2/progress-record.md:167`
- **Marker text**:
  ```
  | 2026-05-01 | **[review]** TODO 扫描：全代码仅 3 处 TODO 全是合法前瞻性标记
  (A1 中流执行 / context compaction 钩子点)，无历史包袱 |
  ```
- **Context**: A historical dev log entry citing 3 TODOs (A1 / context compaction). This audit (2026-05-11) finds only 1 TODO-style marker in backend Go code (the §S20-annotated Anthropic 5MB one), zero referring to A1 or context compaction. Suggests the prior TODOs were cleaned up in subsequent commits without updating this dev log entry — but dev logs are historical records and shouldn't be retroactively edited per §S19, so this is fine.
- **§S20 evaluation**: N/A — historical log.
- **Classification**: **DOC-ONLY** (historical log entry; should not be edited)
- **Severity**: NONE
- **Recommended action**: none.

---

### [11] sandbox.md mentions §S20 enforcement (no deferred work)
- **Location**: `/Users/SP14921/Documents/Personal/PersonalCodeBase/Forgify/documents/version-1.2/service-design-documents/sandbox.md:151`
- **Marker text**: References §S20 as design justification ("遵守 §S20'留下次'= bug")
- **Context**: This is sandbox.md citing §S20 as justification for *removing* unused EnvManagers — the opposite of a TODO. Not a deferred marker.
- **Classification**: **DOC-ONLY** (legitimate §S20 reference)
- **Severity**: NONE
- **Recommended action**: none.

---

### [12] subagent.md note about Task→Subagent rename
- **Location**: `/Users/SP14921/Documents/Personal/PersonalCodeBase/Forgify/documents/version-1.2/service-design-documents/subagent.md:20`
- **Marker text**: Historical rename note mentioning "task mini-domain (TaskCreate/List/Get/Update 管 TODO)" — domain artifact, not a deferred marker.
- **Classification**: **DOC-ONLY** (rename history)
- **Severity**: NONE
- **Recommended action**: none.

---

### [13] progress-record 2026-05-07 entry — TE-25 留下次 + 理由 (Anthropic 5MB)
- **Location**: `/Users/SP14921/Documents/Personal/PersonalCodeBase/Forgify/documents/version-1.2/progress-record.md:385`
- **Marker text**: Documents the same Anthropic 5MB image deferral as finding [1], records §S20 (a)+(b) justification at commit time.
- **Classification**: **VALID-DEFER** (companion record to finding [1])
- **Severity**: NONE
- **Recommended action**: none.

---

### [14] progress-record 2026-05-08 dev log "**未做**" subsection (Phase 4+)
- **Location**: `/Users/SP14921/Documents/Personal/PersonalCodeBase/Forgify/documents/version-1.2/progress-record.md:396`
- **Marker text**:
  ```
  **未做**（Phase 4+）：subagent_runs/messages 表 backfill+drop（依赖前端切到新 bridge）；
  subagent.go file split（pure refactor，价值低）；前端 chat.js 切新 bridge
  （CLAUDE.md §4 "V1.2 后端期不动前端"，等 Wails 迁移）
  ```
- **Context**: Records 3 explicit non-done items at end of Phase 3 event-log work, each with a one-line justification:
  - subagent_runs/messages drop — frontend dep
  - subagent.go file split — pure refactor, low value
  - frontend chat.js cutover — CLAUDE.md §4 explicit "don't touch frontend during V1.2 backend"
- **§S20 evaluation**:
  - (a) Structural hard constraint? **YES** for items 1 + 3 (frontend dependency, explicit project-level scope rule §4). Item 2 ("pure refactor") is shakier — §S20 禁止 reason includes "我懒" but not "value is low for a pure refactor".
  - (b) Explanation? **YES** — each item has a one-line reason.
- **Classification**: **VALID-DEFER** for items 1+3; **AMBIGUOUS** for item 2 (subagent.go file split).
- **Severity**: LOW
- **Recommended action**: item 2 — user clarify. If "pure refactor, 价值低" is the only reason, §S20 doesn't quite allow it (it's neither "we'll hit a real bug" nor structural). But the work was already not-done and is not a bug — so it sits at the edge of §S20 scope (§S20 is about bug-deferral, not refactor-prioritization).

---

### [15] progress-record 2026-05-07 retrospective about §S20 origin
- **Location**: `/Users/SP14921/Documents/Personal/PersonalCodeBase/Forgify/documents/version-1.2/progress-record.md:384`
- **Marker text**: Historical entry recording that "audit 表里之前列'⏳ 待修'的 4 项重审" — past usage of "待修" describing prior-audit risk-table items now cleaned up.
- **Classification**: **DOC-ONLY** (historical narrative)
- **Severity**: NONE
- **Recommended action**: none.

---

### [16] event-log-protocol.md §10 risk table cites §S20 for dogfood findings
- **Location**: `/Users/SP14921/Documents/Personal/PersonalCodeBase/Forgify/documents/version-1.2/event-log-protocol.md:1146`
- **Marker text**: Risk mitigation row "dogfood 期间发现协议设计漏洞 | 每个 phase 完工 dogfood，发现就 §S20 当场修不留下次"
- **Context**: Citing §S20 as the standing rule. Not a deferred marker.
- **Classification**: **DOC-ONLY** (legitimate §S20 reference)
- **Severity**: NONE
- **Recommended action**: none.

---

## Summary by classification

| Classification | Count |
|---|---|
| LANDMINE | 1 |
| VALID-DEFER | 4 (with 1 borderline — finding [7]) |
| STALE | 1 |
| DOC-ONLY | 9 (of which 3 lie about code state: findings [3] [4] [5]) |
| AMBIGUOUS | 1 (sub-issue inside finding [14]) |

**Note on counts**: finding [14] internally splits into VALID-DEFER + AMBIGUOUS; finding [10] [11] [12] [15] [16] are non-issues counted as DOC-ONLY for completeness.

---

## Top LANDMINEs (by severity)

1. **[6] MED — forge_executions 300-row eviction unimplemented but documented as a target.** Real implementation gap. §S20 禁止理由 #2 ("nobody hit it yet") covers this; the doc itself acknowledges "store 层无 TrimExecutions / 触发器". Either implement or remove the documented target. Single-user local dev keeps the blast radius small but Phase 5 test-batch dogfooding could rack up disk noise.

---

## Top DOC-ONLY issues that lie about code state

If user wants to clean these up:

1. **[3] LOW — chat.md:994** says `app/loop/loop.go` has a TODO hook point. It doesn't. Drop the clause.
2. **[4] LOW — 01-agent-loop.md:242,248** references `chat/stream.go:64` and `chat/tools.go:29`. Both files moved to `app/loop/` and no TODO(A1) marker exists in either current or historical form I could find.
3. **[5] LOW — agent-core-upgrade.md:525** claims `// TODO: context compaction` placeholder was left in code. It isn't there.

These three lie about residual code state. None are bugs in the code (code is clean), only the docs are stale.

---

## Conclusion

The Forgify backend is genuinely clean under §S20. The single backend Go TODO-style marker (anthropic.go:271-285) is **textbook §S20 compliance** — explicit (a) structural + (b) explanation citation, and the deferred work belongs to a layer that doesn't yet exist.

The one actual LANDMINE is in the **database-design.md** documenting a forge_executions 300-row cap that was never implemented in code. This sits at the doc-design-vs-reality fault line: either implement the trim, or drop the cap from the spec.

The remaining doc-only items are mostly historical / planning notes that legitimately use "TODO" / "待修" / "未做" wording but don't violate §S20 (research observations, plan documents, dev logs, §S20 self-references). Three doc passages lie about code state but the lies are about historical placeholders that have since been cleaned — code is correct, docs lag.
