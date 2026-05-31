# disk.go — Phase A audit

**Path**: `backend/internal/app/catalog/disk.go`
**LOC**: 89
**Role**: Atomic JSON read/write for `~/.forgify/.catalog.json`. `loadFromDisk` returns 3-state (cached/missing/corrupted-with-bak-move). `saveToDisk` does `mkdir -p` + `WriteFile(.tmp)` + `Rename`.

## Trace

| site# | file:line | snippet | category | classification | reasoning | severity | user_impact | suggested_fix | status |
|---|---|---|---|---|---|---|---|---|---|
| 1 | disk.go:39-46 | `raw, err := os.ReadFile(path); if err != nil { if errors.Is(err, fs.ErrNotExist) { return nil, nil } return nil, fmt.Errorf("catalog: read %s: %w", path, err) }` | A.1/A.4 | OK | Distinguishes ENOENT (benign first-launch) from other I/O errors (permission, etc.). Wrap uses `%w` and includes path context. Caller (Service.Start) treats `(nil, nil)` as "no cache yet" + treats `(nil, err)` as logged warning + empty cache. §S3 fine — no silent loss; missing file is a documented success state per catalog.md §6 cold-start. | — | — | — | — |
| 2 | disk.go:47-58 | `if err := json.Unmarshal(raw, &cat); err != nil { ... bak := path + ".bak"; _ = os.Rename(path, bak); return nil, fmt.Errorf("catalog: parse %s (moved to %s): %w", path, bak, err) }` | A.1/A.4 | EDGE | The `_ = os.Rename(path, bak)` swallows a Rename error without zap.Warn. Inline comment (lines 49-55) explicitly justifies "best-effort — if the move itself fails (perms) we still return the parse error so the caller doesn't accidentally trust the bad file". §S3 carve-out condition met (justification comment present), but per §S3 spec the exact pattern allowed is `_ = err` **with inline comment explaining why** — this is a `_ = call()` form. Marginal: the comment IS in the surrounding block, not on the line. **LOW severity** because (a) intent documented, (b) the parse error is the actual surfaced failure, (c) bak rename failing is essentially harmless (user can manually inspect the original at `path` since we don't truncate). Could improve by adding zap.Warn for observability. | LOW | None — bad file persists in place if rename fails; user can still inspect at original path. Parse error still surfaces. | Optional: add `if err := os.Rename(path, bak); err != nil { logFromCaller.Warn("catalog: bak rename failed", zap.Error(err)) }` but loadFromDisk is currently log-less — would require threading log through. WAIVE-able; well-documented existing comment. | FOUND |
| 3 | disk.go:71-73 | `if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil { return fmt.Errorf("catalog: mkdir %s: %w", filepath.Dir(path), err) }` | A.4 | OK | `%w` wrap with path context. §S16-compliant pkg locator. | — | — | — | — |
| 4 | disk.go:75-78 | `raw, err := json.MarshalIndent(cat, "", "  "); if err != nil { return fmt.Errorf("catalog: marshal: %w", err) }` | A.4 | OK | Same. | — | — | — | — |
| 5 | disk.go:79-81 | `tmp := path + ".tmp"; if err := os.WriteFile(tmp, raw, 0o644); err != nil { return fmt.Errorf("catalog: write tmp: %w", err) }` | A.4 | OK | Same. | — | — | — | — |
| 6 | disk.go:82-86 | `if err := os.Rename(tmp, path); err != nil { _ = os.Remove(tmp); return fmt.Errorf("catalog: rename: %w", err) }` | A.1/A.4 | EDGE | `_ = os.Remove(tmp)` swallows Remove error without inline comment. **Different from #2**: the rationale here ("clean up partial write before returning original Rename error") is less obvious — Remove failing leaves a stale `.catalog.json.tmp` file. Per §S3 spec carve-out: must have inline comment justifying. Currently has none. **LOW severity** because (a) function still surfaces the meaningful Rename error, (b) stale .tmp is cosmetic — next saveToDisk overwrites it via WriteFile. Could improve with comment + zap.Warn. | LOW | None — stale `.catalog.json.tmp` left until next save. Not visible to user; no data loss. | Add inline comment: `// best-effort cleanup; surface the Rename error as the real failure` and (optionally) zap.Warn on Remove failure. WAIVE-able. | FOUND |
| 7 | disk.go (overall) | format string `"catalog: read %s: %w"` etc. | A.4 | EDGE-style | All 5 wrap sites use prefix `"catalog: <verb>:"` (e.g. `catalog: read`, `catalog: parse`, `catalog: mkdir`, `catalog: marshal`, `catalog: write tmp`, `catalog: rename`). §S16 canonical pattern is `<pkg>.<Method>:`. The prefix here is `<pkg>:` only — no method/function name. Compared with `apikeystore.List: ...` style elsewhere in codebase, this is technically off-pattern but consistent within disk.go. | LOW | None functional — sentinel chain is intact (%w preserved); this is grep-context style only. | Could rename to e.g. `catalog.loadFromDisk: read %s: %w` and `catalog.saveToDisk: mkdir ...`. Pure style. WAIVE-able. | FOUND |

## Sub-checks

A.1 §S3 错误吞没:
  - violations: 2 LOW EDGE (sites #2, #6)
  - rationale: site #2 has inline comment (justified per §S3 carve-out spirit) but uses `_ = call()` form not `_ = err` — borderline. Site #6 lacks justification comment entirely — true §S3 deviation but trivial-impact (Remove of `.tmp`).
  - both flagged LOW because impact is cosmetic / next-write self-heals.

A.2 §S9 detached ctx 终态写:
  - terminal-state writes identified: `saveToDisk` is invoked from polling.go::Refresh (line 248) — see polling.go audit
  - 各自 ctx 来源: N/A — this file does not accept ctx; saveToDisk uses direct os syscalls without context
  - violations: N/A: package os file APIs don't take ctx. The §S9 concern (cancel-mid-write losing terminal state) doesn't apply at the file API level — only at the ctx-level call in polling.go::Refresh. See polling.go audit for §S9 verdict on the disk-write call site.

A.3 §S15 ID 生成:
  - ID generation calls: none
  - violations: N/A: package doesn't generate IDs. Atomic-rename uses fixed `.tmp` / `.bak` suffixes, not unique IDs.

A.4 §S16 错误 wrap 格式:
  - violations: 1 LOW EDGE-style (site #7 — `catalog:` prefix lacks `.<Method>` qualifier across all 6 wrap sites)
  - all 6 sites use `%w` correctly; sentinel chain integrity preserved (caller can `errors.Is` against fs.ErrNotExist etc.)
  - missing: pkg.Method qualifier (style consistency, not functional defect)

A.5 §S17 sentinel 登记 errmap:
  - sentinels defined: none
  - 已登记 errmap: N/A
  - missing: N/A: file defines no sentinels.

## Spot-check

- Verified `loadFromDisk` 3-state contract (cached/nil-nil/nil-err) matches Service.Start switch at polling.go:46-67.
- Verified atomic .tmp + rename pattern matches `infra/mcp/config.go` reference (per file header).
- Verified 0644 perms (vs mcp.json 0600) is documented + correct (catalog has no secrets).
- Verified `errors.Is(err, fs.ErrNotExist)` is the canonical Go 1.13+ way to detect ENOENT — caller `Service.Start` switch at polling.go:46 correctly handles `err == nil && cached == nil` as the missing-file case (no error needed in caller).
