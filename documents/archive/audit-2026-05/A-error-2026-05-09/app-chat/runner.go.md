# audit: backend/internal/app/chat/runner.go

LOC: 246
Read: full file (lines 1-246)

## Trace

| site# | file:line | snippet | category | classification | reasoning | severity | user_impact | suggested_fix | status |
|---|---|---|---|---|---|---|---|---|---|
| 1 | runner.go:36-47 | `func getOrCreateQueue(...) *convQueue { q := &convQueue{...}; actual, loaded := s.queues.LoadOrStore(...); if loaded { return actual.(*convQueue) }; go s.runQueue(...); return q }` | A.1 | OK | sync.Map.LoadOrStore returns (value, loaded); no error path; no §S3 concern | N-A | — | — | — |
| 2 | runner.go:49-71 | `runQueue` — timer + select drain loop. `defer s.queues.Delete(conversationID)` | A.1 | OK | goroutine scheduler; no error path. Map delete on idle is intentional GC strategy. | N-A | — | — | — |
| 3 | runner.go:78-87 | `agentCtx, cancel := context.WithCancel(ctx); ... defer func() { cancel(); ... q.cancel = nil ... }()` | A.1 | OK | proper cancel registration + cleanup; ctx wired to queue's q.cancel for Cancel() to invoke. No error path. | N-A | — | — | — |
| 4 | runner.go:97-98 | `msgID := newMsgID()` (idgenpkg.New("msg")) | A.3 | OK | §S15 ✓ — "msg" prefix matches spec; idgenpkg panics on rand fail | N-A | — | — | — |
| 5 | runner.go:106 | `s.emitter.EmitMessageStart(agentCtx, msgID, chatdomain.RoleAssistant, "", nil)` | A.1 | OK | fire-and-forget emit; emitter handles its own logging per spec; not a terminal write (just opens the assistant message slot for downstream block_start events) | N-A | — | — | — |
| 6 | runner.go:108-119 | `bc, err := llmclientpkg.Resolve(...); if err != nil { code := "LLM_PROVIDER_ERROR"; switch { case errors.Is(err, llmclientpkg.ErrPickModel): code = "MODEL_NOT_CONFIGURED"; case errors.Is(err, llmclientpkg.ErrResolveCreds): code = "API_KEY_PROVIDER_NOT_FOUND" }; s.emitFatalError(...); return }` | A.4 | EDGE | error sentinel-checked + translated to wire code → emitFatalError persists stub message + emits status=error. **Error is fully surfaced to user** (UI renders the error message via the assistant message stub). However, the original err is **passed as `err.Error()` string into emitFatalError, losing wrap chain** — site #6 fork from sentinel. By the time it reaches emitFatalError it's just a string. Not §S16 violation strictly (we're done propagating up — this is a terminal handler). But the wire code translation block is hand-rolled rather than via errmap — duplicated truth source vs §S17. | LOW | UI shows "MODEL_NOT_CONFIGURED" / "API_KEY_PROVIDER_NOT_FOUND" / "LLM_PROVIDER_ERROR" with err.Error() text. Codes here drift if errmap.go changes. Two sources of truth (errmap.go errTable + this manual switch). | could call `responsehttpapi.lookup(err)` to use errmap as single source — but that requires exposing lookup OR a terminal-error→code helper. Defer. | **WAIVED 2026-05-10** — exposing errmap.lookup beyond response/ pkg adds layer dependency for marginal benefit; only 3 codes (LLM_PROVIDER_ERROR / MODEL_NOT_CONFIGURED / API_KEY_PROVIDER_NOT_FOUND); current divergence is bounded by errors.Is sentinel matching, not string-keyed. Audit-recommended defer. |
| 7 | runner.go:136-142 | `result := loopapp.Run(agentCtx, host, bc.Client, baseReq, maxSteps, s.log); s.log.Info("agent run complete", ...)` | A.1 | OK | loop.Run is internal; no error returned (loop.Host.WriteFinalize handles terminal state). Success log captures key metrics (stop reason, tokens). | N-A | — | — | — |
| 8 | runner.go:144-146 | `if task.conv.Title == "" && !task.conv.AutoTitled { go s.autoTitle(context.Background(), task.conv, task.uid, result.LastMessage) }` | **A.2** | **OK (POSITIVE EXAMPLE — §S9 PATTERN)** | autoTitle runs in a fire-and-forget goroutine with `context.Background()` — completely detached from request lifecycle (the agent run might complete and trigger autoTitle even after the user closed the tab). This is the §S9 pattern: a background terminal-state operation (writing the conv title) that must not be cancelled by upstream. autoTitle internally re-stamps uid via reqctxpkg.SetUserID at line 217. | N-A | — | — | — |
| 9 | runner.go:160-162 | `s.log.Error("chat fatal error", zap.String("conversation_id", conv.ID), zap.String("code", code), zap.String("message", message))` | A.1 | OK | proper ERROR log with full context for fatal-before-LLM cases | N-A | — | — | — |
| 10 | runner.go:164-167 | `saveCtx := reqctxpkg.SetUserID(context.Background(), uid); msg := buildMessage(...)` | **A.2** | **OK (POSITIVE EXAMPLE — §S9)** | terminal-state write of stub error message uses detached ctx — same pattern as host.go. The error message MUST be persisted even if user already closed tab; otherwise next time they open the conversation, no record of the failure exists. | N-A | — | — | — |
| 11 | runner.go:168-171 | `if err := s.repo.SaveMessage(saveCtx, msg); err != nil { s.log.Error("CRITICAL: fatal-error stub message persist failed — message lost", zap.String("msg_id", msgID), zap.Error(err)) }` | A.1 | OK | DB save failure logged at ERROR with critical marker; emitFatalError continues to emit StopMessage at site #12 even if save failed (best-effort terminal — UI gets the error via SSE even if DB lost). Same defensive pattern as host.go::WriteFinalize. | N-A | — | — | — |
| 12 | runner.go:175-176 | `s.emitter.StopMessage(ctx, msgID, eventlogdomain.StatusError, ...)` | **A.2** | **EDGE** | uses `ctx` (parameter — caller's agentCtx) NOT the detached `saveCtx` derived at site #10. emitFatalError is called from processTask at site #6 where agentCtx **could** be cancelled (parent context.WithCancel). If user closes tab between Resolve fail and StopMessage emit, ctx is cancelled, emit may early-out before SSE subscribers receive. **Compare to host.go:75 which deliberately uses saveCtx for the same reason**. Inconsistency between fatal-error path (here) and normal-finalize path (host.go::WriteFinalize). | MED | upstream cancel between fatal-error site #6 and StopMessage emit could cause UI to never receive the error message_stop event — UI shows pending forever even though DB has the error. Edge case (very small race window) but identical to the §S9 pattern host.go:75 explicitly fixes. | use `saveCtx` for the StopMessage emit too: change `s.emitter.StopMessage(ctx, ...)` → `s.emitter.StopMessage(saveCtx, ...)` to match host.go:75 | **FIXED 2026-05-09 f272503** |
| 13 | runner.go:181-207 | `func buildSystemPrompt(...) string { ... }` | A.1 | OK | pure string assembly; no error path. Catalog provider nil-check (lines 197-201) is documented graceful degrade (`nil ⇒ skip catalog block`) — not §S3 silent fail. | N-A | — | — | — |
| 14 | runner.go:218-221 | `bc, err := llmclientpkg.Resolve(titleCtx, ...); if err != nil { return }` (silent abort in autoTitle) | A.1 | OK | doc comment at lines 211-215 explicitly designates autoTitle as "Best-effort: any failure aborts silently". autoTitle is non-essential UX — failure means conversation keeps default "untitled" name, no user-visible error. §S3 例外 — "if only resource cleanup or non-essential operation fails, swallow OK". | N-A | — | — | — |
| 15 | runner.go:223-224 | `tCtx, cancel := context.WithTimeout(titleCtx, 10*time.Second); defer cancel()` | A.1 | OK | proper timeout + cleanup | N-A | — | — | — |
| 16 | runner.go:233-236 | `title, err := llminfra.Generate(tCtx, bc.Client, req); if err != nil || title == "" { return }` | A.1 | OK | best-effort autoTitle — same pattern as site #14 | N-A | — | — | — |
| 17 | runner.go:237-242 | `conv.Title = ...; if err := s.convRepo.Save(titleCtx, conv); err != nil { s.log.Warn("auto-title save failed", zap.Error(err)); return }` | A.1 | OK | save failure logged at WARN (proper observability) before silent return — §S3-compliant graceful degrade. autoTitle is observability-grade (UX-only); WARN is appropriate severity since user can still rename manually. | N-A | — | — | — |
| 18 | runner.go:243-245 | `s.notifications.Publish(titleCtx, "conversation", conv.ID, conv); s.log.Info(...)` | A.1 | OK | notifications fire-and-forget; emitter handles its own logging per spec | N-A | — | — | — |

## Sub-check

A.1 §S3 错误吞没:
  - violations: not present
  - notes: site #14, #16, #17 are **documented best-effort silent paths** (autoTitle is observability-grade UX); site #17 logs at WARN before silent return; all comply with §S3 example "如果只是清理资源失败且不影响业务，吞 OK".

A.2 §S9 detached ctx 终态写:
  - terminal-state writes identified: 
    - site #10 (emitFatalError SaveMessage — persists stub error message)
    - site #12 (emitFatalError StopMessage — closes assistant message via SSE)
    - autoTitle SaveMessage at line 239 (best-effort UX — not critical terminal-state)
  - 各自 ctx 来源: 
    - site #10 uses detached `saveCtx` ✓
    - site #12 uses **caller's `ctx` (agentCtx)** — POTENTIAL VIOLATION (see trace site #12)
    - autoTitle saves use `titleCtx` derived from `context.Background()` (line 217 implicit — `ctx` param to autoTitle is passed `context.Background()` at site #8)
  - violations: site #12 (StopMessage emit in emitFatalError — uses caller ctx instead of saveCtx; inconsistent with host.go:75 which explicitly uses detached for the same purpose)

A.3 §S15 ID 生成:
  - ID generation calls: site #4 (newMsgID — "msg" prefix)
  - violations: not present

A.4 §S16 错误 wrap 格式:
  - violations: site #6 (LOW — code translation hand-rolled with manual switch over sentinels; duplicates errmap.go's truth source. Not strictly §S16 since this is the terminal hand-off to UI, not propagation up.)

A.5 §S17 sentinel 登记 errmap:
  - sentinels defined: none in runner.go
  - 已登记 errmap: N/A — runner.go consumes llmclientpkg.ErrPickModel + ErrResolveCreds (these are handled in-place at site #6, not propagated to handlers, so technically don't need errmap rows). However llmclientpkg sentinels' downstream consumers should still be checked separately.
  - missing: N/A — file defines no new sentinels
