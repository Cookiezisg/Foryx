# audit: backend/internal/app/loop/history.go

LOC: 142
Read: full file (lines 1-142)

## Trace

| site# | file:line | snippet | category | classification | reasoning | severity | user_impact | suggested_fix | status |
|---|---|---|---|---|---|---|---|---|---|
| 1 | history.go:40-46 | `func extendHistory(...)` — `msgs, err := BlocksToAssistantLLM(...); if err != nil { return nil, err }` | A.4 | OK | bare-return preserves caller's err untouched; BlocksToAssistantLLM currently returns nil for `err` always (signature future-proofing) so chain is trivial. | N-A | — | — | — |
| 2 | history.go:87-91 | `if json.Unmarshal([]byte(b.Attrs), &attrs) == nil { if v, ok := attrs["tool"].(string); ok { toolName = v } }` (in BlocksToAssistantLLM tool_call branch) | A.1 | EDGE | §S3: Unmarshal err silently dropped — if Attrs JSON is malformed, toolName stays `""`, and the LLM call later sees a tool_call with empty Name (effectively broken). However this is a re-deserializing of in-process generated JSON (stream.go:233 marshaled `{"tool": a.name}` from controlled keys/values), so failure means severe schema/storage drift not user input. No log, no audit trail. | LOW | If DB row's `Attrs` corrupted (manual edit / migration drift), tool name silently goes blank; LLM history shows tool call with empty name → LLM can't identify which tool was called previously. Hard to diagnose. | log at WARN: `s.log.Warn("BlocksToAssistantLLM: malformed tool block Attrs", zap.String("block_id", b.ID), zap.Error(err))` — but this file's free function has no logger. Either thread logger via package-level helper or accept current best-effort behavior. | **FIXED 2026-05-09 26f9c55** (added `*zap.Logger` param to BlocksToAssistantLLM + extendHistory; threaded zap.NewNop in 5 test sites; production caller chat/history.go::blocksToLLM passes s.log) |
| 3 | history.go:88-90 | `if v, ok := attrs["tool"].(string); ok { toolName = v }` — type assertion `_` discard | A.1 | OK | non-error type-assert pattern; `ok bool` is the second return, not an error. §S3 doesn't apply to non-error discards. Same pattern as §S3 spec example. | N-A | — | — | — |
| 4 | history.go:108-111 | `content := b.Content; if content == "" && b.Error != "" { content = b.Error }` (tool_result fallback) | A.1 | OK | not an error site at all — pure data shaping, status="error" with empty Content uses Error field as fallback. Documented intent. | N-A | — | — | — |
| 5 | history.go:134-142 | `func ExtractTextContent` returns `last string` — silent on no-text-blocks (returns "") | A.1 | OK | non-error helper with documented "last text block content" semantics; empty string is the legitimate zero-value when blocks have no text. Caller (chat auto-title / subagent tool_result) handles empty string explicitly. | N-A | — | — | — |

## Sub-check

A.1 §S3 错误吞没:
  - violations: site #2 (LOW) — Unmarshal err silently dropped + no audit log

A.2 §S9 detached ctx 终态写:
  - terminal-state writes identified: none — file is pure conversion logic, no DB writes
  - 各自 ctx 来源: N/A
  - violations: N/A — package's terminal-state-write contract belongs to Host implementations (chatHost / subagentHost), not to history.go

A.3 §S15 ID 生成:
  - ID generation calls: none in this file
  - violations: N/A — file does no ID generation (block IDs come from caller; LLM tc_id reused)

A.4 §S16 错误 wrap 格式:
  - violations: not present — only error path is bare-return at site #1 (canonical for caller-side wrap)

A.5 §S17 sentinel 登记 errmap:
  - sentinels defined: none
  - 已登记 errmap: N/A
  - missing: N/A — file defines no sentinels (pure conversion functions)
