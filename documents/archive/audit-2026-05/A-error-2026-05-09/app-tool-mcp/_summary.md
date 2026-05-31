# Package audit summary: internal/app/tool/mcp

## Spec understanding (Phase A — §S3 / §S9 / §S15 / §S16 / §S17)

- **§S3 错误不吞**: error suppression that hides user-visible failure / data loss / config drift is forbidden. The §S18 friendly tool_result pattern (errors.Is sentinel → human text) is the canonical compliant form here — errors are not swallowed but surfaced as readable LLM-facing text. Bare `_ = err` requires inline justification. `_, _ := json.Marshal(...)` of unfailable primitive types is a documented carve-out but spec calls for inline comment.
- **§S9 detached ctx 终态写**: terminal-state writes that MUST persist regardless of caller cancel use detached ctx. **N/A at this tool layer** — Execute methods delegate to `app/mcp.Service` (mcp.json save, ServerStatus update) and `app/sandbox.Service` (env install) which are responsible for §S9 compliance. Both already covered by app-mcp + app-sandbox audits.
- **§S15 ID 生成**: `<prefix>_<16hex>` via idgenpkg. Tool layer doesn't generate business IDs (server names use the curated catalog short slug, not §S15-style IDs by design).
- **§S16 错误 wrap 格式**: `fmt.Errorf("<pkg>.<Method>: %w", err)`. Package uses `<tool_name>:` helper-style prefix (e.g. `search_mcp.Execute:`, `install_mcp_server:`) — consistent within package, deviates from strict literal `mcptool.<Type>.<Method>:`. Audit-recommended WAIVE per the established "consistency-over-strict-literal" precedent set by app-tool-shell, infra-llm, and others.
- **§S17 errmap 单一事实源**: every sentinel that can reach a handler must be in errmap.go. Package defines 3 local validation sentinels (ErrEmptyQuery, ErrEmptyServer, ErrEmptyTool) — all consumed by Tool framework, never reach errmap (no registration needed). All consumed mcpdomain sentinels (ErrServerNotFound, ErrServerNotConnected, ErrToolNotFound, ErrToolCallTimeout, ErrToolCallFailed, ErrRegistryEntryNotFound, ErrMarketplaceUnavailable, ErrAlreadyInstalled, ErrRequiredEnvMissing, ErrRequiredArgsMissing, ErrInstallFailed, ErrHandshakeFailed) verified registered errmap.go:131-146.

## Files audited

| File | LOC | Sites | OK | POST-FIX | VIOLATION | EDGE |
|---|---|---|---|---|---|---|
| mcp.go | 56 | 1 | 1 | 0 | 0 | 0 |
| search.go | 159 | 6 | 5 | 0 | 0 | 1 |
| call.go | 173 | 5 | 5 | 0 | 0 | 0 |
| list_marketplace.go | 122 | 3 | 1 | 0 | 0 | 2 |
| install_server.go | 242 | 9 | 4 | 0 | 0 | 5 |
| uninstall_server.go | 89 | 4 | 1 | 0 | 0 | 3 |
| **TOTAL** | **841** | **28** | **17** | **0** | **0** | **11** |

## Severity breakdown (only VIOLATION + EDGE)

| Severity | Count | Sites | Status |
|---|---|---|---|
| HIGH | 0 | — | — |
| MED | 0 | — | — |
| LOW (impl-detail leak in friendly text) | 4 | search.go:#5 (~/.forgify/mcp.json mention); call.go:#5 ("MCP settings panel" + mcp.json reference); install_server.go:#1 (description mentions mcp.json); uninstall_server.go:#3 (~/.forgify/mcp.json mention) | FOUND |
| LOW (silent json.Marshal of unfailable types — missing inline ritual) | 4 | list_marketplace.go:#3; install_server.go:#6, #7, #8; uninstall_server.go:#4 | FOUND |
| LOW (validation `errors.New` for "name required" — bare instead of sentinel) | 2 | install_server.go:#2; uninstall_server.go:#1 | FOUND |
| LOW (tool description / suggested_question pattern — borderline by anti-pattern checklist) | 1 | install_server.go:#9 (suggested_question template — accepted per §S18 / "everything in LLM" precedent) | FOUND |

(Counts: 4 + 4 + 2 + 1 = 11 EDGE; matches table above.)

## Cross-cutting

### Sentinel chain integrity (§S17)

Per call.go::mapCallToolErrorToFriendly + install_server.go::Execute switch — all consumed mcpdomain sentinels are in errmap.go (verified errmap.go:131-146). Local validation sentinels (ErrEmptyQuery / ErrEmptyServer / ErrEmptyTool) are framework-consumed, not registered (correctly).

**No missing registrations**.

### Tool result anti-pattern audit (parent's specific concern)

Parent flagged "teaching-style result / impl-detail leak / self-promoting error" as anti-patterns. Findings:

| Pattern | Sites | Verdict |
|---|---|---|
| Teaching-style (LLM-to-user copy in tool result) | search.go:#5 (instruction text); call.go:#5 friendly map | EDGE — accepted per §S18 friendly tool_result; the pattern is the framework's contract, not a leak |
| Impl-detail leak (~/.forgify/mcp.json mentions) | search.go:#5; call.go:#5; install_server.go:#1; uninstall_server.go:#3 | EDGE LOW — file path leaks to user via LLM relay; could trim, but informational |
| Self-promoting (suggesting another product feature) | list_marketplace.go:#2 (BYOK key suggestion when marketplace unavailable) | EDGE LOW — actionable workaround, accepted per §S18 |
| Suggested-question template | install_server.go:#9 | OK by design — `suggested_question` field is contract-named for LLM to relay/rephrase |

**Verdict on the anti-pattern question**: package largely compliant with the §S18 friendly-tool_result contract. Mild impl-detail leaks (file path mentions) are LOW EDGE that could be trimmed for hygiene but aren't functional bugs. Audit-recommend: **WAIVE all 4 impl-detail leak sites** (informational, user can search for path themselves) OR do a single sweep commit to drop "~/.forgify/mcp.json" references from result text — minimal value.

### Detached ctx coverage (§S9) — N/A at this layer

All terminal writes are below this tool layer:
- mcp.json save → app/mcp.Service.AddServer / RemoveServer (app-mcp audit)
- Sandbox env install → app/sandbox.Service.EnsureEnv (app-sandbox audit, fixed e36f890 to use context.Background)
- ServerStatus mutations → app/mcp.Service in-memory (no terminal write)
- Subprocess close → app/mcp.Service.RemoveServer (covered by 26f9c55 orphan-subprocess fix)

Tool layer correctly delegates without re-implementing §S9 logic.

### Style consistency cross-check vs sibling packages

This package is **stylistically consistent** with app-tool-shell + app-tool-search + app-tool-forge:
- Helper-style prefix `<tool_name>:` instead of strict-literal `<pkg>.<Type>.<Method>:`
- §S18 friendly tool_result pattern (errors.Is + sentinel-classified text)
- Validation sentinels are local, framework-consumed
- Local errors.New for "X required" instead of shared package sentinels

The 31 §S16 prefix LOW were WAIVED in app-tool-forge (commit 64d9535) per established precedent. Same WAIVE applies here.

## Spot-check (random clean sites — verify mechanism not rubber-stamping)

7 sites picked from `OK` set across 4 files:

1. **search.go:#3** (search.go:124-131 Execute Unmarshal): verified — `fmt.Errorf("search_mcp.Execute: parse args: %w", err)`. pkg.method prefix + sub-tag `parse args:` + %w. errors.Is unwraps to encoding/json's UnmarshalTypeError chain.
2. **call.go:#4** (call.go:138-141 Service.CallTool err → friendly map): verified — `mapCallToolErrorToFriendly` does errors.Is on 5 mcpdomain sentinels (5 case branches at lines 154-167) before falling to default; not silent fallthrough.
3. **call.go:#5** (mapCallToolErrorToFriendly default at line 165-167): verified — `return fmt.Sprintf("call_mcp %s/%s failed: %v", server, tool, err)` — `%v` is correct here because output is **friendly text content**, not propagated error chain. The function signature returns `string` (single value, not `(string, error)`).
4. **install_server.go:#3** (CheckPermissions → Allow with doc): verified — block comment at lines 95-100 cites the design rationale (LLM-driven ask flow). §S3 documented-intent carve-out.
5. **install_server.go:#5** (Execute switch over 5 sentinels): verified — every case uses errors.Is; default returns errorJSON, not silent. Compliance literal of §S18 + §S17 disciplines.
6. **list_marketplace.go:#1** (ValidateInput → nil): verified — schema at line 46-49 has empty `properties: {}`; nothing to validate. Compliant.
7. **mcp.go:#1** (MCPTools factory): verified — pure struct-literal init; no error paths exist to violate §S3-S17.

All 7 spot-checks confirmed correct classification — mechanism functioning, not rubber-stamping.

## Recommended fix priorities

1. **Silent Marshal inline comments** (LOW × 4 — list_marketplace.go #3 + install_server.go #6/#7/#8 + uninstall_server.go #4): single sweep commit, ~5 lines of inline comments. Style polish only.
2. **§S16 helper-style prefix migration**: WAIVE per established precedent across app-tool-* packages.
3. **Impl-detail leaks in friendly text**: WAIVE per §S18 precedent OR do single sweep dropping `~/.forgify/mcp.json` mentions from 4 sites. User-impact zero either way; hygiene benefit marginal.
4. **Bare errors.New for validation**: WAIVE OR introduce shared mcptool.ErrNameRequired (parity with search.go ErrEmptyQuery / call.go ErrEmptyServer/ErrEmptyTool). Stylistic only.

**Net assessment**: package is **§S3/S9/S15/S16/S17 clean** at the tool layer. 11 EDGE LOW are stylistic / template / informational; no HIGH or MED found. The §S18 friendly tool_result pattern is implemented textbook-correctly, particularly call.go::mapCallToolErrorToFriendly which can be referenced by other tool audits as a model.

## Out-of-scope notes (parent should verify)

1. **Forge-rewrite-coordinated DEFERRED items**: install/uninstall depend on app/forge.Service paths only via mcp service (no direct forge dependency in this tool package). Tool layer is unaffected by forge rewrite.
2. **Phase C (Tool result anti-pattern) cross-fork concern**: per parent's directive this audit fork is Phase A focused; the anti-pattern findings (4 impl-detail leak sites) are noted as LOW EDGE here but properly belong in Phase C's anti-pattern sweep when that runs.
