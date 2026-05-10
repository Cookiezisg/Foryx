# audit: backend/internal/app/tool/shell/manager.go

LOC: 311
Read: full file (lines 1-311)

## Trace

| site# | file:line | snippet | category | classification | reasoning | severity | user_impact | suggested_fix | status |
|---|---|---|---|---|---|---|---|---|---|
| 1 | manager.go:67 | `ErrProcessNotFound = errors.New("background shell process not found")` | A.5 | OK | Package sentinel for Get-by-ID misses; consumed by Get → BashOutput.Execute / KillShell.Execute → both convert to LLM-friendly tool_result string per §S18. Never reaches errmap. | N-A | — | — | — |
| 2 | manager.go:102-116 | appendOutput ring-buffer overflow handling | A.1 | OK | pure mutation, no error returns | N-A | — | — | — |
| 3 | manager.go:124-130 | drainNew snapshot | A.1 | OK | pure read, no error returns | N-A | — | — | — |
| 4 | manager.go:136-154 | markFinished + markErrored | A.1 | OK | both store status under mu; markErrored captures launch err for later /dev endpoint dump | N-A | — | — | — |
| 5 | manager.go:177-180 | `if p.ID == "" { p.ID = idgenpkg.New("bsh") }` | A.3 | OK | §S15 canonical: idgenpkg.New("bsh") — "bsh" prefix matches CLAUDE.md §S15 spec list ("bsh_ Bash 后台 shell 进程"). idgenpkg internal panic-on-rand-fail invariant preserved. | N-A | — | — | — |
| 6 | manager.go:195-203 | `Get(id) → ErrProcessNotFound` | A.4/A.5 | OK | direct sentinel return — innermost layer | N-A | — | — | — |
| 7 | manager.go:255-257 | `if p.launchErr != nil { s.LaunchErr = p.launchErr.Error() }` | A.1 | OK | string dump for /dev/bash-processes inspection endpoint — informational only, not error propagation | N-A | — | — | — |
| 8 | manager.go:298-311 | `Stop(): for _, p := range procs { if p.Cmd == nil || p.Cmd.Process == nil { continue }; _ = p.Cmd.Process.Kill() }` | A.1 | OK | §S3 documented carve-out — comment lines 292-297 explicitly: "Best-effort: failures are swallowed — the OS will reap orphans". Backend-shutdown cleanup path matches §S3 example carve-out for "panic-path / cleanup-on-shutdown". The `_ =` lacks an inline comment per spec literal but the function-level doc comment is comprehensive. | LOW | minor — orphan processes get OS-reaped at parent exit; no user-visible loss | optional: add inline `// _ = err — best-effort kill on shutdown; OS reaps orphans` for spec-literal compliance. Or accept as documented at function level. | FOUND |

## Sub-check

A.1 §S3 错误吞没:
  - violations: not present (site #8 EDGE LOW — documented best-effort with rationale; ritual inline comment missing)

A.2 §S9 detached ctx 终态写:
  - terminal-state writes identified: none (manager.go is in-memory only — backend-process-scoped registry per file header lines 6-9; no DB writes at all)
  - ctx 来源: N/A — no ctx parameters
  - violations: N/A

A.3 §S15 ID 生成:
  - ID generation calls: site #5 — `idgenpkg.New("bsh")` per §S15 canonical (bsh prefix in spec list)
  - violations: not present

A.4 §S16 错误 wrap 格式:
  - violations: not present — file has no fmt.Errorf calls at all (all error paths return sentinel directly or are documented best-effort)

A.5 §S17 sentinel 登记 errmap:
  - sentinels defined: ErrProcessNotFound (line 67)
  - 已登记 errmap: N/A
  - missing: N/A — sentinel consumed via Get inside BashOutput/KillShell Execute methods; converted to LLM-friendly tool_result string per §S18 Tool framework convention. Never reaches errmap.
