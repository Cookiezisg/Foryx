# audit: backend/internal/app/tool/forge/run.go

LOC: 111
Read: full file (lines 1-111)

## Trace

| site# | file:line | snippet | category | classification | reasoning | severity | user_impact | suggested_fix | status |
|---|---|---|---|---|---|---|---|---|---|
| 1 | run.go:37-44 | Identity / Description | N/A | OK | §S18 metadata; description explicitly tells LLM that "Execution failures return ok=false (not an error)" — clear behavior contract. Truncation behavior also documented. Clean. | N-A | — | — | — |
| 2 | run.go:46-55 | Parameters / metadata IsReadOnly=false / others false | N/A | OK | §S18 — RunForge is mutating (executes user code; could write files via forge). RequiresWorkspace=false because forge runs in sandbox not user workspace. | N-A | — | — | — |
| 3 | run.go:66-69 | `ValidateInput=nil / CheckPermissions=Allow` | N/A | OK | matches CreateForge / EditForge pattern; user-confirmed forges are pre-vetted at activation time | N-A | — | — | — |
| 4 | run.go:79-81 | `if err := json.Unmarshal([]byte(argsJSON), &args); err != nil { return "", fmt.Errorf("run_forge: bad args: %w", err) }` | A.4 | EDGE | §S16: `run_forge:` tool-name prefix | LOW | identical UX | tighten | FOUND |
| 5 | run.go:82-85 | `resolved, err := resolveAttachments(ctx, t.attachRepo, args.Input); if err != nil { return "", fmt.Errorf("run_forge: resolve attachments: %w", err) }` | A.4 | EDGE | same prefix; resolveAttachments helper has its own `resolveAttachments:` prefix → composed `run_forge: resolve attachments: resolveAttachments:` (verbose). chatdomain.Err* sentinel chain via repo.GetAttachment. | LOW | identical UX | tighten both prefixes | FOUND |
| 6 | run.go:86-89 | `result, err := t.svc.RunForge(ctx, args.ForgeID, resolved); if err != nil { return "", fmt.Errorf("run_forge: %w", err) }` | A.4 | EDGE | same prefix. NOTE: per Description (#1), the design intent is execution failures return ok=false (in result struct), NOT err. So this err path is for system-level failures (forge missing / venv broken / etc.) — which correctly bubble forgedomain.Err* sentinels. | LOW | identical UX | tighten | FOUND |
| 7 | run.go:97-101 | `output := result.Output; if rawOut, err := json.Marshal(output); err == nil && len(rawOut) > maxOutputBytes { output = fmt.Sprintf("[output truncated: ...]", ...) }` | A.1 | OK | §S3 silent: json.Marshal err ignored via `if ... err == nil && ...` — but result.Output came from forge execution result; if Marshal of `any` value fails it would be a non-recoverable type issue. The `err == nil &&` guard means oversize-truncation only fires on successful marshal — if Marshal fails, truncation skipped, output passed through to next Marshal at #8 where it'd fail again. **Borderline**: documented intent at lines 91-96 is "measure size before truncate". If Marshal fails here, the oversize-check is skipped silently. | LOW | rare path: forge returned a non-marshalable Go value (shouldn't happen — Python output deserializes to basic types). Could bubble forge code bug as silent oversize-skip. | minor: `if rawOut, err := json.Marshal(output); err != nil { ... } else if len(rawOut) > maxOutputBytes { ... truncate ... }` to make the err-path explicit (would still be unreachable in practice). Audit-acceptable as current. | FOUND |
| 8 | run.go:103-109 | `b, _ := json.Marshal(map[string]any{...}); return string(b), nil` | A.1 | OK | basic-type map Marshal, unfailable. result.Output is either basic types from forge or a string from #7 truncation. Discard `_` safe-by-construction. | N-A | — | — | — |

## Sub-check

A.1 §S3 错误吞没:
  - violations: not present (site #7 is borderline-EDGE but unreachable in practice + documented intent)

A.2 §S9 detached ctx 终态写:
  - terminal-state writes identified: none directly. t.svc.RunForge writes execution history (forge_executions table per §S15 fe_ prefix) but that's in app/forge service (DEFERRED).
  - 各自 ctx 来源: request ctx
  - violations: N/A here — service layer pending rewrite

A.3 §S15 ID 生成:
  - ID generation calls: none (RunForge.svc.RunForge generates fe_ prefix internally — out of scope per app/forge audit)
  - violations: N/A

A.4 §S16 错误 wrap 格式:
  - violations: sites #4, #5, #6 (`run_forge:` tool-name prefix). WAIVE per package consistency.

A.5 §S17 sentinel 登记 errmap:
  - sentinels defined: none
  - 已登记 errmap: forgedomain.ErrNotFound (errmap.go:80), ErrRunFailed (errmap.go:86) — reached through svc.RunForge; chatdomain.Err* via repo.GetAttachment
  - missing: not present
