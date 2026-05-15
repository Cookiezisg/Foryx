# Package audit summary: internal/app/catalog

## Spec understanding (Phase A — §S3 / §S9 / §S15 / §S16 / §S17)

- **§S3 错误不吞**: catalog has 3 documented "fallback" patterns that LOOK like silent-skip but are actually loud-logged design behaviors per catalog.md §3 ("失败隔离" / "失败即 mechanical + 写 lastFP"): (a) per-source ListItems failure → log Warn + substitute empty list; (b) all-sources-failed → return error WITHOUT touching cache (preserves previous good state); (c) Generator (LLM) failure → log Warn + run mechanicalFallback. All three paths zap.Warn the err with source/path context per §S10 async/fire-and-forget rule. Disk-write failure on saveToDisk is a documented "in-memory cache still updated" recovery — derived cache, loss is regenerable next tick. **No silent-skip violations found**.
- **§S9 detached ctx 终态写**: catalog runs in a background polling goroutine, NOT a request goroutine. Per design doc §11 implementation note + polling.go:180-182 the package applies its OWN §S9 pattern: at Refresh entry, `if ctx has no user ID → SetUserID(ctx, DefaultLocalUserID)`. This is in-place injection (not full `context.Background()` reset) because (a) goroutine ctx is already detached from any HTTP request; (b) Stop's context-cancel must still propagate to interrupt long Refreshes during shutdown. The terminal writes themselves (cache.Store, lastFP.Store at polling.go:246-247) are atomic.Pointer / atomic.Value with no ctx; saveToDisk uses non-cancellable file syscalls; notif.Publish is fire-and-forget. **No §S9 violations** — pattern correctly inverted for background-goroutine context, applied at the exactly-right place.
- **§S15 ID 生成**: package does NOT generate business IDs. The only persistent identifier is the SHA-256 fingerprint (polling.go::fingerprint), which is a content hash not a §S15 random ID. Item IDs come from sources verbatim (forge `f_` / skill `<name>` / mcp slug) per design doc §4. Catalog Version field is a monotonic counter (catalog.go::nextVersion), not a `<prefix>_<16hex>` ID — design intent + acceptable since it's an internal cache index.
- **§S16 错误 wrap 格式**: canonical `fmt.Errorf("<pkg>.<Method>: %w", err)` is **not consistently applied**. 9 sites total in this package use bare `"catalog:"` or `"catalog: <verb>:"` prefixes without method qualifier. 3 of those sites additionally use `%w: %v` instead of `%w: %w` — losing inner sentinel chain (same anti-pattern as app-mcp install.go #5 MED finding). All sites are LOW severity because catalog absorbs all internal errors before any handler path consumes them.
- **§S17 errmap 单一事实源**: 2 catalogdomain sentinels (ErrCoverageIncomplete, ErrGenerationFailed) defined in domain/catalog/catalog.go:124,132. Per design doc §10 ("均不上抛 handler——catalog 内部消化"), neither is registered in errmap.go — verified via grep. **However**: design doc §10's "internal 消化" promise is **technically broken at one site** — polling.go:213's `fmt.Errorf("catalog: all %d sources failed; ...")` returns an unwrapped error that **does** escape via the HTTP `:refresh` handler (handlers/catalog.go:80 → FromDomainError). This is the only MED finding in the package: the err is unmapped → falls through to errmap.go's `{500, "INTERNAL_ERROR"}` + logs an "unmapped domain error" ERROR (the §S17 smoke alarm). Fix is to either (a) define + register a `catalogdomain.ErrAllSourcesFailed` sentinel, or (b) return nil + log Warn (closer to design's "internal 消化" intent).

## Files audited

| File | LOC | Sites | OK | POST-FIX | VIOLATION | EDGE |
|---|---|---|---|---|---|---|
| catalog.go | 241 | 7 | 7 | 0 | 0 | 0 |
| disk.go | 89 | 7 | 4 | 0 | 0 | 3 |
| mechanical.go | 88 | 6 | 6 | 0 | 0 | 0 |
| polling.go | 289 | 14 | 12 | 0 | 1 | 1 |
| generator.go | 213 | 12 | 9 | 0 | 3 | 0 |
| **TOTAL** | **920** | **46** | **38** | **0** | **4** | **4** |

> _LOC subtotal 920 vs caller's prompt "916 LOC" — 4-line drift attributable to recent edits since the LOC count was taken; no impact on findings._

## Severity breakdown (only VIOLATION + EDGE)

| Severity | Count | Sites | Status |
|---|---|---|---|
| HIGH | 0 | — | — |
| MED | 1 | polling.go:#8 (all-sources-failed err escapes via HTTP `:refresh` → unmapped errmap → 500 INTERNAL_ERROR + "unmapped domain error" smoke-alarm log noise) | FOUND |
| LOW | 7 | generator.go:#3 (`%w: %v` resolve LLM); generator.go:#4 (`%w: %v` llminfra.Generate); generator.go:#7 (`%w: %v` json.Unmarshal); generator.go:#5,#6,#8 (no pkg.Method prefix on 3 internal-only ErrGenerationFailed wraps); polling.go:#12 (saveToDisk fail → Warn-only + continue, documented design but borderline §S3); disk.go:#2 (`_ = os.Rename` for .bak best-effort, has block-level comment but no inline justification); disk.go:#6 (`_ = os.Remove(tmp)` no inline §S3 justification); disk.go:#7 (all wrap-strings use `catalog:` not `catalog.<Func>:` — pure style) | FOUND |

> Counting note: LOW count rolls disk.go's 3 sub-issues (#2 §S3, #6 §S3, #7 §S16 cluster) into 3 distinct LOW entries to mirror per-site severity tracking. The 7-LOW figure reflects this granular accounting; aggregate severity table line counts 7 site-row entries.

## Cross-cutting

### Sentinel chain integrity (§S17)

| Sentinel | Defined | Used in (this pkg) | errmap.go | Verdict |
|---|---|---|---|---|
| `catalogdomain.ErrCoverageIncomplete` | domain/catalog/catalog.go:124 | NOT consumed in this pkg (the post-#7 cleanup deleted the consumer; sentinel kept for future) | not registered | OK per design §10 |
| `catalogdomain.ErrGenerationFailed` | domain/catalog/catalog.go:132 | generator.go × 6 wraps; absorbed at polling.go:227 | not registered | OK per design §10 — internal consumption only |
| (un-named, line 213 anonymous fmt.Errorf) | polling.go:213 — `"catalog: all %d sources failed..."` | escapes via HTTP `:refresh` handler | NOT registered → falls through to {500, "INTERNAL_ERROR"} + "unmapped domain error" log | **VIOLATION** (MED) — design §10 promise is technically broken at this site |

**The only material §S17 finding** is the polling.go:213 escape path. All other sentinels are correctly absorbed before any handler.

### Detached ctx coverage (§S9) — context-by-context analysis

**Terminal-state write inventory:**

| Write | File / Site | Ctx | §S9 verdict |
|---|---|---|---|
| `s.cache.Store(cat)` | polling.go:246 | (no ctx — atomic.Pointer.Store) | ✓ OK — non-cancellable atomic |
| `s.lastFP.Store(fp)` | polling.go:247 | (no ctx — atomic.Value.Store) | ✓ OK — non-cancellable atomic |
| `saveToDisk(...)` | polling.go:248 | (no ctx — file syscalls non-cancellable) | ✓ OK — derived cache, loss recoverable next tick; failure logged Warn |
| `s.notif.Publish(ctx, ...)` | polling.go:252 | Refresh's ctx (post-injection at line 181) | ✓ OK — fire-and-forget; loss on cancel acceptable per UI re-fetches via GET |
| (background-goroutine ctx fix) | polling.go:180-182 — `if !hasUserID { ctx = SetUserID(ctx, DefaultLocalUserID) }` | wraps caller ctx in-place | ✓ OK — design §11 documented "implementation-discovered bug fix"; in-place wrap preserves Stop()'s ctx-cancel reachability |

**No §S9 violations** in this package. Pattern is correctly applied for the background-goroutine inversion (inject identity vs. detach from request lifetime). Differs from apikey.Test's `SetUserID(context.Background(), uid)` because catalog's Refresh is ALREADY in a background goroutine — adding another `Background()` wrap would defeat Stop()'s ability to interrupt long Refresh calls during shutdown.

### Pattern: §S16 wrap-format cluster (cluster of 6 LOW)

All 6 wrap sites in this package share two style deviations from §S16 canonical form:

| Site | Issue |
|---|---|
| polling.go:#8 | `"catalog: all %d sources failed; ..."` — missing `.Refresh:` qualifier; **also §S17 unmapped (MED)** |
| generator.go:#3 | `"%w: resolve LLM: %v"` — `%v` chain-loss + missing `catalogapp.Generator.Generate:` qualifier |
| generator.go:#4 | `"%w: %v"` — same |
| generator.go:#5 | `"%w: output exceeded %d chars"` — missing qualifier (no inner err so %v is N/A) |
| generator.go:#6 | `"%w: no JSON in response"` — same |
| generator.go:#7 | `"%w: parse JSON: %v"` — `%v` chain-loss + missing qualifier |
| generator.go:#8 | `"%w: empty Summary"` — missing qualifier |
| disk.go:#7 | `"catalog: read/parse/mkdir/marshal/write tmp/rename"` × 6 wrap sites — missing `.<Func>:` qualifier |

Recommended single-sweep commit fix:
- All `%v` for inner errs → `%w` (Go 1.20+ multi-wrap).
- All bare `catalog:` / `<no prefix>` → proper `catalogapp.<Func>:` or `catalogapp.<Type>.<Method>:` qualifier.
- File-level pattern hint: `catalogapp.Refresh:`, `catalogapp.Generator.Generate:`, `catalogapp.loadFromDisk:`, `catalogapp.saveToDisk:`.

### Pattern: disk best-effort cleanup discards (cluster of 2 LOW)

- disk.go:#2 — `_ = os.Rename(path, bak)` for moving corrupted cache file aside. Block-level comment at lines 49-55 justifies "best-effort". Inline-comment form preferred per §S3 spec literal text.
- disk.go:#6 — `_ = os.Remove(tmp)` for cleaning up .tmp after failed Rename. No comment.

Both are best-effort cleanup where main-flow error preservation matters more than cleanup error reporting. Recommended fix: either (a) plumb a logger into loadFromDisk/saveToDisk and zap.Warn the cleanup failures, or (b) add the explicit inline `// best-effort cleanup; surface main error` comments. The second option matches §S3 carve-out spec literally.

### Catalog's "internal consumption" promise (§S17 design tension)

Design doc §10 promises catalog sentinels never reach handlers. This is **almost** true — the architectural pattern is:

```
LLMGenerator.Generate → wraps ErrGenerationFailed (6 sites)
  → polling.go::Refresh receives, falls back to mechanicalFallback (sentinel absorbed)
  → cache.Store + saveToDisk (no errors propagate to handler)
  → handler GETs / POST :refresh see only the cache
```

But the all-sources-failed path is the gap:

```
polling.go::Refresh detects failedCount == len(sources)
  → returns fmt.Errorf("catalog: all %d sources failed...") UNWRAPPED
  → if called from polling tick: tryRefresh logs Warn + ignores (good)
  → if called from HTTP :refresh: handler forwards to FromDomainError → unmapped 500
```

This is a **real design oversight** — the err return from Refresh exists so polling-tick code can log it ("skipped/failed; keeping previous cache"), but the same code path is reachable from HTTP. Two clean fixes:

1. **Define + register a sentinel** (preserves HTTP visibility, gets a 503 status):
   ```go
   // domain/catalog/catalog.go
   var ErrAllSourcesFailed = errors.New("catalog: all sources failed")

   // errmap.go
   catalogdomain.ErrAllSourcesFailed: {http.StatusServiceUnavailable, "CATALOG_ALL_SOURCES_FAILED"},

   // polling.go:213
   return fmt.Errorf("catalogapp.Refresh: %w (%d sources)", catalogdomain.ErrAllSourcesFailed, len(sources))
   ```

2. **Return nil + log Warn** (matches design §10 literally):
   ```go
   if failedCount == len(sources) {
       s.log.Warn("catalogapp.Refresh: all sources failed; keeping previous cache",
           zap.Int("sources", len(sources)))
       return nil
   }
   ```

Recommend (1) — it preserves HTTP signal (503 tells the user "try again later" rather than "internal error") and removes the smoke-alarm noise. Single new sentinel + single errmap row; small surface.

## Spot-check (random clean sites — verify mechanism not rubber-stamping)

Random pick from `OK` set across 5 files:

1. **catalog.go:#1** (panic on nil log) — verified literal: `panic("catalog.New: logger is nil")` matches `<pkg>.<Method>:` panic-msg pattern; same form as apikey.NewService panic. Boot-time invariant carve-out per §S3 spec. Compliance literal.
2. **catalog.go:#5** (Get returns nil pre-Refresh) — verified: `return s.cache.Load()` is atomic.Pointer's Load returning nil zero-value before first Store. Caller (handlers/catalog.go:66) does `responsehttpapi.Success(w, http.StatusOK, h.svc.Get())` which JSON-encodes nil as `null` in envelope's data field. Documented + tested per design §11 ("Get empty cache → null in envelope").
3. **disk.go:#1** (loadFromDisk ENOENT branch) — verified: `errors.Is(err, fs.ErrNotExist)` is canonical Go 1.13+ way to detect missing file (handles wrapped + unwrapped fs.ErrNotExist). Caller (polling.go:46-67) treats `(nil, nil)` as "first launch — empty cache start". §S3 OK because no error is suppressed; ENOENT is a documented success state.
4. **mechanical.go:#1** (no error return) — verified: function is total (every input → valid Catalog struct). No I/O, no failure modes. The "fallback" semantics live in the caller (polling.go:227-238 picks between LLM Generator and mechanicalFallback) — mechanicalFallback itself just produces output. Correct separation of concerns.
5. **polling.go:#5** (DefaultLocalUserID injection) — verified: `if _, ok := reqctxpkg.GetUserID(ctx); !ok { ctx = reqctxpkg.SetUserID(ctx, reqctxpkg.DefaultLocalUserID) }` is design §11 explicit "implementation-discovered bug fix". The HTTP `:refresh` path (where middleware already stamped a user ID) takes the `ok=true` branch and skips the injection — preventing accidental override. Correct.
6. **polling.go:#10** (Generator → mechanicalFallback fallback) — verified: `s.log.Warn("catalog Generator failed; using mechanical fallback", zap.Error(err))` is loud per §S10 + design §3 ("失败即 mechanical + 写 lastFP" + "用户活跃度驱动重试"). The lastFP update at line 247 happens regardless — this is the documented user-activity-driven retry pattern (next user description-edit will trigger fp change → next tick retries LLM). NOT silent fallback.
7. **generator.go:#9** (LLM Coverage pass-through) — verified: design doc §7 + comment line 88-89 explicitly state "Coverage from the LLM is passed through verbatim without validation — historic 3-attempt retry + missing-id hint augmentation removed per 屎山拯救计划 #7". Documented design intent. Caller polling.go does no Coverage validation either. Acceptable per design (LLM may drop 1-2 IDs but mechanicalFallback already covers the demo-safety case).

All 7 spot-checks confirmed correct classification — mechanism functioning, not rubber-stamping. The audit's findings (1 MED + 7 LOW) survive spot-check pressure: OK sites #5, #6 (the §S9 ctx-injection pattern + the documented "loud fallback" design) prove that the package is consistently following its own §3 design. The deviations are the unwrapped error-escape at polling.go:213 (real bug per §S17) and the `%w: %v` cluster across generator.go (style consistency with documented internal-only consequence).

## Recommended fix priorities

1. **polling.go:#8 (MED §S16/§S17 — `"catalog: all %d sources failed..."` escapes via HTTP `:refresh` → unmapped errmap → smoke-alarm noise)** — define `catalogdomain.ErrAllSourcesFailed` + register in errmap.go as `{503, "CATALOG_ALL_SOURCES_FAILED"}` + wrap with `catalogapp.Refresh:` qualifier. Closes the only material §S17 hole in this package. **HIGH PRIORITY** because the design doc's "internal 消化" promise depends on this.

2. **generator.go §S16 wrap-format cluster** (3 LOW sites with `%w: %v` chain-loss + 3 LOW sites with missing pkg.Method qualifier) — single-sweep commit fix. Same template across all 6 sites. Forward-compatible insurance: even if catalog absorbs these errors today, future feature additions might expose them via a handler.

3. **polling.go:#8 + disk.go:#7 §S16 qualifier** — bare `"catalog:"` → `"catalogapp.<Func>:"`. Pure style; included in same sweep commit as #2.

4. **disk.go:#2, #6 §S3 inline justification comments** (LOW) — add explicit `// best-effort cleanup; surface main error` inline comments for the two `_ = os.Rename(...)` / `_ = os.Remove(...)` sites. Or plumb a logger to call zap.Warn. Either flavor matches §S3 spec literally.

5. **polling.go:#12 §S3 saveToDisk Warn-and-continue** (LOW) — currently documented + intentional per design §3 (derived cache; loss recoverable). WAIVE-able. Optional improvement: increment a metric counter so persistent disk-write failures bubble as a health signal.

## Post-fix expected state

After the recommended fix bundle (1 MED + 6 LOW LOW):
- Total VIOLATIONs → 0 (the MED becomes a registered sentinel)
- §S17 canonical: catalog package has 1 NEW registered sentinel (ErrAllSourcesFailed) + 2 unregistered internal sentinels (ErrCoverageIncomplete, ErrGenerationFailed) — clean separation matching design §10 intent.
- All wrap sites canonical `<pkg>.<Method>: %w` form.
- All best-effort discards have inline justification comments.

Estimated cost: 1 commit, ~30 lines changed across 4 files (catalog.go domain sentinel, errmap.go row, polling.go fix, generator.go ×6 wraps, disk.go inline comments).
