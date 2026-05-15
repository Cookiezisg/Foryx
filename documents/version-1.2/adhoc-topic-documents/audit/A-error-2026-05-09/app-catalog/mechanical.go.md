# mechanical.go — Phase A audit

**Path**: `backend/internal/app/catalog/mechanical.go`
**LOC**: 88
**Role**: Deterministic no-LLM Catalog builder. Used when (a) Generator is nil (D8-2 default) or (b) Generator returns an error. Produces full per-source enumeration; sacrifices LLM-inferred routing observations but guarantees coverage.

## Trace

| site# | file:line | snippet | category | classification | reasoning | severity | user_impact | suggested_fix | status |
|---|---|---|---|---|---|---|---|---|---|
| 1 | mechanical.go:31-74 | `mechanicalFallback(items, gMap) *catalogdomain.Catalog` — no error return | A.1 | OK | Function is total (every input maps to a valid output). No I/O, no LLM, no failure modes. Returning *Catalog without error is correct given the function's contract. Not §S3 — there's nothing to fail. | N-A | — | — | — |
| 2 | mechanical.go:54 | `fmt.Fprintf(&b, "\n### %s (%d, %s)\n", name, len(srcItems), gran.String())` | A.1 | OK | Fprintf to strings.Builder cannot fail (Builder.Write returns nil error always). Discarding the (n int, err error) return is canonical Go for strings.Builder. | N-A | — | — | — |
| 3 | mechanical.go:61 | `fmt.Fprintf(&b, "- **%s**: %s\n", it.Name, desc)` | A.1 | OK | Same as #2. | N-A | — | — | — |
| 4 | mechanical.go:42, 67 | `b.WriteString("## Available capabilities\n")` / `b.WriteString("\nIf a task could fit ...")` | A.1 | OK | strings.Builder.WriteString returns (n int, err error) where err is always nil per stdlib contract. Discarding both is canonical. | N-A | — | — | — |
| 5 | mechanical.go:81-87 | `groupBySource` — pure helper, returns `map[string][]Item` | A.1 | OK | No error path; pure data shaping. | N-A | — | — | — |
| 6 | mechanical.go:53 | `gran := gMap[name]` (zero-value `Granularity` when key absent) | A.1 | EDGE | Zero-value behavior: `gMap[name]` returns `catalogdomain.PerItem` (per Granularity iota = 0) when source name not in map. Per polling.go::Refresh line 204, gMap is populated for every successful source. **However**, if a source's ListItems returns items but the source's Granularity() panics or is forgotten, gMap could be missing keys — but in practice the same loop in Refresh sets both `items` and `gMap[name]` together so they stay in sync. Not really a §S3 violation — fallback to PerItem is documented in catalog.md §4 as the safe default ("PerItem=0 是新 source 安全默认"). The zero-value default IS the design. | N-A | — | — | — |

## Sub-checks

A.1 §S3 错误吞没:
  - violations: not present
  - rationale: file has no I/O / network / mutex / channel / map-can-fail operation. All functions are pure transformations on slice/map types. The few discarded returns (Fprintf, WriteString) come from strings.Builder methods that stdlib documents as never-failing. The zero-value Granularity fallback (#6) is design intent per catalog.md §4.

A.2 §S9 detached ctx 终态写:
  - terminal-state writes identified: none
  - 各自 ctx 来源: N/A — file accepts no ctx
  - violations: N/A: package builds an in-memory Catalog struct only; no DB / disk / SSE writes happen here. Persistence is the caller's job (polling.go::Refresh writes to disk + cache).

A.3 §S15 ID 生成:
  - ID generation calls: none
  - violations: N/A: package doesn't generate IDs. Coverage map is populated from `it.ID` (caller-supplied source IDs), not generated here.

A.4 §S16 错误 wrap 格式:
  - violations: not present
  - rationale: no `fmt.Errorf` / `errors.New` calls — file produces no errors.

A.5 §S17 sentinel 登记 errmap:
  - sentinels defined: none
  - 已登记 errmap: N/A
  - missing: N/A: file defines no sentinels.

## Spot-check

- Verified `groupBySource` map iteration order is stabilized in mechanicalFallback by `sort.Strings(sourceNames)` (line 37) + per-source `sort.Slice(srcItems, ...)` (line 51). Deterministic output critical for fingerprint stability — matches the polling.go::fingerprint contract.
- Verified `it.Description == ""` empty-desc fallback to `"(no description)"` is harmless — affects display text only; coverage IDs preserved verbatim.
- Verified return shape `&Catalog{Summary, Coverage, GeneratedBy:"mechanical-fallback"}` matches the §10 documented `GeneratedBy` enum from design doc (`"llm"` / `"mechanical-fallback"`). Caller polling.go::Refresh stamps remaining fields (Fingerprint / Version / GeneratedAt / SourcesAt) at line 241-244.
- Verified mechanicalFallback is idempotent for the same input — pure function, no global state, deterministic ordering.
