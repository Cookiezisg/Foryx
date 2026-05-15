# Audit trace: backend/cmd/server/main.go

**File**: `backend/cmd/server/main.go`
**LOC**: 534
**Audit categories**: §S3 / §S9 / §S15 / §S16 / §S17

> **Posture**: This is the binary's `main()` — DI assembly + HTTP boot + graceful shutdown. By spec extract, length and DI density are exempt (§S5/§S12 don't apply). The lens here is narrow:
> 1. **§S3** — Bootstrap fail-fast vs. silent fallback (which "graceful skip on missing component" is legitimate degraded mode vs. which is a hidden bug).
> 2. **§S9** — Background goroutines started in main (sandbox bootstrap / mcp Start / skill Start / catalog Start) — are they detached?
> 3. **§S15** — Should NOT generate business IDs. Verify.
> 4. **§S16** — Should NOT do its own error wrapping at scale (it's not a service); the one adapter `forgeLLMClientAdapter.Generate` is the only `fmt.Errorf` site.
> 5. **§S17** — Does NOT process sentinels (no `errors.Is` to domain sentinels in main; no handler-level error mapping). Verify.

---

## 9-column trace

| site# | file:line | snippet | category | classification | reasoning | severity | user_impact | suggested_fix | status |
|---|---|---|---|---|---|---|---|---|---|
| 1 | main.go:97 | `else if h, err := os.UserHomeDir(); err == nil && h != "" { homeRoot = filepath.Join(h, ".forgify") } else { homeRoot = ".forgify" // working-dir fallback }` | A.1 | OK | §S3 fallback chain on bootstrap path resolution. The `else { homeRoot = ".forgify" }` is a **documented working-dir fallback** (inline comment present). UserHomeDir failure on linux/darwin is exotic (no $HOME) — degraded behavior (writing config in cwd) is acceptable + observable (next config write either succeeds or surfaces a clear FS error). Not a silent fallback. | LOW | — | — | — |
| 2 | main.go:111-115 | `log, err := loggerinfra.New(...)` → `fmt.Fprintf(os.Stderr, "init logger: %v\n", err); os.Exit(1)` | A.1 | OK | §S3 fail-loud bootstrap. Logger init failure → stderr + exit 1. Standard pre-logger pattern (can't `log.Error` before logger exists). | — | — | — | — |
| 3 | main.go:116 | `defer log.Sync() //nolint:errcheck` | A.1 | OK | §S3 example-of-fine: `log.Sync()` on shutdown error is unactionable (process is dying). `//nolint:errcheck` annotation present. **Correct posture but `//nolint:errcheck` is staticcheck-suppressing — note that §S3 spec says "linter-suppressing comments require reason" — here the reason is implicit "shutdown sync, can't propagate".** Marginal — annotation tradition suggests `// _ = err — flushing on shutdown, can't propagate` would be more spec-conforming, but the practical content is the same. | LOW | — | (optional) Replace with `defer func() { _ = log.Sync() }()` + inline reason. | — |
| 4 | main.go:118-121 | `gdb, err := dbinfra.Open(...)` → `log.Error("open db", zap.Error(err)); os.Exit(1)` | A.1 | OK | §S3 fail-loud bootstrap. DB open failure is fatal; clean exit. | — | — | — | — |
| 5 | main.go:123-127 | `defer func() { if err := dbinfra.Close(gdb); err != nil { log.Warn("close db", zap.Error(err)) } }()` | A.1 | OK | §S3 close-on-shutdown. Logging at Warn is fine — process is exiting, can't recover. Spec extract §S3 explicitly allows defer-close logging. | — | — | — | — |
| 6 | main.go:129-146 | `if err := dbinfra.Migrate(gdb, ...); err != nil { log.Error("migrate db", zap.Error(err)); os.Exit(1) }` | A.1 | OK | §S3 fail-loud — schema migration is critical + fatal-on-fail. | — | — | — | — |
| 7 | main.go:148-156 | `fingerprint, err := cryptoinfra.MachineFingerprint()` / `encryptor, err := cryptoinfra.NewAESGCMEncryptor(...)` → both fail-loud + os.Exit(1) | A.1 | OK | §S3 fail-loud — encryption setup is required for apikey domain (which encrypts API keys at rest); silently falling back to plaintext / no-op encryptor would be a security regression. Correct fatal-on-fail. | — | — | — | — |
| 8 | main.go:207-210 | `if err := sandboxSvc.Bootstrap(context.Background()); err != nil { log.Warn("sandbox v2 bootstrap failed (degraded mode active; runtime ops will fail)", zap.Error(err)) }` | A.1 / A.9 | OK | §S3 spec extract: "若某功能在当前环境不可用,必须让调用者看到错误或在文档/启动日志里明确说明". Here the bootstrap failure is logged at **Warn** level with explicit "degraded mode active; runtime ops will fail" message — operator notified at boot, downstream runtime ops will surface their own errors when called (forge service will return `forgedomain.ErrSandboxUnavailable` per errmap). **This is the spec-conforming "degraded mode + fail-loud at use site" pattern.** The 2-step (log.Warn at boot + user-visible error at use) is idiomatic. ⚠ Note: Warn vs Error level — handlers-B4 _summary §3 flagged `mcp.go AddServer 200+Warn` as MED partly because Warn is filterable. Here the same level is acceptable because: (a) clearly tagged "degraded mode active" (b) user impact is documented at boot (c) downstream failures will surface via errmap on first runtime call. | LOW | Bootstrap failures are noisy on first launch in dev (no .forgify/ yet). Current posture: Warn. **Consider Error level** for parity with cross-cutting MED in mcp.go AddServer audit — operator log filters typically watch ERROR. | (optional) `log.Warn` → `log.Error` for sandbox bootstrap parity with mcp/skill/catalog start failures (sites 12/14/16). | — |
| 9 | main.go:202-206 | `// PluginSandbox v2 — unified runtime/env service. Bootstrap extracts the embedded mise binary; failure flips degraded mode (chat-only path stays alive) but is non-fatal.` | A.1 | OK | §S3 documentation discipline — degraded-mode contract is documented inline. Reinforces site 8 OK. | — | — | — | — |
| 10 | main.go:207 | `sandboxSvc.Bootstrap(context.Background())` | A.2 | OK | §S9 — bootstrap call uses `context.Background()`, which is **correct** for boot-time invocation (no request context exists yet). **This is NOT a "terminal write" — it's bootstrap.** Background ctx is exactly the right choice. | — | — | — | — |
| 11 | main.go:211 | `registerSandboxStack(sandboxSvc, log)` | A.1 | OK | §S3 — registerSandboxStack helper at L506 returns early on `miseBin == ""` (Bootstrap failed). Documented inline. The `_ *zap.Logger` arg is unused inside the helper (signature retained for future use). Not a violation — degraded-mode skip path is documented. | — | — | — | — |
| 12 | main.go:326-328 | `if err := mcpService.Start(context.Background()); err != nil { log.Warn("mcp start partial failure (some servers may be unreachable)", zap.Error(err)) }` | A.1 / A.9 | OK | §S3 degraded mode + documentation discipline. mcp.Start does parallel connect-30s-timeout per server; per-server failures don't block boot. The Warn message is **concrete** ("some servers may be unreachable") not vague. Operator can investigate via subsequent /api/v1/mcp endpoints. Same posture as site 8. ⚠ Same Error-vs-Warn observation as site 8 — handlers-B4 _summary cross-cutting flag. | LOW | Cross-cutting: mcp Start / sandbox Bootstrap / skill Start / catalog Start all use Warn; consider Error for visibility parity. Not fatal as-is. | (optional) Promote 4 boot failure logs to Error level in one batch. | — |
| 13 | main.go:326 | `mcpService.Start(context.Background())` | A.2 | OK | §S9 — bootstrap, not terminal write. Background ctx correct. mcp.Start spawns its own per-server health-check goroutines (audited in app-mcp); responsibility for those internal goroutines lives there. | — | — | — | — |
| 14 | main.go:348-350 | `if err := skillService.Start(context.Background()); err != nil { log.Warn("skill start failed (continuing with empty cache)", zap.Error(err)) }` | A.1 / A.9 | OK | §S3 — skill scanner failure → empty cache, documented in log. Skill Start launches the 1s polling goroutine; goroutine ownership is in app-skill scope. Same Warn-vs-Error posture. | LOW | Same cross-cutting Warn-vs-Error as sites 8/12. | (optional) Same fix. | — |
| 15 | main.go:348 | `skillService.Start(context.Background())` | A.2 | OK | §S9 — bootstrap. Polling goroutine is detached by design (lives for app lifetime); proper context plumbing for that goroutine is app-skill's responsibility (deferred to app-skill audit). | — | — | — | — |
| 16 | main.go:372-374 | `if err := catalogService.Start(context.Background()); err != nil { log.Warn("catalog start failed (continuing without catalog injection)", zap.Error(err)) }` | A.1 / A.9 | OK | §S3 — catalog start failure → no system-prompt injection, chat path still works without catalog hints. Documented impact. Same level posture. | LOW | Same cross-cutting Warn-vs-Error as sites 8/12/14. | (optional) Same fix. | — |
| 17 | main.go:372 | `catalogService.Start(context.Background())` | A.2 | OK | §S9 — bootstrap. Catalog 1s polling goroutine ownership in app-catalog scope. | — | — | — | — |
| 18 | main.go:367 | `catalogService := catalogapp.New(filepath.Join(homeRoot, ".catalog.json"), notificationsPub, log)` | A.1 | OK | §S3 — wiring; no error path. | — | — | — | — |
| 19 | main.go:379-383 | `listener, err := net.Listen("tcp", ...)` → `log.Error("listen", zap.Error(err)); os.Exit(1)` | A.1 | OK | §S3 fail-loud — port listen failure is fatal. | — | — | — | — |
| 20 | main.go:443-448 | `go func() { if err := srv.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) { log.Error("serve", zap.Error(err)); stop() } }()` | A.1 | OK | §S3 — server-loop error handler. `errors.Is(err, http.ErrServerClosed)` correctly distinguishes "graceful shutdown" from real error; real error → log.Error + signal-stop (triggers main goroutine to exit). Background goroutine pattern is **textbook**: error reaches main via signal channel. | — | — | — | — |
| 21 | main.go:443-448 | `go func() { ... srv.Serve(...) ... }()` | A.2 | OK | §S9 — fire-and-forget server goroutine. `srvBaseCtx` is the parent of all request contexts (not this goroutine's own ctx). The goroutine doesn't itself do "terminal writes" — it accepts connections; per-request goroutines are spawned by net/http. **Background goroutine pattern is correct.** Per §S10: "异步或 fire-and-forget 必须打 log" — log.Error on error path is present. | — | — | — | — |
| 22 | main.go:464-466 | `if err := srv.Shutdown(shutdownCtx); err != nil { log.Error("shutdown", zap.Error(err)) }` | A.1 | OK | §S3 — shutdown failure logged at Error (correct level, since this is the last chance to surface it before exit). | — | — | — | — |
| 23 | main.go:480-491 | `forgeLLMClientAdapter.Generate(...)`: `bc, err := llmclientpkg.Resolve(...); if err != nil { return "", fmt.Errorf("forgeLLMClient: %w", err) }` then `return llminfra.Generate(...)` | A.4 | OK | §S16 — wrap format `"forgeLLMClient: %w"` follows `<pkg>.<Method>: %w` pattern. ⚠ Strictly speaking, spec wants `"<pkg>.<Method>: %w"` — here it's `"forgeLLMClient: %w"` (struct name, not method). The struct's only method is `Generate` so `"forgeLLMClient.Generate: %w"` would be more spec-conforming. Marginal — adapter's name uniquely identifies the site, and the call chain is short. | LOW | Spec wants `<pkg>.<Method>` — current `forgeLLMClient` is struct-name only. | Replace `fmt.Errorf("forgeLLMClient: %w", err)` with `fmt.Errorf("forgeLLMClient.Generate: %w", err)`. | — |
| 24 | main.go:485-490 | `return llminfra.Generate(ctx, bc.Client, llminfra.Request{...})` | A.4 | OK | §S16 — bare return of llminfra.Generate's error. infra/llm.Generate is responsible for its own wrap; main's adapter doesn't need to wrap a successful call's return. ✓ Spec-conforming. | — | — | — | — |
| 25 | main.go:506-533 | `registerSandboxStack(svc *sandboxapp.Service, _ *zap.Logger)` — registers installers + env managers; no error paths | A.1 | OK | §S3 — pure registration helper, no error returns. Underscored logger param `_ *zap.Logger` is fine (registry calls don't fail; if they did, sandboxapp would surface them). | — | — | — | — |
| 26 | main.go:521-527 | `for kind, defaultVer := range map[string]string{ "python": "3.12", "node": "22", "uv": "0.11.4" } { svc.RegisterInstaller(...) }` | A.1 | OK | §S3 — RegisterInstaller has no error return; pure registration. Fine. | — | — | — | — |
| 27 | main.go (whole file) | (no `errors.Is(err, <sentinel>)` checks against domain sentinels; no `responsehttpapi.FromDomainError` calls; only stdlib `errors.Is(err, http.ErrServerClosed)` at L444) | A.5 | OK | §S17 — main does NOT process domain sentinels. The single `errors.Is` use is against stdlib `http.ErrServerClosed` (server lifecycle, not domain error). No errmap responsibility here. ✓ Architecturally clean. | — | — | — | — |
| 28 | main.go (whole file) | (no `idgen.New(...)` calls; no `crypto/rand` use; no business ID generation) | A.3 | OK | §S15 — main is DI assembly only. Does NOT generate business IDs. ✓ Correct. | — | — | — | — |
| 29 | main.go:285 | `tools = append(tools, webtool.WebTools(modelService, apikeyService, llmFactory, mcpapp.NewSearchRouter(mcpService), log)...)` | A.1 | OK | §S3 — wiring; no error path. | — | — | — | — |
| 30 | main.go:287 | `defer shells.Manager.Stop() // graceful shutdown: kill any background children` | A.1 | OK | §S3 — Stop() return value discarded. **In-line comment** documents the intent. Stop() likely returns no error or unactionable error; if it does return one, current code drops it silently. ⚠ Worth checking shelltool.NewShellTools.Manager.Stop signature — if Stop returns error, this would be a §S3 LOW (defer-close-fire-and-forget pattern needs reason comment). Comment mentions intent but not "why we discard error." | LOW | Defer-close on tool manager — if `shells.Manager.Stop()` returns error, it's silently dropped. Either confirm signature is `func()` (no error) or add explicit `// _ = err — shutdown, can't propagate`. | Verify shells.Manager.Stop signature in app-tool-shell audit; if it returns error, change to `defer func() { if err := shells.Manager.Stop(); err != nil { log.Warn("shell manager stop", zap.Error(err)) } }()` for parity with DB close at L123-127. | — |
| 31 | main.go:104-109 | `var broadcaster *loggerinfra.LogBroadcaster; var logExtras []zapcore.Core; if *dev { broadcaster = loggerinfra.NewLogBroadcaster(); logExtras = []zapcore.Core{broadcaster} }` | A.1 | OK | §S3 — dev-only path; no error sources. Fine. | — | — | — | — |
| 32 | main.go:177-180 | `if *dev { llmFactory.SetTracer(llminfra.NewTraceRecorder()); log.Info(...) }` | A.1 | OK | §S3 — dev-only; no error path. | — | — | — | — |
| 33 | main.go:428-429 | `srvBaseCtx, cancelBase := context.WithCancel(context.Background()); defer cancelBase()` | A.2 | OK | §S9 — `srvBaseCtx` is the **parent of all request contexts**, by design. This is NOT a "terminal write" pattern — it's lifecycle ctx for HTTP handlers. The defer + later explicit cancelBase at L460 is double-cancel-safe (Cancel is idempotent). **Correct shutdown pattern; documented at L417-427.** | — | — | — | — |
| 34 | main.go:440 | `ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)` | A.2 | OK | §S9 — signal-listener ctx; lifecycle, not terminal write. Correct. | — | — | — | — |

---

## Sub-check matrix

### A.1 §S3 错误吞没
- **Concrete violations**: none. Sites 1, 8, 11, 12, 14, 16 are **degraded-mode bootstrap fallbacks** with documented operator-visible logging — spec-conforming per §S3 "若某功能在当前环境不可用,必须让调用者看到错误或在文档/启动日志里明确说明".
- **LOW marginal observations**:
  - Site 3 (`defer log.Sync() //nolint:errcheck`) — annotation-style suppression; works but `// _ = err — shutdown, can't propagate` would be more spec-aligned. Marginal.
  - Sites 8, 12, 14, 16 — cross-cutting Warn-vs-Error level for boot-time degraded-mode fallbacks (sandbox / mcp / skill / catalog Start). Operator log filters typically watch ERROR; consider Error level for parity with handlers-B4 _summary §3 cross-cutting flag. Not fatal as-is.
  - Site 30 (`defer shells.Manager.Stop()`) — defer with discarded return; needs signature verification + reason comment if Stop returns error.
- **Net A.1**: 0 concrete violations; 5 LOW marginal observations (1× annotation-style nit, 4× cross-cutting Warn level, 1× defer-close pending signature verify).

### A.2 §S9 detached ctx 终态写
- **Terminal-state writes identified**: **none in main.go**. Main does **bootstrap** (sites 10/13/15/17 use `context.Background()` correctly because no request context exists at boot) and **lifecycle** (sites 21/33/34 are HTTP server / signal context infrastructure, not terminal writes).
- **Background goroutines started in main**:
  - **Site 21** (`go func() { srv.Serve(...) }`) — fire-and-forget HTTP server goroutine. Internal request handling is net/http stdlib responsibility; per-request `r.Context()` derives from `srvBaseCtx` (correct lifecycle). Not a "terminal write" goroutine.
  - **Embedded goroutines spawned by Start methods** (mcp / skill / catalog) — those goroutines live inside app-mcp / app-skill / app-catalog; **detached-context responsibility is theirs**, not main's. Main's job is to call Start with `context.Background()` (correct — boot-time, no request ctx).
- **N/A reason**: main.go does NOT itself perform terminal writes — it calls `Bootstrap` / `Start` / `Migrate` / `Listen` / `Serve` / `Shutdown`. The goroutines those Starts launch are detached by design; their ctx-handling is audited under app-* packages.
- **Violations**: not present.

### A.3 §S15 ID 生成
- **ID generation calls**: none.
- **Self-rolled `crypto/rand` use**: none.
- **N/A reason**: main.go is DI assembly; does NOT generate business IDs. Spec compliance is structural — main has no business reason to ever produce IDs.
- **Violations**: N/A: package doesn't generate business IDs.

### A.4 §S16 错误 wrap 格式
- **`fmt.Errorf` calls**: 1 site total — `forgeLLMClientAdapter.Generate` at L483.
  - `fmt.Errorf("forgeLLMClient: %w", err)` — uses `%w` correctly; **prefix is struct-name only**, spec wants `<pkg>.<Method>` → should be `forgeLLMClient.Generate`.
- **Violations**: 1 LOW (site 23) — prefix incomplete (`forgeLLMClient` should be `forgeLLMClient.Generate`).
- **Other paths**: site 24 (returning `llminfra.Generate`'s err bare) is correct — infra/llm wraps its own; pass-through unwrapped is fine.
- **`errors.New` direct concatenation**: not present.
- **`%v` instead of `%w`**: not present.

### A.5 §S17 sentinel 登记 errmap
- **Sentinels defined in main.go**: none. main does not declare sentinels.
- **Sentinel-handling in main.go**: none. The single `errors.Is` call at L444 is against `http.ErrServerClosed` (stdlib lifecycle marker, not a domain error / not registered in errmap by design).
- **Already registered in errmap**: N/A.
- **Missing**: N/A: file defines no sentinels. Architecturally, main is decoupled from errmap — only `transport/httpapi/response/errmap.go` and the handlers it serves participate.

---

## Severity summary

| Severity | Count | Sites |
|---|---|---|
| HIGH | 0 | — |
| MED | 0 | — |
| LOW | 7 | 1 (working-dir fallback marginal observation), 3 (`defer log.Sync //nolint:errcheck` — annotation style), 8/12/14/16 (Warn-vs-Error cross-cutting on bootstrap fallback logs), 23 (forgeLLMClient prefix incomplete), 30 (`defer shells.Manager.Stop()` discards potential error) |

**Net cmd/server**: 0 HIGH / 0 MED / 7 LOW (5 cross-cutting cosmetic / 2 §S16 wrap-format prefix tightening).

**Architectural assessment**: main.go is **textbook clean** for §S3/§S9/§S15/§S17 — bootstrap fail-fast pattern is rigorous (DB / fingerprint / encryptor / migrate / listen all `os.Exit(1)`); degraded-mode fallbacks (sandbox / mcp / skill / catalog) are documented inline + logged at boot + downstream user-facing errors via errmap. Background goroutine count is small (1 server-Serve goroutine + Starts that spawn their own internal goroutines audited downstream). No business ID generation, no sentinel processing — clean separation of concerns.

**Only concrete LOW**: site 23 (`fmt.Errorf("forgeLLMClient: %w", err)` should be `forgeLLMClient.Generate`). 1-line fix.

**Cross-cutting LOW cluster** (sites 8/12/14/16): 4 boot-time degraded-mode logs are `log.Warn`. Per handlers-B4 _summary §3 (mcp.go AddServer 200+Warn MED), Error level catches operator log filters reliably. Optional 1-batch promotion to Error level for parity.
