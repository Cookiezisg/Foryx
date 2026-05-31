# Audit trace — `internal/app/tool/skill/search.go`

**LOC**: 175 (incl. doc + import + schema literal)
**Role**: `search_skills` system tool — LLM discovery / L1 catalog query.

## 9-column trace

| site# | file:line | snippet | category | classification | reasoning | severity | user_impact | suggested_fix | status |
|---|---|---|---|---|---|---|---|---|---|
| 1 | search.go:33 | ``var ErrEmptyQuery = errors.New("query is required and must be non-empty")`` | A.5 | OK | Local validation sentinel; consumed by Tool framework (returned from ValidateInput → framework wraps as failed tool_result). Never reaches handler/errmap. Same precedent as mcp.go::ErrEmptyQuery / ErrEmptyServer / ErrEmptyTool — registered as "framework-consumed, not registered (correctly)" in app-tool-mcp summary. | N-A | — | — | — |
| 2 | search.go:79-87 | 9-method §S18 surface (Identity 3 + static-metadata 3 + Args-dep hooks 2 + Execute 1 = 9). `IsReadOnly() bool { return true }` / `NeedsReadFirst() bool { return false }` / `RequiresWorkspace() bool { return false }` | — | OK | Full §S18 9-method shape, all explicit (no BaseTool embedding). search_skills is read-only catalog query — true/false/false consistent with §S18 §8 table for read-only catalog tools (parity with search_mcp). | N-A | — | — | — |
| 3 | search.go:91-102 | ``func (t *SearchSkills) ValidateInput(args json.RawMessage) error { var a struct{ Query string }; if err := json.Unmarshal(args, &a); err != nil { return fmt.Errorf("search_skills.ValidateInput: %w", err) } if strings.TrimSpace(a.Query) == "" { return ErrEmptyQuery } return nil }`` | A.4 | OK | `<tool_name>.<Method>:` prefix + `%w` wrap. Helper-style prefix consistent with sibling app-tool-mcp / app-tool-shell / app-tool-search precedent (WAIVED in audit-precedent — see app-tool-mcp summary §"Style consistency"). Sentinel returned naked (correct: no extra wrap; framework rendering uses errors.Is). | N-A | — | — | — |
| 4 | search.go:104-106 | ``func (t *SearchSkills) CheckPermissions(_ json.RawMessage, _ toolapp.PermissionMode) toolapp.PermissionResult { return toolapp.PermissionAllow }`` | — | OK | Read-only discovery → unconditional Allow (matches search_mcp / get_forge / search_forge precedent). No error path. | N-A | — | — | — |
| 5 | search.go:128-135 | ``if err := json.Unmarshal([]byte(argsJSON), &args); err != nil { return "", fmt.Errorf("search_skills.Execute: parse args: %w", err) }`` | A.4 | OK | `<tool_name>.<Method>:` prefix + sub-tag `parse args:` + `%w`. Helper-style prefix (audit-precedent WAIVE, same as mcp/shell/forge). Returns `("", err)` — caller sees real error (no silent zero-value swallow). | N-A | — | — | — |
| 6 | search.go:141-151 | ``skills, err := t.svc.Search(ctx, args.Query, topK); if err != nil { return fmt.Sprintf("Search failed: %v. The skills catalog needs a chat model configured...", err), nil }`` | A.1 | EDGE | §S18 friendly-tool_result pattern: error rendered as user-readable text + returns `nil` error. Comment at 142-149 documents why: LLM-ranking failure is the typical case (no chat model configured) and search still works in alpha-order fallback. **Mild concern**: this is the kind of thing §S3 typical-violation #6 ("silent fallback: upstream failed, plan B without telling caller") flags — but here the caller IS told (LLM sees the failure text). Borderline OK per §S18 friendly-tool_result precedent (parity with mcp call.go::mapCallToolErrorToFriendly). The friendly text uses `%v` (not `%w`), which is correct because output is friendly-content not propagated error chain. **Sub-concern**: the friendly text leaks an impl-detail ("chat model configured") which is teaching-style — same LOW-EDGE pattern flagged in app-tool-mcp summary anti-pattern audit (search.go:#5). | LOW | LLM may relay the "configure chat model" hint to user verbatim — minor UX leak but actionable. | WAIVE per §S18 friendly-tool_result precedent (app-tool-mcp summary §"Style consistency"). Optional: trim "The skills catalog needs a chat model configured..." sentence to just "Search failed: %v." since LLM can infer from context. | FOUND |
| 7 | search.go:153-155 | ``if len(skills) == 0 { return "No skills installed. Have the user install one (drag a SKILL.md folder into the skills panel, or write one to ~/.forgify/skills/<name>/SKILL.md).", nil }`` | — | EDGE | Empty-result message is teaching-style + impl-detail leak (`~/.forgify/skills/<name>/SKILL.md` path mention). Same precedent as app-tool-mcp `~/.forgify/mcp.json` LOW-EDGE entries (search.go:#5 / call.go:#5 / install_server.go:#1 / uninstall_server.go:#3). Not a §S3/§S16 violation; flagged for parity with app-tool-mcp anti-pattern findings. | LOW | User sees skill-folder path in tool result — informational, harmless. | WAIVE per app-tool-mcp impl-detail-leak precedent. Optional: trim path mention to "No skills installed. Use the skills panel to install one." | FOUND |
| 8 | search.go:166-169 | ``body, err := json.MarshalIndent(out, "", "  "); if err != nil { return "", fmt.Errorf("search_skills.Execute: marshal result: %w", err) }`` | A.4 | OK | `<tool_name>.<Method>:` prefix + sub-tag `marshal result:` + `%w`. Returns `("", err)` — propagates real error (no silent swallow). Note: `out` is a `[]searchResult` of plain string/bool/[]string fields — `MarshalIndent` is effectively unfailable in practice, but the code still handles the error properly (textbook §S3 — does NOT use `_, _ := json.Marshal(...)` pattern that app-tool-mcp had to flag as missing-inline-comment LOW). | N-A | — | — | — |
| 9 | search.go:175 | ``var _ toolapp.Tool = (*SearchSkills)(nil)`` | — | OK | Compile-time interface assertion — best practice (catches §S18 9-method drift at build time). | N-A | — | — | — |

## Sub-checks

```
A.1 §S3 错误吞没:
  - violations: not present
  - EDGE LOW (friendly-fallback when LLM ranking fails): site#6 — accepted per §S18 friendly-tool_result precedent (caller is informed via tool_result text + LLM sees error). Not silent.
A.2 §S9 detached ctx 终态写:
  - terminal-state writes identified: none
  - 各自 ctx 来源: N/A
  - violations: N/A: search_skills is read-only discovery; no DB/file writes. svc.Search is a pure read against in-memory catalog (skillapp.Service.Search).
A.3 §S15 ID 生成:
  - ID generation calls: none
  - violations: N/A: package generates no business IDs (skill names come from author / are user-supplied dir names, not §S15 prefix_<16hex> format by spec design — same precedent as MCP server slugs).
A.4 §S16 错误 wrap 格式:
  - violations: not present
  - all 3 fmt.Errorf calls (sites #3 ValidateInput, #5 Execute parse, #8 Execute marshal) use `<tool_name>.<Method>:` prefix + `%w` — helper-style prefix matches sibling app-tool-mcp / app-tool-shell / app-tool-search precedent (WAIVED group-wide). Strict-literal would be `searchskills.SearchSkills.Execute:` but precedent across tool packages is `<tool_name>.<Method>:` for LLM-facing tools.
A.5 §S17 sentinel 登记 errmap:
  - sentinels defined: ErrEmptyQuery (line 33)
  - 已登记 errmap: not registered
  - missing: ErrEmptyQuery NOT registered — but this is correct (framework-consumed only, not handler-consumed). Same precedent as mcp.go::ErrEmptyQuery / app-tool-mcp summary §"sentinels in errmap" — local validation sentinels for ValidateInput don't reach handler.
  - skilldomain sentinels consumed: none directly in this file (Service.Search returns only LLM-resolution errors; no skilldomain.Err* sentinel surfaces here)
  - cross-check: skilldomain sentinels (ErrSkillNotFound, ErrInvalidFrontmatter, ErrBodyTooLarge, ErrNameConflict, ErrInvalidName) all 5 registered errmap.go:153-157 — verified.
```

## File verdict

**Clean** — 9-method §S18 shape correct; all 3 fmt.Errorf wraps compliant; sentinel hygiene correct (local + framework-consumed). Two LOW-EDGE entries (sites #6 / #7) are §S18 friendly-tool_result content style (impl-detail leak in user-facing text) — same precedent as app-tool-mcp WAIVE.
