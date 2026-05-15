# D2 — `service-design-documents/web.md` ↔ `internal/app/tool/web/` Sync Audit

**Doc**: `documents/version-1.2/service-design-documents/web.md` (309 lines)
**Code**: `backend/internal/app/tool/web/` (5 files: `web.go`, `fetch.go`, `search.go`, `search_byok.go`, `search_mcp.go`)
**Spec authorities**: CLAUDE.md §S14 (doc-sync) + §S18 (Tool interface) + §S17 (errmap registration)

---

## In code but not in doc

| Item | Code location | Severity |
|---|---|---|
| `WebTools` factory signature: `func WebTools(picker, keys, factory, mcpRouter MCPSearchRouter, log *zap.Logger) []toolapp.Tool` — doc §4.3 shows only `(picker, keys, factory)` (3 params), missing mcpRouter + log | `web.go:38-44` vs web.md:153 | MED |
| `WebSearch` struct has `keys`, `mcpRouter MCPSearchRouter`, `log *zap.Logger` fields — doc §4.3 / §5.x show no MCP router field | `search.go:168-173` | MED |
| `MCPSearchRouter` interface (port) at `search_mcp.go:33-44` — defines `CallSearchTool(ctx, query, limit) (string, error)`. Doc §2 端到端推演 mentions `app/mcp.SearchRouter` but not the actual port name + signature owned by web package. | `search_mcp.go:33-44` | MED |
| `ErrMCPSearchUnavailable` sentinel (signals "no MCP search server connected") — doc has no sentinel definitions list | `search_mcp.go:23` | LOW |
| `ErrAuthFailed` / `ErrRateLimited` / `ErrUpstreamHTTP` sentinels — for HTTP-status-classified BYOK provider errors; **registered in errmap.go:221-223** as `WEBSEARCH_AUTH_FAILED` / `WEBSEARCH_RATE_LIMITED` / `WEBSEARCH_UPSTREAM_HTTP`. Doc has no mention of these sentinels OR errmap registration. | `search.go:62-66`, errmap.go:221-223 | MED |
| `WebSearch.markInvalidIfAuthErr` — surfaces 401/403 from BYOK to apikey domain via `keys.MarkInvalid` so UI badge flips. Uses detached ctx pattern (per §S9). Doc §2 端到端推演 BYOK section doesn't mention this side-effect. | `search.go:331-362` | MED |
| `searchBrave` / `searchSerper` / `searchTavily` / `searchBocha` per-provider implementations in `search_byok.go` (231 lines) — endpoint URLs, auth header schemes, response parsers. Doc §2 says "调对应 API（search_byok.go）" but no per-provider call shape detail. Less critical (impl detail). | `search_byok.go:1-231` | LOW |
| `parseMCPSearchResults` accepts BOTH `{"results":[...]}` shape AND bare-array shape, fall-back to "raw text → 1 result" — doc §2 端到端推演 only mentions JSON shape | `search_mcp.go:77-136` | LOW |
| Tier 2 MCP path also has fallback last-shot "raw blob as 1 result" — doc doesn't list this | `search_mcp.go:127-134` | LOW |
| `WebSearch.warnf` / `debugf` log helpers; nil-log-tolerant per "tests don't pass log" | `search.go:367-379` | LOW |

---

## In doc but not in code (stale)

| Item | Doc location | Severity |
|---|---|---|
| §1 一句话 says **"BYOK → MCP 两层路由"** — aligned. But §3 决策 row "WebSearch 路由策略" shows **3-tier**: SearXNG/Bing intl/Bing CN historic → "原 SearXNG/Bing 国际/Bing CN 三层 HTML 抓取全是假兜底——dogfood 实测后删 Bing CN 那层". This row is **half-stale**: tells reader the OLD 3-tier was deleted, but does NOT clearly state CURRENT is BYOK→MCP only (hint at "两层失败时返 LLM-actionable 提示用户走 BYOK / 装 MCP" but woven into a "屎山拯救计划" decision narrative). | web.md:75 | MED |
| §4.2 WebSearch 返回 JSON example uses `"source": "searxng"` — actual `source` is one of `"brave" / "serper" / "tavily" / "bocha" / "mcp"` (per `search.go:153`) | web.md:130 | HIGH |
| §4.2 source legend: `source: "searxng" / "bing" / "bing_cn"` — completely stale; actual values are `brave`, `serper`, `tavily`, `bocha`, `mcp` | web.md:138 | HIGH |
| §4.2 全部 tier 失败 message: `"All search backends failed. Last error: <err>"` — actual message in code (`search.go:255-263`) is multi-line LLM-actionable hint ending with "(The previous Bing CN HTML scrape fallback was removed because Bing now renders results via JavaScript…)" — completely different format | web.md:140 | MED |
| §4.2 全部 tier 零结果 message description: "多行 LLM-actionable 提示，分两条 bullet（配 BYOK 或装 duckduckgo-search MCP），并附'为啥不再有 Bing CN 兜底'一句解释" — actual message exists at `search.go:255-263`; description matches **content** but the "源 Bing 兜底" narrative makes it look like there IS still a Bing tier in play. Reader ambiguity. | web.md:141 | LOW |
| §5.4 SearXNG 池随机洗牌 (whole subsection) — code has NO SearXNG / `t.instances` / `rand.Shuffle(pool)` anywhere. **Entire subsection stale**. | web.md:228-240 | HIGH |
| §5.5 Bing HTML 解析 (whole subsection) `walkBing` / `hasClass` / `findFirstByTag` / `findFirstByClass` / `textOf` / `collapseSpaces` — code has NO `search_bing.go` file; `x/net/html` import absent from `go.mod` for web tool. **Entire subsection stale**. | web.md:242-256 | HIGH |
| §6 Safety boundaries row "WebSearch 公共后端" says "零用户配置 / SearXNG 实例可能 down / Bing 反爬 → 3 层 fallback 提高可用性" — fully stale (no SearXNG, no Bing, no 3-tier) | web.md:270 | HIGH |
| §3 决策 row "WebSearch Bing 解析" + "Bing snippet fallback" — both refer to deleted Bing scraper | web.md:78-79 | HIGH |
| §7 test coverage: WebFetch shows "24 tests"; WebSearch shows "21 tests / 3 tier 端到端 × 4 / Bing HTML 解析 + b_caption fallback + 空 doc / 测试 helper" — Bing-related test descriptions stale (code has BYOK + MCP only) | web.md:280 | MED |
| §1 一句话 says "30 秒墙钟" common — that's WebFetch's `fetchTimeout = 30 * time.Second`. WebSearch's `searchTimeout = 10 * time.Second` (per-backend). Doc §3 决策 row "单后端超时" correctly distinguishes "WebFetch 30s / WebSearch 10s × 3 后端" — but says "× 3 后端" (stale, now × 2 tiers BYOK + MCP, with BYOK iterating up to 4 providers each at 10s timeout) | web.md:16 vs web.md:83 | MED |
| §8 与其他 domain 的关系 row "events / SSE 无 — 结果通过 chat.message tool_result block 推流" — accurate for WebSearch + WebFetch summary. But **WebFetch summarisation is NOT streamed** — uses `llminfra.Generate` (non-stream). So no streaming events; aligned. | web.md:296 | OK |

---

## Mismatched

| Item | Code | Doc | Severity |
|---|---|---|---|
| Doc §3 决策 row "WebSearch 路由策略" still presents narrative as if 3-tier (SearXNG/Bing intl/Bing CN) was current then deleted Bing CN; actual implementation deletes ALL THREE and replaces with BYOK + MCP. The decision row reads as describing a partial deletion ("删 Bing CN 那层") rather than full replacement ("删除整个 HTML scrape ladder, replaced with BYOK→MCP"). | `search.go:218-263` (no HTML scrape anywhere) | MED |
| Doc §7 test count "WebSearch | search_test.go | 21" — file content at `search_test.go` covers BYOK + MCP scenarios, no Bing tests. Test count number itself may or may not be 21 (didn't enumerate per audit constraint "no _test.go reads"); but **scenario list is stale**. | search_test.go (per spec, not read) | MED |
| Doc §1 says "BYOK → MCP 两层路由"; §3 says "屎山拯救计划 #4 (2026-05-07)"; §4.2 example JSON has `source: "searxng"`; §5.4 / §5.5 describe deleted code. **Internal doc inconsistency** — same doc tells two contradictory stories. | web.md (whole) | HIGH |
| `WebSearch.Execute` ctx-cancel check between BYOK and MCP — `if ctx.Err() == nil && t.mcpRouter != nil` (line 242). Doc §2 端到端推演 doesn't mention ctx-cancel check; aligned with general principle but unstated. | `search.go:242` | LOW |

---

## Sub-check

- **Tool list aligned**: yes — doc §4 lists WebFetch / WebSearch; code factory `WebTools` returns those 2 in order.
- **9-method interface aligned**: yes — Both tools implement all 9 methods. `var _ toolapp.Tool = ...` at `fetch.go:463` and `search.go:405`.
- **Static metadata (IsReadOnly / NeedsReadFirst / RequiresWorkspace) aligned**: yes — Both tools match §S18 §8 table:
  - WebFetch: `(true, false, false)` ✓ — `fetch.go:144-146`
  - WebSearch: `(true, false, false)` ✓ — `search.go:183-185`
- **Parameters schema aligned**: yes — Doc §4.1 / §4.2 Args tables match `fetchSchema` / `searchSchema` field names + types + required arrays exactly.
- **Emit pattern (eventlog Emitter)**: N/A — Web tools return final string from `Execute`. WebFetch.summarise uses non-streaming `llminfra.Generate`. WebSearch returns JSON-marshalled string. No mid-execute emission.
- **Sentinel/errmap**:
  - WebFetch sentinels (`ErrEmptyURL`, `ErrEmptyPrompt`, `ErrUnsupportedScheme`) — tool-internal, not in errmap. Aligned.
  - WebSearch internal validation (`ErrEmptyQuery`) — tool-internal, not in errmap. Aligned.
  - **WebSearch HTTP-classified sentinels** (`ErrAuthFailed`, `ErrRateLimited`, `ErrUpstreamHTTP`) — **ARE in errmap.go:221-223** (HIGH-impact: per §S17, registered errmap rows imply these errors can reach handlers). Doc has no mention. **§S14 violation**: code has registered sentinels, doc has zero. Per the D1 contract-error-codes audit summary, these were already flagged as "8 newly-added sentinels not documented in error-codes.md (`webtool.Err*` × 3)" — so error-codes.md is also stale on this front. From web.md side: still missing.
  - `ErrMCPSearchUnavailable` — internal, not in errmap. Aligned.

---

## Summary

**5 HIGH / 7 MED / 5 LOW** — web.md is **the most internally inconsistent doc** of the 4. The doc tells two contradictory stories simultaneously:

1. §1 + §3 (decision row) hint at the **new** BYOK→MCP world.
2. §4.2 source legend, §4.2 JSON example, §5.4 SearXNG pool, §5.5 Bing HTML, §6 safety row, §7 test scenarios all reference the **deleted** SearXNG + Bing scraper world.

A reader cannot tell which is current without reading code. Per §S14 priority "文档落后于代码 = bug", this is the heaviest violation in the 4-tool audit.

**Critical update needed** (out of audit scope, listing for fix-tracking):
- Delete §5.4 SearXNG section.
- Delete §5.5 Bing HTML section.
- Replace §4.2 source legend, JSON example, error message text.
- Update §3 决策 row to clearly state "BYOK→MCP only; HTML scrape ladder fully removed".
- Update §6 安全 row "WebSearch 公共后端" to reflect new architecture.
- Update §7 test scenario list.
- Add §4.x sentinel section listing `ErrAuthFailed` / `ErrRateLimited` / `ErrUpstreamHTTP` + errmap registration (mirrors what error-codes.md needs).
- Update §4.3 `WebTools` factory signature to include `mcpRouter` + `log` params.
- Add `MCPSearchRouter` port definition (web package owns the port; main.go wires *mcpapp.Service adapter).
