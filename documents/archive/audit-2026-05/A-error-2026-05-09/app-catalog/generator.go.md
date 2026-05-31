# generator.go — Phase A audit

**Path**: `backend/internal/app/catalog/generator.go`
**LOC**: 213
**Role**: LLM-driven Summary builder (`LLMGenerator` implementing the `Generator` interface). Single-attempt design (post-2026-05-08 屎山拯救计划 #7). All failures wrap to `catalogdomain.ErrGenerationFailed` so `Service.Refresh` falls back to `mechanicalFallback`. Plus prompt builder + `trimResp` helper.

## Trace

| site# | file:line | snippet | category | classification | reasoning | severity | user_impact | suggested_fix | status |
|---|---|---|---|---|---|---|---|---|---|
| 1 | generator.go:71-81 | `NewLLMGenerator(...)` — `if log == nil { log = zap.NewNop() }` | A.1 | OK | Defensive nil-handling, not error suppression. zap.NewNop is the documented stdlib pattern. | — | — | — | — |
| 2 | generator.go:96-98 | `if len(items) == 0 { return mechanicalFallback(items, gMap), nil }` | A.1 | OK | Empty-input optimization: don't waste an LLM call when there's literally nothing to summarize. mechanicalFallback returns header-only output (no items section). Documented; not silent fallback. | — | — | — | — |
| 3 | generator.go:100-103 | `bundle, err := llmclientpkg.Resolve(...); if err != nil { return nil, fmt.Errorf("%w: resolve LLM: %v", catalogdomain.ErrGenerationFailed, err) }` | A.1/A.4 | VIOLATION | **§S16 violation**: format string is `"%w: resolve LLM: %v"`. The outer `%w` wraps `ErrGenerationFailed` correctly, but the inner err is rendered with `%v` which **drops the inner sentinel chain**. If `llmclientpkg.Resolve` returned an error wrapping e.g. `apikeydomain.ErrNotFoundForProvider` or `modeldomain.ErrNotConfigured`, callers using `errors.Is` against those sentinels can no longer discriminate the underlying cause. Per §S16 spec: "禁止: 自创新前缀代替 %w 包装：return fmt.Errorf("xxx: %v", err) ❌（%v 不能 unwrap）". Same anti-pattern flagged as MED in app-mcp install.go #5. **Also missing pkg.Method qualifier** — should be `catalogapp.Generator.Generate: ...` or similar. **However**, classification is mitigated to MED rather than HIGH because: (a) the err is fully absorbed by Service.Refresh at polling.go:227 (caller never sees it; mechanical fallback always runs); (b) catalog never propagates this err to a handler (per design §10). The only consumer is the Warn log at polling.go:228 — `zap.Error(err)` calls err.Error() which still includes the `%v`-rendered text, so debug visibility is preserved. **Net impact**: pure §S16 style nonconformance with documented internal-only consumption. | LOW | None — err absorbed inside polling.go::Refresh; logged Warn only. No handler path. errors.Is still works against ErrGenerationFailed (the outer %w is correct). | Change to `fmt.Errorf("catalogapp.Generator.Generate: %w: resolve LLM: %w", catalogdomain.ErrGenerationFailed, err)` for full Go-1.20 multi-wrap chain. Cosmetic in current call graph; correct on principle. Same fix needed at sites #4, #5, #6, #7, #8. | FOUND |
| 4 | generator.go:115-118 | `raw, err := llminfra.Generate(...); if err != nil { g.log.Warn("catalog generation LLM call failed", zap.Error(err)); return nil, fmt.Errorf("%w: %v", catalogdomain.ErrGenerationFailed, err) }` | A.1/A.4 | VIOLATION | **Same §S16 issue as #3** (`%v` for inner err drops chain). **Also**: this swallows llminfra sentinels — could be `llminfra.ErrAuthFailed` / `ErrRateLimited` / `ErrBadRequest` etc., all of which are registered in errmap.go:183-187. The Warn log preserves err text but `errors.Is(err, llminfra.ErrAuthFailed)` from any caller of LLMGenerator.Generate would fail. Same MED→LOW mitigation: catalog absorbs the err so no caller actually does this. The §S16 deviation is real but consequence is cosmetic. | LOW | Same as #3 — err absorbed inside polling.go. No external caller does errors.Is on the inner llm sentinel. | Change to `fmt.Errorf("catalogapp.Generator.Generate: %w: %w", catalogdomain.ErrGenerationFailed, err)`. | FOUND |
| 5 | generator.go:120-124 | `if len(raw) > generatorOutputCharCap { ... return nil, fmt.Errorf("%w: output exceeded %d chars", catalogdomain.ErrGenerationFailed, generatorOutputCharCap) }` | A.4 | OK | This site does NOT have an inner err to wrap — the `%d` is just a literal value, not an error. Correct use of `%w` against ErrGenerationFailed. Missing `<pkg>.<Method>:` qualifier per §S16 (style). Same LOW-style flag. | LOW | None functional. | Add `catalogapp.Generator.Generate:` prefix. | FOUND |
| 6 | generator.go:126-131 | `jsonStr, ok := llmparsepkg.ExtractJSON(raw); if !ok { ... return nil, fmt.Errorf("%w: no JSON in response", catalogdomain.ErrGenerationFailed) }` | A.4 | OK | `ExtractJSON` returns (string, bool) — there's no underlying error to wrap. `%w` against ErrGenerationFailed is the only wrap; correct. Missing pkg.Method qualifier (style only). | LOW | None functional. | Add `catalogapp.Generator.Generate:` prefix. | FOUND |
| 7 | generator.go:137-140 | `if err := json.Unmarshal(...); err != nil { ... return nil, fmt.Errorf("%w: parse JSON: %v", catalogdomain.ErrGenerationFailed, err) }` | A.4 | VIOLATION | **Same §S16 issue as #3, #4** (`%v` for inner err). json.Unmarshal errors are typed (`*json.SyntaxError`, `*json.UnmarshalTypeError`); using `%w` would let callers `errors.As` to extract them. Same MED→LOW mitigation. | LOW | Same — internal-only consumption. | Change to `fmt.Errorf("catalogapp.Generator.Generate: %w: parse JSON: %w", catalogdomain.ErrGenerationFailed, err)`. | FOUND |
| 8 | generator.go:142-145 | `if strings.TrimSpace(parsed.Summary) == "" { ... return nil, fmt.Errorf("%w: empty Summary", catalogdomain.ErrGenerationFailed) }` | A.4 | OK | No inner err; `%w` against ErrGenerationFailed is correct. Missing pkg.Method qualifier (style only). | LOW | None functional. | Add `catalogapp.Generator.Generate:` prefix. | FOUND |
| 9 | generator.go:147-152 | `return &catalogdomain.Catalog{Summary: parsed.Summary, Coverage: parsed.Coverage, GeneratedBy: "llm"}, nil` | A.1 | OK | Coverage from LLM passed verbatim per design doc §7 ("LLM 返的 Coverage 原样透传不校验") + §11 testing notes. Documented contract. Not silent fallback. | — | — | — | — |
| 10 | generator.go:188-206 | `buildPrompt(items, gMap)` — pure string assembly with strings.Builder + Fprintf | A.1 | OK | No I/O; Fprintf to strings.Builder cannot fail (same as mechanical.go). | — | — | — | — |
| 11 | generator.go:208-213 | `trimResp(s, n)` — pure helper | A.1 | OK | No error path. | — | — | — | — |
| 12 | generator.go:107-114 | `llminfra.Generate(ctx, bundle.Client, llminfra.Request{...})` — request struct uses `bundle.Key` / `bundle.BaseURL` | A.5 | OK | `llminfra.Generate` returns possible sentinels (ErrAuthFailed, ErrRateLimited, etc., all registered in errmap.go:183-187). Caller absorbs → no escape to handler → §S17 satisfied because catalog policy is internal-consumption (not unmapped). | — | — | — | — |

## Sub-checks

A.1 §S3 错误吞没:
  - violations: not present
  - rationale: every error path either (a) logs Warn before wrapping (sites #4) or (b) wraps + returns to caller (sites #3, #5-8). The mechanicalFallback substitution happens at the Service.Refresh layer (polling.go:227-231) where it's loud-logged. Generator itself does not do silent fallback — it returns the err and lets caller decide.

A.2 §S9 detached ctx 终态写:
  - terminal-state writes identified: none in this file (generator is read-only — produces a struct, no DB / disk / SSE writes)
  - 各自 ctx 来源: ctx is passed through to llmclientpkg.Resolve (apikey lookup) + llminfra.Generate (LLM HTTP call); no ctx-derived writes here
  - violations: N/A: file performs no terminal writes. The catalog's only terminal write is in polling.go::Refresh (already audited).

A.3 §S15 ID 生成:
  - ID generation calls: none — generator outputs a Catalog with caller-supplied IDs; no random ID generation
  - violations: N/A: package doesn't generate business IDs.

A.4 §S16 错误 wrap 格式:
  - violations: 3 sites with inner-err `%v` chain-loss (sites #3, #4, #7); 6 sites with missing `<pkg>.<Method>:` qualifier (sites #3-8 all use bare `"<msg>"` instead of `"catalogapp.Generator.Generate: <msg>"`)
  - all severity LOW because catalog absorbs Generator errors before any handler / errors.Is consumer; consequences are cosmetic (debug log readability) not functional
  - cluster-pattern: same fix template across all 6 sites — recommend single-sweep commit if fixed

A.5 §S17 sentinel 登记 errmap:
  - sentinels defined: none in this file
  - sentinels CONSUMED: catalogdomain.ErrGenerationFailed (wrapped 6×); transitively llminfra.* (wrapped via #4)
  - 已登记 errmap: N/A — catalogdomain.ErrGenerationFailed intentionally NOT in errmap per design §10
  - missing: not present in this file (Generator errors are consumed internally by polling.go::Refresh, never reach a handler)

## Spot-check

- Verified single-attempt design matches design doc §7 post-#7-cleanup commitment. No retry loop; no missing-id hint augmentation; no callLLMWithKeyRotation. Code matches doc.
- Verified `bundle.Client` / `bundle.Key` / `bundle.BaseURL` come from llmclientpkg.Resolve — same pattern as forge / skill / mcp search per design doc §7 prompt block 5 + §8.3 main.go wiring.
- Verified Generator outputs `GeneratedBy: "llm"` literal — matches mechanicalFallback's `"mechanical-fallback"` enum from mechanical.go:72. Catalog domain doesn't validate this string (it's documentary).
- Verified `generatorOutputCharCap = 10 * 1024` matches design doc §7 quote ("~10 KB defensive cap").
- Verified Coverage pass-through (no validation) is documented + intentional — design doc §7 + comment line 88-89.
- Cross-checked llminfra.ErrAuthFailed / ErrRateLimited registration in errmap.go:183-187: confirmed registered. So even though catalog %v-wraps them at site #4, if the chain were ever exposed via a handler (e.g. future feature), they'd map cleanly. The fix to `%w` at #4 is forward-compatible insurance.
