# audit: backend/internal/app/mcp/calltool.go

LOC: 333
Read: full file (lines 1-333)

## Trace

| site# | file:line | snippet | category | classification | reasoning | severity | user_impact | suggested_fix | status |
|---|---|---|---|---|---|---|---|---|---|
| 1 | calltool.go:56-58 | `if state == nil { return "", fmt.Errorf("mcpapp.CallTool: %w: %q", mcpdomain.ErrServerNotFound, server) }` | A.4 | OK | §S16 canonical: pkg.Method prefix + %w; sentinel `ErrServerNotFound` registered errmap.go:127 | N-A | — | — | — |
| 2 | calltool.go:59-62 | `if !hasClient || !mcpdomain.IsCallable(state.Status) { return "", fmt.Errorf("mcpapp.CallTool %s: %w (status=%s)", server, mcpdomain.ErrServerNotConnected, state.Status) }` | A.4 | OK | §S16 canonical pattern + sentinel `ErrServerNotConnected` errmap.go:128 | N-A | — | — | — |
| 3 | calltool.go:68-71 | `if !toolExists(state.Tools, tool) { return "", fmt.Errorf("mcpapp.CallTool %s/%s: %w", server, tool, mcpdomain.ErrToolNotFound) }` | A.4 | OK | §S16 canonical + ErrToolNotFound errmap.go:129 | N-A | — | — | — |
| 4 | calltool.go:73-79 | `cctx, cancel := context.WithTimeout(ctx, timeout); defer cancel(); result, err := client.CallTool(cctx, ...); s.recordCallResult(ctx, server, err); return result, err` | A.1/A.4 | OK | bare return: client.CallTool wraps internally with ErrToolCallTimeout/ErrToolCallFailed sentinels per mcp.md §11; recordCallResult is best-effort health bookkeeping (err already returned to caller, no second use to bubble) | N-A | — | — | — |
| 5 | calltool.go:96-101 | `all := s.ListTools(ctx); if len(all) == 0 { return []mcpdomain.ToolDef{}, nil } if len(all) <= topK { return all, nil }` | A.1 | OK | non-error early returns; mcp.md §6 "少时直接全返" documented design | N-A | — | — | — |
| 6 | calltool.go:115-119 | `bundle, err := llmclientpkg.Resolve(...); if err != nil { em.StopBlock(ctx, progID, eventlogdomain.StatusError, err); return nil, fmt.Errorf("mcpapp.Search: resolve LLM: %w", err) }` | A.4 | OK | §S16 canonical with sub-segment "resolve LLM"; emits progress block error before returning (audit trail in eventlog) | N-A | — | — | — |
| 7 | calltool.go:120-131 | `resp, err := llminfra.Generate(...); if err != nil { em.StopBlock(ctx, progID, eventlogdomain.StatusError, err); return nil, fmt.Errorf("mcpapp.Search: llm: %w", err) }` | A.4 | OK | §S16 canonical | N-A | — | — | — |
| 8 | calltool.go:132 | `em.StopBlock(ctx, progID, eventlogdomain.StatusCompleted, nil)` | A.1 | OK | progress block normal close; emit failure on this call would not affect business correctness; emitter no-op fallback per pkg/eventlog.From | N-A | — | — | — |
| 9 | calltool.go:134-153 | `indices, err := parseRankedIndices(resp, len(all)); if err != nil { ... s.log.Warn("mcp search rank parse failed", ...); return nil, fmt.Errorf("mcpapp.Search: ranking failed; LLM should retry or refine query: %w", err) }` | A.1/A.4 | OK | §S16 canonical with diagnostic suffix; logs at WARN with response snippet for debugging; doc comment cites design rationale (避免误导性 alpha-fallback per 屎山拯救计划 #4) — exemplary §S3+S16 implementation | N-A | — | — | — |
| 10 | calltool.go:155-165 | indices iteration with `if idx < 0 \|\| idx >= len(all) { continue }` | A.1 | OK | parseRankedIndices already pre-filters at line 306-310; this is defense-in-depth with explicit skip — no error to swallow | N-A | — | — | — |
| 11 | calltool.go:180-182 | `if state == nil { return nil, fmt.Errorf("mcpapp.HealthCheck: %w: %q", mcpdomain.ErrServerNotFound, name) }` | A.4 | OK | §S16 canonical; sentinel registered | N-A | — | — | — |
| 12 | calltool.go:187-191 | `if !hasClient { res.Healthy = false; res.Error = "server not connected"; return res, nil }` | A.1 | OK | non-error result path: HealthCheck design intent per mcp.md §5.6 returns HealthResult with Healthy/Error fields rather than sentinel — UI consumes the struct, not error | N-A | — | — | — |
| 13 | calltool.go:197-203 | `tools, err := client.ListTools(cctx); res.LatencyMs = ...; if err != nil { res.Healthy = false; res.Error = err.Error(); return res, nil }` | A.1 | OK | same design intent — HealthCheck never errors out; reports failure inline; explicit Healthy=false + Error fields | N-A | — | — | — |
| 14 | calltool.go:224-232 | `func (s *Service) recordCallResult(_ context.Context, name string, err error) { ... state := s.states[name]; if state == nil { return }` | A.1 | OK | `_ context.Context` ctx is unused (signature param for future use); `state == nil` early-return is internal-state guard not error suppression | N-A | — | — | — |
| 15 | calltool.go:233-248 | recordCallResult body — increments counters, transitions Status without writing to DB (in-memory only) | A.2 | OK | NOT a terminal-state DB write — mcp ServerStatus is purely in-memory map, never persisted; mcp.md §5.6 spec says "被动 health" purely lives in s.states map; recovery on restart is by re-Connect not state-restore | N-A | — | — | — |
| 16 | calltool.go:256-261 | `func (s *Service) resolveCallTimeout(cfg mcpdomain.ServerConfig) time.Duration { if cfg.TimeoutSec > 0 { ... } return defaultCallTimeout }` | A.1 | OK | pure timeout calculation; no errors involved | N-A | — | — | — |
| 17 | calltool.go:296-304 | `jsonStr, ok := llmparsepkg.ExtractJSON(resp); if !ok { return nil, fmt.Errorf("no JSON in response: %q", trimResp(resp, 200)) } var raw []int; if err := json.Unmarshal(...); err != nil { return nil, fmt.Errorf("parse JSON: %w", err) }` | A.4 | EDGE | §S16: line 299 returns plain `fmt.Errorf("no JSON in response: %q", ...)` with NO sentinel and NO pkg.Method prefix; line 303 `fmt.Errorf("parse JSON: %w", err)` similarly lacks pkg.Method prefix. Both errors are wrapped at site #9 with `mcpapp.Search: ranking failed: %w` so caller can identify the call site, but the inner unwrap shows no clear sentinel-anchored path. Functionally OK because parseRankedIndices is unexported helper called only from Search; but inconsistent with §S16 literal `<pkg>.<Method>:` prefix. | LOW | error message at debug shows "mcpapp.Search: ranking failed; LLM should retry or refine query: parse JSON: invalid character ..." — readable but lacks helper's loc | could prefix with `mcpapp.parseRankedIndices:` for consistency | **FIXED 2026-05-09 505d6e3** |
| 18 | calltool.go:317-324 | `func toolExists(tools []mcpdomain.ToolDef, name string) bool { for _, t := range tools { if t.Name == name { return true } } return false }` | A.1 | OK | pure boolean check; no errors involved | N-A | — | — | — |

## Sub-check

A.1 §S3 错误吞没:
  - violations: not present
  - Note: HealthResult-as-return pattern (sites #12, #13) is design intent per mcp.md §5.6, NOT silent fallback — error info is captured in HealthResult.Error field

A.2 §S9 detached ctx 终态写:
  - terminal-state writes identified: none
  - 各自 ctx 来源: N/A
  - violations: N/A — calltool.go performs no DB writes. recordCallResult (sites #14-15) mutates only in-memory state (`s.states` map), which is per-process volatile by design per mcp.md §5.6 (health counters not persisted; recovery via re-Connect on restart)

A.3 §S15 ID 生成:
  - ID generation calls: none
  - violations: N/A — calltool.go does not generate business IDs (no idgenpkg.New, no crypto/rand calls)

A.4 §S16 错误 wrap 格式:
  - violations: 1 EDGE LOW (site #17 — parseRankedIndices internal helper missing `<pkg>.<Method>:` prefix on its inner errors; outer Search call site does wrap correctly)
  - All other 8 fmt.Errorf calls follow canonical `<pkg>.<Method>:` + %w form

A.5 §S17 sentinel 登记 errmap:
  - sentinels defined: none (file consumes mcpdomain sentinels but defines none)
  - 已登记 errmap: ErrServerNotFound (errmap.go:127), ErrServerNotConnected (errmap.go:128), ErrToolNotFound (errmap.go:129) — all consumed sentinels are registered
  - missing: N/A — all consumed sentinels properly registered
