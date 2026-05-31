# audit: backend/internal/app/tool/search/glob.go

LOC: 303
Read: full file (lines 1-303)

## Trace

| site# | file:line | snippet | category | classification | reasoning | severity | user_impact | suggested_fix | status |
|---|---|---|---|---|---|---|---|---|---|
| 1 | glob.go:104-107 | `if cwd, err := os.Getwd(); err == nil { a.Path = cwd }` | A.1 | EDGE | §S3: silent fallthrough — if `os.Getwd()` fails, `a.Path` stays "" and downstream `pathGuard.Allow("")` will reject. So the failure surfaces, but no log records *why* cwd resolution failed (rare: chdir into deleted dir). The downstream rejection IS the visible signal, which is acceptable per §S3 ("不影响业务"). | LOW | none in practice — pathGuard catches empty path with a clean reason; user sees "path required" rather than "cwd unavailable". | optional `// _ = err — cwd unavailable falls to pathGuard's empty-path rejection`; no functional change | FOUND |
| 2 | glob.go:169-171 | `if err := json.Unmarshal(args, &a); err != nil { return fmt.Errorf("Glob.ValidateInput: %w", err) }` | A.4 | EDGE | §S16: pkg.Method prefix uses `Glob.ValidateInput:` not the canonical `searchtool.Glob.ValidateInput:` form; package name `searchtool` is the §S13 alias for callers, while the type itself is `Glob`. Project pattern (other tools like `apikey.HTTPTester.Test:`) qualifies with package name. Functionally OK (sentinel chain preserved via %w). | LOW | grep traceability slightly weaker — `Glob.` could collide with future Glob types. | tighten to `searchtool.Glob.ValidateInput:` for consistency, OR accept as project convention given package alias is `searchtool` only at call sites | FOUND |
| 3 | glob.go:172-174 | `if a.Pattern == "" { return ErrEmptyPattern }` | A.4 | OK | direct sentinel return at validation site (deepest layer). | N-A | — | — | — |
| 4 | glob.go:175-177 | `if a.Path != "" && !filepath.IsAbs(a.Path) { return errors.New("path must be absolute when provided") }` | A.4 | EDGE | §S16: NO pkg.Method prefix + NO sentinel + uses `errors.New` for caller-side validation. Caller is `Tool.ValidateInput` interface which framework propagates as `tool_result` string to LLM, NOT through `responsehttpapi.FromDomainError`, so unmapped-domain-error alarm won't fire. **However** errors.Is can't discriminate from other validation errors in this same function. | LOW | LLM sees clear text "path must be absolute when provided" with no operator log noise (validation errors don't reach errmap). Style inconsistency vs site #3. | introduce sentinel like `ErrPathMustBeAbsolute` OR wrap with `searchtool.Glob.ValidateInput:` prefix | FOUND |
| 5 | glob.go:178-180 | `if a.Limit < 0 { return errors.New("limit must be non-negative") }` | A.4 | EDGE | same pattern as #4 | LOW | same | same | FOUND |
| 6 | glob.go:203-205 | `if err := json.Unmarshal([]byte(argsJSON), &args); err != nil { return "", fmt.Errorf("Glob.Execute: %w", err) }` | A.4 | EDGE | §S16: same prefix-style note as #2 (Glob.Execute vs searchtool.Glob.Execute). | LOW | same | same | FOUND |
| 7 | glob.go:208-210 | `if ok, reason := t.pathGuard.Allow(args.Path); !ok { return reason, nil }` | A.1 | OK | pathGuard returns (bool, reason-string). The "reason" surfaces to LLM as the tool result, NOT swallowed. §S3 not violated. | N-A | — | — | — |
| 8 | glob.go:213-219 | `info, err := os.Stat(root); if err != nil { if errors.Is(err, fs.ErrNotExist) { return "Search root not found: " + root, nil }; return fmt.Sprintf("Cannot access %s: %v", root, err), nil }` | A.1/A.4 | OK | filesystem error converted to LLM-readable tool_result string (per Execute's design contract, lines 188-200). The `%v` is intentional — error becomes user-facing text, not propagated as Go error. **However** §S3 concerned with "silent" — here the error IS reported back, just as a string vehicle. Compliant. | N-A | — | — | — |
| 9 | glob.go:220-222 | `if !info.IsDir() { return "Search root must be a directory: " + root, nil }` | A.4 | OK | tool_result string contract; not an error path. | N-A | — | — | — |
| 10 | glob.go:229-232 | `relMatches, err := doublestar.Glob(os.DirFS(root), pattern); if err != nil { return fmt.Sprintf("Invalid glob pattern %q: %v", args.Pattern, err), nil }` | A.1 | OK | same string-vehicle pattern as #8; doublestar parse error → friendly string. | N-A | — | — | — |
| 11 | glob.go:236-237 | `if ctx.Err() != nil { break }` | A.4 | OK | ctx cancellation correctly checked inside loop; break exits with whatever's already collected. Note this means *partial* results return on cancel — caller (LLM) sees what was found before cancel. **Borderline**: should partial results be returned, or should we error out? Current behavior is reasonable for an interactive tool — partial is more useful than nothing. No log either way. | N-A | — | — | — |
| 12 | glob.go:243-246 | `st, err := os.Lstat(full); if err != nil { continue // unreadable entry — silently skip; consistent with rg/Walk pattern }` | A.1 | OK | §S3 carve-out: per-entry filesystem error during walk is documented best-effort. Inline comment cites the rationale ("consistent with rg/Walk pattern"). The Glob walk should not fail-loud on a single unreadable file — that would defeat the search-purpose. Compliant per §S3 spec ("如果只是清理资源失败且不影响业务，吞 OK"). | N-A | — | — | — |
| 13 | glob.go:277-280 | `body, err := json.MarshalIndent(out, "", "  "); if err != nil { return "", fmt.Errorf("Glob.Execute: marshal result: %w", err) }` | A.4 | EDGE | §S16: same prefix-style note as #2/#6. Marshal of well-typed struct is unfailable in practice (no NaN/Inf/cyclic from primitives), but defensive wrap is correct style. | LOW | unreachable in practice | optional comment noting unfailable; or accept as defensive | FOUND |

## Sub-check

A.1 §S3 错误吞没:
  - violations: not present (sites #1, #7, #8, #10, #11, #12 all surface the error appropriately — either via tool_result string, ctx-break, or documented walk-tolerant skip)

A.2 §S9 detached ctx 终态写:
  - terminal-state writes identified: none
  - 各自 ctx 来源: N/A
  - violations: N/A — package is filesystem-search; no DB writes anywhere

A.3 §S15 ID 生成:
  - ID generation calls: none
  - violations: N/A — package doesn't generate business IDs

A.4 §S16 错误 wrap 格式:
  - violations: sites #2, #4, #5, #6, #13 (LOW — `Glob.<Method>:` prefix vs canonical `searchtool.Glob.<Method>:`; sites #4 #5 use `errors.New` for caller-validation strings without sentinel — but framework-internal so unmapped-error alarm won't fire)

A.5 §S17 sentinel 登记 errmap:
  - sentinels defined: none in this file (ErrEmptyPattern lives in grep.go per file comment lines 54-57)
  - 已登记 errmap: N/A
  - missing: N/A — file defines no sentinels
