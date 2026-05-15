# audit: backend/internal/app/tool/mcp/uninstall_server.go

LOC: 89
Read: full file (lines 1-89)

## Trace

| site# | file:line | snippet | category | classification | reasoning | severity | user_impact | suggested_fix | status |
|---|---|---|---|---|---|---|---|---|---|
| 1 | uninstall_server.go:49-60 | `ValidateInput: var a struct{Name string}; if err := json.Unmarshal(args, &a); err != nil { return fmt.Errorf("uninstall_mcp_server: bad args: %w", err) }; if strings.TrimSpace(a.Name) == "" { return errors.New("uninstall_mcp_server: name is required") }` | A.4/A.5 | EDGE | same pattern as install_server.go #2: helper-style prefix + bare errors.New for validation. Tool-framework-consumed; never reaches errmap. | LOW | identical UX. | same as install_server.go #2 (introduce shared sentinel for parity, optional) | FOUND |
| 2 | uninstall_server.go:66-72 | `Execute: var args struct{Name string}; if err := json.Unmarshal([]byte(argsJSON), &args); err != nil { return "", fmt.Errorf("uninstall_mcp_server: %w", err) }` | A.4 | OK | §S16 canonical wrap with helper-style prefix; sentinel chain preserved. | N-A | — | — | — |
| 3 | uninstall_server.go:74-80 | `if err := t.svc.RemoveServer(ctx, args.Name); err != nil { if errors.Is(err, mcpdomain.ErrServerNotFound) { return errorJSON("not_installed", fmt.Sprintf("No installed server named %q. Check the MCP servers UI or ~/.forgify/mcp.json for installed names.", args.Name)), nil }; return "", fmt.Errorf("uninstall_mcp_server: %w", err) }` | A.1/A.4 | EDGE | (a) §S18 friendly path for ErrServerNotFound is correct. (b) friendly text **mentions ~/.forgify/mcp.json** — same impl-detail leak pattern as search.go #5. (c) default case wraps and propagates with %w (canonical). | LOW | (a/c): N/A. (b): LLM may parrot file path to user. | trim file-path mention from friendly text; or WAIVE per §S18 precedent | FOUND |
| 4 | uninstall_server.go:81-87 | `envelope := map[string]any{...}; b, _ := json.Marshal(envelope); return string(b), nil` | A.1 | EDGE | silent Marshal of map[string]any with primitives — same unfailable pattern as install_server.go #6/#7/#8 + list_marketplace.go #3. | LOW | zero. | add inline comment | FOUND |

## Sub-check

A.1 §S3 错误吞没:
  - violations: not present
  - EDGE: 2 sites (#3 impl-detail leak in friendly text — WAIVE-acceptable per §S18 precedent; #4 silent Marshal — add comment)

A.2 §S9 detached ctx 终态写:
  - terminal-state writes identified: none at this layer
  - 各自 ctx 来源: N/A
  - violations: N/A — RemoveServer's terminal writes (mcp.json delete, subprocess Close) are at app/mcp service layer (covered in app-mcp audit, including the orphan-subprocess Close §S3 fix in 26f9c55).

A.3 §S15 ID 生成:
  - violations: N/A — package doesn't generate business IDs

A.4 §S16 错误 wrap 格式:
  - violations: not present (#1 helper-style WAIVE; #2/#3-default canonical)

A.5 §S17 sentinel 登记 errmap:
  - sentinels defined: none in this file
  - mcpdomain sentinels consumed: ErrServerNotFound (errmap.go:131 ✓)
  - missing: none
