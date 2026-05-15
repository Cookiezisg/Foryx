# forge.go audit trace (handlers-B4)

**File**: `backend/internal/transport/httpapi/handlers/forge.go` (406 LOC)
**Scope**: §S3 / §S9 / §S15 / §S16 / §S17
**Note**: forge service backend rewrite is in flight (M17). HIGH/MED findings only count as CRITICAL when they cause data loss / deployment break; otherwise → DEFER. LOW → DEFER blanket. Findings still recorded for post-rewrite review.

## 9-column trace

| site# | file:line | snippet | category | classification | reasoning | severity | user_impact | suggested_fix | status |
|---|---|---|---|---|---|---|---|---|---|
| 1 | forge.go:79 | `if err := decodeJSON(r, &req); err != nil { responsehttpapi.FromDomainError(w, h.log, err); return }` | A.4/A.5 | OK | decodeJSON helper (apikey.go:217) returns `fmt.Errorf("handlers.decodeJSON: %w", joinInvalidRequest(err))`; joins ErrInvalidRequest sentinel; errmap maps it to 400 INVALID_REQUEST. §S16 / §S17 compliant. | N-A | — | — | — |
| 2 | forge.go:83-90 | `t, err := h.svc.Create(r.Context(), forgeapp.CreateInput{...}); if err != nil { responsehttpapi.FromDomainError(...) }` | A.2/A.4 | EDGE | Create is a terminal write (forge row insert + version row + pending row). Handler passes `r.Context()` straight to service — handler-layer §S9 stance is pass-through (per handlers-B3 _summary §3 — detached responsibility lives in app layer). Forge service detached-ctx audit is forge-app scope. | LOW | If user disconnects mid-Create, partial writes may roll back; handler doesn't pre-detach. | EDGE-FLAG → forge-app audit batch. | DEFER |
| 3 | forge.go:91 | `responsehttpapi.Created(w, t)` | — | OK | 201 envelope on create — §N2 compliant. | N-A | — | — | — |
| 4 | forge.go:94-106 | `func (h *ForgeHandler) List(...)` — paginationpkg.Parse + svc.List read-only | A.4 | OK | All errors via FromDomainError. Pure read path. | N-A | — | — | — |
| 5 | forge.go:108-115 | `Get(... r.PathValue("id"))` | A.4 | OK | Read-only; svc returns sentinel (forgedomain.ErrNotFound) → errmap 404 TOOL_NOT_FOUND (errmap.go:81). | N-A | — | — | — |
| 6 | forge.go:117-136 | `Update` PATCH with all-pointer fields → svc.Update | A.2/A.4 | EDGE | Terminal write (UPDATE forges SET …). r.Context() pass-through to service — same stance as Create (site 2). | LOW | Same as site 2 — partial mid-update with cancel risk. | EDGE-FLAG → forge-app audit. | DEFER |
| 7 | forge.go:138-144 | `Delete(... r.PathValue("id"))` → svc.Delete + NoContent(204) | A.2/A.4 | EDGE | Terminal write (soft-delete sets deleted_at). Same r.Context() pass-through. | LOW | Same EDGE-FLAG. | EDGE-FLAG → forge-app audit. | DEFER |
| 8 | forge.go:148-160 | `Import` — `var data json.RawMessage; decodeJSON(r, &data); svc.Import(... []byte(data))` | A.2/A.4 | EDGE | Decoding into RawMessage means **decodeJSON's DisallowUnknownFields is silently bypassed** (RawMessage absorbs anything). This is intentional (Import payload is opaque snapshot from Export), but it deviates from the strict-decode handler norm. Terminal write (creates forge + version). | LOW | Import payload schema validation deferred entirely to service — if service silently accepts garbage, debug noise on bad imports. | Document in handler comment that RawMessage is intentional / leave to service validation; no change. EDGE-FLAG → forge-app audit. | DEFER |
| 9 | forge.go:167-187 | `postOnForge` action dispatcher; default → `responsehttpapi.Error(w, http.StatusNotFound, "NOT_FOUND", "unknown action: "+action, nil)` | A.4/A.5 | EDGE | Inline `"NOT_FOUND"` wire-code literal — handler-local, no sentinel. Per cross-cutting handlers-B3 _summary §5 (wire code ad-hoc), this is a known mixed pattern across handlers. Not strict §S17 violation (no sentinel). Same with line 170 fallback. | LOW | None — message is correct; just contributes to wire-code style drift. | EDGE-FLAG → cross-cutting wire-code policy decision. | DEFER |
| 10 | forge.go:189-203 | `Run` — decodeJSON + svc.RunForge (terminal: forge_executions row insert) | A.2/A.4 | EDGE | RunForge writes execution record (kind=run) + may modify sandbox state (long-running). r.Context() pass-through. **Long-running risk** — if browser cancels mid-run, executions row may not commit. | LOW | User cancel during run may leave incomplete forge_executions row (handler-layer not responsible). | EDGE-FLAG → forge-app audit (execution row write must use detached ctx). | DEFER |
| 11 | forge.go:205-217 | `Export` writes raw `application/json` body via `_, _ = w.Write(data)` | A.1 | OK | `_, _ = w.Write(data)` — explicit ignore with **inline comment** (lines 213-215) explaining client disconnect; status code already sent. Per §S3 example exception "_ = err with inline comment". | N-A | — | — | — |
| 12 | forge.go:219-233 | `Revert` decodeJSON + svc.RevertToVersion | A.2/A.4 | EDGE | Terminal write (creates new version row pointing back to old code). r.Context() pass-through. | LOW | Mid-revert cancel risk. | EDGE-FLAG → forge-app audit. | DEFER |
| 13 | forge.go:235-250 | `RunAllTests` — `results, err := h.svc.RunAllTests(...); for _, r := range results { if r.Pass != nil && *r.Pass { passed++ } }` then envelope | A.4 | OK | Read-then-aggregate; service returns []TestResult with err for batch failure. No silent swallow — err handled at line 238. | N-A | — | — | — |
| 14 | forge.go:255-268 | `GenerateTestCases` — `if n, err := strconv.Atoi(s); err == nil && n > 0 && n <= 20 { count = n }` | A.1 | EDGE | Atoi err silently ignored — but classification: **silent fallback by design** (`?count=foo` → falls back to default 5). Not data-loss; query-param parse. Same pattern as paginationpkg. | LOW | Garbage `count` query silently uses default — minor UX confusion (debug); could log if user repeatedly fails. | EDGE — silent fallback by design; consider adding inline comment "intentional silent fallback to default 5" to anchor §S3 audit. | DEFER |
| 15 | forge.go:262-267 | `result, err := h.svc.GenerateTestCases(r.Context(), id, count); if err != nil ... ; responsehttpapi.Success(w, http.StatusOK, result)` | A.2/A.4 | EDGE | LLM-driven generation (long-running, calls external API + writes test_cases rows). r.Context() pass-through; if browser disconnects during long LLM call, ctx cancels and test_cases may not all commit. | LOW | Mid-generate cancel could leave partial test_cases batch. | EDGE-FLAG → forge-app audit (per-test-case write should be in tx + detached). | DEFER |
| 16 | forge.go:272-279 | `ListVersions` read-only | A.4 | OK | Standard read. | N-A | — | — | — |
| 17 | forge.go:281-293 | `GetVersion` — `v, err := strconv.Atoi(r.PathValue("version")); if err != nil { responsehttpapi.Error(w, http.StatusBadRequest, "INVALID_REQUEST", "version must be an integer", nil); return }` | A.4/A.5 | EDGE | Inline `"INVALID_REQUEST"` literal wire-code + free-form message — bypasses errmap. Per handlers-B3 _summary §5, this is a known mixed pattern. **Not strict §S17 violation** (no sentinel). Could rewrap as `errorsdomain.ErrInvalidRequest` and FromDomainError, but message specificity ("version must be an integer") would lose unless wrapped. | LOW | Wire-code style drift. | EDGE-FLAG → wire-code policy. Or: `responsehttpapi.FromDomainError(w, h.log, fmt.Errorf("handlers.GetVersion: version must be an integer: %w", errorsdomain.ErrInvalidRequest))`. | DEFER |
| 18 | forge.go:297-321 | Pending family: `GetPending`, `AcceptPending`, `RejectPending` | A.2/A.4 | EDGE | AcceptPending and RejectPending are terminal writes (apply pending → new active version + clear pending; reject → drop pending). r.Context() pass-through. | LOW | Mid-accept cancel risk (could leave half-applied pending). | EDGE-FLAG → forge-app audit. | DEFER |
| 19 | forge.go:325-332 | `ListTestCases` read-only | A.4 | OK | Standard read. | N-A | — | — | — |
| 20 | forge.go:334-352 | `CreateTestCase` decodeJSON + svc.CreateTestCase + Created | A.2/A.4 | EDGE | Terminal write (test_cases row insert with ID `tc_<hex>`). r.Context() pass-through. | LOW | Mid-create cancel risk; same handler-layer stance. | EDGE-FLAG → forge-app audit. | DEFER |
| 21 | forge.go:354-360 | `DeleteTestCase` — `r.PathValue("tcId")` then svc.DeleteTestCase | A.2/A.4 | EDGE | Terminal soft-delete. Same pattern. | LOW | Same. | EDGE-FLAG → forge-app audit. | DEFER |
| 22 | forge.go:365-377 | `postOnTestCase` action dispatch (only "run") with NOT_FOUND fallback | A.4/A.5 | EDGE | Same inline `"NOT_FOUND"` ad-hoc wire-code as site 9. Handler-local literal. | LOW | Wire-code style drift. | EDGE-FLAG → cross-cutting policy. | DEFER |
| 23 | forge.go:371 | `result, err := h.svc.RunTestCase(r.Context(), tcID, "")` | A.2/A.4 | EDGE | Terminal write (forge_executions row, kind=test, single test case). r.Context() pass-through; same long-running cancel risk as site 10. | LOW | Mid-run cancel may not commit forge_executions row. | EDGE-FLAG → forge-app audit. | DEFER |
| 24 | forge.go:387-406 | `ListExecutions` read-only with cursor pagination + filter | A.4 | OK | Standard read. Filters by kind/batchId/cursor — pass-through to service. | N-A | — | — | — |
| 25 | forge.go (entire file) | No `f_<16hex>` / `fv_<16hex>` / `tc_<hex>` / `fe_<hex>` ID generation | A.3 | OK | Handler does not generate business IDs — IDs are produced by forge-app service via `idgenpkg.New(...)`. Handler only reads `r.PathValue("id")` / `r.PathValue("version")` / `r.PathValue("tcId")`. §S15 N/A at handler layer. | N-A | — | — | — |
| 26 | forge.go (entire file) | No `var Err...` / sentinel definitions | A.5 | OK | File defines zero sentinels. Consumes forgedomain sentinels via FromDomainError; all forgedomain.Err* are registered (errmap.go:81-94, 14 entries: ErrNotFound, ErrDuplicateName, ErrVersionNotFound, ErrPendingNotFound, ErrPendingConflict, ErrTestCaseNotFound, ErrRunFailed, ErrASTParseError, ErrImportInvalid, ErrEnvNotReady, ErrNoActiveVersion, ErrEnvFailed, ErrSandboxUnavailable, ErrDependencyResolution). | N-A | — | — | — |

## Sub-check summary

**A.1 §S3 错误吞没**:
- violations: not present at handler layer
- EDGE notes: site 11 (Write late-error explicit ignore — has inline comment, OK); site 14 (Atoi silent fallback — by design, recommend inline comment anchor — DEFER)

**A.2 §S9 detached ctx 终态写**:
- terminal-state writes identified: sites 2 (Create) / 6 (Update) / 7 (Delete) / 8 (Import) / 10 (Run) / 12 (Revert) / 15 (GenerateTestCases) / 18 (Accept/Reject Pending) / 20 (CreateTestCase) / 21 (DeleteTestCase) / 23 (RunTestCase)
- ctx 来源: 全部 `r.Context()` 透传
- violations: not present at handler layer — per handlers-B3 _summary §3, handler-layer r.Context() pass-through is the standard §S9 stance; detached responsibility belongs to forge-app service. **All 11 terminal sites EDGE-FLAG → forge-app audit batch** (especially long-running Run/RunTestCase/GenerateTestCases — must use detached ctx for forge_executions row write + LLM-generated test_cases batch).

**A.3 §S15 ID 生成**:
- ID generation calls: 0 in handler
- violations: N/A — handler does not generate business IDs (forge IDs `f_/fv_/tc_/fe_` produced by forge-app service via idgenpkg.New)

**A.4 §S16 错误 wrap 格式**:
- violations: not present (all `fmt.Errorf` use is inside helpers, not in this file)
- EDGE: sites 9/22 (`"NOT_FOUND"` inline wire-code), 17 (`"INVALID_REQUEST"` inline wire-code with free-form message). Both deviate from "FromDomainError everywhere" pattern but are not strict §S16 violations (no sentinel involved).

**A.5 §S17 sentinel 登记 errmap**:
- sentinels defined in file: none
- forgedomain sentinels consumed (via FromDomainError): all 14 already registered at errmap.go:81-94 — fully covered
- missing: none

## File-level findings summary

| Severity | Count | Sites |
|---|---|---|
| HIGH | 0 | — |
| MED | 0 | — |
| LOW | 11 | sites 2, 6, 7, 8, 10, 12, 14, 15, 17, 18, 20, 21, 22, 23 (all EDGE-LOW; mostly terminal-state EDGE-FLAG to forge-app + 3 wire-code ad-hoc) |

**Net forge.go**: 0 HIGH / 0 MED / ~11 LOW (EDGE classification — handler-layer-clean; all forge-app audit deferrals).

## Status: forge backend rewrite (M17) DEFER

Per task instructions, forge.go non-CRITICAL findings (HIGH/MED that don't cause data loss or deployment break; all LOW) are filed as **DEFER** — to be resolved during the M17 forge backend rewrite (which will revisit the entire forge service stack). Handler layer is architecturally clean: thin handler, FromDomainError throughout, sentinel registration complete, only pre-existing wire-code drift cross-cutting carries over.

**No CRITICAL findings that escape DEFER** — file is in a healthy state; all real risk lives in forge-app service (long-running execution writes, LLM-driven test-case generation), which is exactly where the M17 rewrite will rebuild detached-ctx + tx semantics.
