# Package audit summary: internal/app/apikey

## Spec understanding (Phase A — §S3 / §S9 / §S15 / §S16 / §S17)

- **§S3 错误不吞**: any path where suppression of error → user-visible wrong state / data loss / config failure is forbidden. Bare `_ = err` requires inline justification comment. `defer X.Close()` on read-only resources is acceptable. Soft-degrade on parse failure must at least log at WARN — silent fallthrough is the §S3 prohibition.
- **§S9 detached ctx 终态写**: writes that finalize a user-visible state (test result, final assistant message, audit log) must use `reqctxpkg.SetUserID(context.Background(), uid)` — the request ctx may be cancelled by browser hard-refresh / tab close, and a cancelled write leaves the row at stale prior status with no audit trail.
- **§S15 ID 生成**: business IDs go through `idgenpkg.New(prefix)`; `crypto/rand` only; `rand.Read` failure must `panic` (entropy corruption produces collision IDs).
- **§S16 错误 wrap 格式**: `fmt.Errorf("<pkg>.<Method>: %w", err)` — must include pkg.method prefix AND %w (not %v/%s); bare-return of upstream sentinel is acceptable when sentinel preserved; never `errors.New("foo: " + err.Error())`.
- **§S17 errmap 单一事实源**: every sentinel that can reach a handler via `responsehttpapi.FromDomainError` must have an `errTable` row in `errmap.go`; otherwise it triggers the "unmapped domain error" ERROR-level alarm.

## Files audited

| File | LOC | Sites | OK | POST-FIX | VIOLATION | EDGE |
|---|---|---|---|---|---|---|
| apikey.go | 296 | 30 | 25 | 3 | 1 | 3 |
| tester.go | 412 | 13 | 9 | 0 | 1 | 5 |
| providers.go | 145 | 3 | 3 | 0 | 0 | 0 |
| **TOTAL** | **853** | **46** | **37** | **3** | **2** | **8** |

## Severity breakdown (only VIOLATION + EDGE)

| Severity | Count | Sites | Status |
|---|---|---|---|
| HIGH | 1 | apikey.go:#28 (MarkInvalid §S9 violation — terminal write uses raw ctx) | **FIXED 410f664** |
| MED | 1 | tester.go:#4 (line 112 default branch — no %w / no sentinel; will pollute "unmapped domain error" alarm) | **FIXED 410f664** (panic) |
| LOW | 5 | apikey.go:#7 (validateCreate prefix style), #17 (Test bare-return inconsistency vs Create wrap), #24 (Test failure path bare-return); tester.go:#2, #3 (`apikeytester:` prefix vs `<pkg>.<Method>:` form) | **FIXED 1b96a5e** |
| LOW (waived) | 2 | tester.go:#12, #13 (silent JSON parse on connectivity probe) | **WAIVED 2026-05-09** — soft-degrade is documented design intent for connectivity probing; logging would add noise without enabling action |

## Cross-cutting

### Sentinel chain integrity (§S17)
- **All consumed sentinels (apikeydomain.Err*, reqctxpkg.ErrMissingUserID) are registered** in errmap.go (verified row-by-row at errmap.go:45-52, 163).
- **Gap**: tester.go site #4 (line 112 default branch) returns a plain `fmt.Errorf` with no sentinel — when triggered, this hits the `unmapped` path. Not a missing-registration issue (no sentinel exists to register); fix is to introduce one or panic.

### Detached ctx coverage (§S9)
- **Test()** (POST-FIX OK as of d8a5161, 2026-05-09): three terminal writes all use `detached`. ✓
- **MarkInvalid()** (VIOLATION): mirrors Test()'s defect class — `s.repo.UpdateTestResult(ctx, ...)` on the **request ctx**. When the upstream caller (e.g. chat hits 401 mid-stream) gets ctx-cancelled simultaneously, MarkInvalid silently drops the status flip; the API key shows green "OK" in UI while actually being invalid. The d8a5161 fix only covered Test, not MarkInvalid — same bug pattern survives.

### Style consistency
- Wrap-vs-bare-return inconsistency in apikey.go: Create() wraps with `fmt.Errorf("apikey.Service.Create: %w", err)` (lines 88, 92) but Test() bare-returns (lines 184, 222). Both preserve sentinel chain so neither violates §S16 strictly. Project should pick one and standardize. The Create-style wrap adds call-site context to error messages and should be the canonical pattern.
- tester.go uses `apikeytester:` as error prefix — descriptive but not the canonical `apikey.HTTPTester.Test:` form per §S16.

## Spot-check (random clean sites — verify mechanism not rubber-stamping)

Random seed: 7 sites picked from `OK`/`POST-FIX OK` set:

1. **apikey.go:#3** (line 86-89, RequireUserID wrap in Create): verified canonical — pkg.method prefix `apikey.Service.Create:` + `%w`; sentinel `reqctxpkg.ErrMissingUserID` is in errmap.go:163. Compliance literal.
2. **apikey.go:#5** (line 96/295, newID): verified `idgenpkg.New("aki")` — `aki` matches §S15 spec list ("aki_ apikey"). Implementation panic-on-rand-fail handled inside idgenpkg per spec.
3. **apikey.go:#20** (line 198, detached): verified — exact `reqctxpkg.SetUserID(context.Background(), uid)` pattern from §S9 spec. Doc comment at lines 168-179 cites §S9 explicitly.
4. **apikey.go:#26** (line 244, ResolveCredentials decrypt wrap): verified `fmt.Errorf("apikey.Service.ResolveCredentials: decrypt: %w", err)` — pkg.method prefix + %w + sentinel preserved through encryptor's inner wrap chain.
5. **tester.go:#10** (line 339, defer resp.Body.Close()): verified — §S3 spec example explicitly carves this out as exception. `resp.Body.Close()` after read on read-only HTTP response body — Close error is unactionable (data already consumed).
6. **tester.go:#9** (line 336-338, network err bare return): verified — lowest-level network error has no sentinel chain to preserve; bare return is canonical Go idiom for transport errors. Caller (probes) converts to TestResult.Message via err.Error().
7. **providers.go:#2** (line 127-130, IsValidProvider): verified — `_, ok := providers[name]` — `_` discards the **ProviderMeta** value, not an error. §S3 doesn't apply to non-error discards.

All 7 spot-checks confirmed correct classification — mechanism is functioning, not rubber-stamping.
