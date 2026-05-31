# audit: backend/internal/app/chat/chat.go

LOC: 390
Read: full file (lines 1-390)

## Trace

| site# | file:line | snippet | category | classification | reasoning | severity | user_impact | suggested_fix | status |
|---|---|---|---|---|---|---|---|---|---|
| 1 | chat.go:132-134 | `if log == nil { panic("chat.NewService: logger is nil") }` | A.1 | OK | panic on nil-logger is wiring-time guard; §S3 example pattern | N-A | — | — | — |
| 2 | chat.go:138-140 | `if emitter == nil { emitter = eventlogpkg.From(context.Background()) }` | A.1 | OK | nil-tolerant fallback to no-op Emitter (documented at lines 109-120 — "Either can be nil → no-op fallback used by tests"). Not silent fallback per §S3 — this is graceful degradation when emitter not wired (test ctx); production main.go always wires it. | N-A | — | — | — |
| 3 | chat.go:141-143 | `if notifications == nil { notifications = notificationspkg.From(context.Background()) }` | A.1 | OK | same pattern as site #2 — documented nil-tolerant fallback for non-SSE tests | N-A | — | — | — |
| 4 | chat.go:218-221 | `if int64(len(fileBytes)) > chatdomain.MaxAttachmentBytes { return nil, chatdomain.ErrAttachmentTooLarge }` | A.4 | OK | direct sentinel return at innermost validation site; sentinel registered errmap.go:62 | N-A | — | — | — |
| 5 | chat.go:222-225 | `uid, err := reqctxpkg.RequireUserID(ctx); if err != nil { return nil, fmt.Errorf("chat.Service.UploadAttachment: %w", err) }` | A.4 | OK | §S16 canonical: `<pkg>.<Method>:` prefix + `%w`; sentinel `reqctxpkg.ErrMissingUserID` registered errmap.go:163 | N-A | — | — | — |
| 6 | chat.go:227 | `id := newAttachmentID()` (calls `idgenpkg.New("att")` per util.go) | A.3 | OK | §S15: prefix "att" matches spec list ("att_" attachment); idgenpkg internally panics on rand fail | N-A | — | — | — |
| 7 | chat.go:232-234 | `if err := os.MkdirAll(storageDir, 0750); err != nil { return nil, fmt.Errorf("chat.Service.UploadAttachment: mkdir: %w", err) }` | A.4 | OK | §S16 canonical with sub-tag "mkdir:" | N-A | — | — | — |
| 8 | chat.go:235-237 | `if err := os.WriteFile(storagePath, fileBytes, 0640); err != nil { return nil, fmt.Errorf("chat.Service.UploadAttachment: write: %w", err) }` | A.4 | OK | §S16 canonical with "write:" sub-tag | N-A | — | — | — |
| 9 | chat.go:247-253 | `if err := s.repo.SaveAttachment(ctx, a); err != nil { if cleanErr := os.RemoveAll(...); cleanErr != nil { s.log.Warn(..., zap.Error(cleanErr)) } return nil, err }` | A.4 | EDGE | bare `return nil, err` — inconsistent with sites 5/7/8 which wrap. Sentinel preserved either way (repo.SaveAttachment wraps internally). Cleanup err properly captured to log.Warn ✓ — that part is correct. | LOW | identical UX (errmap matches inner sentinel either way); breaks §S16 wrap consistency within UploadAttachment | wrap: `return nil, fmt.Errorf("chat.Service.UploadAttachment: %w", err)` | **FIXED 2026-05-09 f272503** |
| 10 | chat.go:248-251 | `if cleanErr := os.RemoveAll(storageDir); cleanErr != nil { s.log.Warn("failed to clean up attachment directory after save error", zap.String("dir", storageDir), zap.Error(cleanErr)) }` | A.1 | OK | cleanup error properly logged at WARN with original context preserved (zap.Error). Per §S3 example "if only resource cleanup fails and doesn't impact business, swallowing OK". | N-A | — | — | — |
| 11 | chat.go:273-276 | `conv, err := s.convRepo.Get(ctx, conversationID); if err != nil { return "", err }` | A.4 | EDGE | bare-return — inconsistent with site #12 which wraps. Sentinel preserved (convRepo.Get wraps internally). | LOW | identical UX; breaks §S16 wrap consistency within Send | wrap: `return "", fmt.Errorf("chat.Service.Send: %w", err)` | **FIXED 2026-05-09 f272503** |
| 12 | chat.go:277-280 | `uid, err := reqctxpkg.RequireUserID(ctx); if err != nil { return "", fmt.Errorf("chat.Service.Send: %w", err) }` | A.4 | OK | §S16 canonical | N-A | — | — | — |
| 13 | chat.go:287-291 | `att, err := s.repo.GetAttachment(ctx, attID); if err != nil { return "", fmt.Errorf("chat.Service.Send: attachment %q: %w", attID, err) }` | A.4 | OK | §S16 canonical with attachment ID context | N-A | — | — | — |
| 14 | chat.go:298-303 | `if b, err := json.Marshal(attrs); err == nil { attrsJSON = string(b) }` (no else branch — error silently dropped, attrsJSON stays empty) | **A.1** | **VIOLATION** | **§S3 silent error**: json.Marshal failure silently drops both the err and the attachments data; user's attachment refs are lost from Message.Attrs without any log/return path. Realistically attrs is a `map[string]any` containing `[]chatdomain.AttachmentRef` which always serializes cleanly — but the silent drop is what §S3 explicitly forbids ("严禁用'静默跳过'掩盖失败"). The error is **not even logged**. | MED | if json.Marshal ever fails (e.g. NaN/Inf in some future field), user-uploaded attachments silently disappear from the saved message — the message saves but attrs is empty. UI shows message text without the attached files. No log audit trail. | minimum: log at WARN with err. Better: `if b, err := json.Marshal(attrs); err != nil { return "", fmt.Errorf("chat.Service.Send: marshal attrs: %w", err) } else { attrsJSON = string(b) }` — let caller see the bug | **FIXED 2026-05-09 f272503** |
| 15 | chat.go:309-314 | `blocks = append(blocks, chatdomain.Block{ ID: newBlockID(), ... })` | A.3 | OK | §S15: newBlockID → idgenpkg.New("blk") per util.go; "blk" matches spec | N-A | — | — | — |
| 16 | chat.go:317 | `msgID := newMsgID()` | A.3 | OK | §S15: idgenpkg.New("msg"); "msg" matches spec | N-A | — | — | — |
| 17 | chat.go:327-329 | `if err := s.repo.SaveMessage(ctx, userMsg); err != nil { return "", err }` | A.4 | EDGE | bare-return — same inconsistency as site #11. Note: this is the user message persist (synchronous in Send) — failure means user's message never reached DB. **However**, ctx here is the request ctx, not a "terminal write after agent run" — if request is cancelled before this Save, the user wouldn't be waiting for a response anyway. Not §S9. | LOW | identical UX with wrap inconsistency | wrap: `return "", fmt.Errorf("chat.Service.Send: %w", err)` | **FIXED 2026-05-09 f272503** |
| 18 | chat.go:337-339 | `emitCtx := reqctxpkg.WithConversationID(ctx, conversationID); emitCtx = eventlogpkg.With(emitCtx, s.emitter); s.emitUserMessage(emitCtx, userMsg)` | A.1 | OK | doc comment at lines 162-170 explicitly designates emitUserMessage as "best-effort" (any failure logs and continues). Inside emitUserMessage (lines 171-182), all em.* calls are fire-and-forget — emitter implementation handles its own logging (per pkg/eventlog spec). User message already saved at site #17 — emit is observability sidecar, not terminal-state. | N-A | — | — | — |
| 19 | chat.go:341-342 | `agentCtx := reqctxpkg.SetUserID(context.Background(), uid); agentCtx = reqctxpkg.SetLocale(agentCtx, reqctxpkg.GetLocale(ctx))` | **A.2** | **OK (POSITIVE EXAMPLE)** | **§S9 textbook compliance**: agent run is detached from request lifetime — cancelation of the HTTP request must NOT kill the in-flight agent. This is the same `reqctxpkg.SetUserID(context.Background(), uid)` pattern that §S9 spec example calls out for terminal writes (writeAndPublish(fatal=true) downstream uses this ctx). Documented in chat.md §6 lines 358 + 1011: "终态写 detached context — writeAndPublish(fatal=true) 用 context.Background() 用户取消时 ctx 已 cancelled，但终态消息必须落库". | N-A | this is the canonical pattern — preserve as reference | — | — |
| 20 | chat.go:346-350 | `select { case q.ch <- task: default: return "", chatdomain.ErrStreamInProgress }` | A.4 | OK | direct sentinel return when channel full; ErrStreamInProgress registered errmap.go:60 (409 STREAM_IN_PROGRESS) | N-A | — | — | — |
| 21 | chat.go:363-367 | `func Cancel(...) error { v, ok := s.queues.Load(...); if !ok { return chatdomain.ErrStreamNotFound } }` | A.4 | OK | direct sentinel; ErrStreamNotFound registered errmap.go:59 | N-A | — | — | — |
| 22 | chat.go:372-374 | `if cancel == nil { return chatdomain.ErrStreamNotFound }` | A.4 | OK | direct sentinel | N-A | — | — | — |
| 23 | chat.go:375-382 | `cancel(); for { select { case <-q.ch: default: return nil } }` | A.1 | OK | drain loop; no error path; intentional discard of in-flight tasks (Cancel semantic: stop and clear) | N-A | — | — | — |
| 24 | chat.go:388-390 | `func ListMessages(...) ... { return s.repo.ListMessagesByConversation(...) }` | A.4 | OK | direct passthrough; repo wraps | N-A | — | — | — |

## Sub-check

A.1 §S3 错误吞没:
  - violations: site #14 (json.Marshal silent drop in Send — attachments data silently lost on err)

A.2 §S9 detached ctx 终态写:
  - terminal-state writes identified: none directly in chat.go (terminal writes happen in runner.go via writeAndPublish(fatal=true)). The `agentCtx` derived at site #19 is the **detached ctx that downstream writeAndPublish uses** — chat.go is correctly **setting up** detached ctx for the entire agent run, and runner.go inherits it.
  - 各自 ctx 来源: site #17 SaveMessage uses request ctx (correct — user message must wait for HTTP request anyway); site #19 derives detached ctx from `context.Background()` for the agent run handed to runner.go via the queued task
  - violations: not present in chat.go (need to verify runner.go uses agentCtx for terminal writes — that's runner.go's audit scope)

A.3 §S15 ID 生成:
  - ID generation calls: sites #6 (newAttachmentID — "att"), #15 (newBlockID — "blk"), #16 (newMsgID — "msg")
  - violations: not present (all delegate to idgenpkg.New with §S15-compliant prefixes)

A.4 §S16 错误 wrap 格式:
  - violations: sites #9, #11, #17 (LOW — bare `return nil, err` / `return "", err` instead of canonical wrap; sentinel preserved so functional behavior unchanged but inconsistent with sites #5/#7/#8/#12/#13 within same functions)

A.5 §S17 sentinel 登记 errmap:
  - sentinels defined: none (chat.go uses sentinels from chatdomain — definitions in domain/chat/, not this file)
  - 已登记 errmap: chatdomain.ErrAttachmentTooLarge (errmap.go:62), chatdomain.ErrStreamInProgress (errmap.go:60), chatdomain.ErrStreamNotFound (errmap.go:59), reqctxpkg.ErrMissingUserID (errmap.go:163), forgedomain./convdomain. sentinels via repo wraps registered elsewhere
  - missing: N/A — file defines no new sentinels
