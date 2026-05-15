# Dead-logic findings — app/chat + app/loop + pkg/eventlog

Audit scope: `internal/app/chat/`, `internal/app/loop/`, `internal/pkg/eventlog/`. Production `.go` files only (no `_test.go`).

Reference commit for the "legacy" wave that left these residues: `f92f84e` (feat(eventlog+notifications): final cleanup — 双协议清算). That commit deleted `domain/events`, `infra/events/memory`, `chatHost.Publish` / `WriteCheckpoint`, `chatdomain.Block.Data`-shape, the snapshot publish path through `host.Publish`, the 60fps throttle, `domain/events.ChatMessage`, etc. Comments + dead branches predating the cleanup are the primary harvest.

## Per-finding

### [1] `streamLLM` godoc claims "dual-write alongside legacy snapshot path" — but the snapshot path is gone
- **Location**: `backend/internal/app/loop/stream.go:30-46`
- **Claims to do**: The function-level godoc has two stanzas. Lines 30-34 say "Per-event emit fires real-time … on the eventlog Bridge — no snapshot publish path". Lines 36-46 then say "Event-log dual-write (Phase 2): also emits … on the recursive-event-log Bridge alongside the legacy snapshot path". The two stanzas contradict each other.
- **Reality**: After commit `f92f84e` the snapshot publish path is deleted; `host.Publish` and `host.WriteCheckpoint` no longer exist on the `loop.Host` interface. There is exactly one push path (emit). Lines 36-46 (English + Chinese) are stale and mislead readers into thinking there are still two SSE pipelines.
- **Severity**: HIGH
- **Fix**: Delete lines 36-46 entirely. Lines 30-34 already describe the post-cleanup reality correctly.
- **Risk**: Comment-only; no caller-side change.

### [2] `Result.Status` is hardcoded to `Completed` — error / cancelled cases lie
- **Location**: `backend/internal/app/loop/loop.go:159-172` (Result construction)
- **Claims to do**: The Result struct returned at end of `Run` has `Status: chatdomain.StatusCompleted` literally, regardless of how the loop terminated. The `Status` field on `loop.Result` is exported and consumed by `subagent/spawn.go:216` (`case result.Status == chatdomain.StatusError`).
- **Reality**: `loop.Run` writes the correct terminal status into `host.WriteFinalize` (e.g. `StatusError` for LLM stream error or `HISTORY_EXTEND_FAILED`), then `break`s out of the loop. The construction at line 164-172 then returns `Status: chatdomain.StatusCompleted` regardless. So:
  - LoadHistory failure → handled by the early-return at line 92 (`Result{Status: StatusError, ...}`) — correct, but this is the only path that reports Status=Error.
  - LLM stream error → host gets `StatusError`, but Result reports `StatusCompleted`.
  - History-extend error → same: host gets `StatusError`, but Result reports `StatusCompleted`.
  - Cancelled → host gets `StatusCancelled`, Result.StopReason=`StopReasonCancelled` (which spawn.go *does* read), but Result.Status=`StatusCompleted`.
  
  Net effect: in `subagent/spawn.go` the `case result.Status == chatdomain.StatusError` branch fires only for LoadHistory errors; for LLM-stream-error cases it falls through to `default → spawn.Status = StatusCompleted`, mapping a real failure to "completed". The hardcoded constant at loop.go:166 makes a downstream error-classification branch dead in 2 of 3 error paths. `chat` doesn't read `Result.Status` so chat is unaffected — but `Result.Status` as a field on the public Result type is shadow-truthful.
- **Severity**: HIGH
- **Fix**: Track the actual final status alongside `stopReason` / `errCode` / `errMsg` in the loop body and return it. Or: delete `Result.Status` entirely and fold the contract into `StopReason` (subagent already special-cases `StopReasonCancelled` / `StopReasonMaxTokens`; only the explicit "stream error" needs a way through, and `errCode != ""` could carry it).
- **Risk**: `subagent/spawn.go:216` is the sole non-test consumer; either fix the field or rewire that branch to look at `StopReason == StopReasonError`. This audit lives in chat/loop/eventlog but the consumer touch is in subagent — flag for cross-domain note when the real fix lands.

### [3] `runner.go::emitFatalError` builds saveCtx without conversationID, so the StopMessage emit silently no-ops
- **Location**: `backend/internal/app/chat/runner.go:155-185`
- **Claims to do**: Per the godoc + the §S9 detached-ctx comment (lines 173-182), `emitFatalError` should both persist the stub assistant message AND emit `message_stop` on the event-log bridge so the SSE stream's streaming bubble closes. The pattern is documented as "same §S9 reasoning as the SaveMessage above and host.go::WriteFinalize::StopMessage".
- **Reality**: Line 164 builds `saveCtx := reqctxpkg.SetUserID(context.Background(), uid)` — does NOT then call `WithConversationID(saveCtx, conv.ID)`. The chat host's `WriteFinalize` does both (host.go:54-55: `SetUserID(...)`+`WithConversationID(saveCtx, h.convID)`). The `emitter.StopMessage(saveCtx, …)` at line 183 calls `requireConv(ctx, "StopMessage")` inside the Emitter, which warn-logs `emit skipped: no conversationID in ctx` and returns without publishing. The persist (`SaveMessage`) is fine — it doesn't need convID. So a Resolve failure today: SaveMessage stub OK, but no `message_stop` event ever fires for the assistant message that was opened at runner.go:106. UI sees a perpetually-streaming bubble.
- **Severity**: HIGH (this audit only marks dead logic — but classifying line 183's StopMessage as "dead" because it cannot fire is the cleanest framing; the underlying user-visible behaviour is a bug).
- **Fix**: Add `saveCtx = reqctxpkg.WithConversationID(saveCtx, conv.ID)` after line 164, mirroring the host.go fix from commit `fa9b8c4`. After that, the StopMessage actually emits.
- **Risk**: The "publishes its chat.message snapshot" wording in the godoc (line 150) is also stale — `chat.message` was the deleted `eventsdomain.ChatMessage` event. After fixing, godoc should reference `message_stop` on the event-log bridge.

### [4] `chat.go::Send` redundantly stamps the emitter into ctx
- **Location**: `backend/internal/app/chat/chat.go:350-352`
- **Claims to do**: `emitCtx = eventlogpkg.With(emitCtx, s.emitter)` so subsequent emit work can pull the emitter from ctx.
- **Reality**: `emitUserMessage` at line 171 takes `em := s.emitter` directly — it never goes through `eventlogpkg.From(ctx)`. None of the Emitter methods themselves re-read the emitter from ctx; they call Bridge.Publish + repo writes. So the `eventlogpkg.With` stamp on line 351 has no consumer. Compare runner.go:90 where `eventlogpkg.With(agentCtx, s.emitter)` IS load-bearing (streamLLM and runOneTool both pull `eventlogpkg.From(ctx)`).
- **Severity**: LOW
- **Fix**: Delete line 351. Keep line 350 (`WithConversationID`) — the Bridge needs that.
- **Risk**: None.

### [5] `host.go` line 78: `_ = ctx // legacy param retained for loop.Host signature` — wording lies
- **Location**: `backend/internal/app/chat/host.go:78`
- **Claims to do**: Comment says `ctx` is a "legacy" param kept only to satisfy the loop.Host interface.
- **Reality**: `ctx` is NOT legacy. `loop.Host.WriteFinalize`'s ctx parameter is required by the interface (loop.go:52) and is actively used by the *other* Host implementation: `subagent/host.go:83` calls `reqctxpkg.RequireUserID(ctx)` to seed its own saveCtx. Calling chat-host's preference for the construction-time uid+convID "legacy" misrepresents the interface contract. Better wording: "this Host has uid+convID stamped on h, so we use detached saveCtx; the interface ctx is needed by other Hosts (subagent)."
- **Severity**: LOW (wording / comment-tone issue; underlying `_ = ctx` is fine).
- **Fix**: Replace the comment with the accurate explanation, or just drop the inline comment — the reason this Host doesn't need ctx is clear enough from the surrounding detached-saveCtx logic.
- **Risk**: None.

### [6] `pkg/eventlog/eventlog.go::From` godoc claims "logs a warning" but the function never does
- **Location**: `backend/internal/pkg/eventlog/eventlog.go:484-496`
- **Claims to do**: Lines 485-489 godoc: "Returning a no-op (vs nil) lets callers always invoke methods without nil-checks; missing emitter logs a warning so wiring bugs surface."
- **Reality**: The function body has no log call — it just returns `noopEmitter{}`. Wiring bugs (caller forgot to do `eventlogpkg.With(ctx, emitter)`) surface only via the per-call `requireConv` warn-skip messages downstream, NOT from `From` itself. The godoc lies about behaviour.
- **Severity**: MED (godoc misrepresents the silent fallback; an operator looking at logs to debug "why isn't the emitter wired" will assume `From` would have warned).
- **Fix**: Either delete the "logs a warning so wiring bugs surface" claim, or actually add the warn log. (The latter is noisy if From is called many times per request — likely undesirable; delete the claim.)
- **Risk**: None for the comment delete.

### [7] `pkg/eventlog::WithParent` godoc claims "Tool framework wraps Tool.Execute with WithParent" — but no caller exists
- **Location**: `backend/internal/pkg/eventlog/eventlog.go:511-520`
- **Claims to do**: Exported helper that's a thin wrapper over `reqctxpkg.WithParentBlockID`. Godoc claims the tool framework uses it: "Tool framework wraps Tool.Execute with `WithParent(ctx, toolCallBlockID)` so any block the tool starts is auto-parented under tool_call."
- **Reality**: No production caller. `loop/tools.go:106` directly does `reqctxpkg.WithParentBlockID(toolCtx, tc.ID)` — bypasses this wrapper entirely. `WithParent` exists, the wrapper is correct, but it's an unused abstraction over a one-line reqctx call. Godoc cites a contract that the framework doesn't actually honor.
- **Severity**: MED (false godoc + dead exported wrapper).
- **Fix**: Delete `WithParent` and its godoc; callers already go through reqctx directly. Or, alternatively, make tools.go:106 use `WithParent` — but that's pure aesthetic; the function isn't earning its weight as an abstraction layer.
- **Risk**: Trivial — confirm no test imports it (`eventlog_test.go` was excluded from this read; verify before delete).

### [8] `pkg/eventlog::MustFrom` is exported but never called
- **Location**: `backend/internal/pkg/eventlog/eventlog.go:498-509`
- **Claims to do**: Returns the Emitter from ctx, panics if absent. Godoc: "Use only at places where missing emitter is unambiguously a wiring bug."
- **Reality**: Grep shows no production caller. Pure dead exported symbol. Speculative API.
- **Severity**: LOW
- **Fix**: Delete. If a future caller genuinely needs panic-on-missing, they can write the 3-line check inline.
- **Risk**: None.

### [9] `pkg/eventlog::StartMessage` is exported but no production caller
- **Location**: `backend/internal/pkg/eventlog/eventlog.go:69` (interface) + line 207-230 (impl) + line 526-528 (no-op)
- **Claims to do**: Mints a fresh `msg_<16hex>` ID and emits `message_start`. The interface comment positions it as the high-level entry point.
- **Reality**: All production message_start emits go through `EmitMessageStart` (caller-supplied ID) — runner.go:106, chat.go:173 (via emitUserMessage), subagent/spawn.go (placeholder). Why? Because every chat-flow caller pre-mints the message ID upstream so the SSE consumer sees the slot ID before the first token. `StartMessage`'s "mint internally" semantic doesn't match any caller's needs. Test `eventlog_test.go` may exercise it but no production code does.
- **Severity**: LOW (interface decoration; harmless but suggests the API was designed before the caller patterns settled).
- **Fix**: Either remove `StartMessage` from the interface (and the impl + noop impl), or accept it as a future extension hook. If kept, at minimum the godoc should note "currently only used by tests; production callers all use EmitMessageStart."
- **Risk**: Removal touches the interface, the emitter struct, and noop — small surface but visible.

### [10] `pkg/eventlog::StartBlockUnder` exported but no production caller
- **Location**: `backend/internal/pkg/eventlog/eventlog.go:93` (interface) + line 345-368 (impl) + line 532-534 (no-op)
- **Claims to do**: Like `StartBlock` but takes explicit parentID + messageID. Godoc: "Used when the framework needs to override the ctx-derived parent (e.g. tool framework wraps Tool.Execute with a fresh parent)."
- **Reality**: No production caller. Tests + `installprogress_test.go::fakeEmitter` both reference it but no real production code path. The "tool framework wraps" use-case described in godoc is the same one that should use `WithParent` (finding [7]) — both APIs were designed for a wiring style the codebase didn't ultimately adopt.
- **Severity**: LOW
- **Fix**: Same as [9] — either drop or downgrade godoc to "test-only".
- **Risk**: Minor.

### [11] `saveBlockRow` dead-code in `seq == 0` short-circuit
- **Location**: `backend/internal/pkg/eventlog/eventlog.go:242-245`
- **Claims to do**: `if em.repo == nil || seq == 0 { return }`. The `em.repo == nil` branch protects test callers that pass nil repo. The `seq == 0` branch suggests "if publish failed (which returns 0 seq), skip the DB write".
- **Reality**: Both production callers gate `saveBlockRow` on `if ok` from `publish` — see line 339-341 (StartBlock) and line 408-410 (EmitBlockStart). When `ok=false` (publish failed) the saveBlockRow call is never reached. So inside `saveBlockRow`, `seq == 0` is structurally unreachable. The check is defensive against a code path that doesn't exist.
- **Severity**: MED (defensive code that hides whether the upstream gate is the real contract).
- **Fix**: Drop the `|| seq == 0` clause. If a future refactor forgets the `if ok` gate, the row would still be written with seq=0 — better to make that visible (will violate the UNIQUE(conversation_id, seq) constraint and surface fast).
- **Risk**: Trivial. Read the two callers — if neither gates on `ok`, the audit is wrong; both currently do.

### [12] `eventlog.New` godoc references "legacy callers" that no longer exist
- **Location**: `backend/internal/pkg/eventlog/eventlog.go:126-138`
- **Claims to do**: Godoc says "repo is optional — when non-nil, every block_start / block_delta / block_stop also dual-writes to the message_blocks_v2 table … Pass nil for tests / legacy callers that don't need DB persistence."
- **Reality**: After commit `f92f84e` the table is just `message_blocks` (the `_v2` suffix was retired). All production callers (main.go, harness.go) pass non-nil repo. There are no "legacy callers" in production — only tests pass nil. The "legacy callers" wording paints the nil-repo path as supporting a class of real callers that doesn't exist.
- **Severity**: LOW (godoc wording; functionality is fine — tests do still need nil-repo support).
- **Fix**: Replace "tests / legacy callers" with "tests" in both English and Chinese. Update "message_blocks_v2" → "message_blocks". The "Phase 2B" parenthetical is now historical too — drop.
- **Risk**: None.

### [13] `eventlog.go` "messages persist via legacy chat repo" wording is stale
- **Location**: `backend/internal/pkg/eventlog/eventlog.go:131-132` (godoc) + line 303-304 (inline)
- **Claims to do**: "Message lifecycle (message_start / message_stop) does NOT dual-write — messages persist through the legacy chat repo until Phase 4 cutover."
- **Reality**: There is no "legacy chat repo" vs "new chat repo" distinction; there is one chat Repository. "Until Phase 4 cutover" implies a future migration that isn't documented anywhere — the current architecture (per backend-design.md / progress-record.md) doesn't track a planned event-log message persistence cutover. The intent ("we don't dual-write messages, only blocks") is correct, but the framing makes a future state up.
- **Severity**: LOW
- **Fix**: Drop the "legacy" + "Phase 4 cutover" phrasing; the relevant statement is just "Messages are persisted via the chat Repository's SaveMessage path (chat/host.go); the Emitter only persists blocks."
- **Risk**: None.

### [14] `chat.go` package file-listing comment lists deleted table + event names
- **Location**: `backend/internal/app/chat/chat.go:18-22`
- **Claims to do**: Quick file-table for navigation. Line 20: `host.go     — chatHost implements loop.Host (writes chat_messages, fires chat.message)`.
- **Reality**: 
  - Table is `messages`, not `chat_messages` (commit `f92f84e` renamed; see chatdomain.Message.TableName at domain/chat/chat.go:74).
  - `chat.message` was the `eventsdomain.ChatMessage` entity-snapshot event type, deleted with `domain/events`. host.go now emits `message_stop` on the event-log bridge — a different protocol with a different name.
- **Severity**: MED (this is the first thing a new contributor reads when entering the package).
- **Fix**: Update the bullet to match reality: `host.go — chatHost implements loop.Host (persists Message rows + emits message_stop on the event-log bridge)`.
- **Risk**: None.

### [15] `runner.go::processTask` comment references deleted `chat.message` event
- **Location**: `backend/internal/app/chat/runner.go:92-96`
- **Claims to do**: "Allocate the assistant msgID up front so pre-LLM errors emit a stub assistant Message — every chat.message event must carry a real Message."
- **Reality**: There is no longer a `chat.message` event type to "carry a Message". The msgID-up-front pattern is still useful (the SSE stream needs a stable assistant message ID for `EmitMessageStart` at line 106 + `StopMessage` at host.go:75 / runner.go:183), but the justification cites a deleted event family.
- **Severity**: MED (misleads anyone trying to understand why the pre-allocation is necessary).
- **Fix**: Reword to reference the current contract: "Pre-allocate the assistant msgID so the event-log `message_start` (line 106) can be emitted with a stable ID before LLM resolution; pre-LLM errors then have a valid msgID to attach the `message_stop` to." Drop the `chat.message` reference.
- **Risk**: None.

### [16] `tools.go::runOneTool` computes `elapsedMs` only to discard it
- **Location**: `backend/internal/app/loop/tools.go:108-110, 146`
- **Claims to do**: Three lines compute elapsed time for the tool execution: `start := time.Now()`, then `elapsedMs := time.Since(start).Milliseconds()`, then `_ = elapsedMs // legacy elapsedMs no longer carried in Block (UI gets it via DB row updated_at - created_at)`.
- **Reality**: The legacy `chatdomain.ToolResultData.ElapsedMs` JSON wrapper that consumed this value was deleted in commit `f92f84e` (see git diff: the `d, _ := json.Marshal(chatdomain.ToolResultData{ … ElapsedMs: elapsedMs})` block was removed). The `_ = elapsedMs` parking is purely vestigial. The comment claiming "UI gets it via DB row updated_at - created_at" is plausible-defensible but doesn't justify computing the value at all.
- **Severity**: MED (3 lines of compute that produce no observable effect; not expensive but actively misleading).
- **Fix**: Delete `start := time.Now()` (line 108), `elapsedMs := time.Since(start).Milliseconds()` (line 110), and `_ = elapsedMs` (line 146) entirely.
- **Risk**: None — confirmed `elapsedMs` is referenced nowhere else in the function.

### [17] `assembleBlocks` mints fresh `blk_<id>` IDs for text/reasoning that no consumer ever reads
- **Location**: `backend/internal/app/loop/stream.go:202-248`
- **Claims to do**: Build the in-memory `chatdomain.Block` slice for "in-loop history conversion (BlocksToAssistantLLM) and the loop.Result.Blocks return". The godoc explicitly states "fields filled: ID + Type + Content + (tool_call: Attrs)".
- **Reality**: 
  - For text/reasoning blocks (lines 205-222): a fresh `idgenpkg.New("blk")` is generated, but `BlocksToAssistantLLM` reads only `b.Type` and `b.Content` for these types — `b.ID` is never read. The DB write path doesn't go through these in-memory blocks (DB-side IDs are minted earlier in `streamLLM` at lines 89, 100 and passed via `EmitBlockStart`). The IDs minted inside `assembleBlocks` are write-only.
  - The `Status: eventlogdomain.StatusCompleted` and `CreatedAt: time.Now().UTC()` fields (lines 210-211, 219-220, 243-244) are also set but never consumed: `BlocksToAssistantLLM` ignores both Status and CreatedAt for all block types. The DB rows get their CreatedAt/UpdatedAt from the store layer (`saveBlockRow` at eventlog.go:270 and `chatstore.SaveBlock`'s gorm hooks).
  - The godoc itself lists "ID + Type + Content + (tool_call: Attrs)" — Status and CreatedAt aren't even in the list, but the code fills them. The code disagrees with its own contract.
- **Severity**: LOW (cosmetic — wasted idgen calls + wasted Status/CreatedAt assignment, but cheap).
- **Fix**: 
  - For text/reasoning, drop `ID: idgenpkg.New("blk")` (use empty string) — saves an idgen+rand syscall per LLM turn.
  - Drop `Status` and `CreatedAt` field assignments to align with the godoc.
- **Risk**: Minor; verify `BlocksToAssistantLLM` handling of empty `b.ID` is benign for non-tool_call types (it is — it only reads `b.ID` in the `BlockTypeToolCall` case).

### [18] `tools.go::runOneTool` returns Block with Status / CreatedAt that nobody reads
- **Location**: `backend/internal/app/loop/tools.go:140-155`
- **Claims to do**: Return an in-memory tool_result block for in-loop history extension. Comment (lines 133-139) calls out `Content` and `ParentBlockID` as the fields needed.
- **Reality**: Same pattern as [17]. `Status` and `CreatedAt` are set but `BlocksToAssistantLLM` ignores both for tool_result blocks (it reads only `b.Content` with `b.Error` fallback and `b.ParentBlockID`). The DB row's status comes from the eventlog Emitter via FinalizeStop, not from this in-memory block.
- **Severity**: LOW
- **Fix**: Drop the Status / CreatedAt assignments in the returned Block.
- **Risk**: None.

### [19] `chat.go::Send` builds a user-message Block with `Status: StatusCompleted` that's never consumed
- **Location**: `backend/internal/app/chat/chat.go:320-328`
- **Claims to do**: Build a single text Block to attach to `userMsg` for both persistence and SSE emit.
- **Reality**: 
  - `repo.SaveMessage` does NOT write Block rows (chatstore.SaveMessage at chat.go:55-71 only writes the messages row; comment at line 47-48 confirms "Blocks are NOT written here").
  - `emitUserMessage` (line 171-182) iterates `msg.Blocks`, reads `b.ID`, `b.Type`, `b.Content`, then hardcodes `eventlogdomain.StatusCompleted` at line 179 in `StopBlock`. It does NOT read `b.Status`.
  - So the `Status: eventlogdomain.StatusCompleted` field set at line 326 is consumed by neither the persistence path nor the emit path. Pure decoration.
- **Severity**: LOW
- **Fix**: Drop `Status: eventlogdomain.StatusCompleted` from the Block literal.
- **Risk**: None.

### [20] `host.go::buildMessage` sets `UpdatedAt` that the store overwrites
- **Location**: `backend/internal/app/chat/host.go:138-140`
- **Claims to do**: Construct an assistant Message row with `UpdatedAt: time.Now().UTC()`.
- **Reality**: `chatstore.SaveMessage` (infra/store/chat/chat.go:59) does `m.UpdatedAt = time.Now().UTC()` — overwriting whatever caller set. The line 139 assignment is dead.
- **Severity**: LOW
- **Fix**: Drop the UpdatedAt assignment in `buildMessage`. Let the store own the timestamp.
- **Risk**: None — store-side already sets it.

### [21] `chatdomain.StatusPending` is filtered in history but never written by chat code
- **Location**: `backend/internal/app/chat/history.go:47` + `host.go:108` + (cross-package mirror) `subagent/host.go:156`
- **Claims to do**: Defensive filter (`m.Status == StatusPending` → skip from LLM history) and defensive mapping (`StatusPending → StatusCompleted` in mapEventLogStatus).
- **Reality**: No code in the audit scope (or chat write paths in general) ever sets a Message to `chatdomain.StatusPending`. `chat.Send` writes user messages with `StatusCompleted` (chat.go:336). Assistant messages flow `StatusStreaming → StatusCompleted/Error/Cancelled`. `StatusPending` exists as a constant (domain/chat/chat.go:82) but nothing uses it as a value. The defensive checks protect against a state that can't currently exist.
- **Severity**: EDGE (the defensiveness is cheap; uncertain whether to keep for future-proofing or drop to reduce noise).
- **Fix**: Either delete the constant + the two defensive cases, or document why it's reserved.
- **Risk**: None for delete; trivial.

### [22] `loop/stream.go` file header: "publish snapshots via host" / "loop.Run owns the persistence cadence"
- **Location**: `backend/internal/app/loop/stream.go:1-5`
- **Claims to do**: Two stale claims in the file-level comment:
  1. "consume stream events, publish snapshots via host" — implies a `host.Publish` snapshot path.
  2. "No DB writes; loop.Run owns the persistence cadence" — implies persistence is centralized.
- **Reality**: 
  1. Snapshot publishing through host is gone (commit `f92f84e` removed `host.Publish` and `host.WriteCheckpoint`); the only push path is direct emit to the event-log Bridge.
  2. Persistence is no longer cadence-managed by loop.Run — block rows are written real-time by the Emitter (saveBlockRow/AppendDelta/FinalizeStop) inside each emit call. loop.Run's only persistence touch is forwarding to `host.WriteFinalize` which writes the messages row. The cadence belongs to the Emitter, not loop.Run.
- **Severity**: MED (file-header staleness — first thing a maintainer reads).
- **Fix**: Replace with: `stream.go — One LLM call: consume stream events, emit block_start/delta/stop on the event-log Bridge, assemble in-memory Blocks for in-loop history conversion. Block rows persist real-time inside the Emitter (pkg/eventlog); loop.Run only writes the final messages row via host.WriteFinalize.`
- **Risk**: None.

### [23] `tools.go::runTools` godoc references "in-loop history extension" return value but caller pattern has shifted
- **Location**: `backend/internal/app/loop/tools.go:30-37`
- **Claims to do**: "Returns the in-memory tool_result block slice for in-loop history extension."
- **Reality**: The blocks returned ARE used by `extendHistory` at loop.go:142-145 → real. This is alive. The phrase "in-memory tool_result block slice" is correct. But the phrase "no snapshot publish path" (line 32) is good — it correctly notes the post-cleanup state. Actually scratch the issue — re-reading, this comment is fine. (Self-correction: removing this finding.)

### [24] `loop.go` doc "Skill fork / Phase 4 workflow LLM nodes are the others" — aspirational
- **Location**: `backend/internal/app/loop/loop.go:1-4` and similar at `chat.go:3-4`
- **Claims to do**: Loop-package and chat-package docs both list `chat / subagent / Skill fork / Phase 4 workflow LLM nodes` as the consumers of the loop ReAct engine.
- **Reality**: Today only `chat` and `subagent` call `loop.Run`. Skill fork doesn't exist (skill currently calls LLM directly via `llmclient.Resolve` in skill/search.go for rerank, not through loop). Phase 4 workflow LLM nodes — Phase 4 is unstarted per progress-record. Aspirational roadmap framing rather than current truth.
- **Severity**: LOW (aspirational + roadmap; not actively misleading the way deleted-event references are).
- **Fix**: Optionally trim to "chat / subagent are the current callers; future phases (Phase 4 workflow nodes) will join."
- **Risk**: None.

## Summary

| Severity | Count | Categories |
|---|---|---|
| HIGH | 3 | stale dual-write godoc (#1) / hardcoded Status defeats error reporting (#2) / silently-broken StopMessage emit due to missing convID stamp (#3) |
| MED | 7 | godoc lies (#6, #7, #14, #15, #22) / dead defensive seq==0 check (#11) / vestigial elapsedMs compute (#16) |
| LOW | 11 | dead exported wrappers (#7's downstream, #8, #9, #10) / dead field assignments (#17, #18, #19, #20) / stale wording (#5, #12, #13, #24) / redundant ctx stamping (#4) |
| EDGE | 1 | StatusPending defensive filter for unreachable state (#21) |

Note on #7: counted as MED above (godoc lies) but the wrapper itself is also dead — same finding has both flavors.

Note on #23: self-corrected during the read; not a finding.

Hot spots:
- `loop/loop.go:166` (Result.Status hardcoded): single one-line change with the highest blast radius. Subagent-side fallout is the real damage; in-scope it's a constant assignment.
- `loop/stream.go:30-46`: stale godoc with internal contradiction. Pure delete.
- `chat/runner.go:155-185`: emitFatalError needs the same convID stamp that `host.WriteFinalize` already has. One-line fix elsewhere; in-scope is to flag the StopMessage call as effectively dead.
- `pkg/eventlog/eventlog.go`: 3 dead exported entry points (`MustFrom`, `WithParent`, `StartMessage`) — the Emitter API was over-designed for caller patterns the codebase didn't adopt.
