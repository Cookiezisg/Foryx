# Package audit summary: internal/app/mcp

## Spec understanding (Phase A — §S3 / §S9 / §S15 / §S16 / §S17)

- **§S3 错误不吞**: error suppression that hides user-visible failure / data loss / config drift is forbidden. `_ = err` requires inline justification. `defer X.Close()` on read-only resources is fine. Documented best-effort soft-fails (subprocess close on shutdown, async per-server connect on Start/Import) require explicit zap.Warn with audit context. Silent fallthrough is the canonical anti-pattern.
- **§S9 detached ctx 终态写**: terminal-state writes that MUST persist regardless of caller-cancel use `reqctxpkg.SetUserID(context.Background(), uid)`. This package's "terminal" writes are mostly **synchronous user-waits-on-HTTP-response** flows (AddServer / RemoveServer / Reconnect / InstallFromRegistry write to mcp.json file), where partial state is recoverable on next request. mcp ServerStatus is **in-memory only** per mcp.md §5.6 — health counter mutations (calltool.recordCallResult) are not "terminal writes" in §S9 sense.
- **§S15 ID 生成**: package does NOT generate business IDs. Server identity is the curated registry slug (`playwright`, `notion`, etc.) per mcp.md §5.5 single-Name-field decision; not subject to §S15 `<prefix>_<16hex>` format.
- **§S16 错误 wrap 格式**: `fmt.Errorf("<pkg>.<Method>: %w", err)` canonical. Bare `return err` preserves sentinel chain but loses call-site loc — flagged LOW for style consistency. The major MED finding is `%w: %v` (drops inner sentinel chain) at install.go #5.
- **§S17 errmap 单一事实源**: All 14 mcpdomain sentinels (errmap.go:127-142) are registered. `ErrSearchServerUnavailable` (searchrouter.go:52) is app-layer, intentionally unregistered per its own doc comment (web layer wraps it). All consumed sentinels (ErrServerNotFound, ErrServerNotConnected, ErrToolNotFound, ErrAlreadyInstalled, ErrRequiredEnvMissing, ErrRequiredArgsMissing, ErrInstallFailed, ErrRegistryEntryNotFound, ErrHandshakeFailed) are at errmap.go.

## Files audited

| File | LOC | Sites | OK | POST-FIX | VIOLATION | EDGE |
|---|---|---|---|---|---|---|
| calltool.go | 333 | 18 | 17 | 0 | 0 | 1 |
| catalogsource.go | 123 | 3 | 3 | 0 | 0 | 0 |
| install.go | 287 | 14 | 10 | 0 | 0 | 4 |
| mcp.go | 553 | 30 | 22 | 0 | 0 | 8 |
| searchrouter.go | 92 | 5 | 2 | 0 | 0 | 3 |
| **TOTAL** | **1388** | **70** | **54** | **0** | **0** | **16** |

## Severity breakdown (only VIOLATION + EDGE)

| Severity | Count | Sites | Status |
|---|---|---|---|
| HIGH | 0 | — | — |
| MED | 1 | install.go:#5 (`%w: %v` loses inner sandbox sentinel chain in InstallFromRegistry — caller can't `errors.Is(err, sandboxdomain.ErrRuntimeNotSupported)` despite that being the underlying cause) | FOUND |
| LOW | 15 | calltool.go:#17 (parseRankedIndices missing pkg prefix); install.go:#1, #7 (bare-return); install.go:#8 (silent Close); mcp.go:#6 (no-sentinel "name required"); mcp.go:#7, #12, #15, #20, #21 (silent `_ = c.Close()` orphan-subprocess pattern × 5); mcp.go:#19 (`connectOne:` missing `mcpapp.` qualifier); mcp.go:#20a, #21a (bare return on Initialize/ListTools fail); mcp.go:#27 (ListRegistry bare passthrough); searchrouter.go:#2 (GetServer err translation drops original); searchrouter.go:#3 (`(status=%s)` prefix without pkg.Method); searchrouter.go:#4 (`json.Marshal` `_` discard without §S3 justification comment) | FOUND |

## Cross-cutting

### Sentinel chain integrity (§S17)

All 14 mcpdomain sentinels (errmap.go:127-142) verified registered by file:

| Sentinel | errmap.go line | First consumed in |
|---|---|---|
| `ErrServerNotFound` | 127 | calltool.go:#1, mcp.go:#11/15/19/24/25 |
| `ErrServerNotConnected` | 128 | calltool.go:#2 |
| `ErrToolNotFound` | 129 | calltool.go:#3 |
| `ErrToolCallFailed` | 130 | (consumed via client.CallTool wrap) |
| `ErrToolCallTimeout` | 131 | (consumed via client.CallTool wrap) |
| `ErrRegistryEntryNotFound` | 132 | install.go:#1 (via GetRegistryEntry) |
| `ErrRuntimeMissing` | 133 | (consumed via sandbox.EnsureEnv chain) |
| `ErrRequiredEnvMissing` | 134 | install.go:#3 |
| `ErrRequiredArgsMissing` | 135 | install.go:#4 |
| `ErrInstallFailed` | 136 | install.go:#5 (with the `%w: %v` chain-loss issue) |
| `ErrMarketplaceUnavailable` | 139 | (consumed only when source switches to network impl) |
| `ErrAlreadyInstalled` | 140 | install.go:#2 |
| `ErrUnsupportedRuntime` | 141 | (consumed via sandbox chain) |
| `ErrHandshakeFailed` | 142 | mcp.go:#20 (via client.Initialize chain) |

**No missing registrations**. The only sentinel-chain concern is install.go:#5's `%w: %v` — should be `%w: %w` (Go 1.20+) to preserve the inner sandbox sentinel for callers using `errors.Is` to discriminate (e.g. UI showing different error UX for "runtime not installable" vs "package fetch failed").

### Detached ctx coverage (§S9) — context-by-context analysis

**Terminal-state write inventory:**

| Write | File / Site | Ctx | §S9 verdict |
|---|---|---|---|
| mcp.json save (AddServer) | mcp.go:#8 (mcpinfra.Save) | request ctx | ✓ OK — synchronous user-waits flow; cancel = abort install (recoverable) |
| mcp.json save (RemoveServer) | mcp.go:#13 | request ctx | ✓ OK |
| mcp.json save (Import) | install.go:#9 | request ctx | ✓ OK |
| connectOne (AddServer) | mcp.go:#10 (cctx WithTimeout(ctx, 3min)) | derived from request ctx | ✓ OK — abort on cancel; ServerStatus stays disconnected; user re-tries |
| connectOne (Reconnect) | mcp.go:#17 (cctx WithTimeout(ctx, 30s)) | derived from request ctx | ✓ OK |
| connectOne async (Start, Import) | mcp.go:#4, install.go:#10 | derived from request ctx via WithTimeout | ✓ OK — Start ctx is process lifetime; Import ctx is request but cancel = abort connect, leaves disconnected |
| sandbox.EnsureEnv (InstallFromRegistry) | install.go:#5 | request ctx | ✓ OK — cancel mid-install = recoverable; user re-runs |
| publishStatus / notif.Publish | mcp.go:#9, #14, #18 | request ctx | ✓ OK — best-effort SSE; loss-on-cancel is acceptable (UI reads ServerStatus on next poll regardless) |
| ServerStatus mutations (in-memory) | calltool.go:#15, mcp.go:#16, #20-22 | (no ctx, in-memory only) | ✓ OK — mcp.md §5.6 explicitly says health counters are volatile |

**No §S9 violations** in this package. Contrast with apikey.Test where the user-visible status flip MUST land regardless of cancel because next-time-the-page-loads visibility depends on it — install/uninstall flows here have the user's response payload BE the terminal state acknowledgment (201 / err), so cancel naturally aborts.

### Pattern: MCP subprocess Close discard (cluster of 5 LOW EDGE)

Cross-cutting cleanup pattern repeated 5× across mcp.go + 1× in install.go:

| Site | Context |
|---|---|
| mcp.go:#7 | AddServer replace path |
| mcp.go:#12 | RemoveServer |
| mcp.go:#15 | Reconnect |
| mcp.go:#20 | connectOne Initialize-fail cleanup |
| mcp.go:#21 | connectOne ListTools-fail cleanup |
| install.go:#8 | Import overwrite=true replace path |

Same fix everywhere: `_ = c.Close()` → `if err := c.Close(); err != nil { s.log.Warn("...", zap.String("server", name), zap.Error(err)) }`.

Risk: orphaned MCP subprocess that survives state mutation; OS reaps when parent exits but until then duplicates run. Per mcp.md §3 "no auto-restart" + §5.7 "Disconnect 时清理 in-flight" the design intent is **loud cleanup failure**, not silent — fix would align with documented design. Recommended single sweep commit.

### Sentinel translation at routing boundary (searchrouter.go #2)

`SearchRouter.CallSearchTool` translates `mcp.GetServer ErrServerNotFound` → `ErrSearchServerUnavailable` for web-tool consumption. Documented intent (lines 44-51): web layer wraps this further into its own ErrMCPSearchUnavailable so web doesn't import mcp domain. Translation is at boundary, sentinel chain stays clean from a §S17 standpoint. **Cross-fork concern**: confirm app/tool/web fork audit verifies the wrap chain — if any handler ever calls SearchRouter.CallSearchTool directly, ErrSearchServerUnavailable becomes unmapped.

## Spot-check (random clean sites — verify mechanism not rubber-stamping)

Random seed: 7 sites picked from `OK` set across all 5 files:

1. **calltool.go:#1** (CallTool ErrServerNotFound at line 56-58): verified — `fmt.Errorf("mcpapp.CallTool: %w: %q", mcpdomain.ErrServerNotFound, server)`. pkg.Method prefix ✓, %w ✓, sentinel registered errmap.go:127. Compliance literal.
2. **calltool.go:#14** (`_ context.Context` parameter): verified — this is **unused function parameter** (interface compatibility for future use), NOT error discard. `_ context.Context` reads as "ctx not used in body" not as "ignored err". §S3 doesn't apply.
3. **catalogsource.go:#1** (ListItems return nil): verified — function signature has error return for `catalogdomain.CatalogSource` interface compliance; ListServers is RWMutex-protected map read with zero failure modes. Returning `(items, nil)` for all-success paths is canonical Go.
4. **install.go:#2** (ErrAlreadyInstalled): verified — `fmt.Errorf("mcpapp.InstallFromRegistry %s: %w", name, mcpdomain.ErrAlreadyInstalled)`. pkg.Method prefix + %w + sentinel registered errmap.go:140 → 409 MCP_ALREADY_INSTALLED. Errors.Is unwrap to sentinel ✓.
5. **mcp.go:#1** (panic on nil): verified — `if log == nil { panic("mcp.New: logger is nil") }` boot-time wiring guard. §S3 spec carves out panic for unrecoverable init-time invariants (same pattern as apikey.NewService panic on nil-logger). Caught at app boot, not runtime.
6. **mcp.go:#4** (async Connect Warn log): verified — `s.log.Warn("mcp connect failed", zap.String("server", n), zap.Error(err))` provides audit trail per §S10 "异步或 fire-and-forget 必须打". connectOne (lines 380-403) ALSO sets `state.LastError = err.Error()` providing second audit source via ServerStatus. Per mcp.md §5.6 "fail-loud, no auto-restart" — exactly the documented contract.
7. **mcp.go:#25** (Stderr "configured-but-not-connected" returns ("", nil)): verified — design intent at lines 451-456 explicitly documents this dual-return: configured server with no client (e.g. handshake failed before stderr arrived) returns empty string + nil error rather than an error. Caller (handler) renders empty stderr panel naturally; not silent fallback because not-found-server still returns sentinel.

All 7 spot-checks confirmed correct classification — mechanism functioning, not rubber-stamping. The audit's findings (1 MED + 15 LOW) survive spot-check pressure: OK sites #1, #4, #5 (canonical fmt.Errorf) prove the §S16 pattern is applied consistently in most of the file, making bare-return and `%w: %v` deviations real inconsistencies not noise.

## Recommended fix priorities

1. **install.go:#5** (MED §S16 — `%w: %v` drops sandbox sentinel) — 1-line fix, swap `%v` for `%w` (Go 1.20+ multi-wrap). Enables `errors.Is(err, sandboxdomain.ErrRuntimeNotSupported)` callers to discriminate. **HIGH PRIORITY**.

2. **MCP subprocess Close orphan pattern** (5× LOW §S3) — single sweep commit replacing `_ = c.Close()` with logged `if err := c.Close(); err != nil { s.log.Warn(...) }` across mcp.go #7/#12/#15/#20/#21 + install.go #8. Aligns with mcp.md §3 "fail-loud" intent.

3. **mcp.go:#6** (LOW §S16/§S17 — `mcpapp.AddServer: server name required` no sentinel) — choose: (a) introduce `mcpdomain.ErrServerNameRequired` + register errmap as 400, OR (b) panic (config-time invariant). Currently never triggers in practice (caller always fills cfg.Name) but flagged for defensive completeness.

4. **§S16 wrap-format consistency** (LOW × 9) — bare-return → wrap pattern: install.go #1, #7; mcp.go #19, #20a, #21a, #27; searchrouter.go #3; calltool.go #17. Pure style cleanup; consider single sweep commit.

5. **searchrouter.go:#4** (LOW §S3 — silent json.Marshal `_`) — one-line inline comment addition documenting that Marshal of {string,int} map is unfailable. Or use `must.Marshal` helper if introducing one.

6. LOW miscellaneous (searchrouter.go #2 GetServer err translation context loss) — optional Debug log; low value, could WAIVE.
