# Audit trace — internal/app/tool/todo/update.go

**LOC**: 152
**Purpose**: TodoUpdate system tool (LLM-facing) — partial update via pointer-field "set vs leave unchanged" semantics. §S18 9-method contract; delegates to `*todoapp.Service.Update` (or `Service.Delete` if `status="deleted"`). Most complex of the 4 todo tools.

## Trace table

| site# | file:line | snippet | category | classification | reasoning | severity | user_impact | suggested_fix | status |
|---|---|---|---|---|---|---|---|---|---|
| 1 | update.go:87-99 | `func (t *TodoUpdate) ValidateInput(args json.RawMessage) error { var a updateRaw; if err := json.Unmarshal(args, &a); err != nil { return fmt.Errorf("TodoUpdate.ValidateInput: %w", err) }; if strings.TrimSpace(a.TodoID) == "" { return errors.New("todo_id is required") }; if a.Status != nil && !tododomain.IsValidStatus(*a.Status) { return tododomain.ErrInvalidStatus }; return nil }` | A.1/A.4/A.5 | EDGE | (a) Unmarshal err: §S16 prefix `TodoUpdate.ValidateInput:` + `%w` ✓. (b) Empty `todo_id` branch returns **bare** `errors.New("todo_id is required")` — same inconsistency flagged in get.go #1 (LOW EDGE). (c) Invalid status branch correctly returns `tododomain.ErrInvalidStatus` (registered errmap.go:98). The mix of "bare errors.New for IDs / domain sentinel for status enum" is internally inconsistent within this single function — though both checks are pre-Execute (intercepted before tool_result wire-up), the inconsistency hurts maintenance. | LOW | LLM sees `validate input: todo_id is required` for empty ID, classified text for invalid status. User impact zero — both messages readable. Maintenance: future maintainer asks "why one bare and one sentinel?" — answer is just historical accident. | WAIVE per app-tool-mcp precedent (consistent with get.go #1 stance). Optional uplift: extract `tododomain.ErrTodoIDRequired` for consistency. | FOUND |
| 2 | update.go:101-103 | `func (t *TodoUpdate) CheckPermissions(_ json.RawMessage, _ toolapp.PermissionMode) toolapp.PermissionResult { return toolapp.PermissionAllow }` | A.1 | OK | Mutating tool but conversation-scoped TODO list is in user's own scope; no path / fs / network involved. PermissionAllow consistent with TodoCreate. | N-A | — | — | — |
| 3 | update.go:125-129 | `func (t *TodoUpdate) Execute(ctx context.Context, argsJSON string) (string, error) { var raw updateRaw; if err := json.Unmarshal([]byte(argsJSON), &raw); err != nil { return "", fmt.Errorf("TodoUpdate.Execute: %w", err) }; ...` | A.4 | OK | §S16 prefix `TodoUpdate.Execute:` + `%w`. | N-A | — | — | — |
| 4 | update.go:133-138 | `if raw.Status != nil && *raw.Status == tododomain.StatusDeleted { if err := t.svc.Delete(ctx, raw.TodoID); err != nil { return classifyTodoErr(err, "delete"), nil }; return fmt.Sprintf(`{"deleted":true,"id":%q}`, raw.TodoID), nil }` | A.1/A.2 | OK | §S18 friendly tool_result on Delete err via shared `classifyTodoErr` (sentinel chain to ErrNotFound + ErrInvalidStatus covered). Soft-delete success returns hand-built JSON literal `{"deleted":true,"id":"..."}` — `fmt.Sprintf` with `%q` on a string is unfailable, no error path needed. §S9 N/A — Delete is part of an in-flight LLM step, ctx pass-through correct (cancel = abort delete is the right semantic). | N-A | — | — | — |
| 5 | update.go:140-151 | `updated, err := t.svc.Update(ctx, raw.TodoID, todoapp.UpdateInput{...}); if err != nil { return classifyTodoErr(err, "update"), nil }; return marshalIndent(updated)` | A.1/A.2 | OK | Same §S18 friendly-classify pattern. Service.Update terminal write delegated; ctx pass-through correct. | N-A | — | — | — |
| 6 | update.go:137 | `return fmt.Sprintf(`+"`{\"deleted\":true,\"id\":%q}`"+`, raw.TodoID), nil` | A.1 | EDGE | Hand-built JSON via `fmt.Sprintf` instead of `json.Marshal`. **Not a §S3-S17 violation** — output is well-formed since `%q` quote-escapes the string per Go spec; if `raw.TodoID` contained a literal backtick / quote it would still render correct JSON. Style-wise inconsistent with the rest of the package (which uses `marshalIndent` / `MarshalIndent`). LLM-facing output is also non-indented unlike all other tool returns. | LOW | LLM sees compact `{"deleted":true,"id":"td_..."}` vs indented JSON elsewhere — readable but inconsistent. | Optional: replace with `marshalIndent(struct{Deleted bool; ID string}{...})` for output-format consistency. WAIVE-eligible (functional behavior is correct). | FOUND |

## Sub-check

- **A.1 §S3 错误吞没**: not present — every error path either returned (Unmarshal, validation) or surfaced via §S18 friendly classify (Service.Delete, Service.Update). Hand-built JSON at site #6 is style only, not a swallow.
- **A.2 §S9 detached ctx 终态写**:
  - terminal-state writes identified: site #4 (Service.Delete soft-delete + SSE broadcast) and site #5 (Service.Update + SSE broadcast)
  - 各自 ctx 来源: passed-through `ctx` from framework (both)
  - violations: not present — TodoUpdate / TodoDelete are part of an in-flight LLM step (LLM is mid-turn calling the tool). Caller cancel = abort the operation is correct semantic; user has not yet seen the change so "no terminal write needed" applies. §S9 detached pattern is for "must-persist-after-cancel" (e.g. final assistant message after streaming cancel). Tool-driven todo mutations are not in that bucket. The `Service.Delete` SSE broadcast must fire for any subscriber UI to drop the deleted todo, but if the call is cancelled before Delete commits, nothing was broadcast — consistent state.
- **A.3 §S15 ID 生成**:
  - ID generation calls: none in this file
  - violations: N/A — TodoUpdate consumes the existing ID, doesn't generate.
- **A.4 §S16 错误 wrap 格式**:
  - violations: site #1 EDGE LOW — bare `errors.New` for "todo_id is required" (consistent with get.go #1 — same WAIVE recommendation).
- **A.5 §S17 sentinel 登记 errmap**:
  - sentinels defined: none new in this file
  - 已登记 errmap: ErrInvalidStatus (errmap.go:98) consumed at site #1, ErrNotFound + ErrInvalidStatus consumed via shared classify
  - missing: bare `errors.New` at site #1 is anonymous — same recommendation as get.go (extract to sentinel + register OR WAIVE). All actual sentinels covered.

**Findings**: 0 HIGH, 0 MED, 2 LOW EDGE (bare `errors.New` for arg validation site #1 — WAIVE; hand-built JSON literal site #6 — WAIVE-eligible style inconsistency). Same WAIVE recommendation as get.go.
