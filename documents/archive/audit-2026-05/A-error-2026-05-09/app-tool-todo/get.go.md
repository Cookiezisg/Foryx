# Audit trace — internal/app/tool/todo/get.go

**LOC**: 80
**Purpose**: TodoGet system tool (LLM-facing) — fetch one todo by ID. §S18 9-method contract; delegates to `*todoapp.Service.Get`. Read-only.

## Trace table

| site# | file:line | snippet | category | classification | reasoning | severity | user_impact | suggested_fix | status |
|---|---|---|---|---|---|---|---|---|---|
| 1 | get.go:50-61 | `func (t *TodoGet) ValidateInput(args json.RawMessage) error { var a struct{TodoID string ...}; if err := json.Unmarshal(args, &a); err != nil { return fmt.Errorf("TodoGet.ValidateInput: %w", err) }; if strings.TrimSpace(a.TodoID) == "" { return errors.New("todo_id is required") }; return nil }` | A.1/A.4/A.5 | EDGE | Unmarshal err: §S16 prefix `TodoGet.ValidateInput:` + `%w` ✓. Empty-ID branch returns **bare** `errors.New("todo_id is required")` instead of a domain sentinel (compare TodoCreate.ValidateInput which uses `tododomain.ErrSubjectRequired`). The bare error is not registered in errmap.go and is not classified by `classifyTodoErr`, so on the very rare path where ValidateInput's error reaches a handler / classify path, it falls through to the §S16 default. **Inconsistency with sibling create.go** which uses a domain sentinel for the parallel "X required" check. Same pattern as app-tool-mcp install_server.go #2 / uninstall_server.go #1 (also flagged LOW EDGE in that fork). | LOW | LLM sees `validate input: todo_id is required` (or generic "Todo get failed: …" if it leaks past the framework's pre-Execute gate); user impact zero — message is still readable. The inconsistency matters more for **maintenance** than runtime: another tool author is now uncertain whether to use sentinels or bare errors here. | Either (a) add `tododomain.ErrTodoIDRequired` sentinel + register errmap (parallel to ErrSubjectRequired) for consistency, OR (b) WAIVE per the same precedent as mcp tool helper-style `errors.New` for arg-validation. Audit-recommend (b) — bare error is acceptable in pre-Execute validation since framework intercepts before tool_result wire-up. | FOUND |
| 2 | get.go:63-65 | `func (t *TodoGet) CheckPermissions(_ json.RawMessage, _ toolapp.PermissionMode) toolapp.PermissionResult { return toolapp.PermissionAllow }` | A.1 | OK | Read-only conversation-scoped TODO fetch; PermissionAllow correct per §S18. | N-A | — | — | — |
| 3 | get.go:67-73 | `func (t *TodoGet) Execute(ctx context.Context, argsJSON string) (string, error) { var args struct{TodoID string ...}; if err := json.Unmarshal([]byte(argsJSON), &args); err != nil { return "", fmt.Errorf("TodoGet.Execute: %w", err) }; ...` | A.4 | OK | §S16 prefix `TodoGet.Execute:` + `%w`. Unmarshal err returned via 2nd return slot. | N-A | — | — | — |
| 4 | get.go:74-78 | `got, err := t.svc.Get(ctx, args.TodoID); if err != nil { return classifyTodoErr(err, "get"), nil }; return marshalIndent(got)` | A.1/A.2 | OK | §S18 friendly tool_result via shared `classifyTodoErr` (see create.go #5 — sentinel chain to ErrNotFound covered). §S9 N/A — read-only Get does not perform terminal write; ctx pass-through is correct (cancel = abort fetch). | N-A | — | — | — |

## Sub-check

- **A.1 §S3 错误吞没**: not present — every error path either returned or surfaced via §S18 friendly text.
- **A.2 §S9 detached ctx 终态写**:
  - terminal-state writes identified: none — TodoGet is a pure read.
  - 各自 ctx 来源: passed-through ctx
  - violations: N/A — read-only tool.
- **A.3 §S15 ID 生成**:
  - ID generation calls: none in this file
  - violations: N/A — TodoGet consumes IDs, doesn't generate them.
- **A.4 §S16 错误 wrap 格式**:
  - violations: site #1 EDGE LOW — bare `errors.New("todo_id is required")` instead of a domain sentinel. NOT a §S16 wrap-format violation per se (it's a fresh error, not a wrap), but inconsistent with sibling `create.go` style.
- **A.5 §S17 sentinel 登记 errmap**:
  - sentinels defined: none (the bare errors.New at site #1 is anonymous, not a named sentinel)
  - 已登记 errmap: N/A
  - missing: the bare `errors.New("todo_id is required")` is **technically unmappable** (no var to register) — but it's intercepted before reaching errmap (framework pre-Execute gate). If maintainer chose to extract it to `var ErrTodoIDRequired = errors.New(...)`, registration would be required. Audit-recommend: either extract + register, or WAIVE.

**Findings**: 0 HIGH, 0 MED, 1 LOW EDGE (bare `errors.New` for arg validation, inconsistent with create.go's sentinel pattern — WAIVE per app-tool-mcp precedent).
