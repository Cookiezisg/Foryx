# audit: backend/internal/app/tool/web/search_byok.go

LOC: 214
Read: full file (lines 1-214)

## Trace

| site# | file:line | snippet | category | classification | reasoning | severity | user_impact | suggested_fix | status |
|---|---|---|---|---|---|---|---|---|---|
| 1 | search_byok.go:32-34 | `req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil); if err != nil { return nil, fmt.Errorf("brave: build: %w", err) }` | A.4 | EDGE | §S16: prefix `brave: build:` — short provider+stage form; lacks `webtool.WebSearch.searchBrave:` pkg.method qualifier. Internal style consistent across all 4 BYOK clients (#1/#5/#7/#9 in this file follow same scheme). Sentinel chain preserved via %w. | LOW | identical UX (caller search.go:#11 logs Warn + falls through); harder to grep | tighten to `webtool.WebSearch.searchBrave: build: %w` — but consistent style argues for keeping current form | FOUND |
| 2 | search_byok.go:51-53 | `if err := json.Unmarshal(body, &resp); err != nil { return nil, fmt.Errorf("brave: parse: %w", err) }` | A.4 | EDGE | same prefix issue | LOW | same | same | FOUND |
| 3 | search_byok.go:67-72 | `payload, _ := json.Marshal(map[string]any{"q": query, "num": limit}); req, err := http.NewRequestWithContext(...); if err != nil { return nil, fmt.Errorf("serper: build: %w", err) }` | A.1/A.4 | EDGE | (a) §S3: `payload, _ := json.Marshal(...)` — silent err discard. Marshal of `map[string]any{string, int}` is unfailable per encoding/json invariant — same as inline-comment treatment in commit 363b084 (loop stream/tools.go) but **missing the inline comment here**. (b) §S16: `serper: build:` — same prefix issue. | LOW × 2 | (a) zero — Marshal can't fail in practice. (b) identical UX. | (a) add inline comment: `// _ = err — Marshal of basic-type {string,int} map is unfailable per encoding/json invariant`; (b) optional pkg qualifier | FOUND |
| 4 | search_byok.go:75-78 | `body, err := t.doSearchHTTP(req, "serper"); if err != nil { return nil, err }` | A.4 | EDGE | bare-return — doSearchHTTP wraps with provider name; preserved here. Style inconsistency vs site #1/#2 which wrap explicitly. | LOW | same | same | FOUND |
| 5 | search_byok.go:86-88 | serper json.Unmarshal err: `fmt.Errorf("serper: parse: %w", err)` | A.4 | EDGE | same prefix issue | LOW | same | same | FOUND |
| 6 | search_byok.go:103-110 | tavily Marshal silent + build err | A.1/A.4 | EDGE | same as #3 | LOW × 2 | same | same | FOUND |
| 7 | search_byok.go:113-116 | tavily doSearchHTTP bare-return | A.4 | EDGE | same as #4 | LOW | same | same | FOUND |
| 8 | search_byok.go:124-126 | tavily parse err | A.4 | EDGE | same prefix issue | LOW | same | same | FOUND |
| 9 | search_byok.go:141-145 | bocha Marshal silent + build err | A.1/A.4 | EDGE | same as #3 | LOW × 2 | same | same | FOUND |
| 10 | search_byok.go:148-151 | bocha doSearchHTTP bare-return | A.4 | EDGE | same as #4 | LOW | same | same | FOUND |
| 11 | search_byok.go:163-165 | bocha parse err | A.4 | EDGE | same prefix issue | LOW | same | same | FOUND |
| 12 | search_byok.go:179-183 | `resp, err := t.httpClient.Do(req); if err != nil { return nil, fmt.Errorf("%s: connection: %w", provider, err) }` (in doSearchHTTP) | A.4 | EDGE | §S16: prefix is `%s: connection:` substituting provider name, no `webtool.<Method>:` qualifier. Consistent internal style across this file's 4 backends. | LOW | same | same | FOUND |
| 13 | search_byok.go:184 | `defer resp.Body.Close()` | A.1 | OK | §S3 carve-out for read-only HTTP body close | N-A | — | — | — |
| 14 | search_byok.go:185-199 | bounded body accumulator: `for { n, rerr := resp.Body.Read(buf); ...; if rerr != nil { break } }` | A.1 | EDGE | §S3: `rerr` (read error) only used as loop-exit signal — non-EOF errors silently truncate body. **However**: io.Reader contract permits this; bufio/io.ReadAll has same behavior. Documented intent: "bounded by 256 KB". Caller proceeds with partial body if any. **Concern**: a network-fault mid-read produces a partial JSON that then fails to parse with site #2 / #5 / #8 / #11 cycle → user sees "parse" error, not the underlying read fault. | LOW | rare (network mid-read fault); user sees parse error instead of read error; debugging harder | check `errors.Is(rerr, io.EOF)` and surface non-EOF as `fmt.Errorf("%s: read: %w", provider, rerr)`; or accept since LLM gets fall-through | FOUND |
| 15 | search_byok.go:200-202 | `if resp.StatusCode/100 != 2 { return nil, fmt.Errorf("%s: HTTP %d: %s", provider, resp.StatusCode, snippet(body, 200)) }` | **A.4/A.5** | **EDGE** | **§S17 KEY CONCERN**: this is **the line that produces the "HTTP 401" / "HTTP 403" string** that search.go:#12's `markInvalidIfAuthErr` greps via `strings.Contains`. Sentinel-less. Format change here would silently break apikey.MarkInvalid auto-flip. **Should** introduce `webtool.ErrAuthFailed` (401/403), `webtool.ErrRateLimited` (429), `webtool.ErrUpstreamHTTP` (other) parallel to llm.ErrAuthFailed (commit 363b084). Wrap with appropriate sentinel based on resp.StatusCode. | MED | hidden coupling: search.go's markInvalidIfAuthErr depends on the `"HTTP 401"` / `"HTTP 403"` substring produced HERE. Refactoring this format string silently breaks the apikey.MarkInvalid trigger chain. Tests on the format string don't exist (search_test.go would need an inspection). | introduce sentinels per §S17 spec; wrap by status: `case 401, 403: return nil, fmt.Errorf("%s: HTTP %d: %w: %s", provider, resp.StatusCode, webtool.ErrAuthFailed, snippet(body, 200))`; etc. Update search.go:#12 to `errors.Is(err, webtool.ErrAuthFailed)`. | FOUND |

## Sub-check

A.1 §S3 错误吞没:
  - violations: site #14 (LOW — bounded read silently treats non-EOF as truncation point)
  - silent Marshal err discards (LOW × 3): sites #3, #6, #9 — unfailable but missing inline comment (same pattern as 363b084 commit fixed)
  - documented OK: sites #13 (defer Close)

A.2 §S9 detached ctx 终态写:
  - terminal-state writes identified: none in this file
  - 各自 ctx 来源: N/A
  - violations: N/A — file performs no DB writes (HTTP-out only)

A.3 §S15 ID 生成:
  - ID generation calls: none
  - violations: N/A — file does not generate business IDs

A.4 §S16 错误 wrap 格式:
  - violations: 11 sites (#1, #2, #4, #5, #7, #8, #10, #11, #12, #15) — all use `<provider>: <stage>:` short prefix instead of `webtool.WebSearch.<Method>: <stage>:` canonical. Internal style is consistent across all 4 BYOK backends. Sentinel chain preserved via %w everywhere.
  - bare-returns inconsistent with sibling wraps: sites #4, #7, #10

A.5 §S17 sentinel 登记 errmap:
  - sentinels defined in this file: none
  - 已登记 errmap: N/A
  - missing: **MED — site #15 produces sentinel-less HTTP-status error** consumed by string-match in search.go:#12. Should introduce `webtool.ErrAuthFailed` (401/403) + `webtool.ErrRateLimited` (429) + `webtool.ErrUpstreamHTTP` (5xx) sentinels and wrap with %w. Register in errmap.go (likely 401/429/502 statuses) parallel to llm.ErrAuthFailed (errmap.go:178-182). Closes the §S17 sentinel chain that lets `errors.Is(err, webtool.ErrAuthFailed)` replace `strings.Contains(msg, "HTTP 401")`.
