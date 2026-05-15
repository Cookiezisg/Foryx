# audit: backend/internal/app/tool/ask/ask.go

LOC: 172
Read: full file (lines 1-172)

## Trace

| site# | file:line | snippet | category | classification | reasoning | severity | user_impact | suggested_fix | status |
|---|---|---|---|---|---|---|---|---|---|
| 1 | ask.go:51-55 | `var ErrEmptyQuestion = errors.New("question is required and must be non-empty")` | A.5 | OK | Validation sentinel consumed by §S18 Tool framework's ValidateInput hook. Framework converts ValidateInput err to LLM-friendly tool_result (never reaches errmap.FromDomainError). Per §S17 spec: "完全包内 / 跨包但只在 service 层消费、handler 层翻译成别的 sentinel 的，不需要登记". Pattern matches app-tool-shell + app-tool-mcp 's framework-consumed validation sentinels (audit-confirmed N/A). | N-A | — | — | — |
| 2 | ask.go:96-100 | `func AskTools(svc *askapp.Service) []toolapp.Tool { return []toolapp.Tool{ &AskUserQuestion{svc: svc, timeout: defaultTimeout} } }` | A.1 | OK | Pure constructor; no error path. | N-A | — | — | — |
| 3 | ask.go:104-106 | `Name() / Description() / Parameters()` (3 Identity getters) | A.1 | OK | Pure getters returning compile-time constants. No error path. | N-A | — | — | — |
| 4 | ask.go:110-112 | `IsReadOnly() / NeedsReadFirst() / RequiresWorkspace()` (3 Static metadata getters) | A.1 | OK | Pure constants per §S18 spec. AskUserQuestion blocks waiting for user input; no file I/O so all three falsey is correct. | N-A | — | — | — |
| 5 | ask.go:119-130 | `func ValidateInput(args json.RawMessage) error { ... if err := json.Unmarshal(args, &a); err != nil { return fmt.Errorf("AskUserQuestion.ValidateInput: %w", err) }; if strings.TrimSpace(a.Question) == "" { return ErrEmptyQuestion }; return nil }` | A.4 | OK | §S16 canonical: pkg.Method prefix + %w on json error path; direct sentinel return for empty question. errors.Is unwrap to ErrEmptyQuestion works. Framework consumes this err (LLM tool_result conversion); never reaches errmap. | N-A | — | — | — |
| 6 | ask.go:132-134 | `func CheckPermissions(...) toolapp.PermissionResult { return toolapp.PermissionAllow }` | A.1 | OK | Constant return; no error path. AskUserQuestion is benign by design. | N-A | — | — | — |
| 7 | ask.go:152-156 | `Execute: callID, _ := reqctxpkg.GetToolCallID(ctx); if callID == "" { return "Cannot ask the user: no tool_call_id in context (chat layer wiring bug).", nil }` | A.1/A.4 | OK | `_` discards `ok bool`, not an error — §S3 doesn't apply (non-error discard). Wiring-bug error returned as friendly LLM-readable string per the package's "tool_result string, not Go err" contract documented at lines 144-145. The string explicitly mentions "chat layer wiring bug" which is a §C concern (impl-detail leak in tool_result), but Phase A scope is §S3-S17; flag for Phase C. | N-A (Phase A) | — | — | — |
| 8 | ask.go:157 | `answer, err := t.svc.Wait(ctx, callID, t.timeout)` | A.4 | OK | Direct call; err is then classified via errors.Is below (sites 9-12). ctx is request ctx — appropriate for blocking wait that should release on conversation interrupt or browser close (NOT a §S9 terminal write situation). | N-A | — | — | — |
| 9 | ask.go:158-160 | `case errors.Is(err, askapp.ErrTimeout): return "User did not respond within the timeout. Re-ask later if still needed.", nil` | A.4 | OK | Sentinel-based discrimination via errors.Is. ErrTimeout is registered errmap.go:159 (504 ASK_TIMEOUT) but here translated to friendly LLM string per the package's contract. Both paths valid: framework gets nil err, LLM sees actionable "re-ask" hint. | N-A | — | — | — |
| 10 | ask.go:161-162 | `case errors.Is(err, context.Canceled): return "Question cancelled by the user (conversation interrupted).", nil` | A.1/A.4 | OK | Standard library context.Canceled → friendly tool_result. errmap.go:181 also registers context.Canceled at 499 CLIENT_CLOSED but here the convention is friendly-string for LLM consumption. The LLM-friendly "conversation interrupted" framing is appropriate. | N-A | — | — | — |
| 11 | ask.go:163-165 | `case err != nil: return fmt.Sprintf("Asking the user failed: %v", err), nil` | A.1/A.4 | EDGE | catch-all err path uses `%v` (not `%w`), but this is going INTO the LLM tool_result string output — not a Go err being propagated. So §S16's %w-required rule doesn't apply (errors.Is consumers don't see this string). However: any unexpected err (e.g. registration failure if svc.Wait surfaces it) would surface as `Asking the user failed: <err.Error()>` directly to the LLM. Acceptable given the contract (tool_result is text, not error chain), but a future caller wanting to discriminate "answer queue full" vs "service shut down" can't via Is. Worth a note for Phase B/C tool-result review but not a §S16 violation per spec. | LOW | LLM gets generic "asking failed" without specific cause; user can't readily diagnose vs registration backlog. | accept; document the contract that ALL svc.Wait errs collapse to friendly string. Or introduce a shared helper that prepends a stable label per err class. Defer until Phase C tool-result review. | FOUND |
| 12 | ask.go:166 | `return answer, nil` | A.1 | OK | Happy path; answer string returned to LLM as tool_result. No error path. | N-A | — | — | — |
| 13 | ask.go:171 | `var _ toolapp.Tool = (*AskUserQuestion)(nil)` | A.1 | OK | Compile-time interface satisfaction check — not a runtime error path. | N-A | — | — | — |

## Sub-check

A.1 §S3 错误吞没:
  - violations: not present
  - Site #7 has `_` discard on `GetToolCallID` 2nd return (ok bool, not error) — non-error discard, §S3 spec carve-out.

A.2 §S9 detached ctx 终态写:
  - terminal-state writes identified: none
  - 各自 ctx 来源: N/A
  - violations: N/A — package does not perform DB writes. AskUserQuestion only blocks on `t.svc.Wait(ctx, ...)` which is a synchronous wait, not a terminal write. Service-layer write of "answer received" lives in `app/ask` (out of scope). The `ctx` passed to Wait is the request ctx, which is correct: when conversation interrupts (browser close / cancel), the wait should unblock with `context.Canceled` and return a friendly tool_result — this is the intended cancellation semantics, not a §S9 violation.

A.3 §S15 ID 生成:
  - ID generation calls: none
  - violations: N/A — package doesn't generate business IDs. Tool-call ID is provided by the LLM (passed via ctx via `reqctxpkg.GetToolCallID`); this matches the §S21 invariant "tool_call block ID 复用 LLM 自带的 tool-call ID（如 tc_xxx）——不走 §S15 prefix 约定（LLM 不知道我们的 ID 体系）".

A.4 §S16 错误 wrap 格式:
  - violations: not present
  - Site #5 (ValidateInput json err) uses canonical `AskUserQuestion.ValidateInput: %w`. Sites 9-11 are tool_result string formatting (not Go err propagation), so §S16's `%w`-required rule doesn't apply per package's documented contract.

A.5 §S17 sentinel 登记 errmap:
  - sentinels defined: ErrEmptyQuestion (ask.go:54)
  - 已登记 errmap: ErrEmptyQuestion is NOT in errmap (verified via grep)
  - missing: N/A — ErrEmptyQuestion is consumed by the §S18 Tool framework's ValidateInput hook; framework converts to LLM-friendly tool_result and the err never reaches `responsehttpapi.FromDomainError`. Same N/A pattern as app-tool-shell ValidateInput sentinels (4 sentinels) and app-tool-mcp validation sentinels — audit-confirmed precedent.
  - Cross-package consumed: askapp.ErrTimeout (errmap.go:159 ✓), askapp.ErrNoPendingQuestion (errmap.go:157 ✓), askapp.ErrAlreadyAnswered (errmap.go:158 ✓). All 3 cross-package askapp sentinels registered.
