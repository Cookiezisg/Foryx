# audit: backend/internal/app/chat/history.go

LOC: 172
Read: full file (lines 1-172)

## Trace

| site# | file:line | snippet | category | classification | reasoning | severity | user_impact | suggested_fix | status |
|---|---|---|---|---|---|---|---|---|---|
| 1 | history.go:24-27 | `// maxHistoryMessages ... 超出的旧消息静默丢弃。` const = 200 | A.1 | OK | documented intent — history truncation by count is a deliberate context-window strategy (not a §S3 silent failure). Not an error path. | N-A | — | — | — |
| 2 | history.go:38-41 | `rows, _, err := s.repo.ListMessagesByConversation(...); if err != nil { return nil, err }` | A.4 | EDGE | bare-return — no `chat.Service.buildHistory:` wrap. Sentinel preserved (repo wraps). | LOW | identical UX with wrap inconsistency vs site #3 | wrap: `return nil, fmt.Errorf("chat.Service.buildHistory: %w", err)` | **FIXED 2026-05-09 f272503** |
| 3 | history.go:54-57 | `msgs, err := s.blocksToLLM(ctx, m); if err != nil { return nil, fmt.Errorf("buildHistory: message %q: %w", m.ID, err) }` | A.4 | EDGE | §S16: has %w ✓; prefix is `buildHistory:` (function name) but missing pkg/Type qualifier — should be `chat.Service.buildHistory:`. Helper-style prefix common but spec literal `<pkg>.<Method>:` form preferred. | LOW | grep traceability slightly weaker | tighten to `chat.Service.buildHistory:` | **FIXED 2026-05-09 f272503** |
| 4 | history.go:62-65 | `msg, err := s.buildUserLLMMessage(ctx, currentUserMsg); if err != nil { return nil, fmt.Errorf("buildHistory: current user msg %q: %w", currentUserMsgID, err) }` | A.4 | EDGE | same prefix issue as site #3 | LOW | same | same | **FIXED 2026-05-09 f272503** |
| 5 | history.go:46-49 | `if m.Status == StatusStreaming || m.Status == StatusPending { continue }` | A.1 | OK | intentional skip per chat.md §6 line 357 — "buildHistory 跳过 streaming/pending 状态的消息，避免把未完成的步骤放进历史重建". Documented invariant, not a §S3 silent fail. | N-A | — | — | — |
| 6 | history.go:50-53 | `if m.ID == currentUserMsgID { currentUserMsg = m; continue }` | A.1 | OK | deliberate hold-out to append last; prevents fast-send race. Documented at lines 30-36. | N-A | — | — | — |
| 7 | history.go:80-83 | `msg, err := s.buildUserLLMMessage(ctx, m); if err != nil { return nil, err }` | A.4 | EDGE | bare-return; sentinel preserved | LOW | identical UX | wrap: `return nil, fmt.Errorf("chat.Service.blocksToLLM: %w", err)` | **FIXED 2026-05-09 f272503** |
| 8 | history.go:85-87 | `case chatdomain.RoleAssistant: return loopapp.BlocksToAssistantLLM(m.Blocks)` | A.4 | OK | direct passthrough to loop.BlocksToAssistantLLM (single source of truth per file header doc) | N-A | — | — | — |
| 9 | history.go:88 | default: `return nil, nil` (no role match) | **A.1** | **EDGE** | **§S3 silent path**: a Message with Role neither User nor Assistant (e.g. system / tool / unknown) returns `nil, nil` — no error, no log. Per chat.md the only roles persisted are User / Assistant; a stray role would indicate corrupted DB or schema drift. Currently any such message would be **silently dropped from history without trace** — buildHistory's caller can't distinguish "no messages" from "unknown role messages skipped". §S3 spec calls out this exact pattern (FTS5 example). | LOW | only triggers if DB has corrupted role or future role added without history.go update — developer bug. Symptom: LLM history missing those messages without explanation. | log at WARN: `s.log.Warn("blocksToLLM: skipping message with unknown role", zap.String("role", string(m.Role)), zap.String("msg_id", m.ID))`; or return error sentinel. | **FIXED 2026-05-09 f272503** |
| 10 | history.go:122-131 | `if err := json.Unmarshal([]byte(m.Attrs), &attrs); err == nil { for _, ref := range attrs.Attachments { ... } }` (no else branch on json.Unmarshal err) | **A.1** | **EDGE** | **§S3 silent**: if Attrs JSON is malformed (corrupted DB / schema drift), all attachments silently disappear from LLM context. The user sees text without their attached files when the LLM responds. No log, no return. Behavior is "treat as no attachments" but **without** observability — same pattern as chat.go:#14 site. | MED | corrupted Attrs JSON in any persisted message → user re-asks something referencing their attached PDF and the LLM has no idea what they're talking about. No way to diagnose from logs. | minimum: `else { s.log.Warn("buildUserLLMMessage: malformed Attrs JSON", zap.String("msg_id", m.ID), zap.Error(err)) }`; better: include the Attrs string in the log so it can be fixed | **FIXED 2026-05-09 f272503** |
| 11 | history.go:124-128 | `part, err := s.attachmentToPart(ctx, ref); if err != nil { s.log.Warn("skipping attachment in LLM history", zap.Error(err)); continue }` | A.1 | OK | documented soft-failure design at lines 95-100: "附件失败属于软失败：记录后跳过". Logged at WARN (proper observability) before skip — this is §S3-compliant graceful degrade with audit trail, not silent. | N-A | — | — | — |
| 12 | history.go:147-151 | `att, err := s.repo.GetAttachment(ctx, ref.AttachmentID); if err != nil { return nil, fmt.Errorf("attachmentToPart: get attachment %q: %w", ref.AttachmentID, err) }` | A.4 | EDGE | §S16: has %w ✓; prefix `attachmentToPart:` is helper-style (function only); spec literal `chat.Service.attachmentToPart:` preferred | LOW | grep traceability | tighten prefix | **FIXED 2026-05-09 f272503** |
| 13 | history.go:153-157 | `data, err := readAndEncode(att.StoragePath); if err != nil { return nil, fmt.Errorf("attachmentToPart: encode image %q: %w", att.ID, err) }` | A.4 | EDGE | same prefix issue as site #12 | LOW | same | same | **FIXED 2026-05-09 f272503** |
| 14 | history.go:164-167 | `text, err := chatinfra.Extract(att.StoragePath, att.MimeType); if err != nil { return nil, fmt.Errorf("attachmentToPart: extract %q: %w", att.ID, err) }` | A.4 | EDGE | same prefix issue | LOW | same | same | **FIXED 2026-05-09 f272503** |

## Sub-check

A.1 §S3 错误吞没:
  - violations: site #9 (unknown role silent drop, no log), #10 (json.Unmarshal silent drop on malformed Attrs JSON, no log)

A.2 §S9 detached ctx 终态写:
  - terminal-state writes identified: none — buildHistory / blocksToLLM / buildUserLLMMessage / attachmentToPart are all read-only data shaping
  - 各自 ctx 来源: caller's ctx (read paths)
  - violations: N/A — no terminal writes in this file

A.3 §S15 ID 生成:
  - ID generation calls: none in this file
  - violations: N/A — file only reads + transforms; no business ID generation

A.4 §S16 错误 wrap 格式:
  - violations: sites #2 (bare-return), #3, #4, #12, #13, #14 (helper-style prefix `attachmentToPart:` / `buildHistory:` instead of canonical `chat.Service.buildHistory:` / `chat.Service.attachmentToPart:`); #7 (bare-return). All preserve sentinel chain so functional behavior unchanged. All LOW per §S16 spec literal compliance.

A.5 §S17 sentinel 登记 errmap:
  - sentinels defined: none
  - 已登记 errmap: N/A — file uses sentinels from chatdomain (registered at errmap.go:58-66)
  - missing: N/A — file defines no new sentinels
