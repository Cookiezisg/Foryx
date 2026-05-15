# audit: backend/internal/infra/sandbox/node.go

LOC: 137
Read: full file (lines 1-137)

## Trace

| site# | file:line | snippet | category | classification | reasoning | severity | user_impact | suggested_fix | status |
|---|---|---|---|---|---|---|---|---|---|
| 1 | node.go:54-57 | `if _, err := os.Stat(pkgJSON); err == nil { return nil }` | A.1 | OK | Stat err is *checked* — `err == nil` guard means "file exists, idempotent skip"; failure (file missing) falls through to create path. Idempotency design. | N-A | — | — | — |
| 2 | node.go:58-60 | `if err := os.MkdirAll(envPath, 0o755); err != nil { return fmt.Errorf("sandbox.NodeEnvManager.CreateEnv: mkdir env: %w", err) }` | A.4 | OK | §S16 canonical | N-A | — | — | — |
| 3 | node.go:72-75 | `data, err := json.MarshalIndent(manifest, "", "  "); if err != nil { return fmt.Errorf("sandbox.NodeEnvManager.CreateEnv: marshal pkg: %w", err) }` | A.4 | OK | §S16 canonical (note: marshal of {string: string/bool} basic-type map cannot fail in practice; wrap is defensive) | N-A | — | — | — |
| 4 | node.go:76-78 | `if err := os.WriteFile(pkgJSON, data, 0o644); err != nil { return fmt.Errorf("sandbox.NodeEnvManager.CreateEnv: write pkg: %w (env: %w)", err, sandboxdomain.ErrEnvCreateFailed) }` | **A.4** | **VIOLATION** | **§S16: dual %w but order is wrong** — the format expects callers to `errors.Is(err, sandboxdomain.ErrEnvCreateFailed)` to discriminate, but the **first** %w is the OS error and the second is the sentinel. Per Go's errors.Is semantics, multi-%w *both* unwrap correctly so `errors.Is(err, ErrEnvCreateFailed)` does work. **However**, the message form `"write pkg: <fserr> (env: <sentinel>)"` reads backwards — sentinel should typically be primary. Cosmetic ordering issue, not a chain breakage. | LOW | error message reads "write pkg: permission denied (env: sandbox: env create failed)" — readable but unconventional. errors.Is still works for both. | swap order: `fmt.Errorf("sandbox.NodeEnvManager.CreateEnv: write pkg: %w: %w", sandboxdomain.ErrEnvCreateFailed, err)` so the sentinel-first message matches mcp install.go:#5 fix pattern (post-505d6e3) | FOUND |
| 5 | node.go:91-93 | `if len(deps) == 0 { return nil }` | A.1 | OK | early exit on empty input — not error suppression | N-A | — | — | — |
| 6 | node.go:102-104 | `return RunWithStderrCapture(cmd, stream, sandboxdomain.ErrDepInstallFailed, fmt.Sprintf("sandbox.NodeEnvManager.InstallDeps %v", deps))` | A.4 | OK | passes canonical msgPrefix to RunWithStderrCapture; sentinel ErrDepInstallFailed registered errmap.go:105. The MED `%v` issue inside RunWithStderrCapture (exec_helper.go #4) is the helper's responsibility, not this caller. | N-A | — | — | — |
| 7 | node.go:115-117 | `func (n *NodeEnvManager) InstallExtras(...) error { return nil }` | A.1 | OK | documented no-op; Playwright extras handled elsewhere. Comment block lines 107-114 explains why this is empty. | N-A | — | — | — |

## Sub-check

A.1 §S3 错误吞没:
  - violations: not present (all errors propagated; documented no-ops are explicit)

A.2 §S9 detached ctx 终态写:
  - terminal-state writes identified: none
  - 各自 ctx 来源: N/A
  - violations: N/A — file performs filesystem + exec; no DB writes

A.3 §S15 ID 生成:
  - ID generation calls: none (`name` field at line 68 derived from envPath basename, not random)
  - violations: N/A — no business ID generation

A.4 §S16 错误 wrap 格式:
  - violations: site #4 (LOW — dual %w but reversed order; functionally correct for errors.Is, message readability minor)

A.5 §S17 sentinel 登记 errmap:
  - sentinels defined: none (consumes sandboxdomain.ErrEnvCreateFailed + ErrDepInstallFailed via wraps)
  - 已登记 errmap: ErrEnvCreateFailed (errmap.go:104) + ErrDepInstallFailed (errmap.go:105) — both registered ✓
  - missing: N/A — file defines no new sentinels
