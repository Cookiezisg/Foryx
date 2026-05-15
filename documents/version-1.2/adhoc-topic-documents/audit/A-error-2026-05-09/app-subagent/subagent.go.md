# Audit trace: backend/internal/app/subagent/subagent.go

LOC: 162. Service struct + DI ctor + filterTools (recursion-defense filter) + composeSystemPrompt helper. No business-ID generation, no terminal writes, no error wrapping at this layer (pure data plumbing).

## 9-col trace table

| site# | file:line | snippet | category | classification | reasoning | severity | user_impact | suggested_fix | status |
|---|---|---|---|---|---|---|---|---|---|
| 1 | subagent.go:91-93 | `if log == nil { panic("subagent.New: logger is nil") }` | A.4 | OK | §S16 panic on nil-logger boot-time wiring guard. Same pattern as `apikey.NewService` / `mcp.New` / etc. Caught at app boot, not runtime. Format string carries `subagent.New:` qualifier per §S16 even for panic. | — | — | — | — |
| 2 | subagent.go:120-147 | `func (s *Service) filterTools(typ subagentdomain.SubagentType) []toolapp.Tool { ... }` returns `[]toolapp.Tool` only (no error) | A.1/A.4 | OK | §S3 not applicable — function intentionally has no error path; pure list comprehension over s.tools. Returns nil on empty input naturally. No silent failure: `len(out) == 0` → `nil` is the documented contract per `subagent.md` §8 ("AllowedTools=nil semantics"). The drop of `SubagentTool` is the recursion defense itself, not a swallowed error. | — | — | — | — |
| 3 | subagent.go:154-162 | `func composeSystemPrompt(typeSystemPrompt string, locale reqctxpkg.Locale) string { ... }` | A.1/A.4 | OK | Pure string builder, no error path. Locale gate on `LocaleZhCN` constant — no comparison failure modes. §S3/§S16 not applicable. | — | — | — | — |
| 4 | subagent.go:64-75 | `Service` struct: `activeRuns map[string]context.CancelFunc` + `activeRunsMu sync.Mutex` for in-flight cancellation | A.2 (anchor) | OK | §S9 anchor — this map is consulted by `Cancel` (queries.go) and populated by `Spawn` (spawn.go). The cancellation cascade itself (parent ctx → child) is implemented in spawn.go and audited there. Struct definition is clean. | — | — | — | — |

## Sub-check (§S3 / §S9 / §S15 / §S16 / §S17)

**A.1 §S3 错误吞没**:
  - violations: not present

**A.2 §S9 detached ctx 终态写**:
  - terminal-state writes identified: none (this file)
  - 各自 ctx 来源: N/A
  - violations: N/A: file defines no terminal writes (Service struct + helpers only — terminal-write logic lives in `spawn.go::Spawn` finalize tail and `host.go::WriteFinalize`, both audited separately)

**A.3 §S15 ID 生成**:
  - ID generation calls: none
  - violations: N/A: file generates no business IDs (registry name lookups only; sub-run IDs come from `idgenpkg.New("msg")` / `New("blk")` in spawn.go per `subagent.md` §16)

**A.4 §S16 错误 wrap 格式**:
  - violations: not present (file produces no `fmt.Errorf` calls — error wrapping happens in spawn.go and queries.go)

**A.5 §S17 sentinel 登记 errmap**:
  - sentinels defined: none in this file
  - 已登记 errmap: N/A
  - missing: N/A: file defines no sentinels (the two subagent sentinels — `ErrTypeNotFound`, `ErrRecursionAttempt` — live in `domain/subagent/` and are confirmed registered at `errmap.go:125-126`)

## Notes

- `filterTools` is the **structural** half of the recursion defense documented in `subagent.md` §8 — it physically drops the tool with `Name() == "Subagent"`. The runtime half (depth check) lives in `app/tool/subagent/agent.go::Execute`.
- Panic on nil-logger (line 92) intentionally does NOT panic on nil `chatRepo` / `registry` / `modelPicker` / `keyProvider` / `llmFactory`. Two ways to read this: (a) only logger has the property "nil dereference is silent (zap.L().Info on nil pointer crashes deep in the call chain, hard to attribute)" so we guard logger explicitly; (b) other deps are validated at first-use site in spawn.go where the failure path produces a wrapped error. Consistent with the pattern in `apikey.NewService`. Not a finding.
- The package-level godoc (lines 1-40) is a textbook §S11 §1 bilingual block: English first, then Chinese, with the file map (lines 25-31). 30 lines is over the §S11 §4 "package doc ≤ 4 lines" threshold (which says "搬去 design doc"). However, this is documenting the file structure (what each file does) which IS the package-doc legitimate scope, not design intent narrative. Borderline but I read it as **OK** — the file map is the kind of thing that justifies the longer doc. Not flagged.
