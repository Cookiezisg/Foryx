# D-redo audit — api-design.md

**Audited file**: `documents/version-1.2/service-contract-documents/api-design.md` (310 lines)
**Audited code**:
- `backend/internal/transport/httpapi/handlers/*.go` (excluding `_test.go` and forge subpackage)
- `backend/internal/transport/httpapi/router/router.go`
- `backend/internal/transport/httpapi/handlers/dev_routes.go` (manifest self-consistency check)

**Method**: read both end-to-end (no grep-skim), cross-checked each documented endpoint against actual mux registrations + handler logic. grep used only to count HandleFunc calls and verify a few cross-references.

---

## In code but not in doc

| Item | Code location | Severity |
|---|---|---|
| (none) | — | — |

After D1 the previously-missing sandbox routes (~12 entries) + `GET /api/v1/conversations/{id}` + `GET /api/v1/mcp-servers/{name}/stderr` were all added to the doc. Production code surface is fully covered now.

---

## In doc but not in code (stale)

| Item | Doc location | Severity |
|---|---|---|
| `POST /api/v1/forges/{id}:duplicate` — "复制 forge（201；复制 active 版本的 code/deps/python/parameters）" | api-design.md line 144 (forge §) | **HIGH** — endpoint is **not implemented**. `forge.go::postOnForge` switch (lines 173-187) only handles `run`/`export`/`revert`/`test`/`generate-test-cases`. There is NO `case "duplicate"`. `forgeapp.Service` has no `Duplicate` method (verified by listing all `func (s *Service)` methods in `app/forge/forge.go`). Calling this route hits the `default` branch → 404 NOT_FOUND "unknown action: duplicate". |

---

## Semantic drift (grep 抓不到的)

| Endpoint | Doc says | Code does | Severity |
|---|---|---|---|
| `GET /api/v1/conversations/{id}/messages` | line 128: "每条消息含 `blocks[]`（**text/reasoning/tool_call/tool_result/attachment_ref**）+ `inputTokens` + `outputTokens`" | Actual block types per `domain/eventlog/eventlog.go` lines 48-78 are 6: `text` / `reasoning` / `tool_call` / `tool_result` / **`progress`** / **`message`**. `attachment_ref` is **not a block type anywhere in the codebase** (grep'd `domain/`, returns 0 hits). Doc lists 5 with 1 nonexistent; missing 2 real types. | **MED** — wire-format-relevant; UI building block-type rendering off this list will fail to render `progress` (sandbox install logs) and `message` (nested subagent runs) blocks. |
| `PATCH /api/v1/conversations/{id}` | line 117: "改名（200）" | Code accepts `{title?, systemPrompt?}` partial-update body; `conversation.go::Rename` (lines 115-128) actually dispatches both fields to `svc.Update(ctx, id, title, systemPrompt)`. Service updates either or both. Doc undersells the endpoint. | LOW — endpoint works fine; doc just describes a subset of behavior. |
| `POST /api/v1/mcp-registry/{name}:install` | line 230: "安装：填 env + args → 写 mcp.json + Connect" (status code unstated) | Returns **201** Created (mcp.go line 461). | LOW — doc silent on status code is not strictly wrong, but reading it one might guess 200; 201 is only in the actual code. |
| `PUT /api/v1/mcp-servers/{name}` | line 218: "增/改配置（写 mcp.json + 立即 Connect）" (status code unstated) | Returns **200** with ServerStatus regardless of connect success (mcp.go lines 201, 213). Connect failure is logged as ERROR but still returns 200; caller distinguishes via `status` field in the body. | LOW — non-obvious behavior unstated; doc body should mention "返 200 + ServerStatus, status 字段反映 connect 是否成功" since N7 spec wouldn't predict this from "PUT 200" alone. |
| `GET /api/v1/mcp-servers/{name}/stderr` | line 217: "取 server stderr 256KB ring buffer（debug 用）" | Returns `{name, stderr, size}` JSON envelope (mcp.go lines 127-131), not raw stderr text. | LOW — response shape unstated; consumer might expect `text/plain` body. |
| `POST /api/v1/skills/{name}:invoke` | line 247: "手动调用（slash command 路径用）" | Body is `{arguments: string[]}` (positional args); returns 200 `{result: out}`. Doc doesn't describe shape. | LOW — body shape unstated. |
| `GET /api/v1/catalog` | line 254: "当前 catalog cache 内容（debug / UI 显示）" | Returns the cached Catalog **or `null` inside envelope** when no Refresh has produced one yet (catalog.go line 65-67 + Service comment). | LOW — null-vs-object behavior unstated; UI consumers need to handle null. |
| `GET /api/v1/sandbox/envs?ownerKind=...` | line 267: "**ownerKind 必填**，否则 400 OWNER_KIND_REQUIRED" | Matches exactly (sandbox.go lines 104-109). | OK |
| `POST /api/v1/forges/{id}:test` (run-all-tests) | line 158: "运行全部测试" | Returns `{total, passed, failed, results}` aggregated envelope (forge.go lines 247-249); not just a list. | LOW — return shape unstated. |
| `POST /api/v1/forges/{id}:generate-test-cases` | line 159: "LLM 生成测试用例（一次性返回 JSON 批量）" | Accepts optional `?count=N` (1-20, default 5) query param (forge.go lines 257-261). | LOW — query param undocumented. |

---

## Sub-check

- **Endpoints aligned**: yes — every code-registered route has a matching doc row (after D1 closure)
- **Method semantics aligned**: yes — POST/GET/PATCH/PUT/DELETE all match between doc and code
- **Body shape aligned**: partial — several endpoints (mcp `:install`, skills `:invoke`, forge `:test`, forge `:generate-test-cases`) document only the path/purpose, not the body or query-param shape; LOW severity
- **Envelope shape aligned**: yes — uniform `{data}` / `{error}` envelope, paged endpoints all use `paginationpkg.Parse` + `responsehttpapi.Paged`
- **Status code semantics aligned**: yes (200/201/204/400/404/409/410/422 all per N2); one undocumented behavior in PUT mcp-server returning 200 even on connect failure (LOW)

---

## Cross-cutting findings

### Diff vs D1 (D-doc-sync-2026-05-10/contract-api.md)

D1 reported (HIGH):
1. ~~Sandbox 12 routes missing from doc~~ — **FIXED** (Phase 4 preparation section added with all 13 sandbox routes; verified line-by-line at doc lines 266-285)
2. ~~`/api/v1/events` legacy stale~~ — **FIXED** (no longer mentioned in api-design.md; only stale comment remains in `eventlog.go:4-5,17-19` referring to `共存` — out of audit scope)
3. ~~3 `/api/v1/subagent-runs*` documented~~ — **FIXED** (subagent section line 203-206 now says "无独立 HTTP 端点" + redirects to messages endpoint)
4. ~~`/api/v1/subagent-types` ghost~~ — **FIXED** (removed from devRoutes manifest; verified `dev_routes.go` lines 119-127 only have the "no HTTP surface" comment)

D1 reported (MED):
1. ~~`:duplicate` not in doc~~ — **status reversed**: doc now lists `:duplicate` (line 144) but **code never had it** and still doesn't. D1 was wrong to claim code had it (the previous audit cited "forge.go:49 via postOnForge switch" but the switch never had this case; verified by reading all of forge.go end-to-end). **This is now a doc-claims-but-code-doesn't issue, demoted from "not in doc" gap to "stale in doc" — promoted to HIGH because dev_routes manifest also lies about it**.
2. ~~`GET /api/v1/conversations/{id}` missing~~ — **FIXED** (doc line 116)
3. ~~`GET /api/v1/mcp-servers/{name}/stderr` missing~~ — **FIXED** (doc line 217)

### New findings unique to D-redo

1. **`:duplicate` ghost route in 3 places**: api-design.md line 144, dev_routes.go line 75 ("forge.Duplicate" handler name), and the api-design.md note at line 144 even describes the expected behavior (201 + active version code/deps/python/parameters copied). Code has none of this. Three files claim it exists; zero implementations. The dev_routes.go manifest file even has its own comment (lines 7-11) requiring it stay in sync with HandleFunc registrations — that invariant is violated for the `:duplicate` row.

2. **Block types listing in api-design.md line 128 is wrong**: lists `attachment_ref` which is **not a block type** anywhere in the codebase; missing 2 real types `progress` + `message`. CLAUDE.md §E1 explicitly says "6 block 类型：`text` / `reasoning` / `tool_call` / `tool_result` / `progress` / `message`". This list contradicts not just code but also another contract doc.

3. **Several endpoints document the path but not the body/return shape**:
   - `POST /forges/{id}:test` returns aggregated `{total, passed, failed, results}` (not just a list)
   - `POST /forges/{id}:generate-test-cases` accepts `?count=N` query param (1-20, default 5)
   - `POST /skills/{name}:invoke` body `{arguments: string[]}` not documented
   - `POST /mcp-registry/{name}:install` body `{env, args}` not documented
   - `GET /mcp-servers/{name}/stderr` JSON envelope `{name, stderr, size}` not documented
   These are LOW (consumer can probe), but if api-design.md is to remain a "一眼索引" it should mention body shape at least.

### Severity rollup

- **HIGH**: 1
  - `:duplicate` documented as ✅ in api-design.md + dev_routes.go but **code has zero implementation**. Any client calling `POST /api/v1/forges/{id}:duplicate` will get 404 "unknown action: duplicate". The dev_routes.go manifest's own contract (line 7-11: "should match len(devRoutes)") is violated.

- **MED**: 1
  - Block-types list in api-design.md line 128 wrong: claims 5 types incl. nonexistent `attachment_ref`, missing 2 real types (`progress`, `message`). Wire-format-relevant; cross-doc inconsistent with CLAUDE.md §E1.

- **LOW**: 8
  - PATCH conversation only mentions "改名"; actually supports partial update incl. `systemPrompt`
  - mcp `:install` doesn't state 201 return
  - mcp PUT 200-even-on-connect-failure unstated
  - mcp stderr JSON-envelope shape unstated
  - skills `:invoke` body shape unstated
  - catalog Get can return null inside envelope, unstated
  - forge `:test` aggregated return shape unstated
  - forge `:generate-test-cases` `?count=` query param unstated

---

## Reasoning

api-design.md has been substantially cleaned up between D1 (2026-05-10) and D-redo (2026-05-11) — all D1 HIGH items closed. The single new HIGH `:duplicate` ghost surface was actually present *before* D1 too (D1 mis-classified it as "in code but not in doc"); careful end-to-end reading reveals it's the opposite — doc + manifest claim a route the code has never implemented. The dev_routes.go manifest amplifies the lie because its file-header invariant guarantees route accuracy but the row was not removed when the feature was deferred or cut.

The MED block-types finding shows the kind of drift only end-to-end reading can catch: a grep for `block` types would land in `domain/eventlog/eventlog.go` enums, but verifying the api-design.md descriptive prose against that enum requires reading both texts. `attachment_ref` looks plausible (attachments are real), but it's not a block-type concept — attachments live as `chatdomain.Attachment` rows referenced by message attrs, not block content.

The LOW findings are mostly api-design.md being a "一眼索引" by design — short prose, points readers to service-design-documents/<domain>.md for shapes. That's the design intent per the doc's own preamble (line 7). Whether to expand the index to include body/return shape is a design call, not a doc-vs-code drift.

---

## Recommended actions (informational; this audit does not modify files)

1. Decide on `:duplicate`: implement (per documented spec) **or** remove from api-design.md line 144 + dev_routes.go line 75. The current state (documented + manifest-claimed + zero code) is the worst combination.
2. Fix block-types list at api-design.md line 128: change `text/reasoning/tool_call/tool_result/attachment_ref` to `text/reasoning/tool_call/tool_result/progress/message` to match CLAUDE.md §E1 and `domain/eventlog/eventlog.go`.
3. Optional (LOW): expand the doc's body-shape coverage for the 6 endpoints listed above, or accept api-design.md is index-only and rely on service-design-documents/<domain>.md for shapes.
