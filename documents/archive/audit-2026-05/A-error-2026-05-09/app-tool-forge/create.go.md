# audit: backend/internal/app/tool/forge/create.go

LOC: 215
Read: full file (lines 1-215)

## Trace

| site# | file:line | snippet | category | classification | reasoning | severity | user_impact | suggested_fix | status |
|---|---|---|---|---|---|---|---|---|---|
| 1 | create.go:57-86 | Identity / Description / Parameters | N/A | OK | §S18 metadata; description is detailed but appropriate for LLM context (notes env_status flow, dependencies, python_version). No teaching-style or self-promoting copy. | N-A | — | — | — |
| 2 | create.go:90-92 | `IsReadOnly()=false / NeedsReadFirst()=false / RequiresWorkspace()=false` | N/A | OK | §S18 metadata — create_forge is mutating but doesn't touch user files (writes go to forge DB / venv dir under managed sandbox path), so RequiresWorkspace=false is accurate | N-A | — | — | — |
| 3 | create.go:97-101 | `ValidateInput=nil / CheckPermissions=Allow` | N/A | OK | mutation tool but Allow at framework level — Phase 4+ scheduler will gate via Ask path per §S18 carve-out. Documented in audit consensus. | N-A | — | — | — |
| 4 | create.go:113-115 | `if err := json.Unmarshal([]byte(argsJSON), &args); err != nil { return "", fmt.Errorf("create_forge: bad args: %w", err) }` | A.4 | EDGE | §S16: `create_forge:` tool-name prefix | LOW | identical UX | tighten prefix | FOUND |
| 5 | create.go:121-122 | `forgeID := forgeapp.NewForgeID(); pendingID := forgeapp.NewVersionID()` | A.3 | OK | §S15: delegates to forgeapp.NewForgeID / NewVersionID (audited as part of app/forge — uses idgenpkg.New("f") / ("fv"); panics on rand.Read fail per §S15 invariant). | N-A | — | — | — |
| 6 | create.go:126-133 | `draft, err := t.svc.CreateDraft(ctx, forgeapp.CreateInput{...}); if err != nil { return "", fmt.Errorf("create_forge: create draft: %w", err) }` | A.4 | EDGE | same prefix issue. CreateDraft is part of app/forge service which is DEFERRED for rewrite — but the error path here is correct: sentinel forgedomain.ErrDuplicateName from svc reaches errmap.go:81 cleanly. | LOW | identical UX | tighten | FOUND |
| 7 | create.go:161-171 | `code, err := streamCode(ctx, ..., picker, keys, factory, func(accumulated string) { draftPending.Code = accumulated; ... }); if err != nil { return "", fmt.Errorf("create_forge: generate code: %w", err) }` | A.4 | EDGE | same prefix. Note: streamCode internal also has helper-style prefix `streamCode:` — composed prefix on caller side becomes `create_forge: generate code: streamCode: <inner>` — verbose but informative. POST-COMMIT-363b084 llm sentinels propagate through chain. | LOW | identical UX | tighten | FOUND |
| 8 | create.go:179-181 | `if err := t.svc.ParseCode(code); err != nil { return "", fmt.Errorf("create_forge: generated code failed AST parse, please regenerate: %w", err) }` | A.4 | EDGE | same prefix. **POSITIVE LLM-FACING UX**: error message says "please regenerate" — actionable signal to the LLM. ParseCode wraps forgedomain.ErrASTParseError (registered errmap.go:87 ✓). | LOW | identical UX | tighten | FOUND |
| 9 | create.go:188-199 | `pending, err := t.svc.CreatePending(ctx, forgeID, forgeapp.PendingSnapshot{...}); if err != nil { return "", fmt.Errorf("create_forge: create pending: %w", err) }` | A.4 | EDGE | same prefix. CreatePending is the synchronous svc.Sync driver — its error includes sandbox install failure (forgedomain.ErrEnvCreateFailed-class). | LOW | identical UX | tighten | FOUND |
| 10 | create.go:201-204 | `var params any; if err := json.Unmarshal([]byte(pending.Parameters), &params); err != nil { return "", fmt.Errorf("create_forge: corrupted parameters after save for forge %q: %w", forgeID, err) }` | A.4 | EDGE | same prefix. **POSITIVE behavior** — surfaces post-save corruption with forge_id. Same pattern as get.go #5/#6 (both surface, search.go #12 silently swallows — package divergence). | LOW | identical UX | tighten | FOUND |
| 11 | create.go:205-213 | `b, _ := json.Marshal(map[string]any{...})` | A.1 | OK | json.Marshal of basic-type map; unfailable. Discard `_` safe-by-construction. | N-A | — | — | — |

## Sub-check

A.1 §S3 错误吞没:
  - violations: not present

A.2 §S9 detached ctx 终态写:
  - terminal-state writes identified: t.svc.CreateDraft (#6), t.svc.CreatePending (#9). Both go through ctx → app/forge service.
  - 各自 ctx 来源: request ctx (LLM tool Execute is synchronous; user-waiting flow per LLM tool calling protocol)
  - violations: N/A here — but **forge service §S9 status is DEFERRED** per app-forge audit findings (forge.go::SyncEnvForVersion §S9 issues). Tool layer correctly delegates; service layer pending rewrite.

A.3 §S15 ID 生成:
  - ID generation calls: forgeapp.NewForgeID (line 121), forgeapp.NewVersionID (line 122). Both delegate to idgenpkg.New per §S15.
  - violations: not present

A.4 §S16 错误 wrap 格式:
  - violations: sites #4, #6, #7, #8, #9, #10 (`create_forge:` tool-name prefix). Audit-recommended WAIVE per package consistency.

A.5 §S17 sentinel 登记 errmap:
  - sentinels defined: none in this file
  - 已登记 errmap: forgedomain.ErrDuplicateName (errmap.go:81), ErrASTParseError (errmap.go:87), ErrEnvCreateFailed (sandboxdomain via app/forge → registered errmap.go:104) — all reached through svc method calls
  - missing: not present
