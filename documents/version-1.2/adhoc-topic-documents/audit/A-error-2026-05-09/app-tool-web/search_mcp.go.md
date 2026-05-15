# audit: backend/internal/app/tool/web/search_mcp.go

LOC: 136
Read: full file (lines 1-136)

## Trace

| site# | file:line | snippet | category | classification | reasoning | severity | user_impact | suggested_fix | status |
|---|---|---|---|---|---|---|---|---|---|
| 1 | search_mcp.go:23 | `var ErrMCPSearchUnavailable = errors.New("mcp search server unavailable")` | A.5 | OK | App-layer sentinel (web tool) for tier-fall-through signaling. Doc comment lines 19-22 documents intent: "router can fall through to the next tier without logging it as a failure". Cross-fork verified `app/mcp/searchrouter.go` site #2 (audit doc 2026-05-09) translates `mcp.GetServer ErrServerNotFound` → this sentinel at the boundary; chain stays clean. NOT in errmap.go because consumed only by `WebSearch.Execute` via `errors.Is`, never bubbled to handler. Per §S17 carve-out: "完全包内 / 跨包但只在 service 层消费" — registration not required. | N-A | — | — | — |
| 2 | search_mcp.go:54-57 | `if t.mcpRouter == nil { return nil, ErrMCPSearchUnavailable }` | A.4 | OK | Direct sentinel return — most-inner layer per §S16. Caller errors.Is at search.go:#6. | N-A | — | — | — |
| 3 | search_mcp.go:58-61 | `raw, err := t.mcpRouter.CallSearchTool(ctx, query, limit); if err != nil { return nil, err }` | A.4 | EDGE | bare-return — MCPSearchRouter.CallSearchTool is documented to return `ErrMCPSearchUnavailable` OR genuine call-failure errors. Bare-return preserves the sentinel for `errors.Is` at search.go:#6. Style inconsistency vs site #4 which wraps. | LOW | identical UX (sentinel preserved); harder to grep | wrap: `return nil, fmt.Errorf("webtool.WebSearch.runMCPSearch: %w", err)` | FOUND |
| 4 | search_mcp.go:62-65 | `results, perr := parseMCPSearchResults(raw); if perr != nil { return nil, fmt.Errorf("mcp: parse: %w", perr) }` | A.4 | EDGE | §S16: prefix `mcp: parse:` — same short-form scheme as search_byok.go's `<provider>: <stage>:`. Sentinel chain preserved. | LOW | same as search_byok | tighten to `webtool.WebSearch.runMCPSearch: parse: %w` | FOUND |
| 5 | search_mcp.go:107-115 | `var keyed struct { Results []item }`; `if err := json.Unmarshal([]byte(raw), &keyed); err == nil && len(keyed.Results) > 0 { ... return out, nil }` | A.1 | OK | §S3 documented multi-shape parser: silent err is **the algorithm** — try shape 1, then shape 2, then plain-text fallback (line 132). The `err == nil` guard is correct dispatch logic. Doc comment 73-76 explicitly: "shape 不匹配但至少抠出 1 条返 nil error；完全 JSON 畸形才报错". | N-A | — | — | — |
| 6 | search_mcp.go:117-125 | bare array shape: `var bare []item`; `if err := json.Unmarshal([]byte(raw), &bare); err == nil && len(bare) > 0 { ... }` | A.1 | OK | same as #5 | N-A | — | — | — |
| 7 | search_mcp.go:127-135 | `if raw != "" { return []searchResult{{Title: "MCP search result", Snippet: raw}}, nil }; return nil, fmt.Errorf("empty MCP response")` | A.4 | EDGE | §S16: `fmt.Errorf("empty MCP response")` no prefix, no sentinel. Reachable when MCP server returned `""` raw — degenerate but possible. Caller search.go:#6 logs Warn + falls through to "no backend" message. | LOW | LLM sees fall-through to no-backend message; debugging requires log inspection | wrap with prefix + `webtool.ErrUpstreamHTTP` (or new `ErrEmptyResponse`): `fmt.Errorf("webtool.WebSearch.parseMCPSearchResults: empty MCP response")` | FOUND |

## Sub-check

A.1 §S3 错误吞没:
  - violations: not present
  - documented multi-shape parser (OK): sites #5, #6 — silent err is the dispatch algorithm

A.2 §S9 detached ctx 终态写:
  - terminal-state writes identified: none
  - 各自 ctx 来源: N/A
  - violations: N/A — file does no DB writes

A.3 §S15 ID 生成:
  - ID generation calls: none
  - violations: N/A — file generates no IDs

A.4 §S16 错误 wrap 格式:
  - violations: site #3 (bare-return inconsistency); site #4 (`mcp: parse:` short prefix); site #7 (`empty MCP response` no prefix no sentinel)

A.5 §S17 sentinel 登记 errmap:
  - sentinels defined in this file: ErrMCPSearchUnavailable (line 23)
  - 已登记 errmap: N/A
  - missing registrations: N/A — sentinel is web-internal, consumed by errors.Is at search.go and translated at the routing boundary. Per §S17 carve-out: not required.
