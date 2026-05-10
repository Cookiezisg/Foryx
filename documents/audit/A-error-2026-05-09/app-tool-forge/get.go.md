# audit: backend/internal/app/tool/forge/get.go

LOC: 90
Read: full file (lines 1-90)

## Trace

| site# | file:line | snippet | category | classification | reasoning | severity | user_impact | suggested_fix | status |
|---|---|---|---|---|---|---|---|---|---|
| 1 | get.go:25-46 | Identity / Description / Parameters / metadata | N/A | OK | §S18 metadata — read-only single forge fetch, accurate | N-A | — | — | — |
| 2 | get.go:51-55 | `ValidateInput / CheckPermissions=Allow` | N/A | OK | no validation; read-only allow | N-A | — | — | — |
| 3 | get.go:63-65 | `if err := json.Unmarshal([]byte(argsJSON), &args); err != nil { return "", fmt.Errorf("get_forge: bad args: %w", err) }` | A.4 | EDGE | §S16: `get_forge:` tool-name prefix vs canonical `forgetool.GetForge.Execute:` | LOW | identical UX | tighten prefix | FOUND |
| 4 | get.go:66-69 | `detail, err := t.svc.GetDetail(ctx, args.ForgeID); if err != nil { return "", fmt.Errorf("get_forge: %w", err) }` | A.4 | EDGE | same prefix issue. forgedomain.ErrNotFound from svc.GetDetail unwraps cleanly to errmap.go:80 → 404 TOOL_NOT_FOUND. | LOW | identical UX (sentinel reaches errmap) | same | FOUND |
| 5 | get.go:71-73 | `var params, ret any; if err := json.Unmarshal([]byte(detail.Parameters), &params); err != nil { return "", fmt.Errorf("get_forge: corrupted parameters for forge %q: %w", args.ForgeID, err) }` | A.4 | EDGE | §S16: same `get_forge:` prefix; **POSITIVE behavior** — unlike search.go #12 which silently swallows the same Unmarshal failure, get.go surfaces it as an error. Inconsistency between search.go (silent on corruption) and get.go (loud on corruption) — both are documented design choices but the divergence may surprise users (search shows the forge, get refuses it). | LOW | identical UX (sentinel preserved); user sees clean error msg with forge_id; harder grep | tighten prefix | FOUND |
| 6 | get.go:74-76 | `if err := json.Unmarshal([]byte(detail.ReturnSchema), &ret); err != nil { return "", fmt.Errorf("get_forge: corrupted return_schema for forge %q: %w", args.ForgeID, err) }` | A.4 | EDGE | same as #5 | LOW | identical UX | same | FOUND |
| 7 | get.go:87-88 | `b, _ := json.Marshal(out); return string(b), nil` | A.1 | OK | json.Marshal of `map[string]any` with basic-type values + already-unmarshaled `params` / `ret` (which are `any`). Unfailable per encoding/json invariant. | N-A | — | — | — |

## Sub-check

A.1 §S3 错误吞没:
  - violations: not present

A.2 §S9 detached ctx 终态写:
  - terminal-state writes identified: none
  - 各自 ctx 来源: N/A
  - violations: N/A — get is read-only

A.3 §S15 ID 生成:
  - ID generation calls: none
  - violations: N/A

A.4 §S16 错误 wrap 格式:
  - violations: sites #3, #4, #5, #6 (`get_forge:` tool-name prefix). Audit-recommended WAIVE per package-wide consistency.

A.5 §S17 sentinel 登记 errmap:
  - sentinels defined: none
  - 已登记 errmap: N/A
  - missing: N/A — file defines no sentinels (consumes forgedomain.ErrNotFound which is registered errmap.go:80 ✓)
