# audit: backend/internal/infra/llm/sanitizer.go

LOC: 114
Read: full file (lines 1-114)

## Trace

| site# | file:line | snippet | category | classification | reasoning | severity | user_impact | suggested_fix | status |
|---|---|---|---|---|---|---|---|---|---|
| 1 | sanitizer.go:69-71 | `if m.Role == RoleTool { continue }` (drop stray RoleTool message) | A.1 | OK | DOCUMENTED design decision — file header lines 1-25 + inline comment lines 64-68 explain that stray tool messages (no preceding assistant tool_calls in the current run) are silently dropped because "there's no way to repair — the LLM has nothing to anchor it to". This is the canonical §S3 carve-out for "documented intent". The whole sanitizer's purpose is recovery from broken history; silent drop here is the recovery, not a bug. | N-A | — | — | — |
| 2 | sanitizer.go:90-95 | `if t.ToolCallID == "" \|\| !expected[t.ToolCallID] { continue }` (drop ID-mismatched stray) | A.1 | OK | same documented carve-out as #1 — stray-within-run tool message dropped. Comment lines 91-94 documents intent. | N-A | — | — | — |
| 3 | sanitizer.go:103-111 | `for _, tc := range m.ToolCalls { if expected[tc.ID] { out = append(out, LLMMessage{ Role: RoleTool, ToolCallID: tc.ID, Content: "[interrupted: tool call did not complete]" }) } }` | A.1 | OK | synthesize stub for missing pairings — protocol-level recovery, not silent failure. Stub content explicitly tells the LLM what happened. | N-A | — | — | — |

## Sub-check（必显式，不许 silence）

A.1 §S3 错误吞没:
  - violations: not present
  - rationale: file is pure-function pairing-invariant enforcer, not error handling. The "silent drops" at sites #1 and #2 are documented design (recovery from corrupted history; nothing to surface as the input is already broken upstream). Stub synthesis at #3 makes the interruption visible to the model.

A.2 §S9 detached ctx 终态写:
  - terminal-state writes identified: none
  - 各自 ctx 来源: N/A
  - violations: N/A — pure function, no ctx, no DB writes.

A.3 §S15 ID 生成:
  - ID generation calls: none
  - violations: N/A — file generates no IDs (only consumes existing LLMToolCall.ID).

A.4 §S16 错误 wrap 格式:
  - violations: not present
  - rationale: function returns `[]LLMMessage` — no error return. No fmt.Errorf calls.

A.5 §S17 sentinel 登记 errmap:
  - sentinels defined: none
  - 已登记 errmap: N/A
  - missing: N/A — file defines no sentinels.
