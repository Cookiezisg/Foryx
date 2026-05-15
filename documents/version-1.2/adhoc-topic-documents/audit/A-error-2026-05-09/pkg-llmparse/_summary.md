# Package audit summary: internal/pkg/llmparse

## Spec understanding (Phase A — §S3 / §S9 / §S15 / §S16 / §S17)

- **§S3 错误不吞**: Two error-adjacent sites; both are documented API-contract returns (boolean parse-failure signal in `ExtractJSON`; probe-pattern boolean in `IsLikelyJSON`). Neither drops a meaningful error — both are deliberate "no actionable error context to convey" designs. Not swallows.
- **§S9 detached ctx 终态写**: **N/A** — pure string-parsing helper; no `ctx` parameter, no terminal-state writes.
- **§S15 ID 生成**: **N/A** — package doesn't generate business IDs.
- **§S16 错误 wrap 格式**: **N/A** — zero `fmt.Errorf` / `errors.New`; package returns no `error` values.
- **§S17 errmap 单一事实源**: **N/A** — no sentinels defined.

## Files audited

| File | LOC | Sites | OK | POST-FIX | VIOLATION | EDGE |
|---|---|---|---|---|---|---|
| extractjson.go | 58 | 2 | 2 | 0 | 0 | 0 |
| **TOTAL** | **58** | **2** | **2** | **0** | **0** | **0** |

## Severity breakdown

| Severity | Count | Status |
|---|---|---|
| HIGH | 0 | — |
| MED | 0 | — |
| LOW | 0 | — |

**Net: 0 violations**.

## Cross-cutting

### Boolean-return contract vs §S3

`llmparse` is the textbook case where §S3's "swallow" rule does **not** apply. Both exported functions return booleans by design:

- `ExtractJSON(s) (string, bool)` — godoc line 15-16 explicitly says `"", false` means "nothing parses". This is the standard "comma-ok" Go idiom (`v, ok := m[key]`) generalized to a parse helper. There's no information loss: the failure mode is binary, not richly typed.
- `IsLikelyJSON(s) bool` — a probe whose entire reason for existing is "does this parse?". The `err == nil` collapse is the function's defining behaviour, not a swallow.

§S3 targets cases where a meaningful error (auth failure, missing file, schema mismatch) gets dropped silently. Here, the only error info available is "json.Unmarshal disagreed" — wrapping that into a typed error would be cosmetic and force callers into pointless `errors.Is` checks.

### Why no errmap entry

The package returns no errors. No sentinel can leak into a handler. `errmap.go` registration is correctly absent.

## Spot-check (random clean sites)

This is a 58-line single-file package with 2 functions. Every line of `ExtractJSON` and every line of `IsLikelyJSON` was traced — no spot-check sampling is meaningful at this scale.

Verified edge: the bracket-fallback at lines 38-47 uses `IsLikelyJSON` (line 43) precisely as the godoc claims — the validator gate prevents stray-bracket false positives in prose-wrapped LLM output. Mechanism aligns with documented intent.

## Recommended fix priorities

**No fixes needed**. Package is §S3/S9/S15/S16/S17 textbook clean.

## Out-of-scope notes

None. This is a 58-line single-file utility package with a tightly scoped contract.
