# Dead-logic audit — sandbox (app + infra + infra/store + domain)

Date: 2026-05-10
Scope: `internal/{app,infra,infra/store,domain}/sandbox/`
Method: read every non-test .go file end-to-end, trace cross-references, check git log when historic context needed.

LOC ≈ 5500 production .go (5 app files / 11 infra / 1 store / 3 domain).

Background: V3 marketplace collapse (commit `862f960`, 2026-05-08) deleted ~3174 LOC across 11 sandbox files (rust/go/java/ruby/php/dotnet/playwright/docker/generic/static + tests). Several stale references survived. S12 file regroup (commit `6291cf3`) renamed envmanager_*.go → kind.go without updating file-header comments.

Severity tally: 3 HIGH / 6 MED / 5 LOW + 4 EDGE.

---

## HIGH

### H1 — Owner.ID semantics doc still says `:` separator (rejected by validator in same file)

- Location: `backend/internal/domain/sandbox/sandbox.go:32`
- Claims: `// - conversation: "<conv_id>:<runtime_kind>"`
- Reality: commit `3cdf18a` (2026-05-09) replaced `:` with `_` because POSIX PATH uses `:` as separator and conv-scratch envs prepend their dir to PATH. `EnsureEnv` in `app/sandbox/sandbox.go:479` *rejects* any `owner.ID` containing `:`. Domain doc tells future caller to construct exactly the form that the validator will refuse.
- Fix: change `"<conv_id>:<runtime_kind>"` to `"<conv_id>_<runtime_kind>"`.
- Risk: a future caller (or LLM-generated forge) reads the doc, builds `cv_xxx:python`, panics or hits 422.

### H2 — codesign.go file header references three deleted files

- Location: `backend/internal/infra/sandbox/codesign.go:2-3, 23-25`
- Claims: header attributes `macCodesign` callers as "ExtractMiseBinary (`bootstrap_mise.go`) and StaticBinaryInstaller (`installer_static.go`)".
- Reality: `bootstrap_mise.go` was renamed → `mise.go` (commit `6291cf3`, S12 regroup). `installer_static.go` / `static.go` plus the `StaticBinaryInstaller` type were deleted in commit `862f960` (V3 collapse, 322 LOC gone). The current sole caller is `ExtractMiseBinary` in `mise.go`.
- Fix: drop StaticBinaryInstaller mention; rename bootstrap_mise.go → mise.go in the header.
- Risk: reader greps for `installer_static.go` / `bootstrap_mise.go`, finds neither, doubts the comment.

### H3 — node.go + python.go + installer.go reference deleted PlaywrightEnvManager

- Locations:
  - `backend/internal/infra/sandbox/node.go:108-114` (English + Chinese both say "PlaywrightEnvManager which orchestrates `playwright install`")
  - `backend/internal/infra/sandbox/python.go:117, 121` (same comment)
  - `backend/internal/domain/sandbox/installer.go:74-78` (`InstallExtras runs post-install steps (e.g. "browsers/chromium" = `playwright install chromium`)`)
- Reality: `playwright.go` (235 LOC) deleted in `862f960`. No `PlaywrightEnvManager` exists. No production code path produces a non-empty `extras` slice (see M1 below). The "browsers/chromium" example doesn't match any current consumer.
- Fix: drop the Playwright references; describe `InstallExtras` as "currently unused — kept on the interface for future post-install steps; both registered managers are no-ops" or remove from the interface (tied to M1).
- Risk: maintainer searches for `PlaywrightEnvManager` to reuse the pattern, finds nothing.

---

## MED

### M1 — `Extras []string` end-to-end is dead-data plumbing

- Locations:
  - Field: `domain/sandbox/sandbox.go:86` (`Env.Extras`), :128 (`EnvSpec.Extras`)
  - Producer paths in `app/sandbox/sandbox.go:490` (`depsEqual(existing.Extras, spec.Extras)`), :529 (`Extras: spec.Extras`), :550-555 (`if len(spec.Extras) > 0 { ... InstallExtras(...) }`)
  - Implementations: `infra/sandbox/python.go:122-124` (`return nil`), `infra/sandbox/node.go:115-117` (`return nil`)
  - Interface method: `domain/sandbox/installer.go:79`
- Claims: extras describe "post-install steps (e.g. `browsers/chromium` for Playwright)".
- Reality: zero callers of `EnsureEnv` set `spec.Extras` (verified by grep across forge/mcp/tool/handlers) — both surviving env managers' `InstallExtras` impls are unconditional `return nil`. The whole pipeline (Env.Extras column, Extras-arm of `depsEqual`, `if len(spec.Extras) > 0`, two no-op methods) plumbs an empty slice. Original sole producer (Playwright) was deleted in V3.
- Severity: MED — schema column persists empty JSON, drift-detector compares two nils, branch never enters.
- Fix: drop `Extras` field from `Env` + `EnvSpec`, drop `InstallExtras` from `EnvManager` interface + 2 no-op impls + `if len(spec.Extras) > 0` arm + Extras field from `depsEqual`. ~20 LOC across 5 files.
- Risk: reduces noise; if Playwright/extras-ish flow returns, restore from git. Aligns with §S20 "no dead code; restore from git when needed".

### M2 — `IsDefault` flag + `FindDefaultRuntime` query are parallel dead-mechanism

- Locations:
  - Field: `domain/sandbox/sandbox.go:56` (`Runtime.IsDefault`)
  - Producer: `app/sandbox/sandbox.go:435` (`IsDefault: spec.Version == ""`)
  - Repo method: `infra/store/sandbox/sandbox.go:86` (`FindDefaultRuntime`)
  - Domain port: `domain/sandbox/sandbox.go:225` (`Repository.FindDefaultRuntime`)
- Claims: `IsDefault marks the kind's default (resolved from empty Version spec)` (domain comment).
- Reality: `EnsureRuntime` resolves empty version via `installer.ResolveDefault(ctx)` (returns the const `defaultVersion` baked into `MiseInstaller` at construction), then immediately calls `FindRuntime(kind, version)` with the concrete string. `FindDefaultRuntime` is **never called from production** (only from `infra/store/sandbox/sandbox_test.go`). The `IsDefault=true` row is created but no production query ever consults `is_default`. The DB index `idx_sr_kind_def` (on `(kind, is_default)`) supports a query that is never run.
- Severity: MED — dual mechanism (constant in installer + DB column) where only the constant is actually used. Misleads anyone trying to understand "how default version selection works".
- Fix: drop `Runtime.IsDefault` field + `idx_sr_kind_def` index + `FindDefaultRuntime` repo method + interface method + the `spec.Version == ""` producer expression. Or, keep the column but document "currently informational; future UI may query".
- Risk: `EnsureRuntime` write becomes simpler. Tests using `IsDefault` (~3 in store_test) need stripping.

### M3 — codesign.go recursive WalkDir is dead-loop logic for the only caller

- Location: `backend/internal/infra/sandbox/codesign.go:75-95` (`filepath.WalkDir` over `root`)
- Claims: comment at `mise.go:107-112` admits *"macCodesign walks recursively but accepts a single-file root just fine — the WalkDir hits exactly one entry"*.
- Reality: with `static.go` / `playwright.go` gone, `macCodesign`'s sole caller is `ExtractMiseBinary` passing a single binary path. The xattr step is fine on a file. The `WalkDir` recurses over a single file (always 1 visit). The "loaded by interpreter (libpython.dylib + stdlib .so)" rationale in the comment refers to the deleted Python static-tarball install path.
- Severity: MED — the dead branch is the entire recursion logic + signed-files counter + WalkDir error plumbing.
- Fix: simplify `macCodesign` to xattr + `codesign --force --sign - <path>` directly. Drop `signed` counter, WalkDir, fs.DirEntry handling (~30 LOC). Or keep WalkDir but acknowledge the simplification when V2 desktop-prod re-introduces multi-file installers.
- Risk: shrinks codesign helper substantially; if a future installer needs multi-file signing the helper is rebuildable.

### M4 — `ExtractMiseBinary` re-derives `dataDir/sandbox` that Service already cached as sandboxRoot

- Locations:
  - Producer: `app/sandbox/sandbox.go:148, 200` — Service stores `sandboxRoot = filepath.Join(dataDir, "sandbox")` at construction; calls `ExtractMiseBinary(ctx, s.dataDir, ...)`.
  - Consumer: `infra/sandbox/mise.go:70, 76, 134` — re-derives `filepath.Join(dataDir, "sandbox", "bin")`, `filepath.Join(dataDir, "sandbox", ".mise.hash")`, `filepath.Join(dataDir, "sandbox")`.
- Claims: dataDir is the parent of sandboxRoot.
- Reality: caller already has `sandboxRoot`; passes parent and forces consumer to recompute the same join in three places. Two instances of "dataDir + sandbox" are now scattered (one in app/sandbox/Bootstrap, three in infra/sandbox/mise.go).
- Severity: MED — drift hazard. If sandboxRoot's relative layout ever changes, only Service.New is updated; ExtractMiseBinary keeps the old layout.
- Fix: change ExtractMiseBinary signature to accept `sandboxRoot` (drop the "sandbox" subdir constant from infra side) — Service passes `s.sandboxRoot` directly. Or vice versa, make Service hold dataDir only and derive `sandboxRoot` lazily.
- Risk: small mechanical refactor; aligns ownership of the layout convention.

### M5 — SpawnLongLived re-fetches env that prepareSpawn already validated

- Location: `backend/internal/app/sandbox/spawn.go:101-105` and the dead-path branches at `spawn.go:102-105, 123` (`envID := ""; if lookupErr == nil { envID = envRow.ID }; ... if envID != "" { ... }`)
- Claims: lookup might fail; envID-empty fallback handles it.
- Reality: by the time we reach line 101, `prepareSpawn` already returned ok at line 88, which means the inner `FindEnvByOwner` at `sandbox.go:205` succeeded. So this lookup at line 101 is a duplicate DB round-trip that always succeeds (modulo a vanishingly small race where the env is destroyed between prepareSpawn and the second lookup — but if so, the spawn already started and the cleanup-on-Wait would still try to clear a non-existent row, which `ClearEnvRunningPID` no-ops).
- Severity: MED — extra DB query per long-lived spawn + dead defensive branch.
- Fix: have `prepareSpawn` return the resolved `envRow` (or its ID) so `SpawnLongLived` reuses it; drop the second lookup + the "envID empty" branch.
- Risk: tightens hot path for stdio MCP server starts and Bash background processes.

### M6 — `OwnerKindMCP` constant unused in production; `OwnerKindSkill` is dead vocabulary

- Locations:
  - Constants: `domain/sandbox/sandbox.go:22-25` define 4 `OwnerKind*`.
  - Production producers: `app/forge/sandbox_adapter.go` uses `OwnerKindForge`; handlers + bash use `OwnerKindConversation`.
  - MCP install path: `app/mcp/install.go:88` writes `Kind: "mcp"` as a string literal — does **not** use `sandboxdomain.OwnerKindMCP`.
  - `OwnerKindSkill`: zero production references (skill domain unimplemented).
- Claims: domain provides the canonical vocabulary for owner kinds, with DB CHECK constraint enforcing the same set.
- Reality: `OwnerKindMCP` is an exported constant only consumed by tests (`infra/store/sandbox/sandbox_test.go`). Producer (`mcp.InstallFromRegistry`) bypasses the constant. `OwnerKindSkill` has no producer at all. Half-applied vocabulary.
- Severity: MED — typo-resistance is the whole point of the constant; production sidesteps it. Also a doc-vs-code mismatch with the DB CHECK list.
- Fix: change `app/mcp/install.go:88` to use `sandboxdomain.OwnerKindMCP`. Drop `OwnerKindSkill` until skill domain ships, or document "reserved for Phase 5 skill envs".
- Risk: trivially mechanical; reduces typo surface.

---

## LOW

### L1 — exec_helper.go file-header comment lists 11 callers; only 4 survive

- Location: `backend/internal/infra/sandbox/exec_helper.go:1-13`
- Claims: `RunWithStderrCapture wraps the install/dep-fetch command pattern used across MiseInstaller / Node / Python / Rust / Go / Java / Ruby / PHP / .NET / Playwright`.
- Reality: Rust/Go/Java/Ruby/PHP/.NET/Playwright managers all deleted in V3 collapse. Surviving callers: `MiseInstaller.Install`, `NodeEnvManager.InstallDeps`, `PythonEnvManager.InstallDeps`. That's 3, not 11.
- Severity: LOW — just stale list in a header comment.
- Fix: shrink list to mise + node + python.

### L2 — python.go + node.go file-header use pre-S12 filenames `envmanager_python.go` / `envmanager_node.go`

- Locations: `python.go:1, 12`; `node.go:1, 8`
- Claims: file-header self-identifies as `envmanager_python.go`. After S12 regroup commit `6291cf3` the file lives at `python.go`.
- Severity: LOW — confusing if reader uses `git log <header-name>`.
- Fix: rename file-header lines.

### L3 — `ErrDockerNotInstalled` / `ErrDockerDaemonDown` declared + errmap-registered, never returned from any code path

- Locations:
  - Declaration: `domain/sandbox/sandbox.go:200-213`
  - errmap entry: `transport/httpapi/response/errmap.go:120-121`
- Claims: errmap comment says *"Phase 5 docker sentinels — pre-registered so future docker-runtime trigger isn't unmapped"*.
- Reality: `docker.go` (392 LOC) deleted in `862f960` — no production code path returns these. They sit waiting for "Phase 5 future" that may never come (V3 collapse explicitly removed Docker runtime support — design decision, not a defer).
- Severity: LOW — sentinels are cheap (just `errors.New` calls) and their explicit pre-registration prevents future "unmapped domain error" alarm if someone re-introduces Docker. Defensible.
- Fix: leave as is, OR drop both sentinels + errmap rows + comments. Per §S20 "don't keep dead reservations", lean toward drop.

### L4 — `RunningStartedAt` field is write-only

- Locations:
  - Producer: `infra/store/sandbox/sandbox.go:323` (`SetEnvRunningPID` sets `running_started_at: time.Now()`)
  - Reset: `infra/store/sandbox/sandbox.go:342` (`ClearEnvRunningPID` zeroes)
  - Reader: only `infra/store/sandbox/sandbox_test.go:468` (test verifies it's set)
- Claims: implicitly tracking "when did the long-lived process start".
- Reality: production code never reads `RunningStartedAt`. `RestoreOrCleanupOnBoot` only consults `RunningPID`. No diagnostic / metric / UI surface displays the start time.
- Severity: LOW — column persists, but harmless. Could be useful for "process running for X" metric if added later.
- Fix: keep + document "diagnostic only, no current reader" OR drop column + DDL change.

### L5 — `ListAvailable` interface method has zero production callers

- Locations:
  - Interface: `domain/sandbox/installer.go:41` (`ListAvailable(ctx) ([]string, error)`)
  - Sole impl: `infra/sandbox/mise.go:450-462` (calls `mise ls-remote <kind>`)
- Claims: comment says *"installable versions for UI pickers"*.
- Reality: no UI picker in the v1.2 codebase calls it. No HTTP handler exposes it. Pure dormant feature waiting for Phase 5 UI.
- Severity: LOW — the impl works and is small. Defensible to keep.
- Fix: leave (clearly future-flagged in comment) OR drop until first consumer needs it.

---

## EDGE (questionable / context-dependent)

### E1 — `ProgressFunc` signature richer than every call site

- Location: `domain/sandbox/sandbox.go:171` defines `func(stage, message string, percent int)`. Sole call site `infra/sandbox/exec_helper.go:58` always invokes with `("running", line, -1)`.
- Reality: `stage` is fixed, `percent` is always -1. The consumer at `pkg/installprogress/installprogress.go:197` formats `[stage] message (NN%)` but the `(NN%)` arm at line 205-207 never fires.
- Severity: EDGE — interface designed for richness producer never delivers. Could be intended for Phase 5 fancier mise hooks.
- Fix: leave OR collapse to `func(message string)`.

### E2 — `MarkReadyForTest` panic guard `if log == nil` defensive-only

- Location: `app/sandbox/sandbox.go:142-144`. Both production callers wire log unconditionally (`cmd/server/main.go:206`, `test/harness/harness.go:271`).
- Reality: nil-log path can never trigger. The panic is purely safety-net.
- Severity: EDGE — harmless 2 LOC.

### E3 — `ownerLock` map key uses `:` separator (not actually a path)

- Location: `app/sandbox/sandbox.go:727` (`key := owner.Kind + ":" + owner.ID`)
- Reality: `owner.ID` already passed `strings.ContainsAny(":;= \t\n\r\x00")` rejection earlier in EnsureEnv, so colons can't appear in owner.ID at this point. The lock-map separator is internal — never serialized to disk / PATH. Safe to keep.
- Severity: EDGE — confusing if reader thinks it's the same separator that triggered the bug.

### E4 — Bootstrap failure path stores `bootstrapped=false` even though that's the zero value

- Location: `app/sandbox/sandbox.go:205` (`s.bootstrapped.Store(false)` after first-time failure).
- Reality: `atomic.Bool` zero state is false; if Bootstrap was never called or called only with prior-run failure, re-storing false is a no-op. RetryBootstrap path makes this slightly meaningful (prior call succeeded → flipped true → retry fails → must reset to false). So actually defensible — only EDGE.
- Fix: leave; the comment could clarify "explicit reset for retry path".

---

## Summary

V3 marketplace collapse (2 days ago) was thorough at code level but left ~10 prose-and-shape stale spots:

- 1× domain doc actively misleading (H1 `:` separator)
- 3× file-header references to deleted files (H2 codesign, L2 envmanager_*, H3 PlaywrightEnvManager)
- 2× large-ish dead mechanisms (M1 Extras pipeline, M2 IsDefault flag)
- 1× recursion-on-single-file dead loop (M3 codesign)
- 1× duplicate DB lookup (M5 SpawnLongLived)
- 1× partial-vocab (M6 OwnerKindMCP unused, OwnerKindSkill no producer)
- Smatter of LOW/EDGE around dormant interface methods + Docker sentinels.

Recommended priority: H1 → H3 → H2 → M1+M2+M3 (sandbox slim-down round 2) → M4+M5 → cleanup.
