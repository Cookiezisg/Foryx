# audit: backend/internal/app/skill/activate.go

LOC: 188
Read: full file (lines 1-188)

## Trace

| site# | file:line | snippet | category | classification | reasoning | severity | user_impact | suggested_fix | status |
|---|---|---|---|---|---|---|---|---|---|
| 1 | activate.go:56-61 | `s.mu.RLock(); skill, ok := s.skills[name]; s.mu.RUnlock(); if !ok { return "", fmt.Errorf("skillapp.Activate: %w: %q", skilldomain.ErrSkillNotFound, name) }` | A.4 | OK | §S16 canonical: pkg.Method + %w wraps sentinel + name context. Sentinel registered errmap.go:149 → 404. | N-A | — | — | — |
| 2 | activate.go:63-66 | `body, err := readBodyWithRetry(skill.BodyPath); if err != nil { return "", fmt.Errorf("skillapp.Activate %s: %w", name, err) }` | A.4 | OK | §S16 canonical wrap with name context. Caller wraps the bare-os err returned by readBodyWithRetry. | N-A | — | — | — |
| 3 | activate.go:67-72 | `if len(body) > skilldomain.MaxBodyBytes { return "", fmt.Errorf("skillapp.Activate %s: %w", name, skilldomain.ErrBodyTooLarge) }` | A.4 | OK | §S16: pkg.Method + name + %w wraps sentinel. Registered errmap.go:151 → 422. | N-A | — | — | — |
| 4 | activate.go:91-93 | `if state, hasState := reqctxpkg.GetAgentState(ctx); hasState { state.SetActiveSkill(skill) }` | A.1 | OK | hasState false branch is documented at file header (§9.4): non-fork retain pre-approval across calls. Missing AgentState (e.g. test harness) silently skips — accepted carve-out per design. SetActiveSkill is in-memory state, no error to handle. | N-A | — | — | — |
| 5 | activate.go:97-102 | `if skill.Frontmatter.Context == "fork" { if depth := reqctxpkg.GetSubagentDepth(ctx); depth >= 1 { s.log.Info("skill activated within subagent; ignoring fork directive", ...); return substituted, nil } ... }` | A.1 | OK | §9.5 depth-guard: nested-fork is invariant violation; logged + downgraded to non-fork (return substituted body). Documented design rationale at file header lines 41-45. | N-A | — | — | — |
| 6 | activate.go:103-105 | `if s.subagent == nil { return "", fmt.Errorf("skillapp.Activate %s: fork requested but SubagentService is nil", name) }` | A.4 | EDGE | §S16: pkg.Method + name prefix ✓ but **NO sentinel + NO %w**. Defensive validation — main.go always wires subagent (per New doc comment line 97-99). Same wiring-bug pattern as mcp.AddServer cfg.Name (resolved as panic). | LOW | hits "unmapped domain error" alarm if a test bypasses subagent wiring AND triggers fork-mode skill — narrow combination | introduce `skilldomain.ErrSubagentUnavailable` + register errmap (422 — fork not supported in this build), OR panic ("config-time invariant per New doc"). Consistent with mcp.AddServer resolution (panic) | FOUND |
| 7 | activate.go:107-110 | `result, err := s.subagent.Spawn(ctx, agentType, substituted, subagentapp.SpawnOpts{}); if err != nil { return "", fmt.Errorf("skillapp.Activate %s: subagent spawn: %w", name, err) }` | A.4 | OK | §S16 canonical wrap. subagent.Spawn returns wrapped sentinel chain (see subagent audit). | N-A | — | — | — |
| 8 | activate.go:124-127 | `body, err := os.ReadFile(path); if err == nil { return body, nil }; if !errors.Is(err, fs.ErrNotExist) { return nil, err }` | A.1/A.4 | EDGE | §S16: bare `return nil, err` for non-ENOENT path (e.g. permission denied) — caller (Activate site #2) wraps with `skillapp.Activate %s: %w`, so call-site context preserved at outer layer. Style consistency would prefer wrap here too with `skillapp.readBodyWithRetry:` prefix. | LOW | identical UX (caller wraps); harder to grep `readBodyWithRetry` call site | wrap: `return nil, fmt.Errorf("skillapp.readBodyWithRetry: %w", err)` for grep | FOUND |
| 9 | activate.go:131-132 | `time.Sleep(bodyReadRetryDelay); return os.ReadFile(path)` | A.4 | EDGE | bare-return of os.ReadFile result — same style as #8. Caller wraps with `skillapp.Activate %s: %w`. | LOW | same as #8 | wrap: `body, err := os.ReadFile(path); if err != nil { return nil, fmt.Errorf("skillapp.readBodyWithRetry: %w", err) }; return body, nil` | FOUND |

## Sub-check

A.1 §S3 错误吞没:
  - violations: not present (sites #4 #5 are documented carve-outs)

A.2 §S9 detached ctx 终态写:
  - terminal-state writes identified: site #4 (SetActiveSkill on AgentState) — but in-memory, NOT a DB write
  - 各自 ctx 来源: site #4 doesn't take ctx (in-memory); site #7 Spawn takes request ctx by design (subagent run is bounded by user's request lifetime)
  - violations: N/A — file does no DB-terminal writes; sets in-memory ActiveSkill flag (which is ephemeral per-conversation state) and dispatches to subagent (whose own §S9 obligations were audited separately)

A.3 §S15 ID 生成:
  - ID generation calls: none
  - violations: N/A — skill names are user-supplied keys, not idgen-generated business IDs

A.4 §S16 错误 wrap 格式:
  - violations: not present in canonical-spec sense
  - LOW EDGE: site #6 (no-sentinel fork-without-subagent), #8 #9 (bare-return inside readBodyWithRetry helper, caller wraps)

A.5 §S17 sentinel 登记 errmap:
  - sentinels defined in this file: none
  - 已登记 errmap (consumed): `skilldomain.ErrSkillNotFound` (line 60), `skilldomain.ErrBodyTooLarge` (line 71) — both registered ✓
  - missing: N/A — file defines no sentinels (site #6 is a candidate for `ErrSubagentUnavailable` if/when introduced)
