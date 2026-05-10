# audit: backend/internal/app/tool/search/grep_stdlib.go

LOC: 585
Read: full file (lines 1-585)

## Trace

| site# | file:line | snippet | category | classification | reasoning | severity | user_impact | suggested_fix | status |
|---|---|---|---|---|---|---|---|---|---|
| 1 | grep_stdlib.go:118-121 | `re, err := compileGrepRegex(args); if err != nil { return fmt.Sprintf("Invalid regex pattern: %v", err), nil }` | A.1 | OK | regex compile error → tool_result string for LLM. Documented contract (§S3 carve-out). | N-A | — | — | — |
| 2 | grep_stdlib.go:123-126 | `candidates, err := collectCandidates(args, isDir); if err != nil { return "", fmt.Errorf("Grep.execStdlib: %w", err) }` | A.4 | EDGE | §S16: prefix `Grep.execStdlib:` not canonical `searchtool.Grep.execStdlib:`. Same as grep.go #3/#6 etc. | LOW | grep traceability | tighten prefix | FOUND |
| 3 | grep_stdlib.go:179-203 | `walkErr := filepath.WalkDir(args.Path, func(path string, d fs.DirEntry, err error) error { if err != nil { ... return filepath.SkipDir / nil } ... })` | A.1 | OK | per-entry walk error → SkipDir or nil; documented carve-out (lines 181-187: "Unreadable subtree: skip silently. Aborting the whole walk would punish the user for one bad permission bit."). §S3 example explicitly allows this for filesystem walk best-effort. | N-A | — | — | — |
| 4 | grep_stdlib.go:233 | `if ok, _ := doublestar.Match(args.Glob, filepath.Base(path)); ok { ... }` | A.1 | EDGE | §S3: 2nd return value `_` discarded — is `error` per doublestar API. doublestar.Match returns error only when glob pattern itself is malformed; if so, `ok=false` is returned alongside, so caller doesn't do something dangerous. **However** the discard is silent: if glob parses on first invocation but fails on per-path call (won't happen in practice — doublestar parses pattern once), this would silently skip. Per audit-pattern this is documented-style-choice but missing inline `_ = err — <reason>` ritual. Same pattern at line 242. | LOW | malformed glob causes empty results silently | inline comment: `// _ = err — doublestar.Match's error path implies pattern syntax bug; ok=false is the user-actionable signal` | FOUND |
| 5 | grep_stdlib.go:238 | `if r, err := filepath.Rel(absRoot, path); err == nil { rel = filepath.ToSlash(r) }` | A.1 | OK | §S3 carve-out via err==nil branch — Rel error means path not under root, in which case `rel` stays as the original `path` (not silenced). Functionally fine: subsequent doublestar.Match against absolute path still tries. | N-A | — | — | — |
| 6 | grep_stdlib.go:242 | `if ok, _ := doublestar.Match(args.Glob, rel); ok { ... }` | A.1 | EDGE | same as #4 | LOW | same | same | FOUND |
| 7 | grep_stdlib.go:264 | `hit, _ := fileHasMatch(p, re, args.Multiline)` | A.1 | EDGE | §S3: `_` is the count which is intentionally unused in files_with_matches mode — this is a non-error discard, not a §S3 violation per the spec carve-out (§S3 only applies to `error` discards). **HOWEVER** the function signature `fileHasMatch (bool, int)` returns no error, but **internally** it swallows file-open and read errors (line 350-369: returns (false, 0) on err). The function-internal swallow is documented at lines 343-348 ("一个坏文件不污染整次搜索") and is a per-spec carve-out for filesystem walk. Caller-side `_ ` here is fine. | N-A | — | — | — |
| 8 | grep_stdlib.go:292 | `_, count := fileHasMatch(p, re, args.Multiline)` | A.1 | OK | mirror of #7 — `_` is bool, count is the int we want; no error involved. | N-A | — | — | — |
| 9 | grep_stdlib.go:349-369 | `func fileHasMatch(path string, re *regexp.Regexp, multiline bool) (bool, int) { if multiline { data, err := readFileBounded(...); if err != nil { return false, 0 } ... }; f, err := os.Open(path); if err != nil { return false, 0 }; ... }` | A.1 | OK | swallows file-open + read-bounded errors → (false, 0). Documented at lines 343-348 with clear rationale ("一个坏文件不污染整次搜索"). §S3 carve-out: "如果只是清理资源失败且不影响业务，吞 OK." | N-A | — | — | — |
| 10 | grep_stdlib.go:362 | `defer f.Close()` | A.1 | OK | read-only file Close — §S3 spec example explicitly allows this carve-out. | N-A | — | — | — |
| 11 | grep_stdlib.go:397-401 | `f, err := os.Open(path); if err != nil { return nil }; defer f.Close()` | A.1 | OK | same carve-out as #9 — silent on per-file open failure during content-mode scan. | N-A | — | — | — |
| 12 | grep_stdlib.go:401 | `defer f.Close()` | A.1 | OK | same as #10. | N-A | — | — | — |
| 13 | grep_stdlib.go:402-411 | `scanner := bufio.NewScanner(f); scanner.Buffer(make([]byte, 64*1024), maxStdlibScannerLine); var lines []string; for scanner.Scan() { lines = append(lines, scanner.Text()) }; if err := scanner.Err(); err != nil { return nil }` | A.1 | EDGE | §S3: scanner.Err() is checked → return nil (skips file). Same carve-out class as #9 (per-file walk-tolerant). **However**: a token-too-long error (line exceeds 8 MiB) silently produces zero matches for that file — user might wonder why their multi-megabyte file isn't matching. Could log or surface. Acceptable per §S3 documented best-effort but worth a note. | LOW | extreme long-line files (rare, e.g. minified JS bundles) silently skipped | optional: `// scanner.Err nonzero typically = bufio.ErrTooLong on >8MiB line; per-file silent skip is consistent with rg's --max-columns default behavior` | FOUND |
| 14 | grep_stdlib.go:467-470 | `data, err := readFileBounded(path, maxStdlibFileBytes); if err != nil { return nil }` | A.1 | OK | same per-file walk-tolerant skip. readFileBounded returns sentinel-style error message but caller just skips on any err. Documented carve-out. | N-A | — | — | — |
| 15 | grep_stdlib.go:556-565 | `func readFileBounded(path string, limit int64) ([]byte, error) { info, err := os.Stat(path); if err != nil { return nil, err }; if info.Size() > limit { return nil, fmt.Errorf("file exceeds %d-byte multiline scan cap", limit) }; return os.ReadFile(path) }` | A.4 | EDGE | §S16: `fmt.Errorf("file exceeds %d-byte multiline scan cap", limit)` — NO pkg.Method prefix, NO sentinel. Caller (fileHasMatch / scanFileContentMultiline) discards the error and returns nil/zero — so the error message never reaches errmap. Fine in practice. | LOW | none in practice — the error is internally swallowed | optional: `searchtool.readFileBounded:` prefix; or convert to a sentinel `errFileTooBig` for caller introspection (not currently needed) | FOUND |

## Sub-check

A.1 §S3 错误吞没:
  - violations: not present (sites #3, #4, #6, #7, #8, #9, #10, #11, #12, #13, #14 are all walk-tolerant filesystem-best-effort skips that §S3 spec explicitly carves out; sites #4, #6, #13 noted as LOW for missing inline ritual comment but not functional violations)

A.2 §S9 detached ctx 终态写:
  - terminal-state writes identified: none
  - 各自 ctx 来源: N/A
  - violations: N/A — package is filesystem-search; no DB writes

A.3 §S15 ID 生成:
  - ID generation calls: none
  - violations: N/A — package doesn't generate business IDs

A.4 §S16 错误 wrap 格式:
  - violations: site #2 (LOW prefix), site #15 (LOW no prefix on internal-only error)

A.5 §S17 sentinel 登记 errmap:
  - sentinels defined: none in this file
  - 已登记 errmap: N/A
  - missing: N/A — file defines no sentinels
