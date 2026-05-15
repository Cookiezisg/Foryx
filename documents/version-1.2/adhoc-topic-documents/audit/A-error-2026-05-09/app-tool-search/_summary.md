# Package audit summary: internal/app/tool/search

## Spec understanding (Phase A — §S3 / §S9 / §S15 / §S16 / §S17)

- **§S3 错误不吞**: error suppression that hides user-visible failure / data loss / config drift is forbidden. Filesystem-walk best-effort skips (per-entry permission denied, malformed file) are explicit §S3 carve-outs ("不影响业务" / "清理资源失败"). Silent fallback that runs different backend without log/audit is the canonical anti-pattern (B2 lesson).
- **§S9 detached ctx 终态写**: N/A for this package — pure filesystem-search tools, no DB writes anywhere.
- **§S15 ID 生成**: N/A — package does not generate business IDs.
- **§S16 错误 wrap 格式**: `fmt.Errorf("<pkg>.<Method>: %w", err)` canonical. The package uses `<Type>.<Method>:` form (e.g. `Glob.ValidateInput:`, `Grep.execRg:`) without the `searchtool.` package qualifier. Project convention elsewhere (apikey.HTTPTester.Test:, mcpapp.Service.Method:) qualifies with package alias. Borderline-compliant; flagged LOW for traceability.
- **§S17 errmap 单一事实源**: ErrEmptyPattern + ErrInvalidOutputMode (in grep.go) are local to ValidateInput which the chat ReAct loop converts to friendly tool_result strings before they ever reach `responsehttpapi.FromDomainError`. Per §S17 carve-out for "完全包内 / 跨包但只在 service 层消费" — registration not required.

## Scope correction (parent-prompt drift)

Parent fork prompt described this package as containing "search backend 路由（Brave/Serper/Tavily/Bocha）" but those live in `app/tool/web/`. This package is **filesystem search** (Grep + Glob system tools). Audit performed on the actual filesystem-search code; web-search routing is out-of-scope and untouched.

## Files audited

| File | LOC | Sites | OK | POST-FIX | VIOLATION | EDGE |
|---|---|---|---|---|---|---|
| search.go | 62 | 2 | 2 | 0 | 0 | 0 |
| glob.go | 303 | 13 | 7 | 0 | 0 | 6 |
| grep.go | 285 | 9 | 3 | 0 | 1 | 5 |
| grep_rg.go | 171 | 3 | 2 | 0 | 0 | 1 |
| grep_stdlib.go | 585 | 15 | 11 | 0 | 0 | 4 |
| **TOTAL** | **1406** | **42** | **25** | **0** | **1** | **16** |

## Severity breakdown (only VIOLATION + EDGE)

| Severity | Count | Sites | Status |
|---|---|---|---|
| HIGH | **1** | grep.go:#9 (rg→stdlib silent fallback — same defect class as B2 bash auto-route silent fallback fixed in commit 888739c; rg backend can rot indefinitely without operator alarm; PCRE-only patterns silently produce different/empty results from RE2 stdlib) | **FOUND — recommend fix** |
| MED | 0 | — | — |
| LOW | 16 | §S16 prefix style (`<Type>.<Method>:` vs canonical `searchtool.<Type>.<Method>:`) — 8 sites; caller-validation `errors.New` without sentinel — 3 sites; missing inline ritual comment on documented `_ = err` carve-outs — 5 sites | FOUND |

## Cross-cutting

### **CRITICAL — rg→stdlib silent fallback (HIGH §S3)**

`grep.go:269-279` silently falls back from ripgrep to stdlib when rg fails for any reason. **This is the same defect class as B2 (bash auto-route silent fallback)** that commit 888739c fixed.

Failure modes hidden today:
1. **rg binary corrupted or PATH changed**: every Grep call silently degrades to slower stdlib indefinitely. Operator never notices.
2. **PCRE-only regex feature**: e.g. `(?<=foo)bar` (lookahead) — rg accepts and matches; Go RE2 stdlib rejects with "error parsing regexp: invalid or unsupported Perl syntax". Today: silent fallback → user thinks they got results, actually got "Invalid regex pattern" tool_result. Or worse, the lookahead pattern is rewritten by user to a non-lookahead form which rg matches differently than RE2 — silently different result sets.
3. **rg version drift**: new rg flag breaks: same outcome.

Recommended fix (mirroring B2's approach):
- Either (a) surface to LLM in tool_result: "rg backend failed: <err>; fallback stdlib results below; pattern features like lookahead/backreference may not match" — gives LLM agency to decide.
- Or (b) operator-side `t.log.Warn(...)` so it lands in dev log even if user-facing is unchanged. Requires adding `*zap.Logger` field to Grep struct.

### Sentinel chain integrity (§S17)

| Sentinel | File | Reaches errmap? |
|---|---|---|
| `ErrEmptyPattern` | grep.go:39 | NO — converted to tool_result by chat loop before `FromDomainError` |
| `ErrInvalidOutputMode` | grep.go:43 | NO — same |

Both correctly unregistered per §S17 carve-out.

### §S16 wrap-format consistency

Package internal style is consistent: every `fmt.Errorf` uses `<TypeName>.<methodName>:` prefix (e.g. `Glob.Execute:`, `Grep.execRg:`). Canonical §S16 spec literal would prefer the package qualifier (`searchtool.Glob.Execute:`) but project elsewhere uses both forms. Single sweep commit could standardize but value is marginal.

### §S9 verification

Confirmed N/A: no DB writes, no terminal-state operations, no detached-ctx need. All operations are pure filesystem reads.

### §S15 verification

Confirmed N/A: no business ID generation. The package does not call `idgenpkg.New` and does not synthesize hex IDs.

## Spot-check (random clean sites — verify mechanism not rubber-stamping)

Random seed: 6 sites picked from `OK` set across 5 files:

1. **search.go:#1** (LookPath result `_` discard): verified — inline comment `// err = not in PATH; treat as fallback` makes the discard rationale explicit. §S3 spec example carve-out ("`_ = err` 带行内注释说明为什么吞").
2. **glob.go:#7** (pathGuard.Allow returning reason as tool_result): verified — pathGuard explicitly returns `(ok bool, reason string)` API; reason becomes the user-facing tool_result on rejection. Not a silent path.
3. **glob.go:#11** (ctx.Err break in walk loop): verified — partial results returned on cancel is documented design choice; LLM gets "what we found before cancel" rather than no result. Not §S9 because no DB write.
4. **grep_rg.go:#1** (rg exit code 1 = no matches, treated as nil): verified against doc comment lines 44-49 explicit exit-code semantics. Compliant with §S3 documented-intent.
5. **grep_stdlib.go:#3** (WalkDir per-entry skip): verified against lines 181-187 doc comment explicitly citing rationale. §S3 carve-out for filesystem walk best-effort.
6. **grep_stdlib.go:#9** (`fileHasMatch` returning (false, 0) on file-open error): verified against lines 343-348 doc comment ("一个坏文件不污染整次搜索"). §S3 carve-out compliant.

All 6 spot-checks confirmed correct classification. The audit's primary find (grep.go:#9 HIGH §S3 rg→stdlib silent fallback) survives spot-check pressure: the package's other §S3 carve-outs are all properly documented and walk-tolerant by design, making the single silent-without-log fallback at grep.go:#9 stand out as a clear defect pattern (and one with prior precedent at B2).

## Recommended fix priorities

1. **grep.go:#9** (HIGH §S3 — rg→stdlib silent fallback) — **immediate fix**. Same defect class as B2 fixed in 888739c. Adding `*zap.Logger` field + Warn before fallback is the minimum; surfacing to LLM in tool_result is even better. Same level of priority as the bash auto-route silent fallback.

2. **§S16 prefix consistency** (LOW × 8) — single sweep commit could replace `Glob.<Method>:` → `searchtool.Glob.<Method>:` and `Grep.<Method>:` → `searchtool.Grep.<Method>:` across grep.go, grep_rg.go, grep_stdlib.go, glob.go. Pure style, no behavior change.

3. **caller-validation `errors.New`** (LOW × 3) — could introduce sentinel or wrap with prefix. WAIVE-tier: framework-internal, never reaches errmap.

4. **`_ = err` ritual comments** (LOW × 5) — sites with documented carve-out behavior but missing the inline `// _ = err — <reason>` ritual the spec calls for. Marginal value to add; can WAIVE.

## Out-of-scope notes

- `app/tool/web/` (Brave/Serper/Tavily/Bocha search-backend routing) was the actual subject of the parent prompt's "search backend 路由" reference. That package needs a separate audit fork — flag for follow-up.
- Test files (`glob_test.go`, `grep_test.go`) not audited per directive ("不读 _test.go").
