# Audit trace: backend/internal/pkg/llmparse/extractjson.go

**LOC**: 58
**Sites identified**: 2 (one boolean parse-failure return = `ExtractJSON`'s `"", false`; one `json.Unmarshal` error-to-bool collapse in `IsLikelyJSON`).

## 9-column trace table

| site# | file:line | snippet | category | classification | reasoning | severity | user_impact | suggested_fix | status |
|---|---|---|---|---|---|---|---|---|---|
| 1 | extractjson.go:48 | `return "", false` (after all fence + bracket attempts fail) | A.1 | OK | Not a §S3 swallow. The function's contract is `(string, bool)` — `false` IS the documented "nothing parses" signal (godoc line 15-16: "Returns \"\", false when nothing parses"). No `error` is being silently dropped here; the function elects not to surface a typed error because there's nothing actionable to convey beyond "no JSON found". Callers (forge GenerateTestCases / search_forges per godoc line 2) can decide their own error semantics. This is API design, not error swallow. | — | — | — | — |
| 2 | extractjson.go:55-58 | `func IsLikelyJSON(s string) bool { var v any; return json.Unmarshal([]byte(s), &v) == nil }` | A.1 | OK | Not a §S3 swallow. `IsLikelyJSON` is a **probe** — its entire purpose is "does this parse?". The `err` from `json.Unmarshal` is not dropped; it is **consumed** as the inverse of the boolean return. Per §S3 example list, this is the validator/probe pattern (callers want yes/no, not error context); the godoc explicitly names it `IsLikely…` to make the boolean contract visible. The single caller site (line 43) uses the bool to drive bracket-fallback validation — exactly the documented "bracket fallback validates via json.Unmarshal to avoid stray-bracket false positives" use case (line 16-17). | — | — | — | — |

## Sub-checks

**A.1 §S3 错误吞没**
- violations: not present
- The two error-adjacent sites are both API-contract returns, not swallows:
  - `ExtractJSON` returns `(string, bool)` per godoc; `false` is the documented sentinel for "nothing parses" — a deliberate choice to keep the helper pithy (callers don't need a typed parse-error).
  - `IsLikelyJSON` is a boolean probe; consuming `err == nil` as the truth value is exactly the probe pattern.

**A.2 §S9 detached ctx 终态写**
- terminal-state writes identified: none
- 各自 ctx 来源: N/A
- violations: N/A — pure string-parsing helper. No `ctx` parameter, no DB / network / terminal-state writes.

**A.3 §S15 ID 生成**
- ID generation calls: none
- violations: N/A — package doesn't generate business IDs.

**A.4 §S16 错误 wrap 格式**
- violations: not present
- No `fmt.Errorf` calls; no `errors.New`; no error returns at all. There's nothing to wrap.

**A.5 §S17 sentinel 登记 errmap**
- sentinels defined: none
- 已登记 errmap: N/A
- missing: N/A — file defines no sentinels; no `var Err... = errors.New(...)`.
