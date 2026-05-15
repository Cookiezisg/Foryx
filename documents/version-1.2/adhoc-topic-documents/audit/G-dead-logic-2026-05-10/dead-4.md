# Dead-logic audit — mcp (app + infra + app/tool + domain)

Date: 2026-05-10
Scope: `internal/{app,infra,domain}/mcp/` + `internal/app/tool/mcp/`. forge untouched. Test files skipped per instruction.
Method: read every non-test .go file end-to-end, cross-check producers/consumers, consult `git log` + `mcp.md` design doc when historic context needed.

LOC ≈ 2400 production .go (5 app/mcp / 4 infra/mcp / 2 domain/mcp / 6 app/tool/mcp + 1 transport handler).

Background: V3 marketplace transition (commits `ede777d` 2026-05-09 search→list rename, `fa9b8c4` 2026-05-08 events→notifications switch, `53b805e` 2026-05-07 V2 marketplace) replaced "whole-snapshot SSE event" with "per-name `mcp_server` notification" and replaced "search-based marketplace tool" with "list-everything marketplace tool". Several stale references survived both transitions.

Severity tally: 1 HIGH / 5 MED / 5 LOW + 3 EDGE.

---

## HIGH

### H1 — `snapshotLocked` is a complete dead function (vestige of integral-snapshot SSE)

- Location: `backend/internal/app/mcp/mcp.go:577-588`
- Claims: godoc says "snapshotLocked builds the SSE event payload from current states. Caller MUST hold s.mu.RLock."
- Reality: zero callers in the entire repo (`grep -rn "snapshotLocked"` returns only the definition itself, twice — function + godoc). Original V1 (commit `5b66f72`) had `publishSnapshot(ctx)` which built the snapshot and pushed `eventsdomain.MCP{Servers: [...]}` SSE event; commit `fa9b8c4` switched to per-server `notif.Publish("mcp_server", name, &snap)` via the new `publishStatus` helper at line 368, but `snapshotLocked` was left orphaned. The "stable-order snapshot" use case is now served by `ListServers` (line 457-466) which is byte-for-byte the same logic.
- Severity: HIGH — exported-style helper that grepping a future maintainer would mistake for live infrastructure; perfect duplicate of `ListServers` body. `staticcheck U1000` would flag it but the function is private.
- Fix: delete the 12-line function and its godoc.
- Risk: zero — provably unreachable.

---

## MED

### M1 — `recordCallResult` godoc lies about publishing SSE on degraded transition

- Location: `backend/internal/app/mcp/calltool.go:209-249` (godoc 209-223)
- Claims: godoc says "if ConsecutiveFailures hits degradedThreshold (3) and current status is ready, transitions to degraded **+ publishes SSE**" and "if status was degraded, transitions back to ready **+ publishes SSE (auto-heal)**".
- Reality: function body lines 224-249 mutates state in-place but **never calls `publishStatus`**. Original V1 (commit `5b66f72`) had a `publish bool` flag set on transitions and a final `if publish { s.publishSnapshot(ctx) }` block. Commit `fa9b8c4` (2026-05-08) deliberately removed the publish call per design decision in `mcp.md` line 388 ("内部状态翻 degraded，不主动 publish——下次 ListServers / Health 端点拉时即时看到") + line 402 ("通知边界：仅 AddServer / RemoveServer / connectOne / Reconnect 等显式生命周期事件 publish；per-call 失败累计触发的 ready→degraded **不**主动推"). The doc and code now agree on the design but the godoc still describes the old behavior. Smell-tell: function signature is `recordCallResult(_ context.Context, ...)` — the unused `ctx` arg is the residue of the old `publishStatus(ctx, ...)` call.
- Severity: MED — future maintainer reads the godoc, expects degraded transition to ship over SSE, builds a dependent feature on that wrong assumption (e.g. adds frontend handler for "auto-degraded" event that never fires).
- Fix: rewrite godoc to match: "Increments TotalCalls; on err: bumps TotalFailures+ConsecutiveFailures, sets LastError; ≥3 consecutive while ready → degraded (in-memory only — frontend sees this on next ListServers / health-check poll, no SSE). On success: clears ConsecutiveFailures, sets LastSuccessAt; degraded→ready auto-heal (also in-memory only). Per mcp.md §5.6 通知边界." Then remove the unused `ctx` arg from the signature.
- Risk: minor refactor; one call site at calltool.go:78 to update.

### M2 — `RegistryEntry.DefaultTimeoutSec` field doesn't exist but 5 docstrings claim a 3-tier timeout precedence using it

- Locations:
  - `backend/internal/app/mcp/mcp.go:9-10, 24-25, 50, 53` (Service package doc + `defaultCallTimeout` doc)
  - `backend/internal/app/mcp/calltool.go:40, 46, 253-255` (CallTool doc + `resolveCallTimeout` doc)
  - `backend/internal/domain/mcp/mcp.go:82` (ServerConfig doc)
- Claims: §5.7 precedence is `ServerConfig.TimeoutSec > RegistryEntry.DefaultTimeoutSec > 30s default` ("用户配置 > Registry 默认 > 全局 30s 兜底").
- Reality: `RegistryEntry` struct (`backend/internal/domain/mcp/registry.go:30-73`) has Name/Description/Homepage/Runtime/Version/InstallCmd/RequiredEnv/RequiredArgs/Category/Tier/Notes — **no `DefaultTimeoutSec` field**. Curated entries in `curated_registry.go` set no such field. `resolveCallTimeout` body (calltool.go:256-261) is just two-tier: `cfg.TimeoutSec > 0` else `defaultCallTimeout`. The 3-tier precedence is fiction. V1 schema (commit `5b66f72`) had `DefaultTimeoutSec` on the in-code Registry struct; V2/V3 marketplace transitions (commits `53b805e`, `ede772f`) dropped it but left every comment site untouched.
- Severity: MED — five mutually-reinforcing comments + a Chinese phrase "在 install 时已从 RegistryEntry.DefaultTimeoutSec 复制过来" that is also fabricated (install.go:142-148 builds the ServerConfig with no TimeoutSec field copy at all, so the per-entry default is implicitly 30s for every curated server).
- Fix: collapse all five sites to two-tier precedence ("ServerConfig.TimeoutSec > 30s default"), drop the "RegistryEntry.DefaultTimeoutSec" mentions everywhere. Update mcp.md §5.7 if it also mentions it.
- Risk: maintainer adds a `DefaultTimeoutSec` field to one entry expecting it to take effect, finds nothing changes, debugs through resolveCallTimeout.

### M3 — `ErrHandshakeFailed` declared + errmap-registered + LLM-tool switch-cased — zero producers

- Locations:
  - Declaration: `backend/internal/domain/mcp/registry.go:182-189`
  - errmap entry: `backend/internal/transport/httpapi/response/errmap.go:173`
  - Consumer (Is-check only): `backend/internal/app/tool/mcp/install_server.go:155-156`
- Claims: "Server installed successfully but failed the MCP initialize handshake. Caller can still retry connection later via Reconnect; the server stays in the registry with status=failed."
- Reality: `grep -rn "ErrHandshakeFailed"` returns 5 hits — declaration + errmap + 2 godoc lines + the install_server.go switch case. **No `fmt.Errorf("...%w", ErrHandshakeFailed)` and no `return ErrHandshakeFailed` exists anywhere in the codebase.** The handshake-failure path in `connectOne` (mcp.go:406-419) returns `fmt.Errorf("mcpapp.connectOne: initialize: %w", err)` where the wrapped err comes from `client.Initialize` which itself returns `ErrServerNotConnected` (client.go:171). So `InstallFromRegistry`'s sandbox-then-AddServer-then-connect chain never produces `ErrHandshakeFailed`; the install_server.go `case errors.Is(err, ErrHandshakeFailed)` arm is dead.
- Severity: MED — the LLM tool has a code-branch that's unreachable; the errmap row reserves status=502 + `MCP_HANDSHAKE_FAILED` for an HTTP path that never errors with this sentinel; the design intent (distinguish "subprocess started but won't speak MCP" from "subprocess didn't start") is unimplemented.
- Fix: option A — actually wire it: in `connectOne`, when `client.Initialize` returns an error AND the underlying cmd actually started (i.e. process spawned + we got past `cmd.Start`-implicit), wrap with `ErrHandshakeFailed` instead of letting `ErrServerNotConnected` flow through. Option B (cheaper) — delete the sentinel + errmap row + install_server.go switch arm and document handshake failures collapse to `ErrServerNotConnected` (which is already the case).
- Risk: option B aligns code with reality, ~6 LOC delete; option A delivers the originally-promised distinction, ~15 LOC across 2 files.

### M4 — `ErrMarketplaceUnavailable` declared + errmap-registered + 2 LLM-tool checks — zero producers (post-V3-curation)

- Locations:
  - Declaration: `backend/internal/domain/mcp/registry.go:149-158`
  - errmap entry: `backend/internal/transport/httpapi/response/errmap.go:170`
  - Consumers (Is-check only): `backend/internal/app/tool/mcp/list_marketplace.go:77-79` + `backend/internal/app/tool/mcp/install_server.go:124-127`
- Claims: "Registry source could not fetch the catalog (network down, API error, etc.). UI / LLM should advise the user to check connectivity or configure a BYOK search key as fallback."
- Reality: only one production `RegistrySource` exists — `CuratedRegistrySource` in `infra/mcp/curated_registry.go`. Its `List` (lines 73-79) returns `(out, nil)` unconditionally; its `Get` (lines 85-94) returns either the entry or `ErrRegistryEntryNotFound`. Neither path can return `ErrMarketplaceUnavailable`. The sentinel is V2 residue: V2 fetched from `registry.modelcontextprotocol.io` over HTTP and the network could fail; V3 (commit `ede781f` 2026-05-09) collapsed to in-memory hardcoded curated catalog where "fetch" is always synchronous + always succeeds. Test fixture (`test/harness/test_registry.go`) likewise can't error.
- Severity: MED — both LLM tools (list_mcp_marketplace, install_mcp_server) carry a "marketplace unavailable, suggest BYOK fallback" branch that's unreachable. The advice is also misleading post-V3 since BYOK search is a different system entirely (web search providers, not MCP).
- Fix: drop the sentinel + errmap row + 2 consumer arms (~25 LOC across 4 files). If a future `OfficialRegistrySource` (HTTP-fetch) is reintroduced for a "show all registered MCPs, not just curated" feature, restore from git.
- Risk: aligns code with V3 design intent ("curated only — small enough to list, hand-verified"). No user-visible behavior change.

### M5 — `ServerStatus.PID` field declared, never written, never read

- Location: `backend/internal/domain/mcp/mcp.go:129` (`PID int \`json:"pid,omitempty"\``)
- Claims: implicit — field name + json tag suggest "current subprocess PID for the user/UI to see in the MCP servers panel".
- Reality: zero producers anywhere in mcp domain. `connectOne` (mcp.go:406-447) never sets it. `infra/mcp/client.go` keeps `cmd *exec.Cmd` but never reads `cmd.Process.Pid` to surface upward. The field always serializes as omitted (zero int). Frontend / testend MCP tab can't display anything because there's no producer. The SDK's `mcpsdk.CommandTransport` actually owns the `cmd` lifecycle once `Connect` is called, but the wrapper at `client.go:153` could capture `cmd.Process.Pid` after start.
- Severity: MED — declared API surface that consumers might assume works (mcp.md §5.6 mentions `pid` as one of the ServerStatus fields). Either implement or remove.
- Fix: option A — write it: in `Initialize` after `sdkClient.Connect` succeeds, set `c.cmd.Process.Pid` and surface via a new accessor or by making `Client.PID()` part of the interface; in `connectOne` after success at line 444, read it and stamp `state.PID`. Option B — remove the field + json tag + mcp.md mentions.
- Risk: option A is ~10 LOC across 2 files; option B is 1 LOC delete + 1 doc edit.

---

## LOW

### L1 — Stale "search_mcp_marketplace" name in 3 source comments + 1 progress event tag (V3 rename incomplete)

- Locations:
  - Progress block tag: `backend/internal/app/mcp/calltool.go:113` (`"tool": "search_mcp_marketplace"`)
  - Comments: calltool.go:106, 110, 142, 147 (4 godoc/comment lines mention "search_mcp_marketplace")
  - Tool doc: `backend/internal/app/tool/mcp/install_server.go:61, 68, 122` (3 LLM-facing description strings)
- Claims: implicit — tool is named `search_mcp_marketplace`.
- Reality: V3 rename (commit `ede781f` 2026-05-09) renamed the tool to `list_mcp_marketplace` (registered name in list_marketplace.go:52). The rerank progress block at calltool.go:111-113 fires inside `Service.Search` which is called from `SearchMCP.Execute` (tool name `search_mcp_tools`) — the tag value `"tool": "search_mcp_marketplace"` is wrong on **both** counts (wrong old name AND wrong tool family — Service.Search is the connected-server tool search, never the marketplace search). The 4 calltool comment lines reference the same wrong/dead name. The 3 install_server.go LLM-facing strings tell the LLM "Pick from search_mcp_marketplace results" but no such tool is registered; LLM has to guess the right tool is `list_mcp_marketplace`.
- Severity: LOW — frontend that reads progress block `attrs.tool` for UI labeling shows the wrong tool name; LLM follows install_server.go description and tries to invoke a non-existent tool, has to retry with the correct name.
- Fix: in calltool.go:113, change tag to `"tool": "search_mcp_tools"` (the real caller). Remove or update the 4 comment mentions (they're stale narrative). In install_server.go:61, 68, 122, replace "search_mcp_marketplace" with "list_mcp_marketplace".
- Risk: ~7 line edits across 2 files.

### L2 — `var _ mcpinfra.MergeResult` keep-alive at end of mcp.go HTTP handler is genuinely dead

- Location: `backend/internal/transport/httpapi/handlers/mcp.go:469`
- Claims: comment says "Compile-time keep-alive — silences unused-import lint when refactors trim the file."
- Reality: `mcpinfra` is already used implicitly by `h.svc.Import(...)` at line 367 — `Service.Import` returns `mcpinfra.MergeResult` (signature in `app/mcp/install.go:175`). Go's import-tracking counts that as a use. Confirmed by removing the keep-alive line and noting compile would still succeed (handler infers `res` as `mcpinfra.MergeResult`). The comment is also misleading — no refactor has trimmed `mcpinfra` use; the import is held by the live `Service.Import` call site.
- Severity: LOW — single dead line + misleading comment + cargo-cult pattern that may spread to other handlers.
- Fix: delete line 469 + the 4-line bilingual godoc above it. Verify `go build ./...` still passes.
- Risk: zero — Go compiler enforces the invariant.

### L3 — Service.Start godoc mentions "publish one final event after all connects settle" but no batch publish exists

- Location: `backend/internal/app/mcp/mcp.go:202-204`
- Claims: "Parallel-connect; collect names so the snapshot publish is one final event after all connects settle. 并发连；收集 name，所有连完后发一次最终快照。"
- Reality: code at lines 205-217 does parallel-connect via wg, but **never publishes anything aggregate**. Each `connectOne` call publishes its own per-server `mcp_server` notification (mcp.go:417, 433, 446). After `wg.Wait()` returns, Start just `return nil`. So there's no "one final event" — there are N per-server events. This is V1 commentary surviving the V3 notif transition (commit `fa9b8c4` switched from `events.MCP{Servers: [...]}` whole-snapshot to per-server `notif.Publish`). The phrase "collect names" doesn't even match the goroutine code which iterates the configs map directly.
- Severity: LOW — 2-line comment-only mismatch; behavior is correct (per-server events match the `mcp_server` notification protocol).
- Fix: rewrite the comment: "Parallel-connect every server; each connectOne publishes its own mcp_server notification on completion. wg ensures Start returns only after every connect (success or failure) has settled."
- Risk: trivial.

### L4 — `RegistryEntry.Version` field declared, never produced, never consumed

- Location: `backend/internal/domain/mcp/registry.go:45` (`Version string \`json:"version,omitempty"\` // pinned version; empty means "latest"`)
- Claims: "pinned version; empty means latest".
- Reality: zero curated entries set it (`grep "Version:" curated_registry.go` returns nothing). No code path reads `entry.Version`. `InstallCmd.Args` carries version pins inline (e.g. `"@playwright/mcp@latest"`, `"chrome-devtools-mcp@latest"`), bypassing the field entirely. The Version-empty-means-latest semantics is doubly fictional: not only does no consumer interpret an empty field as "latest", but the install command's `@latest` suffix is doing all the actual version resolution.
- Severity: LOW — declared API surface that's always empty; misleading "pinned version" hint when versions are baked into InstallCmd.Args strings.
- Fix: remove the field. If a future "user can pin to a specific MCP version" feature lands, reintroduce with an actual producer + consumer.
- Risk: 1-line delete + check no JSON contract test asserts the field is present.

### L5 — `ListTools` godoc claims "call_mcp's catalog presentation" as second consumer

- Location: `backend/internal/app/mcp/mcp.go:502-505`
- Claims: "Used by Search when total tool count <= topK (skip ranking) and by call_mcp's catalog presentation."
- Reality: only consumer of `Service.ListTools` is `Service.Search` at calltool.go:95. `call_mcp_tool` (app/tool/mcp/call.go) calls `Service.CallTool(ctx, server, tool, args)` directly with named server+tool — never lists/enumerates anything. Likely "catalog presentation" referred to a planned UI panel that didn't ship.
- Severity: LOW — 1-line comment-only mismatch.
- Fix: drop the "and by call_mcp's catalog presentation" phrase from both English + Chinese godoc.
- Risk: trivial.

---

## EDGE

### E1 — AddServer publishes "disconnected" status that's immediately overwritten by connectOne's "ready"/"failed"

- Location: `backend/internal/app/mcp/mcp.go:291` (`s.publishStatus(ctx, cfg.Name)` after writing config) → `mcp.go:295` (calls connectOne which itself publishes).
- Observation: between AddServer's publish at 291 (status=disconnected) and connectOne's publish (status=ready or failed), the UI receives `disconnected → ready` (or `disconnected → failed`). The intermediate `connecting` state set at connectOne:402 is NOT published. So the disconnected publish exists for the brief window users see "Connecting..." in the UI before the connect resolves.
- Severity: EDGE — intentional UX ("user clicked install, show 'connecting' before result arrives") but the implementation accomplishes it by publishing `disconnected` then leaving the user to interpret `connecting` only on the next poll. If frontend shows `disconnected` literally, this is a visible bug. If frontend shows "Connecting..." for any non-ready state, this is harmless.
- Fix: replace the publishStatus(disconnected) call at 291 with a publish of explicit `connecting` status, OR add an explicit publishStatus call right after `state.Status = StatusConnecting` at connectOne:402. The current "publish disconnected, then connect" is a half-finished UX pattern.
- Risk: needs frontend audit before changing — testend MCP tab might literally render "disconnected" momentarily; current state is "ugly but not broken".

### E2 — Reconnect mutates state to "disconnected" + resets counters but doesn't publish before connectOne

- Location: `backend/internal/app/mcp/mcp.go:335-361`
- Observation: at lines 348-352 Reconnect sets `state.Status = StatusDisconnected; state.LastError = ""; state.ConsecutiveFailures = 0` then drops the lock at 353. Connect runs at 357. Frontend doesn't see the disconnected+reset state; only the final ready/failed publish from connectOne. Asymmetric with AddServer (which DOES publish the disconnected intermediate). If the user clicks "Reconnect" on a degraded server, the UI keeps showing "degraded" until connectOne resolves, even though the server is actually mid-restart.
- Severity: EDGE — could be intentional (don't flicker the UI through "disconnected" on a transient action) but is asymmetric with AddServer and undocumented.
- Fix: either add `s.publishStatus(ctx, name)` after line 353 to surface the reset, or document the asymmetry in the godoc.
- Risk: behavioral change visible to UI; coordinate with frontend.

### E3 — `Service.Start` always returns nil but signature carries error

- Location: `backend/internal/app/mcp/mcp.go:180-220`
- Observation: every error path (corrupt mcp.json at line 182, per-server connect failure at line 212) is logged and swallowed. The signature `func (s *Service) Start(ctx context.Context) error` plus the caller pattern at `cmd/server/main.go:326` (`if err := mcpService.Start(...); err != nil { log.Warn(...) }`) suggests partial-failure reporting was once planned. Since corrupt-json + per-server failures are intentionally non-fatal (mcp.md §5.7 末段), the function effectively cannot fail.
- Severity: EDGE — cosmetic; caller's `if err != nil` branch is dead code, but the dead-code consequence is "log nothing" which is harmless.
- Fix: option A — change signature to `func (s *Service) Start(ctx context.Context)` (no error return) and remove the dead caller branch in main.go. Option B — leave it for forward compatibility (e.g. if Start ever wants to fail-loud on a fundamentally broken setup).
- Risk: option A is a minor signature change. Currently low priority.

---

## Cross-cutting observations

1. **The whole-snapshot → per-name notification migration left more residue than typical refactors.** H1 (snapshotLocked function), L3 (Start aggregate-publish comment), and `ServerStatus` godoc at `domain/mcp/mcp.go:118-125` ("the SSE event family ships the whole snapshot so the UI replaces local state on every change") all describe the deleted V1 protocol. Domain-level godoc lying is especially dangerous since domain/mcp.md design doc readers might cross-reference the godoc.

2. **V3 marketplace transition (search → list) was incomplete in tool descriptions.** L1 (search_mcp_marketplace references) is the most user-visible — LLMs follow install_server.go's description that says "Pick from search_mcp_marketplace results" and waste a turn discovering the actual tool name `list_mcp_marketplace`. The progress event tag `"tool": "search_mcp_marketplace"` is also doubly wrong (rename + wrong tool family).

3. **3 sentinels declared without producers (M3 ErrHandshakeFailed, M4 ErrMarketplaceUnavailable; PID is a field, not a sentinel).** Per §S17 every sentinel must be in errmap; the inverse should also be true — every errmap entry should have a producer somewhere reachable in production. The 2 dead sentinels suggest the errmap audit didn't cross-check producers.

4. **Curated catalog has 21 entries (matches audit prompt expectation).** All 21 produce valid InstallCmd.Args and Tier values; no entry is a "0-test" placeholder. Quality is high — every entry has a Notes field for first-run gotchas.

5. **No subprocess auto-restart code anywhere — design intent (`mcp.md §3` "no auto-restart") fully honored.** This is a positive finding: where many subprocess managers grow restart loops over time, mcp held the line.

6. **5 system tools wired correctly into LLM registry.** `cmd/server/main.go:329` `tools = append(tools, mcptool.MCPTools(mcpService)...)` registers all 5 (search_mcp_tools, call_mcp_tool, list_mcp_marketplace, install_mcp_server, uninstall_mcp_server). Each has 9-method `toolapp.Tool` interface satisfied. Names match registered names (no rename drift inside the tool layer).

7. **Two-path import (multipart + JSON) at handlers/mcp.go:269-373 is clean.** Both paths converge on the same `mcpinfra.Merge` call; no vestigial fields in either parsing branch. Audit prompt question 5 negative.

8. **InstallFromRegistry has no progress reporting dead-shell.** The installprogress helper at install.go:107-115 is the live wire to sandboxapp; no orphan "report progress to nil channel" branches survive. Audit prompt question 6 negative.
