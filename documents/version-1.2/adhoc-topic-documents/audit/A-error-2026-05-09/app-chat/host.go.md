# audit: backend/internal/app/chat/host.go

LOC: 129
Read: full file (lines 1-129)

## Trace

| site# | file:line | snippet | category | classification | reasoning | severity | user_impact | suggested_fix | status |
|---|---|---|---|---|---|---|---|---|---|
| 1 | host.go:39-41 | `func (h *chatHost) LoadHistory(ctx) ... { return h.svc.buildHistory(ctx, h.convID, h.userMsgID) }` | A.4 | OK | direct passthrough; buildHistory wraps internally | N-A | — | — | — |
| 2 | host.go:43-45 | `func (h *chatHost) Tools() ...` | A.1 | OK | pure getter; no error path | N-A | — | — | — |
| 3 | host.go:54-55 | `saveCtx := reqctxpkg.SetUserID(context.Background(), h.uid); saveCtx = reqctxpkg.WithConversationID(saveCtx, h.convID)` | **A.2** | **OK (POSITIVE EXAMPLE — TEXTBOOK §S9)** | **§S9 textbook compliance**: terminal-state write detached from request ctx. Comment at lines 47-53 explicitly explains: "Detached context: a cancelled upstream stream must not block the terminal write OR the message_stop emit". Re-stamps both uid (for user-scoped store) and convID (for emit routing). Cited in chat.md §6 line 358 + §11 line 1011 as the canonical pattern. | N-A | preserve as reference for §S9 implementations | — | — |
| 4 | host.go:58-67 | `if err := h.svc.repo.SaveMessage(saveCtx, msg); err != nil { h.svc.log.Error("CRITICAL: final assistant message persist failed — message lost", ...); msg.Status = chatdomain.StatusError; msg.StopReason = chatdomain.StopReasonError; msg.ErrorCode = "INTERNAL_ERROR"; msg.ErrorMessage = "failed to save assistant message to database" }` | A.1 | OK | error not "swallowed" — logged at ERROR with full context (msg_id, conversation_id, err) AND msg fields are mutated to reflect the persistence failure so the downstream eventlog StopMessage at site #5 emits status=error to subscribers. This is the §S3-compliant pattern: the error has both an audit trail (zap.Error log) and a user-visible signal (status=error in the SSE stream). The fact that the assistant message text is gone is itself reported via SSE. WriteFinalize is best-effort by loop.Host contract — it doesn't return error. | N-A | — | — | — |
| 5 | host.go:75-77 | `h.svc.emitter.StopMessage(saveCtx, h.msgID, mapEventLogStatus(msg.Status), msg.StopReason, msg.ErrorCode, msg.ErrorMessage, msg.InputTokens, msg.OutputTokens)` | **A.2** | **OK (POSITIVE EXAMPLE)** | uses detached `saveCtx` (not request ctx) per §S9 — the emit must reach SSE subscribers even when upstream was cancelled. Comment at lines 69-74: "Bridge.Publish 在订阅者（SSE 流）拿到 message_stop 前触发 ctx.Done 早退". Emitter is fire-and-forget by spec; no return value to check. | N-A | — | — | — |
| 6 | host.go:78 | `_ = ctx // legacy param retained for loop.Host signature` | A.1 | OK | `_ = ctx` with inline comment explaining why the request ctx is intentionally NOT used (would defeat detached-ctx pattern). §S3 example: "_ ignore must have inline comment explaining why" — compliant. | N-A | — | — | — |
| 7 | host.go:86 | `_ = blocks` (unused parameter, blocks param of WriteFinalize) | A.1 | OK | `_ = blocks` with comment block at lines 80-85 explaining why blocks param exists (loop.Host interface compliance) but isn't used (blocks already written in real-time via emit). §S3-compliant ignored value. | N-A | — | — | — |
| 8 | host.go:92-103 | `func mapEventLogStatus(s string) string { switch s { case StatusStreaming: ... default: return eventlogdomain.StatusCompleted } }` | A.1 | EDGE | default returns Completed — not "silent fail". Status mapping is total over chatdomain status values (4 known: Streaming/Completed/Error/Cancelled, plus Pending). For unknown input the default-Completed is the most-permissive choice but **doesn't preserve "what was the unknown value"** — possible §S3 silent path if a future status is added without updating this switch. However the input domain is fully under chatdomain, and any drift would be caught by eventlog protocol violation downstream. Borderline. | LOW | unlikely; only triggers if a new chatdomain.Status* is added without updating mapEventLogStatus — caught quickly by chat tests | could log a WARN on default branch: `panic` is too aggressive (status mapping shouldn't crash request); WARN log + return Completed acceptable | **FIXED 2026-05-10** — converted mapEventLogStatus from free function to chatHost method so it can access the service logger. Added explicit cases for Completed + Pending so the default branch truly catches drift. Default now logs Warn with status + msg_id. |
| 9 | host.go:111-129 | `func buildMessage(...) *chatdomain.Message { return &chatdomain.Message{...} }` | A.1 | OK | pure constructor; no error path; no ID generation (msgID passed in from caller) | N-A | — | — | — |

## Sub-check

A.1 §S3 错误吞没:
  - violations: site #8 (mapEventLogStatus default branch silently returns Completed for unknown input — LOW)

A.2 §S9 detached ctx 终态写:
  - terminal-state writes identified: site #4 (SaveMessage — terminal assistant message persistence) and site #5 (StopMessage emit — terminal SSE event)
  - 各自 ctx 来源: both use `saveCtx` derived at site #3 from `context.Background()` via `reqctxpkg.SetUserID(...)` + `WithConversationID(...)` — fully detached from the request ctx that loop.Run was invoked under
  - violations: **not present** — this file is the canonical §S9 implementation example

A.3 §S15 ID 生成:
  - ID generation calls: none in this file
  - violations: N/A — host.go uses msgID passed in from caller (chat.Service.Send generated it via newMsgID); not generating new IDs

A.4 §S16 错误 wrap 格式:
  - violations: not present — site #1 is direct passthrough; site #4's error doesn't return (logged + status mutated). No fmt.Errorf in this file.

A.5 §S17 sentinel 登记 errmap:
  - sentinels defined: none
  - 已登记 errmap: N/A — host.go uses constants from chatdomain (StatusError / StopReasonError) but no sentinel definitions
  - missing: N/A — file defines no new sentinels
