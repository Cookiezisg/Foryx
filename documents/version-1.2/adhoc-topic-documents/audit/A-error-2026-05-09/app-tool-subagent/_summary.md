# Package audit summary: internal/app/tool/subagent

## Spec understanding (Phase A — §S3 / §S9 / §S15 / §S16 / §S17)

- **§S3 错误不吞**: error suppression hiding user-visible failure / data loss / config drift forbidden. §S18 friendly tool_result pattern (status string ↦ readable LLM-facing text) is canonical compliant form. Package godoc lines 17-21 explicitly states this carve-out: "max-turns / cancelled terminations are converted to friendly tool_result strings so the parent LLM can read the situation and decide how to proceed; only hard sentinels (recursion / unknown type) escape as Go errors." Identical posture to mcptool::mapCallToolErrorToFriendly.
- **§S9 detached ctx 终态写**: terminal-state writes that MUST persist regardless of caller cancel use detached ctx. **N/A at this tool layer** — Execute delegates to `subagentapp.Service.Spawn` which is responsible for §S9 compliance (subagent_runs row insert/finalize, eventlog block stop on cancellation, message_blocks emit). Both already covered by app-subagent audit (out of scope here). Tool layer only forwards `ctx` (line 186) without persistence — correct delegation.
- **§S15 ID 生成**: `<prefix>_<16hex>` via idgenpkg. Tool layer doesn't generate business IDs. The `sar_` / `smm_` prefixes (subagent run / subagent message per §S15) are generated downstream in `app/subagent.Service.Spawn` and the eventlog emitter — out of scope for this audit.
- **§S16 错误 wrap 格式**: `fmt.Errorf("<pkg>.<Method>: %w", err)`. Package uses **strict-literal** `SubagentTool.<Method>:` form (sites #4, #6, #7). Three wrap sites total — all literally compliant; no migration needed (unlike app-tool-mcp's helper-style which was WAIVED). Bare propagation at site #8 (`return "", err` after Spawn) is correct — avoids re-wrap that would duplicate or shadow upstream prefixes.
- **§S17 errmap 单一事实源**: every sentinel that can reach a handler must be in errmap.go. Package defines 2 local validation sentinels (ErrEmptyPrompt, ErrEmptyType) — both consumed inside Tool framework (runOneTool ↦ failed tool_result), never reach errmap (no registration needed, mirrors app-tool-mcp pattern). Two consumed `subagentdomain` sentinels (ErrRecursionAttempt, ErrTypeNotFound) verified registered errmap.go:120-121.

## Files audited

| File | LOC | Sites | OK | POST-FIX | VIOLATION | EDGE |
|---|---|---|---|---|---|---|
| agent.go | 235 | 11 | 11 | 0 | 0 | 0 |
| **TOTAL** | **235** | **11** | **11** | **0** | **0** | **0** |

(Single-file package. `agent_test.go` excluded per audit scope.)

## Severity breakdown

| Severity | Count | Sites | Status |
|---|---|---|---|
| HIGH | 0 | — | — |
| MED | 0 | — | — |
| LOW | 0 | — | — |

**Zero violations.** Package is §S3 / §S9 / §S15 / §S16 / §S17 textbook-clean.

## Cross-cutting

### Sentinel chain integrity (§S17)

- **Local sentinels** (not registered, framework-consumed): `ErrEmptyPrompt`, `ErrEmptyType` — both validation-layer, returned by `ValidateInput` and converted to failed tool_result by `runOneTool`. Never bubble to `responsehttpapi.FromDomainError`. Same precedent as `mcptool.ErrEmptyServer`/`ErrEmptyTool`/`ErrEmptyQuery` in app-tool-mcp.
- **Domain sentinels propagated** (handler-bound, must be in errmap):
  - `subagentdomain.ErrRecursionAttempt` — wrapped at agent.go:173-174 with `SubagentTool.Execute: %w (depth=%d)`. Registered errmap.go:121 → 422 SUBAGENT_RECURSION ✓
  - `subagentdomain.ErrTypeNotFound` — flows up via `Service.Spawn`'s `%w` chain to agent.go:194 (`return "", err`). Registered errmap.go:120 → 404 SUBAGENT_TYPE_NOT_FOUND ✓
- **Status-string carve-outs**: `subagentapp.StatusMaxTurns` / `StatusCancelled` / `StatusFailed` are NOT Go error sentinels — they're string constants on `SpawnResult.Status`. Per §S18 friendly tool_result pattern, Execute converts these to readable LLM-facing text (sites #9, #10) and never returns a corresponding Go error. errmap.go:113-119 inline comment explicitly preempts this design: "Only the first two reach handlers."

**No missing registrations. Chain integrity preserved end-to-end.**

### Recursion-guard structure (parent's specific concern: §S18 + §S9 lifecycle)

Recursion defense is two-layered per package godoc:

1. **Structural** (`subagentapp.Service.Spawn`'s tool-list filter, out of scope): drops `Subagent` from sub-runner's tool list before `loop.Run`, so sub-LLM physically cannot see the tool name. Primary defense.
2. **Runtime** (this file, agent.go:172-175): `if depth := reqctxpkg.GetSubagentDepth(ctx); depth >= 1 { return ErrRecursionAttempt … }`. Belt-and-suspenders if a future bridge bug or test path leaks `Subagent` into a sub-runner's tool list.

Audit verdict: both layers correctly implemented at this file's surface. The runtime guard reads `reqctxpkg.GetSubagentDepth(ctx)` (verified in `pkg/reqctx/agentrun.go:129`) which returns 0 for main-chat ctx and ≥1 for sub-runner ctx. Correctly wraps the sentinel with `%w` so `errors.Is(err, subagentdomain.ErrRecursionAttempt)` matches at handler.

### Detached ctx coverage (§S9) — N/A at this layer

All terminal writes happen below tool layer:
- `subagent_runs` row insert/finalize → `app/subagent.Service.Spawn` (out of scope)
- eventlog block start/stop on cancellation → `app/subagent.Service` via `loop.Run` host hooks (out of scope)
- message_blocks emit (sub-run final message) → `app/subagent.Service` via host's `WriteFinalize` (out of scope)

Tool layer correctly delegates without re-implementing §S9 logic. Same posture as app-tool-mcp / app-tool-forge.

### Style consistency cross-check vs sibling packages

This package is **stylistically consistent and slightly stricter** than app-tool-mcp / app-tool-shell / app-tool-forge:

- §S16 prefix: **strict-literal `SubagentTool.<Method>:`** form (no helper-style `<tool_name>:` deviation that those siblings used and got WAIVED)
- §S18 friendly tool_result: pattern matches mcptool::mapCallToolErrorToFriendly textbook reference
- Validation sentinels: local, framework-consumed (mirrors mcptool / forgetool)
- Hard sentinels: routed through domain layer (`subagentdomain.Err*`), registered in errmap

The package is a **good model** to reference when auditing future system tools — particularly the recursion-guard wrap pattern at agent.go:172-175 and the friendly-status switch at agent.go:203-215 are textbook §S18 / §S3 / §S16 implementations.

## Spot-check (random clean sites — verify mechanism not rubber-stamping)

5 sites picked from `OK` set:

1. **agent.go:#4** (ValidateInput Unmarshal wrap): verified — `fmt.Errorf("SubagentTool.ValidateInput: %w", err)`. `<Type>.<Method>:` prefix + %w. `errors.Is(err, &json.SyntaxError{...})` chain preserved through wrap.
2. **agent.go:#6** (Execute recursion-check): verified — `fmt.Errorf("SubagentTool.Execute: %w (depth=%d)", subagentdomain.ErrRecursionAttempt, depth)` correctly puts sentinel into `%w` slot (the *first* %-verb in this format string). `errors.Is(err, subagentdomain.ErrRecursionAttempt)` unwraps. Trailing `(depth=%d)` is informational, doesn't break chain.
3. **agent.go:#8** (Spawn err propagation): verified — bare `return "", err` correctly preserves Spawn's upstream wrap chain. Spawn (`app/subagent/spawn.go`) wraps its returned errors with `%w` and `subagentdomain.ErrTypeNotFound` at innermost; bubbling unchanged keeps `errors.Is(err, ErrTypeNotFound)` working at errmap.lookup.
4. **agent.go:#9** (Status switch friendly conversion): verified — three case branches (`StatusMaxTurns` / `StatusCancelled` / `StatusFailed`) each return `(string, nil)` per §S18 friendly tool_result; default falls to `res.Result` for `StatusCompleted`. No silent fallthrough; status info preserved as text in all branches.
5. **agent.go:#11** (appendNote helper): verified — pure formatting, idempotent, separator (`\n\n`) ensures LLM can disambiguate framework note from sub-runner output.

All 5 spot-checks confirmed correct classification — mechanism functioning, not rubber-stamping.

## Recommended fix priorities

**None.** Package is §S3/S9/S15/S16/S17 textbook-clean. No HIGH / MED / LOW found.

## Out-of-scope notes (parent should verify)

1. **errmap.go:114 doc nit**: comment names "ErrMaxTurnsExceeded / ErrCancelled" sentinels but actual code uses `subagentapp.StatusMaxTurns` / `StatusCancelled` *string constants* on `SpawnResult.Status` field — those Go-error sentinels don't exist. The comment is descriptive of the friendly-conversion behavior but mis-names the entities. Doc-only inconsistency in errmap.go, **not in scope** for this audit; surface in errmap.go's own doc-cleanup pass.
2. **app/subagent.Service.Spawn lifecycle**: §S9 detached-ctx for finalize-on-cancel writes, §S15 `sar_`/`smm_` ID generation, §S16 wrap chain into `subagentdomain.ErrTypeNotFound` — all live downstream of this tool layer in `app/subagent/spawn.go`. Verify in app-subagent audit (if it exists) or as a follow-up.
3. **Tool framework standard-field injection** (`summary` / `destructive` / `execution_group` per §S18 §2): `Parameters()` returns `subagentSchema` *without* these three fields. Per §S18 §2 the framework injects them at tool-list-build time via `injectStandardFields`/`StripStandardFields`. Verify in app/tool framework audit; not this file's concern.
