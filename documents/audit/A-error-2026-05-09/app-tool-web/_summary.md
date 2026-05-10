# Package audit summary: internal/app/tool/web

## Spec understanding (Phase A — §S3 / §S9 / §S15 / §S16 / §S17)

- **§S3 错误不吞**: silent fallback / silent error swallow that hides user-visible failure or data loss is forbidden. `_ = err` requires inline justification. Documented soft-fail with audit log via zap is OK. Tool-result-as-LLM-string surface (per §S18) is NOT silent swallow — the LLM-facing error message IS the audit trail.
- **§S9 detached ctx 终态写**: terminal-state writes that must persist regardless of cancel use `reqctxpkg.SetUserID(context.Background(), uid)`. In web tool this applies to `apikey.MarkInvalid` calls from search.go (HTTP 401/403 detection from BYOK providers) — currently implemented correctly per §S9 model pattern.
- **§S15 ID 生成**: package does NOT generate business IDs. No idgenpkg.New calls.
- **§S16 错误 wrap 格式**: `fmt.Errorf("<pkg>.<Method>: %w", err)` canonical. Package consistently uses 2 short-form schemes:
  - Tool-method form (`WebFetch.Execute:`, `WebSearch.ValidateInput:`) — missing `webtool.` pkg qualifier
  - Provider-stage form (`brave: build:`, `mcp: parse:`) — short label, no qualifier
  Both preserve sentinel chain via %w; deviation is from §S16 spec literal only.
- **§S17 errmap 单一事实源**: 4 sentinels declared in this package (ErrEmptyURL / ErrEmptyPrompt / ErrUnsupportedScheme / ErrEmptyQuery / ErrMCPSearchUnavailable — actually 5). All consumed within Tool framework / web internal routing, none reach FromDomainError. Per §S17 carve-out: registration not required for these. **However**: a sentinel-chain GAP exists for HTTP-status detection — search_byok.go produces sentinel-less HTTP-status errors that search.go's MarkInvalid path detects via `strings.Contains(msg, "HTTP 401")`. This is fragile and out of step with the llm.ErrAuthFailed pattern introduced in commit 363b084.

## Files audited

| File | LOC | Sites | OK | POST-FIX | VIOLATION | EDGE |
|---|---|---|---|---|---|---|
| web.go | 79 | 3 | 3 | 0 | 0 | 0 |
| fetch.go | 463 | 21 | 9 | 0 | 0 | 12 |
| search.go | 372 | 15 | 7 | 0 | 1 | 7 |
| search_byok.go | 214 | 15 | 1 | 0 | 0 | 14 |
| search_mcp.go | 136 | 7 | 4 | 0 | 0 | 3 |
| **TOTAL** | **1264** | **61** | **24** | **0** | **1** | **36** |

## Severity breakdown (only VIOLATION + EDGE)

| Severity | Count | Sites | Status |
|---|---|---|---|
| HIGH | 0 | — | — |
| MED | 2 | search.go:#8 (silent swallow of non-ErrNotFoundForProvider errors in tryBYOKProvider — same defect class as B2 bash auto-route silent fallback); search_byok.go:#15 + search.go:#12 (sentinel-less HTTP-status string-match coupling for MarkInvalid trigger — `strings.Contains(msg, "HTTP 401")` is fragile to format change) | FOUND |
| LOW (§S16 prefix style) | ~25 | All `WebFetch.<Method>:` / `WebSearch.<Method>:` / `<provider>: <stage>:` sites — internal style consistent but missing pkg qualifier per spec literal | FOUND |
| LOW (§S3 silent w/o comment) | 4 | search_byok.go:#3, #6, #9 (Marshal of basic-type maps unfailable but missing inline comment); search.go:#9, #10 (defensive fall-throughs without log) | FOUND |
| LOW (other) | 2 | search.go:#3 (errors.New for negative limit, no sentinel); search.go:#7 (tool-result string contains implementation history — separate "教学式 result" anti-pattern phase) | FOUND |

## Cross-cutting

### Sentinel chain integrity (§S17) — most important finding

**Package's own sentinels (5 total) are correctly carved out of errmap registration** per §S17's "完全包内 / 跨包但只在 service 层消费" rule:
- ErrEmptyURL / ErrEmptyPrompt / ErrUnsupportedScheme (fetch.go:79-95) — Tool-framework ValidateInput consumers
- ErrEmptyQuery (search.go:75) — same
- ErrMCPSearchUnavailable (search_mcp.go:23) — web-internal routing fall-through

**Critical gap**: `markInvalidIfAuthErr` in search.go:#12 detects 401/403 via `strings.Contains(msg, "HTTP 401")` matched against the sentinel-less error string produced by `doSearchHTTP` in search_byok.go:#15:

```go
// search_byok.go:201
return nil, fmt.Errorf("%s: HTTP %d: %s", provider, resp.StatusCode, snippet(body, 200))

// search.go:310
if !strings.Contains(msg, "HTTP 401") && !strings.Contains(msg, "HTTP 403") {
    return
}
```

This is **the same defect class as the resolved-via-`%w` issue in mcp install.go:#5 (commit 505d6e3) and the llm sentinel-chain issue resolved in commit 363b084 — but at the BYOK web-search layer, parallel and not yet covered by either prior fix**.

The recently-added `llm.ErrAuthFailed` (commit 363b084) does NOT apply here because BYOK search calls bypass `infra/llm` entirely — they speak directly to Brave / Serper / Tavily / Bocha REST APIs.

**Recommendation**: introduce parallel sentinels in webtool:
- `webtool.ErrAuthFailed` (401/403)
- `webtool.ErrRateLimited` (429)
- `webtool.ErrUpstreamHTTP` (other 4xx/5xx)

Wrap doSearchHTTP HTTP-status errors with `%w`, register all 3 in errmap.go (parallel to llm.ErrAuthFailed lines 178-182 — likely 401/429/502). Update `markInvalidIfAuthErr` to use `errors.Is(err, webtool.ErrAuthFailed)` instead of string match.

### Detached ctx coverage (§S9)

**One terminal-state write identified**: `apikey.MarkInvalid` call from `markInvalidIfAuthErr` (search.go:308-329). **POST-FIX OK**:
- Line 322-325 correctly uses `reqctxpkg.SetUserID(context.Background(), uid)` per §S9 model pattern.
- Caller (apikey.Service.MarkInvalid, fixed in commit 410f664) also implements §S9 detached ctx internally.

The §S9 chain is complete IF the trigger fires — but the trigger gate (`strings.Contains(msg, "HTTP 401")`) is the §S17 fragility (see above). Once §S17 sentinel chain is closed, MarkInvalid becomes reliably reachable from authentic 401/403 responses across all 4 BYOK providers.

### Cross-fork verification (mcp.searchrouter)

Cross-fork verified per directive: app-mcp/searchrouter.go.md site #2 (`ErrSearchServerUnavailable` translation at boundary) — at the **mcp side**, `ErrServerNotFound` is wrapped into `ErrSearchServerUnavailable` and returned via the MCPSearchRouter port. At the **web side**, search_mcp.go:#1-#3 receives this exactly: declares `ErrMCPSearchUnavailable` separately, MCPSearchRouter interface returns it, search.go:#6 errors.Is matches it. **Translation chain is clean and bidirectional** — neither side imports the other's domain. Audit at mcp side flagged a "future-proofing concern" (site #2 in mcp/searchrouter.go.md) that the original err is dropped during the translate; that concern is mcp-internal and doesn't affect web-side correctness.

### WebFetch parallel concern (apikey.MarkInvalid in summary path)

Sub-check finding (fetch.go:#21): `WebFetch.summarise` calls `llminfra.Generate` which (post-commit 363b084) wraps HTTP 401 with `llm.ErrAuthFailed`. **WebFetch does NOT call apikey.MarkInvalid** on these auth failures. Stale web_summary BYOK keys (or chat scenario keys when web_summary unconfigured) silently keep failing without UI badge flip. This is a parallel gap to the search-side fix; could be resolved by adding `markInvalidIfAuthErr`-equivalent to WebFetch.summarise's err path, gated on `errors.Is(err, llminfra.ErrAuthFailed)`.

## Spot-check (random clean sites — verify mechanism not rubber-stamping)

Random seed: 6 sites picked from `OK` set across 4 files:

1. **fetch.go:#7** (Invalid URL handling): verified — converts url.Parse err to `fmt.Sprintf("Invalid URL %q: %v", ...)` returned with nil Go err. This is the §S18 Tool contract: tool-result IS the user-facing message, Go error reserved for framework bugs only. Compliance literal per spec. Not a §S3 silent (LLM sees the error context verbatim).
2. **fetch.go:#12** (fetchContent two-tier with ctx-cancel guard): verified — explicit `errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)` check before falling through to fetchDirect. This is the **right way to do soft-fallback with §S3 escape hatch** — distinguishes "Jina down, retry" from "user cancelled, abort". Comment lines 274-277 explicitly cite the design.
3. **search.go:#5** (BYOK iteration with ctx-cancel guard): verified — `for ... { if ctx.Err() != nil { break }; ... }`. Symmetric ctx-cancel guard inside the iteration loop. Documented soft-fail per §S3 carve-out.
4. **search.go:#11** (provider call failure path): verified — `t.warnf(...)` (zap audit log) + `t.markInvalidIfAuthErr(...)` (apikey lifecycle) + `return nil, "", false` (fall through). Three-prong handling: log + side effect + dispatch decision. **Correctly NOT silent** — has audit trail.
5. **search.go:#12 §S9 part** (markInvalidIfAuthErr ctx detach): verified — exact `reqctxpkg.SetUserID(context.Background(), uid)` form. Doc comment 316-320 cites the rationale ("MarkInvalid expects ctx with userID; ... detached context retains the user ID so background invocations work too"). Mirrors the apikey.Service.MarkInvalid §S9 model fix from commit ff8fd77.
6. **search_mcp.go:#1** (ErrMCPSearchUnavailable sentinel design): verified — declared at app/web layer (not domain layer) because consumed only by web-internal routing. Doc comment 18-22 explicitly justifies the sentinel's purpose ("router can fall through to the next tier without logging it as a failure"). Cross-fork chain verified via mcp/searchrouter.go.md.

All 6 spot-checks confirmed correct classification — mechanism functioning, not rubber-stamping. The audit's primary findings (search.go:#8 silent swallow, search_byok.go:#15 + search.go:#12 sentinel-chain gap) survive spot-check pressure: the OK sites #11 (correct fall-through with audit log), #12§S9 (correct detached ctx), and #5 (correct ctx-cancel guard) prove the package generally implements §S3/§S9 correctly. The MED findings are real fragilities, not noise.

## Recommended fix priorities

1. **search.go:#8** (MED §S3 — silent swallow of non-ErrNotFoundForProvider errors) — narrow the silent path: `if errors.Is(err, apikeydomain.ErrNotFoundForProvider) { return nil, "", false }; if err != nil { t.warnf(...); return nil, "", false }`. Closes the B2-class silent fallback at decryption / store / ctx error paths. **HIGH PRIORITY**.

2. **search_byok.go:#15 + search.go:#12** (MED §S17 — sentinel-less HTTP-status string match) — coordinated 3-step fix:
   - Add `webtool.ErrAuthFailed` / `ErrRateLimited` / `ErrUpstreamHTTP` sentinels in web package
   - Update doSearchHTTP to wrap by status code
   - Register in errmap.go (401/429/502 — parallel to llm.ErrAuthFailed lines 178-182)
   - Replace `strings.Contains` in `markInvalidIfAuthErr` with `errors.Is`

3. **§S16 prefix migration** (LOW × ~25 sites) — single sweep commit replacing `WebFetch.<Method>:` → `webtool.WebFetch.<Method>:` and `<provider>: <stage>:` → `webtool.WebSearch.search<Provider>: <stage>:`. Pure style polish; no behavior change. Same scheme as commit 363b084's llm/openai → llm.openai migration.

4. **Marshal silent comments** (LOW × 3 — search_byok.go:#3, #6, #9) — add inline `// _ = err — Marshal of basic-type {string,int} map is unfailable per encoding/json invariant`. Mirrors loop stream/tools.go fixes in commit 363b084.

5. **search.go:#7 tool-result implementation history leak** — out-of-scope for §S3 phase; flagged for separate "教学式 result" / tool-result anti-pattern phase.

6. **search.go:#9, #10 (defensive fall-throughs)** — add t.warnf for audit trail; LOW value but cheap to fix.

7. **fetch.go:#21 cross-fork concern** — consider mirroring search.go's markInvalidIfAuthErr in WebFetch.summarise so web_summary BYOK keys auto-flip on 401/403. Should land after the webtool sentinel introduction in fix #2.

## Out-of-scope notes (parent should verify)

1. **search_byok.go test coverage of HTTP-status format**: search_test.go was not read (excluded by directive) — should verify whether any test pins the current `"%s: HTTP %d: %s"` format string. If yes, fix #2 above needs coordinated test update.
2. **Bing CN scrape removal history (search.go:#7)**: implementation-history pollution in tool-result is real (LLM will repeat to user). Worth a separate "tool-result-as-copy" cleanup phase across all LLM-facing tool result strings.
3. **WebFetch missing MarkInvalid call (fetch.go:#21)**: parallel to the search-side gap — if fix #2 lands, fetch.go:#21 should similarly invalidate stale web_summary keys on auth failure. Not strictly in scope of this audit (no current violation, just a missing feature).
