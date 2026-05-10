# audit: backend/internal/app/tool/web/fetch.go

LOC: 463
Read: full file (lines 1-463)

## Trace

| site# | file:line | snippet | category | classification | reasoning | severity | user_impact | suggested_fix | status |
|---|---|---|---|---|---|---|---|---|---|
| 1 | fetch.go:79-95 | `var ( ErrEmptyURL = errors.New(...); ErrEmptyPrompt = ...; ErrUnsupportedScheme = ... )` | A.5 | OK | 3 ValidateInput sentinels declared. Consumed by §S18 Tool framework (toolapp.Tool.ValidateInput → friendly tool_result string), do **not** propagate to handler/errmap path. Per §S17 spec: "完全包内 / 跨包但只在 service 层消费" — registration not required. Verified errmap.go has no entries for these and shouldn't. | N-A | — | — | — |
| 2 | fetch.go:158-160 | `if err := json.Unmarshal(args, &a); err != nil { return fmt.Errorf("WebFetch.ValidateInput: %w", err) }` | A.4 | EDGE | §S16: prefix is `WebFetch.ValidateInput:` not canonical `webtool.WebFetch.ValidateInput:` (missing pkg qualifier). Sentinel chain preserved via %w. Same scheme used consistently across the package's 3 fmt.Errorf sites (#2/#3/#4) — internal consistency holds, just deviates from §S16 literal. | LOW | identical UX (Tool framework converts to tool_result string regardless); harder to grep call sites | tighten to `webtool.WebFetch.ValidateInput:` for spec literal | FOUND |
| 3 | fetch.go:167-170 | `u, err := url.Parse(a.URL); if err != nil { return fmt.Errorf("WebFetch.ValidateInput: %w", err) }` | A.4 | EDGE | same prefix issue as site #2 | LOW | same as #2 | same as #2 | FOUND |
| 4 | fetch.go:171-173 | `if u.Scheme != "http" && u.Scheme != "https" { return ErrUnsupportedScheme }` | A.4 | OK | direct sentinel return — most-inner layer per §S16 ("sentinel 在最里层") | N-A | — | — | — |
| 5 | fetch.go:177-179 | `func (t *WebFetch) CheckPermissions(...) toolapp.PermissionResult { return toolapp.PermissionAllow }` | A.1 | OK | Pure return, no error path | N-A | — | — | — |
| 6 | fetch.go:195-197 | `if err := json.Unmarshal([]byte(argsJSON), &args); err != nil { return "", fmt.Errorf("WebFetch.Execute: %w", err) }` | A.4 | EDGE | same §S16 prefix issue | LOW | same | same | FOUND |
| 7 | fetch.go:199-202 | `parsed, err := url.Parse(args.URL); if err != nil { return fmt.Sprintf("Invalid URL %q: %v", args.URL, err), nil }` | A.1/A.4 | OK | Per §S18 Tool contract: errors converted to friendly LLM-facing strings; Go err return reserved for framework bugs only. `nil` Go err here = "tool consumed the failure". §S3 not applicable (LLM sees rich message). The `%v` of a parse error here is for human-readable message, NOT errors.Is chain (since we return string anyway). | N-A | — | — | — |
| 8 | fetch.go:203-205 | `if reason := guardHostname(parsed.Hostname()); reason != "" { return reason, nil }` | A.1 | OK | SSRF check returns string explanation — same Tool-result pattern as site #7 | N-A | — | — | — |
| 9 | fetch.go:207-210 | `content, err := fetchContent(ctx, args.URL); if err != nil { return fmt.Sprintf("Failed to fetch %s: %v", args.URL, err), nil }` | A.1/A.4 | OK | Same Tool-result pattern. Note err originates from fetchContent's two-tier fallback; final err here is whichever tier last failed. Documented "LLM-friendly" surface. | N-A | — | — | — |
| 10 | fetch.go:215-222 | `summary, err := t.summarise(...); if err != nil { return fmt.Sprintf("Summarisation failed (%v). Raw content...: %s", err, truncate(content, 4096)), nil }` | A.1/A.4 | OK | LLM-side failure → error info + truncated raw content surfaced for LLM to reason over. Best practice for tool-as-LLM-facing failure: never surrender silently, always provide context. | N-A | — | — | — |
| 11 | fetch.go:257-264 | `func ssrfCheckRedirect(req, via) error { if len(via) >= 10 { return errors.New("stopped after 10 redirects") }; if reason := guardHostname(req.URL.Hostname()); reason != "" { return fmt.Errorf("redirect blocked: %s", reason) }; return nil }` | A.4 | EDGE | §S16: errors.New + fmt.Errorf both lack pkg.method prefix and are sentinel-less. **However**: these are returned to the http.Client's redirect machinery, not propagated up the application error chain. The http.Client wraps them in *url.Error which Execute (#9) converts to user-string. Sentinel chain not relevant here. | LOW | error message reads as "Get \"...\": stopped after 10 redirects" (Go std url.Error wraps); fine for LLM | optional: introduce `ErrTooManyRedirects` / `ErrRedirectBlocked` sentinels if any consumer needs to discriminate; otherwise keep as-is | FOUND |
| 12 | fetch.go:271-280 | `func fetchContent(ctx, target) (string, error) { if body, err := fetchViaJina(ctx, target); err == nil { return body, nil } else if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) { return "", err }; return fetchDirect(ctx, target) }` | A.1 | OK | Two-tier fallback with **explicit ctx-cancel guard** before falling through. §S3 carve-out: documented soft-fail with canonical context-error escape hatch. Comment lines 274-277 cite the design choice. NOT a silent fallback like B2 bash auto-route — the ctx-canceled path bubbles out cleanly. | N-A | — | — | — |
| 13 | fetch.go:289-291 | `req, err := http.NewRequestWithContext(...); if err != nil { return "", err }` (in fetchViaJina) | A.4 | EDGE | bare-return — no `webtool.fetchViaJina:` prefix. http.NewRequestWithContext err is rare (URL parse fail) and is converted to user-string at Execute. | LOW | identical UX; harder to grep | optional wrap | FOUND |
| 14 | fetch.go:307-310 | bare-return same pattern in fetchDirect | A.4 | EDGE | same as #13 | LOW | same | same | FOUND |
| 15 | fetch.go:322-325 | `resp, err := fetchClient.Do(req); if err != nil { return "", err }` (in doRequest) | A.4 | EDGE | bare-return — http transport err wraps go std *url.Error. Same as #13/#14. | LOW | same | same | FOUND |
| 16 | fetch.go:326 | `defer resp.Body.Close()` | A.1 | OK | §S3 spec example carve-out: "defer f.Close() 在只读路径（Close 返错对调用方无意义）" — read-only HTTP response body | N-A | — | — | — |
| 17 | fetch.go:327-329 | `if resp.StatusCode < 200 \|\| resp.StatusCode >= 300 { return "", fmt.Errorf("http status %d", resp.StatusCode) }` | A.4 | EDGE | §S16: sentinel-less + no pkg.method prefix. **Reachable via fetchContent → Execute → user string.** Could wrap with `llminfra.ErrProviderError` (commit 363b084 sentinel) if we want to errors.Is HTTP failures across infra/llm + webtool — but webtool fetch is not LLM-call (it's content retrieval), so semantically doesn't fit llm sentinels. New `webtool.ErrUpstreamHTTP` would be cleanest but value low (no current consumer uses errors.Is). | LOW | LLM gets "Failed to fetch <url>: http status 503" which is debuggable | optional: introduce `webtool.ErrUpstreamHTTP` sentinel; or accept current pattern | FOUND |
| 18 | fetch.go:330-333 | `body, err := io.ReadAll(io.LimitReader(...)); if err != nil { return "", err }` | A.4 | EDGE | bare-return — io read err propagates as user-string at Execute | LOW | same as #13 | same | FOUND |
| 19 | fetch.go:367-372 | `ips, err := net.LookupIP(host); if err != nil { return fmt.Sprintf("Cannot resolve host %s: %v", host, err) }` | A.1 | OK | Returns rejection-message string (not error) since this is the SSRF guard helper. Consumed by Execute as a "failure reason" string. Same Tool-friendly pattern as #7-#10. | N-A | — | — | — |
| 20 | fetch.go:413-416 | `bundle, err := llmclientpkg.ResolveForWebSummary(...); if err != nil { return "", err }` (in summarise) | A.4 | EDGE | bare-return — llmclient err already wrapped. Sentinel chain preserved (e.g. `llmclientpkg.ErrPickModel` / `ErrResolveCreds` registered at chat handler level). Surfaced to Execute #10 as user-string. | LOW | identical UX | optional wrap | FOUND |
| 21 | fetch.go:418-426 | `out, err := llminfra.Generate(...); if err != nil { return "", err }` | A.4 | EDGE | bare-return — `llminfra.Generate` err may now wrap `llm.ErrAuthFailed` / `ErrRateLimited` sentinels (commit 94ab56a / 363b084). Sentinel chain preserved through bare return. **However**: WebFetch does NOT call apikey.MarkInvalid on auth failures here — different from search.go which does. **Cross-fork concern**: should WebFetch also call MarkInvalid when 401/403 arrives? The user's web_summary scenario API key would be invalidated and they'd see badge flip. | LOW (style) + possible MED missing-feature | LOW: identical UX. MED: stale web_summary keys silently keep failing without UI flip. | (a) wrap with `webtool.WebFetch.summarise:`; (b) consider mirroring search.go's markInvalidIfAuthErr in summarise — out-of-scope for this audit but worth flagging | FOUND |

## Sub-check

A.1 §S3 错误吞没:
  - violations: not present (all error paths surface to LLM as friendly strings per §S18 Tool contract; no silent swallowing)

A.2 §S9 detached ctx 终态写:
  - terminal-state writes identified: none in this file
  - 各自 ctx 来源: N/A
  - violations: N/A — fetch.go performs no DB writes; LLM summary generation is read-path

A.3 §S15 ID 生成:
  - ID generation calls: none
  - violations: N/A — file does not generate business IDs

A.4 §S16 错误 wrap 格式:
  - violations: sites #2, #3, #6 (`WebFetch.ValidateInput:` / `WebFetch.Execute:` — missing pkg qualifier per spec literal); sites #11, #13, #14, #15, #17, #18, #20 (bare-return / no-prefix in helper functions). All preserve sentinel chains; LOW per spec consistency.

A.5 §S17 sentinel 登记 errmap:
  - sentinels defined: ErrEmptyURL, ErrEmptyPrompt, ErrUnsupportedScheme (lines 79-95)
  - 已登记 errmap: N/A — these are Tool-framework-internal (consumed by ValidateInput hook, never reach FromDomainError); registration would be a no-op at best.
  - missing: N/A per §S17 "完全包内 / 跨包但只在 service 层消费" carve-out
