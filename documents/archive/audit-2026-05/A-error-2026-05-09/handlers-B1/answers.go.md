# answers.go — audit trace

**Path**: `backend/internal/transport/httpapi/handlers/answers.go`
**LOC**: 87
**Role**: `AnswerHandler` for `POST /api/v1/conversations/{id}/answers` — closes the loop from user's UI answer back to the blocked AskUserQuestion tool. Decision D11: no separate event family; question rides chat.message SSE.

## 9-col trace

| site# | file:line | snippet | category | classification | reasoning | severity | user_impact | suggested_fix | status |
|---|---|---|---|---|---|---|---|---|---|
| 1 | answers.go:71-76 | `var req answerRequest; if err := decodeJSON(r, &req); err != nil { responsehttpapi.FromDomainError(w, h.log, err); return }` | A.1 | OK | Standard decodeJSON → ErrInvalidRequest → 400 envelope path. | N-A | — | — | — |
| 2 | answers.go:77-81 | `if req.ToolCallID == "" { responsehttpapi.Error(w, http.StatusBadRequest, "INVALID_REQUEST", "toolCallId is required", nil); return }` | A.1 | EDGE | Handler-side required-field check returns 400 directly via `responsehttpapi.Error` (not via FromDomainError → sentinel). Per §S6 thin handler this is borderline business logic in transport layer; per §6 反校验剧场 it's defensible because (a) `Resolve("", "")` would map to 404 ASK_NO_PENDING_QUESTION which is a misleading wire code for a malformed-body case, (b) explicit "toolCallId is required" gives the FE a clearer error than the generic sentinel. The check is **input shape**, not business validation — closer to JSON schema than to domain logic. Note: `req.Answer == ""` is **not** rejected (intentional? Empty answer may be valid UX — user pressed Cancel). Consider making the answer-empty semantics explicit somewhere. | LOW | None for the current FE flow; only matters if a buggy client sends `{}` — they get a clear 400 vs. confusing 404. | (a) Add an `ErrToolCallIDRequired` sentinel to askapp + errmap row so handler stays thin; OR (b) leave as-is and add a brief inline comment justifying the manual check. | WAIVED (审 input-shape 校验属 JSON-schema 层而非业务，§N1 + §S6 都不视为 violation；引入 sentinel 仅给一个调用点登记 = boilerplate 多于价值) |
| 3 | answers.go:82-86 | `if err := h.svc.Resolve(req.ToolCallID, req.Answer); err != nil { responsehttpapi.FromDomainError(w, h.log, err); return }; responsehttpapi.NoContent(w)` | A.1/A.5 | OK | `Resolve(toolCallID, answer)` is in-memory rendezvous (ask/ask.go:135 — pure channel send, no ctx, no DB). All 3 ask sentinels registered in errmap (errmap.go:175-177): `ErrNoPendingQuestion` 404 / `ErrAlreadyAnswered` 409 / `ErrTimeout` 504. `Resolve` does NOT take ctx — that's a service-layer design choice (in-memory channel, no IO), so handler not passing ctx is correct. NoContent envelope = 204 (per §N2 "204 删除" pattern; here action complete with no body). | N-A | — | — | — |

## Sub-checks

**A.1 §S3 错误吞没**:
- violations: not present (every error path produces an envelope; no `_` discards, no silent fallback)

**A.2 §S9 detached ctx 终态写**:
- terminal-state writes identified: site 3 `Resolve` delivers answer via channel (in-memory)
- 各自 ctx 来源: N/A — Resolve takes no ctx parameter (ask/ask.go:135) because rendezvous is in-process channel send (cap 1, never blocks, no IO)
- violations: N/A: there is no DB write at all in this handler path. The "terminal state" is entirely on the receiving Wait() side (the tool is blocked there); ctx semantics there are ask service concern, not this handler.

**A.3 §S15 ID 生成**:
- ID generation calls: none
- violations: N/A: handler does not mint IDs; toolCallId is supplied by the caller (originally from the LLM tool_call_id, see §S21 invariant)

**A.4 §S16 错误 wrap 格式**:
- violations: not present (handler does not wrap; forwards via `FromDomainError`)

**A.5 §S17 sentinel 登记 errmap**:
- sentinels defined: none in this file
- 已登记 errmap (consumed transitively):
  - `errorsdomain.ErrInvalidRequest` — errmap.go:44 (via decodeJSON)
  - `askapp.ErrNoPendingQuestion` — errmap.go:175
  - `askapp.ErrAlreadyAnswered` — errmap.go:176
  - `askapp.ErrTimeout` — errmap.go:177
- missing: none — all 3 ask sentinels are registered (errmap comment "ask service (AskUserQuestion answer-delivery handler)" at errmap.go:173-177 is exactly this handler)

## Summary

- Sites: 3
- Violations: 1 LOW (site 2 — handler-side required-field check; EDGE per §6 反校验剧场, classified LOW because user impact is purely wire-code clarity for buggy client)
- Verdict: clean handler. Single LOW-severity question on whether the inline `req.ToolCallID == ""` check should be lifted to a sentinel. Current behavior is functionally correct and gives clearer error code than letting it cascade to ErrNoPendingQuestion. Authorial choice; documenting the trade-off would be nice but not a §S3-§S17 violation.
