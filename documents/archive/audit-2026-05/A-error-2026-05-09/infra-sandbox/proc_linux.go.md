# audit: backend/internal/infra/sandbox/proc_linux.go

LOC: 56
Read: full file (lines 1-56)

## Trace

| site# | file:line | snippet | category | classification | reasoning | severity | user_impact | suggested_fix | status |
|---|---|---|---|---|---|---|---|---|---|
| 1 | proc_linux.go:36-41 | `func setupProcessGroup(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true, Pdeathsig: syscall.SIGTERM} }` | A.1 | OK | no error path; struct field assignment. PR_SET_PDEATHSIG layer documented at file header. | N-A | — | — | — |
| 2 | proc_linux.go:51-56 | `func killProcessGroup(cmd *exec.Cmd) error { if cmd.Process == nil { return nil }; return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }` | A.4 | OK | identical pattern to proc_darwin.go #2; bare passthrough of syscall err is conventional. Cross-platform shape consistent with darwin. | N-A | — | — | — |

## Sub-check

A.1 §S3 错误吞没:
  - violations: not present

A.2 §S9 detached ctx 终态写:
  - terminal-state writes identified: none
  - 各自 ctx 来源: N/A
  - violations: N/A — pure syscall wrapping

A.3 §S15 ID 生成:
  - ID generation calls: none
  - violations: N/A — no ID generation

A.4 §S16 错误 wrap 格式:
  - violations: not present (bare passthrough of syscall err at lowest layer)

A.5 §S17 sentinel 登记 errmap:
  - sentinels defined: none
  - 已登记 errmap: N/A
  - missing: N/A — file defines no sentinels
