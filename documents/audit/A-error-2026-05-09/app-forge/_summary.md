# Package audit summary: internal/app/forge

## Spec understanding (Phase A — §S3 / §S9 / §S15 / §S16 / §S17)

- **§S3 错误不吞**: error suppression that hides user-visible failure / data loss / config drift is forbidden. `_ = err` requires inline justification; `defer X.Close()` on read-only / cleanup-after-error paths is acceptable. Documented soft-fails (boot-time best-effort, retention failures, optional reread failures) require explicit zap.Warn audit logs per §S10. Silent fallback (no log + no return) is the canonical anti-pattern.
- **§S9 detached ctx 终态写**: terminal-state DB writes that MUST persist regardless of caller-cancel use `context.Background()` (or `reqctxpkg.SetUserID(context.Background(), uid)` when caller-uid is needed). In forge: `SyncEnvForVersion`'s 4 `UpdateVersionEnvStatus` calls are terminal env-state transitions (pending → syncing → ready/failed) — currently they ride request ctx, mirroring the apikey.Test pre-fix and sandbox.EnsureEnv pre-fix defect class.
- **§S15 ID 生成**: business IDs flow through `idgenpkg.New(prefix)`. Forge uses 5 prefixes ("f" / "fv" / "tc" / "fe" / "b") — all match CLAUDE.md §S15 spec list explicitly. Plus `ComputeEnvID` produces a content-derived `env_<hash>` ID — not a §S15 random business ID, but the prefix collision with §S15 conventions is a §S14 doc-sync concern.
- **§S16 错误 wrap 格式**: `fmt.Errorf("<pkg>.<Method>: %w", err)` canonical. Three sites violate by using `%w: %v` (the inner err loses sentinel chain) — same defect class as the mcp install.go:#5 `%w: %v→%w: %w` fix from commit 505d6e3.
- **§S17 errmap 单一事实源**: every sentinel that can reach a handler must be in errmap.go. All forge domain sentinels (ErrNotFound / ErrDuplicateName / ErrVersionNotFound / ErrPendingNotFound / ErrPendingConflict / ErrTestCaseNotFound / ErrRunFailed / ErrASTParseError / ErrImportInvalid / ErrEnvNotReady / ErrNoActiveVersion / ErrEnvFailed / ErrSandboxUnavailable / ErrDependencyResolution) are at errmap.go:78-91. One site (GenerateTestCases LLM-no-JSON) uses string-only fmt.Errorf — could pollute "unmapped domain error" alarm if propagated to handler.

## Files audited

| File | LOC | Sites | OK | POST-FIX | VIOLATION | EDGE |
|---|---|---|---|---|---|---|
| ast.go | 221 | 7 | 2 | 0 | 1 | 4 |
| catalogsource.go | 94 | 1 | 0 | 0 | 0 | 1 |
| forge.go | 1670 | 52 | 17 | 0 | 4 | 31 |
| sandbox_adapter.go | 316 | 11 | 4 | 0 | 1 | 6 |
| sandbox_types.go | 131 | 2 | 1 | 0 | 0 | 1 |
| **TOTAL** | **2432** | **73** | **24** | **0** | **6** | **43** |

## Severity breakdown (only VIOLATION + EDGE)

| Severity | Count | Sites | Status |
|---|---|---|---|
| HIGH | 2 | sandbox_adapter.go:#1 (PythonPath silent EnsureTool failure → fall-through to system python3 in ast path); forge.go:#44 (SyncEnvForVersion silent UpdateVersionEnvStatus + §S9 violation — env stuck at syncing on dual-failure) | FOUND |
| MED | 4 | ast.go:#5 (`%w: %v` cmd.Output exec.ExitError chain loss); forge.go:#27 (`%w: %v` RunForge sandbox.Run sentinel loss); forge.go:#28 (`%w: %v` RunTestCase sandbox.Run sentinel loss); forge.go:#39 (`%w: %v` parse interpreter-missing detection); forge.go:#20 + #21 (silent Unmarshal / silent reread fallback in CreatePending) | FOUND |
| LOW | 37 | wrap-format style consistency (~25 bare-return vs wrap inconsistencies + ~6 helper-style prefix without `.Method`); Marshal-discard polish on basic-type maps without inline justification (~4); UNIQUE string-match for ErrDuplicateName (~3); various passthrough returns | FOUND |

## Cross-cutting

### **CRITICAL — B1 regression risk** (highest priority) — **FIXED 2026-05-10 ff8fd77**

**`sandbox_adapter.go` lines 100, 157, 254** still use `<forgeID>:<envID>` owner.ID format with literal `:` (PATH-meta character). Recent commit e36f890 added `sandboxdomain.ErrInvalidOwnerID` rejecting `:` / `;` / `=` / whitespace / NUL in `sandboxapp.EnsureEnv`. **After e36f890 deploys, every forge Sync / Run / DestroyEnv call will be rejected** because the adapter generates owner.IDs that the new validation explicitly forbids.

The B1 fix (commit 3cdf18a) changed bash auto-route owner.ID from `cv_xxx:python` → `cv_xxx_python` for the same reason, but missed updating the forge adapter. The audit pre-existing (B1 / B2 / B3) tests cover bash path but not forge.

**FIXED in commit ff8fd77** — changed `:` → `_` in all 3 code sites + Destroy() prefix matcher + 4 doc comments via replace_all. forge re-enabled end-to-end on next deploy.

### Sentinel chain integrity (§S17)

All 14 forgedomain sentinels (errmap.go:78-91) verified registered. The package's 4 §S16 violations all involve `%w: %v` patterns that drop INNER sentinel chains:

| Site | Outer sentinel (preserved) | Inner sentinel (dropped) | Discrimination loss |
|---|---|---|---|
| ast.go:#5 | errASTProcess (internal) | os.PathError / exec.ExitError | "Python missing" vs "Python crashed" indistinguishable |
| forge.go:#27 (RunForge) | forgedomain.ErrRunFailed | sandbox.ErrSpawnTimeout / ErrSpawnFailed / ErrEnvCreateFailed | "user code timed out" vs "user code crashed" vs "venv corrupt" indistinguishable |
| forge.go:#28 (RunTestCase) | forgedomain.ErrRunFailed | (same as #27) | (same as #27) |
| forge.go:#39 (parse) | forgedomain.ErrASTParseError | os.PathError (when interpreter missing) | "your code is bad Python" vs "our Python sandbox unbootstrapped" indistinguishable — leads to LLM looping regenerating valid code that can never run |

All 4 fix to `%w: %w` (Go 1.20+ multi-wrap) — same approach as mcp install.go fix in 505d6e3.

### Detached ctx coverage (§S9) — context-by-context analysis

**Terminal-state write inventory:**

| Write | File / Site | Ctx | §S9 verdict |
|---|---|---|---|
| forge create / forge update / forge delete (DB rows) | forge.go various | request ctx | ✓ OK — synchronous user-waits flow; cancel = abort, recoverable |
| version save (initial / after pending accept / on revert) | forge.go various | request ctx | ✓ OK — same |
| **SyncEnvForVersion: UpdateVersionEnvStatus(syncing)** | forge.go:1474 | request ctx | ⚠ LOW (transient; sandbox sync hasn't started) |
| **SyncEnvForVersion: UpdateVersionEnvStatus(failed) on malformed deps** | forge.go:1469 (`_ = ...`) | request ctx | **⚠ MED §S9 + §S3 silent** |
| **SyncEnvForVersion: UpdateVersionEnvStatus(failed) on sync error** | forge.go:1495 (`_ = ...`) | request ctx | **⚠ MED §S9 + §S3 silent** |
| **SyncEnvForVersion: UpdateVersionEnvStatus(ready) success path** | forge.go:1499 | request ctx | **⚠ MED §S9 — caller cancel mid-success drops env=ready; row stuck at syncing forever** |
| recordExecution save | forge.go:#30 | request ctx | ✓ OK — execution is the user-waits flow itself |
| sandbox.Sync internal writes | (delegated) | request ctx → SandboxAdapter → sandboxapp.Service | ✓ already audited in app-sandbox; e36f890 fixed UpdateEnv to context.Background() |

**§S9 verdict for forge package**: SyncEnvForVersion's 4 UpdateVersionEnvStatus calls are the §S9 hot spot — same defect class as sandbox.go::EnsureEnv pre-fix (e36f890). Recommended fix: switch all 4 to `context.Background()`. Forge env table is per-forge (forge_versions table); doesn't filter by uid — same justification as sandbox env table fix.

### §S15 ID generation summary

| Prefix | Used by | Spec list match |
|---|---|---|
| `f_` | NewForgeID, Create / CreateDraft via newID("f") | ✓ |
| `fv_` | NewVersionID, CreatePending, newVersion via newID("fv") | ✓ |
| `tc_` | CreateTestCase, GenerateTestCases via newID("tc") | ✓ |
| `fe_` | recordExecution via newID("fe") | ✓ |
| `b_` | RunAllTests via newID("b") | ✓ |
| `env_` (hash) | ComputeEnvID — NOT a random business ID; content-derived hash | §S14 doc-sync concern (not §S15 violation) |

All idgenpkg.New() calls use approved prefixes. ComputeEnvID is the only "looks like §S15 ID but isn't" — flagged for §S14 doc audit.

### sandbox_adapter PythonPath fall-through (§S3 HIGH)

Site #1 in sandbox_adapter.go is a textbook §S3 silent fallback bug — exactly the defect class fixed in B2 commit 888739c (bash auto-route silent fallback that fell through to system Python). Same pattern here: PythonPath() silently swallows EnsureTool error; downstream caller in ast.go:151 falls through to system PATH `python3`. User's forges run against unintended Python version with arbitrary system packages instead of the bundled python-build-standalone.

The fix requires (a) logging the error inside the sync.Once.Do, AND (b) changing the contract — the doc comment claims "caller treats '' as 'AST parse unavailable' and degrades gracefully", but ast.go ACTUALLY treats "" as "use system python3". One of those needs to change. Simplest: have PythonPath() return both path AND error, log warning at SandboxAdapter, and let ast.go propagate "AST parse unavailable" to LLM with sentinel.

## Spot-check (random clean sites — verify mechanism not rubber-stamping)

Random 7 sites picked from `OK` set:

1. **forge.go:#1** (panic on nil log): verified — exact `panic("forgeapp.NewService: logger is nil")` form, same canonical wiring-time invariant pattern as apikey.NewService:#1 + mcp.go:#1 + sandbox.go:#1 audited earlier.
2. **forge.go:#26** (ensureRunnable ErrEnvFailed wrap with `%w: %s`): verified — `fmt.Errorf("%w: %s", forgedomain.ErrEnvFailed, av.EnvError)`. **Distinct from #27/#28/#39's `%w: %v` defect**: here `av.EnvError` is plain string (column type), not error type, so `%s` is correct. Sentinel preserved through %w. Compliance literal.
3. **forge.go:#42** (attachPending tolerant policy): verified — code at line 1382-1392 explicitly handles ErrPendingNotFound as nil-soft-success while wrapping other errors with `forgeapp.attachPending: %w`. Comment block at 1372-1381 documents the prior silent-fallback bug ("an earlier silent-fallback version of this helper made GET responses lie when SQLite hiccupped") — explicit anti-pattern remediation. **Excellent §S3 compliance**.
4. **forge.go:#22** (auto-accept warn log): verified — `s.log.Warn("forgeapp.CreatePending: first-create auto-accept failed (caller can retry via accept_forge)", zap.String("forge_id", forgeID), zap.String("version_id", updated.ID), zap.Error(acceptErr))` provides full audit trail with all relevant identifiers. Documented intent at lines 798-801. §S10 compliant.
5. **forge.go:#46** (trimEnvBuffer log + return): verified — both ListEnvIDsForForge and DestroyEnv failures have Warn logs with forge_id + env_id + Error. Documented intent at 1527-1530 ("Errors are logged, never returned"). Compliance literal.
6. **forge.go:#47** (newID helper using idgenpkg.New): verified — exact `idgenpkg.New(prefix)` form per §S15. idgenpkg internal panic-on-rand-fail invariant.
7. **sandbox_adapter.go:#3** (Run mkdir/write/marshal canonical wraps): verified — all three wrap with `forgeapp.SandboxAdapter.Run: <stage>: %w` pkg.Method prefix + sub-tag + %w. Compliance literal across error path.

All 7 spot-checks confirmed correct classification — mechanism functioning, not rubber-stamping. The audit's 6 violations + 43 EDGEs survive spot-check pressure: the OK sites prove the canonical patterns are achievable in this package; the deviations at #27/#28/#39/#44 are real inconsistencies, not noise.

## Recommended fix priorities

1. **CRITICAL — sandbox_adapter.go owner.ID `:` → `_`** (B1 regression): change 3 sites to use `_` separator before/together with e36f890 deployment, OR widen sandbox validation to allow `:` for forge owner kind. **Without this fix, forge is broken end-to-end after e36f890 lands.**

2. **HIGH — sandbox_adapter.go:#1 PythonPath silent fallback** (§S3): change PythonPath to return `(string, error)`, log warn on EnsureTool failure, fix ast.go contract so "" doesn't fall through to system python3. Same defect class as B2 silent bash fallback.

3. **HIGH — forge.go:#44 SyncEnvForVersion** (§S3 + §S9 dual): (a) log discarded UpdateVersionEnvStatus failures; (b) switch all 4 UpdateVersionEnv* calls to `context.Background()` to match sandbox.go::EnsureEnv §S9 fix in e36f890. Without this, env can stick at syncing forever on dual-failure (DB hiccup during install).

4. **MED — `%w: %v` sentinel chain breaks** (§S16, 4 sites): ast.go:#5 / forge.go:#27 / forge.go:#28 / forge.go:#39. All 1-line fixes (`%v` → `%w`, Go 1.20+ multi-wrap). Same approach as mcp install.go fix in 505d6e3.

5. **MED — CreatePending silent Unmarshal + reread fallback** (§S3, sites #20 + #21): add Warn logs for diagnosability.

6. **LOW — wrap-format consistency sweep** (~25 sites): bare-return → wrap pattern across most CRUD methods. Pure style cleanup; consider single sweep commit. Same pattern as the apikey / chat / mcp sweep commits.

7. **LOW — Marshal-discard inline justifications** (~5 sites): Marshal of basic-type maps with no `_ = err //` comment. Same polish pattern as 505d6e3 batch.

8. **LOW — UNIQUE string-match for ErrDuplicateName** (3 sites): brittle `strings.Contains(err.Error(), "UNIQUE")` — should ideally use typed driver error. Defer if not a regression-risk priority.

## Out-of-scope notes (parent should verify)

1. **forge_redesign documents** at `/documents/version-1.2/adhoc-topic-documents/forge_redesign/` (untracked in git) — this audit didn't read them. If forge is actively being redesigned, the §S9 / §S16 fix recommendations should be coordinated with the redesign rather than committing in parallel.
2. **`env_<hash>` prefix** for ComputeEnvID is not in CLAUDE.md §S15 spec list — §S14 doc-sync concern for Phase B audit.
3. **forge handler error path** (out of forge scope; in transport layer) — needs to verify forge.go:#35 GenerateTestCases LLM-no-JSON string-only error doesn't propagate to errmap as unmapped. Cross-fork concern when transport/httpapi/handlers fork audits this.
