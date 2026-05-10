# audit: backend/internal/app/tool/forge/search.go

LOC: 170
Read: full file (lines 1-170)

## Trace

| site# | file:line | snippet | category | classification | reasoning | severity | user_impact | suggested_fix | status |
|---|---|---|---|---|---|---|---|---|---|
| 1 | search.go:34-39 | `Name() string { return "search_forges" }` + Description | N/A | OK | §S18 identity — no error path | N-A | — | — | — |
| 2 | search.go:55-57 | `IsReadOnly()=true / NeedsReadFirst()=false / RequiresWorkspace()=false` | N/A | OK | §S18 metadata — search reads forge library only, accurate | N-A | — | — | — |
| 3 | search.go:62-66 | `ValidateInput(json.RawMessage) error { return nil }` + CheckPermissions=Allow | N/A | OK | no validation needed; all-allow per §S18 example for read-only tool | N-A | — | — | — |
| 4 | search.go:75-77 | `if err := json.Unmarshal([]byte(argsJSON), &args); err != nil { return "", fmt.Errorf("search_forges: bad args: %w", err) }` | A.4 | EDGE | §S16: `search_forges:` prefix is the LLM-facing tool name, not the canonical `<pkg>.<Type>.<Method>:` form. Sentinel chain preserved via %w. Style consistent across all 5 forge tools. | LOW | identical UX | tighten to `forgetool.SearchForge.Execute: bad args: %w` for §S16 spec literal | FOUND |
| 5 | search.go:82-85 | `forges, err := t.svc.ListAll(ctx); if err != nil { return "", fmt.Errorf("search_forges: list: %w", err) }` | A.4 | EDGE | same prefix issue as #4 | LOW | identical UX | same | FOUND |
| 6 | search.go:86-89 | `if len(forges) == 0 { b, _ := json.Marshal([]any{}); return string(b), nil }` | A.1 | OK | empty-list case; json.Marshal of `[]any{}` is unfailable per encoding/json invariant. Discard `_` is safe-by-construction. | N-A | — | — | — |
| 7 | search.go:102-105 | `bc, err := llmclientpkg.Resolve(ctx, t.picker, t.keys, t.factory); if err != nil { return "", fmt.Errorf("search_forges: %w", err) }` | A.4 | EDGE | same prefix issue. POSITIVE: `llmclientpkg.ErrPickModel` / `ErrResolveCreds` sentinels reach errmap; chain intact. | LOW | identical UX | same | FOUND |
| 8 | search.go:106-114 | `resp, err := llminfra.Generate(ctx, bc.Client, ...); if err != nil { return "", fmt.Errorf("search_forges: llm: %w", err) }` | A.4 | EDGE | same prefix issue. POST-COMMIT-363b084: llminfra.Generate errors now carry `llm.ErrAuthFailed` / `ErrRateLimited` etc. sentinels; chain discriminative. | LOW | identical UX | same | FOUND |
| 9 | search.go:120-123 | `jsonStr, ok := llmparsepkg.ExtractJSON(resp); if !ok { return "", fmt.Errorf("search_forges: LLM response contained no JSON: %q", resp) }` | A.4 | EDGE | §S16: NO sentinel + NO %w (no upstream err to wrap; this IS the originating error). Same pattern as openai.go:#9 "no choices" before commit 363b084 which was wrapped with ErrProviderError. Could similarly wrap with llm.ErrProviderError. | LOW | hits "unmapped domain error" alarm if reached; LLM gets error msg with full resp dump (could be very long if LLM responded with prose) | wrap with sentinel: `fmt.Errorf("forgetool.SearchForge.Execute: LLM response contained no JSON: %w", llminfra.ErrProviderError)` + truncate resp | FOUND |
| 10 | search.go:124-126 | `if err = json.Unmarshal([]byte(jsonStr), &ranked); err != nil { return "", fmt.Errorf("search_forges: parse ranking: %w", err) }` | A.4 | EDGE | same prefix issue | LOW | identical UX | same | FOUND |
| 11 | search.go:135-138 | `fetched, err := t.svc.GetForgesByIDs(ctx, ids); if err != nil { return "", fmt.Errorf("search_forges: fetch: %w", err) }` | A.4 | EDGE | same prefix issue | LOW | identical UX | same | FOUND |
| 12 | search.go:159-161 | `var params, ret any; json.Unmarshal([]byte(f.Parameters), &params) //nolint:errcheck; json.Unmarshal([]byte(f.ReturnSchema), &ret) //nolint:errcheck` | **A.1** | EDGE | §S3 silent: json.Unmarshal err discarded (intentionally — comment 156-158 explicitly "DB data corrupted → keep forge with nil schemas rather than aborting search"). Documented intent + `//nolint:errcheck` ritual present. **However**: if DB row corrupted, LLM sees forge with `parameters: null` / `returnSchema: null` — LLM cannot generate a valid run_forge call without the parameters spec. Effectively: corrupted forge becomes invisible to search ranking but visible to user as broken row. **Recommendation**: log Warn so corruption surfaces in operator log; keep current behavior (don't abort search) since that's the documented design. | LOW | LLM may rank corrupted forge highly but then fail when run_forge sees null parameters. Audit-trail concern only — current behavior correct, just silent. | add `s.log.Warn("SearchForge: malformed forge.Parameters in DB", zap.String("forge_id", f.ID), zap.Error(err))` — but SearchForge has no logger field. Same chat/loop refactor pattern (commit 26f9c55) of adding logger field via constructor. | FOUND |
| 13 | search.go:167-168 | `b, _ := json.Marshal(out); return string(b), nil` | A.1 | OK | json.Marshal of `[]result` (basic types: string + nested any from json.Unmarshal which becomes basic types) — unfailable per encoding/json invariant. Ritual `_` discard without inline comment is style-only. | N-A | — | — | — |

## Sub-check

A.1 §S3 错误吞没:
  - violations: not present (silent path at #12 is documented intent + `//nolint:errcheck` ritual; LOW EDGE for missing log)

A.2 §S9 detached ctx 终态写:
  - terminal-state writes identified: none
  - 各自 ctx 来源: N/A
  - violations: N/A — search is read-only (ListAll + GetForgesByIDs). No DB writes.

A.3 §S15 ID 生成:
  - ID generation calls: none in this file
  - violations: N/A

A.4 §S16 错误 wrap 格式:
  - violations: sites #4, #5, #7, #8, #9, #10, #11 (`search_forges:` tool-name prefix instead of canonical `forgetool.<Type>.<Method>:`). Same package-wide pattern; audit-recommended WAIVE per "consistency-over-strict-literal" precedent (other tool packages had similar).
  - site #9 also missing sentinel (NO %w + NO ErrProviderError) — a post-363b084 cleanup target.

A.5 §S17 sentinel 登记 errmap:
  - sentinels defined: none
  - 已登记 errmap: N/A
  - missing: N/A — file defines no sentinels (consumes forgedomain.* registered errmap.go:80-88 ✓)
