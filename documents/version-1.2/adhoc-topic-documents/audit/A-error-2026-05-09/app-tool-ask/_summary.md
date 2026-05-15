# Package audit summary: internal/app/tool/ask

## Spec understanding (Phase A — §S3 / §S9 / §S15 / §S16 / §S17)

- **§S3 错误不吞**: error suppression that hides user-visible failure / data loss / config drift is forbidden. `_ = err` requires inline justification. Tool-result string conversion (not Go err propagation) is the ask package's documented contract — failures land as LLM-readable strings, not errors. This is NOT §S3 silent fallback because the LLM sees the cause and can reword/retry.
- **§S9 detached ctx 终态写**: terminal-state writes that MUST land use `reqctxpkg.SetUserID(context.Background(), uid)`. Ask package does NO DB writes; svc.Wait is a synchronous block. Cancellation via request ctx is intentional (conversation interrupt → wait returns context.Canceled → friendly LLM string). No §S9 concern.
- **§S15 ID 生成**: business IDs flow through `idgenpkg.New(prefix)`. Ask package does NOT generate IDs; tool-call ID comes from the LLM via reqctx.GetToolCallID, which matches §S21 invariant for LLM-supplied IDs.
- **§S16 错误 wrap 格式**: `fmt.Errorf("<pkg>.<Method>: %w", err)` canonical. Ask package follows this for the one Go err path (ValidateInput json error). Friendly tool_result string output uses %v which is correct because it's text formatting, not error chain propagation.
- **§S17 errmap 单一事实源**: every sentinel that can reach a handler must be in errmap.go. Ask defines 1 sentinel (ErrEmptyQuestion), framework-consumed and never reaches handler — N/A registration. Cross-package askapp sentinels (ErrTimeout / ErrNoPendingQuestion / ErrAlreadyAnswered) all registered.

## Files audited

| File | LOC | Sites | OK | POST-FIX | VIOLATION | EDGE |
|---|---|---|---|---|---|---|
| ask.go | 172 | 13 | 12 | 0 | 0 | 1 |
| **TOTAL** | **172** | **13** | **12** | **0** | **0** | **1** |

## Severity breakdown (only VIOLATION + EDGE)

| Severity | Count | Sites | Status |
|---|---|---|---|
| HIGH | 0 | — | — |
| MED | 0 | — | — |
| LOW | 1 | ask.go:#11 (catch-all `%v` in tool_result formatting — by-contract, defer to Phase C tool-result review) | FOUND |

## Cross-cutting

### Sentinel chain integrity (§S17)

Package-defined sentinels (1):
- `ErrEmptyQuestion` (ask.go:54) — framework-consumed via ValidateInput hook; NOT registered in errmap. Per §S17 spec: "完全包内 / 跨包但只在 service 层消费、handler 层翻译成别的 sentinel 的，不需要登记". Pattern matches:
  - app-tool-shell: 4 ValidateInput sentinels framework-consumed (audit-confirmed N/A)
  - app-tool-mcp: 2 framework-consumed validation sentinels (audit-confirmed N/A)

Cross-package sentinels consumed (3, all registered):
- `askapp.ErrTimeout` → errmap.go:159 (504 ASK_TIMEOUT) ✓
- `askapp.ErrNoPendingQuestion` → errmap.go:157 (404 ASK_NO_PENDING_QUESTION) ✓
- `askapp.ErrAlreadyAnswered` → errmap.go:158 (409 ASK_ALREADY_ANSWERED) ✓

Sentinels translated to friendly tool_result strings here (LLM consumption), not propagated as Go err. Both paths are valid by package contract documented at lines 144-145.

### Detached ctx coverage (§S9)

| Operation | File / Site | Ctx | §S9 verdict |
|---|---|---|---|
| svc.Wait blocking on user answer | ask.go:#8 (line 157) | request ctx | ✓ OK — intentional cancellation: ctx-cancel = "conversation interrupted" returns friendly LLM string. NOT a terminal write; no DB persistence in this path. Service-layer answer persistence is in `app/ask` (separate audit scope). |

Package has NO terminal-state writes. §S9 N/A.

### Tool-result-as-error pattern

The ask package's contract (documented at ask.go:144-145, 151) is that `Execute` returns `(string, nil)` for ALL outcomes — happy path AND failure. Failures become LLM-readable strings:
- "Cannot ask the user: no tool_call_id..." (wiring bug)
- "User did not respond within the timeout..." (timeout)
- "Question cancelled by the user..." (cancellation)
- "Asking the user failed: ..." (catch-all)

This is the §S18 / §C "friendly tool_result" pattern — same as `app/tool/mcp/call.go::mapCallToolErrorToFriendly` which prior audit (app-tool-mcp) called "model implementation". The downside: the catch-all `%v` formatting at site #11 collapses unexpected err classes — flagged LOW for Phase C tool-result review (not a §S3-S17 violation by Phase A spec).

### Phase C concerns (out-of-scope but flagged)

- ask.go:155 friendly text mentions "chat layer wiring bug" — internal architecture leak in LLM-visible string. Same impl-detail-leak class as Phase C audit highlighted in app-tool-mcp 4 sites (`~/.forgify/mcp.json` mentions). Defer to Phase C.

## Spot-check (random clean sites)

Random seed: 5 sites picked from `OK` set:

1. **ask.go:#1** (ErrEmptyQuestion sentinel definition): verified — `errors.New("question is required and must be non-empty")`. No %w/%v issues; sentinel-only. Framework consumes via ValidateInput → friendly tool_result. Compliance literal.

2. **ask.go:#5** (ValidateInput json err wrap): verified — `fmt.Errorf("AskUserQuestion.ValidateInput: %w", err)`. pkg.Method prefix ✓, %w ✓. Note: spec example uses `<pkg>.<Method>:` like `apikeystore.List:` — `AskUserQuestion.ValidateInput:` swaps Type for pkg, but per audit precedent (forge tools, mcp tools using `<Tool>.<Method>:`) this is the established package convention. Compliance.

3. **ask.go:#7** (`callID, _ := reqctxpkg.GetToolCallID(ctx)` discard): verified — second return is `ok bool` (verified by reqctxpkg pattern), not an error. §S3 spec carve-out for non-error discards. Compliance.

4. **ask.go:#9** (errors.Is(err, askapp.ErrTimeout) → friendly string): verified — sentinel-based discrimination, ErrTimeout registered errmap.go:159 (so errors.Is correctly chains via package boundary), friendly text ("Re-ask later if still needed") is LLM-actionable. Both ends of the contract honored.

5. **ask.go:#10** (errors.Is(err, context.Canceled) → friendly string): verified — stdlib sentinel, registered errmap.go:181 (499 CLIENT_CLOSED). Friendly text correctly attributes to "conversation interrupted" (the natural user-visible cause when context.Canceled fires here, since no other cancellation path applies in this code). Compliance.

All 5 spot-checks confirmed correct classification — mechanism functioning, not rubber-stamping. Package is small and architecturally clean: §S18 Tool 9-method contract honored, error paths classified via errors.Is (not strings, unlike webtool pre-fix), tool-result string format is the package's documented contract.

## Recommended fix priorities

**No HIGH/MED priorities.** Single LOW (ask.go:#11 catch-all `%v` in tool_result string) is BY-CONTRACT acceptable per package design but worth a Phase C revisit alongside the ask.go:155 impl-detail-leak ("chat layer wiring bug" leak in LLM-visible string).

Phase A scope: **0 fixes required.** Package is §S3/S9/S15/S16/S17 compliant.

## Out-of-scope flags (for parent / Phase C)

- ask.go:155 leaks "chat layer wiring bug" wording into LLM tool_result — impl-detail leak (§C).
- ask.go:163-164 catch-all `%v` formatter — defer to Phase C as part of broader tool-result string review.
