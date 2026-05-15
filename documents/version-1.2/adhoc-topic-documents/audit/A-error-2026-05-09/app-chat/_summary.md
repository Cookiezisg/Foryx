# Package audit summary: internal/app/chat

## Spec understanding (Phase A — §S3 / §S9 / §S15 / §S16 / §S17)

- **§S3 错误不吞**: error suppression that hides a user-visible failure / data loss / config drift is forbidden. `_ = err` requires inline justification. `defer X.Close()` on read-only resources or panic-path cleanup is allowed. Documented best-effort soft-fail is fine if it's logged with audit context (zap.Error). Silent fallback (json.Unmarshal err == nil branch only / no else) is the canonical anti-pattern §S3 forbids.
- **§S9 detached ctx 终态写**: terminal-state writes — assistant message finalize, error message stub persist, autoTitle save, eventlog StopMessage emit — must use `reqctxpkg.SetUserID(context.Background(), uid)` so a cancelled request ctx (browser close mid-stream) doesn't leave the row at stale prior state with no audit trail. chat package documents this pattern explicitly in chat.md §6.
- **§S15 ID 生成**: business IDs flow through `idgenpkg.New(prefix)`. chat uses three: "msg" (message), "blk" (block), "att" (attachment) — all match the §S15 spec list.
- **§S16 错误 wrap 格式**: `fmt.Errorf("<pkg>.<Method>: %w", err)` canonical form. Bare `return err` preserves sentinel chain (functionally OK) but breaks call-site grep traceability — flagged as LOW for style consistency.
- **§S17 errmap 单一事实源**: chatdomain sentinels (ErrAttachmentTooLarge / ErrStreamInProgress / ErrStreamNotFound / etc.) are all registered in errmap.go:58-66; chat package itself defines no new sentinels.

## Files audited

| File | LOC | Sites | OK | POST-FIX | VIOLATION | EDGE |
|---|---|---|---|---|---|---|
| chat.go | 390 | 24 | 19 | 0 | 1 | 4 |
| history.go | 172 | 14 | 7 | 0 | 0 | 7 (2 §S3 silent + 5 §S16 style) |
| host.go | 129 | 9 | 7 | 0 | 0 | 2 |
| runner.go | 246 | 18 | 14 | 0 | 0 | 4 |
| util.go | 35 | 2 | 1 | 0 | 0 | 1 |
| **TOTAL** | **972** | **67** | **48** | **0** | **1** | **18** |

## Severity breakdown (only VIOLATION + EDGE)

| Severity | Count | Sites | Status |
|---|---|---|---|
| HIGH | 0 | — | — |
| MED | 2 | chat.go:#14 (json.Marshal silent drop in Send when serializing attachments), runner.go:#12 (emitFatalError StopMessage uses caller ctx instead of detached — risks UI never receiving error message_stop on cancel race) | **FIXED f272503** |
| LOW (functional) | 11 | chat.go:#9, #11, #17 (§S16 bare-return); history.go:#2, #3, #4, #7, #12, #13, #14 (§S16); history.go:#9 (§S3 unknown role silent drop), #10 (§S3 malformed Attrs JSON silent drop); util.go:#2 (§S16 prefix style) | **FIXED f272503** |
| LOW (pending review) | 2 | host.go:#8 (mapEventLogStatus default branch — wiring-bug or N/A?); runner.go:#6 (manual error-code switch duplicating errmap — refactor or WAIVE per §S17-adjacent reasoning) | FOUND |

(Note: chat.go:#14 listed twice because it's MED-severity §S3 violation but qualifies for both. Counted once in totals.)

## Cross-cutting

### Sentinel chain integrity (§S17)
- All chat domain sentinels consumed in this package (`chatdomain.ErrAttachmentTooLarge`, `chatdomain.ErrStreamInProgress`, `chatdomain.ErrStreamNotFound`) are registered in errmap.go (rows 58-66).
- `chatdomain.RoleAssistant / RoleUser` and status constants are non-error consts, no errmap concern.
- Package itself defines NO new sentinels — clean separation domain (sentinels) ↔ app (logic).
- Consumed cross-package: `reqctxpkg.ErrMissingUserID` (errmap.go:163 ✓), `convdomain.ErrNotFound` via convRepo (errmap.go:55 ✓).
- runner.go:#6 has a manual code switch that **duplicates** errmap.go's truth source — flagged §S17-adjacent (not strict violation since sentinels aren't propagated past this point).

### Detached ctx coverage (§S9) — most important cross-cutting concern in this package

**Terminal-state writes inventory (chat package + downstream):**

| Write | Location | Ctx | §S9 verdict |
|---|---|---|---|
| Save user message (synchronous) | chat.go:#17 (SaveMessage in Send) | request ctx | ✓ OK — user is waiting on HTTP request; no async cancel concern |
| Agent run lifecycle | chat.go:#19 (agentCtx for queued task) | derived from `context.Background()` via reqctxpkg.SetUserID | ✓ OK — agent must outlive request ctx (chat.md §6) |
| Save assistant FINAL message | host.go:#4 (SaveMessage in WriteFinalize) | `saveCtx` from `context.Background()` | ✓ OK — canonical §S9 example |
| Emit assistant message_stop | host.go:#5 (StopMessage) | `saveCtx` (detached) | ✓ OK — canonical |
| Save fatal-error stub message | runner.go:#10 (SaveMessage in emitFatalError) | `saveCtx` from `context.Background()` | ✓ OK |
| Emit fatal-error message_stop | runner.go:#12 (StopMessage in emitFatalError) | **caller's ctx (agentCtx)** | **⚠ MED — inconsistent with host.go:#5; if user closes tab between Resolve fail and StopMessage emit, UI never sees the error_stop event** |
| autoTitle save + notify | runner.go:#8 (autoTitle goroutine on context.Background()) | titleCtx derived from `context.Background()` via reqctxpkg.SetUserID | ✓ OK |

**One MED violation found** at runner.go:#12. The fix is one-line (use `saveCtx` instead of `ctx` for the StopMessage emit) and exactly mirrors host.go:#5's pattern.

### §S3 silent drops surfaced

Two §S3 silent paths in chat package:

| Site | What's silent | Trigger | Audit trail |
|---|---|---|---|
| chat.go:#14 | json.Marshal(attrs) err — attachments lost from Message.Attrs | (theoretically) NaN/Inf/cyclic in attrs map | none — no log, no return |
| history.go:#10 | json.Unmarshal(Attrs) err — attachments lost from LLM context | corrupted DB row, schema drift | none — no log, no return |
| history.go:#9 | unknown Role silently dropped from history | stray role / future role not yet wired | none — no log |

Both #10 and #9 are particularly insidious because they hide DB-corruption symptoms: the message saves successfully but its attachments aren't shown in subsequent LLM context — debugging would require diffing what the LLM saw vs what's in DB.

## Spot-check (random clean sites — verify mechanism not rubber-stamping)

Random seed: 6 sites picked from `OK` set across 5 files:

1. **chat.go:#19** (agentCtx detached): verified — exact `reqctxpkg.SetUserID(context.Background(), uid)` form from §S9 spec; chat.md §6 line 358 explicitly cites this as the canonical pattern. Compliance literal.
2. **chat.go:#21** (Cancel sentinel): verified — `chatdomain.ErrStreamNotFound` returned directly (innermost); errmap.go:59 maps to 404 STREAM_NOT_FOUND. Sentinel chain trivially preserved.
3. **history.go:#11** (attachment soft-fail with WARN log): verified — `s.log.Warn("skipping attachment in LLM history", zap.Error(err))` matches §S3 documented soft-fail pattern (lines 95-100 of history.go explicitly designate this as best-effort with audit trail). Log includes original err for diagnosis.
4. **host.go:#3** (saveCtx derivation): verified — `reqctxpkg.SetUserID(context.Background(), h.uid)` + `WithConversationID(saveCtx, h.convID)`. The convID re-stamp is critical because emit needs it for routing (per chat.md §11 line 1011). Doc comment at host.go:47-53 cites the cancel-race scenario explicitly.
5. **host.go:#6** (`_ = ctx` with comment): verified — inline comment "legacy param retained for loop.Host signature" explains why request ctx is intentionally not used. §S3 example: "_ ignore must have inline comment explaining why" — compliance literal.
6. **runner.go:#10** (saveCtx in emitFatalError): verified — `saveCtx := reqctxpkg.SetUserID(context.Background(), uid)`. Same form as host.go:#3. (Notably: the SaveMessage at runner.go:#11 IS using saveCtx ✓, but the StopMessage at #12 is NOT — that's the inconsistency this audit caught.)

All 6 spot-checks confirmed correct classification — mechanism functioning, not rubber-stamping. The audit's primary find (runner.go:#12 §S9 inconsistency) survives spot-check pressure: the OK sites #4 (host.go saveCtx) and #6 (runner.go saveCtx for SaveMessage) prove the §S9 pattern is generally applied correctly in this package, which makes #12's deviation a real bug not a noise finding.

## Recommended fix priorities

1. **runner.go:#12** (MED §S9 — emitFatalError StopMessage uses caller ctx) — 1-line fix, mirrors existing host.go pattern, prevents UI hang on cancel-race; HIGH PRIORITY.
2. **chat.go:#14** (MED §S3 — json.Marshal silent attachment drop) — minimum: log at WARN; better: surface as error.
3. **history.go:#9, #10** (LOW §S3 — silent drops without audit trail) — add WARN logs with diagnostic context.
4. LOW §S16 wrap-format consistency (chat.go:#9, #11, #17, history.go:#2, #3, #4, #7, #12, #13, #14, util.go:#2) — pure style cleanup, no functional change. Consider batching as a single sweep commit.
5. LOW miscellaneous (host.go:#8 mapEventLogStatus default, runner.go:#6 manual code switch) — defer / WAIVE depending on user preference.
