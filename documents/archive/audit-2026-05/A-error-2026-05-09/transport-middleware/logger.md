# Audit: backend/internal/transport/httpapi/middleware/logger.go

**LOC**: 83 (production); function `RequestLogger` + `statusRecorder` adapter.

## Purpose

Emit one structured log line per request (method, path, status, bytes, elapsed). Must be placed INSIDE Recover so access logs see 500s. Wraps `ResponseWriter` to capture status + byte count + Flush passthrough for SSE.

## Trace table

| site# | file:line | snippet | category | classification | reasoning | severity | user_impact | suggested_fix | status |
|---|---|---|---|---|---|---|---|---|---|
| 1 | logger.go:65-72 | `func (r *statusRecorder) Write(b []byte) (int, error) { ... n, err := r.ResponseWriter.Write(b); r.bytes += n; return n, err }` | A.1 | OK | §S3 — `Write` properly returns `err` to caller (handler / earlier middleware). Not swallowed. Recording `r.bytes += n` always (even on partial / error) is the standard `http.ResponseWriter` semantic. | — | — | — | — |
| 2 | logger.go:79-83 | `func (r *statusRecorder) Flush() { if f, ok := r.ResponseWriter.(http.Flusher); ok { f.Flush() } }` | A.1 | OK | §S3 — `http.Flusher.Flush()` returns no error (interface signature `Flush()`). Type-assertion guard handles the case where the underlying writer isn't a Flusher (silent skip is correct: caller can't do anything about it). Not "silent fallback masking failure" — there's no failure mode to mask. | — | — | — | — |
| 3 | logger.go:24-30 | `log.Info("http request", zap.String("method", ...), ..., zap.Int("status", rec.status), ...)` | A.1 | OK | §S3 — info-level log of every request including non-2xx. Status reflected from `statusRecorder`. Non-2xx are visible to operators. No swallow. | — | — | — | — |

## Sub-checks

```
A.1 §S3 错误吞没:
  - violations: not present
A.2 §S9 detached ctx 终态写:
  - terminal-state writes identified: none (log emit is fire-and-forget, not "terminal write" in §S9 sense per spec line 51 — log writes are explicitly excluded)
  - 各自 ctx 来源: N/A
  - violations: N/A: middleware doesn't perform DB / persistent terminal writes
A.3 §S15 ID 生成:
  - ID generation calls: none
  - violations: N/A: package doesn't generate business IDs
A.4 §S16 错误 wrap 格式:
  - violations: not present (no fmt.Errorf calls; only zap.Error usage)
A.5 §S17 sentinel 登记 errmap:
  - sentinels defined: none
  - 已登记 errmap: none
  - missing: N/A: file defines no sentinels
```

## Findings

**Clean** — no §S3/S9/S15/S16/S17 issues. The Flusher type-assertion silent-skip is correct (no error path to expose). Write properly propagates upstream errors.
