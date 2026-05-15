# Audit trace — internal/app/tool/todo/create.go

**LOC**: 148
**Purpose**: TodoCreate system tool (LLM-facing) — implements §S18 9-method contract; delegates to `*todoapp.Service.Create`. Hosts shared helpers `classifyTodoErr` + `marshalIndent` used by all 4 todo tools (so they live in this file as the alphabetically-first concept file rather than splitting into `helpers.go` per §S12 "by concept" rule — borderline, see Findings).

## Trace table

| site# | file:line | snippet | category | classification | reasoning | severity | user_impact | suggested_fix | status |
|---|---|---|---|---|---|---|---|---|---|
| 1 | create.go:74-85 | `func (t *TodoCreate) ValidateInput(args json.RawMessage) error { var a struct{Subject string ...}; if err := json.Unmarshal(args, &a); err != nil { return fmt.Errorf("TodoCreate.ValidateInput: %w", err) }; if strings.TrimSpace(a.Subject) == "" { return tododomain.ErrSubjectRequired }; return nil }` | A.1/A.4/A.5 | OK | §S16 prefix `TodoCreate.ValidateInput:` + `%w`; §S18 returns sentinel `tododomain.ErrSubjectRequired` (registered errmap.go:97). No silent error path — Unmarshal err and empty-subject both surfaced. | N-A | — | — | — |
| 2 | create.go:87-89 | `func (t *TodoCreate) CheckPermissions(_ json.RawMessage, _ toolapp.PermissionMode) toolapp.PermissionResult { return toolapp.PermissionAllow }` | A.1 | OK | All 4 todo tools mutate / read conversation-scoped TODO list; no path / fs / network involved. PermissionAllow is correct per §S18 (no permission gate needed). Ignored args are typed as blank identifier — not a §S3 error suppression. | N-A | — | — | — |
| 3 | create.go:95-104 | `func (t *TodoCreate) Execute(ctx context.Context, argsJSON string) (string, error) { var args struct{...}; if err := json.Unmarshal([]byte(argsJSON), &args); err != nil { return "", fmt.Errorf("TodoCreate.Execute: %w", err) }; ...` | A.4 | OK | §S16 prefix `TodoCreate.Execute:` + `%w`. Unmarshal err returned to framework via 2nd return slot (will surface as failed tool_result text). | N-A | — | — | — |
| 4 | create.go:105-113 | `created, err := t.svc.Create(ctx, todoapp.CreateInput{...}); if err != nil { return classifyTodoErr(err, "create"), nil }; return marshalIndent(created)` | A.1/A.2 | OK | §S18 friendly tool_result pattern: Service err → human text via `classifyTodoErr`, returned as 1st arg with `nil` err — LLM sees readable text instead of error chain. NOT silent — sentinel branches preserve recovery hints, default branch surfaces full err.Error() (`%v` is fine for human-text content per app-tool-mcp precedent call.go:#5). §S9 N/A — service.Create owns the terminal write; ctx passes through unchanged (todo write is part of an ongoing user/LLM step, not a post-cancel finalization — caller cancel = abort is correct). | N-A | — | — | — |
| 5 | create.go:125-135 | `func classifyTodoErr(err error, op string) string { switch { case errors.Is(err, tododomain.ErrNotFound): return "Todo not found in this conversation." case errors.Is(err, tododomain.ErrSubjectRequired): return "..." case errors.Is(err, tododomain.ErrInvalidStatus): return "..." } return fmt.Sprintf("Todo %s failed: %v", op, err) }` | A.1/A.4/A.5 | OK | §S18 sentinel-classified friendly text. All 3 tododomain sentinels (ErrNotFound / ErrSubjectRequired / ErrInvalidStatus) have errmap.go:96-98 entries — chain integrity intact via `errors.Is`. Default uses `%v` which IS correct here because the function returns a friendly string (not propagated error chain) — same pattern as app-tool-mcp call.go:#5 (verified consistent across other tool packages). | N-A | — | — | — |
| 6 | create.go:141-147 | `func marshalIndent(v any) (string, error) { body, err := json.MarshalIndent(v, "", "  "); if err != nil { return "", fmt.Errorf("marshal: %w", err) }; return string(body), nil }` | A.4 | EDGE | §S16 prefix is `marshal:` — missing the `<pkg>.<Method>` literal (`todotool.marshalIndent:` would be canonical). Helper-style prefix consistent with sibling app-tool-mcp / app-tool-shell / app-tool-forge precedent which the audit-summary chain has WAIVED (commit 64d9535). Also: `json.MarshalIndent` over a `*tododomain.Todo` (struct of plain types) is essentially unfailable; the err path is reachable only on impossible cycles. | LOW | LLM sees `marshal: <err>` in the rare failure path — no `tool/`-layer locator, but human-readable. | WAIVE per established `<helper>:` prefix precedent (commit 64d9535). Optional: `todotool.marshalIndent:` for parity with strict §S16. | FOUND |

## Sub-check

- **A.1 §S3 错误吞没**: not present — every error path either returns the err to framework (Unmarshal in ValidateInput / Execute), surfaces via §S18 friendly text (Service.Create), or wraps via `%w` (marshalIndent).
- **A.2 §S9 detached ctx 终态写**:
  - terminal-state writes identified: site #4 (Service.Create persists Todo + publishes notification SSE)
  - 各自 ctx 来源: passed-through `ctx` from framework
  - violations: not present — Todo creation is an in-flight user operation step, NOT a post-cancel finalization. If LLM/caller cancels mid-call, aborting the create is the correct semantic (no data loss — user just won't see the new todo). §S9 detached pattern is for "must-persist-after-cancel" terminal writes (e.g. final assistant message after streaming cancel); todo create is not in that bucket.
- **A.3 §S15 ID 生成**:
  - ID generation calls: none in this file
  - violations: N/A — `td_` prefix ID is generated inside `*todoapp.Service.Create` (delegated to `idgenpkg.New("td")` per §S15); tool layer correctly defers.
- **A.4 §S16 错误 wrap 格式**:
  - violations: site #6 EDGE LOW — `marshal:` helper-style prefix lacks `todotool.marshalIndent:` literal. Consistent with WAIVED app-tool-* precedent.
- **A.5 §S17 sentinel 登记 errmap**:
  - sentinels defined: none in this file (all consumed sentinels are from `tododomain`)
  - 已登记 errmap: ErrNotFound (errmap.go:96), ErrSubjectRequired (errmap.go:97), ErrInvalidStatus (errmap.go:98) — all 3 present
  - missing: all registered. Note: tool layer doesn't reach errmap directly (errors are converted to friendly text via §S18); registration matters because the same sentinels DO reach handlers via the REST `POST /api/v1/todos` path. Coverage independently confirmed.

**Findings**: 0 HIGH, 0 MED, 1 LOW EDGE (`marshal:` helper-style prefix on §S16 — WAIVE per established precedent). File is §S18 textbook-clean for the create + shared-helper layer.
