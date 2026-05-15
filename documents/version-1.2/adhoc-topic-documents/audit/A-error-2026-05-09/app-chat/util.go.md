# audit: backend/internal/app/chat/util.go

LOC: 35
Read: full file (lines 1-35)

## Trace

| site# | file:line | snippet | category | classification | reasoning | severity | user_impact | suggested_fix | status |
|---|---|---|---|---|---|---|---|---|---|
| 1 | util.go:11-13 | `func newMsgID() string { return idgenpkg.New("msg") }` + `newBlockID()` ("blk") + `newAttachmentID()` ("att") | A.3 | OK | §S15 canonical: uses `idgenpkg.New(prefix)` — prefixes "msg", "blk", "att" all match §S15 spec list ("msg_" message / "blk_" block / "att_" attachment). idgenpkg internally panics on rand.Read fail per §S15. | N-A | — | — | — |
| 2 | util.go:30 | `data, err := os.ReadFile(path); if err != nil { return "", fmt.Errorf("readAndEncode: %w", err) }` | A.4 | EDGE | §S16: has `%w` ✓; prefix is `readAndEncode:` (function name) but NOT `chat.readAndEncode:` (no pkg.func form). Borderline — §S16 spec says `<pkg>.<Method>:` literal; for an unexported file-level helper this is a soft target. | LOW | grep traceability slightly weaker — `readAndEncode:` could be ambiguous if another package has a same-named helper | could be `fmt.Errorf("chat.readAndEncode: %w", err)` for spec literal compliance | **FIXED 2026-05-09 f272503** |

## Sub-check

A.1 §S3 错误吞没:
  - violations: not present

A.2 §S9 detached ctx 终态写:
  - terminal-state writes identified: none
  - 各自 ctx 来源: N/A
  - violations: N/A — file is pure utility (ID gen, base64 encode, truncate, file read); no DB writes, no terminal-state operations

A.3 §S15 ID 生成:
  - ID generation calls: site #1 (newMsgID/newBlockID/newAttachmentID all delegate to idgenpkg.New)
  - violations: not present

A.4 §S16 错误 wrap 格式:
  - violations: site #2 (LOW — `readAndEncode:` prefix, missing pkg qualifier; could be tightened)

A.5 §S17 sentinel 登记 errmap:
  - sentinels defined: none
  - 已登记 errmap: N/A
  - missing: N/A — file defines no sentinels (only ID-gen + utility helpers)
