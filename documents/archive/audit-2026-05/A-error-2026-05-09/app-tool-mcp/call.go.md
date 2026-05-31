# audit: backend/internal/app/tool/mcp/call.go

LOC: 173
Read: full file (lines 1-173)

## Trace

| site# | file:line | snippet | category | classification | reasoning | severity | user_impact | suggested_fix | status |
|---|---|---|---|---|---|---|---|---|---|
| 1 | call.go:28-31 | `var ( ErrEmptyServer = errors.New("server is required and must be non-empty"); ErrEmptyTool = errors.New("tool is required and must be non-empty") )` | A.5 | OK | local validation sentinels — same pattern as search.go #1; consumed by §S18 Tool framework. | N-A | — | — | — |
| 2 | call.go:97-112 | `ValidateInput: if err := json.Unmarshal(args, &a); err != nil { return fmt.Errorf("call_mcp.ValidateInput: %w", err) }; if strings.TrimSpace(a.Server) == "" { return ErrEmptyServer }; if strings.TrimSpace(a.Tool) == "" { return ErrEmptyTool }` | A.4 | OK | §S16 canonical + sentinel returns. Compliance literal. | N-A | — | — | — |
| 3 | call.go:128-136 | `Execute: var args struct{...}; if err := json.Unmarshal([]byte(argsJSON), &args); err != nil { return "", fmt.Errorf("call_mcp.Execute: parse args: %w", err) }` | A.4 | OK | §S16 canonical with sub-tag. | N-A | — | — | — |
| 4 | call.go:138-141 | `out, err := t.svc.CallTool(ctx, args.Server, args.Tool, args.Args); if err != nil { return mapCallToolErrorToFriendly(args.Server, args.Tool, err), nil }` | A.1 | OK | §S18 friendly tool_result pattern — sentinel-classified error → human-readable text. **Errors are NOT swallowed** (mapCallToolErrorToFriendly walks errors.Is for each sentinel) — this is the canonical §S18 implementation. Other audits should reference this pattern. | N-A | — | — | — |
| 5 | call.go:153-167 | `func mapCallToolErrorToFriendly: switch { case errors.Is(err, ErrServerNotFound): ...; case errors.Is(err, ErrServerNotConnected): ...; ... case errors.Is(err, ErrToolCallFailed): return fmt.Sprintf("MCP call %s/%s failed: %v", server, tool, err); default: return fmt.Sprintf("call_mcp %s/%s failed: %v", server, tool, err) }` | A.1 | OK | All five sentinel branches use errors.Is (correct unwrap chain). Default case `%v` is safe — output is friendly tool_result text, not propagated error chain (§S16 doesn't apply to friendly text). User-impact text references `mcp.json` and "Reconnect button in MCP settings panel" (impl-detail leak; same EDGE precedent as search #5 — accept per §S18). | N-A | — | — | — |

## Sub-check

A.1 §S3 错误吞没:
  - violations: not present (all error paths surfaced as friendly tool_result text or wrapped)

A.2 §S9 detached ctx 终态写:
  - terminal-state writes identified: none
  - 各自 ctx 来源: N/A
  - violations: N/A — Execute delegates to Service.CallTool; the underlying MCP subprocess call is via stdio, not DB. No terminal writes at this layer.

A.3 §S15 ID 生成:
  - violations: N/A — package doesn't generate business IDs

A.4 §S16 错误 wrap 格式:
  - violations: not present (#2 / #3 canonical; #5 friendly text not subject to §S16)

A.5 §S17 sentinel 登记 errmap:
  - sentinels defined: ErrEmptyServer, ErrEmptyTool (line 28-31)
  - 已登记 errmap: N/A — validation sentinels consumed by Tool framework
  - missing: N/A — by design
  - mcpdomain sentinels consumed (errors.Is) via mapCallToolErrorToFriendly: ErrServerNotFound, ErrServerNotConnected, ErrToolNotFound, ErrToolCallTimeout, ErrToolCallFailed — all 5 in errmap.go:131-135 ✓ (also reachable via the friendly map; no errmap fallthrough concern because Execute returns text not error)
