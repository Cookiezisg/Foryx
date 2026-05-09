# audit: backend/internal/app/sandbox/restore.go

LOC: 118
Read: full file (lines 1-118)

## Trace

| site# | file:line | snippet | category | classification | reasoning | severity | user_impact | suggested_fix | status |
|---|---|---|---|---|---|---|---|---|---|
| 1 | restore.go:51 | `func (s *Service) RestoreOrCleanupOnBoot(ctx context.Context)` (no error return) | A.1 | OK | Function returns nothing by design — boot-time cleanup that **must not** fail boot. File-header rationale lines 27-30 explicit: "boot must proceed even if cleanup partial". Internally all errors fan out to `s.log.Warn` per §S10 "异步或 fire-and-forget 必须打". Documented intent. | N-A | — | — | — |
| 2 | restore.go:52-57 | `envs, err := s.repo.ListEnvsWithRunningPID(ctx); if err != nil { s.log.Warn("sandbox boot scan: list envs with running pid failed (skipping cleanup)", zap.Error(err)); return }` | A.1 | OK | §S3 documented soft-fail: log + return. Boot continues with partial scan rather than aborting entire startup. Logged at WARN per §S10 (fire-and-forget = must log). Audit trail OK. | N-A | — | — | — |
| 3 | restore.go:75-79 | `if err := s.repo.ClearEnvRunningPID(ctx, e.ID); err != nil { s.log.Warn("sandbox boot scan: clear running_pid failed", zap.String("env_id", e.ID), zap.Error(err)) }` | A.1 | OK | Per-env clear failure logged with env_id; loop continues to next env. §S10 fire-and-forget audit. The next boot scan will retry (idempotent). | N-A | — | — | — |
| 4 | restore.go:96-98 | `if pid <= 0 { return false }` | A.1 | OK | Validation guard: pid <= 0 means no PID stored (column was 0/NULL). Not an error path; boolean return represents "no kill attempted". Documented boolean semantics. | N-A | — | — | — |
| 5 | restore.go:99-102 | `p, err := os.FindProcess(pid); if err != nil { return false }` | A.1 | EDGE | §S3: stdlib err discarded by returning bool false. **Justification valid**: `os.FindProcess` on unix never errors per Go stdlib docs (always returns Process struct that may be invalid); on Windows it does error if process gone. Either way, "process gone" → can't kill → return false is semantically correct. **No log though**. Could add `s.log.Debug` for diagnostic but Info-level operator log noise vs LOW value tradeoff. | LOW | none in practice — Windows FindProcess err on dead pid is the documented "dead process" path | optional: `s.log.Debug("killIfAlive: FindProcess failed (process likely dead)", zap.Int("pid", pid), zap.Error(err))` for debugging. WAIVE-eligible since loop already logged kill outcome at Info. | FOUND |
| 6 | restore.go:103-108 | `if runtime.GOOS != "windows" { if err := p.Signal(syscall.Signal(0)); err != nil { return false } }` | A.1 | OK | Signal(0) is a probe; err means "process gone" — exactly the documented intent (lines 104-105). Returning false is correct semantic. No data loss. | N-A | — | — | — |
| 7 | restore.go:112-116 | `if err := p.Kill(); err != nil { // Race: process exited between probe and kill. Treat as not-killed. return false }` | A.1 | OK | Race-condition guard with explicit inline comment lines 113-114 documenting why err is dropped. §S3 example pattern: comment explains why. | N-A | — | — | — |

## Sub-check

A.1 §S3 错误吞没:
  - violations: not present (site #5 LOW EDGE — discoverable err on Windows FindProcess gets bool-false return; documented behavior, optional Debug log)

A.2 §S9 detached ctx 终态写:
  - terminal-state writes identified: site #3 (s.repo.ClearEnvRunningPID — clears manifest column)
  - 各自 ctx 来源: site #3 uses parameter `ctx` (passed from main.go boot path)
  - violations: N/A — boot-path ctx is `context.Background()` from main.go (not request-derived); clearing running_pid is best-effort cleanup not strictly required for correctness (next boot retries). Detached pattern not needed because there's no cancel risk in boot path. **Verified ctx source**: main.go calls `Service.RestoreOrCleanupOnBoot(ctx)` where ctx is context.Background-derived (not request ctx).

A.3 §S15 ID 生成:
  - ID generation calls: none
  - violations: N/A — file does not generate business IDs (consumes existing env IDs from manifest)

A.4 §S16 错误 wrap 格式:
  - violations: not present — no `fmt.Errorf` wrap calls in this file (errors are logged-and-swallowed per documented design, not propagated)

A.5 §S17 sentinel 登记 errmap:
  - sentinels defined: none
  - 已登记 errmap: N/A
  - missing: N/A — file defines no sentinels (consumed errors are stdlib + repo wrap chains)
