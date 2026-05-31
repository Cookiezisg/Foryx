# Audit trace: backend/internal/app/subagent/queries.go

LOC: 28. Single function `Cancel` — preempts a running sub-run via its registered cancel func from `Service.activeRuns`. No-op when run-not-found (already terminated or never spawned).

## 9-col trace table

| site# | file:line | snippet | category | classification | reasoning | severity | user_impact | suggested_fix | status |
|---|---|---|---|---|---|---|---|---|---|
| 1 | queries.go:19-28 | `func (s *Service) Cancel(_ context.Context, runID string) error { s.activeRunsMu.Lock(); cancel, ok := s.activeRuns[runID]; s.activeRunsMu.Unlock(); if !ok { return nil }; cancel(); return nil }` | A.1/A.2/A.4 | OK | §S3 ✓: `if !ok { return nil }` is **not** a silent error suppression — `ok=false` means "run not found", which is documented as a benign race per `subagent.md` §8.5 ("v1 不实现 cancel API；主对话 cancel 间接 cancel 所有 sub" → followup design admits external cancel is best-effort: terminated runs and never-spawned runs both legitimately yield `!ok`). Returning `nil` (success) preserves caller idempotency — POST `/cancel` against an already-finished run shouldn't 404 because the user-visible outcome ("run is no longer running") matches. The doc comment at line 14-16 explicitly documents this contract ("已终止或没起过——与 finish 竞态无害"). §S2 §exception 1 ("如果只是清理资源失败且不影响业务，吞 OK") applies — though here it's not even a failure, it's "already in target state". §S9 ✓: `Cancel` operates purely on in-memory map lookup + cancel func invocation; no DB write, no emit — `_ context.Context` parameter is unused (terminal-state-write contract doesn't apply because there's no write). §S16 ✓: returns nil on the only path; no error wrapping needed. | — | — | — | — |

## Sub-check (§S3 / §S9 / §S15 / §S16 / §S17)

**A.1 §S3 错误吞没**:
  - violations: not present (the `!ok` early-return is documented benign-race contract per `subagent.md` §8.5, not silent failure)

**A.2 §S9 detached ctx 终态写**:
  - terminal-state writes identified: none
  - 各自 ctx 来源: N/A — Cancel does not write DB / does not emit; pure in-memory map lookup + context-cancel invocation
  - violations: N/A: file does no terminal writes (any DB / emit consequences flow back through the cancelled goroutine's WriteFinalize path in host.go, audited there)

**A.3 §S15 ID 生成**:
  - ID generation calls: none
  - violations: N/A: file generates no business IDs

**A.4 §S16 错误 wrap 格式**:
  - violations: not present (only `return nil` paths)

**A.5 §S17 sentinel 登记 errmap**:
  - sentinels defined: none
  - 已登记 errmap: N/A
  - missing: N/A: file defines no sentinels

## Notes

- Cancel's "no-op when not found" contract is intentionally permissive. A stricter alternative would return `subagentdomain.ErrRunNotFound` so handlers can 404 on cancelled-twice — but `subagent.md` §11 documents that there's no HTTP `POST /subagent-runs/{id}:cancel` endpoint in V1 (only internal call from chat.host.go's parent-cancel cascade), so 404 vs 200 is moot. The internal caller (chat) doesn't care about not-found-ness either.
- The `_ context.Context` parameter is API-shape padding — `Cancel` was likely designed to accept ctx for future tracing/logging hooks but doesn't currently use it. §S3 doesn't apply (no error to discard, just an unused argument).
- File length 28 LOC and single-function scope make this the smallest audit target in the package; no edge cases beyond the ones above.
