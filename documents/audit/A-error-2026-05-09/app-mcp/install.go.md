# audit: backend/internal/app/mcp/install.go

LOC: 287
Read: full file (lines 1-287)

## Trace

| site# | file:line | snippet | category | classification | reasoning | severity | user_impact | suggested_fix | status |
|---|---|---|---|---|---|---|---|---|---|
| 1 | install.go:46-49 | `entry, err := s.GetRegistryEntry(ctx, name); if err != nil { return nil, err }` | A.4 | EDGE | bare return — sentinel `ErrRegistryEntryNotFound` preserved (errmap.go:132); inconsistent with rest of file which wraps. Same style inconsistency as flagged in app-chat history.go #2 / chat.go #11 / apikey site #17. Functionally OK — sentinel reaches errmap; loses call-site loc only. | LOW | identical UX (errmap matches sentinel → 404 MCP_REGISTRY_ENTRY_NOT_FOUND); harder to grep call site | wrap to match site #5/#6: `return nil, fmt.Errorf("mcpapp.InstallFromRegistry %s: %w", name, err)` | **FIXED 2026-05-09 505d6e3** |
| 2 | install.go:53-59 | `s.mu.RLock(); _, collided := s.configs[name]; s.mu.RUnlock(); if collided { return nil, fmt.Errorf("mcpapp.InstallFromRegistry %s: %w", name, mcpdomain.ErrAlreadyInstalled) }` | A.4 | OK | §S16 canonical: pkg.Method prefix + %w; sentinel `ErrAlreadyInstalled` registered errmap.go:140 | N-A | — | — | — |
| 3 | install.go:66-69 | `if missing := missingEnvKeys(...); len(missing) > 0 { return nil, fmt.Errorf("mcpapp.InstallFromRegistry %s: %w: %s", name, mcpdomain.ErrRequiredEnvMissing, strings.Join(missing, ", ")) }` | A.4 | OK | §S16 canonical with diagnostic suffix + sentinel `ErrRequiredEnvMissing` registered errmap.go:134 | N-A | — | — | — |
| 4 | install.go:70-73 | `if missing := missingArgKeys(...); len(missing) > 0 { return nil, fmt.Errorf("mcpapp.InstallFromRegistry %s: %w: %s", name, mcpdomain.ErrRequiredArgsMissing, strings.Join(missing, ", ")) }` | A.4 | OK | §S16 canonical + ErrRequiredArgsMissing errmap.go:135 | N-A | — | — | — |
| 5 | install.go:107-119 | `_, ensureErr := installprogresspkg.Run(ctx, ..., func(progress) { return s.sandbox.EnsureEnv(ctx, owner, spec, progress) }); if ensureErr != nil { return nil, fmt.Errorf("mcpapp.InstallFromRegistry %s: %w: %v", name, mcpdomain.ErrInstallFailed, ensureErr) }` | A.4 | EDGE | §S16: format string is `%w: %v` — `mcpdomain.ErrInstallFailed` is wrapped with %w (sentinel preserved, errmap.go:136 maps to 502), but the inner upstream `ensureErr` is rendered with `%v` instead of nested wrap. errors.Is unwraps only to ErrInstallFailed; the original sandbox error chain (ErrRuntimeInstallFailed / ErrEnvCreateFailed sentinels per sandboxdomain) is **lost** for callers that want to discriminate. | MED | LLM/UI sees "install failed: <stderr tail>" but cannot programmatically detect runtime-not-supported vs install-script-failed. Most callers just show the message, but errors.Is(err, sandboxdomain.ErrRuntimeNotSupported) returns false despite the underlying cause. | switch to nested wrap: `return nil, fmt.Errorf("mcpapp.InstallFromRegistry %s: %w: %w", name, mcpdomain.ErrInstallFailed, ensureErr)` (Go 1.20+ supports multi-%w) — preserves both sentinels in chain | **FIXED 2026-05-09 26f9c55** |
| 6 | install.go:137-139 | `if err := s.AddServer(ctx, cfg); err != nil { return nil, fmt.Errorf("mcpapp.InstallFromRegistry %s: %w", name, err) }` | A.4 | OK | §S16 canonical | N-A | — | — | — |
| 7 | install.go:141-144 | `st, err := s.GetServer(ctx, name); if err != nil { return nil, err }` | A.4 | EDGE | bare return — same style inconsistency as site #1; sentinel preserved | LOW | same as site #1 | wrap: `return nil, fmt.Errorf("mcpapp.InstallFromRegistry %s: %w", name, err)` | **FIXED 2026-05-09 505d6e3** |
| 8 | install.go:181-182 | `if c, ok := s.clients[name]; ok { _ = c.Close(); delete(s.clients, name) }` | A.1 | EDGE | §S3: `_ = c.Close()` discards Close error without inline justification comment. Per §S3 spec example: `_ = err` requires inline comment; here Close() failure on stdio MCP client could mean the previous subprocess wedged but importing a fresh one over the top still works at app level. **However**, Close failure on a stdio Client typically means the subprocess didn't reap — that subprocess is now an orphan. Worth at minimum a Warn log. | LOW | orphaned MCP subprocess survives `Import overwrite=true` if Close fails; eventually OS reaps when parent exits, but until then duplicates run | log: `if err := c.Close(); err != nil { s.log.Warn("mcp.Service.Import: stale client close failed (subprocess may be orphaned)", zap.String("server", name), zap.Error(err)) }` | **FIXED 2026-05-09 26f9c55** |
| 9 | install.go:188-190 | `if err := mcpinfra.Save(s.configPath, configsCopy); err != nil { return res, fmt.Errorf("mcpapp.Import: save mcp.json: %w", err) }` | A.4 | OK | §S16 canonical with sub-segment "save mcp.json"; returns `res` along with err so caller sees what was attempted | N-A | — | — | — |
| 10 | install.go:194-203 | `for _, name := range res.Imported { go func(n string) { ... if err := s.connectOne(cctx, n); err != nil { s.log.Warn("mcp imported server connect failed", ...) } }(name) }` | A.1 | OK | per-server async Connect: failure logged at WARN with server name + err — visible in dev logs; ServerStatus also captures the failure (per connectOne implementation, see mcp.go); per mcp.md §5.6 design intent "fail-loud, no auto-restart" — Warn log + ServerStatus.LastError is the documented audit trail | N-A | — | — | — |
| 11 | install.go:219-227 | `func missingEnvKeys(...) []string { ... return missing }` | A.1 | OK | pure helper; no errors | N-A | — | — | — |
| 12 | install.go:234-246 | `func missingArgKeys(...) []string { ... }` | A.1 | OK | pure helper | N-A | — | — | — |
| 13 | install.go:253-259 | `func substituteArgs(args, vars) []string` | A.1 | OK | pure helper | N-A | — | — | — |
| 14 | install.go:268-287 | `func expandVars(s, vars) string` | A.1 | OK | pure string scan | N-A | — | — | — |

## Sub-check

A.1 §S3 错误吞没:
  - violations: 1 EDGE LOW (site #8 — `_ = c.Close()` lacks inline justification + no Warn log; orphaned subprocess risk on Import overwrite path)
  - silent fallback: not present
  - documented soft-fails (site #10 connectOne async): correctly logged at WARN with full context

A.2 §S9 detached ctx 终态写:
  - terminal-state writes identified: 
    - site #5 (sandboxapp.EnsureEnv via installprogresspkg) — uses caller ctx; this is install-time DB write into sandbox_envs table
    - site #6 (s.AddServer → mcp.json file write + state mutation)
    - site #9 (mcpinfra.Save → mcp.json file write)
  - 各自 ctx 来源: all three use caller's request ctx (HTTP handler ctx for /mcp-registry/{name}:install or import endpoint)
  - violations: not present
  - Reasoning: For install/import flows the **user is synchronously waiting** on the HTTP response. If they cancel mid-install, aborting the install is **desired** behavior — partial state is recoverable (re-run install, or remove failed entry). This is mcp.md §3 "fail-loud, no auto-restart" intent: don't silently complete a cancelled install. Contrasts with apikey.Test where the user-visible status flip MUST land regardless of cancel because next-time-the-page-loads visibility depends on it. Install path's terminal state IS the response payload — the user already knows whether it succeeded/failed by whether they got a 201 or err.
  - Caveat: site #10 (async Connect post-import) **could** be argued as terminal-write since it mutates ServerStatus, but ServerStatus is in-memory (per mcp.md §5.6) and the goroutine uses `cctx, cancel := context.WithTimeout(ctx, initializeTimeout)` derived from request ctx — if request ctx is cancelled, async Connect aborts, leaving ServerStatus at "disconnected" (initial state). Next request to GET /mcp-servers shows the same state, no data loss. Acceptable.

A.3 §S15 ID 生成:
  - ID generation calls: none in this file
  - violations: N/A — InstallFromRegistry uses `entry.Name` (curated catalog short slug) directly as mcp.json key + Owner.ID per mcp.md §5.5 "single Name field" decision (no separate alias). Sandbox owner.ID = `name` is plain string (not `<prefix>_<16hex>`) but mcp owner.ID isn't a §S15 business ID — it's a deterministic key derived from registry slug, which is fine per the decision.

A.4 §S16 错误 wrap 格式:
  - violations: 
    - 2 LOW EDGE (sites #1, #7 — bare returns, style inconsistency with rest of file's wrap pattern; sentinel chain preserved)
    - 1 MED EDGE (site #5 — `%w: %v` loses inner sandbox sentinel chain; should use `%w: %w` to preserve both)

A.5 §S17 sentinel 登记 errmap:
  - sentinels defined: none (file consumes mcpdomain sentinels)
  - 已登记 errmap: ErrRegistryEntryNotFound (132), ErrAlreadyInstalled (140), ErrRequiredEnvMissing (134), ErrRequiredArgsMissing (135), ErrInstallFailed (136) — all consumed sentinels registered
  - missing: N/A — all consumed sentinels registered
