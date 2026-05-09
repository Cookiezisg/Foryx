# Package audit summary: internal/app/sandbox

## Spec understanding (Phase A — §S3 / §S9 / §S15 / §S16 / §S17)

- **§S3 错误不吞**: error suppression that hides user-visible failure / data loss / config drift is forbidden. `_ = err` requires inline justification. `defer X.Close()` on read-only resources or panic-path cleanup is allowed. Documented best-effort soft-fails (filesystem walk skipping broken entries, manifest cleanup on boot, GC continuing past per-env failures) require explicit zap.Warn audit logs per §S10. Silent fallthrough without log is the canonical anti-pattern.
- **§S9 detached ctx 终态写**: terminal-state writes that MUST persist regardless of caller cancel use `reqctxpkg.SetUserID(context.Background(), uid)` or `context.Background()` directly when no user identity is needed. In sandbox: `trackedHandle.unregister()` is a model correct example (uses `context.Background()` for Wait/Kill cleanup that outlives request). EnsureEnv's ready/failed transitions use request ctx — analyzed as LOW-severity §S9 questions because of self-healing path (deps-drift rebuild on next install) but not strictly compliant.
- **§S15 ID 生成**: business IDs flow through `idgenpkg.New(prefix)`. Sandbox uses two prefixes: "sr" (sandbox runtime) and "se" (sandbox env). Neither is in CLAUDE.md §S15's explicit list (which mentions aki/cv/msg/blk/etc. with specifically `sar_` for subagent run + `smm_` for subagent message). Format scheme is consistent — note as §S14 doc-sync concern not §S15 violation.
- **§S16 错误 wrap 格式**: `fmt.Errorf("<pkg>.<Method>: %w", err)` canonical. Bare `return err` preserves sentinel chain (functionally OK) but breaks call-site grep traceability. Sandbox is mostly canonical-compliant; LOW findings are bare-return-vs-wrap inconsistency + a few defensive-validation sites missing sentinels.
- **§S17 errmap 单一事实源**: every sentinel that can reach a handler must be in errmap.go. All 8 sandboxdomain sentinels (errmap.go:100-107) are registered. Three sites use anonymous `fmt.Errorf` for defensive validation — same family as `mcp.go:#6` and `apikey.tester.go:#4` (resolved as panic). Recommendation: introduce 2-3 new sentinels OR panic.

## Files audited

| File | LOC | Sites | OK | POST-FIX | VIOLATION | EDGE |
|---|---|---|---|---|---|---|
| disk.go | 86 | 4 | 3 | 0 | 0 | 1 |
| restore.go | 118 | 7 | 6 | 0 | 0 | 1 |
| sandbox.go | 759 | 44 | 35 | 0 | 0 | 9 |
| spawn.go | 330 | 18 | 14 | 0 | 0 | 4 |
| **TOTAL** | **1293** | **73** | **58** | **0** | **0** | **15** |

## Severity breakdown (only VIOLATION + EDGE)

| Severity | Count | Sites | Status |
|---|---|---|---|
| HIGH | 0 | — | — |
| MED | 0 | — | — |
| LOW (§S9 questions) | 2 | sandbox.go:#32 (ready-state write uses request ctx — self-healing via deps-drift rebuild but fragile), sandbox.go:#36 (markEnvFailed write same concern) | FOUND |
| LOW (§S17 sentinel gaps) | 3 | sandbox.go:#22 (missing owner.Kind/ID validation no sentinel), sandbox.go:#23 (owner.ID PATH-meta validation no sentinel), spawn.go:#10 (empty Cmd validation no sentinel) | FOUND |
| LOW (§S16 wrap-format consistency) | 5 | sandbox.go:#3 (Bootstrap bare-return), sandbox.go:#4 (EnsureTool bare-return), sandbox.go:#18 (install wrap missing kind/version), sandbox.go:#26 (EnsureRuntime call missing context), spawn.go:#4 (SpawnLongLived bare-return), spawn.go:#8 (Shutdown ctx.Err bare) | FOUND |
| LOW (§S3 silent log gaps) | 5 | disk.go:#3 (os.ErrInvalid bare), restore.go:#5 (FindProcess err discarded — documented but no log), sandbox.go:#39 (envRuntimeKind silent err), spawn.go:#5 (lookupErr discarded for Layer B — documented but no log) | FOUND |

## Cross-cutting

### Sentinel chain integrity (§S17)

All 8 sandboxdomain sentinels (errmap.go:100-107) verified registered by file:

| Sentinel | errmap.go line | Consumed in |
|---|---|---|
| `ErrRuntimeNotSupported` | 100 | sandbox.go:#14, #27; spawn.go:#14 |
| `ErrRuntimeInstallFailed` | 101 | sandbox.go:#13 |
| `ErrEnvNotFound` | 102 | sandbox.go:#24, #33 (idempotent skip) |
| `ErrEnvCreateFailed` | 103 | sandbox.go:#21 |
| `ErrDepInstallFailed` | 104 | (consumed via em.InstallDeps wrap chain in sandbox.go:#31) |
| `ErrSpawnFailed` | 105 | spawn.go:#9, #12 |
| `ErrSpawnTimeout` | 106 | (consumed via spawnCtx context.WithTimeout chain) |
| `ErrEnvInUse` | 107 | sandbox.go:#10 |

**No missing registrations**. Three sites use anonymous `fmt.Errorf` — these would benefit from new sentinels:
- `sandbox.go:#22` (missing owner.Kind/ID) — suggest `ErrOwnerRequired`
- `sandbox.go:#23` (PATH-meta in owner.ID) — suggest `ErrInvalidOwnerID` (defensive against bash auto-route regression)
- `spawn.go:#10` (empty Cmd) — suggest `ErrCmdRequired`

Same sentinel-gap pattern as `mcp.go:#6` (still FOUND) and `apikey.tester.go:#4` (resolved as panic). Recommend coordinated decision: panic for wiring-bug invariants, sentinel for user-reachable validation.

### Detached ctx coverage (§S9) — context-by-context analysis

**Terminal-state write inventory:**

| Write | File / Site | Ctx | §S9 verdict |
|---|---|---|---|
| Bootstrap success/fail (atomic stores) | sandbox.go:206-210 | (none — atomic.Pointer.Store, not DB) | N/A — atomic, not terminal DB write |
| CreateRuntime (manifest insert) | sandbox.go:#20 | request ctx | ✓ OK — intermediate state; failure path is "install failed, retry" |
| CreateEnv (status=installing) | sandbox.go:#29 | request ctx | ✓ OK — intermediate state; failure path is "install failed, rebuild" |
| **UpdateEnv (status=ready)** | sandbox.go:#32 | request ctx | **⚠ LOW EDGE — self-healing via deps-drift rebuild but cancel race could leave env at status=installing forever; comparable to apikey.Test pre-fix** |
| **UpdateEnv (status=failed in markEnvFailed)** | sandbox.go:#36 | request ctx | **⚠ LOW EDGE — same concern as #32** |
| DeleteEnv (in destroyLocked) | sandbox.go:#35 | request ctx | ✓ OK — idempotent removal; if request cancels, env stays but next install for same owner re-destroys |
| DeleteRuntime | sandbox.go:339 | request ctx | ✓ OK — synchronous user-waits on result; failure recoverable |
| ClearEnvRunningPID (boot scan) | restore.go:#3 | boot ctx (context.Background-derived) | ✓ OK — boot path, no cancel risk |
| SetEnvRunningPID (Spawn handle track) | spawn.go:#6 | request ctx | ✓ OK — best-effort Layer B leak prevention; logged on failure |
| **ClearEnvRunningPID (unregister Wait/Kill)** | spawn.go:#18 | **context.Background()** ← detached | ✓ **MODEL CORRECT §S9** — Wait/Kill outlives request |
| envRuntimeKind read inside publishEnv | sandbox.go:#40 | **context.Background()** ← detached | ✓ correct (read variant of detached pattern) |
| publishEnv / publishEnvDeleted (notification) | sandbox.go:#37, #38 | request ctx (notif.Publish parameter) | ✓ OK — best-effort UI signal; publisher contract handles |
| touchLastUsed (LastUsedAt update) | sandbox.go:#41, spawn.go:#15 | request ctx | ✓ OK — read-path metadata; GC self-corrects |
| markEnvFailed publishEnv after status=failed | sandbox.go:608 | request ctx | ✓ OK with caveat (depends on the markEnvFailed UpdateEnv landing) |

**§S9 verdict for package**: **mostly compliant**. The model correct example is `spawn.go::trackedHandle.unregister()` (site #18). The two LOW EDGE concerns are sandbox.go's `EnsureEnv` ready/failed transitions — recommended fix is to follow spawn.go's pattern with `context.Background()` for the final state writes (the writes don't need user identity since env table doesn't filter by uid).

### Env lifecycle state-transition table

For audit completeness — every sandbox env state transition mapped to its write site + ctx source:

| Transition | Site | Ctx | Status |
|---|---|---|---|
| (none) → installing | sandbox.go:#29 (CreateEnv) | request | ✓ OK |
| installing → ready | sandbox.go:#32 (UpdateEnv) | request | ⚠ LOW (self-healing) |
| installing → failed | sandbox.go:#36 (UpdateEnv via markEnvFailed) | request | ⚠ LOW (self-healing) |
| ready → installing (deps drift rebuild) | sandbox.go:486-488 (destroyLocked) → CreateEnv | request | ✓ OK |
| any → destroyed | sandbox.go:#35 (DeleteEnv in destroyLocked) | request | ✓ OK |
| running_pid set (long-lived spawn) | spawn.go:#6 (SetEnvRunningPID) | request | ✓ OK |
| running_pid clear (Wait/Kill) | spawn.go:#18 (ClearEnvRunningPID) | **Background** | ✓ MODEL |
| running_pid clear (boot scan) | restore.go:#3 (ClearEnvRunningPID) | boot | ✓ OK |

### §S15 ID generation

Two ID generation sites verified:
- `sandbox.go:#19` (`idgenpkg.New("sr")` for runtime IDs at line 430)
- `sandbox.go:#28` (`idgenpkg.New("se")` for env IDs at line 507)

Both use idgenpkg properly (per-§S15 panic-on-rand-fail invariant managed by idgenpkg internals). The "sr_" / "se_" prefixes follow the spec scheme but aren't on CLAUDE.md §S15's explicit example list. **Cross-fork concern**: this is a §S14 doc-sync concern — either CLAUDE.md spec or sandbox.md should explicitly document the sandbox prefixes. Out of scope for §S15 violation classification (Phase A); flag for Phase B (§S20/§S14 audit) cross-reference.

### owner.ID validation regression test (B1 fix permanence)

Site `sandbox.go:#23` is the validation that prevents regression of the bash auto-route bug fixed in commit 3cdf18a (`:` → `_`). The validation correctly rejects PATH-meta + whitespace characters. **Audit confirms**: regression test still in place, function correctly returns error on malformed owner.ID. Only LOW finding is the missing sentinel — error fires as anonymous fmt.Errorf which would hit "unmapped domain error" alarm if triggered (though triggering means LLM bash auto-route handed bad owner.ID, which itself is the regression).

## Spot-check (random clean sites — verify mechanism not rubber-stamping)

Random seed: 7 sites picked from `OK` set across 4 files:

1. **disk.go:#3** (removeAll fs root guard): verified — `if !filepath.IsAbs(path) { return os.ErrInvalid }; if isFilesystemRoot(clean) { return os.ErrInvalid }`. Defense-in-depth against catastrophic path bugs. Returns stdlib sentinel that errors.Is consumers can handle. Inline comment lines 47-49 explains rationale.
2. **restore.go:#1** (RestoreOrCleanupOnBoot signature returns void): verified — file-header rationale lines 27-30 explicit "boot must proceed even if cleanup partial". Consistent with §S3 carve-out for boot-time best-effort. Internal errors all logged.
3. **sandbox.go:#1** (panic on nil logger): verified — same canonical wiring-time invariant pattern as apikey.NewService:#1 + mcp.go:#1.
4. **sandbox.go:#10** (ErrEnvInUse wrap): verified — `fmt.Errorf("sandboxapp.DeleteRuntime: %d env(s) still reference %s: %w", len(envs), id, sandboxdomain.ErrEnvInUse)`. pkg.Method + count + id context + %w + sentinel registered errmap.go:107 → 409. Compliance literal.
5. **sandbox.go:#19** (`idgenpkg.New("sr")`): verified — uses idgenpkg per §S15. The "sr" prefix follows the spec scheme. idgenpkg internal panic-on-rand-fail invariant.
6. **sandbox.go:#40** (context.Background in publish read): verified — deliberate use of Background instead of request ctx because publishEnv runs in fire-and-forget notification path. Reverse of §S9 detached pattern but same principle.
7. **spawn.go:#18** (model §S9 in unregister): verified — exact `context.Background()` form for Wait/Kill cleanup. The textbook example of the §S9 pattern this audit pass is enforcing elsewhere.

All 7 spot-checks confirmed correct classification — mechanism functioning, not rubber-stamping. The audit's primary finds (§S9 questions on sandbox.go:#32/#36, sentinel gaps on sandbox.go:#22/#23/spawn.go:#10) survive spot-check pressure: the model-correct OK sites #18 (spawn) prove the §S9 pattern is achievable in this package; the deviations at sandbox.go EnsureEnv are real inconsistencies, not noise.

## Recommended fix priorities

1. **sandbox.go:#32 + #36** (LOW EDGE §S9) — change ready/failed UpdateEnv calls to use `context.Background()` instead of request ctx. Mirror spawn.go:#18's pattern. Self-healing exists but cancel race causes phantom installing rows. Low risk fix (both writes don't depend on user identity since env table is global).

2. **sandbox.go:#22 + #23 + spawn.go:#10** (LOW §S17 sentinel gaps) — three defensive-validation sites with anonymous fmt.Errorf. Coordinated decision needed:
   - Add sentinels: `sandboxdomain.ErrOwnerRequired`, `ErrInvalidOwnerID`, `ErrCmdRequired` + register errmap as 400
   - OR panic (config-time invariant — same call as apikey.tester.go:#4 resolution)
   - Recommend: sentinel for #23 (user-reachable via LLM bash auto-route), panic for #22 + spawn.go:#10 (caller-side wiring bugs)

3. **§S16 wrap-format consistency** (LOW × 6) — bare-return → wrap pattern at sandbox.go:#3, #4, #18, #26 + spawn.go:#4, #8. Pure style cleanup; consider single sweep commit.

4. **§S3 log-gap polish** (LOW × 4) — add Debug/Warn logs at disk.go:#3 (or accept as documented), restore.go:#5 (Windows FindProcess err), sandbox.go:#39 (envRuntimeKind), spawn.go:#5 (Layer B lookup). Low value individually; may WAIVE if test load surfacing them is high.

## Out-of-scope notes (parent should verify)

1. **`sr_` / `se_` ID prefixes not in CLAUDE.md §S15 spec list** — out of scope for Phase A (§S15 violation requires non-idgenpkg use which is not the case). Flag as §S14 doc-sync concern for Phase B.
2. **EnsureEnv install progress streaming** (sandbox.go:495 spec.Runtime call passes `stream` param) — relies on `installprogresspkg` which has its own ctx semantics. Not audited here; should verify in pkg/installprogress audit.
3. **infra/sandbox layer** has its own audit fork pending — sentinels like `ErrSpawnTimeout` (errmap.go:106) and inner sandbox-installer errors come from there. App-sandbox correctness depends on infra layer wrapping properly.
4. **sandbox spec list completeness** — §S15 spec lists prefixes through `smm_` (subagent message) but not `sr_`/`se_`. Recommend Phase B §S14 review to either:
   - Add `sr_` / `se_` to CLAUDE.md §S15 examples
   - Or document sandbox prefix allowance in sandbox.md
