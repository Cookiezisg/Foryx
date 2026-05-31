# D2 chat — service-design-documents/chat.md ↔ code gap report

Date: 2026-05-10
Audited doc: `documents/version-1.2/service-design-documents/chat.md` (1121 lines)
Code surface:
- `backend/internal/domain/chat/chat.go`
- `backend/internal/app/chat/{chat,runner,host,history,util}.go`
- `backend/internal/infra/store/chat/chat.go`
- `backend/internal/transport/httpapi/handlers/chat.go`
- `backend/internal/transport/httpapi/router/router.go`

The chat domain is the most stale of any service-design doc audited so far. The doc was authored before two major refactors landed: (a) the event-log protocol unification (Phase 1-3, 2026-05-08) which replaced the entity-state `chat.message` SSE event with the recursive 5-event × 6-block-type protocol and (b) the loop extraction (chat → loop) which moved streamLLM / runTools out of `app/chat/` to `app/loop/`. The doc still describes both as "in flight" or "Phase 6 entity-state" — code is well past that.

---

## In code but not in doc

| Item | Code location | Severity |
|---|---|---|
| `Message.ParentBlockID` field (subagent sub-runs reference parent block via this column) | `domain/chat/chat.go:54` | HIGH |
| `Message.Attrs` JSON field (carries user-message attachment refs + sub-run kind/type/maxTurns) | `domain/chat/chat.go:62` | HIGH |
| New `Block` struct schema with `ConversationID` / `ParentBlockID` / `Seq int64` (not int) / `Attrs` (renamed from Data) / `Content` (raw text, no JSON wrapper) / `Status` / `Error` / `UpdatedAt` columns | `domain/chat/chat.go:110-123` | HIGH |
| Block has GORM CHECK constraints on `type` (6 values) + `status` (4 values) declared via tags | `domain/chat/chat.go:116,119` | HIGH |
| Block has unique index `idx_blocks_conv_seq (conversation_id, seq)` | `domain/chat/chat.go:112,115` | HIGH |
| Block has indexes `idx_blocks_message_id` + `idx_blocks_parent_block_id` | `domain/chat/chat.go:113,114` | MED |
| `chatdomain.ErrMessageNotFound` sentinel — registered in errmap as `MESSAGE_NOT_FOUND` 404 | `domain/chat/chat.go:228` + `errmap.go:61` | MED |
| `chatdomain.ErrBlockNotFound` sentinel (defined but no errmap entry — internally surfaced by AppendDelta/FinalizeStop/GetBlock) | `domain/chat/chat.go:229` | LOW |
| `Repository.SaveMessage` (replaces doc's `repo.Save`) | `domain/chat/chat.go:255` | HIGH |
| `Repository.GetMessage` | `domain/chat/chat.go:262` | MED |
| `Repository.SaveBlock` (block_start + block_stop write path) | `domain/chat/chat.go:279` | HIGH |
| `Repository.AppendDelta` (NOT `AppendBlockContent` as doc claims §X) | `domain/chat/chat.go:286` | HIGH |
| `Repository.FinalizeStop` (NOT `FinalizeBlock` as doc claims §X) | `domain/chat/chat.go:293` | HIGH |
| `Repository.GetBlock` | `domain/chat/chat.go:298` | MED |
| `Repository.ListBlocksByConversation` (history replay endpoint backend) | `domain/chat/chat.go:305` | MED |
| `Repository.ListBlocksByMessage` | `domain/chat/chat.go:310` | LOW |
| `Repository.ReplayEventsAfter` (full block-as-events stream reconstruction) | `domain/chat/chat.go:320` | HIGH |
| `chatdomain.ReplayEnvelope` wire shape for HTTP refetch endpoint | `domain/chat/chat.go:221-225` | HIGH |
| Service depends on `eventlogpkg.Emitter` (NOT `eventsdomain.Bridge` as doc claims §10.1) | `app/chat/chat.go:90,127` | HIGH |
| Service depends on `notificationspkg.Publisher` for autoTitle entity broadcasts (was `eventsdomain.Bridge`) | `app/chat/chat.go:91,128` | HIGH |
| Service has `catalog catalogdomain.SystemPromptProvider` field + `SetSystemPromptProvider(p)` method (not in doc Service struct) | `app/chat/chat.go:106,202` | MED |
| `Service.emitUserMessage` — burst-emit user message to event-log bridge after Save | `app/chat/chat.go:171` | MED |
| `Service.Send` writes `Message.Attrs` JSON `{"attachments":[{...}]}` for user messages (not block-stored) | `app/chat/chat.go:296,337` | MED |
| `app/chat/host.go::chatHost` implements `loop.Host` for main conversation pipeline (file not listed in doc §5.1) | `app/chat/host.go:31` | HIGH |
| `chatHost.WriteFinalize` is the assistant-message terminal write entry point (replaces doc's writeAndPublish/publishMessageSnapshot/emitFatalError trio) | `app/chat/host.go:47` | HIGH |
| `chatHost.mapEventLogStatus` translates `chatdomain.Status*` → `eventlogdomain.Status*`, with Warn-log fallback for unknown states | `app/chat/host.go:100` | MED |
| `app/loop/loop.go` is the shared ReAct engine — chat invokes `loop.Run` rather than running its own ReAct loop in `runner.go::agentRun` (doc §5.2 still describes inline agentRun) | `app/loop/loop.go:75` (chat consumer at `app/chat/runner.go:136`) | HIGH |
| `loopapp.Run` returns `loop.Result` with `Steps int` field; consumers can read `result.LastMessage` / `result.StopReason` | `app/loop/loop.go:58-66` | MED |
| `runner.processTask` injects `WithConversationID + WithAgentState + eventlog.With + WithMessageID` into agent ctx (replaces doc's `agentRun` ctx prep) | `app/chat/runner.go:88-98` | LOW |
| `runner.processTask` calls `s.emitter.EmitMessageStart(...)` to open assistant slot before LLM resolve (doc §5.2 says `publishMessageSnapshot`) | `app/chat/runner.go:106` | HIGH |
| `runner.emitFatalError` uses detached `saveCtx` for both SaveMessage AND `s.emitter.StopMessage(...)` (race-safe) | `app/chat/runner.go:164-184` | LOW |
| `autoTitle` publishes via `s.notifications.Publish(titleCtx, "conversation", conv.ID, conv)` — NOT `eventsdomain.ConversationTitleUpdated` | `app/chat/runner.go:251` | HIGH |
| `history.go::buildHistory` uses `loopapp.BlocksToAssistantLLM` (loop helper, not chat-private function) — doc §6.5 references `blocksToAssistantLLM` as chat-private | `app/chat/history.go:86` | MED |
| `history.go::buildUserLLMMessage` reads attachments from `Message.Attrs` JSON (not from `Block` records) | `app/chat/history.go:134-150` | MED |
| `infra/store/chat/chat.go::Store.SaveMessage` upserts via ON CONFLICT — DoUpdates also includes `error_code` / `error_message` / `attrs` / `updated_at` columns (doc §6.3 only lists status/stopReason/inputTokens/outputTokens) | `infra/store/chat/chat.go:60-66` | LOW |
| `infra/store/chat/chat.go::Store.attachBlocks` orders by `seq ASC` not `created_at` | `infra/store/chat/chat.go:163` | LOW |
| `infra/store/chat/chat.go` has no `tx.Transaction` / atomicity wrapping for block writes — block ops are single-row UPDATE/INSERT | `infra/store/chat/chat.go:188-244` | LOW |
| 4 HTTP routes registered (NOT 5 as doc §15 implementation checklist claims): POST attachments / POST messages / DELETE stream / GET messages | `transport/httpapi/handlers/chat.go:42-47` | LOW |
| `chatHost` has `userMsgID` field used by `LoadHistory` to seed buildHistory with the triggering user message | `app/chat/host.go:36` + `app/chat/host.go:39` | LOW |
| `loop.Result.Status` is hardcoded to `chatdomain.StatusCompleted` regardless of actual flow termination (only `StopReason` distinguishes); subagent re-maps this in `spawn.go::Spawn` | `app/loop/loop.go:166-172` | LOW |

## In doc but not in code (stale)

| Item | Doc location | Severity |
|---|---|---|
| `Block.Data string` field (claims JSON-wrapped content for all block types) — replaced by `Block.Content string` (raw text) + `Block.Attrs string` (JSON metadata) | chat.md:535 | HIGH |
| `Block.Seq int` — actual is `int64` | chat.md:533 | LOW |
| `Block` struct missing all the fields code has: `ConversationID` / `ParentBlockID` / `Status` / `Error` / `UpdatedAt` | chat.md:530-538 | HIGH |
| Block type `attachment_ref` (with JSON shape `{attachmentId, fileName, mimeType}`) — schema-unification deleted this; user-message attachments now live in `Message.Attrs` JSON | chat.md:548 | HIGH |
| 5-block-type model (text / reasoning / tool_call / tool_result / attachment_ref) — actual eventlog enum is 6 (text / reasoning / tool_call / tool_result / progress / message) | chat.md:540-548, 1034 | HIGH |
| Block JSON-wrapped data shape (`tool_call` = `{"id":...,"name":...,"summary":...,"arguments":...}` etc.) — actual blocks store raw text in `Content` and metadata in `Attrs` JSON | chat.md:544-548 | HIGH |
| `Message` struct missing `ParentBlockID` / `Attrs` fields (both exist in code; central to subagent sub-run model) | chat.md:496-512 | HIGH |
| `Message` Role values listed as `user | assistant` only — code defines `RoleUser` / `RoleAssistant` (doc OK), but commentary in §6.1 states "tool 角色已移除" while code adds no compile-time enforcement (still string column) | chat.md:521 | LOW |
| `app/chat/` 6-file layout claim `chat.go / runner.go / stream.go / tools.go / history.go / util.go` — actual layout is 5 files: `chat.go / runner.go / host.go / history.go / util.go` (no stream.go, no tools.go in chat) | chat.md:307-314 | HIGH |
| `app/chat/runner.go` listed contents claim `streamLLM` / `runTools` / `partitionByExecutionGroup` / `extendHistory` live in chat — they all moved to `app/loop/` (stream.go / tools.go / history.go in loop pkg) | chat.md:307-314, 451-484 | HIGH |
| `Service.writeAndPublish(ctx, msgID, convID, uid, blocks, status, stopReason, errorCode, errorMessage, in, out, fatal)` — does NOT exist anywhere in code | chat.md:451-484, 731-745 | HIGH |
| `Service.publishMessageSnapshot` — does NOT exist anywhere in code | chat.md:733-735, 740-744 | HIGH |
| `Service.stampBlocks` — does NOT exist anywhere in code | chat.md:309, 1049 | MED |
| `runner.go` "三个发布 helper" trio (publishMessageSnapshot / writeAndPublish / emitFatalError) — only `emitFatalError` survives. SSE publishing is now via `eventlogpkg.Emitter` (Service field) for blocks/messages, and `notificationspkg.Publisher` for entity events | chat.md:454-460, 729-737 | HIGH |
| `s.bridge.Publish(ctx, convID, eventsdomain.ChatMessage{Message: msg})` — `domain/events` package was deleted; bridge field replaced by `emitter` + `notifications`. No call site exists | chat.md:483 | HIGH |
| `eventsdomain.Bridge` field on `Service` struct (`bridge eventsdomain.Bridge`) | chat.md:829 | HIGH |
| Phase 6 "entity-state" SSE protocol where `chat.message` carries full Message snapshot with embedded blocks — the actual production protocol is event-log (5 events × 6 block types per event-log-protocol.md) | chat.md:659-758 | HIGH |
| §8 entire chapter "SSE 事件 (Phase 6 重构 · entity-state 模型)" — 100% stale (Phase 1-3 of event-log protocol replaced this; doc §X "事件日志协议接入" mentions dual-write but is dwarfed by §8 stating chat.message is the active model) | chat.md:659-758 | HIGH |
| §8.1 ASCII diagram showing `event: chat.message` payload sequence — actual SSE wire is `event: message_start / block_start / block_delta / block_stop / message_stop` | chat.md:665-678 | HIGH |
| §8.2 `events.ChatMessage` event struct (with embedded Message + 3 subagent fields) — does NOT exist (`domain/events` deleted) | chat.md:684-712 | HIGH |
| §8.4 旧事件 → 字段对照 table — listing legacy events `chat.token` / `chat.reasoning_token` / `chat.tool_call_start` / `chat.tool_call` / `chat.tool_result` / `chat.done` / `chat.error` and saying they map to chat.message — these legacy event types were consolidated INTO event-log's 5-event protocol, NOT into a single chat.message event | chat.md:747-758 | HIGH |
| §13 错误码 table claim `LLM_PROVIDER_ERROR | 502` — actual `chat.ErrProviderUnavailable` maps to HTTP 502 in errmap.go (correct), but doc lists wire code as `LLM_PROVIDER_ERROR` while not mentioning the shared mapping with `llminfra.ErrProviderError` (D1 already flagged this in error-codes.md, not re-flagging) | chat.md:982-993 | LOW |
| §13 says the only sentinels are 7 listed; doc misses `chat.ErrMessageNotFound` (registered in errmap as MESSAGE_NOT_FOUND/404) | chat.md:982-989 | MED |
| §13 doesn't mention `chat.ErrBlockNotFound` sentinel (defined in code, surfaced via AppendDelta / FinalizeStop / GetBlock — not registered in errmap because emitter handles internally) | chat.md:982-989 | LOW |
| §10.3 claim `runner.go` defines `convQueue` with `cancel context.CancelFunc` field only — code adds `agentState *agentstatepkg.AgentState` field carrying must-Read-first SeenFiles / Cwd / etc. | chat.md:861-867 | LOW |
| §10.4 buildSystemPrompt formula: `[base] + [conv.system_prompt] + [locale]` — code adds **catalog block** between system_prompt and locale (only when SetSystemPromptProvider was called) | chat.md:874-883 | MED |
| §11.1 完整调用链 ascii diagram is wrong: writeAndPublish/publishMessageSnapshot don't exist; agentRun is gone (became chatHost + loop.Run); the streamLLM "每个 EventX → publish(chat.message 快照)" line is wrong (now: streamLLM lives in loop pkg, emits per-event block_start/delta/stop on the eventlog bridge, no chat.message snapshot publishing) | chat.md:907-944 | HIGH |
| §15 implementation checklist `[x]` items reference paths/files that don't exist: `app/chat/runner.go — getOrCreateQueue / runQueue / processTask / agentRun (ReAct loop, …) / writeAndPublish (fatal 模式分支) / publishMessageSnapshot / emitFatalError / stampBlocks / autoTitle` (writeAndPublish/publishMessageSnapshot/stampBlocks gone; agentRun replaced by loop.Run) | chat.md:1049 | HIGH |
| §15 `[x] app/chat/stream.go — streamLLM (iter.Seq) + assembleBlocks + extractToolCalls + parseToolArgs` — file does not exist | chat.md:1050 | HIGH |
| §15 `[x] app/chat/tools.go — runTools (sync.WaitGroup 并行) + runOneTool (注入 msgID/toolCallID) + executeTool` — file does not exist (these moved to `app/loop/tools.go`) | chat.md:1051 | HIGH |
| §15 `[x] domain/events/types.go — Phase 6 entity-state 模型: ChatMessage / Forge / Conversation / Task 4 个事件...` — `domain/events` package was DELETED (D1 covered the legacy chat.message removal in events-design.md; flagging here because chat.md §15 still claims this file is part of chat domain implementation) | chat.md:1035 | HIGH |
| §15 `[x] handlers/chat.go — 5 端点：POST attachments / POST messages / DELETE stream / GET messages / GET events SSE (keep-alive ping)` — only 4 routes registered; `GET events SSE` was the legacy /api/v1/events endpoint (removed when domain/events was deleted) | chat.md:1056 | MED |
| §X event-log protocol接入 table claims `repo.AppendBlockContent(blockID, delta)` and `repo.FinalizeBlock(blockID, status, errStr)` — actual method names are `AppendDelta` and `FinalizeStop` | chat.md:1110-1111 | MED |
| §11.2 ascii sequence claims `event: message_start  / block_start / block_delta / block_stop / message_stop` — but the consumer ASCII at §8 (line 668) STILL claims `event: chat.message`. The doc has two conflicting active-protocol descriptions | chat.md:947-961 vs 665-678 | HIGH |
| §X "dual-write 状态" table — table itself is helpful but the "Phase 4 cutover" framing makes it sound like the old `chat.message` event is still live; in reality `domain/events` is gone so dual-write completed | chat.md:1069-1081 | MED |
| §X table row `Service.Send (user msg 落库后) → legacy: 经 chat.message 通过 messages list 间接 → 新 emit` — legacy column references no-longer-extant code path | chat.md:1075 | LOW |
| §X table row `runner.processTask (assistant 槽开) → legacy: host.Publish 首帧` — `host.Publish` (events bridge Publish) does not exist; chatHost has no Publish method | chat.md:1076 | LOW |
| §X table row `runner.emitFatalError → legacy: bridge.Publish(chat.message status=error) → new em.StopMessage` — the legacy column is gone, only the new emit path remains | chat.md:1077 | LOW |
| §X table row `streamLLM (per LLM event) → legacy: publishThrottled / publishNow → host.Publish snapshot → new per-event` — streamLLM now lives in `app/loop/stream.go`; the legacy throttle path no longer exists | chat.md:1078 | LOW |
| §X "tool_call ID 复用" claim that "stream.go 在 EventToolStart 时 EmitBlockStart(event.ToolID, ...)" — actual code is in `loop/stream.go`, not chat | chat.md:1117 | LOW |
| §13 错误码 table doesn't mention `chat.ErrMessageNotFound` registered in errmap | chat.md:982-989 | MED |
| §X "Phase 4 cutover" / "subagent_runs / subagent_messages 表 Phase 4 cutover 删" framing — these tables are already gone (removed in 2026-05 schema unification per CLAUDE.md preamble + subagent.md §3 strikethrough) | chat.md (subagent.md cross-ref §X) | LOW (D1 covered) |
| `Service.Cancel` claim "drains any pending tasks" — code drains via for-loop on q.ch (correct) but doc wording is imprecise about q.cancel = nil semantics | chat.md:374-379 | LOW |
| Tools listed in §2.3 / §4.4: lists `Subagent` is missing, but it's a 2026-05 system tool registered (forgify subagent system tool) — doc table cuts off the 21st tool | chat.md:84-95 (table missing Subagent row) | LOW (chat doc partly out of date with subagent integration) |

## Mismatched

| Item | Code | Doc | Severity |
|---|---|---|---|
| Service struct field set | `repo / convRepo / modelPicker / keyProvider / llmFactory / tools / emitter / notifications / dataDir / log / queues / catalog` (12 fields) | `repo / convRepo / modelPicker / keyProvider / llmFactory / tools / bridge / dataDir / log / queues` (10 fields, with `bridge eventsdomain.Bridge`) | HIGH |
| Block.Type column constraint enforcement | `gorm:"check:type IN (...)"` tag → DB CHECK constraint at AutoMigrate | doc says §D3 enforced but doesn't show the CHECK in the struct example | MED |
| Block.Status constraint | GORM-tag CHECK 4 values | doc §6.2 doesn't mention status field at all | HIGH |
| Block.Seq type | `int64` | `int` | LOW |
| Block.MessageID JSON tag | `messageId` (exported in JSON) | `-` (hidden) | LOW |
| Repository method names | `SaveMessage / GetMessage / ListMessagesByConversation / SaveBlock / AppendDelta / FinalizeStop / GetBlock / ListBlocksByConversation / ListBlocksByMessage / ReplayEventsAfter / SaveAttachment / GetAttachment` (12 methods) | doc §6.x shows `repo.Save(msg)` shape only; doesn't enumerate Repository interface | HIGH |
| Service exposes `SetSystemPromptProvider(p)` for catalog block | yes — affects buildSystemPrompt output | doc §10.4 system prompt formula doesn't include catalog block | MED |
| Service Send pre-LLM error mapping | `ErrPickModel → MODEL_NOT_CONFIGURED`, `ErrResolveCreds → API_KEY_PROVIDER_NOT_FOUND`, default → `LLM_PROVIDER_ERROR` (via runner emitFatalError) | doc §11.1 says same mapping but the implementation uses llmclientpkg.Resolve sentinel rather than direct chatdomain calls | LOW |
| Cancel queue drain semantics | for-loop drains channel, returns nil after empty (best-effort) | doc §10.3 says "drains the queue" (correct in spirit) | LOW |
| chat.go file header claim that file lists "queue management constants" | actual: chat.go has queueCapacity + convQueue/queuedTask types | doc §5.1 says "queue 管理常量" — match | OK |
| HTTP route count claim "5 端点" | actual: 4 routes registered (POST attachments / POST messages / DELETE stream / GET messages); §X (eventlog SSE) is registered by EventLogHandler in eventlog.go, not chat handler | doc §15 says 5 endpoints in handlers/chat.go | MED |
| Chat doc §1.3 "每个小轮次只有一次 Tool Call" — design principle | code allows multiple parallel tool calls per turn (LLM can emit multiple tool_calls in one assistant message; runTools partitions by execution_group) | LOW (this is a design-principle statement; the implementation is consistent with the parallel-batch model — but the principle wording "只有一次 Tool Call" is misleading) |

## Sub-check

- Entities (messages/message_blocks/attachments) aligned: **no** — Block schema fundamentally different (Data → Content+Attrs split; +ConversationID/ParentBlockID/Status/Error/UpdatedAt columns; type enum doc says 5 values, code 6 values; CHECK constraints undocumented). Message struct missing ParentBlockID + Attrs fields.
- Service methods aligned: **no** — `writeAndPublish` / `publishMessageSnapshot` / `stampBlocks` doc-only; `SetSystemPromptProvider` / `emitUserMessage` code-only; bridge dependency replaced by emitter+notifications.
- Endpoints aligned: **partial** — 4 routes in code match 4 of 7 in doc table. SSE eventlog/notifications endpoints (3 of the 7) are correctly attributed to eventlog.go / notifications.go in `transport/httpapi/handlers/`, but doc claim "handlers/chat.go — 5 端点 ... GET events SSE" is wrong.
- Sentinels aligned: **partial** — 8 of 9 chat sentinels in errmap (correct); doc §13 lists only 7 (omits ErrMessageNotFound, ErrBlockNotFound).
- §S21 invariants doc 与代码 aligned: **mostly yes** — code does enforce per-conversation seq monotonic uniqueness via `idx_blocks_conv_seq` UNIQUE(conversation_id, seq), 6-type/4-status CHECKs, append-only via AppendDelta SQL `content || ?`. Doc §X correctly documents these as invariants. `block_start.parentId` invariant (must point to a real prior message ID or block ID — top-level uses messageId) is honored by Service.Send/runner/emitter chain. Doc §X §"DB 写入" describes parent_block_id fallback rule (empty → top-level → wire ParentID = MessageID), and infra/store/chat/chat.go::ReplayEventsAfter implements this exactly. **HIGH-severity mismatch**: §8 entire chapter (entity-state SSE chat.message protocol) violates §S21 invariant model entirely — that protocol is no longer in production.
- 端到端推演 valid (transport→app→domain→infra): **partially valid** — the §11.1 ascii flow correctly traces transport (handler.SendMessage → Service.Send) → app (queue → processTask → agentRun → streamLLM → runTools → writeAndPublish) but agentRun + streamLLM + writeAndPublish are stale identifiers. The actual flow is: transport → ChatHandler.SendMessage → app/chat/Service.Send (with emitUserMessage) → queue → app/chat/runner.processTask → loop.Run (consuming chatHost) → host.WriteFinalize → infra/store/chat persist + emitter.StopMessage. End-to-end shape is correct; identifiers are wrong.
- Phase 4 schema-unification 已反映: **partial** — §X "事件日志协议接入" mentions schema unification + dual-write but is buried under §1-§14 still describing the legacy entity-state model as active. §X §"subagent 表的命运" still says "Phase 4 cutover 删" — those tables are already gone (D1 covered). The doc reads as if Phase 4 is in flight; reality is it's done.

---

## Summary counts

- HIGH: 25 issues (multi-field schema drift in §6 / §8, 6 of 8 file-layout claims wrong, missing Repository interface, vanished bridge field, entire §8 SSE chapter outdated)
- MED: 14 issues
- LOW: 16 issues
- Total: 55 issues

The chat doc requires a substantial rewrite. Recommended scope:
1. **§5 Pipeline architecture**: rewrite to describe loop.Run + chatHost (not agentRun + writeAndPublish trio)
2. **§6 messages/blocks**: rewrite Block + Message structs to match domain code; add ParentBlockID + Attrs + Status/Error/UpdatedAt columns; replace 5-type with 6-type (text/reasoning/tool_call/tool_result/progress/message); replace Data JSON shapes with Content (raw) + Attrs (metadata)
3. **§8 SSE 事件**: replace entire entity-state chapter with event-log protocol summary (link to event-log-protocol.md)
4. **§10 Service struct**: replace bridge with emitter + notifications; add catalog provider
5. **§11 调用链**: replace agentRun-centric trace with loop.Run + chatHost trace
6. **§13 错误码**: add ErrMessageNotFound / ErrBlockNotFound rows; clarify LLM_PROVIDER_ERROR shared mapping
7. **§15 实现清单**: rewrite implementation checklist to match actual file layout (no stream.go / tools.go / domain/events; add host.go; reference loop pkg)
