# Package audit summary: internal/app/skill

## Spec understanding (Phase A — §S3 / §S9 / §S15 / §S16 / §S17)

- **§S3 错误不吞**: silent suppression that hides user-visible failure / data loss / config drift is forbidden. `_ = err` requires inline justification. Documented soft-fail with audit log (zap.Warn) is acceptable carve-out per spec; silent fallthrough without log is the canonical anti-pattern. The package's per-skill scan loop (scan.go:#3, #4) and the polling loop (polling.go:#1, #4) are textbook soft-fail-with-log examples.
- **§S9 detached ctx 终态写**: terminal-state writes that MUST land regardless of caller cancel use `reqctxpkg.SetUserID(context.Background(), uid)`. **In skill package the on-disk SKILL.md file IS the source of truth** — the in-memory cache is a 1s-polled index; if a request ctx cancels mid-mutation after the file is written, the file persists and the next polling tick re-syncs the cache. Distinct from apikey.Test where test_status is in-DB-only. Therefore skill mutations correctly use request ctx without §S9 violation.
- **§S15 ID 生成**: package does NOT generate business IDs. Skill names are user-supplied strings (via frontmatter.name or directory basename) that double as stable identifiers — same pattern as MCP curated registry's short-slug naming. Not subject to §S15 `<prefix>_<16hex>` format.
- **§S16 错误 wrap 格式**: `fmt.Errorf("<pkg>.<Method>: %w", err)` canonical. Outer-layer methods (skill.go::Get, mutate.go::Create/Replace/Delete, activate.go::Activate, scan.go::Scan, search.go::Search) all use the canonical form correctly. Helper-only functions (writeSkillDir, parseSkillDir, readBodyWithRetry, parseRankedIndices) use helper-style prefix `<helper>:` instead — caller wraps with full pkg.Method, so call-site context preserved at outer layer; LOW EDGE for grep traceability.
- **§S17 errmap 单一事实源**: 5 skilldomain sentinels (ErrSkillNotFound / ErrInvalidFrontmatter / ErrBodyTooLarge / ErrNameConflict / ErrInvalidName) all registered in errmap.go:149-153. No new sentinels defined in this package.

## Files audited

| File | LOC | Sites | OK | POST-FIX | VIOLATION | EDGE |
|---|---|---|---|---|---|---|
| skill.go | 168 | 4 | 4 | 0 | 0 | 0 |
| scan.go | 259 | 14 | 9 | 0 | 2 | 3 |
| activate.go | 188 | 9 | 6 | 0 | 0 | 3 |
| mutate.go | 214 | 14 | 10 | 0 | 0 | 4 |
| import.go | 149 | 9 | 9 | 0 | 0 | 0 |
| polling.go | 102 | 4 | 4 | 0 | 0 | 0 |
| search.go | 164 | 9 | 7 | 0 | 0 | 2 |
| catalogsource.go | 53 | 4 | 4 | 0 | 0 | 0 |
| **TOTAL** | **1297** | **67** | **53** | **0** | **2** | **12** |

## Severity breakdown (only VIOLATION + EDGE)

| Severity | Count | Sites | Status |
|---|---|---|---|
| HIGH | 0 | — | — |
| MED | 2 | scan.go:#9 (`%w: %v` truncates inner err in splitFrontmatter wrap), scan.go:#10 (same pattern in yaml.Unmarshal wrap) | FOUND |
| LOW (§S16 helper-style prefix) | 7 | scan.go:#7 (`read SKILL.md:`), mutate.go:#11 #12 #13 #14a (writeSkillDir helper), activate.go:#8 #9 (readBodyWithRetry), search.go:#8 #9 (parseRankedIndices) | FOUND |
| LOW (§S17 no-sentinel defensive validation) | 2 | scan.go:#1 (empty skillsDir wiring-bug), activate.go:#6 (fork-without-subagent wiring-bug) | FOUND |
| LOW (§S3 missing inline ritual comment) | 2 | scan.go:#6 (yaml.Marshal err discard in fingerprint — provably unfailable), mutate.go:#14b (`_ = os.Remove(tmp)` cleanup-after-rename-fail) | FOUND |
| LOW (§S16 bare-errors.New for inner-fence errors) | 1 | scan.go:#13 (splitFrontmatter open/close-fence bare strings — only meaningful after #9 #10 fixed) | FOUND |

## Cross-cutting

### Sentinel chain integrity (§S17)

All 5 skilldomain sentinels (errmap.go:149-153) verified consumed:

| Sentinel | errmap.go line | Wrapped at |
|---|---|---|
| `ErrSkillNotFound` | 149 | skill.go:148, mutate.go:46, mutate.go:113 (Replace), mutate.go:141 (Delete), activate.go:60 |
| `ErrInvalidFrontmatter` | 150 | scan.go:162 (#9 — `%v` issue), scan.go:167 (#10 — `%v` issue), scan.go:240, scan.go:243, scan.go:255 |
| `ErrBodyTooLarge` | 151 | scan.go:157, mutate.go:179 (validateBodySize), activate.go:71 |
| `ErrNameConflict` | 152 | mutate.go:80 (Create) |
| `ErrInvalidName` | 153 | mutate.go:161 (validateName) |

**No missing registrations**. The two `%w: %v` violations at scan.go:#9 #10 preserve the **outer** sentinel (ErrInvalidFrontmatter) but truncate the **inner** fence/yaml-parse error. Same defect class as mcp install.go:#5 (resolved 505d6e3) + infra/sandbox sites (resolved d6b626f).

### Detached ctx coverage (§S9) — distinguishing from apikey.Test pattern

**Terminal-state writes inventory:**

| Write | File / Site | Ctx | §S9 verdict |
|---|---|---|---|
| writeSkillDir to disk (Create) | mutate.go:#5 | request ctx (passed to writeSkillDir → os.WriteFile/os.Rename) | ✓ **OK — file IS source of truth** |
| Scan rebuild after write (Create/Replace/Delete) | mutate.go:#6, #8 | request ctx | ✓ OK — Scan builds in-memory index; cancel just delays cache by ≤1s (next poll tick) |
| os.RemoveAll (Delete) | mutate.go:#8 | request ctx | ✓ OK — atomic dir removal; cancel either succeeded or didn't |
| Import batch writes + post-batch Scan | import.go:#8, #9 | request ctx | ✓ OK — same source-of-truth reasoning |
| notif.Publish on fingerprint change | scan.go:#5 | request ctx (or boot/poll ctx) | ✓ OK — best-effort SSE; subscriber missing one snapshot is recovered by next 1s tick |
| SetActiveSkill on AgentState | activate.go:#4 | (no ctx — in-memory state) | ✓ OK — in-memory only |
| Subagent.Spawn (fork mode) | activate.go:#7 | request ctx | ✓ OK — subagent run IS bounded by user's request lifetime; subagent's own §S9 obligations audited separately |

**§S9 verdict for package**: **fully compliant**. The 1s polling loop + on-disk source-of-truth means cache writes are NOT terminal in the §S9 sense — failed cache rebuilds self-heal at next tick. Distinct from apikey.Test where test_status lives ONLY in DB and a cancel-mid-write leaves stale state with no recovery path. The package design is structurally §S9-correct without needing detached ctx.

### Per-row error UX pattern (import.go) — model §S3 example

import.go is a textbook §S3 + per-row UX example for the package: every err encountered in batch processing goes into ImportResult.Errors with the file's name (lines 86, 93, 100, 107, 113, 124, 135). NEVER silently dropped. UI gets per-row outcome rendering for free. Other packages with batch-processing should follow this pattern.

### Helper-style prefix consistency

7 LOW EDGE findings cluster around helpers using `<helper>:` instead of canonical `<pkg>.<helper>:`:
- scan.go::parseSkillDir (`read SKILL.md:`)
- mutate.go::writeSkillDir (4 wraps: mkdir / marshal / write tmp / rename)
- activate.go::readBodyWithRetry (2 bare-returns)
- search.go::parseRankedIndices (2 wraps: no JSON / parse JSON)

In every case the **caller** wraps with canonical `skillapp.<Outer>:` so the call-site context is preserved at outer layer. Helper-style is internally consistent within the package — same precedent as forge audit and infra/llm audit's `consistency-over-strict-literal` WAIVE.

### Defensive-validation no-sentinel pattern

2 LOW EDGE findings:
- scan.go:#1 (empty skillsDir) — wiring-bug invariant; same family as mcp.go:#6 (resolved as panic)
- activate.go:#6 (fork-without-subagent) — wiring-bug invariant; main.go always wires subagent

Both are pure wiring-bugs that should never trigger in production. Recommend resolution as **panic** for consistency with apikey.HTTPTester / mcp.AddServer / sandbox.EnsureEnv pattern, OR introduce sentinels (`skilldomain.ErrSkillsDirUnconfigured`, `skilldomain.ErrSubagentUnavailable`) + register errmap for defensive completeness.

## Spot-check (random clean sites — verify mechanism not rubber-stamping)

Random seed: 7 sites picked from `OK` set across files:

1. **skill.go:#3** (Get returning ErrSkillNotFound directly): verified — innermost-layer sentinel return per §S16 spec line 88 ("直接返 sentinel（最里层无需 wrap）"). errmap.go:149 → 404. Compliance literal.
2. **scan.go:#3** (per-skill skip with Warn log): verified — file header §S3 rationale at lines 32-36 explicitly cites "one bad skill must not silence the catalog"; Warn log includes dir + Error so author can debug. §S3 documented soft-fail carve-out.
3. **mutate.go:#9** (validateName ErrInvalidName wrap): verified — `fmt.Errorf("skillapp.validateName: %w: %q (must match %s)", skilldomain.ErrInvalidName, name, nameRegexp.String())`. pkg.Method + sentinel + diagnostic context. Sentinel registered errmap.go:153 → 422.
4. **import.go:#9** (post-batch Scan with %w wrap): verified — `fmt.Errorf("skillapp.Import: post-batch rescan: %w", err)`. Returns `(res, err)` so caller still sees per-file outcomes.
5. **activate.go:#5** (depth guard for nested-fork): verified — `s.log.Info("skill activated within subagent; ignoring fork directive", ...)` with depth + skill name; documented design rationale at file header lines 41-45 (§9.5 invariant). NOT a silent skip — Info-level audit trail.
6. **search.go:#5** (rank parse failure → alpha fallback): verified — `s.log.Warn(... fields including response_snippet ...)` then returns alpha-order top K. File header lines 79-84 cite the documented soft-fail intent. UX continuity preserved.
7. **polling.go:#4** (rescan err Warn log): verified — `s.log.Warn("skill rescan failed", zap.Error(err))`. File header lines 84-86 explicitly cite per-tick retry rationale. §S10 audit log + §S3 carve-out compliance.

All 7 spot-checks confirmed correct classification — mechanism functioning, not rubber-stamping. The package's design (file-system source-of-truth + 1s polling + soft-fail-with-log) is structurally compliant with §S3 + §S9; the 2 MED findings are isolated `%w: %v` truncations following the same defect class already fixed across 3 prior packages.

## Recommended fix priorities

1. **scan.go:#9 + #10** (MED §S16 — `%w: %v` truncates inner err in ErrInvalidFrontmatter wraps) — 2-line fix swap `%v` for `%w` (Go 1.20+ multi-wrap). Same pattern as mcp install.go:#5 (resolved 505d6e3) and infra/sandbox sites (resolved d6b626f). HIGH PRIORITY for §S16 chain consistency.

2. **scan.go:#1 + activate.go:#6** (LOW §S17 — defensive-validation sites with no sentinel) — coordinated decision needed: panic per "config-time invariant" pattern (consistent with mcp.AddServer / apikey.HTTPTester / sandbox.EnsureEnv resolutions), OR introduce 2 sentinels (`skilldomain.ErrSkillsDirUnconfigured`, `skilldomain.ErrSubagentUnavailable`) + register errmap. Recommend **panic** for consistency.

3. **§S16 helper-style prefix consistency** (7 LOW EDGE — scan / mutate / activate / search helpers) — pure-style sweep matching forge / infra-llm audits' "consistency-over-strict-literal" WAIVE precedent. Caller-wraps preserves call-site context at outer layer; **recommend WAIVE**.

4. **search.go:#8 + #9** (LOW §S16 EDGE — parseRankedIndices wraps) — could also wrap with `llminfra.ErrProviderError` to mirror commit 64d9535 in app-tool-forge for cross-package consistency. Pure refactor, no UX change.

5. **scan.go:#6** (LOW §S3 — yaml.Marshal err discard in fingerprint) — add inline ritual comment per §S3 spec: `// _ = err — yaml.Marshal of basic-type frontmatter struct is unfailable; fingerprint stays stable`.

6. **mutate.go:#14b** (LOW §S3 — `_ = os.Remove(tmp)` cleanup-after-rename-fail) — same; add inline ritual.

## Out-of-scope notes (parent should verify)

1. **subagent.Service.Spawn semantics for fork-mode skill** — activate.go:#7 propagates Spawn errors as-is. Cross-fork concern: if subagent audit finds Spawn returns sentinel-less errors, fork-mode skill failures would hit unmapped errmap. Subagent audit not yet completed (not in this package's scope).
2. **app/tool/skill** — the LLM-facing tools (search_skills, activate_skill) live in a separate package per skill.md §8. Audit the Tool 9-method §S18 conformance + tool result UX hygiene there separately.
3. **catalog generator integration** — skill catalogsource.go is consumed by app/catalog; changes to Item.ID stability (currently skill.Name) would ripple through catalog fingerprinting. Cross-fork concern only on schema changes.
