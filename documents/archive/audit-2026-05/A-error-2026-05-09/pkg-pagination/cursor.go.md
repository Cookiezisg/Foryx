# Audit trace: backend/internal/pkg/pagination/cursor.go

**LOC**: 101
**Sites identified**: 6 (1 query parse + 4 cursor encode/decode + 1 sentinel-registration check).

## 9-column trace table

| site# | file:line | snippet | category | classification | reasoning | severity | user_impact | suggested_fix | status |
|---|---|---|---|---|---|---|---|---|---|
| 1 | cursor.go:53-56 | `n, err := strconv.Atoi(raw); if err != nil \|\| n < 1 { return Params{}, fmt.Errorf("limit must be a positive integer: %w", errorsdomain.ErrInvalidRequest) }` | A.1 / A.4 | EDGE | A.1: not a swallow — both `Atoi` failure AND business rule (`n < 1`) collapse to the same `ErrInvalidRequest` sentinel (correct: API contract is "must be positive integer"; what particular reason it failed is not actionable for the caller). The strconv `err` is intentionally not threaded through because the user-facing message already explains the rule. A.4: **§S16 prefix missing** — wrap should be `fmt.Errorf("pagination.Parse: limit must be a positive integer: %w", errorsdomain.ErrInvalidRequest)`. The `%w` chain itself is correct (errors.Is to ErrInvalidRequest works → errmap lookup at errmap.go:44 succeeds → 400 INVALID_REQUEST), so the user impact is purely log-traceability (no `pagination.Parse:` locator in stack). | LOW | Server logs lack package/method locator on this error; user response unaffected (still 400 INVALID_REQUEST). | Add `pagination.Parse:` prefix per §S16: `fmt.Errorf("pagination.Parse: limit must be a positive integer: %w", errorsdomain.ErrInvalidRequest)`. | FOUND |
| 2 | cursor.go:77-80 | `raw, err := json.Marshal(v); if err != nil { return "", fmt.Errorf("encode cursor: %w", err) }` | A.4 | EDGE | §S16 prefix missing — should be `fmt.Errorf("pagination.EncodeCursor: %w", err)`. The `%w` chain is correct. Note: `json.Marshal` failure for cursor structs is essentially impossible in practice (the only Cursor type is `{time.Time, string}`, both trivially serializable; callers with custom cursor types would hit this), so user-impact is theoretical. Still §S16-non-canonical. | LOW | Theoretical (json.Marshal of cursor structs ~never fails); when it does, server log lacks package/method locator. No registered errmap row for raw json marshal err so this would surface as 500 INTERNAL_ERROR via `unmapped domain error` log. | Add prefix: `fmt.Errorf("pagination.EncodeCursor: %w", err)`. (Optional: also wrap to `errorsdomain.ErrInternal` so the unmapped-error log doesn't fire.) | FOUND |
| 3 | cursor.go:93-96 | `raw, err := base64.RawURLEncoding.DecodeString(cursor); if err != nil { return fmt.Errorf("decode cursor: %w", errorsdomain.ErrInvalidRequest) }` | A.1 / A.4 | EDGE | A.1: not a swallow — `base64` decode `err` collapses to `ErrInvalidRequest` (correct: the user supplied a malformed cursor; the specific base64 failure mode is not actionable). The base64 `err` is dropped from the chain, but the sentinel category is right. A.4: **§S16 prefix missing** — should be `fmt.Errorf("pagination.DecodeCursor: %w", errorsdomain.ErrInvalidRequest)`. The `%w` chain to `ErrInvalidRequest` is correct (errmap.go:44 lookup → 400). | LOW | Server logs lack package/method locator; user response unaffected (400 INVALID_REQUEST). | Add prefix: `fmt.Errorf("pagination.DecodeCursor: %w", errorsdomain.ErrInvalidRequest)`. | FOUND |
| 4 | cursor.go:97-99 | `if err := json.Unmarshal(raw, v); err != nil { return fmt.Errorf("unmarshal cursor: %w", errorsdomain.ErrInvalidRequest) }` | A.1 / A.4 | EDGE | A.1: same pattern as site 3 — `Unmarshal` `err` collapses to `ErrInvalidRequest` (correct). A.4: **§S16 prefix missing** — should be `fmt.Errorf("pagination.DecodeCursor: %w", errorsdomain.ErrInvalidRequest)`. Note: same prefix as site 3 because both are inside `DecodeCursor`. The `%w` chain is correct. | LOW | Same as site 3 — log locator missing only. | Add prefix: `fmt.Errorf("pagination.DecodeCursor: %w", errorsdomain.ErrInvalidRequest)`. | FOUND |
| 5 | cursor.go:74-76 | `if v == nil { return "", nil }` (EncodeCursor nil short-circuit) | A.1 | OK | Not a swallow — nil input is the documented signal for "no more pages" (godoc line 70: "nil → \"\" (no more pages)"). Returning empty cursor + nil error is the contract. | — | — | — | — |
| 6 | cursor.go:90-92 | `if cursor == "" { return nil }` (DecodeCursor empty short-circuit) | A.1 | OK | Not a swallow — empty cursor is documented no-op (godoc line 84-85: "Empty cursor is a no-op (v untouched)"). The caller passes a `v` pointer expecting it to remain at its zero value. | — | — | — | — |

## Sub-checks

**A.1 §S3 错误吞没**
- violations: not present
- Edge cases mapped to documented sentinels (sites 1, 3, 4): the underlying `strconv` / `base64` / `json.Unmarshal` errors are intentionally collapsed into `ErrInvalidRequest`. This is the standard "input-validation funnel" pattern, not a swallow — the caller gets a properly-typed sentinel that maps to 400 via errmap. No `_ = err`, no silent fallback, no `if err != nil { return nil }` patterns.
- Sites 5 + 6 (nil/empty short-circuits) are documented contract returns, not swallows.

**A.2 §S9 detached ctx 终态写**
- terminal-state writes identified: none
- 各自 ctx 来源: N/A
- violations: N/A — pure helper package (HTTP query parsing + base64/JSON encoding). No DB / network / terminal-state writes; no `ctx` parameter at all.

**A.3 §S15 ID 生成**
- ID generation calls: none
- violations: N/A — package doesn't generate business IDs.

**A.4 §S16 错误 wrap 格式**
- violations: 4 sites missing `<pkg>.<Method>:` prefix (sites 1, 2, 3, 4)
- All wraps correctly use `%w` (sentinel chain intact, `errors.Is` works, errmap lookup succeeds for ErrInvalidRequest). The deficiency is purely the locator prefix:
  - line 55 has prefix `"limit must be a positive integer: "` (human-readable, not pkg.method)
  - line 79 has prefix `"encode cursor: "` (descriptive, not pkg.method)
  - line 95 has prefix `"decode cursor: "` (descriptive, not pkg.method)
  - line 98 has prefix `"unmarshal cursor: "` (descriptive, not pkg.method)
- Per §S16: format must be `fmt.Errorf("<pkg>.<Method>: %w", err)`. All 4 should be `pagination.Parse: ...` / `pagination.EncodeCursor: %w` / `pagination.DecodeCursor: %w`. Severity LOW because user response is unaffected (errmap chain works); only server-side log traceability is degraded.

**A.5 §S17 sentinel 登记 errmap**
- sentinels defined: none (file declares no `var Err...` of its own)
- 已登记 errmap: N/A for own sentinels; the consumed sentinel `errorsdomain.ErrInvalidRequest` IS registered (errmap.go:44 → `{http.StatusBadRequest, "INVALID_REQUEST"}`)
- missing: none — file leans on `errorsdomain.ErrInvalidRequest` exclusively, which is registered. No new sentinel introduced by this package.
