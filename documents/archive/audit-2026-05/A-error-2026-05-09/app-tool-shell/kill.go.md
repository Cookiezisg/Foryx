# audit: backend/internal/app/tool/shell/kill.go

LOC: 110
Read: full file (lines 1-110)

## Trace

| site# | file:line | snippet | category | classification | reasoning | severity | user_impact | suggested_fix | status |
|---|---|---|---|---|---|---|---|---|---|
| 1 | kill.go:61-63 | `if err := json.Unmarshal(args, &a); err != nil { return fmt.Errorf("KillShell.ValidateInput: %w", err) }` | A.4 | OK | §S16 canonical | N-A | — | — | — |
| 2 | kill.go:64-66 | `if strings.TrimSpace(a.ShellID) == "" { return errors.New("shell_id is required") }` | A.4/A.5 | EDGE | §S16: inline `errors.New(...)` rather than a package-level `var Err...`. Functionally OK for ValidateInput context (§S18 framework converts to tool_result), but inconsistent with bash.go's ErrEmptyCommand + output.go's ErrEmptyBashID which use vars. errors.Is comparison won't match across calls (each errors.New produces a distinct identity). | LOW | minor — internal-only; never reaches errmap; only impact is style consistency | promote to package-level `var ErrEmptyShellID = errors.New("shell_id is required")` matching the pattern in bash.go / output.go | FOUND |
| 3 | kill.go:82-84 | `if err := json.Unmarshal([]byte(argsJSON), &args); err != nil { return "", fmt.Errorf("KillShell.Execute: %w", err) }` | A.4 | OK | §S16 canonical | N-A | — | — | — |
| 4 | kill.go:86-89 | `proc, err := t.mgr.Get(args.ShellID); if err != nil { return fmt.Sprintf("Background shell process not found: %s", args.ShellID), nil }` | A.1/A.4 | OK | ErrProcessNotFound → friendly tool_result string. §S18 carve-out + idempotent design per file header lines 1-8. | N-A | — | — | — |
| 5 | kill.go:92-99 | `if proc.Cmd != nil && proc.Cmd.Process != nil { if err := proc.Cmd.Process.Kill(); err == nil { wasRunning = true } }` | A.1 | OK | §S3 documented carve-out — comment lines 93-95 explicit: "Best-effort; kill on already-exited proc returns ESRCH which we treat as 'already done' rather than user-facing error." Caller-side state (wasRunning) tracks whether the kill was effective; no silent failure. | N-A | — | — | — |
| 6 | kill.go:100 | `t.mgr.Remove(args.ShellID)` | A.1 | OK | Remove returns no error (in-memory map delete); idempotent | N-A | — | — | — |

## Sub-check

A.1 §S3 错误吞没:
  - violations: not present

A.2 §S9 detached ctx 终态写:
  - terminal-state writes identified: none (no DB writes; in-memory registry mutation only)
  - ctx 来源: ctx unused (Execute signature has _, no propagation needed for in-memory ops)
  - violations: N/A

A.3 §S15 ID 生成:
  - ID generation calls: none (consumes IDs only)
  - violations: N/A

A.4 §S16 错误 wrap 格式:
  - violations: site #2 (LOW EDGE — inline errors.New rather than var; consistency-only concern, no functional gap)

A.5 §S17 sentinel 登记 errmap:
  - sentinels defined: none defined here (the ValidateInput inline errors.New at site #2 isn't a true package-level sentinel)
  - 已登记 errmap: N/A
  - missing: N/A — file defines no package-level sentinels; consumes ErrProcessNotFound from manager.go
