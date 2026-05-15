# C-tool-result audit-2: LLM-facing tool reality check (Phase C fork #2)

Date: 2026-05-11
Scope: 5 tool sub-packages (mcp, skill, subagent, todo, ask) + framework main file (`tool.go` + `loop/tools.go` runOneTool/executeAfterPermission paths). forge sub-package skipped per request (user rewriting).

Anti-pattern legend:
- AP1: teaching/imperative opening ("Load a skill's full instructions and start following them.")
- AP2: over-markdown / decoration in result strings or descriptions
- AP3: meta-talk ("Use when:" / "After choosing..." / "consider re-spawning..." / process narration to LLM)
- AP4: backend leakage (file paths, UI references, sentinel chains, internal terms like "wiring bug" / "SSE", DB column names)
- AP5: verbose success ("Server X installed and connected (status=connected).")
- AP6: format inconsistency across siblings (one returns JSON, another plain string, another wrapped envelope)
- AP7: Description vs reality drift (comment claims behavior code doesn't have)

Severity:
- HIGH = obviously teaches the LLM bad habits or leaks production-internal text on every call
- MED = redundant / decorative / noisy, costs tokens, slightly polluted output
- LOW = wording preference / minor inconsistency

---

## 0. Framework path (loop/tools.go, tool.go) — error → tool_result

This is the choke-point for every tool failure. Per `executeTool` / `executeAfterPermission`:

| File:line | Code | Issue |
|---|---|---|
| `internal/app/loop/tools.go:191` | `return fmt.Sprintf("input validation failed: %s", err.Error()), err.Error(), false` | **HIGH AP4** — raw `err.Error()` (sentinel chain) goes to LLM. ValidateInput returning `fmt.Errorf("call_mcp.ValidateInput: %w", err)` leaks the package prefix verbatim. LLM sees `input validation failed: call_mcp.ValidateInput: json: cannot unmarshal ...` |
| `internal/app/loop/tools.go:220` | `return "permission denied for this call", "permission denied", false` | LOW AP3 — terse, but no detail of *why*; LLM can't react meaningfully. Acceptable but blank. |
| `internal/app/loop/tools.go:244-246` | `return output, err.Error(), false` / `return err.Error(), err.Error(), false` | **HIGH AP4** — raw `err.Error()` to LLM as both output text and Block.Error. Every tool that returns Go errors (and not friendly tool_result strings) leaks its full `%w`-wrapped chain. No sanitization layer. |
| `internal/app/loop/tools.go:185` | `msg := fmt.Sprintf("tool %q not found", name)` | LOW AP3 — fine; says what's wrong. |

**Net**: tool implementations using friendly-string pattern (call_mcp / ask / skill_search / etc.) are protected. Anything returning Go err **with** `%w` wrapping (subagent.Spawn err pass-through, list_mcp_marketplace internal failure, activate_skill default branch, todo classifyTodoErr fallback) leaks `<pkg>.<Method>: <wrapped chain>` into LLM context.

Per §S16, every error in the project is wrapped `<pkg>.<Method>: %w`. So the sentinel chain reaching LLM might look like:
`subagent.Spawn: subagentapp.runReact: llm.Generate: deepseek api error: 401 unauthorized: invalid_api_key`

That's 5 layers of internal scaffolding visible to the model.

### 0.1 StripStandardFields — clean

`tool.go:336-370` correctly removes summary/destructive/execution_group before passing args to Execute. Tools should not see the three injected fields. No leakage in this direction.

### 0.2 injectStandardFields — clean

`tool.go:214-286` injects fields into Parameters schema correctly. Field descriptions (lines 240, 244, 250) are tool-author voice, not LLM voice.

---

## 1. mcp/search.go — search_mcp_tools

**Description (39-56)**, lines counted from file:

- Line 39 opening: "Search across all connected MCP servers for tools matching a natural-language query." — fine
- **Line 45-50 "Use when:" bullets** — **MED AP3** meta-instructional list teaching LLM when to call.
- **Line 52-56 "Don't use when:" bullets** — **MED AP3** more teaching; "those are faster and don't depend on user-installed MCP servers" is implementation editorialising.

**Parameters schema (58-73)** — fine. `query` and `top_k` descriptions are concrete.

**Execute return paths**:

| Line | Return | AP |
|---|---|---|
| 142 | `Sprintf("Search failed: %v. Please ensure an MCP server is connected and a chat model is configured.", err)` | **HIGH AP4** — `%v err` leaks raw sentinel chain (e.g. `mcp.Search: llm.Generate: deepseek api: ...`). "Please ensure..." also AP3 meta-instructional. |
| 146 | `"No MCP tools found. Ensure at least one MCP server is configured in ~/.forgify/mcp.json and connected."` | **HIGH AP4** — `~/.forgify/mcp.json` is backend filesystem path. The LLM doesn't need to know where the file lives; UI / install_mcp_server is the entry point. |
| 130, 151 | `Errorf("search_mcp.Execute: parse args: %w", err)` | MED AP4 — wrapped error to LLM via framework path 0.3. |

---

## 2. mcp/call.go — call_mcp_tool

**Description (35-46)**:

- Line 35 opening: fine.
- **Line 37-42 "Workflow:" enumeration** — **MED AP3** workflow narration with numbered steps (1./2./3.) telling LLM the calling pattern. The schema can carry "args matches inputSchema from search_mcp" without prose.
- Line 44-46 "If the tool fails ... result string explains what happened so you can adjust args, pick a different tool, or surrender." — **MED AP3** meta-process talk + the casual word "surrender" is odd in a system prompt.

**Parameters schema (48-66)** — fine.

**Execute / mapCallToolErrorToFriendly (153-168)**:

| Line | Code | AP |
|---|---|---|
| 156 | `Sprintf("MCP server %q is not configured. Use search_mcp to see available servers, or ask the user to install/configure %q first.", server, server)` | LOW AP3 — meta-instructional ("Use search_mcp..."). Tool-name reference is ok-ish since search_mcp is also LLM-visible. |
| 158 | `"... is not connected (status check failed). The user may need to fix the server's configuration or click 'Reconnect' in the MCP settings panel."` | **HIGH AP4** — leaks UI implementation detail to LLM. "click 'Reconnect' in the MCP settings panel" is a UI affordance the LLM has no ability to invoke and shouldn't be relaying as advice. Also AP3 meta. |
| 160 | `"MCP tool %q does not exist on server %q. Use search_mcp to discover the correct tool name."` | LOW AP3 |
| 162 | `"... timed out. The tool may be slow (browser automation, big query) — consider re-trying with a more specific query, or asking the user to extend the per-server timeout in mcp.json."` | **HIGH AP4** — leaks `mcp.json` (backend file). Plus AP3 meta-instructional. |
| 164 | `"MCP call %s/%s failed: %v"` (with `mcpdomain.ErrToolCallFailed`) | **HIGH AP4** — `%v err` leaks raw chain. Plus the sentinel `ErrToolCallFailed` already prefixes with `tool call failed:`, so LLM sees stutter: `MCP call X/Y failed: tool call failed: <inner>`. |
| 166 | default branch `"call_mcp %s/%s failed: %v"` | **HIGH AP4** — `%v err` raw chain (subprocess stderr / connection refused / etc.). |

---

## 3. mcp/list_marketplace.go — list_mcp_marketplace

**Description (32-43)**:

- Line 32 opening: fine.
- **Line 34** category enumeration: "browser (playwright, chrome-devtools), web-data (...) ... sandbox (e2b)" — **MED AP3** — this is a marketing flyer of available servers. The actual tool *returns* this catalog as JSON; the description shouldn't repeat it as narrative.
- **Line 36-41 "Each entry includes:" enumeration** — **MED AP3** — describes the JSON shape in prose. Schema's job, not description's. "tier: 0=zero-config, 1=one API key..." is implementation editorial.
- Line 40: "(each env carries a setupURL — pass that link to the user so they can grab the key)" — **MED AP3** meta-instructional.
- Line 41: "first-run gotchas (chromium downloads, Notion sharing rituals, OAuth flow expectations) — surface these to the user when proposing the install" — **MED AP3** meta-instructional. "Notion sharing rituals" is colorful but not informative.
- **Line 43 "After choosing... call install_mcp_server({name})... use the ask tool to confirm... before the second call (with confirmed=true) actually installs"** — **MED AP3** — workflow chaining narration. Tells LLM how to use a different tool.

This whole description is essentially an internal designer's talk-track.

**Execute (69-75)**:

- Line 72: `return "", fmt.Errorf("list_mcp_marketplace: %w", err)` — **MED AP4** — raw `err.Error()` reaches LLM via framework line 246. `t.svc.ListRegistry` failure path (registry parse / IO) leaks.

---

## 4. mcp/install_server.go — install_mcp_server

**Description (54-63)**:

- **Line 54 "Two-phase flow:"** + **lines 56-58 "PHASE 1 (discovery)" / "PHASE 2 (commit)"** — **HIGH AP3** — entire description is workflow narration with phase labels in caps. The schema already has `confirmed` field; description should say what the field does, not narrate a multi-step process. This is Spec-by-storytelling.
- Line 56 "Use \`ask\` to relay the question to the user, then collect any required env / args values. Always relay the entry's notes to the user — they describe first-run gotchas (chromium downloads, OAuth flows, etc)." — **HIGH AP3** chain-of-tools meta-instruction.
- Line 58 "On success returns the new ServerStatus; on failure returns a structured error (already_installed / missing_required_args / install_failed / handshake_failed) with hints for recovery." — MED AP3 schema-by-prose.
- Line 60-63 "Notes:" subsection — MED AP3.

**Parameters schema (65-74)** — fine but field descriptions repeat info from main description (the "Phase 1/2" labels).

**Execute return paths**:

| Line | Code | AP |
|---|---|---|
| 122 | `Sprintf("Server %q not found in marketplace. Use list_mcp_marketplace to discover available servers.", args.Name)` | LOW AP3 |
| 142 | `Sprintf("A server named %q is already configured. Uninstall it first via uninstall_mcp_server.", args.Name)` | LOW AP3 |
| 145 | `Sprintf("Missing required env: %v. Ask the user for these values, then retry with env={...}.", err.Error())` | **HIGH AP4** — `%v err.Error()` leaks `mcpapp.installFromRegistry: required env missing: GITHUB_TOKEN, NOTION_API_KEY` style chain. Plus AP3. |
| 148 | same pattern for required args | **HIGH AP4** |
| 150 | `Sprintf("Install failed: %v", err)` (with ErrInstallFailed sentinel) | **HIGH AP4** — `%v err` leaks raw chain (subprocess stderr / spawn error / mise install logs). Likely the ugliest LLM-facing path in mcp. |
| 152 | default `errorJSON("install_failed", err.Error())` | **HIGH AP4** — same. |
| 218 | success message: `"Server %q installed and connected (status=%s)."` inside envelope | **MED AP5** — the envelope already has `status: "installed"` and `server: {...status: ...}`. Restating in `message` is redundant. |

**phase1Envelope** (161-208) — string-builds a multi-line "Proceed?" question. The question text itself is a tool-result string the LLM will copy into AskUserQuestion — most of this is fine, BUT:
- Line 172-173 "I'd like to install the MCP server..." — first-person LLM ventriloquism ("I'd like to") seeded by the tool. Mild AP3.
- Line 195 "Proceed?" appended unconditionally — fine.

---

## 5. mcp/uninstall_server.go — uninstall_mcp_server

**Description (31)**:
- "Uninstall a previously-installed MCP server. Removes it from mcp.json and disconnects the subprocess." — **MED AP4** leaks `mcp.json` (backend file) in description, in front of the LLM, every call. Plus "disconnects the subprocess" leaks subprocess implementation detail.

**Execute (66-88)**:

| Line | Code | AP |
|---|---|---|
| 77 | `Sprintf("No installed server named %q. Check the MCP servers UI or ~/.forgify/mcp.json for installed names.", args.Name)` | **HIGH AP4** — leaks BOTH "MCP servers UI" (UI surface) AND `~/.forgify/mcp.json` filesystem path. Worst of both worlds. |
| 79 | `Errorf("uninstall_mcp_server: %w", err)` | MED AP4 — raw chain via framework. |
| 84 | `Sprintf("Server %q uninstalled and disconnected.", args.Name)` inside envelope | **MED AP5** — envelope already has `status: "uninstalled"` and `name`. "disconnected" leaks impl. |

---

## 6. mcp cross-cutting (5 tools)

**AP6 format inconsistency** — the 5 tools return wildly different shapes for parallel cases:

| Tool | Success shape | Friendly-error shape |
|---|---|---|
| search_mcp_tools | `MarshalIndent([]ToolDef{...})` (JSON array, indented) | plain English string ("No MCP tools found...") |
| call_mcp_tool | Whatever the underlying server returned (string passthrough) | plain English string ("MCP server X is not configured...") |
| list_mcp_marketplace | `Marshal([]result{...})` (JSON array, NOT indented) | (no friendly path; goes through framework as raw err) |
| install_mcp_server | JSON envelope `{status, name, server, message}` | JSON envelope `{status:"error", error: "<code>", message: "..."}` |
| uninstall_mcp_server | JSON envelope `{status, name, message}` | JSON envelope `{status:"error", error: "<code>", message: "..."}` |

Three different shapes. LLM sees JSON-array, JSON-envelope, plain-string — and per-tool the success vs failure shape changes too. **MED AP6.**

**AP4 cross-cutting backend leak count**:
- "~/.forgify/mcp.json" appears 3x (search line 146, call line 162, uninstall line 77)
- "MCP settings panel" / "MCP servers UI" appears 2x (call line 158, uninstall line 77)
- raw `%v err` chain leak appears 6+ times across the family

---

## 7. skill/search.go — search_skills

**Description (35-51)**:

- Line 35-39 opening: fine.
- **Line 41-45 "Use when:"** — **MED AP3** meta-instructional bullets, mirrors mcp/search.go pattern.
- **Line 47-51 "Don't use when:"** — **MED AP3** more teaching. "you've recently activated a skill in this conversation (it's still active until another activate_skill replaces it)" — exposes session-state implementation detail.

**Parameters schema (53-68)** — fine.

**Execute return paths**:

| Line | Code | AP |
|---|---|---|
| 150 | `Sprintf("Search failed: %v. The skills catalog needs a chat model configured to rank results when there are many candidates.", err)` | **HIGH AP4** — `%v err` raw chain. Plus AP3 advisory ("needs a chat model configured to rank..."). |
| 154 | `"No skills installed. Have the user install one (drag a SKILL.md folder into the skills panel, or write one to ~/.forgify/skills/<name>/SKILL.md)."` | **HIGH AP4** — leaks UI instruction ("drag a SKILL.md folder into the skills panel") AND filesystem path (`~/.forgify/skills/<name>/SKILL.md`). LLM cannot drag UI files; LLM does not need to know where on disk skills live. |
| 168 | `Errorf("search_skills.Execute: marshal result: %w", err)` | low — `json.MarshalIndent` of a slice of strings effectively can't fail at runtime, but if it did, the chain leaks. |

---

## 8. skill/activate.go — activate_skill

**Description (28-48)**:

- **Line 28 opening: "Load a skill's full instructions and start following them."** — **HIGH AP1** imperative directive. "start following them" is a behavioral nudge baked into the description. Description should describe what the tool does, not order the LLM.
- **Line 30-38 "After activation:" 3-step process narration** — **HIGH AP3** explains internal mechanism (allowed-tools pre-approval, body substitution, fork subagent dispatch). The LLM doesn't need to model these mechanics; it just calls the tool and reads the result.
- "your conversation context isn't polluted with intermediate steps" (line 37-38) — **MED AP4** internal-architecture editorial.
- **Line 40-44 "Use when:"** — **MED AP3**.
- **Line 46-48 "Don't use when:"** — **MED AP3**. "body load is wasted budget if you weren't going to follow it" — colorful internal wording.

**Parameters schema (50-64)** — fine.

**Execute (125-150)**:

| Line | Code | AP |
|---|---|---|
| 141 | `Sprintf("Skill %q not found. Call search_skills first to see what's available, or check ~/.forgify/skills/ for installed skills.", args.Name)` | **HIGH AP4** — leaks `~/.forgify/skills/` filesystem path. Also AP3 meta-instructional. |
| 143 | `Sprintf("Skill %q body exceeds the %d-byte limit. The user should shrink the SKILL.md (move long instructions into resource files referenced by ${CLAUDE_SKILL_DIR}/...).", args.Name, skilldomain.MaxBodyBytes)` | **HIGH AP4** — leaks `SKILL.md` file convention + `${CLAUDE_SKILL_DIR}/...` template variable. AP3 meta-instructional. "The user should shrink..." pushes work onto the user via the LLM relay. |
| 148 | `return "", fmt.Errorf("activate_skill: %w", err)` | **HIGH AP4** — raw chain leak via framework path. Subagent spawn failure: `activate_skill: subagent.Spawn: <full chain>`. Comment claims "subagent spawn failure / unexpected I/O. Pass through with the actual error so the LLM has something concrete to report" — but the "concrete" thing is actually wrapped scaffolding text. **AP7 description-vs-reality**: comment says it's helpful; reality is sentinel-chain noise. |

---

## 9. subagent/agent.go — Subagent

**Description (59-69)**:

- Line 59-61 opening: fine, factual.
- **Line 63-67 "Use for:"** — **MED AP3** meta-instructional bullets with type→use mapping ("searching large codebases (subagent_type=\"Explore\")"). The schema can carry the canonical types; description shouldn't pre-bind use-cases.
- **Line 69 "Be specific in `prompt` — the subagent does not see your conversation."** — **MED AP3** writing advice to the LLM.

**Parameters schema (71-88)**:

- Line 76-77 `subagent_type` description: "Which subagent to spawn. Available: Explore, Plan, general-purpose." — fine, this is the schema's job (enumeration).
- **Line 81 prompt description**: "Task description for the subagent. Be specific — the subagent does not see your conversation history." — **LOW AP3** repeats the description's "Be specific" admonition.

**Execute (171-216)**:

| Line | Code | AP |
|---|---|---|
| 173-174 | `return "", fmt.Errorf("SubagentTool.Execute: %w (depth=%d)", subagentdomain.ErrRecursionAttempt, depth)` | **HIGH AP4** — raw error to LLM via framework line 246. LLM sees `SubagentTool.Execute: subagent recursion attempt (depth=1)`. The `(depth=N)` value is debug-internal scaffolding. |
| 174 | comment-vs-code: "recursion → return ErrRecursionAttempt as Go err so the chat layer surfaces 'permission denied' tool_result text" (lines 163-164) | **HIGH AP7** — claim is wrong. Recursion error is returned from Execute (post-permission); framework path 244-246 is `err.Error()`, not "permission denied" string at line 220. Comment describes a code path that doesn't exist. |
| 194 | `return "", err` (Spawn failed, pass-through) | **HIGH AP4** — comment says "Spawn already wraps with %w so errmap can match the sentinel" — true for errmap, but this is the **tool result** path, not HTTP. The full wrap chain (`subagentapp.Spawn: subagentapp.runReact: llm.Generate: ...`) lands in LLM context. Errmap is for HTTP envelope, not tool_result. **AP7 comment shows confusion between HTTP-error and tool-result paths.** |
| 205 | `appendNote(res.Result, "subagent hit max turns; consider re-spawning with more turns or refining the prompt")` | **MED AP3** — "consider re-spawning with more turns or refining the prompt" is meta-instructional advice. |
| 207 | `appendNote(res.Result, "subagent was cancelled")` | LOW — neutral. |
| 210 | `appendNote(res.Result, fmt.Sprintf("subagent failed: %s", res.ErrorMsg))` | MED AP4 — `res.ErrorMsg` source unverified; if it's a wrapped chain from the sub-loop, it leaks. |
| 212 | `Sprintf("Subagent %s failed: %s", res.Type, res.ErrorMsg)` | MED AP4 — same. |
| **AP6 cross**: 4 distinct shapes for one tool's return: bare result (line 214) / result+note (205, 207, 210) / new prefix string (212). LLM has to handle 4 different completion shapes from this single tool. | | **MED AP6** |

**appendNote (224-230)** — `[note: ...]` framework annotation; conceptually OK, but the *content* of notes is meta-talk (see lines 205, 210).

---

## 10. todo/create.go — TodoCreate

**Description (21-30)**:

- Line 21 opening: fine.
- **Line 23 "Usage:"** — **MED AP3** preamble label.
- **Line 24 "Use this when planning multi-step work the user can watch progress on."** — **MED AP3** meta-instructional + leaks UI semantics ("the user can watch progress").
- Line 25-28 schema-restating bullets (`subject` / `description` / `active_form` / `blocked_by`) — **MED AP2** these belong in the schema's field descriptions (and they're already there).
- Line 29 "New todos start in status \"pending\". Use TodoUpdate to move them to \"in_progress\" / \"completed\"." — **MED AP3** lifecycle narration + tool-chain instruction.
- **Line 30 "The returned JSON includes the assigned todo ID — keep it for follow-up TodoUpdate calls."** — **MED AP3** advice to LLM ("keep it for follow-up...").

**Parameters schema (32-54)** — fine, concrete.

**Execute (95-115)**:

| Line | Code | AP |
|---|---|---|
| 103 | `Errorf("TodoCreate.Execute: %w", err)` | MED AP4 — raw chain via framework. |
| 134 | `classifyTodoErr` default: `Sprintf("Todo %s failed: %v", op, err)` | **HIGH AP4** — `%v err` raw chain to LLM. |

`classifyTodoErr` (125-135) for known sentinels is OK (lines 127-132 give plain-English friendly strings). Default branch is the leak.

`marshalIndent` (141-147): `Errorf("marshal: %w", err)` — minor; `json.MarshalIndent` of a fully-typed entity is unlikely to fail at runtime.

---

## 11. todo/list.go — TodoList

**Description (18-24)**:

- Line 18 opening: fine.
- **Line 20 "Usage:"** preamble — **MED AP3**.
- **Line 22 "Todos are ordered by created_at ascending so you see them in the order they were added."** — **HIGH AP4** — leaks DB column name `created_at` to LLM. Should be "ordered by creation time" or simply not stated (the JSON's order is the order).
- **Line 23 "Soft-deleted todos are excluded."** — **HIGH AP4** — leaks "soft-delete" backend implementation concept (`deleted_at IS NULL`, §D1). LLM doesn't need to model this; deleted items just don't exist from its perspective.
- **Line 24 "Use this to decide which todo to work on next or to summarise progress to the user."** — **MED AP3** meta-instructional.

**Execute (55-72)**:

- Line 60-66 wraps todos as `{total: N, todos: [...]}`. **MED AP6** — duplicates `len(todos)` in `total`. Other todo tools return raw entity JSON; this one wraps. Inconsistent.
- Line 58 friendly path; 69 wrap to framework.

---

## 12. todo/get.go — TodoGet

**Description (18-22)**:

- Line 18 opening: fine.
- **Line 20-22 "Usage:" + bullets** — **LOW AP3** less verbose than create/list/update but same pattern.
- Line 22 "Returns the todo as JSON, or a not-found message if the ID does not belong to this conversation." — fine.

**Parameters schema (24-33)**:
- Line 30 "ID of the todo to fetch (e.g. td_abc123…)." — exposes ID prefix convention (§S15). Acceptable since LLM has to handle these IDs.

**Execute (67-79)** — clean; passes through `classifyTodoErr` (which has the line 134 leak as called out).

---

## 13. todo/update.go — TodoUpdate

**Description (22-30)**:

- Line 22 opening: fine.
- **Line 24 "Usage:"** preamble — **MED AP3**.
- Line 26 "Provide only the fields you want to change; omitted fields stay as-is." — **LOW AP3** acceptable schema-semantic clarification.
- **Line 27 "...status... Use \"deleted\" to remove a todo; the deletion broadcasts an SSE update so any UI drops it."** — **HIGH AP4** — leaks "broadcasts an SSE update" backend infra to LLM. SSE is invisible to LLM; mentioning it is pure bleed.
- Line 28-29 schema-restating bullets — **MED AP2**.

**Parameters schema (32-67)** — fine.

**Execute (125-152)**:

- Line 137 `return Sprintf(`{"deleted":true,"id":%q}`, raw.TodoID), nil` — **MED AP6** — manually-constructed JSON literal that doesn't match the entity-JSON shape that other Todo tools return (TodoCreate/TodoUpdate/TodoGet return full marshalled Todo). Two distinct success shapes inside the same tool depending on input.
- Line 128 wrap to framework: `Errorf("TodoUpdate.Execute: %w", err)` — MED AP4.

---

## 14. ask/ask.go — AskUserQuestion

**Description (59-65)**:

- Line 59 opening: fine.
- **Line 61 "Usage:"** preamble — **MED AP3**.
- Line 62 schema-restating `question` — **LOW AP2**.
- **Line 63 long behavioral lecture: "...the UI renders them as click-to-pick buttons alongside a free-text box. The user is NOT restricted to options — they may type anything (confirm with their own wording, decline, change parameters, ask back). You INTERPRET the resulting answer string to decide the next action."** — **HIGH AP3** — exhaustive meta-instructional commentary teaching LLM how to interpret results. "click-to-pick buttons alongside a free-text box" is UI surface (**AP4**). "You INTERPRET" caps emphasis is teaching tone.
- Line 64 "The tool blocks for up to 5 minutes; if the user does not respond, the result is \"User did not respond within the timeout\"." — **MED AP3** — telling LLM exactly what the timeout string is. Useful for matching, but verbose.
- **Line 65 "Use this when you genuinely need user input to proceed (ambiguous request, destructive action confirmation, missing data). Do NOT use it for things you can deduce from context."** — **HIGH AP3** — heavy-handed teaching. Caps "Do NOT". This is system-prompt instruction smuggled into a tool description.

**Parameters schema (67-81)** — fine, but `options` description (line 78) repeats the description's UI talk: "the UI renders them as click-to-pick buttons; user is NOT restricted to these and may also type freely". **LOW AP4** — `the UI renders them as click-to-pick buttons` is UI internal.

**Execute (152-167)**:

| Line | Code | AP |
|---|---|---|
| 155 | `"Cannot ask the user: no tool_call_id in context (chat layer wiring bug)."` | **HIGH AP4** — "(chat layer wiring bug)" is internal-architecture noise. LLM doesn't know what "chat layer" means; it's a developer's panic comment surfacing to model. |
| 160 | `"User did not respond within the timeout. Re-ask later if still needed."` | **MED AP3** — "Re-ask later if still needed" instruction to LLM. |
| 162 | `"Question cancelled by the user (conversation interrupted)."` | LOW AP4 — "(conversation interrupted)" mild internal-state leak; mostly OK. |
| 164 | `Sprintf("Asking the user failed: %v", err)` | **HIGH AP4** — `%v err` raw chain. |

---

## Cross-cutting summary

### AP4 backend leakage census (HIGH only)

| Bleed type | Count | Files |
|---|---|---|
| Filesystem path (`~/.forgify/...`) | 5 | mcp/search:146, mcp/call:162, mcp/uninstall:31,77, skill/search:154, skill/activate:141 |
| UI surface (panel, button, drag, click 'Reconnect') | 4 | mcp/call:158, mcp/uninstall:77, skill/search:154, ask:63,78 |
| `%v err` raw sentinel chain | 12+ | mcp/search:142, mcp/call:164,166, mcp/install:145,148,150,152, skill/search:150, skill/activate:148, subagent:174,194, todo/create:134, ask:164 |
| Internal terms ("SSE update", "wiring bug", "subprocess", "soft-delete", "broadcasts") | 5 | todo/list:23, todo/update:27, mcp/uninstall:31, ask:155 + skill/activate:37 |
| DB column / convention names (`created_at`, `SKILL.md`, `mcp.json`) | 6 | todo/list:22, mcp/uninstall:31, mcp/search:146, mcp/call:162, mcp/uninstall:77, skill/activate:143 |

### AP3 meta-instructional census

Almost every tool description has a "Use when:"/"Don't use when:"/"Usage:" preamble teaching the LLM when to call it. Forgify's pattern is heavier than typical Anthropic SDK descriptions:

- mcp/search.go: "Use when:" + "Don't use when:" sections
- mcp/list_marketplace.go: "Each entry includes:" enumeration + "After choosing..." chaining
- mcp/install_server.go: Phase-1/Phase-2 narrative
- skill/search.go: "Use when:" + "Don't use when:"
- skill/activate.go: "After activation:" 3-step + "Use when:" + "Don't use when:"
- subagent: "Use for:" + "Be specific..."
- ask: "Usage:" + heavy advisory + "Do NOT use it for things..."
- todo/create.go: "Usage:" + lifecycle narration
- todo/list.go: "Usage:"
- todo/get.go: "Usage:"
- todo/update.go: "Usage:"

This is consistent house-style but a heavy AP3 footprint — the LLM reads ~6× the necessary context vs schema-only descriptions.

### AP6 format inconsistency census

- **mcp family**: 5 tools × 3 distinct success shapes (raw string / JSON array / JSON envelope), and the same family mixes plain-English-on-error with JSON-envelope-on-error.
- **subagent**: 1 tool × 4 distinct return shapes (bare result / result+note / "Subagent X failed: Y" prefix string / "Cannot ask..." for unrelated wiring failure).
- **todo family**: TodoUpdate's `{"deleted":true,"id":...}` doesn't match TodoCreate/TodoUpdate/TodoGet's marshalled-entity JSON. TodoList wraps in `{total, todos}` while sibling tools don't wrap.

LLM has to pattern-match across families.

### AP7 description vs reality (drift)

- subagent/agent.go:163-164 comment claims recursion takes "permission denied" framework path — but code at line 173 returns Go err post-permission. (HIGH)
- subagent/agent.go:191-194 comment claims errmap protection of LLM-facing string — errmap is HTTP envelope path, not tool_result path. (HIGH)
- skill/activate.go:120-123 comment claims default-branch error-passthrough is "concrete" for the LLM — reality is `%w`-wrapped sentinel chain noise. (MED)

---

## Tally

- **HIGH**: 24 findings  
  - AP3 (heavy teaching descriptions): 6 (skill/activate opening, ask 2x, mcp/install 2x, mcp/list_marketplace structure)
  - AP4 (filesystem/UI/sentinel-chain leak): 14 — concentrated in mcp family + skill family + subagent recursion path
  - AP7 (description vs reality drift): 2 in subagent + 1 in skill/activate
  - AP1 (imperative opening): 1 (skill/activate "...start following them.")

- **MED**: 31 findings  
  - "Use when:" / "Don't use when:" / "Usage:" preambles across 11 tools (~11 findings)
  - Schema-restating bullets in descriptions (~6)
  - Verbose successes / format inconsistencies (~6)
  - Workflow narration ("After activation:", "Phase 1/2", "Workflow:") (~5)
  - "consider re-spawning..." / "Re-ask later if still needed" advisory notes (~3)

- **LOW**: 9 findings  
  - duplicated schema-vs-description copy
  - mild meta-instructional one-liners
  - debug-prefix terseness

**Total: 64 findings** (24 HIGH / 31 MED / 9 LOW).

### Highest-priority fixes (HIGH-only triage)

1. **Framework**: add a sanitization layer at `loop/tools.go:244-246` so `err.Error()` doesn't leak `<pkg>.<Method>: %w` chains to LLM. Either map known sentinels to friendly strings family-by-family, OR strip leading `pkg.Method: ` prefixes before producing tool_result text.
2. **AP4 path leaks**: scrub `~/.forgify/...` / `mcp.json` / `SKILL.md` / `${CLAUDE_SKILL_DIR}` / DB column names (`created_at`) from every LLM-facing string. The LLM doesn't operate at filesystem level for these resources.
3. **AP4 UI bleed**: scrub "MCP servers UI", "click 'Reconnect'", "drag a SKILL.md folder", "click-to-pick buttons", "the user can watch progress" from descriptions and result strings.
4. **AP3 preambles**: trim "Use when:"/"Don't use when:" sections — tool registry's job to teach the LLM, not 11 individual descriptions teaching the same pattern.
5. **AP7 subagent comments**: fix lines 163-164 and 191-194 in `subagent/agent.go` — the described error-handling paths don't exist.
6. **AP1 skill/activate opening**: "Load a skill's full instructions and start following them." → demote imperative; describe what the tool returns, not what the LLM should do.
