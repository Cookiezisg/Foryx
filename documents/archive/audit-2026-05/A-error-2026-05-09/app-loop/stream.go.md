# audit: backend/internal/app/loop/stream.go

LOC: 301
Read: full file (lines 1-301)

## Trace

| site# | file:line | snippet | category | classification | reasoning | severity | user_impact | suggested_fix | status |
|---|---|---|---|---|---|---|---|---|---|
| 1 | stream.go:62 | `em := eventlogpkg.From(ctx)` | A.1 | OK | non-error helper; `From` returns no-op when ctx lacks emitter (per pkg/eventlog contract). Safe nil-tolerant. | N-A | — | — | — |
| 2 | stream.go:63 | `msgID, _ := reqctxpkg.GetMessageID(ctx)` | A.1 | OK | non-error multi-return; second value is `ok bool`, intentionally discarded — caller checks `msgID != ""` at every emit site (lines 88, 99, 118). Idiomatic Go. §S3 doesn't apply to non-error discards. | N-A | — | — | — |
| 3 | stream.go:69-79 | `closeText := func(status string) { if textBlockID != "" { em.StopBlock(ctx, textBlockID, status, nil) ... } }` (and closeReason similarly) | A.1 | OK | em.StopBlock returns no error per pkg/eventlog contract; pure side-effect call. | N-A | — | — | — |
| 4 | stream.go:144-153 | `case llminfra.EventError: if ctx.Err() != nil { stopReason = chatdomain.StopReasonCancelled } else { stopReason = chatdomain.StopReasonError; if event.Err != nil { errMsg = event.Err.Error() } }` | A.1 | OK | EventError is the producer's error path; ctx-aware classification (cancel vs error); errMsg captured for downstream WriteFinalize. No swallow. | N-A | — | — | — |
| 5 | stream.go:177-179 | `if ctx.Err() != nil && stopReason == chatdomain.StopReasonEndTurn { stopReason = chatdomain.StopReasonCancelled }` | A.1 | OK | post-loop cancel detection — covers stream that ended cleanly but ctx was cancelled before EventError arrived. Good defensive check. | N-A | — | — | — |
| 6 | stream.go:202-244 | `func assembleBlocks(...) []chatdomain.Block { ... }` — generates 3 block types | A.3 | — | see sites 7, 8 | N-A | — | — | — |
| 7 | stream.go:207, 216 | `idgenpkg.New("blk")` (reasoning + text blocks) | A.3 | OK | §S15 canonical: `idgenpkg.New("blk")` matches spec list ("blk_" block); idgenpkg internally panics on rand.Read fail per §S15. | N-A | — | — | — |
| 8 | stream.go:235 | `ID: a.id` (tool_call block reuses LLM tool-call ID) | A.3 | OK | per design (event-log-protocol.md §3 / file comment lines 39-46): tool_call block ID = LLM tc_id. NOT a §S15 violation because LLM-supplied IDs are external — §S15 governs IDs we mint internally. The §S15 prefix list explicitly notes this exception by reference. | N-A | — | — | — |
| 9 | stream.go:232-233 | `argsJSON, _ := json.Marshal(args)` and `attrsJSON, _ := json.Marshal(map[string]any{"tool": a.name})` | A.1 | EDGE | §S3: Marshal err discarded twice. Both inputs are programmatically constructed: `args` is `map[string]any` from parseToolArgs (line 287-289 returns either Unmarshal'd args or `{"raw": raw}` — both JSON-marshalable basic types); `attrsJSON` marshals `{"tool": string}` — string never fails. So Marshal here CANNOT realistically fail (the only path would be unsafe.Pointer / channel / func types, which can't enter via parseToolArgs). However, no inline comment explains why the err is safe to drop. | LOW | none in practice — inputs are tightly typed. | add inline comment: `_ = err — args/attrs are programmatically built map[string]any of basic types; Marshal cannot fail`. Or change to `argsJSON, _ := json.Marshal(args) // safe: args is map[string]basic-types`. Style polish; not a real bug. | **FIXED 2026-05-09 505d6e3** (added inline comment documenting unfailable basic-type Marshal) |
| 10 | stream.go:281-291 | `func parseToolArgs(raw string) (...) { ... if err := json.Unmarshal([]byte(stripped), &args); err != nil \|\| args == nil { return fields, map[string]any{"raw": raw} } ... }` | A.1 | OK | §S3: Unmarshal err ALSO triggers fallback, but fallback is documented intent (lines 275-280: "JSON 损坏时把原文塞 args[\"raw\"] 让 LLM 仍能看到自己发了什么") and the fallback IS visible to the LLM (it sees its own malformed input echoed back, can self-correct). This is the canonical "soft-degrade with audit trail" §S3 example. | N-A | — | — | — |
| 11 | stream.go:295-301 | `func sortInts(a []int) { ... }` | A.1 | OK | pure helper, no errors. | N-A | — | — | — |

## Sub-check

A.1 §S3 错误吞没:
  - violations: site #9 (LOW — Marshal err discarded with no audit comment, though inputs are safe by construction)

A.2 §S9 detached ctx 终态写:
  - terminal-state writes identified: none — stream.go performs em.* emits (eventlog Bridge) and accumulates blocks in memory; no DB writes
  - 各自 ctx 来源: emit calls all use the request `ctx`
  - violations: N/A — em.* methods are not "terminal-state writes" in the §S9 sense. They're real-time event publishes that ride the request lifetime intentionally; if ctx cancels, the emit stops mid-stream which is correct (downstream block_stop is emitted at the close in lines 164-175 covering the cancelled status). The eventlog Bridge's ring buffer + replay handles client reconnection. DB persistence is at host.WriteFinalize (covered separately in loop.go).

A.3 §S15 ID 生成:
  - ID generation calls: `idgenpkg.New("blk")` at lines 89, 100, 207, 216 (4 call sites for text + reasoning block IDs)
  - violations: not present — all use `idgenpkg.New(prefix)`; "blk" prefix matches §S15 spec list; LLM-tool-call-ID reuse at line 235 is by design exception (event-log-protocol.md §3)

A.4 §S16 错误 wrap 格式:
  - violations: not present — file makes no fmt.Errorf calls. Only error captured is `event.Err.Error()` → string into errMsg field (line 150), which is for UI-facing error message not domain error chain.

A.5 §S17 sentinel 登记 errmap:
  - sentinels defined: none
  - 已登记 errmap: N/A
  - missing: N/A — file defines no sentinels (it's a pure stream consumer, returns blocks + stop info via plain types not error)
