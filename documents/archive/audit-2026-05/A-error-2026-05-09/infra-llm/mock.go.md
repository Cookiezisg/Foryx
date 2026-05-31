# audit: backend/internal/infra/llm/mock.go

LOC: 192
Read: full file (lines 1-192)

## Trace

| site# | file:line | snippet | category | classification | reasoning | severity | user_impact | suggested_fix | status |
|---|---|---|---|---|---|---|---|---|---|
| 1 | mock.go:171-174 | `yield(StreamEvent{ Type: EventError, Err: errors.New("mock-llm: queue empty — push a script via /dev/mock-llm/scripts before sending the chat message") })` | A.4 | EDGE | §S16: bare `errors.New(...)` not via sentinel + no `infra/llm.MockClient.Stream:` pkg.Method prefix; **but** this is dev-only mock used by `/dev/mock-llm/*` testing surface — error doesn't need errmap registration (it's a fresh err every call, not a sentinel callers can `errors.Is` against). The error message itself is descriptive + actionable for the dev/tester reading the dev console. | LOW | dev-only path; if mock queue empty during testend testing the LLM trace shows this clear msg — works as designed. Not user-facing in production (mock provider is dev/test only). | optional: introduce package-private sentinel `errMockQueueEmpty` if any test wants to assert on it. Otherwise §S16 strict literal compliance not justified for dev-only path. **Likely WAIVE.** | FOUND |
| 2 | mock.go:181-189 | `for _, ev := range script.Events { select { case <-ctx.Done(): return; default: } if !yield(ev) { return } }` | A.1 | OK | ctx.Done check between yields — clean cancellation. yield returns bool; false (consumer stopped) cleanly returns. No errors to suppress; events flow as-is from MockScript. | N-A | — | — | — |
| 3 | mock.go:177-180 | `if script.ErrAfter != nil { yield(StreamEvent{Type: EventError, Err: script.ErrAfter}); return }` | A.1 | OK | tester-supplied ErrAfter is propagated unchanged via EventError — exactly what the test API contract promises (line 49). No suppression. | N-A | — | — | — |

## Sub-check（必显式，不许 silence）

A.1 §S3 错误吞没:
  - violations: not present
  - rationale: file is dev-only mock; the only error paths are queue-empty (site #1) and tester-injected ErrAfter (site #3). Both surface as EventError, never silently swallowed. ctx.Done check (site #2) is clean cooperative cancel.

A.2 §S9 detached ctx 终态写:
  - terminal-state writes identified: none
  - 各自 ctx 来源: N/A
  - violations: N/A — file performs no DB writes; in-memory queue + counters only.

A.3 §S15 ID 生成:
  - ID generation calls: none
  - violations: N/A — file generates no business IDs.

A.4 §S16 错误 wrap 格式:
  - violations: site #1 (LOW EDGE — dev-only `errors.New` bare; descriptive but not pkg.Method-prefixed sentinel)

A.5 §S17 sentinel 登记 errmap:
  - sentinels defined: none (site #1's error is fresh each call, not a `var Err...` sentinel)
  - 已登记 errmap: N/A
  - missing: N/A — dev-only path; queue-empty error doesn't propagate to handler errmap (chat runner consumes via Stream iterator, surfaces as LLM_STREAM_ERROR).
