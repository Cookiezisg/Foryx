# audit: backend/internal/app/sandbox/spawn.go

LOC: 330
Read: full file (lines 1-330)

## Trace

| site# | file:line | snippet | category | classification | reasoning | severity | user_impact | suggested_fix | status |
|---|---|---|---|---|---|---|---|---|---|
| 1 | spawn.go:52-56 | `Spawn: cmd, cwd, env, err := s.prepareSpawn(...); if err != nil { return nil, err }` | A.4 | OK | bare-return; prepareSpawn (sites #11-15) wraps everything internally with `sandboxapp.Spawn:` prefix. Sentinel chain preserved. Adding another wrap at this level would just be `sandboxapp.Spawn: sandboxapp.Spawn: ...`. Canonical "delegate-and-passthrough" pattern. | N-A | — | — | — |
| 2 | spawn.go:67-73 | `return sandboxinfra.SpawnOnce(spawnCtx, sandboxinfra.SpawnOptions{...})` | A.4 | OK | bare passthrough to infra layer; infra wraps internally with `sandbox.SpawnOnce:` prefix. | N-A | — | — | — |
| 3 | spawn.go:85-87 | `SpawnLongLived: cmd, cwd, env, err := s.prepareSpawn(...); if err != nil { return nil, err }` | A.4 | OK | same as site #1 | N-A | — | — | — |
| 4 | spawn.go:90-98 | `inner, err := sandboxinfra.SpawnLongLived(...); if err != nil { return nil, err }` | A.4 | EDGE | bare-return — infra/sandbox.SpawnLongLived wraps internally; sentinel preserved. Style inconsistency vs prepareSpawn site #11 which wraps. | LOW | identical UX (sentinel from infra reaches errmap) | wrap: `return nil, fmt.Errorf("sandboxapp.SpawnLongLived: %w", err)` for grep | FOUND |
| 5 | spawn.go:101-105 | `envRow, lookupErr := s.repo.FindEnvByOwner(ctx, owner.Kind, owner.ID); envID := ""; if lookupErr == nil { envID = envRow.ID }` | A.1 | EDGE | §S3: lookupErr DISCARDED with no log. **Documented intent** at line 102-104 ("envID is empty if lookupErr — Layer B leak prevention is best-effort"); however no log means failures are invisible. The Layer B leak prevention (manifest running_pid tracking) won't work for this spawn but the spawn succeeds — could orphan a process at next crash without paper trail. | LOW | Layer B leak prevention silently disabled for this handle if lookup fails; no audit trail of why | add `s.log.Debug("sandbox SpawnLongLived: env lookup for PID tracking failed (Layer B disabled for this handle)", zap.String("owner_kind", owner.Kind), zap.String("owner_id", owner.ID), zap.Error(lookupErr))` so the silent disable surfaces in dev logs | FOUND |
| 6 | spawn.go:123-130 | `if envID != "" { if err := s.repo.SetEnvRunningPID(ctx, envID, inner.PID()); err != nil { s.log.Warn("sandbox: track running pid failed", zap.String("env_id", envID), zap.Int("pid", inner.PID()), zap.Error(err)) } }` | A.1 | OK | §S3 documented soft-fail with audit log + inline rationale lines 116-122. Layer B is best-effort. §S10 fire-and-forget log compliance. | N-A | — | — | — |
| 7 | spawn.go:151-157 | `if err := t.inner.Kill(); err != nil { s.log.Warn("sandbox shutdown: kill handle failed", zap.Int("pid", t.inner.PID()), zap.String("owner_kind", t.owner.Kind), zap.String("owner_id", t.owner.ID), zap.Error(err)) }` | A.1 | OK | §S3 documented soft-fail per Shutdown's contract (lines 134-141). Per-handle kill failure logged + continue to next handle. OS reaping covers any survivors. | N-A | — | — | — |
| 8 | spawn.go:174-180 | `select { case <-done: ...; return nil; case <-ctx.Done(): ...; return ctx.Err() }` | A.4 | EDGE | §S16: `return ctx.Err()` at line 179 returns stdlib context.Canceled or context.DeadlineExceeded directly — NO pkg.Method prefix wrap. Caller (main.go's shutdown hook) likely just logs and exits, but errors.Is(err, context.DeadlineExceeded) works. **Reasoning to NOT VIOLATE**: ctx errors are well-known stdlib sentinels; errmap.go:179-180 maps them. Bare-return preserves sentinel. | LOW | identical UX (errmap maps both stdlib ctx errs); harder to grep call site | wrap: `return fmt.Errorf("sandboxapp.Shutdown: %w", ctx.Err())` for grep traceability | FOUND |
| 9 | spawn.go:190-192 | `if !s.IsReady() { return "", "", nil, fmt.Errorf("sandboxapp.Spawn: %w", sandboxdomain.ErrSpawnFailed) }` | A.4 | OK | §S16 canonical with sentinel registered errmap.go:105 → 502 SANDBOX_SPAWN_FAILED | N-A | — | — | — |
| 10 | spawn.go:193-195 | `if opts.Cmd == "" { return "", "", nil, fmt.Errorf("sandboxapp.Spawn: empty Cmd") }` | A.4 | EDGE | §S16: pkg.Method prefix ✓; **NO sentinel**, **NO `%w`**. Same defensive-validation pattern as sandbox.go:#22 + mcp.go:#6 + apikey.tester.go:#4. Reachability: Spawn callers always provide Cmd (forge / mcp / chat); this is wiring-bug defensive. | LOW | hits "unmapped domain error" alarm + 500 if triggered (programmer-side bug) | introduce sentinel `sandboxdomain.ErrCmdRequired` + register errmap as 400, OR panic per "config-time invariant" — same decision as sandbox.go:#22 | FOUND |
| 11 | spawn.go:197-200 | `envRow, err := s.repo.FindEnvByOwner(ctx, owner.Kind, owner.ID); if err != nil { return "", "", nil, fmt.Errorf("sandboxapp.Spawn: lookup env %s/%s: %w", owner.Kind, owner.ID, err) }` | A.4 | OK | §S16 canonical with context | N-A | — | — | — |
| 12 | spawn.go:201-203 | `if envRow.Status != sandboxdomain.EnvStatusReady { return "", "", nil, fmt.Errorf("sandboxapp.Spawn: env %s status=%s: %w", envRow.ID, envRow.Status, sandboxdomain.ErrSpawnFailed) }` | A.4 | OK | §S16 canonical; sentinel ErrSpawnFailed errmap.go:105 | N-A | — | — | — |
| 13 | spawn.go:205-208 | `rt, err := s.repo.GetRuntime(ctx, envRow.RuntimeID); if err != nil { return "", "", nil, fmt.Errorf("sandboxapp.Spawn: lookup runtime %s: %w", envRow.RuntimeID, err) }` | A.4 | OK | §S16 canonical | N-A | — | — | — |
| 14 | spawn.go:213-215 | `if !ok { return "", "", nil, fmt.Errorf("sandboxapp.Spawn: no env manager for kind %s: %w", rt.Kind, sandboxdomain.ErrRuntimeNotSupported) }` | A.4 | OK | §S16 canonical | N-A | — | — | — |
| 15 | spawn.go:226-231 | `envRow.LastUsedAt = time.Now(); if updateErr := s.repo.UpdateEnv(ctx, envRow); updateErr != nil { s.log.Warn("sandbox: spawn touch last_used_at failed", zap.String("env_id", envRow.ID), zap.Error(updateErr)) }` | A.1/A.2 | OK | LastUsedAt update; same pattern as sandbox.go:#41 (touchLastUsed). Best-effort, logged. **§S9 not applicable** — touch is read-path tracking metadata not terminal write. | N-A | — | — | — |
| 16 | spawn.go:301-305 | `Wait: err := t.inner.Wait(); t.unregister(); return err` | A.4 | OK | bare-return; inner.Wait wraps internally per infra contract. unregister is bookkeeping side-effect, not error path. | N-A | — | — | — |
| 17 | spawn.go:307-311 | `Kill: err := t.inner.Kill(); t.unregister(); return err` | A.4 | OK | same as #16 | N-A | — | — | — |
| 18 | spawn.go:319-329 | `unregister: t.service.activeHandles.Delete(t.id); if t.envID == "" { return }; if err := t.service.repo.ClearEnvRunningPID(context.Background(), t.envID); err != nil { t.service.log.Warn("sandbox: clear running pid failed", zap.String("env_id", t.envID), zap.Error(err)) }` | A.1/A.2 | OK | **§S9 detached pattern correctly applied**: line 324 uses `context.Background()` for ClearEnvRunningPID write — explicitly detached from request ctx because unregister() is called from Wait/Kill which may run after request ctx expired. **This is a model correct §S9 implementation.** Best-effort log on failure per §S10. | N-A | — | — | — |

## Sub-check

A.1 §S3 错误吞没:
  - violations: not present
  - EDGE/LOW: site #5 (lookupErr discarded for Layer B PID tracking — documented intent but no log; LOW with optional Debug log fix)

A.2 §S9 detached ctx 终态写:
  - terminal-state writes identified:
    - site #15 (LastUsedAt UpdateEnv during Spawn — touch metadata, NOT terminal)
    - site #18 (ClearEnvRunningPID in unregister — terminal cleanup) ← **uses context.Background() correctly**
  - 各自 ctx 来源:
    - site #15 uses request ctx (touch is best-effort metadata; not terminal)
    - site #18 uses context.Background() (correctly detached because Wait/Kill outlives request)
  - violations: not present — site #18 is a model correct §S9 implementation
  - **Notable**: spawn.go gets §S9 right where sandbox.go EnsureEnv has the question-mark sites #32/#36

A.3 §S15 ID 生成:
  - ID generation calls: none
  - violations: N/A — file does not generate business IDs (uses existing env IDs via FindEnvByOwner)

A.4 §S16 错误 wrap 格式:
  - violations:
    - site #4 (LOW — SpawnLongLived bare-return on inner err)
    - site #8 (LOW — Shutdown returns ctx.Err() bare; stdlib sentinel preserved, just lacks pkg.Method prefix)
    - site #10 (LOW — empty Cmd validation: no sentinel, no %w; same as sandbox.go:#22 + mcp.go:#6)

A.5 §S17 sentinel 登记 errmap:
  - sentinels defined: none in this file
  - 已登记 errmap (consumed):
    - sandboxdomain.ErrSpawnFailed (errmap.go:105, sites #9, #12)
    - sandboxdomain.ErrRuntimeNotSupported (errmap.go:100, site #14)
  - missing:
    - **suggested new sentinel** for site #10 (currently anonymous fmt.Errorf): `sandboxdomain.ErrCmdRequired` (or similar) — same family as sandbox.go:#22 sentinel-gap concern
  - all consumed sentinels properly registered ✓

## Cross-cutting observations

### §S9 model implementation (site #18)

`trackedHandle.unregister()` at line 319-329 is a **textbook correct §S9** detached-ctx implementation:
- Wait/Kill may be called long after the original request ctx expired
- Uses `context.Background()` explicitly for the cleanup write
- Failure logged with audit context + continues (graceful)

This contrasts with sandbox.go EnsureEnv sites #32/#36 which DO use request ctx for ready/failed terminal writes. Both are by the same author; spawn.go got it right. The fix recommendation for sandbox.go is "follow spawn.go's pattern".

### Consistency of sentinel-gap pattern

Three sites in this package use anonymous `fmt.Errorf` for defensive validation:
- spawn.go:#10 (empty Cmd)
- sandbox.go:#22 (missing owner.Kind/ID)
- sandbox.go:#23 (PATH-meta in owner.ID)

All three would benefit from sentinels. Whether to add them OR panic is the same decision as `mcp.go:#6` and (resolved as panic in `apikey.tester.go:#4`). Recommend coordinated decision across all 3-4 sites.

## Spot-check (random clean sites)

Random seed: 5 sites picked from `OK` set:

1. **site #6** (SetEnvRunningPID failure log): verified — `s.log.Warn("sandbox: track running pid failed", zap.String("env_id", envID), zap.Int("pid", inner.PID()), zap.Error(err))`. §S3 documented soft-fail with full audit context (env_id + pid + error). Lines 116-122 inline comment explains why best-effort is acceptable here. §S10 compliance.
2. **site #11** (lookup env wrap): verified — `fmt.Errorf("sandboxapp.Spawn: lookup env %s/%s: %w", owner.Kind, owner.ID, err)`. pkg.Method prefix + sub-tag + owner context + %w. Compliance literal.
3. **site #14** (no env manager wrap): verified — `fmt.Errorf("sandboxapp.Spawn: no env manager for kind %s: %w", rt.Kind, sandboxdomain.ErrRuntimeNotSupported)`. pkg.Method + context + sentinel registered errmap.go:100. Errors.Is unwrap chain correct.
4. **site #18** (model §S9): verified — exact `context.Background()` form for Wait/Kill cleanup. The pattern is canonical: bookkeeping cleanup must outlive request ctx.
5. **site #15** (LastUsedAt touch best-effort log): verified — `s.log.Warn("sandbox: spawn touch last_used_at failed", zap.String("env_id", envRow.ID), zap.Error(updateErr))`. Same touchLastUsed pattern as sandbox.go:#41. §S9 not applicable (touch is metadata-only; GC self-corrects on next cycle).

All 5 spot-checks confirmed correct classification — mechanism functioning, not rubber-stamping.
