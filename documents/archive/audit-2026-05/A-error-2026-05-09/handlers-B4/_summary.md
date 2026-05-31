# Package audit summary: transport/httpapi/handlers (B4 — 3 large files)

## Spec understanding (Phase A — §S3 / §S9 / §S15 / §S16 / §S17)

- **§S3 错误不吞**: B4 3 files yield **3 MED + 4 LOW concrete violations** + many EDGE-LOW handler-pass-through. Highlights:
  - **mcp.go site 23 (MED)** — `errors.Is(err, errEmptyBody)` against unproduced sentinel always-false → all decode errs silently treated as "no body", masking malformed JSON; user sees "MCP_REQUIRED_ENV_MISSING" instead of "INVALID_REQUEST".
  - **mcp.go site 15 (MED)** — multipart silent break on non-EOF io err — **mirrors handlers-B3 skills.go MED**, identical pattern + identical 1-line fix (`io.ReadAll(io.LimitReader(...))`).
  - **mcp.go site 6 (MED)** — AddServer error → `zap.Warn` + 200 OK with status row. Design-debatable per mcp.md §10 ("PUT returns 200 regardless of connect success") but borderline §S3 — silent fallback at log level masks original err from caller.
  - **dev.go sites 11/14/17/19 (LOW)** — `if err == nil { ... }` / `continue` silent-skip cluster in dev SQL/Schema/Collections — turns dev console into unreliable diagnostic surface. 1-line `h.log.Warn` per site fixes all 4.
- **§S9 detached ctx 终态写**: B4 3 files **0 violation at handler layer** — full pass-through to app services per established stance. **17 EDGE-FLAGS** identified for app-layer audits:
  - **forge.go**: 11 terminal sites → forge-app audit (longest-running: Run, RunTestCase, GenerateTestCases LLM call)
  - **mcp.go**: 6 terminal sites → mcp-app audit. **Most §S9-critical site: InstallFromRegistry (site 24)** — sandboxapp install (mise + npm) can take **minutes**; cancel risk is highest in entire B4 batch.
  - **dev.go**: 1 terminal site → tool-framework audit (InvokeTool can call any tool's terminal write).
- **§S15 ID 生成**: B4 3 files **0 violation** — handler does not generate business IDs (forge IDs by forge-app idgenpkg; MCP servers keyed by name; dev.go no business IDs, only `time.Now().Unix()` cache-buster which is §S15 N/A by design).
- **§S16 错误 wrap 格式**: B4 **9 LOW EDGE** — all trace to **same cross-cutting decodeJSONLimit refactor** identified in handlers-B3 _summary §1, plus dev-only divergence which is accepted posture. **mcp.go site 4** is the cleanest single-fix opportunity (replace inline `json.NewDecoder(...).Decode` with `decodeJSONLimit` after B3 refactor lands).
- **§S17 errmap 单一事实源**: B4 3 files **fully covered** — 14 forgedomain + 14 mcpdomain sentinels all registered. **mcp.go has 1 dead sentinel `errEmptyBody`** (site 26 LOW) — never produced, never registered (correctly), but should be deleted.

## Files audited

| File | LOC | Sites | OK | EDGE | VIOLATION |
|---|---|---|---|---|---|
| forge.go | 406 | 26 | 12 | 14 | 0 (all EDGE-FLAG → forge-app DEFER) |
| mcp.go | 483 | 29 | 5 | 18 | 6 (3 MED + 3 LOW concrete) |
| dev.go | 490 | 27 | 14 | 9 | 4 LOW (silent-skip cluster) |
| **TOTAL** | **1379** | **82** | **31** | **41** | **10** |

## Severity breakdown

| Severity | Count | Sites |
|---|---|---|
| HIGH | 0 | — |
| MED | 3 | mcp.go site 6 (AddServer 200+Warn silent fallback), mcp.go site 15 (multipart silent break — mirrors B3 skills.go MED), mcp.go site 23 (errEmptyBody dead-branch silent swallow) |
| LOW | 27 | forge.go: 14 EDGE-LOW (terminal pass-through + wire-code drift); mcp.go: 12 LOW (mix of concrete §S3 silent-fallback + EDGE-LOW); dev.go: 13 LOW (4 concrete silent-skip + 9 EDGE-LOW dev-only divergence) |

**Net B4**: 0 HIGH / 3 MED / 27 LOW. forge.go contributes 0 concrete violations (all DEFER); mcp.go owns all 3 MED; dev.go contributes 4 concrete LOW + 9 EDGE.

## Status (post-fix)

| Site | Severity | Status | Commit |
|---|---|---|---|
| forge.go (all 14 EDGE) | LOW | DEFER | M17 forge rewrite (task #145) |
| mcp.go site 6 (AddServer 200+Warn) | MED | FIXED-partial | this batch — Warn→Error log level (设计 per mcp.md §10 保留；log 级别让 observability 捞到) |
| mcp.go site 15 (multipart silent break) | MED | FIXED | this batch — io.ReadAll(io.LimitReader) 替代手卷 loop |
| mcp.go site 23 (errEmptyBody dead-branch) | MED | FIXED | this batch — `errors.Is(err, io.EOF)` 区分空 body vs malformed JSON；显式 400 |
| mcp.go site 25/26 (readAll fragility + dead errEmptyBody) | LOW | FIXED | this batch — 删 readAll helper + errEmptyBody sentinel |
| mcp.go site 27 (var _ = MergeResult{}) | LOW | FIXED | this batch — 改 `var _ mcpinfra.MergeResult` 零分配 |
| mcp.go site 4 (decodeJSONLimit consumer) | LOW | EDGE-DEFER | 待 B3 decodeJSONLimit refactor 推广（mcp.go 用 inline json.NewDecoder 不是 decodeJSONLimit consumer——待跨 helper 统一） |
| mcp.go 12 LOW (其他 wire-code / EDGE-FLAG) | LOW | EDGE-FLAG | mcp-app audit batch（终态写 detached-ctx 责任） |
| dev.go site 11 (rows.Scan silent continue) | LOW | FIXED | this batch — 加 h.log.Warn |
| dev.go site 14 (Schema name scan silent) | LOW | FIXED | this batch — 加 h.log.Warn |
| dev.go site 17 (PRAGMA + colRows.Scan silent) | LOW | FIXED | this batch — 加 h.log.Warn |
| dev.go site 19 (ListCollections ReadDir silent) | LOW | FIXED | this batch — 加 h.log.Warn |
| dev.go 9 EDGE (dev-only divergence) | LOW | WAIVED | dev-only 故意，与 B2 dev_info.go / B3 dev_mock_llm.go 一致 posture |

## Cross-cutting findings

### 1. Multipart silent break — recurring MED (mcp.go site 15 mirrors B3 skills.go)

**Same defect, same fix, **two** files now**:

```go
// mcp.go:295-310 — same hand-rolled loop pattern as skills.go (B3)
for {
    n, rerr := file.Read(buf)
    if n > 0 { raw = append(raw, buf[:n]...); ... }
    if rerr != nil { break }    // ❌ EOF undistinguished from real err
}
```

**Two-file fix**: replace both with `io.ReadAll(io.LimitReader(file, importMaxBytes+1))`. **§S20 "no留下次"** — both should be one batch fix. Forge-related upload paths should also be reviewed for the same anti-pattern.

### 2. mcp.go errEmptyBody dead-branch (MED, site 23 + 26)

The errEmptyBody sentinel is declared but **never produced** — `errors.Is(err, errEmptyBody)` at site 23 is statically always-false. The conditional silently swallows ALL decode errors as "no body provided", causing a critical UX bug:

- User submits malformed JSON for `:install` body
- Decode fails → `body.Env = nil; body.Args = nil`
- Service runs with no env/args → returns `ErrRequiredEnvMissing`
- User sees "MCP_REQUIRED_ENV_MISSING" while their actual problem was JSON syntax

**1-block fix**:
```go
if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, importMaxBytes)).Decode(&body); err != nil {
    if errors.Is(err, io.EOF) {
        // Empty body OK — entry may have no RequiredEnv/Args.
        body.Env = nil; body.Args = nil
    } else {
        responsehttpapi.Error(w, http.StatusBadRequest, "INVALID_REQUEST",
            "failed to parse install body: "+err.Error(), nil)
        return
    }
}
```

Plus: delete the dead `errEmptyBody = errors.New("empty body")` sentinel and its 7-line "future strict mode" comment. Dead code masking real bug.

### 3. mcp.go AddServer 200+Warn (MED, site 6) — design-debatable

This is **deliberate** per mcp.md §10 ("PUT returns 200 with ServerStatus regardless of connect succeed — caller checks status field"). But it violates §S3 spirit:
- Original error visible only in zap.Warn (log filters tuned to ERROR will miss it)
- HTTP 200 OK + status row "looks successful"
- Caller MUST inspect ServerStatus.Status field

**Three options**:
1. **Status quo (defended by mcp.md)** — accept design; recommend log at Error level (not Warn) so log filters catch it.
2. **Add `error` field to response payload** — explicit "these worked, this didn't" while keeping 200 OK.
3. **Switch to 207 Multi-Status** semantics — but Go stdlib doesn't have this; rare HTTP code.

**Recommend**: PM/design discussion (this is a §S3 vs. §N1/§N2 trade-off question). Until resolved: minimum at log Error level.

### 4. mcp.go readAll string-comparison EOF (LOW, sites 16/25)

Custom `readAll` helper uses `err.Error() == "EOF"` instead of `errors.Is(err, io.EOF)`. Currently works because `http.MaxBytesReader` returns plain io.EOF, but fragile to any custom Reader wrapping. **Trivial fix**: delete `readAll` entirely; use stdlib `io.ReadAll(body)`. Net code reduction, more correct.

### 5. dev.go silent-skip cluster (LOW, sites 11/14/17/19)

Four `if err == nil { ... }` / `continue`-on-err patterns in dev SQL / Schema / Collections endpoints:
- Hide the very errors operators are trying to find via dev console
- Same file already has the right pattern at sites 391/396 (`h.log.Warn(...); continue`) — just inconsistent

**4-line fix**: add `h.log.Warn(...)` before each silent continue/fallback. Cost: 4 lines + minor log noise; benefit: dev console reliability. **§S20 "no留下次"** clear-cut.

### 6. forge.go terminal-write EDGE-FLAG cluster (DEFER → forge-app)

11 terminal-write sites in forge.go all r.Context() pass-through to forge-app. Per established §S9 stance (handlers-B3 _summary §3), handler-layer pass-through is the standard pattern — detached responsibility lives in forge-app service. **All deferred to forge-app audit batch** + M17 rewrite.

**Most-critical sites for forge-app to address during M17**:
- **Run / RunTestCase / RunAllTests** — forge_executions row writes during long sandboxed exec
- **GenerateTestCases** — long LLM call + bulk test_cases batch insert
- **Import** — multi-row insert (forge + version + pending) needs transactional commit

### 7. mcp.go terminal-write EDGE-FLAG cluster (→ mcp-app)

6 terminal-write sites in mcp.go all r.Context() pass-through. **Most-critical**: site 24 InstallFromRegistry — sandboxapp runtime install (mise + npm) can take **minutes**. Browser hard-refresh / tab-close cancel is highly likely. **mcp-app must use detached ctx for InstallFromRegistry's mcp.json write + Connect** — else user gets half-installed server (runtime gone, mcp.json says installed, can't connect).

### 8. Wire-code style drift (cross-cutting LOW, all 3 files)

- forge.go: `"NOT_FOUND"` (sites 9/22), `"INVALID_REQUEST"` (site 17 inline message)
- mcp.go: `"INVALID_REQUEST"` (sites 4/9/12/13/17/18/22), `"MCP_COMMAND_REQUIRED"` (site 5 ad-hoc — could be mcpdomain.ErrCommandRequired sentinel)
- dev.go: dev-only custom envelopes (sites 6/7/8/9/12/13/22) — accepted divergence

Pattern: handler-local literal wire-codes mixed with errmap-driven sentinels. Per handlers-B3 _summary §5, project-level policy needed in `service-contract-documents/error-codes.md`. **LOW** — non-strict §S17 violation; suggest cross-cutting decodeJSONLimit refactor (B3 §1) lands first to clean up the bulk.

### 9. dev-only handler posture established

All three of B2 dev_info.go / B3 dev_mock_llm.go / B4 dev.go follow the same posture:
- Custom `{"error":"..."}` envelopes (not project-standard wire-code envelope)
- Verbatim `err.Error()` to operator (not stripped to "internal error" like prod)
- Tolerant of dev-mode misconfigurations with quiet fallbacks

This is **codified and consistent** — `--dev` flag gates registration, never reachable in prod. EDGE-LOW classification across all dev-only divergences.

## Recommended fix priorities

By §S20 + §S14 — B4 contributes **0 HIGH / 3 MED / 27 LOW**:

1. **mcp.go site 23 (errEmptyBody dead-branch) — MED**: 1-block fix; critical UX bug masking JSON errors. **Trivial; do now.**
2. **mcp.go site 15 (multipart silent break) — MED**: 1-line fix mirroring B3 skills.go MED fix (`io.ReadAll(io.LimitReader(...))`). **Same batch as B3 if not already merged.**
3. **mcp.go site 6 (AddServer 200+Warn) — MED**: design discussion needed (PM/architecture). Until resolved, minimum: change `zap.Warn` → `zap.Error`.
4. **dev.go silent-skip cluster (sites 11/14/17/19) — LOW (×4)**: 4-line fix adding `h.log.Warn` before silent continue. Dev console reliability boost. **Trivial; do as part of B3/B4 cross-cutting.**
5. **mcp.go site 4 (decodeJSONLimit consumer) — LOW**: pending B3 decodeJSONLimit refactor → 1-line replacement of inline `json.NewDecoder(...)` with helper.
6. **mcp.go site 25/26 (readAll fragility + dead errEmptyBody) — LOW**: delete `readAll` (use stdlib `io.ReadAll`), delete `errEmptyBody` + comment. Net code reduction, more correct.
7. **mcp.go site 27 (`var _ = mcpinfra.MergeResult{}`) — LOW**: change to `var _ mcpinfra.MergeResult` (no allocation). Trivial polish.
8. **forge.go all findings — DEFER**: M17 rewrite revisits entire stack.

## Out-of-scope notes

1. `_test.go` files per fork constraint (not read).
2. `forgeapp.Service.*` (forge service detached-ctx + execution row writes + LLM-driven test-case generation) — forge-app audit batch / M17 rewrite scope.
3. `mcpapp.Service.*` (AddServer / RemoveServer / Reconnect / HealthCheck / Import / InstallFromRegistry / ListRegistry) — mcp-app audit batch. **Highest-priority service-layer audit in B4 is `InstallFromRegistry` detached-ctx semantics** (long-running, high cancel risk).
4. Tool framework's `Tool.Execute(ctx, ...)` ctx-handling responsibility (dev.go site 24 pass-through) — tool framework / app-tool audit batch.
5. `responsehttpapi.StreamSSE` SSE plumbing & `loggerinfra.LogBroadcaster` Subscribe/Ring — separate response-pkg / logger-infra audit batches.
6. `infra/llm.Factory.SetTracer` (referenced in dev.go register block but handler is in dev_mock_llm.go — covered in B3) — already audited.
