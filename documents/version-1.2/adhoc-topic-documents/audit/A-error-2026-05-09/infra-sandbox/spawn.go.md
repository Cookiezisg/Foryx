# audit: backend/internal/infra/sandbox/spawn.go

LOC: 221
Read: full file (lines 1-221)

## Trace

| site# | file:line | snippet | category | classification | reasoning | severity | user_impact | suggested_fix | status |
|---|---|---|---|---|---|---|---|---|---|
| 1 | spawn.go:102-105 | `if runErr == nil { result.Ok = true; return result, nil }` | A.1 | OK | success path; not an error case | N-A | — | — | — |
| 2 | spawn.go:109-114 | `var exitErr *exec.ExitError; if errors.As(runErr, &exitErr) { result.Ok = false; result.ExitCode = exitErr.ExitCode(); return result, nil }` | A.1 | OK | **deliberately downgrades runErr to non-error path** for "subprocess ran but exited non-zero" — documented intent at line 107-108. Caller (LLM tool runner) treats Ok=false as a tool result not Go error. Per §S3 "judgement key": this is NOT user-visible state loss, it IS the user-visible state (exit code). Compliant with documented carve-out. | N-A | — | — | — |
| 3 | spawn.go:121-123 | `if errors.Is(runErr, context.DeadlineExceeded) { return result, fmt.Errorf("sandbox.SpawnOnce: %w", sandboxdomain.ErrSpawnTimeout) }` | A.4 | OK | §S16 canonical; sentinel ErrSpawnTimeout registered errmap.go:107 → 504 | N-A | — | — | — |
| 4 | spawn.go:124 | `return result, fmt.Errorf("sandbox.SpawnOnce: %w (cause: %w)", sandboxdomain.ErrSpawnFailed, runErr)` | A.4 | OK | **canonical multi-`%w`** (Go 1.20+) — both sentinel ErrSpawnFailed AND underlying runErr preserved in chain; errors.Is can match both. **Reference impl** for the multi-%w fix needed in mise.go:#19/#20 + python.go:#4 + exec_helper.go:#4. | N-A | — | — | — |
| 5 | spawn.go:144-147 | `stdin, err := cmd.StdinPipe(); if err != nil { return nil, fmt.Errorf("sandbox.SpawnLongLived: stdin pipe: %w (spawn: %w)", err, sandboxdomain.ErrSpawnFailed) }` | A.4 | OK | multi-`%w` canonical — both stdin pipe err AND ErrSpawnFailed preserved. Reverse order from #4 (err first, sentinel second) but errors.Is matches either way. Slight inconsistency with #4 in ordering; not a violation per Go semantics. | N-A | — | — | — |
| 6 | spawn.go:148-152 | `stdout, err := cmd.StdoutPipe(); if err != nil { _ = stdin.Close(); return nil, fmt.Errorf("sandbox.SpawnLongLived: stdout pipe: %w (spawn: %w)", err, sandboxdomain.ErrSpawnFailed) }` | A.1/A.4 | EDGE | **§S3**: `_ = stdin.Close()` discards Close error in the cleanup-after-failure branch. **Functional**: cleanup of pipe whose creation already failed; Close failure here means pipe was probably never fully open. Per §S3 spec carve-out: panic-path cleanup is OK. **However**: spec says inline comment required. Wrap is canonical multi-%w. | LOW | minor — leaked pipe FD if Close fails (rare); next process exit cleans up | add inline comment: `_ = stdin.Close() // best-effort cleanup; pipe creation failed downstream is the actionable error` | FOUND |
| 7 | spawn.go:153-158 | `stderrR, err := cmd.StderrPipe(); if err != nil { _ = stdin.Close(); _ = stdout.Close(); return nil, fmt.Errorf("sandbox.SpawnLongLived: stderr pipe: %w (spawn: %w)", err, sandboxdomain.ErrSpawnFailed) }` | A.1/A.4 | EDGE | same dual `_ = .Close()` discards pattern as #6 | LOW | same as #6 | same as #6 | FOUND |
| 8 | spawn.go:163-168 | `if err := cmd.Start(); err != nil { _ = stdin.Close(); _ = stdout.Close(); _ = stderrR.Close(); return nil, fmt.Errorf("sandbox.SpawnLongLived: start: %w (spawn: %w)", err, sandboxdomain.ErrSpawnFailed) }` | A.1/A.4 | EDGE | same triple `_ = .Close()` discards pattern; same family as #6/#7 | LOW | same as #6 | same as #6 | FOUND |
| 9 | spawn.go:203 | `func (h *longLivedHandle) Wait() error { return h.cmd.Wait() }` | A.4 | OK | bare passthrough — caller receives exec.Cmd.Wait()'s error directly which is *exec.ExitError or other stdlib err. Not wrapping at this level is correct for a thin handle wrapper; caller gets the chain to inspect. | N-A | — | — | — |
| 10 | spawn.go:211 | `func (h *longLivedHandle) Kill() error { return killProcessGroup(h.cmd) }` | A.4 | OK | bare passthrough; killProcessGroup is platform-specific (proc_*.go) | N-A | — | — | — |

## Sub-check

A.1 §S3 错误吞没:
  - violations: sites #6, #7, #8 (LOW EDGE — `_ = .Close()` cleanup-after-pipe-failure without inline comment; documented carve-out per §S3 panic-path cleanup but missing the comment ritual)

A.2 §S9 detached ctx 终态写:
  - terminal-state writes identified: none
  - 各自 ctx 来源: N/A
  - violations: N/A — file is process-spawn helper; no DB writes

A.3 §S15 ID 生成:
  - ID generation calls: none
  - violations: N/A — no business ID generation (PIDs are OS-assigned, not application IDs)

A.4 §S16 错误 wrap 格式:
  - violations: not present — site #4 + #5/#6/#7/#8 are **canonical multi-%w examples** that should be referenced when fixing the `%v` defects elsewhere (mise.go:#19/#20, python.go:#4, exec_helper.go:#4)

A.5 §S17 sentinel 登记 errmap:
  - sentinels defined: none (consumes sandboxdomain.ErrSpawnFailed + ErrSpawnTimeout via wraps)
  - 已登记 errmap: ErrSpawnFailed (errmap.go:106) + ErrSpawnTimeout (errmap.go:107) — both registered ✓
  - missing: N/A — file defines no new sentinels
