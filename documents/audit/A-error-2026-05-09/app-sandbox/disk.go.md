# audit: backend/internal/app/sandbox/disk.go

LOC: 86
Read: full file (lines 1-86)

## Trace

| site# | file:line | snippet | category | classification | reasoning | severity | user_impact | suggested_fix | status |
|---|---|---|---|---|---|---|---|---|---|
| 1 | disk.go:29-42 | `_ = filepath.WalkDir(root, func(...) error { if err != nil { return nil // skip broken entries } ... })` | A.1 | OK | §S3 carve-out: file header doc-comment lines 1-9 explicitly states "failures are logged inside Service rather than returned, since these are best-effort adjuncts to manifest persistence". `_ = filepath.WalkDir(...)` discard has rationale: this is a best-effort size estimator for UI display; misreporting "0" is the documented graceful degradation. The inner `return nil` on entry error is also documented intent ("skip broken entries"). | N-A | — | — | — |
| 2 | disk.go:36-39 | `info, err := d.Info(); if err != nil { return nil }` | A.1 | OK | Same documented design as site #1 — broken entries skipped silently per file-header rationale. The `return nil` continues the walk past one bad entry rather than aborting. Documented intent + has rationale. | N-A | — | — | — |
| 3 | disk.go:54-63 | `func removeAll(path string) error { if !filepath.IsAbs(path) { return os.ErrInvalid } clean := filepath.Clean(path); if isFilesystemRoot(clean) { return os.ErrInvalid }; return os.RemoveAll(clean) }` | A.4 | EDGE | Returns `os.ErrInvalid` directly (sentinel from stdlib `os` package). No pkg.Method prefix. **Reasoning to NOT flag as VIOLATION**: `os.ErrInvalid` is a stdlib well-known sentinel — wrapping with `fmt.Errorf("sandbox.removeAll: %w", os.ErrInvalid)` would still preserve it. The bare-return preserves the sentinel; just lacks call-site loc. | LOW | identical UX (errors.Is(err, os.ErrInvalid) works either way) | wrap with `fmt.Errorf("sandbox.removeAll: invalid path %q: %w", path, os.ErrInvalid)` for grep-traceability | FOUND |
| 4 | disk.go:62 | `return os.RemoveAll(clean)` (bare passthrough) | A.4 | OK | `os.RemoveAll` returns its own well-known err shape; bare-return at this level is canonical Go for "I'm just delegating to stdlib". This isn't propagating a domain sentinel; it's a thin wrapper. Documented as "guard against catastrophic paths" — the guard is the validation, not the wrapping. | N-A | — | — | — |

## Sub-check

A.1 §S3 错误吞没:
  - violations: not present (sites #1, #2 documented soft-degrade with explicit file-header rationale)

A.2 §S9 detached ctx 终态写:
  - terminal-state writes identified: none
  - 各自 ctx 来源: N/A
  - violations: N/A — file is pure utility (filesystem walk + remove); no DB writes, no terminal-state operations, no ctx threading

A.3 §S15 ID 生成:
  - ID generation calls: none
  - violations: N/A — file does not generate business IDs

A.4 §S16 错误 wrap 格式:
  - violations: site #3 (LOW — `os.ErrInvalid` returned bare; could be wrapped for grep-traceability but functionally OK since stdlib sentinel preserved)

A.5 §S17 sentinel 登记 errmap:
  - sentinels defined: none
  - 已登记 errmap: N/A
  - missing: N/A — file defines no sentinels (only consumes stdlib `os.ErrInvalid` which doesn't need errmap registration as it never reaches handler — caller in sandbox.go translates to domain sentinel)
