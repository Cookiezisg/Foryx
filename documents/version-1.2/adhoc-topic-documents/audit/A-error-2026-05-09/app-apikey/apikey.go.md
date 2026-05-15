# audit: backend/internal/app/apikey/apikey.go

LOC: 296
Read: full file (lines 1-296)

## Trace

| site# | file:line | snippet | category | classification | reasoning | severity | user_impact | suggested_fix | status |
|---|---|---|---|---|---|---|---|---|---|
| 1 | apikey.go:47-49 | `if log == nil { panic("apikey.NewService: logger is nil") }` | A.1 | OK | wiring-time guard; panic is correct for nil-logger bug per §S3 example pattern | N-A | — | — | — |
| 2 | apikey.go:83-85 | `if err := validateCreate(in); err != nil { return nil, err }` | A.1/A.4 | OK | bare return preserves sentinel (validateCreate returns apikeydomain.* sentinels); §S16 spec criteria target fmt.Errorf/errors.New, not bare returns | N-A | — | — | — |
| 3 | apikey.go:86-89 | `uid, err := reqctxpkg.RequireUserID(ctx); if err != nil { return nil, fmt.Errorf("apikey.Service.Create: %w", err) }` | A.4 | OK | §S16 canonical: pkg.Method prefix + %w; sentinel reqctxpkg.ErrMissingUserID preserved (errmap.go:163) | N-A | — | — | — |
| 4 | apikey.go:90-93 | `ciphertext, err := s.encryptor.Encrypt(...); if err != nil { return nil, fmt.Errorf("apikey.Service.Create: encrypt: %w", err) }` | A.4 | OK | §S16 canonical: prefix + %w | N-A | — | — | — |
| 5 | apikey.go:96 | `ID: newID()` (calls `idgenpkg.New("aki")` at line 295) | A.3 | OK | uses idgenpkg.New per §S15; "aki" prefix matches spec list ("aki_" apikey) | N-A | — | — | — |
| 6 | apikey.go:108-110 | `if err := s.repo.Save(ctx, k); err != nil { return nil, err }` | A.1/A.4 | OK | bare return; repo.Save wraps with `apikeystore.Save: %w` internally; sentinel chain preserved | N-A | — | — | — |
| 7 | apikey.go:118-133 | `func validateCreate(in CreateInput) error { ... }` | A.4 | EDGE | line 120 `return fmt.Errorf("provider %q: %w", in.Provider, apikeydomain.ErrInvalidProvider)` — has %w but **prefix is "provider %q" not "apikey.validateCreate" or similar pkg.method**. §S16 spec text "必含 `<pkg>.<Method>:` 前缀" — value of %q substitutes for prefix here. Borderline: helpful debug context vs spec literal compliance. | LOW | minor — error message reads "provider \"openai\": invalid provider" which is informative but loses call-site loc | could be `fmt.Errorf("apikey.validateCreate: provider %q: %w", in.Provider, apikeydomain.ErrInvalidProvider)` or accept current as data-rich variant | **FIXED 2026-05-09 1b96a5e** |
| 8 | apikey.go:122-124 | `if strings.TrimSpace(in.Key) == "" { return apikeydomain.ErrKeyRequired }` | A.4 | OK | direct sentinel return at validation site (deepest layer for this check) | N-A | — | — | — |
| 9 | apikey.go:125 | `meta, _ := GetProviderMeta(in.Provider)` | A.1 | OK | second return is `ok bool` (verified in providers.go pattern), not error; §S3 doesn't apply to non-error discard | N-A | — | — | — |
| 10 | apikey.go:126-128 | `if meta.BaseURLRequired && ... { return apikeydomain.ErrBaseURLRequired }` | A.4 | OK | direct sentinel return | N-A | — | — | — |
| 11 | apikey.go:129-131 | `if in.Provider == "custom" && ... { return apikeydomain.ErrAPIFormatRequired }` | A.4 | OK | direct sentinel return | N-A | — | — | — |
| 12 | apikey.go:136-138 | `k, err := s.repo.Get(ctx, id); if err != nil { return nil, err }` | A.4 | OK | bare return, repo.Get wraps internally | N-A | — | — | — |
| 13 | apikey.go:147-149 | `if err := s.repo.Save(ctx, k); err != nil { return nil, err }` | A.4 | OK | bare return, repo.Save wraps internally | N-A | — | — | — |
| 14 | apikey.go:153-155 | `func Delete(ctx, id) error { return s.repo.Delete(ctx, id) }` | A.4 | OK | direct passthrough; repo wraps | N-A | — | — | — |
| 15 | apikey.go:157-159 | `func Get(ctx, id) ... { return s.repo.Get(ctx, id) }` | A.4 | OK | direct passthrough | N-A | — | — | — |
| 16 | apikey.go:161-163 | `func List(ctx, filter) ... { return s.repo.List(ctx, filter) }` | A.4 | OK | direct passthrough | N-A | — | — | — |
| 17 | apikey.go:181-184 | `uid, err := reqctxpkg.RequireUserID(ctx); if err != nil { return nil, err }` | A.4 | EDGE | bare return — inconsistent with Create line 88 which wraps (`fmt.Errorf("apikey.Service.Create: %w", err)`). Sentinel preserved either way; this is style inconsistency — Create's wrap pattern is canonical per §S16 | LOW | identical UX (errmap maps sentinel to 500 INTERNAL_ERROR either way) | wrap to match Create: `return nil, fmt.Errorf("apikey.Service.Test: %w", err)` | **FIXED 2026-05-09 1b96a5e** (Test wrapped both RequireUserID + repo.Get error sites) |
| 18 | apikey.go:185-187 | `k, err := s.repo.Get(ctx, id); if err != nil { return nil, err }` | A.4 | OK | bare return; repo wraps | N-A | — | — | — |
| 19 | apikey.go:189-192 | `plain, err := s.encryptor.Decrypt(...); if err != nil { return nil, fmt.Errorf("apikey.Service.Test: decrypt: %w", err) }` | A.4 | OK | §S16 canonical | N-A | — | — | — |
| 20 | apikey.go:198 | `detached := reqctxpkg.SetUserID(context.Background(), uid)` | A.2 | POST-FIX OK | per §S9: terminal-write detached ctx pattern; commit d8a5161 (2026-05-09) introduced this; doc comment at lines 168-179 explicitly cites §S9 | N-A | — | — | — |
| 21 | apikey.go:199 | `result, err := s.tester.Test(ctx, ...)` | A.4 | OK | uses ctx (request lifetime) for upstream probe — correct per §S9 ("探测仍用 ctx，客户端断开能立即 cancel") | N-A | — | — | — |
| 22 | apikey.go:200-211 | `if err != nil { ...UpdateTestResult(detached, ...) ... return nil, fmt.Errorf("apikey.Service.Test: tester: %w", err) }` | A.2/A.4 | POST-FIX OK | detached ctx for terminal write; log.Warn surfaces inner failure (§S3 OK — not silent); outer fmt.Errorf §S16 canonical | N-A | — | — | — |
| 23 | apikey.go:207-210 | `if uerr := s.repo.UpdateTestResult(detached, ...); uerr != nil { s.log.Warn(...) }` | A.1 | OK | error logged at WARN with full context (key_id + test_err + uerr); §S3 explicitly NOT silent — comment at lines 201-206 documents the best-effort intent + reasoning | N-A | — | — | — |
| 24 | apikey.go:221-223 | `if upErr := s.repo.UpdateTestResult(detached, id, status, errMsg, models); upErr != nil { return nil, upErr }` | A.2/A.4 | EDGE | detached ctx OK (§S9 ✓). bare return upErr — wraps lost; same style inconsistency as site #17. Caller errmap will still match repo's inner sentinel | LOW | identical UX | wrap: `return nil, fmt.Errorf("apikey.Service.Test: persist result: %w", upErr)` | **FIXED 2026-05-09 1b96a5e** |
| 25 | apikey.go:237-241 | `k, err := s.repo.GetByProvider(ctx, provider); if err != nil { return apikeydomain.Credentials{}, err }` | A.4 | OK | bare return + zero-value entity (idiomatic Go) | N-A | — | — | — |
| 26 | apikey.go:242-245 | `plain, err := s.encryptor.Decrypt(...); if err != nil { return apikeydomain.Credentials{}, fmt.Errorf("apikey.Service.ResolveCredentials: decrypt: %w", err) }` | A.4 | OK | §S16 canonical | N-A | — | — | — |
| 27 | apikey.go:248 | `if meta, ok := GetProviderMeta(provider); ok { ... }` | A.1 | OK | bool ok, not error | N-A | — | — | — |
| 28 | apikey.go:260-273 | `func MarkInvalid(ctx, provider, reason) error { ... s.repo.UpdateTestResult(ctx, ...) ... }` | **A.2** | **VIOLATION** | **§S9 violation**: MarkInvalid is called by upstream chat/LLM caller when they hit 401/403. The UpdateTestResult write is a **terminal-state write** — it changes the user-visible status of an API key from "OK" to "error". If the upstream caller's ctx is cancelled mid-call (browser close, stream cancel), this write fails and **the key stays "OK" while actually being invalid**. Identical bug pattern to Test() that was fixed in d8a5161 — but Test was the only path fixed, MarkInvalid was missed | **HIGH** | API key shows green "OK" status in UI even though upstream returned 401/403; user has no way to know key is bad until they manually retest. Same defect class as the apikey.Test bug we just fixed | derive `detached := reqctxpkg.SetUserID(context.Background(), uid)` (need to RequireUserID first like Test does); use detached for UpdateTestResult; ctx for any GetByProvider read | **FIXED 2026-05-09 410f664** |
| 29 | apikey.go:265-267 | `if err := s.repo.UpdateTestResult(ctx, k.ID, ...); err != nil { return err }` | A.4 | OK (§S16) | bare return preserves sentinel; §S16 doesn't penalize bare returns; the §S9 issue is ctx not the wrap (see site #28) | N-A | — | — | — |
| 30 | apikey.go:295 | `func newID() string { return idgenpkg.New("aki") }` | A.3 | OK | §S15 canonical: idgenpkg.New, prefix per spec list (aki_) | N-A | — | — | — |

## Sub-check

A.1 §S3 错误吞没:
  - violations: not present (line 207-210 logs at WARN with full context; line 125, 248 are bool not error; line 47 panic is wiring guard)

A.2 §S9 detached ctx 终态写:
  - terminal-state writes identified at sites: #20 (Test success path UpdateTestResult), #22 (Test failure path UpdateTestResult), #28 (MarkInvalid UpdateTestResult)
  - 各自 ctx 来源: site #20/#22 = detached (POST-FIX OK); site #28 = ctx (raw r.Context())
  - violations: site #28 (MarkInvalid uses raw ctx for terminal write — same defect class as the d8a5161-fixed bug, but the fix only covered Test() not MarkInvalid)

A.3 §S15 ID 生成:
  - ID generation calls: site #5/#30 = idgenpkg.New("aki") via newID()
  - violations: not present (idgenpkg.New is canonical; "aki" prefix matches spec list)

A.4 §S16 错误 wrap 格式:
  - violations: not present at the strict level (no `%v` instead of `%w`; no `errors.New` cat-and-glue)
  - style edges: site #7 (validateCreate uses `provider %q:` substitute prefix instead of `pkg.method:`); sites #17, #24 (Test bare-returns where Create-style wrap would add call-site context — sentinel preserved either way)

A.5 §S17 sentinel 登记 errmap:
  - sentinels defined in this file: none (file uses `apikeydomain.Err*` and `reqctxpkg.ErrMissingUserID`, defined elsewhere)
  - 已登记 errmap: apikeydomain.ErrInvalidProvider (errmap.go:47), apikeydomain.ErrKeyRequired (errmap.go:50), apikeydomain.ErrBaseURLRequired (errmap.go:48), apikeydomain.ErrAPIFormatRequired (errmap.go:49), reqctxpkg.ErrMissingUserID (errmap.go:163)
  - missing: N/A — file defines no new sentinels
