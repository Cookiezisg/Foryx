# audit: backend/internal/app/sandbox/sandbox.go

LOC: 759
Read: full file (lines 1-759)

## Trace

| site# | file:line | snippet | category | classification | reasoning | severity | user_impact | suggested_fix | status |
|---|---|---|---|---|---|---|---|---|---|
| 1 | sandbox.go:142-144 | `if log == nil { panic("sandboxapp.New: nil logger") }` | A.1 | OK | wiring-time guard; §S3 carve-out for unrecoverable init invariants. Same pattern as apikey.NewService / mcp.New. | N-A | — | — | — |
| 2 | sandbox.go:184-189 | `func (s *Service) BootstrapError() error { if e := s.bootstrapErr.Load(); e != nil { return *e } return nil }` | A.4 | OK | atomic-pointer load; nil pointer → nil error is canonical Go pattern. Not a wrap site. | N-A | — | — | — |
| 3 | sandbox.go:200-207 | `miseBin, err := sandboxinfra.ExtractMiseBinary(...); if err != nil { s.log.Warn("sandbox bootstrap failed (degraded mode active)", zap.Error(err)); captured := err; s.bootstrapErr.Store(&captured); s.bootstrapped.Store(false); return err }` | A.4 | EDGE | bare `return err` — no `sandboxapp.Bootstrap:` prefix. **Reasoning**: caller (main.go) uses err only to log boot status; not propagated through error chain that needs `errors.Is`. The captured pointer + log already provides context. Style inconsistency vs. EnsureRuntime/EnsureEnv which wrap. | LOW | identical UX (caller in main.go logs / continues to degraded mode regardless) | wrap: `return fmt.Errorf("sandboxapp.Bootstrap: %w", err)` for grep traceability | FOUND |
| 4 | sandbox.go:265-281 | `func EnsureTool(...): rt, err := s.EnsureRuntime(...); if err != nil { return "", err }; ...; bin, err := installer.Locate(...); if err != nil { return "", fmt.Errorf("sandboxapp.EnsureTool %s: %w", kind, err) }` | A.4 | EDGE | Two issues: (a) line 268 `return "", err` bare — propagates EnsureRuntime's already-wrapped error. (b) line 274 wraps with sentinel correctly. The bare-return at 268 is OK style ("the inner already wrapped this") but inconsistent within same function. | LOW | identical UX (sentinel preserved either way) | line 268 wrap: `return "", fmt.Errorf("sandboxapp.EnsureTool %s: %w", kind, err)` for consistency | FOUND |
| 5 | sandbox.go:286-288 | `func ListRuntimes(ctx) ([]..., error) { return s.repo.ListRuntimes(ctx) }` | A.4 | OK | thin passthrough; repo wraps internally per repo conventions. Not adding a wrap layer is canonical. | N-A | — | — | — |
| 6 | sandbox.go:293-295 | `func ListEnvs(ctx, ownerKind) ([]..., error) { return s.repo.ListEnvsByOwnerKind(ctx, ownerKind) }` | A.4 | OK | same as #5 | N-A | — | — | — |
| 7 | sandbox.go:300-302 | `func TotalDiskUsage(ctx) (int64, error) { return s.repo.TotalSizeBytes(ctx) }` | A.4 | OK | same | N-A | — | — | — |
| 8 | sandbox.go:309-311 | `func GetEnv(ctx, id) (*sandboxdomain.Env, error) { return s.repo.GetEnv(ctx, id) }` | A.4 | OK | same; ErrEnvNotFound sentinel preserved through bare-return + already in errmap.go:102 | N-A | — | — | — |
| 9 | sandbox.go:321-340 | `DeleteRuntime: rt, err := s.repo.GetRuntime(ctx, id); if err != nil { return fmt.Errorf("sandboxapp.DeleteRuntime: get %s: %w", id, err) }; ... if len(envs) > 0 { return fmt.Errorf("sandboxapp.DeleteRuntime: %d env(s) still reference %s: %w", len(envs), id, sandboxdomain.ErrEnvInUse) }; ... if err := removeAll(rtPath); err != nil { s.log.Warn(...) }; return s.repo.DeleteRuntime(ctx, id)` | A.1/A.4 | OK | All wraps use `<pkg>.<Method>:` prefix + `%w`. ErrEnvInUse correctly registered errmap.go:107. Disk removeAll failure logged + continues to row deletion (manifest stays consistent — deferred file cleanup over orphaned row). Final bare-passthrough of repo.DeleteRuntime is canonical. | N-A | — | — | — |
| 10 | sandbox.go:330-333 | `if len(envs) > 0 { return fmt.Errorf("sandboxapp.DeleteRuntime: %d env(s) still reference %s: %w", len(envs), id, sandboxdomain.ErrEnvInUse) }` | A.4 | OK | §S16 canonical: pkg.Method prefix + %w sentinel preserved → errmap.go:107 → 409 SANDBOX_ENV_IN_USE | N-A | — | — | — |
| 11 | sandbox.go:335-338 | `if err := removeAll(rtPath); err != nil { s.log.Warn("sandbox: delete runtime dir failed (continuing to delete row)", zap.String("path", rtPath), zap.Error(err)) }` | A.1 | OK | §S3 documented soft-fail with audit log + inline comment ("continuing to delete row"). Manifest consistency wins over disk cleanup; user can retry. | N-A | — | — | — |
| 12 | sandbox.go:350-371 | `GC: stale, err := s.repo.ListEnvsLastUsedBefore(...); if err != nil { return 0, fmt.Errorf("sandboxapp.GC: list stale: %w", err) }; for _, e := range stale { ... if err := s.Destroy(ctx, owner); err != nil { s.log.Warn("sandbox GC: destroy env failed (continuing)", ...); continue }; removed++ }` | A.1/A.4 | OK | List failure wrapped + propagated. Per-env Destroy failure logged + continue (best-effort cleanup; remaining envs still GC'd). Final Info log summarizes outcome — provides audit trail. §S10 fire-and-forget log compliance. | N-A | — | — | — |
| 13 | sandbox.go:381-384 | `if !s.IsReady() { return nil, fmt.Errorf("sandboxapp.EnsureRuntime: %w", sandboxdomain.ErrRuntimeInstallFailed) }` | A.4 | OK | §S16 canonical with sentinel registered errmap.go:101 → 502 SANDBOX_RUNTIME_INSTALL_FAILED | N-A | — | — | — |
| 14 | sandbox.go:389-391 | `if !ok { return nil, fmt.Errorf("sandboxapp.EnsureRuntime %s: %w", spec.Kind, sandboxdomain.ErrRuntimeNotSupported) }` | A.4 | OK | §S16 canonical; sentinel errmap.go:100 → 422 SANDBOX_RUNTIME_NOT_SUPPORTED | N-A | — | — | — |
| 15 | sandbox.go:394-400 | `if version == "" { v, err := installer.ResolveDefault(ctx); if err != nil { return nil, fmt.Errorf("sandboxapp.EnsureRuntime: resolve default %s: %w", spec.Kind, err) } version = v }` | A.4 | OK | wraps with sub-tag "resolve default"; §S16 canonical | N-A | — | — | — |
| 16 | sandbox.go:404-408 | `if existing, err := s.repo.FindRuntime(ctx, spec.Kind, version); err == nil { return existing, nil } else if !errors.Is(err, gorm.ErrRecordNotFound) { return nil, fmt.Errorf("sandboxapp.EnsureRuntime: lookup %s@%s: %w", spec.Kind, version, err) }` | A.4 | OK | §S16 canonical; gorm.ErrRecordNotFound is the "not found, proceed to install" branch (correct). Other errors wrapped. | N-A | — | — | — |
| 17 | sandbox.go:418-422 | `if existing, err := s.repo.FindRuntime(ctx, spec.Kind, version); err == nil { return existing, nil } else if !errors.Is(err, gorm.ErrRecordNotFound) { return nil, fmt.Errorf("sandboxapp.EnsureRuntime: re-lookup %s@%s: %w", spec.Kind, version, err) }` | A.4 | OK | same pattern as #16 | N-A | — | — | — |
| 18 | sandbox.go:424-427 | `relPath, err := installer.Install(ctx, version, s.sandboxRoot, stream); if err != nil { return nil, fmt.Errorf("sandboxapp.EnsureRuntime: %w", err) }` | A.4 | EDGE | wraps with `%w` but missing the `<spec.Kind>@<version>` context that other wraps include. Style inconsistency vs site #15 / #16 / #17. Sentinel chain (ErrRuntimeInstallFailed from infra/sandbox) preserved through `%w`. | LOW | identical UX (sentinel reaches errmap); harder to grep call site | wrap: `return nil, fmt.Errorf("sandboxapp.EnsureRuntime: install %s@%s: %w", spec.Kind, version, err)` for consistency | FOUND |
| 19 | sandbox.go:430 | `ID: idgenpkg.New("sr")` | A.3 | OK | §S15 canonical: idgenpkg.New with "sr" prefix (sandbox runtime). The "sr" prefix isn't on the §S15 spec list explicitly (the list mentions aki/cv/msg/blk/etc.) but spec is illustrative not exhaustive — sandbox infra layer using sr_ for runtime IDs is consistent with the documented prefix scheme. idgenpkg internally panics on rand.Read fail per §S15. | N-A | — | — | — |
| 20 | sandbox.go:439-441 | `if err := s.repo.CreateRuntime(ctx, runtime); err != nil { return nil, fmt.Errorf("sandboxapp.EnsureRuntime: persist %s@%s: %w", spec.Kind, version, err) }` | A.4 | OK | §S16 canonical with context | N-A | — | — | — |
| 21 | sandbox.go:454-455 | `if !s.IsReady() { return nil, fmt.Errorf("sandboxapp.EnsureEnv: %w", sandboxdomain.ErrEnvCreateFailed) }` | A.4 | OK | §S16 canonical; sentinel errmap.go:103 → 502 SANDBOX_ENV_CREATE_FAILED | N-A | — | — | — |
| 22 | sandbox.go:457-459 | `if owner.Kind == "" \|\| owner.ID == "" { return nil, fmt.Errorf("sandboxapp.EnsureEnv: missing owner.Kind or owner.ID") }` | A.4 | EDGE | §S16: pkg.Method prefix ✓; **NO sentinel**, **NO `%w`**. Same defensive-validation pattern as the apikey audit's tester.go:#4 (now FIXED to panic) and the mcp audit's mcp.go:#6 (still FOUND). Reachability: owner.Kind/ID are filled by callers (forge / mcp / chat) before EnsureEnv; this is wiring-bug defensive. **However**: if triggered, hits "unmapped domain error" alarm + 500 INTERNAL_ERROR. Consistent with mcp.go:#6 EDGE — same fix question (sentinel vs panic). | LOW | hits unmapped warning if triggered (programmer-side bug, not user input) | introduce `sandboxdomain.ErrOwnerRequired` + register errmap as 400, OR panic per "config-time invariant" pattern (same call as mcp.go:#6) | **FIXED 2026-05-10 e36f890** (resolved as panic — wiring-bug invariant) |
| 23 | sandbox.go:469-471 | `if strings.ContainsAny(owner.ID, ":;= \t\n\r\x00") { return nil, fmt.Errorf("sandboxapp.EnsureEnv: owner.ID contains PATH-meta or whitespace character: %q", owner.ID) }` | A.4 | EDGE | §S16: pkg.Method prefix ✓; **NO sentinel**, **NO `%w`**. **POST-FIX context**: this is the validation regression test from B1 fix (commit 3cdf18a `:` → `_`). The error is user-facing (LLM bash auto-route hands an owner.ID; if it contained `:` somewhere we'd reject). Same sentinel/panic decision as #22 — but this one IS reachable in practice (LLM input). Should have a sentinel. | LOW | hits unmapped warning if triggered (rare — owner.ID generated by sandbox internally; only path is bash auto-route) | introduce `sandboxdomain.ErrInvalidOwnerID` + register errmap as 400 BAD_REQUEST so the 500 unmapped warn doesn't fire | **FIXED 2026-05-10 e36f890** (introduced sandboxdomain.ErrInvalidOwnerID, registered errmap.go:108 as 400 SANDBOX_INVALID_OWNER_ID, wrapped at site with %w) |
| 24 | sandbox.go:479-491 | `if existing, err := s.repo.FindEnvByOwner(ctx, owner.Kind, owner.ID); err == nil { ... destroy stale ... } else if !errors.Is(err, sandboxdomain.ErrEnvNotFound) { return nil, fmt.Errorf("sandboxapp.EnsureEnv: lookup %s/%s: %w", owner.Kind, owner.ID, err) }` | A.4 | OK | §S16 canonical; ErrEnvNotFound is the "not found, build new env" branch (correct). | N-A | — | — | — |
| 25 | sandbox.go:486-488 | `if err := s.destroyLocked(ctx, owner, existing); err != nil { return nil, fmt.Errorf("sandboxapp.EnsureEnv: destroy stale: %w", err) }` | A.4 | OK | §S16 canonical with sub-tag "destroy stale" | N-A | — | — | — |
| 26 | sandbox.go:495-498 | `rt, err := s.EnsureRuntime(ctx, spec.Runtime, stream); if err != nil { return nil, fmt.Errorf("sandboxapp.EnsureEnv: %w", err) }` | A.4 | EDGE | wraps but missing context (no kind/version). Style inconsistency. Same pattern as #18. | LOW | identical UX | wrap with context: `return nil, fmt.Errorf("sandboxapp.EnsureEnv: ensure runtime %s: %w", spec.Runtime.Kind, err)` | FOUND |
| 27 | sandbox.go:503-504 | `if !ok { return nil, fmt.Errorf("sandboxapp.EnsureEnv %s: no env manager registered: %w", spec.Runtime.Kind, sandboxdomain.ErrRuntimeNotSupported) }` | A.4 | OK | §S16 canonical with sentinel | N-A | — | — | — |
| 28 | sandbox.go:507 | `envID := idgenpkg.New("se")` | A.3 | OK | §S15 canonical: "se" prefix for sandbox env. Same prefix-scheme rationale as #19. | N-A | — | — | — |
| 29 | sandbox.go:526-528 | `if err := s.repo.CreateEnv(ctx, env); err != nil { return nil, fmt.Errorf("sandboxapp.EnsureEnv: persist row: %w", err) }; s.publishEnv(ctx, env) // status=installing` | A.2/A.4 | OK | §S16 canonical wrap. **§S9 verdict**: this is a "create installing-state row" write — uses request ctx (parameter). However, EnsureEnv is **not** a terminal write per §S9: cancel mid-CreateEnv would mean user aborted install before any work happened; partial state acceptable (next request retries from scratch). The terminal write is the status=ready transition at site #32. | N-A | — | — | — |
| 30 | sandbox.go:531-535 | `runtimePath := filepath.Join(s.sandboxRoot, rt.Path); if err := em.CreateEnv(ctx, runtimePath, envPath); err != nil { s.markEnvFailed(ctx, env, err); return nil, fmt.Errorf("sandboxapp.EnsureEnv create: %w", err) }` | A.4 | OK | §S16 canonical with sub-tag; markEnvFailed updates DB row + publishEnv before propagating. Failure path is durable. | N-A | — | — | — |
| 31 | sandbox.go:536-545 | `if err := em.InstallDeps(...); err != nil { s.markEnvFailed(ctx, env, err); return nil, fmt.Errorf("sandboxapp.EnsureEnv deps: %w", err) }; if len(spec.Extras) > 0 { if err := em.InstallExtras(...); err != nil { s.markEnvFailed(ctx, env, err); return nil, fmt.Errorf("sandboxapp.EnsureEnv extras: %w", err) } }` | A.4 | OK | same pattern as #30 — fail path goes through markEnvFailed (which uses request ctx — see §S9 site #36). Wrap is canonical. | N-A | — | — | — |
| 32 | sandbox.go:547-554 | `env.Status = sandboxdomain.EnvStatusReady; env.SizeBytes = computeDirSize(envPath); env.UpdatedAt = time.Now(); if err := s.repo.UpdateEnv(ctx, env); err != nil { return nil, fmt.Errorf("sandboxapp.EnsureEnv: persist ready: %w", err) }; s.publishEnv(ctx, env) // status=ready` | **A.2** | **EDGE** | **§S9 question**: ready-state write uses request ctx. **However**: install completion is *the* user-waiting flow — caller (chat install_mcp_server tool / forge.RunForge / etc.) blocks on this call. If the request ctx is cancelled before this write lands, the env row stays at status=installing (eternal phantom). Next install attempt finds the stale row and triggers destroy+rebuild via line 486 (which is correct). **Verdict**: NOT a §S9 violation strictly — the cancelled-write recovery path exists (rebuild-on-deps-mismatch). But this is fragile: a user who cancels mid-install and never re-tries leaves a phantom. Comparable to apikey.Test's pre-fix state. | LOW | env row stays at status=installing forever if request cancelled mid-write; cleaned up on next install attempt for same owner OR by sandbox GC after LastUsedAt expiry. Self-healing but slow. | use detached: `detached := reqctxpkg.SetUserID(context.Background(), uid)` (need owner→uid mapping somewhere; or skip uid since CreateEnv didn't take one) — but the owner.Kind/ID is sufficient identity. Could just `context.Background()` for the final write since this isn't user-scoped DB access (env table doesn't filter by uid). | **FIXED 2026-05-10 e36f890** (used `context.Background()` directly for ready-transition UpdateEnv since env table doesn't filter by uid; matches spawn.go::trackedHandle.unregister model §S9 example) |
| 33 | sandbox.go:566-572 | `Destroy: existing, err := s.repo.FindEnvByOwner(...); if errors.Is(err, sandboxdomain.ErrEnvNotFound) { return nil }; if err != nil { return fmt.Errorf("sandboxapp.Destroy: lookup %s/%s: %w", owner.Kind, owner.ID, err) }` | A.4 | OK | §S16 canonical; ErrEnvNotFound idempotent return-nil path is documented "not an error" semantic. | N-A | — | — | — |
| 34 | sandbox.go:582-585 | `destroyLocked: envPath := filepath.Join(s.sandboxRoot, env.Path); if err := removeAll(envPath); err != nil { s.log.Warn("sandbox destroy: rm env dir failed (continuing to delete row)", zap.String("path", envPath), zap.Error(err)) }` | A.1 | OK | §S3 documented soft-fail with audit log; continues to row delete → manifest consistency wins. | N-A | — | — | — |
| 35 | sandbox.go:586-589 | `if err := s.repo.DeleteEnv(ctx, env.ID); err != nil { return fmt.Errorf("sandboxapp.Destroy: delete row %s: %w", env.ID, err) }; s.publishEnvDeleted(ctx, env.ID); return nil` | A.4 | OK | §S16 canonical | N-A | — | — | — |
| 36 | sandbox.go:599-609 | `markEnvFailed: env.Status = sandboxdomain.EnvStatusFailed; env.ErrorMsg = cause.Error(); env.UpdatedAt = time.Now(); if err := s.repo.UpdateEnv(ctx, env); err != nil { s.log.Warn("sandbox: failed-status persist failed", zap.String("env_id", env.ID), zap.Error(err)) }; s.publishEnv(ctx, env) // status=failed` | **A.2** | **EDGE** | **§S9 verdict**: terminal-state write of "this env failed" uses request ctx. Same concern as #32 — if cancelled, env row stays at installing while disk is partially built. **But**: markEnvFailed is called from EnsureEnv's failure paths (#30, #31). The original failure already occurred; whether the markEnvFailed write lands or not, the caller gets an error back. If write fails, next install for same owner triggers deps-drift rebuild (line 486 destroyLocked path). Self-healing same as #32. | LOW | env row stays at installing forever if request cancelled between em.CreateEnv-fail and markEnvFailed write; cleaned up by next install or GC | same fix question as #32: detached ctx for the failed-state persist. publishEnv too if want consistent state. | FOUND |
| 37 | sandbox.go:626-645 | `publishEnv: ...; s.notif.Publish(ctx, "sandbox_env", env.ID, map[string]any{...})` | A.1/A.2 | OK | nil-check at line 627-629 (notif==nil → no-op). Publish failure is the notif's responsibility (notificationspkg.Publisher contract — fire-and-forget); no return here means caller doesn't see publish failures. **§S9 not applicable** — notification is best-effort UI signal, not "must persist". §S3 OK because publisher contract handles failures internally with WARN. | N-A | — | — | — |
| 38 | sandbox.go:647-655 | `publishEnvDeleted: ...; s.notif.Publish(ctx, "sandbox_env", envID, map[string]any{"id": envID, "deleted": true})` | A.1/A.2 | OK | same pattern as #37 | N-A | — | — | — |
| 39 | sandbox.go:664-670 | `envRuntimeKind: rt, err := s.repo.GetRuntime(context.Background(), env.RuntimeID); if err != nil \|\| rt == nil { return "" }` | A.1 | EDGE | §S3: err discarded with implicit "return empty string" — but file-header comment lines 657-663 documents this exact intent: "Best-effort — failures (rare; only if the runtime row was deleted out from under the env) yield "" rather than blocking the notification publish." Documented intent. **However**: no log even at Debug level; if this fires we have no diagnostic. Could add s.log.Debug for traceability. | LOW | none in practice — runtime row delete with surviving env row would be a manifest consistency violation that should panic somewhere upstream | optional Debug log: `s.log.Debug("sandbox: envRuntimeKind lookup failed (rare manifest inconsistency)", zap.String("env_id", env.ID), zap.String("runtime_id", env.RuntimeID), zap.Error(err))` — WAIVE-eligible | FOUND |
| 40 | sandbox.go:665 | `s.repo.GetRuntime(context.Background(), env.RuntimeID)` (uses Background ctx) | A.2 | OK | **Notable §S9 detail**: this read DELIBERATELY uses `context.Background()` instead of the parameter `ctx`. The function is called from publishEnv/publishEnvDeleted which run inside notification publish path (best-effort, fire-and-forget). Using request ctx would let cancellation race with notification publishing — using Background ensures the lookup completes regardless of upstream cancel. This is actually a __correct__ use of detached-ctx pattern (read variant). | N-A | — | — | — |
| 41 | sandbox.go:675-682 | `touchLastUsed: env.LastUsedAt = time.Now(); if err := s.repo.UpdateEnv(ctx, env); err != nil { s.log.Warn("sandbox: touch last_used_at failed", zap.String("env_id", env.ID), zap.Error(err)) }` | A.2 | OK | LastUsedAt update is best-effort (used for GC eligibility). Failure logged + continue. **§S9 not applicable** — this is read-path tracking, not terminal write; if the update fails, GC will incorrectly think env is fresher than it is, which just delays GC by one cycle. Acceptable. | N-A | — | — | — |
| 42 | sandbox.go:687-690 | `kindLock: mu, _ := s.installLocks.LoadOrStore(kind, &sync.Mutex{}); return mu.(*sync.Mutex)` | A.1 | OK | sync.Map.LoadOrStore returns (any, loaded bool) — the bool is "was it already there"; not an error. `_ =` here is the documented sync.Map idiom, not error suppression. §S3 doesn't apply (non-error discard). | N-A | — | — | — |
| 43 | sandbox.go:695-699 | `ownerLock: ...; mu, _ := s.envLocks.LoadOrStore(key, &sync.Mutex{}); return mu.(*sync.Mutex)` | A.1 | OK | same pattern as #42 | N-A | — | — | — |
| 44 | sandbox.go:741-744 | `MarkReadyForTest: s.miseBin = miseBin; s.bootstrapped.Store(true)` | A.1 | OK | test helper with explicit "ForTest" suffix and big don't-call-from-prod warning at lines 731-734. No error path. | N-A | — | — | — |

## Sub-check

A.1 §S3 错误吞没:
  - violations: not present (sites #11, #12, #34, #36 are documented soft-fails with audit log; sites #39, #41 are best-effort with rationale)
  - EDGE/LOW notes: site #39 (envRuntimeKind silent err — could add Debug log; documented best-effort)

A.2 §S9 detached ctx 终态写:
  - terminal-state writes identified: 
    - site #29 (CreateEnv at status=installing — intermediate state)
    - site #32 (UpdateEnv at status=ready — terminal success state)
    - site #36 (UpdateEnv via markEnvFailed at status=failed — terminal failure state)
    - site #35 (DeleteEnv in destroyLocked — terminal removed state)
  - 各自 ctx 来源:
    - sites #29, #32, #36, #35 all use request ctx (parameter passed from caller)
  - violations: 
    - **site #32 (LOW EDGE)** — ready-state write uses request ctx; cancel race could leave env at installing forever (self-healing via deps-drift rebuild but slow)
    - **site #36 (LOW EDGE)** — failed-state write same ctx concern; same self-healing
    - sites #29, #35 are not terminal writes (intermediate / idempotent removal)
  - **Note**: site #40 envRuntimeKind correctly uses context.Background() for the read inside publish path — proper detached pattern.

A.3 §S15 ID 生成:
  - ID generation calls: 
    - site #19 (`idgenpkg.New("sr")` for runtime IDs at line 430)
    - site #28 (`idgenpkg.New("se")` for env IDs at line 507)
  - violations: not present — both use idgenpkg with documented prefixes; sandbox uses "sr_" / "se_" which are not in the §S15 explicit example list but are consistent with the format scheme. Spec list is illustrative ("如 aki_ apikey / mc_ model config / cv_ conversation / msg_ message / att_ attachment / blk_ block / f_ forge / fv_ forge version / tc_ test case / fe_ forge execution / b_ forge test 批跑 batch / td_ todo / bsh_ Bash 后台 shell 进程 / sar_ subagent run / smm_ subagent message").
  - **Cross-package note**: §S15 spec explicitly lists `sar_` for subagent run and `smm_` for subagent message but does NOT list `sr_` or `se_` for sandbox runtime/env. The prefixes follow the convention but the spec list is incomplete. Could add to spec or note as design-intent expansion.

A.4 §S16 错误 wrap 格式:
  - violations: 
    - site #3 (LOW — Bootstrap bare-return)
    - site #4 (LOW — EnsureTool bare-return on inner-already-wrapped err)
    - site #18 (LOW — install wrap missing kind/version context)
    - site #22 (LOW — owner missing validation: no sentinel, no %w; same as mcp.go:#6)
    - site #23 (LOW — owner.ID PATH-meta validation: no sentinel, no %w)
    - site #26 (LOW — EnsureRuntime call inside EnsureEnv missing context)

A.5 §S17 sentinel 登记 errmap:
  - sentinels defined: none in this file
  - 已登记 errmap (consumed):
    - sandboxdomain.ErrRuntimeNotSupported (errmap.go:100, sites #14, #27)
    - sandboxdomain.ErrRuntimeInstallFailed (errmap.go:101, site #13)
    - sandboxdomain.ErrEnvNotFound (errmap.go:102, sites #24, #33)
    - sandboxdomain.ErrEnvCreateFailed (errmap.go:103, site #21)
    - sandboxdomain.ErrEnvInUse (errmap.go:107, site #10)
  - missing: 
    - **suggested new sentinels** for sites #22, #23 (currently anonymous fmt.Errorf): `sandboxdomain.ErrOwnerRequired` + `sandboxdomain.ErrInvalidOwnerID` — see EDGE notes
  - all consumed sentinels properly registered ✓

## Cross-cutting observations

### Detached ctx pattern (§S9) — comparison with apikey

apikey.Test used detached ctx for terminal writes (FIXED in d8a5161). Sandbox EnsureEnv has the **same pattern shape** but uses request ctx for ready/failed state writes. The difference is that:
- **apikey.Test** is a probe — user expects immediate sync result, but the result must persist regardless of UI cancel.
- **sandbox.EnsureEnv** is install — long-running; user is waiting on a streaming response.
- Both write a "what happened" terminal state to DB.

The case for fixing sandbox sites #32 + #36 to use detached ctx is **less clear than apikey** because:
1. There's a self-healing path (next-install destroy+rebuild on deps drift)
2. Sandbox GC eventually cleans up phantoms
3. No false-OK badge surfaces in UI (UI shows env status from DB; phantom rows show "installing" forever which is at worst confusing, not deceptive)

But §S9 spec text "终态写入（必须落库的最后一步）" applies — this IS the terminal write. Recommend FIXED at LOW priority.

### Owner validation sentinels (§S17)

Sites #22 and #23 both produce `fmt.Errorf` with no sentinel. Same pattern as `mcp.go:#6` (still FOUND) and the panic-resolution choice from `apikeytester:#4`. Three options for sandbox:
- (a) Introduce sentinels + register errmap (consistent with sandbox's existing 8-sentinel pattern)
- (b) Panic (config-time invariant — owner.Kind/ID always provided by trusted callers)
- (c) Accept and add `errmap` "fallback for app-level fmt.Errorf"

Recommendation: (a) since these errors are user-reachable (LLM-driven owner.ID validation at site #23 in particular).

### IDgen prefix list completeness (§S15)

Sandbox uses `sr_` (runtime) and `se_` (env) prefixes which are not explicitly in CLAUDE.md §S15's prefix list. The spec is illustrative but doc-and-code drift exists. Recommend either:
- Update CLAUDE.md §S15 to include `sr_` / `se_` (spec-side fix)
- Or document in sandbox.md that these prefixes follow §S15 format scheme

This is a §S14 doc-sync concern, not §S15 violation.

## Spot-check (random clean sites)

Random seed: 6 sites picked from `OK` set:

1. **site #1** (panic on nil logger): verified — exact §S3 carve-out for unrecoverable wiring bugs. Same pattern as apikey.NewService:#1 + mcp.go:#1.
2. **site #10** (ErrEnvInUse wrap): verified — `fmt.Errorf("sandboxapp.DeleteRuntime: %d env(s) still reference %s: %w", len(envs), id, sandboxdomain.ErrEnvInUse)` — pkg.Method prefix ✓ + count + id context + %w + sentinel registered errmap.go:107 → 409. Compliance literal.
3. **site #19** (`idgenpkg.New("sr")`): verified — uses idgenpkg per §S15; "sr_" prefix is documented sandbox runtime ID (sandbox.md §1 mentions `runtimes/` dir hierarchy implying per-runtime IDs). idgenpkg internal panic-on-rand-fail is invariant.
4. **site #34** (rm env dir failure log): verified — `s.log.Warn("sandbox destroy: rm env dir failed (continuing to delete row)", zap.String("path", envPath), zap.Error(err))`. §S3 documented soft-fail with audit log + inline rationale "continuing to delete row" matches §S10 fire-and-forget log requirement.
5. **site #40** (context.Background in publish read): verified — deliberate use of Background instead of request ctx because publishEnv runs in fire-and-forget notification path. The reverse of §S9 detached pattern (read instead of write) but same principle: avoid request-ctx cancel from breaking best-effort publish work.
6. **site #42** (`mu, _ := s.installLocks.LoadOrStore(...)`): verified — `_` discards `loaded bool`, NOT an error. sync.Map idiom; §S3 carve-out for non-error discards.

All 6 spot-checks confirmed correct classification. The audit's primary finds (sites #22, #23 sentinel gaps + #32, #36 §S9 questions) are real concerns surfaced by examining each error site against spec — not artifacts of pattern matching.
