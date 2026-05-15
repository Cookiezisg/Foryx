# D2 subagent — service-design-documents/subagent.md ↔ code gap report

Date: 2026-05-10
Audited doc: `documents/version-1.2/service-design-documents/subagent.md` (824 lines)
Code surface:
- `backend/internal/domain/subagent/subagent.go`
- `backend/internal/app/subagent/{subagent,spawn,host,queries,registry}.go`
- `backend/internal/app/tool/subagent/agent.go`

The subagent doc is in a partial mid-migration state. The doc explicitly flags the obsolete sections (top-of-file warning + §3 strikethrough) and §16 documents the event-log protocol接入. However, large parts of §3 / §5 / §6 / §10 / §11 / §13 still describe the legacy two-table model as if it's active reference material. The schema-unification work landed; the doc needs to drop the obsolete content rather than just label it deprecated.

---

## In code but not in doc

| Item | Code location | Severity |
|---|---|---|
| `subagentdomain.RoleUser / RoleAssistant / RoleTool / RoleSystem` constants — mirrored from chatdomain.Role* | `domain/subagent/subagent.go:46-51` | LOW |
| `subagentdomain.SubagentType` struct + 6 fields (Name / Description / SystemPrompt / AllowedTools / DefaultModel / DefaultMaxTurns) is the only domain struct exported (the doc-listed `SubagentRun` / `SubagentMessage` / `Repository` interface DO NOT exist in code) | `domain/subagent/subagent.go:64-71` | HIGH |
| Service field `chatRepo chatdomain.Repository` — sub-Message persistence path (replaces doc-claimed `subagentdomain.Repository`) | `app/subagent/subagent.go:65,84` | HIGH |
| Service field `registry *Registry` (concrete pointer to `app/subagent/registry.go::Registry`, NOT `map[string]subagentdomain.SubagentType` as doc §6 claims) | `app/subagent/subagent.go:66` | MED |
| Service field `modelPicker / keyProvider / llmFactory` direct deps (replaces doc-claimed `pickModel func(ctx)→(client,req,err)` closure injection) | `app/subagent/subagent.go:68-70` | MED |
| Service field `activeRunsMu / activeRuns map[string]CancelFunc` — for in-flight Cancel | `app/subagent/subagent.go:73-74` | MED |
| `Service.SetTools` post-construction injection method | `app/subagent/subagent.go:109` | LOW |
| `Service.filterTools(typ)` — physical Subagent removal + AllowedTools whitelist filter | `app/subagent/subagent.go:120-147` | MED |
| `composeSystemPrompt(typeSystemPrompt, locale)` — prepends standard Forgify subagent preamble + appends zh-CN locale hint (doc only says "type 的 system prompt + locale 提示") | `app/subagent/subagent.go:154-162` | LOW |
| `defaultRunTimeout = 5 * time.Minute` constant | `app/subagent/spawn.go:36` | LOW |
| `subagent.StatusCompleted / StatusMaxTurns / StatusCancelled / StatusFailed` 4 status values (doc §3 claims 5 with `running`) | `app/subagent/spawn.go:44-49` | MED |
| `SpawnOpts {MaxTurns int, Model string}` — but `Model string` is NEVER consumed by `Spawn` (deadcode-style stale field) | `app/subagent/spawn.go:55-58` (declared but unused — see `Spawn` body line 86-289) | LOW |
| `SpawnResult` struct fields: `RunID / Type / Status / ErrorMsg / Result / TokensIn / TokensOut / StepsUsed` (8 fields total) | `app/subagent/spawn.go:67-76` | MED |
| `SpawnResult.RunID` doubles as `subMsgID` (the placeholder `messages` row PK with `msg_<16hex>` prefix; NOT `sar_<16hex>`) | `app/subagent/spawn.go:64,69,114` | HIGH |
| `Spawn` accepts `parentCtx`, reads `parentMsgID / parentToolCallID / parentConvID / uid` from reqctx | `app/subagent/spawn.go:92-95` | LOW |
| `Spawn` minted a fresh `subMsgID` (msg_<16hex>) for the sub-Message row + `msgBlockID` (blk_<16hex>) for the placeholder message-block | `app/subagent/spawn.go:114-117` | LOW |
| `Spawn` only emits BlockStart + MessageStart **when both parentToolCallID AND parentMsgID are non-empty** — silent skip when ctx lacks them (e.g. direct service.Spawn call from tests/internal flow) | `app/subagent/spawn.go:115-130` | LOW |
| `Spawn` registers cancel funcs in `s.activeRuns[subMsgID]` and removes via defer | `app/subagent/spawn.go:151-158` | LOW |
| `Spawn` recovers panics inside `loop.Run` via deferred recover wrapping; converts to `runErr = fmt.Errorf("subagent panic: %v", r)` | `app/subagent/spawn.go:184-193` | MED |
| `Spawn` reconciles sub-Message row's status when `spawn.Status != StatusCompleted` — uses detached `reconcileCtx (uid + convID)` to fetch + write the message back via chatRepo | `app/subagent/spawn.go:233-248` | MED |
| `Spawn` closes the placeholder message-block on parent's eventlog with `closeStatus = Completed/Error/Cancelled` matching SpawnResult.Status — uses detached `stopCtx` per §S9 | `app/subagent/spawn.go:261-272` | HIGH |
| `Spawn` records token log via `state.AddSubagentTokens(subMsgID, typ.Name, in, out)` against `parentCtx`'s AgentState (not ctx-injected at call site, read via `reqctxpkg.GetAgentState`) | `app/subagent/spawn.go:276-278` | LOW |
| `subagentHost` struct (loop.Host implementation) with fields: `svc / subMsgID / parentConvID / parentBlockID / uid / typeName / maxTurns / tools / userPrompt / systemPrompt` | `app/subagent/host.go:42-53` | HIGH |
| `subagentHost.LoadHistory` returns `[user prompt]` only — no DB lookup (sub-runs have isolated context) | `app/subagent/host.go:60-64` | MED |
| `subagentHost.Tools` returns `h.tools` (per-spawn filtered list) | `app/subagent/host.go:69-71` | LOW |
| `subagentHost.WriteFinalize` saves Message row to `chatRepo` with `ParentBlockID=msgBlockID`, `Role=assistant`, `Attrs=JSON({"kind":"subagent_run","type","runId","maxTurns"})` | `app/subagent/host.go:81-115` | HIGH |
| `subagentHost.WriteFinalize` emits `eventlogpkg.Emitter.StopMessage(saveCtx, subMsgID, status, ...)` on detached saveCtx | `app/subagent/host.go:129-131` | HIGH |
| `subagentHost.mapEventLogStatus` translates chatdomain → eventlogdomain status, with Warn-log fallback (mirror of chat/host.go) | `app/subagent/host.go:148-163` | LOW |
| `Service.Cancel(_, runID)` impl: looks up runID in `s.activeRuns` map, calls cancel; returns nil whether or not found (race-with-finish is benign) | `app/subagent/queries.go:19-28` | LOW |
| `Registry` (concrete type) has `Get(name) (SubagentType, bool)` and `List() []SubagentType` (sorted by Name) — NOT `s.registry: map[string]subagentdomain.SubagentType` map literal as doc §6 implies | `app/subagent/registry.go:75-122` | MED |
| `Registry.NewRegistry()` lazy-init via `sync.Once` on first Get/List | `app/subagent/registry.go:83-97` | LOW |
| `Registry.ensureIndexed` defaults DefaultMaxTurns to 25 if zero on entry | `app/subagent/registry.go:91` | LOW |
| `app/tool/subagent/agent.go::SubagentTool` is the actual system-tool implementation file (doc §7 calls it `internal/app/tool/subagent/agent.go` — match) | `app/tool/subagent/agent.go:95` | OK |
| `app/tool/subagent/agent.go::SubagentTools(svc) []toolapp.Tool` factory function — doc doesn't show this | `app/tool/subagent/agent.go:102-106` | LOW |
| `SubagentTool.ErrEmptyPrompt / ErrEmptyType` validation sentinels | `app/tool/subagent/agent.go:48-55` | LOW |
| `SubagentTool.ValidateInput` rejects empty `subagent_type` / `prompt` (whitespace-trimmed) — doc §7 only sketches Execute, doesn't show ValidateInput | `app/tool/subagent/agent.go:132-147` | LOW |
| `SubagentTool.Execute` recursion-defense: `reqctxpkg.GetSubagentDepth(ctx) >= 1` (doc §7 says `>= 1` is correct) | `app/tool/subagent/agent.go:172-175` | OK |
| `SubagentTool.Execute` uses 4-way `res.Status` switch: completed (return res.Result), max_turns (`appendNote ... "consider re-spawning..."`), cancelled (`appendNote ... "subagent was cancelled"`), failed (returns `Subagent X failed: <err>` raw or appendNote with body) | `app/tool/subagent/agent.go:203-215` | MED |
| `appendNote(body, note string)` helper formats `"<body>\n\n[note: <note>]"` | `app/tool/subagent/agent.go:224-230` | LOW |
| Schema unification removed all subagent_runs / subagent_messages tables — `infra/store/subagent/` directory does NOT exist; subagent reuses `chat.Repository.SaveMessage` via `chatRepo` | implied by `infra/store/` listing (no subagent dir) | HIGH |

## In doc but not in code (stale)

| Item | Doc location | Severity |
|---|---|---|
| `SubagentRun` struct (~24 fields including `LastToolCalled / LastToolArgsBrief / LastToolResultBrief / LastStepDurationMs / LastStepAt / TotalTokensIn / TotalTokensOut / StepsUsed / Model / StartedAt / EndedAt / ErrorMsg`) — does NOT exist in code; subagent state lives in chatdomain.Message + chat blocks | subagent.md:88-119 | HIGH (already flagged via top-of-file warning, but still inline) |
| `SubagentMessage` struct (`SubagentRunID / Seq / Role / Blocks []chatdomain.Block / PromptTokens / CompletionTokens`) — does NOT exist in code | subagent.md:124-135 | HIGH (already flagged via warning) |
| `subagent_runs` / `subagent_messages` tables (TableName methods) — gone | subagent.md:119, 136 | HIGH (already flagged) |
| `idx_smm_run_seq (subagent_run_id, seq)` composite index — gone with the table | subagent.md:154-156 | LOW (already flagged) |
| `subagentdomain.ErrMaxTurnsExceeded` sentinel — does NOT exist | subagent.md:165 | MED |
| `subagentdomain.ErrCancelled` sentinel — does NOT exist | subagent.md:166 | MED |
| §3 sentinel list claims 4 sentinels — actual is 2 (`ErrTypeNotFound` + `ErrRecursionAttempt`) | subagent.md:160-168 | MED |
| `Repository` interface with `CreateRun / GetRun / UpdateRun / ListRunsByConversation / AppendMessage / UpdateMessage / ListMessagesByRun` (7 methods) — none exist in domain layer; subagent uses chatdomain.Repository directly | subagent.md:222-235 | HIGH |
| `infra/store/subagent/subagent.go` GORM implementation — does NOT exist (no subagent store dir) | subagent.md:237 | HIGH |
| `Service` struct with `repo subagentdomain.Repository / registry map[string]SubagentType / tools / bridge eventsdomain.Bridge / pickModel / log` — actual struct uses chatRepo + concrete *Registry + modelPicker/keyProvider/llmFactory + activeRuns map; no bridge field | subagent.md:243-252 | HIGH |
| `Service.Spawn(ctx, typeName, prompt, opts) (*SpawnResult, error)` — exists with same signature ✓ | subagent.md:263 | OK |
| `Service.Cancel(ctx, runID) error` — exists with same signature ✓ | subagent.md:264 | OK |
| `Service.Get(ctx, runID) (*SubagentRun, error)` — does NOT exist | subagent.md:265 | HIGH |
| `Service.ListTypes() []SubagentType` — does NOT exist on Service; lives on `*Registry.List()` | subagent.md:266 | MED |
| `Service.ListByConversation(ctx, conversationID) ([]*SubagentRun, error)` — does NOT exist (queries.go has comment "future ListActive, GetRunByID") | subagent.md:267 | HIGH |
| `SpawnOpts.MaxTurns` and `Model string` — doc claims `Model "" = use type.DefaultModel ?? PickForChat` is the resolution rule; code never reads `SpawnOpts.Model` (deadcode field; resolution happens via `llmclientpkg.Resolve` against modelPicker/keyProvider) | subagent.md:253-256 | LOW |
| `SpawnResult { Run *SubagentRun, Result string }` — doc shape; actual is 8-field flat struct (`RunID / Type / Status / ErrorMsg / Result / TokensIn / TokensOut / StepsUsed`) | subagent.md:259-261 | HIGH |
| §6 "ReAct 循环复用 Host 接口" section lists `Host.SystemPrompt(ctx)` + `Host.OnInitialPublish` + `Host.OnStreamCheckpoint` + `Host.OnStepComplete` — actual `loop.Host` interface has only 3 methods: `LoadHistory / Tools / WriteFinalize` | subagent.md:278-300 | HIGH |
| §6 `loop.Result` claim of fields `Blocks / Status / StopReason / TokensIn / TokensOut / Steps / LastMessage` — code matches (7 fields, all listed) | subagent.md:289-296 | OK |
| §7 SubagentTool.Execute pseudo-code calls `subagentapp.SpawnOpts{MaxTurns: args.MaxTurns}` only — code matches ✓ | subagent.md:394-395 | OK |
| §7 SubagentTool.Execute pseudo-code shows status switch with `"max_turns"` / `"cancelled"` / default — code uses `subagentapp.StatusMaxTurns` / `StatusCancelled` constants and ALSO has a `StatusFailed` branch which doc omits | subagent.md:400-409 | MED |
| §8.5 `Subagent 总超时` block claims sub-runner overrides timeout via `SubagentType` — code uses constant `defaultRunTimeout = 5 * time.Minute`; SubagentType has NO timeout field | subagent.md:459-466 | MED |
| §8.5 Panic recovery pseudo-code references `s.bridge.Publish(ctx, "", eventsdomain.Subagent{SubagentRun: run})` — `domain/events` deleted, no Subagent event type, no bridge field | subagent.md:472-484 | HIGH |
| §9 `SubagentTokenLog` agentstate API claims `agentstate.AddSubagentTokens(typeName, in, out)` — actual signature is `AddSubagentTokens(runID, typeName, in, out)` (additional first arg) | subagent.md:520-526, 536-538 | MED |
| §9 `SubagentTokenEntry { TypeName, TokensIn, TokensOut, RunID }` claim — actual struct (would need verification of agentstate pkg, but doc lists 4 fields) | subagent.md:537-541 | LOW |
| §9 zap log claim `"subagent.run.completed"` message with `run_id / type / status / tokens_in / tokens_out / steps / duration_ms` — actual log message in spawn.go:280 is `"subagent run terminated"` and lacks `duration_ms` field | subagent.md:551-555 | LOW |
| §10 SSE event chat.message section claims subagent uses `eventsdomain.ChatMessage` event with embedded `*subagentdomain.SubagentRun` — `domain/events` and `SubagentRun` are both gone; subagent now emits via `eventlogpkg.Emitter` (5 events × 6 block types per event-log-protocol.md) | subagent.md:563-670 | HIGH (D1 partially covered events-design.md side; subagent.md still has the full chapter) |
| §10 entire chapter "SSE 事件 — 全合到 chat.message 一条流" — 100% stale (event-log unification replaced this) | subagent.md:561-670 | HIGH |
| §10 backend implementation note `events.ChatMessage` struct in `domain/events/types.go` — domain/events package was deleted | subagent.md:657-666 | HIGH (D1 covered events-design.md; here it's still claimed as canonical) |
| §11 HTTP API table — 4 listed routes: `GET /conversations/{id}/subagent-runs`, `GET /subagent-runs/{id}`, `GET /subagent-runs/{id}/messages`, `GET /subagent-types` (D1 already flagged in api-design.md, NOT re-flagging here per scope) | subagent.md:673-684 | (D1 covered) |
| §11 prose `GET /messages 让前端能"回放"历史 subagent run` — wholesale obsolete (not registered in router; sub-run transcript reads via standard /api/v1/conversations/{id}/messages with sub-Message rows that have parentBlockId set) | subagent.md:683 | LOW (D1 covered the route absence) |
| §12 errmap table claims 4 sentinels: ErrTypeNotFound (404), ErrRecursionAttempt (422), ErrMaxTurnsExceeded (not to handler), ErrCancelled (not to handler) — only first 2 actually exist; latter 2 don't have sentinels at all (terminal status is just a string field on SpawnResult) | subagent.md:689-694 | MED |
| §13 测试覆盖 table claim 71 tests across 7 layers — out-of-scope to verify, but several layer file paths reference dead code: `internal/infra/store/subagent/subagent_test.go` (no such dir), `internal/domain/events/types_test.go` (events deleted), `handlers/subagent_test.go` (no subagent HTTP handler) | subagent.md:702-712 | MED |
| §14 与其他 domain 的关系: `chat: 无直接依赖` — doc claim is correct (subagent depends on `chatdomain.Repository` interface not chat service); but the doc also says `Skill 'context: fork' 时 chatHost 内部可触发 spawn` — Skill domain has not been integrated into chat host at the time of this audit (no spawn call site in chat/host.go) | subagent.md:723 | LOW |
| §14 包依赖方向: `events → chatdomain + subagentdomain` — events package deleted | subagent.md:738 | LOW (D1 covered) |
| §15 演化方向 #4 "subagent 内嵌套 当前禁；未来如有强需求…" — code currently has hard-coded `depth >= 1 → reject` in SubagentTool.Execute; doc note correct ✓ | subagent.md:749-753 | OK |
| §16 entire "事件日志协议接入" chapter is the most accurate part — describes the right protocol, but conflicts with §10 entire chapter still describing the legacy chat.message+SubagentRun model | subagent.md:757-825 | HIGH (internal contradiction) |
| §16 claim "subagent_runs / subagent_messages 两张表 Phase 4 cutover 删" — already done; reads as if pending | subagent.md:821-822 | LOW (D1 covered) |
| §16 dual-write claim "Phase 1-3 dual-write 期间两张表照常写" — dual-write phase is over; tables gone | subagent.md:822 | LOW |
| §3 ID prefix claim `SubagentRun.ID = sar_<16hex>` and `SubagentMessage.ID = smm_<16hex>` — both prefixes are obsolete; sub-run uses `msg_<16hex>` (since it IS a message row) | subagent.md:143, 148 | HIGH |

## Mismatched

| Item | Code | Doc | Severity |
|---|---|---|---|
| Sentinel count | 2 in domain (`ErrTypeNotFound / ErrRecursionAttempt`) | 4 (adds `ErrMaxTurnsExceeded` / `ErrCancelled`) | MED |
| ID prefix for sub-runs | `msg_<16hex>` (subMsgID, the sub-Message row) | `sar_<16hex>` per §S15 + `smm_<16hex>` for messages | HIGH |
| Service ctor signature | `New(chatRepo, registry *Registry, modelPicker, keyProvider, llmFactory, log)` (6 args) | doc §6 implies `New(repo, registry map, tools, bridge, pickModel, log)` (6 args, different shape) | HIGH |
| `Service.Spawn` signature | `Spawn(parentCtx, typeName, prompt, opts) (*SpawnResult, error)` | matches ✓ | OK |
| `Service.Cancel` signature | `Cancel(_, runID) error` (ignores ctx, drops error if not found) | matches ✓ | OK |
| `Service.Get` signature | does NOT exist | `Get(ctx, runID) (*SubagentRun, error)` | HIGH |
| `Service.ListTypes` location | on `*Registry.List()` (registry exposed via `s.registry`) | claim is on Service | MED |
| `Service.ListByConversation` | does NOT exist | `ListByConversation(ctx, convID) ([]*SubagentRun, error)` | HIGH |
| Subagent `Status` value set | `completed / max_turns / cancelled / failed` (4 values per spawn.go:44-49) | `running / completed / cancelled / max_turns / failed` (5 values; "running" is intermediate state for SubagentRun.Status DB field which doesn't exist anymore) | MED |
| `subagentdomain.SubagentType.AllowedTools` semantics | `len(...) > 0 → whitelist`, else `nil → "inherit parent registry minus Subagent tool"` (filterTools impl) | doc §6 same semantics ✓ | OK |
| Recursion defense layer 1 | `Service.filterTools` removes any tool with `Name() == "Subagent"` regardless of AllowedTools | doc §8 same ✓ | OK |
| Recursion defense layer 2 | `SubagentTool.Execute` checks `GetSubagentDepth(ctx) >= 1` | doc §8 same ✓ | OK |
| Sub-run total timeout | `defaultRunTimeout = 5 * time.Minute` (constant) | "5 minutes default, can override per SubagentType" — SubagentType has NO timeout field | MED |
| Cancellation cascade | `subCtx, cancel := context.WithTimeout(...)` — derives from parentCtx via `reqctxpkg.With...` chain so parent cancel cascades | doc §8.5 says correct ✓ | OK |
| Panic recovery | `defer recover()` wrapping `loop.Run` call → `runErr = fmt.Errorf("subagent panic: %v", r)` → SpawnResult.Status = StatusFailed + ErrorMsg | doc §8.5 says publishes `bridge.Publish(eventsdomain.Subagent)` — wrong path; code doesn't publish a panic event, just persists via host.WriteFinalize on the way out + logs Error | HIGH |
| LLM resolution | `llmclientpkg.Resolve(parentCtx, modelPicker, keyProvider, llmFactory)` (resolves bundle{ModelID, Key, BaseURL, Client}) | doc §6 says `pickModel func(ctx)→(client,req,err)` closure — different shape | MED |
| Chat repo dependency | `chatRepo chatdomain.Repository` (uses SaveMessage / GetMessage) | doc §6 doesn't mention chatRepo; says `repo subagentdomain.Repository` | HIGH |
| Spawn does AgentState token bookkeeping | `state.AddSubagentTokens(subMsgID, typ.Name, in, out)` (3 args minus `runID` = subMsgID) | doc says `state.AddSubagentTokens(typeName, in, out)` (2 args, no runID) | MED |
| Sub-Message status reconciliation | "Reconcile sub-Message row's status with the subagent-bucket re-mapping" — Spawn explicitly fetches via `chatRepo.GetMessage(reconcileCtx, subMsgID)` then `SaveMessage` to overwrite Status — only when Status != Completed; docu doesn't describe this | (no doc coverage) | MED |
| Block-stop on placeholder message-block | `em.StopBlock(stopCtx, msgBlockID, closeStatus, nil)` — closeStatus mapped from spawn.Status (Failed→Error, Cancelled→Cancelled, else Completed) | doc §16 mentions placeholder block_stop but doesn't show the status-mapping branch | LOW |
| §6 "包依赖方向" diagram | code matches: `loop` standalone; chat → loop; subagent → loop; subagent → chatdomain (via chatRepo) | doc shows correct ✓ | OK |

## Sub-check

- Entities (messages/message_blocks/attachments) aligned: **N/A** — subagent has no entities. Sub-run is a `messages` row (chatdomain.Message) with attrs.kind=subagent_run + parent_block_id; sub-run transcript is `message_blocks` rows. Doc §3 SubagentRun / SubagentMessage structs are obsolete. The top-of-file warning + §3 strikethrough acknowledge this, but the inline content remains as ~40 lines of obsolete struct definitions.
- Service methods aligned: **no** — `Get` / `ListTypes` (on Service) / `ListByConversation` are doc-claimed but don't exist. `SetTools` exists in code but not in doc API list.
- Endpoints aligned: **N/A** (no HTTP endpoints — D1 already documented this in api-design.md; the HTTP API table in subagent.md §11 should be deleted).
- Sentinels aligned: **partial** — 2 of 2 actual sentinels in errmap (correct, D1 lists those rows). Doc claims 4 sentinels (2 are fake — `ErrMaxTurnsExceeded` / `ErrCancelled` don't exist).
- §S21 invariants doc 与代码 aligned: **mostly yes** in §16 (correct event-log接入 model) but §10 violates them entirely (legacy chat.message+SubagentRun protocol). Sub-run flow honors §S21: `block_start.parentId` = `parentToolCallID` (real prior block ID); `message_start.parentBlockId` = `msgBlockID` (real prior block ID); `block.status` flows streaming → Completed/Error/Cancelled (mapped via `closeStatus` switch); `message.status` similarly via WriteFinalize. seq is monotonic (per-conversation). Tool_call ID reuse rule (LLM's `tc_xxx` ID becomes block PK) honored upstream by chat/loop/stream.go.
- 端到端推演 valid (transport→app→domain→infra): **partially valid** — §2 ascii flow's high-level shape is correct (LLM tool call → SubagentTool.Execute → Service.Spawn → loop.Run → tool_result back to parent LLM). Specific details misalign: `repo.CreateRun` / `repo.UpdateRun` / `repo.ListRunsByConversation` paths claimed in §2 don't exist (no subagent repo); actual write path is `chatRepo.SaveMessage(saveCtx, msg)` via `subagentHost.WriteFinalize`.
- Phase 4 schema-unification 已反映: **partially** — top-of-file warning correctly flags the obsolete §3 content. §16 documents new event-log protocol接入. BUT: §3 inline still has full obsolete struct dumps; §5 (Repository) describes a non-existent interface; §6 describes a Service struct with obsolete dependency shape (bridge instead of emitter, repo instead of chatRepo); §10 has the entire legacy chat.message+SubagentRun SSE chapter; §11 has the obsolete HTTP API table. The doc reads as half-deprecated, half-active.

---

## Summary counts

- HIGH: 17 issues (entire Repository interface obsolete, Service struct shape misaligned, ID prefix misalignment, §10 legacy SSE chapter, §11 HTTP API table)
- MED: 17 issues
- LOW: 16 issues
- Total: 50 issues

The subagent doc requires aggressive deletion + restructuring rather than just inline edits:
1. **§3 obsolete content**: delete the SubagentRun / SubagentMessage struct dumps (top-of-file warning is good but obsolete content shouldn't sit inline)
2. **§5 Repository interface**: delete entirely (no subagent Repository in code; uses chatdomain.Repository directly)
3. **§6 Service**: rewrite to match actual struct (chatRepo + concrete *Registry + modelPicker/keyProvider/llmFactory + activeRuns; no bridge field; ctor takes 6 args; no ListByConversation/Get methods)
4. **§7 SubagentTool**: add ValidateInput / Failed status branch / 9-method full table
5. **§8.5 Panic recovery**: remove `bridge.Publish` claim — replace with description of WriteFinalize + recover wrapping
6. **§9 Token Accounting**: fix AddSubagentTokens signature (3 args, not 2)
7. **§10 SSE event**: delete entire chapter; replace with link to event-log-protocol.md
8. **§11 HTTP API**: delete entire table (no HTTP API)
9. **§12 错误码**: 2 sentinels not 4
10. **§13 测试覆盖**: revise test counts to match current layer paths
11. **§16 event-log接入**: re-position as the active protocol description; remove "Phase 4 cutover" framing (cutover is done)
