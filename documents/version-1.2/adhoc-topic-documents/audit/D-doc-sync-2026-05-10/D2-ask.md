# D2 Doc-Sync Audit — ask

Scope:
- Doc: `documents/version-1.2/service-design-documents/ask.md`
- Code: `backend/internal/app/ask/` + `backend/internal/app/tool/ask/` + `backend/internal/transport/httpapi/handlers/answers.go`
- Note: doc explicitly states "无 entity / 无持久化", confirmed: no `internal/domain/ask/` directory exists. ✅

D1 already covered contract-level errmap entries; below: design-doc-vs-code drifts only.

---

## In code but not in doc

| Item | Code location | Severity |
|---|---|---|
| Handler method is named `Submit`, not `Post` as doc §6 says | `backend/internal/transport/httpapi/handlers/answers.go:71` | LOW |
| `Submit` does an explicit early `INVALID_REQUEST` check on empty `req.ToolCallID` (responds 400 with hand-rolled message before svc.Resolve). Doc §6 handler pseudo-code skips this branch. | `answers.go:77-80` | LOW |
| `pendingCount()` test-only helper is exported package-internal but not in doc | `internal/app/ask/ask.go:167-171` | LOW |

## In doc but not in code (stale)

| Item | Doc location | Severity |
|---|---|---|
| **§2 端到端推演** lists `pkg/reqctx.RequireConversationID — handler 校验 path 中 conv 存在（与 §S14 一致）`. Handler does **NOT** call `RequireConversationID` — it ignores `convID` entirely (no `r.PathValue("id")` call, no validation). The path captures `{id}` but the handler reads only `req.ToolCallID` from body. Doc §6 handler pseudo-code line 217 also says `convID := pathParam(r, "id")` — code does **NOT** read it. | ask.md:54, 217 | **MED** |
| **§4 Service struct** doc shows `func (s *Service) Wait(ctx context.Context, toolCallID string, timeout time.Duration) (string, error)` — matches. But §4.1 Wait code excerpt shows `select { case ans := <-ch: return ans, nil; case <-timer.C: return "", ErrTimeout; case <-ctx.Done(): return "", ctx.Err() }` — code matches but the order in code is `case ans := <-ch / case <-timer.C / case <-ctx.Done()` (lines 107-113). ✅ | — | OK |
| **§5.1 ValidateInput sentinel `ErrEmptyQuestion` — question 缺 / 空 / 仅空白** (line 177). Code `ValidateInput` only fails when `strings.TrimSpace(a.Question) == ""` after Unmarshal — JSON unmarshal err returns wrapped `fmt.Errorf("AskUserQuestion.ValidateInput: %w", err)` not `ErrEmptyQuestion`. Doc says only `ErrEmptyQuestion` is the sentinel; doc misses the second possible failure mode (malformed JSON args). | ask.md:177 vs ask.go:119-129 | LOW |
| **§5.2 AskTools 工厂** doc shows `&AskUserQuestion{svc: svc, timeout: defaultTimeout}` initialiser. Match. | — | OK |
| **§9 测试覆盖** table line "Pipeline | `backend/test/uxtask/uxtask_test.go::TestUxTask_AskUserQuestionAnswerDelivered` + `_AnswerEndpoint_UnknownCallID_404` | 2 场景". Reality: dir is `backend/test/uxtodo/`, file is `uxtodo_test.go`, function name is `TestUxTodo_AnswerEndpoint_UnknownCallID_404` (not `TestUxTask_*`). Test renamed during task→todo rename; doc not updated. | ask.md:294 | **MED** |
| **§10 与其他 domain 的关系** row `agentstate` says "不依赖". Match. ✅ | — | OK |
| **§S20 sentinel `ErrAlreadyAnswered`** — doc §4.3 + §7 retain it for "字典完整性" "concept-by-people-readable". Code line 50 indeed defines + comments "保留导出". `errmap.go:185` registers `ASK_ALREADY_ANSWERED`. Match. | — | OK |
| **§3 决策表 row "HTTP 端点 RESTful"** says "`POST /api/v1/conversations/{id}/answers` body 含 toolCallId + answer ... 当前不强制校验 callID 属于该 conv". Code matches: Resolve only takes `(toolCallID, answer)`, no conv-scoping. ✅ | — | OK |

## Mismatched (different details)

| Item | Code | Doc | Severity |
|---|---|---|---|
| Handler method name | `Submit` (`answers.go:71`) | doc §6 calls it `AnswerHandler.Post` (lines 217, 221) | **MED** |
| `Submit` empty-ID branch | Hand-rolls 400 INVALID_REQUEST when `req.ToolCallID == ""` | Doc §6 pseudo-code shows only `decodeJSON → svc.Resolve → response.NoContent`; missing the empty-ID gate | LOW |
| **§4.3 sentinel comment** | `ErrAlreadyAnswered` comment says "保留导出，当前不再产生" — code line 50 comment is "Resolve was called twice for the same tool_call ID. The first answer is the answer of record." (slightly different framing — older intent). Match in spirit. | LOW |
| **§5.1 timeout** | `defaultTimeout = 5 * time.Minute` (`tool/ask/ask.go:46`) | Doc says 5 分钟 + 测试时可覆盖到 500ms — confirmed: `timeout` field is overridable on the struct. Match. | OK |
| **§4 Service struct** | `pending map[string]chan string` | Doc says `pending map[string]chan string` — match | OK |
| Pipeline test path | `backend/test/uxtodo/uxtodo_test.go:153` `TestUxTodo_AnswerEndpoint_UnknownCallID_404` | doc says `backend/test/uxtask/uxtask_test.go::TestUxTask_AskUserQuestionAnswerDelivered` + `_AnswerEndpoint_UnknownCallID_404`. **Filename + dir + test prefix all stale.** No `uxtask` dir; no `TestUxTask_AskUserQuestionAnswerDelivered` symbol. | ask.md:294 | **MED** |
| **§5.1 Args description** | Code `Description()` is the literal multi-line `askDescription` constant (lines 59-65) covering question + options + 5min behavior + "Use this when you genuinely need user input" | Doc §5.1 Args table has 2 rows (question/options); silent on rest | LOW |
| **§4.3 sentinels list — only 3** | Code defines exactly 3 (`ErrNoPendingQuestion` / `ErrAlreadyAnswered` / `ErrTimeout`) plus tool-side `ErrEmptyQuestion`. Match. | OK |

## Sub-check

- **Entities aligned**: **N/A** — domain has no entities (verified: no `domain/ask/` directory; doc §1 explicitly says so).
- **Service methods aligned**: **Yes** — `NewService / Wait / Resolve / cleanup / pendingCount` (5 methods, 3 public). Doc §4 covers `NewService / Wait / Resolve` (3) — `cleanup` is an internal helper noted at §8.2; `pendingCount` is unmentioned (test-only). Match modulo internal helpers.
- **Endpoints aligned**: **Yes** — single endpoint `POST /api/v1/conversations/{id}/answers` registered (`answers.go:47`); D1 contract-doc audit covered this.
- **Sentinels aligned**: **Yes** — 3 service-side + 1 tool-side sentinel in code; 3 service-side documented in §4.3 + §7, tool-side `ErrEmptyQuestion` in §5.1. All 3 service-side mapped in `errmap.go:184-186`.
- **端到端推演 valid**: **Mostly** — §2 chain accurate at layer-flow level. Drifts:
  - (a) `pkg/reqctx.RequireConversationID — handler 校验 path 中 conv 存在` claim is FALSE — handler ignores convID entirely (single-user reality, but doc lies about the wiring).
  - (b) Handler method name `Post` should be `Submit`.
  - (c) Doc §6 handler skeleton does not show the explicit empty-ID 400 gate.

---

## File-naming / location drift summary

- **Pipeline test mis-cited**: dir + file + test-prefix all stale (uxtask → uxtodo rename in 2026-05-05 per CLAUDE.md project-special note "对话级 TODO，2026-05-05 由原 `tk_` task 改名"). The §9 reference predates that rename.
- **Handler method name** drift (`Post` vs `Submit`) is a low-impact rename but appears twice in §6 doc.

---

**Totals:** 0 HIGH / 4 MED / 5 LOW
