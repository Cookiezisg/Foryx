# agent.go — audit trace

Path: `backend/internal/app/tool/subagent/agent.go` (235 LOC, no `_test.go` audited per scope)

Single file in package; implements the `Subagent` system tool (LLM-facing), 9 §S18 methods. Delegates spawn lifecycle to `subagentapp.Service` (out of scope; see app-subagent audit if exists). This audit covers the tool-layer surface only: the 9 method bodies + validation sentinels + Execute terminal-state mapping.

## Trace table (9 columns + status)

| site# | file:line | snippet | category | classification | reasoning | severity | user_impact | suggested_fix | status |
|---|---|---|---|---|---|---|---|---|---|
| 1 | agent.go:47-55 | `var ( ErrEmptyPrompt = errors.New("prompt is required and must be non-empty"); ErrEmptyType = errors.New("subagent_type is required and must be non-empty") )` | A.5 | OK | Validation sentinels consumed by Tool framework (`runOneTool` ↦ failed tool_result). Never bubble to `responsehttpapi.FromDomainError` because they're wrapped inside §S18 tool_result text path, not handler-returned. Mirrors `mcptool.ErrEmptyServer` / `forgetool` validation sentinels — same precedent set in app-tool-mcp `_summary.md` ("Local validation sentinels are framework-consumed, not registered (correctly)"). No errmap row required. | — | — | — | — |
| 2 | agent.go:110-112 | `func (t *SubagentTool) Name() string { return "Subagent" }` / `Description()` / `Parameters()` | A.4 | OK | §S18 Identity 3 methods — pure returns, no error paths. `Parameters()` exposes `subagentSchema` (json.RawMessage const) without injecting the 3 standard fields (`summary`/`destructive`/`execution_group`); injection happens framework-side per §S18 §2 — correct. | — | — | — | — |
| 3 | agent.go:121-123 | `IsReadOnly() bool { return false }` / `NeedsReadFirst() bool { return false }` / `RequiresWorkspace() bool { return false }` | A.4 | OK | §S18 static-metadata 3 methods. `IsReadOnly=false` because sub-runner can invoke Write/Edit/Bash via inherited registry — conservatively correct per the inline godoc rationale. `NeedsReadFirst`/`RequiresWorkspace` both false because Subagent itself doesn't touch the FS — sub-runner's tools each enforce their own. | — | — | — | — |
| 4 | agent.go:132-147 | `func (t *SubagentTool) ValidateInput(args json.RawMessage) error { … if err := json.Unmarshal(args, &a); err != nil { return fmt.Errorf("SubagentTool.ValidateInput: %w", err) } … return ErrEmptyType … return ErrEmptyPrompt … }` | A.4 | OK | §S16 wrap: `<Type>.<Method>: %w` literal-form. Returns sentinels directly at innermost layers (line 141, 144), wraps json.Unmarshal failure at line 138 with proper prefix + %w. `errors.Is(err, ErrEmptyPrompt)` chain unbroken. | — | — | — | — |
| 5 | agent.go:149-151 | `func (t *SubagentTool) CheckPermissions(_ json.RawMessage, _ toolapp.PermissionMode) toolapp.PermissionResult { return toolapp.PermissionAllow }` | A.1 | OK | No error path; permission gate is framework-driven. Subagent currently always-allow because gating happens via the upstream chat-layer permission mode (LLM can spawn subagents whenever invoked), not at this level. Identical pattern to `searchforge` / `getforge` permission-allow. | — | — | — | — |
| 6 | agent.go:171-175 | `func (t *SubagentTool) Execute(ctx context.Context, argsJSON string) (string, error) { if depth := reqctxpkg.GetSubagentDepth(ctx); depth >= 1 { return "", fmt.Errorf("SubagentTool.Execute: %w (depth=%d)", subagentdomain.ErrRecursionAttempt, depth) } … }` | A.4 / A.5 | OK | §S16: `<Type>.<Method>:` prefix + sentinel via `%w` + extra `(depth=%d)` context. `errors.Is(err, subagentdomain.ErrRecursionAttempt)` unwraps cleanly. §S17: `subagentdomain.ErrRecursionAttempt` registered errmap.go:121 → 422 SUBAGENT_RECURSION. | — | — | — | — |
| 7 | agent.go:177-184 | `var args struct { … }; if err := json.Unmarshal([]byte(argsJSON), &args); err != nil { return "", fmt.Errorf("SubagentTool.Execute: parse args: %w", err) }` | A.4 | OK | §S16: `<Type>.<Method>: parse args: %w` — multi-segment prefix, %w preserved. Note this is a defensive double-parse: ValidateInput already ran the same Unmarshal at site #4, so practically argsJSON has been pre-validated by the framework before Execute fires (per §S18 hook chain). The duplicated parse is acceptable defensive style — handler-layer can never rely on framework wiring. | — | — | — | — |
| 8 | agent.go:186-195 | `res, err := t.svc.Spawn(ctx, args.SubagentType, args.Prompt, subagentapp.SpawnOpts{ MaxTurns: args.MaxTurns }); if err != nil { … return "", err }` | A.1 / A.4 / A.5 | OK | Hard-error path: `Spawn` returns errors for type-not-found / persist failure / LLM resolve failure. Comment at line 190-194 explicitly documents reliance on Spawn's already-wrapped %w chain so errmap can match `subagentdomain.ErrTypeNotFound` (registered errmap.go:120 → 404 SUBAGENT_TYPE_NOT_FOUND). No double-wrap — propagating upstream `%w` chain unchanged is correct per §S16 (don't re-wrap with `%v` and lose the chain; bare `return "", err` keeps it intact). §S3: errors not swallowed, they bubble. §S17: only `ErrTypeNotFound` and `ErrRecursionAttempt` reach handler from this tool — both registered. | — | — | — | — |
| 9 | agent.go:203-215 | `switch res.Status { case subagentapp.StatusMaxTurns: return appendNote(res.Result, "subagent hit max turns; …"), nil; case subagentapp.StatusCancelled: return appendNote(res.Result, "subagent was cancelled"), nil; case subagentapp.StatusFailed: if strings.TrimSpace(res.Result) != "" { return appendNote(res.Result, fmt.Sprintf("subagent failed: %s", res.ErrorMsg)), nil } return fmt.Sprintf("Subagent %s failed: %s", res.Type, res.ErrorMsg), nil; default: return res.Result, nil }` | A.1 | OK | §S18 / §S3 friendly tool_result path. `StatusMaxTurns` / `StatusCancelled` / `StatusFailed` are sub-run **terminal-status strings** (not Go errors) sourced from `subagentapp.SpawnResult.Status`. Per §S3: errors are NOT swallowed — they're surfaced to the LLM as readable text (max-turns hint / cancellation note / failure msg), which is the textbook friendly-tool_result pattern explicitly carved out by §S18 and the package godoc lines 17-21 ("max-turns / cancelled terminations are converted to friendly tool_result strings"). Identical pattern to `mcptool::mapCallToolErrorToFriendly` — referenced in app-tool-mcp `_summary.md` as a model. The errmap.go inline comment lines 113-119 explicitly preempts this: "Only the first two reach handlers; ErrMaxTurnsExceeded / ErrCancelled are converted to friendly tool_result strings by SubagentTool.Execute and never propagate." | — | — | — | — |
| 10 | agent.go:209-212 | (Failed sub-branch detail) `if strings.TrimSpace(res.Result) != "" { return appendNote(res.Result, fmt.Sprintf("subagent failed: %s", res.ErrorMsg)), nil } return fmt.Sprintf("Subagent %s failed: %s", res.Type, res.ErrorMsg), nil` | A.1 | OK | Both branches preserve `res.ErrorMsg` (populated by Spawn whenever Status=Failed). Empty-result branch falls back to a synthesized message containing the type slug + ErrorMsg — no info lost. §S3 not violated; failure mode is **visible** to the LLM as a typed message. | — | — | — | — |
| 11 | agent.go:224-230 | `func appendNote(body, note string) string { body = strings.TrimSpace(body); if body == "" { return fmt.Sprintf("[note: %s]", note) }; return body + "\n\n[note: " + note + "]" }` | A.4 | OK | Pure string-formatting helper. No error paths. Used only by the §S18 friendly-status switch above. | — | — | — | — |

(11 sites total — corresponding to the 9 §S18 methods + 2 helpers; ValidateInput, Execute, Spawn-hard-err, Status-switch, and Failed-sub-branch each broken out separately to match the violation-finding granularity used in app-tool-mcp.)

## Sub-checks

```
A.1 §S3 错误吞没:
  - violations: not present
  - notes: site #9 (status switch) / #10 (failed branch) are §S18 friendly-tool_result conversions, NOT swallowed errors —
    failure information remains visible to LLM as readable text (MaxTurns hint / "subagent was cancelled" / "subagent failed: <ErrorMsg>");
    spec §S3 + package godoc explicitly carve out this pattern; identical to mcptool::mapCallToolErrorToFriendly precedent.
  - sites #6, #7, #8 (recursion / parse / Spawn err) all bubble error up cleanly with %w chain preserved. No "_ = err",
    no `if err != nil { return nil }`, no silent fallback.

A.2 §S9 detached ctx 终态写:
  - terminal-state writes identified: NONE in this file
  - 各自 ctx 来源: N/A
  - violations: N/A — package only forwards ctx into Service.Spawn (line 186); does not perform any DB / file persistence
    itself. Terminal-state writes (subagent_runs row insert/finalize, message_blocks emit, eventlog Stop on cancellation)
    happen inside `app/subagent.Service` (out of scope; covered by app-subagent audit). Tool-layer correctly delegates
    without re-implementing §S9 logic — same posture as app-tool-mcp / app-tool-forge.

A.3 §S15 ID 生成:
  - ID generation calls: NONE (no idgen.New, no crypto/rand, no math/rand, no UnixNano)
  - violations: N/A — package doesn't generate business IDs. The `sar_` / `smm_` prefixes (subagent run ID / subagent
    message ID per §S15) are generated inside `app/subagent.Service.Spawn` and the eventlog block emitter respectively,
    both downstream of this tool layer (out of scope for this audit).

A.4 §S16 错误 wrap 格式:
  - violations: not present
  - 3 wrap sites total: site #4 (`SubagentTool.ValidateInput: %w`) / site #6 (`SubagentTool.Execute: %w (depth=%d)`)
    / site #7 (`SubagentTool.Execute: parse args: %w`). All use literal `<Type>.<Method>:` prefix + %w. Sentinels
    (ErrEmptyType / ErrEmptyPrompt / ErrRecursionAttempt) are at innermost layer.
  - bare `return "", err` at site #8 propagates upstream Spawn's wrap chain unchanged — correct (don't re-wrap to avoid
    losing or duplicating prefixes).

A.5 §S17 sentinel 登记 errmap:
  - sentinels defined in this package:
    - ErrEmptyPrompt (errors.New) — local validation, framework-consumed via runOneTool failed tool_result; no errmap row needed
    - ErrEmptyType (errors.New) — same as above
  - sentinels imported & propagated to handler (§S17-relevant):
    - subagentdomain.ErrRecursionAttempt — site #6, registered errmap.go:121 → 422 SUBAGENT_RECURSION ✓
    - subagentdomain.ErrTypeNotFound — site #8 (via Spawn's %w chain), registered errmap.go:120 → 404 SUBAGENT_TYPE_NOT_FOUND ✓
  - errmap.go:113-119 inline comment explicitly states only these two propagate; MaxTurns / Cancelled are handled inline
    via §S18 friendly-text conversion (sites #9 / #10) — verified matches code.
  - missing: all registered.
  - doc nit (out of scope): errmap.go:114 mentions "ErrMaxTurnsExceeded / ErrCancelled" by name but the actual code uses
    StatusMaxTurns / StatusCancelled string constants (subagentapp.SpawnResult.Status field) rather than Go error sentinels —
    the comment names entities that don't exist as `var Err...`. Doc-only inconsistency in errmap.go, not a code bug.
    Flagged here only to surface it; fix belongs in a doc-cleanup pass on errmap.go, not this audit's scope.
```
