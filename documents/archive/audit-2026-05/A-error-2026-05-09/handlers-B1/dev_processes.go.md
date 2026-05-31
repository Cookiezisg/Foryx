# dev_processes.go — audit trace

**Path**: `backend/internal/transport/httpapi/handlers/dev_processes.go`
**LOC**: 43
**Role**: Single dev-only handler `(h *DevHandler).BashProcesses` for `GET /dev/bash-processes?sample=N`. Read-only inspection of Bash tool's background-process registry. Returns metadata + optional ring-buffer tail sample.

## 9-col trace

| site# | file:line | snippet | category | classification | reasoning | severity | user_impact | suggested_fix | status |
|---|---|---|---|---|---|---|---|---|---|
| 1 | dev_processes.go:29-33 | `if s := r.URL.Query().Get("sample"); s != "" { if n, err := strconv.Atoi(s); err == nil && n >= 0 { sample = n } }` | A.1 | EDGE | Bad `?sample=foo` silently falls back to default 2048; condition `err == nil && n >= 0` rejects negative + non-numeric without returning 400. Per §S3 this is a "silent fallback" pattern, but `sample` is an optional UI knob (how many tail bytes to include) on a dev-only endpoint — no data loss, no business impact. Default-on-bad-input is acceptable UX for testend; bumping to 400 INVALID_REQUEST is technically more correct but inflates dev friction. | LOW | Bad query param yields default sample size silently — testend operator sees response with 2048 tail bytes when they typed `sample=abc`, may not notice. | Optional: return 400 INVALID_REQUEST when `s != "" && err != nil` so testend reveals typos; current behavior is OK for dev tool. | FIXED-doc (this commit — added §6 反校验剧场 inline 注释满足 §S3 字面"silent skip 必须带注释"，行为不变) |
| 2 | dev_processes.go:37-43 | `snaps := h.shellManager.Snapshots(sample); responsehttpapi.Success(w, http.StatusOK, map[string]any{...})` | A.1 | OK | `Snapshots` returns `[]Snap` synchronously, no error path; `Success` envelope is the §N1 happy path. Handler is acceptably thin (decode query → call manager → write envelope). | N-A | — | — | — |

## Sub-checks

**A.1 §S3 错误吞没**:
- violations: site 1 (LOW — query-param parse silent fallback to default; dev-only convenience knob, classified EDGE; §S3 strict reading would call this "silent skip" but no data / business loss)

**A.2 §S9 detached ctx 终态写**:
- terminal-state writes identified: none
- 各自 ctx 来源: N/A
- violations: N/A: read-only handler, no DB writes / no terminal state

**A.3 §S15 ID 生成**:
- ID generation calls: none
- violations: N/A: handler reads existing process registry; does not mint IDs

**A.4 §S16 错误 wrap 格式**:
- violations: not present (no `fmt.Errorf` / `errors.New` calls; no error paths to wrap)

**A.5 §S17 sentinel 登记 errmap**:
- sentinels defined: none
- 已登记 errmap: N/A
- missing: N/A: file defines no sentinels and never calls `responsehttpapi.FromDomainError`

## Summary

- Sites: 2
- Violations: 1 LOW (site 1 dev-only query-param silent fallback — EDGE per §6 反校验剧场 thinking; could be tightened to 400 but current is acceptable for testend)
- Verdict: clean handler, only knob-parse fallback raises any §S3 flag; classifying LOW because user impact is negligible on dev-only endpoint.
