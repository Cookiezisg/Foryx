# audit: backend/internal/app/tool/shell/shell.go

LOC: 79
Read: full file (lines 1-79)

## Trace

| site# | file:line | snippet | category | classification | reasoning | severity | user_impact | suggested_fix | status |
|---|---|---|---|---|---|---|---|---|---|
| 1 | shell.go:69-79 | `func NewShellTools(sandbox *sandboxapp.Service) *ShellTools { mgr := NewProcessManager(); return &ShellTools{Manager: mgr, Tools: []toolapp.Tool{&Bash{...}, &BashOutput{...}, &KillShell{...}}} }` | A.1 | OK | factory; no error returns. nil sandbox is documented OK per comments lines 58-68 — Bash will skip auto-route for nil sandbox (handled in bash.go::maybeAutoRoute site #8). | N-A | — | — | — |

## Sub-check

A.1 §S3 错误吞没:
  - violations: not present

A.2 §S9 detached ctx 终态写:
  - terminal-state writes identified: none
  - violations: N/A — pure factory

A.3 §S15 ID 生成:
  - ID generation calls: none (NewProcessManager doesn't generate IDs at construction)
  - violations: N/A

A.4 §S16 错误 wrap 格式:
  - violations: not present (no fmt.Errorf calls)

A.5 §S17 sentinel 登记 errmap:
  - sentinels defined: none
  - missing: N/A — package factory file
