# audit: backend/internal/app/skill/search.go

LOC: 164
Read: full file (lines 1-164)

## Trace

| site# | file:line | snippet | category | classification | reasoning | severity | user_impact | suggested_fix | status |
|---|---|---|---|---|---|---|---|---|---|
| 1 | search.go:34-43 | `if topK <= 0 { topK = 3 }; all := s.List(ctx); if len(all) == 0 { return []*skilldomain.Skill{}, nil }; if len(all) <= topK { return all, nil }` | A.1 | OK | empty / small-catalog short-circuits — no LLM call, no rerank cost. Documented design at file header. | N-A | — | — | — |
| 2 | search.go:54-56 | `em := eventlogpkg.From(ctx); progID := em.StartBlock(ctx, eventlogdomain.BlockTypeProgress, ...)` | A.1 | OK | §S18-style progress block emit. Eventlog `From(ctx)` doc says it's no-op when called outside a chat context, so test paths don't crash. | N-A | — | — | — |
| 3 | search.go:58-62 | `bundle, err := llmclientpkg.Resolve(...); if err != nil { em.StopBlock(ctx, progID, eventlogdomain.StatusError, err); return nil, fmt.Errorf("skillapp.Search: resolve LLM: %w", err) }` | A.4 | OK | §S16 canonical wrap. progress block correctly closed with error status before return. | N-A | — | — | — |
| 4 | search.go:63-74 | `resp, err := llminfra.Generate(...); if err != nil { em.StopBlock(ctx, progID, eventlogdomain.StatusError, err); return nil, fmt.Errorf("skillapp.Search: llm: %w", err) }; em.StopBlock(ctx, progID, eventlogdomain.StatusCompleted, nil)` | A.4 | OK | §S16 canonical. Generate error wraps llminfra sentinel (commit 363b084's llm.ErrAuthFailed / etc.) so chain reaches errmap. | N-A | — | — | — |
| 5 | search.go:77-89 | `indices, err := parseRankedIndices(resp, len(all)); if err != nil { s.log.Warn("skill search rank parse failed; falling back to alpha order", ...); return all[:min(topK, len(all))], nil }` | A.1 | OK | §S3 documented soft-fail: parse fail returns alpha-order top K; Warn log includes query + response_snippet + Error so author can debug. Returning fallback is the documented intent (file header lines 79-84) — search must always return *something* usable rather than fail the LLM call. | N-A | — | — | — |
| 6 | search.go:91-100 | `out := make([]*skilldomain.Skill, 0, len(indices)); for _, idx := range indices { if idx < 0 \|\| idx >= len(all) { continue }; out = append(out, all[idx]); ... }` | A.1 | OK | bounds-check filters bad indices silently — but only "bad index from LLM" is filtered; not an error path. | N-A | — | — | — |
| 7 | search.go:101-103 | `if len(out) == 0 { return all[:min(topK, len(all))], nil }` | A.1 | OK | LLM returned all-out-of-bounds indices → fall back to alpha. Same intent as #5. | N-A | — | — | — |
| 8 | search.go:134-137 | `parseRankedIndices: jsonStr, ok := llmparsepkg.ExtractJSON(resp); if !ok { return nil, fmt.Errorf("no JSON in response: %q", trimResp(resp, 200)) }` | A.4 | EDGE | §S16: prefix is `no JSON in response:` — no `skillapp.parseRankedIndices:` qualifier, no sentinel. **Same defect class as forge audit's app-tool-forge/search.go #9 (resolved 64d9535 by wrapping with llminfra.ErrProviderError)**. Caller (Search site #5) only logs at Warn and returns alpha fallback — caller doesn't propagate up to errmap, so unmapped-domain-error alarm doesn't fire. **However** if Search ever propagates the parse error directly (e.g. via different config), this becomes alarm-pollution. | LOW | currently no UX impact (Search swallows + falls back); future change could expose | wrap with llminfra.ErrProviderError sentinel + pkg.Method prefix: `fmt.Errorf("skillapp.parseRankedIndices: %w: no JSON in response: %q", llminfra.ErrProviderError, trimResp(resp, 200))` — mirrors recent commit 64d9535 in app-tool-forge | FOUND |
| 9 | search.go:140-142 | `if err := json.Unmarshal([]byte(jsonStr), &raw); err != nil { return nil, fmt.Errorf("parse JSON: %w", err) }` | A.4 | EDGE | §S16: prefix is `parse JSON:` — no `skillapp.parseRankedIndices:` qualifier. Same caller-soft-fall-back rationale as #8. | LOW | same as #8 | wrap: `fmt.Errorf("skillapp.parseRankedIndices: parse JSON: %w", err)` | FOUND |

## Sub-check

A.1 §S3 错误吞没:
  - violations: not present (sites #5, #6, #7 are documented soft-fails per file header design)

A.2 §S9 detached ctx 终态写:
  - terminal-state writes identified: none — Search returns ranked list, doesn't write
  - 各自 ctx 来源: request ctx for LLM + emit calls (correct — these ARE bounded by user's request lifetime)
  - violations: N/A — file is read-only Search + LLM rerank

A.3 §S15 ID 生成:
  - ID generation calls: none
  - violations: N/A

A.4 §S16 错误 wrap 格式:
  - violations: not present in canonical-spec sense for outer-layer Search; LOW EDGE at sites #8, #9 (helper-style prefix in parseRankedIndices, caller-bounded so doesn't reach errmap)

A.5 §S17 sentinel 登记 errmap:
  - sentinels defined in this file: none
  - 已登记 errmap (consumed via Generate / Resolve wrapped chain): `llminfra.ErrAuthFailed` etc. (commits 363b084 / 94ab56a) all registered ✓
  - missing: N/A — file defines no sentinels (parseRankedIndices's bare fmt.Errorf at #8 #9 would benefit from llminfra.ErrProviderError wrap per consistency with forge audit fix in 64d9535)
