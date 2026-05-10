# Audit trace: backend/internal/pkg/idgen/idgen.go

**LOC**: 25
**Sites identified**: 2 (one ID generation site = the canonical §S15 implementation; one panic-on-rand-fail guard).

## 9-column trace table

| site# | file:line | snippet | category | classification | reasoning | severity | user_impact | suggested_fix | status |
|---|---|---|---|---|---|---|---|---|---|
| 1 | idgen.go:19-25 | `func New(prefix string) string { var b [8]byte; if _, err := rand.Read(b[:]); err != nil { panic(...) }; return prefix + "_" + hex.EncodeToString(b[:]) }` | A.3 | OK | §S15 canonical implementation. (a) Uses `crypto/rand` (not `math/rand`). (b) 8 random bytes → 16 hex = exactly the spec format. (c) `rand.Read` failure path panics with informative message — matches §S15 "rand.Read 失败必须 panic"; godoc explicitly justifies the panic ("a broken entropy source would silently produce colliding IDs"). (d) Format string `prefix + "_" + hex.EncodeToString(b[:])` produces `<prefix>_<16hex>` exactly. This is the file every `newID()` in the codebase delegates to per §S15. | — | — | — | — |
| 2 | idgen.go:21-23 | `if _, err := rand.Read(b[:]); err != nil { panic(fmt.Sprintf("idgen: crypto/rand failed: %v", err)) }` | A.1 | OK | Not a §S3 swallow — error is **propagated as panic**. §S15 explicitly mandates panic on `rand.Read` failure (entropy source broken → silent collision risk is worse than crashing). The `%v` here is correct (not §S16 violation): panic message is a free-form string for the recover/log layer, not a wrapped error chain that anyone needs to `errors.Is` against. | — | — | — | — |

## Sub-checks

**A.1 §S3 错误吞没**
- violations: not present
- The single error path (`rand.Read` failure) propagates as panic with descriptive message. No `_ = err`, no silent fallback, no swallow.

**A.2 §S9 detached ctx 终态写**
- terminal-state writes identified: none
- 各自 ctx 来源: N/A
- violations: N/A — package is a pure ID-minting helper, no DB / store / network calls; no `ctx` parameter at all.

**A.3 §S15 ID 生成**
- ID generation calls: 1 site (`New`, the canonical implementation)
- violations: not present
- Validation:
  - `crypto/rand` used (line 8 import + line 21 call) — not `math/rand` / `time.Now()`.
  - 8 bytes → `hex.EncodeToString` produces 16 hex chars — matches spec.
  - Format string is `prefix + "_" + hex(...)` = `<prefix>_<16hex>` exactly.
  - `rand.Read` failure → `panic` with descriptive message — matches §S15 hard requirement.

**A.4 §S16 错误 wrap 格式**
- violations: not present
- No `fmt.Errorf` calls; no `errors.New`; no error returns. Only an `fmt.Sprintf` inside `panic()`, which is a panic message, not a wrapped error chain.

**A.5 §S17 sentinel 登记 errmap**
- sentinels defined: none
- 已登记 errmap: N/A
- missing: N/A — file defines no sentinels; no `var Err... = errors.New(...)`.
