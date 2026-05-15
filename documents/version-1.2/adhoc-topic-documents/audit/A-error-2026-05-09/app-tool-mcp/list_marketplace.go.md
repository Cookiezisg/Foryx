# audit: backend/internal/app/tool/mcp/list_marketplace.go

LOC: 122
Read: full file (lines 1-122)

## Trace

| site# | file:line | snippet | category | classification | reasoning | severity | user_impact | suggested_fix | status |
|---|---|---|---|---|---|---|---|---|---|
| 1 | list_marketplace.go:62 | `func (t *ListMCPMarketplace) ValidateInput(json.RawMessage) error { return nil }` | A.1 | OK | empty schema (`properties: {}`); ValidateInput correctly no-ops. | N-A | — | — | — |
| 2 | list_marketplace.go:70-83 | `Execute: all, err := t.svc.ListRegistry(ctx); if err != nil { if errors.Is(err, mcpdomain.ErrMarketplaceUnavailable) { return fmt.Sprintf("Marketplace unavailable: %v. The user can configure a search-category API key (Brave / Serper / Tavily / Bocha) for web search instead, or retry later.", err), nil }; return "", fmt.Errorf("list_mcp_marketplace: %w", err) }` | A.1/A.4 | EDGE | (a) ErrMarketplaceUnavailable → §S18 friendly tool_result with workaround suggestion (BYOK key). **Self-promoting/teaching pattern** by audit's anti-pattern checklist — but per audit precedent (apple-mcp design discussion) accepted as actionable LLM context. (b) prefix `list_mcp_marketplace:` — Tool Name() form, not canonical `mcptool.ListMCPMarketplace.Execute:`. Helper-style consistent with package precedent (audit-recommended WAIVE per "consistency-over-strict-literal"). | LOW | (a) LLM may parrot BYOK suggestion to user; user-friendly when desired. (b) grep traceability slightly weaker. | (a) WAIVE per §S18 precedent; (b) WAIVE per package style precedent | FOUND |
| 3 | list_marketplace.go:119 | `b, _ := json.Marshal(out)` | A.1 | EDGE | §S3: silent json.Marshal err discard. `out` is `[]result` where result is concrete struct of strings/ints/known struct slices — Marshal can't fail at runtime per encoding/json invariant. Same pattern as loop tools.go #3 fixed in 505d6e3. Missing inline `// _ = err — basic-types unfailable` ritual. | LOW | zero impact — Marshal of these types is a Go invariant. Audit-trail concern only. | add inline comment: `b, _ := json.Marshal(out) // _ = err — Marshal of struct/string/int slices cannot fail` | FOUND |

## Sub-check

A.1 §S3 错误吞没:
  - violations: not present
  - 2 EDGE noted: site #2 (friendly-text BYOK suggestion — audit-recommend WAIVE per §S18) and site #3 (silent Marshal — missing inline comment, recommend add)

A.2 §S9 detached ctx 终态写:
  - terminal-state writes identified: none
  - 各自 ctx 来源: N/A
  - violations: N/A — Execute is read-only catalog enumeration

A.3 §S15 ID 生成:
  - violations: N/A — package doesn't generate business IDs

A.4 §S16 错误 wrap 格式:
  - violations: not present (#2(b) is style EDGE — helper-style prefix consistent with package; WAIVE-acceptable per established precedent)

A.5 §S17 sentinel 登记 errmap:
  - sentinels defined: none in this file
  - mcpdomain sentinels consumed via errors.Is: ErrMarketplaceUnavailable (errmap.go:143 ✓)
  - missing: none
