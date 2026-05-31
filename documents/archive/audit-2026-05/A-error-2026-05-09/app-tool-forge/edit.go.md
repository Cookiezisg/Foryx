# audit: backend/internal/app/tool/forge/edit.go

LOC: 227
Read: full file (lines 1-227)

## Trace

| site# | file:line | snippet | category | classification | reasoning | severity | user_impact | suggested_fix | status |
|---|---|---|---|---|---|---|---|---|---|
| 1 | edit.go:64-92 | Identity / Description / Parameters | N/A | OK | §S18 metadata; description notes the dual-path flow (instruction = code regen, no instruction = metadata-only). Clean. | N-A | — | — | — |
| 2 | edit.go:96-98 | `IsReadOnly()=false / NeedsReadFirst()=false / RequiresWorkspace()=false` | N/A | OK | §S18 metadata; mutating but doesn't touch user fs (forge venv under managed sandbox dir) | N-A | — | — | — |
| 3 | edit.go:103-107 | `ValidateInput=nil / CheckPermissions=Allow` | N/A | OK | matches CreateForge pattern | N-A | — | — | — |
| 4 | edit.go:120-122 | `if err := json.Unmarshal([]byte(argsJSON), &args); err != nil { return "", fmt.Errorf("edit_forge: bad args: %w", err) }` | A.4 | EDGE | §S16: `edit_forge:` tool-name prefix | LOW | identical UX | tighten | FOUND |
| 5 | edit.go:123-126 | `current, err := t.svc.Get(ctx, args.ForgeID); if err != nil { return "", fmt.Errorf("edit_forge: get forge: %w", err) }` | A.4 | EDGE | same prefix. forgedomain.ErrNotFound chain preserved → 404 | LOW | identical UX | tighten | FOUND |
| 6 | edit.go:138-141 | `if current.Pending != nil { if err := t.svc.RejectPending(ctx, args.ForgeID); err != nil { return "", fmt.Errorf("edit_forge: reject existing pending: %w", err) } }` | A.4 | EDGE | same prefix. Reject is a state mutation but error path is "couldn't clean previous pending" — surfaced cleanly. | LOW | identical UX | tighten | FOUND |
| 7 | edit.go:148 | `pendingID := forgeapp.NewVersionID()` | A.3 | OK | §S15 delegated | N-A | — | — | — |
| 8 | edit.go:186-196 | `newCode, err := streamCode(ctx, buildEditPrompt(...), ..., func(accumulated){...}); if err != nil { return "", fmt.Errorf("edit_forge: generate code: %w", err) }` | A.4 | EDGE | same prefix. Composed prefix = `edit_forge: generate code: streamCode: <inner>` | LOW | identical UX | tighten | FOUND |
| 9 | edit.go:199-201 | `if err := t.svc.ParseCode(newCode); err != nil { return "", fmt.Errorf("edit_forge: generated code failed AST parse, please regenerate: %w", err) }` | A.4 | EDGE | same prefix. Same actionable "please regenerate" pattern as create.go #8. | LOW | identical UX | tighten | FOUND |
| 10 | edit.go:205-208 | `pending, err := t.svc.CreatePending(ctx, args.ForgeID, snap); if err != nil { return "", fmt.Errorf("edit_forge: create pending: %w", err) }` | A.4 | EDGE | same prefix | LOW | identical UX | tighten | FOUND |
| 11 | edit.go:210-216 | `b, _ := json.Marshal(map[string]any{...})` | A.1 | OK | basic-type map Marshal, unfailable | N-A | — | — | — |
| 12 | edit.go:221-226 | `func pickNonEmpty(a, b string) string { ... }` | N/A | OK | pure utility, no error path | N-A | — | — | — |

## Sub-check

A.1 §S3 错误吞没:
  - violations: not present

A.2 §S9 detached ctx 终态写:
  - terminal-state writes identified: t.svc.RejectPending (#6), t.svc.CreatePending (#10). Both go through ctx.
  - 各自 ctx 来源: request ctx (synchronous tool execution)
  - violations: N/A here — service layer DEFERRED per forge rewrite

A.3 §S15 ID 生成:
  - ID generation calls: forgeapp.NewVersionID (line 148). Delegates to idgenpkg.
  - violations: not present

A.4 §S16 错误 wrap 格式:
  - violations: sites #4, #5, #6, #8, #9, #10 (`edit_forge:` tool-name prefix). WAIVE per package consistency.

A.5 §S17 sentinel 登记 errmap:
  - sentinels defined: none in this file
  - 已登记 errmap: forgedomain.ErrNotFound, ErrPendingNotFound (errmap.go:80, 83), ErrPendingConflict (errmap.go:84), ErrASTParseError (errmap.go:87) — all reached through svc method calls
  - missing: not present
