# audit: backend/internal/infra/sandbox/python.go

LOC: 166
Read: full file (lines 1-166)

## Trace

| site# | file:line | snippet | category | classification | reasoning | severity | user_impact | suggested_fix | status |
|---|---|---|---|---|---|---|---|---|---|
| 1 | python.go:71-73 | `if _, err := os.Stat(venvDir); err == nil { return nil }` | A.1 | OK | idempotency check; err checked via guard | N-A | — | — | — |
| 2 | python.go:74-76 | `if err := os.MkdirAll(envPath, 0o755); err != nil { return fmt.Errorf("sandbox.PythonEnvManager.CreateEnv: mkdir env: %w", err) }` | A.4 | OK | §S16 canonical | N-A | — | — | — |
| 3 | python.go:77-80 | `uvBin, err := p.tools.EnsureTool(ctx, "uv", ""); if err != nil { return fmt.Errorf("sandbox.PythonEnvManager.CreateEnv: locate uv: %w", err) }` | A.4 | OK | §S16 canonical | N-A | — | — | — |
| 4 | python.go:81-85 | `cmd := exec.CommandContext(ctx, uvBin, "venv", ...); if out, err := cmd.CombinedOutput(); err != nil { return fmt.Errorf("sandbox.PythonEnvManager.CreateEnv %s: %w: %v (uv output: %s)", venvDir, sandboxdomain.ErrEnvCreateFailed, err, ...) }` | **A.4** | **VIOLATION** | **§S16: `%w: %v`** — sentinel ErrEnvCreateFailed wrapped with %w but the underlying *exec.ExitError is %v. Same defect class as exec_helper.go:#4 + mise.go:#19/#20. errors.Is(err, ErrEnvCreateFailed) works ✓ but errors.Is(err, &exec.ExitError{}) fails. | MED | callers wanting to programmatically discriminate "uv venv failed because Python missing" vs "uv venv failed because of disk full" can't via errors.Is — both look like ErrEnvCreateFailed | switch to multi-`%w`: `fmt.Errorf("...: %w: %w (uv output: %s)", sandboxdomain.ErrEnvCreateFailed, err, ...)` | FOUND |
| 5 | python.go:100-102 | `if len(deps) == 0 { return nil }` | A.1 | OK | early exit on empty input | N-A | — | — | — |
| 6 | python.go:103-106 | `uvBin, err := p.tools.EnsureTool(ctx, "uv", ""); if err != nil { return fmt.Errorf("sandbox.PythonEnvManager.InstallDeps: locate uv: %w", err) }` | A.4 | OK | §S16 canonical | N-A | — | — | — |
| 7 | python.go:111-114 | `return RunWithStderrCapture(cmd, stream, sandboxdomain.ErrDepInstallFailed, fmt.Sprintf("sandbox.PythonEnvManager.InstallDeps %v", deps))` | A.4 | OK | passes canonical msgPrefix; sentinel registered | N-A | — | — | — |
| 8 | python.go:122-124 | `func InstallExtras(...) error { return nil }` | A.1 | OK | documented no-op (line 116-121 explains Python has no extras) | N-A | — | — | — |

## Sub-check

A.1 §S3 错误吞没:
  - violations: not present (all errors propagated; documented no-ops are explicit; uv CombinedOutput err handled with rich output context)

A.2 §S9 detached ctx 终态写:
  - terminal-state writes identified: none
  - 各自 ctx 来源: N/A
  - violations: N/A — file performs filesystem + uv exec; no DB writes

A.3 §S15 ID 生成:
  - ID generation calls: none
  - violations: N/A — no business ID generation

A.4 §S16 错误 wrap 格式:
  - violations: site #4 (**MED** — `%w: %v` breaks underlying ExitError chain in CreateEnv); mirrors exec_helper.go:#4 + mise.go:#19/#20

A.5 §S17 sentinel 登记 errmap:
  - sentinels defined: none (consumes sandboxdomain.ErrEnvCreateFailed + ErrDepInstallFailed via wraps)
  - 已登记 errmap: ErrEnvCreateFailed (errmap.go:104) + ErrDepInstallFailed (errmap.go:105) — both registered ✓
  - missing: N/A — file defines no new sentinels
