# audit: backend/internal/infra/llm/adapter.go

LOC: 300
Read: full file (lines 1-300)

## Trace

| site# | file:line | snippet | category | classification | reasoning | severity | user_impact | suggested_fix | status |
|---|---|---|---|---|---|---|---|---|---|
| 1 | adapter.go:91-92 | `func (baseAdapter) BeforeRequest(*Request) {}` + `AfterStreamEvent` returns `[]StreamEvent{ev}` | A.1 | OK | no-op default implementations; no error returns by interface design — pure transformation hooks. §S3 doesn't apply (no error to suppress). | N-A | — | — | — |
| 2 | adapter.go:264-271 | `func lookupAdapter(name string) Adapter { for _, a := range adapters { if a.Name() == name { return a } } return openaiAdapter{} }` | A.1 | OK | unknown provider → falls back to OpenAI baseline. **DOCUMENTED at lines 257-263** — explicit design decision so user typos / new untested providers stay functional. Returns concrete value not error; not §S3 silent (downstream wire client will surface real provider mismatch as HTTP error from upstream). | N-A | — | — | — |
| 3 | adapter.go:288-300 | `adapterWrappedClient.Stream` — passes through `c.inner.Stream(ctx, req)`, transforms events; no error path | A.1 | OK | iterator-based pass-through; underlying error events flow through StreamEvent.Type=`error` rather than Go error returns. §S3 doesn't apply (no error to suppress at this layer). | N-A | — | — | — |

## Sub-check（必显式，不许 silence）

A.1 §S3 错误吞没:
  - violations: not present
  - rationale: file is pure provider-config registry + no-op hooks + iterator pass-through. No `_ = err`, no `if err != nil { return nil }`, no defer Close. The fallback at site #2 is documented explicit choice, not silent failure.

A.2 §S9 detached ctx 终态写:
  - terminal-state writes identified: none
  - 各自 ctx 来源: N/A
  - violations: N/A — file performs no DB / persistent writes; ctx is only passed through to inner Stream client.

A.3 §S15 ID 生成:
  - ID generation calls: none
  - violations: N/A — file generates no business IDs.

A.4 §S16 错误 wrap 格式:
  - violations: not present
  - rationale: file makes ZERO `fmt.Errorf` calls. No errors propagate from this layer.

A.5 §S17 sentinel 登记 errmap:
  - sentinels defined: none
  - 已登记 errmap: N/A
  - missing: N/A — file defines no sentinels.
