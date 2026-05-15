# util.go — audit trace

**Path**: `backend/internal/transport/httpapi/handlers/util.go`
**LOC**: 24
**Role**: Single helper `idAndAction(r, key)` that splits a path segment shaped like `"<id>:<action>"`. Used by apikey / forge handlers to dispatch `POST /{id}:action` URLs (§N5).

## 9-col trace

| site# | file:line | snippet | category | classification | reasoning | severity | user_impact | suggested_fix | status |
|---|---|---|---|---|---|---|---|---|---|
| 1 | util.go:16-24 | `func idAndAction(r *http.Request, key string) (id, action string, ok bool) { raw := r.PathValue(key); for i := 0; ... }` | A.1/A.4 | OK | Pure string-split helper. No `error` returned, no `_ = err` to evaluate, no fmt.Errorf. PathValue returns "" if key missing — the loop returns `(raw, "", false)` for a non-colon raw, which is the documented contract. | N-A | — | — | — |

## Sub-checks

**A.1 §S3 错误吞没**:
- violations: not present (no error path in this file)

**A.2 §S9 detached ctx 终态写**:
- terminal-state writes identified: none
- 各自 ctx 来源: N/A
- violations: N/A: file is a pure string helper with no ctx / no DB writes

**A.3 §S15 ID 生成**:
- ID generation calls: none
- violations: N/A: file does not generate business IDs (it only parses `{id}:{action}` from URL path)

**A.4 §S16 错误 wrap 格式**:
- violations: not present (no `fmt.Errorf` / `errors.New` calls)

**A.5 §S17 sentinel 登记 errmap**:
- sentinels defined: none
- 已登记 errmap: N/A
- missing: N/A: file defines no sentinels

## Summary

- Sites: 1
- Violations: 0 (0 HIGH / 0 MED / 0 LOW)
- Verdict: textbook-clean. 24-line pure path-parsing helper, no error / ctx / ID / sentinel surface to audit against §S3-§S17.
