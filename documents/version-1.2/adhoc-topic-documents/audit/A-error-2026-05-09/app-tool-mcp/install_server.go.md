# audit: backend/internal/app/tool/mcp/install_server.go

LOC: 242
Read: full file (lines 1-242)

## Trace

| site# | file:line | snippet | category | classification | reasoning | severity | user_impact | suggested_fix | status |
|---|---|---|---|---|---|---|---|---|---|
| 1 | install_server.go:54-63 | tool description text — "name doubles as the mcp.json key — no separate alias" + "already_installed means..." | A.1 | EDGE | **Implementation-detail leak** in tool description: mentions `mcp.json` storage file. Per audit anti-pattern checklist this is a "implementation leak" — LLM sees and may relay to user. **However**: description is for LLM context (helping it pick the right tool), not user-facing copy; user only sees what LLM chooses to relay. Acceptable per §S18 precedent. | LOW | LLM may parrot file path "mcp.json" to user; informational. | could trim to "name is the canonical short slug; already_installed means an existing config conflicts". Stylistic only. | FOUND |
| 2 | install_server.go:84-93 | `ValidateInput: var a installArgs; if err := json.Unmarshal(args, &a); err != nil { return fmt.Errorf("install_mcp_server: bad args: %w", err) }; if strings.TrimSpace(a.Name) == "" { return errors.New("install_mcp_server: name is required") }` | A.4/A.5 | EDGE | (a) prefix `install_mcp_server:` — Tool Name() helper-style, consistent. (b) `errors.New` for "name required" — bare errors.New, no sentinel. **However**: ValidateInput errors are consumed by Tool framework and converted to tool_result string; never reach errmap. Sentinel registration N/A. | LOW | identical UX (LLM sees friendly tool_result either way). | could introduce shared `mcptool.ErrNameRequired` for parity with search.go ErrEmptyQuery — purely stylistic | FOUND |
| 3 | install_server.go:101-103 | `CheckPermissions(...) PermissionResult { return PermissionAllow }` | A.1 | OK | doc comment lines 95-100 explicitly explains why (LLM-driven ask flow handles real consent; framework-level Ask would pop UI dialog out-of-band). §S3 documented-intent carve-out. Compliance literal. | N-A | — | — | — |
| 4 | install_server.go:112-129 | `Execute: var args installArgs; if err := json.Unmarshal([]byte(argsJSON), &args); err != nil { return "", fmt.Errorf("install_mcp_server: %w", err) }; entry, err := t.svc.GetRegistryEntry(ctx, args.Name); if err != nil { if errors.Is(err, mcpdomain.ErrRegistryEntryNotFound) { return errorJSON("not_in_registry", ...), nil }; if errors.Is(err, mcpdomain.ErrMarketplaceUnavailable) { return errorJSON("marketplace_unavailable", ...), nil }; return "", fmt.Errorf("install_mcp_server: %w", err) }` | A.1/A.4 | OK | §S16 canonical wraps + §S18 sentinel-classified friendly returns. ErrRegistryEntryNotFound + ErrMarketplaceUnavailable both registered errmap (errmap.go:136, :143 — covered if any path Returns err to handler instead of friendly text). | N-A | — | — | — |
| 5 | install_server.go:140-159 | `st, err := t.svc.InstallFromRegistry(ctx, args.Name, args.Env, args.Arguments); switch { case err == nil: ...; case errors.Is(err, ErrAlreadyInstalled): ...; case errors.Is(err, ErrRequiredEnvMissing): ...; case errors.Is(err, ErrRequiredArgsMissing): ...; case errors.Is(err, ErrInstallFailed): ...; case errors.Is(err, ErrHandshakeFailed): ...; default: return errorJSON("install_failed", err.Error()), nil }` | A.1 | OK | textbook §S18: 5 sentinel branches all errors.Is + friendly errorJSON. Default case captures unexpected errs as `install_failed` errorJSON (LLM-readable, not silent). All 5 sentinels registered errmap.go:140-146 (post-fix d6b626f for sandbox sentinel chain truncation). Compliance literal. | N-A | — | — | — |
| 6 | install_server.go:212 | `b, _ := json.Marshal(envelope)` (in phase1Envelope) | A.1 | EDGE | silent Marshal of `map[string]any` with primitives + struct slices — same unfailable pattern as list_marketplace.go #3. Missing inline ritual. | LOW | zero impact (Go invariant). | add comment | FOUND |
| 7 | install_server.go:226 | `b, _ := json.Marshal(envelope)` (in successJSON) | A.1 | EDGE | same as #6. | LOW | zero. | add comment | FOUND |
| 8 | install_server.go:239 | `b, _ := json.Marshal(envelope)` (in errorJSON) | A.1 | EDGE | same as #6. | LOW | zero. | add comment | FOUND |
| 9 | install_server.go:165-201 | `phase1Envelope: builds suggested_question with "It needs the following environment variables: ..." copy + Notes pass-through; envelope[suggested_question] = qb.String()` | A.1 | EDGE | **suggested_question pattern** — LLM-to-user copy template embedded in tool result. Per anti-pattern checklist this looks like "teaching/copy" but the field is named `suggested_question` (clear contract: it's a *suggestion* the LLM may rephrase). Per §S18 / project preference for "everything in LLM" this is the canonical install confirmation flow. Not a violation. | N-A | — | — | — |

## Sub-check

A.1 §S3 错误吞没:
  - violations: not present
  - EDGE notes: 4 sites (#1 mild impl-detail leak; #2 bare errors.New for validation; #6/#7/#8 silent Marshal of unfailable types; #9 suggested_question template — none functional issues)

A.2 §S9 detached ctx 终态写:
  - terminal-state writes identified: none at this layer
  - 各自 ctx 来源: N/A
  - violations: N/A — InstallFromRegistry's terminal writes (mcp.json save, sandbox env install, ServerStatus update) are at app/mcp service layer (covered in app-mcp audit) and infra/sandbox layer (covered in infra-sandbox audit). Tool layer just delegates.

A.3 §S15 ID 生成:
  - violations: N/A — package doesn't generate business IDs

A.4 §S16 错误 wrap 格式:
  - violations: not present (all wraps canonical + helper-style WAIVE-acceptable)

A.5 §S17 sentinel 登记 errmap:
  - sentinels defined: none in this file
  - mcpdomain sentinels consumed: ErrRegistryEntryNotFound, ErrMarketplaceUnavailable, ErrAlreadyInstalled, ErrRequiredEnvMissing, ErrRequiredArgsMissing, ErrInstallFailed, ErrHandshakeFailed — all 7 registered errmap.go:136-146 ✓
  - missing: none
