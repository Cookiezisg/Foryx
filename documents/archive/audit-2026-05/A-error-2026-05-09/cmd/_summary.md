# Package audit summary: cmd/ (Phase A — error handling + data integrity)

## Files audited

| File | LOC | Sites | OK | EDGE | VIOLATION |
|---|---|---|---|---|---|
| server/main.go | 534 | 34 | 27 | 0 | 7 LOW (5 cross-cutting + 2 §S16 prefix) |
| resources/main.go | 330 | 32 | 31 | 0 | 1 concrete LOW (§S3 inline reason) + ~12 marginal LOW (cross-cutting cosmetic) |
| **TOTAL** | **864** | **66** | **58** | **0** | **8 concrete + ~12 marginal LOW** |

## Spec understanding (Phase A — §S3 / §S9 / §S15 / §S16 / §S17)

Both files are `package main` binaries. **Posture is fundamentally different from app/handler/infra packages**:

- **server/main.go** — DI assembly + HTTP boot + graceful shutdown. Does NOT process domain sentinels; does NOT generate business IDs; does NOT do terminal-state writes itself (calls Bootstrap/Start/Migrate/Listen/Serve/Shutdown — internal goroutines those Starts launch are detached by design and audited under app-* packages).
- **resources/main.go** — build-time CLI (one-shot, exits). No request contexts, no terminal writes, no goroutines, no domain sentinels.

The lens here narrows to:
1. **§S3** — bootstrap fail-fast vs. silent fallback (server: DB / fingerprint / encryptor / migrate / listen — all `os.Exit(1)`; sandbox / mcp / skill / catalog Start — documented degraded mode). Resources: hash mismatch / unsupported platform / SHA file fetch — all fail-loud.
2. **§S9** — N/A for resources (build-time CLI). Server: only the HTTP server-Serve goroutine is owned by main; mcp/skill/catalog Start spawn their own internal goroutines whose ctx-handling is app-* responsibility.
3. **§S15** — N/A both files (no business ID generation).
4. **§S16** — server has 1 wrap site (`forgeLLMClientAdapter.Generate`) with prefix `forgeLLMClient:` instead of strict `forgeLLMClient.Generate:` — LOW. Resources has ~12 wrap sites with action-named prefixes (`download:`, `gunzip:`, etc.) consistent with Go cmd/-tool idioms; spec-strict interpretation flags as LOW cross-cutting.
5. **§S17** — N/A both files (no sentinel handling, no errmap path).

## Severity breakdown

| Severity | Count | Sites |
|---|---|---|
| HIGH | 0 | — |
| MED | 0 | — |
| LOW | 8 concrete + ~12 marginal | server 7 LOW; resources 1 concrete + ~12 marginal |

## Cross-cutting findings

### 1. Bootstrap fail-fast vs. degraded-mode discipline (server, sites 8/12/14/16) — cross-cutting LOW

Four boot-time component starts (sandbox v2 Bootstrap, mcp Start, skill Start, catalog Start) follow identical posture: `if err := X.Start(ctx); err != nil { log.Warn("X failed (degraded mode)", zap.Error(err)) }`. The contract:

- Component is non-essential for chat-only path
- Failure is documented inline (with concrete impact: "runtime ops will fail" / "some servers may be unreachable" / "continuing with empty cache" / "continuing without catalog injection")
- Downstream user-facing errors will surface via errmap on first runtime call (`forgedomain.ErrSandboxUnavailable` etc.)

**This is the spec-conforming "degraded mode + fail-loud at use site" pattern per §S3.**

The cross-cutting LOW: all 4 use **`log.Warn`**, not **`log.Error`**. Per handlers-B4 _summary §3 (mcp.go AddServer 200+Warn MED), Error level catches operator log filters more reliably. Optional 1-batch promotion to Error for visibility parity. Not fatal as-is.

### 2. forgeLLMClientAdapter.Generate prefix style (server, site 23) — concrete LOW

```go
return "", fmt.Errorf("forgeLLMClient: %w", err)
```

Spec §S16: `<pkg>.<Method>: %w`. Current prefix is struct-name only. **1-line fix**:

```go
return "", fmt.Errorf("forgeLLMClient.Generate: %w", err)
```

Practical impact: minimal — adapter has 1 method; stderr trace is unambiguous. But spec-strict.

### 3. resources.go _= os.Remove(tmp) inline reason comment (resources, site 17) — concrete LOW

```go
if _, err := io.Copy(out, r); err != nil {
    out.Close()
    _ = os.Remove(tmp)        // ❌ no inline reason
    return fmt.Errorf("write %s: %w", tmp, err)
}
```

Strict §S3 spec extract: `_ = err` 带行内注释**说明为什么吞**. Reason is obvious in context (best-effort cleanup after copy failure already captured) but spec wants it explicit. **1-line fix**:

```go
_ = os.Remove(tmp) // best-effort cleanup; copy err already captured
```

### 4. resources.go fmt.Errorf prefix style (resources, ~10 sites) — cross-cutting LOW

Action-named prefixes (`download:`, `gunzip:`, `unzip:`, `tar next:`, `open <path>:`, `write <path>:`, `close <path>:`, `get <url>:`, `checksum lookup:`) instead of spec-strict `<pkg>.<Method>:`. This is consistent with Go stdlib `cmd/` tool idioms (operational readability over method-naming).

**Spec-strict interpretation**: every `fmt.Errorf` should be `<pkg>.<Method>: %w`. Service code spec wants this for `errors.Is` unwrap-chain consumers; build-time CLI has no such consumers (errors terminate via `log.Fatalf` printing the `%w` chain to stderr). **LOW marginal.**

### 5. cmd/ package boundary respect — clean

Neither file:
- Generates business IDs (§S15 N/A — DI assembly + binary-fetcher, no business state)
- Defines or processes domain sentinels (§S17 N/A — main does not participate in errmap; resources is build-time)
- Performs terminal-state writes that need detached ctx (§S9 N/A — server Bootstraps with `context.Background()` correctly; resources is one-shot CLI)

This is **structurally correct** — `cmd/` is the assembly + bootstrap edge of the architecture, not the business-logic layer. Domain concerns (sentinels, IDs, terminal writes) live in `internal/{domain,app,infra}/` and are audited there.

## Status (post-fix)

| Site | Severity | Status | Commit |
|---|---|---|---|
| server site 23 (`forgeLLMClient` prefix) | LOW | FIXED | this batch — `forgeLLMClient.Generate:` |
| server sites 8/12/14/16 (4 bootstrap Warn-vs-Error) | LOW | WAIVED | bootstrap degraded-mode 设计契约 per server/main.go inline comments；Warn 是合理选择（log filter 改造决定推迟到 ops side） |
| server site 3 (`defer log.Sync //nolint:errcheck`) | LOW | WAIVED | annotation style；现状合规无需改 |
| server site 30 (`defer shells.Manager.Stop()`) | LOW | EDGE-FLAG | 等 app-tool-shell audit 验签名 |
| resources site 17 (`_ = os.Remove(tmp)` no comment) | LOW | FIXED | this batch — 加 best-effort cleanup §S3 例外注释 |
| resources ~12 marginal LOW | LOW | WAIVED | build-time CLI 无 errors.Is unwrap consumer；prefix style 与 Go cmd/-tool idiom 一致 |

## Architectural assessment

Both files are **textbook clean** for §S3 critical paths:

- **server/main.go**: bootstrap fail-fast is rigorous (DB / fingerprint / encryptor / migrate / listen all `os.Exit(1)`); degraded-mode fallbacks (sandbox / mcp / skill / catalog) are documented inline + logged at boot + downstream errors surface via errmap. Background goroutine count is small (1 server-Serve goroutine + Starts that spawn their own internal goroutines audited downstream). No business ID generation, no sentinel processing — clean separation.

- **resources/main.go**: gold-standard fail-loud on supply-chain integrity (hash mismatch, asset-not-found, unsupported platform, SHA file fetch). Operator-facing stderr UX is good — every fail message names the specific failure + relevant identifiers (asset name, platform, hash values, status code). No silent platform fallback, no skipped checksum fallback.

**No HIGH or MED violations across both files. All findings are LOW-severity cosmetic / spec-strict prefix style / annotation patterns.**

The `cmd/` boundary correctly stays out of domain concerns — main is assembly + bootstrap; resources is build-time. Sentinels, IDs, terminal-write semantics belong to `internal/` packages audited under their own batches.

## Recommended fix priorities

By §S20 + §S14 — cmd/ contributes **0 HIGH / 0 MED / 8 concrete LOW + ~12 marginal LOW**:

1. **resources site 17 (`_ = os.Remove(tmp)` no comment) — LOW**: 1-line inline comment. **Trivial; do now per §S20.**
2. **server site 23 (`forgeLLMClient` prefix) — LOW**: 1-line fix to `forgeLLMClient.Generate`. **Trivial; do now.**
3. **server sites 8/12/14/16 (Warn → Error promotion cluster) — LOW (cross-cutting)**: 4-line batch optional; defer to operator log-filter discussion (matches handlers-B4 _summary §3 cross-cutting). Either now or with that batch.
4. **server site 3 (`defer log.Sync //nolint:errcheck`) — LOW (annotation style)**: cosmetic; defer as part of "annotation style sweep" if/when project does one.
5. **server site 30 (`defer shells.Manager.Stop()`) — LOW**: verify signature in app-tool-shell audit; gate on that finding before changing.
6. **resources ~12 marginal LOW (prefix style)**: cross-cutting cosmetic; build-time CLI context makes impact zero. **Defer or waive.**

## Out-of-scope notes

1. `_test.go` files per fork constraint (not read).
2. `app-sandbox`, `app-mcp`, `app-skill`, `app-catalog` Service.Start internals (the goroutines they launch + ctx-handling) — separate app-* audit batches.
3. `app-tool-shell` ShellManager.Stop signature (referenced server site 30) — app-tool-shell audit.
4. `infra/llm` Generate / Factory wrap behavior (called from server's `forgeLLMClientAdapter`) — infra-llm audit batch.
5. `cryptoinfra.MachineFingerprint` / `NewAESGCMEncryptor` / `DeriveKey` (referenced server sites 148-156) — separate crypto-infra audit (not in current batch list; appears already covered by errmap registration of `cryptoinfra.ErrUnsupportedVersion`).
6. `loggerinfra.New` / `LogBroadcaster` — separate logger-infra audit batch (not yet listed).
7. `routerhttpapi.New` Deps wiring (server L390) — transport-router audit batch.
8. `dbinfra.Open` / `Close` / `Migrate` — separate db-infra audit batch (not yet listed).
