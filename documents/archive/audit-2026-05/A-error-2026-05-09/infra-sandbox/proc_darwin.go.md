# audit: backend/internal/infra/sandbox/proc_darwin.go

LOC: 54
Read: full file (lines 1-54)

## Trace

| site# | file:line | snippet | category | classification | reasoning | severity | user_impact | suggested_fix | status |
|---|---|---|---|---|---|---|---|---|---|
| 1 | proc_darwin.go:40-42 | `func setupProcessGroup(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true} }` | A.1 | OK | no error path; setting struct field | N-A | — | — | — |
| 2 | proc_darwin.go:49-54 | `func killProcessGroup(cmd *exec.Cmd) error { if cmd.Process == nil { return nil }; return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }` | A.4 | OK | bare return of stdlib syscall err — caller (cmd.Cancel callback) inspects directly. Not a §S16 violation: passing through stdlib err at the lowest level is conventional Go. Idempotent guard for Process==nil prevents nil deref. | N-A | — | — | — |

## Sub-check

A.1 §S3 错误吞没:
  - violations: not present (file is 2 simple platform-specific functions; no error suppression)

A.2 §S9 detached ctx 终态写:
  - terminal-state writes identified: none
  - 各自 ctx 来源: N/A
  - violations: N/A — no DB writes; pure syscall wrapping

A.3 §S15 ID 生成:
  - ID generation calls: none
  - violations: N/A — no ID generation (PIDs are OS-assigned)

A.4 §S16 错误 wrap 格式:
  - violations: not present (bare passthrough of syscall err at the lowest layer is the conventional pattern; caller wraps if needed)

A.5 §S17 sentinel 登记 errmap:
  - sentinels defined: none
  - 已登记 errmap: N/A
  - missing: N/A — file defines no sentinels
