# Package audit summary: internal/pkg/idgen

## Spec understanding (Phase A — §S3 / §S9 / §S15 / §S16 / §S17)

- **§S3 错误不吞**: One error path (`rand.Read` failure) — propagated as `panic` with descriptive message, exactly as §S15 mandates. No swallowing.
- **§S9 detached ctx 终态写**: **N/A** — pure helper function, no `ctx` parameter, no DB / network / terminal-state writes.
- **§S15 ID 生成**: This **is** the §S15 canonical implementation. Validated: `crypto/rand` (not `math/rand`); 8 bytes → 16 hex; `<prefix>_<16hex>` format; `rand.Read` failure panics. Textbook clean.
- **§S16 错误 wrap 格式**: **N/A** — zero `fmt.Errorf` / `errors.New`. The lone `fmt.Sprintf` is inside a `panic()` (free-form panic message, not a wrapped error chain).
- **§S17 errmap 单一事实源**: **N/A** — no sentinels defined.

## Files audited

| File | LOC | Sites | OK | POST-FIX | VIOLATION | EDGE |
|---|---|---|---|---|---|---|
| idgen.go | 25 | 2 | 2 | 0 | 0 | 0 |
| **TOTAL** | **25** | **2** | **2** | **0** | **0** | **0** |

## Severity breakdown

| Severity | Count | Status |
|---|---|---|
| HIGH | 0 | — |
| MED | 0 | — |
| LOW | 0 | — |

**Net: 0 violations**.

## Cross-cutting

### Canonical §S15 implementation

This package is the **single source of truth** for §S15 ID format. CLAUDE.md §S15 explicitly references it:

> 实现统一在 `pkg/idgen.New(prefix)`

The audit confirmed the implementation matches the spec word-for-word:
1. `crypto/rand.Read(b[:])` over 8 bytes (line 21)
2. `panic` on `rand.Read` failure (line 22) — godoc at line 14-15 explicitly explains the safety rationale
3. `prefix + "_" + hex.EncodeToString(b[:])` produces `<prefix>_<16hex>` (line 24)

Every other `newID()` / inline ID minting in the codebase delegates to this `New(prefix)` per §S15. Auditing those delegations is downstream concern — at the source it's clean.

### Why the panic message uses `%v` is not §S16-relevant

`fmt.Sprintf("idgen: crypto/rand failed: %v", err)` is a panic argument, not a wrapped error returned to a caller. §S16's `%w` requirement applies to error chains that need to remain `errors.Is`-traversable. A panic propagates a string up through `recover` / log middleware — there's no chain to preserve. The `%v` is correct here.

## Spot-check (random clean sites)

This package only has 1 function (`New`, 7 lines of body). Every byte was line-by-line verified against §S15 — no spot-check sampling needed at this scale.

## Recommended fix priorities

**No fixes needed**. Package is §S3/S9/S15/S16/S17 textbook clean and serves as the §S15 reference.

## Out-of-scope notes

None. This is a 25-line file doing one job correctly.
