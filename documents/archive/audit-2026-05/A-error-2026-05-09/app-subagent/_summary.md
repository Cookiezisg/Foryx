# Package audit summary: internal/app/subagent

## Spec understanding (Phase A — §S3 / §S9 / §S15 / §S16 / §S17)

- **§S3 错误不吞**: error suppression that hides user-visible failure / data loss / config drift is forbidden. `_ = err` requires inline justification. The package's main §S3 surface is `host.go` — sub-Message terminal write logging is correct (CRITICAL on save fail), but two soft-fall sites lack proper instrumentation: `json.Marshal(attrs)` discard without doc comment + `mapEventLogStatus` default branch silently mapping unknown chatdomain status to Completed. Plus one site in `spawn.go` (`if existing, err := ...; err == nil && existing != nil` clobbers DB-failure path under "row not yet visible" path).
- **§S9 detached ctx 终态写**: this is the **headline finding for the package**. The brief flags this as "最近 fix 过类似 chat.host.go". The chat-side fix (chat/host.go:54-77) made BOTH the SaveMessage AND the StopMessage emit use the detached `saveCtx`. subagent's WriteFinalize is the parallel function — it correctly detached the SaveMessage but kept the StopMessage on the cancellable original `ctx`. Same class of bug applies to `spawn.go::Spawn` finalize tail's `em.StopBlock` for the placeholder message-block. Two sites, same root cause, same shape of fix.
- **§S15 ID 生成**: clean. The post-2026-05 schema unification (per `subagent.md` file-header warning) eliminated `sar_` / `smm_` prefixes; sub-run ID is the sub-Message ID itself. spawn.go uses `idgenpkg.New("msg")` and `idgenpkg.New("blk")` exclusively — both compliant with §S15 panic-on-rand-failure delegated to pkg/idgen.
- **§S16 错误 wrap 格式**: 1 LOW deviation in spawn.go's panic-recovery path (`fmt.Errorf("subagent panic: %v", r)` lacks `subagentapp.Spawn:` prefix — `%v` itself is acceptable since panic value isn't a sentinel). Otherwise all `fmt.Errorf` calls are canonical pkg.Method + `%w` form (spawn.go:#1, #3).
- **§S17 errmap 单一事实源**: clean. Both subagent sentinels (`ErrTypeNotFound`, `ErrRecursionAttempt`) are registered at errmap.go:125-126. The errmap comment block at lines 114-124 explicitly documents WHY only these two exist (max-turns + cancellation surface as `SpawnResult.Status` string constants `StatusMaxTurns` / `StatusCancelled` per spawn.go:46-47, never as Go errors at handler boundary) — exact match between code and registration.

## Files audited

| File | LOC | Sites | OK | POST-FIX | VIOLATION | EDGE |
|---|---|---|---|---|---|---|
| subagent.go | 162 | 4 | 4 | 0 | 0 | 0 |
| spawn.go | 278 | 16 | 13 | 0 | 0 | 3 |
| host.go | 148 | 8 | 5 | 0 | 1 | 2 |
| queries.go | 28 | 1 | 1 | 0 | 0 | 0 |
| registry.go | 121 | 6 | 6 | 0 | 0 | 0 |
| **TOTAL** | **737** | **35** | **29** | **0** | **1** | **5** |

## Severity breakdown (only VIOLATION + EDGE)

| Severity | Count | Sites | Status |
|---|---|---|---|
| HIGH | 0 | — | — |
| MED | 2 | host.go:#6 (`em.StopMessage(ctx, ...)` uses cancellable ctx — sub-message stuck in `streaming` per §S21 invariant when parent chat cancels mid-sub-run); spawn.go:#13 (`em.StopBlock(parentCtx, ...)` uses cancellable parent ctx for placeholder message-block close — frontend hangs on dangling `block_start` when parent chat cancels mid-Spawn) | FOUND |
| LOW | 4 | spawn.go:#9 (panic wrap missing `subagentapp.Spawn:` prefix); spawn.go:#11 (`if existing, err := ...; err == nil && existing != nil { ... }` clobbers DB-error path with row-not-found path); host.go:#4 (`json.Marshal(attrs)` discard without §S3 inline justification); host.go:#8 (mapEventLogStatus default branch silently maps unknown status to Completed without Warn — analogous chat function logs Warn) | FOUND |

## Cross-cutting

### Sentinel chain integrity (§S17)

All 2 subagentdomain sentinels (errmap.go:125-126) verified registered:

| Sentinel | errmap.go line | First consumed in |
|---|---|---|
| `subagentdomain.ErrTypeNotFound` | 125 | spawn.go:#1 (Spawn type lookup) |
| `subagentdomain.ErrRecursionAttempt` | 126 | (consumed in app/tool/subagent/agent.go::Execute, audited separately — not in this fork's scope) |

**No missing registrations**. The errmap comment at lines 114-124 explicitly documents the design decision that `ErrMaxTurnsExceeded` / `ErrCancelled` (proposed in `subagent.md` §3 archived design) were dropped in favor of `SpawnResult.Status` string constants — preventing those two from creating "unmapped domain error" alarms. The constants `StatusCompleted` / `StatusMaxTurns` / `StatusCancelled` / `StatusFailed` (spawn.go:45-49) are the only "terminal status surface" the SubagentTool sees, and Tool.Execute renders them as `tool_result` text per `subagent.md` §3 last paragraph + §7.

### Detached ctx coverage (§S9) — context-by-context analysis

**Terminal-state write inventory:**

| Write | File / Site | Ctx | §S9 verdict |
|---|---|---|---|
| Sub-Message body persist (terminal) | host.go:#5 (chatRepo.SaveMessage) | `saveCtx` (detached, uid-stamped) | ✓ OK |
| Sub-Message status reconcile (post-loop re-map) | spawn.go:#11 (chatRepo.SaveMessage in `if spawn.Status != StatusCompleted` block) | `reconcileCtx` (detached, uid + convID) | ✓ OK |
| Sub-Message message_stop emit | host.go:#6 (em.StopMessage) | `ctx` (request ctx with timeout) | ✗ **VIOLATION** — should use saveCtx |
| Placeholder message-block block_stop emit | spawn.go:#13 (em.StopBlock) | `parentCtx` (chat agentRun's ctx) | ✗ **VIOLATION** — should use detached ctx |
| Cancel-registered cleanup | spawn.go:#8 (defer delete from activeRuns map) | (no ctx, in-memory only) | ✓ OK — pure in-memory mutation |
| agentstate token log append | spawn.go:#14 (state.AddSubagentTokens) | (no ctx — direct method call on AgentState) | ✓ OK — per `agentstate` package contract, this is in-memory accumulator, loss-on-cancel acceptable |
| Operator audit log | spawn.go:#15 (s.log.Info "subagent run terminated") | (no ctx — zap doesn't take ctx) | ✓ OK — log writer is independent of request lifetime |

**The pattern in WriteFinalize and Spawn finalize tail is half-applied detached ctx**: the **DB write** is detached (correct), but the **emit** uses the cancellable ctx. The chat/host.go fix (referenced in audit brief) is the model — it detaches BOTH. Subagent's two sites need the analogous fix.

### Pattern: "Detached DB but cancellable emit" cluster (spawn.go:#13 + host.go:#6)

Both VIOLATION sites are the same architectural mistake:

| Site | What writes | Detached? |
|---|---|---|
| host.go:#6 | em.StopMessage on sub-Message (the parent of all sub-block emits) | ✗ — uses request ctx |
| spawn.go:#13 | em.StopBlock on placeholder message-block (parent of sub-Message in event tree) | ✗ — uses parent ctx |

Both fix to: build detached emit ctx with uid + convID stamps (mirroring chat/host.go:54-55), pass to the em call. Single sweep commit — same shape of change at both sites. Reference snippet from chat/host.go:73-77:

```go
// Event-log: close the assistant message via the detached ctx so a
// cancelled upstream doesn't trip Bridge.Publish's ctx.Done early-out
// before subscribers (the SSE stream) receive message_stop.
h.svc.emitter.StopMessage(saveCtx, h.msgID, h.mapEventLogStatus(msg.Status), ...)
```

The comment + ctx swap pattern transfers verbatim to both subagent sites. **HIGH PRIORITY** because the user-visible symptom (sub-message stuck in streaming, dangling block_start in event tree) is reproducible by any "user closes browser tab during sub-run" sequence — common edge case.

### Pattern: mapEventLogStatus drift detection (host.go:#8)

`subagentHost.mapEventLogStatus` is the parallel of `chatHost.mapEventLogStatus` (chat/host.go:100-114). Chat side WAS hardened with a Warn log on default branch to surface chatdomain.Status* drift. subagent side is the OLDER unhardened version. Single-commit alignment: convert from free function to method on `*subagentHost`, add `h.svc.log.Warn` in default. Trivial change — chat/host.go has the pattern to copy.

### Why no sentinels need adding to errmap

`subagent.md` §12 originally proposed 4 sentinels:
- `ErrTypeNotFound` ✓ registered
- `ErrRecursionAttempt` ✓ registered
- `ErrMaxTurnsExceeded` — never created; replaced by `StatusMaxTurns` constant
- `ErrCancelled` — never created; replaced by `StatusCancelled` constant

The constant-vs-sentinel decision is documented in BOTH errmap.go:114-124 AND subagent.md §12 footnote — design alignment is verified. Spawn returns the SpawnResult struct with Status field; the SubagentTool wrapper (in app/tool/subagent, audited separately) renders these as tool_result text strings. No errmap row needed because they never enter the error path.

## Spot-check (random clean sites — verify mechanism not rubber-stamping)

Random sample of 7 sites picked from `OK` set across all 5 files:

1. **subagent.go:#1** (panic on nil-logger): verified — `panic("subagent.New: logger is nil")` carries `subagent.New:` qualifier per §S16. Same pattern as `apikey.NewService` / `mcp.New`. Boot-time wiring guard is §S3 §exception.
2. **subagent.go:#2** (filterTools — drop SubagentTool): verified — `if t.Name() == "Subagent" { continue }` is the structural recursion-defense per `subagent.md` §8 ("filterTools 过滤掉 SubagentTool 自身"). Returns `nil` when result is empty (line 144) — that's the documented `AllowedTools=nil` semantics for `general-purpose`. Compliance literal.
3. **spawn.go:#1** (ErrTypeNotFound wrap): verified — `fmt.Errorf("subagentapp.Spawn: %w: %q", subagentdomain.ErrTypeNotFound, typeName)`. pkg.Method prefix ✓, `%w` wraps sentinel ✓, `%q` adds typeName for log readability without breaking unwrap chain. errmap.go:125 → 404 SUBAGENT_TYPE_NOT_FOUND. Canonical.
4. **spawn.go:#7** (subCtx construction with `defer cancel()`): verified — context.WithTimeout overlay on parentCtx for 5-min total-run cap per `subagent.md` §8.5. `defer cancel()` releases timer resources. Parent-cancel cascade contract documented (parent ctx cancel naturally propagates to subCtx). This IS the §S9 ctx-construction site, NOT a violation — terminal writes that follow loop.Run are the §S9 surface, audited at sites #11/#13 (and host.go).
5. **host.go:#3** (saveCtx construction with uid fallback chain): verified — starts from `context.Background()`, prefers ctx-provided uid, falls back to `h.uid`. Final-else missing-uid path lets chatRepo.SaveMessage fail with `reqctxpkg.ErrMissingUserID` (errmap.go:185 → 500), which the line 113 CRITICAL log surfaces. Soft-degrade is observable, not silent. Compliance with §S9 detached-ctx pattern.
6. **host.go:#5** (terminal Message SaveMessage with CRITICAL log): verified — uses `saveCtx`, logs Error level on failure with sub_msg_id + zap.Error. Direct mirror of chat/host.go:58-67 pattern. The "CRITICAL: subagent terminal Message write failed" message is the §S3 textbook compliant logging.
7. **registry.go:#5** (Get returns `(value, bool)`): verified — Go convention for "lookup or absent". Caller (spawn.go:88) checks `!ok` and translates to `ErrTypeNotFound` sentinel. The bool is documented absence signal, not swallowed error per §S3.

All 7 spot-checks confirmed correct classification — mechanism functioning, not rubber-stamping. The audit's findings (1 VIOLATION + 5 EDGE) survive spot-check pressure: the canonical §S16 wrap at #1, the canonical §S9 detached-ctx at host.go:#5, and the canonical recursion-defense at subagent.go:#2 prove the package authors KNOW these patterns. The two MED §S9 violations are oversights of the analogous emit-side detach (recently learned in chat/host.go), not foundational misunderstandings.

## Recommended fix priorities

1. **§S9 detached emit cluster (host.go:#6 + spawn.go:#13)** — single sweep commit. Two MED violations, same root cause (detached ctx applied only to DB write, not emit), same fix shape (swap cancellable ctx for detached saveCtx/closeCtx with uid + convID stamps). Reference: chat/host.go:54-77 already has the model implementation. **HIGH PRIORITY** because user-visible symptom (dangling streaming sub-message + dangling block_start) is reproducible by common "user closes browser tab during sub-run" sequence.

2. **host.go:#8 (mapEventLogStatus drift detection)** — convert to method on `*subagentHost`, add `h.svc.log.Warn` in default branch with `s` + `h.subMsgID` context. Mirrors chat/host.go:111-113. ~5 line change, trivial.

3. **spawn.go:#11 (GetMessage err clobbered with not-found)** — split `if existing, err := ...; err == nil && existing != nil` into two branches: log Warn on `err != nil` (DB failure or ownership mismatch), silently skip on `existing == nil` (race with loop.Run pre-write — not an error). ~3 line change.

4. **host.go:#4 (json.Marshal `_` discard)** — add inline comment per §S3 §exception: `attrsJSON, _ := json.Marshal(attrs) // _ = err — attrs is map of string/int, json.Marshal cannot fail`. 1-line change.

5. **spawn.go:#9 (panic wrap missing prefix)** — pure style: `fmt.Errorf("subagentapp.Spawn panic: %v", r)`. 1-line change. Could WAIVE if commit-thrift, but consistent prefix grep makes operator audit easier.

All 5 fixes together = ~15 lines + 1 method-conversion. Single PR feasible.

## Cross-fork concerns

- **app/tool/subagent fork** (separate audit) consumes both subagent sentinels: `ErrTypeNotFound` propagates via Spawn return; `ErrRecursionAttempt` is returned directly by Tool.Execute when `GetSubagentDepth(ctx) >= 1`. Confirm that fork's audit verifies the depth-check path returns the sentinel without wrapping (so errmap can match it).
- **app/loop fork** (separate audit) is the consumer of `subagentHost` via `loop.Run`. Confirm loop.Host's `WriteFinalize` is called exactly once per sub-run (otherwise host.go:#5 + #6 fire twice → duplicate sub-Message rows / duplicate StopMessage emits, both §S21 invariant violations).
- **chat/host.go** (already audited or pending) is the reference implementation for the §S9 detached-emit pattern that subagent's WriteFinalize+Spawn need to copy. If the chat fork found additional refinements, they should propagate here.
