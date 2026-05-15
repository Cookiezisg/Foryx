# audit: backend/internal/app/loop/tools.go

LOC: 302
Read: full file (lines 1-302)

## Trace

| site# | file:line | snippet | category | classification | reasoning | severity | user_impact | suggested_fix | status |
|---|---|---|---|---|---|---|---|---|---|
| 1 | tools.go:38-74 | `runTools` — orchestrator; never errors per contract (lines 32-37 doc) | A.1 | OK | by-design no-error contract; failures convert to ok=false blocks. | N-A | — | — | — |
| 2 | tools.go:50-72 | `var mu sync.Mutex; for _, b := range batches { ... go func(it indexedCall) { defer wg.Done(); blk := runOneTool(ctx, ...); mu.Lock(); blocks[it.idx] = blk; mu.Unlock() }(item) ... }` | A.1 | OK | concurrency primitive; sync.Mutex / sync.WaitGroup return no errors. mu.Lock() / Unlock() panic on misuse but that's framework guard not §S3 swallow. | N-A | — | — | — |
| 3 | tools.go:93 | `argsJSON, _ := json.Marshal(tc.Arguments)` | A.1 | EDGE | §S3: Marshal err discarded with no comment. tc.Arguments is `map[string]any` from parseToolArgs — same constraint as stream.go:#9 (basic types only, can't realistically fail). LLM-supplied input but already parsed through stream.go safe path. | LOW | none in practice — input is constrained `map[string]any` of basic types. | add inline comment: `_ = err — tc.Arguments built by parseToolArgs from map[string]basic-types; Marshal cannot fail`. Style polish. | **FIXED 2026-05-09 505d6e3** (added inline comment documenting unfailable basic-type Marshal) |
| 4 | tools.go:100-101 | `toolCtx := reqctxpkg.WithToolCallID(ctx, tc.ID); toolCtx = reqctxpkg.WithParentBlockID(toolCtx, tc.ID)` | A.1 | OK | non-error helpers; pure ctx-value attach. | N-A | — | — | — |
| 5 | tools.go:111 | `resultBlockID := idgenpkg.New("blk")` | A.3 | OK | §S15 canonical: idgenpkg.New("blk") matches spec list ("blk_" block); panic-on-rand-fail handled inside idgenpkg. | N-A | — | — | — |
| 6 | tools.go:113-126 | `em.EmitBlockStart(...); em.DeltaBlock(...); em.StopBlock(...)` (real-time eventlog emit for tool_result) | A.1 | OK | em.* return no errors per pkg/eventlog contract; pure side-effect. | N-A | — | — | — |
| 7 | tools.go:141 | `_ = elapsedMs // legacy elapsedMs no longer carried in Block (UI gets it via DB row updated_at - created_at)` | A.1 | OK | `_ = ...` with **inline comment explaining why** — exactly the §S3 example for safe discard. Compliance literal. | N-A | — | — | — |
| 8 | tools.go:158-160 | `type stringErr string; func (e stringErr) Error() string { return string(e) }` | A.4 | OK | tiny error type for passing string through StopBlock's `error` parameter; documented purpose at lines 153-156. Not a sentinel chain (StopBlock consumes for logging only). | N-A | — | — | — |
| 9 | tools.go:167-176 | `executeTool` — `if t == nil { msg := fmt.Sprintf("tool %q not found", name); return msg, msg, false }` | A.1/A.4 | EDGE | §S3: t==nil is a wiring bug ("tool not found in registry"). Currently returns friendly msg as both output AND errMsg, sets ok=false → LLM sees "tool not found" and can react. **However**: this masks a programming error (registry mis-wiring) as a runtime user-recoverable error. The error_msg field doesn't reach errmap (Execute returns plain `string` not `error` for runtime tool result), so wiring bugs only surface in DB rows. No log. | LOW | minor — LLM might decide to skip or retry; unlikely to surface as user-visible bug since registry wiring is set at boot and stable. Hard to debug if it does happen because no operator log fires. | add `log.Warn("executeTool: tool not in registry", zap.String("tool", name))` so wiring bugs leave a paper trail, even though the user-facing path is correct. | **FIXED 2026-05-09 26f9c55** |
| 10 | tools.go:173-176 | `if err := t.ValidateInput(argsJSON); err != nil { log.Warn("tool validate failed", zap.String("tool", name), zap.Error(err)); return fmt.Sprintf("input validation failed: %s", err.Error()), err.Error(), false }` | A.1/A.4 | OK | err is logged at WARN with structured fields AND returned as both output (for LLM) and errMsg (for DB). Both audit + user-facing covered. The `fmt.Sprintf("...: %s", err.Error())` is acceptable here because the result is an LLM-facing tool_result string (not a domain error chain) — sentinel preservation is irrelevant for tool result strings. | N-A | — | — | — |
| 11 | tools.go:191-199 | `if state, hasState := reqctxpkg.GetAgentState(ctx); hasState { if state.IsToolPreApprovedBySkill(name, argsJSON) { ... return executeAfterPermission(...) } }` | A.1 | OK | non-error helper chains; `_, hasState := ...` is the standard ok-bool pattern. | N-A | — | — | — |
| 12 | tools.go:201-212 | `switch t.CheckPermissions(...) { case PermissionDeny: ... return ...; case PermissionAsk: /* fall through to Allow */ }` | A.1 | OK | PermissionAsk fall-through is documented intent (lines 205-211: Phase 4+ scheduler will treat Ask as suspension; Phase 3 currently treats as Allow). Comment explicitly states why. §S3 compliant — not a swallow, deliberate single-user-local design. | N-A | — | — | — |
| 13 | tools.go:223-233 | `executeAfterPermission` — `output, err := t.Execute(ctx, string(argsJSON)); if err != nil { log.Warn(...); if output != "" { return output, err.Error(), false } return err.Error(), err.Error(), false }` | A.1/A.4 | OK | err is logged AND surfaced as both output + errMsg. Full audit trail. Same `err.Error()` stringification pattern as site #10 — appropriate for tool result strings. | N-A | — | — | — |
| 14 | tools.go:266-302 | `partitionByExecutionGroup` — pure logic, no errors | A.1 | OK | sort.Ints (line 296) returns no error; pure deterministic partitioning. | N-A | — | — | — |

## Sub-check

A.1 §S3 错误吞没:
  - violations: site #3 (LOW — Marshal err discarded with no comment, though safe by construction); site #9 (LOW — `tool %q not found` masks wiring bug with no operator log)

A.2 §S9 detached ctx 终态写:
  - terminal-state writes identified: none — file performs no DB writes; only em.* emits (real-time event log) which are not "terminal" in §S9 sense
  - 各自 ctx 来源: ctx is request ctx; tool implementations use reqctx-attached toolCtx
  - violations: N/A — runOneTool's persistence belongs to host.WriteFinalize at end of loop.Run, not per-tool. Real-time block emits ride request lifetime intentionally (replay buffer covers reconnects).

A.3 §S15 ID 生成:
  - ID generation calls: `idgenpkg.New("blk")` at line 111 (single call site for tool_result block IDs)
  - violations: not present — uses idgenpkg, "blk" prefix matches §S15 spec list

A.4 §S16 错误 wrap 格式:
  - violations: not present in domain-error sense. The `fmt.Sprintf("...: %s", err.Error())` calls at sites #9, #10, #13 produce LLM-facing tool_result strings, not domain errors — sentinel chain preservation doesn't apply (Execute returns `(string, error)` for ok-bool tuple, not for errors.Is consumers). The stringErr type at site #8 is a string-to-error wrapper with documented limited purpose.

A.5 §S17 sentinel 登记 errmap:
  - sentinels defined: none (only the stringErr local type which is not a sentinel)
  - 已登记 errmap: N/A
  - missing: N/A — file defines no sentinels (returns plain types from public API)
