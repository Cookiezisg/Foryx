# Audit trace: backend/cmd/resources/main.go

**File**: `backend/cmd/resources/main.go`
**LOC**: 330
**Audit categories**: §S3 / §S9 / §S15 / §S16 / §S17

> **Posture**: This is a build-time tool (`make resources` / `go run ./cmd/resources`) that downloads + verifies + extracts mise binaries into the source tree for `go:embed`. **It is NOT a service** — runs once, exits. By spec extracts:
> 1. **§S3** — Download / hash / extraction failures must be **fail-loud**. Critical: a silent platform fallback or hash-mismatch swallow ships a binary that won't work / is unverified.
> 2. **§S9** — N/A: build-time CLI; no request contexts; no terminal-state writes that need detached ctx.
> 3. **§S15** — Should NOT generate business IDs (this writes binaries on disk, not DB rows).
> 4. **§S16** — `fmt.Errorf` wraps must follow `<pkg>.<Method>: %w`. Note this is a `package main` so `<pkg>` becomes implicit; spec language is service-oriented but the principle (informative prefix + `%w` over `%v`) still applies.
> 5. **§S17** — N/A: no domain sentinels; no errmap path. Errors here surface to operator stderr via `log.Fatalf` / `return err` chains.

---

## 9-column trace

| site# | file:line | snippet | category | classification | reasoning | severity | user_impact | suggested_fix | status |
|---|---|---|---|---|---|---|---|---|---|
| 1 | resources.go:107-114 | `version := os.Getenv("MISE_VERSION"); if version == "" { fmt.Println("→ resolving latest mise release ..."); version = mustLatestTag("jdx/mise") } if !strings.HasPrefix(version, "v") { version = "v" + version }` | A.1 | OK | §S3 — env-not-set falls through to `mustLatestTag` which is itself fail-loud (log.Fatalf on any error path). The "default to latest" behavior is documented in package godoc L22-24. Not a silent fallback — it's documented degraded mode → still fail-loud on actual download failure. | — | — | — | — |
| 2 | resources.go:116-119 | `targets := []platform{currentPlatform()}; if *allPlatforms { targets = supported }` | A.1 | OK | §S3 — clear platform selection logic; `currentPlatform()` is fail-loud on unsupported host (site 4). | — | — | — | — |
| 3 | resources.go:124-127 | `if !*force && fileExists(out) { fmt.Printf("✓ already present: %s\n", out); continue }` | A.1 | OK | §S3 — `continue` here is not silent-skip-on-error; it's a documented "already cached" optimization. fileExists at L327-330 doesn't distinguish stat error from "not exists" but for this use case (cache-hit check) any stat error correctly falls through to redownload (file-not-readable behaves like file-not-present, redownload tries again). Reasonable. ⚠ Marginal — a permission-denied stat would mask a real FS issue, but redownload would surface it via writeBinary's open call. Pragmatic. | LOW | `fileExists` collapses "doesn't exist" with "stat error" (e.g. permission-denied). Practical impact: redownload + open → real error surfaces at write time. Not fatal. | (optional) `fileExists` could distinguish `os.IsNotExist(err)` from other errors, but the redownload path handles both correctly. Cosmetic. | — |
| 4 | resources.go:128-130 | `if err := os.MkdirAll(p.outDir(), 0o755); err != nil { log.Fatalf("mkdir %s: %v", p.outDir(), err) }` | A.1 / A.4 | OK | §S3 fail-loud. Note: `log.Fatalf("mkdir %s: %v", ..., err)` uses `%v` not `%w` — but **this is `log.Fatalf` (terminates process), not error wrapping for upstream propagation** — `%v` is correct here (no caller to unwrap). Spec extract §S16 applies to errors that propagate; `log.Fatalf` is the terminal sink. ✓ | — | — | — | — |
| 5 | resources.go:131-133 | `if err := fetchOne(version, p); err != nil { log.Fatalf("%s: %v", p.key(), err) }` | A.1 / A.4 | OK | §S3 fail-loud; §S16 — same as site 4: `%v` is correct in log.Fatalf (terminal sink). | — | — | — | — |
| 6 | resources.go:147-156 | `currentPlatform() platform { for _, p := range supported { if p.goos == runtime.GOOS && ... { return p } } log.Fatalf("unsupported host platform %s/%s; mise embed only ships %d targets", ..., len(supported)) return platform{} }` | A.1 | OK | §S3 fail-loud. Unsupported host → log.Fatalf with explicit message naming the host platform + count of supported targets. Operator gets actionable error. The trailing `return platform{}` is unreachable but required for Go compiler. ✓ Spec-conforming; **no silent platform fallback** (which is the audit risk for this file). | — | — | — | — |
| 7 | resources.go:166-169 | `body, err := httpGetBytes(url); if err != nil { return fmt.Errorf("download: %w", err) }` | A.4 | OK | §S16 — uses `%w` correctly. Prefix is `download:` (action-named, not pkg.method). Given this is a package-main build tool with linear call chain (`fetchOne` → `httpGetBytes`), action-named prefixes are operationally readable in stderr trace. ⚠ Strict §S16 wants `<pkg>.<Method>`. Looser interpretation: `fetchOne` is the calling method name; `download:` describes the action that failed. **Acceptable for build-time CLI** because there's no service-layer caller doing `errors.Is` / no errmap match path. | LOW | Wrap prefix is action-named (`download:`) not method-named (`fetchOne.download:`). Per strict §S16: `<pkg>.<Method>: %w`. For build-time CLI with linear chains, action-naming is operationally clearer; spec-strict interpretation flags it. | (optional, strict-spec) `fmt.Errorf("fetchOne.download: %w", err)`. Practical value low — operator sees the chain as `<platform>: download: <real err>` either way. | — |
| 8 | resources.go:177-180 | `sums, err := httpGetBytes(sumsURL); if err != nil { return fmt.Errorf("download SHASUMS256.txt: %w", err) }` | A.1 / A.4 | OK | §S3 — fail-loud on SHA file fetch. **Critical anti-supply-chain check**: if SHASUMS256.txt download fails, we abort before trusting an unverified binary. ✓ Same §S16 prefix-style observation as site 7. | LOW | Same as site 7 — action-named prefix vs method-named. | (same as site 7) | — |
| 9 | resources.go:181-184 | `want, err := lookupSum(sums, assetName); if err != nil { return fmt.Errorf("checksum lookup: %w", err) }` | A.1 / A.4 | OK | §S3 — fail-loud if SHA lookup fails (asset name not in SHASUMS256.txt → likely upstream rename / version mismatch). **Critical**: never fall back to "skip checksum because we couldn't find the line". ✓ | LOW | Same as site 7 prefix style. | (same) | — |
| 10 | resources.go:185-189 | `gotSum := sha256.Sum256(body); got := hex.EncodeToString(gotSum[:]); if got != want { return fmt.Errorf("sha256 mismatch: want %s got %s", want, got) }` | A.1 | **OK — CRITICAL FAIL-LOUD** | §S3 — **the supply-chain integrity gate**. Hash mismatch → abort with both expected + actual hex. Never silent-fallback. **Gold-standard §S3 compliance** for an external-binary fetcher. ✓ | — | — | — | — |
| 11 | resources.go:192-195 | `if p.archExt == ".tar.gz" { return extractTarGz(body, p.binName, p.outBin()) } return extractZip(body, p.binName, p.outBin())` | A.1 | OK | §S3 — switching on archive format; both branches return errors fail-loud. ✓ | — | — | — | — |
| 12 | resources.go:204-209 | `extractTarGz(...)`: `gz, err := gzip.NewReader(...); if err != nil { return fmt.Errorf("gunzip: %w", err) } defer gz.Close()` | A.1 / A.4 | OK | §S3 — defer gz.Close on read-only path is allowed per spec extract §S3 example. §S16 — `gunzip:` action-name prefix, same observation as site 7. | LOW | Same prefix style. | (same) | — |
| 13 | resources.go:211-218 | `for { hdr, err := tr.Next(); if err == io.EOF { return fmt.Errorf("%s not found in tarball", binName) } if err != nil { return fmt.Errorf("tar next: %w", err) } ... }` | A.1 / A.4 | OK | §S3 — exhausts tarball without finding binary → fail-loud with binary name. ✓ Other tar errors → wrap+propagate. **Stdlib best practice**: `errors.Is(err, io.EOF)` would be more robust than `err == io.EOF`, but for `tar.Reader.Next` the stdlib documentation guarantees plain `io.EOF` (no wrapping), so direct comparison works. Marginal modernization opportunity. | LOW | `err == io.EOF` direct comparison vs `errors.Is(err, io.EOF)`. tar.Reader doesn't wrap, so equivalent in practice. Project-wide pattern is `errors.Is`. | (optional) `if errors.Is(err, io.EOF)` for codebase consistency. Practical impact: zero. | — |
| 14 | resources.go:219-222 | `if hdr.Typeflag != tar.TypeReg || filepath.Base(hdr.Name) != binName { continue } return writeBinary(tr, dst)` | A.1 | OK | §S3 — `continue` skips non-matching entries (intended); first matching regular file is written. Not silent error swallow — error-free iteration over manifest. ✓ | — | — | — | — |
| 15 | resources.go:230-234 | `extractZip(...)`: `zr, err := zip.NewReader(...); if err != nil { return fmt.Errorf("unzip: %w", err) }` | A.1 / A.4 | OK | §S3 fail-loud; §S16 same prefix-style observation. | LOW | Same as site 7. | (same) | — |
| 16 | resources.go:235-247 | `for _, f := range zr.File { if filepath.Base(f.Name) != binName { continue } rc, err := f.Open(); if err != nil { return fmt.Errorf("open zip entry: %w", err) } err = writeBinary(rc, dst); rc.Close(); return err } return fmt.Errorf("%s not found in zip", binName)` | A.1 / A.4 | OK | §S3 — exhausts zip without match → fail-loud. **Subtle**: `rc.Close()` return is discarded; in zip extraction this is unactionable (we already have the bytes; close failure on read-only path doesn't affect correctness). Per §S3 spec extract: "defer f.Close() 在只读路径（Close 返错对调用方无意义）" — this is **not deferred** but the same principle applies. ⚠ Marginal — `rc.Close()` is not deferred so it doesn't fall under that exception verbatim; but the spec's intent (close-error on read-only is unactionable) covers this. Note also that `err = writeBinary(rc, dst); rc.Close(); return err` will return writeBinary's err even if rc.Close() also fails — close failure is masked. Build-time tool; impact bounded. | LOW | `rc.Close()` return discarded inline (not deferred). Per §S3: read-only Close is unactionable but pattern is non-deferred. Marginal. | (optional) `defer rc.Close()` for spec-pattern parity, restructure to `if err := writeBinary(rc, dst); err != nil { return err }`. Cosmetic. | — |
| 17 | resources.go:254-269 | `writeBinary(r io.Reader, dst string)`: `tmp := dst + ".tmp"; out, err := os.OpenFile(tmp, ...); if err != nil { ... } if _, err := io.Copy(out, r); err != nil { out.Close(); _ = os.Remove(tmp); return ... } if err := out.Close(); err != nil { return ... } return os.Rename(tmp, dst)` | A.1 | OK | §S3 — atomic-write pattern. **Subtle**: `out.Close()` after Copy failure (L261) discards close error — but we're already returning the Copy error, and tmp file is deleted via `_ = os.Remove(tmp)`. The `_ = os.Remove(tmp)` is a documented-by-context cleanup-on-error pattern: we already failed; removing the half-written tmp is best-effort. Spec-conforming via §S3 cleanup-in-failure-path exception. ⚠ Marginal: `_ = os.Remove(tmp)` lacks an inline comment explaining why ignored — strict §S3 wants "_ = err **带行内注释**说明为什么吞". | LOW | `_ = os.Remove(tmp)` lacks `// _ = err — best-effort cleanup, real failure already captured` comment. Strict §S3 spec wants reason on every `_ =`. | Add inline comment: `_ = os.Remove(tmp) // best-effort cleanup; copy err already captured`. 1-line touch. | FOUND |
| 18 | resources.go:255-259 | `out, err := os.OpenFile(tmp, ...); if err != nil { return fmt.Errorf("open %s: %w", tmp, err) }` | A.4 | OK | §S16 — `open <path>: %w` action-named; same observation as site 7. | LOW | Same prefix style. | (same) | — |
| 19 | resources.go:260-264 | `if _, err := io.Copy(out, r); err != nil { out.Close(); _ = os.Remove(tmp); return fmt.Errorf("write %s: %w", tmp, err) }` | A.4 / A.1 | OK | §S16 — same prefix style. §S3 — `out.Close()` return discarded after Copy failure (we're already failing; close on a failed write is unactionable). Same posture as site 17. | LOW | Same prefix style + same `_ = os.Remove(tmp)` no-comment as site 17. | (same as site 17) | — |
| 20 | resources.go:265-267 | `if err := out.Close(); err != nil { return fmt.Errorf("close %s: %w", tmp, err) }` | A.1 | OK | §S3 — Close on write path correctly checked (write-Close failure means data may not have flushed; must surface). ✓ | — | — | — | — |
| 21 | resources.go:268 | `return os.Rename(tmp, dst)` | A.4 | OK | §S16 — bare return of os.Rename's error without prefix. ⚠ Strict §S16 wants `<pkg>.<Method>:` prefix. Pragmatic for build-time tool; if rename fails, operator gets stdlib error message + Go file:line in panic-context. **Less spec-conforming than other sites** but consistent with operational utility. | LOW | Bare unwrapped return; no `writeBinary.rename:` prefix. | (optional) `if err := os.Rename(tmp, dst); err != nil { return fmt.Errorf("rename %s → %s: %w", tmp, dst, err) }`. | — |
| 22 | resources.go:281-285 | `lookupSum(sums []byte, assetName string)`: `for _, line := range strings.Split(...) { ... } return "", fmt.Errorf("no entry for %s in SHASUMS256.txt", assetName)` | A.1 | OK | §S3 — exhausts SHASUMS256.txt without match → fail-loud with asset name in message. ✓ | — | — | — | — |
| 23 | resources.go:294-303 | `httpGetBytes(url string)`: `resp, err := http.Get(url); if err != nil { return nil, fmt.Errorf("get %s: %w", url, err) } defer resp.Body.Close(); if resp.StatusCode/100 != 2 { return nil, fmt.Errorf("get %s: status %s", url, resp.Status) } return io.ReadAll(io.LimitReader(resp.Body, 100<<20))` | A.1 / A.4 | OK | §S3 — non-2xx status → fail-loud with status text; defer body.Close on read-only path (spec-allowed); 100 MB cap defends against pathological responses. ⚠ **Subtle**: `io.ReadAll` return is unwrapped — if read fails mid-stream (network drop), error returns bare. Per strict §S16 should wrap. Pragmatic for build-tool. ⚠ **Subtle 2**: `resp.Body.Close()` deferred return discarded — read-only HTTP body, unactionable, spec-allowed. | LOW | `io.ReadAll` return unwrapped; status-code path uses `%s` not `%w` (no inner error to wrap, status is a value-string, so `%s` is correct here). | (optional) `body, err := io.ReadAll(...); if err != nil { return nil, fmt.Errorf("read body: %w", err) }`. | — |
| 24 | resources.go:296-298 | `resp, err := http.Get(url); if err != nil { return nil, fmt.Errorf("get %s: %w", url, err) }` | A.4 | OK | §S16 — `get <url>: %w` action-named. Same observation as site 7. | LOW | Same prefix style. | (same) | — |
| 25 | resources.go:300-301 | `if resp.StatusCode/100 != 2 { return nil, fmt.Errorf("get %s: status %s", url, resp.Status) }` | A.4 | OK | §S16 — `%s` correct here (no inner error to unwrap; `resp.Status` is a string value not an error). ✓ Spec-conforming use of `%s`. | — | — | — | — |
| 26 | resources.go:299 | `defer resp.Body.Close()` | A.1 | OK | §S3 — read-only HTTP body close; spec-allowed exception. ✓ | — | — | — | — |
| 27 | resources.go:303 | `return io.ReadAll(io.LimitReader(resp.Body, 100<<20))` | A.4 | OK | §S16 — bare return of io.ReadAll; if Read fails mid-stream, propagates without context. Strict §S16 wants wrap. **Pragmatic for build-time CLI; the wrapping convention matters most for service code where errors.Is unwraps need to find sentinels**. Here there are no sentinels. | LOW | Bare return; no `httpGetBytes.read: %w` wrap. | (optional) wrap. | — |
| 28 | resources.go:310-325 | `mustLatestTag(repo string) string`: 3 fatal paths (httpGetBytes / Unmarshal / empty tag_name) all use `log.Fatalf`. | A.1 | OK | §S3 fail-loud — all 3 failure modes terminate via log.Fatalf with the specific failure message. ✓ Critical: empty `tag_name` from API → fatal (don't silently fall through to building a URL with empty version). Spec-conforming. | — | — | — | — |
| 29 | resources.go:327-330 | `fileExists(path string) bool { _, err := os.Stat(path); return err == nil }` | A.1 | OK | §S3 — collapses error path. As discussed at site 3, this is a cache-hit pre-check; any failure (not-exists, permission-denied) correctly falls through to redownload. Practical impact: bounded. ⚠ Marginal — strict §S3 would prefer `os.IsNotExist(err)` to distinguish cache-miss from real FS issue. | LOW | Same as site 3. | (optional) Distinguish `os.IsNotExist` from other stat errors. Cosmetic. | — |
| 30 | resources.go (whole file) | (no `idgen.New(...)` calls; no `crypto/rand` use) | A.3 | OK | §S15 — build-time tool writes binary files to disk; does NOT generate business IDs. ✓ N/A. | — | — | — | — |
| 31 | resources.go (whole file) | (no `errors.Is(err, <sentinel>)` against domain sentinels; no errmap path) | A.5 | OK | §S17 — build-time tool, no domain sentinels, no handler path. The sole `err == io.EOF` direct comparison (site 13) is stdlib lifecycle marker, not a domain sentinel. ✓ N/A. | — | — | — | — |
| 32 | resources.go (whole file) | (no goroutines; no terminal-state writes that need detached ctx; no `context.Background()` patterns wired to writes) | A.2 | OK | §S9 — build-time CLI runs once + exits; no requests, no contexts, no terminal writes. ✓ N/A: package doesn't do terminal writes (build-time CLI, no request lifecycle). | — | — | — | — |

---

## Sub-check matrix

### A.1 §S3 错误吞没
- **Concrete violations**: none. **Critical fail-loud sites are gold-standard**:
  - Hash mismatch (site 10) → abort with both want + got values
  - Unsupported host platform (site 6) → log.Fatalf with platform name + count of supported
  - SHASUMS256.txt fetch failure (site 8) → fail-loud (never silently skip checksum)
  - SHASUMS256.txt asset-not-found (site 9) → fail-loud (never fallback to "skip verification")
  - Tarball/zip exhausted without binary (sites 13/16) → fail-loud with binary name
  - GitHub releases API empty `tag_name` (site 28) → fatal (never build URL with empty version)
- **LOW marginal observations**:
  - Site 3/29 (`fileExists` collapses stat-error with not-exists) — practical cache-hit pre-check; redownload path catches real issues.
  - Site 13 (`err == io.EOF` direct comparison) — works with stdlib tar.Reader (no wrapping); project-wide pattern is `errors.Is`. Cosmetic.
  - Site 16 (`rc.Close()` non-deferred read-only close discard) — spec exception covers read-only Close; minor pattern divergence (not deferred).
  - **Site 17 — concrete §S3 LOW**: `_ = os.Remove(tmp)` lacks inline comment explaining why ignored. Strict spec extract §S3 wants `// _ = err — <reason>` on every `_=`. The reason is obvious in context (best-effort cleanup after copy failure) but the spec wants it explicit. **1-line fix; FOUND status.**
- **Net A.1**: 0 HIGH, 0 MED, **1 concrete LOW (site 17 — `_ = os.Remove(tmp)` no reason comment)** + 4 marginal LOW.

### A.2 §S9 detached ctx 终态写
- **Terminal-state writes identified**: **none**. This is a build-time CLI; no DB writes, no request contexts, no SSE, no detached ctx semantics apply.
- **Goroutines**: none.
- **N/A reason**: package doesn't do terminal writes (build-time CLI runs to completion + exits).
- **Violations**: not present.

### A.3 §S15 ID 生成
- **ID generation calls**: none.
- **Self-rolled `crypto/rand` use**: none.
- **N/A reason**: package writes binaries to disk; doesn't generate business IDs.
- **Violations**: N/A: package doesn't generate business IDs.

### A.4 §S16 错误 wrap 格式
- **`fmt.Errorf` calls**: ~12 sites total.
  - All use `%w` correctly when wrapping inner errors (sites 7/8/9/12/15/18/19/20/24).
  - Status-code formatting (site 25) uses `%s` correctly because `resp.Status` is a string, not an error.
  - Bare returns without wrap (sites 21/27 — `os.Rename`, `io.ReadAll`) — strict §S16 wants prefix wrap.
- **Prefix-style observation (cross-cutting LOW)**: prefixes are **action-named** (`download:`, `gunzip:`, `unzip:`, `tar next:`, `open <path>:`, etc.) rather than spec-strict `<pkg>.<Method>:`. This is a build-time CLI with linear call chains — operational readability of stderr trace favors action-naming over method-naming. Consistent with Go stdlib idioms (cmd/ tools). **Spec-strict interpretation flags as LOW**; pragmatic interpretation accepts.
- **Violations**: 0 concrete (all uses of `%v` / bare-return are in `log.Fatalf` terminal sinks where `%v` is the correct stdlib idiom OR are pragmatic build-time choices).
  - Cross-cutting LOW pattern: prefix style. Consistent within the file; diverges from service-code spec strictness. ~10 sites.

### A.5 §S17 sentinel 登记 errmap
- **Sentinels defined in resources.go**: none.
- **Sentinel-handling**: none. Sole `err == io.EOF` is stdlib lifecycle marker.
- **N/A reason**: build-time CLI; no domain sentinels; no handler path; no errmap participation.
- **Violations**: N/A: file defines no sentinels.

---

## Severity summary

| Severity | Count | Sites |
|---|---|---|
| HIGH | 0 | — |
| MED | 0 | — |
| LOW | 1 concrete + ~12 marginal | **Concrete**: site 17 (`_ = os.Remove(tmp)` no inline reason comment per strict §S3). **Marginal cluster**: prefix-style `<action>:` vs spec-strict `<pkg>.<Method>:` (sites 7/8/9/12/15/18/19/24); `fileExists` stat-error collapse (sites 3/29); `err == io.EOF` direct comparison (site 13); inline `rc.Close()` non-deferred (site 16); bare returns at 21/27. |

**Net cmd/resources**: 0 HIGH / 0 MED / 1 concrete LOW + ~12 marginal LOW.

**Architectural assessment**: cmd/resources is **textbook fail-loud** for the high-stakes paths (hash verification, platform support, asset lookup). Operator-facing UX is good — every fail-loud message names the specific failure mode + relevant identifiers (asset name, platform, hash values, status code). Marginal LOWs are stylistic — prefix style favors action-naming over method-naming (consistent with Go cmd/-tool idioms; spec was written for service code).

**Only concrete fix needed**: site 17 — add 1 inline comment to `_ = os.Remove(tmp)` to satisfy strict §S3 "_ = err 带行内注释". 1-line touch; trivial.

**Cross-cutting cosmetic**: ~10 `fmt.Errorf` prefixes could be method-named for spec-strict §S16 parity. Build-time CLI context makes the practical impact zero (no errors.Is unwrap chain consumers).
