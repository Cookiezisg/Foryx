# Audit: backend/internal/transport/httpapi/response/envelope.go

**LOC**: 86 (production); helpers `Success`, `Created`, `NoContent`, `Paged`, `Error`, internal `writeJSON`.

## Purpose

N1 envelope writers. Success → `{"data": ...}`. Failure → `{"error": {"code", "message", "details"}}`. Single source of truth for handler-side response shapes (handlers MUST go through this package; no direct `w.Write` / `json.Encode` per package godoc).

## Trace table

| site# | file:line | snippet | category | classification | reasoning | severity | user_impact | suggested_fix | status |
|---|---|---|---|---|---|---|---|---|---|
| 1 | envelope.go:81-85 | `func writeJSON(w http.ResponseWriter, status int, body envelope) { w.Header().Set("Content-Type", "application/json"); w.WriteHeader(status); _ = json.NewEncoder(w).Encode(body) }` | A.1 | EDGE | §S3 — `_ = json.NewEncoder(w).Encode(body)` swallows the encode error. **However**: (a) the body is always our own `envelope` struct (concrete type), only `Data any` could fail — but at this point `w.WriteHeader(status)` already fired, so changing the wire bytes is impossible (same Go-HTTP "can't unflush" pattern as Recover). (b) The "no inline comment" point is the gap — current §S3 spec line 12-14 says `_ = func()` without a justifying comment is the typical violation pattern. Compare to recover.go which DOES annotate the same flushed-no-recovery situation. Recommend a 1-line comment matching recover.go style ("// _ = err — body already on wire, no recovery possible"). | LOW | None — failure mode is post-headers-flushed and there's nothing actionable. Operator wouldn't see a log line if `Data` is non-encodable, but in practice all `Data` values are concrete domain types JSON-tested via pipeline | Add inline comment: `// _ = err — header already flushed; encode failure is unrecoverable at this point and Data shape is type-checked at compile time.` Optional: log via passed-in zap if we make writeJSON take a logger (overkill for the actual risk). | FOUND |
| 2 | envelope.go:36-38 | `func Success(w http.ResponseWriter, status int, body any) { writeJSON(w, status, envelope{Data: body}) }` | A.1 | OK | §S3 — pure delegation to writeJSON; no error path of its own. | — | — | — | — |
| 3 | envelope.go:43-45 | `func Created(w http.ResponseWriter, body any) { Success(w, http.StatusCreated, body) }` | A.1 | OK | §S3 — pure delegation. | — | — | — | — |
| 4 | envelope.go:50-52 | `func NoContent(w http.ResponseWriter) { w.WriteHeader(http.StatusNoContent) }` | A.1 | OK | §S3 — `WriteHeader` returns no error. No-body 204 is correct semantic. | — | — | — | — |
| 5 | envelope.go:59-65 | `func Paged(w http.ResponseWriter, items any, nextCursor string, hasMore bool) { env := envelope{Data: items, HasMore: &hasMore}; if nextCursor != "" { env.NextCursor = &nextCursor }; writeJSON(w, http.StatusOK, env) }` | A.1 | OK | §S3 — pure construction + delegation; no error opportunity beyond writeJSON's. | — | — | — | — |
| 6 | envelope.go:73-79 | `func Error(w http.ResponseWriter, status int, code, message string, details any) { writeJSON(w, status, envelope{Error: &errorBody{Code: code, Message: message, Details: details}}) }` | A.1 | OK | §S3 — pure construction + delegation. The actual sentinel→code translation is in errmap.go::FromDomainError; this is the lowest-level emit primitive. | — | — | — | — |

## Sub-checks

```
A.1 §S3 错误吞没:
  - violations: [site#1] (writeJSON: `_ = json.NewEncoder(w).Encode(body)` lacks inline justification per §S3 example 1)
A.2 §S9 detached ctx 终态写:
  - terminal-state writes identified: none (these are wire-write helpers, not DB persistence)
  - 各自 ctx 来源: N/A (functions don't take ctx; they write to ResponseWriter directly)
  - violations: N/A: package doesn't perform DB / persistent terminal writes
A.3 §S15 ID 生成:
  - ID generation calls: none
  - violations: N/A: package doesn't generate business IDs
A.4 §S16 错误 wrap 格式:
  - violations: not present (no fmt.Errorf calls)
A.5 §S17 sentinel 登记 errmap:
  - sentinels defined: none
  - 已登记 errmap: N/A — this file is consumed BY errmap.go::FromDomainError as the underlying writer
  - missing: N/A: file defines no sentinels (errorBody is wire shape, not Go sentinel)
```

## Findings

**1 LOW EDGE**: writeJSON's `_ = json.NewEncoder(w).Encode(body)` is missing the inline justification §S3 spec recommends. Functionally correct (post-WriteHeader nothing can recover), just lacks the documentation ritual. Recover middleware annotates the same situation; envelope should match for consistency.
