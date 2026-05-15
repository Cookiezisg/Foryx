# D2 Doc-Sync Audit — apikey

Scope:
- Doc: `documents/version-1.2/service-design-documents/apikey.md`
- Code: `backend/internal/{domain,app,infra/store}/apikey/` + `backend/internal/transport/httpapi/handlers/apikey.go`

D1 already scanned the contract documents. Below: only design-doc-vs-code drifts not subsumed by D1's `_summary.md`.

---

## In code but not in doc

| Item | Code location | Severity |
|---|---|---|
| `MachineFingerprintEncryptor` / `cryptodomain.Encryptor` import alias is `cryptodomain`, not `crypto` as doc renders it (§8 Service struct) | `internal/app/apikey/apikey.go:23,36` | LOW |
| `RequireUserID` (returns `(string, error)` + sentinel `reqctxpkg.ErrMissingUserID`) is the actual helper used; doc §3 + §15.2 say `reqctx.GetUserID(ctx)` | `internal/pkg/reqctx/reqctx.go:48`, `internal/app/apikey/apikey.go:86,181,271`, `internal/infra/store/apikey/apikey.go:50,74,132,169,191` | LOW |
| `Service.Test` & `Service.MarkInvalid` use **detached ctx** for `UpdateTestResult` writes (§S9 terminal-state pattern). Doc §8 Test 流程 + §15.2 not reflecting detached pattern. | `internal/app/apikey/apikey.go:198,207,221,279-280` | LOW |
| `Service.Test` requires `RequireUserID` upfront (terminal-write pre-flight), not "缺失 = 接线 bug，上抛" implied at §8 (Service.Test step 1 jumps straight to repo.Get) | `internal/app/apikey/apikey.go:181-184` | LOW |
| Anthropic provider DefaultBaseURL is `https://api.anthropic.com` and the tester calls `/v1/messages` (i.e. POST to `<baseURL>/v1/messages`); doc tester table §9 row matches but §4 LLM table row notes the DefaultBaseURL value with no version suffix — minor consistency note (already correct) | `internal/app/apikey/providers.go:78`, `tester.go:268` | LOW |

## In doc but not in code (stale)

| Item | Doc location | Severity |
|---|---|---|
| **§5 Sentinel 错误 (`internal/domain/apikey/apikey.go`)** doc claims sentinel + Repository + KeyProvider all in `apikey.go`. ✅ matches. But §17 implementation list line `internal/domain/apikey/providers.go — TestMethod 枚举 (5 个) + ProviderMeta + 11 providers 白名单 + GetProviderMeta / IsValidProvider / ListProviders` is **wrong** — file does not exist; provider registry now lives at `internal/app/apikey/providers.go` | apikey.md:71, 832 | **HIGH** |
| §17 also references `internal/domain/apikey/providers_test.go — 5 个白名单完整性测试`. **No such file**; test file is `internal/app/apikey/providers_test.go` | apikey.md:833 | **HIGH** |
| **§4 Provider count** says "11 LLM 生产 + 1 LLM dev mock + 4 搜索（共 16）". Doc §17 line 832 says "11 providers 白名单". Code has **12 LLM (11 prod + 1 mock) + 4 search = 16 total** registered in `providers` map. Numbers in §4 (16) match; §17 line "11 providers 白名单" is stale (should be 16). | apikey.md:832 | LOW |
| **§4 TestMethod 枚举 (5 个)** count is wrong — code has **7** (`get_models / anthropic_ping / google_list_models / ollama_tags / custom / always_ok / search_ping`); doc text §9 table only lists 5 (missing `TestMethodAlwaysOK` for mock and `TestMethodSearchPing` for search) | apikey.md:106-114, 484-490 | **MED** |
| **§4 Provider table for LLM** lists `mock` row with `TestMethod` `TestMethodAlwaysOK` (line 88) — that's correct. But **§9 HTTPTester dispatch table** (lines 484-490) omits `TestMethodAlwaysOK` and `TestMethodSearchPing` rows — doc reader thinks unknown TestMethod. | apikey.md:484-490 | **MED** |
| **§5 APIKey struct** doc line 144-160 lists fields **without** `ModelsFound`. Then field-说明 table 180 lists `ModelsFound` as a separate row. The displayed Go struct literal is missing the `ModelsFound []string` line. | apikey.md:144-160 | **MED** |
| §6.3 `KeyProvider` interface code excerpt: doc shows correct signature; OK. But §6.4 says "MaskKey | `MaskKey(string) string` | `app/apikey/mask.go`" — **`mask.go` does not exist**; `MaskKey` is in `app/apikey/apikey.go:298-308` | apikey.md:264 | **MED** |
| §6.4 says "Service (CRUD + Test 编排) | `Service` | `app/apikey/service.go`". File is **`apikey.go`**, not `service.go`. Doc §17 also lists "`internal/app/apikey/service.go`" — stale (S12 rename used `apikey.go` as the package main file). | apikey.md:260, 844 | **MED** |
| §17 lists `internal/app/apikey/keyprovider.go — Service 实现 apikeydomain.KeyProvider + 编译期 var _ 守护`. **No `keyprovider.go`** — `var _ apikeydomain.KeyProvider = (*Service)(nil)` is in `apikey.go:80`; `ResolveCredentials` + `MarkInvalid` methods are also in `apikey.go:237,270` | apikey.md:845 | **HIGH** |
| §17 lists `internal/transport/httpapi/middleware/auth.go — InjectUserID`. Path is `internal/transport/httpapi/middleware/auth.go` — let me confirm it exists. Doc claim is correct. But doc §3 implementation table line 64 uses path `internal/pkg/reqctx/userid.go` — that file does **not** exist; the userID helpers live in `internal/pkg/reqctx/reqctx.go` | apikey.md:64 | **MED** |
| §11 数据库表 SQL says explicit `CREATE INDEX idx_api_keys_user_id` + `idx_api_keys_user_provider`. Code GORM tag declares both indexes via `index:idx_api_keys_user_id;index:idx_api_keys_user_provider,priority:1` on UserID and `priority:2` on Provider. **D1 _summary HIGH #5** already flagged that the actual index that GORM emits with mixed priorities is suspect (single-col on Provider). This is a code-bug + design-doc-mirror; classifying here as already-covered-by-D1 (not double flag). | apikey.md:646-648 | (covered by D1) |
| §13 错误码 table row 3 says `INVALID_PROVIDER` HTTP `400` sentinel `apikey.ErrInvalidProvider`. Errmap is correct. But doc §13 footer "**13. 错误码（8 个 sentinel，全已实现 ✅）**" — code ships 8 sentinels (ErrNotFound / ErrNotFoundForProvider / ErrInvalidProvider / ErrBaseURLRequired / ErrAPIFormatRequired / ErrKeyRequired / ErrTestFailed / ErrInvalid). Match. | — | OK |
| §15.2 step `→ encryptor.Decrypt(KeyEncrypted) → 明文 / 密文损坏 → 上抛 500`. Code wraps as `fmt.Errorf("apikey.Service.Test: decrypt: %w", err)` then ALSO writes `UpdateTestResult(detached, ..., TestStatusError, err.Error(), nil)` if tester errs — but **decrypt errors return early before the tester-error path**, so a decrypt failure leaves `test_status` unchanged. Doc step doesn't note this. | apikey.md:770-771 | LOW |
| §17 lists `internal/app/apikey/tester.go` — match. But doc §17 also says "21 个 httptest 用例" — out of scope (no test file scan). | — | — |

## Mismatched (different details)

| Item | Code | Doc | Severity |
|---|---|---|---|
| Number of public Service methods | 8 (Create / Update / Delete / Get / List / Test / ResolveCredentials / MarkInvalid) | §8 says "6 个公开 + 2 个 KeyProvider 实现" — mathematically same, just labelled. Match. | — |
| `Service.Test` log fields | `key_id` / `provider` / `ok` / `latency_ms` (line 224-228) | §8 流程 step 5 says `log.Info("apikey tested", key_id, provider, ok, latency_ms)`. Match. | OK |
| **`MarkInvalid` semantics** | requires UserID upfront, fetches `GetByProvider`, writes `UpdateTestResult` on detached ctx. **Updates by `k.ID`** (the picked APIKey row) | §6.3 KeyProvider doc says "`MarkInvalid(ctx, provider, reason) error`" — matches signature. §15.3 chat path describes the call. ✅ | OK |
| `validateCreate` ordering | provider whitelist → key non-empty → baseURL → APIFormat | §8 Create 流程 step 1 lists same 4 checks in same order. Match. | OK |
| **`Service.Test` `RequireUserID` precondition** | If user-id missing in ctx, `Test` returns wrapped `ErrMissingUserID` **before** repo.Get — note: `Test` checks user upfront; doc shows `repo.Get(ctx, id)` first. Pre-flight in code is silent in the doc. | apikey.md:768-769 vs apikey.go:181-184 | **MED** |
| **§9 baseURL 规范化** doc says "仍空 (ollama / custom) → 返 ErrBaseURLRequired". Code adds extra branch: `if effective == "" && meta.TestMethod != TestMethodAlwaysOK` — `mock` provider has empty DefaultBaseURL but TestMethod = AlwaysOK, so it bypasses the empty-baseURL guard. Doc §9 doesn't mention this carve-out. | apikey.md:493-494 vs apikey.go:82 | LOW |
| **§9 模型列表解析 — Google + Ollama** doc table claims separate parsers per provider. Code uses one shared `parseModelsByName` for both. Doc tester.go §parseXxx claims "`{"models":[{"name":"..."}]}`" — code matches. **Doc table is mostly right** but says "Google" + "Ollama" with two rows; underlying is one helper. (Cosmetic; doc not wrong.) | apikey.md:506-508 | LOW |
| **§14 chat domain 调 LLM 时 — `MarkInvalid(ctx, provider, streamErr.Error())`** doc shows. Code matches; OK. | — | OK |
| **§S20 default mock-llm provider DefaultBaseURL** says (none) — code `mock` provider has **no DefaultBaseURL at all** and `BaseURLRequired` is unset (false default). Doc §4 mock row "默认 base_url" col `—`, "`base_url` 必填" col "否". Match. | — | OK |
| **§9 hand-rolled HTTP** justification — `infra/llm` claim. ✅ doc-only, no code conflict. | — | OK |

## Sub-check

- **Entities aligned**: **No** — `APIKey` struct in doc §5 (lines 144-160) is missing the `ModelsFound []string` field that exists in code line 36. Field table at line 180 lists it but the embedded Go literal contradicts.
- **Service methods aligned**: **Yes** — 8 methods all match in signature. Note `Service.Test` and `Service.MarkInvalid` use detached-ctx pattern not described in doc §8 / §15.
- **Endpoints aligned**: **Yes** — 5 endpoints (POST/GET/PATCH/DELETE/POST :test) registered in `apikey.go:50-55`, doc §6.2 lists same 5. (D1 covered contract level.)
- **Sentinels aligned**: **Yes** — 8 sentinels in code, all mapped in `errmap.go:48-55`, doc §5 §13 table cover same 8. Errmap entry `ErrTestFailed` is registered (line 54) **even though** §13 says "handler 直接 synthesize，不经 errmap" — code matches the synthesize path (`apikey.go:193`); errmap entry is harmless redundancy.
- **端到端推演 valid**: **Mostly** — §15 chains are accurate at the layer-flow level. Drifts: (a) `RequireUserID` not `GetUserID`; (b) detached-ctx terminal write on `Test`/`MarkInvalid` not depicted; (c) `Service.Test` upfront `RequireUserID` not in step 1; (d) decrypt failure path not covered.

---

## File-naming drift summary (S12 rename impact)

The biggest cluster of doc-vs-code drift is the §17 "实现清单" — it still uses old per-concept filenames (`service.go`, `keyprovider.go`, `mask.go`) while the code has consolidated into the §S12 main-file convention (`apikey.go` is the package main file holding Service + KeyProvider impl + MaskKey + ID helper). Same applies to `domain/apikey/providers.go` reference (§17 line 832-833) — the registry now lives in `app/apikey/providers.go`.

Severity: 3 HIGH (whole files referenced don't exist) + 4 MED (file names wrong) + ~5 LOW (fine-grained drift).

---

**Totals:** 3 HIGH / 6 MED / 9 LOW
