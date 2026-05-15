# audit: backend/internal/infra/llm/factory.go

LOC: 140
Read: full file (lines 1-140)

## Trace

| site# | file:line | snippet | category | classification | reasoning | severity | user_impact | suggested_fix | status |
|---|---|---|---|---|---|---|---|---|---|
| 1 | factory.go:81-85 | `func (f *Factory) Build(cfg Config) (Client, string, error) { baseURL, err := resolveBaseURL(cfg); if err != nil { return nil, "", err } }` | A.4 | EDGE | bare `return nil, "", err` — `resolveBaseURL` (line 137) wraps with `llm: %s provider requires base_url` prefix but no `%w` and no sentinel; this site preserves what's there but doesn't add `infra/llm.Factory.Build:` pkg.method context. | LOW | identical UX (`err.Error()` reaches caller); call-site grep harder | wrap: `return nil, "", fmt.Errorf("llm.Factory.Build: %w", err)` for grep + still-unwrapped propagation | FOUND |
| 2 | factory.go:113 | `client = &adapterWrappedClient{inner: client, adapter: lookupAdapter(cfg.Provider)}` | A.1 | OK | no error path; adapter lookup falls through to openaiAdapter on unknown provider per adapter.go:#2 documented design. | N-A | — | — | — |
| 3 | factory.go:128-139 | `func resolveBaseURL(cfg Config) (string, error) { ... return "", fmt.Errorf("llm: %s provider requires base_url", cfg.Provider) }` | A.4 | EDGE | §S16 violation: error string `llm: %s provider requires base_url` has prefix `llm:` (not `infra/llm.<Type>.<Method>:`) AND **no sentinel** AND **no `%w`** (no inner error to wrap, but lack of sentinel means errmap will hit "unmapped domain error" 500). Reachability: only triggers when caller passes empty BaseURL on a "custom"-like provider whose adapter has no default — user-input path through PUT /api-keys. | LOW | hits "unmapped domain error" alarm + 500 INTERNAL_ERROR if a custom provider is configured without base_url; caller (apikey handler) sees generic 500 instead of clean 400 | introduce `llmdomain.ErrBaseURLRequired` sentinel + register errmap as 400 BAD_REQUEST. Or panic if call sites are guaranteed to validate (but PUT /api-keys flow doesn't validate this — apikey.validateCreate only checks meta.BaseURLRequired, not whether provider has a default). Sentinel preferred. | FOUND |

## Sub-check（必显式，不许 silence）

A.1 §S3 错误吞没:
  - violations: not present
  - rationale: file is pure dispatch — no `_ = err`, no `if err != nil { return nil }`, no defer Close. Error from resolveBaseURL is propagated.

A.2 §S9 detached ctx 终态写:
  - terminal-state writes identified: none
  - 各自 ctx 来源: N/A
  - violations: N/A — file performs no DB writes; Build is a wiring-time constructor, doesn't touch ctx.

A.3 §S15 ID 生成:
  - ID generation calls: none
  - violations: N/A — file generates no business IDs.

A.4 §S16 错误 wrap 格式:
  - violations: site #1 (bare-return loses pkg.Method context — LOW), site #3 (no sentinel, plain `llm:` prefix not `infra/llm.<Func>:` — LOW)

A.5 §S17 sentinel 登记 errmap:
  - sentinels defined: none (file has no `var Err...` declarations)
  - 已登记 errmap: N/A
  - missing: N/A — file defines no sentinels (BUT site #3 should introduce one — flagged as §S16 violation above)
