# audit: backend/internal/app/loop/loop.go

LOC: 183
Read: full file (lines 1-183)

## Trace

| site# | file:line | snippet | category | classification | reasoning | severity | user_impact | suggested_fix | status |
|---|---|---|---|---|---|---|---|---|---|
| 1 | loop.go:83-85 | `if log == nil { log = zap.NewNop() }` | A.1 | OK | nil-tolerant log fallback; not an error site. Defensive against test wiring. | N-A | — | — | — |
| 2 | loop.go:87-93 | `history, err := host.LoadHistory(ctx); if err != nil { host.WriteFinalize(ctx, nil, ..., "INTERNAL_ERROR", "load history: "+err.Error(), 0, 0); return Result{...} }` | A.1/A.4 | EDGE | §S3: err is consumed by writing it into errMsg field of the terminal Message — log audit trail comes from the `host.WriteFinalize` SaveMessage call (errMsg recorded in DB) AND chat's chatHost will log Error on failure to persist. So error IS surfaced (user sees message_status=error in UI with code INTERNAL_ERROR + message). However: (a) original err is `string-concatenated` not `%w`-wrapped — sentinel chain breaks at this point. (b) "INTERNAL_ERROR" is a string literal not a sentinel. The errMsg+errCode pair eventually surfaces in UI but doesn't propagate sentinels for `errors.Is` tests. | LOW | minimal — caller sees Result.Status=error and UI shows the error message. Sentinel-chain breakage doesn't matter past this point because Run() returns Result not error. | accept current pattern (this is a UI-facing error format, not a sentinel chain). Could optionally log with `log.Error("loop: load history failed", zap.Error(err))` for separate operator audit, since the user-facing message hides original err structure. | **WAIVED 2026-05-10** — UI-facing errMsg field, not a sentinel chain context; audit-recommended waive (see _summary recommended-fix #4). |
| 3 | loop.go:116 | `aBlocks, toolCalls, sr, em, iT, oT := streamLLM(ctx, client, req)` — multi-return; `em` is errMsg string from streamLLM | A.1 | OK | non-error multi-return; streamLLM's contract returns errMsg as `string` (not `error`). Used at line 129 (errMsg = em) when stopReason indicates error. No discard. | N-A | — | — | — |
| 4 | loop.go:124-134 | `if stopReason == ...Cancelled \|\| stopReason == ...Error { ... host.WriteFinalize(ctx, allBlocks, status, stopReason, errCode, errMsg, ...); finalWritten = true; break }` | A.2 | OK | stopReason driven by streamLLM's ctx.Err() check (stream.go:145-152); when ctx is cancelled, host.WriteFinalize is called. Host MUST detach ctx internally per Host interface contract (loop.go:46-47 doc). loop's responsibility is to call WriteFinalize; detached-ctx-for-persist is host's responsibility. | N-A | — | — | — |
| 5 | loop.go:137 | `host.WriteFinalize(ctx, allBlocks, ...Completed, stopReason, "", "", ...)` (no-tool-calls branch) | A.2 | OK | same as #4 — ctx passed through, host detaches internally. | N-A | — | — | — |
| 6 | loop.go:142 | `rBlocks := runTools(ctx, toolCalls, byName, log)` — runTools never errors per its contract | A.1 | OK | runTools converts every failure to ok=false tool_result blocks (per tools.go:78-81 contract); no error to handle. | N-A | — | — | — |
| 7 | loop.go:145-154 | `history, err = extendHistory(history, aBlocks, rBlocks); if err != nil { log.Error("extend history failed", zap.Error(err)); ... host.WriteFinalize(ctx, allBlocks, ...Error, ..., errCode, errMsg, ...) }` | A.1/A.4 | OK | err is logged (zap.Error) AND propagated as errCode + errMsg into the terminal Message via WriteFinalize. Both audit trail (operator log) + user-facing path (DB row) covered. errCode is "HISTORY_EXTEND_FAILED" — string literal, not a sentinel — but doesn't reach errmap (loop returns Result, not error). | N-A | — | — | — |
| 8 | loop.go:159-162 | `if !finalWritten { stopReason = ...MaxTokens; host.WriteFinalize(ctx, allBlocks, ...Completed, stopReason, "", "", ...) }` (max-steps fall-through) | A.2 | OK | terminal write at exhausted-step boundary. ctx → host (host detaches). Status is Completed even though hit max — that's a design choice (not bug). | N-A | — | — | — |
| 9 | loop.go:177-183 | `func toolsByName(tools []toolapp.Tool) map[string]toolapp.Tool { m := make(...); for _, t := range tools { m[t.Name()] = t }; return m }` | A.1 | OK | pure helper, no errors, no I/O. | N-A | — | — | — |

## Sub-check

A.1 §S3 错误吞没:
  - violations: not present (site #2 is EDGE — err makes it into terminal Message via errMsg field, so user does see it; no log audit at the loop layer is the only quibble)

A.2 §S9 detached ctx 终态写:
  - terminal-state writes identified: sites #2, #4, #5, #7, #8 (all `host.WriteFinalize` calls)
  - 各自 ctx 来源: all pass `ctx` (the request ctx) through to host
  - violations: not present — Host interface contract (loop.go:46-47) explicitly delegates detached-ctx duty to Host implementations. loop.go is correct: it wouldn't be appropriate for the engine to derive a detached ctx because it doesn't have user identity (Host owns that). chatHost (chat/host.go) and subagentHost (subagent/host.go) are responsible for the detach.

A.3 §S15 ID 生成:
  - ID generation calls: none in this file
  - violations: N/A — loop.go orchestrates only; ID generation is in stream.go (idgenpkg.New("blk")) and tools.go (idgenpkg.New("blk"))

A.4 §S16 错误 wrap 格式:
  - violations: not present — file makes no `fmt.Errorf` calls. The one error-stringification (site #2 `"load history: "+err.Error()`) is for UI-facing errMsg field, not for domain error chain. Result is not `error`-typed.

A.5 §S17 sentinel 登记 errmap:
  - sentinels defined: none
  - 已登记 errmap: N/A
  - missing: N/A — file defines no sentinels (engine returns Result struct, not error). The string codes "INTERNAL_ERROR" / "HISTORY_EXTEND_FAILED" / "LLM_STREAM_ERROR" are stored in Message.ErrorCode; UI maps them to display strings independently of errmap.go.
