# audit: backend/internal/app/apikey/tester.go

LOC: 412
Read: full file (lines 1-412)

**Convention note (line 1-8 of file)**: This file's design contract is "outcomes (401/5xx/net-fail/ctx-cancel) surface in TestResult; `error` reserved for programmer bugs (unknown provider, missing baseURL)." So every probe-helper returns `*TestResult` (no error). Network/HTTP failures become `TestResult{OK:false, Message:"..."}` — by design, not §S3 violation.

## Trace

| site# | file:line | snippet | category | classification | reasoning | severity | user_impact | suggested_fix | status |
|---|---|---|---|---|---|---|---|---|---|
| 1 | tester.go:59-61 | `if client == nil { client = &http.Client{Timeout: 10 * time.Second} }` | A.1 | OK | nil-default; not error | N-A | — | — | — |
| 2 | tester.go:74-77 | `meta, ok := GetProviderMeta(provider); if !ok { return nil, fmt.Errorf("apikeytester: unknown provider %q: %w", provider, apikeydomain.ErrInvalidProvider) }` | A.4 | EDGE | §S16: prefix is `apikeytester:` not canonical `apikey.HTTPTester.Test:` form. Has %w + sentinel preserved. The "apikeytester:" custom prefix is descriptive but deviates from spec literal `<pkg>.<Method>:` | LOW | identical UX (errmap matches sentinel ErrInvalidProvider→400 INVALID_PROVIDER); harder to grep by call-site | switch to `apikey.HTTPTester.Test: ...` or accept descriptive prefix as project convention | **FIXED 2026-05-09 1b96a5e** |
| 3 | tester.go:79-84 | `if effective == "" && meta.TestMethod != TestMethodAlwaysOK { return nil, fmt.Errorf("apikeytester: baseURL required for provider %q: %w", provider, apikeydomain.ErrBaseURLRequired) }` | A.4 | EDGE | same `apikeytester:` prefix issue as site #2 | LOW | identical UX | same as site #2 | **FIXED 2026-05-09 1b96a5e** |
| 4 | tester.go:111-113 | `default: return nil, fmt.Errorf("apikeytester: unhandled test method %q", meta.TestMethod)` | A.4 | **VIOLATION** | **§S16: NO %w + NO sentinel**. "default" branch returns plain error string with no domain sentinel. errors.Is can't unwrap to anything. errmap will fall to "unmapped domain error" log at ERROR + 500 INTERNAL_ERROR | MED | only triggers if a new `TestMethod*` constant is added in providers.go but switch isn't extended (developer-time bug). Behavior: 500 + ERROR log polluting smoke alarm (the very pattern §S17 is designed to prevent) | introduce sentinel `apikeydomain.ErrUnhandledTestMethod` (or reuse ErrInvalidProvider), wrap with `%w`. Or use panic at this site since it's a wiring bug | **FIXED 2026-05-09 410f664** (resolved as panic — config-time invariant violation, not runtime user error) |
| 5 | tester.go:125-138 | `func testSearchPing(ctx, provider, baseURL, key) *TestResult { switch provider { ... default: return &TestResult{OK: false, Message: "..."} } }` | A.1 | OK | OK = unknown provider returns TestResult{OK:false} (per file's design contract, line 1-8) | N-A | — | — | — |
| 6 | tester.go:145-148, 167-169, 189-191, 211-213, 231-233, 259-261, 283-285, 307-309 | (8 occurrences) `req, err := http.NewRequestWithContext(...); if err != nil { return &TestResult{OK: false, Message: "build request: " + err.Error()} }` | A.1 | OK | per file convention: probe-internal error → TestResult{OK:false}, not Go error. NewRequest only fails on URL parse / invalid method — programmer bug surface, but design says non-fatal soft-fail | N-A | — | — | — |
| 7 | tester.go:151-154, 172-175, 194-197, 217-220, 236-239, 266-269, 287-290, 311-314 | (8 occurrences) `body, status, latency, err := t.do(req); if err != nil { return &TestResult{OK: false, Message: "connection failed: " + err.Error(), LatencyMs: latency} }` | A.1 | OK | per file convention; net failure → TestResult{OK:false} | N-A | — | — | — |
| 8 | tester.go:155-157, 176-178, 198-200, 221-223, 240-242, 270-272, 291-293, 315-317 | (8 occurrences) `if status != http.StatusOK { return &TestResult{OK: false, Message: formatHTTPError(...), LatencyMs: latency} }` | A.1 | OK | per file convention; non-200 → TestResult{OK:false} | N-A | — | — | — |
| 9 | tester.go:336-338 | `resp, err := t.client.Do(req); ... if err != nil { return nil, 0, latency, err }` | A.4 | OK | bare return of network err; wrapping not needed at this lowest layer (callers wrap or convert to TestResult.Message via err.Error()) — sentinel-free network error, no chain to preserve | N-A | — | — | — |
| 10 | tester.go:339 | `defer resp.Body.Close()` | A.1 | OK | §S3 explicit exception: "defer f.Close() 在只读路径（Close 返错对调用方无意义）"; HTTP response body Close on read-completed buffer is canonical | N-A | — | — | — |
| 11 | tester.go:340-343 | `body, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024)); if err != nil { return nil, resp.StatusCode, latency, err }` | A.4 | OK | bare return | N-A | — | — | — |
| 12 | tester.go:376-378 | `if err := json.Unmarshal(body, &resp); err != nil { return nil }` (in parseOpenAIModels) | A.1 | EDGE | §S3 strict: "严禁用静默跳过掩盖失败" — JSON parse failure silenced. Comment at 365-369 documents intent: "连通性仍报告成功" — soft-degrade is intentional (probe = connectivity, not data correctness). Caller will see ModelsFound=nil. **However** error is **not even logged** — there's no audit trail when upstream returns malformed model list. §S3 spec example "FTS5 虚拟表没建成但触发器建成了，INSERT 时才炸" is the warning pattern this matches | LOW | UI shows `connected, 0 models available` instead of `connected, N models available`. User can't distinguish "provider has no models" from "provider returned malformed JSON" — but neither is user-actionable | log at WARN: `s.log.Warn("parseOpenAIModels: malformed JSON from upstream", zap.Error(err))` — but this file has no logger. Either add logger to HTTPTester or accept current behavior (low risk) | **WAIVED 2026-05-09** — connectivity probe by design soft-degrades; "connected, 0 models" vs "connected, malformed list" not user-actionable; logging would add noise without enabling action. Documented intent at lines 365-369. |
| 13 | tester.go:401-403 | `if err := json.Unmarshal(body, &resp); err != nil { return nil }` (in parseModelsByName) | A.1 | EDGE | identical pattern to site #12 — parseModelsByName silently drops parse error | LOW | same as site #12 | same as site #12 | **WAIVED 2026-05-09** — same reason as site #12 |

## Sub-check

A.1 §S3 错误吞没:
  - violations (strict): site #12, #13 (both `parseOpenAIModels` / `parseModelsByName` silent JSON-parse failure — documented intent but no log audit trail)
  - non-violations (per file convention, line 1-8): sites #5-8 — these convert net/HTTP failures to TestResult{OK:false} by design contract

A.2 §S9 detached ctx 终态写:
  - terminal-state writes identified: none
  - 各自 ctx 来源: N/A
  - violations: N/A — file is a stateless HTTP-tester; performs no DB writes, generates no terminal state

A.3 §S15 ID 生成:
  - ID generation calls: none
  - violations: N/A — file generates no business IDs (probe-only)

A.4 §S16 错误 wrap 格式:
  - violations: site #4 (line 112 `default: return nil, fmt.Errorf("apikeytester: unhandled test method %q", ...)` — NO %w, NO sentinel; will trigger errmap "unmapped domain error" alarm)
  - style edges: sites #2, #3 (uses `apikeytester:` prefix instead of `apikey.HTTPTester.Test:` form — sentinel-preservation works but spec literal compliance fails)

A.5 §S17 sentinel 登记 errmap:
  - sentinels defined in this file: none (`var Err... = errors.New(...)` not present)
  - 已登记 errmap (sentinels USED here): apikeydomain.ErrInvalidProvider (errmap.go:47), apikeydomain.ErrBaseURLRequired (errmap.go:48)
  - missing: N/A — file defines no new sentinels; consumed sentinels all registered. (BUT site #4's plain non-sentinel error is what would hit "unmapped" — see A.4)
