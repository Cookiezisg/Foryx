# Package audit summary: internal/infra/sandbox

## Spec understanding (Phase A — §S3 / §S9 / §S15 / §S16 / §S17)

- **§S3 错误不吞**: error suppression that hides user-visible failure / data loss / config drift is forbidden. `_ = err` requires inline justification (`// best-effort cleanup; ...` or similar). `defer X.Close()` on read-only resources or panic-path cleanup is allowed. The package's exec_helper.go file-header comment cites this rule explicitly — the helper exists *because* the prior D2 work violated it. Documented best-effort patterns (boot-time PR_SET_PDEATHSIG via setupProcessGroup, mise.toml hash file) require WARN logs which the file mostly provides.
- **§S9 detached ctx 终态写**: terminal-state DB writes must use `reqctxpkg.SetUserID(context.Background(), uid)`. **N/A for this package** — infra/sandbox is the lowest layer; it performs filesystem operations, exec.Cmd spawns, and platform-specific syscalls. All DB writes happen in the `app/sandbox` layer (already audited; FIXED 2026-05-09 e36f890 / 0d4a48e). This package's role is to return error chains accurately so the caller can persist correctly-formed sentinel-wrapped errors.
- **§S15 ID 生成**: business IDs flow through `idgenpkg.New(prefix)` at the app layer. **N/A** — infra/sandbox is below the ID-generation layer. The hash computation in `mise.go::ExtractMiseBinary` uses `sha256` for content-addressed integrity, which is correct (not a random ID).
- **§S16 错误 wrap 格式**: `fmt.Errorf("<pkg>.<Method>: %w", err)` canonical form. The package mostly complies for top-of-package functions (`sandbox.MiseInstaller.Install`, `sandbox.ExtractMiseBinary`, `sandbox.SpawnOnce`, etc.), with two systematic gaps: (a) descriptive prefixes vs canonical pkg.Method form for unexported helpers (`xattr -dr:`, `taskkill:`, `CreateJobObject:`); (b) **`%w: %v` defects in mise.go where + ListAvailable that break the underlying ExitError chain** (same defect as exec_helper.go pre-existing MED).
- **§S17 errmap 单一事实源**: every sentinel that can reach a handler must be in errmap.go. The package consumes 6 sandboxdomain sentinels (`ErrRuntimeInstallFailed`, `ErrEnvCreateFailed`, `ErrDepInstallFailed`, `ErrSpawnFailed`, `ErrSpawnTimeout`, plus newly-added `ErrInvalidOwnerID` / `ErrCmdRequired` from the e36f890/0d4a48e fixes). All 8 sandboxdomain sentinels registered errmap.go:101-110 ✓. **No sentinels are defined in this package itself** — by design (infra layer wraps app-layer sentinels).

## Files audited

| File | LOC | Sites | OK | POST-FIX | VIOLATION | EDGE |
|---|---|---|---|---|---|---|
| codesign.go | 101 | 5 | 2 | 0 | 0 | 3 |
| exec_helper.go | 83 | 4 | 2 | 0 | 1 | 1 |
| mise.go | 472 | 20 | 11 | 0 | 2 | 7 |
| node.go | 137 | 7 | 6 | 0 | 1 | 0 |
| python.go | 166 | 8 | 7 | 0 | 1 | 0 |
| spawn.go | 221 | 10 | 7 | 0 | 0 | 3 |
| proc_darwin.go | 54 | 2 | 2 | 0 | 0 | 0 |
| proc_linux.go | 56 | 2 | 2 | 0 | 0 | 0 |
| proc_windows.go | 150 | 6 | 2 | 0 | 0 | 4 |
| embed_mise.md (6 files) | 60 | 2 | 2 | 0 | 0 | 0 |
| **TOTAL** | **1500** | **66** | **43** | **0** | **5** | **18** |

## Severity breakdown (only VIOLATION + EDGE)

| Severity | Count | Sites | Status |
|---|---|---|---|
| HIGH | 0 | — | — |
| MED | 5 | exec_helper.go:#4 (`%w: %v: %s` breaks ExitError chain — most central since used by all installers); mise.go:#19 (where `%v`); mise.go:#20 (ListAvailable `%v`); python.go:#4 (CreateEnv `%w: %v`); node.go:#4 (CreateEnv dual-`%w` reversed-order — minor severity but consistent fix needed) | FOUND |
| LOW (§S16 prefix style) | 9 | codesign.go #1, #4, #5; mise.go #10, #16; proc_windows.go #2, #3, #4, #6 | FOUND |
| LOW (§S3 silent w/o comment) | 5 | mise.go #6 (`_ = os.Remove(tmp)` cleanup), mise.go #17 (`_ = os.ReadDir` error-msg construct), mise.go #18 (`_ = filepath.WalkDir` best-effort search); spawn.go #6, #7, #8 (`_ = .Close()` cleanup-after-pipe-fail × 3); proc_windows.go #3, #4 (`_ = CloseHandle` cleanup-after-Job-init-fail × 2). All are documented carve-outs (panic-path cleanup, best-effort search, error-message construction) but missing the inline `// _ = err — <reason>` ritual the spec calls for. | FOUND |
| LOW (§S3 scanner err) | 1 | exec_helper.go #3 (scanner.Err() unchecked after Scan loop) | FOUND |

## Cross-cutting

### Sentinel chain integrity (§S17)

All 8 sandboxdomain sentinels (errmap.go:101-110) verified consumed correctly:

| Sentinel | errmap.go line | Wrapped by infra/sandbox at |
|---|---|---|
| `ErrRuntimeNotSupported` | 101 | (consumed by app layer; not wrapped here) |
| `ErrRuntimeInstallFailed` | 102 | mise.go:67 (no embed binary case) — direct return; exec_helper.go:72 — passed in by RunWithStderrCapture sentinel param |
| `ErrEnvNotFound` | 103 | (app layer) |
| `ErrEnvCreateFailed` | 104 | python.go:84 (CreateEnv exec err); node.go:77 (CreateEnv WriteFile err) |
| `ErrDepInstallFailed` | 105 | python.go:112 (InstallDeps via RunWithStderrCapture); node.go:103 (InstallDeps via RunWithStderrCapture) |
| `ErrSpawnFailed` | 106 | spawn.go #4, #5/#6/#7/#8 (SpawnOnce + SpawnLongLived pipe-fail × 3 + start-fail) |
| `ErrSpawnTimeout` | 107 | spawn.go #3 (SpawnOnce DeadlineExceeded) |
| `ErrEnvInUse` | 108 | (app layer) |
| `ErrInvalidOwnerID` (new from e36f890) | 109 | (app layer; infra not directly involved) |
| `ErrCmdRequired` (new from 0d4a48e) | 110 | (app layer) |

**No missing registrations**. All sentinel chains preserved EXCEPT where `%v` truncates the underlying ExitError (exec_helper.go #4, mise.go #19/#20, python.go #4) — those preserve the *sandboxdomain* sentinel but lose the *exec.ExitError* layer that callers may want for retry-vs-surface decisions.

### Detached ctx coverage (§S9)

**N/A for this package** — infra/sandbox performs no DB writes. The terminal-state-write concern is fully owned by `app/sandbox.Service` (audited separately; FIXED 2026-05-10 e36f890 — `EnsureEnv ready` and `markEnvFailed` both switched to `context.Background()` per spawn.go:trackedHandle.unregister model pattern).

### Cross-platform consistency (proc_*.go)

| Platform | LOC | killProcessGroup mechanism | Error wrap style |
|---|---|---|---|
| darwin | 54 | `syscall.Kill(-pid, SIGKILL)` | bare passthrough (canonical) |
| linux | 56 | `syscall.Kill(-pid, SIGKILL)` + PR_SET_PDEATHSIG | bare passthrough (canonical) |
| windows | 150 | `taskkill /T /F /PID + Job Object kill-on-close` | wrapped with `taskkill: %w (output: %s)` |

The wrap-style divergence is **functionally appropriate**: unix `syscall.Kill` is a self-contained errno (bare passthrough conventional), Windows `taskkill` is exec-based and benefits from output capture in the wrap. The only finding is **descriptive Windows-API-name prefixes** vs canonical `<pkg>.<Method>:` form — LOW severity, doesn't break errors.Is.

### Install-path §S3 status (mise / npm / pip / uv)

| Installer | Path | §S3 status |
|---|---|---|
| mise (runtime install) | `mise.go::MiseInstaller.Install` → RunWithStderrCapture(ErrRuntimeInstallFailed) | ✓ stderr captured + sentinel wrapped; exec_helper.go #4 MED affects ExitError chain only |
| mise (binary extract) | `mise.go::ExtractMiseBinary` → in-process WriteFile + Rename + Codesign | ✓ all errors propagated with sentinel; idempotency preserved |
| npm (deps install) | `node.go::InstallDeps` → RunWithStderrCapture(ErrDepInstallFailed) | ✓ same as mise, MED in helper applies |
| uv (Python venv create) | `python.go::CreateEnv` → exec.CombinedOutput + manual wrap | ⚠ MED #4 — `%w: %v` breaks ExitError chain |
| uv (Python deps install) | `python.go::InstallDeps` → RunWithStderrCapture(ErrDepInstallFailed) | ✓ same as mise, MED in helper applies |

The single most central fix is **exec_helper.go #4** — switching `%w: %v: %s` to multi-`%w: %w: %s` (Go 1.20+) propagates correctly through Mise + Node + Python install paths simultaneously. spawn.go #4 already demonstrates the canonical multi-%w form.

### §S15 ID generation

**N/A** — infra/sandbox does not generate business IDs. The only "identifier-like" computation is `sha256.Sum256(miseBinary)` for content-addressed binary integrity, which is correct (not random; deterministic from content).

## Spot-check (random clean sites — verify mechanism not rubber-stamping)

Random seed: 6 sites picked from `OK` set across files:

1. **mise.go:#1** (line 65-68 ExtractMiseBinary len-zero check): verified — `fmt.Errorf("sandbox.ExtractMiseBinary: no mise binary embedded for %s/%s: %w", runtime.GOOS, runtime.GOARCH, sandboxdomain.ErrRuntimeInstallFailed)`. Canonical pkg.Method ✓ + %w with registered sentinel (errmap.go:102) ✓ + platform context for diagnosis.
2. **spawn.go:#4** (line 124 multi-%w wrap): verified — `fmt.Errorf("sandbox.SpawnOnce: %w (cause: %w)", sandboxdomain.ErrSpawnFailed, runErr)`. Both sentinel and underlying runErr preserved; `errors.Is(err, ErrSpawnFailed)` AND `errors.As(err, &exec.ExitError{})` both work. Reference impl for the multi-%w fix needed in 4 sites elsewhere.
3. **spawn.go:#2** (lines 109-114 SpawnOnce ExitError downgrade): verified — `errors.As(runErr, &exitErr)` + return `Ok=false` instead of Go error. **Documented intent** at lines 107-108 ("subprocess ran but failed — surface as Ok=false, not Go error") + caller (LLM tool runner) treats Ok=false as tool result. Per §S3 carve-out: this is *intended user-visible state*, not loss.
4. **proc_windows.go:#5** (line 120-130 setupProcessGroup `_ = EnsureMasterJob()`): verified — extensive inline comment lines 121-130 explicitly explains "Best-effort. If the job init failed... we still proceed — the spawn just won't get the catastrophic-cleanup safety net. Service.Shutdown() (Layer A) and boot-time PID scan (Layer B) still work." Compliance literal with §S3 spec example for `_ = err` with inline justification.
5. **mise.go:#8** (lines 124-126 hash file write best-effort): verified — `if err := ...; err != nil { log.Warn("mise hash file write failed (will re-extract next boot)", zap.Error(err)) }`. Best-effort soft-fail with WARN log audit trail; comment block lines 119-123 explains idempotency rationale. Compliant with §S3 documented-soft-fail carve-out.
6. **node.go:#6** (line 102-104 InstallDeps via RunWithStderrCapture): verified — `RunWithStderrCapture(cmd, stream, sandboxdomain.ErrDepInstallFailed, fmt.Sprintf("sandbox.NodeEnvManager.InstallDeps %v", deps))`. Canonical msgPrefix passed through; sentinel registered errmap.go:105. The MED `%v` issue inside RunWithStderrCapture (exec_helper.go #4) is the helper's responsibility — caller-side use is correct.

All 6 spot-checks confirmed correct classification — mechanism not rubber-stamping. The package's design (infra-layer error wrapping with sandboxdomain sentinels passed in) achieves clean separation, and most violations are localized to the **5 MED `%w: %v` sites** which need a coordinated multi-%w sweep.

## Recommended fix priorities

1. **exec_helper.go:#4 (MED §S16)** — central multi-`%w` fix. Switching `"%s: %w: %v: %s"` to `"%s: %w: %w: %s"` cascades the fix through every installer (Mise, Node, Python via RunWithStderrCapture). **Single highest-leverage change in the package.**

2. **mise.go #19 + #20 (MED §S16)** — `where` and `ListAvailable` `%v` → `%w` for ExitError chain preservation. Same defect class as #1.

3. **python.go:#4 (MED §S16)** — `CreateEnv` exec err `%w: %v` → multi-`%w`. Mirrors #1.

4. **node.go:#4 (LOW-leaning-MED §S16)** — dual-%w order swap so sentinel-first matches package convention (mcp install.go:#5 fixed in 505d6e3).

5. **§S3 inline-comment ritual** for cleanup-path `_ = X.Close() / _ = os.Remove() / _ = filepath.WalkDir()` sites — 8 sites total (mise.go #6/#17/#18, spawn.go #6/#7/#8, proc_windows.go #3/#4). All are documented carve-outs but missing the spec-required inline comment. Mechanical sweep, no behavior change.

6. **§S16 prefix-style consistency** — 9 sites with descriptive prefixes (`xattr -dr:`, `codesign %s:`, `taskkill:`, `CreateJobObject:`, etc.) → tighten to `sandbox.<funcName>:` form. Pure style cleanup; sentinel chain unaffected. Consider single sweep commit.

7. **exec_helper.go:#3 (LOW §S3)** — scanner.Err() unchecked. Optional: append `[scanner error: ...]` to captured tail so audit trail survives. Low value; may WAIVE.

## Out-of-scope notes (parent should verify)

1. **app/sandbox layer §S9 fixes (e36f890)** — confirmed correct from this audit's perspective: the EnsureEnv and markEnvFailed `context.Background()` switch correctly mirrors `spawn.go::trackedHandle.unregister` pattern. No further coordination needed.

2. **`sr_` / `se_` ID prefixes** — flagged by app-sandbox audit as §S14 doc-sync concern (CLAUDE.md §S15 example list). Not in scope for infra/sandbox audit since this layer does not generate IDs. Phase B §S14 review should add to spec or sandbox.md.

3. **embed_mise_*.go fallback semantics** — audit confirms the unsupported-platform path (empty `var miseBinary []byte` + len-zero check at mise.go:65-68) correctly triggers ErrRuntimeInstallFailed which Service.Bootstrap then handles via Degraded Mode. End-to-end intent is sound; only marginal §S16 finding is the embedded message at mise.go:#1 reads awkwardly when triggered ("no mise binary embedded for darwin/arm64: sandbox: runtime install failed") but functionally correct.

4. **Cross-platform setupProcessGroup error handling** — proc_windows.go's `_ = EnsureMasterJob()` is the only platform that has a non-trivial setup that can fail; both unix variants are infallible struct assignments. No cross-platform §S3 inconsistency.
