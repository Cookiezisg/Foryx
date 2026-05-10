# audit: backend/internal/app/tool/search/grep_rg.go

LOC: 171
Read: full file (lines 1-171)

## Trace

| site# | file:line | snippet | category | classification | reasoning | severity | user_impact | suggested_fix | status |
|---|---|---|---|---|---|---|---|---|---|
| 1 | grep_rg.go:42 | `out, err := cmd.Output()` then exit-code branch | A.4 | OK | `cmd.Output()` err handled below at line 50; exit-code-1 (no matches) deliberately treated as not-an-error. Doc comment lines 44-49 explicitly documents the rg exit-code semantics. Compliant per §S3 documented-intent carve-out. | N-A | — | — | — |
| 2 | grep_rg.go:50-58 | `if err != nil { var ee *exec.ExitError; if errors.As(err, &ee) { if ee.ExitCode() == 1 { return noMatchesMessage(args), nil }; return "", fmt.Errorf("Grep.execRg: %w (stderr: %s)", err, stderrSnippet(ee.Stderr)) }; return "", fmt.Errorf("Grep.execRg: %w", err) }` | A.4 | EDGE | §S16: prefix `Grep.execRg:` not canonical `searchtool.Grep.execRg:`; %w preserves *exec.ExitError, stderr snippet appended for debug. **Note**: this error is what grep.go:#9 silently swallows on the rg→stdlib fallback path — wrap is correct here, the §S3 violation is at the caller. | LOW | grep traceability | tighten prefix to `searchtool.Grep.execRg:` | FOUND |
| 3 | grep_rg.go:41 | `cmd := exec.CommandContext(ctx, t.rgPath, cmdArgs...) //nolint:gosec // rgPath came from exec.LookPath; args are constructed from validated grepArgs.` | A.4 | OK | nolint comment justifies why gosec exec-input rule is suppressed; both inputs are project-controlled (LookPath result + validated args). Compliant. | N-A | — | — | — |

## Sub-check

A.1 §S3 错误吞没:
  - violations: not present in this file (the silent-fallback violation that hides this file's errors is in grep.go:#9 — caller-side)

A.2 §S9 detached ctx 终态写:
  - terminal-state writes identified: none
  - 各自 ctx 来源: N/A
  - violations: N/A — no DB writes; this is a shell-out helper

A.3 §S15 ID 生成:
  - ID generation calls: none
  - violations: N/A

A.4 §S16 错误 wrap 格式:
  - violations: site #2 (LOW — prefix style)

A.5 §S17 sentinel 登记 errmap:
  - sentinels defined: none
  - 已登记 errmap: N/A
  - missing: N/A — file defines no sentinels
