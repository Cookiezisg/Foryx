# Package audit summary: internal/app/tool/forge

## Spec understanding (Phase A — §S3 / §S9 / §S15 / §S16 / §S17)

- **§S3 错误不吞**: error suppression that hides user-visible failure / data loss / config drift is forbidden. Documented soft-fails with audit trails OK; silent fallthrough without log is the canonical anti-pattern.
- **§S9 detached ctx 终态写**: terminal-state writes that MUST persist regardless of caller cancel use detached ctx. **In this package**: tool layer delegates to t.svc.<Method>; the §S9 obligation lives in app/forge service (DEFERRED pending forge rewrite). Tool-level ctx is request ctx — synchronous tool execution where caller-cancel cleanly aborts, so no §S9 violation at this layer.
- **§S15 ID 生成**: business IDs flow through `forgeapp.NewForgeID()` / `NewVersionID()` (delegate to idgenpkg per §S15). Tool layer doesn't generate IDs directly.
- **§S16 错误 wrap 格式**: canonical form is `<pkg>.<Method>:` prefix with `%w`. **Package-wide deviation**: all 5 forge tools use the LLM-facing tool name as prefix (`search_forges:` / `get_forge:` / `create_forge:` / `edit_forge:` / `run_forge:`) instead of canonical `forgetool.<Type>.<Method>:`. Style is internally consistent; functional UX identical. Audit-recommended WAIVE per "consistency-over-strict-literal" precedent (other tool packages had similar).
- **§S17 errmap 单一事实源**: 9 forgedomain sentinels (errmap.go:80-88) all consumed correctly via t.svc.<Method>. Tool layer defines NO sentinels.

## Files audited

| File | LOC | Sites | OK | POST-FIX | VIOLATION | EDGE |
|---|---|---|---|---|---|---|
| forge.go     | 197 |  5 | 1 | 0 | 0 |  4 |
| search.go    | 170 | 13 | 5 | 0 | 0 |  8 |
| get.go       |  90 |  7 | 3 | 0 | 0 |  4 |
| create.go    | 215 | 11 | 4 | 0 | 0 |  7 |
| edit.go      | 227 | 12 | 5 | 0 | 0 |  7 |
| run.go       | 111 |  8 | 4 | 0 | 0 |  4 |
| **TOTAL** | **1010** | **56** | **22** | **0** | **0** | **34** |

## Severity breakdown (only VIOLATION + EDGE)

| Severity | Count | Sites | Status |
|---|---|---|---|
| HIGH | 0 | — | — |
| MED | 0 | — | — |
| LOW (§S16 prefix style — package-wide) | 31 | All `<tool_name>:` prefix instead of `forgetool.<Type>.<Method>:` form across 5 tools + 4 helper-style sites in forge.go (`resolveAttachments:` / `streamCode:`) | FOUND |
| LOW (§S16 missing sentinel) | 1 | search.go:#9 (no-JSON response wraps no sentinel; could use llminfra.ErrProviderError per commit 363b084) | FOUND |
| LOW (§S3 silent w/o log) | 1 | search.go:#12 (json.Unmarshal silent on DB-corrupted forge.Parameters/ReturnSchema; documented intent + nolint:errcheck ritual present, but no Warn log — also no logger field on SearchForge struct) | FOUND |
| LOW (§S3 unreachable but explicit) | 1 | run.go:#7 (json.Marshal `err==nil &&` guard for size-check; oversize-skip silent on Marshal-fail. Unreachable in practice — Python output is basic types.) | FOUND |

## Cross-cutting

### Sentinel chain integrity (§S17)

All forge sentinels (errmap.go:80-88) consumed via t.svc.* method calls. Tool layer is pure delegation; no own sentinels. Cross-domain consumers also clean:
- `chatdomain.Err*` (errmap.go:55-66): via repo.GetAttachment in run.go #5
- `apikeydomain.Err*` (errmap.go:67-79): via llmclientpkg.Resolve indirectly
- `llminfra.Err*` (errmap.go:177-181, post-363b084): via streamCode + Generate paths — sentinel chain fully discriminative for HTTP 401/429/etc.
- `forgedomain.Err*` (errmap.go:80-88): via svc methods

**No missing registrations**.

### §S16 prefix style — package-wide pattern

All 5 forge tools use the LLM-facing tool name as the fmt.Errorf prefix. This is intentional grep/log readability ("which tool surfaced the error?") but deviates from §S16's canonical `<pkg>.<Type>.<Method>:` form. Equivalent practice in other audited packages (mcp / web / sandbox tools) was either accepted as-is or mass-renamed. Decision is largely cosmetic since:
- Sentinel chains are intact (all use `%w`)
- Error log readability is high (tool name immediately identifies origin)
- errmap matching works regardless

**Recommended WAIVE** per "consistency-over-strict-literal" precedent set by other tool packages.

### Behavior divergence: corrupted forge.Parameters across get/search

- **get.go #5/#6**: surfaces `corrupted parameters / return_schema for forge "f_xxx": <err>` — clean error, user sees specific forge_id
- **search.go #12**: silently swallows the same Unmarshal failure — keeps the forge in search ranking with `parameters: null`, leaving the LLM to discover the corruption when it later calls run_forge

Documented design intent in search.go (lines 156-158) but the **divergence** between "search shows the broken forge" and "get refuses the broken forge" may surprise. **Recommended fix** (LOW): add Warn log in search.go #12 path so corruption surfaces in operator log without changing the search-doesn't-abort contract. Requires adding logger field to SearchForge struct (similar to chat history.go::BlocksToAssistantLLM logger threading in commit 26f9c55).

### Tool result UX hygiene (cross-fork Phase C preview)

Quick scan for tool-result anti-patterns flagged earlier (teaching errors / implementation leaks / self-promoting errors):

| Tool | Description quality | Result error quality | Anti-pattern? |
|---|---|---|---|
| search_forges | clear single-purpose; mentions get_forge as next-step (legitimate hint, not self-promo) | wrapped errors with %w + truncated context | clean |
| get_forge | clear; mentions verifying before running | clean errors with forge_id context | clean |
| create_forge | longer Description with env_status flow + dep notes — appropriate complexity for create flow | "please regenerate" actionable hint on AST fail (POSITIVE — LLM can react) | clean |
| edit_forge | dual-path noted in Description (instruction = code regen, no instruction = metadata-only) | similar actionable hints | clean |
| run_forge | explicitly tells LLM "Execution failures return ok=false (not an error)" — clear contract; output truncation 50KB documented | result has ok/output/error/elapsed_ms shape; truncation message is explicit | clean |

**No teaching-style errors / implementation leaks / self-promoting copy detected** (Phase C preview clean).

## Spot-check (random clean sites — verify mechanism not rubber-stamping)

5 sites picked from `OK` set across files:

1. **forge.go:#5** (extractCode): pure utility, no error returns — N/A category accurate. Function is straightforward fence-stripping; no §S3/§S9 surface.
2. **search.go:#2** (RequiresWorkspace=false): verified — search reads forge library only; no user-fs touch. §S18 metadata accurate.
3. **search.go:#13** (Marshal of result slice): verified — `result` struct fields are string + already-unmarshaled `any` (basic types from json.Unmarshal). encoding/json invariant: such Marshal is unfailable. Discard `_` safe-by-construction.
4. **create.go:#5** (NewForgeID/NewVersionID): verified — both delegate to forgeapp's idgenpkg-backed ID generators which panic on rand.Read fail per §S15.
5. **run.go:#1** (Description with truncation contract): verified — description explicitly tells LLM about ok=false contract + 50KB truncation; manages LLM expectations directly. POSITIVE example of clean §S18 description copy.

All 5 spot-checks confirmed correct classification — mechanism functioning, not rubber-stamping. The audit's primary findings (31 §S16 prefix LOW + 1 silent-corruption-without-log + 1 missing-sentinel + 1 unreachable-Marshal-skip) are real but uniformly LOW; the package is **architecturally clean** modulo the package-wide prefix style choice.

## Recommended fix priorities

1. **search.go:#9** (LOW §S16/§S17 — no-JSON LLM response wraps no sentinel) — wrap with `llminfra.ErrProviderError` per the post-363b084 sentinel pattern. 1-line fix.

2. **search.go:#12** (LOW §S3 — silent corrupted-forge in search results) — add logger field to SearchForge struct + log Warn on Unmarshal-fail. ~10-line refactor (logger threading similar to commit 26f9c55).

3. **§S16 package-wide prefix migration** (LOW × 31) — WAIVE per package consistency / audit precedent. If user wants strict literal compliance, batch sweep can convert all 5 tools' `<tool_name>:` → `forgetool.<Type>.<Method>:` prefix. Pure style; no functional change.

4. **run.go:#7** (LOW §S3 — Marshal err silent on size-check) — re-structure as explicit if/else for clarity. Unreachable in practice; could WAIVE.

## Out-of-scope notes (parent should verify)

1. **app/forge service §S9 obligations DEFERRED** per app-forge audit (forge backend rewriting). Tool layer correctly delegates; no §S9 fix possible at this layer until rewrite.
2. **forge_redesign/** untracked dir noted earlier — same forge rewrite signal. Tool-layer fixes here may carry over after rewrite (LLM-facing tool API stable across rewrites).
3. **streamCode helper** could benefit from logger threading similar to BlocksToAssistantLLM pattern (commit 26f9c55) for malformed-stream observability — but currently propagates errors via %w which is sufficient.
