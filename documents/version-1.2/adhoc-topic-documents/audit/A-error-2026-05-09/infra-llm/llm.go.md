# audit: backend/internal/infra/llm/llm.go

LOC: 216
Read: full file (lines 1-216)

## Trace

| site# | file:line | snippet | category | classification | reasoning | severity | user_impact | suggested_fix | status |
|---|---|---|---|---|---|---|---|---|---|
| 1 | llm.go:205-216 | `func Generate(ctx, c Client, req Request) (string, error) { var sb strings.Builder; for event := range c.Stream(ctx, req) { switch event.Type { case EventText: sb.WriteString(event.Delta); case EventError: return "", event.Err } } return sb.String(), nil }` | A.4 | EDGE | bare-return on EventError — `event.Err` is whatever the underlying provider client emitted (openai.go / anthropic.go / mock.go produce raw errors that may or may not have pkg.Method prefixes). Generate adds no `infra/llm.Generate:` wrap context. **Plus**: silent treatment of unknown event types — switch falls through without logging when event.Type is one of EventReasoning / EventToolStart / EventToolDelta / EventFinish (these ARE expected to be silently consumed for non-streaming use) — that part is design-correct, NOT §S3 violation. The bare-return is the only flag here. | LOW | identical UX (the underlying err reaches caller; sentinel chain — if any — preserved). Call-site grep harder; new readers can't tell whether Generate adds context or just propagates. | wrap: `return "", fmt.Errorf("llm.Generate: %w", event.Err)` for grep + propagation visibility. | FOUND |

## Sub-check（必显式，不许 silence）

A.1 §S3 错误吞没:
  - violations: not present
  - rationale: file is mostly type definitions (StreamEventType / Role / LLMMessage / etc. ); only function with error handling is Generate, and EventError is propagated (not swallowed). The non-EventError event types (EventReasoning / EventToolStart / EventToolDelta / EventFinish) silently ignored is design-correct for a non-streaming consumer (they're info, not errors). Comment at lines 199-204 documents intent.

A.2 §S9 detached ctx 终态写:
  - terminal-state writes identified: none
  - 各自 ctx 来源: N/A
  - violations: N/A — file performs no DB writes; ctx flows through to Stream consumer.

A.3 §S15 ID 生成:
  - ID generation calls: none
  - violations: N/A — file generates no business IDs (LLMToolCall.ID is provider-supplied LLM tool-call ID, not Forgify-issued).

A.4 §S16 错误 wrap 格式:
  - violations: site #1 (Generate bare-return — LOW). Inner provider errors propagate without `llm.Generate:` pkg.Method prefix.

A.5 §S17 sentinel 登记 errmap:
  - sentinels defined: none
  - 已登记 errmap: N/A
  - missing: N/A — file defines no sentinels (only StreamEventType + Role string-typed enums).
