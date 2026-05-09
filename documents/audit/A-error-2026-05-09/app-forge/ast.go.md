# audit: backend/internal/app/forge/ast.go

LOC: 221
Read: full file (lines 1-221)

## Trace

| site# | file:line | snippet | category | classification | reasoning | severity | user_impact | suggested_fix | status |
|---|---|---|---|---|---|---|---|---|---|
| 1 | ast.go:154-157 | `tmp, err := os.CreateTemp("", "forgify-ast-*.py"); if err != nil { return nil, fmt.Errorf("parseForgeCode: create temp: %w", err) }` | A.4 | EDGE | §S16: prefix is `parseForgeCode:` (function name only) without `forgeapp.` qualifier; %w ✓; sentinel chain not relevant (stdlib err). Same helper-style as flagged in apikey.tester.go LOW. | LOW | grep traceability slightly weaker | tighten to `forgeapp.parseForgeCode: create temp: %w` for spec literal compliance | FOUND |
| 2 | ast.go:158 | `defer os.Remove(tmp.Name())` | A.1 | OK | `defer X.Remove()` on temp file — read-after-close path; remove failure means stale temp file (cleaned up at next reboot or by OS tmp janitor). §S3 carve-out for cleanup-after-use. No log expected. | N-A | — | — | — |
| 3 | ast.go:159-162 | `if _, err = tmp.WriteString(astScript); err != nil { tmp.Close(); return nil, fmt.Errorf("parseForgeCode: write script: %w", err) }` | A.1/A.4 | EDGE | (a) §S16: same helper-style prefix as #1. (b) §S3: `tmp.Close()` discards Close error here — but path is fail-fast (already returning err), so Close error is unreachable info. acceptable per §S3 carve-out for cleanup-on-error path. | LOW (a) | grep traceability | tighten prefix to `forgeapp.parseForgeCode:` | FOUND |
| 4 | ast.go:163 | `tmp.Close()` (success path) | A.1 | EDGE | §S3: Close error discarded silently. Close failure on a temp file with WriteString already succeeded means buffer flush failed — could mean partial astScript content. Subsequent cmd.Output() would parse a partial script and produce a Python SyntaxError (caught and reported via raw.Error path), so this is self-healing. **However** spec example "_ ignore must have inline comment" suggests this should at least be wrapped in a check or commented. | LOW | minor — Close failure leads to garbled astScript → Python parse error → wrapped via errASTProcess; observable via err return | wrap: `if err := tmp.Close(); err != nil { return nil, fmt.Errorf("forgeapp.parseForgeCode: close temp: %w", err) }` OR add `// _ = err — flush already happened in WriteString` comment | FOUND |
| 5 | ast.go:165-173 | `cmd := exec.Command(pythonPath, tmp.Name()); cmd.Stdin = ...; out, err := cmd.Output(); if err != nil { if exitErr, ok := err.(*exec.ExitError); ok && len(exitErr.Stderr) > 0 { return nil, fmt.Errorf("%w: %s", errASTProcess, exitErr.Stderr) } return nil, fmt.Errorf("%w: %v", errASTProcess, err) }` | A.4 | **VIOLATION** | §S16: line 172 uses `%v` not `%w` for the original `err` — sentinel chain breaks. errors.Is(err, errASTProcess) succeeds (wrapped at outer), but errors.Is on the inner err (e.g. exec.ErrNotFound) returns false because %v drops the chain. Same defect class as mcp install.go:#5 `%w: %v` (FIXED in 505d6e3). Also note: prefix is missing pkg.method qualifier on both wraps (just sentinel-only `errASTProcess`). | MED | callers cannot programmatically detect "python binary not found" vs "python crashed" — both wrapped under same sentinel; if user's sandbox bootstrap failed, this wraps the underlying os.PathError but loses it | switch line 172 to `%w: %w`: `return nil, fmt.Errorf("forgeapp.parseForgeCode: %w: %w", errASTProcess, err)`; line 170 also: `return nil, fmt.Errorf("forgeapp.parseForgeCode: %w: %s", errASTProcess, exitErr.Stderr)` (Stderr is []byte literal, no chain to preserve, %s OK) | FOUND |
| 6 | ast.go:191-193 | `if err = json.Unmarshal(out, &raw); err != nil { return nil, fmt.Errorf("parseForgeCode: unmarshal: %w", err) }` | A.4 | EDGE | §S16: helper-style prefix (no pkg qualifier). Functionally OK — %w + sentinel chain preserved. | LOW | minor grep traceability | tighten to `forgeapp.parseForgeCode: unmarshal: %w` | FOUND |
| 7 | ast.go:194-196 | `if raw.Error != "" { return nil, fmt.Errorf("parseForgeCode: %w: %s", errASTProcess, raw.Error) }` | A.4 | EDGE | §S16: helper-style prefix. %w on sentinel ✓; raw.Error is plain string (not error type) so %s is correct here. Minor LOW. | LOW | minor grep traceability | tighten to `forgeapp.parseForgeCode: %w: %s` | FOUND |
| 8 | ast.go:221 | `var errASTProcess = fmt.Errorf("ast parse failed")` | A.5 | OK | sentinel defined locally (lowercase = unexported, package-only). Caller (forge.go) maps this to `forgedomain.ErrASTParseError` (which IS in errmap.go:85). Per §S17 step 3 ("完全包内 / 跨包但只在 service 层消费、handler 层翻译成别的 sentinel 的，不需要登记"), this internal sentinel doesn't need errmap row. | N-A | — | — | — |

## Sub-check

A.1 §S3 错误吞没:
  - violations: not present (sites #2, #3b, #4 are EDGE LOW for cleanup paths; #4 is the closest concern but self-healing through subsequent parse error)

A.2 §S9 detached ctx 终态写:
  - terminal-state writes identified: none
  - 各自 ctx 来源: N/A
  - violations: N/A — file is pure parse helper (subprocess + JSON decode); no DB writes, no terminal-state operations

A.3 §S15 ID 生成:
  - ID generation calls: none (no idgen.New / no self-rand; just file/exec helpers)
  - violations: N/A — package doesn't generate business IDs in this file

A.4 §S16 错误 wrap 格式:
  - violations: site #5 (MED — `%v` instead of `%w` on outer err in cmd.Output failure path; sentinel chain breaks for errors.Is on inner err)
  - LOW EDGE on prefix style: sites #1, #3, #6, #7 (all use `parseForgeCode:` instead of canonical `forgeapp.parseForgeCode:`)

A.5 §S17 sentinel 登记 errmap:
  - sentinels defined: errASTProcess (package-internal, lowercase)
  - 已登记 errmap: N/A — not exported, mapped via forgedomain.ErrASTParseError at higher layer (errmap.go:85)
  - missing: N/A — file's only sentinel is internal; caller translates
