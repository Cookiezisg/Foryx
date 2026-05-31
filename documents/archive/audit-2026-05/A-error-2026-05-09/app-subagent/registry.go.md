# Audit trace: backend/internal/app/subagent/registry.go

LOC: 121. Built-in SubagentType catalog: 3 V1 types (Explore / Plan / general-purpose) + Registry indexer (sync.Once-protected). No business-ID generation, no terminal writes, no error wrapping (Get returns `(value, bool)`, List returns just `[]value`).

## 9-col trace table

| site# | file:line | snippet | category | classification | reasoning | severity | user_impact | suggested_fix | status |
|---|---|---|---|---|---|---|---|---|---|
| 1 | registry.go:45-67 | `var builtInTypes = []subagentdomain.SubagentType{ {Name: "Explore", ..., AllowedTools: []string{"Read", "Glob", "Grep", "LS", "search_forges"}, DefaultMaxTurns: 30}, {Name: "Plan", ...}, {Name: "general-purpose", AllowedTools: nil, DefaultMaxTurns: 25} }` | A.1/A.4 | OK | Pure data declaration; no error / control-flow paths. The `general-purpose` type's `AllowedTools: nil` (line 64) is the documented "inherit parent registry minus Subagent" sentinel value, consumed by Service.filterTools (subagent.go:120) — explicit comment on line 64 confirms intent. | — | — | — | — |
| 2 | registry.go:75-78 | `type Registry struct { once sync.Once; idx map[string]subagentdomain.SubagentType }` | A.1 | OK | Concurrency-safe lazy-init pattern; `sync.Once.Do` semantics ensure idx is built exactly once across goroutines. No error paths. | — | — | — | — |
| 3 | registry.go:83-85 | `func NewRegistry() *Registry { return &Registry{} }` | A.1/A.4 | OK | Trivial ctor. | — | — | — | — |
| 4 | registry.go:87-97 | `func (r *Registry) ensureIndexed() { r.once.Do(func() { r.idx = make(map[string]subagentdomain.SubagentType, len(builtInTypes)); for _, t := range builtInTypes { if t.DefaultMaxTurns <= 0 { t.DefaultMaxTurns = defaultMaxTurns }; r.idx[t.Name] = t } }) }` | A.1 | OK | sync.Once.Do has no error return. Loop body is pure assignment. The `t.DefaultMaxTurns <= 0` defensive default applies the constant `defaultMaxTurns = 25` (line 39) — intentional fallback, not silent failure (each builtInType in §1 already declares an explicit DefaultMaxTurns ≥ 25, so this branch never triggers in practice but defends against future edits that drop the field). | — | — | — | — |
| 5 | registry.go:102-106 | `func (r *Registry) Get(name string) (subagentdomain.SubagentType, bool) { r.ensureIndexed(); t, ok := r.idx[name]; return t, ok }` | A.1 | OK | `(value, bool)` Go convention for "lookup or absent" — caller (subagent.go:88 `Spawn` site) checks `!ok` and converts to `subagentdomain.ErrTypeNotFound`. §S3 not violated because the `bool` IS the documented "not found" signal, not a swallowed error. | — | — | — | — |
| 6 | registry.go:113-121 | `func (r *Registry) List() []subagentdomain.SubagentType { r.ensureIndexed(); out := make(...); for _, t := range r.idx { out = append(out, t) }; sort.Slice(out, ...); return out }` | A.1 | OK | Pure list builder + alphabetic sort. No error paths. Stable ordering per the comment ("LLM 描述和 HTTP 列表跨调用保持确定") — compliance with `subagent.md` §11 HTTP API expectation. | — | — | — | — |

## Sub-check (§S3 / §S9 / §S15 / §S16 / §S17)

**A.1 §S3 错误吞没**:
  - violations: not present

**A.2 §S9 detached ctx 终态写**:
  - terminal-state writes identified: none
  - 各自 ctx 来源: N/A
  - violations: N/A: file does no IO / emit / persistence — pure in-memory data + lazy-init Registry

**A.3 §S15 ID 生成**:
  - ID generation calls: none
  - violations: N/A: file generates no business IDs (Registry stores SubagentType keyed by Name string, not by generated ID)

**A.4 §S16 错误 wrap 格式**:
  - violations: not present (file has no `fmt.Errorf` calls — no error path exists)

**A.5 §S17 sentinel 登记 errmap**:
  - sentinels defined: none in this file
  - 已登记 errmap: N/A
  - missing: N/A: file defines no sentinels (the `ErrTypeNotFound` sentinel that drives Get's `!ok` translation lives in domain/subagent/subagent.go:80 — registered errmap.go:125)

## Notes

- This file is mostly a static catalog. The audit is short because all functions are deterministic (no IO, no goroutine fan-out, no error returns) and the data is pure declarative configuration. Largest risk in this file class would be a typo in the AllowedTools strings (e.g. lowercase `"read"` not matching `Tool.Name() == "Read"`) — but that's a unit-test concern, not §S3/§S9/§S15/§S16/§S17 territory.
- The `defaultMaxTurns = 25` defensive fallback at line 39 + 91-93 is "right place to define a magic number" hygiene; not in audit scope.
- `sync.Once`-based lazy init means the registry is rebuilt on every fresh Service construction (since Registry is per-Service via NewRegistry). For a single-user local-only app this is fine; would be a candidate for `var pkg-level registry = ...` if registries became shared across many services, but V1 doesn't need that.
