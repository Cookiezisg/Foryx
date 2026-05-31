# Package audit summary: internal/app/tool/skill

## Spec understanding (Phase A — §S3 / §S9 / §S15 / §S16 / §S17)

- **§S3 错误不吞**: error suppression that hides user-visible failure / data loss / config drift is forbidden. The §S18 friendly tool_result pattern (errors.Is sentinel → human text) is the canonical compliant form here — errors are not swallowed but surfaced as readable LLM-facing text. Bare `_ = err` requires inline justification.
- **§S9 detached ctx 终态写**: terminal-state writes that MUST persist regardless of caller cancel use detached ctx. **N/A at this tool layer** — Execute methods delegate to `skillapp.Service.Search` (read-only) and `skillapp.Service.Activate` (in-memory AgentState mutation + optional subagent spawn). State-mutation §S9 concerns belong to the Service layer (covered by app-skill audit).
- **§S15 ID 生成**: `<prefix>_<16hex>` via idgenpkg. **N/A** — skill names are author-supplied directory names under `~/.forgify/skills/<name>/`, not server-generated business IDs (same precedent as MCP server slugs).
- **§S16 错误 wrap 格式**: `fmt.Errorf("<pkg>.<Method>: %w", err)`. Package uses `<tool_name>(.<Method>)?:` helper-style prefix (e.g. `search_skills.Execute:`, `activate_skill.ValidateInput:`, `activate_skill:`) — consistent within package, deviates from strict literal `skilltool.<Type>.<Method>:`. Audit-recommended **WAIVE** per the established "consistency-over-strict-literal" precedent across app-tool-* packages (forge / mcp / shell / search confirmed).
- **§S17 errmap 单一事实源**: every sentinel that can reach a handler must be in errmap.go. Package defines 2 local validation sentinels (`ErrEmptyQuery` in search.go, `ErrEmptyName` in activate.go) — both consumed by Tool framework only, never reach errmap (no registration needed). All 5 consumed/relevant `skilldomain` sentinels (ErrSkillNotFound, ErrInvalidFrontmatter, ErrBodyTooLarge, ErrNameConflict, ErrInvalidName) verified registered at errmap.go:153-157.

## Files audited

| File | LOC | Sites | OK | POST-FIX | VIOLATION | EDGE |
|---|---|---|---|---|---|---|
| skill.go | 38 | 1 | 1 | 0 | 0 | 0 |
| search.go | 175 | 9 | 7 | 0 | 0 | 2 |
| activate.go | 154 | 10 | 8 | 0 | 0 | 2 |
| **TOTAL** | **367** | **20** | **16** | **0** | **0** | **4** |

## Severity breakdown (only VIOLATION + EDGE)

| Severity | Count | Sites | Status |
|---|---|---|---|
| HIGH | 0 | — | — |
| MED | 0 | — | — |
| LOW (impl-detail leak in friendly text) | 3 | search.go:#6 ("chat model configured" hint); search.go:#7 (`~/.forgify/skills/<name>/SKILL.md` path); activate.go:#7 (`~/.forgify/skills/` path) | FOUND |
| LOW (teaching-style content in friendly text) | 1 | activate.go:#8 (`${CLAUDE_SKILL_DIR}` template hint) | FOUND |

(Counts: 3 + 1 = 4 EDGE LOW; matches table above.)

## Cross-cutting

### Sentinel chain integrity (§S17)

Per activate.go::Execute switch — both consumed `skilldomain` sentinels (ErrSkillNotFound, ErrBodyTooLarge) match via `errors.Is` and are registered errmap.go:153, 155. Default branch propagates via `fmt.Errorf("activate_skill: %w", err)` — chain integrity preserved.

Local validation sentinels (`ErrEmptyQuery` search.go:33, `ErrEmptyName` activate.go:26) are framework-consumed via ValidateInput, never reach errmap (correctly).

**No missing registrations**.

### Tool result anti-pattern audit (parent's specific concern)

Parent flagged "teaching-style result / impl-detail leak / self-promoting error" as anti-patterns. Findings:

| Pattern | Sites | Verdict |
|---|---|---|
| Teaching-style (LLM-to-user copy in tool result) | search.go:#7 ("Have the user install one..." instruction); activate.go:#8 (`${CLAUDE_SKILL_DIR}` template hint) | EDGE LOW — accepted per §S18 friendly tool_result; pattern is the framework's contract. |
| Impl-detail leak (file-system path mentions in user-facing text) | search.go:#6 ("chat model configured" hint); search.go:#7 (`~/.forgify/skills/<name>/SKILL.md` path); activate.go:#7 (`~/.forgify/skills/` path) | EDGE LOW — file-path leaks via LLM relay; could trim, but informational. Same precedent as app-tool-mcp's `~/.forgify/mcp.json` mentions. |
| Self-promoting (suggesting another product feature) | activate.go:#7 ("Call search_skills first") | OK by design — actionable cross-tool hint, parity with app-tool-mcp's marketplace BYOK suggestion. |
| Default-branch swallow check | activate.go:#9 (Execute switch default) | OK — propagates real error via `%w` (not silent), inline comment documents intent. Textbook §S3. |

**Verdict on the anti-pattern question**: package largely compliant with the §S18 friendly-tool_result contract. Mild impl-detail leaks (file-path mentions) are LOW EDGE that could be trimmed for hygiene but aren't functional bugs. Audit-recommend: **WAIVE all 4 sites** (informational, user can search for path themselves) OR do a single sweep commit to drop file-path / template-variable references from result text — minimal value.

### Detached ctx coverage (§S9) — N/A at this layer

All terminal writes are below this tool layer:
- `skillapp.Service.Search` → read-only (no writes)
- `skillapp.Service.Activate` → in-memory `AgentState.ActiveSkill` mutation + body re-read + optional subagent spawn (covered by app-skill audit)

Tool layer correctly delegates without re-implementing §S9 logic. ctx is passed straight through.

### §S18 9-method compliance

Both LLM-facing tools have full 9-method explicit declaration (no BaseTool embedding):

| Tool | Identity (Name/Desc/Params) | Static (RO/NRF/RW) | Args-dep (Validate/CheckPerm) | Execute | Compile-check |
|---|---|---|---|---|---|
| `SearchSkills` | search.go:79-81 | true/false/false (read-only catalog query) | search.go:91-106 | search.go:128 | search.go:175 ✓ |
| `ActivateSkill` | activate.go:75-77 | false/false/false (state mutation per block-comment) | activate.go:96-111 | activate.go:125 | activate.go:154 ✓ |

Both tools have compile-time `var _ toolapp.Tool = (*X)(nil)` assertion — best practice (catches §S18 9-method drift at build time).

ActivateSkill::IsReadOnly = false is explicitly justified by inline block comment (lines 81-89) — exactly the §S11 "non-obvious WHY" pattern. Justification: AgentState.ActiveSkill mutation has observable effect on subsequent tool dispatches (allowed-tools pre-approval), so it should serialize in same execution_group as other state-mutating tools, not run in parallel.

### Style consistency cross-check vs sibling packages

This package is **stylistically consistent** with app-tool-mcp / app-tool-shell / app-tool-search / app-tool-forge:
- Helper-style prefix `<tool_name>:` instead of strict-literal `<pkg>.<Type>.<Method>:`
- §S18 friendly tool_result pattern (errors.Is + sentinel-classified text)
- Validation sentinels are local, framework-consumed
- Default-branch error propagation uses `%w` wrap (no silent swallow)

The 31 §S16 prefix LOW were WAIVED in app-tool-forge (commit 64d9535) per established precedent. Same WAIVE applies here.

## Spot-check (random clean sites — verify mechanism not rubber-stamping)

5 sites picked from `OK` set across 3 files:

1. **skill.go:#1** (skill.go:33-38 SkillTools factory): verified — pure struct-literal init; no error paths, no business IDs, no terminal writes. Nothing of §S3-S17 to violate.
2. **search.go:#3** (search.go:91-102 ValidateInput): verified — `fmt.Errorf("search_skills.ValidateInput: %w", err)` + sentinel return for trim-empty case. Helper-style pkg.method prefix + %w. errors.Is chain preserved.
3. **search.go:#5** (search.go:128-135 Execute Unmarshal): verified — `fmt.Errorf("search_skills.Execute: parse args: %w", err)`. pkg.method + sub-tag `parse args:` + `%w`. Returns `("", err)` — no silent swallow.
4. **activate.go:#3** (activate.go:96-107 ValidateInput): verified — same pattern as search.go:#3. Compliant.
5. **activate.go:#9** (activate.go:144-149 default branch): verified — `default: return "", fmt.Errorf("activate_skill: %w", err)` with documented intent comment. Critical: this is the textbook §S3 + §S16 pattern — does NOT silently fall through, does propagate sentinel chain, does have intent comment. Reference-quality.

All 5 spot-checks confirmed correct classification — mechanism functioning, not rubber-stamping.

## Recommended fix priorities

1. **Impl-detail leaks in friendly text** (LOW × 3 — search.go:#6 + search.go:#7 + activate.go:#7): WAIVE per app-tool-mcp impl-detail-leak precedent OR do single sweep dropping path mentions. User-impact zero either way; hygiene benefit marginal.
2. **Teaching-style content** (LOW × 1 — activate.go:#8 `${CLAUDE_SKILL_DIR}` hint): WAIVE per §S18 friendly tool_result precedent OR trim verbose explanation.
3. **§S16 helper-style prefix migration**: WAIVE per established precedent across app-tool-* packages.

**Net assessment**: package is **§S3/S9/S15/S16/S17 clean** at the tool layer. 4 EDGE LOW are stylistic / template / informational; no HIGH or MED found. The §S18 friendly tool_result pattern is implemented textbook-correctly, particularly activate.go::Execute switch (lines 134-149) which can be referenced by other tool audits as a model: errors.Is on each named sentinel, %w wrap on default branch, intent-comment documenting fallthrough policy.

## Out-of-scope notes (parent should verify)

1. **app-skill audit dependency**: this audit assumes `skillapp.Service.Activate`'s in-memory AgentState mutation does not require detached-ctx (Service layer concern, app-skill audit). If Service-side §S9 audit identifies AgentState mutation as a "terminal write" (e.g. activation must persist past ctx cancel), that's not fixable at this tool layer — it'd require Service-internal change.
2. **Phase C (Tool result anti-pattern) cross-fork concern**: per parent's directive this audit fork is Phase A focused; the anti-pattern findings (3 impl-detail leak sites + 1 teaching-style site) are noted as LOW EDGE here but properly belong in Phase C's anti-pattern sweep when that runs.
3. **Subagent spawn failure path** (activate.go:#9 default branch): unmapped errors from `Service.Activate` get wrapped as `activate_skill: %w` and returned to framework — user sees a failed tool_result with the actual underlying error string. If Service ever introduces a sentinel for "subagent recursion" or "subagent type missing", those should be added to the friendly switch (activate.go:139-149) AND verified for errmap.go registration. Currently activate_skill consumes 2 of 5 skilldomain sentinels via friendly map; the other 3 (InvalidFrontmatter / NameConflict / InvalidName) flow only through HTTP CRUD handlers, not this tool.
