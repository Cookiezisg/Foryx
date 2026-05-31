# audit: backend/internal/app/mcp/mcp.go

LOC: 553
Read: full file (lines 1-553)

## Trace

| site# | file:line | snippet | category | classification | reasoning | severity | user_impact | suggested_fix | status |
|---|---|---|---|---|---|---|---|---|---|
| 1 | mcp.go:135-140 | `if log == nil { panic("mcp.New: logger is nil") } if source == nil { panic("mcp.New: registry source is nil") }` | A.1 | OK | wiring-time guards; panics on nil deps are correct for §S3 (catches injection bugs at boot rather than later silent NPE) | N-A | — | — | — |
| 2 | mcp.go:141-143 | `if notif == nil { notif = notificationspkg.New(nil, log) }` | A.1 | OK | nil-tolerant fallback to no-op publisher (per notificationspkg.New); documented in mcp.md design — tests that don't exercise SSE pass nil; not silent failure | N-A | — | — | — |
| 3 | mcp.go:181-189 | `configs, err := mcpinfra.Load(s.configPath); if err != nil { s.log.Error("mcp.json load failed; starting with no servers", ...); configs = map[string]mcpdomain.ServerConfig{} }` | A.1 | OK | mcp.md §5.7 末段 documented design: corrupt mcp.json → log + treat as empty; explicit Error log surfaces issue to user; not silent | N-A | — | — | — |
| 4 | mcp.go:206-217 | parallel `go func(n string) { ... if err := s.connectOne(cctx, n); err != nil { s.log.Warn("mcp connect failed", ...) } }(name)` | A.1 | OK | per-server async Connect: failure logged with server name; ServerStatus also records (per connectOne lines 380-403); mcp.md §5 documented "fail-loud, no auto-restart" pattern with audit trail | N-A | — | — | — |
| 5 | mcp.go:227-239 | `func (s *Service) Stop(_ context.Context) error { ... for name, c := range clients { if err := c.Close(); err != nil { s.log.Warn("mcp close failed", ...) } } return nil }` | A.1 | OK | Close failure logged at WARN with server name; subprocess cleanup is best-effort during shutdown — OS reaps orphans; explicit Warn log per §S10 ("异步或 fire-and-forget 必须打") | N-A | — | — | — |
| 6 | mcp.go:250-252 | `if cfg.Name == "" { return fmt.Errorf("mcpapp.AddServer: server name required") }` | A.4 | EDGE | §S16: has pkg.Method prefix but **NO sentinel + NO %w** — this error has nothing to wrap (no upstream cause). The "server name required" condition could be a sentinel like `ErrServerNameRequired` for errmap registration; without one, errmap will fall to "unmapped domain error" path → 500 INTERNAL_ERROR + ERROR log. **Reachability**: AddServer is called from PUT /mcp-servers/{name} handler (which fills cfg.Name from URL path so always non-empty in practice) AND from InstallFromRegistry (also fills cfg.Name from registry entry). User-direct path: drag-import via Import → AddServer chain doesn't go through this since Import has its own iteration. Reachable in theory if a future call site bypasses AddServer's caller validation. | LOW | only triggers on programmer-side wiring slip; ergonomically returns "INTERNAL_ERROR" + logs unmapped warning instead of clean 400 | introduce sentinel `mcpdomain.ErrServerNameRequired` + register errmap as 400 BAD_REQUEST; OR change to `panic` (config-time invariant per the same logic as apikeytester:#4 — `cfg.Name == ""` is wiring bug not user error) | **FIXED 2026-05-09 505d6e3** |
| 7 | mcp.go:254-258 | `if existing, ok := s.clients[cfg.Name]; ok { _ = existing.Close(); delete(s.clients, cfg.Name) }` | A.1 | EDGE | §S3: `_ = existing.Close()` discards Close error without inline justification — same orphan-subprocess concern as install.go #8. AddServer's "replace existing" path is documented in lines 241-248 as deliberate; but Close failure means subprocess might wedge until OS reaps. | LOW | orphan MCP subprocess on Close-failure during AddServer replace path | wrap in if-err: `if err := existing.Close(); err != nil { s.log.Warn("mcpapp.AddServer: stale client close failed (subprocess may be orphaned)", zap.String("server", cfg.Name), zap.Error(err)) }` | **FIXED 2026-05-09 26f9c55** |
| 8 | mcp.go:268-270 | `if err := mcpinfra.Save(s.configPath, configsCopy); err != nil { return fmt.Errorf("mcpapp.AddServer: save mcp.json: %w", err) }` | A.4 | OK | §S16 canonical with sub-segment | N-A | — | — | — |
| 9 | mcp.go:271 | `s.publishStatus(ctx, cfg.Name)` | A.1 | OK | publishStatus is best-effort notification (returns no error per signature line 342); design intent — caller doesn't block on notif fan-out | N-A | — | — | — |
| 10 | mcp.go:275-277 | `if err := s.connectOne(cctx, cfg.Name); err != nil { return fmt.Errorf("mcpapp.AddServer: connect: %w", err) }` | A.4 | OK | §S16 canonical | N-A | — | — | — |
| 11 | mcp.go:286-290 | `s.mu.Lock(); if _, ok := s.configs[name]; !ok { s.mu.Unlock(); return fmt.Errorf("mcpapp.RemoveServer: %w: %q", mcpdomain.ErrServerNotFound, name) }` | A.4 | OK | §S16 canonical with sentinel | N-A | — | — | — |
| 12 | mcp.go:291-294 | `if c, ok := s.clients[name]; ok { _ = c.Close(); delete(s.clients, name) }` | A.1 | EDGE | §S3: same orphan-subprocess pattern as #7 / install.go #8. RemoveServer is the symmetric "user removes server" entry — Close failure here also leaks. | LOW | orphan subprocess on RemoveServer Close failure | log: `if err := c.Close(); err != nil { s.log.Warn("mcpapp.RemoveServer: subprocess close failed (may be orphaned)", zap.String("server", name), zap.Error(err)) }` | **FIXED 2026-05-09 26f9c55** |
| 13 | mcp.go:300-302 | `if err := mcpinfra.Save(s.configPath, configsCopy); err != nil { return fmt.Errorf("mcpapp.RemoveServer: save mcp.json: %w", err) }` | A.4 | OK | §S16 canonical | N-A | — | — | — |
| 14 | mcp.go:303-304 | `s.notif.Publish(ctx, "mcp_server", name, map[string]any{"name": name, "deleted": true})` | A.1 | OK | best-effort notification (Publish returns no err); mcp.md §5.6 documents notify-on-state-change pattern; not silent (any internal Publish issue logs in notificationspkg) | N-A | — | — | — |
| 15 | mcp.go:313-321 | `s.mu.Lock(); if _, ok := s.configs[name]; !ok { ... return fmt.Errorf("mcpapp.Reconnect: %w: %q", mcpdomain.ErrServerNotFound, name) } if c, ok := s.clients[name]; ok { _ = c.Close(); delete(s.clients, name) }` | A.1 | EDGE | same orphan-subprocess pattern (3rd occurrence: #7, #12, #15) — Reconnect Close failure → orphan | LOW | orphan subprocess on Reconnect failure | same fix as #7 #12 — log Warn with err | **FIXED 2026-05-09 26f9c55** |
| 16 | mcp.go:322-326 | `if state, ok := s.states[name]; ok { state.Status = StatusDisconnected; state.LastError = ""; state.ConsecutiveFailures = 0 }` | A.1 | OK | state mutation, no errors | N-A | — | — | — |
| 17 | mcp.go:331-333 | `if err := s.connectOne(cctx, name); err != nil { return fmt.Errorf("mcpapp.Reconnect: %w", err) }` | A.4 | OK | §S16 canonical | N-A | — | — | — |
| 18 | mcp.go:342-354 | `func (s *Service) publishStatus(ctx, name)` — signature returns nothing; best-effort | A.1 | OK | design intent: best-effort fire-and-forget; if state missing returns silently (line 350-352); notif.Publish internal failures handled by notificationspkg | N-A | — | — | — |
| 19 | mcp.go:366-373 | `s.mu.RLock(); cfg, ok := s.configs[name]; ... if !ok || state == nil { return fmt.Errorf("connectOne: %w: %q", mcpdomain.ErrServerNotFound, name) }` | A.4 | EDGE | §S16: prefix is `connectOne:` not canonical `mcpapp.connectOne:` (missing pkg qualifier); functionally OK (sentinel preserved) but inconsistent with rest of file | LOW | identical UX (sentinel reaches errmap); harder to grep | switch to `mcpapp.connectOne:` for consistency | **FIXED 2026-05-09 505d6e3** |
| 20 | mcp.go:380-390 | `if err := client.Initialize(ctx); err != nil { ... s.mu.Unlock(); _ = client.Close(); s.publishStatus(ctx, name); return err }` | A.1/A.4 | EDGE | Two issues: (a) bare `return err` — sentinel from infra/mcp.client (could be ErrHandshakeFailed errmap.go:142) preserved but no call-site context; (b) `_ = client.Close()` ignored without inline comment | LOW (a) + LOW (b) | bare-return: same style as other LOW entries; Close-discard: failed Initialize + failed Close means subprocess might hang in connecting state | (a) wrap: `return fmt.Errorf("mcpapp.connectOne: initialize: %w", err)`; (b) log Warn on Close err | **FIXED 2026-05-09 505d6e3** |
| 21 | mcp.go:392-403 | `tools, err := client.ListTools(ctx); if err != nil { ... _ = client.Close(); s.publishStatus(ctx, name); return err }` | A.1/A.4 | EDGE | same dual issues as #20 (bare-return + silent Close) | LOW × 2 | same | same | **FIXED 2026-05-09 505d6e3** |
| 22 | mcp.go:405-415 | success path: state.Status = ready, ConnectedAt set, LastError cleared, Tools cached, clients map updated, publishStatus called | A.1 | OK | success-path mutation; no errors | N-A | — | — | — |
| 23 | mcp.go:425-434 | `func (s *Service) ListServers(_) []mcpdomain.ServerStatus { s.mu.RLock(); defer s.mu.RUnlock(); ... return out }` | A.1 | OK | pure read; no errors | N-A | — | — | — |
| 24 | mcp.go:439-448 | `func (s *Service) GetServer(_, name) (*mcpdomain.ServerStatus, error) { ... if !ok { return nil, fmt.Errorf("mcpapp.GetServer: %w: %q", mcpdomain.ErrServerNotFound, name) } ...}` | A.4 | OK | §S16 canonical | N-A | — | — | — |
| 25 | mcp.go:457-468 | `func (s *Service) Stderr(name) (string, error) { ... if _, ok := s.states[name]; !ok { return "", fmt.Errorf("mcpapp.Stderr: %w: %q", ...) } c, ok := s.clients[name]; if !ok { return "", nil } return c.StderrTail(), nil }` | A.1/A.4 | OK | §S16 canonical for not-found path; "configured but not connected" returns ("", nil) per documented intent (lines 451-456) — not silent fallback, design contract | N-A | — | — | — |
| 26 | mcp.go:478-497 | `func (s *Service) ListTools(_) []mcpdomain.ToolDef { ... }` | A.1 | OK | pure read aggregation; no errors | N-A | — | — | — |
| 27 | mcp.go:510-512 | `func (s *Service) ListRegistry(ctx) ([]..., error) { return s.source.List(ctx) }` | A.4 | EDGE | bare passthrough — source.List returns sentinel from CuratedRegistrySource (which currently never errors per infra/mcp/curated_registry.go); no wrap. If source ever switches to a network-backed implementation, ErrMarketplaceUnavailable would surface unwrapped (sentinel preserved through bare return, but no call-site loc). | LOW | identical UX (sentinel preserved); harder to grep | optional wrap: `if err != nil { return nil, fmt.Errorf("mcpapp.ListRegistry: %w", err) }` | **FIXED 2026-05-09 505d6e3** |
| 28 | mcp.go:518-524 | `func (s *Service) GetRegistryEntry(ctx, name) (*..., error) { e, err := s.source.Get(ctx, name); if err != nil { return nil, fmt.Errorf("mcpapp.GetRegistryEntry %s: %w", name, err) } return e, nil }` | A.4 | OK | §S16 canonical | N-A | — | — | — |
| 29 | mcp.go:533-539 | `func (s *Service) cloneConfigsLocked() map[...]` | A.1 | OK | pure helper; no errors | N-A | — | — | — |
| 30 | mcp.go:545-552 | `func (s *Service) snapshotLocked() []mcpdomain.ServerStatus` | A.1 | OK | pure helper; no errors. Note: this function is unused in current code — staticcheck flagged it as U1000 in prior runs, but not a §S3 concern (it's documentation-grade dead code) | N-A | — | — | — |

## Sub-check

A.1 §S3 错误吞没:
  - violations: 5 EDGE LOW — all the same pattern (`_ = c.Close()` on subprocess wrap during state mutation):
    - site #7 (AddServer replace path)
    - site #12 (RemoveServer)
    - site #15 (Reconnect)
    - site #20b (connectOne Initialize-fail cleanup)
    - site #21b (connectOne ListTools-fail cleanup)
  - Each represents potential MCP subprocess orphan on Close failure; consistent fix = log Warn with server name + err
  - silent fallback: not present
  - documented soft-fails (mcp.json load #3, async Connect #4, Stop close #5): all have explicit log + audit trail per §S10 / §S3 soft-fail pattern

A.2 §S9 detached ctx 终态写:
  - terminal-state writes identified:
    - site #8 (mcpinfra.Save in AddServer)
    - site #13 (mcpinfra.Save in RemoveServer)
    - site #10 (s.connectOne in AddServer — uses cctx via WithTimeout(ctx, addServerTimeout))
    - site #17 (s.connectOne in Reconnect — uses cctx via WithTimeout(ctx, initializeTimeout))
    - site #14 (s.notif.Publish in RemoveServer)
    - site #9 (s.publishStatus in AddServer)
    - state mutations in connectOne (#20-22): in-memory only, not DB writes
  - 各自 ctx 来源: all use caller's request ctx (or cctx derived from it via WithTimeout)
  - violations: not present
  - Reasoning: Same as install.go A.2 sub-check — install/lifecycle flows are **synchronous user-waits-on-HTTP-response** semantics. User cancel mid-AddServer/RemoveServer is **desired** abort; partial state is recoverable on next request. mcpinfra.Save is file-system not DB; if user cancels mid-Save the write is atomic per mcpinfra spec (rename pattern) so no torn write. The async Connect goroutine in Start (#4) and Import uses `cctx, cancel := WithTimeout(ctx, initializeTimeout)` derived from caller ctx — if request ctx cancels, async Connect aborts too, leaving ServerStatus=Disconnected (initial state, no data loss). Acceptable per mcp.md §5 design.

A.3 §S15 ID 生成:
  - ID generation calls: none in this file
  - violations: N/A — mcp uses `cfg.Name` (user-supplied / curated slug) as identifier per mcp.md §5.5 (no separate alias). Not a §S15 business ID.

A.4 §S16 错误 wrap 格式:
  - violations:
    - 1 EDGE LOW (site #6 — `mcpapp.AddServer: server name required` no sentinel; would trigger unmapped-error alarm if called)
    - 1 EDGE LOW (site #19 — `connectOne:` prefix missing `mcpapp.` qualifier)
    - 3 EDGE LOW (sites #20a, #21a, #27 — bare returns inconsistent with rest of file)
  - All 16 other fmt.Errorf calls follow canonical `<pkg>.<Method>:` + %w form

A.5 §S17 sentinel 登记 errmap:
  - sentinels defined: none in this file (consumes mcpdomain sentinels)
  - 已登记 errmap (mcp domain consumed in this file):
    - ErrServerNotFound (errmap.go:127) — used at sites #11, #15, #19, #24, #25
    - ErrAlreadyInstalled — referenced via install.go (not directly here)
    - infra/mcp client returns ErrHandshakeFailed (142) — surfaced at site #20
  - missing: N/A — all consumed sentinels registered. EDGE: site #6's plain string `"server name required"` has no sentinel to register (would need `mcpdomain.ErrServerNameRequired` to be created first).
