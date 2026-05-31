# audit: backend/internal/app/tool/mcp/search.go

LOC: 159
Read: full file (lines 1-159)

## Trace

| site# | file:line | snippet | category | classification | reasoning | severity | user_impact | suggested_fix | status |
|---|---|---|---|---|---|---|---|---|---|
| 1 | search.go:32-35 | `var ErrEmptyQuery = errors.New("query is required and must be non-empty")` | A.5 | OK | validation sentinel consumed by §S18 Tool framework. ValidateInput returns it; framework converts to tool_result string. Never reaches errmap, no registration needed. | N-A | — | — | — |
| 2 | search.go:98-109 | `func ValidateInput(args): if err := json.Unmarshal(args, &a); err != nil { return fmt.Errorf("search_mcp.ValidateInput: %w", err) }; if strings.TrimSpace(a.Query) == "" { return ErrEmptyQuery }` | A.4 | OK | §S16: pkg.method-style prefix (`search_mcp.ValidateInput:`) + %w wrap; sentinel return for empty. Tool prefix `search_mcp` matches Name() string `search_mcp_tools` (close enough to canonical for grep). | N-A | — | — | — |
| 3 | search.go:124-131 | `func Execute(ctx, argsJSON): var args struct{...}; if err := json.Unmarshal([]byte(argsJSON), &args); err != nil { return "", fmt.Errorf("search_mcp.Execute: parse args: %w", err) }` | A.4 | OK | §S16 canonical: prefix + sub-tag `parse args:` + %w. Compliance literal. | N-A | — | — | — |
| 4 | search.go:137-143 | `tools, err := t.svc.Search(ctx, args.Query, topK); if err != nil { return fmt.Sprintf("Search failed: %v. Please ensure an MCP server is connected and a chat model is configured.", err), nil }` | A.1 | OK | §S18 friendly tool_result pattern — Service.Search failure (LLM rerank fail / transient) is converted to text the LLM reads; not a §S3 silent fallback because err is fully surfaced as result-string content. NOTE the result is "teaching-style" (tells LLM to suggest user configure model), but per audit precedent (mcp / web tool result discussion) this is accepted as actionable LLM context, not user-facing copy. | N-A | — | — | — |
| 5 | search.go:145-147 | `if len(tools) == 0 { return "No MCP tools found. Ensure at least one MCP server is configured in ~/.forgify/mcp.json and connected.", nil }` | A.1 | EDGE | Empty result returns instructional text — same teaching-style pattern as #4. **MILD reference to ~/.forgify/mcp.json is impl-detail leak**, but §6 反校验剧场 + §S18 friendly-result lets this through. Borderline. | LOW | LLM may parrot the file path to user; user-friendly when the user is curious where state lives. | could trim to `"No MCP tools found. Ensure at least one MCP server is configured and connected."` (drop file path); WAIVE-acceptable per §S18 precedent | FOUND |
| 6 | search.go:149-152 | `body, err := json.MarshalIndent(tools, "", "  "); if err != nil { return "", fmt.Errorf("search_mcp.Execute: marshal result: %w", err) }` | A.4 | OK | §S16 canonical with sub-tag. MarshalIndent of `[]ToolDef` — basic struct slice — practically unfailable but %w wrap is defensive-correct. | N-A | — | — | — |

## Sub-check

A.1 §S3 错误吞没:
  - violations: not present
  - 1 EDGE noted (#5 mild impl-detail leak in tool-result text — §S18 borderline; audit-recommend WAIVE)

A.2 §S9 detached ctx 终态写:
  - terminal-state writes identified: none
  - 各自 ctx 来源: N/A
  - violations: N/A — Execute delegates to Service.Search which is read-only LLM rerank; no DB writes here. Service-layer §S9 covered in app-mcp audit.

A.3 §S15 ID 生成:
  - violations: N/A — package doesn't generate business IDs

A.4 §S16 错误 wrap 格式:
  - violations: not present (sites #2 / #3 / #6 all pkg.method + %w canonical)

A.5 §S17 sentinel 登记 errmap:
  - sentinels defined: ErrEmptyQuery (line 35)
  - 已登记 errmap: N/A — local validation sentinel consumed by Tool framework, never reaches errmap.FromDomainError
  - missing: N/A — by design (validation errors → tool_result string)
