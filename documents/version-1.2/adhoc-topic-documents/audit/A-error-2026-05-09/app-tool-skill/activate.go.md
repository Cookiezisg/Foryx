# Audit trace — `internal/app/tool/skill/activate.go`

**LOC**: 154 (incl. doc + import + schema literal)
**Role**: `activate_skill` system tool — load skill body, set allowed-tools pre-approval, dispatch subagent if context=fork.

## 9-column trace

| site# | file:line | snippet | category | classification | reasoning | severity | user_impact | suggested_fix | status |
|---|---|---|---|---|---|---|---|---|---|
| 1 | activate.go:26 | ``var ErrEmptyName = errors.New("name is required and must be non-empty")`` | A.5 | OK | Local validation sentinel; consumed by Tool framework via ValidateInput → never reaches handler/errmap. Same precedent as ErrEmptyQuery (search.go:33) + app-tool-mcp::ErrEmptyServer / ErrEmptyTool. Framework-consumed local sentinel = correctly NOT registered in errmap. | N-A | — | — | — |
| 2 | activate.go:75-92 | 9-method §S18 surface (Identity 3 + static-metadata 3 + Args-dep hooks 2 + Execute 1 = 9). `IsReadOnly() bool { return false }` (state mutation) / `NeedsReadFirst() bool { return false }` / `RequiresWorkspace() bool { return false }`. Lines 81-89 explain why IsReadOnly=false: AgentState write of allowed-tools pre-approval. | — | OK | Full §S18 9-method shape, all explicit. Block comment at 81-89 is exactly the §S11 "non-obvious WHY" pattern: documents why IsReadOnly=false (state mutation observable in subsequent tool dispatches) — textbook. | N-A | — | — | — |
| 3 | activate.go:96-107 | ``func (t *ActivateSkill) ValidateInput(args json.RawMessage) error { var a struct{ Name string }; if err := json.Unmarshal(args, &a); err != nil { return fmt.Errorf("activate_skill.ValidateInput: %w", err) } if strings.TrimSpace(a.Name) == "" { return ErrEmptyName } return nil }`` | A.4 | OK | `<tool_name>.<Method>:` prefix + `%w` wrap. Helper-style prefix consistent with sibling app-tool-mcp / app-tool-shell / app-tool-search precedent (WAIVED in audit-precedent — see app-tool-mcp summary §"Style consistency"). | N-A | — | — | — |
| 4 | activate.go:109-111 | ``func (t *ActivateSkill) CheckPermissions(_ json.RawMessage, _ toolapp.PermissionMode) toolapp.PermissionResult { return toolapp.PermissionAllow }`` | — | OK | Unconditional Allow. Reasoning: activate_skill itself has no destructive effect — the **skill's body** then drives subsequent tool calls which each go through their own CheckPermissions. The skill's allowed-tools is pre-approval, not bypass — non-allowed tools still go through normal permission flow. Permission decisions are layered correctly. | N-A | — | — | — |
| 5 | activate.go:125-132 | ``if err := json.Unmarshal([]byte(argsJSON), &args); err != nil { return "", fmt.Errorf("activate_skill.Execute: parse args: %w", err) }`` | A.4 | OK | `<tool_name>.<Method>:` prefix + sub-tag `parse args:` + `%w`. Returns `("", err)` — caller sees real error. | N-A | — | — | — |
| 6 | activate.go:134-149 | ``out, err := t.svc.Activate(ctx, args.Name, args.Arguments); if err == nil { return out, nil }; switch { case errors.Is(err, skilldomain.ErrSkillNotFound): return fmt.Sprintf("Skill %q not found...", args.Name), nil; case errors.Is(err, skilldomain.ErrBodyTooLarge): return fmt.Sprintf("Skill %q body exceeds the %d-byte limit...", args.Name, skilldomain.MaxBodyBytes), nil; default: return "", fmt.Errorf("activate_skill: %w", err) }`` | A.1/A.4 | OK | §S18 friendly-tool_result switch — exactly the pattern app-tool-mcp summary spot-check #2 cited as "model" (call.go::mapCallToolErrorToFriendly). Each branch uses `errors.Is` against named skilldomain sentinel (not string match); default branch propagates real error via `%w` (no silent swallow); none of the `nil` returns are silent fallbacks (LLM sees the friendly text). **§S16 default branch** uses `<tool_name>:` prefix + `%w` — helper-style precedent. | N-A | — | — | — |
| 7 | activate.go:141 (within site #6) | ``return fmt.Sprintf("Skill %q not found. Call search_skills first to see what's available, or check ~/.forgify/skills/ for installed skills.", args.Name), nil`` | A.1 | EDGE | §S18 friendly-tool_result with impl-detail leak (`~/.forgify/skills/` path mention). Same pattern as app-tool-mcp impl-detail-leak LOWs (search.go:#5 / call.go:#5 / install_server.go:#1 / uninstall_server.go:#3). Not a §S3 violation — caller IS told (LLM sees the failure). Stylistic-hygiene only. | LOW | LLM may relay the path mention to user verbatim — informational, harmless. | WAIVE per app-tool-mcp impl-detail-leak precedent. Optional: trim to "Skill %q not found. Call search_skills first to see what's available." | FOUND |
| 8 | activate.go:142-143 (within site #6) | ``return fmt.Sprintf("Skill %q body exceeds the %d-byte limit. The user should shrink the SKILL.md (move long instructions into resource files referenced by ${CLAUDE_SKILL_DIR}/...).", args.Name, skilldomain.MaxBodyBytes), nil`` | A.1 | EDGE | §S18 friendly-tool_result with teaching-style content (`${CLAUDE_SKILL_DIR}` template variable hint). Same pattern as app-tool-mcp teaching-style EDGE entries. | LOW | LLM may relay teaching-style hint to user — informational. | WAIVE per app-tool-mcp teaching-style precedent. Optional: trim to "Skill %q body exceeds the %d-byte limit. The user should shrink SKILL.md." | FOUND |
| 9 | activate.go:144-149 (default branch within site #6) | ``default: // Subagent spawn failure / unexpected I/O. Pass through with the actual error so the LLM has something concrete to report. return "", fmt.Errorf("activate_skill: %w", err)`` | A.1/A.4 | OK | Default branch correctly **propagates** unmapped error via `%w` (not silent fallback). Inline comment documents intent ("Pass through with the actual error so the LLM has something concrete to report") — textbook §S3 + §S11 non-obvious-WHY. Returns non-nil error so framework renders as failed tool_result (caller sees error). | N-A | — | — | — |
| 10 | activate.go:154 | ``var _ toolapp.Tool = (*ActivateSkill)(nil)`` | — | OK | Compile-time interface assertion. | N-A | — | — | — |

## Sub-checks

```
A.1 §S3 错误吞没:
  - violations: not present
  - EDGE LOW (impl-detail leak in friendly text): site#7 (~/.forgify/skills/ path), site#8 (${CLAUDE_SKILL_DIR} template hint) — WAIVE per app-tool-mcp precedent.
  - default-branch swallow check: site#9 — explicitly NOT silent (returns wrapped error to framework, LLM sees it via failed tool_result rendering). Compliant.
A.2 §S9 detached ctx 终态写:
  - terminal-state writes identified: none at this layer
  - 各自 ctx 来源: N/A
  - violations: N/A: tool layer delegates terminal writes to skillapp.Service.Activate (covered by app-skill audit). The Service-level §S9 concern (does AgentState/skill-active state mutation need detached ctx?) is the Service's responsibility, not this tool wrapper's. Tool just passes ctx through.
A.3 §S15 ID 生成:
  - ID generation calls: none
  - violations: N/A: package generates no business IDs (skill name comes from caller args, not server-generated; skills are dir-named under ~/.forgify/skills/<name>/ by spec).
A.4 §S16 错误 wrap 格式:
  - violations: not present
  - all 3 fmt.Errorf calls (sites #3 ValidateInput, #5 Execute parse, #9 Execute default) use `<tool_name>(.<Method>)?:` prefix + `%w`. Helper-style prefix matches sibling tool packages — WAIVED per app-tool-mcp / app-tool-forge precedent.
  - skilldomain sentinel returns (sites #6 ErrSkillNotFound branch, ErrBodyTooLarge branch): naked sentinel preserved via errors.Is — chain integrity intact. No %v / %s breakage.
A.5 §S17 sentinel 登记 errmap:
  - sentinels defined: ErrEmptyName (line 26)
  - 已登记 errmap: not registered
  - missing: ErrEmptyName NOT registered — but correctly so (framework-consumed only via ValidateInput, never reaches handler). Same precedent as ErrEmptyQuery / mcp.go ErrEmptyServer/ErrEmptyTool.
  - skilldomain sentinels consumed: ErrSkillNotFound + ErrBodyTooLarge (sites #6) — both registered errmap.go:153, 155. The other 3 skilldomain sentinels (ErrInvalidFrontmatter / ErrNameConflict / ErrInvalidName) don't surface through this tool path (they're handler/Service-creation paths, not activate-flow); their registration is verified at errmap.go:154/156/157.
  - cross-check: all 5 skilldomain sentinels registered errmap.go:153-157 ✓
```

## File verdict

**Clean** — 9-method §S18 shape correct; sentinel chain integrity preserved (errors.Is on skilldomain sentinels, default branch uses %w wrap); friendly-tool_result switch is textbook §S18 (matches app-tool-mcp model). Two LOW-EDGE entries (sites #7 / #8) are §S18 friendly-text content style — same WAIVE precedent as app-tool-mcp impl-detail-leaks.
