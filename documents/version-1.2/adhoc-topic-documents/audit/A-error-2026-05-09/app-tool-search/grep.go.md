# audit: backend/internal/app/tool/search/grep.go

LOC: 285
Read: full file (lines 1-285)

## Trace

| site# | file:line | snippet | category | classification | reasoning | severity | user_impact | suggested_fix | status |
|---|---|---|---|---|---|---|---|---|---|
| 1 | grep.go:36-44 | `var ( ErrEmptyPattern = errors.New("pattern is required..."); ErrInvalidOutputMode = errors.New(...) )` | A.5 | EDGE | §S17: 2 sentinels defined. Both reach Execute via ValidateInput; `Tool.Execute` returns string-or-error to the framework which produces tool_result strings (NOT through `responsehttpapi.FromDomainError`). So they don't strictly need errmap registration, but the unmapped-error alarm path does require all sentinels registered. **However**: ValidateInput error is intercepted by the chat ReAct loop and converted to friendly tool_result string at the loop boundary, so it never reaches `FromDomainError`. Compliant with §S17 carve-out for "完全包内 / 跨包但只在 service 层消费、handler 层翻译成别的 sentinel 的，不需要登记". | LOW | sentinels never reach errmap; unregistered is the correct state | confirm via grep that ValidateInput errors don't propagate to a httphandler — if they ever do, register | FOUND |
| 2 | grep.go:163-167 | `if cwd, err := os.Getwd(); err == nil { a.Path = cwd }` | A.1 | EDGE | same pattern as glob.go:#1 — silent cwd fallback. Same reasoning, same severity. | LOW | same | optional inline comment | FOUND |
| 3 | grep.go:211-213 | `if err := json.Unmarshal(args, &a); err != nil { return fmt.Errorf("Grep.ValidateInput: %w", err) }` | A.4 | EDGE | §S16: prefix `Grep.ValidateInput:` not canonical `searchtool.Grep.ValidateInput:`. Same as glob.go #2. | LOW | grep traceability | tighten to `searchtool.Grep.ValidateInput:` | FOUND |
| 4 | grep.go:223-225 | `if a.After < 0 \|\| a.Before < 0 \|\| a.Around < 0 \|\| a.HeadLimit < 0 { return errors.New("-A / -B / -C / head_limit must be non-negative") }` | A.4 | EDGE | §S16: `errors.New` no sentinel + no pkg.Method prefix. Same as glob.go #4/#5 (validation-only, framework-internal). | LOW | LLM sees clean string | introduce sentinel `ErrInvalidNumericArg` OR wrap with prefix | FOUND |
| 5 | grep.go:226-228 | `if a.Path != "" && !filepath.IsAbs(a.Path) { return errors.New("path must be absolute when provided") }` | A.4 | EDGE | same as #4 / glob.go:#4 | LOW | same | same | FOUND |
| 6 | grep.go:250-252 | `if err := json.Unmarshal([]byte(argsJSON), &args); err != nil { return "", fmt.Errorf("Grep.Execute: %w", err) }` | A.4 | EDGE | §S16: same prefix-style as #3 | LOW | same | same | FOUND |
| 7 | grep.go:255-257 | `if ok, reason := t.pathGuard.Allow(args.Path); !ok { return reason, nil }` | A.1 | OK | pathGuard reason → tool_result string. Same pattern as glob.go #7. | N-A | — | — | — |
| 8 | grep.go:260-266 | `info, err := os.Stat(cleaned); if err != nil { if os.IsNotExist(err) { return "Search root not found: " + cleaned, nil }; return fmt.Sprintf("Cannot access %s: %v", cleaned, err), nil }` | A.1 | OK | filesystem error → tool_result string. Same pattern as glob.go:#8. | N-A | — | — | — |
| 9 | grep.go:269-279 | `if t.rgPath != "" { out, err := t.execRg(ctx, args); if err != nil { return t.execStdlib(ctx, args, info.IsDir()) }; return out, nil }; return t.execStdlib(ctx, args, info.IsDir())` | **A.1** | **VIOLATION** | **§S3 silent fallback — same defect class as B2 bash auto-route fix (commit 888739c)**. When `t.execRg(ctx, args)` returns an error for ANY reason, the code silently swallows it and falls through to `execStdlib`. The inline comment "rg 异常失败 → fallback 到 stdlib，让搜索仍能成功（包文档保证两后端同 surface）" justifies the fallback semantically, but **NO log** records that rg failed. Operator has zero visibility that the rg backend is broken. **Concrete failure modes**: (a) rg binary corrupted or rg ABI changed in update — every Grep call silently degrades to slower stdlib without anyone knowing; (b) rg's PCRE regex pattern that Go's RE2 doesn't support — user thinks they got results from rg but actually got stdlib's RE2 interpretation. The comment claims "包文档保证两后端同 surface" but rg has PCRE-ish features (lookaround, backreferences) that Go RE2 explicitly rejects — surfaces are NOT identical for those patterns. | **HIGH** | LLM sees subtly-different results with no operator log; rg backend can rot indefinitely without alarm. Identical defect pattern to bash auto-route silent fallback that commit 888739c fixed by surfacing error to LLM. | log Warn before fallback: `t.log.Warn("Grep.Execute: rg backend failed, falling back to stdlib", zap.String("rg_path", t.rgPath), zap.Error(err))`. **However Grep struct has no log field currently** — needs adding. **Better**: surface to LLM as part of tool_result so the LLM can decide (e.g. "rg failed: <err>; results below come from stdlib fallback which doesn't support PCRE features"). NOTE: if user's pattern uses PCRE-only feature, stdlib will silently produce empty results — this is the worst case. | FOUND |

## Sub-check

A.1 §S3 错误吞没:
  - violations: site #9 (HIGH — rg→stdlib silent fallback, same class as B2 bash silent fallback fixed in 888739c)

A.2 §S9 detached ctx 终态写:
  - terminal-state writes identified: none
  - 各自 ctx 来源: N/A
  - violations: N/A — package is filesystem-search; no DB writes

A.3 §S15 ID 生成:
  - ID generation calls: none
  - violations: N/A — package doesn't generate business IDs

A.4 §S16 错误 wrap 格式:
  - violations: sites #3, #4, #5, #6 (LOW — prefix style + caller-validation `errors.New`; framework-internal so no errmap reach)

A.5 §S17 sentinel 登记 errmap:
  - sentinels defined: ErrEmptyPattern (line 39), ErrInvalidOutputMode (line 43)
  - 已登记 errmap: neither — but neither needs registration (Tool.ValidateInput errors converted to friendly tool_result by chat ReAct loop, never reach `responsehttpapi.FromDomainError`)
  - missing: N/A — registration not required per §S17 carve-out for framework-intercepted errors
