# audit: backend/internal/infra/sandbox/exec_helper.go

LOC: 83
Read: full file (lines 1-83)

## Trace

| site# | file:line | snippet | category | classification | reasoning | severity | user_impact | suggested_fix | status |
|---|---|---|---|---|---|---|---|---|---|
| 1 | exec_helper.go:46-49 | `stderrPipe, err := cmd.StderrPipe(); if err != nil { return fmt.Errorf("%s: stderr pipe: %w", msgPrefix, err) }` | A.4 | OK | §S16: msgPrefix from caller is canonical pkg.Method (e.g. `sandbox.NodeEnvManager.InstallDeps lodash`); `%w` ✓; sentinel chain preserved through stdlib exec.* err. Per file-header comment, msgPrefix locates the call site. | N-A | — | — | — |
| 2 | exec_helper.go:50-52 | `if err := cmd.Start(); err != nil { return fmt.Errorf("%s: start: %w", msgPrefix, err) }` | A.4 | OK | §S16: same as #1; canonical wrap | N-A | — | — | — |
| 3 | exec_helper.go:55-65 | `scanner := bufio.NewScanner(stderrPipe); for scanner.Scan() { ... }` | A.1 | EDGE | §S3: scanner.Err() not checked after the loop — if the read failed mid-scan (closed pipe, partial line), the loop exits silently and we go straight to cmd.Wait. **However**: cmd.Wait will surface the underlying issue (process likely failed too if pipe broke), and tail buffer holds whatever was captured. This is a documented "buffer-and-continue" pattern. | LOW | minor — if scanner errors on partial buffer (e.g. line > 64KB default), tail still captures up to that point + cmd.Wait's err is the authoritative signal. The unchecked scanner.Err() means a malformed-but-non-fatal stderr line could silently truncate the captured tail without an audit trail. | optional: `if serr := scanner.Err(); serr != nil { tail = append(tail, []byte(fmt.Sprintf("\n[scanner error: %v]\n", serr))...) }` to preserve audit trail in the captured tail | FOUND |
| 4 | exec_helper.go:67-74 | `if err := cmd.Wait(); err != nil { snippet := strings.TrimSpace(string(tail)); ... return fmt.Errorf("%s: %w: %v: %s", msgPrefix, sentinel, err, snippet) }` | **A.4** | **VIOLATION** | **§S16: `%w: %v: %s`** — the sentinel is wrapped with %w (so errors.Is(err, ErrRuntimeInstallFailed) works), but the **original cmd.Wait err is rendered with %v**, breaking the chain to the underlying *exec.ExitError. Same defect class as mcp install.go:#5 (resolved 505d6e3 with multi-%w) and forge audit's `%w: %v` MEDs. Caller wanting `errors.Is(err, &exec.ExitError{})` or unwrap-to-ExitError loses the chain. | MED | callers (Node/Python/Mise installers) cannot discriminate "process exited 1 vs exec failed to start" via errors.Is — both look like `ErrRuntimeInstallFailed` only. Diagnostic info is in the message but not programmatic. Same impact as mcp install.go pre-fix. | switch to multi-`%w` (Go 1.20+): `return fmt.Errorf("%s: %w: %w: %s", msgPrefix, sentinel, err, snippet)` — preserves both sentinel and underlying ExitError | FOUND |

## Sub-check

A.1 §S3 错误吞没:
  - violations: site #3 (LOW EDGE — scanner.Err() unchecked after Scan loop)

A.2 §S9 detached ctx 终态写:
  - terminal-state writes identified: none
  - 各自 ctx 来源: N/A
  - violations: N/A — helper just runs an exec.Cmd; no DB access, no ctx-driven writes

A.3 §S15 ID 生成:
  - ID generation calls: none
  - violations: N/A — no ID generation

A.4 §S16 错误 wrap 格式:
  - violations: site #4 (MED — `%w: %v: %s` breaks underlying ExitError chain; same defect as mcp install.go:#5 fixed in 505d6e3)

A.5 §S17 sentinel 登记 errmap:
  - sentinels defined: none (file has compile-time check for sandboxdomain.ErrRuntimeInstallFailed but does not define new sentinels)
  - 已登记 errmap: N/A — file just consumes sentinels from sandboxdomain
  - missing: N/A — file defines no sentinels of its own
