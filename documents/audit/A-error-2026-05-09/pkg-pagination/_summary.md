# Package audit summary: internal/pkg/pagination

## Spec understanding (Phase A — §S3 / §S9 / §S15 / §S16 / §S17)

- **§S3 错误不吞**: No swallows. The 3 places where a low-level err is collapsed into `ErrInvalidRequest` (strconv / base64 / json.Unmarshal) are documented input-validation funneling — the caller gets the right HTTP-tier sentinel, the underlying parse error is not actionable. The 2 short-circuits (nil cursor → empty / empty cursor → no-op) are documented contract returns.
- **§S9 detached ctx 终态写**: **N/A** — pure helper package (query parsing + base64/JSON cursor encoding); no `ctx` parameter, no terminal-state writes.
- **§S15 ID 生成**: **N/A** — package doesn't generate business IDs.
- **§S16 错误 wrap 格式**: **4 LOW violations** — all 4 `fmt.Errorf` calls correctly use `%w` (so `errors.Is` traversal to `ErrInvalidRequest` works and errmap.go:44 lookup succeeds → 400 INVALID_REQUEST), but none use the canonical `<pkg>.<Method>:` prefix. They use descriptive English prefixes ("limit must be...", "encode cursor:", "decode cursor:", "unmarshal cursor:") instead of `pagination.Parse:` / `pagination.EncodeCursor:` / `pagination.DecodeCursor:`. User response is unaffected; only server-log traceability degrades (no package/method locator on stack).
- **§S17 errmap 单一事实源**: Package defines no sentinels of its own. The one sentinel it consumes (`errorsdomain.ErrInvalidRequest`) is registered at errmap.go:44.

## Files audited

| File | LOC | Sites | OK | POST-FIX | VIOLATION | EDGE |
|---|---|---|---|---|---|---|
| cursor.go | 101 | 6 | 2 | 0 | 0 | 4 |
| **TOTAL** | **101** | **6** | **2** | **0** | **0** | **4** |

## Severity breakdown

| Severity | Count | Status |
|---|---|---|
| HIGH | 0 | — |
| MED | 0 | — |
| LOW | 4 | FOUND (all §S16 prefix-missing) |

**Net: 4 LOW violations** — all the same pattern (§S16 missing `<pkg>.<Method>:` prefix), no user-facing impact, log-traceability only.

## Cross-cutting

### §S16 prefix pattern

The 4 LOW violations are all instances of the same micro-pattern: the wrap uses a descriptive English phrase as the prefix instead of the canonical `<pkg>.<Method>:` locator. The fix is mechanical:

| Line | Current | Should be |
|---|---|---|
| 55 | `fmt.Errorf("limit must be a positive integer: %w", errorsdomain.ErrInvalidRequest)` | `fmt.Errorf("pagination.Parse: limit must be a positive integer: %w", errorsdomain.ErrInvalidRequest)` |
| 79 | `fmt.Errorf("encode cursor: %w", err)` | `fmt.Errorf("pagination.EncodeCursor: %w", err)` |
| 95 | `fmt.Errorf("decode cursor: %w", errorsdomain.ErrInvalidRequest)` | `fmt.Errorf("pagination.DecodeCursor: %w", errorsdomain.ErrInvalidRequest)` |
| 98 | `fmt.Errorf("unmarshal cursor: %w", errorsdomain.ErrInvalidRequest)` | `fmt.Errorf("pagination.DecodeCursor: %w", errorsdomain.ErrInvalidRequest)` |

The chain (`errors.Is` → `ErrInvalidRequest`) is correct in all 4 cases — `errmap.go:44` does fire properly and clients see the expected `400 INVALID_REQUEST`. So the fix is server-log hygiene, not user-impact.

### Why these are LOW, not MED

§S16 violations elsewhere have been categorized at the spec author's discretion. This package's failure mode is:
- The `%w` chain works (the load-bearing part — `errors.Is` traversal preserved → errmap lookup succeeds → correct HTTP status + wire code reaches client).
- Only the package/method locator is missing from server logs.

Per §S16 spec text "无定位上下文" — this is exactly the deficiency described, but it does not lose error semantics. Compared to the other §S16 failure modes (`%v` instead of `%w`, or `errors.New` over an `err.Error()` string — both of which break the chain and lose sentinel identity), prefix-only-missing is the mildest variant. LOW.

### Sentinel consumption is clean

Site 1, 3, 4 all wrap `errorsdomain.ErrInvalidRequest`, which is the canonical "the input the user sent doesn't make sense" sentinel — registered at errmap.go:44. The pkg package introduces no new sentinels of its own (correct: pagination is a horizontal helper, the appropriate failure semantics already exist in `domain/errors`).

### Why the bracket-fallback validator pattern is OK

Sites 5 + 6 (nil/empty short-circuits) look superficially like swallows but are documented contract returns:
- `EncodeCursor(nil) → ("", nil)`: caller's nil signal that there's no next page (pagination has hit end). Returning empty cursor is the wire-format convention.
- `DecodeCursor("", v) → nil`: caller didn't send a cursor; the `v` pointer keeps its zero value (= "start from the beginning"). Standard cursor pagination convention.

Both are exercised by every store list endpoint in the codebase.

## Spot-check (random clean sites)

This is a 101-line single-file package — every line was traced. No sampling needed at this scale.

Verified errmap chain end-to-end: `errorsdomain.ErrInvalidRequest` → wrapped via `%w` in cursor.go:55/95/98 → `responsehttpapi.FromDomainError` calls `lookup` (errmap.go:254-261) → `errors.Is` traversal succeeds → returns `{http.StatusBadRequest, "INVALID_REQUEST"}` (errmap.go:44) → client sees `{"error": {"code": "INVALID_REQUEST", "message": "..."}}` with HTTP 400. Mechanism intact despite the missing pkg.method prefix.

## Recommended fix priorities

**4 LOW**: §S16 prefix on lines 55, 79, 95, 98. Mechanical edit, ~4 char additions per line. No semantic change to user response. Server logs gain locator context for grep/filter. Recommend bundling with the next pkg-area cleanup commit; not standalone-urgent.

## Out-of-scope notes

1. Site 2 (line 79, `EncodeCursor` json.Marshal failure path) is theoretically dead — the only `Cursor` type in this file is `{time.Time, string}`, both trivially Marshalable. If a caller passed a custom struct that fails to marshal, the resulting raw `err` would surface as 500 INTERNAL_ERROR via the `unmapped domain error` warning path. A defense-in-depth refactor would wrap to `errorsdomain.ErrInternal` so errmap never warns. Not a Phase A audit concern (no current caller triggers it).
2. The `EncodeCursor(any)` interface accepts arbitrary callers; the implicit "Marshalable type" contract is not enforced at the type system. That's an API design call, not a §S3-S17 issue.
