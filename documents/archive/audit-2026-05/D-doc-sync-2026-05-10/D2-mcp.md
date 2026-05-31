# D2 — mcp.md ↔ code gap report

Audited `documents/version-1.2/service-design-documents/mcp.md` against `internal/{domain,app,infra/mcp,infra/store/mcp}/mcp/` + `transport/httpapi/handlers/mcp.go`.

D1 already covered the `mcp_server` notification and the `GET /api/v1/mcp-servers/{name}/stderr` route gap; this report focuses on design-doc-vs-code drift specific to `mcp.md`.

---

## In code but not in doc

| Item | Code location | Severity |
|---|---|---|
| `mcpdomain.ErrMarketplaceUnavailable` sentinel | `internal/domain/mcp/registry.go:158` | MED |
| `mcpdomain.ErrAlreadyInstalled` sentinel (used by Service.InstallFromRegistry collision check) | `internal/domain/mcp/registry.go:166` | MED |
| `mcpdomain.ErrUnsupportedRuntime` sentinel | `internal/domain/mcp/registry.go:175` | MED |
| `mcpdomain.ErrHandshakeFailed` sentinel | `internal/domain/mcp/registry.go:189` | MED |
| 5 system tools (doc §8 only describes 2 — `search_mcp` / `call_mcp`); code has `search_mcp_tools` / `call_mcp_tool` / `list_mcp_marketplace` / `install_mcp_server` / `uninstall_mcp_server` | `internal/app/tool/mcp/{search,call,list_marketplace,install_server,uninstall_server}.go` | HIGH |
| `mcpapp.SearchRouter` (port satisfier for app/tool/web's WebSearch routing to duckduckgo MCP) | `internal/app/mcp/searchrouter.go` | MED |
| `mcpapp.Service.Stderr(name)` method (used by `GET /mcp-servers/{name}/stderr`) | `internal/app/mcp/mcp.go:489` | MED |
| `Client.StderrTail()` API (256 KB ring buffer) on stdio Client interface | `internal/infra/mcp/client.go` (per `mcp.go:499`) | MED |
| `Service.Import(ctx, incoming, overwrite)` method exposing `MergeResult` | `internal/app/mcp/install.go:175` | MED |
| `mcpinfra.Merge` + `MergeResult` infra helpers | `internal/infra/mcp/config.go` | LOW |
| `installprogresspkg.Run` integration: sandbox install progress emits eventlog progress block under install_mcp_server tool_call (vs doc §10 still saying "free-text install progress goes through chat.message tool_call") | `internal/app/mcp/install.go:107` | MED |
| Service.publishStatus + RemoveServer notification publish (`mcp_server` type) | `internal/app/mcp/mcp.go:368,326` | LOW (D1 covered) |
| `defaultCallTimeout` = 30s + `addServerTimeout` = 3min + `initializeTimeout` = 30s constants (doc §5.7 mentions only the 30s default) | `internal/app/mcp/mcp.go:54-80` | LOW |
| `degradedThreshold = 3` constant | `internal/app/mcp/mcp.go:61` | LOW |
| `Service.SetClientFactory` test injection point | `internal/app/mcp/mcp.go:166` | LOW |
| `notificationspkg.Publisher` field — V3 uses notifications, not events bridge | `internal/app/mcp/mcp.go:105` | LOW |

## In doc but not in code (stale)

| Item | Doc location | Severity |
|---|---|---|
| §3 "事件 bridge" claim — code uses `notificationspkg.Publisher` not `eventsdomain.Bridge`. Doc §6 Service field still shows `bridge eventsdomain.Bridge`. | mcp.md:503 | MED |
| §6 Service `recordCallResult` example uses `s.bridge.Publish(ctx, "", eventsdomain.MCP{...})` | mcp.md:355,363 vs `app/mcp/calltool.go:224` (no bridge; counters update only — no publish on degraded transition) | HIGH (doc claims SSE on degraded transition; code does not publish anymore on degraded transition — only on AddServer/RemoveServer/connectOne) |
| §7 Client interface signature shown without `StderrTail()` method | mcp.md:555-560 | MED |
| §8 Tool names `search_mcp` / `call_mcp` (no `_tools` / `_tool` suffix; missing 3 tools) | mcp.md:574-628 | HIGH |
| §9 SSE 事件 — doc shows `eventsdomain.MCP` struct + `EventName() string` + "全 server 状态快照"; code uses notifications package with type `mcp_server` and per-name single ServerStatus payloads (not full snapshot) | mcp.md:632-646 | HIGH |
| §10 HTTP API table missing `GET /api/v1/mcp-servers/{name}/stderr` | mcp.md:653-661 (D1 covered, not duplicating) | — |
| §10 `:enable` / `:disable` text says they don't exist — confirmed correct | mcp.md:679 | — |
| §12 CatalogSource example shows `EventTopics() []string` method on the catalogSource interface; code has only `Name()` / `Granularity()` / `ListItems()` | mcp.md:734 vs `app/mcp/catalogsource.go` + `domain/catalog/source.go:95` | MED |
| §14 与其他 domain 的关系 row "events" — events bridge actually replaced by notifications package + eventlog Emitter | mcp.md:782 | LOW |
| §6 Service struct `llm llmclientpkg.Resolver` claim — code uses 3-tuple `modelPicker / keyProvider / llmFactory` directly (per `app/mcp/mcp.go:101-104`) | mcp.md:505 | LOW |
| §5.5 Tier table claims "google-workspace" + "ms365" curated entries with OAuth UX; need to confirm 21 list match curated_registry.go | (verify against curated_registry.go) | — |

## Mismatched

| Item | Code | Doc | Severity |
|---|---|---|---|
| Sentinel count | 14 (10 in mcp.go + 4 in registry.go) | 10 listed in §4 + §11 | MED |
| §11 errmap table has 10 sentinels | errmap registers 14 | mcp.md:710-720 vs `errmap.go:160-176` | MED |
| §3 originally said "stdio only V1 + bridge events" — both incorrect: V3 now uses notifications package | matches code  | mcp.md:84,89 | LOW |
| §6 Service has `Search` returning `[]mcpdomain.ToolDef` directly; matches code | matches | — |
| §5.5 RegistrySource interface: doc shows `List(ctx)` + `Get(ctx, name)` | matches | mcp.md:280-283 vs `domain/mcp/registry.go:130-145` | — |
| §5.5 RegistryEntry struct: doc shows fields including TimeoutSec implication; code has no DefaultTimeoutSec on RegistryEntry — only ServerConfig.TimeoutSec | mcp.md:232-244 (no DefaultTimeoutSec field shown) vs `registry.go:30-72` (no DefaultTimeoutSec field) | LOW (consistent — but §5.7 precedence text mentions RegistryEntry.DefaultTimeoutSec which doesn't exist) |
| §5.7 Per-server timeout precedence chain says "RegistryEntry.DefaultTimeoutSec" exists | code has no such field; resolveCallTimeout only checks ServerConfig.TimeoutSec | mcp.md:441,448-453 vs `app/mcp/calltool.go:256-261` | MED |
| §10 `:install` on registry path returns 201 — matches | matches | — |
| §10 `:install` body says `{env, args}` only — but code phase1Envelope flow described in §5.5 mentions `confirmed: true`; HTTP path seems to skip phase1 | mcp.md:670-676 vs `transport/httpapi/handlers/mcp.go:425-462` | LOW |
| §5.5 phase1 envelope flow: doc describes a "needsConfirmation" two-step flow for `install_mcp_server` LLM tool | code: actual flow in `install_server.go` may differ — would need deeper read | LOW (didn't fully audit tool internals; description-quality concern only) |
| §6 ListServers returns `[]ServerStatus` sorted by name — matches | matches | — |
| §6 CallTool default timeout 30s — matches | matches | — |

## Sub-check
- Entities aligned: yes — ServerConfig / ServerStatus / ToolDef / RegistryEntry / HealthResult all match code 1:1
- Service methods aligned: **partial** — doc §6 lists 14 methods + 1 helper; code has those plus `Stderr` / `SetClientFactory` (test) / `Import` documented separately. `Service.Search` signature matches.
- Endpoints aligned: **almost** — D1 already noted `/stderr` missing from api-design.md; mcp.md §10 also misses it
- Sentinels aligned: **no** — 4 marketplace V3 sentinels (`ErrMarketplaceUnavailable` / `ErrAlreadyInstalled` / `ErrUnsupportedRuntime` / `ErrHandshakeFailed`) not in §4 or §11. errmap has them all wired (`MCP_MARKETPLACE_UNAVAILABLE` / `MCP_ALREADY_INSTALLED` / `MCP_UNSUPPORTED_RUNTIME` / `MCP_HANDSHAKE_FAILED`).
- Cross-domain deps aligned: **partial** — sandbox dep correct (PluginSandbox port via `SandboxInstaller` interface); chat dep via search/call tools correct; catalog dep correct. Events bridge dep (§14) is stale — V3 uses notifications package + eventlog Emitter.
- 端到端推演 valid: **partial** — §2 "运行期 — search" describes ranking through "ranking LLM"; matches code but mentions `forge search 模式 A` which is OK. §2 "运行期 — call" matches. Subprocess lifecycle §2 says "不静默 auto-restart"; matches code (RemoveServer needs explicit Reconnect).
- Phase 5 / V3 / 21-curated 大变更已反映: **partial** — V3 status header ("✅ Marketplace V3 — curated 2026-05-08 / search→list 2026-05-09") + §5.5 (curated marketplace) + §5.5 RegistrySource port are well-documented. But §6 still references `eventsdomain.Bridge`, §8 still describes 2 tools (not 5), §9 still describes `eventsdomain.MCP` snapshot push, §6 `recordCallResult` still publishes "events" — multiple V2-era artifacts left intact.

---

## Summary

- HIGH: 3 (only 2 of 5 system tools in §8; SSE event family `eventsdomain.MCP` × full-snapshot vs code's `mcp_server` × per-name notification — major shape change; recordCallResult bridge.Publish on degraded transition no longer happens)
- MED: 8 (4 V3 sentinels missing from §4/§11; SearchRouter / Stderr / Import / installprogress integration / Bridge→Publisher field type / EventTopics in CatalogSource interface / RegistryEntry.DefaultTimeoutSec doesn't exist + §5.7 chain references it / phase1 confirm flow vague)
- LOW: 6 (timeout constants / SetClientFactory / catalogSource event_topics / install body shape / Resolver vs 3-tuple / events relation)
